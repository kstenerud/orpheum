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

// NodeHeader carries the node's id, from the hello reply, on every request after the hello.
const NodeHeader = "Orpheum-Node-ID"

// maxHelloBytes bounds a hello; a real one is a few hundred bytes.
const maxHelloBytes = 1 << 20

// Settings is what the service reads from the settings.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
	Bool(ctx context.Context, key settings.Key) (bool, error)
}

// Sink receives a node's report records, in order, a batch at a time: the ingest.
type Sink interface {
	Records(ctx context.Context, nodeID int64, records []nodeproto.Record) error
}

// ArtworkSink receives a node's answer to an artwork command: the artwork store's one way in
// (D212 §8).
type ArtworkSink interface {
	Answered(ctx context.Context, nodeID int64, result nodeproto.ArtworkResult)
}

// PrepareSink receives a node's reports on a prepare: how far it is, and how it ended.
type PrepareSink interface {
	Reported(ctx context.Context, nodeID int64, report nodeproto.PrepareReport)
}

// SubtitleSink receives a node's answer to an extract_subtitle command: the text, or why none.
type SubtitleSink interface {
	Extracted(ctx context.Context, nodeID int64, answer nodeproto.SubtitleText)
}

// Service is the core's side of the node channel (cut-a §4, D207): it enrols nodes, queues their
// commands, receives their reports and knows when each last spoke.
type Service struct {
	database *store.Database
	clock    clock.Clock
	settings Settings
	sink     Sink
	logger   *slog.Logger
	pollHold time.Duration

	artwork   ArtworkSink
	prepare   PrepareSink
	subtitles SubtitleSink

	mutex    sync.Mutex
	channels map[int64]*channel
}

// ReceiveArtwork names where the nodes' artwork answers go. Set it before the channel serves; an
// answer that arrives with none set is refused.
func (service *Service) ReceiveArtwork(sink ArtworkSink) {
	service.artwork = sink
}

// ReceivePrepare names where the nodes' prepare reports go. Set it before the channel serves; a
// report that arrives with none set is refused.
func (service *Service) ReceivePrepare(sink PrepareSink) {
	service.prepare = sink
}

// ReceiveSubtitles names where the nodes' subtitle answers go. Set it before the channel serves;
// an answer that arrives with none set is refused.
func (service *Service) ReceiveSubtitles(sink SubtitleSink) {
	service.subtitles = sink
}

// channel is what the core holds for one connected node.
type channel struct {
	name  string
	roots []string // as its last hello named them (D107)
	// confinement is what its last hello said about FFmpeg's confinement, per mechanism (D112).
	confinement []nodeproto.Confinement
	// prepared is what its last status said its prepared directory holds (D217 §5).
	prepared  *nodeproto.PreparedUsage
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
		database: database, clock: clockSource, settings: settingsService, sink: sink, logger: logger,
		pollHold: pollHold, channels: map[int64]*channel{},
	}
}

// Handler serves the node channel's paths.
func (service *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeproto.PathHello, service.hello)
	mux.HandleFunc("POST "+nodeproto.PathReport, service.report)
	mux.HandleFunc("GET "+nodeproto.PathCommands, service.commands)
	mux.HandleFunc("POST "+nodeproto.PathArtwork, service.artworkAnswer)
	mux.HandleFunc("POST "+nodeproto.PathPrepare, service.prepareReport)
	mux.HandleFunc("POST "+nodeproto.PathSubtitle, service.subtitleAnswer)
	return mux
}

// hello enrols a node it has not seen, by name (D207), and queues a scan of every root it names
// when scan_on_node_hello says so.
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
	scanOnHello, err := service.settings.Bool(ctx, settings.ScanOnNodeHello)
	if err != nil {
		service.fail(ctx, writer, "read scan_on_node_hello", err)
		return
	}
	service.connect(node.ID, hello)
	service.logger.InfoContext(ctx, "a node said hello", "node", hello.Name, "nodeId", node.ID,
		"version", hello.Version, "ffmpeg", hello.FFmpegVersion, "roots", hello.Roots,
		"confinement", hello.Confinement)
	if scanOnHello {
		for _, root := range hello.Roots {
			service.Enqueue(node.ID, nodeproto.Command{Type: nodeproto.CommandScan, Scan: &nodeproto.Scan{Root: root}})
		}
	}
	writeJSON(writer, nodeproto.HelloReply{NodeID: node.ID, ReportSeq: node.ReportSeq})
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
		found, err := store.One(queries.GetLiveNodeByName(ctx, name))
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
	existing, found := service.channels[nodeID]
	if !found {
		existing = &channel{name: hello.Name, nextSeq: 1, wake: make(chan struct{})}
		service.channels[nodeID] = existing
	}
	existing.lastHeard = service.clock.Now()
	existing.roots = slices.Clone(hello.Roots)
	existing.confinement = slices.Clone(hello.Confinement)
}

// Confinement is what the named node's last hello since the core started said about FFmpeg's
// confinement, per mechanism; false when it has not said hello since (D112).
func (service *Service) Confinement(name string) ([]nodeproto.Confinement, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for _, existing := range service.channels {
		if existing.name == name && !existing.lastHeard.IsZero() {
			return slices.Clone(existing.confinement), true
		}
	}
	return nil, false
}

// PreparedUsage is what the named node's last status said its prepared directory holds, and
// whether it said (D217 §5).
func (service *Service) PreparedUsage(name string) (nodeproto.PreparedUsage, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for _, existing := range service.channels {
		if existing.name == name && existing.prepared != nil {
			return *existing.prepared, true
		}
	}
	return nodeproto.PreparedUsage{}, false
}

// keepStatus keeps what a node's status says about it.
func (service *Service) keepStatus(nodeID int64, status *nodeproto.Status) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if existing, found := service.channels[nodeID]; found && status.Prepared != nil {
		usage := *status.Prepared
		existing.prepared = &usage
	}
}

// Roots are the media roots the named node configured, as its last hello since the core started
// said, with its id; false when it has not said hello since (D107: roots are configured on the
// node, never remotely).
func (service *Service) Roots(name string) (int64, []string, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for id, queue := range service.channels {
		if queue.name == name {
			return id, slices.Clone(queue.roots), true
		}
	}
	return 0, nil, false
}

// Enqueue queues a command for a node, numbering it, and wakes a waiting long-poll.
func (service *Service) Enqueue(nodeID int64, command nodeproto.Command) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	queue, found := service.channels[nodeID]
	if !found {
		return
	}
	command.Seq = queue.nextSeq
	queue.nextSeq++
	queue.commands = append(queue.commands, command)
	close(queue.wake)
	queue.wake = make(chan struct{})
}

// Connected is the node heard from most recently, and whether any has said hello since the core
// started: where work that any node can do is sent.
func (service *Service) Connected() (int64, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	var latest time.Time
	found := int64(0)
	for id, queue := range service.channels {
		if found == 0 || queue.lastHeard.After(latest) {
			found, latest = id, queue.lastHeard
		}
	}
	return found, found != 0
}

// IsConnected is whether a node has said hello since the core started: where work on its own
// files can be sent.
func (service *Service) IsConnected(nodeID int64) bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	_, found := service.channels[nodeID]
	return found
}

// LastHeard is when the named node last said anything, and whether it ever has.
func (service *Service) LastHeard(name string) (time.Time, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	for _, queue := range service.channels {
		if queue.name == name {
			return queue.lastHeard, true
		}
	}
	return time.Time{}, false
}

// heard notes that a node spoke.
func (service *Service) heard(nodeID int64) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if queue, found := service.channels[nodeID]; found {
		queue.lastHeard = service.clock.Now()
	}
}

// nodeID reads the node's id from a request after its hello, answering 400 or 409 if it is
// missing or unknown.
func (service *Service) nodeID(writer http.ResponseWriter, request *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(request.Header.Get(NodeHeader), 10, 64)
	if err != nil {
		http.Error(writer, "every request after the hello names its node in "+NodeHeader, http.StatusBadRequest)
		return 0, false
	}
	service.mutex.Lock()
	_, known := service.channels[id]
	service.mutex.Unlock()
	if !known {
		http.Error(writer, "say hello first: the core does not know this node", http.StatusConflict)
		return 0, false
	}
	return id, true
}

func (service *Service) fail(ctx context.Context, writer http.ResponseWriter, doing string, err error) {
	service.logger.ErrorContext(ctx, "the node channel failed", "doing", doing, "err", err)
	http.Error(writer, doing+" failed", http.StatusInternalServerError)
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		// The node hung up; it will say hello again.
		return
	}
}
