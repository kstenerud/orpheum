// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build dev

package main

import (
	"log/slog"
	"net/http"
	"testing"
)

// A development build lets a discovery's offers check the machine's own addresses, for its browser
// tests on one machine.
func TestRelaxForDevelopment_LetsADevelopmentBuildCheckItsOwnAddresses(t *testing.T) {
	t.Parallel()
	rtc := newWebRTC(t.Context(), http.NotFoundHandler(), WebRTCConfig{MaxConnections: 64},
		slog.New(slog.DiscardHandler))
	if !rtc.AcceptsAnyRelayedAddress() {
		t.Error("a development build still refuses a discovery's offers its own addresses")
	}
}
