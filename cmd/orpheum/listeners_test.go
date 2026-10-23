// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"net"
	"testing"
)

// One HTTP server serves both the house's port and the one this machine alone reaches (D334 §4):
// the joined listener takes each one's connections, and closing it closes both.
func TestJoinListeners_AcceptsFromEachAndClosesThemAll(t *testing.T) {
	t.Parallel()
	listen := func() net.Listener {
		t.Helper()
		listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		return listener
	}
	house, thisMachine := listen(), listen()
	joined := joinListeners(house, thisMachine)
	if joined.Addr().String() != house.Addr().String() {
		t.Errorf("the joined address is %s; want the first's, %s", joined.Addr(), house.Addr())
	}
	for _, listener := range []net.Listener{house, thisMachine} {
		client, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		served, err := joined.Accept()
		if err != nil {
			t.Fatal(err)
		}
		if served.LocalAddr().String() != listener.Addr().String() {
			t.Errorf("a connection to %s came in on %s", listener.Addr(), served.LocalAddr())
		}
		_ = client.Close() //nolint:errcheck // the test is over
		_ = served.Close() //nolint:errcheck // the test is over
	}
	if err := joined.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := joined.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Errorf("after the close Accept = %v; want net.ErrClosed", err)
	}
	if _, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", thisMachine.Addr().String()); err == nil {
		t.Error("the second listener still takes connections after the close")
	}
}
