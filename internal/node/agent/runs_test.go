// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The work these tests hold is a run of chunks, the one kind of work the node makes in its prepared
// directory since the whole-film prepare was retired (D344); fakeChunkMaker makes it.

// answerTimeout is how long a test waits for the node's answer before it fails, so a report that
// never comes fails the test in seconds rather than at the suite's deadline.
const answerTimeout = 10 * time.Second

// jobRunReport is a run's report with the job id and attempt token its result carried.
type jobRunReport struct {
	JobID        int64
	AttemptToken string
	nodeproto.MakeChunksReport
}

// finalRunReport is the core's next word on how a run of chunks ended.
func finalRunReport(t *testing.T, core *fakeCore) jobRunReport {
	t.Helper()
	result := testutil.Receive(t, core.chunkRuns, "how a run of chunks ended")
	return jobRunReport{JobID: result.JobID, AttemptToken: result.AttemptToken, MakeChunksReport: *result.MakeChunks}
}

// nextProgress is the core's next word on where an attempt is.
func nextProgress(t *testing.T, core *fakeCore) nodeproto.Progress {
	t.Helper()
	return testutil.Receive(t, core.progress, "a job's progress")
}

// chunkableFilm writes film.mkv under root and returns it named by root, path and payload hash.
func chunkableFilm(t *testing.T, root *confine.MediaRoot) nodeproto.BytesFile {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	return nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
	}
}

// runOf is a run of two chunks of source's stream of kind into storeDir: a quality's holds an encode
// slot, a copied picture's or a sound's a remux slot.
func runOf(source nodeproto.BytesFile, storeDir string, kind nodeproto.ChunkStreamKind) nodeproto.MakeChunks {
	return nodeproto.MakeChunks{
		Video: source, StoreDir: storeDir, Stream: nodeproto.ChunkStream{Kind: kind}, BoundsMs: []int64{0, 2000, 4000},
	}
}

// sendRun commands a run of source's copied picture into storeDir as job, in a remux slot.
func (core *fakeCore) sendRun(job int64, source nodeproto.BytesFile, storeDir string) {
	core.sendMakeChunks(job, runOf(source, storeDir, nodeproto.StreamPicture))
}

// A run the disk has no room for is refused for want of room, not failed, and one FFmpeg fails on
// fails with FFmpeg's words.
func TestAgent_ARunTheDiskHasNoRoomForIsRefusedAndOneFFmpegFailsOnFails(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	source := chunkableFilm(t, root)
	core.sendRun(1, source, "3/chunks-full/0123456789abcdef")
	if refused := finalRunReport(t, core); refused.State != nodeproto.RunRefused ||
		refused.RefusalCode != nodeproto.RefusedNoRoom || !strings.Contains(refused.Refusal, "past its limit") ||
		refused.Failure != "" {
		t.Errorf("a run the disk has no room for ended %+v, want refused for want of room, not failed", refused)
	}
	core.sendRun(2, source, "3/chunks-failing/0123456789abcdef")
	if failed := finalRunReport(t, core); failed.State != nodeproto.RunFailed ||
		!strings.Contains(failed.Failure, "Invalid data") || failed.Refusal != "" {
		t.Errorf("a run FFmpeg failed on ended %+v, want failed with FFmpeg's words", failed)
	}
}

// A run of a file under no root, or of a directory, is refused before anything runs.
func TestAgent_ARunOfAFileUnderNoRootOrOfADirectoryIsRefused(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	source := chunkableFilm(t, root)
	elsewhere := source
	elsewhere.RootPath = t.TempDir()
	core.sendRun(1, elsewhere, "3/chunks-picture/0123456789abcdef")
	if refused := finalRunReport(t, core); refused.State != nodeproto.RunRefused ||
		!strings.Contains(refused.Refusal, "none of this node's roots") {
		t.Errorf("a run under another root ended %+v, want refused", refused)
	}
	if err := os.Mkdir(filepath.Join(root.Path(), "Extras"), 0o700); err != nil {
		t.Fatal(err)
	}
	directory := source
	directory.Path = []byte("Extras")
	core.sendRun(2, directory, "3/chunks-picture/0123456789abcdef")
	if refused := finalRunReport(t, core); refused.State != nodeproto.RunRefused ||
		!strings.Contains(refused.Refusal, "not a regular file") {
		t.Errorf("a run of a directory ended %+v, want refused as no regular file", refused)
	}
}

// evictingShelf tells evictions of each eviction.
type evictingShelf struct{ evictions chan struct{} }

func (shelf evictingShelf) Evict(context.Context) (prepared.Usage, error) {
	shelf.evictions <- struct{}{}
	return prepared.Usage{}, nil
}

func (evictingShelf) LastUsage() (prepared.Usage, bool) { return prepared.Usage{}, false }

func (evictingShelf) EvictUnfinished() ([]string, error) { return nil, nil }

func (evictingShelf) SetStoreTiers(map[string]nodeproto.KeepTier) {}

func (evictingShelf) CheckStoresOf(_ context.Context, fileID int64) nodeproto.ChunksChecked {
	return nodeproto.ChunksChecked{FileID: fileID}
}

// A run adds to the prepared directory, so once it ends the directory is brought back within its
// budget then, not only when the next run makes room (D217 §5).
func TestAgent_AFinishedRunKeepsThePreparedDirectoryWithinItsBudget(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	shelf := evictingShelf{evictions: make(chan struct{}, 4)}
	root, _ := startAgentWithShelf(t, socket, shelf)
	core.sendRun(1, chunkableFilm(t, root), "3/chunks-picture/0123456789abcdef")
	if done := finalRunReport(t, core); done.State != nodeproto.RunDone {
		t.Fatalf("the run ended %+v, want done", done)
	}
	select {
	case <-shelf.evictions:
	case <-time.After(answerTimeout):
		t.Error("the prepared directory was not kept within its budget after the run")
	}
}

func TestAgent_ACancelStopsARunningRunAtOnceAndNothingMoreIsReportedForIt(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	source := chunkableFilm(t, root)
	// Two slow runs fill both remux slots, so a third runs only once a cancel frees one.
	core.sendRun(1, source, "3/chunks-slow/0123456789abcdef")
	core.sendRun(2, source, "4/chunks-slow/0123456789abcdef")
	for range 2 {
		if started := nextProgress(t, core); started.State != nodeproto.ProgressRunning {
			t.Fatalf("a slow run's first report = %+v, want running", started)
		}
	}
	core.commands <- nodeproto.Command{CommandSeq: 3, JobID: 1, Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
	core.sendRun(4, source, "5/chunks-picture/0123456789abcdef")
	if next := finalRunReport(t, core); next.JobID != 4 || next.State != nodeproto.RunDone {
		t.Errorf("the next final report = %+v, want job 4 done and nothing for the cancelled job 1", next)
	}
}

func TestAgent_ARunWaitingForASlotSaysSoAtOnceAndAtEachTick(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, tick := startAgent(t, socket)
	source := chunkableFilm(t, root)
	encode := func(job int64, dir string) {
		core.sendMakeChunks(job, runOf(source, dir, nodeproto.StreamQuality))
	}
	// One encode slot: the first encode runs, the second waits for it.
	encode(1, "3/chunks-slow/0123456789abcdef")
	if first := nextProgress(t, core); first.JobID != 1 || first.State != nodeproto.ProgressRunning {
		t.Fatalf("the first encode's report = %+v, want running", first)
	}
	encode(2, "4/chunks-q400k360p/0123456789abcdef")
	if waiting := nextProgress(t, core); waiting.JobID != 2 || waiting.State != nodeproto.ProgressWaiting {
		t.Fatalf("the second encode's report = %+v, want waiting, at once", waiting)
	}
	tick <- time.Time{}
	if again := nextProgress(t, core); again.JobID != 2 || again.State != nodeproto.ProgressWaiting {
		t.Errorf("at the next tick = %+v, want waiting again, so the core keeps waiting", again)
	}
	core.commands <- nodeproto.Command{CommandSeq: 3, JobID: 1, Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
	if done := finalRunReport(t, core); done.JobID != 2 || done.State != nodeproto.RunDone {
		t.Errorf("once the slot is free = %+v, want the second encode done", done)
	}
}

// A run's end the core did not take is posted again, not dropped: without it the core would wait,
// then ask for the same chunks again.
func TestAgent_ARunsEndTheCoreRefusedArrivesOnRetry(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	core.refuseFirstPosts(chunksResult, 1)
	core.sendRun(1, chunkableFilm(t, root), "3/chunks-picture/0123456789abcdef")
	if done := finalRunReport(t, core); done.JobID != 1 || done.State != nodeproto.RunDone {
		t.Errorf("the run's end after a refusal = %+v, want done", done)
	}
}

// blockingScanner begins an examining, says so, and holds it until its work is stopped, going between
// files as it is told.
type blockingScanner struct {
	fakeScanner
	examining chan struct{}
}

func (scanner blockingScanner) Examine(
	ctx context.Context, _ *confine.MediaRoot, _ []nodeproto.ExamineFile, between func() error,
) ([]nodeproto.ScanRecord, error) {
	scanner.examining <- struct{}{}
	for ctx.Err() == nil {
		if err := between(); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Millisecond):
		}
	}
	return nil, ctx.Err()
}

// A run is held beside a scan's examining, not queued behind it: on devnas the work a person asked
// for waited for a 55-second scan to end (the jobs rebuild's 4d).
func TestAgent_ARunRunsWhileAnExaminingIsStillGoing(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	scanner := blockingScanner{examining: make(chan struct{}, 1)}
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: scanner, Artworker: fakeArtworker{}, ChunkMaker: fakeChunkMaker{},
	})
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 1, Work: nodeproto.Work{
		Type: nodeproto.CommandExamine, Examine: &nodeproto.Examine{
			RootPath: root.Path(), Files: []nodeproto.ExamineFile{{Path: nodeproto.Path{Text: "a.mkv"}}},
		},
	}}
	testutil.Receive(t, scanner.examining, "the examining beginning")
	core.sendRun(2, chunkableFilm(t, root), "3/chunks-picture/0123456789abcdef")
	if final := finalRunReport(t, core); final.JobID != 2 || final.State != nodeproto.RunDone {
		t.Errorf("while the examining went on, the run ended %+v, want job 2 done", final)
	}
}

// The node holds work by its attempt: two attempts at one job are two, a cancel stops only the
// attempt it names, and every report names the attempt it is about (D238 §2). A command with no
// attempt is refused.
func TestAgent_HoldsWorkByItsAttemptAndEveryReportNamesIt(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	source := chunkableFilm(t, root)
	attemptAt := func(seq, job int64, token, dir string) nodeproto.Command {
		run := runOf(source, dir, nodeproto.StreamPicture)
		return nodeproto.Command{
			CommandSeq:   seq,
			JobID:        job,
			AttemptToken: token,
			Work:         nodeproto.Work{Type: nodeproto.CommandMakeChunks, MakeChunks: &run},
		}
	}
	// Two slow attempts at job 1 fill both remux slots.
	core.commands <- attemptAt(1, 1, "old", "3/chunks-slow/0123456789abcdef")
	core.commands <- attemptAt(2, 1, "new", "3/chunks-slow/0123456789abcdef")
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
	core.commands <- nodeproto.Command{
		CommandSeq:   3,
		JobID:        1,
		AttemptToken: "old",
		Work:         nodeproto.Work{Type: nodeproto.CommandCancel},
	}
	core.commands <- attemptAt(4, 4, "only", "5/chunks-picture/0123456789abcdef")
	if next := finalRunReport(t, core); next.JobID != 4 || next.AttemptToken != "only" ||
		next.State != nodeproto.RunDone {
		t.Errorf("the next final report = %+v, want job 4's attempt only done, in the slot attempt old freed", next)
	}
	core.sendsTokenless(5)
	core.commands <- attemptAt(5, 5, "", "6/chunks-picture/0123456789abcdef")
	if refused := testutil.Receive(t, core.refusals, "the refusal of a command naming no attempt"); refused.JobID != 5 {
		t.Errorf("the refusal named job %d, want job 5, whose command named no attempt", refused.JobID)
	}
}
