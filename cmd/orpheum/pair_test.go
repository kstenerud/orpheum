// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fakeCore answers pair's requests as the core's API does, connected to a discovery already or not,
// and keeps each request's method and path, with what the claim was given.
type fakeCore struct {
	mutex       sync.Mutex
	asked       []string
	claimed     map[string]string
	isConnected bool
}

func (core *fakeCore) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	core.mutex.Lock()
	defer core.mutex.Unlock()
	asked := request.Method + " " + strings.TrimPrefix(request.URL.Path, "/api/v1")
	core.asked = append(core.asked, asked)
	var body struct {
		Password string `json:"password"`
		BaseURL  string `json:"baseUrl"`
		Key      string `json:"key"`
	}
	if request.Body != http.NoBody {
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil && request.Method != http.MethodGet {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
	}
	answer := func(status int, value map[string]string) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		if err := json.NewEncoder(writer).Encode(value); err != nil {
			panic(err) // a map of strings always encodes
		}
	}
	switch asked {
	case "POST /session":
		if body.Password != "the-password" {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusUnauthorized)
			refusal := `{"error": {"message": "wrong name or password"}}`
			_, _ = writer.Write([]byte(refusal)) //nolint:errcheck // the test reads what it can
			return
		}
		answer(http.StatusCreated, map[string]string{"token": "the-token"})
	case "GET /system/discovery":
		baseURL := ""
		if core.isConnected {
			baseURL = "https://discovery.example"
		}
		answer(http.StatusOK, map[string]string{"baseUrl": baseURL})
	case "POST /system/discovery":
		core.isConnected = request.Header.Get("Authorization") == "Bearer the-token"
		answer(http.StatusOK, map[string]string{"baseUrl": body.BaseURL, "state": "pending"})
	case "POST /system/discovery/claim":
		core.claimed = map[string]string{"key": body.Key, "password": body.Password}
		answer(http.StatusOK, map[string]string{"state": "claimed", "realm": "the-smiths"})
	case "DELETE /session":
		writer.WriteHeader(http.StatusNoContent)
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}

// The install script pairs the core by its account's key with the admin's password it was just
// given, in the same sitting (D334 §3): pair signs in, connects the core to the discovery when it is
// connected to none, enters the key with the password, and signs out.
func TestPair_ConnectsWhenNeededEntersTheKeyAndSignsOut(t *testing.T) {
	t.Parallel()
	for _, isConnected := range []bool{false, true} {
		core := &fakeCore{isConnected: isConnected}
		server := httptest.NewServer(core)
		paired, err := pair(t.Context(), server.Client(), server.URL+"/api/v1", pairing{
			name: "karl", password: "the-password", key: "abcd-efgh-jkmn-pqrs",
			discoveryURL: "https://discovery.example",
		})
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if paired != "paired: realm the-smiths" {
			t.Errorf("pair said %q; want the realm", paired)
		}
		want := []string{
			"POST /session", "GET /system/discovery", "POST /system/discovery",
			"POST /system/discovery/claim", "DELETE /session",
		}
		if isConnected {
			want = slices.Delete(want, 2, 3)
		}
		if !slices.Equal(core.asked, want) {
			t.Errorf("connected %v: pair asked %v; want %v", isConnected, core.asked, want)
		}
		if core.claimed["key"] != "abcd-efgh-jkmn-pqrs" || core.claimed["password"] != "the-password" {
			t.Errorf("the claim was given %v; want the key and the password", core.claimed)
		}
	}
}

func TestPair_SaysWhatTheCoreRefused(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(&fakeCore{})
	defer server.Close()
	_, err := pair(t.Context(), server.Client(), server.URL+"/api/v1", pairing{
		name: "karl", password: "not-it", key: "abcd-efgh-jkmn-pqrs", discoveryURL: "https://discovery.example",
	})
	if err == nil || !strings.Contains(err.Error(), "wrong name or password") {
		t.Errorf("pair with the wrong password = %v; want the core's own words", err)
	}
}
