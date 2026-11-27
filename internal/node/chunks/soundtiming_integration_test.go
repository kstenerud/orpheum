// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package chunks_test

import (
	"log/slog"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/chunks"
	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/node/probe"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// soundFrameSeconds is an AAC frame at 44.1 kHz.
const soundFrameSeconds = 1024.0 / 44100

// soundStretchSeconds is how late filmWhoseSoundStartsLate's sound is muxed, its first packet lasting
// until then.
const soundStretchSeconds = 5

// filmWhoseSoundStartsLate is a twelve-second MP4 of H.264 and stereo AAC at 44.1 kHz whose first
// sound packet lasts soundStretchSeconds and every other one 23 ms, as file 18306's lasts 31.9 s
// (staging, 2026-11-22; D534): taken at its times, its sound starts that late. Its sound, past the
// first packet, a frame of the encoder's priming, is a tone from 1 s to 2 s and from 6.5 s to 7 s.
func filmWhoseSoundStartsLate(t *testing.T) string {
	t.Helper()
	sound := filepath.Join(t.TempDir(), "timing.m4a")
	testutil.GenerateMedia(t, sound,
		"-f", "lavfi",
		"-i", "aevalsrc=if(between(t\\,1\\,2)+between(t\\,6.5\\,7)\\,sin(2*PI*1000*t)/2\\,0):s=44100:d=12",
		"-c:a", "aac", "-ac", "2")
	path := filepath.Join(t.TempDir(), "a film whose sound starts late (2020).mp4")
	// Without an edit list the MP4 muxer starts the sound at the film's start, stretching its first
	// packet, the encoder's priming, over the offset. The sound runs on past the picture's end.
	testutil.GenerateMedia(t, path,
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=12",
		"-itsoffset", strconv.Itoa(soundStretchSeconds), "-i", "file:"+sound,
		"-map", "0:v", "-map", "1:a", "-c:v", "libx264", "-preset", "fast", "-bf", "0", "-g", "48",
		"-keyint_min", "48", "-sc_threshold", "0", "-c:a", "copy", "-use_editlist", "0")
	return path
}

// The sound made of a film whose first sound packet alone lasts five seconds, as its probe says, starts
// with the picture, what follows the first packet straight after it: its first tone at its time in the
// sound, a second and a frame in, made from the film's start, and its second, 6.5 s and a frame in, made
// by a run led in from 4 s, seeking to 2 s, both cut at their chunks' bounds. Taken at the file's
// times, the made sound was five seconds late, silent for them (D534 §1; file 18306, staging,
// 2026-11-22).
func TestMake_AMadeSoundOfAStretchedFirstPacketStartsWithThePicture(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	source := filmWhoseSoundStartsLate(t)
	if times := sourceTimes(t, runner, source, "a:0", false); len(times) < 3 ||
		math.Abs(times[1]-times[0]-soundStretchSeconds) > 0.1 || math.Abs(times[2]-times[1]-soundFrameSeconds) > 0.001 {
		t.Fatalf("the film's sound packets start at %v s; want the first lasting %d s, the next a frame",
			times[:min(3, len(times))], soundStretchSeconds)
	}
	record, err := probe.NewProber(runner, time.Minute).Probe(t.Context(), execx.Film{Path: source})
	if err != nil {
		t.Fatal(err)
	}
	stretch := record.Streams[1].Audio.FirstPacketStretchMs
	if stretch == 0 {
		t.Fatalf("the probe says the film's sound has no stretched first packet: %+v", record.Streams[1])
	}
	grid := keyframeGrid(t, runner, source)
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264, slog.New(slog.DiscardHandler))
	storeDir := "1/chunks-sound0/0123456789abcdef"
	makeInTwoRunsOf(t, maker, source, grid, nodeproto.MakeChunks{
		Stream: nodeproto.ChunkStream{
			Kind: nodeproto.StreamSound, SoundKbps: 128, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
		},
		StoreDir: storeDir, SoundFirstPacketStretchMs: stretch,
	})
	made := joined(t, filepath.Join(directory, storeDir), len(grid)-1)
	offset := float64(chunks.TimelineOffsetMs) / 1000
	for _, tone := range []struct {
		after, want float64
		run         string
	}{{0, 1 + soundFrameSeconds, "from the film's start"}, {4, 6.5 + soundFrameSeconds, "led in from 4 s"}} {
		if at := toneOnsetAfter(t, runner, made, tone.after) - offset; math.Abs(at-tone.want) > 0.010 {
			t.Errorf("the sound made %s has its tone at %.3f s in the film; want the sound's %.3f s",
				tone.run, at, tone.want)
		}
	}
}

// soundGapStartSeconds is when filmWhoseSoundHasAGap's sound has its gap, of soundGapSeconds.
const (
	soundGapStartSeconds = 1
	soundGapSeconds      = 3
)

// filmWhoseSoundHasAGap is a twelve-second MP4 of H.264 and stereo AAC at 44.1 kHz whose sound,
// silent, stops a second in and goes on three seconds later, its packets timed so, every one a frame
// long but the one before the gap, which lasts over it; then, two seconds into the sound, a tone,
// five seconds and a frame, its priming, into the film.
func filmWhoseSoundHasAGap(t *testing.T) string {
	t.Helper()
	sound := filepath.Join(t.TempDir(), "soundtiming.m4a")
	testutil.GenerateMedia(t, sound,
		"-f", "lavfi", "-i", "aevalsrc=if(gte(t\\,2)\\,sin(2*PI*1000*t)/2\\,0):s=44100:d=8",
		"-c:a", "aac", "-ac", "2")
	path := filepath.Join(t.TempDir(), "a film whose sound has a gap (2020).mp4")
	testutil.GenerateMedia(t, path,
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=12", "-i", "file:"+sound,
		"-map", "0:v", "-map", "1:a", "-c:v", "libx264", "-preset", "fast", "-bf", "0", "-g", "48",
		"-keyint_min", "48", "-sc_threshold", "0", "-c:a", "copy",
		"-bsf:a", "setts=ts=if(gte(TS\\,"+strconv.Itoa(soundGapStartSeconds*44100)+")\\,TS+"+
			strconv.Itoa(soundGapSeconds*44100)+"\\,TS)", "-use_editlist", "0", "-t", "12")
	return path
}

// The sound made of a film whose sound has a gap three seconds long, a second in, is cut at its chunks'
// bounds, silent over the gap, its tone at its time in the film, made from the film's start and by a
// run whose seek lands inside the gap: FFmpeg's encode counted samples, not timestamps, so the run's
// chunk 0 held what follows the gap three seconds early, and was refused at its bounds, and a run
// seeking into the gap put the sound after it early (D533 §1; idiosyncrasies.md). A gap at the sound's
// start, its first packet lasting it, is no gap but a wrong time (D534 §1;
// TestMake_AMadeSoundOfAStretchedFirstPacketStartsWithThePicture).
func TestMake_AMadeSoundFillsAGapInTheFilmsSoundWithSilence(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	source := filmWhoseSoundHasAGap(t)
	times := sourceTimes(t, runner, source, "a:0", false)
	gapAt := slices.IndexFunc(times[1:], func(at float64) bool { return at > soundGapStartSeconds+0.5 })
	if math.Abs(times[1]-times[0]-soundFrameSeconds) > 0.001 || gapAt < 0 ||
		math.Abs(times[gapAt+1]-times[gapAt]-soundGapSeconds-soundFrameSeconds) > 0.001 {
		t.Fatalf("the film's sound packets start at %v s; want a frame each, then a gap of %d s at %d s",
			times[:min(3, len(times))], soundGapSeconds, soundGapStartSeconds)
	}
	grid := keyframeGrid(t, runner, source)
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264, slog.New(slog.DiscardHandler))
	storeDir := "1/chunks-sound0/0123456789abcdef"
	// The first run reads past the gap; the second, led in from 4 s, seeks to 2 s, inside it.
	makeInTwoRuns(t, maker, source, storeDir, grid, nodeproto.ChunkStream{
		Kind: nodeproto.StreamSound, SoundKbps: 128, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
	})
	made := toneOnset(t, runner, joined(t, filepath.Join(directory, storeDir), len(grid)-1)) -
		float64(chunks.TimelineOffsetMs)/1000
	if want := 2 + soundGapSeconds + soundFrameSeconds; math.Abs(made-want) > 0.025 {
		t.Errorf("the sound made has its tone at %.3f s in the film; want the film's %.3f s", made, want)
	}
}
