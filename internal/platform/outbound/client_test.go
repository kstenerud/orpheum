// SPDX-License-Identifier: GPL-3.0-or-later

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
	client, err := NewClient(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Error("the transport has a Proxy function; it would read HTTP_PROXY (D204)")
	}
	if client.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", client.Timeout)
	}

	// The positive sibling: the client still reaches a server directly.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
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
