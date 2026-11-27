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

// A node's speed key changes with its FFmpeg's version or build and with its chosen encoder, as its
// hello says them, so its runs' figures start over (start-quality-by-node-speed.md §3); a node not
// heard has none.
func TestSpeedKey_ChangesWithTheNodesFFmpegAndEncoder(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	hello := nodeproto.Hello{
		Name: "local", RootPaths: []string{"/media"}, FFmpegVersion: "7.1", FFmpegSha256: "ab12",
		Hardware: nodeproto.Hardware{ChosenEncoder: nodeproto.EncoderX264},
	}
	reply := harness.hello(t, hello)
	first := harness.service.SpeedKey(reply.NodeID)
	if first == "" {
		t.Fatal("a node heard has no speed key")
	}
	if got := harness.service.SpeedKey(reply.NodeID + 1); got != "" {
		t.Errorf("a node never heard has the speed key %q; want none", got)
	}
	keys := map[string]string{"first": first}
	for name, changed := range map[string]func(*nodeproto.Hello){
		"FFmpeg's version": func(hello *nodeproto.Hello) { hello.FFmpegVersion = "7.2" },
		"FFmpeg's build":   func(hello *nodeproto.Hello) { hello.FFmpegSha256 = "cd34" },
		"the encoder":      func(hello *nodeproto.Hello) { hello.Hardware.ChosenEncoder = nodeproto.EncoderVAAPI },
	} {
		next := hello
		changed(&next)
		again := harness.hello(t, next)
		key := harness.service.SpeedKey(again.NodeID)
		for before, earlier := range keys {
			if key == earlier {
				t.Errorf("a changed %s leaves the speed key %q as %s's", name, key, before)
			}
		}
		keys[name] = key
	}
}
