// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"bytes"
	"context"
	"encoding/hex"
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

	waiting *waiting[nodeproto.PrepareReport]
}

func newPrepares(parts Parts) *prepares {
	return &prepares{
		database: parts.Database, cache: parts.Cache, queue: parts.Queue, nodes: parts.Nodes, settings: parts.Settings,
		random: parts.Random,
		clock:  parts.Clock, logger: parts.Logger, maxSilence: parts.MaxSilence,
		waiting: newWaiting[nodeproto.PrepareReport](parts.Logger, "a prepare report"),
	}
}

// want makes sure a rendition of file in profile is complete or on its way, at the highest
// priority, and returns its row. One whose source changed, that failed, or that was made with
// another segment length than hls_segment_length_s now asks for, is prepared again.
func (all *prepares) want(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, profile nodeproto.PrepareProfile,
) (cachegen.PreparedRendition, error) {
	var row cachegen.PreparedRendition
	segmentLength, settingErr := all.settings.Int(ctx, settings.HLSSegmentLengthS)
	if settingErr != nil {
		return row, fmt.Errorf("read hls_segment_length_s: %w", settingErr)
	}
	runID, runErr := all.newRunID()
	if runErr != nil {
		return row, runErr
	}
	err := all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		key := cachegen.GetRenditionParams{SourceFile: file.ID, Profile: string(profile)}
		found, err := store.One(queries.GetRendition(ctx, key))
		switch {
		case errors.Is(err, store.ErrNotFound):
			_, err = queries.InsertRendition(ctx, cachegen.InsertRenditionParams{
				SourceFile: file.ID, PayloadHash: file.PayloadHash, Profile: string(profile), NodeID: file.NodeID,
				SegmentLengthS: segmentLength, RunID: runID, CreatedAt: all.clock.Now().UnixMilli(),
			})
		case err != nil:
		case !bytes.Equal(found.PayloadHash, file.PayloadHash) || found.State == renditionFailed ||
			found.SegmentLengthS != segmentLength:
			err = queries.RestartRendition(ctx, cachegen.RestartRenditionParams{
				PayloadHash: file.PayloadHash, NodeID: file.NodeID, SegmentLengthS: segmentLength, RunID: runID,
				ID: found.ID,
			})
		}
		if err != nil {
			return fmt.Errorf("record the rendition: %w", err)
		}
		row, err = queries.GetRendition(ctx, key)
		return err
	})
	if err != nil {
		return row, fmt.Errorf("want a rendition of file %d: %w", file.ID, err)
	}
	if row.State != renditionComplete {
		_, err = all.queue.Enqueue(ctx, jobs.New{
			Kind: jobs.KindPrepare, Subject: row.ID, Priority: jobs.PriorityPlayNow,
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
		key := cachegen.GetRenditionParams{SourceFile: file.ID, Profile: string(profile)}
		found, readErr := queries.GetRendition(ctx, key)
		if readErr != nil {
			return readErr
		}
		return queries.RestartRendition(ctx, cachegen.RestartRenditionParams{
			PayloadHash: file.PayloadHash, NodeID: file.NodeID, SegmentLengthS: found.SegmentLengthS, RunID: runID,
			ID: found.ID,
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
	raw := make([]byte, runIDBytes)
	if _, err := io.ReadFull(all.random, raw); err != nil {
		return "", fmt.Errorf("name a rendition's run: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// handle runs one prepare job: it commands the node and waits for its reports, recording each,
// until the node says the rendition is done or failed, or says nothing for too long.
func (all *prepares) handle(ctx context.Context, job jobs.Job, made making) error {
	command, nodeID := made.command, made.nodeID
	answers := all.waiting.await(job.ID)
	defer all.waiting.forget(job.ID)
	if err := all.running(ctx, job.Subject, 0); err != nil {
		return err
	}
	if err := all.carrying(ctx, job.Subject, made.gains); err != nil {
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
				if err := all.progressed(ctx, job, report.PreparedMs, made.durationMs); err != nil {
					return err
				}
				timer.Reset(all.maxSilence)
			case nodeproto.PrepareDone:
				return all.finish(ctx, job.Subject, &report, "")
			case nodeproto.PrepareFailed:
				// FFmpeg failing on a file fails again on it: this is not retried.
				failure := errors.New(report.Failure)
				return jobs.Permanent(errors.Join(failure, all.finish(ctx, job.Subject, nil, report.Failure)))
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

// progressed records how far a running prepare is, on its rendition and, when the source's length
// is known, on its job for the job view.
func (all *prepares) progressed(ctx context.Context, job jobs.Job, preparedMs, durationMs int64) error {
	if err := all.running(ctx, job.Subject, preparedMs); err != nil {
		return err
	}
	if durationMs <= 0 {
		return nil
	}
	return all.queue.SetProgress(ctx, job.ID, min(float64(preparedMs)/float64(durationMs), 1))
}

// abandon ends a cancelled prepare: the node stops its FFmpeg, and the rendition is not made, so
// a later play prepares it again rather than waiting on one no one is making.
func (all *prepares) abandon(ctx context.Context, job jobs.Job, nodeID int64) error {
	all.nodes.Enqueue(nodeID, cancelCommand(job.ID))
	failed := all.finish(context.WithoutCancel(ctx), job.Subject, nil, jobs.ErrCancelled.Error())
	return jobs.Permanent(errors.Join(jobs.ErrCancelled, failed))
}

// carrying records the gains a making puts in the rendition's audio tracks (D226 §4).
func (all *prepares) carrying(ctx context.Context, rendition int64, gains string) error {
	err := all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.SetRenditionGains(ctx, cachegen.SetRenditionGainsParams{AudioGainsDb: gains, ID: rendition})
	})
	if err != nil {
		return fmt.Errorf("record rendition %d's gains: %w", rendition, err)
	}
	return nil
}

func (all *prepares) running(ctx context.Context, rendition, preparedMs int64) error {
	err := all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		progress := cachegen.SetRenditionRunningParams{PreparedMs: preparedMs, ID: rendition}
		return queries.SetRenditionRunning(ctx, progress)
	})
	if err != nil {
		return fmt.Errorf("record rendition %d running: %w", rendition, err)
	}
	return nil
}

// finish records a rendition complete with its size, what made its video and how fast, or
// failed with why.
func (all *prepares) finish(ctx context.Context, rendition int64, done *nodeproto.PrepareReport, failure string) error {
	now := all.clock.Now().UnixMilli()
	err := all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if done != nil {
			// What made it and how fast, for the System page (D225 §3); a node that did not say
			// leaves them empty.
			var videoEncoder *string
			if done.VideoEncoder != "" {
				videoEncoder = &done.VideoEncoder
			}
			var speed *float64
			if done.RealtimeFactor > 0 {
				speed = &done.RealtimeFactor
			}
			return queries.SetRenditionComplete(ctx, cachegen.SetRenditionCompleteParams{
				SizeBytes: &done.SizeBytes, FinishedAt: &now, VideoEncoder: videoEncoder, Speed: speed, ID: rendition,
			})
		}
		return queries.SetRenditionFailed(ctx, cachegen.SetRenditionFailedParams{
			Error: &failure, FinishedAt: &now, ID: rendition,
		})
	})
	if err != nil {
		return fmt.Errorf("record rendition %d finished: %w", rendition, err)
	}
	return nil
}

// reported hands a node's report to the job waiting for it.
func (all *prepares) reported(ctx context.Context, nodeID int64, report nodeproto.PrepareReport) {
	all.waiting.deliver(ctx, nodeID, report.JobID, report)
}

// prepareCommand is the node's command for a rendition of file: its source by identity, its
// directory, its picture's codec and every audio track (D217 §3).
func prepareCommand(
	source nodeproto.BytesFile, fileID int64, profile nodeproto.PrepareProfile, runID string, probe nodeproto.Probe,
) nodeproto.Prepare {
	command := nodeproto.Prepare{
		Source: source, Profile: profile,
		RenditionDir: strconv.FormatInt(fileID, 10) + "/" + string(profile) + "/" + runID,
	}
	if picture, found := decide.MainVideo(probe); found {
		command.VideoStreamIndex, command.VideoCodec = picture.StreamIndex, picture.Codec
	}
	for index, stream := range audioStreams(probe) {
		language := stream.LanguageTag
		if language == "" {
			language = "und"
		}
		command.Audio = append(command.Audio, nodeproto.PrepareAudio{
			AudioTrackIndex: index, LanguageTag: language, ChannelCount: stream.Audio.ChannelCount,
		})
	}
	return command
}
