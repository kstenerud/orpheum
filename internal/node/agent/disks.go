// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/diskspace"
)

// disks are the filesystems the node's roots and prepared directory are on, each measured now
// (D225 §3), and for a root, the facts the core tells its disk back by (D42, D208).
func (agent *Agent) disks(ctx context.Context) []nodeproto.Disk {
	paths := slices.Clone(agent.hello.RootPaths)
	if agent.workers.PreparedDirectory != "" {
		paths = append(paths, agent.workers.PreparedDirectory)
	}
	disks := make([]nodeproto.Disk, 0, len(paths))
	for _, path := range paths {
		disk := nodeproto.Disk{Path: path}
		// Through the disk gate, so a disk that does not answer leaves the status sent, saying so, where
		// the measure blocked it, and the node fell silent (staging, 2026-10-21).
		measuring := diskwatch.Operation{Root: path, Path: path, Window: measureStallWindow}
		measured, err := diskwatch.Run(ctx, agent.diskGate, measuring,
			func() (diskMeasure, error) { return agent.measureDisk(path) }, func(diskMeasure) {})
		if err != nil {
			disk.Failure = err.Error()
			disk.IsNotAnswering = errors.Is(err, diskwatch.ErrStalled) || diskwatch.IsDiskFailure(err)
		} else {
			disk.TotalBytes, disk.AvailableBytes = measured.space.TotalBytes, measured.space.AvailableBytes
			disk.Device, disk.IsMountPoint = measured.facts.FilesystemDevice, measured.facts.IsMountPoint
		}
		agent.answering.note(ctx, agent, disk)
		disks = append(disks, disk)
	}
	return disks
}

// diskMeasure is what measuring one disk found.
type diskMeasure struct {
	space diskspace.Space
	// facts are a root's; zero for the prepared directory.
	facts confine.Facts
}

// measureDisk measures the disk path is on, and when path is a root, reads its facts.
func (agent *Agent) measureDisk(path string) (diskMeasure, error) {
	space, err := agent.measureSpace(path)
	if err != nil {
		return diskMeasure{}, err
	}
	root, isRoot := agent.roots[path]
	if !isRoot {
		return diskMeasure{space: space}, nil
	}
	facts, err := root.Facts()
	if err != nil {
		return diskMeasure{}, err
	}
	return diskMeasure{space: space, facts: facts}, nil
}

// measureStallWindow is how long measuring a disk's space may take before the status says it is not
// answering: a statfs answers in milliseconds, or from a disk asleep within seconds, and the status
// waits on it.
const measureStallWindow = 5 * time.Second

// diskAnswering is which disks the last status found not answering, so a change is logged once:
// when a share went and when it came back was not in the node's log (staging, 2026-10-21).
type diskAnswering struct {
	mutex        sync.Mutex
	notAnswering map[string]bool // guarded by mutex: by path
}

// note logs a disk that stopped answering, or answers again, since the last status.
func (answering *diskAnswering) note(ctx context.Context, agent *Agent, disk nodeproto.Disk) {
	answering.mutex.Lock()
	defer answering.mutex.Unlock()
	if answering.notAnswering == nil {
		answering.notAnswering = map[string]bool{}
	}
	was := answering.notAnswering[disk.Path]
	switch {
	case disk.IsNotAnswering && !was:
		agent.logger.WarnContext(ctx, "a disk stopped answering", "path", disk.Path, "failure", disk.Failure)
	case !disk.IsNotAnswering && was:
		agent.logger.InfoContext(ctx, "a disk answers again", "path", disk.Path)
	}
	answering.notAnswering[disk.Path] = disk.IsNotAnswering
}
