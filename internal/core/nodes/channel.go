// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// serveCommands answers a long-poll: the commands numbered after the one the node names, as soon
// as there are any, or none once pollHold has passed. Naming a number acknowledges every command up
// to it, and those are dropped.
func (service *Service) serveCommands(writer http.ResponseWriter, request *http.Request) {
	nodeID, ok := service.knownNodeIDOf(writer, request)
	if !ok {
		return
	}
	ackedSeq, err := strconv.ParseInt(request.URL.Query().Get(nodeproto.QueryAfterCommandSeq), 10, 64)
	if err != nil {
		http.Error(writer, "commands?"+nodeproto.QueryAfterCommandSeq+"=<the last command's seq, or 0>",
			http.StatusBadRequest)
		return
	}
	service.notePoll(nodeID, 1)
	defer service.notePoll(nodeID, -1)
	hold := service.clock.NewTimer(service.pollHold)
	defer hold.Stop()
	answer := func(ctx context.Context, pending []nodeproto.Command) {
		// Each answer carries the reader limits as the settings say now, so a change reaches the
		// node within a poll's hold (D240 §5).
		limits, err := service.readerLimits(ctx, nodeID, service.devicesOf(nodeID))
		if err != nil {
			service.logger.WarnContext(ctx, "could not read the devices' reader limits; the node keeps its last",
				"nodeId", nodeID, "err", err)
		}
		writeJSON(writer, nodeproto.Commands{Commands: pending, DeviceReaderLimits: limits})
	}
	for {
		pending, wake := service.dropAckedAndListPending(nodeID, ackedSeq)
		if len(pending) > 0 {
			answer(request.Context(), pending)
			return
		}
		select {
		case <-wake:
		case <-hold.C():
			answer(request.Context(), []nodeproto.Command{})
			return
		case <-request.Context().Done():
			return
		}
	}
}

// dropAckedAndListPending drops the commands a node has acknowledged and returns those after
// ackedSeq, with the channel that is closed when another is queued.
func (service *Service) dropAckedAndListPending(nodeID, ackedSeq int64) ([]nodeproto.Command, <-chan struct{}) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node := service.nodesHeardSinceStart[nodeID]
	kept := node.commands[:0]
	for _, command := range node.commands {
		if command.CommandSeq > ackedSeq {
			kept = append(kept, command)
		}
	}
	node.commands = kept
	return append([]nodeproto.Command(nil), kept...), node.wake
}

// maxStatusBytes bounds a status: a few hundred bytes, and a hundred more for each attempt the node
// holds, so thousands fit.
const maxStatusBytes = 4 << 20

// takeStatus takes a known node's status, at most maxStatusBytes of JSON: what it holds is reconciled
// with what it was given (D234 §6), and the last is kept, in memory and in persistent.db, for the
// System page and the doctor.
func (service *Service) takeStatus(writer http.ResponseWriter, request *http.Request) {
	nodeID, ok := service.knownNodeIDOf(writer, request)
	if !ok {
		return
	}
	var status nodeproto.Status
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxStatusBytes)).Decode(&status); err != nil {
		http.Error(writer, "a status is JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	ctx := request.Context()
	service.noteHeard(nodeID)
	service.keepStatus(ctx, nodeID, &status)
	encoded, err := json.Marshal(status)
	if err != nil {
		service.fail(ctx, writer, "encode the status", err)
		return
	}
	text, statusAt := string(encoded), service.clock.Now().UnixMilli()
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpdateNodeStatus(ctx, sqlcgen.UpdateNodeStatusParams{
			LastStatusJSON: &text, LastStatusAt: &statusAt, ID: nodeID,
		})
	})
	if err != nil {
		service.fail(ctx, writer, "keep the node's status", err)
		return
	}
	writeJSON(writer, struct{}{}) // the node reads a JSON answer
}

// maxResultBytes bounds a result. The largest is an artwork re-encode's: seven sizes of our own
// WebP, the largest 3840×2160, in base64; real ones are a few megabytes. A subtitle's is its text
// twice, each at most the node's own bound of 16 MiB, in base64; a prepare report and a loudness
// measurement are a few numbers.
const maxResultBytes = 64 << 20

// takeResult reads a known node's word on how a job ended, at most maxResultBytes of JSON, and hands
// it to the result sink, which records it; its answer is the node's acknowledgement (D234 §5).
// With nowhere to hand it, or a sink that could not record it, the result is refused, and the node
// sends it again.
func (service *Service) takeResult(writer http.ResponseWriter, request *http.Request) {
	nodeID, result, ok := readPost[nodeproto.Result](service, writer, request, maxResultBytes)
	if !ok {
		return
	}
	var err error
	if result.Held != nil {
		result, err = service.readHeld(request.Context(), nodeID, result)
	}
	if err == nil {
		err = service.resultSink.Deliver(request.Context(), nodeID, result)
	}
	switch {
	case errors.Is(err, ErrMalformedResult):
		http.Error(writer, err.Error(), http.StatusBadRequest)
	case err != nil:
		service.fail(request.Context(), writer, "record a node's result", err)
	default:
		writeJSON(writer, struct{}{}) // the node reads a JSON answer
	}
}

// readHeld fetches a result the node holds, too large to post, which its post named.
func (service *Service) readHeld(ctx context.Context, nodeID int64, posted nodeproto.Result) (nodeproto.Result, error) {
	if service.heldResults == nil {
		return nodeproto.Result{}, errors.New("this core cannot fetch a held result")
	}
	result, err := service.heldResults.ReadHeldResult(ctx, nodeID, posted)
	if err != nil {
		return nodeproto.Result{}, fmt.Errorf("take node %d's held result: %w", nodeID, err)
	}
	return result, nil
}

// maxProgressBytes bounds a progress post: a few numbers.
const maxProgressBytes = 4 << 10

// takeProgress reads a known node's word on where an attempt is, at most maxProgressBytes of JSON,
// and hands it to the job waiting for it. It is best effort: one nothing waits for is dropped.
func (service *Service) takeProgress(writer http.ResponseWriter, request *http.Request) {
	if nodeID, progress, ok := readPost[nodeproto.Progress](service, writer, request, maxProgressBytes); ok {
		service.resultSink.DeliverProgress(request.Context(), nodeID, progress)
		writeJSON(writer, struct{}{}) // the node reads a JSON answer
	}
}

// maxChangedBytes bounds a watcher's notice: a root and a folder under it.
const maxChangedBytes = 64 << 10

// takeChanged reads a known node watcher's notice of a change, at most maxChangedBytes of JSON, and
// hands it on, which scans the folder when its root's library is watched (D272 §9). One that cannot
// be taken is answered with an error, which the node logs; the root's periodic scan finds the change.
func (service *Service) takeChanged(writer http.ResponseWriter, request *http.Request) {
	nodeID, ok := service.knownNodeIDOf(writer, request)
	if !ok {
		return
	}
	service.noteHeard(nodeID)
	var changed nodeproto.Changed
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxChangedBytes)).Decode(&changed); err != nil {
		http.Error(writer, "a notice of a change is JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if service.changes != nil {
		ctx := request.Context()
		if err := service.changes.TakeChange(ctx, nodeID, changed.RootPath, changed.Path); err != nil {
			service.fail(ctx, writer, "take a watcher's notice", err)
			return
		}
	}
	writeJSON(writer, struct{}{}) // the node reads a JSON answer
}

// readPost reads a known node's post to the result sink, at most maxBytes of JSON, and notes that
// the node was heard. It answers the node itself, and reports false, for an unknown node, a core
// with no result sink, or a body that is not the JSON it should be.
func readPost[Body any](
	service *Service, writer http.ResponseWriter, request *http.Request, maxBytes int64,
) (int64, Body, bool) {
	var body Body
	nodeID, ok := service.knownNodeIDOf(writer, request)
	if !ok {
		return 0, body, false
	}
	service.noteHeard(nodeID)
	if service.resultSink == nil {
		http.Error(writer, "this core takes no results", http.StatusServiceUnavailable)
		return 0, body, false
	}
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxBytes)).Decode(&body); err != nil {
		http.Error(writer, "the body is not the JSON it should be: "+err.Error(), http.StatusBadRequest)
		return 0, body, false
	}
	return nodeID, body, true
}
