// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks"
)

// filmOnesLadder is a film's ladder, lowest first: three qualities made for players, then the
// film's own picture.
var filmOnesLadder = []chunks.Rung{
	{StreamName: "q400k360p", AverageKbps: 496},
	{StreamName: "q1200k480p", AverageKbps: 1296},
	{StreamName: "q2000k540p", AverageKbps: 2096},
	{StreamName: "picture", AverageKbps: 5096},
}

// playingAt is a playback of film 1 loading rung, uncapped, started at startedAt.
func playingAt(token string, rung int, startedAt time.Time) lowerable {
	return lowerable{
		token: token, fileID: 1, startedAt: startedAt, rungs: filmOnesLadder, loads: rung,
		ceiling: len(filmOnesLadder) - 1,
	}
}

// Over the share, the playback needing most is lowered first, onto the quality another playback of
// its film loads when the next lower one is made for no one, so one encode feeds both (D286 §4, §5).
func TestUploadControl_LowersTheOneNeedingMostOntoAQualityAlreadyMade(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	playbacks := []lowerable{playingAt("mac", 3, now), playingAt("phone", 1, now)}
	control := &uploadControl{}
	isChanged, isOverAtLowest := control.look(playbacks, 0, 5000, now)
	if !isChanged || isOverAtLowest || playbacks[0].ceiling != 1 || playbacks[1].ceiling != 3 {
		t.Errorf("over the share, ceilings = %d, %d (%t, %t); want the Mac's at the phone's 480p",
			playbacks[0].ceiling, playbacks[1].ceiling, isChanged, isOverAtLowest)
	}
	if got := playbacks[0].ceilingKbps(); got != 1296 {
		t.Errorf("the Mac is told %d kbit/s; want 1296", got)
	}
}

// Of playbacks needing the same, the newest is lowered first.
func TestUploadControl_LowersTheNewestOfThoseNeedingTheSame(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	playbacks := []lowerable{playingAt("first", 2, now), playingAt("second", 2, now.Add(time.Minute))}
	if isChanged, _ := (&uploadControl{}).look(playbacks, 0, 4000, now); !isChanged ||
		playbacks[0].rung() != 2 || playbacks[1].rung() != 1 {
		t.Errorf("rungs = %d, %d; want the newer lowered", playbacks[0].rung(), playbacks[1].rung())
	}
}

// A lowered playback is raised a quality at a time, each once the need has had room for roomFor,
// and not while raising it would leave no room.
func TestUploadControl_RaisesOneQualityAtATimeOnceThereIsRoom(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	mac := playingAt("mac", 3, now)
	mac.ceiling = 1
	playbacks := []lowerable{mac, playingAt("phone", 1, now)}
	control := &uploadControl{}
	const shareKbps = 5000 // raised needs at most 3750
	if isChanged, _ := control.look(playbacks, 0, shareKbps, now); isChanged {
		t.Fatal("raised at once; want room for roomFor first")
	}
	if isChanged, _ := control.look(playbacks, 0, shareKbps, now.Add(roomFor)); !isChanged ||
		playbacks[0].ceiling != 2 {
		t.Fatalf("after roomFor, the Mac's ceiling = %d; want raised to 2", playbacks[0].ceiling)
	}
	playbacks[0].loads = 2 // its player climbed
	for _, at := range []time.Duration{roomFor + time.Second, 3 * roomFor} {
		if isChanged, _ := control.look(playbacks, 0, shareKbps, now.Add(at)); isChanged {
			t.Errorf("at %v the Mac was raised to %d; want it kept, its own picture leaving no room", at,
				playbacks[0].ceiling)
		}
	}
}

// With every playback at its lowest and the need still over the share, the look says so: the last
// resort is the newest member's (D286 §5).
func TestUploadControl_SaysWhenEveryoneIsAtTheirLowestAndItIsStillOver(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	playbacks := []lowerable{playingAt("one", 1, now), playingAt("two", 1, now)}
	if _, isOverAtLowest := (&uploadControl{}).look(playbacks, 500, 1000, now); !isOverAtLowest ||
		playbacks[0].rung() != 0 || playbacks[1].rung() != 0 {
		t.Errorf("rungs = %d, %d, over at lowest %t; want both at the lowest, still over",
			playbacks[0].rung(), playbacks[1].rung(), isOverAtLowest)
	}
}

// The newest member is sent away once the need has been over the share with every quality at its
// lowest for overAtLowestFor, and not again until what the one sent away asked for has stopped
// counting (D286 §5).
func TestLastResort_IsDueAfterOverAtLowestForAndSettlesAfterEach(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC)
	resort := &lastResort{}
	if resort.isDue(true, start) || resort.isDue(true, start.Add(overAtLowestFor-time.Second)) {
		t.Fatal("due before overAtLowestFor")
	}
	if !resort.isDue(true, start.Add(overAtLowestFor)) {
		t.Fatal("not due after overAtLowestFor")
	}
	resort.sentAwayAt = start.Add(overAtLowestFor)
	if resort.isDue(true, start.Add(overAtLowestFor+askedWithin)) {
		t.Error("due again before the one sent away stopped counting")
	}
	if !resort.isDue(true, resort.sentAwayAt.Add(sentAwaySettles)) {
		t.Error("not due again after sentAwaySettles")
	}
	if resort.isDue(false, start.Add(time.Hour)) || resort.isDue(true, start.Add(time.Hour)) {
		t.Error("due at once after the need fell under the share; want overAtLowestFor again")
	}
}
