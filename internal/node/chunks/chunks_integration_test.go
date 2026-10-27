// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package chunks_test

import (
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
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

// film makes a twelve-second MP4 with a keyframe every two seconds, B-frames, so a picture is shown
// after it is decoded, and stereo AAC.
func film(t *testing.T, runner *execx.Runner) string {
	t.Helper()
	return filmWithKeyframesEvery(t, runner, 48)
}

// filmWithKeyframesEvery is film with a keyframe every so many of its 24 pictures a second.
func filmWithKeyframesEvery(t *testing.T, runner *execx.Runner, pictures int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a film (2020).mp4")
	every := strconv.Itoa(pictures)
	_, err := runner.Run(t.Context(), execx.FFmpeg, confine.Access{}, []string{
		"-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=12",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=12",
		"-c:v", "libx264", "-preset", "fast", "-bf", "2", "-g", every, "-keyint_min", every, "-sc_threshold", "0",
		"-c:a", "aac", "-ac", "2", "file:" + path,
	}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// inMatroska is the film copied into Matroska, which FFmpeg seeks by decode time: it moves a seek
// 130 ms earlier, so a seek at a keyframe lands on the one before it (idiosyncrasies.md).
func inMatroska(t *testing.T, runner *execx.Runner, source string) string {
	t.Helper()
	return remuxed(t, runner, source, "a film (2020).mkv")
}

// remuxed is the film copied, every stream, into the container its name's extension names.
func remuxed(t *testing.T, runner *execx.Runner, source, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	_, err := runner.Run(t.Context(), execx.FFmpeg, confine.Access{}, []string{
		"-v", "error", "-i", "file:" + source, "-map", "0", "-c", "copy", "file:" + path,
	}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// keyframeGrid is the film's chunks: its picture's keyframes as FFmpeg's timeline has them, which a
// read-through finds (internal/node/readthrough), a keyframe every two seconds; then its end.
func keyframeGrid(t *testing.T, runner *execx.Runner, source string) []int64 {
	t.Helper()
	var grid []int64
	for _, shown := range sourceTimes(t, runner, source, "v:0", true) {
		grid = append(grid, int64(math.Round(shown*1000)))
	}
	if len(grid) != 6 {
		t.Fatalf("the film has keyframes at %v; want six, two seconds apart", grid)
	}
	return append(grid, grid[0]+12_000)
}

// sourceTimes are the shown times, in seconds, of the source's packets of stream, sorted; its
// keyframes alone when keyframesOnly.
func sourceTimes(t *testing.T, runner *execx.Runner, source, stream string, keyframesOnly bool) []float64 {
	t.Helper()
	listed, err := runner.Run(t.Context(), execx.FFprobe, confine.Access{}, []string{
		"-v", "error", "-select_streams", stream, "-show_entries", "packet=pts_time,flags", "-of", "csv=p=0",
		"file:" + source,
	}, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	var times []float64
	for line := range strings.Lines(string(listed)) {
		value, flags, _ := strings.Cut(strings.TrimSpace(line), ",")
		shown, err := strconv.ParseFloat(value, 64)
		if err == nil && (!keyframesOnly || strings.Contains(flags, "K")) {
			times = append(times, shown)
		}
	}
	slices.Sort(times)
	return times
}

// packetSizes are the sizes of the packets of stream, in decode order: a copied picture's are the
// source's, packet for packet, where times alone match a run begun a keyframe early.
func packetSizes(t *testing.T, runner *execx.Runner, path, stream string) []string {
	t.Helper()
	listed, err := runner.Run(t.Context(), execx.FFprobe, confine.Access{}, []string{
		"-v", "error", "-select_streams", stream, "-show_entries", "packet=size", "-of", "csv=p=0", "file:" + path,
	}, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(listed))
}

// joined is a store's init and its chunks written as one file, what a player plays.
func joined(t *testing.T, storePath string, chunkCount int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "joined.mp4")
	contents, err := os.ReadFile(filepath.Join(storePath, chunks.InitName))
	if err != nil {
		t.Fatal(err)
	}
	for n := range chunkCount {
		chunk, err := os.ReadFile(filepath.Join(storePath, chunks.ChunkName(n)))
		if err != nil {
			t.Fatal(err)
		}
		contents = append(contents, chunk...)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil { //nolint:gosec // the test's own temporary file
		t.Fatal(err)
	}
	return path
}

// packetTimes are the shown times, in seconds, of the packets of the stream joined from a store's
// init and its chunks, in order: what a player plays.
func packetTimes(t *testing.T, runner *execx.Runner, storePath string, chunkCount int) []float64 {
	t.Helper()
	return sourceTimes(t, runner, joined(t, storePath, chunkCount), "0", false)
}

// makeInTwoRuns makes stream's chunks 0–2 in one run and 3–5 in another, the second with its
// sound's lead-in, as two players asking far apart would.
func makeInTwoRuns(
	t *testing.T, maker *chunks.Maker, source, storeDir string, grid []int64, stream nodeproto.ChunkStream,
) {
	t.Helper()
	for _, first := range []int{0, 3} {
		command := nodeproto.MakeChunks{
			VideoStreamIndex: 0, VideoCodec: "h264", Stream: stream, StoreDir: storeDir,
			FirstChunk: first, BoundsMs: grid[first : first+4],
		}
		if stream.Kind == nodeproto.StreamPicture {
			command.PictureSeekMs = (grid[first] + grid[first+1]) / 2 // midway to the next keyframe
			// MPEG-TS: midway back to the keyframe before, or the film's start.
			if strings.HasSuffix(source, ".ts") {
				command.IsPictureSeekBeforeKeyframe, command.PictureSeekMs = true, 0
				if first > 0 {
					command.PictureSeekMs = (grid[first-1] + grid[first]) / 2
				}
			}
		}
		if stream.Kind == nodeproto.StreamSound && first > 0 {
			command.SoundLeadInMs = grid[first-1]
		}
		if last, err := maker.Make(t.Context(), execx.Film{Path: source}, command, nil); err != nil || last != first+2 {
			t.Fatalf("the run from %d made through %d, %v", first, last, err)
		}
	}
}

// Chunks made by two runs far apart join into one stream: every picture once, at its time in the
// film, the second run's first chunk shown at its bound, the copied picture and the encoded quality
// alike, from MP4 and from Matroska, which FFmpeg seeks differently (idiosyncrasies.md); and the
// sound continuous, its join under a frame (D247 §10; slow-links.md §10).
func TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	source := film(t, runner)
	grid := keyframeGrid(t, runner, source)
	offset := float64(chunks.TimelineOffsetMs) / 1000
	pictures := sourceTimes(t, runner, source, "v:0", false)
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	matroska := inMatroska(t, runner, source)
	transport := remuxed(t, runner, source, "a film (2020).ts")
	picture := nodeproto.ChunkStream{Kind: nodeproto.StreamPicture}
	quality := nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 300, Height: 96}
	// MPEG-TS carries its picture's packets in another framing than MP4: its copy's packets are
	// the film copied whole into MP4.
	transportCopied := remuxed(t, runner, transport, "a film (2020) from MPEG-TS.mp4")
	for storeDir, made := range map[string]struct {
		source, packetsAs string
		stream            nodeproto.ChunkStream
	}{
		"1/chunks-picture/0123456789abcdef":  {source, source, picture},
		"2/chunks-picture/0123456789abcdef":  {matroska, matroska, picture},
		"3/chunks-picture/0123456789abcdef":  {transport, transportCopied, picture},
		"1/chunks-q300k96p/0123456789abcdef": {source, source, quality},
		"2/chunks-q300k96p/0123456789abcdef": {matroska, matroska, quality},
	} {
		makeInTwoRuns(t, maker, made.source, storeDir, grid, made.stream)
		if made.stream.Kind == nodeproto.StreamPicture {
			stored := packetSizes(t, runner, joined(t, filepath.Join(directory, storeDir), len(grid)-1), "0")
			if want := packetSizes(t, runner, made.packetsAs, "v:0"); !slices.Equal(stored, want) {
				t.Errorf("%s: the copied picture's packets are not the film's, packet for packet", storeDir)
			}
		}
		times := packetTimes(t, runner, filepath.Join(directory, storeDir), len(grid)-1)
		if len(times) != len(pictures) {
			t.Errorf("%s: %d pictures, want the film's %d", storeDir, len(times), len(pictures))
			continue
		}
		for index, shown := range times {
			if want := pictures[index] + offset; math.Abs(shown-want) > 0.002 {
				t.Errorf("%s: picture %d is shown at %.4f s, want the film's %.4f s, offset",
					storeDir, index, shown, want)
				break
			}
		}
	}
	soundStore := "1/chunks-sound0/0123456789abcdef"
	makeInTwoRuns(t, maker, source, soundStore, grid, nodeproto.ChunkStream{
		Kind: nodeproto.StreamSound, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
	})
	times := packetTimes(t, runner, filepath.Join(directory, soundStore), len(grid)-1)
	const frame = 1024.0 / 48000
	for index := 1; index < len(times); index++ {
		if step := times[index] - times[index-1]; step < frame/2 || step > 2*frame {
			t.Errorf("the sound steps %.4f s at %.3f s; want about a frame, %.4f s", step, times[index], frame)
		}
	}
	if len(times) == 0 || math.Abs(times[0]-(offset-frame)) > 0.002 || times[len(times)-1] < offset+11.9 {
		t.Errorf("the sound runs from %.4f to %.4f s; want from its priming, a frame before the film, to the end",
			times[0], times[len(times)-1])
	}
}

// An MPEG-TS whose one keyframe is its first is one chunk, made whole: FFmpeg seeks MPEG-TS to the
// frame at or before a time, not to a keyframe, so a copy seeking past the keyframe had none to
// start at and wrote no picture at all ("no init or no fragments"; the container sweep, 2026-10-27).
// Seeking to the film's start, the copy starts at its keyframe (idiosyncrasies.md).
func TestMake_AnMPEGTSWhoseOnlyKeyframeIsItsFirstIsOneChunkMadeWhole(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	transport := remuxed(t, runner, filmWithKeyframesEvery(t, runner, 1000), "a film (2020).ts")
	if keyframes := sourceTimes(t, runner, transport, "v:0", true); len(keyframes) != 1 {
		t.Fatalf("the film has keyframes at %v; want its first alone", keyframes)
	}
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	storeDir := "1/chunks-picture/0123456789abcdef"
	command := nodeproto.MakeChunks{
		VideoStreamIndex: 0, VideoCodec: "h264", Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
		StoreDir: storeDir, BoundsMs: []int64{0, 12_000}, IsPictureSeekBeforeKeyframe: true,
	}
	if last, err := maker.Make(t.Context(), execx.Film{Path: transport}, command, nil); err != nil || last != 0 {
		t.Fatalf("the run made through %d, %v; want its one chunk", last, err)
	}
	stored := packetSizes(t, runner, joined(t, filepath.Join(directory, storeDir), 1), "0")
	copied := remuxed(t, runner, transport, "a film (2020) from MPEG-TS.mp4")
	if want := packetSizes(t, runner, copied, "v:0"); !slices.Equal(stored, want) {
		t.Errorf("the chunk holds %d pictures; want the film's %d, packet for packet", len(stored), len(want))
	}
}
