// SPDX-License-Identifier: AGPL-3.0-or-later

package execx

import (
	"slices"
	"sync"
	"time"
)

// StuckProcess is a tool's process left running after it outlived SIGKILL: waiting on a device,
// which it does until the device answers, if ever (D233 §10).
type StuckProcess struct {
	PID  int
	Tool Tool
	// Reading is the paths the run was allowed to read: where the device it waits on is.
	Reading []string
	// Since is when it was left running.
	Since time.Time
}

// stuckProcesses are a runner's processes left running, until each ends.
type stuckProcesses struct {
	mutex sync.Mutex
	byPID map[int]StuckProcess // guarded by mutex
}

func (stuck *stuckProcesses) add(process StuckProcess) {
	stuck.mutex.Lock()
	defer stuck.mutex.Unlock()
	stuck.byPID[process.PID] = process
}

func (stuck *stuckProcesses) remove(pid int) {
	stuck.mutex.Lock()
	defer stuck.mutex.Unlock()
	delete(stuck.byPID, pid)
}

// Stuck is the runner's processes left running after they outlived their kill, oldest first.
func (runner *Runner) Stuck() []StuckProcess {
	runner.stuck.mutex.Lock()
	defer runner.stuck.mutex.Unlock()
	processes := make([]StuckProcess, 0, len(runner.stuck.byPID))
	for _, process := range runner.stuck.byPID {
		processes = append(processes, process)
	}
	slices.SortFunc(processes, func(a, b StuckProcess) int { return a.Since.Compare(b.Since) })
	return processes
}
