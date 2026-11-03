// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package chunks_test

import (
	"log/slog"
	"math"
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

// pictureWithBFrames makes a twelve-second MP4 of codec, H.264 or HEVC, three B-frames between its
// pictures and a closed keyframe every two seconds, so its pictures are shown after they are decoded.
func pictureWithBFrames(t *testing.T, codec string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a "+codec+" film (2020).mp4")
	encoder := []string{
		"-c:v", "libx264", "-preset", "fast", "-bf", "3", "-g", "48", "-keyint_min", "48",
		"-sc_threshold", "0",
	}
	if codec == "hevc" {
		// hev1, as file 5667 is tagged: its parameter sets in the stream.
		encoder = []string{
			"-c:v", "libx265", "-tag:v", "hev1",
			"-x265-params", "bframes=3:keyint=48:min-keyint=48:scenecut=0:open-gop=0:log-level=error",
		}
	}
	testutil.GenerateMedia(t, path,
		append([]string{"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=12"}, encoder...)...)
	return path
}

// flattened is source copied with each picture shown at its decode time, as file 5667's every packet
// is: its B-frames carry no display times (design/plans/audio-switch-and-retiming-2026-11-01.md
// Part 2). Its decode times start at zero.
func flattened(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flattened "+filepath.Base(source))
	testutil.GenerateMedia(t, path, "-i", "file:"+source, "-map", "0", "-c", "copy",
		"-bsf:v", "setts=dts=DTS-STARTDTS:pts=DTS-STARTDTS")
	return path
}

// compositionOffsets are how long after it is decoded each picture packet of path is shown, in
// seconds, in decode order.
func compositionOffsets(t *testing.T, runner *execx.Runner, path string) []float64 {
	t.Helper()
	listed, err := runner.Run(t.Context(), execx.FFprobe, confine.Access{}, []string{
		"-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time,dts_time", "-of", "csv=p=0",
		"file:" + path,
	}, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	var offsets []float64
	for line := range strings.Lines(string(listed)) {
		shownText, decodedText, _ := strings.Cut(strings.TrimSpace(line), ",")
		shown, shownErr := strconv.ParseFloat(shownText, 64)
		decoded, decodedErr := strconv.ParseFloat(decodedText, 64)
		if shownErr == nil && decodedErr == nil {
			offsets = append(offsets, shown-decoded)
		}
	}
	return offsets
}

// guessedAtTheFilmsEnd is how many of a re-timed film's last pictures dts2pts guesses the times of,
// having no pictures after them to take them from: three B-frames and the picture they lean on, and
// one more in H.264, measured with the pinned FFmpeg.
const guessedAtTheFilmsEnd = 5

// retimingFixture makes runs of chunks of a flattened clip, source, its picture in codec and cut on
// grid, into stores under one prepared directory.
type retimingFixture struct {
	maker         *chunks.Maker
	directory     string
	source, codec string
	grid          []int64
}

// makeTwoRuns makes stream's chunks 0–2 of the clip in one run and 3–5 in another, re-timed as
// isMarked says, into storeDir, and returns the store's path.
func (fixture retimingFixture) makeTwoRuns(
	t *testing.T, storeDir string, stream nodeproto.ChunkStream, isMarked bool,
) string {
	t.Helper()
	grid := fixture.grid
	for _, first := range []int{0, 3} {
		command := nodeproto.MakeChunks{
			VideoStreamIndex: 0, VideoCodec: fixture.codec, Stream: stream, StoreDir: storeDir, FirstChunk: first,
			BoundsMs: grid[first : first+4], NeedsPictureRetiming: isMarked,
		}
		if stream.Kind == nodeproto.StreamPicture {
			command.PictureSeekMs = (grid[first] + grid[first+1]) / 2
		}
		last, err := fixture.maker.Make(t.Context(), execx.Film{Path: fixture.source}, command, nil)
		if err != nil || last != first+2 {
			t.Fatalf("%s %s: the run from %d made through %d, %v", fixture.codec, stream.Kind, first, last, err)
		}
	}
	return filepath.Join(fixture.directory, storeDir)
}

// A picture whose B-frames carry no display times, as file 5667's do not, is re-timed when its run
// says so: its chunks, made by two runs far apart, joined, have the composition offsets of the
// picture before it was flattened, packet for packet, but the film's last few, whose times dts2pts
// guesses; unmarked, they have none, which a browser plays dropping the reordered frames; and an
// encode of it re-timed has every picture once (design/plans/audio-switch-and-retiming-2026-11-01.md
// §2.2). Both H.264 and HEVC.
func TestMake_APictureWhoseBFramesCarryNoDisplayTimesIsReTimedWhenMarked(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	picture := nodeproto.ChunkStream{Kind: nodeproto.StreamPicture}
	quality := nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 300, Height: 96}
	for fileID, codec := range []string{"h264", "hevc"} {
		original := pictureWithBFrames(t, codec)
		source := flattened(t, original)
		// The core's grid of the flattened film: its keyframes as its timeline has them, their decode times.
		grid := keyframeGrid(t, runner, source)
		fixture := retimingFixture{maker: maker, directory: directory, source: source, codec: codec, grid: grid}
		want := compositionOffsets(t, runner, original)
		file := strconv.Itoa(fileID + 1)
		marked := fixture.makeTwoRuns(t, file+"/chunks-picture/1123456789abcdef", picture, true)
		checkOffsetsAreTheFilmsOwn(t, codec, compositionOffsets(t, runner, joined(t, marked, len(grid)-1)), want)
		unmarked := fixture.makeTwoRuns(t, file+"/chunks-picture/0123456789abcdef", picture, false)
		if got := compositionOffsets(t, runner, joined(t, unmarked, len(grid)-1)); slices.ContainsFunc(got,
			func(offset float64) bool { return offset > 0.001 }) {
			t.Errorf("%s, unmarked: pictures shown after they are decoded; want none, as the source is", codec)
		}
		encoded := fixture.makeTwoRuns(t, file+"/chunks-q300k96p/0123456789abcdef", quality, true)
		checkEncodeIsEveryPicture(t, codec, packetTimes(t, runner, encoded, len(grid)-1),
			sourceTimes(t, runner, original, "v:0", false))
	}
}

// checkOffsetsAreTheFilmsOwn holds that a re-timed copy's pictures are each shown as long after they
// are decoded as the film's own were, but the film's last few, which dts2pts guesses at the end of its
// input, and that most are shown after they are decoded: its B-frames reordered.
func checkOffsetsAreTheFilmsOwn(t *testing.T, codec string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d pictures, want the film's %d", codec, len(got), len(want))
		return
	}
	for index := range len(got) - guessedAtTheFilmsEnd {
		if math.Abs(got[index]-want[index]) > 0.002 {
			t.Errorf("%s: picture %d is shown %.4f s after it is decoded, want the film's own %.4f s",
				codec, index, got[index], want[index])
			return
		}
	}
	reordered := 0
	for _, offset := range got {
		if offset > 0.001 {
			reordered++
		}
	}
	if reordered < len(got)/2 {
		t.Errorf("%s: %d of %d pictures shown after they are decoded; want B-frames reordered", codec, reordered,
			len(got))
	}
}

// pictureSeconds is how long each picture of pictureWithBFrames lasts, at 24 a second.
const pictureSeconds = 1.0 / 24

// checkEncodeIsEveryPicture holds that an encode of a re-timed film, its two runs joined, shows its
// pictures from the film's first to its last, a picture's length apart, none twice and none past the
// film's end: but for the film's last two, whose times dts2pts guesses, which the encoder may drop.
// Re-timed alike, the runs' keyframes fall at their bounds, so the muxer cuts there.
func checkEncodeIsEveryPicture(t *testing.T, codec string, times, pictures []float64) {
	t.Helper()
	offset := float64(chunks.TimelineOffsetMs) / 1000
	if len(times) < len(pictures)-2 || len(times) > len(pictures) {
		t.Errorf("%s: the encode has %d pictures, want the film's %d, or two fewer at its end", codec, len(times),
			len(pictures))
		return
	}
	if math.Abs(times[0]-(pictures[0]+offset)) > 0.002 {
		t.Errorf("%s: the encode starts at %.4f s, want the film's start, %.4f s", codec, times[0], pictures[0]+offset)
	}
	for index := 1; index < len(times); index++ {
		if math.Abs(times[index]-times[index-1]-pictureSeconds) > 0.002 {
			t.Errorf("%s: the encode's picture %d is shown %.4f s after the one before, want %.4f s",
				codec, index, times[index]-times[index-1], pictureSeconds)
			return
		}
	}
}

// A re-timed copy is cut at a keyframe shown sooner after its decode time than the run's first
// keyframe is: an open-GOP HEVC film, as The Gold Rush's x265 is, shows each keyframe after the pictures
// decoded after it that it leads, and dts2pts times it so, here 292 ms late at 2.042 s, 208 ms at
// 4.375 s and 125 ms at 6.042 s. The muxer, timing its cuts from the run's first keyframe as shown,
// took the one at 4.375 s for 84 ms early, and a cut 50 ms early missed it: the run held chunks 1 and
// 2 in one raw file, as The Gold Rush's held 349 and 350, which could never be made (staging,
// 2026-11-03; toolkit.md item 12).
func TestMake_ARetimedCopyIsCutAtAKeyframeShownSoonerThanItsFirst(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(t.TempDir(), "an open gop film (2020).mp4")
	// Seven B-frames in a fixed pattern, so each keyframe forced off it leads its own number of them.
	testutil.GenerateMedia(t, original, "-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=12",
		"-c:v", "libx265", "-tag:v", "hev1", "-force_key_frames", "0,2.25,4.5,6.083,8.5,10.083",
		"-x265-params", "bframes=7:b-adapt=0:keyint=600:min-keyint=1:scenecut=0:open-gop=1:log-level=error")
	source := flattened(t, original)
	grid := keyframeGrid(t, runner, source)
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	command := nodeproto.MakeChunks{
		VideoStreamIndex: 0, VideoCodec: "hevc", Stream: nodeproto.ChunkStream{Kind: nodeproto.StreamPicture},
		StoreDir: "1/chunks-picture/0123456789abcdef", FirstChunk: 1, BoundsMs: grid[1:5],
		PictureSeekMs: (grid[1] + grid[2]) / 2, NeedsPictureRetiming: true,
	}
	last, err := maker.Make(t.Context(), execx.Film{Path: source}, command, nil)
	if err != nil || last != 3 {
		t.Errorf("a re-timed copy of chunks 1–3, on keyframes at %v ms, made through %d, %v; want 3", grid[1:5],
			last, err)
	}
}
