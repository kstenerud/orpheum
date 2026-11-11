// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/pkg/strictjson"
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
// completes once the last batch is read. A scan in progress is a row in cache.db, one per node and
// root path: work is per absolute path, done once. A second request for its root joins it while its
// listing is not with the node, and otherwise marks it to run once more after it (D236 §3).
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
	// prober asks a node whether a root can be read now (SetProber); nil probes nothing. Guarded by
	// mutex: it is set once the sweeps may already run.
	prober RootProber
	// onEnded is told each time a scan has ended, so work that waits for a root's scan to be over
	// runs then (OnEnded); nil tells nothing. Set before the scans run.
	onEnded func()
}

// OnEnded sets what is told each time a scan has ended, its root no longer scanning. The pass that
// follows a scan runs inside its completion, while the scan is still in progress, so a rescan waiting
// for its library's scan to be over saw it scanning and, asked again by nothing, waited for the next
// hourly sweep (staging's "Rescan this" of the owner's film, 2026-10-27).
func (scans *Scans) OnEnded(tell func()) { scans.onEnded = tell }

// RequestOnHello asks for the scan a node's hello brings of a root a library covers: in the
// background, and before the queue's own work when the root, or a disk inside it, is unavailable, as
// the scan that brings it back (D240 §1, D328): behind a routine scan of a larger root on the same
// disk, it waited a minute for the disk's one reader (staging, 2026-10-21).
//
// It joins a scan of the root whose listing is not yet taken, even one with the node: the hello
// resolved every attempt given to the node's earlier start (nodes.Exchanges.Interrupt), so a listing
// with it now went to the start that said hello, and lists what is there since. A restart of core and
// node had each root listed twice, the retried listing and then the hello's, 45 s each over a slow
// share (staging, 2026-10-27).
func (scans *Scans) RequestOnHello(ctx context.Context, nodeID int64, root string) error {
	return scans.requestBringingBack(ctx, nodeID, root, true)
}

// RequestOnReturn asks for the scan of a library folder whose disk answers again, at the priority
// RequestOnHello's would have. Unlike a hello's, it joins no listing already with the node, which
// may have gone to it while the disk did not answer.
func (scans *Scans) RequestOnReturn(ctx context.Context, nodeID int64, root string) error {
	return scans.requestBringingBack(ctx, nodeID, root, false)
}

// requestBringingBack is RequestOnHello's ask, isNodesHello, or RequestOnReturn's.
func (scans *Scans) requestBringingBack(ctx context.Context, nodeID int64, root string, isNodesHello bool) error {
	places, err := unavailableOfNode(ctx, scans.ingester.database.Read(), nodeID)
	if err != nil {
		return err
	}
	priority := jobs.PriorityBackground
	if slices.ContainsFunc(places, func(place Unavailable) bool { return isAtOrUnder([]byte(place.Path), root) }) {
		priority = jobs.PriorityBringsBack
	}
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	return scans.request(ctx, scanAsk{
		nodeID: nodeID, root: root, scope: wholeRoot, priority: priority, isNodesHello: isNodesHello,
	})
}

// RequestUnder asks for a scan of what is under scope, a folder relative to a node's root, which
// removes nothing outside it: a watcher's notice that something changed there (D272 §9). Every ask
// for a scan comes here or to RequestOnHello and RequestOnReturn: the schedule, a person's Scan
// button, a library or folder added, a rescan (D357), a watcher's notice, a file a person asked to
// play. A root has one scan in progress, which an ask joins or follows (joinOrAgain).
func (scans *Scans) RequestUnder(ctx context.Context, nodeID int64, root, scope string, priority int64) error {
	return scans.RequestReadingAgain(ctx, nodeID, root, scope, nil, priority)
}

// RequestReadingAgain is RequestUnder's ask for a scan that also reads again what readAgain names,
// changed on disk or not, and compares each probe with the one kept: a full rescan's (D392 §2).
func (scans *Scans) RequestReadingAgain(
	ctx context.Context, nodeID int64, root, scope string, readAgain ReadAgain, priority int64,
) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	return scans.request(ctx, scanAsk{
		nodeID: nodeID, root: root, scope: cleanScope(scope), priority: priority, readAgain: readAgain.cleaned(),
	})
}

// scanAsk is a scan asked for: of what under which node's root, at what priority, whether by the
// node's hello, and what it reads again changed on disk or not.
type scanAsk struct {
	nodeID       int64
	root         string
	scope        string
	priority     int64
	isNodesHello bool
	readAgain    ReadAgain
}

// request is RequestUnder, the mutex held. The folder asked for is scanned within the outermost live
// library folder on its node at or above it, of whichever library, its scan's root: a place two
// libraries' folders cover is scanned once (D363 §1, §2).
func (scans *Scans) request(ctx context.Context, ask scanAsk) error {
	folders, err := scans.ingester.database.Read().ListLiveRootPathsOnNode(ctx, ask.nodeID)
	if err != nil {
		return fmt.Errorf("list node %d's library folders: %w", ask.nodeID, err)
	}
	if root, scope := scanRootOf(ask.root, ask.scope, folders); root != ask.root {
		scans.logger.DebugContext(ctx, "a folder inside another library folder is scanned within it",
			"nodeId", ask.nodeID, "folder", ask.root, "askedScopePath", ask.scope, "rootPath", root, "scopePath", scope)
		ask.readAgain = ask.readAgain.rebasedOn(ask.root, folders)
		ask.root, ask.scope = root, scope
	}
	readAgain, err := ask.readAgain.encoded()
	if err != nil {
		return err
	}
	var scanID int64
	var joined *cachegen.Scan
	err = scans.cache.Write(ctx, func(queries *cachegen.Queries) error {
		id, insertErr := store.OrNotFound(queries.InsertScan(ctx, cachegen.InsertScanParams{
			NodeID: ask.nodeID, RootPath: ask.root, Priority: ask.priority, StartedAt: scans.clock.Now().UnixMilli(),
			ScopePath: ask.scope, ReadAgainJSON: readAgain,
		}))
		if !errors.Is(insertErr, store.ErrNotFound) {
			scanID = id
			return insertErr
		}
		inProgress, getErr := queries.GetScanOfRoot(ctx, cachegen.GetScanOfRootParams{
			NodeID: ask.nodeID, RootPath: ask.root,
		})
		if getErr != nil {
			return getErr
		}
		var joinErr error
		joined, joinErr = joinOrAgain(ctx, queries, inProgress, ask)
		return joinErr
	})
	if err != nil {
		return fmt.Errorf("ask for a scan of %s on node %d: %w", ask.root, ask.nodeID, err)
	}
	if joined != nil {
		return scans.raiseJoined(ctx, *joined, ask)
	}
	if scanID == 0 {
		return nil
	}
	return scans.queueListing(ctx, scanID, ask.nodeID, ask.root, ask.priority)
}

// joinOrAgain takes an ask for a scan of a root whose scan is in progress, and returns that scan as
// the ask joined it, or nil when one more is to run after it (D236 §3). The ask joins it while its
// listing is not with the node, since the node will list what is there when it is: its scope widened
// to cover both, at the higher priority, reading again what either reads again. A node's hello's ask
// also joins a listing with the node, of a scope covering it (RequestOnHello). Otherwise the listing
// may have passed what the ask is about, so one more scan follows, of a scope covering every ask made
// meanwhile, reading again what any of them reads again.
func joinOrAgain(
	ctx context.Context, queries *cachegen.Queries, inProgress cachegen.Scan, ask scanAsk,
) (*cachegen.Scan, error) {
	if inProgress.ListedAt == nil {
		isWithNode := inProgress.ListingSentAt != nil
		covering := widerScope(inProgress.ScopePath, ask.scope)
		if !isWithNode || (ask.isNodesHello && covering == cleanScope(inProgress.ScopePath)) {
			readAgain, err := joinedReadAgain(inProgress.ReadAgainJSON, ask.readAgain)
			if err != nil {
				return nil, err
			}
			joined := inProgress
			joined.ScopePath, joined.Priority = covering, max(inProgress.Priority, ask.priority)
			joined.ReadAgainJSON = readAgain
			err = queries.JoinScan(ctx, cachegen.JoinScanParams{
				ScopePath: joined.ScopePath, Priority: joined.Priority, ReadAgainJSON: readAgain, ID: inProgress.ID,
			})
			return &joined, err
		}
	}
	again, alreadyAgain := ask.scope, (*string)(nil)
	if inProgress.IsAnotherScanQueued != 0 {
		again, alreadyAgain = widerScope(ptr.ValueOrZero(inProgress.AgainScopePath), ask.scope),
			inProgress.AgainReadAgainJSON
	}
	readAgain, err := joinedReadAgain(alreadyAgain, ask.readAgain)
	if err != nil {
		return nil, err
	}
	_, err = queries.SetScanAgain(ctx, cachegen.SetScanAgainParams{
		NodeID: ask.nodeID, RootPath: ask.root, AgainScopePath: &again, AgainReadAgainJSON: readAgain,
	})
	return nil, err
}

// joinedReadAgain is what a scan whose row says stored reads again and what an ask reads again,
// together, as the row keeps it.
func joinedReadAgain(stored *string, asked ReadAgain) (*string, error) {
	kept, err := decodeReadAgain(stored)
	if err != nil {
		return nil, err
	}
	return kept.with(asked).encoded()
}

// raiseJoined raises the listing of a scan an ask joined to the scan's priority, as a request merging
// into its queued job does (jobs.Queue.Enqueue); a listing with the node is not queued again, which
// would hand it out twice. Its batches, not yet queued, take the scan's priority when they are.
func (scans *Scans) raiseJoined(ctx context.Context, scan cachegen.Scan, ask scanAsk) error {
	isWithNode := scan.ListingSentAt != nil
	scans.logger.DebugContext(ctx, "a scan asked for joins the one in progress of its root", "nodeId", scan.NodeID,
		"rootPath", scan.RootPath, "askedScopePath", ask.scope, "scopePath", scan.ScopePath,
		"askedPriority", ask.priority, "priority", scan.Priority, "isNodesHello", ask.isNodesHello,
		"isListingWithNode", isWithNode, "readAgain", ptr.ValueOrZero(scan.ReadAgainJSON))
	if isWithNode {
		return nil
	}
	_, err := scans.jobs.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindScan, Target: jobs.Target{ScanID: scan.ID}, Priority: scan.Priority,
		Reason:      listingReason(scan.RootPath),
		Destination: jobs.NodeDestination(scan.NodeID),
	})
	if err != nil {
		return fmt.Errorf("raise the listing of %s: %w", scan.RootPath, err)
	}
	return nil
}

// listingReason is why a scan's listing job exists, shown to the admin.
func listingReason(root string) string {
	return "lists " + root + " to find what is new, changed or gone"
}

// ReadNow has a file its listing stored unread read before what else waits, for a person who asked
// to play it (D304 §4): the batch of the scan in progress that holds it is raised to a person's
// priority, as a request merging into a queued job raises it; with no such batch, a scan of the
// file's folder is asked for at that priority.
func (scans *Scans) ReadNow(ctx context.Context, nodeID int64, reportedPath []byte) error {
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
	return scans.requestFolderOf(ctx, nodeID, reportedPath, jobs.PriorityPersonsRequest)
}

// requestFolderOf asks for a scan, at priority, of the folder of the file a node reported at
// reportedPath, within the library folder that holds it; none when no live one does.
func (scans *Scans) requestFolderOf(ctx context.Context, nodeID int64, reportedPath []byte, priority int64) error {
	roots, err := scans.ingester.database.Read().ListLiveRootPathsOnNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("list node %d's library folders: %w", nodeID, err)
	}
	for _, root := range roots {
		if relative, under := RelativeToRoot(reportedPath, root); under {
			return scans.RequestUnder(ctx, nodeID, root, path.Dir(string(relative)), priority)
		}
	}
	return nil
}

// queueListing queues a scan's listing. A scan whose job could not be queued is forgotten, so a
// later request starts it again rather than waiting on it.
func (scans *Scans) queueListing(ctx context.Context, scanID, nodeID int64, root string, priority int64) error {
	_, err := scans.jobs.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindScan, Target: jobs.Target{ScanID: scanID}, Priority: priority,
		Reason:      listingReason(root),
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
		readAgain, err := decodeReadAgain(row.ReadAgainJSON)
		if err != nil {
			return err
		}
		err = scans.request(ctx, scanAsk{
			nodeID: row.NodeID, root: row.RootPath, scope: row.ScopePath, priority: row.Priority, readAgain: readAgain,
		})
		if err != nil {
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
	errorLimit, err := scans.settings.Int(ctx, settings.ScanRootErrorLimit)
	if err != nil {
		return fmt.Errorf("read %s: %w", settings.ScanRootErrorLimit, err)
	}
	// Marked with the node in the write that reads what it lists: an ask after it no longer joins it.
	var scan cachegen.Scan
	err = scans.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var sendErr error
		scan, sendErr = store.OrNotFound(queries.SendScanListing(ctx, cachegen.SendScanListingParams{
			ListingSentAt: new(scans.clock.Now().UnixMilli()), ID: job.ScanID,
		}))
		return sendErr
	})
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("hand scan %d's listing to its node: %w", job.ScanID, err)
	}
	// With how long it waited in the core's queue: where the minute a root took to come back went was
	// not in the log (staging, 2026-10-21).
	scans.logger.InfoContext(ctx, "a scan's listing goes to its node",
		slogx.EventKey, "scanListingSent",
		"nodeId", scan.NodeID, "rootPath",
		scan.RootPath, "scopePath", scan.ScopePath, "jobId", job.ID, "attempt", job.AttemptNumber,
		"waitedMs", scans.clock.Now().UnixMilli()-scan.StartedAt, "readAgain", ptr.ValueOrZero(scan.ReadAgainJSON))
	return scans.assigner.Assign(ctx, scan.NodeID, job, nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandList, List: &nodeproto.List{
			RootPath: scan.RootPath, Path: scan.ScopePath, ErrorLimit: int(errorLimit),
		},
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
	if decodeErr := strictjson.Unmarshal([]byte(batch.FilesJSON), &opened.files); decodeErr != nil {
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
