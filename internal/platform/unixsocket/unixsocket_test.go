// SPDX-License-Identifier: GPL-3.0-or-later

package unixsocket_test

import (
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

func TestTransport_ReachesTheSocketWhateverTheHost(t *testing.T) {
	t.Parallel()
	socket := filepath.Join(t.TempDir(), "s.sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{
		ReadHeaderTimeout: time.Minute,
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(writer, "on the socket") //nolint:errcheck // a test's answer
		}),
	}
	go func() { _ = server.Serve(listener) }() //nolint:errcheck // ends when the test closes it
	t.Cleanup(func() { _ = server.Close() })   //nolint:errcheck // test teardown
	client := &http.Client{Transport: unixsocket.Transport(socket)}
	for _, host := range []string{"node", "core"} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+host+"/x", http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("host %s: %v", host, err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close() //nolint:errcheck // read whole
		if err != nil || string(body) != "on the socket" {
			t.Errorf("host %s answered %q, %v; want the socket's answer", host, body, err)
		}
	}
}
