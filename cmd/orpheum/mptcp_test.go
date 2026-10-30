// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"net"
	"testing"
)

// No listener or dialer of the binary uses Multipath TCP, which go.mod's `godebug multipathtcp=0`
// turns off: an MPTCP listener, Go's default since 1.24, exposed CVE-2026-98432's kernel
// use-after-free to remote code execution (the owner, 2026-10-30).
func TestSockets_UseNoMultipathTCP(t *testing.T) {
	t.Parallel()
	var listening net.ListenConfig
	if listening.MultipathTCP() {
		t.Error("a listener uses Multipath TCP by default; go.mod's godebug multipathtcp=0 should turn it off")
	}
	var dialing net.Dialer
	if dialing.MultipathTCP() {
		t.Error("a dialer uses Multipath TCP by default; go.mod's godebug multipathtcp=0 should turn it off")
	}
}
