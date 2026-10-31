// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/encode"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// fakeFFmpeg stands in for FFmpeg's segment muxer: it writes raw, one file after another into the
// run's raw directory, listing each once it is whole, at the span between the cuts it was told, or
// as spans says, and says its progress after each; then it ends with err.
type fakeFFmpeg struct {
	raw     map[int][]byte
	numbers []int
	spans   map[int]string
	err     error
	ran     execx.WatchedRun
}

// listedSpan is raw file number's span as the muxer lists it, "<start>,<end>" in seconds: between
// the cuts around it, or a tenth of a second past the last for what is written past the run.
func (fake *fakeFFmpeg) listedSpan(arguments []string, number int) string {
	if span, isSet := fake.spans[number]; isSet {
		return span
	}
	first, _ := strconv.Atoi(valueOf(arguments, "-segment_start_number")) //nolint:errcheck // the maker's own
	cuts := []string{"0"}
	cuts = append(cuts, strings.Split(valueOf(arguments, "-segment_times"), ",")...)
	index := number - first
	if index+1 < len(cuts) {
		return cuts[index] + "," + cuts[index+1]
	}
	last, _ := strconv.ParseFloat(cuts[len(cuts)-1], 64) //nolint:errcheck // the maker's own
	return cuts[len(cuts)-1] + "," + strconv.FormatFloat(last+0.1, 'f', 3, 64)
}

func (fake *fakeFFmpeg) RunWatched(_ context.Context, run execx.WatchedRun) (execx.Ran, error) {
	fake.ran = run
	listPath := valueOf(run.Arguments, "-segment_list")
	rawPath := filepath.Dir(listPath)
	list := ""
	for _, number := range fake.numbers {
		name := fmt.Sprintf("%05d.mp4", number)
		if err := os.WriteFile(filepath.Join(rawPath, name), fake.raw[number], 0o600); err != nil {
			return execx.Ran{}, err
		}
		list += name + "," + fake.listedSpan(run.Arguments, number) + "\n"
		if err := os.WriteFile(listPath, []byte(list), 0o600); err != nil {
			return execx.Ran{}, err
		}
		run.Watch.OnProgress(execx.Progress{Position: time.Second})
	}
	return execx.Ran{}, fake.err
}

// makerOver is a maker writing into a fresh prepared directory, with FFmpeg faked.
func makerOver(t *testing.T, fake *fakeFFmpeg) (*Maker, *prepared.Shelf, string) {
	t.Helper()
	directory := t.TempDir()
	shelf := prepared.New(directory, prepared.Budget{MaxBytes: 1 << 30, DiskUsedLimitPercent: 100},
		clocktest.NewFake(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	return NewMaker(fake, shelf, directory, encode.EncoderX264), shelf, directory
}

// storedDecodeTime is the first decode time of chunk n in the store.
func storedDecodeTime(t *testing.T, directory, storeDir string, n int) uint64 {
	t.Helper()
	chunk, err := os.ReadFile(filepath.Join(directory, storeDir, ChunkName(n)))
	if err != nil {
		t.Fatal(err)
	}
	return decodeTimesOf(t, chunk)[0]
}

// A picture's chunks are made from FFmpeg's raw files as each is listed, each at its time in the
// film less its first sample's composition offset, so it is shown at its bound; the init is written
// once, what FFmpeg wrote past the run is dropped, and the raw directory goes (D247 §10).
func TestMake_PutsEachChunkAtItsTimeAsFFmpegFinishesIt(t *testing.T) {
	t.Parallel()
	command := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	fake := &fakeFFmpeg{
		raw: map[int][]byte{
			10: rawOf(24000, fragmentOf(0, 240, 2002)),
			11: rawOf(24000, fragmentOf(0, 90, 2002)),
			12: rawOf(24000, fragmentOf(0, 239, 2002), fragmentOf(5000, 10, 0)),
			13: rawOf(24000, fragmentOf(0, 2, 2002)), // past the run's end
		},
		numbers: []int{10, 11, 12, 13},
	}
	maker, shelf, directory := makerOver(t, fake)
	var told []int
	last, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mp4"}, command,
		func(n int) { told = append(told, n) })
	if err != nil || last != 12 {
		t.Fatalf("the run made through %d, %v; want 12", last, err)
	}
	if !slices.Equal(told, []int{10, 11, 12}) {
		t.Errorf("the chunks told are %v, want 10, 11, 12 in order", told)
	}
	for index, n := range []int{10, 11, 12} {
		ticks := (command.BoundsMs[index]+TimelineOffsetMs)*24000/1000 - 2002
		want := uint64(ticks) //nolint:gosec // a test's positive time
		if got := storedDecodeTime(t, directory, command.StoreDir, n); got != want {
			t.Errorf("chunk %d is decoded from %d, want %d: shown at its bound", n, got, want)
		}
	}
	if shelf.HasFile(command.StoreDir, ChunkName(13)) {
		t.Error("what FFmpeg wrote past the run's end was made a chunk")
	}
	if init, err := os.ReadFile(filepath.Join(directory, command.StoreDir, InitName)); err != nil ||
		string(init) != string(initOf(24000)) {
		t.Errorf("the store's init is %d bytes, %v; want the raw files' init", len(init), err)
	}
	if raws, err := filepath.Glob(filepath.Join(directory, command.StoreDir, "raw-*")); err != nil || len(raws) != 0 {
		t.Errorf("the run left its raw directory: %v", raws)
	}
	if !slices.Contains(fake.ran.Access.Write, filepath.Join(directory, command.StoreDir)) {
		t.Errorf("FFmpeg may write %v; want the store", fake.ran.Access.Write)
	}
}

// An MPEG-TS picture's run seeks at or before its first keyframe, FFmpeg seeking it to a frame, not
// a keyframe (idiosyncrasies.md): the film's first run from the film's start, a later one from
// before its keyframe; each makes its chunks.
func TestMake_AnMPEGTSPicturesRunSeeksAtOrBeforeItsFirstKeyframe(t *testing.T) {
	t.Parallel()
	first := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	first.FirstChunk, first.BoundsMs = 0, []int64{0, 10_010, 20_020}
	later := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	later.PictureSeekMs = 93_844
	for name, command := range map[string]nodeproto.MakeChunks{"the film's first": first, "a later": later} {
		command.IsPictureSeekBeforeKeyframe = true
		if name == "the film's first" {
			command.PictureSeekMs = 0
		}
		lastOfRun := command.FirstChunk + len(command.BoundsMs) - 2
		fake := &fakeFFmpeg{raw: map[int][]byte{}}
		for n := command.FirstChunk; n <= lastOfRun; n++ {
			fake.raw[n] = rawOf(24000, fragmentOf(0, 240, 0))
			fake.numbers = append(fake.numbers, n)
		}
		maker, _, _ := makerOver(t, fake)
		if last, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.ts"}, command, nil); err != nil ||
			last != lastOfRun {
			t.Errorf("%s run seeking to %d ms made through %d, %v; want %d", name, command.PictureSeekMs, last, err,
				lastOfRun)
		}
	}
}

// A sound's lead-in is dropped, and each chunk starts after the run's samples before it, from the
// run's start less the encoder's priming (slow-links.md §10).
func TestMake_TimesASoundBySamplesAfterItsLeadIn(t *testing.T) {
	t.Parallel()
	command := run(nodeproto.ChunkStream{Kind: nodeproto.StreamSound, Sound: &nodeproto.SoundTrack{ChannelCount: 2}})
	command.SoundLeadInMs = 88_839
	fake := &fakeFFmpeg{
		raw: map[int][]byte{
			9:  rawOf(48000, fragmentOf(0, 470, 0)),
			10: rawOf(48000, fragmentOf(0, 469, 0)),
			11: rawOf(48000, fragmentOf(0, 176, 0)),
		},
		numbers: []int{9, 10, 11},
	}
	maker, shelf, directory := makerOver(t, fake)
	if _, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mp4"}, command, nil); err != nil {
		t.Fatal(err)
	}
	if shelf.HasFile(command.StoreDir, ChunkName(9)) {
		t.Error("the lead-in was made a chunk")
	}
	start := (int64(88_839) + TimelineOffsetMs) * 48 // the run's start, in ticks
	if got := storedDecodeTime(t, directory, command.StoreDir, 10); got != uint64(start-aacPriming+470*1024) {
		t.Errorf("chunk 10 starts at %d, want after the lead-in's 470 frames, less the priming", got)
	}
	if got := storedDecodeTime(t, directory, command.StoreDir, 11); got != uint64(start-aacPriming+(470+469)*1024) {
		t.Errorf("chunk 11 starts at %d, want after 939 frames, less the priming", got)
	}
}

// A sound copied has its lead-in dropped, and each chunk ends where the muxer lists it ending, at the
// source's own time in the film to the microsecond, and starts as long before as its samples last:
// not at its bound, nor after a count of frames, nor at the list's start, which for a run's first
// file is no packet's (D368 §1).
func TestMake_TimesACopiedSoundByItsListedEndInTheFilm(t *testing.T) {
	t.Parallel()
	command := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSoundCopied, Sound: &nodeproto.SoundTrack{ChannelCount: 6},
	})
	command.SoundLeadInMs = 88_839
	fake := &fakeFFmpeg{
		raw: map[int][]byte{
			9:  rawOf(48000, fragmentOf(0, 470, 0)),
			10: rawOf(48000, lastingFragmentOf(469)),
			11: rawOf(48000, lastingFragmentOf(176)),
			12: rawOf(48000, lastingFragmentOf(467)),
		},
		numbers: []int{9, 10, 11, 12},
		spans: map[int]string{
			9: "0.000000,98.858667", 10: "98.858667,108.864000", 11: "108.864000,112.618667",
			12: "112.618667,122.581333",
		},
	}
	maker, shelf, directory := makerOver(t, fake)
	if last, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mp4"}, command, nil); err != nil || last != 12 {
		t.Fatalf("the run made through %d, %v; want chunk 12", last, err)
	}
	if shelf.HasFile(command.StoreDir, ChunkName(9)) {
		t.Error("the lead-in was made a chunk")
	}
	for chunk, end := range map[int]struct {
		seconds float64
		samples uint64
	}{10: {108.864, 469}, 11: {112.618667, 176}, 12: {122.581333, 467}} {
		want := uint64(math.Round((end.seconds+float64(TimelineOffsetMs)/1000)*48000)) - end.samples*aacFrameTicks
		if got := storedDecodeTime(t, directory, command.StoreDir, chunk); got != want {
			t.Errorf("chunk %d starts at %d, want %d: its listed end in the film, offset, less its %d samples",
				chunk, got, want, end.samples)
		}
	}
	// A run from the film's start: its first file is listed from zero, its first packet, the priming,
	// moved to a tick before the film, lasting a tick to the next.
	first := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSoundCopied, Sound: &nodeproto.SoundTrack{ChannelCount: 2},
	})
	first.FirstChunk, first.BoundsMs = 0, []int64{21, 2_005}
	ticks := []uint32{1}
	for range 94 {
		ticks = append(ticks, 1024)
	}
	fromStart := &fakeFFmpeg{
		raw: map[int][]byte{0: rawOf(48000, timedFragmentOf(ticks...))}, numbers: []int{0},
		spans: map[int]string{0: "0.000000,2.005333"},
	}
	maker, _, directory = makerOver(t, fromStart)
	if last, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mp4"}, first, nil); err != nil || last != 0 {
		t.Fatalf("the run from the start made through %d, %v; want chunk 0", last, err)
	}
	if got, want := storedDecodeTime(t, directory, first.StoreDir, 0), uint64(TimelineOffsetMs*48-1); got != want {
		t.Errorf("chunk 0 starts at %d, want %d: a tick before the film, offset", got, want)
	}
}

// A sound copied whose film's first chunk starts 10.176 s in makes chunk 0 of the raw file after its
// lead-in from the film's start, which is dropped, each chunk placed by its listed end; a raw file
// holding the film from its start to chunk 0's end, as the muxer wrote one when not cut at chunk 0's
// start, is never made a chunk, and the failure says where the run read from and first cut (A
// Hollow Continent, staging, 2026-10-28).
func TestMake_ACopiedSoundWhoseFilmsFirstChunkStartsLateDropsWhatIsBeforeIt(t *testing.T) {
	t.Parallel()
	command := run(nodeproto.ChunkStream{
		Kind: nodeproto.StreamSoundCopied, Sound: &nodeproto.SoundTrack{ChannelCount: 6},
	})
	command.FirstChunk, command.BoundsMs = 0, []int64{10_176, 12_262, 22_522}
	fake := &fakeFFmpeg{
		raw: map[int][]byte{
			0: rawOf(48000, lastingFragmentOf(478)),
			1: rawOf(48000, lastingFragmentOf(97)),
			2: rawOf(48000, lastingFragmentOf(481)),
		},
		numbers: []int{0, 1, 2},
		spans:   map[int]string{0: "0.000000,10.197167", 1: "10.197167,12.266500", 2: "12.266500,22.527167"},
	}
	maker, shelf, directory := makerOver(t, fake)
	if last, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mp4"}, command, nil); err != nil || last != 1 {
		t.Fatalf("the run made through %d, %v; want chunk 1", last, err)
	}
	for chunk, end := range map[int]struct {
		seconds float64
		samples uint64
	}{0: {12.2665, 97}, 1: {22.527167, 481}} {
		want := uint64(math.Round((end.seconds+float64(TimelineOffsetMs)/1000)*48000)) - end.samples*aacFrameTicks
		if got := storedDecodeTime(t, directory, command.StoreDir, chunk); got != want {
			t.Errorf("chunk %d starts at %d, want %d: its listed end in the film, offset, less its %d samples",
				chunk, got, want, end.samples)
		}
	}
	if shelf.HasFile(command.StoreDir, ChunkName(2)) {
		t.Error("a chunk past the run's was made")
	}
	whole := &fakeFFmpeg{
		raw:     map[int][]byte{0: rawOf(48000, lastingFragmentOf(575))},
		numbers: []int{0}, spans: map[int]string{0: "0.000000,12.266500"},
	}
	maker, shelf, _ = makerOver(t, whole)
	last, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mp4"}, command, nil)
	isSaid := err != nil && strings.Contains(err.Error(), "read from 0 ms, its first cut at 10.176 s")
	if last != -1 || !errors.Is(err, errNotHeld) || !isSaid {
		t.Errorf("a raw file of the film from its start to chunk 0's end made through %d, %v; want none, saying "+
			"where the run read from and first cut", last, err)
	}
	if shelf.HasFile(command.StoreDir, ChunkName(0)) {
		t.Error("chunk 0 was made of the film from its start")
	}
}

// A run FFmpeg fails keeps the chunks it finished, and says how far it got.
func TestMake_AFailedRunKeepsWhatItFinished(t *testing.T) {
	t.Parallel()
	command := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
	fake := &fakeFFmpeg{
		raw:     map[int][]byte{10: rawOf(24000, fragmentOf(0, 240, 0))},
		numbers: []int{10},
		err:     errors.New("Invalid data found when processing input"),
	}
	maker, shelf, _ := makerOver(t, fake)
	last, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mp4"}, command, nil)
	if err == nil || last != 10 || !shelf.HasFile(command.StoreDir, ChunkName(10)) {
		t.Errorf("a failed run made through %d, %v; want chunk 10 kept and the failure said", last, err)
	}
}

// FFmpeg writes only in the store the command names, inside the node's prepared directory: the one
// directory its confinement may write and every path its arguments name are there. A store the core
// names any other way, absolute, climbing, or not of a store's shape, is refused before anything is
// made or run, so no request steers where FFmpeg writes, as Plex2Shell's transcoder let one
// (security-week-2026-10-30 §5).
func TestMake_FFmpegWritesOnlyInItsStoreInsideThePreparedDirectory(t *testing.T) {
	t.Parallel()
	command := run(nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 300, Height: 96})
	command.PictureSeekMs = 0
	fake := &fakeFFmpeg{}
	maker, _, directory := makerOver(t, fake)
	if _, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mp4"}, command, nil); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(directory, command.StoreDir)
	if !slices.Equal(fake.ran.Access.Write, []string{store}) {
		t.Errorf("FFmpeg may write %q, want only its store %s", fake.ran.Access.Write, store)
	}
	for _, argument := range fake.ran.Arguments {
		if filepath.IsAbs(argument) && !strings.HasPrefix(argument, store+string(filepath.Separator)) {
			t.Errorf("FFmpeg was told to write %s, outside its store %s", argument, store)
		}
	}

	for _, storeDir := range []string{
		"/etc/0123456789abcdef", "../7/chunks-x/0123456789abcdef", "7/chunks-x/../0123456789abcdef",
		"7/chunks-x/0123456789abcdef/..", "7/chunks-x/0123456789abcdef/../../../../etc",
		"./7/chunks-x/0123456789abcdef", "7//chunks-x/0123456789abcdef", "7/chunks-x/0123456789abcdef/",
		"7/chunks-x/0123456789abcdef\n", "7/chunks-x/0123456789abcdef\x00/../..", `7\..\..\chunks-x\0123456789abcdef`,
		"7/chunks-../0123456789abcdef", "",
	} {
		hostile := command
		hostile.StoreDir = storeDir
		fake := &fakeFFmpeg{}
		maker, _, directory := makerOver(t, fake)
		if _, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mp4"}, hostile, nil); !errors.Is(err, ErrBadRun) {
			t.Errorf("the store %q: %v, want it refused", storeDir, err)
		}
		if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 || fake.ran.Arguments != nil {
			t.Errorf("the store %q: something was made or run", storeDir)
		}
	}
}

// A run not of a run's shape is refused before anything is made.
func TestMake_RefusesARunNotOfARunsShape(t *testing.T) {
	t.Parallel()
	picture := nodeproto.ChunkStream{Kind: nodeproto.StreamPicture}
	climbing := run(picture)
	climbing.StoreDir = "7/../../etc/0123456789abcdef"
	falling := run(picture)
	falling.BoundsMs = []int64{5000, 4000}
	soundless := run(nodeproto.ChunkStream{Kind: nodeproto.StreamSound})
	leadInOnAPicture := run(picture)
	leadInOnAPicture.SoundLeadInMs = 1000
	seekAtTheKeyframe := run(picture)
	seekAtTheKeyframe.PictureSeekMs = seekAtTheKeyframe.BoundsMs[0]
	seekPastTheNextChunk := run(picture)
	seekPastTheNextChunk.PictureSeekMs = seekPastTheNextChunk.BoundsMs[1]
	seekingQuality := run(nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 300, Height: 96})
	seekingQuality.PictureSeekMs = 103_854
	beforeButPast := run(picture)
	beforeButPast.IsPictureSeekBeforeKeyframe = true
	qualityBefore := run(nodeproto.ChunkStream{Kind: nodeproto.StreamQuality, BitRateKbps: 300, Height: 96})
	qualityBefore.IsPictureSeekBeforeKeyframe = true
	for name, command := range map[string]nodeproto.MakeChunks{
		"a store outside the shape": climbing, "bounds not rising": falling,
		"a sound with no track": soundless, "a lead-in on a picture": leadInOnAPicture,
		"a picture seeking at its keyframe":                  seekAtTheKeyframe,
		"a picture seeking past its first chunk":             seekPastTheNextChunk,
		"a quality with a picture's seek":                    seekingQuality,
		"a picture to seek before its keyframe seeking past": beforeButPast,
		"a quality told to seek before its keyframe":         qualityBefore,
	} {
		fake := &fakeFFmpeg{}
		maker, _, directory := makerOver(t, fake)
		_, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mp4"}, command, nil)
		if !errors.Is(err, ErrBadRun) {
			t.Errorf("%s: %v, want it refused", name, err)
		}
		if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 || fake.ran.Arguments != nil {
			t.Errorf("%s: something was made or run", name)
		}
	}
}

// A raw file whose listed span is not its chunk's is never made a chunk, and fails the run, keeping
// the chunks before it: FFmpeg missed a cut or cut a keyframe off, or was stopped in the file and
// listed it (idiosyncrasies.md). The run's last chunk, which FFmpeg's
// end may close before its bound, as at a film's end, is taken only when FFmpeg ended of itself.
func TestMake_NeverMakesAChunkOfAFileNotCutAtItsBounds(t *testing.T) {
	t.Parallel()
	picture := func(numbers ...int) map[int][]byte {
		raw := map[int][]byte{}
		for _, number := range numbers {
			raw[number] = rawOf(24000, fragmentOf(0, 240, 0))
		}
		return raw
	}
	stalled := errors.New("stalled")
	for _, test := range []struct {
		name      string
		spans     map[int]string
		numbers   []int
		err       error
		wantLast  int
		wantError bool
	}{
		{"a cut missed, holding two chunks", map[int]string{10: "0,13.714"}, []int{10, 11, 12, 13}, nil, -1, true},
		{"a cut a keyframe early", map[int]string{11: "9.300,14.000"}, []int{10, 11, 12, 13}, nil, 10, true},
		{"a file FFmpeg was stopped in", map[int]string{11: "9.960,11.000"}, []int{10, 11}, stalled, 10, true},
		{"the last chunk, stopped in", map[int]string{12: "13.714,15.000"}, []int{10, 11, 12}, stalled, 11, true},
		{
			"the last chunk, ended early by the film's end",
			map[int]string{12: "13.714,22.900"},
			[]int{10, 11, 12},
			nil, 12, false,
		},
	} {
		fake := &fakeFFmpeg{raw: picture(test.numbers...), numbers: test.numbers, spans: test.spans, err: test.err}
		maker, shelf, _ := makerOver(t, fake)
		command := run(nodeproto.ChunkStream{Kind: nodeproto.StreamPicture})
		last, err := maker.Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, command, nil)
		if last != test.wantLast || (err != nil) != test.wantError {
			t.Errorf("%s: the run made through %d, %v; want %d, failing %v", test.name, last, err, test.wantLast,
				test.wantError)
		}
		for n := test.wantLast + 1; n <= 12; n++ {
			if shelf.HasFile(command.StoreDir, ChunkName(n)) {
				t.Errorf("%s: chunk %d was made of a file not cut at its bounds", test.name, n)
			}
		}
	}
}
