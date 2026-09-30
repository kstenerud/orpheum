// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// stalledTwice begins the failure kept for a target whose FFmpeg stalled on two attempts in a row.
const stalledTwice = "FFmpeg stalled twice: "

// stallOutcome is a handler's answer to a node's word that the FFmpeg working on job stalled and
// was stopped. A hang says nothing of the target the first time, so the job is tried again at no
// cost of an attempt (jobs.MarkStalled); when its last attempt stalled too, the stall is the
// target's failure, which keepFailure keeps, and the job fails. D231's build replaces this with
// an item fact counted apart from the job's attempts.
func stallOutcome(
	job jobs.Job, nodeID int64, what string, stall nodeproto.Stall, keepFailure func(failure string) error,
) error {
	stalled := fmt.Errorf("node %d stopped the FFmpeg of %s: %s at %d ms of its output, %d ms without moving: %s",
		nodeID, what, stall.Cause, stall.LastPositionMs, stall.WithoutAdvanceMs, stall.Detail)
	if !job.PreviousAttemptStalled {
		return jobs.MarkStalled(stalled)
	}
	failure := stalledTwice + stalled.Error()
	return jobs.MarkPermanent(errors.Join(errors.New(failure), keepFailure(failure)))
}
