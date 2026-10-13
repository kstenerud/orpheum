// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import "time"

// Bounds are a server's bounds on each connection, set shorter than the core's for the tests.
type Bounds struct {
	ConnectWithin   time.Duration
	PerSource       int
	Channels        int
	RequestDeadline time.Duration
}

// SetBounds sets server's bounds; a zero field keeps the core's.
func SetBounds(server *Server, set Bounds) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if set.ConnectWithin > 0 {
		server.bounds.connectWithin = set.ConnectWithin
	}
	if set.PerSource > 0 {
		server.bounds.perSource = set.PerSource
	}
	if set.Channels > 0 {
		server.bounds.channels = set.Channels
	}
	if set.RequestDeadline > 0 {
		server.bounds.requestDeadline = set.RequestDeadline
	}
}

// ConnectionCount is how many connections server holds.
func ConnectionCount(server *Server) int {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return len(server.connections)
}

// CheckRelayedOffer is checkRelayedOffer.
var CheckRelayedOffer = checkRelayedOffer

// IsFromOutside is isFromOutside.
var IsFromOutside = isFromOutside
