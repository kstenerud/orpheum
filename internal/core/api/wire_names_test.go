// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// TestWireNames_AreTheOwnersChoice pins the wire names the owner chose before v0.3.3 (the owner,
// 2026-11-09): a strict decoder ignores a key it does not know, so a body that went back to an old
// name would be read as empty, not refused.
func TestWireNames_AreTheOwnersChoice(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		body any
		want string
	}{
		{"Next Up names its item", api.NextUpBody{ItemID: 7}, `"itemId":7`},
		{"plays names its item", api.PlaysBody{ItemID: 7}, `"itemId":7`},
		{"a watched mark names its item", api.WatchedStateBody{ItemID: 7}, `"itemId":7`},
		{"a film without sound names its item", api.FilmWithoutSoundBody{ItemID: 7}, `"itemId":7`},
		{"a state says it is playing", together.Inbound{IsPlaying: true}, `"isPlaying":true`},
		{"a state says it is buffering", together.Inbound{IsBuffering: true}, `"isBuffering":true`},
		{"a state says its page is hidden", together.Inbound{IsPageHidden: true}, `"isPageHidden":true`},
		{"a front message says the tab is behind", live.Inbound{IsBehind: true}, `"isBehind":true`},
		{"a verdict says the connection is held", rtcapi.Verdict{IsHeld: true}, `"isHeld":true`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := json.Marshal(test.body)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.Contains(string(encoded), test.want) {
				t.Errorf("%s; want it to hold %s", encoded, test.want)
			}
		})
	}
}
