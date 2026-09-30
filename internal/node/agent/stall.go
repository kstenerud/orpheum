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
		Cause: cause, LastPositionMs: stalled.LastPosition.Milliseconds(),
		WithoutAdvanceMs: stalled.WithoutAdvance.Milliseconds(), Detail: err.Error(),
	}
}

// stallDetail is what a log says of a stall: its detail, or nothing when there was none.
func stallDetail(stall *nodeproto.Stall) string {
	if stall == nil {
		return ""
	}
	return stall.Detail
}
