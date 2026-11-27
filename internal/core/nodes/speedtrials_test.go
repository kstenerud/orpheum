// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The core keeps a node's speed trials from its last status, and has none of a node that said none.
func TestSpeedTrials_AreKeptFromTheNodesLastStatus(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media"}})
	if got := harness.service.SpeedTrials(reply.NodeID); got != nil {
		t.Errorf("before any status the node's trials are %+v; want none", got)
	}
	harness.postStatus(t, reply.NodeID, nodeproto.Status{SentAt: 1})
	if got := harness.service.SpeedTrials(reply.NodeID); got != nil {
		t.Errorf("a status without trials leaves %+v; want none", got)
	}
	trials := []nodeproto.SpeedTrial{
		{
			Codec: "hevc", BitDepth: 10, Height: 2160, MadeKbps: 8000, MadeHeight: 2160,
			SpeedPercent: 60, DecodedOn: "gpu",
		},
		{Codec: "av1", BitDepth: 10, Height: 1080, MadeKbps: 400, MadeHeight: 360, NotTried: "no encoder"},
	}
	harness.postStatus(t, reply.NodeID, nodeproto.Status{SentAt: 2, SpeedTrials: trials})
	if got := harness.service.SpeedTrials(reply.NodeID); !slices.Equal(got, trials) {
		t.Errorf("the node's trials are %+v; want %+v", got, trials)
	}
	if got := harness.service.SpeedTrials(reply.NodeID + 1); got != nil {
		t.Errorf("a node never heard has trials %+v; want none", got)
	}
}
