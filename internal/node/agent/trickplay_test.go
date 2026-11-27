// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/node/trickplay"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeTrickplayer gives a sheet for each index not made of its sheets, in order, stopping at the
// first its taker refuses, and records the command it was given.
type fakeTrickplayer struct {
	mutex  sync.Mutex
	sheets int
	asked  []nodeproto.MakeTrickplay
}

func (fake *fakeTrickplayer) Make(
	_ context.Context, _ execx.Film, command nodeproto.MakeTrickplay, sheetMade trickplay.SheetMade,
) error {
	fake.mutex.Lock()
	fake.asked = append(fake.asked, command)
	fake.mutex.Unlock()
	for index := range fake.sheets {
		if slices.Contains(command.SheetsMade, index) {
			continue
		}
		if err := sheetMade(index, sheetBytes(index)); err != nil {
			return err
		}
	}
	return nil
}

// sheetBytes is the fake sheet of index, or of an unreadable size at index unreadableSheet: an
// extended WebP's header of 10 rows of pictures 180 px high, its width the index's, so each sheet
// is its own.
func sheetBytes(index int) []byte {
	if index == unreadableSheet {
		return []byte(strconv.Itoa(index))
	}
	width := uint32(3200 + index) //nolint:gosec // a test's few sheets
	sheet := []byte("RIFF\x16\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x00\x00\x00\x00")
	sheet = append(sheet, binary.LittleEndian.AppendUint32(nil, width-1)[:3]...)
	return append(sheet, 0x07, 0x07, 0) // 1800 high, less one
}

// unreadableSheet is the index of the one sheet sheetBytes gives no size of.
const unreadableSheet = 7

// trickplayCommand asks for the trickplay of a file written to root as "film", named as hashed.
func trickplayCommand(t *testing.T, root, hashed string, made []int) nodeproto.Command {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "film.mkv"), []byte("film"), 0o600); err != nil {
		t.Fatal(err)
	}
	return nodeproto.Command{CommandSeq: 1, JobID: 9, AttemptToken: "attempt", Work: nodeproto.Work{
		Type: nodeproto.CommandMakeTrickplay,
		MakeTrickplay: &nodeproto.MakeTrickplay{
			Video: nodeproto.BytesFile{
				RootPath: root, Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, hashed),
			},
			IntervalMs: 10000, PictureWidth: 320, SheetColumns: 10, SheetRows: 10, WebPQuality: 60,
			SheetsMade: made,
		},
	}}
}

func quickRetry(node *Agent) {
	node.answerRetry = answerRetry{firstWait: time.Millisecond, longestWait: 2 * time.Millisecond}
}

// trickplay.md §3: each sheet is posted to the core as a piece of its own as it is made, in order,
// one the core did not take posted again until it does, those made already not again; then the run's
// end.
func TestAgent_PostsEachTrickplaySheetUntilTakenThenTheRunsEnd(t *testing.T) {
	t.Parallel()
	fake := &fakeTrickplayer{sheets: 4}
	core, socket := startFakeCore(t)
	core.sheetStatuses = []int{http.StatusServiceUnavailable}
	root, _ := startAgentWithWorkers(t, socket,
		Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Trickplayer: fake}, quickRetry)
	core.commands <- trickplayCommand(t, root.Path(), "film", []int{1})
	var posted []int
	for range 3 {
		sheet := testutil.Receive(t, core.sheets, "a sheet")
		if sheet.JobID != 9 || sheet.AttemptToken != "attempt" || !slices.Equal(sheet.WebP, sheetBytes(sheet.Index)) ||
			sheet.PictureHeight != 180 {
			t.Errorf("posted %+v; want sheet %d of job 9's attempt, its pictures 180 px high", sheet, sheet.Index)
		}
		posted = append(posted, sheet.Index)
	}
	if !slices.Equal(posted, []int{0, 2, 3}) {
		t.Errorf("posted sheets %v; want 0, 2 and 3, sheet 0 again after the core did not take it", posted)
	}
	answer := testutil.Receive(t, core.trickplays, "the run's end")
	if done := *answer.Trickplay; answer.JobID != 9 || done != (nodeproto.TrickplayResult{}) {
		t.Errorf("the run ended %+v; want job 9 done", answer)
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if len(fake.asked) != 1 || !slices.Equal(fake.asked[0].SheetsMade, []int{1}) {
		t.Errorf("asked %+v; want one run told sheet 1 is made", fake.asked)
	}
}

// A sheet the core refuses for what it is ends the run, which the node answers as a refusal: no
// sheet after it is made.
func TestAgent_ASheetTheCoreRefusesEndsTheTrickplayRun(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	core.sheetStatuses = []int{nodeproto.StatusResultRejected}
	root, _ := startAgentWithWorkers(t, socket,
		Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Trickplayer: &fakeTrickplayer{sheets: 3}},
		quickRetry)
	core.commands <- trickplayCommand(t, root.Path(), "film", nil)
	answer := *testutil.Receive(t, core.trickplays, "the run's end").Trickplay
	if answer.Refusal == "" || answer.Failure != "" {
		t.Errorf("the run ended %+v; want it refused, not the file's failure", answer)
	}
	select {
	case sheet := <-core.sheets:
		t.Errorf("posted sheet %d after the core refused one", sheet.Index)
	default:
	}
}

// trickplay.md §5: a sheet whose pictures' height cannot be read from it is FFmpeg's output gone
// wrong, the run's failure, and is not posted: the core could not cut its pictures.
func TestAgent_ASheetWhoseSizeCannotBeReadIsTheRunsFailure(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgentWithWorkers(t, socket,
		Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Trickplayer: &fakeTrickplayer{sheets: 9}},
		quickRetry)
	core.commands <- trickplayCommand(t, root.Path(), "film", []int{0, 1, 2, 3, 4, 5, 6})
	answer := *testutil.Receive(t, core.trickplays, "the run's end").Trickplay
	if answer.Failure == "" || answer.Refusal != "" {
		t.Errorf("a sheet of no size ended the run %+v; want the file's failure", answer)
	}
	select {
	case sheet := <-core.sheets:
		t.Errorf("posted sheet %d of no size", sheet.Index)
	default:
	}
}

// D100: a video changed since it was named is refused unmade, and a node with no maker refuses.
func TestAgent_RefusesTheTrickplayOfAChangedVideoOrWithNoMaker(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		what   string
		maker  Trickplayer
		hashed string
		code   nodeproto.RefusalCode
	}{
		{"a changed video", &fakeTrickplayer{sheets: 2}, "what it was", nodeproto.RefusedChanged},
		{"no maker", nil, "film", ""},
	} {
		core, socket := startFakeCore(t)
		workers := Workers{Scanner: fakeScanner{}, Artworker: fakeArtworker{}}
		if check.maker != nil {
			workers.Trickplayer = check.maker
		}
		root, _ := startAgentWithWorkers(t, socket, workers, quickRetry)
		core.commands <- trickplayCommand(t, root.Path(), check.hashed, nil)
		answer := *testutil.Receive(t, core.trickplays, "the run's end").Trickplay
		if answer.Refusal == "" || answer.RefusalCode != check.code {
			t.Errorf("%s ended %+v; want refused with code %q", check.what, answer, check.code)
		}
		select {
		case sheet := <-core.sheets:
			t.Errorf("%s posted sheet %d", check.what, sheet.Index)
		default:
		}
	}
}

// receiveSheet hands the test each trickplay sheet the node posts, past the first sheetStatuses,
// each answered with its status instead.
func (core *fakeCore) receiveSheet(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		var sheet nodeproto.TrickplaySheet
		err := json.NewDecoder(request.Body).Decode(&sheet)
		if isMalformed(err) {
			t.Error(err)
		}
		if err != nil {
			return
		}
		core.mutex.Lock()
		status := 0
		if len(core.sheetStatuses) > 0 {
			status, core.sheetStatuses = core.sheetStatuses[0], core.sheetStatuses[1:]
		}
		core.mutex.Unlock()
		if status != 0 {
			http.Error(writer, "not taken", status)
			return
		}
		if !handOn(core, core.sheets, sheet) {
			return
		}
		_, _ = writer.Write([]byte("{}")) //nolint:errcheck // a node that went away has no use for it
	}
}
