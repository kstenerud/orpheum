// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// A server with a public address for the relay serves voice's TURN relay beside the API, and one
// without serves none (D243 §8).
func TestServe_RelaysVoiceOnlyWithARelayAddress(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	config.TURN = TURNConfig{
		PublicAddress: "127.0.0.1", Listen: "127.0.0.1:0", RelayPortMin: 51000, RelayPortMax: 51999,
	}
	relaying := newCapturedLog("voiceRelayStarted")
	startServing(t, config, relaying, "voiceRelayStarted", nil)

	config, _ = testConfig(t)
	quiet := newCapturedLog("ready")
	startServing(t, config, quiet, "ready", nil)
	if len(testutil.LinesWith(quiet.String(), "voiceRelayStarted")) > 0 {
		t.Errorf("a server with no turn.public_address relays voice:\n%s", quiet)
	}
}
