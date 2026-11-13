// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package chunks_test

import (
	"bytes"
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
	"github.com/kstenerud/orpheum/internal/node/container"
	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// film makes a twelve-second MP4 with a keyframe every two seconds, B-frames, so a picture is shown
// after it is decoded, and stereo AAC.
func film(t *testing.T) string {
	t.Helper()
	return filmWithKeyframesEvery(t, 48)
}

// filmWithKeyframesEvery is film with a keyframe every so many of its 24 pictures a second.
func filmWithKeyframesEvery(t *testing.T, pictures int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a film (2020).mp4")
	every := strconv.Itoa(pictures)
	testutil.GenerateMedia(t, path,
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=12",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=12",
		"-c:v", "libx264", "-preset", "fast", "-bf", "2", "-g", every, "-keyint_min", every, "-sc_threshold", "0",
		"-c:a", "aac", "-ac", "2")
	return path
}

// inMatroska is the film copied into Matroska, which FFmpeg seeks by decode time: it moves a seek
// 130 ms earlier, so a seek at a keyframe lands on the one before it (idiosyncrasies.md).
func inMatroska(t *testing.T, source string) string {
	t.Helper()
	return remuxed(t, source, "a film (2020).mkv")
}

// remuxed is the film copied, every stream, into the container its name's extension names.
func remuxed(t *testing.T, source, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	testutil.GenerateMedia(t, path, "-i", "file:"+source, "-map", "0", "-c", "copy")
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

// initTimescale is the ticks per second its store's init gives its track (its mdhd's).
func initTimescale(t *testing.T, storePath string) uint32 {
	t.Helper()
	init, err := os.ReadFile(filepath.Join(storePath, chunks.InitName))
	if err != nil {
		t.Fatal(err)
	}
	timescale, err := chunks.Timescale(init)
	if err != nil {
		t.Fatal(err)
	}
	return timescale
}

// packetTimes are the shown times, in seconds, of the packets of the stream joined from a store's
// init and its chunks, in order: what a player plays.
func packetTimes(t *testing.T, runner *execx.Runner, storePath string, chunkCount int) []float64 {
	t.Helper()
	return sourceTimes(t, runner, joined(t, storePath, chunkCount), "0", false)
}

// openGOPFilm is film's picture in HEVC with open GOPs, in Matroska: each keyframe after the first
// has pictures decoded after it and shown before it, whose times FFmpeg gets wrong in a copy begun
// at that keyframe (idiosyncrasies.md).
func openGOPFilm(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "an open film (2020).mkv")
	testutil.GenerateMedia(t, path,
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=12",
		"-c:v", "libx265", "-preset", "fast",
		"-x265-params", "keyint=48:min-keyint=48:scenecut=0:open-gop=1:bframes=4:b-adapt=0:log-level=error")
	return path
}

// pictureSeek is where a copied picture's run from chunk first seeks, as the core sends it: midway
// from its lead-in's keyframe, or the film's first, to the next; an MPEG-TS's midway back to the
// keyframe before, or the film's start, said to be before it.
func pictureSeek(source string, grid []int64, first int) (seekMs int64, isBeforeKeyframe bool) {
	from := max(first-1, 0)
	if !strings.HasSuffix(source, ".ts") {
		return (grid[from] + grid[from+1]) / 2, false
	}
	if from == 0 {
		return 0, true
	}
	return (grid[from-1] + grid[from]) / 2, true
}

// makeInTwoRuns makes stream's chunks 0–2 in one run and 3–5 in another, the second led in from the
// chunk before, as the core sends them for two players asking far apart.
func makeInTwoRuns(
	t *testing.T, maker *chunks.Maker, source, storeDir string, grid []int64, stream nodeproto.ChunkStream,
) {
	t.Helper()
	codec := "h264"
	if strings.Contains(source, "open film") {
		codec = "hevc"
	}
	for _, first := range []int{0, 3} {
		command := nodeproto.MakeChunks{
			VideoStreamIndex: 0, VideoCodec: codec, Stream: stream, StoreDir: storeDir,
			FirstChunk: first, BoundsMs: grid[first : first+4],
		}
		if (stream.Kind.IsSound() || stream.Kind == nodeproto.StreamPicture) && first > 0 {
			command.LeadInMs = grid[first-1]
		}
		if stream.Kind == nodeproto.StreamPicture {
			command.PictureSeekMs, command.IsPictureSeekBeforeKeyframe = pictureSeek(source, grid, first)
		}
		if last, err := maker.Make(t.Context(), execx.Film{Path: source}, command, nil); err != nil || last != first+2 {
			t.Fatalf("the run from %d made through %d, %v", first, last, err)
		}
	}
}

// Chunks made by two runs far apart join into one stream: every picture once, at its time in the
// film, the second run's first chunk shown at its bound, the copied picture and the encoded quality
// alike, from MP4 and from Matroska, which FFmpeg seeks differently, and from an open GOP's HEVC,
// whose pictures shown before a keyframe FFmpeg times right only when led in (idiosyncrasies.md); and
// the sound continuous, its join under a frame (D247 §10; slow-links.md §10).
func TestMake_TwoRunsChunksJoinIntoOneStreamAtTheFilmsTimes(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	source := film(t)
	grid := keyframeGrid(t, runner, source)
	offset := float64(chunks.TimelineOffsetMs) / 1000
	pictures := sourceTimes(t, runner, source, "v:0", false)
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	matroska := inMatroska(t, source)
	openGOP := openGOPFilm(t)
	transport := remuxed(t, source, "a film (2020).ts")
	picture := nodeproto.ChunkStream{Kind: nodeproto.StreamPicture}
	quality := nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 300, Height: 96}
	// MPEG-TS carries its picture's packets in another framing than MP4: its copy's packets are
	// the film copied whole into MP4.
	transportCopied := remuxed(t, transport, "a film (2020) from MPEG-TS.mp4")
	timescales := map[string]uint32{}
	for storeDir, made := range map[string]struct {
		source, packetsAs string
		stream            nodeproto.ChunkStream
	}{
		"1/chunks-picture/0123456789abcdef":  {source, source, picture},
		"2/chunks-picture/0123456789abcdef":  {matroska, matroska, picture},
		"3/chunks-picture/0123456789abcdef":  {transport, transportCopied, picture},
		"4/chunks-picture/0123456789abcdef":  {openGOP, openGOP, picture},
		"1/chunks-q300k96p/0123456789abcdef": {source, source, quality},
		"2/chunks-q300k96p/0123456789abcdef": {matroska, matroska, quality},
		"3/chunks-q300k96p/0123456789abcdef": {transport, transportCopied, quality},
	} {
		makeInTwoRuns(t, maker, made.source, storeDir, grid, made.stream)
		timescales[storeDir] = initTimescale(t, filepath.Join(directory, storeDir))
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
	encodesCountInTheirCopiesTimescale(t, timescales, "1", "2", "3")
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

// encodesCountInTheirCopiesTimescale fails the test where a film's encoded quality's track counts in
// a timescale other than its copied picture's, the MP4's, Matroska's and MPEG-TS's alike, so hls.js
// takes no new time offset at a switch between them: timescales holds each store's, by its directory.
func encodesCountInTheirCopiesTimescale(t *testing.T, timescales map[string]uint32, files ...string) {
	t.Helper()
	for _, file := range files {
		copied := timescales[file+"/chunks-picture/0123456789abcdef"]
		encoded := timescales[file+"/chunks-q300k96p/0123456789abcdef"]
		if copied == 0 || encoded != copied {
			t.Errorf("film %s: the encode's track counts in 1/%d, the copy's in 1/%d; want the copy's",
				file, encoded, copied)
		}
	}
}

// filmWithSound is film, a keyframe every two seconds, with its sound in codec, of so many channels.
func filmWithSound(t *testing.T, codec string, channels int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a film in "+codec+" of "+strconv.Itoa(channels)+" channels (2020).mp4")
	testutil.GenerateMedia(t, path,
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=24:duration=12",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=12",
		"-c:v", "libx264", "-preset", "fast", "-bf", "2", "-g", "48", "-keyint_min", "48", "-sc_threshold", "0",
		"-c:a", codec, "-ac", strconv.Itoa(channels))
	return path
}

// packetHashes are the SHA-256 of each packet of stream, in decode order: its bytes, bit for bit.
func packetHashes(t *testing.T, runner *execx.Runner, path, stream string) []string {
	t.Helper()
	listed, err := runner.Run(t.Context(), execx.FFprobe, confine.Access{}, []string{
		"-v", "error", "-select_streams", stream, "-show_data_hash", "SHA256", "-show_entries", "packet=data_hash",
		"-of", "csv=p=0", "file:" + path,
	}, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	var hashes []string
	for line := range strings.Lines(string(listed)) {
		if hash := strings.Trim(strings.TrimSpace(line), ","); hash != "" {
			hashes = append(hashes, hash)
		}
	}
	return hashes
}

// streamFacts are a file's first stream's codec and channels, as ffprobe says them.
func streamFacts(t *testing.T, runner *execx.Runner, path string) string {
	t.Helper()
	listed, err := runner.Run(t.Context(), execx.FFprobe, confine.Access{}, []string{
		"-v", "error", "-select_streams", "0", "-show_entries", "stream=codec_name,channels", "-of", "csv=p=0",
		"file:" + path,
	}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Trim(strings.TrimSpace(string(listed)), ",")
}

// A sound copied, made in two runs far apart, is the film's own, every channel: its packets the
// source's, bit for bit, each once, at the source's times in the film, of a 5.1 AAC film, a stereo one
// and a 5.1 AC-3 one alike, but the first, an AAC's priming before the film, a tick before its next
// (copiedSoundTimes); the light track made of the same 5.1 film is stereo AAC, its packets none of the
// source's (D368 §1, §2).
func TestMake_ACopiedSoundIsTheFilmsOwnPacketsBitForBitAtTheirTimes(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	offset := float64(chunks.TimelineOffsetMs) / 1000
	for fileID, sound := range map[int]struct {
		codec    string
		channels int
	}{1: {"aac", 6}, 2: {"aac", 2}, 4: {"ac3", 6}} {
		channels := sound.channels
		source := filmWithSound(t, sound.codec, channels)
		grid := keyframeGrid(t, runner, source)
		storeDir := strconv.Itoa(fileID) + "/chunks-sound0-own/0123456789abcdef"
		makeInTwoRuns(t, maker, source, storeDir, grid, nodeproto.ChunkStream{
			Kind: nodeproto.StreamSoundCopied, Sound: &nodeproto.SoundTrack{ChannelCount: channels},
		})
		stored := joined(t, filepath.Join(directory, storeDir), len(grid)-1)
		if facts, want := streamFacts(t, runner, stored), sound.codec+","+strconv.Itoa(channels); facts != want {
			t.Errorf("%d channels: the copy is %s, want %s", channels, facts, want)
		}
		got, want := packetHashes(t, runner, stored, "0"), packetHashes(t, runner, source, "a:0")
		if !slices.Equal(got, want) {
			t.Errorf("%d channels: the copy's %d packets are not the film's %d, bit for bit, each once",
				channels, len(got), len(want))
		}
		times := packetTimes(t, runner, filepath.Join(directory, storeDir), len(grid)-1)
		filmTimes := sourceTimes(t, runner, source, "a:0", false)
		if len(times) != len(filmTimes) || len(times) < 2 || math.Abs(times[0]-offset) > 0.002 || times[0] >= times[1] {
			t.Fatalf("%d channels: %d packets from %v s; want the film's %d from its start, offset, in order",
				channels, len(times), times[:min(2, len(times))], len(filmTimes))
		}
		for index := 1; index < len(times); index++ {
			if got, want := times[index]-times[1], filmTimes[index]-filmTimes[1]; math.Abs(got-want) > 0.001 {
				t.Errorf("%d channels: packet %d is at %.4f s into the sound, want the film's %.4f s",
					channels, index, got, want)
				break
			}
		}
	}
	surround := filmWithSound(t, "aac", 6)
	grid := keyframeGrid(t, runner, surround)
	lightStore := "3/chunks-sound0-96k/0123456789abcdef"
	makeInTwoRuns(t, maker, surround, lightStore, grid, nodeproto.ChunkStream{
		Kind: nodeproto.StreamSound, SoundKbps: 96, Sound: &nodeproto.SoundTrack{ChannelCount: 6},
	})
	light := joined(t, filepath.Join(directory, lightStore), len(grid)-1)
	if facts := streamFacts(t, runner, light); facts != "aac,2" {
		t.Errorf("the light track is %s, want stereo AAC", facts)
	}
	own := map[string]bool{}
	for _, hash := range packetHashes(t, runner, surround, "a:0") {
		own[hash] = true
	}
	for _, hash := range packetHashes(t, runner, light, "0") {
		if own[hash] {
			t.Fatal("the light track holds a packet of the film's own sound; want it made")
		}
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
	transport := remuxed(t, filmWithKeyframesEvery(t, 1000), "a film (2020).ts")
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
	copied := remuxed(t, transport, "a film (2020) from MPEG-TS.mp4")
	if want := packetSizes(t, runner, copied, "v:0"); !slices.Equal(stored, want) {
		t.Errorf("the chunk holds %d pictures; want the film's %d, packet for packet", len(stored), len(want))
	}
}

// filmStartingPastItsFirstKeyframe is a 5.1 AAC film copied from 0.1 s in: the MP4 muxer's edit
// list puts its first keyframe, and its sound's priming, before the film's start, flagged discarded,
// so a read-through counts its next keyframe, 1.875 s in, as its first, and its grid starts there,
// the shape of A Hollow Continent's, whose next is 10.176 s in (staging, 2026-10-28).
func filmStartingPastItsFirstKeyframe(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a film cut past its first keyframe (2020).mp4")
	testutil.GenerateMedia(t, path, "-ss", "0.1", "-i", "file:"+filmWithSound(t, "aac", 6), "-map", "0", "-c", "copy")
	return path
}

// countedKeyframes are the film's picture's keyframes, in milliseconds, a read-through counts: those
// not flagged discarded (internal/node/readthrough).
func countedKeyframes(t *testing.T, runner *execx.Runner, source string) []int64 {
	t.Helper()
	listed, err := runner.Run(t.Context(), execx.FFprobe, confine.Access{}, []string{
		"-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time,flags", "-of", "csv=p=0",
		"file:" + source,
	}, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	var keyframes []int64
	for line := range strings.Lines(string(listed)) {
		value, flags, _ := strings.Cut(strings.TrimSpace(line), ",")
		shown, err := strconv.ParseFloat(value, 64)
		if err == nil && strings.Contains(flags, "K") && !strings.Contains(flags, "D") {
			keyframes = append(keyframes, int64(math.Round(shown*1000)))
		}
	}
	slices.Sort(keyframes)
	return keyframes
}

// A sound copied of a film whose grid starts past its start, its first keyframe before it dropped,
// is cut at chunk 0's start as its picture is: two runs' chunks are the film's packets from the
// first at or after chunk 0's start, bit for bit, each once, at their times; cut only at the bounds
// after its start, chunk 0 held the film from its start and was refused, and the film could not
// play (A Hollow Continent, staging, 2026-10-28).
func TestMake_ACopiedSoundOfAFilmWhoseGridStartsPastItsStartIsCutAtItsFirstChunk(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	source := filmStartingPastItsFirstKeyframe(t)
	keyframes := countedKeyframes(t, runner, source)
	if len(keyframes) != 5 || keyframes[0] < 1_000 {
		t.Fatalf("the film's counted keyframes are %v; want five, the first past a second in", keyframes)
	}
	grid := slices.Concat(keyframes, []int64{keyframes[len(keyframes)-1] + 2_000})
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	storeDir := "1/chunks-sound0-own/0123456789abcdef"
	stream := nodeproto.ChunkStream{Kind: nodeproto.StreamSoundCopied, Sound: &nodeproto.SoundTrack{ChannelCount: 6}}
	for _, command := range []nodeproto.MakeChunks{
		{Stream: stream, StoreDir: storeDir, FirstChunk: 0, BoundsMs: grid[0:3]},
		{Stream: stream, StoreDir: storeDir, FirstChunk: 2, BoundsMs: grid[2:], LeadInMs: grid[1]},
	} {
		wantLast := command.FirstChunk + len(command.BoundsMs) - 2
		last, err := maker.Make(t.Context(), execx.Film{Path: source}, command, nil)
		if err != nil || last != wantLast {
			t.Fatalf("the run from chunk %d made through %d, %v; want %d", command.FirstChunk, last, err, wantLast)
		}
	}
	chunkCount := len(grid) - 1
	stored := joined(t, filepath.Join(directory, storeDir), chunkCount)
	times := packetTimes(t, runner, filepath.Join(directory, storeDir), chunkCount)
	got := packetHashes(t, runner, stored, "0")
	filmTimes, filmHashes := sourceTimes(t, runner, source, "a:0", false), packetHashes(t, runner, source, "a:0")
	const frame = 1024.0 / 48000
	offset := float64(chunks.TimelineOffsetMs) / 1000
	first := float64(grid[0]) / 1000
	if len(times) == 0 {
		t.Fatal("the copy holds no sound")
	}
	starts := times[0] - offset
	if starts < first-0.001 || starts > first+frame {
		t.Fatalf("the sound starts at %.4f s; want the first packet at or after chunk 0's start, %.3f s", starts, first)
	}
	from := slices.IndexFunc(filmTimes, func(shown float64) bool { return shown >= starts-0.0005 })
	if from < 0 || from+len(got) > len(filmHashes) || !slices.Equal(got, filmHashes[from:from+len(got)]) {
		t.Fatalf("the copy's %d packets are not the film's from %.4f s, bit for bit, each once", len(got), starts)
	}
	for index, shown := range times {
		if want := filmTimes[from+index] + offset; math.Abs(shown-want) > 0.001 {
			t.Errorf("packet %d is at %.4f s, want the film's %.4f s, offset", index, shown, want)
			break
		}
	}
}

// A film whose first keyframe its edit shows before its start, flagged discarded, is cut from its
// start: its index counts that keyframe at 0, before the next keyframes a read-through counts, and
// its copied picture's chunks from that grid, made in two runs, hold every picture of the film once,
// the first chunk's from its start; cut from its next keyframe, its first seconds never played (the
// owner's film, staging, 2026-11-04).
func TestMake_AFilmWhoseFirstKeyframeIsBeforeItsStartIsCutFromItsStart(t *testing.T) {
	t.Parallel()
	runner, err := execx.NewRunner(testutil.FFmpegDirectory(t))
	if err != nil {
		t.Fatal(err)
	}
	source := filmStartingPastItsFirstKeyframe(t)
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	indexed, err := container.KeyframesMs(bytes.NewReader(contents), int64(len(contents)), 0)
	counted := countedKeyframes(t, runner, source)
	// The index puts a keyframe of a film under an edit a picture later than ffprobe (code-debt.md).
	const picture = 1000 / 24
	isCounted := len(indexed) == len(counted)+1 && len(indexed) >= 6 && indexed[0] == 0
	for index := 1; isCounted && index < len(indexed); index++ {
		isCounted = math.Abs(float64(indexed[index]-counted[index-1])) <= picture
	}
	if err != nil || !isCounted {
		t.Fatalf("the index's keyframes are %v, %v; want 0, then those counted, %v", indexed, err, counted)
	}
	grid := slices.Concat(indexed[:6], []int64{indexed[5] + 2_000})
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	maker := chunks.NewMaker(runner, shelf, directory, encode.EncoderX264)
	storeDir := "1/chunks-picture/0123456789abcdef"
	makeInTwoRuns(t, maker, source, storeDir, grid, nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	stored := packetSizes(t, runner, joined(t, filepath.Join(directory, storeDir), len(grid)-1), "0")
	if want := packetSizes(t, runner, source, "v:0"); !slices.Equal(stored, want) {
		t.Errorf("the chunks hold %d pictures; want the film's %d, packet for packet", len(stored), len(want))
	}
	times := packetTimes(t, runner, filepath.Join(directory, storeDir), len(grid)-1)
	offset := float64(chunks.TimelineOffsetMs) / 1000
	if len(times) == 0 || times[0] < offset-0.001 || times[0] > offset+0.25 {
		t.Errorf("the first picture is shown at %v; want it at the film's start, %.3f s, or a few frames after",
			times, offset)
	}
}
