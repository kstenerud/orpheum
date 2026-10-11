// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"

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
	if service.isRefused(ctx, writer, node.ID, hello) {
		return
	}
	scanOnHello, err := service.settings.Bool(ctx, settings.ShouldScanOnNodeHello)
	if err != nil {
		service.fail(ctx, writer, "read should_scan_on_node_hello", err)
		return
	}
	service.connect(node.ID, hello)
	// The work given to another start of the node is lost with it, whether or not the core heard
	// that start: each attempt keeps the start it was given to (D238 §3).
	if service.resultSink != nil {
		service.resultSink.Interrupt(ctx, node.ID, hello.StartID)
	}
	service.logger.InfoContext(ctx, "a node said hello", "nodeName", hello.Name, "nodeId", node.ID,
		"startId", hello.StartID, "build", hello.Build.String(), "ffmpegVersion", hello.FFmpegVersion,
		"rootPaths", hello.RootPaths, "byteSocket", hello.ByteSocketPath,
		"confinement", hello.Confinement, "capacity", hello.Capacity, "devices", hello.Devices)
	// A hello is the node's probe: the work parked on it, and on any node, resumes (D231 §6).
	service.destinations.Unpark(ctx, jobs.NodeDestination(node.ID))
	service.destinations.Unpark(ctx, jobs.DestinationAnyNode)
	// Its slots and devices may be more than its last hello said, or its limits since raised: the work
	// waiting for room on it is looked at again (D240 §5).
	for _, slot := range hello.Capacity.Slots() {
		service.destinations.Unpark(ctx, jobs.SlotDestination(node.ID, string(slot)))
	}
	for _, device := range hello.Devices {
		service.destinations.Unpark(ctx, jobs.ReaderDestination(node.ID, device.Key))
	}
	if scanOnHello {
		if scansErr := service.askForScans(ctx, node.ID, hello.RootPaths); scansErr != nil {
			service.fail(ctx, writer, "ask for the roots' scans", scansErr)
			return
		}
	}
	if service.scans != nil {
		if markErr := service.markRootsNotNamed(ctx, node.ID, hello.RootPaths); markErr != nil {
			service.fail(ctx, writer, "make the roots the hello does not name unavailable", markErr)
			return
		}
	}
	limits, err := service.readerLimits(ctx, node.ID, hello.Devices)
	if err != nil {
		service.fail(ctx, writer, "read the devices' reader limits", err)
		return
	}
	writeJSON(writer, nodeproto.HelloReply{
		NodeID: node.ID, KeptRenditionDirs: service.keptRenditions(ctx, node.ID, hello.Renditions),
		DeviceReaderLimits: limits,
	})
}

// isRefused answers a hello the core cannot take, and says so: one in another version of the
// protocol, whose node waits for an update (D233 §9), or one that does not say where its byte
// socket is, from which the core reads every byte of the node's (D242 §7).
func (service *Service) isRefused(
	ctx context.Context, writer http.ResponseWriter, nodeID int64, hello nodeproto.Hello,
) bool {
	if !service.speaksThisProtocol(nodeID, hello.ProtocolVersion) {
		service.logger.WarnContext(ctx, "refused a node that speaks another version of the node protocol",
			"nodeName", hello.Name, "nodeId", nodeID, "nodeProtocol", hello.ProtocolVersion,
			"coreProtocol", nodeproto.ProtocolVersion, "build", hello.Build.String())
		http.Error(writer, fmt.Sprintf("this core speaks node protocol %d, and this node %d: update it",
			nodeproto.ProtocolVersion, hello.ProtocolVersion), http.StatusConflict)
		return true
	}
	if !filepath.IsAbs(hello.ByteSocketPath) {
		http.Error(writer, "a hello says where its node's byte socket is, as an absolute path",
			http.StatusBadRequest)
		return true
	}
	return false
}

// askForScans asks for a scan of each of a node's roots a live library covers on it, no person
// waiting for it (D240 §1).
func (service *Service) askForScans(ctx context.Context, nodeID int64, rootPaths []string) error {
	if service.scans == nil {
		return nil
	}
	roots, err := service.coveredRoots(ctx, nodeID, rootPaths)
	if err != nil {
		return fmt.Errorf("read the libraries' roots: %w", err)
	}
	for _, root := range roots {
		if err := service.scans.Request(ctx, nodeID, root, jobs.PriorityBackground); err != nil {
			return err
		}
	}
	return nil
}

// markRootsNotNamed makes each library root on the node that its hello does not name unavailable:
// its files are not there to read, and nothing is removed on that absence; a hello that names it
// again asks for its scan, which brings it back (D241 §2).
func (service *Service) markRootsNotNamed(ctx context.Context, nodeID int64, named []string) error {
	libraryRoots, err := service.database.Read().ListLiveRootsOfNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("read node %d's library roots: %w", nodeID, err)
	}
	for _, root := range libraryRoots {
		if slices.Contains(named, root.RootPath) {
			continue
		}
		if err := service.scans.MarkRootNotNamed(ctx, nodeID, root.RootPath); err != nil {
			return err
		}
	}
	return nil
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

// keptRenditions reconciles the index with the renditions a hello lists and returns those it keeps
// (D238 §2). Without an index, or when it cannot be reconciled, every one is kept: the node evicts
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
		service.logger.ErrorContext(ctx, "could not reconcile the index with a node's renditions; keeping them all",
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
	delete(service.nodesHeardSinceStart, nodeID)
	return false
}

// connect registers a node's channel, keeping its queue across a reconnection, with the roots
// and the start its hello named.
func (service *Service) connect(nodeID int64, hello nodeproto.Hello) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	existing, found := service.nodesHeardSinceStart[nodeID]
	if !found {
		existing = &connectedNode{name: hello.Name, nextCommandSeq: 1, wake: make(chan struct{})}
		service.nodesHeardSinceStart[nodeID] = existing
	}
	existing.lastHeard = service.clock.Now()
	existing.hello = hello
}

// RemoveNode removes a node an admin retired (D231 §7), as Remove does, and forgets it: its
// queued commands are dropped, and its next request is told to say hello first.
func (service *Service) RemoveNode(ctx context.Context, nodeID int64) error {
	if err := Remove(ctx, service.database, service.clock, nodeID); err != nil {
		return err
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	delete(service.nodesHeardSinceStart, nodeID)
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
