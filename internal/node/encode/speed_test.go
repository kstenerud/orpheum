// SPDX-License-Identifier: AGPL-3.0-or-later

package encode_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
)

// fakeSpeedRunner is a machine whose speed trial reports speeds, one progress block each, and then
// ends with err; it keeps the run it was given.
type fakeSpeedRunner struct {
	speeds []float64
	err    error
	run    execx.WatchedRun
}

func (machine *fakeSpeedRunner) RunWatched(_ context.Context, run execx.WatchedRun) (execx.Ran, error) {
	machine.run = run
	for _, speed := range machine.speeds {
		run.Watch.OnProgress(execx.Progress{RealtimeFactor: speed})
	}
	return execx.Ran{}, machine.err
}

// The speed trial encodes the lowest quality with the node's encoder, confined as its runs are, and
// its speed is FFmpeg's last word on it, in percent of real time (D457).
func TestMeasureSpeed_IsFFmpegsLastSpeedEncodingTheLowestQuality(t *testing.T) {
	t.Parallel()
	slow := &fakeSpeedRunner{speeds: []float64{0, 0.9, 0.62}}
	percent, err := encode.MeasureSpeed(t.Context(), slow, encode.EncoderX264)
	if err != nil || percent != 62 {
		t.Errorf("a trial last saying 0.62x measured %d%%, %v; want 62%%", percent, err)
	}
	if got := after(slow.run.Arguments, "-vf"); got != "scale=-2:360" {
		t.Errorf("the trial scales with %q; want the lowest quality's height, 360", got)
	}
	if got := after(slow.run.Arguments, "-b:v"); got != "400k" {
		t.Errorf("the trial encodes at %q; want the lowest quality's rate, 400k", got)
	}
	if got := after(slow.run.Arguments, "-i"); got != "testsrc2=size=1920x1080:rate=24:duration=10" {
		t.Errorf("the trial reads %q; want ten seconds of a generated 1080p picture", got)
	}
	if slow.run.Access.UsesGPU {
		t.Errorf("x264's trial was granted the GPU: %+v", slow.run.Access)
	}
	nvidia := &fakeSpeedRunner{speeds: []float64{12}}
	percent, err = encode.MeasureSpeed(t.Context(), nvidia, encode.EncoderNVENC)
	if err != nil || percent != 1200 || !nvidia.run.Access.UsesGPU ||
		after(nvidia.run.Arguments, "-c:v") != "h264_nvenc" {
		t.Errorf("NVENC's trial measured %d%%, %v, with %+v; want 1200%%, on NVENC with the GPU granted",
			percent, err, nvidia.run)
	}
}

// A trial its timeout cut short gives the speed it had reached, as a machine too slow to finish it
// is the one the trial is for; one that never said a speed measured nothing.
func TestMeasureSpeed_ATrialCutShortGivesTheSpeedItReachedAndOneThatSaidNoneFails(t *testing.T) {
	t.Parallel()
	cut := &fakeSpeedRunner{speeds: []float64{0.2}, err: context.DeadlineExceeded}
	if percent, err := encode.MeasureSpeed(t.Context(), cut, encode.EncoderX264); err != nil || percent != 20 {
		t.Errorf("a trial cut short at 0.2x measured %d%%, %v; want 20%%", percent, err)
	}
	failed := &fakeSpeedRunner{err: errors.New("exit status 1")}
	if percent, err := encode.MeasureSpeed(t.Context(), failed, encode.EncoderX264); err == nil || percent != 0 {
		t.Errorf("a trial that said no speed measured %d%%, %v; want an error", percent, err)
	}
	silent := &fakeSpeedRunner{}
	if _, err := encode.MeasureSpeed(t.Context(), silent, encode.EncoderX264); err == nil {
		t.Error("a trial that ended saying no speed measured one; want an error")
	}
}
