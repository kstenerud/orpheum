// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The scans' answers to a place's availability (D328): asking a place's node whether it can be read,
// bringing it back, and making it unavailable when work stalls on its disk or its node's hello no
// longer names it. availability.go writes what they decide.

// RootProber asks a node whether a place, a root or a disk inside one, can be read now, outside
// every queue:
// nodes.ByteSocketClient (D327).
type RootProber interface {
	ProbePlace(ctx context.Context, nodeID int64, root string) (nodeproto.PlaceProbe, error)
}

// SetProber has the scans probe roots through prober, which is made after them: the byte socket's
// client tells them of stalls.
func (scans *Scans) SetProber(prober RootProber) {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scans.prober = prober
}

// probeWait bounds how long one ask waits on a node's probe: one that has not answered by then is a
// disk not answering, the node's probe going on for the next ask to share (D327 §2). A filesystem
// that answers does so in milliseconds.
const probeWait = 2 * time.Second

// ProbeAndBringBack asks the root's node whether it can be read now, and brings it back when it can
// with its last complete scan's facts (D326, D327 §3); it says what the probe found and whether the
// root came back. A node that did not answer in time is not answering; with no prober, the probe's
// state is empty: nothing was asked.
func (scans *Scans) ProbeAndBringBack(ctx context.Context, nodeID int64, root string) (
	nodeproto.PlaceProbe, bool, error,
) {
	scans.mutex.Lock()
	prober := scans.prober
	scans.mutex.Unlock()
	if prober == nil {
		return nodeproto.PlaceProbe{Path: root}, false, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeWait)
	defer cancel()
	probe, err := prober.ProbePlace(probeCtx, nodeID, root)
	if err != nil {
		if ctx.Err() != nil {
			return nodeproto.PlaceProbe{}, false, ctx.Err()
		}
		return nodeproto.PlaceProbe{Path: root, State: nodeproto.ProbeNotAnswering, Failure: err.Error()}, false,
			nil
	}
	if probe.State != nodeproto.ProbeReadable {
		return probe, false, nil
	}
	isBack, err := scans.BringBackRoot(ctx, nodeID, root, probe.Device, probe.IsMountPoint)
	return probe, isBack, err
}

// NewScans returns the scans.
func NewScans(parts ScansParts) *Scans {
	return &Scans{
		cache: parts.Cache, ingester: parts.Ingester, jobs: parts.Jobs, assigner: parts.Assigner,
		settings: parts.Settings, clock: parts.Clock, logger: parts.Logger,
		completions: map[int64]nodeproto.ScanComplete{},
	}
}

// Request asks for a scan of a node's whole root at priority. One asked for while the root's scan
// runs starts none: that one runs once more after it, however many ask (D236 §3).
func (scans *Scans) Request(ctx context.Context, nodeID int64, root string, priority int64) error {
	return scans.RequestUnder(ctx, nodeID, root, wholeRoot, priority)
}

// MarkStalled is the core's answer to work that stalled waiting on a disk under rootPath (D233 §10):
// the deepest place the scans know that holds waitingOn, a disk mounted inside the root or else the
// root, is unavailable from now, so plays of what is under it fail at once and say so (D241), and a
// scan of the root is asked for, whose reading brings it back and the work parked on it (D231 §6,
// D328 §5). A disk dead inside a root made the whole root unavailable. It returns the place and since
// when; waitingOn empty is the root. A scan already running is asked for again after it ends.
func (scans *Scans) MarkStalled(ctx context.Context, nodeID int64, rootPath, waitingOn string) (
	place string, since time.Time, err error,
) {
	place, err = scans.deepestKnownPlace(ctx, nodeID, rootPath, waitingOn)
	if err != nil {
		return "", time.Time{}, err
	}
	since, err = scans.ingester.MarkPlaceUnavailable(ctx, nodeID, place, rootPath, causeNotAnswering)
	if err != nil {
		return "", time.Time{}, err
	}
	scans.logger.WarnContext(ctx, "work stalled on a disk; it is unavailable until it answers or a scan reads it",
		"nodeId", nodeID, "rootPath", rootPath, "place", place, "since", since)
	return place, since, scans.Request(ctx, nodeID, rootPath, jobs.PriorityPersonsRequest)
}

// deepestKnownPlace is the deepest disk mounted inside rootPath that the scans know and that holds
// waitingOn, or rootPath when none does: found from the scans' record of the mounts, so nothing on
// the dead disk is read to find it (D328 §5).
func (scans *Scans) deepestKnownPlace(ctx context.Context, nodeID int64, rootPath, waitingOn string) (string, error) {
	if waitingOn == "" {
		return rootPath, nil
	}
	mounts, err := scans.ingester.database.Read().ListRootMounts(ctx, sqlcgen.ListRootMountsParams{
		NodeID: nodeID, RootPath: rootPath,
	})
	if err != nil {
		return "", fmt.Errorf("list the disks mounted inside %s: %w", rootPath, err)
	}
	place := rootPath
	for _, mount := range mounts {
		if isAtOrUnder([]byte(waitingOn), mount.MountPath) && len(mount.MountPath) > len(place) {
			place = mount.MountPath
		}
	}
	return place, nil
}

// BringBackRoot is the core's answer to a node's status saying a root's disk, which had stopped
// answering, answers again: the root is available again at once when its disk is the one its last
// complete scan read, and a scan in the background reads what changed meanwhile (D233 §10). It says
// whether it was brought back.
func (scans *Scans) BringBackRoot(
	ctx context.Context, nodeID int64, rootPath string, device uint64, isMountPoint bool,
) (bool, error) {
	isBack, err := scans.ingester.BringBackRoot(ctx, nodeID, rootPath, device, isMountPoint)
	if err != nil || !isBack {
		return false, err
	}
	scans.logger.InfoContext(ctx, "a root's disk answers again; it is available before its scan reads it",
		"nodeId", nodeID, "rootPath", rootPath)
	return true, scans.Request(ctx, nodeID, rootPath, jobs.PriorityBackground)
}

// MarkRootNotNamed makes a library root its node's hello no longer names unavailable from now, unless it
// already was: plays of what is under it fail at once and say so, and nothing is removed on the
// absence; a hello that names it again asks for its scan, which brings it back (D241 §2).
func (scans *Scans) MarkRootNotNamed(ctx context.Context, nodeID int64, rootPath string) error {
	since, err := scans.ingester.MarkPlaceUnavailable(ctx, nodeID, rootPath, rootPath, causeNotNamed)
	if err != nil {
		return err
	}
	scans.logger.WarnContext(ctx, "a node no longer names a library root; it is unavailable until it does",
		"nodeId", nodeID, "rootPath", rootPath, "since", since)
	return nil
}
