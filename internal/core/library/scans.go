// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ScanJobs are where a scan's jobs are queued: jobs.Queue.
type ScanJobs interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
}

// Assigner gives a job's work to a node: nodes.Exchanges.
type Assigner interface {
	Assign(ctx context.Context, nodeID int64, job jobs.Job, command nodeproto.Command) error
}

// ScanSettings are the settings a scan reads: the library's rules and the size of a batch.
type ScanSettings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
	String(ctx context.Context, key settings.Key) (string, error)
}

// ScansParts are what the scans are built from.
type ScansParts struct {
	Cache    *store.Cache
	Ingester *Ingester
	Jobs     ScanJobs
	Assigner Assigner
	Settings ScanSettings
	Clock    clock.Clock
	Logger   *slog.Logger
}

// Scans run the scans of nodes' roots (D240): a scan job lists a root and compares the listing with
// what is stored, and an examine job reads a batch of the files it found new or changed; the scan
// completes once the last batch is read. A scan in progress is a row in cache.db, which a second
// request for its root marks to run once more after it (D236 §3).
type Scans struct {
	cache    *store.Cache
	ingester *Ingester
	jobs     ScanJobs
	assigner Assigner
	settings ScanSettings
	clock    clock.Clock
	logger   *slog.Logger

	mutex sync.Mutex
	// completions are each listed scan's end, as its listing gave it, held until its last batch is
	// read. Guarded by mutex.
	completions map[int64]nodeproto.ScanComplete
}

// NewScans returns the scans.
func NewScans(parts ScansParts) *Scans {
	return &Scans{
		cache: parts.Cache, ingester: parts.Ingester, jobs: parts.Jobs, assigner: parts.Assigner,
		settings: parts.Settings, clock: parts.Clock, logger: parts.Logger,
		completions: map[int64]nodeproto.ScanComplete{},
	}
}

// Request asks for a scan of a node's whole root at priority. One asked for while the root's scan
// runs starts none: that one runs once more after it, however many ask (D236 §3).
func (scans *Scans) Request(ctx context.Context, nodeID int64, root string, priority int64) error {
	return scans.RequestUnder(ctx, nodeID, root, wholeRoot, priority)
}

// RequestUnder asks for a scan of what is under scope, a folder relative to a node's root, which
// removes nothing outside it: a watcher's notice that something changed there (D272 §9). One asked
// for while the root's scan runs starts none: one more runs after it, of a scope that covers every
// request made meanwhile.
func (scans *Scans) RequestUnder(ctx context.Context, nodeID int64, root, scope string, priority int64) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	return scans.request(ctx, nodeID, root, cleanScope(scope), priority)
}

// request is RequestUnder, the mutex held.
func (scans *Scans) request(ctx context.Context, nodeID int64, root, scope string, priority int64) error {
	var scanID int64
	err := scans.cache.Write(ctx, func(queries *cachegen.Queries) error {
		id, insertErr := store.OrNotFound(queries.InsertScan(ctx, cachegen.InsertScanParams{
			NodeID: nodeID, RootPath: root, Priority: priority, StartedAt: scans.clock.Now().UnixMilli(),
			ScopePath: scope,
		}))
		if !errors.Is(insertErr, store.ErrNotFound) {
			scanID = id
			return insertErr
		}
		running, getErr := queries.GetScanOfRoot(ctx, cachegen.GetScanOfRootParams{NodeID: nodeID, RootPath: root})
		if getErr != nil {
			return getErr
		}
		again := scope
		if running.IsAnotherScanQueued != 0 {
			again = widerScope(ptr.ValueOrZero(running.AgainScopePath), scope)
		}
		_, againErr := queries.SetScanAgain(ctx, cachegen.SetScanAgainParams{
			NodeID: nodeID, RootPath: root, AgainScopePath: &again,
		})
		return againErr
	})
	if err != nil {
		return fmt.Errorf("ask for a scan of %s on node %d: %w", root, nodeID, err)
	}
	if scanID == 0 {
		return nil
	}
	return scans.queueListing(ctx, scanID, nodeID, root, priority)
}

// ReadNow has a file its listing stored unread read before what else waits, for a person who asked
// to play it (D304 §4): the batch of the scan in progress that holds it is raised to a person's
// priority, as a request merging into a queued job raises it; with no such batch, a scan of the
// file's folder is asked for at that priority.
func (scans *Scans) ReadNow(ctx context.Context, libraryID, nodeID int64, reportedPath []byte) error {
	inProgress, err := scans.cache.Read().ListScans(ctx)
	if err != nil {
		return fmt.Errorf("list the scans in progress: %w", err)
	}
	for _, scan := range inProgress {
		relative, under := RelativeToRoot(reportedPath, scan.RootPath)
		if scan.NodeID != nodeID || !under {
			continue
		}
		fragment, encodeErr := json.Marshal(nodeproto.Path{Text: string(relative)})
		if encodeErr != nil {
			return fmt.Errorf("encode %s: %w", relative, encodeErr)
		}
		// As keepBatches encoded it: {"text":"…" and, for a name that is not UTF-8, its raw bytes after.
		batchID, findErr := store.OrNotFound(scans.cache.Read().GetUndoneScanBatchHolding(ctx,
			cachegen.GetUndoneScanBatchHoldingParams{
				ScanID: scan.ID, PathFragment: strings.TrimSuffix(string(fragment), "}"),
			}))
		if errors.Is(findErr, store.ErrNotFound) {
			continue
		}
		if findErr != nil {
			return fmt.Errorf("find the batch that reads %s: %w", relative, findErr)
		}
		_, err = scans.jobs.Enqueue(ctx, jobs.Request{
			Kind: jobs.KindExamine, Target: jobs.Target{ScanBatchID: batchID}, Priority: jobs.PriorityPersonsRequest,
			Reason:      "reads a file a person asked to play",
			Destination: jobs.NodeDestination(nodeID),
		})
		if err != nil {
			return fmt.Errorf("raise scan batch %d: %w", batchID, err)
		}
		return nil
	}
	roots, err := scans.ingester.database.Read().ListLibraryRootsOnNode(ctx, sqlcgen.ListLibraryRootsOnNodeParams{
		LibraryID: libraryID, NodeID: nodeID,
	})
	if err != nil {
		return fmt.Errorf("list library %d's roots: %w", libraryID, err)
	}
	for _, root := range roots {
		if relative, under := RelativeToRoot(reportedPath, root); under {
			return scans.RequestUnder(ctx, nodeID, root, path.Dir(string(relative)), jobs.PriorityPersonsRequest)
		}
	}
	return nil
}

// MarkRootStalled is the core's answer to work that stalled waiting on a root's disk (D233 §10): the
// root is unavailable from now, so plays of what is under it fail at once and say so (D241), and a
// scan of it is asked for, whose reading it brings it back and the work parked on it (D231 §6). It
// returns since when it is unavailable. A scan already running is asked for again after it ends.
func (scans *Scans) MarkRootStalled(ctx context.Context, nodeID int64, rootPath string) (time.Time, error) {
	since, err := scans.ingester.MarkRootUnavailable(ctx, nodeID, rootPath)
	if err != nil {
		return time.Time{}, err
	}
	scans.logger.WarnContext(ctx, "work stalled on a root's disk; it is unavailable until a scan reads it",
		"nodeId", nodeID, "rootPath", rootPath, "since", since)
	return since, scans.Request(ctx, nodeID, rootPath, jobs.PriorityPersonsRequest)
}

// MarkRootNotNamed makes a library root its node's hello no longer names unavailable from now, unless it
// already was: plays of what is under it fail at once and say so, and nothing is removed on the
// absence; a hello that names it again asks for its scan, which brings it back (D241 §2).
func (scans *Scans) MarkRootNotNamed(ctx context.Context, nodeID int64, rootPath string) error {
	since, err := scans.ingester.MarkRootUnavailable(ctx, nodeID, rootPath)
	if err != nil {
		return err
	}
	scans.logger.WarnContext(ctx, "a node no longer names a library root; it is unavailable until it does",
		"nodeId", nodeID, "rootPath", rootPath, "since", since)
	return nil
}

// queueListing queues a scan's listing. A scan whose job could not be queued is forgotten, so a
// later request starts it again rather than waiting on it.
func (scans *Scans) queueListing(ctx context.Context, scanID, nodeID int64, root string, priority int64) error {
	_, err := scans.jobs.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindScan, Target: jobs.Target{ScanID: scanID}, Priority: priority,
		Reason:      "lists " + root + " to find what is new, changed or gone",
		Destination: jobs.NodeDestination(nodeID),
	})
	if err == nil {
		return nil
	}
	deleteErr := scans.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.DeleteScan(ctx, scanID)
	})
	return errors.Join(fmt.Errorf("queue the listing of %s: %w", root, err), deleteErr)
}

// Resume reconciles the scans in cache.db with a core that has just started. One whose listing was taken
// starts again: what the ingest compared is lost with the core. One not yet listed is queued again,
// which a job still queued for it absorbs.
func (scans *Scans) Resume(ctx context.Context) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	rows, err := scans.cache.Read().ListScans(ctx)
	if err != nil {
		return fmt.Errorf("list the scans in progress: %w", err)
	}
	for _, row := range rows {
		if row.ListedAt == nil {
			if err := scans.queueListing(ctx, row.ID, row.NodeID, row.RootPath, row.Priority); err != nil {
				return err
			}
			continue
		}
		scans.logger.InfoContext(ctx, "a scan the core stopped during starts again", "nodeId", row.NodeID,
			"rootPath", row.RootPath)
		if err := scans.end(ctx, row); err != nil {
			return err
		}
		if err := scans.request(ctx, row.NodeID, row.RootPath, row.ScopePath, row.Priority); err != nil {
			return err
		}
	}
	return nil
}

// ScanRoot names a root by its node.
type ScanRoot struct {
	NodeID   int64
	RootPath string
}

// ScanProgress is how far a root's scan in progress is (D236 §2): listing, or listed and reading
// the files its listing found new or changed; and whether one more runs after it (D236 §3).
type ScanProgress struct {
	IsListed            bool
	ListedCount         int64
	ToExamineCount      int64
	ExaminedCount       int64
	IsAnotherScanQueued bool
}

// RootsOnQuickPass are a node's roots whose scan in progress has not yet listed them, or not yet
// read every file its listing found new or changed: its quick pass, which a background read of whole
// files on the same disk waits for (D255).
func (scans *Scans) RootsOnQuickPass(ctx context.Context, nodeID int64) ([]string, error) {
	progress, err := scans.InProgress(ctx)
	if err != nil {
		return nil, err
	}
	var roots []string
	for root, scan := range progress {
		if root.NodeID == nodeID && (!scan.IsListed || scan.ExaminedCount < scan.ToExamineCount) {
			roots = append(roots, root.RootPath)
		}
	}
	return roots, nil
}

// InProgress is every scan in progress, by its root.
func (scans *Scans) InProgress(ctx context.Context) (map[ScanRoot]ScanProgress, error) {
	rows, err := scans.cache.Read().ListScans(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the scans in progress: %w", err)
	}
	progress := make(map[ScanRoot]ScanProgress, len(rows))
	for _, row := range rows {
		progress[ScanRoot{NodeID: row.NodeID, RootPath: row.RootPath}] = ScanProgress{
			IsListed: row.ListedAt != nil, ListedCount: row.ListedCount, ToExamineCount: row.ToExamineCount,
			ExaminedCount: row.ExaminedCount, IsAnotherScanQueued: row.IsAnotherScanQueued != 0,
		}
	}
	return progress, nil
}

// HandleScan runs one scan job (jobs.KindScan): the root's node lists it. A scan that ended, or whose
// listing is taken, needs nothing more.
func (scans *Scans) HandleScan(ctx context.Context, job jobs.Job) error {
	scan, err := store.OrNotFound(scans.cache.Read().GetScan(ctx, job.ScanID))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read scan %d: %w", job.ScanID, err)
	}
	if scan.ListedAt != nil {
		return nil
	}
	return scans.assigner.Assign(ctx, scan.NodeID, job, nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{RootPath: scan.RootPath, Path: scan.ScopePath},
	}})
}

// HandleExamine runs one examine job (jobs.KindExamine): the root's node reads the batch's files. A
// batch whose scan ended, or that is read, needs nothing more.
func (scans *Scans) HandleExamine(ctx context.Context, job jobs.Job) error {
	opened, isOpen, err := scans.openBatch(ctx, job.ScanBatchID)
	if err != nil || !isOpen {
		return err
	}
	examine := nodeproto.Examine{RootPath: opened.scan.RootPath, Files: opened.files}
	return scans.assigner.Assign(ctx, opened.scan.NodeID, job, nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandExamine, Examine: &examine,
	}})
}

// openedBatch is a batch still to be read, its files and its scan.
type openedBatch struct {
	batch cachegen.ScanBatch
	files []nodeproto.ExamineFile
	scan  cachegen.Scan
}

// openBatch is a batch still to be read, and false for one read or whose scan ended.
func (scans *Scans) openBatch(ctx context.Context, batchID int64) (openedBatch, bool, error) {
	var opened openedBatch
	batch, err := store.OrNotFound(scans.cache.Read().GetScanBatch(ctx, batchID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && batch.IsDone != 0) {
		return opened, false, nil
	}
	if err != nil {
		return opened, false, fmt.Errorf("read scan batch %d: %w", batchID, err)
	}
	opened.batch = batch
	if decodeErr := json.Unmarshal([]byte(batch.FilesJSON), &opened.files); decodeErr != nil {
		return opened, false, fmt.Errorf("read batch %d's files: %w", batch.ID, decodeErr)
	}
	opened.scan, err = store.OrNotFound(scans.cache.Read().GetScan(ctx, batch.ScanID))
	if errors.Is(err, store.ErrNotFound) {
		return opened, false, nil
	}
	if err != nil {
		return opened, false, fmt.Errorf("read scan %d: %w", batch.ScanID, err)
	}
	return opened, true, nil
}

// ScanAssignee is how a scan job given to a node is resolved: its listing is compared, and the
// batches it leaves are queued (TakeListing).
func (scans *Scans) ScanAssignee(runner nodes.Resolver) nodes.Assignee {
	return scanWorkAssignee(runner, listingAnswer,
		func(ctx context.Context, job jobs.Job, records []nodeproto.ScanRecord) error {
			return scans.takeListing(ctx, job.ScanID, records)
		})
}

// listingAnswer is what a node answered a scan's listing with.
func listingAnswer(job jobs.Job, result nodeproto.Result) nodeAnswer {
	listing := ptr.ValueOrZero(result.Listing)
	return nodeAnswer{
		about: fmt.Sprintf("scan %d's listing", job.ScanID), isAnswered: result.Listing != nil,
		refusal: listing.Refusal, stall: listing.Stall, records: listing.Records,
	}
}

// ExamineAssignee is how an examine job given to a node is resolved: what it read is ingested into
// its scan, and the scan completes after its last batch.
func (scans *Scans) ExamineAssignee(runner nodes.Resolver) nodes.Assignee {
	return scanWorkAssignee(runner, examinedAnswer,
		func(ctx context.Context, job jobs.Job, records []nodeproto.ScanRecord) error {
			return scans.takeExamined(ctx, job.ScanBatchID, records)
		})
}

// examinedAnswer is what a node answered a scan batch's examining with.
func examinedAnswer(job jobs.Job, result nodeproto.Result) nodeAnswer {
	examined := ptr.ValueOrZero(result.Examined)
	return nodeAnswer{
		about: fmt.Sprintf("scan batch %d", job.ScanBatchID), isAnswered: result.Examined != nil,
		refusal: examined.Refusal, stall: examined.Stall, records: examined.Records,
	}
}

// scanWorkAssignee is how a scan's job given to a node is resolved: the records the node's answer
// carries, by answerOf, are taken by take (takeRecords); one the node lost is tried again
// (nodes.UnansweredOutcome).
func scanWorkAssignee(
	runner nodes.Resolver, answerOf func(job jobs.Job, result nodeproto.Result) nodeAnswer,
	take func(ctx context.Context, job jobs.Job, records []nodeproto.ScanRecord) error,
) nodes.Assignee {
	return nodes.Assignee{
		Runner: runner,
		Answer: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			answer := answerOf(job, result)
			taking := func() error { return take(ctx, job, answer.records) }
			if err := takeRecords(job, attempt.NodeID, answer, taking); err != nil {
				return fmt.Errorf("node %d: %w", attempt.NodeID, err)
			}
			return nil
		},
		Unanswered: func(_ context.Context, job jobs.Job, _ nodes.Attempt, err error) error {
			return nodes.UnansweredOutcome(job, err, nil)
		},
	}
}

// nodeAnswer is what a node answered a listing or an examining, about, with: whether it answered
// that at all, its refusal or its stall, if either, and its records.
type nodeAnswer struct {
	about      string
	isAnswered bool
	refusal    string
	stall      *nodeproto.Stall
	records    []nodeproto.ScanRecord
}

// takeRecords takes what a node answered about, by take, unless it answered with something else or
// refused, which fails the job, or its reading stalled, which is the tool's failure: tried again,
// and the job's own the second time (nodes.StallOutcome). A stalled reading's records are never
// taken: a listing cut short is not a root's contents.
func takeRecords(job jobs.Job, nodeID int64, answer nodeAnswer, take func() error) error {
	switch {
	case !answer.isAnswered:
		return fmt.Errorf("answered %s with something else", answer.about)
	case answer.refusal != "":
		return jobs.MarkPermanent(fmt.Errorf("refused %s: %s", answer.about, answer.refusal))
	case answer.stall != nil:
		return nodes.StallOutcome(job, nodes.StallError(nodeID, answer.about, *answer.stall), jobs.StalledTwice, nil)
	}
	return take()
}

// takeListing compares a scan's listing and queues the batches of files it needs read; a scan with
// none to read completes at once.
func (scans *Scans) takeListing(ctx context.Context, scanID int64, records []nodeproto.ScanRecord) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scan, err := store.OrNotFound(scans.cache.Read().GetScan(ctx, scanID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && scan.ListedAt != nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read scan %d: %w", scanID, err)
	}
	rules, perBatch, err := scans.readRules(ctx)
	if err != nil {
		return err
	}
	taken, err := scans.ingester.TakeListing(ctx, scan.NodeID, scan.RootPath, scan.ScopePath, records, rules)
	if err != nil {
		return err
	}
	scans.completions[scan.ID] = taken.Complete
	batchIDs, err := scans.keepBatches(ctx, scan, taken, batchesOf(taken.ToExamine, perBatch))
	if err != nil {
		return err
	}
	scans.logger.InfoContext(ctx, "a node listed a root", "nodeId", scan.NodeID, "rootPath", scan.RootPath,
		"listedCount", taken.ListedCount, "toExamineCount", len(taken.ToExamine), "batchCount", len(batchIDs))
	if len(batchIDs) == 0 {
		return scans.complete(ctx, scan)
	}
	for _, batchID := range batchIDs {
		_, err := scans.jobs.Enqueue(ctx, jobs.Request{
			Kind: jobs.KindExamine, Target: jobs.Target{ScanBatchID: batchID}, Priority: scan.Priority,
			Reason:      "reads what is new or changed under " + scan.RootPath,
			Destination: jobs.NodeDestination(scan.NodeID),
		})
		if err != nil {
			return fmt.Errorf("queue scan batch %d: %w", batchID, err)
		}
	}
	return nil
}

// keepBatches records a scan's listing taken, with its batches, and returns their ids.
func (scans *Scans) keepBatches(
	ctx context.Context, scan cachegen.Scan, taken TakenListing, batches [][]nodeproto.ExamineFile,
) ([]int64, error) {
	batchIDs := make([]int64, 0, len(batches))
	err := scans.cache.Write(ctx, func(queries *cachegen.Queries) error {
		err := queries.SetScanListed(ctx, cachegen.SetScanListedParams{
			ListedAt: new(scans.clock.Now().UnixMilli()), ListedCount: int64(taken.ListedCount),
			ToExamineCount: int64(len(taken.ToExamine)), ID: scan.ID,
		})
		if err != nil {
			return err
		}
		for _, files := range batches {
			encoded, err := json.Marshal(files)
			if err != nil {
				return fmt.Errorf("encode a batch's files: %w", err)
			}
			id, err := queries.InsertScanBatch(ctx, cachegen.InsertScanBatchParams{
				ScanID: scan.ID, FilesJSON: string(encoded), FileCount: int64(len(files)),
			})
			if err != nil {
				return err
			}
			batchIDs = append(batchIDs, id)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("record the listing of %s: %w", scan.RootPath, err)
	}
	return batchIDs, nil
}

// takeExamined ingests what a node read of a batch into its scan.
func (scans *Scans) takeExamined(ctx context.Context, batchID int64, records []nodeproto.ScanRecord) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	opened, isOpen, err := scans.openBatch(ctx, batchID)
	if err != nil || !isOpen {
		return err
	}
	if err := scans.ingester.IngestScan(ctx, opened.scan.NodeID, opened.scan.RootPath, records); err != nil {
		return err
	}
	return scans.finishBatch(ctx, opened)
}

// RecordExamineFailure records a batch the node could not read as given up: each of its files is one
// the scan could not read, so none of them is removed (D40), and the scan goes on to complete.
func (scans *Scans) RecordExamineFailure(ctx context.Context, job jobs.Job, failure string) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	opened, isOpen, err := scans.openBatch(ctx, job.ScanBatchID)
	if err != nil || !isOpen {
		return err
	}
	records := make([]nodeproto.ScanRecord, 0, len(opened.files))
	for _, file := range opened.files {
		records = append(records, nodeproto.ScanRecord{Type: nodeproto.RecordError, Error: &nodeproto.ScanError{
			Path: file.Path, Scope: nodeproto.ScopeEntry, Code: nodeproto.CodeIOError, OccurrenceCount: 1,
			PlatformError: nodeproto.PlatformError{
				Text: "the node did not read it: " + failure, OwnerUID: -1, GroupGID: -1,
			},
		}})
	}
	if err := scans.ingester.IngestScan(ctx, opened.scan.NodeID, opened.scan.RootPath, records); err != nil {
		return err
	}
	return scans.finishBatch(ctx, opened)
}

// RecordScanFailure gives up a scan whose listing failed: nothing it did not see is removed.
func (scans *Scans) RecordScanFailure(ctx context.Context, job jobs.Job, failure string) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scan, err := store.OrNotFound(scans.cache.Read().GetScan(ctx, job.ScanID))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read scan %d: %w", job.ScanID, err)
	}
	scans.logger.WarnContext(ctx, "a scan was given up: its root could not be listed", "nodeId", scan.NodeID,
		"rootPath", scan.RootPath, "failure", failure)
	return scans.endThenAgain(ctx, scan)
}

// finishBatch counts a batch read, and completes its scan after its last.
func (scans *Scans) finishBatch(ctx context.Context, opened openedBatch) error {
	batch, scan := opened.batch, opened.scan
	var undone int64
	err := scans.cache.Write(ctx, func(queries *cachegen.Queries) error {
		finished, err := queries.SetScanBatchDone(ctx, batch.ID)
		if err != nil || finished == 0 {
			return err
		}
		err = queries.AddScanExamined(ctx, cachegen.AddScanExaminedParams{FileCount: batch.FileCount, ID: scan.ID})
		if err != nil {
			return err
		}
		undone, err = queries.CountUndoneScanBatches(ctx, scan.ID)
		return err
	})
	if err != nil {
		return fmt.Errorf("count batch %d read: %w", batch.ID, err)
	}
	if undone > 0 {
		return nil
	}
	return scans.complete(ctx, scan)
}

// complete ends a scan whose files are all read with its listing's end, applying its removals (D39)
// and handing its library on to identification, then starts the one asked for meanwhile, if one was.
func (scans *Scans) complete(ctx context.Context, scan cachegen.Scan) error {
	complete, isHeld := scans.completions[scan.ID]
	if !isHeld {
		complete = nodeproto.ScanComplete{RootPath: scan.RootPath, IsStoppedByErrors: true}
	}
	err := scans.ingester.IngestScan(ctx, scan.NodeID, scan.RootPath, []nodeproto.ScanRecord{{
		Type: nodeproto.RecordScanComplete, ScanComplete: &complete,
	}})
	if errors.Is(err, errScopeNeedsTheRoot) {
		scans.logger.InfoContext(ctx, "a scan of a folder found what only a scan of its whole root may judge; "+
			"the root is scanned", "nodeId", scan.NodeID, "rootPath", scan.RootPath, "scopePath", scan.ScopePath,
			"why", err.Error())
		if endErr := scans.end(ctx, scan); endErr != nil {
			return endErr
		}
		return scans.request(ctx, scan.NodeID, scan.RootPath, wholeRoot, scan.Priority)
	}
	if err != nil {
		return err
	}
	if counted, readErr := scans.cache.Read().GetScan(ctx, scan.ID); readErr == nil {
		scan = counted
	}
	scans.logger.InfoContext(ctx, "a node finished a scan", "nodeId", scan.NodeID, "rootPath", scan.RootPath,
		"listedCount", scan.ListedCount, "examinedCount", scan.ExaminedCount, "errorCount", complete.ErrorCount,
		"isStoppedByErrors", complete.IsStoppedByErrors)
	return scans.endThenAgain(ctx, scan)
}

// endThenAgain ends a scan, then starts the one asked for while it ran.
func (scans *Scans) endThenAgain(ctx context.Context, scan cachegen.Scan) error {
	if err := scans.end(ctx, scan); err != nil {
		return err
	}
	if scan.IsAnotherScanQueued == 0 {
		return nil
	}
	again := wholeRoot
	if scan.AgainScopePath != nil {
		again = *scan.AgainScopePath
	}
	return scans.request(ctx, scan.NodeID, scan.RootPath, again, scan.Priority)
}

// end forgets a scan, its batches and what the ingest held of it.
func (scans *Scans) end(ctx context.Context, scan cachegen.Scan) error {
	delete(scans.completions, scan.ID)
	scans.ingester.AbandonScan(scan.NodeID, scan.RootPath)
	err := scans.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.DeleteScan(ctx, scan.ID)
	})
	if err != nil {
		return fmt.Errorf("end the scan of %s: %w", scan.RootPath, err)
	}
	return nil
}

// readRules reads the library's rules for what a scan reads, and how many videos a batch holds.
func (scans *Scans) readRules(ctx context.Context) (ScanRules, int, error) {
	videos, err := scans.settings.String(ctx, settings.ScanVideoExtensions)
	if err != nil {
		return ScanRules{}, 0, fmt.Errorf("read scan_video_extensions: %w", err)
	}
	subtitles, err := scans.settings.String(ctx, settings.ScanSubtitleExtensions)
	if err != nil {
		return ScanRules{}, 0, fmt.Errorf("read scan_subtitle_extensions: %w", err)
	}
	perBatch, err := scans.settings.Int(ctx, settings.ScanVideosPerExamineBatch)
	if err != nil {
		return ScanRules{}, 0, fmt.Errorf("read scan_videos_per_examine_batch: %w", err)
	}
	rules := ScanRules{VideoExtensions: extensionSet(videos), SubtitleExtensions: extensionSet(subtitles)}
	return rules, int(perBatch), nil
}

// extensionSet is a setting's comma-separated extensions as a set.
func extensionSet(listed string) map[string]bool {
	set := map[string]bool{}
	for extension := range strings.SplitSeq(listed, ",") {
		set[extension] = true
	}
	return set
}
