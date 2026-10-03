// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// stalledTwice begins the failure kept for a target whose FFmpeg stalled on two attempts in a row.
const stalledTwice = "FFmpeg stalled twice: "

// stallOutcome is a handler's answer to a node's word that the FFmpeg working on job stalled and
// was stopped: nodes.StallOutcome, with what the node saw.
func stallOutcome(
	job jobs.Job, nodeID int64, what string, stall nodeproto.Stall, keepFailure func(failure string) error,
) error {
	return nodes.StallOutcome(job, nodes.StallError(nodeID, "the FFmpeg of "+what, stall), stalledTwice, keepFailure)
}

// refusalError is a node's refusal of work it did not try, which says nothing of the source and is
// tried again on the job's back-off. A file changed since the scan says so: the scan that sees the
// change ends the old file's jobs (S74).
func refusalError(nodeID int64, what, refusal string, code nodeproto.RefusalCode) error {
	if code == nodeproto.RefusedChanged {
		return fmt.Errorf("node %d refused %s: its file changed since the scan, which the next scan "+
			"of its root ends the job for: %s", nodeID, what, refusal)
	}
	return fmt.Errorf("node %d refused %s: %s", nodeID, what, refusal)
}
