// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeMeasurer answers with each track's stream index as its loudness, and the path it was given
// as the source's range, so a test sees which file was measured; stream unmeasurable fails, as
// FFmpeg does on a file it cannot decode.
type fakeMeasurer struct{}

// unmeasurable is the stream index FFmpeg cannot measure.
const unmeasurable = 99

func (fakeMeasurer) Measure(
	_ context.Context, videoPath string, tracks []nodeproto.LoudnessTrack,
) ([]nodeproto.TrackLoudness, error) {
	if tracks[0].StreamIndex == stallingStreamIndex {
		return nil, fmt.Errorf("measure the loudness: %w", errStalled)
	}
	if tracks[0].StreamIndex == unmeasurable {
		return nil, errors.New("ffmpeg: Invalid data found when processing input")
	}
	measured := make([]nodeproto.TrackLoudness, 0, len(tracks))
	for _, track := range tracks {
		measured = append(measured, nodeproto.TrackLoudness{
			StreamIndex: track.StreamIndex,
			Stereo: nodeproto.Measurement{
				IntegratedLUFS: -float64(track.StreamIndex), RangeLU: float64(len(videoPath)),
			},
		})
	}
	return measured, nil
}

// jobLoudness is a loudness measurement with the job id its result carried.
type jobLoudness struct {
	JobID int64
	nodeproto.LoudnessResult
}

func receiveLoudness(t *testing.T, core *fakeCore) jobLoudness {
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), answerTimeout)
	defer stop()
	select {
	case result := <-core.loudness:
		return jobLoudness{JobID: result.JobID, LoudnessResult: *result.Loudness}
	case <-ctx.Done():
		t.Fatal("no loudness reached the core")
	}
	return jobLoudness{}
}

func TestAgent_MeasuresOnlyTheVideoTheCoreNamed(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	measure := func(job int64, named nodeproto.BytesFile, streamIndex int) jobLoudness {
		command := nodeproto.MeasureLoudness{
			Video: named, Tracks: []nodeproto.LoudnessTrack{{StreamIndex: streamIndex, ChannelCount: 6}},
		}
		core.commands <- nodeproto.Command{
			CommandSeq: job,
			JobID:      job,
			Work:       nodeproto.Work{Type: nodeproto.CommandMeasureLoudness, MeasureLoudness: &command},
		}
		return receiveLoudness(t, core)
	}
	measured := measure(1, video, 1)
	wantPath := video.RootPath + "/film.mkv"
	if measured.JobID != 1 || measured.Refusal != "" || len(measured.Tracks) != 1 ||
		measured.Tracks[0].Stereo.IntegratedLUFS != -1 || measured.Tracks[0].Stereo.RangeLU != float64(len(wantPath)) {
		t.Errorf("measuring the named video: %+v, want stream 1 of %s", measured, wantPath)
	}
	changed := video
	changed.PayloadHash = strings.Repeat("0", 64)
	refused := measure(2, changed, 1)
	if refused.Refusal == "" || !strings.Contains(refused.Refusal, "not the one named") || refused.Failure != "" ||
		refused.RefusalCode != nodeproto.RefusedChanged {
		t.Errorf("measuring a changed video: %+v, want it refused, not failed (D100)", refused)
	}
	failed := measure(3, video, unmeasurable)
	if failed.Failure == "" || !strings.Contains(failed.Failure, "Invalid data") || failed.Refusal != "" {
		t.Errorf("a video FFmpeg cannot measure: %+v, want FFmpeg's failure, which is the file's", failed)
	}
}
