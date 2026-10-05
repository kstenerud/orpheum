// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// fakeFFmpeg stands in for FFmpeg's segment muxer: it writes raw, one file after another into the
// run's raw directory, listing each once it is whole, and says its progress after each; then it
// ends with err.
type fakeFFmpeg struct {
	raw     map[int][]byte
	numbers []int
	err     error
	ran     execx.WatchedRun
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
		list += name + ",0,0\n"
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
		clocktest.NewFake(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)))
	return NewMaker(fake, shelf, directory, prepare.EncoderX264), shelf, directory
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
	last, err := maker.Make(t.Context(), "/films/a.mp4", command, func(n int) { told = append(told, n) })
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

// A sound's lead-in is dropped, and each chunk starts after the run's samples before it, from the
// run's start less the encoder's priming (slow-links.md §10).
func TestMake_TimesASoundBySamplesAfterItsLeadIn(t *testing.T) {
	t.Parallel()
	command := run(nodeproto.ChunkStream{Kind: nodeproto.StreamSound, Sound: &nodeproto.PrepareAudio{ChannelCount: 2}})
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
	if _, err := maker.Make(t.Context(), "/films/a.mp4", command, nil); err != nil {
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
	last, err := maker.Make(t.Context(), "/films/a.mp4", command, nil)
	if err == nil || last != 10 || !shelf.HasFile(command.StoreDir, ChunkName(10)) {
		t.Errorf("a failed run made through %d, %v; want chunk 10 kept and the failure said", last, err)
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
	for name, command := range map[string]nodeproto.MakeChunks{
		"a store outside the shape": climbing, "bounds not rising": falling,
		"a sound with no track": soundless, "a lead-in on a picture": leadInOnAPicture,
	} {
		fake := &fakeFFmpeg{}
		maker, _, directory := makerOver(t, fake)
		if _, err := maker.Make(t.Context(), "/films/a.mp4", command, nil); !errors.Is(err, ErrBadRun) {
			t.Errorf("%s: %v, want it refused", name, err)
		}
		if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 || fake.ran.Arguments != nil {
			t.Errorf("%s: something was made or run", name)
		}
	}
}
