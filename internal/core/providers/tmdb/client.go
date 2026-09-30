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
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// BaseURL is TMDB's API, version 3.
const BaseURL = "https://api.themoviedb.org/3"

// providerName is TMDB as a person knows it, in a rate limit's error.
const providerName = "TMDB"

// maxResponseBytes bounds a response body: a film's details with its credits are tens of
// kilobytes, so anything near this is not TMDB.
const maxResponseBytes = 8 << 20

// Keys opens the TMDB key; secrets.Store is one.
type Keys interface {
	Open(ctx context.Context, name secrets.Name) (secrets.Value, error)
}

// Client is TMDB behind the provider interface. Every call goes through TMDB's API provider
// service (APIService), which keeps to its rate and owns its health and its key's state.
type Client struct {
	http     *http.Client
	baseURL  string
	keys     Keys
	provider *providers.Service
}

var _ identify.Provider = (*Client)(nil)

// New returns a TMDB client calling baseURL (BaseURL, or a test's fake) through httpClient, which
// comes from internal/platform/outbound, asking provider (APIService) before each call.
func New(httpClient *http.Client, baseURL string, keys Keys, provider *providers.Service) *Client {
	return &Client{http: httpClient, baseURL: strings.TrimSuffix(baseURL, "/"), keys: keys, provider: provider}
}

// RatePerSecond is the request rate we keep to with TMDB, both its API and its image server:
// "somewhere in the 40 requests per second range", its developer documentation's upper limit
// (developer.themoviedb.org/docs/rate-limiting; provider-etiquette.md). The image server
// documents no request rate, only 20 connections at once, and we fetch one image at a time.
const RatePerSecond = 40

// BackgroundShare is the part of TMDB's rate background work may use, so a person's request
// always finds room (the owner, 2026-09-30; D231 §5).
const BackgroundShare = 0.8

// APIService is TMDB's API as a provider service: its destination tmdb-api, its rate, and its
// key, whose absence or refusal parks it until the key changes (D231 §5).
func APIService(health providers.Health, clockSource clock.Clock) *providers.Service {
	return providers.New(providers.Config{
		Name: providerName, Destination: jobs.DestinationTMDBAPI,
		Limits:          providers.Limits{RatePerSecond: RatePerSecond, BackgroundShare: BackgroundShare},
		CredentialsName: "key",
		CredentialsFailure: func(err error) providers.CredentialsState {
			switch {
			case errors.Is(err, identify.ErrNoKey):
				return providers.CredentialsMissing
			case errors.Is(err, identify.ErrKeyRefused):
				return providers.CredentialsRefused
			}
			return ""
		},
	}, health, clockSource)
}

// ImagesService is TMDB's image server as a provider service: its destination tmdb-images and its
// rate. It needs no key.
func ImagesService(health providers.Health, clockSource clock.Clock) *providers.Service {
	return providers.New(providers.Config{
		Name: "TMDB's image server", Destination: jobs.DestinationTMDBImages,
		Limits: providers.Limits{RatePerSecond: RatePerSecond, BackgroundShare: BackgroundShare},
	}, health, clockSource)
}

// Name is "tmdb".
func (*Client) Name() string { return "tmdb" }

// get calls one endpoint with the key and returns the body of a 200, once TMDB's provider says
// the call may go.
func (client *Client) get(ctx context.Context, path string, parameters url.Values) ([]byte, error) {
	var body []byte
	err := client.provider.Call(ctx, providers.CostRequest, func(ctx context.Context) error {
		var callErr error
		body, callErr = client.call(ctx, path, parameters)
		return callErr
	})
	return body, err //nolint:wrapcheck // the call's own error, or the provider's refusal, each naming TMDB
}

// call calls one endpoint with the key and returns the body of a 200.
func (client *Client) call(ctx context.Context, path string, parameters url.Values) ([]byte, error) {
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
	answer, err := outbound.Exchange(client.http, request, maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("tmdb %s: %w", path, err)
	}
	if answer.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("tmdb %s: %w", path, identify.ErrKeyRefused)
	}
	return answer.Body, statusError("tmdb "+path, answer, identify.ErrNoMatch)
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

// statusError turns a reply of TMDB's API or its image host that is not 200 into the errors its
// callers act on: 404 is notFound, the caller's word for "TMDB has no such thing"; the rest are
// every provider's (outbound.StatusError). The subject names what was asked, "tmdb /movie/1".
func statusError(subject string, answer outbound.Answer, notFound error) error {
	if answer.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s: %w", subject, notFound)
	}
	if err := outbound.StatusError(providerName, answer); err != nil {
		return fmt.Errorf("%s: %w", subject, err)
	}
	return nil
}
