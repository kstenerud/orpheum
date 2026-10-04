// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"errors"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// stallOf is what the core is told of an FFmpeg run the watchdog stopped, or nil when err is not
// a stall: a stall says nothing of the file's bytes, apart from FFmpeg failing on them.
func stallOf(err error) *nodeproto.Stall {
	stalled, isStalled := errors.AsType[*execx.StalledError](err)
	if !isStalled {
		return nil
	}
	var cause nodeproto.StallCause
	switch stalled.Cause {
	case execx.StallNeverMoved:
		cause = nodeproto.StallNeverMoved
	case execx.StallStoodStill:
		cause = nodeproto.StallStoodStill
	case execx.StallPastDeadline:
		cause = nodeproto.StallPastDeadline
	}
	return &nodeproto.Stall{
		Cause: cause, Condition: nodeproto.StallCondition(stalled.Condition), IsLeftRunning: stalled.IsLeftRunning,
		WaitingOnPath:  stalled.WaitingOnPath,
		LastPositionMs: stalled.LastPosition.Milliseconds(), WithoutAdvanceMs: stalled.WithoutAdvance.Milliseconds(),
		Detail: err.Error(),
	}
}

// StuckOf is what the node's status says of the processes its runner left running (D233 §10).
func StuckOf(processes []execx.StuckProcess) []nodeproto.Stuck {
	stuck := make([]nodeproto.Stuck, 0, len(processes))
	for _, process := range processes {
		stuck = append(stuck, nodeproto.Stuck{
			Tool: string(process.Tool), PID: process.PID, Paths: process.Reading, Since: process.Since.UnixMilli(),
		})
	}
	return stuck
}

// stallDetail is what a log says of a stall: its detail, or nothing when there was none.
func stallDetail(stall *nodeproto.Stall) string {
	if stall == nil {
		return ""
	}
	return stall.Detail
}
