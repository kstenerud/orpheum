// SPDX-License-Identifier: GPL-3.0-or-later

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

// Sink receives a node's report records, in order, a batch at a time: the ingest.
type Sink interface {
	IngestRecords(ctx context.Context, nodeID int64, records []nodeproto.Record) error
}

// ResultSink receives a node's word on a job, to hand to the job that asked: Exchanges. It is the
// one way a node's results reach the core; an artwork result, the artwork store's one way in
// (D212 §8).
type ResultSink interface {
	Deliver(ctx context.Context, nodeID int64, result nodeproto.Result)
	// DeliverProgress hands a node's word on where an attempt is to the job waiting for it.
	DeliverProgress(ctx context.Context, nodeID int64, progress nodeproto.Progress)
	// Interrupt tells the jobs waiting on a node that it started again, losing what it held.
	Interrupt(ctx context.Context, nodeID int64)
}

// Destinations are the job queue's parked destinations (jobs.Queue): a node's hello brings back
// the work parked on it, and its status showing room the prepares parked on its prepared
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
	recordSink   Sink
	destinations Destinations
	logger       *slog.Logger
	pollHold     time.Duration

	resultSink       ResultSink
	prepareSlotsSink PrepareSlotsSink

	mutex          sync.Mutex
	connectedNodes map[int64]*connectedNode
	// protocolByNodeID is the protocol version each node that needs updating last said hello in:
	// another than this core's, so it was refused (D233 §9). Guarded by mutex.
	protocolByNodeID map[int64]int
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
	// Sink receives each node's report records: the ingest.
	Sink Sink
	// Destinations are the job queue's parked destinations, which a hello and a status bring back.
	Destinations Destinations
	Logger       *slog.Logger
	// PollHold is how long a command long-poll waits before it answers with nothing.
	PollHold time.Duration
}

// NewService returns the service.
func NewService(parts ServiceParts) *Service {
	return &Service{
		database: parts.Database, clock: parts.Clock, settings: parts.Settings, recordSink: parts.Sink,
		destinations: parts.Destinations, logger: parts.Logger, pollHold: parts.PollHold,
		connectedNodes: map[int64]*connectedNode{}, protocolByNodeID: map[int64]int{},
	}
}

// Handler serves the node channel's paths.
func (service *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeproto.PathHello, service.hello)
	mux.HandleFunc("POST "+nodeproto.PathReport, service.serveReport)
	mux.HandleFunc("GET "+nodeproto.PathCommands, service.serveCommands)
	mux.HandleFunc("POST "+nodeproto.PathResult, service.takeResult)
	mux.HandleFunc("POST "+nodeproto.PathProgress, service.takeProgress)
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
	_, known := service.connectedNodes[id]
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
