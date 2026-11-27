// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// A trickplay run posts each sheet as it is made, before its result (trickplay.md section 3), so a run
// stopped loses at most the sheet in progress. A sheet is taken only for an attempt the core still
// holds, by its node and token: one for a run no longer wanted, cancelled, resolved or given to
// another node, is answered nodeproto.StatusResultRejected, which ends the node's run.

// ErrAttemptNotHeld is a piece of work posted for an attempt the core does not hold: a run no longer
// wanted. The node is told to stop (nodeproto.StatusResultRejected).
var ErrAttemptNotHeld = errors.New("the core holds no such attempt")

// SheetTaker keeps one sheet of a trickplay attempt the core holds: trickplay.Service.TakeSheet.
type SheetTaker func(ctx context.Context, attempt Attempt, sheet nodeproto.TrickplaySheet) error

// TakeSheetsBy names the keeper of the trickplay runs' sheets. Set it before the channel serves.
func (all *Exchanges) TakeSheetsBy(taker SheetTaker) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	all.sheetTaker = taker
}

// DeliverTrickplaySheet hands a node's sheet to its keeper (TrickplaySheets): ErrAttemptNotHeld for an
// attempt the core does not hold for this node, of this job, at trickplay; ErrNotResolvedYet before
// a keeper is named.
func (all *Exchanges) DeliverTrickplaySheet(ctx context.Context, nodeID int64, sheet nodeproto.TrickplaySheet) error {
	all.mutex.Lock()
	held, found := all.attempts[sheet.AttemptToken]
	taker := all.sheetTaker
	var attempt Attempt
	if found {
		attempt = held.Attempt
	}
	all.mutex.Unlock()
	if !found || attempt.NodeID != nodeID || attempt.JobID != sheet.JobID || attempt.Work.MakeTrickplay == nil {
		return fmt.Errorf("sheet %d of job %d: %w", sheet.Index, sheet.JobID, ErrAttemptNotHeld)
	}
	if taker == nil {
		return fmt.Errorf("a trickplay sheet: %w", ErrNotResolvedYet)
	}
	return taker(ctx, attempt, sheet)
}

// TrickplaySheets take the nodes' trickplay sheets: Exchanges.
type TrickplaySheets interface {
	DeliverTrickplaySheet(ctx context.Context, nodeID int64, sheet nodeproto.TrickplaySheet) error
}

// SetTrickplaySheets names where the nodes' trickplay sheets go. Set it before the channel serves; a
// sheet that arrives with none set is refused, and the node sends it again.
func (service *Service) SetTrickplaySheets(sheets TrickplaySheets) {
	service.trickplaySheets = sheets
}

// maxSheetBytes bounds a sheet's post: a hundred pictures of 320 px at quality 60, 1 to 3 kB each
// (trickplay.md section 1), in base64, with room for a picture far busier than those measured.
const maxSheetBytes = 8 << 20

// takeTrickplaySheet reads a known node's trickplay sheet, at most maxSheetBytes of JSON, and hands
// it on. A sheet for a run no longer wanted, or one the core cannot take as it is, is answered
// nodeproto.StatusResultRejected, which ends the run; one it could not keep is refused, and the node
// sends it again.
func (service *Service) takeTrickplaySheet(writer http.ResponseWriter, request *http.Request) {
	nodeID, ok := service.knownNodeIDOf(writer, request)
	if !ok {
		return
	}
	service.noteHeard(nodeID)
	if service.trickplaySheets == nil {
		http.Error(writer, "this core takes no trickplay sheets", http.StatusServiceUnavailable)
		return
	}
	var sheet nodeproto.TrickplaySheet
	if err := strictjson.Decode(http.MaxBytesReader(writer, request.Body, maxSheetBytes), &sheet); err != nil {
		http.Error(writer, "a trickplay sheet is JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	err := service.trickplaySheets.DeliverTrickplaySheet(request.Context(), nodeID, sheet)
	switch {
	case errors.Is(err, ErrAttemptNotHeld), errors.Is(err, ErrMalformedResult):
		http.Error(writer, err.Error(), nodeproto.StatusResultRejected)
	case err != nil:
		service.fail(request.Context(), writer, "keep a node's trickplay sheet", err)
	default:
		writeJSON(writer, struct{}{}) // the node reads a JSON answer
	}
}

// priorityOnNode is the priority a node is told a job's work has: its own, but a trickplay's never a
// person's (trickplay.md section 4, question 4). Interest raises a trickplay's place in the queue
// like any of its film's jobs, to a play's for a film played; on its node it stays background work,
// never play-driven, never bumping work for a person nor passing its slot's window, so a raised run
// never takes the CPU or the disk from the playback that raised it.
func priorityOnNode(work nodeproto.Work, priority int64) int64 {
	if work.MakeTrickplay != nil {
		return min(priority, jobs.PriorityPersonsRequest-1)
	}
	return priority
}
