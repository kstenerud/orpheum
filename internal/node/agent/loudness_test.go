// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeMeasurer answers with each track's stream index as its loudness, and the path it was given
// as the source's range, so a test sees which file was measured.
type fakeMeasurer struct{}

func (fakeMeasurer) Measure(
	_ context.Context, videoPath string, tracks []nodeproto.LoudnessTrack,
) ([]nodeproto.TrackLoudness, error) {
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

func receiveLoudness(t *testing.T, core *fakeCore) nodeproto.Loudness {
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), patience)
	defer stop()
	select {
	case answer := <-core.loudness:
		return answer
	case <-ctx.Done():
		t.Fatal("no loudness reached the core")
	}
	return nodeproto.Loudness{}
}

func TestAgent_MeasuresOnlyTheVideoTheCoreNamed(t *testing.T) {
	t.Parallel()
	video, _, core := extracting(t)
	measure := func(job int64, named nodeproto.BytesFile) nodeproto.Loudness {
		command := nodeproto.MeasureLoudness{
			Job: job, Video: named, Audio: []nodeproto.LoudnessTrack{{StreamIndex: 1, Channels: 6}},
		}
		core.commands <- nodeproto.Command{Seq: job, Type: nodeproto.CommandMeasureLoudness, MeasureLoudness: &command}
		return receiveLoudness(t, core)
	}
	measured := measure(1, video)
	wantPath := video.Root + "/film.mkv"
	if measured.Job != 1 || measured.Refused != "" || len(measured.Tracks) != 1 ||
		measured.Tracks[0].Stereo.IntegratedLUFS != -1 || measured.Tracks[0].Stereo.RangeLU != float64(len(wantPath)) {
		t.Errorf("measuring the named video: %+v, want stream 1 of %s", measured, wantPath)
	}
	changed := video
	changed.Hash = strings.Repeat("0", 64)
	refused := measure(2, changed)
	if refused.Refused == "" || !strings.Contains(refused.Refused, "not the one named") {
		t.Errorf("measuring a changed video: %+v, want it refused (D100)", refused)
	}
}
