// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
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
	<-harness.nodes.commanded
	answer()
	return <-finished
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
			playback.TakeLoudness(t.Context(), 1, nodeproto.LoudnessResult{JobID: 95, Stall: testStall})
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
			playback.TakeSubtitleText(t.Context(), 1, nodeproto.SubtitleText{JobID: 96, Stall: testStall})
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
