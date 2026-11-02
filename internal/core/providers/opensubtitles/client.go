// SPDX-License-Identifier: AGPL-3.0-or-later

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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
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

// providerName is OpenSubtitles as a person knows it, in a rate limit's error.
const providerName = "OpenSubtitles"

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

// Service is OpenSubtitles as a provider service: its destination, the rate we keep to with it
// (opensubtitles_requests_per_second), its day of downloads with a percent of them held back for
// people (opensubtitles_downloads_reserved_for_people_percent), and its login's state.
func Service(
	health providers.Health, clockSource clock.Clock, settingsReader providers.Settings,
) *providers.Service {
	return providers.New(providers.Config{
		Name: providerName, Destination: jobs.DestinationOpenSubtitles,
		Limits: providers.SettingsLimits(settingsReader, settings.OpenSubtitlesRequestsPerSecond, "",
			settings.OpenSubtitlesDownloadsReservedForPeoplePercent),
		CredentialsName: "login",
		CredentialsFailure: func(err error) providers.CredentialsState {
			switch {
			case errors.Is(err, ErrNoLogin):
				return providers.CredentialsMissing
			case errors.Is(err, ErrLoginRefused):
				return providers.CredentialsRefused
			}
			return ""
		},
	}, health, clockSource)
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
	// provider is OpenSubtitles' provider service (Service), asked before every request.
	provider *providers.Service

	mutex sync.Mutex
	// session is the current login, empty before the first (guarded by mutex).
	session session
	// refusedLoginFingerprint is the fingerprint of a login OpenSubtitles refused, which is not
	// tried again (guarded by mutex).
	refusedLoginFingerprint [sha256.Size]byte
	// linkByProviderFileID holds each download's link while it works, so fetching the same file
	// again spends no second download (guarded by mutex).
	linkByProviderFileID map[int64]heldLink
	// linkRetryDelays are the waits before a link that failed as unavailable is fetched again
	// within one Download: a 502, 503 or 504 is retried after a second, as OpenSubtitles asks
	// (opensubtitles-api.md §3), and once more, after two.
	linkRetryDelays []time.Duration
}

// session is one login: its token, where its requests go and until when it is used.
type session struct {
	token     string
	baseURL   string
	usedUntil time.Time
}

// New returns a client calling base (BaseURL, or a test's fake) through httpClient, which comes
// from internal/platform/outbound, asking provider (Service) before every request.
func New(
	httpClient *http.Client, base string, loginSecrets LoginSecrets, clockSource clock.Clock,
	provider *providers.Service,
) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSuffix(base, "/"))
	if err != nil {
		return nil, fmt.Errorf("opensubtitles: the base URL: %w", err)
	}
	return &Client{
		http: httpClient, base: parsed, loginSecrets: loginSecrets, clock: clockSource, provider: provider,
		linkByProviderFileID: map[int64]heldLink{}, linkRetryDelays: []time.Duration{time.Second, 2 * time.Second},
	}, nil
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
	login := apiRequest{cost: providers.CostRequest, method: http.MethodPost, body: body}
	err = client.call(ctx, login, client.base.String()+"/login", "", &answer)
	if errors.Is(err, errUnauthorized) || errors.Is(err, errBadRequest) {
		client.refusedLoginFingerprint = fingerprint
		return session{}, ErrLoginRefused
	}
	if err != nil {
		return session{}, err
	}
	if answer.Token == "" {
		return session{}, errors.New("opensubtitles: a login answered with no token")
	}
	// The key and the token go wherever the login says, so it is checked before anything is sent
	// there (go.md §21): OpenSubtitles' own host, or the base's; anything else keeps the base.
	sessionBaseURL := client.base.String()
	if host := strings.TrimSpace(answer.BaseURL); client.isOwnHost(host) {
		sessionBaseURL = client.base.Scheme + "://" + host + client.base.Path
	}
	client.session = session{token: answer.Token, baseURL: sessionBaseURL, usedUntil: client.clock.Now().Add(tokenLife)}
	if answer.User.AllowedDownloads > 0 {
		client.provider.RecordAllowance(answer.User.AllowedDownloads)
	}
	return client.session, nil
}

// ownDomain is OpenSubtitles' domain: its API, its VIP API and its download links are under it.
const ownDomain = "opensubtitles.com"

// hostNamePattern is a host name alone: no port, no user, nothing else.
var hostNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// isOwnHost says whether host, as a login or a download link names it, is OpenSubtitles' own:
// opensubtitles.com or a name under it, or the host of the base the client was given.
func (client *Client) isOwnHost(host string) bool {
	host = strings.ToLower(host)
	if host == "" {
		return false
	}
	if host == strings.ToLower(client.base.Host) {
		return true
	}
	return hostNamePattern.MatchString(host) && (host == ownDomain || strings.HasSuffix(host, "."+ownDomain))
}

// loginAnswer is what a login answers: the token, and the host the account's requests go to.
//
//nolint:tagliatelle // OpenSubtitles' field names
type loginAnswer struct {
	Token   string `json:"token"`
	BaseURL string `json:"base_url"`
	User    struct {
		AllowedDownloads int `json:"allowed_downloads"`
	} `json:"user"`
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

// apiRequest is one request to OpenSubtitles' API, as a caller asks it: what it costs of the
// account (CostCounted for a download), its method, its path under the session's base, its query
// and its JSON body.
type apiRequest struct {
	cost   providers.Cost
	method string
	path   string
	query  url.Values
	body   []byte
}

// callAuthorised makes one request with the current login, and logs in again once if OpenSubtitles
// no longer accepts it; a token answered invalid again is OpenSubtitles failing.
func (client *Client) callAuthorised(ctx context.Context, request apiRequest, into any) error {
	for attempt := 0; ; attempt++ {
		current, err := client.currentSession(ctx)
		if err != nil {
			return err
		}
		address := current.baseURL + request.path
		if len(request.query) > 0 {
			// Sorted and lower-case, as OpenSubtitles asks, so no redirect is needed (best-practices).
			address += "?" + request.query.Encode()
		}
		err = client.call(ctx, request, address, current.token, into)
		isStale := errors.Is(err, errUnauthorized) || errors.Is(err, errTokenInvalid)
		switch {
		case isStale && attempt > 0 && errors.Is(err, errTokenInvalid):
			return fmt.Errorf("%w: %w", outbound.ErrUnavailable, err)
		case !isStale || attempt > 0:
			return err
		}
		client.forget(current)
	}
}

// errUnauthorized is a 401: a login refused, or a token no longer accepted.
var errUnauthorized = errors.New("opensubtitles: unauthorized")

// errBadRequest is a 400. To a login it is a refused one: OpenSubtitles answers a wrong username
// or password, or an email given as the username, with 400 "invalid username/password", not 401.
var errBadRequest = errors.New("opensubtitles: bad request")

// errTokenInvalid is a 500 whose answer says "invalid" to a request with a token: how
// OpenSubtitles answers a token that expired or that it dropped, which a new login cures
// (opensubtitles-api.md §3). Once a new login has not cured it, OpenSubtitles is failing
// (callAuthorised); the first says nothing of the host.
var errTokenInvalid = errors.New("opensubtitles: the token was answered invalid")

// call makes one request to address, with token when there is one, once OpenSubtitles' provider
// says it may go, and decodes a 200's JSON answer into into.
func (client *Client) call(ctx context.Context, request apiRequest, address, token string, into any) error {
	//nolint:wrapcheck // the request's own error, or the provider's refusal, each naming OpenSubtitles
	return client.provider.Call(ctx, request.cost, func(ctx context.Context) error {
		return client.exchange(ctx, request.method, address, token, request.body, into)
	})
}

// exchange makes one request and decodes a 200's JSON answer into into.
func (client *Client) exchange(ctx context.Context, method, address, token string, body []byte, into any) error {
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
	answer, err := outbound.Exchange(client.http, request, maxResponseBytes)
	if err != nil {
		return fmt.Errorf("opensubtitles: %w", err)
	}
	if token != "" && answer.StatusCode == http.StatusInternalServerError &&
		bytes.Contains(bytes.ToLower(answer.Body), []byte("invalid")) {
		return errTokenInvalid
	}
	if err := statusError(answer); err != nil {
		return err
	}
	if err := json.Unmarshal(answer.Body, into); err != nil {
		return fmt.Errorf("opensubtitles: read the answer: %w", err)
	}
	return nil
}

// statusError turns an answer that is not 200 into the errors a caller acts on: OpenSubtitles'
// own, and then every provider's (outbound.StatusError).
func statusError(answer outbound.Answer) error {
	switch answer.StatusCode {
	case http.StatusUnauthorized:
		return errUnauthorized
	case http.StatusNotAcceptable:
		var spent downloadAnswer
		_ = json.Unmarshal(answer.Body, &spent) //nolint:errcheck // a quota without a time is still spent
		return &providers.QuotaSpentError{ResetAt: spent.resetAt()}
	}
	err := outbound.StatusError(providerName, answer)
	if err != nil && answer.StatusCode == http.StatusBadRequest {
		return fmt.Errorf("%w: %w", errBadRequest, err)
	}
	if err != nil {
		return fmt.Errorf("opensubtitles: %w", err)
	}
	return nil
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
