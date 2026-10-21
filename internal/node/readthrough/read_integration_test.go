// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package readthrough_test

import (
	"math"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/readthrough"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A 1 kHz sine of amplitude 0.1 in one channel is -23.0 LUFS by BS.1770's definition, and each
// further channel of it adds its power. So a stereo track of it is -20.0 LUFS, a 5.1 track with it
// in the front three is -18.2, and that track's standard stereo mix, (L + 0.707 C) / 2.414 on each
// side, is -23.0: renormalised, 4.8 dB quieter than the track itself (D226 §1). Its picture, 25
// frames a second, has a keyframe every 50 frames: at 0, 2, 4, 6 and 8 seconds.
const (
	tone    = "0.1*sin(2*PI*1000*t)"
	silence = "0"
)

func TestRead_AToneOfKnownLevelMeasuresAsItIsAndItsPictureHasItsKeyframes(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	film := filepath.Join(t.TempDir(), "film.mkv")
	_, err = runner.Run(t.Context(), execx.FFmpeg, confine.Access{}, []string{
		"-v", "error", "-f", "lavfi", "-i", "color=c=black:s=64x64:d=10",
		"-f", "lavfi", "-i", "aevalsrc=" + tone + "|" + tone + "|" + tone + "|" + silence + "|" + silence + "|" +
			silence + ":channel_layout=5.1:d=10",
		"-f", "lavfi", "-i", "aevalsrc=" + tone + "|" + tone + ":channel_layout=stereo:d=10",
		"-map", "0", "-map", "1", "-map", "2", "-c:v", "libx264", "-preset", "ultrafast",
		"-x264-params", "keyint=50:min-keyint=50:scenecut=0", "-c:a", "flac",
		"file:" + film,
	}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := readthrough.NewReader(runner, 20<<20).Read(t.Context(), film,
		[]nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 6}, {StreamIndex: 2, ChannelCount: 2}},
		&nodeproto.VideoTimelineAsk{VideoStreamIndex: 0}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkTimeline(t, findings.VideoTimeline)
	measured := findings.Loudness
	if len(measured) != 2 || measured[0].Multichannel == nil {
		t.Fatalf("measured %+v; want the 5.1 track as it is and as stereo, and the stereo track", measured)
	}
	for _, check := range []struct {
		what        string
		measurement nodeproto.Measurement
		lufs, peak  float64
	}{
		{"the 5.1 track as it is", *measured[0].Multichannel, -18.2, -20.0},
		{"the 5.1 track's stereo mix", measured[0].Stereo, -23.0, -23.0},
		{"the stereo track", measured[1].Stereo, -20.0, -20.0},
	} {
		got := check.measurement
		if math.Abs(got.IntegratedLUFS-check.lufs) > 0.3 || got.TruePeakDBTP == nil ||
			math.Abs(*got.TruePeakDBTP-check.peak) > 0.3 {
			t.Errorf("%s: %+v; want %.1f LUFS with a true peak of %.1f dBTP", check.what, got, check.lufs, check.peak)
		}
	}
}

// checkTimeline fails unless the timeline has the picture's five keyframes and its ten seconds,
// each second's bytes counted, a second with a keyframe heavier than the one after it.
func checkTimeline(t *testing.T, timeline *nodeproto.VideoTimeline) {
	t.Helper()
	if timeline == nil {
		t.Fatal("the read found no timeline")
	}
	if !slices.Equal(timeline.KeyframesMs, []int64{0, 2000, 4000, 6000, 8000}) {
		t.Errorf("the keyframes are %v ms; want every two seconds from 0 to 8000", timeline.KeyframesMs)
	}
	seconds := timeline.BytesInEachSecond
	if len(seconds) != 10 || slices.Contains(seconds, 0) {
		t.Fatalf("the bytes in each second are %v; want ten seconds, each with bytes", seconds)
	}
	for second := 0; second < 10; second += 2 {
		if seconds[second] <= seconds[second+1] {
			t.Errorf("second %d, with a keyframe, has %d bytes, and the one after it %d; want the keyframe's "+
				"second heavier", second, seconds[second], seconds[second+1])
		}
	}
}
