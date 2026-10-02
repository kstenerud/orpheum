// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// maxHelloBytes bounds a hello: a few hundred bytes, and as many again for each rendition the node
// holds, so thousands fit.
const maxHelloBytes = 8 << 20

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
	if !service.speaksThisProtocol(node.ID, hello.ProtocolVersion) {
		service.logger.WarnContext(ctx, "refused a node that speaks another version of the node protocol",
			"nodeName", hello.Name, "nodeId", node.ID, "nodeProtocol", hello.ProtocolVersion,
			"coreProtocol", nodeproto.ProtocolVersion, "build", hello.Build.String())
		http.Error(writer, fmt.Sprintf("this core speaks node protocol %d, and this node %d: update it",
			nodeproto.ProtocolVersion, hello.ProtocolVersion), http.StatusConflict)
		return
	}
	scanOnHello, err := service.settings.Bool(ctx, settings.ShouldScanOnNodeHello)
	if err != nil {
		service.fail(ctx, writer, "read should_scan_on_node_hello", err)
		return
	}
	if service.connect(node.ID, hello) {
		service.logger.InfoContext(ctx, "a node started again; the work it held is lost", "nodeName", hello.Name,
			"nodeId", node.ID)
		if service.resultSink != nil {
			service.resultSink.Interrupt(ctx, node.ID)
		}
	}
	service.logger.InfoContext(ctx, "a node said hello", "nodeName", hello.Name, "nodeId", node.ID,
		"build", hello.Build.String(), "ffmpegVersion", hello.FFmpegVersion, "rootPaths", hello.RootPaths,
		"confinement", hello.Confinement, "capacity", hello.Capacity)
	if service.prepareSlotsSink != nil {
		service.prepareSlotsSink(service.prepareSlotCount())
	}
	// A hello is the node's probe: the work parked on it, and on any node, resumes (D231 §6).
	service.destinations.Unpark(ctx, jobs.NodeDestination(node.ID))
	service.destinations.Unpark(ctx, jobs.DestinationAnyNode)
	if scanOnHello {
		roots, err := service.coveredRoots(ctx, node.ID, hello.RootPaths)
		if err != nil {
			service.fail(ctx, writer, "read the libraries' roots", err)
			return
		}
		for _, root := range roots {
			service.Enqueue(node.ID, nodeproto.Command{
				Work: nodeproto.Work{Type: nodeproto.CommandScan, Scan: &nodeproto.Scan{RootPath: root}},
			})
		}
	}
	writeJSON(writer, nodeproto.HelloReply{
		NodeID: node.ID, LastIngestedReportSeq: node.LastIngestedReportSeq,
		KeptRenditionDirs: service.keptRenditions(ctx, node.ID, hello.Renditions),
	})
}

// coveredRoots are the roots, of those a node's hello names, that a live library covers on it: a
// scan of any other stores nothing, and would only hold the node's scans of those that do (the
// owner, 2026-10-02). Creating a library over a root asks for its scan.
func (service *Service) coveredRoots(ctx context.Context, nodeID int64, roots []string) ([]string, error) {
	covered := make([]string, 0, len(roots))
	for _, root := range roots {
		_, err := store.OrNotFound(service.database.Read().GetLibraryForRoot(ctx, sqlcgen.GetLibraryForRootParams{
			NodeID: nodeID, RootPath: root,
		}))
		switch {
		case errors.Is(err, store.ErrNotFound):
		case err != nil:
			return nil, fmt.Errorf("read the library over %s: %w", root, err)
		default:
			covered = append(covered, root)
		}
	}
	return covered, nil
}

// keptRenditions squares the index with the renditions a hello lists and returns those it keeps
// (D238 §2). Without an index, or when it cannot be squared, every one is kept: the node evicts
// nothing for the core's failure.
func (service *Service) keptRenditions(
	ctx context.Context, nodeID int64, renditions []nodeproto.RenditionManifest,
) []string {
	listed := make([]string, 0, len(renditions))
	for _, manifest := range renditions {
		listed = append(listed, manifest.Prepare.RenditionDir)
	}
	if service.renditionIndex == nil {
		return listed
	}
	kept, err := service.renditionIndex.AdoptRenditions(ctx, nodeID, renditions)
	if err != nil {
		service.logger.ErrorContext(ctx, "could not square the index with a node's renditions; keeping them all",
			"nodeId", nodeID, "err", err)
		return listed
	}
	return kept
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

// speaksThisProtocol is whether a node said hello in this core's version of the node protocol. One
// that did not is no longer connected, whatever it was, and needs updating; its work waits for it
// (D233 §9).
func (service *Service) speaksThisProtocol(nodeID int64, protocol int) bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if protocol == nodeproto.ProtocolVersion {
		delete(service.protocolByNodeID, nodeID)
		return true
	}
	service.protocolByNodeID[nodeID] = protocol
	delete(service.connectedNodes, nodeID)
	return false
}

// connect registers a node's channel, keeping its queue across a reconnection, with the roots
// its hello named. It reports whether the node started again since its last hello to this core:
// another start id (D238 §3).
func (service *Service) connect(nodeID int64, hello nodeproto.Hello) (isRestarted bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	existing, found := service.connectedNodes[nodeID]
	if !found {
		existing = &connectedNode{name: hello.Name, nextCommandSeq: 1, wake: make(chan struct{})}
		service.connectedNodes[nodeID] = existing
	}
	isRestarted = found && existing.hello.StartID != hello.StartID
	existing.lastHeard = service.clock.Now()
	existing.hello = hello
	return isRestarted
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
