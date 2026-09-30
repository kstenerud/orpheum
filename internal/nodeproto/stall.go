// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// StallCause is why the node stopped an FFmpeg run as stalled.
type StallCause string

// The causes of a stall.
const (
	// StallNeverMoved is a run whose position did not move within its start grace.
	StallNeverMoved StallCause = "never_moved"
	// StallStoodStill is a run whose position stood still for its stall window.
	StallStoodStill StallCause = "stood_still"
	// StallPastDeadline is a run that moved but did not end by its deadline.
	StallPastDeadline StallCause = "past_deadline"
)

// Stall is what the node saw of an FFmpeg run it stopped because the run's position did not
// advance: apart from a failure, which is FFmpeg's word on the bytes, and a refusal, which is the
// node not trying. The node has freed the run's slot.
type Stall struct {
	Cause StallCause `json:"cause"`
	// LastPositionMs is the furthest FFmpeg said it had written, in milliseconds of its output.
	LastPositionMs int64 `json:"lastPositionMs"`
	// WithoutAdvanceMs is how long the position had not moved when the run was stopped.
	WithoutAdvanceMs int64 `json:"withoutAdvanceMs"`
	// Detail is the end of FFmpeg's log, for the person reading why.
	Detail string `json:"detail,omitempty"`
}
