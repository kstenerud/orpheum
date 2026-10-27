// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// NodeIDHeader carries the node's id, from the hello reply, on every request after the hello.
const NodeIDHeader = "Orpheum-Node-ID"

// Settings is what the service reads from the settings.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
	Bool(ctx context.Context, key settings.Key) (bool, error)
}

// ScanRequester asks for a scan of a node's root: library.Scans (D240).
type ScanRequester interface {
	// RequestOnHello asks for the scan a node's hello brings of a root a library covers: before other
	// work when the root is unavailable, as the scan that brings it back (D240 §1, D328).
	RequestOnHello(ctx context.Context, nodeID int64, root string) error
	// MarkRootNotNamed makes a library root its node's hello no longer names unavailable from now
	// (D241 §2).
	MarkRootNotNamed(ctx context.Context, nodeID int64, root string) error
	// MarkStalled makes the deepest place the scans know under a root that holds waitingOn, whose
	// disk is not answering, unavailable from now, and asks for the scan that brings it back (D233
	// §10, D328 §5).
	MarkStalled(ctx context.Context, nodeID int64, root, waitingOn string) (place string, since time.Time, err error)
	// BringBackRoot makes an unavailable root whose disk answers again available at once, when the disk
	// is the one its last complete scan read, and says whether it did (D233 §10).
	BringBackRoot(ctx context.Context, nodeID int64, root string, device uint64, isMountPoint bool) (bool, error)
}

// ChangeTaker says which library folders a node's watcher watches, and takes its word that something
// changed under one: library.ScanSchedule, which scans the folder when its library is watched (D272
// §9).
type ChangeTaker interface {
	// WatchedFolders are the folders the node watches, which its hello's reply and each poll's answer
	// name (nodeproto.HelloReply.WatchFolders).
	WatchedFolders(ctx context.Context, nodeID int64) ([]string, error)
	TakeChange(ctx context.Context, nodeID int64, root, folder string) error
}

// ResultSink receives a node's word on a job, to hand to the job that asked: Exchanges. It is the
// one way a node's results reach the core; an artwork result, the artwork store's one way in
// (D212 §8).
type ResultSink interface {
	// Deliver records a result and hands it to the job waiting for it; an error leaves it
	// unacknowledged.
	Deliver(ctx context.Context, nodeID int64, result nodeproto.Result) error
	// DeliverProgress hands a node's word on where an attempt is to the job waiting for it.
	DeliverProgress(ctx context.Context, nodeID int64, progress nodeproto.Progress)
	// Interrupt tells the jobs given to a node under another start than startID, the one its hello
	// names, that it started again, losing what it held (D238 §3).
	Interrupt(ctx context.Context, nodeID int64, startID string)
	// Reconcile reconciles the jobs given to a node with the attempts its status says it holds, but for
	// those whose command, onTheirWay, it has not taken yet (D234 §6).
	Reconcile(ctx context.Context, nodeID int64, held []nodeproto.HeldAttempt, onTheirWay map[string]bool)
}

// Destinations are the job queue's parked destinations (jobs.Queue): a node's hello brings back
// the work parked on it, and its status showing room the runs parked on its prepared
// directory (D231 §5, §11).
type Destinations interface {
	Unpark(ctx context.Context, destination jobs.Destination)
}

// Service is the core's side of the node channel (cut-a §4, D207): it enrols nodes, queues their
// commands, receives their reports and knows when each last spoke and whether it is connected.
type Service struct {
	database     *store.Database
	clock        clock.Clock
	settings     Settings
	destinations Destinations
	logger       *slog.Logger
	pollHold     time.Duration

	resultSink  ResultSink
	heldResults HeldResultReader
	scans       ScanRequester
	// changes takes the nodes' watchers' notices; nil drops them.
	changes ChangeTaker
	// storeTiers says how strongly each chunk store is kept (D309 §8); nil sends none.
	storeTiers StoreTiers

	mutex                sync.Mutex
	nodesHeardSinceStart map[int64]*connectedNode
	// protocolByNodeID is the protocol version each node that needs updating last said hello in:
	// another than this core's, so it was refused (D233 §9). Guarded by mutex.
	protocolByNodeID map[int64]int
}

// HeldResultReader fetches a result a node holds, too large to post, from its byte socket:
// ByteSocketClient.
type HeldResultReader interface {
	ReadHeldResult(ctx context.Context, nodeID int64, posted nodeproto.Result) (nodeproto.Result, error)
}

// SetHeldResultReader names how a result a node holds is fetched. Set it before the channel serves;
// a held result that arrives with none set is refused.
func (service *Service) SetHeldResultReader(reader HeldResultReader) {
	service.heldResults = reader
}

// SetScans names what a hello asks for its roots' scans of. Set it before the channel serves; with
// none set, a hello asks for no scan.
func (service *Service) SetScans(scans ScanRequester) {
	service.scans = scans
}

// StoreTiers says how strongly each chunk store is kept, by its directory: playback.Service.StoreTiers.
type StoreTiers func(ctx context.Context) (map[string]nodeproto.KeepTier, error)

// SetStoreTiers names what says how strongly each chunk store is kept, sent with every answer to a
// node's poll (D309 §8). Set it before the channel serves; with none set, none is sent, and the
// nodes remove the least lately used first.
func (service *Service) SetStoreTiers(tiers StoreTiers) {
	service.storeTiers = tiers
}

// SetChanges names what takes the nodes' watchers' notices. Set it before the channel serves; with
// none set, a notice is taken and dropped.
func (service *Service) SetChanges(changes ChangeTaker) {
	service.changes = changes
}

// SetResultSink names where the nodes' results go. Set it before the channel serves; a result that
// arrives with none set is refused.
func (service *Service) SetResultSink(sink ResultSink) {
	service.resultSink = sink
}

// ServiceParts are what the service is built from.
type ServiceParts struct {
	Database *store.Database
	Clock    clock.Clock
	Settings Settings
	// Destinations are the job queue's parked destinations, which a hello and a status bring back.
	Destinations Destinations
	Logger       *slog.Logger
	// PollHold is how long a command long-poll waits before it answers with nothing.
	PollHold time.Duration
}

// NewService returns the service.
func NewService(parts ServiceParts) *Service {
	return &Service{
		database: parts.Database, clock: parts.Clock, settings: parts.Settings,
		destinations: parts.Destinations, logger: parts.Logger, pollHold: parts.PollHold,
		nodesHeardSinceStart: map[int64]*connectedNode{}, protocolByNodeID: map[int64]int{},
	}
}

// Handler serves the node channel's paths.
func (service *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeproto.PathHello, service.hello)
	mux.HandleFunc("POST "+nodeproto.PathStatus, service.takeStatus)
	mux.HandleFunc("GET "+nodeproto.PathCommands, service.serveCommands)
	mux.HandleFunc("POST "+nodeproto.PathResult, service.takeResult)
	mux.HandleFunc("POST "+nodeproto.PathProgress, service.takeProgress)
	mux.HandleFunc("POST "+nodeproto.PathChanged, service.takeChanged)
	return mux
}

// knownNodeIDOf reads the node's id from a request after its hello, answering 400 or 409 if it is
// missing or unknown.
func (service *Service) knownNodeIDOf(writer http.ResponseWriter, request *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(request.Header.Get(NodeIDHeader), 10, 64)
	if err != nil {
		http.Error(writer, "every request after the hello names its node in "+NodeIDHeader, http.StatusBadRequest)
		return 0, false
	}
	service.mutex.Lock()
	_, known := service.nodesHeardSinceStart[id]
	service.mutex.Unlock()
	if !known {
		http.Error(writer, "say hello first: the core does not know this node", http.StatusConflict)
		return 0, false
	}
	return id, true
}

func (service *Service) fail(ctx context.Context, writer http.ResponseWriter, action string, err error) {
	service.logger.ErrorContext(ctx, "the node channel failed", "action", action, "err", err)
	http.Error(writer, action+" failed", http.StatusInternalServerError)
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		// The node hung up; it will say hello again.
		return
	}
}
