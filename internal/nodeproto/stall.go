// SPDX-License-Identifier: AGPL-3.0-or-later

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

// StallCondition is what a stalled run was doing, as the node read it from /proc (D233 §5).
type StallCondition string

// The conditions of a stalled run.
const (
	// ConditionWaitingOnDisk is a run waiting on a device: a read from a dead disk or a hung network
	// mount, which says the run's root is not answering (D233 §10).
	ConditionWaitingOnDisk StallCondition = "waiting_on_disk"
	// ConditionSpinning is a run that kept a processor busy without moving.
	ConditionSpinning StallCondition = "spinning"
	// ConditionStuck is a run that did nothing.
	ConditionStuck StallCondition = "stuck"
)

// Stall is what the node saw of an FFmpeg run it stopped because the run's position did not
// advance: apart from a failure, which is FFmpeg's word on the bytes, and a refusal, which is the
// node not trying. The node has freed the run's slot.
type Stall struct {
	Cause     StallCause     `json:"cause"`
	Condition StallCondition `json:"condition"`
	// IsLeftRunning is a run whose process outlived its kill, waiting on a device: the node lists
	// it in its status's Stuck until it ends, and it holds no slot.
	IsLeftRunning bool `json:"isLeftRunning,omitempty"`
	// WaitingOnPath is, for a run waiting on its disk, the file it was blocked on, which says
	// whose disk did not answer: its source's, or its prepared directory's; empty when the node
	// could not tell.
	WaitingOnPath string `json:"waitingOnPath,omitempty"`
	// LastPositionMs is the furthest FFmpeg said it had written, in milliseconds of its output.
	LastPositionMs int64 `json:"lastPositionMs"`
	// WithoutAdvanceMs is how long the position had not moved when the run was stopped.
	WithoutAdvanceMs int64 `json:"withoutAdvanceMs"`
	// Detail is the end of FFmpeg's log, for the person reading why.
	Detail string `json:"detail,omitempty"`
}
