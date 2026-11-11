// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package chunks_test

import (
	"encoding/binary"
	"log/slog"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/chunks"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// toneAtMs is when aviWithSoundAheadOfItsPicture's sound, silent before, starts its tone.
const toneAtMs = 3_000

// aviWithSoundAheadOfItsPicture makes a twelve-second AVI as XviD and VirtualDub made them: an MPEG-4
// Part 2 picture with B-frames at 25 a second, and stereo MP3 at 48 kHz interleaved half a second
// ahead of it, its first half second before the picture's first packet in the file (an episode of the
// owner's series, staging, 2026-11-01). The sound is silent until a tone at toneAtMs.
func aviWithSoundAheadOfItsPicture(t *testing.T) string {
	t.Helper()
	sound := filepath.Join(t.TempDir(), "a tone at three seconds.mp3")
	testutil.GenerateMedia(t, sound,
		"-f", "lavfi", "-i", "aevalsrc=if(gte(t\\,3)\\,sin(2*PI*1000*t)/2\\,0):s=48000:d=12.5",
		"-c:a", "libmp3lame", "-b:a", "128k", "-ac", "2")
	path := filepath.Join(t.TempDir(), "a film (1986).avi")
	// The sound's packets timed half a second early are interleaved half a second ahead of the
	// picture's; an AVI's MP3 is timed by its bytes, so in the file it still starts with the picture.
	testutil.GenerateMedia(t, path,
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=25:duration=12",
		"-itsoffset", "-0.5", "-i", "file:"+sound,
		"-map", "0:v", "-map", "1:a", "-c:v", "mpeg4", "-tag:v", "XVID", "-bf", "2", "-g", "50", "-c:a", "copy",
		"-t", "12")
	return path
}

// toneOnset is when the sound of path's first audio stream reaches its tone, in seconds on path's own
// timeline: its first packet's time and the samples decoded before the tone.
func toneOnset(t *testing.T, runner *execx.Runner, path string) float64 {
	t.Helper()
	const rate = 8000
	decoded, err := runner.Run(t.Context(), execx.FFmpeg, confine.Access{}, []string{
		"-v", "error", "-i", "file:" + path, "-map", "0:a:0", "-ac", "1", "-ar", "8000", "-f", "s16le", "pipe:1",
	}, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	firstAt := 0.0
	if times := sourceTimes(t, runner, path, "a:0", false); len(times) > 0 {
		firstAt = times[0]
	}
	for at := 0; at+1 < len(decoded); at += 2 {
		sample := int16(binary.LittleEndian.Uint16(decoded[at:])) //nolint:gosec // G115: a sample's bits
		if sample > 4000 || sample < -4000 {
			return firstAt + float64(at/2)/rate
		}
	}
	t.Fatalf("%s has no tone", path)
	return 0
}

// The sound of an AVI interleaved ahead of its picture, made from the film's start, is at its times
// in the film: FFmpeg moves a seek to the start of a film with B-frames before it, and the AVI demuxer
// seeking there lost the sound's first half second, which the run, timed by its samples, played that
// much early (idiosyncrasies.md). A run of the film's start reads it unsought.
func TestMake_AnAVIsSoundInterleavedAheadOfItsPictureIsAtItsTimesFromTheFilmsStart(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	source := aviWithSoundAheadOfItsPicture(t)
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	storeDir := "1/chunks-sound0/0123456789abcdef"
	command := nodeproto.MakeChunks{
		VideoStreamIndex: 0, VideoCodec: "mpeg4", StoreDir: storeDir, FirstChunk: 0,
		BoundsMs: []int64{0, 2_000, 4_000, 6_000},
		Stream: nodeproto.ChunkStream{
			Kind: nodeproto.StreamSound, SoundKbps: 128, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
		},
	}
	if last, err := maker.Make(t.Context(), execx.Film{Path: source}, command, nil); err != nil || last != 2 {
		t.Fatalf("the run made through %d, %v", last, err)
	}
	inFilm := toneOnset(t, runner, source)
	if math.Abs(inFilm-float64(toneAtMs)/1000) > 0.05 {
		t.Fatalf("the film's tone starts at %.3f s; want about %.3f s, as it was made", inFilm, float64(toneAtMs)/1000)
	}
	store := joined(t, filepath.Join(directory, storeDir), 3)
	made := toneOnset(t, runner, store) - float64(chunks.TimelineOffsetMs)/1000
	if math.Abs(made-inFilm) > 0.025 {
		t.Errorf("the sound made has its tone at %.3f s in the film; want the film's %.3f s", made, inFilm)
	}
}
