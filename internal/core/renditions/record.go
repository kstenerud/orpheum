// SPDX-License-Identifier: GPL-3.0-or-later

package renditions

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// recordPrepare records a making of a rendition done, or failed with FFmpeg's word on its source,
// by its run (D238 §2), as it arrives, whether or not its job still waits, before the node is told
// it was taken (S81); a rendition made again since, under another run, is not touched (G13).
func (service *Service) recordPrepare(ctx context.Context, nodeID int64, result nodeproto.Result) error {
	report, command := result.Prepare, result.Work.Prepare
	if report == nil || command == nil {
		return fmt.Errorf("a prepare's result carries no report, or names no prepare: %w", nodes.ErrMalformedResult)
	}
	var done *nodeproto.PrepareReport
	switch report.State {
	case nodeproto.PrepareDone:
		done = report
	case nodeproto.PrepareFailed:
	case nodeproto.PrepareRefused, nodeproto.PrepareStalled:
		return nil // says nothing of the source, and the job decides
	}
	run, err := parseRenditionDir(command.RenditionDir)
	if err != nil {
		return fmt.Errorf("%w: %w", nodes.ErrMalformedResult, err)
	}
	isCurrent, err := service.finish(ctx, run, done, report.Failure)
	if err == nil && !isCurrent {
		service.logger.InfoContext(ctx, "a node's answer on a making of a rendition no longer current was not recorded",
			"nodeId", nodeID, "jobId", result.JobID, "renditionDir", command.RenditionDir)
	}
	return err
}
