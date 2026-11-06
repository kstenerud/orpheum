// SPDX-License-Identifier: AGPL-3.0-or-later

package unixsocket_test

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestClient_ReachesTheSocketWhateverTheHost(t *testing.T) {
	t.Parallel()
	socket := filepath.Join(testutil.SocketDirectory(t), "s.sock")
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
	client := unixsocket.Client(socket, 0)
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

// A socket serves only the holder of its key, kept beside it; a client reads the key as it sends,
// so it reaches a listener that started again with a new one (D331 §1).
func TestRequireKey_ServesOnlyTheHolderOfTheKeyKeptBesideTheSocket(t *testing.T) {
	t.Parallel()
	socket := filepath.Join(testutil.SocketDirectory(t), "core.sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var key atomic.Value
	key.Store("first")
	refused := atomic.Int32{}
	served := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "served") //nolint:errcheck // a test's answer
	})
	server := &http.Server{
		ReadHeaderTimeout: time.Minute,
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			current, _ := key.Load().(string) //nolint:errcheck // the test stores only strings
			unixsocket.RequireKey(current, served, func(*http.Request) { refused.Add(1) }).ServeHTTP(writer, request)
		}),
	}
	go func() { _ = server.Serve(listener) }() //nolint:errcheck // ends when the test closes it
	t.Cleanup(func() { _ = server.Close() })   //nolint:errcheck // test teardown
	status := func(client *http.Client) int {
		t.Helper()
		request, requestErr := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://core/x", http.NoBody)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		response, sendErr := client.Do(request)
		if sendErr != nil {
			t.Fatal(sendErr)
		}
		_ = response.Body.Close() //nolint:errcheck // only the status matters
		return response.StatusCode
	}
	client := unixsocket.Client(socket, 0)
	if got := status(client); got != http.StatusUnauthorized {
		t.Errorf("a request with no key beside the socket: %d; want 401", got)
	}
	if err = unixsocket.WriteKey(socket, "first"); err != nil {
		t.Fatal(err)
	}
	if got := status(client); got != http.StatusOK {
		t.Errorf("a request with the key beside the socket: %d; want 200", got)
	}
	key.Store("second")
	if got := status(client); got != http.StatusUnauthorized {
		t.Errorf("a request with a key the listener no longer has: %d; want 401", got)
	}
	if err = unixsocket.WriteKey(socket, "second"); err != nil {
		t.Fatal(err)
	}
	if got := status(client); got != http.StatusOK {
		t.Errorf("a request after the listener's new key was written: %d; want 200, read as it is sent", got)
	}
	if got := refused.Load(); got != 2 {
		t.Errorf("the listener was told of %d refusals; want 2", got)
	}
	info, err := os.Stat(unixsocket.KeyPath(socket))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the key is %v, %v; want mode 0600", info.Mode().Perm(), err)
	}
}
