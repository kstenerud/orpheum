// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestRead_ReadsOnlyTheFileAndMeasuresEachSoundByName(t *testing.T) {
	t.Parallel()
	logged := "[aac @ 0x1] a warning while decoding\n" +
		testutil.EBUR128Summary("multichannel_1", "-24.5", "18.2", "-1.3") +
		testutil.EBUR128Summary("stereo_1", "-31.9", "17.0", "-6.4") +
		testutil.EBUR128Summary("stereo_2", "-70.0", "0.0", "-inf") +
		"[out#0/null @ 0x2] video:0KiB audio:1KiB\n"
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stderr: logged})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := NewReader(runner, 20<<20).Read(t.Context(), "/media/Film (2020)/film.mkv",
		[]nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 6}, {StreamIndex: 2, ChannelCount: 2}}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	arguments := fake.OnlyCall(t).Arguments
	checkArguments(t, arguments)
	if slices.Contains(arguments, "framecrc") || findings.VideoTimeline != nil {
		t.Errorf("a read asked only for loudness ran %q and found a timeline %+v; want neither", arguments,
			findings.VideoTimeline)
	}
	measured := findings.Loudness
	if len(measured) != 2 || measured[0].Multichannel == nil || measured[0].Multichannel.IntegratedLUFS != -24.5 ||
		*measured[0].Multichannel.TruePeakDBTP != -1.3 || measured[0].Stereo.IntegratedLUFS != -31.9 ||
		measured[0].Stereo.RangeLU != 17.0 || measured[1].Multichannel != nil ||
		measured[1].Stereo.IntegratedLUFS != -70 || measured[1].Stereo.TruePeakDBTP != nil {
		t.Errorf("measured %+v; want the 5.1 track as it is and as stereo, and the silent one with no peak", measured)
	}
}

// checkArguments fails unless FFmpeg read only the file, by the file protocol, and sent each sound
// of stream 1 (5.1) and stream 2 (stereo) through its own named filter, the stereo mix prepare's own.
func checkArguments(t *testing.T, arguments []string) {
	t.Helper()
	input := slices.Index(arguments, "-i")
	graph := arguments[slices.Index(arguments, "-filter_complex")+1]
	if input < 2 || arguments[input-2] != "-protocol_whitelist" || arguments[input-1] != "file" ||
		arguments[input+1] != "file:/media/Film (2020)/film.mkv" || !slices.Contains(arguments, "null") {
		t.Errorf("ran ffmpeg %q; want the file read with only the file protocol, into the null muxer", arguments)
	}
	for _, want := range []string{
		"ebur128@multichannel_1=peak=true", prepare.StandardMix + ",ebur128@stereo_1=peak=true",
		"[0:2]" + prepare.StereoAsIsMix + ",ebur128@stereo_2=peak=true",
	} {
		if !strings.Contains(graph, want) {
			t.Errorf("the graph %q lacks %q: each sound through its own named filter, the stereo mix prepare's own "+
				"(D226 §1)", graph, want)
		}
	}
}

func TestRead_ATrackFFmpegSaidNothingOfIsAFailure(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t,
		testutil.FakeReply{Stderr: testutil.EBUR128Summary("stereo_1", "-20.0", "3.0", "-9.0")})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewReader(runner, 20<<20).Read(t.Context(), "/media/film.mkv",
		[]nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}, {StreamIndex: 3, ChannelCount: 2}}, nil, 0)
	if err == nil || !strings.Contains(err.Error(), "stream 3") {
		t.Errorf("with no summary for stream 3: %v, want a failure naming it", err)
	}
}

func TestRead_ARunAskedForNothingIsRefusedWithoutRunning(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReader(runner, 20<<20).Read(t.Context(), "/media/film.mkv", nil, nil, 0); err == nil {
		t.Error("a read asked for nothing succeeded; want it refused")
	}
	if calls := fake.Calls(t); len(calls) != 0 {
		t.Errorf("a read asked for nothing ran FFmpeg %d times; want none", len(calls))
	}
}
