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

	"github.com/kstenerud/orpheum/internal/core/jobs"
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
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
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
	status         *nodeproto.Status
	commands       []nodeproto.Command
	nextCommandSeq int64
	wake           chan struct{}
	lastHeard      time.Time
	// openPollCount is how many long-polls for its commands are open: while one is, it is
	// connected however long ago it last spoke.
	openPollCount int
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
		connectedNodes: map[int64]*connectedNode{},
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
	// A hello is the node's probe: the work parked on it, and on any node, resumes (D231 §6).
	service.destinations.Unpark(ctx, jobs.NodeDestination(node.ID))
	service.destinations.Unpark(ctx, jobs.DestinationAnyNode)
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
		existing = &connectedNode{name: hello.Name, nextCommandSeq: 1, wake: make(chan struct{})}
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

// keepStatus keeps what a node's status says about it. A prepared directory whose disk is within
// its limit again brings back the prepares parked on it for want of room (D231 §5).
func (service *Service) keepStatus(ctx context.Context, nodeID int64, status *nodeproto.Status) {
	service.mutex.Lock()
	if existing, found := service.connectedNodes[nodeID]; found {
		kept := *status
		existing.status = &kept
	}
	service.mutex.Unlock()
	if usage := status.PreparedUsage; usage != nil && usage.DiskUsedPercent <= usage.DiskUsedLimitPercent {
		service.destinations.Unpark(ctx, jobs.PreparedDirectoryDestination(nodeID))
	}
}

// ConnectedNodeRoots are the media roots the named node configured, as its last hello said, with
// its id, while it is connected; false when it is not (D107: roots are configured on the node,
// never remotely).
func (service *Service) ConnectedNodeRoots(ctx context.Context, name string) (int64, []string, bool) {
	silentAfter := service.silentAfter(ctx)
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for id, node := range service.connectedNodes {
		if node.name == name && service.isConnectedLocked(node, silentAfter) {
			return id, slices.Clone(node.hello.RootPaths), true
		}
	}
	return 0, nil, false
}

// Connection is whether a node is connected, as the core sees it (D231 §11).
type Connection string

// A node's connections.
const (
	// Connected is a node with a long-poll for its commands open, or heard within
	// node_silent_after_seconds.
	Connected Connection = "connected"
	// Silent is a node that said hello since the core started and has not been heard since
	// node_silent_after_seconds ago.
	Silent Connection = "silent"
	// AwaitingHello is a node enrolled that has not said hello since the core started.
	AwaitingHello Connection = "awaiting_hello"
)

// NodeConnection is whether a node is connected and when it was last heard; zero when it has
// not said hello since the core started.
func (service *Service) NodeConnection(ctx context.Context, nodeID int64) (Connection, time.Time) {
	silentAfter := service.silentAfter(ctx)
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node, found := service.connectedNodes[nodeID]
	switch {
	case !found:
		return AwaitingHello, time.Time{}
	case service.isConnectedLocked(node, silentAfter):
		return Connected, node.lastHeard
	}
	return Silent, node.lastHeard
}

// RemoveNode removes a node an admin retired (D231 §7), as Remove does, and forgets it: its
// queued commands are dropped, and its next request is told to say hello first.
func (service *Service) RemoveNode(ctx context.Context, nodeID int64) error {
	if err := Remove(ctx, service.database, service.clock, nodeID); err != nil {
		return err
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	delete(service.connectedNodes, nodeID)
	return nil
}

// Remove marks a node an admin retired as gone (D231 §7): its name, saying hello again, enrols a
// new node.
func Remove(ctx context.Context, database *store.Database, clockSource clock.Clock, nodeID int64) error {
	err := database.Write(ctx, func(queries *sqlcgen.Queries) error {
		nowMs := clockSource.Now().UnixMilli()
		_, err := queries.RemoveNode(ctx, sqlcgen.RemoveNodeParams{RemovedAt: &nowMs, ID: nodeID})
		return err
	})
	if err != nil {
		return fmt.Errorf("remove node %d: %w", nodeID, err)
	}
	return nil
}

// IsConnected is whether a node is connected now: a long-poll for its commands open, or heard
// within node_silent_after_seconds. Work on its files can be sent to it.
func (service *Service) IsConnected(ctx context.Context, nodeID int64) bool {
	connection, _ := service.NodeConnection(ctx, nodeID)
	return connection == Connected
}

// ConnectedNodeID is the connected node heard from most recently, where work that any node can
// do is sent; false when none is connected.
func (service *Service) ConnectedNodeID(ctx context.Context) (int64, bool) {
	silentAfter := service.silentAfter(ctx)
	service.mutex.Lock()
	defer service.mutex.Unlock()
	var latest time.Time
	latestNodeID := int64(0)
	for id, node := range service.connectedNodes {
		if service.isConnectedLocked(node, silentAfter) && (latestNodeID == 0 || node.lastHeard.After(latest)) {
			latestNodeID, latest = id, node.lastHeard
		}
	}
	return latestNodeID, latestNodeID != 0
}

// isConnectedLocked is whether node is connected; the caller holds the mutex.
func (service *Service) isConnectedLocked(node *connectedNode, silentAfter time.Duration) bool {
	return node.openPollCount > 0 || service.clock.Now().Sub(node.lastHeard) < silentAfter
}

// silentAfter is node_silent_after_seconds, or its starting value when it cannot be read.
func (service *Service) silentAfter(ctx context.Context) time.Duration {
	silentAfter, err := service.settings.Duration(ctx, settings.NodeSilentAfterSeconds)
	if err != nil {
		service.logger.WarnContext(ctx, "could not read node_silent_after_seconds; taking 30 s", "err", err)
		return defaultSilentAfter
	}
	return silentAfter
}

// defaultSilentAfter is node_silent_after_seconds' starting value, for when it cannot be read. It
// is fixed, not a setting of its own: a fallback that keeps a node's connection judged when the
// settings cannot be read.
const defaultSilentAfter = 30 * time.Second

// Send queues a command for a node that is connected, numbering it, and wakes a waiting
// long-poll. To a node that is not, it queues nothing and answers that the job is parked on the
// node until its hello (D231 §11).
func (service *Service) Send(ctx context.Context, nodeID int64, command nodeproto.Command) error {
	connection, lastHeard := service.NodeConnection(ctx, nodeID)
	if connection != Connected {
		return jobs.ParkOnNode(nodeID, lastHeard, ErrNodeSilent)
	}
	service.Enqueue(nodeID, command)
	return nil
}

// Unanswered is a job's wait for a node's answer that ran out: the node going away, which parks
// the job on it, when it is no longer connected; otherwise err, the job's own failure.
func (service *Service) Unanswered(ctx context.Context, nodeID int64, err error) error {
	if connection, lastHeard := service.NodeConnection(ctx, nodeID); connection != Connected {
		return jobs.ParkOnNode(nodeID, lastHeard, err)
	}
	return err
}

// Enqueue queues a command for a node that has said hello, numbering it, and wakes a waiting
// long-poll; one for a node that has not is dropped. Work goes through Send, which parks it on a
// node that is not connected; a cancel goes straight here.
func (service *Service) Enqueue(nodeID int64, command nodeproto.Command) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node, found := service.connectedNodes[nodeID]
	if !found {
		return
	}
	command.CommandSeq = node.nextCommandSeq
	node.nextCommandSeq++
	node.commands = append(node.commands, command)
	close(node.wake)
	node.wake = make(chan struct{})
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

// notePoll notes a long-poll for a node's commands opening (+1) or ending (-1): a node that holds
// one open is connected.
func (service *Service) notePoll(nodeID int64, change int) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if node, found := service.connectedNodes[nodeID]; found {
		node.openPollCount += change
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
