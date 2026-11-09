// SPDX-License-Identifier: AGPL-3.0-or-later

package cloudflaretest

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/strictjson"
)

// Token is the token the fake accepts.
const Token = "the-zone-token"

// Keys opens the fake's token, or none when empty.
type Keys string

// Open is the token, as secrets.Store opens it.
func (keys Keys) Open(context.Context, secrets.Name) (secrets.Value, error) {
	if keys == "" {
		return secrets.Value{}, secrets.ErrNotSet
	}
	return secrets.NewValue(string(keys)), nil
}

// Fake is Cloudflare, serving the zones it holds.
type Fake struct {
	Server *httptest.Server
	mutex  sync.Mutex
	// zones are the zones the token reaches, by id.
	zones map[string]string
	// records are the TXT records set now, by id: their name and value.
	records map[string][2]string
	// everSet are every TXT record set, by name, in order.
	everSet map[string][]string
	lastID  int
}

// New serves a fake Cloudflare whose token reaches zones, by name, until the test ends.
func New(t *testing.T, zoneNames ...string) *Fake {
	t.Helper()
	fake := &Fake{zones: map[string]string{}, records: map[string][2]string{}, everSet: map[string][]string{}}
	for index, name := range zoneNames {
		fake.zones[fmt.Sprintf("zone%d", index+1)] = name
	}
	fake.Server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.Server.Close)
	return fake
}

// TXT is the value of the TXT record of name set now, or "" when there is none.
func (fake *Fake) TXT(name string) string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	for _, record := range fake.records {
		if record[0] == name {
			return record[1]
		}
	}
	return ""
}

// EverSet are the values every TXT record of name was set to, in order.
func (fake *Fake) EverSet(name string) []string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]string(nil), fake.everSet[name]...)
}

// RecordCount is how many records are set now.
func (fake *Fake) RecordCount() int {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return len(fake.records)
}

// zonesPage is how many zones Cloudflare lists in one answer when per_page does not say.
const zonesPage = 20

// zonesAnswer is the first page of the zones whose name is query's name, every zone when it names
// none, in the order of their ids: per_page of them, or Cloudflare's default.
func (fake *Fake) zonesAnswer(query url.Values) []map[string]string {
	ids := slices.Sorted(maps.Keys(fake.zones))
	pageSize := zonesPage
	if asked, err := strconv.Atoi(query.Get("per_page")); err == nil && asked > 0 {
		pageSize = asked
	}
	zones := []map[string]string{}
	for _, id := range ids {
		if name := query.Get("name"); name != "" && fake.zones[id] != name {
			continue
		}
		if len(zones) < pageSize {
			zones = append(zones, map[string]string{"id": id, "name": fake.zones[id]})
		}
	}
	return zones
}

func (fake *Fake) serve(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("Authorization") != "Bearer "+Token {
		writer.WriteHeader(http.StatusForbidden)
		return
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	path := request.URL.Path
	switch {
	case request.Method == http.MethodGet && path == "/zones":
		answer(writer, fake.zonesAnswer(request.URL.Query()))
	case request.Method == http.MethodPost && strings.HasSuffix(path, "/dns_records"):
		var record struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if strictjson.Decode(request.Body, &record) != nil || record.Type != "TXT" {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		fake.lastID++
		id := fmt.Sprintf("record%d", fake.lastID)
		fake.records[id] = [2]string{record.Name, record.Content}
		fake.everSet[record.Name] = append(fake.everSet[record.Name], record.Content)
		answer(writer, map[string]string{"id": id})
	case request.Method == http.MethodDelete && strings.Contains(path, "/dns_records/"):
		id := path[strings.LastIndex(path, "/")+1:]
		delete(fake.records, id)
		answer(writer, map[string]string{"id": id})
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}

// answer writes Cloudflare's envelope around result.
func answer[T []map[string]string | map[string]string](writer http.ResponseWriter, result T) {
	encoded, err := json.Marshal(struct {
		Success bool `json:"success"`
		Result  T    `json:"result"`
	}{Success: true, Result: result})
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(encoded) //nolint:errcheck // the client went; a test's fake
}
