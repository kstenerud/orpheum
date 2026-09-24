// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

// Service is the core's side of the node channel (cut-a §4, D207): it enrols nodes, queues their
// commands, receives their reports and knows when each last spoke.
type Service struct {
	database *store.Database
	clock    clock.Clock
	settings Settings
	sink     Sink
	logger   *slog.Logger
	pollHold time.Duration

	mutex    sync.Mutex
	channels map[int64]*channel
}

// channel is what the core holds for one connected node.
type channel struct {
	name      string
	commands  []nodeproto.Command
	nextSeq   int64
	wake      chan struct{}
	lastHeard time.Time
}

// NewService returns the service. The pollHold argument is how long a command long-poll waits
// before it answers with nothing.
func NewService(
	database *store.Database, now clock.Clock, settingsService Settings, sink Sink, logger *slog.Logger,
	pollHold time.Duration,
) *Service {
	return &Service{
		database: database, clock: now, settings: settingsService, sink: sink, logger: logger, pollHold: pollHold,
		channels: map[int64]*channel{},
	}
}

// Handler serves the node channel's three paths.
func (service *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeproto.PathHello, service.hello)
	mux.HandleFunc("POST "+nodeproto.PathReport, service.report)
	mux.HandleFunc("GET "+nodeproto.PathCommands, service.commands)
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
	service.connect(node.ID, hello.Name)
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
func Enrol(ctx context.Context, database *store.Database, now clock.Clock, name string) (int64, error) {
	node, err := enrol(ctx, database, now, name)
	return node.ID, err
}

func enrol(
	ctx context.Context, database *store.Database, now clock.Clock, name string,
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
		enrolledAt := now.Now().UnixMilli()
		id, err := queries.InsertNode(ctx, sqlcgen.InsertNodeParams{Name: name, EnrolledAt: enrolledAt})
		if err != nil {
			return fmt.Errorf("enrol node %q: %w", name, err)
		}
		node = sqlcgen.GetLiveNodeByNameRow{ID: id, Name: name, EnrolledAt: enrolledAt}
		return nil
	})
	return node, err
}

// connect registers a node's channel, keeping its queue across a reconnection.
func (service *Service) connect(nodeID int64, name string) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	existing, found := service.channels[nodeID]
	if !found {
		existing = &channel{name: name, nextSeq: 1, wake: make(chan struct{})}
		service.channels[nodeID] = existing
	}
	existing.lastHeard = service.clock.Now()
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
