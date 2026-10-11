// SPDX-License-Identifier: AGPL-3.0-or-later

package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// BaseURL is Cloudflare's API, version 4.
const BaseURL = "https://api.cloudflare.com/client/v4"

// providerName is Cloudflare as a person knows it, in an error.
const providerName = "Cloudflare"

// maxResponseBytes bounds an answer: a zone list or a record is a few kilobytes.
const maxResponseBytes = 1 << 20

// challengeTTLSeconds is the TXT record's time to live: Cloudflare's least, since the record lives
// only while the challenge is answered.
const challengeTTLSeconds = 60

var (
	// ErrTokenRefused is a token not set, or one Cloudflare refused: the admin sets or changes
	// dns_provider_token.
	ErrTokenRefused = errors.New("the DNS provider's token is not set, or Cloudflare refused it")
	// ErrNoZone is a name in no zone the token reaches.
	ErrNoZone = errors.New("no zone the DNS provider's token reaches holds this name")
	// ErrMalformed is an answer that is not Cloudflare's shape.
	ErrMalformed = errors.New("Cloudflare's answer was not what it sends") //nolint:staticcheck // ST1005: its name
)

// Keys opens the DNS provider's token; secrets.Store is one.
type Keys interface {
	Open(ctx context.Context, name secrets.Name) (secrets.Value, error)
}

// Client is Cloudflare's DNS API.
type Client struct {
	http    *http.Client
	baseURL string
	keys    Keys
}

// New returns a client calling baseURL (BaseURL, or a test's fake) through httpClient, which comes
// from internal/platform/outbound.
func New(httpClient *http.Client, baseURL string, keys Keys) *Client {
	return &Client{http: httpClient, baseURL: strings.TrimSuffix(baseURL, "/"), keys: keys}
}

// TXTRecord is a TXT record set, which RemoveTXT removes.
type TXTRecord struct {
	ZoneID   string
	RecordID string
}

// SetTXT sets a TXT record of name to value, DNS only, in the zone the token reaches that holds
// name.
func (client *Client) SetTXT(ctx context.Context, name, value string) (TXTRecord, error) {
	zoneID, err := client.zoneOf(ctx, name)
	if err != nil {
		return TXTRecord{}, err
	}
	body, err := json.Marshal(map[string]any{
		"type": "TXT", "name": name, "content": value, "ttl": challengeTTLSeconds, "proxied": false,
	})
	if err != nil {
		return TXTRecord{}, fmt.Errorf("encode a TXT record: %w", err)
	}
	var created struct {
		ID string `json:"id"`
	}
	if callErr := client.call(ctx, http.MethodPost, "/zones/"+url.PathEscape(zoneID)+"/dns_records", body,
		&created); callErr != nil {
		return TXTRecord{}, callErr
	}
	if created.ID == "" {
		return TXTRecord{}, fmt.Errorf("%w: a record with no id", ErrMalformed)
	}
	return TXTRecord{ZoneID: zoneID, RecordID: created.ID}, nil
}

// RemoveTXT removes a record SetTXT set.
func (client *Client) RemoveTXT(ctx context.Context, record TXTRecord) error {
	var removed struct {
		ID string `json:"id"`
	}
	return client.call(ctx, http.MethodDelete,
		"/zones/"+url.PathEscape(record.ZoneID)+"/dns_records/"+url.PathEscape(record.RecordID), nil, &removed)
}

// zoneOf is the id of the zone the token reaches that holds name: the one whose name is name or
// the longest suffix of it, asked for by name, longest first. A list of every zone is paged, and
// read one page at a time it missed a zone beyond the first page (M4's audit).
func (client *Client) zoneOf(ctx context.Context, name string) (string, error) {
	labels := strings.Split(name, ".")
	for first := range len(labels) - 1 {
		var zones []struct {
			ID string `json:"id"`
		}
		path := "/zones?name=" + url.QueryEscape(strings.Join(labels[first:], "."))
		if err := client.call(ctx, http.MethodGet, path, nil, &zones); err != nil {
			return "", err
		}
		if len(zones) > 0 {
			return zones[0].ID, nil
		}
	}
	return "", fmt.Errorf("%s: %w", name, ErrNoZone)
}

// call makes one request with the token and reads its result into result: Cloudflare's envelope,
// {"success": true, "result": …}.
func (client *Client) call(ctx context.Context, method, path string, body []byte, result any) error {
	token, err := client.keys.Open(ctx, secrets.DNSProviderToken)
	if errors.Is(err, secrets.ErrNotSet) || (err == nil && token.IsZero()) {
		return ErrTokenRefused
	}
	if err != nil {
		return fmt.Errorf("open the DNS provider's token: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cloudflare %s %s: %w", method, path, err)
	}
	request.Header.Set("Authorization", "Bearer "+token.Reveal())
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	answer, err := outbound.Exchange(client.http, request, maxResponseBytes)
	if err != nil {
		return fmt.Errorf("cloudflare %s %s: %w", method, path, err)
	}
	if answer.StatusCode == http.StatusUnauthorized || answer.StatusCode == http.StatusForbidden {
		return fmt.Errorf("cloudflare %s %s: %w", method, path, ErrTokenRefused)
	}
	if statusErr := outbound.StatusError(providerName, answer); statusErr != nil {
		return fmt.Errorf("cloudflare %s %s: %w", method, path, statusErr)
	}
	var envelope struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
	}
	if json.Unmarshal(answer.Body, &envelope) != nil || !envelope.Success ||
		json.Unmarshal(envelope.Result, result) != nil {
		return fmt.Errorf("cloudflare %s %s: %w", method, path, ErrMalformed)
	}
	return nil
}
