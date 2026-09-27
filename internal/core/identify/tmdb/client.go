// SPDX-License-Identifier: GPL-3.0-or-later

package tmdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/secrets"
)

// BaseURL is TMDB's API, version 3.
const BaseURL = "https://api.themoviedb.org/3"

// maxResponseBytes bounds a response body: a film's details with its credits are tens of
// kilobytes, so anything near this is not TMDB.
const maxResponseBytes = 8 << 20

// Keys opens the TMDB key; secrets.Store is one.
type Keys interface {
	Open(ctx context.Context, name secrets.Name, owner *int64) (secrets.Value, error)
}

// Client is TMDB behind the provider interface.
type Client struct {
	http *http.Client
	base string
	keys Keys
}

var _ identify.Provider = (*Client)(nil)

// New returns a TMDB client calling base (BaseURL, or a test's fake) through httpClient, which
// comes from internal/platform/outbound.
func New(httpClient *http.Client, base string, keys Keys) *Client {
	return &Client{http: httpClient, base: strings.TrimSuffix(base, "/"), keys: keys}
}

// Name is "tmdb".
func (*Client) Name() string { return "tmdb" }

// get calls one endpoint with the key and returns the body of a 200.
func (client *Client) get(ctx context.Context, path string, parameters url.Values) ([]byte, error) {
	key, err := client.keys.Open(ctx, secrets.TMDBAPIKey, nil)
	if errors.Is(err, secrets.ErrNotSet) || (err == nil && key.IsZero()) {
		return nil, fmt.Errorf("tmdb: %w", identify.ErrNoKey)
	}
	if err != nil {
		return nil, fmt.Errorf("tmdb: open the key: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.base+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("tmdb %s: %w", path, err)
	}
	authorise(request, parameters, key)
	request.Header.Set("Accept", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		// url.Error repeats the URL, which may carry the key: keep only the cause.
		if urlErr, isURL := errors.AsType[*url.Error](err); isURL {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("tmdb %s: %w", path, err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // a read-only body
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("tmdb %s: read the response: %w", path, err)
	}
	// An answer past the bound is refused, never read cut short: a truncated answer can still
	// parse, as something other than what TMDB sent.
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("tmdb %s: the response is larger than %d bytes", path, maxResponseBytes)
	}
	return body, statusError(path, response, body)
}

// authorise presents the key: a v4 read-access token as a bearer token, a v3 key as api_key.
func authorise(request *http.Request, parameters url.Values, key secrets.Value) {
	text := key.Reveal()
	if strings.HasPrefix(text, "eyJ") {
		request.Header.Set("Authorization", "Bearer "+text)
	} else {
		parameters = cloneWith(parameters, "api_key", text)
	}
	request.URL.RawQuery = parameters.Encode()
}

func cloneWith(parameters url.Values, name, value string) url.Values {
	clone := url.Values{}
	maps.Copy(clone, parameters)
	clone.Set(name, value)
	return clone
}

// statusError turns a reply that is not 200 into the provider errors the matcher acts on.
func statusError(path string, response *http.Response, body []byte) error {
	switch response.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("tmdb %s: %w", path, identify.ErrNoMatch)
	case http.StatusUnauthorized:
		return fmt.Errorf("tmdb %s: %w", path, identify.ErrKeyRefused)
	case http.StatusTooManyRequests:
		if after, found := retryAfter(response.Header); found {
			return fmt.Errorf("tmdb %s: %w", path, &identify.RateLimitedError{After: after})
		}
	}
	excerpt := string(body[:min(len(body), 200)])
	return fmt.Errorf("tmdb %s: status %d: %s", path, response.StatusCode, excerpt)
}

// maxRetryAfter caps how long a Retry-After may hold us: a server that asks for a day is more
// likely wrong than meaning it (D213 §5).
const maxRetryAfter = time.Hour

// retryAfter reads a Retry-After in either form: seconds, or an HTTP date, measured against the
// reply's own Date so no clock of ours is needed. It is capped at an hour.
func retryAfter(header http.Header) (time.Duration, bool) {
	value := strings.TrimSpace(header.Get("Retry-After"))
	var after time.Duration
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		after = time.Duration(seconds) * time.Second
	} else {
		at, atErr := http.ParseTime(value)
		sent, sentErr := http.ParseTime(header.Get("Date"))
		if atErr != nil || sentErr != nil {
			return 0, false
		}
		after = max(at.Sub(sent), 0)
	}
	return min(after, maxRetryAfter), true
}

func yearOf(date string) int {
	year, err := strconv.Atoi(strings.SplitN(date, "-", 2)[0])
	if err != nil || year < 1 {
		return 0
	}
	return year
}
