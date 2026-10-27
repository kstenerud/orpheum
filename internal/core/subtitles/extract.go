// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// HandleExtractSubtitle runs one extraction job (jobs.KindExtractSubtitle), whose target is a file:
// the node that holds it extracts every text track in it not yet made, a video's from one read of
// it or a subtitle file's whole (D340), and each track's text, or its own failure, is kept by that
// file's payload hash as its answer arrives (record). FFmpeg's failure is not retried.
func (service *Service) HandleExtractSubtitle(ctx context.Context, job jobs.Job) error {
	streams, err := service.database.Read().ListSubtitleStreamsInFile(ctx, job.FileID)
	if err != nil {
		return fmt.Errorf("read the subtitle tracks in file %d: %w", job.FileID, err)
	}
	if len(streams) == 0 {
		return jobs.MarkPermanent(
			fmt.Errorf("no subtitle track is in file %d: it was scanned again or is gone", job.FileID))
	}
	stream := streams[0]
	if stream.PayloadHash == nil {
		return jobs.MarkPermanent(fmt.Errorf("file %d has no payload hash", job.FileID))
	}
	unmade, err := service.tracksWithoutText(ctx, streams)
	if err != nil || len(unmade) == 0 {
		return err // every track's text was kept since the job was queued
	}
	file := library.Located{
		Subject: fmt.Sprintf("file %d", job.FileID), NodeID: stream.NodeID,
		ReportedPathRaw: stream.ReportedPathRaw, PayloadHash: stream.PayloadHash,
	}
	if parked := library.ParkIfUnavailable(ctx, service.database.Read(), service.nodes, job, file); parked != nil {
		return parked
	}
	bytesFile, err := library.BytesFileOf(ctx, service.database.Read(), file)
	if err != nil {
		return jobs.MarkPermanent(err)
	}
	command := &nodeproto.ExtractSubtitle{Sidecar: &bytesFile}
	if unmade[0].StreamIndex != nil {
		command = &nodeproto.ExtractSubtitle{Video: &bytesFile}
		for _, track := range unmade {
			command.StreamIndexes = append(command.StreamIndexes, int(ptr.ValueOrZero(track.StreamIndex)))
		}
	}
	return service.makeSubtitleText(ctx, job, stream.NodeID, command)
}

// tracksWithoutText are the text tracks among streams, all in one file, that have no text kept nor a
// failure: those an extraction of the file makes.
func (service *Service) tracksWithoutText(
	ctx context.Context, streams []sqlcgen.ListSubtitleStreamsInFileRow,
) ([]sqlcgen.ListSubtitleStreamsInFileRow, error) {
	var unmade []sqlcgen.ListSubtitleStreamsInFileRow
	for _, stream := range streams {
		if stream.Codec == nil || !decide.IsTextSubtitle(*stream.Codec) {
			continue
		}
		key := textKey(stream.PayloadHash, stream.StreamIndex)
		_, err := store.OrNotFound(service.database.Read().GetSubtitleText(ctx, key))
		if errors.Is(err, store.ErrNotFound) {
			unmade = append(unmade, stream)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read the text of stream %d: %w", stream.ID, err)
		}
	}
	return unmade, nil
}

// makeSubtitleText gives a node the making of subtitles' text, which says later how it ended:
// its recorder keeps each text where the command's source says (record), and the job is
// resolved by Assignee.
func (service *Service) makeSubtitleText(
	ctx context.Context, job jobs.Job, nodeID int64, command *nodeproto.ExtractSubtitle,
) error {
	if _, err := subtitleTextKeys(*command); err != nil {
		return jobs.MarkPermanent(err)
	}
	return service.exchanges.Assign(ctx, nodeID, job, nodeproto.Command{
		Work: nodeproto.Work{Type: nodeproto.CommandExtractSubtitle, ExtractSubtitle: command},
	})
}

// Assignee is how a subtitle's making given to a node, an extraction or a fetched subtitle's
// conversion, is resolved. FFmpeg's failure is kept and not retried, a video's track's as its own
// (D340), and its job fails; a node's refusal, which says nothing of the source's bytes, is not
// kept, and the job is tried again.
func (service *Service) Assignee(runner nodes.Resolver) nodes.Assignee {
	keepFailure := func(ctx context.Context, attempt nodes.Attempt) func(failure string) error {
		return func(failure string) error {
			keys, err := subtitleTextKeys(ptr.ValueOrZero(attempt.Work.ExtractSubtitle))
			if err != nil {
				return err
			}
			return service.finishSubtitles(ctx, keptAlike(keys, nodeproto.SubtitleText{Failure: failure}))
		}
	}
	return nodes.Assignee{
		Runner: runner,
		Answer: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			about := fmt.Sprintf("%s job %d", job.Kind, job.ID)
			if result.Subtitle == nil {
				return fmt.Errorf("node %d answered %s with something other than a subtitle", attempt.NodeID, about)
			}
			answer := *result.Subtitle
			switch {
			case answer.Refusal != "":
				return nodes.RefusalError(attempt.NodeID, about, answer.Refusal, answer.RefusalCode)
			case answer.Stall != nil:
				return nodes.FFmpegStallOutcome(job, attempt.NodeID, about, *answer.Stall, keepFailure(ctx, attempt))
			case answer.Failure != "":
				return jobs.MarkPermanent(errors.New(answer.Failure))
			case trackFailures(answer.Tracks) != "":
				return jobs.MarkPermanent(errors.New(trackFailures(answer.Tracks)))
			}
			return nil
		},
		Unanswered: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, err error) error {
			return nodes.UnansweredOutcome(job, err, keepFailure(ctx, attempt))
		},
	}
}

// trackFailures are the failures of a video's tracks that could not be made, each by its stream
// index, or "" when every one was made.
func trackFailures(tracks []nodeproto.TrackText) string {
	var failures []string
	for _, track := range tracks {
		if track.Failure != "" {
			failures = append(failures, fmt.Sprintf("stream %d: %s", track.StreamIndex, track.Failure))
		}
	}
	return strings.Join(failures, "; ")
}

// RecordFailure records an extract_subtitle or convert_fetched_subtitle job that failed as the
// failure of each track it was to make and has no text or failure kept (D231 §4, §10, D340): the
// playback shows it failed, and the next play of its video asks again, a play being the request.
func (service *Service) RecordFailure(ctx context.Context, job jobs.Job, failure string) error {
	var keys []sqlcgen.GetSubtitleTextParams
	switch job.Kind {
	case jobs.KindExtractSubtitle:
		streams, err := service.database.Read().ListSubtitleStreamsInFile(ctx, job.FileID)
		if err != nil {
			return fmt.Errorf("read the subtitle tracks in file %d: %w", job.FileID, err)
		}
		if len(streams) == 0 || streams[0].PayloadHash == nil {
			return nil // its file was scanned again or is gone
		}
		unmade, err := service.tracksWithoutText(ctx, streams)
		if err != nil {
			return err
		}
		for _, stream := range unmade {
			keys = append(keys, textKey(stream.PayloadHash, stream.StreamIndex))
		}
	case jobs.KindConvertFetchedSubtitle:
		fetched, err := store.OrNotFound(service.database.Read().GetFetchedSubtitle(ctx, job.FetchedSubtitleID))
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read fetched subtitle %d: %w", job.FetchedSubtitleID, err)
		}
		keys = append(keys, fetchedKey(fetched.BodyGzip))
	case jobs.KindFetchMetadata, jobs.KindMakeArtworkSizes, jobs.KindReadThrough, jobs.KindMakeChunks,
		jobs.KindScan, jobs.KindExamine, jobs.KindBackup, jobs.KindRenewCertificate:
		return fmt.Errorf("job %d: %s is not a subtitle's job", job.ID, job.Kind)
	}
	return service.finishSubtitles(ctx, keptAlike(keys, nodeproto.SubtitleText{Failure: "given up: " + failure}))
}
