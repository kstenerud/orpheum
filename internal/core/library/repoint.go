// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/pathtree"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// FolderMove is a person saying folder Path on node NodeName is now folder NewPath on node NewNodeName
// (D429 §2): any folder, at any level, the same node or another.
type FolderMove struct {
	NodeName    string
	Path        string
	NewNodeName string
	NewPath     string
}

// RepointRefusal is one reason a repoint is refused, which the dry run shows before the person
// confirms (repoint-library.md, "The spec").
type RepointRefusal string

// The reasons a repoint is refused.
const (
	// RefusedNothingToMove is a folder under which nothing is known: no place, no library folder.
	RefusedNothingToMove RepointRefusal = "nothingToMove"
	// RefusedSameFolder is a folder said to be where it is.
	RefusedSameFolder RepointRefusal = "sameFolder"
	// RefusedTargetNotReadable is a new folder its node does not list as readable: missing, not
	// answering, or failing.
	RefusedTargetNotReadable RepointRefusal = "targetNotReadable"
	// RefusedTargetHoldsKnown is a new folder under which places or library folders are already known:
	// a repoint never merges into them (the owner, 2026-11-06).
	RefusedTargetHoldsKnown RepointRefusal = "targetHoldsKnown"
	// RefusedLibraryWouldNest is a move after which a library would have a folder at, inside or above
	// another of its own (D363 §1).
	RefusedLibraryWouldNest RepointRefusal = "libraryWouldNest"
	// RefusedScanInProgress is a scan in progress of a folder at, under or above either folder: its
	// batches name the old paths (the wargame's §4.7).
	RefusedScanInProgress RepointRefusal = "scanInProgress"
)

// ErrRepointRefused is a repoint refused for the reasons its RefusedError carries.
var ErrRepointRefused = errors.New("the repoint is refused")

// ErrNotAFolderPath is a folder that is not an absolute, clean path.
var ErrNotAFolderPath = errors.New("not an absolute, clean folder path")

// RefusedError is a repoint refused, with the dry run that says why.
type RefusedError struct {
	Preview RepointPreview
}

func (refused *RefusedError) Error() string {
	return fmt.Sprintf("%s: %v", ErrRepointRefused, refused.Preview.Refusals)
}

func (refused *RefusedError) Unwrap() error { return ErrRepointRefused }

// MovedPath is a place's path now and after the move.
type MovedPath struct {
	Path    string
	NewPath string
}

// MovedRoot is a library's folder a repoint moves with the folder.
type MovedRoot struct {
	LibraryID   int64
	LibraryName string
	Path        string
	NewPath     string
	IsLive      bool
}

// RepointedLibrary is how many live files a library lists now and would not after the move (Loses), and
// the reverse (Gains). Roles reach libraries (D243), so it is also who can see them.
type RepointedLibrary struct {
	LibraryID   int64
	LibraryName string
	LosesCount  int
	GainsCount  int
}

// repointExampleCount is how many examples a dry run shows of what moves and of what conflicts.
const repointExampleCount = 5

// RepointPreview is a repoint's dry run: what it would move and change, what is in its way, and each
// reason it would be refused (repoint-library.md, "The spec").
type RepointPreview struct {
	LivePlaceCount    int
	RemovedPlaceCount int
	Examples          []MovedPath
	Roots             []MovedRoot
	Libraries         []RepointedLibrary
	IsCrossNode       bool
	// Target is the new folder as its node lists it.
	Target           nodeproto.FolderListing
	ConflictCount    int
	ConflictExamples []string
	// LookAlikeCount are places whose text is under the folder but whose bytes are not: another folder
	// on disk, which does not move.
	LookAlikeCount int
	Refusals       []RepointRefusal
}

// Repointed is what a repoint moved.
type Repointed struct {
	LivePlaceCount    int
	RemovedPlaceCount int
	RootCount         int
}

// repointPlan is a repoint worked out against the database: the preview, and what to write.
type repointPlan struct {
	preview   RepointPreview
	fromNode  int64
	toNode    int64
	from, to  string
	places    []sqlcgen.ListPlacesUnderRow
	movedRoot []MovedRoot
}

// newRaw is a place's bytes under the new folder: the new folder's, then what followed the old one.
func (plan *repointPlan) newRaw(raw []byte) []byte {
	return slices.Concat([]byte(plan.to), raw[len(plan.from):])
}

// newRootPath is a library folder's path at or under the new folder.
func (plan *repointPlan) newRootPath(rootPath string) string {
	return plan.to + rootPath[len(plan.from):]
}

// PreviewRepoint is a repoint's dry run (repoint-library.md, "The spec"): it changes nothing.
func (admin *Admin) PreviewRepoint(ctx context.Context, move FolderMove) (RepointPreview, error) {
	ends, err := admin.findRepointEnds(ctx, move)
	if err != nil {
		return RepointPreview{}, err
	}
	plan, err := admin.planRepoint(ctx, admin.database.Read(), move, ends.fromNode, ends.toNode)
	if err != nil {
		return RepointPreview{}, err
	}
	if err := admin.refuseOutside(ctx, &plan, ends.target); err != nil {
		return RepointPreview{}, err
	}
	return plan.preview, nil
}

// Repoint is folder X on node A now folder Y on node B (D429 §2): in one transaction, every place under
// X, live and removed, and every library folder at or under X, take Y, the old folders' scan state is
// forgotten, and a scan of the new place is asked for, which confirms each moved file by its read. It
// refuses (RefusedError) for any reason its dry run gives, checked again inside the transaction, and
// never merges into what is known at Y (the owner, 2026-11-06). The user it names asked, for the record.
func (admin *Admin) Repoint(ctx context.Context, move FolderMove, byUserID int64) (Repointed, error) {
	ends, err := admin.findRepointEnds(ctx, move)
	if err != nil {
		return Repointed{}, err
	}
	var plan repointPlan
	var mounts []string
	err = admin.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var planErr error
		plan, planErr = admin.planRepoint(ctx, queries, move, ends.fromNode, ends.toNode)
		if planErr != nil {
			return planErr
		}
		if refuseErr := admin.refuseOutside(ctx, &plan, ends.target); refuseErr != nil {
			return refuseErr
		}
		if len(plan.preview.Refusals) > 0 {
			return &RefusedError{Preview: plan.preview}
		}
		var moveErr error
		mounts, moveErr = writeRepoint(ctx, queries, &plan)
		return moveErr
	})
	if err != nil {
		return Repointed{}, err
	}
	admin.log().InfoContext(ctx, "repointed a folder", "byUserId", byUserID,
		"nodeName", move.NodeName, "path", plan.from, "newNodeName", move.NewNodeName, "newPath", plan.to,
		"livePlaceCount", plan.preview.LivePlaceCount, "removedPlaceCount", plan.preview.RemovedPlaceCount,
		"rootCount", len(plan.movedRoot))
	if err := admin.afterRepoint(ctx, &plan, mounts); err != nil {
		return Repointed{}, err
	}
	return Repointed{
		LivePlaceCount: plan.preview.LivePlaceCount, RemovedPlaceCount: plan.preview.RemovedPlaceCount,
		RootCount: len(plan.movedRoot),
	}, nil
}

// repointEnds are the old folder's node, any live one, and the new folder's, which must be connected
// with the new folder beneath one of its configured roots, and how that node lists the new folder.
type repointEnds struct {
	fromNode, toNode int64
	target           nodeproto.FolderListing
}

// findRepointEnds finds a repoint's ends, or why it has none: a path that is not clean, no such node,
// or a new folder outside the new node's roots or on a node not connected.
func (admin *Admin) findRepointEnds(ctx context.Context, move FolderMove) (repointEnds, error) {
	for _, folder := range []string{move.Path, move.NewPath} {
		if !pathtree.IsClean(folder) {
			return repointEnds{}, fmt.Errorf("%q: %w", folder, ErrNotAFolderPath)
		}
	}
	node, err := store.OrNotFound(admin.database.Read().GetLiveNodeByName(ctx, move.NodeName))
	if errors.Is(err, store.ErrNotFound) {
		return repointEnds{}, fmt.Errorf("node %q: %w", move.NodeName, ErrNoNode)
	}
	if err != nil {
		return repointEnds{}, fmt.Errorf("read node %q: %w", move.NodeName, err)
	}
	toNode, err := admin.libraryFolderOn(ctx, move.NewNodeName, move.NewPath)
	if err != nil {
		return repointEnds{}, err
	}
	target, err := admin.Folders(ctx, move.NewNodeName, move.NewPath)
	if err != nil {
		return repointEnds{}, err
	}
	return repointEnds{fromNode: node.ID, toNode: toNode, target: target}, nil
}

// refuseOutside adds the refusals the database cannot say: the new folder not readable, and a scan in
// progress of a folder at, under or above either folder.
func (admin *Admin) refuseOutside(ctx context.Context, plan *repointPlan, target nodeproto.FolderListing) error {
	plan.preview.Target = target
	if target.State != nodeproto.ProbeReadable {
		plan.preview.Refusals = append(plan.preview.Refusals, RefusedTargetNotReadable)
	}
	scanning, err := admin.scans.InProgress(ctx)
	if err != nil {
		return fmt.Errorf("read the scans in progress: %w", err)
	}
	for root := range scanning {
		if (root.NodeID == plan.fromNode && isNested(root.RootPath, plan.from)) ||
			(root.NodeID == plan.toNode && isNested(root.RootPath, plan.to)) {
			plan.preview.Refusals = append(plan.preview.Refusals, RefusedScanInProgress)
			break
		}
	}
	return nil
}

// isNested is whether two folders are one, or one is inside the other.
func isNested(a, b string) bool {
	return pathtree.IsAtOrUnder(a, b) || pathtree.IsAtOrUnder(b, a)
}

// planRepoint works a repoint out against the database: what moves, what each library gains and
// loses, what is in its way.
func (admin *Admin) planRepoint(
	ctx context.Context, queries *sqlcgen.Queries, move FolderMove, fromNode, toNode int64,
) (repointPlan, error) {
	plan := repointPlan{fromNode: fromNode, toNode: toNode, from: move.Path, to: move.NewPath}
	plan.preview.IsCrossNode = fromNode != toNode
	if fromNode == toNode && plan.from == plan.to {
		plan.preview.Refusals = append(plan.preview.Refusals, RefusedSameFolder)
	}
	after, before := PathRange(plan.from)
	places, err := queries.ListPlacesUnder(ctx, sqlcgen.ListPlacesUnderParams{
		NodeID: fromNode, Folder: []byte(plan.from), After: after, Before: before,
	})
	if err != nil {
		return repointPlan{}, fmt.Errorf("list the places under %s: %w", plan.from, err)
	}
	plan.takePlaces(places)
	lookAlikes, err := queries.CountLookAlikePlacesUnder(ctx, sqlcgen.CountLookAlikePlacesUnderParams{
		NodeID: fromNode, TextAfter: PathText(after), TextBefore: PathText(before), After: after, Before: before,
	})
	if err != nil {
		return repointPlan{}, fmt.Errorf("count the places spelled like %s: %w", plan.from, err)
	}
	plan.preview.LookAlikeCount = int(lookAlikes)
	fromRoots, err := queries.ListRootsOfNodeWithRetired(ctx, fromNode)
	if err != nil {
		return repointPlan{}, fmt.Errorf("list node %d's library folders: %w", fromNode, err)
	}
	toRoots := fromRoots
	if toNode != fromNode {
		if toRoots, err = queries.ListRootsOfNodeWithRetired(ctx, toNode); err != nil {
			return repointPlan{}, fmt.Errorf("list node %d's library folders: %w", toNode, err)
		}
	}
	plan.takeRoots(fromRoots)
	if len(places) == 0 && len(plan.movedRoot) == 0 {
		plan.preview.Refusals = append(plan.preview.Refusals, RefusedNothingToMove)
	}
	if err := plan.findConflicts(ctx, queries, toRoots); err != nil {
		return repointPlan{}, err
	}
	plan.compareLibraries(fromRoots, toRoots)
	return plan, nil
}

// takePlaces takes the places that move, counting the live and the removed and keeping a few as
// examples.
func (plan *repointPlan) takePlaces(places []sqlcgen.ListPlacesUnderRow) {
	plan.places = places
	for _, place := range places {
		if place.RemovedAt == nil {
			plan.preview.LivePlaceCount++
		} else {
			plan.preview.RemovedPlaceCount++
		}
		if len(plan.preview.Examples) < repointExampleCount {
			plan.preview.Examples = append(plan.preview.Examples, MovedPath{
				Path: place.Path, NewPath: PathText(plan.newRaw(place.ReportedPathRaw)),
			})
		}
	}
}

// takeRoots takes the library folders, live and retired, at or under the folder that moves.
func (plan *repointPlan) takeRoots(fromRoots []sqlcgen.ListRootsOfNodeWithRetiredRow) {
	for _, root := range fromRoots {
		if pathtree.IsAtOrUnder(root.RootPath, plan.from) {
			plan.movedRoot = append(plan.movedRoot, MovedRoot{
				LibraryID: root.LibraryID, LibraryName: root.LibraryName, Path: root.RootPath,
				NewPath: plan.newRootPath(root.RootPath), IsLive: root.IsLive == 1,
			})
		}
	}
	plan.preview.Roots = plan.movedRoot
}

// isMoving is whether a place or folder on a node is under the folder that moves.
func (plan *repointPlan) isMoving(nodeID int64, folder string) bool {
	return nodeID == plan.fromNode && pathtree.IsAtOrUnder(folder, plan.from)
}

// findConflicts finds what is already known at the new folder and is not moving: places under it,
// live or removed, and library folders at or under it, live or retired. Any one refuses the repoint:
// it never merges (the owner, 2026-11-06).
func (plan *repointPlan) findConflicts(
	ctx context.Context, queries *sqlcgen.Queries, toRoots []sqlcgen.ListRootsOfNodeWithRetiredRow,
) error {
	after, before := PathRange(plan.to)
	known, err := queries.ListPlacesUnder(ctx, sqlcgen.ListPlacesUnderParams{
		NodeID: plan.toNode, Folder: []byte(plan.to), After: after, Before: before,
	})
	if err != nil {
		return fmt.Errorf("list the places under %s: %w", plan.to, err)
	}
	var conflicts []string
	for _, place := range known {
		if !plan.isMoving(plan.toNode, string(place.ReportedPathRaw)) {
			conflicts = append(conflicts, place.Path)
		}
	}
	for _, root := range toRoots {
		if pathtree.IsAtOrUnder(root.RootPath, plan.to) && !plan.isMoving(plan.toNode, root.RootPath) {
			conflicts = append(conflicts, root.RootPath)
		}
	}
	plan.preview.ConflictCount = len(conflicts)
	plan.preview.ConflictExamples = conflicts[:min(len(conflicts), repointExampleCount)]
	if len(conflicts) > 0 {
		plan.preview.Refusals = append(plan.preview.Refusals, RefusedTargetHoldsKnown)
	}
	return nil
}

// placedRoot is a live library folder on a node, before or after the move.
type placedRoot struct {
	libraryID   int64
	libraryName string
	nodeID      int64
	path        string
}

// compareLibraries counts, for each library, the moved live files it lists now and would not after,
// and the reverse, and refuses a move after which a library would hold a folder at, inside or above
// another of its own (D363 §1).
func (plan *repointPlan) compareLibraries(fromRoots, toRoots []sqlcgen.ListRootsOfNodeWithRetiredRow) {
	beforeRoots, afterRoots := plan.placeRoots(fromRoots, toRoots)
	plan.refuseNesting(afterRoots)
	plan.countLibraryChanges(beforeRoots, afterRoots)
}

// placeRoots are the live library folders on the two nodes, before the move and after it.
func (plan *repointPlan) placeRoots(
	fromRoots, toRoots []sqlcgen.ListRootsOfNodeWithRetiredRow,
) (beforeRoots, afterRoots []placedRoot) {
	for _, root := range fromRoots {
		if root.IsLive != 1 {
			continue
		}
		placed := placedRoot{
			libraryID: root.LibraryID, libraryName: root.LibraryName, nodeID: plan.fromNode, path: root.RootPath,
		}
		beforeRoots = append(beforeRoots, placed)
		if plan.isMoving(plan.fromNode, root.RootPath) {
			placed.nodeID, placed.path = plan.toNode, plan.newRootPath(root.RootPath)
		}
		afterRoots = append(afterRoots, placed)
	}
	if plan.toNode != plan.fromNode {
		for _, root := range toRoots {
			if root.IsLive == 1 {
				afterRoots = append(afterRoots, placedRoot{
					libraryID: root.LibraryID, libraryName: root.LibraryName, nodeID: plan.toNode, path: root.RootPath,
				})
			}
		}
	}
	return beforeRoots, afterRoots
}

// countLibraryChanges counts, for each library, the moved live files it lists before and not after,
// and the reverse.
func (plan *repointPlan) countLibraryChanges(beforeRoots, afterRoots []placedRoot) {
	changes := map[int64]*RepointedLibrary{}
	change := func(root placedRoot) *RepointedLibrary {
		if changes[root.libraryID] == nil {
			changes[root.libraryID] = &RepointedLibrary{LibraryID: root.libraryID, LibraryName: root.libraryName}
		}
		return changes[root.libraryID]
	}
	for _, place := range plan.places {
		if place.RemovedAt != nil {
			continue
		}
		before := coveringLibraries(beforeRoots, plan.fromNode, place.ReportedPathRaw)
		after := coveringLibraries(afterRoots, plan.toNode, plan.newRaw(place.ReportedPathRaw))
		for libraryID, root := range before {
			if _, kept := after[libraryID]; !kept {
				change(root).LosesCount++
			}
		}
		for libraryID, root := range after {
			if _, had := before[libraryID]; !had {
				change(root).GainsCount++
			}
		}
	}
	for _, each := range changes {
		plan.preview.Libraries = append(plan.preview.Libraries, *each)
	}
	slices.SortFunc(plan.preview.Libraries, func(a, b RepointedLibrary) int { return int(a.LibraryID - b.LibraryID) })
}

// refuseNesting refuses a move after which a library would have a moved folder at, inside or above
// another of its live folders on the new node (D363 §1).
func (plan *repointPlan) refuseNesting(afterRoots []placedRoot) {
	for _, moved := range plan.movedRoot {
		if !moved.IsLive {
			continue
		}
		for _, other := range afterRoots {
			if other.libraryID != moved.LibraryID || other.nodeID != plan.toNode || other.path == moved.NewPath {
				continue
			}
			if isNested(moved.NewPath, other.path) {
				plan.preview.Refusals = append(plan.preview.Refusals, RefusedLibraryWouldNest)
				return
			}
		}
	}
}

// coveringLibraries are the libraries one of whose live folders on the node holds the bytes.
func coveringLibraries(roots []placedRoot, nodeID int64, raw []byte) map[int64]placedRoot {
	covering := map[int64]placedRoot{}
	for _, root := range roots {
		if root.nodeID == nodeID && isAtOrUnder(raw, root.path) {
			covering[root.libraryID] = root
		}
	}
	return covering
}

// repointAside is the prefix a repoint first moves a path under: no real path has it, since every real
// one starts with "/", so the second step's UNIQUE checks, made row by row, meet nothing still at a
// new path (moving /nest to /nest/sub; visibility-at-scale §5).
const repointAside = "\x00"

// writeRepoint writes a planned repoint: the paths identification read follow their places, every
// place and library folder goes aside and then to its new place, and the old folders' scan state is
// forgotten. It returns the disks that were mounted under the old folder.
func writeRepoint(ctx context.Context, queries *sqlcgen.Queries, plan *repointPlan) ([]string, error) {
	for _, place := range plan.places {
		newPath := PathText(plan.newRaw(place.ReportedPathRaw))
		// Read from the place before it moves (D208 §1).
		err := queries.RepointFileFilings(ctx, sqlcgen.RepointFileFilingsParams{Path: &newPath, ID: place.ID})
		if err != nil {
			return nil, fmt.Errorf("move place %d's identified path: %w", place.ID, err)
		}
		err = queries.MovePlace(ctx, sqlcgen.MovePlaceParams{
			NodeID: plan.fromNode, Path: repointAside + place.Path,
			ReportedPathRaw: slices.Concat([]byte(repointAside), place.ReportedPathRaw), ID: place.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("move place %d aside: %w", place.ID, err)
		}
	}
	for _, root := range plan.movedRoot {
		if err := moveLibraryRoot(ctx, queries, sqlcgen.MoveLibraryRootParams{
			LibraryID: root.LibraryID, NodeID: plan.fromNode, RootPath: root.Path,
			NewNodeID: plan.fromNode, NewRootPath: repointAside + root.Path,
		}); err != nil {
			return nil, err
		}
	}
	forgetDisk := int64(0)
	if plan.toNode != plan.fromNode {
		forgetDisk = 1
	}
	for _, place := range plan.places {
		newRaw := plan.newRaw(place.ReportedPathRaw)
		err := queries.MovePlace(ctx, sqlcgen.MovePlaceParams{
			NodeID: plan.toNode, Path: PathText(newRaw), ReportedPathRaw: newRaw, IsFinal: 1, ForgetDisk: forgetDisk,
			ID: place.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("move place %d to %s: %w", place.ID, PathText(newRaw), err)
		}
	}
	for _, root := range plan.movedRoot {
		if err := moveLibraryRoot(ctx, queries, sqlcgen.MoveLibraryRootParams{
			LibraryID: root.LibraryID, NodeID: plan.fromNode, RootPath: repointAside + root.Path,
			NewNodeID: plan.toNode, NewRootPath: root.NewPath,
		}); err != nil {
			return nil, err
		}
	}
	return forgetFolderState(ctx, queries, plan.fromNode, plan.from)
}

// moveLibraryRoot moves one library folder, live or retired, from one place to another.
func moveLibraryRoot(ctx context.Context, queries *sqlcgen.Queries, move sqlcgen.MoveLibraryRootParams) error {
	count, err := queries.MoveLibraryRoot(ctx, move)
	if err != nil {
		return fmt.Errorf("move library %d's folder %q: %w", move.LibraryID, move.RootPath, err)
	}
	if count == 0 {
		return fmt.Errorf("library %d's folder %q: %w", move.LibraryID, move.RootPath, ErrNoSuchRoot)
	}
	return nil
}

// forgetFolderState forgets the scan state of every folder at or under a repointed folder: its facts,
// its last scan, the disks mounted in it and its places' availability, all rebuilt by the scan of the
// new place; carried over, the old facts would mislead the removal checks there (D208 §3). It returns
// the disks that were mounted there, whose parked work goes.
func forgetFolderState(ctx context.Context, queries *sqlcgen.Queries, nodeID int64, folder string) ([]string, error) {
	after, before := PathRange(folder)
	rootPath, textAfter, textBefore := strings.TrimSuffix(path.Clean(folder), "/"), string(after), string(before)
	mounts, err := queries.ListRootMountsAtOrUnder(ctx, sqlcgen.ListRootMountsAtOrUnderParams{
		NodeID: nodeID, RootPath: rootPath, After: textAfter, Before: textBefore,
	})
	if err != nil {
		return nil, fmt.Errorf("list the disks mounted under %s: %w", folder, err)
	}
	if err := queries.ForgetRootStatesAtOrUnder(ctx, sqlcgen.ForgetRootStatesAtOrUnderParams{
		NodeID: nodeID, RootPath: rootPath, After: textAfter, Before: textBefore,
	}); err != nil {
		return nil, fmt.Errorf("forget the state of the folders under %s: %w", folder, err)
	}
	if err := queries.ForgetRootScansAtOrUnder(ctx, sqlcgen.ForgetRootScansAtOrUnderParams{
		NodeID: nodeID, RootPath: rootPath, After: textAfter, Before: textBefore,
	}); err != nil {
		return nil, fmt.Errorf("forget the scans of the folders under %s: %w", folder, err)
	}
	if err := queries.ForgetRootMountsAtOrUnder(ctx, sqlcgen.ForgetRootMountsAtOrUnderParams{
		NodeID: nodeID, RootPath: rootPath, After: textAfter, Before: textBefore,
	}); err != nil {
		return nil, fmt.Errorf("forget the disks mounted under %s: %w", folder, err)
	}
	return mounts, forgetPlacesAtOrUnder(ctx, queries, nodeID, rootPath, textAfter, textBefore)
}

// afterRepoint tells the nodes their watched folders changed, cancels the work parked on the old
// folders and the disks mounted under them, and asks for a scan of the new place: of each moved live
// library folder, or, when none moved, of the new folder within each live library folder that holds
// it. The scan reads each moved file again and confirms it (D432 §6).
func (admin *Admin) afterRepoint(ctx context.Context, plan *repointPlan, mounts []string) error {
	admin.watchedFoldersChanged()
	var forgotten jobs.RetiredWork
	for _, root := range plan.movedRoot {
		forgotten.Destinations = append(forgotten.Destinations, jobs.RootDestination(plan.fromNode, root.Path))
	}
	for _, mount := range mounts {
		forgotten.Destinations = append(forgotten.Destinations, jobs.RootDestination(plan.fromNode, mount))
	}
	if _, err := admin.retirement.Work.CancelRetired(ctx, forgotten); err != nil {
		return fmt.Errorf("forget the old folders' waiting work: %w", err)
	}
	scanned := map[string]bool{}
	for _, root := range plan.movedRoot {
		if root.IsLive && !scanned[root.NewPath] {
			scanned[root.NewPath] = true
			if err := admin.scans.Request(ctx, plan.toNode, root.NewPath, jobs.PriorityPersonsRequest); err != nil {
				return err
			}
		}
	}
	if len(scanned) > 0 {
		return nil
	}
	folders, err := admin.database.Read().ListLiveRootPathsOnNode(ctx, plan.toNode)
	if err != nil {
		return fmt.Errorf("list node %d's library folders: %w", plan.toNode, err)
	}
	for _, folder := range folders {
		if pathtree.IsAtOrUnder(plan.to, folder) {
			err := admin.scans.RequestUnder(ctx, plan.toNode, folder, plan.to, jobs.PriorityPersonsRequest)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
