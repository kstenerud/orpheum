// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	corenodes "github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// testStall is a node's word that the FFmpeg it ran stood still at 4 s for three minutes.
var testStall = &nodeproto.Stall{
	Cause: nodeproto.StallStoodStill, LastPositionMs: 4000, WithoutAdvanceMs: 180_000, Detail: "ffmpeg stalled",
}

// answerAfterStall runs handle as job's attempt after one that stalled or not, answers the node's
// command with answer, and returns how the job ended.
func answerAfterStall(
	t *testing.T, harness testHarness, job jobs.Job, handle jobs.Handler, answer func(),
) error {
	t.Helper()
	finished := harness.handle(t, handle, job)
	harness.nextCommand(t)
	answer()
	//nolint:wrapcheck // the job's own error, which the test reads as it is
	return testutil.Receive(t, finished, "the job to end")
}

func TestReadThrough_AStallIsTriedAgainAndTwoInARowAreTheFilesFailure(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	measure := func(previousAttemptStalled bool) error {
		job := jobs.Job{
			ID: 95, Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 2},
			PreviousAttemptToolFailed: previousAttemptStalled,
		}
		return answerAfterStall(t, harness, job, harness.readThrough.HandleReadThrough, func() {
			harness.answerReadThrough(t, 95, nodeproto.ReadThroughResult{Stall: testStall})
		})
	}
	if err := measure(false); err == nil || !strings.HasPrefix(err.Error(), "stalled: ") {
		t.Errorf("a first stall ended %v; want it tried again as a stall", err)
	}
	if ids := harness.filesToReadThrough(t); !slices.Equal(ids, []int64{1, 2}) {
		t.Errorf("after one stall on film 2, a sweep finds %v; want it asked again, the hang not its fault", ids)
	}
	if err := measure(true); err == nil || !strings.Contains(err.Error(), "FFmpeg stalled twice") {
		t.Errorf("a second stall in a row ended %v; want the file's failure", err)
	}
	if ids := harness.filesToReadThrough(t); !slices.Equal(ids, []int64{1}) {
		t.Errorf("after two stalls on film 2, a sweep finds %v; want only film 1", ids)
	}
}

func TestSubtitles_AStallIsTriedAgainAndTwoInARowAreTheTracksFailure(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveSubtitles(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	decided := harness.play(t, user, chrome(2))
	extract := func(previousAttemptStalled bool) error {
		job := jobs.Job{
			ID: 96, Kind: jobs.KindExtractSubtitle, Target: jobs.Target{FileID: subtitleFileID},
			PreviousAttemptToolFailed: previousAttemptStalled,
		}
		return answerAfterStall(t, harness, job, harness.subtitles.HandleExtractSubtitle, func() {
			harness.answerSubtitle(t, 96, nodeproto.SubtitleText{Stall: testStall})
		})
	}
	if err := extract(false); err == nil || !strings.HasPrefix(err.Error(), "stalled: ") {
		t.Errorf("a first stall ended %v; want it tried again as a stall", err)
	}
	if waiting := harness.getBytePath(t, decided.Subtitles[0].ASSURL, user, true, ""); waiting.status !=
		http.StatusTooEarly {
		t.Errorf("a track stalled once: %d %s; want 425 while it is tried again", waiting.status, waiting.body)
	}
	if err := extract(true); err == nil || !strings.Contains(err.Error(), "FFmpeg stalled twice") {
		t.Errorf("a second stall in a row ended %v; want the track's failure", err)
	}
	failed := harness.getBytePath(t, decided.Subtitles[0].ASSURL, user, true, "")
	if failed.status != http.StatusBadGateway || !strings.Contains(failed.body, "FFmpeg stalled twice") {
		t.Errorf("a track stalled twice: %d %s; want it failed, saying so", failed.status, failed.body)
	}
}

// A node that started again lost the measurement and the extraction it held: each is an attempt
// like any other's failure, tried again, and never the target's, however many in a row (D248).
func TestNodeWork_ItsNodeStartingAgainIsTriedAgainAndNeverTheTargetsFailure(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.shelveSubtitles(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	decided := harness.play(t, user, chrome(2))
	interrupt := func() { harness.exchanges.Interrupt(t.Context(), 1, "started again") }
	measure := func(previousAttemptToolFailed bool) error {
		job := jobs.Job{
			ID: 95, Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 2},
			PreviousAttemptToolFailed: previousAttemptToolFailed,
		}
		return answerAfterStall(t, harness, job, harness.readThrough.HandleReadThrough, interrupt)
	}
	extract := func(previousAttemptToolFailed bool) error {
		job := jobs.Job{
			ID: 96, Kind: jobs.KindExtractSubtitle, Target: jobs.Target{FileID: subtitleFileID},
			PreviousAttemptToolFailed: previousAttemptToolFailed,
		}
		return answerAfterStall(t, harness, job, harness.subtitles.HandleExtractSubtitle, interrupt)
	}
	for _, previousAttemptToolFailed := range []bool{false, true} {
		for kind, run := range map[string]func(bool) error{"a measurement": measure, "an extraction": extract} {
			if err := run(previousAttemptToolFailed); !errors.Is(err, corenodes.ErrNodeRestarted) {
				t.Errorf("%s whose node started again ended %v; want it tried again, the work lost", kind, err)
			}
		}
		if ids := harness.filesToReadThrough(t); !slices.Equal(ids, []int64{1, 2}) {
			t.Errorf("after restarts during film 2's measurement, a sweep finds %v; want it asked again", ids)
		}
	}
	track := harness.getBytePath(t, decided.Subtitles[0].ASSURL, user, true, "")
	if track.status == http.StatusBadGateway {
		t.Errorf("a track whose node lost its extraction twice: %d %s; want it not failed", track.status, track.body)
	}
}
