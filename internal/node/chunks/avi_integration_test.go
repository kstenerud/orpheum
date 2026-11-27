// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package chunks_test

import (
	"bytes"
	"encoding/binary"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
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

// aviIndexingEveryFrameAKeyframe makes a twelve-second AVI of H.264 without B-frames, its keyframes
// as unevenly apart as file 17099's (staging, 2026-11-16), whose index marks every frame a keyframe:
// the AVI demuxer seeks by its index and lands on the frame at the seek, keyframe or not, while a copy's
// parser finds the true keyframes and starts at the first after it (idiosyncrasies.md).
func aviIndexingEveryFrameAKeyframe(t *testing.T) string {
	t.Helper()
	made := filepath.Join(t.TempDir(), "a film (1986).avi")
	testutil.GenerateMedia(t, made,
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=25:duration=12",
		"-c:v", "libx264", "-preset", "fast", "-bf", "0", "-g", "1000", "-sc_threshold", "0",
		"-force_key_frames", "0,4.48,7.32,7.96,8.32,10.64", "-pix_fmt", "yuv420p")
	contents, err := os.ReadFile(made)
	if err != nil {
		t.Fatal(err)
	}
	// The legacy index, idx1, last in the file: an entry of 16 bytes for each packet, its chunk's id,
	// then its flags, AVIIF_KEYFRAME (0x10) among them, then its place and size.
	at := bytes.LastIndex(contents, []byte("idx1"))
	if at < 0 || at+8 > len(contents) {
		t.Fatal("the AVI made has no idx1 index")
	}
	end := min(len(contents), at+8+int(binary.LittleEndian.Uint32(contents[at+4:])))
	marked := 0
	for entry := at + 8; entry+16 <= end; entry += 16 {
		if bytes.HasSuffix(contents[entry:entry+4], []byte("dc")) {
			binary.LittleEndian.PutUint32(contents[entry+4:], binary.LittleEndian.Uint32(contents[entry+4:])|0x10)
			marked++
		}
	}
	if marked != 300 {
		t.Fatalf("the index marked %d pictures keyframes; want the film's 300", marked)
	}
	path := filepath.Join(t.TempDir(), "a film (1986).avi")
	if err := os.WriteFile(path, contents, 0o600); err != nil { //nolint:gosec // G703: the test's own temporary file
		t.Fatal(err)
	}
	return path
}

// A copied picture of an AVI whose index marks every frame a keyframe starts each run at its first
// chunk's keyframe, though the demuxer's seek lands on a frame between keyframes: a run seeking past
// the keyframe started at the next one, its first raw file a keyframe interval long, and the node
// refused every run at its bounds (file 17099, staging, 2026-11-16; D500 §1). Its chunks, from two
// runs, the second led in, are the film's pictures packet for packet at the film's times.
func TestMake_ACopiedPictureOfAnAVIIndexingEveryFrameAKeyframeStartsAtItsKeyframe(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	source := aviIndexingEveryFrameAKeyframe(t)
	// The fixture is the film's: the demuxer's seek lands on a frame that is no keyframe.
	landed, err := runner.Run(t.Context(), execx.FFprobe, confine.Access{}, []string{
		"-v", "error", "-select_streams", "v:0", "-show_entries", "packet=flags", "-of", "csv=p=0",
		"-read_intervals", "3.0%+0.1", "file:" + source,
	}, 1<<20)
	if first, _, _ := strings.Cut(string(landed), "\n"); err != nil || strings.Contains(first, "K") {
		t.Fatalf("a seek to 3.0 s lands on a packet flagged %q, %v; want one between keyframes", first, err)
	}
	// An AVI's picture has no shown times of its own for ffprobe to list; the film copied into MP4 has
	// them, and the packets a copy into MP4 makes of it.
	copied := remuxed(t, source, "a film (1986) from AVI.mp4")
	var grid []int64
	for _, shown := range sourceTimes(t, runner, copied, "v:0", true) {
		grid = append(grid, int64(math.Round(shown*1000)))
	}
	if !slices.Equal(grid, []int64{0, 4_480, 7_320, 7_960, 8_320, 10_640}) {
		t.Fatalf("the film's keyframes are at %v ms; want those it was made with", grid)
	}
	grid = append(grid, 12_000)
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264, slog.New(slog.DiscardHandler))
	storeDir := "17/chunks-picture/0123456789abcdef"
	makeInTwoRuns(t, maker, source, storeDir, grid, nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	stored := packetSizes(t, runner, joined(t, filepath.Join(directory, storeDir), len(grid)-1), "0")
	if want := packetSizes(t, runner, copied, "v:0"); !slices.Equal(stored, want) {
		t.Errorf("the copied picture's %d packets are not the film's %d, packet for packet", len(stored), len(want))
	}
	pictures := sourceTimes(t, runner, copied, "v:0", false)
	times := packetTimes(t, runner, filepath.Join(directory, storeDir), len(grid)-1)
	if len(times) != len(pictures) {
		t.Fatalf("%d pictures, want the film's %d", len(times), len(pictures))
	}
	offset := float64(chunks.TimelineOffsetMs) / 1000
	for index, shown := range times {
		if want := pictures[index] + offset; math.Abs(shown-want) > 0.002 {
			t.Fatalf("picture %d is shown at %.4f s, want the film's %.4f s, offset", index, shown, want)
		}
	}
}

// toneOnset is when the sound of path's first audio stream reaches its tone, in seconds on path's own
// timeline: its first packet's time and the samples decoded before the tone.
func toneOnset(t *testing.T, runner *execx.Runner, path string) float64 {
	t.Helper()
	return toneOnsetAfter(t, runner, path, 0)
}

// toneOnsetAfter is toneOnset of a tone that starts after seconds of path's sound decoded: the second
// of two, when the first ends before then.
func toneOnsetAfter(t *testing.T, runner *execx.Runner, path string, after float64) float64 {
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
	for at := 2 * int(after*rate); at+1 < len(decoded); at += 2 {
		sample := int16(binary.LittleEndian.Uint16(decoded[at:])) //nolint:gosec // G115: a sample's bits
		if sample > 4000 || sample < -4000 {
			return firstAt + float64(at/2)/rate
		}
	}
	t.Fatalf("%s has no tone after %.1f s", path, after)
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
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264, slog.New(slog.DiscardHandler))
	storeDir := "1/chunks-sound0/0123456789abcdef"
	command := nodeproto.MakeChunks{
		VideoStreamIndex: 0, VideoCodec: "mpeg4", StoreDir: storeDir, FirstChunk: 0,
		BoundsMs: []int64{0, 2_000, 4_000, 6_000},
		Stream: nodeproto.ChunkStream{
			Kind: nodeproto.StreamSound, SoundKbps: 128, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
		},
	}
	if last, _, err := maker.Make(t.Context(), execx.Film{Path: source}, command, nil); err != nil || last != 2 {
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
