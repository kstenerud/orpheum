// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// framecrcOfAPicture is framecrc's output for a picture in a 1/90000 time base: a keyframe at 0,
// B-frames shown before the P-frame decoded ahead of them, a keyframe at 2.0 s written twice, a
// corrupt keyframe, a packet with no time, and side data after a keyframe's line.
const framecrcOfAPicture = "#extradata 0:       41, 0x22a30dd9\n" +
	"#software: Lavf62.12.102\n" +
	"#tb 0: 1/90000\n" +
	"#media_type 0: video\n" +
	"0,      -3003,          0,     3003,     5000, 0x979a3453\n" +
	"0,          0,      12012,     3003,      300, 0xaa8105ed, F=0x0\n" +
	"0,       3003,       3003,     3003,      200, 0x6b640488, F=0x0\n" +
	"0,       6006,       6006,     3003,      200, 0x58e80443, F=0x0\n" +
	"0,      90000,      90000,     3003,      700, 0x5fb3047c, F=0x0\n" +
	"0,     180000,     180000,     3003,     4000, 0xfbf107e7, S=1,        8, 0x00010001\n" +
	"0,     180001,     180000,     3003,       10, 0x74a004bf\n" +
	"0,     270000,     270000,     3003,     3000, 0x5d130462, F=0x3\n" +
	"0, -9223372036854775808, -9223372036854775808, 3003, 50, 0x68ae04c3, F=0x0\n" +
	"0,     360000,     360000,     3003,     4500, 0xd91306dd, F=0x11\n"

func TestPacketLines_KeepTheKeyframesInOrderOnceAndTheBytesOfEachSecond(t *testing.T) {
	t.Parallel()
	var lines packetLines
	// FFmpeg's writes end anywhere, mid-line among them.
	for chunk := range slices.Chunk([]byte(framecrcOfAPicture), 37) {
		if written, err := lines.Write(chunk); err != nil || written != len(chunk) {
			t.Fatalf("Write took %d of %d bytes: %v", written, len(chunk), err)
		}
	}
	timeline, err := lines.timeline()
	if err != nil {
		t.Fatal(err)
	}
	// The keyframe at 3.0 s is corrupt; the one at 4.0 s is a keyframe and disposable.
	if !slices.Equal(timeline.KeyframesMs, []int64{0, 2000, 4000}) {
		t.Errorf("the keyframes are %v ms; want 0, 2000 and 4000: in order, each once, a corrupt one left out",
			timeline.KeyframesMs)
	}
	if !slices.Equal(timeline.BytesInEachSecond, []int64{5700, 700, 4010, 3000, 4500}) {
		t.Errorf("the bytes in each second are %v; want every timed packet's bytes in the second it is shown in",
			timeline.BytesInEachSecond)
	}
}

// A keyframe an MP4's edit shows before the film's start, flagged discarded (F=0x5), is the film's
// first keyframe at its start when no other is at or before it, since the film's first pictures are
// decoded from it; one discarded after the start, or with a keyframe at the start, is not (A Perfect
// World, staging, 2026-11-04).
func TestPacketLines_AKeyframeDiscardedBeforeTheStartIsTheFilmsFirstAtItsStart(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		packets string
		want    []int64
	}{
		{"before the start", "0, -100, -100, 40, 900, 0x1, F=0x5\n0, -50, -50, 40, 9, 0x2, F=0x4\n" +
			"0, 0, 0, 40, 9, 0x3, F=0x0\n0, 10176, 10176, 40, 900, 0x4\n", []int64{0, 10176}},
		{"after the start", "0, 0, 0, 40, 9, 0x3, F=0x0\n0, 500, 500, 40, 900, 0x1, F=0x5\n" +
			"0, 10176, 10176, 40, 900, 0x4\n", []int64{10176}},
		{"with one at the start", "0, -100, -100, 40, 900, 0x1, F=0x5\n0, 0, 0, 40, 900, 0x4\n" +
			"0, 2000, 2000, 40, 900, 0x5\n", []int64{0, 2000}},
	} {
		var lines packetLines
		if _, err := lines.Write([]byte("#tb 0: 1/1000\n" + test.packets)); err != nil {
			t.Fatal(err)
		}
		timeline, err := lines.timeline()
		if err != nil || !slices.Equal(timeline.KeyframesMs, test.want) {
			t.Errorf("a discarded keyframe %s: the keyframes are %v, %v; want %v", test.name,
				timeline.KeyframesMs, err, test.want)
		}
	}
}

func TestPacketLines_WhatFFmpegWroteThatCannotBeReadIsSaid(t *testing.T) {
	t.Parallel()
	for name, written := range map[string]string{
		"no time base":       "0, 0, 0, 1, 100, 0x1\n",
		"a broken line":      "#tb 0: 1/1000\n0, 0, zero, 1, 100, 0x1\n",
		"a broken time base": "#tb 0: 1/0\n0, 0, 0, 1, 100, 0x1\n",
		"a packet in a week": "#tb 0: 1/1000\n0, 0, 604800000, 1, 100, 0x1\n",
		"nothing at all":     "",
	} {
		var lines packetLines
		if _, err := lines.Write([]byte(written)); err != nil {
			t.Fatal(err)
		}
		if timeline, err := lines.timeline(); err == nil {
			t.Errorf("%s: read as %+v; want it said that it cannot be read", name, timeline)
		}
	}
}

func TestPacketLines_APictureWithNoKeyframeHasAnEmptyTimeline(t *testing.T) {
	t.Parallel()
	var lines packetLines
	if _, err := lines.Write([]byte("#tb 0: 1/1000\n0, 0, 0, 40, 100, 0x1, F=0x0")); err != nil {
		t.Fatal(err)
	}
	timeline, err := lines.timeline()
	if err != nil || timeline.KeyframesMs == nil || len(timeline.KeyframesMs) != 0 ||
		!slices.Equal(timeline.BytesInEachSecond, []int64{100}) {
		t.Errorf("a picture with no keyframe, its last line unended: %+v, %v; want no keyframes, as an empty "+
			"list, and its one packet's bytes", timeline, err)
	}
}

func TestRead_TheTimelineComesFromTheSameReadAsTheLoudness(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{
		Stdout: framecrcOfAPicture, Stderr: testutil.EBUR128Summary("stereo_2", "-20.0", "3.0", "-9.0"),
	})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	findings, err := NewReader(runner, 20<<20).Read(t.Context(), execx.Film{Path: "/media/film.mkv"},
		[]nodeproto.LoudnessTrack{{StreamIndex: 2, ChannelCount: 2}},
		&nodeproto.VideoTimelineAsk{VideoStreamIndex: 1}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	arguments := strings.Join(fake.OnlyCall(t).Arguments, " ")
	if !strings.Contains(arguments, "-f null - -map 0:1 -c copy -f framecrc pipe:1") ||
		strings.Count(arguments, "-i ") != 1 {
		t.Errorf("ran ffmpeg %q; want one read of the file into the loudness's null output and the picture, "+
			"stream 1 of the file, copied into framecrc on stdout", arguments)
	}
	if len(findings.Loudness) != 1 || findings.Loudness[0].Stereo.IntegratedLUFS != -20 ||
		findings.VideoTimeline == nil || !slices.Equal(findings.VideoTimeline.KeyframesMs, []int64{0, 2000, 4000}) {
		t.Errorf("found %+v; want the track's loudness and the picture's keyframes from the one run", findings)
	}

	onlyTimeline := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: framecrcOfAPicture})
	timelineRunner, err := execx.NewRunner(onlyTimeline.Directory)
	if err != nil {
		t.Fatal(err)
	}
	findings, err = NewReader(timelineRunner, 20<<20).Read(t.Context(), execx.Film{Path: "/media/film.mkv"}, nil,
		&nodeproto.VideoTimelineAsk{VideoStreamIndex: 0}, 0, nil)
	arguments = strings.Join(onlyTimeline.OnlyCall(t).Arguments, " ")
	if err != nil || findings.Loudness != nil || findings.VideoTimeline == nil ||
		strings.Contains(arguments, "ebur128") || !strings.HasSuffix(arguments, "-map 0:0 -c copy -f framecrc pipe:1") {
		t.Errorf("a read asked only for the timeline ran %q and found %+v, %v; want only the picture copied",
			arguments, findings, err)
	}
}
