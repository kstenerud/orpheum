// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// NodeIDHeader carries the node's id, from the hello reply, on every request after the hello.
const NodeIDHeader = "Orpheum-Node-ID"

// maxHelloBytes bounds a hello; a real one is a few hundred bytes.
const maxHelloBytes = 1 << 20

// Settings is what the service reads from the settings.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
	Bool(ctx context.Context, key settings.Key) (bool, error)
}

// Sink receives a node's report records, in order, a batch at a time: the ingest.
type Sink interface {
	IngestRecords(ctx context.Context, nodeID int64, records []nodeproto.Record) error
}

// ArtworkSink receives a node's answer to an artwork command: the artwork store's one way in
// (D212 §8).
type ArtworkSink interface {
	TakeArtworkAnswer(ctx context.Context, nodeID int64, result nodeproto.ArtworkResult)
}

// PrepareSink receives a node's reports on a prepare: how far it is, and how it ended.
type PrepareSink interface {
	TakePrepareReport(ctx context.Context, nodeID int64, report nodeproto.PrepareReport)
}

// SubtitleSink receives a node's answer to an extract_subtitle command: the text, or why none.
type SubtitleSink interface {
	TakeSubtitleText(ctx context.Context, nodeID int64, answer nodeproto.SubtitleText)
}

// LoudnessSink receives a node's answer to a measure_loudness command: the measurements, or why
// none (D226).
type LoudnessSink interface {
	TakeLoudness(ctx context.Context, nodeID int64, answer nodeproto.LoudnessResult)
}

// Service is the core's side of the node channel (cut-a §4, D207): it enrols nodes, queues their
// commands, receives their reports and knows when each last spoke.
type Service struct {
	database   *store.Database
	clock      clock.Clock
	settings   Settings
	recordSink Sink
	logger     *slog.Logger
	pollHold   time.Duration

	artworkSink  ArtworkSink
	prepareSink  PrepareSink
	subtitleSink SubtitleSink
	loudnessSink LoudnessSink

	mutex          sync.Mutex
	connectedNodes map[int64]*connectedNode
}

// SetArtworkSink names where the nodes' artwork answers go. Set it before the channel serves; an
// answer that arrives with none set is refused.
func (service *Service) SetArtworkSink(sink ArtworkSink) {
	service.artworkSink = sink
}

// SetPrepareSink names where the nodes' prepare reports go. Set it before the channel serves; a
// report that arrives with none set is refused.
func (service *Service) SetPrepareSink(sink PrepareSink) {
	service.prepareSink = sink
}

// SetSubtitleSink names where the nodes' subtitle answers go. Set it before the channel serves;
// an answer that arrives with none set is refused.
func (service *Service) SetSubtitleSink(sink SubtitleSink) {
	service.subtitleSink = sink
}

// SetLoudnessSink names where the nodes' loudness answers go. Set it before the channel serves;
// an answer that arrives with none set is refused.
func (service *Service) SetLoudnessSink(sink LoudnessSink) {
	service.loudnessSink = sink
}

// connectedNode is what the core holds for one connected node.
type connectedNode struct {
	name string
	// hello is its last hello: its roots (D107), confinement (D112) and hardware (D225).
	hello nodeproto.Hello
	// status is its last status, nil before the first: what its prepared directory holds (D217
	// §5) and its disks (D225).
	status    *nodeproto.Status
	commands  []nodeproto.Command
	nextSeq   int64
	wake      chan struct{}
	lastHeard time.Time
}

// NewService returns the service. The pollHold argument is how long a command long-poll waits
// before it answers with nothing.
func NewService(
	database *store.Database, clockSource clock.Clock, settingsService Settings, sink Sink, logger *slog.Logger,
	pollHold time.Duration,
) *Service {
	return &Service{
		database: database, clock: clockSource, settings: settingsService, recordSink: sink, logger: logger,
		pollHold: pollHold, connectedNodes: map[int64]*connectedNode{},
	}
}

// Handler serves the node channel's paths.
func (service *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeproto.PathHello, service.hello)
	mux.HandleFunc("POST "+nodeproto.PathReport, service.serveReport)
	mux.HandleFunc("GET "+nodeproto.PathCommands, service.serveCommands)
	mux.HandleFunc("POST "+nodeproto.PathArtwork, service.artworkAnswer)
	mux.HandleFunc("POST "+nodeproto.PathPrepare, service.prepareReport)
	mux.HandleFunc("POST "+nodeproto.PathSubtitle, service.subtitleAnswer)
	mux.HandleFunc("POST "+nodeproto.PathLoudness, service.loudnessAnswer)
	return mux
}

// hello enrols a node it has not seen, by name (D207), and queues a scan of every root it names
// when should_scan_on_node_hello says so.
func (service *Service) hello(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	var hello nodeproto.Hello
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxHelloBytes)).Decode(&hello); err != nil {
		http.Error(writer, "a hello is JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if hello.Name == "" {
		http.Error(writer, "a hello names its node", http.StatusBadRequest)
		return
	}
	node, err := enrol(ctx, service.database, service.clock, hello.Name)
	if err != nil {
		service.fail(ctx, writer, "enrol the node", err)
		return
	}
	scanOnHello, err := service.settings.Bool(ctx, settings.ShouldScanOnNodeHello)
	if err != nil {
		service.fail(ctx, writer, "read should_scan_on_node_hello", err)
		return
	}
	service.connect(node.ID, hello)
	service.logger.InfoContext(ctx, "a node said hello", "nodeName", hello.Name, "nodeId", node.ID,
		"build", hello.Build.String(), "ffmpegVersion", hello.FFmpegVersion, "rootPaths", hello.RootPaths,
		"confinement", hello.Confinement)
	if scanOnHello {
		for _, root := range hello.RootPaths {
			service.Enqueue(node.ID, nodeproto.Command{
				Type: nodeproto.CommandScan,
				Scan: &nodeproto.Scan{RootPath: root},
			})
		}
	}
	writeJSON(writer, nodeproto.HelloReply{NodeID: node.ID, LastIngestedReportSeq: node.LastIngestedReportSeq})
}

// Enrol returns the id of the live node called name, creating its row if there is none (D207):
// the first hello does this, and so does a dev seed that names a library's node before the
// node has spoken.
func Enrol(ctx context.Context, database *store.Database, clockSource clock.Clock, name string) (int64, error) {
	node, err := enrol(ctx, database, clockSource, name)
	return node.ID, err
}

func enrol(
	ctx context.Context, database *store.Database, clockSource clock.Clock, name string,
) (sqlcgen.GetLiveNodeByNameRow, error) {
	var node sqlcgen.GetLiveNodeByNameRow
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		found, err := store.OrNotFound(queries.GetLiveNodeByName(ctx, name))
		if err == nil {
			node = found
			return nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		enrolledAt := clockSource.Now().UnixMilli()
		id, err := queries.InsertNode(ctx, sqlcgen.InsertNodeParams{Name: name, EnrolledAt: enrolledAt})
		if err != nil {
			return fmt.Errorf("enrol node %q: %w", name, err)
		}
		node = sqlcgen.GetLiveNodeByNameRow{ID: id, Name: name, EnrolledAt: enrolledAt}
		return nil
	})
	return node, err
}

// connect registers a node's channel, keeping its queue across a reconnection, with the roots
// its hello named.
func (service *Service) connect(nodeID int64, hello nodeproto.Hello) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	existing, found := service.connectedNodes[nodeID]
	if !found {
		existing = &connectedNode{name: hello.Name, nextSeq: 1, wake: make(chan struct{})}
		service.connectedNodes[nodeID] = existing
	}
	existing.lastHeard = service.clock.Now()
	existing.hello = hello
}

// Facts are what a node last said about itself since the core started: its hello, its last
// status (nil before the first) and when it was last heard, with its id (D225).
type Facts struct {
	NodeID    int64
	Hello     nodeproto.Hello
	Status    *nodeproto.Status
	LastHeard time.Time
}

// Facts is what the named node last said about itself; false when it has not said hello since
// the core started.
func (service *Service) Facts(name string) (Facts, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for id, existing := range service.connectedNodes {
		if existing.name == name && !existing.lastHeard.IsZero() {
			return Facts{
				NodeID: id, Hello: existing.hello, Status: existing.status, LastHeard: existing.lastHeard,
			}, true
		}
	}
	return Facts{}, false
}

// keepStatus keeps what a node's status says about it.
func (service *Service) keepStatus(nodeID int64, status *nodeproto.Status) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if existing, found := service.connectedNodes[nodeID]; found {
		kept := *status
		existing.status = &kept
	}
}

// ConnectedNodeRoots are the media roots the named node configured, as its last hello since the core started
// said, with its id; false when it has not said hello since (D107: roots are configured on the
// node, never remotely).
func (service *Service) ConnectedNodeRoots(name string) (int64, []string, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for id, node := range service.connectedNodes {
		if node.name == name {
			return id, slices.Clone(node.hello.RootPaths), true
		}
	}
	return 0, nil, false
}

// Enqueue queues a command for a node, numbering it, and wakes a waiting long-poll.
func (service *Service) Enqueue(nodeID int64, command nodeproto.Command) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node, found := service.connectedNodes[nodeID]
	if !found {
		return
	}
	command.CommandSeq = node.nextSeq
	node.nextSeq++
	node.commands = append(node.commands, command)
	close(node.wake)
	node.wake = make(chan struct{})
}

// LastHeardNodeID is the node heard from most recently, and whether any has said hello since the core
// started: where work that any node can do is sent.
func (service *Service) LastHeardNodeID() (int64, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	var latest time.Time
	latestNodeID := int64(0)
	for id, node := range service.connectedNodes {
		if latestNodeID == 0 || node.lastHeard.After(latest) {
			latestNodeID, latest = id, node.lastHeard
		}
	}
	return latestNodeID, latestNodeID != 0
}

// IsConnected is whether a node has said hello since the core started: where work on its own
// files can be sent.
func (service *Service) IsConnected(nodeID int64) bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	_, found := service.connectedNodes[nodeID]
	return found
}

// LastHeard is when the named node last said anything, and whether it ever has.
func (service *Service) LastHeard(name string) (time.Time, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for _, node := range service.connectedNodes {
		if node.name == name {
			return node.lastHeard, true
		}
	}
	return time.Time{}, false
}

// noteHeard notes that a node spoke.
func (service *Service) noteHeard(nodeID int64) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if node, found := service.connectedNodes[nodeID]; found {
		node.lastHeard = service.clock.Now()
	}
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
