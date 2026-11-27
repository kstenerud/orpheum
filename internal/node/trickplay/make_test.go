// SPDX-License-Identifier: AGPL-3.0-or-later

package trickplay

import (
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// webpOf is a WebP-shaped file of body, of an even length as a RIFF chunk is: its header and body.
func webpOf(body string) []byte {
	size := uint32(4 + len(body)) //nolint:gosec // short
	file := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, size)...)
	return append(append(file, "WEBP"...), body...)
}

// fakeRunner writes its output to a run's stdout in the pieces given, and records the run.
type fakeRunner struct {
	pieces [][]byte
	ran    execx.WatchedRun
	err    error
}

func (fake *fakeRunner) RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error) {
	fake.ran = run
	for _, piece := range fake.pieces {
		if _, err := run.StdoutTo.Write(piece); err != nil {
			return execx.Ran{}, errors.Join(err, ctx.Err())
		}
	}
	return execx.Ran{}, fake.err
}

func testCommand() nodeproto.MakeTrickplay {
	return nodeproto.MakeTrickplay{
		IntervalMs: 10000, PictureWidth: 320, SheetColumns: 10, SheetRows: 10, WebPQuality: 60,
	}
}

// trickplay.md §3: the sheets are cut from FFmpeg's output however it comes, each given with its
// index, from the first not made, in order, those already made not given again.
func TestMake_GivesEachSheetAsItIsCutFromTheOutputAndNoneAlreadyMade(t *testing.T) {
	t.Parallel()
	sheets := [][]byte{webpOf("twos"), webpOf("three!"), webpOf("four")}
	output := slices.Concat(sheets...)
	// Cut mid-header and mid-body.
	fake := &fakeRunner{pieces: [][]byte{output[:5], output[5:20], output[20:]}}
	command := testCommand()
	command.SheetsMade = []int{0, 1, 3}
	var given []int
	err := NewMaker(fake, 1<<20).Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, command,
		func(index int, webp []byte) error {
			given = append(given, index)
			if want := sheets[index-2]; !slices.Equal(webp, want) {
				t.Errorf("sheet %d is %q; want %q", index, webp, want)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(given, []int{2, 4}) {
		t.Errorf("gave sheets %v; want 2 and 4, from the first not made, not 3, made already", given)
	}
	if !slices.Contains(fake.ran.Arguments, "-ss") || !fake.ran.IsIdle {
		t.Errorf("ran %+v; want a run sought to sheet 2, idle", fake.ran)
	}
}

// trickplay.md §3: a run from the start does not seek, and its run is idle.
func TestMake_ARunFromTheStartSeeksNowhereAndIsIdle(t *testing.T) {
	t.Parallel()
	fake := &fakeRunner{}
	if err := NewMaker(fake, 1<<20).Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, testCommand(),
		func(int, []byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(fake.ran.Arguments, "-ss") || !fake.ran.IsIdle {
		t.Errorf("ran %+v; want an idle run from the start", fake.ran)
	}
}

// A sheet its taker refuses ends the run, which answers the refusal, not a cancel; so does output
// that is not a WebP, and output that ends partway through one.
func TestMake_ASheetRefusedOrOutputThatIsNotWholeSheetsEndsTheRun(t *testing.T) {
	t.Parallel()
	refused := errors.New("the core is gone")
	for _, check := range []struct {
		what   string
		pieces [][]byte
		taker  SheetMade
		want   func(error) bool
	}{
		{
			"a refused sheet",
			[][]byte{webpOf("ones"), webpOf("twos")},
			func(int, []byte) error { return refused },
			func(err error) bool { return errors.Is(err, refused) && !errors.Is(err, context.Canceled) },
		},
		{
			"output not a WebP",
			[][]byte{[]byte("this is not a webp file")},
			func(int, []byte) error { return nil },
			func(err error) bool { return err != nil && !errors.Is(err, context.Canceled) },
		},
		{
			"output ended partway",
			[][]byte{webpOf("ones")[:14]},
			func(int, []byte) error { return nil },
			func(err error) bool { return err != nil },
		},
	} {
		fake := &fakeRunner{pieces: check.pieces}
		err := NewMaker(fake, 1<<20).Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, testCommand(), check.taker)
		if !check.want(err) {
			t.Errorf("%s ended the run %v", check.what, err)
		}
	}
}

// Sizes no sheet can be made of are refused before FFmpeg runs.
func TestMake_RefusesSizesNoSheetCanBeMadeOf(t *testing.T) {
	t.Parallel()
	for _, change := range []func(*nodeproto.MakeTrickplay){
		func(command *nodeproto.MakeTrickplay) { command.IntervalMs = 0 },
		func(command *nodeproto.MakeTrickplay) { command.PictureWidth = 0 },
		func(command *nodeproto.MakeTrickplay) { command.SheetColumns = 0 },
		func(command *nodeproto.MakeTrickplay) { command.SheetRows = -1 },
		func(command *nodeproto.MakeTrickplay) { command.WebPQuality = 101 },
	} {
		command := testCommand()
		change(&command)
		fake := &fakeRunner{}
		err := NewMaker(fake, 1<<20).Make(t.Context(), execx.Film{Path: "/films/a.mkv"}, command,
			func(int, []byte) error { return nil })
		if err == nil || fake.ran.Arguments != nil {
			t.Errorf("%+v ran FFmpeg, ending %v; want it refused first", command, err)
		}
	}
}
