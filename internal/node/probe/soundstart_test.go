// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// probeCalls are the ffprobe runs of a probe of "/media/film.mp4", each answered with output.
func probeCalls(t *testing.T, output []byte) []testutil.FakeCall {
	t.Helper()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: string(output)})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewProber(runner, time.Minute).Probe(t.Context(), execx.Film{Path: "/media/film.mp4"}); err != nil {
		t.Fatal(err)
	}
	return fake.Calls(t)
}

// checkListedAlone checks that one of calls lists the first packets of stream 1, the sound, alone,
// of the film by the file: protocol.
func checkListedAlone(t *testing.T, calls []testutil.FakeCall) {
	t.Helper()
	at := slices.IndexFunc(calls, func(call testutil.FakeCall) bool {
		return slices.Contains(call.Arguments, "-select_streams")
	})
	if at < 0 {
		t.Fatalf("no run listed a stream alone: %+v", calls)
	}
	alone := calls[at].Arguments
	if index := slices.Index(alone, "-select_streams"); alone[index+1] != "1" {
		t.Errorf("the second run's arguments %q do not list the sound's stream alone", alone)
	}
	if alone[len(alone)-1] != "file:/media/film.mp4" {
		t.Errorf("the second run's arguments %q do not name the film by the file: protocol", alone)
	}
}

// soundFrameTicks is an AAC frame at 44.1 kHz, in its stream's time base of 1/44100.
const soundFrameTicks = 1024

// ffprobeOutputOfSound is ffprobe's output for a file of a picture and an AAC sound at 44.1 kHz whose
// first packets last durations, in ticks of 1/44100, each after a picture packet; nil for a packet
// saying none.
func ffprobeOutputOfSound(t *testing.T, durations []*int64) []byte {
	t.Helper()
	type packet struct {
		StreamIndex int    `json:"stream_index"`
		Pts         int64  `json:"pts"`
		Duration    *int64 `json:"duration,omitempty"`
	}
	picture := int64(512)
	packets := make([]packet, 0, 2*len(durations))
	sound := int64(0)
	for index, duration := range durations {
		packets = append(packets, packet{StreamIndex: 0, Pts: int64(index) * picture, Duration: &picture},
			packet{StreamIndex: 1, Pts: sound, Duration: duration})
		if duration != nil {
			sound += *duration
		}
	}
	output, err := json.Marshal(map[string]any{
		"format": map[string]any{"format_name": "mov,mp4,m4a,3gp,3g2,mj2"},
		"streams": []map[string]any{
			{"index": 0, "codec_type": "video", "codec_name": "h264", "time_base": "1/12288"},
			{"index": 1, "codec_type": "audio", "codec_name": "aac", "time_base": "1/44100", "sample_rate": "44100"},
		},
		"packets": packets,
	})
	if err != nil {
		t.Fatal(err)
	}
	return output
}

// soundPackets are durations of a sound's first packets: first, then count frames.
func soundPackets(first int64, count int) []*int64 {
	durations := []*int64{&first}
	for range count {
		frame := int64(soundFrameTicks)
		durations = append(durations, &frame)
	}
	return durations
}

// A sound whose first packet alone lasts seconds, each packet after it a frame, is marked with how much
// longer the first lasts than the second, as file 18306's does (D534 §1): its first lasting 31.9 s.
// Nothing else is: a first packet of a frame, or of under a second, or followed by long packets, or too
// few read to judge by.
func TestParse_ASoundWhoseFirstPacketAloneLastsSecondsIsMarkedWithTheStretch(t *testing.T) {
	t.Parallel()
	unknown := soundPackets(1407883, 20)
	unknown[5] = nil
	long := soundPackets(1407883, 20)
	longer := int64(44100)
	long[7] = &longer
	for _, test := range []struct {
		name      string
		durations []*int64
		want      int64
	}{
		{"a first packet of 31.9 s, as file 18306's", soundPackets(1407883, 20), 31902},
		{"a first packet of 5 s", soundPackets(220500, 20), 4977},
		{"every packet a frame", soundPackets(soundFrameTicks, 20), 0},
		{"a first packet of under a second", soundPackets(40000, 20), 0},
		{"a packet after it lasting a second", long, 0},
		{"a packet after it saying no length", unknown, 0},
		{"too few after it read to judge by", soundPackets(1407883, 5), 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			record, err := Parse(ffprobeOutputOfSound(t, test.durations))
			if err != nil {
				t.Fatal(err)
			}
			if got := record.Streams[1].Audio.FirstPacketStretchMs; got != test.want {
				t.Errorf("the first packet's stretch is %d ms, want %d", got, test.want)
			}
			if record.Streams[0].Audio.FirstPacketStretchMs != 0 {
				t.Error("the picture is marked with a sound's stretch")
			}
		})
	}
}

// A sound whose first packet, among the file's first ffprobe lists, lasts seconds, with too few of its
// packets after it listed to judge by, is listed alone, by its stream, as file 18306's is: its first
// 200 packets hold one of its sound, the picture's filling the 31.9 s it lasts. A file whose sound is
// judged by the first packets is probed once.
func TestProbe_ASoundWhoseLongFirstPacketTheFirstPacketsCannotJudgeIsListedAlone(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		packets  []*int64
		wantRuns int
	}{
		{"its first packet's followers unlisted", soundPackets(1407883, 3), 2},
		{"its first packet's followers listed", soundPackets(1407883, 20), 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := probeCalls(t, ffprobeOutputOfSound(t, test.packets))
			if len(calls) != test.wantRuns {
				t.Fatalf("ffprobe ran %d times, want %d", len(calls), test.wantRuns)
			}
			if test.wantRuns > 1 {
				checkListedAlone(t, calls)
			}
		})
	}
}
