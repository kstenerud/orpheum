// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package execx_test

import (
	"bytes"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/chunks"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// longFilm is an MP4 of ten minutes, its movie box last: a tiny picture of 24 pictures a second, a
// keyframe every two seconds, B-frames, and stereo AAC, so its index has many samples.
func longFilm(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a film (2020).mp4")
	testutil.GenerateMedia(t, path,
		"-f", "lavfi", "-i", "testsrc2=size=32x32:rate=24:duration=600",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=600",
		"-c:v", "libx264", "-preset", "ultrafast", "-bf", "2", "-g", "48", "-keyint_min", "48", "-sc_threshold", "0",
		"-c:a", "aac", "-ac", "2")
	return path
}

// keyframesNear are the film's picture's keyframes from fromS seconds, count of them, in ms.
func keyframesNear(t *testing.T, runner *execx.Runner, path string, fromS float64, count int) []int64 {
	t.Helper()
	listed, err := runner.Run(t.Context(), execx.FFprobe, confine.Access{}, []string{
		"-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time,flags", "-of", "csv=p=0",
		"file:" + path,
	}, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	var grid []int64
	for line := range strings.Lines(string(listed)) {
		value, flags, _ := strings.Cut(strings.TrimSpace(line), ",")
		shown, err := strconv.ParseFloat(value, 64)
		if err == nil && strings.Contains(flags, "K") && shown >= fromS && len(grid) < count {
			grid = append(grid, int64(math.Round(shown*1000)))
		}
	}
	return grid
}

// streamedFilmOf is the runner's film at path streamed to a run of chunks of command.
func streamedFilmOf(
	t *testing.T, runner *execx.Runner, maker *chunks.Maker, path string, command nodeproto.MakeChunks,
) execx.Film {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() }) //nolint:errcheck // read only
	facts, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	stretch := maker.StretchOf(command)
	disks := diskwatch.NewGate(clocktest.NewFake(time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)))
	film, err := runner.FilmOf(t.Context(), path, execx.OpenedFilm{
		File: file, Facts: facts, Disks: disks, Root: filepath.Dir(path), Window: time.Minute, Stretch: &stretch,
	}, execx.ReadsWholeFilm)
	if err != nil {
		t.Fatal(err)
	}
	return film
}

// storeBytes are a store's init and its chunks, joined.
func storeBytes(t *testing.T, directory string, chunkCount int) []byte {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(directory, chunks.InitName))
	if err != nil {
		t.Fatal(err)
	}
	for n := range chunkCount {
		chunk, err := os.ReadFile(filepath.Join(directory, chunks.ChunkName(n+40)))
		if err != nil {
			t.Fatal(err)
		}
		contents = append(contents, chunk...)
	}
	return contents
}

// A run of chunks in the middle of a film served with its index cut down makes the chunks it makes
// from the film whole, byte for byte: its copied picture, its copied sound, its encoded picture and
// its encoded sound, their packets at the same times (D512).
func TestFilmOf_AFilmServedCutDownMakesTheChunksOfTheWholeFilm(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	cutting, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	runner.StreamFilms()
	cutting.CuttingDownFrom(1).StreamFilms()
	source := longFilm(t)
	grid := keyframesNear(t, runner, source, 298, 5)
	if len(grid) != 5 {
		t.Fatalf("the film has keyframes %v from 298 s; want five", grid)
	}
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	sound := &nodeproto.SoundTrack{ChannelCount: 2}
	for name, stream := range map[string]nodeproto.ChunkStream{
		"picture":    {Kind: nodeproto.StreamPicture},
		"q300k96p":   {Kind: nodeproto.StreamQuality, BitRateKbps: 300, Height: 32},
		"sound0-own": {Kind: nodeproto.StreamSoundCopied, Sound: sound},
		"sound0-96k": {Kind: nodeproto.StreamSound, SoundKbps: 96, Sound: sound},
	} {
		command := chunksCommandOf(stream, grid)
		command.StoreDir = "1/chunks-" + name + "/0123456789abcdef"
		whole, isWholeCutDown := madeBy(t, runner, maker, source, command, filepath.Join(directory, command.StoreDir))
		command.StoreDir = "2/chunks-" + name + "/0123456789abcdef"
		cut, isCutDown := madeBy(t, cutting, maker, source, command, filepath.Join(directory, command.StoreDir))
		if isWholeCutDown || !isCutDown {
			t.Fatalf("%s: the films are cut down %v and %v; want only the cutting runner's",
				name, isWholeCutDown, isCutDown)
		}
		if !bytes.Equal(cut, whole) {
			first := 0
			for first < min(len(cut), len(whole)) && cut[first] == whole[first] {
				first++
			}
			t.Errorf("%s: the cut-down film made %d bytes, the whole film %d, differing from byte %d; want the same",
				name, len(cut), len(whole), first)
		}
	}
}

// chunksCommandOf is a run of chunks 40 to 42 of the stream, bounded by the keyframes of grid after
// its first, led in from its first unless the picture is encoded.
func chunksCommandOf(stream nodeproto.ChunkStream, grid []int64) nodeproto.MakeChunks {
	command := nodeproto.MakeChunks{
		VideoStreamIndex: 0, VideoCodec: "h264", Stream: stream, FirstChunk: 40, BoundsMs: grid[1:],
	}
	if stream.Kind != nodeproto.StreamQuality {
		command.LeadInMs, command.IsLedIn = grid[0], true
	}
	return command
}

// madeBy are the init and chunks the runner's run of command makes from the film at path, into
// store, and whether the film was served cut down.
func madeBy(
	t *testing.T, runner *execx.Runner, maker *chunks.Maker, path string, command nodeproto.MakeChunks, store string,
) ([]byte, bool) {
	t.Helper()
	film := streamedFilmOf(t, runner, maker, path, command)
	if last, err := maker.Make(t.Context(), film, command, nil); err != nil || last != 42 {
		t.Fatalf("%s, cut down %v: the run made through %d, %v; want chunk 42", store, film.IsIndexCutDown, last, err)
	}
	return storeBytes(t, store, 3), film.IsIndexCutDown
}
