// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A request in flight when the server is told to stop is finished before the databases close: the
// front doors close first. They used to close last, and a request still being answered as the
// server stopped met a closed database and answered "internal error" (2026-10-06).
//
// The request is held in flight by sending its headers alone, asking to be told to go on: the
// server's "100 Continue" says its handler is running and reading the body. The listener refusing a
// new connection then says the server has begun to close its doors; only then is the body sent.
// Closing them last, the databases were closed by then.
func TestServe_FinishesARequestInFlightBeforeItClosesTheDatabases(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	setUpOwner(t, server, path, "owner")
	address := strings.TrimPrefix(strings.TrimSuffix(server.baseURL, api.Prefix), "http://")
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(t.Context(), "tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	body := fmt.Sprintf(`{"name":"owner","password":%q,"device":{"name":"Shutdown test","platform":"web"}}`,
		ownerPassword)
	head := fmt.Sprintf("POST %s/session HTTP/1.1\r\nHost: orpheum.test\r\nContent-Type: application/json\r\n"+
		"Content-Length: %d\r\nExpect: 100-continue\r\n\r\n", api.Prefix, len(body))
	if _, writeErr := io.WriteString(conn, head); writeErr != nil {
		t.Fatal(writeErr)
	}
	reader := bufio.NewReader(conn)
	goOn, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := goOn.Body.Close(); closeErr != nil || goOn.StatusCode != http.StatusContinue {
		t.Fatalf("the server did not ask for the body: %d %v", goOn.StatusCode, closeErr)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.stop() }()
	waitUntilRefused(t, dialer, address)
	if _, writeErr := io.WriteString(conn, body); writeErr != nil {
		t.Fatal(writeErr)
	}
	answer, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	answered, err := io.ReadAll(answer.Body)
	if closeErr := answer.Body.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	if answer.StatusCode != http.StatusCreated {
		t.Errorf("a sign-in in flight as the server stopped answered %d %s; want it signed in, the "+
			"database still open", answer.StatusCode, answered)
	}
	if err := testutil.Receive(t, stopped, "the server to stop"); err != nil {
		t.Errorf("the server stopped with %v", err)
	}
}

// waitUntilRefused returns once a connection to address is refused: the server has closed its
// listener.
func waitUntilRefused(t *testing.T, dialer *net.Dialer, address string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), serveWaitLimit)
	defer cancel()
	for {
		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			if ctx.Err() != nil {
				t.Fatalf("the server still took connections after %v", serveWaitLimit)
			}
			return
		}
		if closeErr := conn.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
}
