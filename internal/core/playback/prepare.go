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
func (all *prepares) handle(ctx context.Context, job jobs.Job, command nodeproto.Prepare, nodeID int64) error {
	answers := all.waiting.await(job.ID)
	defer all.waiting.forget(job.ID)
	if err := all.running(ctx, job.Subject, 0); err != nil {
		return err
	}
	command.Job = job.ID
	all.nodes.Enqueue(nodeID, nodeproto.Command{Type: nodeproto.CommandPrepare, Prepare: &command})
	timer := time.NewTimer(all.maxSilence)
	defer timer.Stop()
	for {
		select {
		case report := <-answers:
			switch report.State {
			case nodeproto.PrepareRunning:
				if err := all.running(ctx, job.Subject, report.PreparedMs); err != nil {
					return err
				}
				timer.Reset(all.maxSilence)
			case nodeproto.PrepareDone:
				return all.finish(ctx, job.Subject, &report.SizeBytes, "")
			case nodeproto.PrepareFailed:
				// FFmpeg failing on a file fails again on it: this is not retried.
				failure := errors.New(report.Error)
				return jobs.Permanent(errors.Join(failure, all.finish(ctx, job.Subject, nil, report.Error)))
			}
		case <-timer.C:
			return fmt.Errorf("node %d said nothing about prepare %s for %s",
				nodeID, command.RenditionDir, all.maxSilence)
		case <-ctx.Done():
			return fmt.Errorf("wait for prepare %s: %w", command.RenditionDir, ctx.Err())
		}
	}
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

// finish records a rendition complete with its size, or failed with why.
func (all *prepares) finish(ctx context.Context, rendition int64, size *int64, failure string) error {
	now := all.clock.Now().UnixMilli()
	err := all.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if size != nil {
			return queries.SetRenditionComplete(ctx, cachegen.SetRenditionCompleteParams{
				SizeBytes: size, FinishedAt: &now, ID: rendition,
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
	all.waiting.deliver(ctx, nodeID, report.Job, report)
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
		command.VideoStreamIndex, command.VideoCodec = picture.Index, picture.Codec
	}
	index := 0
	for _, stream := range probe.Streams {
		if stream.Kind != "audio" {
			continue
		}
		language := stream.LanguageTag
		if language == "" {
			language = "und"
		}
		command.Audio = append(command.Audio, nodeproto.PrepareAudio{
			AudioTrackIndex: index, Language: language, Channels: stream.Audio.Channels,
		})
		index++
	}
	return command
}
