// SPDX-License-Identifier: GPL-3.0-or-later

package tmdb

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// BaseURL is TMDB's API, version 3.
const BaseURL = "https://api.themoviedb.org/3"

// maxResponseBytes bounds a response body: a film's details with its credits are tens of
// kilobytes, so anything near this is not TMDB.
const maxResponseBytes = 8 << 20

// Keys opens the TMDB key; secrets.Store is one.
type Keys interface {
	Open(ctx context.Context, name secrets.Name) (secrets.Value, error)
}

// Client is TMDB behind the provider interface.
type Client struct {
	http    *http.Client
	baseURL string
	keys    Keys
}

var _ identify.Provider = (*Client)(nil)

// New returns a TMDB client calling baseURL (BaseURL, or a test's fake) through httpClient, which
// comes from internal/platform/outbound.
func New(httpClient *http.Client, baseURL string, keys Keys) *Client {
	return &Client{http: httpClient, baseURL: strings.TrimSuffix(baseURL, "/"), keys: keys}
}

// Name is "tmdb".
func (*Client) Name() string { return "tmdb" }

// get calls one endpoint with the key and returns the body of a 200.
func (client *Client) get(ctx context.Context, path string, parameters url.Values) ([]byte, error) {
	key, err := client.keys.Open(ctx, secrets.TMDBAPIKey)
	if errors.Is(err, secrets.ErrNotSet) || (err == nil && key.IsZero()) {
		return nil, fmt.Errorf("tmdb: %w", identify.ErrNoKey)
	}
	if err != nil {
		return nil, fmt.Errorf("tmdb: open the key: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, http.NoBody)
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
	body, err := bounded.ReadAll(response.Body, maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("tmdb %s: the response: %w", path, err)
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
		if after, found := outbound.RetryAfter(response.Header); found {
			return fmt.Errorf("tmdb %s: %w", path, &identify.RateLimitedError{RetryAfter: after})
		}
	}
	excerpt := string(body[:min(len(body), 200)])
	return fmt.Errorf("tmdb %s: status %d: %s", path, response.StatusCode, excerpt)
}
