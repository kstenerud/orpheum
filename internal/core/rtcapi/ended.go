// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// endedMessage is the line each connection that connected logs as it ends, with its SCTP figures: so
// a slow connection says whether its window was cut, and how far, rather than leaving the Throughput
// probe's figure to be guessed at (webrtc-throughput-review-2026-10-28.md, fix 5; D397).
const endedMessage = "rtc connection ended"

// connectionEnd is what a connection's end line needs from its connecting, and that it is said once.
type connectionEnd struct {
	mutex       sync.Mutex
	path        Path
	connectedAt time.Time
	said        bool
}

// connected notes that the connection connected at at, taking path.
func (end *connectionEnd) connected(path Path, at time.Time) {
	end.mutex.Lock()
	defer end.mutex.Unlock()
	end.path, end.connectedAt = path, at
}

// take is the path and the time of connecting, and whether the end is still to be said: once, and
// only for a connection that connected.
func (end *connectionEnd) take() (Path, time.Time, bool) {
	end.mutex.Lock()
	defer end.mutex.Unlock()
	if end.said || end.connectedAt.IsZero() {
		return "", time.Time{}, false
	}
	end.said = true
	return end.path, end.connectedAt, true
}

// sctpFigures are the fields of the end line from a connection's SCTP transport: what it sent and
// received, and its congestion window, smoothed round trip, receiver window and packet size at the
// end. Its retransmission and T3 timeout counts are pion's alone (sctp v1.12.0's associationStats,
// unexported, written only to its debug log), so the line has none.
func sctpFigures(stats webrtc.SCTPTransportStats) []any {
	return []any{
		"bytesSent", stats.BytesSent, "bytesReceived", stats.BytesReceived,
		"cwndBytes", stats.CongestionWindow,
		"srttMs", float64(time.Duration(stats.SmoothedRoundTripTime*float64(time.Second)).Microseconds()) / 1000,
		"rwndBytes", stats.ReceiverWindow, "mtu", stats.MTU,
	}
}

// logEnded logs a connection's end line, once, if it connected: read while its SCTP association is
// still there, before the core closes the connection or as SCTP tells it closed, since pion drops
// the association, and every figure with it, when the connection is closed.
func (server *Server) logEnded(connection *webrtc.PeerConnection, held *heldConnection) {
	if held == nil {
		return
	}
	path, connectedAt, isToSay := held.end.take()
	if !isToSay {
		return
	}
	fields := []any{
		"connection", held.name(), pathField, string(path),
		"durationMs", server.clock.Now().Sub(connectedAt).Milliseconds(),
	}
	if sctp := connection.SCTP(); sctp != nil {
		fields = append(fields, sctpFigures(sctp.Stats())...)
	}
	server.logger.Info(endedMessage, fields...)
}

// endWithItsSCTP has a connection end as its SCTP does, its figures still there: the end line is
// read, and the connection, which carries nothing else, closed (settings' DisableCloseByDTLS).
func (server *Server) endWithItsSCTP(connection *webrtc.PeerConnection, held *heldConnection) {
	connection.SCTP().OnClose(func(error) {
		server.logEnded(connection, held)
		go func() { _ = connection.Close() }() //nolint:errcheck // its SCTP is gone, closed either way
	})
}
