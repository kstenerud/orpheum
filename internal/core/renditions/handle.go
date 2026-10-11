// SPDX-License-Identifier: AGPL-3.0-or-later

package renditions

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// HandlePrepare runs one prepare job (jobs.KindPrepare): the node that holds the source makes the
// rendition, and this waits for it.
func (service *Service) HandlePrepare(ctx context.Context, job jobs.Job) error {
	rendition, err := store.OrNotFound(service.cache.Read().GetRenditionByID(ctx, job.RenditionID))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.MarkPermanent(fmt.Errorf("no rendition %d", job.RenditionID))
	}
	if err != nil {
		return fmt.Errorf("read rendition %d: %w", job.RenditionID, err)
	}
	order, err := service.prepareOrderFor(ctx, job, rendition)
	if errors.Is(err, library.ErrNoFile) || errors.As(err, new(*library.UnreadableError)) {
		run := renditionRun{fileID: rendition.SourceFile, profile: rendition.Profile, runID: rendition.RunID}
		return jobs.MarkPermanent(errors.Join(err, service.failMaking(ctx, run, err.Error())))
	}
	if err != nil {
		return err
	}
	return service.handle(ctx, job, order)
}

// RecordPrepareFailure records a prepare job that failed for good as its rendition's making failed,
// whatever ended it: a rendition left being made would have its player wait for it until reloaded,
// and a failed one is prepared again by the next play (G8). A rendition gone, or made, needs nothing.
func (service *Service) RecordPrepareFailure(ctx context.Context, job jobs.Job, failure string) error {
	rendition, err := store.OrNotFound(service.cache.Read().GetRenditionByID(ctx, job.RenditionID))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read rendition %d: %w", job.RenditionID, err)
	}
	run := renditionRun{fileID: rendition.SourceFile, profile: rendition.Profile, runID: rendition.RunID}
	return service.failMaking(ctx, run, failure)
}

// prepareOrder is a rendition to have made: the node's command, the node that holds the source, and
// the source's length in milliseconds, or 0 when its probe did not say.
type prepareOrder struct {
	// run is the making the command is for, which its answer is recorded by.
	run        renditionRun
	command    nodeproto.Prepare
	nodeID     int64
	durationMs int64
	// carriedGainsJSON is the gains its audio tracks carry, as JSON, as its rendition records
	// them (D226 §4).
	carriedGainsJSON string
}

// prepareOrderFor is what a rendition's making needs.
func (service *Service) prepareOrderFor(
	ctx context.Context, job jobs.Job, rendition cachegen.PreparedRendition,
) (prepareOrder, error) {
	file, err := library.PlayableFile(ctx, service.database.Read(), rendition.SourceFile)
	if err != nil {
		return prepareOrder{}, err
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return prepareOrder{}, err
	}
	parked := library.ParkIfUnavailable(ctx, service.database.Read(), service.nodes, job, library.LocatedOf(file))
	if parked != nil {
		return prepareOrder{}, parked
	}
	source, err := library.BytesFileOf(ctx, service.database.Read(), library.LocatedOf(file))
	if err != nil {
		return prepareOrder{}, err
	}
	run := renditionRun{fileID: file.ID, profile: rendition.Profile, runID: rendition.RunID}
	command := prepareCommand(source, file.ID, nodeproto.PrepareProfile(rendition.Profile), rendition.RunID, probe)
	command.SegmentLengthSeconds = int(rendition.SegmentLengthSeconds)
	levels, err := service.loudness.Levels(ctx, file.PayloadHash)
	if err != nil {
		return prepareOrder{}, err
	}
	gains := decide.LevelPrepare(&command, levels, probe)
	return prepareOrder{
		run: run, command: command, nodeID: file.NodeID, durationMs: probe.Format.DurationMs, carriedGainsJSON: gains,
	}, nil
}
