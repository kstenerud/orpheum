// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakePreparer reports half the source done, then a size that names the input it was given, made
// with Quick Sync at 6.5 times real time; a
// rendition directory ending in /slow runs until it is stopped.
type fakePreparer struct{}

func (fakePreparer) Prepare(
	ctx context.Context, input string, command nodeproto.Prepare, progress func(preparedMs int64),
) (prepare.Made, error) {
	progress(500)
	if strings.HasSuffix(command.RenditionDir, "/slow") {
		<-ctx.Done()
		return prepare.Made{}, ctx.Err()
	}
	return prepare.Made{SizeBytes: int64(len(input)), VideoEncoder: "h264_qsv", RealtimeFactor: 6.5}, nil
}

// answerTimeout is how long a test waits for the node's answer before it fails, so a report that
// never comes fails the test in seconds rather than at the suite's deadline.
const answerTimeout = 10 * time.Second

// nextPrepareReport is the core's next report from a prepare.
func nextPrepareReport(t *testing.T, core *fakeCore) nodeproto.PrepareReport {
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), answerTimeout)
	defer stop()
	select {
	case report := <-core.prepareReports:
		return report
	case <-ctx.Done():
		t.Fatal("no prepare report reached the core")
	}
	return nodeproto.PrepareReport{}
}

// finalPrepareReport is the core's last word from a prepare: done or failed, past any waiting or running.
func finalPrepareReport(t *testing.T, core *fakeCore) nodeproto.PrepareReport {
	t.Helper()
	for {
		report := nextPrepareReport(t, core)
		if report.State != nodeproto.PrepareRunning && report.State != nodeproto.PrepareWaiting {
			return report
		}
	}
}

func TestAgent_PreparesOnlyTheFileTheCoreNamed(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := hash.PayloadIdentity(strings.NewReader("a film"), 6)
	if err != nil {
		t.Fatal(err)
	}
	source := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: hex.EncodeToString(identity.Hash[:]),
	}
	send := func(job int64, source nodeproto.BytesFile) {
		core.commands <- nodeproto.Command{CommandSeq: job, Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{
			JobID: job, Source: source, Profile: nodeproto.ProfileRemux, RenditionDir: "3/remux/0123456789abcdef",
		}}
	}
	send(1, source)
	done := finalPrepareReport(t, core)
	wantInput := root.Path() + "/film.mkv"
	if done.JobID != 1 || done.State != nodeproto.PrepareDone || done.SizeBytes != int64(len(wantInput)) ||
		done.VideoEncoder != "h264_qsv" || done.RealtimeFactor != 6.5 {
		t.Errorf("a prepare of the source file ended %+v, want done, run on %s, with the encoder and speed "+
			"the preparer said", done, wantInput)
	}
	changed := source
	changed.PayloadHash = strings.Repeat("0", 64)
	send(2, changed)
	if refused := finalPrepareReport(t, core); refused.State != nodeproto.PrepareFailed ||
		!strings.Contains(refused.Failure, "not the one named") {
		t.Errorf("a prepare of a changed file ended %+v, want refused", refused)
	}
	elsewhere := source
	elsewhere.RootPath = t.TempDir()
	send(3, elsewhere)
	if refused := finalPrepareReport(t, core); refused.State != nodeproto.PrepareFailed ||
		!strings.Contains(refused.Failure, "none of this node's roots") {
		t.Errorf("a prepare under another root ended %+v, want refused", refused)
	}
	if err := os.Mkdir(filepath.Join(root.Path(), "Extras"), 0o700); err != nil {
		t.Fatal(err)
	}
	directory := source
	directory.Path = []byte("Extras")
	send(4, directory)
	if refused := finalPrepareReport(t, core); refused.State != nodeproto.PrepareFailed ||
		!strings.Contains(refused.Failure, "not a regular file") {
		t.Errorf("a prepare of a directory ended %+v, want refused as no regular file", refused)
	}
}

func TestAgent_ACancelStopsARunningPrepareAtOnceAndNothingMoreIsReportedForIt(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := hash.PayloadIdentity(strings.NewReader("a film"), 6)
	if err != nil {
		t.Fatal(err)
	}
	source := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: hex.EncodeToString(identity.Hash[:]),
	}
	commandFor := func(job int64, dir string) nodeproto.Command {
		return nodeproto.Command{CommandSeq: job, Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{
			JobID: job, Source: source, Profile: nodeproto.ProfileRemux, RenditionDir: dir,
		}}
	}
	// Two slow prepares fill both remux slots, so a third runs only once a cancel frees one.
	core.commands <- commandFor(1, "3/remux/slow")
	core.commands <- commandFor(2, "4/remux/slow")
	for range 2 {
		if started := nextPrepareReport(t, core); started.State != nodeproto.PrepareRunning {
			t.Fatalf("a slow prepare's first report = %+v, want running", started)
		}
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 3, Type: nodeproto.CommandCancel,
		Cancel: &nodeproto.Cancel{JobID: 1},
	}
	core.commands <- commandFor(4, "5/remux/0123456789abcdef")
	if next := finalPrepareReport(t, core); next.JobID != 4 || next.State != nodeproto.PrepareDone {
		t.Errorf("the next final report = %+v, want job 4 done and nothing for the cancelled job 1", next)
	}
}

func TestAgent_APrepareWaitingForASlotSaysSoAtOnceAndAtEachTick(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, tick := startAgent(t, socket)
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := hash.PayloadIdentity(strings.NewReader("a film"), 6)
	if err != nil {
		t.Fatal(err)
	}
	source := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: hex.EncodeToString(identity.Hash[:]),
	}
	encode := func(job int64, dir string) nodeproto.Command {
		return nodeproto.Command{CommandSeq: job, Type: nodeproto.CommandPrepare, Prepare: &nodeproto.Prepare{
			JobID: job, Source: source, Profile: nodeproto.ProfileH264At8M, RenditionDir: dir,
		}}
	}
	// One encode slot: the first encode runs, the second waits for it.
	core.commands <- encode(1, "3/h264-8m/slow")
	if first := nextPrepareReport(t, core); first.JobID != 1 || first.State != nodeproto.PrepareRunning {
		t.Fatalf("the first encode's report = %+v, want running", first)
	}
	core.commands <- encode(2, "4/h264-8m/0123456789abcdef")
	if waiting := nextPrepareReport(t, core); waiting.JobID != 2 || waiting.State != nodeproto.PrepareWaiting {
		t.Fatalf("the second encode's report = %+v, want waiting, at once", waiting)
	}
	tick <- time.Time{}
	if again := nextPrepareReport(t, core); again.JobID != 2 || again.State != nodeproto.PrepareWaiting {
		t.Errorf("at the next tick = %+v, want waiting again, so the core keeps waiting", again)
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 3, Type: nodeproto.CommandCancel,
		Cancel: &nodeproto.Cancel{JobID: 1},
	}
	if done := finalPrepareReport(t, core); done.JobID != 2 || done.State != nodeproto.PrepareDone {
		t.Errorf("once the slot is free = %+v, want the second encode done", done)
	}
}
