// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// maxLockedReportBytes bounds a locked node's report: its name, build and a few mechanisms.
const maxLockedReportBytes = 64 << 10

// lockedNode is what the core holds of a node that started locked (D450 §1): what it lacks, and when it
// last said so.
type lockedNode struct {
	report    nodeproto.LockedReport
	lastHeard time.Time
}

// UnconfinedLeave is an admin's leave for a node to run FFmpeg without the confinement its kernel
// lacks (D450 §1): who gave it, and when.
type UnconfinedLeave struct {
	AcceptedBy string
	AcceptedAt time.Time
}

// takeLocked answers a locked node's report (D450 §1): it enrols the node by name, as a hello does,
// keeps what it lacks for the System page, and says whether an admin lets it run unconfined.
func (service *Service) takeLocked(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	var report nodeproto.LockedReport
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxLockedReportBytes))
	if err := decoder.Decode(&report); err != nil {
		http.Error(writer, "a locked report is JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if report.Name == "" {
		http.Error(writer, "a locked report names its node", http.StatusBadRequest)
		return
	}
	node, err := enrol(ctx, service.database, service.clock, report.Name)
	if err != nil {
		service.fail(ctx, writer, "enrol the node", err)
		return
	}
	if !service.speaksThisProtocol(node.ID, report.ProtocolVersion) {
		http.Error(writer, fmt.Sprintf("this core speaks node protocol %d, and this node %d: update it",
			nodeproto.ProtocolVersion, report.ProtocolVersion), http.StatusConflict)
		return
	}
	_, isAllowed, err := service.UnconfinedLeave(ctx, node.ID)
	if err != nil {
		service.fail(ctx, writer, "read the node's leave to run unconfined", err)
		return
	}
	service.mutex.Lock()
	_, wasLocked := service.lockedByNodeID[node.ID]
	service.lockedByNodeID[node.ID] = lockedNode{report: report, lastHeard: service.clock.Now()}
	delete(service.nodesHeardSinceStart, node.ID)
	service.mutex.Unlock()
	if !wasLocked {
		service.logger.WarnContext(ctx, "a node started locked: its kernel lacks confinement its config requires, "+
			"so it runs no FFmpeg until an admin lets it on the System page",
			slogx.EventKey, "nodeLocked", "nodeName", report.Name, "nodeId", node.ID,
			"confinement", report.Confinement, "build", report.Build.String())
	}
	writeJSON(writer, nodeproto.LockedReply{MayRunUnconfined: isAllowed})
}

// mayRunUnconfined is whether an admin lets a node run unconfined now, for a hello's reply and each
// poll's answer; false, which stops a node running by the leave, when it cannot be read.
func (service *Service) mayRunUnconfined(ctx context.Context, nodeID int64) bool {
	_, isAllowed, err := service.UnconfinedLeave(ctx, nodeID)
	if err != nil {
		service.logger.WarnContext(ctx, "could not read a node's leave to run unconfined; the answer withholds it",
			"nodeId", nodeID, "err", err)
	}
	return isAllowed
}

// UnconfinedLeave is the admin's leave for a node to run without the confinement its kernel lacks;
// false when there is none (D450 §1).
func (service *Service) UnconfinedLeave(ctx context.Context, nodeID int64) (UnconfinedLeave, bool, error) {
	row, err := store.OrNotFound(service.database.Read().GetUnconfinedLeave(ctx, nodeID))
	if errors.Is(err, store.ErrNotFound) {
		return UnconfinedLeave{}, false, nil
	}
	if err != nil {
		return UnconfinedLeave{}, false, fmt.Errorf("read node %d's leave to run unconfined: %w", nodeID, err)
	}
	return UnconfinedLeave{AcceptedBy: row.AcceptedBy, AcceptedAt: time.UnixMilli(row.AcceptedAt)}, true, nil
}

// LockedConfinement is what a locked node said it lacks, and when it last said it; false for a node
// not locked since the core started (D450 §1).
func (service *Service) LockedConfinement(nodeID int64) ([]nodeproto.Confinement, time.Time, bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	locked, found := service.lockedByNodeID[nodeID]
	return locked.report.Confinement, locked.lastHeard, found
}

// AcceptUnconfined records an admin's leave for the node called nodeName to run FFmpeg without the
// confinement its kernel lacks, who and when, and logs it (D450 §1): the node's next report starts
// it. Accepting it again keeps the first.
func (service *Service) AcceptUnconfined(ctx context.Context, nodeName string, userID int64) error {
	now := service.clock.Now()
	nodeID, err := service.changeLeave(ctx, nodeName, func(queries *sqlcgen.Queries, nodeID int64) error {
		_, err := queries.AcceptUnconfinedNode(ctx, sqlcgen.AcceptUnconfinedNodeParams{
			NodeID: nodeID, AcceptedByUserID: userID, AcceptedAt: now.UnixMilli(),
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("let node %s run unconfined: %w", nodeName, err)
	}
	service.logger.WarnContext(ctx, "an admin let a node run FFmpeg without the confinement its kernel lacks",
		slogx.EventKey, "unconfinedLeaveGiven", "nodeName", nodeName, "nodeId", nodeID, "userId", userID,
		"at", now)
	return nil
}

// WithdrawUnconfined withdraws an admin's leave for the node called nodeName to run unconfined, and logs
// who and when (D450 §1): the node's waiting poll is answered at once, and the node stops and starts
// locked again.
func (service *Service) WithdrawUnconfined(ctx context.Context, nodeName string, userID int64) error {
	nodeID, err := service.changeLeave(ctx, nodeName, func(queries *sqlcgen.Queries, nodeID int64) error {
		_, err := queries.WithdrawUnconfinedNode(ctx, nodeID)
		return err
	})
	if err != nil {
		return fmt.Errorf("withdraw node %s's leave to run unconfined: %w", nodeName, err)
	}
	service.mutex.Lock()
	if node, found := service.nodesHeardSinceStart[nodeID]; found {
		node.isLeaveChanged = true
		close(node.wake)
		node.wake = make(chan struct{})
	}
	service.mutex.Unlock()
	service.logger.WarnContext(ctx, "an admin withdrew a node's leave to run without confinement",
		slogx.EventKey, "unconfinedLeaveWithdrawn", "nodeName", nodeName, "nodeId", nodeID, "userId", userID,
		"at", service.clock.Now())
	return nil
}

// changeLeave runs change on the live node called nodeName, ErrNoNode when there is none.
func (service *Service) changeLeave(
	ctx context.Context, nodeName string, change func(*sqlcgen.Queries, int64) error,
) (int64, error) {
	var nodeID int64
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		node, err := store.OrNotFound(queries.GetLiveNodeByName(ctx, nodeName))
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("node %q: %w", nodeName, ErrNoNode)
		}
		if err != nil {
			return err
		}
		nodeID = node.ID
		return change(queries, nodeID)
	})
	return nodeID, err
}

// takeLeaveChanged is whether a node's leave to run unconfined was withdrawn since a poll last answered
// it, which it forgets.
func (service *Service) takeLeaveChanged(nodeID int64) bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	node, found := service.nodesHeardSinceStart[nodeID]
	if !found {
		return false
	}
	isChanged := node.isLeaveChanged
	node.isLeaveChanged = false
	return isChanged
}
