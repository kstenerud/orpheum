// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
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
	finished := make(chan error, 1)
	go func() { finished <- handle(t.Context(), job) }()
	harness.nextCommand(t)
	answer()
	//nolint:wrapcheck // the job's own error, which the test reads as it is
	return testutil.Receive(t, finished, "the job to end")
}

func TestLoudness_AStallIsTriedAgainAndTwoInARowAreTheFilesFailure(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	playback := harness.server.dependencies.Load().Playback
	measure := func(previousAttemptStalled bool) error {
		job := jobs.Job{
			ID: 95, Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: 2},
			PreviousAttemptToolFailed: previousAttemptStalled,
		}
		return answerAfterStall(t, harness, job, playback.HandleMeasureLoudness, func() {
			harness.answerLoudness(t, 95, nodeproto.LoudnessResult{Stall: testStall})
		})
	}
	if err := measure(false); err == nil || !strings.HasPrefix(err.Error(), "stalled: ") {
		t.Errorf("a first stall ended %v; want it tried again as a stall", err)
	}
	if ids := harness.filesToMeasure(t); !slices.Equal(ids, []int64{1, 2}) {
		t.Errorf("after one stall on film 2, a sweep finds %v; want it asked again, the hang not its fault", ids)
	}
	if err := measure(true); err == nil || !strings.Contains(err.Error(), "FFmpeg stalled twice") {
		t.Errorf("a second stall in a row ended %v; want the file's failure", err)
	}
	if ids := harness.filesToMeasure(t); !slices.Equal(ids, []int64{1}) {
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
	playback := harness.server.dependencies.Load().Playback
	extract := func(previousAttemptStalled bool) error {
		job := jobs.Job{
			ID: 96, Kind: jobs.KindExtractSubtitle, Target: jobs.Target{StreamID: subtitleStreamID},
			PreviousAttemptToolFailed: previousAttemptStalled,
		}
		return answerAfterStall(t, harness, job, playback.HandleExtractSubtitle, func() {
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

// A node that started again lost the measurement and the extraction it held (D238 §3): each is the
// tool's failure, as a stall is, tried again at no cost the first time and the target's failure on
// the second in a row.
func TestNodeWork_ItsNodeStartingAgainIsTriedAgainAndTwoInARowAreTheTargetsFailure(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.shelveAudioStreams(t)
	harness.shelveSubtitles(t)
	harness.nodes.commanded = make(chan nodeproto.Command, 2)
	decided := harness.play(t, user, chrome(2))
	playback := harness.server.dependencies.Load().Playback
	interrupt := func() { harness.exchanges.Interrupt(t.Context(), 1) }
	measure := func(previousAttemptToolFailed bool) error {
		job := jobs.Job{
			ID: 95, Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: 2},
			PreviousAttemptToolFailed: previousAttemptToolFailed,
		}
		return answerAfterStall(t, harness, job, playback.HandleMeasureLoudness, interrupt)
	}
	extract := func(previousAttemptToolFailed bool) error {
		job := jobs.Job{
			ID: 96, Kind: jobs.KindExtractSubtitle, Target: jobs.Target{StreamID: subtitleStreamID},
			PreviousAttemptToolFailed: previousAttemptToolFailed,
		}
		return answerAfterStall(t, harness, job, playback.HandleExtractSubtitle, interrupt)
	}
	for kind, run := range map[string]func(bool) error{"a measurement": measure, "an extraction": extract} {
		if err := run(false); err == nil || !strings.HasPrefix(err.Error(), "interrupted: ") {
			t.Errorf("%s whose node started again ended %v; want it tried again as the tool's failure", kind, err)
		}
	}
	if ids := harness.filesToMeasure(t); !slices.Equal(ids, []int64{1, 2}) {
		t.Errorf("after one restart during film 2's measurement, a sweep finds %v; want it asked again", ids)
	}
	for kind, run := range map[string]func(bool) error{"a measurement": measure, "an extraction": extract} {
		if err := run(true); err == nil || !strings.Contains(err.Error(), "the tool failed on two attempts in a row") {
			t.Errorf("%s's second tool failure in a row ended %v; want the target's failure", kind, err)
		}
	}
	if ids := harness.filesToMeasure(t); !slices.Equal(ids, []int64{1}) {
		t.Errorf("after two tool failures on film 2, a sweep finds %v; want only film 1", ids)
	}
	failed := harness.getBytePath(t, decided.Subtitles[0].ASSURL, user, true, "")
	if failed.status != http.StatusBadGateway || !strings.Contains(failed.body, "two attempts in a row") {
		t.Errorf("a track whose tool failed twice: %d %s; want it failed, saying so", failed.status, failed.body)
	}
}
