// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"log/slog"
	"sync"
)

// Absences are the places at which a read found its file not there: its node answered that no file
// is at the path, or that the one there is not the file named (D100). A file renamed or deleted at a
// place is not known gone until a scan of it completes, and until then a read of the file skips the
// place for another its reader reaches (D363 §8): a play of a file whose first place was renamed away
// answered 404 though its copy stood (staging, 2026-10-27, VQ81). A place noted asks for a scan of its
// folder; a complete scan of a folder forgets those under it, having seen each file there again or
// removed the place. They are the core's memory alone: a restart forgets them, and the first read
// after finds the place again.
type Absences struct {
	logger *slog.Logger
	// scanFolderOf asks for a scan of the folder a place is in (Scans); nil asks none. Set once,
	// before any read.
	scanFolderOf func(ctx context.Context, nodeID int64, reportedPath []byte) error

	mutex  sync.Mutex
	places map[absentPlace]struct{} // guarded by mutex
}

// absentPlace is a place, by its node and the path its node reported.
type absentPlace struct {
	nodeID int64
	path   string
}

// NewAbsences knows of no place yet, and asks no scan of one. The core keeps one, its ingester's
// (Ingester.Absences), which the scans ask by; a test may make its own.
func NewAbsences(logger *slog.Logger) *Absences {
	return &Absences{logger: logger, places: map[absentPlace]struct{}{}}
}

// Note records that a read of the file at a place found it not there, for why, the node's word, and
// asks for a scan of its folder. A nil Absences notes nothing.
func (absences *Absences) Note(ctx context.Context, nodeID int64, reportedPath []byte, why string) {
	if absences == nil {
		return
	}
	absences.mutex.Lock()
	_, isKnown := absences.places[absentPlace{nodeID: nodeID, path: string(reportedPath)}]
	absences.places[absentPlace{nodeID: nodeID, path: string(reportedPath)}] = struct{}{}
	absences.mutex.Unlock()
	if isKnown {
		return
	}
	absences.logger.WarnContext(ctx, "a read found its file not at this place; reads of it go to another "+
		"place until a scan sees it here", "nodeId", nodeID, "path", string(reportedPath), "why", why)
	if absences.scanFolderOf == nil {
		return
	}
	if err := absences.scanFolderOf(ctx, nodeID, reportedPath); err != nil {
		absences.logger.WarnContext(ctx, "could not ask for a scan of a place a read found its file not at",
			"nodeId", nodeID, "path", string(reportedPath), "err", err)
	}
}

// IsAbsent is whether a read found the file at a place not there since a scan last saw it. A nil
// Absences knows of none.
func (absences *Absences) IsAbsent(nodeID int64, reportedPath []byte) bool {
	if absences == nil {
		return false
	}
	absences.mutex.Lock()
	defer absences.mutex.Unlock()
	_, isAbsent := absences.places[absentPlace{nodeID: nodeID, path: string(reportedPath)}]
	return isAbsent
}

// forgetUnder forgets the places at or under folder on a node, which a complete scan of it judged.
func (absences *Absences) forgetUnder(nodeID int64, folder string) {
	absences.mutex.Lock()
	defer absences.mutex.Unlock()
	for place := range absences.places {
		if place.nodeID == nodeID && isAtOrUnder([]byte(place.path), folder) {
			delete(absences.places, place)
		}
	}
}
