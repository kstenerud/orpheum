// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// QuickPasses says where scans are still on their quick pass (D255): library.Scans.
type QuickPasses interface {
	// RootsOnQuickPass are a node's roots whose scan has not yet listed them, or not yet read every
	// file its listing found new or changed.
	RootsOnQuickPass(ctx context.Context, nodeID int64) ([]string, error)
}

// SetQuickPasses names where scans are still on their quick pass. Set it before the channel serves.
func (all *Exchanges) SetQuickPasses(passes QuickPasses) {
	all.mutex.Lock()
	defer all.mutex.Unlock()
	all.quickPasses = passes
}

// quickPassRecheckEvery is how often background work waiting for a scan's quick pass asks again
// whether it has ended: a quick pass reads a batch of files in seconds, and a read through a whole
// film takes minutes, so half a minute costs the detailed pass little.
const quickPassRecheckEvery = 30 * time.Second

// readsWholeFiles is whether work reads a video through: a read-through, but one of its index alone,
// or extracting a subtitle from inside one. Such work, in the background, waits for a scan's quick
// pass of its disk (D255).
func readsWholeFiles(work nodeproto.Work) bool {
	return (work.ReadThrough != nil && !work.ReadThrough.IsFromIndexOnly()) ||
		(work.ExtractSubtitle != nil && work.ExtractSubtitle.Video != nil)
}

// parkBehindQuickPass parks background work that reads whole files on a device that a scan is still on
// its quick pass of, so the whole library is listed, identified and playable before any film is read
// through (D255; the owner, 2026-10-07: "It should be doing a quick scan first and then fill in with
// the more detailed scan after"). It waits on a destination of its own, not the device's readers,
// which the scan's own work needs, and asks again every quickPassRecheckEvery. It is nil otherwise.
func (all *Exchanges) parkBehindQuickPass(
	ctx context.Context, nodeID int64, resources Resources, work nodeproto.Work, device string,
) error {
	all.mutex.Lock()
	passes := all.quickPasses
	all.mutex.Unlock()
	if passes == nil || device == "" || !readsWholeFiles(work) {
		return nil
	}
	roots, err := passes.RootsOnQuickPass(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("find the scans on their quick pass: %w", err)
	}
	for _, root := range roots {
		if resources.Devices.KeyOf(root) != device {
			continue
		}
		return jobs.Park(jobs.Parking{
			Destination: jobs.QuickPassDestination(nodeID, device), Reason: "a scan reads the disk's new files first",
			ProbeAt: all.clock.Now().Add(quickPassRecheckEvery),
		}, fmt.Errorf("node %d's scan of %s has not read its new files yet: %w", nodeID, root, errBehindQuickPass))
	}
	return nil
}

// errBehindQuickPass is background work that waits for a scan's quick pass of its disk.
var errBehindQuickPass = errors.New("a scan's quick pass of its disk comes first")
