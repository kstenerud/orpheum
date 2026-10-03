// SPDX-License-Identifier: GPL-3.0-or-later

package renditions

import (
	"bytes"
	"cmp"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/langtag"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
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

// want makes sure a rendition of file in profile is complete or on its way, at the highest
// priority, and returns its row. One whose source changed, that failed, that was made with
// another segment length than hls_segment_length_seconds now asks for, or whose audio tracks were tagged
// with other languages than the file's probe now gives them, is prepared again: a relabelled track
// keeps the payload hash (D179), and the master playlist names each track's language.
func (service *Service) want(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, profile nodeproto.PrepareProfile,
) (cachegen.PreparedRendition, error) {
	var row cachegen.PreparedRendition
	segmentLengthS, settingErr := service.settings.Int(ctx, settings.HLSSegmentLengthSeconds)
	if settingErr != nil {
		return row, fmt.Errorf("read hls_segment_length_seconds: %w", settingErr)
	}
	runID, runErr := service.newRunID()
	if runErr != nil {
		return row, runErr
	}
	probe, probeErr := library.ProbeOf(file.ProbeJSON)
	if probeErr != nil {
		return row, probeErr
	}
	languages := audioLanguagesOf(preparedAudio(probe))
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		key := cachegen.GetRenditionOfFileAndProfileParams{SourceFile: file.ID, Profile: string(profile)}
		existing, err := store.OrNotFound(queries.GetRenditionOfFileAndProfile(ctx, key))
		switch {
		case errors.Is(err, store.ErrNotFound):
			_, err = queries.InsertRendition(ctx, cachegen.InsertRenditionParams{
				SourceFile: file.ID, SourcePayloadHash: file.PayloadHash, Profile: string(profile), NodeID: file.NodeID,
				SegmentLengthSeconds: segmentLengthS, RunID: runID, CreatedAt: service.clock.Now().UnixMilli(),
			})
		case err != nil:
		case !bytes.Equal(existing.SourcePayloadHash, file.PayloadHash) || existing.State == renditionFailed ||
			existing.SegmentLengthSeconds != segmentLengthS ||
			existing.State == renditionComplete && existing.AudioLanguagesJSON != languages:
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
		// The run its job makes is in its arguments: a making restarted while an earlier run's job is
		// with its node is not that job's, so it is queued after it rather than dropped (G13).
		args, argsErr := json.Marshal(prepareArgs{RunID: row.RunID})
		if argsErr != nil {
			return row, fmt.Errorf("encode a prepare's arguments: %w", argsErr)
		}
		_, err = service.queue.Enqueue(ctx, jobs.Request{
			Kind: jobs.KindPrepare, Target: jobs.Target{RenditionID: row.ID}, Priority: jobs.PriorityPlayNow,
			ArgsJSON: string(args),
			Reason:   "someone is waiting to watch it", Destination: jobs.NodeDestination(file.NodeID),
		})
	}
	return row, err
}

// prepareArgs are a prepare job's arguments: the run of the rendition it makes.
type prepareArgs struct {
	RunID string `json:"runId"`
}

// redo prepares a rendition again from the start, whatever state its row is in.
func (service *Service) redo(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, profile nodeproto.PrepareProfile,
) error {
	runID, err := service.newRunID()
	if err != nil {
		return err
	}
	err = service.cache.Write(ctx, func(queries *cachegen.Queries) error {
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
	_, err = service.want(ctx, file, profile)
	return err
}

// runIDBytes is a run id's random name's length before hex: eight bytes never repeat in practice,
// even across a cache.db rebuilt from nothing.
const runIDBytes = 8

// newRunID names one making of a rendition: its own directory on the node, so no request can
// reach a copy that an earlier making left and a later one is overwriting.
func (service *Service) newRunID() (string, error) {
	raw, err := randbytes.Read(service.random, runIDBytes)
	if err != nil {
		return "", fmt.Errorf("name a rendition's run: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// handle runs one prepare job: it records the making begun and gives it to the node, which says
// later how it ended (D234 §7, Assignee).
func (service *Service) handle(ctx context.Context, job jobs.Job, order prepareOrder) error {
	command := order.command
	answer := service.exchanges.Assign(ctx, order.nodeID, job, nodeproto.Command{
		Work: nodeproto.Work{Type: nodeproto.CommandPrepare, Prepare: &command},
	})
	// A making begins only once its job is given to the node: one parked waiting for the node, or for
	// room on it, has not begun. The node is sent it after the handler answers.
	if !jobs.IsAssigned(answer) {
		return answer
	}
	if err := service.recordRunning(ctx, order.run, 0); err != nil {
		return err
	}
	languages := audioLanguagesOf(command.AudioTracks)
	if err := service.recordAudio(ctx, job.RenditionID, order.carriedGainsJSON, languages); err != nil {
		return err
	}
	return answer
}

// Assignee is how a prepare given to a node is resolved: by its node's word on how it ended, by its
// silence or restart, by its progress, and by a cancel.
func (service *Service) Assignee(runner nodes.Resolver) nodes.Assignee {
	return nodes.Assignee{
		Runner: runner,
		Answer: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			run, err := runOf(attempt)
			if err != nil {
				return jobs.MarkPermanent(err)
			}
			if result.Prepare == nil {
				return fmt.Errorf("node %d answered prepare %s with something other than a prepare report",
					attempt.NodeID, run.dir())
			}
			return service.takeEnd(ctx, job, attempt.NodeID, run, *result.Prepare)
		},
		Unanswered: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, err error) error {
			run, runErr := runOf(attempt)
			if runErr != nil {
				return jobs.MarkPermanent(runErr)
			}
			return nodes.UnansweredOutcome(job, err, func(failure string) error {
				return service.failMaking(ctx, run, failure)
			})
		},
		Progress: service.takeProgress,
		Cancelled: func(ctx context.Context, attempt nodes.Attempt) error {
			// The rendition is not made, so a later play prepares it again rather than waiting on one
			// no one is making.
			run, err := runOf(attempt)
			if err != nil {
				return err
			}
			return service.failMaking(ctx, run, jobs.ErrCancelled.Error())
		},
	}
}

// runOf is the making an attempt at a prepare is.
func runOf(attempt nodes.Attempt) (renditionRun, error) {
	if attempt.Work.Prepare == nil {
		return renditionRun{}, fmt.Errorf("%s job %d's attempt names no prepare", attempt.Kind, attempt.JobID)
	}
	return parseRenditionDir(attempt.Work.Prepare.RenditionDir)
}

// takeProgress takes the node's word on where a prepare is. Waiting for a slot of its kind, or
// paused for play-driven work, is a word about the node, which keeps the job from being given up for
// silence; how long it may wait is the core's to bound by the node's word on whether the work ahead
// of it moves (D233 §5).
func (service *Service) takeProgress(ctx context.Context, attempt nodes.Attempt, progress nodeproto.Progress) error {
	run, err := runOf(attempt)
	if err != nil {
		return err
	}
	switch progress.State {
	case nodeproto.ProgressWaiting, nodeproto.ProgressPaused:
		return nil
	case nodeproto.ProgressRunning:
		return service.recordProgress(ctx, attempt.JobID, run, progress.PreparedThroughMs,
			attempt.Work.Prepare.SourceDurationMs)
	}
	return nil
}

// takeEnd takes the node's word on how a prepare ended, and says how the job ended. What the word
// establishes, the making done or FFmpeg failing on its source, its recorder recorded before the
// job heard it (recordPrepare).
func (service *Service) takeEnd(
	ctx context.Context, job jobs.Job, nodeID int64, run renditionRun, report nodeproto.PrepareReport,
) error {
	renditionDir := run.dir()
	switch report.State {
	case nodeproto.PrepareDone:
		return nil
	case nodeproto.PrepareFailed:
		// FFmpeg failing on a file fails again on it: this is not retried.
		return jobs.MarkPermanent(errors.New(report.Failure))
	case nodeproto.PrepareStalled:
		return nodes.FFmpegStallOutcome(job, nodeID, "prepare "+renditionDir, ptr.ValueOrZero(report.Stall),
			func(failure string) error { return service.failMaking(ctx, run, failure) })
	case nodeproto.PrepareRefused:
		refused := nodes.RefusalError(nodeID, "prepare "+renditionDir, report.Refusal, report.RefusalCode)
		if report.RefusalCode == nodeproto.RefusedNoRoom {
			// Its prepared directory's disk is full: the node's prepares wait until its status
			// shows room (D231 §5).
			return jobs.Park(jobs.Parking{
				Destination: jobs.PreparedDirectoryDestination(nodeID), Reason: "its disk has no room",
				AwaitsSignal: true,
			}, refused)
		}
		// Any other refusal says nothing of the source's bytes (a file changed since the scan):
		// the job is tried again, as when the node says nothing.
		return refused
	}
	return fmt.Errorf("node %d ended prepare %s in a state this core does not know: %q", nodeID, renditionDir,
		report.State)
}

// recordProgress records how far a running prepare is, on its making and, when the source's length
// is known, on its job for the job view.
func (service *Service) recordProgress(
	ctx context.Context, jobID int64, run renditionRun, preparedThroughMs, durationMs int64,
) error {
	if err := service.recordRunning(ctx, run, preparedThroughMs); err != nil {
		return err
	}
	if durationMs <= 0 {
		return nil
	}
	return service.queue.SetProgress(ctx, jobID, min(float64(preparedThroughMs)/float64(durationMs), 1))
}

// recordAudio records what a making puts in the rendition's audio tracks: the gains in their
// sound (D226 §4) and the languages they are tagged with.
func (service *Service) recordAudio(ctx context.Context, renditionID int64, gainsJSON, languagesJSON string) error {
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.SetRenditionAudio(ctx, cachegen.SetRenditionAudioParams{
			CarriedGainsDBJSON: gainsJSON, AudioLanguagesJSON: languagesJSON,
			ID: renditionID,
		})
	})
	if err != nil {
		return fmt.Errorf("record rendition %d's audio: %w", renditionID, err)
	}
	return nil
}

// recordRunning records how far a making is while it has not ended.
func (service *Service) recordRunning(ctx context.Context, run renditionRun, preparedThroughMs int64) error {
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.SetRenditionRunning(ctx, cachegen.SetRenditionRunningParams{
			PreparedThroughMs: preparedThroughMs, SourceFile: run.fileID, Profile: run.profile, RunID: run.runID,
		})
	})
	if err != nil {
		return fmt.Errorf("record rendition %s running: %w", run.dir(), err)
	}
	return nil
}

// renditionRun is one making of a rendition, as its directory on the node names it,
// <file_id>/<profile>/<run_id> (nodeproto.Prepare.RenditionDir): what a node's answer on a prepare
// is recorded by, so a rendition made again since, under another run, is never taken for it (G13).
type renditionRun struct {
	fileID  int64
	profile string
	runID   string
}

func (run renditionRun) dir() string {
	return strconv.FormatInt(run.fileID, 10) + "/" + run.profile + "/" + run.runID
}

// parseRenditionDir is the making a rendition directory names.
func parseRenditionDir(dir string) (renditionRun, error) {
	parts := strings.Split(dir, "/")
	if len(parts) == 3 {
		fileID, err := strconv.ParseInt(parts[0], 10, 64)
		if err == nil && parts[1] != "" && parts[2] != "" {
			return renditionRun{fileID: fileID, profile: parts[1], runID: parts[2]}, nil
		}
	}
	return renditionRun{}, fmt.Errorf("%q names no making of a rendition, <file_id>/<profile>/<run_id>", dir)
}

// finish records a making of a rendition complete with its size, what made its video and how fast,
// or failed with why. A rendition made again since, under another run, is not touched, and
// isCurrent says so.
func (service *Service) finish(
	ctx context.Context, run renditionRun, doneReport *nodeproto.PrepareReport, failure string,
) (isCurrent bool, err error) {
	now := service.clock.Now().UnixMilli()
	var touched int64
	err = service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		var writeErr error
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
			touched, writeErr = queries.SetRenditionComplete(ctx, cachegen.SetRenditionCompleteParams{
				SizeBytes: &doneReport.SizeBytes, FinishedAt: &now, VideoEncoder: videoEncoder,
				RealtimeFactor: realtimeFactor,
				SourceFile:     run.fileID, Profile: run.profile, RunID: run.runID,
			})
			return writeErr
		}
		touched, writeErr = queries.SetRenditionFailed(ctx, cachegen.SetRenditionFailedParams{
			Failure: &failure, FinishedAt: &now, SourceFile: run.fileID, Profile: run.profile, RunID: run.runID,
		})
		return writeErr
	})
	if err != nil {
		return false, fmt.Errorf("record rendition %s finished: %w", run.dir(), err)
	}
	return touched > 0, nil
}

// failMaking records a making of a rendition failed, for a handler whose job ends with it.
func (service *Service) failMaking(ctx context.Context, run renditionRun, failure string) error {
	_, err := service.finish(ctx, run, nil, failure)
	return err
}

// prepareCommand is the node's command for a rendition of file: its source by identity, its
// directory, its picture's codec and every audio track (D217 §3).
func prepareCommand(
	source nodeproto.BytesFile, fileID int64, profile nodeproto.PrepareProfile, runID string, probe nodeproto.Probe,
) nodeproto.Prepare {
	command := nodeproto.Prepare{
		Video: source, Profile: profile,
		RenditionDir:     renditionRun{fileID: fileID, profile: string(profile), runID: runID}.dir(),
		SourceDurationMs: probe.Format.DurationMs,
	}
	if picture, found := decide.MainVideo(probe); found {
		command.VideoStreamIndex, command.VideoCodec = picture.StreamIndex, picture.Codec
	}
	command.AudioTracks = preparedAudio(probe)
	return command
}

// preparedAudio are a source's audio tracks as a prepare carries them: each in its place, tagged
// with its language, langtag.Undetermined when the probe gave none.
func preparedAudio(probe nodeproto.Probe) []nodeproto.PrepareAudio {
	var tracks []nodeproto.PrepareAudio
	for index, stream := range decide.AudioStreams(probe) {
		language := cmp.Or(stream.Language, langtag.Undetermined)
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
