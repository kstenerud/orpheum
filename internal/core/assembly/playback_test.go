// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A playback a film's page warmed up, which never played, is no one watching: its file's work is
// the page's to raise, at a look's priority, not a play's (D324 §2; the owner's staging, 2026-10-21,
// where opening 12 Patient Sailors's page raised its read-through to a play's).
func TestFilesPlayed_AreThoseOfThePlaybacksThatPlayed(t *testing.T) {
	t.Parallel()
	files := filesPlayed([]playback.NowPlaying{{FileID: 1, HasPlayed: true}, {FileID: 2}})
	if !slices.Equal(files, []int64{1}) {
		t.Errorf("files played %v; want file 1 alone, file 2 only warmed up", files)
	}
}

// The server wires a way to throw out every thing derived from a file's probe, so a file read again
// whose probe changed leaves nothing made from its old facts (D392 §3).
func TestBuild_EveryThingDerivedFromAProbeCanBeThrownOut(t *testing.T) {
	t.Parallel()
	services, err := Build(t.Context(), testParts(t))
	if err != nil {
		t.Fatal(err)
	}
	if unwired := services.ingester.UnwiredDroppers(); len(unwired) != 0 {
		t.Errorf("the ingest has no way to throw out %v", unwired)
	}
}

// The server's read-through logs how long each read of a file's index took, for whether
// playLadderWaitMs outlasts it: the server's logger reaches it (D414 §5).
func TestBuild_TheReadThroughLogsAnIndexReadsTime(t *testing.T) {
	t.Parallel()
	parts := testParts(t)
	var logged strings.Builder
	parts.Logger = slog.New(slog.NewTextHandler(&logged, nil))
	services, err := Build(t.Context(), parts)
	if err != nil {
		t.Fatal(err)
	}
	job := jobs.Job{ID: 9, Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 4}}
	attempt := nodes.Attempt{
		NodeID: 1, RunningSince: parts.Clock.Now().Add(-time.Second),
		Work: nodeproto.Work{ReadThrough: &nodeproto.ReadThrough{
			VideoTimeline: &nodeproto.VideoTimelineAsk{IsFromIndexOnly: true},
		}},
	}
	answer := nodeproto.Result{ReadThrough: &nodeproto.ReadThroughResult{}}
	if err := services.ReadThrough.Assignee(nil).Answer(t.Context(), job, attempt, answer); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logged.String(), "a file's keyframes were read from its index") {
		t.Errorf("logged %s; want the index read's time", logged.String())
	}
}
