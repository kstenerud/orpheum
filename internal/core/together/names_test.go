// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"encoding/json"
	"testing"
)

// Every value the together protocol names is camelCase, as its keys are (the owner, 2026-11-09):
// a group's state is said in lower case, as its events are.
func TestNames_TheGroupStatesTheProtocolSaysAreCamelCase(t *testing.T) {
	t.Parallel()
	for _, each := range []struct {
		state GroupState
		want  string
	}{
		{GroupIdle, "idle"},
		{GroupWaiting, "waiting"},
		{GroupPaused, "paused"},
		{GroupPlaying, "playing"},
	} {
		said, err := json.Marshal(GroupView{State: each.state})
		if err != nil {
			t.Fatal(err)
		}
		var read struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(said, &read); err != nil || read.State != each.want {
			t.Errorf("a group's state is said %q (%v); want %q", read.State, err, each.want)
		}
	}
}
