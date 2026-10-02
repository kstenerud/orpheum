// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakePreparer reports half the source done, then a size that names the input it was given, made
// with Quick Sync at 6.5 times real time; a rendition directory ending in /slow runs until it is
// stopped, one ending in /full finds the disk with no room, one ending in /broken fails as
// FFmpeg would, and one ending in /stalled is stopped by the watchdog.
type fakePreparer struct{}

func (fakePreparer) Prepare(
	ctx context.Context, input string, command nodeproto.Prepare, progress func(preparedThroughMs int64),
) (prepare.Made, error) {
	progress(500)
	if strings.HasSuffix(command.RenditionDir, "/slow") {
		<-ctx.Done()
		return prepare.Made{}, ctx.Err()
	}
	if strings.HasSuffix(command.RenditionDir, "/full") {
		return prepare.Made{}, fmt.Errorf("prepare %s: %w", command.RenditionDir, prepared.ErrNoRoom)
	}
	if strings.HasSuffix(command.RenditionDir, "/stalled") {
		return prepare.Made{}, fmt.Errorf("prepare %s: %w", command.RenditionDir, errStalled)
	}
	if strings.HasSuffix(command.RenditionDir, "/broken") {
		return prepare.Made{}, errors.New("ffmpeg exited 1: Invalid data found when processing input")
	}
	return prepare.Made{SizeBytes: int64(len(input)), VideoEncoder: "h264_qsv", RealtimeFactor: 6.5}, nil
}

// answerTimeout is how long a test waits for the node's answer before it fails, so a report that
// never comes fails the test in seconds rather than at the suite's deadline.
const answerTimeout = 10 * time.Second

// jobPrepareReport is a prepare report with the job id and attempt token its result carried.
type jobPrepareReport struct {
	JobID        int64
	AttemptToken string
	nodeproto.PrepareReport
}

// finalPrepareReport is the core's next word on how a prepare ended.
func finalPrepareReport(t *testing.T, core *fakeCore) jobPrepareReport {
	t.Helper()
	result := testutil.Receive(t, core.prepareReports, "how a prepare ended")
	return jobPrepareReport{JobID: result.JobID, AttemptToken: result.AttemptToken, PrepareReport: *result.Prepare}
}

// nextProgress is the core's next word on where an attempt is.
func nextProgress(t *testing.T, core *fakeCore) nodeproto.Progress {
	t.Helper()
	return testutil.Receive(t, core.progress, "a job's progress")
}

// preparableFilm writes film.mkv under root and returns it named by root, path and payload hash.
func preparableFilm(t *testing.T, root *confine.MediaRoot) nodeproto.BytesFile {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	return nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
	}
}

// sendPrepare commands a remux of source into renditionDir as job.
func (core *fakeCore) sendPrepare(job int64, source nodeproto.BytesFile, renditionDir string) {
	core.commands <- nodeproto.Command{
		CommandSeq: job, Type: nodeproto.CommandPrepare, JobID: job, Prepare: &nodeproto.Prepare{
			Video: source, Profile: nodeproto.ProfileRemux, RenditionDir: renditionDir,
		},
	}
}

func TestAgent_PreparesOnlyTheFileTheCoreNamed(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	source := preparableFilm(t, root)
	send := func(job int64, source nodeproto.BytesFile) { core.sendPrepare(job, source, "3/remux/0123456789abcdef") }
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
	if refused := finalPrepareReport(t, core); refused.State != nodeproto.PrepareRefused ||
		!strings.Contains(refused.Refusal, "not the one named") || refused.Failure != "" {
		t.Errorf("a prepare of a changed file ended %+v, want refused", refused)
	}
	elsewhere := source
	elsewhere.RootPath = t.TempDir()
	send(3, elsewhere)
	if refused := finalPrepareReport(t, core); refused.State != nodeproto.PrepareRefused ||
		!strings.Contains(refused.Refusal, "none of this node's roots") {
		t.Errorf("a prepare under another root ended %+v, want refused", refused)
	}
	if err := os.Mkdir(filepath.Join(root.Path(), "Extras"), 0o700); err != nil {
		t.Fatal(err)
	}
	directory := source
	directory.Path = []byte("Extras")
	send(4, directory)
	if refused := finalPrepareReport(t, core); refused.State != nodeproto.PrepareRefused ||
		!strings.Contains(refused.Refusal, "not a regular file") {
		t.Errorf("a prepare of a directory ended %+v, want refused as no regular file", refused)
	}
}

func TestAgent_APrepareTheDiskHasNoRoomForIsRefusedAndOneFFmpegFailsOnFails(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	source := preparableFilm(t, root)
	core.sendPrepare(1, source, "3/remux/full")
	if refused := finalPrepareReport(t, core); refused.State != nodeproto.PrepareRefused ||
		refused.RefusalCode != nodeproto.RefusedNoRoom || !strings.Contains(refused.Refusal, "past its limit") ||
		refused.Failure != "" {
		t.Errorf("a prepare the disk has no room for ended %+v, want refused for want of room, not failed", refused)
	}
	core.sendPrepare(2, source, "3/remux/broken")
	if failed := finalPrepareReport(t, core); failed.State != nodeproto.PrepareFailed ||
		!strings.Contains(failed.Failure, "Invalid data") || failed.Refusal != "" {
		t.Errorf("a prepare FFmpeg failed on ended %+v, want failed with FFmpeg's words", failed)
	}
}

// evictingShelf tells evictions of each eviction.
type evictingShelf struct{ evictions chan struct{} }

func (shelf evictingShelf) Evict() (prepared.Usage, error) {
	shelf.evictions <- struct{}{}
	return prepared.Usage{}, nil
}

func (evictingShelf) LastUsage() (prepared.Usage, bool) { return prepared.Usage{}, false }

// A prepare adds to the prepared directory, so once it ends the directory is brought back within its
// budget then, not only when the next prepare makes room (D217 §5).
func TestAgent_AFinishedPrepareKeepsThePreparedDirectoryWithinItsBudget(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	shelf := evictingShelf{evictions: make(chan struct{}, 4)}
	root, _ := startAgentWithShelf(t, socket, shelf)
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 1, Type: nodeproto.CommandPrepare, JobID: 1, Prepare: &nodeproto.Prepare{
			Profile: nodeproto.ProfileRemux, RenditionDir: "3/remux/0123456789abcdef",
			Video: nodeproto.BytesFile{
				RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
			},
		},
	}
	if done := finalPrepareReport(t, core); done.State != nodeproto.PrepareDone {
		t.Fatalf("the prepare ended %+v, want done", done)
	}
	select {
	case <-shelf.evictions:
	case <-time.After(answerTimeout):
		t.Error("the prepared directory was not kept within its budget after the prepare")
	}
}

func TestAgent_ACancelStopsARunningPrepareAtOnceAndNothingMoreIsReportedForIt(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
	}
	commandFor := func(job int64, dir string) nodeproto.Command {
		payload := &nodeproto.Prepare{Video: source, Profile: nodeproto.ProfileRemux, RenditionDir: dir}
		return nodeproto.Command{CommandSeq: job, Type: nodeproto.CommandPrepare, JobID: job, Prepare: payload}
	}
	// Two slow prepares fill both remux slots, so a third runs only once a cancel frees one.
	core.commands <- commandFor(1, "3/remux/slow")
	core.commands <- commandFor(2, "4/remux/slow")
	for range 2 {
		if started := nextProgress(t, core); started.State != nodeproto.ProgressRunning {
			t.Fatalf("a slow prepare's first report = %+v, want running", started)
		}
	}
	core.commands <- nodeproto.Command{CommandSeq: 3, Type: nodeproto.CommandCancel, JobID: 1}
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
	source := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
	}
	encode := func(job int64, dir string) nodeproto.Command {
		payload := &nodeproto.Prepare{Video: source, Profile: nodeproto.ProfileH264At8M, RenditionDir: dir}
		return nodeproto.Command{CommandSeq: job, Type: nodeproto.CommandPrepare, JobID: job, Prepare: payload}
	}
	// One encode slot: the first encode runs, the second waits for it.
	core.commands <- encode(1, "3/h264-8m/slow")
	if first := nextProgress(t, core); first.JobID != 1 || first.State != nodeproto.ProgressRunning {
		t.Fatalf("the first encode's report = %+v, want running", first)
	}
	core.commands <- encode(2, "4/h264-8m/0123456789abcdef")
	if waiting := nextProgress(t, core); waiting.JobID != 2 || waiting.State != nodeproto.ProgressWaiting {
		t.Fatalf("the second encode's report = %+v, want waiting, at once", waiting)
	}
	tick <- time.Time{}
	if again := nextProgress(t, core); again.JobID != 2 || again.State != nodeproto.ProgressWaiting {
		t.Errorf("at the next tick = %+v, want waiting again, so the core keeps waiting", again)
	}
	core.commands <- nodeproto.Command{CommandSeq: 3, Type: nodeproto.CommandCancel, JobID: 1}
	if done := finalPrepareReport(t, core); done.JobID != 2 || done.State != nodeproto.PrepareDone {
		t.Errorf("once the slot is free = %+v, want the second encode done", done)
	}
}

// A prepare's end the core did not take is posted again, not dropped: without it the core would
// wait, then prepare the same rendition again from nothing.
func TestAgent_APreparesEndTheCoreRefusedArrivesOnRetry(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	// The first post is the prepare's progress, which is not tried again; the second is its end.
	core.refuseFirstPosts(prepareResult, 2)
	core.sendPrepare(1, preparableFilm(t, root), "3/remux/0123456789abcdef")
	if done := finalPrepareReport(t, core); done.JobID != 1 || done.State != nodeproto.PrepareDone {
		t.Errorf("the prepare's end after a refusal = %+v, want done", done)
	}
}

// blockingScanner begins a scan, says so, and holds it until the session ends.
type blockingScanner struct{ scanning chan struct{} }

func (scanner blockingScanner) Scan(ctx context.Context, root *confine.MediaRoot, emit scan.Emit) error {
	begin := nodeproto.Record{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{RootPath: root.Path()}}
	if err := emit(begin); err != nil {
		return err
	}
	scanner.scanning <- struct{}{}
	<-ctx.Done()
	return ctx.Err()
}

// A prepare is held beside a scan, not queued behind it: on devnas a prepare a person asked for
// waited for a 55-second scan to end (the jobs rebuild's 4d).
func TestAgent_APrepareRunsWhileAScanIsStillGoing(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	scanner := blockingScanner{scanning: make(chan struct{}, 1)}
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: scanner, Artworker: fakeArtworker{}, Preparer: fakePreparer{},
	})
	core.commands <- nodeproto.Command{
		CommandSeq: 1, Type: nodeproto.CommandScan, Scan: &nodeproto.Scan{RootPath: root.Path()},
	}
	testutil.Receive(t, scanner.scanning, "the scan beginning")
	core.sendPrepare(2, preparableFilm(t, root), "3/remux/0123456789abcdef")
	if final := finalPrepareReport(t, core); final.JobID != 2 || final.State != nodeproto.PrepareDone {
		t.Errorf("while the scan went on, the prepare ended %+v, want job 2 done", final)
	}
}

// The node holds work by its attempt: two attempts at one job are two, a cancel stops only the
// attempt it names, and every report names the attempt it is about (D238 §2). A command with no
// attempt is refused.
func TestAgent_HoldsWorkByItsAttemptAndEveryReportNamesIt(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	source := preparableFilm(t, root)
	attemptAt := func(seq, job int64, token, dir string) nodeproto.Command {
		payload := &nodeproto.Prepare{Video: source, Profile: nodeproto.ProfileRemux, RenditionDir: dir}
		return nodeproto.Command{
			CommandSeq: seq, Type: nodeproto.CommandPrepare, JobID: job, AttemptToken: token, Prepare: payload,
		}
	}
	// Two slow attempts at job 1 fill both remux slots.
	core.commands <- attemptAt(1, 1, "old", "3/remux/slow")
	core.commands <- attemptAt(2, 1, "new", "3/remux/slow")
	started := map[string]bool{}
	for range 2 {
		report := nextProgress(t, core)
		if report.JobID != 1 || report.State != nodeproto.ProgressRunning {
			t.Fatalf("a slow attempt's first report = %+v, want job 1 running", report)
		}
		started[report.AttemptToken] = true
	}
	if !started["old"] || !started["new"] {
		t.Fatalf("the running reports named attempts %v, want old and new, each its own", started)
	}
	core.commands <- nodeproto.Command{CommandSeq: 3, Type: nodeproto.CommandCancel, JobID: 1, AttemptToken: "old"}
	core.commands <- attemptAt(4, 4, "only", "5/remux/0123456789abcdef")
	if next := finalPrepareReport(t, core); next.JobID != 4 || next.AttemptToken != "only" ||
		next.State != nodeproto.PrepareDone {
		t.Errorf("the next final report = %+v, want job 4's attempt only done, in the slot attempt old freed", next)
	}
	core.sendsTokenless(5)
	core.commands <- attemptAt(5, 5, "", "6/remux/0123456789abcdef")
	if refused := testutil.Receive(t, core.refusals, "the refusal of a command naming no attempt"); refused.JobID != 5 {
		t.Errorf("the refusal named job %d, want job 5, whose command named no attempt", refused.JobID)
	}
}
