// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// A playback whose player wants a quality its node cannot make fast enough is moved onto the
// quality below it, told so with its chunks for cpuCeilingFor, whether or not another playback loads
// that one; where any of the quality's players is at the lowest, none is moved, and the run is evicted
// (D309 §6, D286 §4). A playback whose interest lapsed wants nothing, and is not counted.
func TestMoveOffStream_MovesItsPlayersDownAQuality(t *testing.T) {
	t.Parallel()
	clock := clocktest.NewFake(time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC))
	wanting := func(stream string, heardAt time.Time) *tokenPlayback {
		return &tokenPlayback{
			fileID: 1, ladder: filmOnesLadder, interest: playerInterest{picture: stream}, lastUsedAt: heardAt,
		}
	}
	service := uploadService(clock, 0, map[string]*tokenPlayback{
		"mac": wanting("q2000k540p", clock.Now()), "phone": wanting("q400k360p", clock.Now()),
		"gone": wanting("q400k360p", clock.Now().Add(-interestLapsesAfter-time.Second)),
	})
	if !service.MoveOffStream(1, "q2000k540p") || service.CeilingOf("mac") != 1296 {
		t.Fatalf("the Mac's ceiling = %d; want moved down onto 480p, 1296", service.CeilingOf("mac"))
	}
	if service.CeilingOf("phone") != 0 {
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
