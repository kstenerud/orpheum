// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// A playback on a quality its node cannot make fast enough is moved onto the highest lower quality
// another playback of its film loads, told so with its chunks for cpuCeilingFor; where any of the
// quality's playbacks has none to move to, none is moved (D286 §4).
func TestMoveOffStream_MovesItsPlaybacksOntoAQualityAnotherLoads(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC))
	asking := func(stream string) *tokenPlayback {
		return &tokenPlayback{
			fileID: 1, ladder: filmOnesLadder, pictureStream: stream,
			picturesNeed: streamNeed{kbps: 1, at: clock.Now()},
		}
	}
	service := uploadService(clock, 0, map[string]*tokenPlayback{
		"mac": asking("q2000k540p"), "phone": asking("q400k360p"), "tablet": asking("q1200k480p"),
	})
	if !service.MoveOffStream(1, "q2000k540p") || service.CeilingOf("mac") != 1296 {
		t.Fatalf("the Mac's ceiling = %d; want moved onto the tablet's 480p, 1296", service.CeilingOf("mac"))
	}
	if service.CeilingOf("phone") != 0 || service.CeilingOf("tablet") != 0 {
		t.Error("a playback of another quality was moved")
	}
	clock.Advance(cpuCeilingFor)
	if service.CeilingOf("mac") != 0 {
		t.Errorf("after cpuCeilingFor the Mac's ceiling = %d; want none", service.CeilingOf("mac"))
	}
	if service.MoveOffStream(1, "q400k360p") || service.CeilingOf("phone") != 0 {
		t.Error("the phone, on the lowest quality, was said moved; want nothing to move it to")
	}
}
