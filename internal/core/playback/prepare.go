// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The states of a prepared rendition, as cache.db names them.
const (
	renditionQueued   = "queued"
	renditionRunning  = "running"
	renditionComplete = "complete"
	renditionFailed   = "failed"
)

// prepares has renditions made: it keeps their rows, queues their jobs, and runs each job by
// commanding the node that holds the source and waiting for its reports (cut-a §6).
type prepares struct {
	database   *store.Database
	cache      *store.Cache
	queue      Queue
	nodes      Nodes
	settings   Settings
	random     io.Reader
	clock      clock.Clock
	logger     *slog.Logger
	maxSilence time.Duration

	reportAnswers *answerRouter[nodeproto.PrepareReport]
}

func newPrepares(parts Parts) *prepares {
	return &prepares{
		database: parts.Database, cache: parts.Cache, queue: parts.Queue, nodes: parts.Nodes, settings: parts.Settings,
		random: parts.Random,
		clock:  parts.Clock, logger: parts.Logger, maxSilence: parts.MaxSilence,
		reportAnswers: newAnswerRouter[nodeproto.PrepareReport](parts.Logger, "a prepare report"),
	}
}

// want makes sure a rendition of file in profile is complete or on its way, at the highest
// priority, and returns its row. One whose source changed, that failed, that was made with
// another segment length than hls_segment_length_s now asks for, or whose audio tracks were tagged
// with other languages than the file's probe now gives them, is prepared again: a relabelled track
// keeps the payload hash (D179), and the master playlist names each track's language.
func (all *prepares) want(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, profile nodeproto.PrepareProfile,
) (cachegen.PreparedRendition, error) {
	var row cachegen.PreparedRendition
	segmentLengthS, settingErr := all.settings.Int(ctx, settings.HLSSegmentLengthS)
	if settingErr != nil {
		return row, fmt.Errorf("read hls_segment_length_s: %w", settingErr)
	}
	runID, runErr := all.newRunID()
	if runErr != nil {
		return row, runErr
	}
	probe, probeErr := decodeProbe(file.ProbeJson)
	if probeErr != nil {
		return row, probeErr
	}
	languages := audioLanguagesOf(preparedAudio(probe))
	err := all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		key := cachegen.GetRenditionOfFileAndProfileParams{SourceFile: file.ID, Profile: string(profile)}
		existing, err := store.OrNotFound(queries.GetRenditionOfFileAndProfile(ctx, key))
		switch {
		case errors.Is(err, store.ErrNotFound):
			_, err = queries.InsertRendition(ctx, cachegen.InsertRenditionParams{
				SourceFile: file.ID, SourcePayloadHash: file.PayloadHash, Profile: string(profile), NodeID: file.NodeID,
				SegmentLengthSeconds: segmentLengthS, RunID: runID, CreatedAt: all.clock.Now().UnixMilli(),
			})
		case err != nil:
		case !bytes.Equal(existing.SourcePayloadHash, file.PayloadHash) || existing.State == renditionFailed ||
			existing.SegmentLengthSeconds != segmentLengthS ||
			existing.State == renditionComplete && existing.AudioLanguagesJson != languages:
			err = queries.RestartRendition(ctx, cachegen.RestartRenditionParams{
				SourcePayloadHash: file.PayloadHash, NodeID: file.NodeID, SegmentLengthSeconds: segmentLengthS,
				RunID: runID,
				ID:    existing.ID,
			})
		}
		if err != nil {
			return fmt.Errorf("record the rendition: %w", err)
		}
		row, err = queries.GetRenditionOfFileAndProfile(ctx, key)
		return err
	})
	if err != nil {
		return row, fmt.Errorf("want a rendition of file %d: %w", file.ID, err)
	}
	if row.State != renditionComplete {
		_, err = all.queue.Enqueue(ctx, jobs.Request{
			Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: row.ID}, Priority: jobs.PriorityPlayNow,
			Reason: "someone is waiting to watch it",
		})
	}
	return row, err
}

// redo prepares a rendition again from the start, whatever state its row is in.
func (all *prepares) redo(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, profile nodeproto.PrepareProfile,
) error {
	runID, err := all.newRunID()
	if err != nil {
		return err
	}
	err = all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		key := cachegen.GetRenditionOfFileAndProfileParams{SourceFile: file.ID, Profile: string(profile)}
		existing, readErr := queries.GetRenditionOfFileAndProfile(ctx, key)
		if readErr != nil {
			return readErr
		}
		return queries.RestartRendition(ctx, cachegen.RestartRenditionParams{
			SourcePayloadHash: file.PayloadHash, NodeID: file.NodeID,
			SegmentLengthSeconds: existing.SegmentLengthSeconds,
			RunID:                runID,
			ID:                   existing.ID,
		})
	})
	if err != nil {
		return fmt.Errorf("prepare file %d again: %w", file.ID, err)
	}
	_, err = all.want(ctx, file, profile)
	return err
}

// runIDBytes is a run id's random name's length before hex: eight bytes never repeat in practice,
// even across a cache.db rebuilt from nothing.
const runIDBytes = 8

// newRunID names one making of a rendition: its own directory on the node, so no request can
// reach a copy that an earlier making left and a later one is overwriting.
func (all *prepares) newRunID() (string, error) {
	raw, err := randbytes.Read(all.random, runIDBytes)
	if err != nil {
		return "", fmt.Errorf("name a rendition's run: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// handle runs one prepare job: it commands the node and waits for its reports, recording each,
// until the node says the rendition is done or failed, or says nothing for too long.
func (all *prepares) handle(ctx context.Context, job jobs.Job, order prepareOrder) error {
	command, nodeID := order.command, order.nodeID
	answers := all.reportAnswers.await(job.ID)
	defer all.reportAnswers.forget(job.ID)
	if err := all.recordRunning(ctx, job.RenditionID, 0); err != nil {
		return err
	}
	languages := audioLanguagesOf(command.AudioTracks)
	if err := all.recordAudio(ctx, job.RenditionID, order.carriedGainsJSON, languages); err != nil {
		return err
	}
	command.JobID = job.ID
	all.nodes.Enqueue(nodeID, nodeproto.Command{Type: nodeproto.CommandPrepare, Prepare: &command})
	timer := time.NewTimer(all.maxSilence)
	defer timer.Stop()
	for {
		select {
		case report := <-answers:
			switch report.State {
			case nodeproto.PrepareWaiting:
				timer.Reset(all.maxSilence) // the node holds it, waiting for a slot of its kind
			case nodeproto.PrepareRunning:
				if err := all.recordProgress(ctx, job, report.PreparedThroughMs, order.durationMs); err != nil {
					return err
				}
				timer.Reset(all.maxSilence)
			case nodeproto.PrepareDone:
				return all.finish(ctx, job.RenditionID, &report, "")
			case nodeproto.PrepareFailed:
				// FFmpeg failing on a file fails again on it: this is not retried.
				failure := errors.New(report.Failure)
				return jobs.MarkPermanent(errors.Join(failure, all.finish(ctx, job.RenditionID, nil, report.Failure)))
			}
		case <-timer.C:
			return fmt.Errorf("node %d said nothing about prepare %s for %s",
				nodeID, command.RenditionDir, all.maxSilence)
		case <-ctx.Done():
			if errors.Is(context.Cause(ctx), jobs.ErrCancelled) {
				return all.abandon(ctx, job, nodeID)
			}
			return fmt.Errorf("wait for prepare %s: %w", command.RenditionDir, ctx.Err())
		}
	}
}

// recordProgress records how far a running prepare is, on its rendition and, when the source's length
// is known, on its job for the job view.
func (all *prepares) recordProgress(ctx context.Context, job jobs.Job, preparedThroughMs, durationMs int64) error {
	if err := all.recordRunning(ctx, job.RenditionID, preparedThroughMs); err != nil {
		return err
	}
	if durationMs <= 0 {
		return nil
	}
	return all.queue.SetProgress(ctx, job.ID, min(float64(preparedThroughMs)/float64(durationMs), 1))
}

// abandon ends a cancelled prepare: the node stops its FFmpeg, and the rendition is not made, so
// a later play prepares it again rather than waiting on one no one is making.
func (all *prepares) abandon(ctx context.Context, job jobs.Job, nodeID int64) error {
	all.nodes.Enqueue(nodeID, cancelCommand(job.ID))
	finishErr := all.finish(context.WithoutCancel(ctx), job.RenditionID, nil, jobs.ErrCancelled.Error())
	return jobs.MarkPermanent(errors.Join(jobs.ErrCancelled, finishErr))
}

// recordAudio records what a making puts in the rendition's audio tracks: the gains in their
// sound (D226 §4) and the languages they are tagged with.
func (all *prepares) recordAudio(ctx context.Context, renditionID int64, gainsJSON, languagesJSON string) error {
	err := all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.SetRenditionAudio(ctx, cachegen.SetRenditionAudioParams{
			CarriedGainsDbJson: gainsJSON, AudioLanguagesJson: languagesJSON,
			ID: renditionID,
		})
	})
	if err != nil {
		return fmt.Errorf("record rendition %d's audio: %w", renditionID, err)
	}
	return nil
}

func (all *prepares) recordRunning(ctx context.Context, renditionID, preparedThroughMs int64) error {
	err := all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		progress := cachegen.SetRenditionRunningParams{PreparedThroughMs: preparedThroughMs, ID: renditionID}
		return queries.SetRenditionRunning(ctx, progress)
	})
	if err != nil {
		return fmt.Errorf("record rendition %d running: %w", renditionID, err)
	}
	return nil
}

// finish records a rendition complete with its size, what made its video and how fast, or
// failed with why.
func (all *prepares) finish(
	ctx context.Context, renditionID int64, doneReport *nodeproto.PrepareReport, failure string,
) error {
	now := all.clock.Now().UnixMilli()
	err := all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if doneReport != nil {
			// What made it and how fast, for the System page (D225 §3); a node that did not say
			// leaves them empty.
			var videoEncoder *string
			if doneReport.VideoEncoder != "" {
				videoEncoder = &doneReport.VideoEncoder
			}
			var realtimeFactor *float64
			if doneReport.RealtimeFactor > 0 {
				realtimeFactor = &doneReport.RealtimeFactor
			}
			return queries.SetRenditionComplete(ctx, cachegen.SetRenditionCompleteParams{
				SizeBytes: &doneReport.SizeBytes, FinishedAt: &now, VideoEncoder: videoEncoder,
				RealtimeFactor: realtimeFactor,
				ID:             renditionID,
			})
		}
		return queries.SetRenditionFailed(ctx, cachegen.SetRenditionFailedParams{
			Failure: &failure, FinishedAt: &now, ID: renditionID,
		})
	})
	if err != nil {
		return fmt.Errorf("record rendition %d finished: %w", renditionID, err)
	}
	return nil
}

// reported hands a node's report to the job waiting for it.
func (all *prepares) reported(ctx context.Context, nodeID int64, report nodeproto.PrepareReport) {
	all.reportAnswers.deliver(ctx, nodeID, report.JobID, report)
}

// prepareCommand is the node's command for a rendition of file: its source by identity, its
// directory, its picture's codec and every audio track (D217 §3).
func prepareCommand(
	source nodeproto.BytesFile, fileID int64, profile nodeproto.PrepareProfile, runID string, probe nodeproto.Probe,
) nodeproto.Prepare {
	command := nodeproto.Prepare{
		Video: source, Profile: profile,
		RenditionDir: strconv.FormatInt(fileID, 10) + "/" + string(profile) + "/" + runID,
	}
	if picture, found := decide.MainVideo(probe); found {
		command.VideoStreamIndex, command.VideoCodec = picture.StreamIndex, picture.Codec
	}
	command.AudioTracks = preparedAudio(probe)
	return command
}

// preparedAudio are a source's audio tracks as a prepare carries them: each in its place, tagged
// with its language, "und" when the probe gave none.
func preparedAudio(probe nodeproto.Probe) []nodeproto.PrepareAudio {
	var tracks []nodeproto.PrepareAudio
	for index, stream := range audioStreams(probe) {
		language := stream.Language
		if language == "" {
			language = "und"
		}
		tracks = append(tracks, nodeproto.PrepareAudio{
			AudioTrackIndex: index, Language: language, ChannelCount: stream.Audio.ChannelCount,
		})
	}
	return tracks
}

// audioLanguagesOf are the languages a prepare tags its audio tracks with, by their place, as a
// rendition records them.
func audioLanguagesOf(tracks []nodeproto.PrepareAudio) string {
	languages := make([]string, 0, len(tracks))
	for _, track := range tracks {
		languages = append(languages, track.Language)
	}
	encoded, err := json.Marshal(languages)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}
