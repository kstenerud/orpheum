// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A browser on the server's own machine reaches the core from Docker's gateway, never home; the port
// that machine alone reaches serves the same server, and a request on it administers where one on the
// house's port does not, which names it (D334 §4).
func TestServe_ThisMachinesPortAdministersWhereTheHousesPortDoesNot(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	config.Server.HomeNetworks = []string{"192.0.2.0/24"}
	captured := newCapturedLog("ready")
	// A free port is found by listening on it and closing it, and another run's test may take it before
	// this one listens again: that is tried again on a new port, not failed.
	var thisMachine string
	var listener net.Listener
	for attempt := 1; listener == nil; attempt++ {
		free, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		thisMachine = free.Addr().String()
		if err = free.Close(); err != nil {
			t.Fatal(err)
		}
		config.Server.ThisMachineListen = thisMachine
		if listener, err = listenForClients(t.Context(), config.Server, captured.logger); err != nil {
			if attempt == 5 {
				t.Fatalf("no free port for this machine's listener after %d tries: %v", attempt, err)
			}
			listener = nil
		}
	}
	server := startServingOn(t, listener, config, captured, "ready", nil)
	setUpOwner(t, server, path, "owner")
	token := signInAs(t, server.baseURL, "owner", ownerPassword)
	_, port, _ := net.SplitHostPort(thisMachine) //nolint:errcheck // the listener's own address
	if refused := requestAPI(t, http.MethodGet, server.baseURL+"/users", token, nil); refused.status !=
		http.StatusForbidden || !strings.Contains(refused.body, "http://localhost:"+port) {
		t.Errorf("GET /users on the house's port = %d %s; want 403 naming http://localhost:%s",
			refused.status, refused.body, port)
	}
	allowed := requestAPI(t, http.MethodGet, "http://"+thisMachine+api.Prefix+"/users", token, nil)
	if allowed.status != http.StatusOK {
		t.Errorf("GET /users on this machine's port = %d %s; want 200", allowed.status, allowed.body)
	}
}

// signalsOn is a writer that keeps what is written and closes seen once it holds want.
type signalsOn struct {
	mutex   sync.Mutex
	written bytes.Buffer
	want    string
	seen    chan struct{}
	once    sync.Once
}

func (writer *signalsOn) Write(text []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	count, err := writer.written.Write(text)
	if bytes.Contains(writer.written.Bytes(), []byte(writer.want)) {
		writer.once.Do(func() { close(writer.seen) })
	}
	return count, err
}

// orpheum core serves the port its config names for the server's own machine beside its own
// (D334 §4): the command, not only serve, listens there.
func TestCore_ServesThisMachinesPortBesideItsOwn(t *testing.T) {
	t.Parallel()
	_, dir := testConfig(t)
	free, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	thisMachine := free.Addr().String()
	if err = free.Close(); err != nil {
		t.Fatal(err)
	}
	instance, err := loadInstance(dir, kindCore)
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(instance.fullConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	named := strings.Replace(string(text), `this_machine_listen = ""`, `this_machine_listen = "`+thisMachine+`"`, 1)
	err = os.WriteFile(instance.fullConfigPath(), []byte(named), 0o600) //nolint:gosec // G703: the test's own file
	if err != nil {
		t.Fatal(err)
	}
	ready := &signalsOn{want: `"logEvent":"ready"`, seen: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan int, 1)
	go func() {
		done <- dispatch(ctx, []string{"core", "--instance", dir}, noEnvironment, nil, io.Discard, ready)
	}()
	testutil.Receive(t, ready.seen, "the core to be ready")
	health := requestAPI(t, http.MethodGet, "http://"+thisMachine+api.Prefix+"/health", "", nil)
	cancel()
	testutil.Receive(t, done, "the core to stop")
	if health.status != http.StatusOK {
		t.Errorf("GET /health on this machine's port = %d %s; want 200", health.status, health.body)
	}
}
