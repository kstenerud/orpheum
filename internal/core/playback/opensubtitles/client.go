// SPDX-License-Identifier: GPL-3.0-or-later

package opensubtitles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// BaseURL is OpenSubtitles' REST API. A login answers with the host its account's requests go to
// next, whose scheme and path are this one's.
const BaseURL = "https://api.opensubtitles.com/api/v1"

// apiKey is Orpheum's key, as OpenSubtitles registered the application. It ships with the
// program, as every published client's does, and no user may enter another (D224 §2, §7).
//
//nolint:gosec // G101: an application's published key, not a credential (D224 §7)
const apiKey = "MzU7F707YIW4oNYUI50kHT8yR6zRtFgH"

// maxResponseBytes bounds an answer: a page of search results is tens of kilobytes.
const maxResponseBytes = 4 << 20

// MaxSubtitleBytes bounds a fetched subtitle's text, as downloaded and once unpacked: a film's is
// at most a few hundred kilobytes.
const MaxSubtitleBytes = 8 << 20

// tokenLife is how long a login is used: OpenSubtitles' tokens last 24 hours, and one is not
// used into its last hour (opensubtitles-api.md §3).
const tokenLife = 23 * time.Hour

// The errors a caller says in its own words.
var (
	// ErrNoLogin is a server whose admin has not entered an OpenSubtitles login.
	ErrNoLogin = errors.New("no OpenSubtitles login is set")
	// ErrLoginRefused is a login OpenSubtitles refused. It is not tried again until the admin
	// changes it: a login is rationed, and a wrong one is not retried (opensubtitles-api.md §3).
	ErrLoginRefused = errors.New("OpenSubtitles refused the login")
)

// QuotaSpentError is an account whose downloads for the day are spent.
type QuotaSpentError struct {
	// ResetAt is when OpenSubtitles says the count starts again; zero when it did not say.
	ResetAt time.Time
}

func (spent *QuotaSpentError) Error() string {
	return "the OpenSubtitles account's downloads are spent until " + spent.ResetAt.Format(time.RFC3339)
}

// RateLimitedError is a request OpenSubtitles asked to be made again later.
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (limited *RateLimitedError) Error() string {
	return "OpenSubtitles asks to wait " + limited.RetryAfter.String()
}

// LoginSecrets opens the login; secrets.Store is one.
type LoginSecrets interface {
	Open(ctx context.Context, name secrets.Name) (secrets.Value, error)
}

// Client is OpenSubtitles, for fetching a subtitle on request (D224): one login, the server's,
// shared by every request for its day.
type Client struct {
	http         *http.Client
	base         *url.URL
	loginSecrets LoginSecrets
	clock        clock.Clock

	mutex sync.Mutex
	// session is the current login, empty before the first (guarded by mutex).
	session session
	// refusedLoginFingerprint is the fingerprint of a login OpenSubtitles refused, which is not
	// tried again (guarded by mutex).
	refusedLoginFingerprint [sha256.Size]byte
}

// session is one login: its token, where its requests go and until when it is used.
type session struct {
	token     string
	baseURL   string
	usedUntil time.Time
}

// New returns a client calling base (BaseURL, or a test's fake) through httpClient, which comes
// from internal/platform/outbound.
func New(httpClient *http.Client, base string, loginSecrets LoginSecrets, clockSource clock.Clock) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSuffix(base, "/"))
	if err != nil {
		return nil, fmt.Errorf("opensubtitles: the base URL: %w", err)
	}
	return &Client{http: httpClient, base: parsed, loginSecrets: loginSecrets, clock: clockSource}, nil
}

// currentSession is the session to send a request with: the one in use, or a new login.
func (client *Client) currentSession(ctx context.Context) (session, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if client.session.token != "" && client.clock.Now().Before(client.session.usedUntil) {
		return client.session, nil
	}
	username, password, err := client.credentials(ctx)
	if err != nil {
		return session{}, err
	}
	fingerprint := sha256.Sum256([]byte(username.Reveal() + "\x00" + password.Reveal()))
	if fingerprint == client.refusedLoginFingerprint {
		return session{}, ErrLoginRefused
	}
	body, err := json.Marshal(map[string]string{"username": username.Reveal(), "password": password.Reveal()})
	if err != nil {
		return session{}, fmt.Errorf("opensubtitles: encode the login: %w", err)
	}
	var answer loginAnswer
	err = client.call(ctx, http.MethodPost, client.base.String()+"/login", "", body, &answer)
	if errors.Is(err, errUnauthorized) {
		client.refusedLoginFingerprint = fingerprint
		return session{}, ErrLoginRefused
	}
	if err != nil {
		return session{}, err
	}
	if answer.Token == "" {
		return session{}, errors.New("opensubtitles: a login answered with no token")
	}
	sessionBaseURL := client.base.String()
	if host := strings.TrimSpace(answer.BaseURL); host != "" && !strings.ContainsAny(host, "/?#@") {
		sessionBaseURL = client.base.Scheme + "://" + host + client.base.Path
	}
	client.session = session{token: answer.Token, baseURL: sessionBaseURL, usedUntil: client.clock.Now().Add(tokenLife)}
	return client.session, nil
}

// loginAnswer is what a login answers: the token, and the host the account's requests go to.
//
//nolint:tagliatelle // OpenSubtitles' field names
type loginAnswer struct {
	Token   string `json:"token"`
	BaseURL string `json:"base_url"`
}

// credentials are the server's OpenSubtitles login (D224 §1).
func (client *Client) credentials(ctx context.Context) (username, password secrets.Value, err error) {
	username, err = client.loginSecrets.Open(ctx, secrets.OpenSubtitlesUsername)
	if err == nil {
		password, err = client.loginSecrets.Open(ctx, secrets.OpenSubtitlesPassword)
		if err == nil && !username.IsZero() && !password.IsZero() {
			return username, password, nil
		}
	}
	if err == nil || errors.Is(err, secrets.ErrNotSet) {
		return secrets.Value{}, secrets.Value{}, ErrNoLogin
	}
	return secrets.Value{}, secrets.Value{}, fmt.Errorf("opensubtitles: open the login: %w", err)
}

// forget drops a session OpenSubtitles no longer accepts, so the next request logs in again.
func (client *Client) forget(stale session) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if client.session.token == stale.token {
		client.session = session{}
	}
}

// callAuthorised makes one request with the current login, and logs in again once if OpenSubtitles
// no longer accepts it.
func (client *Client) callAuthorised(
	ctx context.Context, method, path string, query url.Values, body []byte, into any,
) error {
	for attempt := 0; ; attempt++ {
		current, err := client.currentSession(ctx)
		if err != nil {
			return err
		}
		address := current.baseURL + path
		if len(query) > 0 {
			// Sorted and lower-case, as OpenSubtitles asks, so no redirect is needed (best-practices).
			address += "?" + query.Encode()
		}
		err = client.call(ctx, method, address, current.token, body, into)
		if !errors.Is(err, errUnauthorized) || attempt > 0 {
			return err
		}
		client.forget(current)
	}
}

// errUnauthorized is a 401: a login refused, or a token no longer accepted.
var errUnauthorized = errors.New("opensubtitles: unauthorized")

// call makes one request and decodes a 200's JSON answer into into.
func (client *Client) call(ctx context.Context, method, address, token string, body []byte, into any) error {
	var reader io.Reader = http.NoBody
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, address, reader)
	if err != nil {
		return fmt.Errorf("opensubtitles: %w", err)
	}
	request.Header.Set("Api-Key", apiKey)
	request.Header.Set("Accept", "*/*")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("opensubtitles: %w", err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // a read-only body
	answer, err := bounded.ReadAll(response.Body, maxResponseBytes)
	if err != nil {
		return fmt.Errorf("opensubtitles: the answer: %w", err)
	}
	if err := statusError(response, answer); err != nil {
		return err
	}
	if err := json.Unmarshal(answer, into); err != nil {
		return fmt.Errorf("opensubtitles: read the answer: %w", err)
	}
	return nil
}

// statusError turns an answer that is not 200 into the errors a caller acts on.
func statusError(response *http.Response, answer []byte) error {
	switch response.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return errUnauthorized
	case http.StatusNotAcceptable:
		var spent downloadAnswer
		_ = json.Unmarshal(answer, &spent) //nolint:errcheck // a quota without a time is still spent
		return &QuotaSpentError{ResetAt: spent.resetAt()}
	case http.StatusTooManyRequests:
		after, found := outbound.RetryAfter(response.Header)
		if !found {
			after = time.Second
		}
		return &RateLimitedError{RetryAfter: after}
	}
	return fmt.Errorf("opensubtitles: status %d: %s", response.StatusCode, answer[:min(len(answer), 200)])
}

// numericID is an IMDb or TMDB id as OpenSubtitles takes it: without "tt" or leading zeros; ""
// when it is not one.
func numericID(id string) string {
	number, err := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(id)), "tt"), 10, 64)
	if err != nil || number <= 0 {
		return ""
	}
	return strconv.FormatInt(number, 10)
}
