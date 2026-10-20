// SPDX-License-Identifier: AGPL-3.0-or-later

package outbound

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient_TakesNoProxyFromTheEnvironment(t *testing.T) {
	t.Parallel()
	client, err := NewClient(5*time.Second, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	wrapped, ok := client.Transport.(userAgentTransport)
	if !ok {
		t.Fatalf("transport is %T, want our identified transport", client.Transport)
	}
	transport := wrapped.Transport
	if transport.Proxy != nil {
		t.Error("the transport has a Proxy function; it would read HTTP_PROXY (D204)")
	}
	if client.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", client.Timeout)
	}

	// The positive sibling: the client still reaches a server directly.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if agent := request.Header.Get("User-Agent"); agent != "Orpheum/1.2.3" {
			t.Errorf("User-Agent %q, want Orpheum/1.2.3 (D213 §6)", agent)
		}
		if _, writeErr := io.WriteString(writer, "direct"); writeErr != nil {
			t.Error(writeErr)
		}
	}))
	defer server.Close()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "direct" {
		t.Errorf("body = %q, %v; want %q", body, err, "direct")
	}
}

// A redirect to another origin is not followed: the request's own headers, a provider's key among
// them, would go with it (the Plex-chain review's O1). One within the origin is.
func TestNewClient_FollowsARedirectOnlyWithinItsOrigin(t *testing.T) {
	t.Parallel()
	client, err := NewClient(5*time.Second, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		t.Errorf("another origin was sent the request, its Api-Key %q", request.Header.Get("Api-Key"))
	}))
	defer elsewhere.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/away":
			http.Redirect(writer, request, elsewhere.URL+"/take", http.StatusFound)
		case "/moved":
			http.Redirect(writer, request, "/here", http.StatusMovedPermanently)
		case "/here":
			writer.WriteHeader(http.StatusNoContent)
		}
	}))
	defer provider.Close()
	for _, tc := range []struct {
		path       string
		wantStatus int
	}{{"/away", http.StatusFound}, {"/moved", http.StatusNoContent}} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, provider.URL+tc.path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Api-Key", "the-key")
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		if response.StatusCode != tc.wantStatus {
			t.Errorf("%s answered %d; want %d", tc.path, response.StatusCode, tc.wantStatus)
		}
	}
}
