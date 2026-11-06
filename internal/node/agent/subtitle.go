// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/subtitle"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// Subtitler makes a subtitle's ASS and WebVTT; subtitle.Extractor is the real one.
type Subtitler interface {
	ExtractTracks(ctx context.Context, film execx.Film, streamIndexes []int) ([]subtitle.TrackText, error)
	ConvertText(ctx context.Context, source []byte) (subtitle.Text, error)
}

// subtitleWork is an extraction held until the node's one extraction slot is free, since a video's
// tracks mean reading the whole video, which posts its answer to the core.
func (agent *Agent) subtitleWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.ExtractSubtitle,
) heldWork {
	return agent.answeringWork(ctx, nodeID, answeringJob{
		attempt: held, jobKind: "a subtitle",
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			answer := agent.runExtraction(jobCtx, command)
			return nodeproto.Result{Subtitle: &answer}, answer.Refusal + answer.Failure + stallDetail(answer.Stall) +
				trackFailures(answer.Tracks)
		},
	})
}

// answeringJob is one job whose whole answer is posted to the core when it ends.
type answeringJob struct {
	attempt
	// jobKind names the work in the log.
	jobKind string
	// makeAnswer makes the result, the attempt it answers left for answeringWork to name, and says why
	// nothing was made: a refusal or a failure.
	makeAnswer func(ctx context.Context) (result nodeproto.Result, refusalOrFailure string)
	// stalledAnswer, for a job that reads in the node's own process, is its answer when its reading
	// stops moving for the node's read window: the node answers it so and leaves the reading, which it
	// cannot stop, to end when its disk answers (D233 §10); reading names where. Nil for a job whose
	// FFmpeg execx watches.
	stalledAnswer func(stall nodeproto.Stall) nodeproto.Result
	reading       string
}

// answeringWork is the work of a job that posts its answer to the core when it ends; a job
// cancelled meanwhile posts nothing.
func (agent *Agent) answeringWork(ctx context.Context, nodeID int64, job answeringJob) heldWork {
	var stalls chan time.Duration
	if job.stalledAnswer != nil {
		stalls = make(chan time.Duration, 1)
	}
	return heldWork{stalls: stalls, run: func(jobCtx context.Context) {
		agent.sayProgress(ctx, nodeID, job.progress(nodeproto.ProgressRunning))
		if stuck, isStuck := agent.diskGate.IsStuck(job.reading, job.reading); isStuck && job.stalledAnswer != nil {
			// Its root holds a reading that has not returned: this one would only wait beside it.
			agent.logger.InfoContext(ctx, "a job was not tried: its root is still waiting on its disk",
				slogx.EventKey, "jobNotTriedRootWaiting",
				"jobKind", job.jobKind, "jobId", job.jobID, "rootPath", job.reading, "waitingOn", stuck.Path,
				"stuckSince", stuck.Since)
			agent.postResult(ctx, nodeID, job.jobKind, job.named(job.stalledAnswer(nodeproto.Stall{
				Cause: nodeproto.StallNeverMoved, Condition: nodeproto.ConditionWaitingOnDisk,
				WaitingOnPath: stuck.Path, Detail: "not tried: an earlier reading of " + job.reading +
					" is still waiting on its disk",
			})))
			return
		}
		result, refusalOrFailure, didStall := agent.answerUnlessStalled(jobCtx, job, stalls)
		if didStall {
			agent.postResult(ctx, nodeID, job.jobKind, job.named(result))
			return
		}
		result = job.named(result)
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "a job was cancelled", "jobKind", job.jobKind, "jobId", job.jobID)
			return
		}
		if refusalOrFailure != "" {
			agent.logger.WarnContext(ctx, "a job was refused or failed", "jobKind", job.jobKind, "jobId", job.jobID,
				"refusalOrFailure", refusalOrFailure)
		}
		agent.postResult(ctx, nodeID, job.jobKind, result)
	}}
}

// answerUnlessStalled makes the job's answer, unless its reading stops moving first: then its
// answer is that it stalled on its disk, didStall is true, and its reading is listed as stuck
// until it ends, its own answer dropped. A job without a stalled answer is answered as it ends.
func (agent *Agent) answerUnlessStalled(
	ctx context.Context, job answeringJob, stalls <-chan time.Duration,
) (result nodeproto.Result, refusalOrFailure string, didStall bool) {
	if stalls == nil {
		result, refusalOrFailure = job.makeAnswer(ctx)
		return result, refusalOrFailure, false
	}
	type answer struct {
		result           nodeproto.Result
		refusalOrFailure string
	}
	answered := make(chan answer, 1)
	go func() {
		result, refusalOrFailure := job.makeAnswer(ctx)
		answered <- answer{result, refusalOrFailure}
	}()
	select {
	case made := <-answered:
		return made.result, made.refusalOrFailure, false
	case <-ctx.Done():
		// Cancelled: the job ends now, its reading left to end on its own.
		return nodeproto.Result{}, ctx.Err().Error(), false
	case stood := <-stalls:
		agent.stuckReads.add(job.token, nodeproto.Stuck{
			Tool: stuckReadTool, Paths: []string{job.reading}, Since: agent.clock.Now().UnixMilli(),
		})
		// Its root holds no other reading until this one returns (diskwatch).
		release, _ := agent.diskGate.Hold(job.reading, job.reading)
		go func() {
			<-answered
			agent.stuckReads.remove(job.token)
			release()
		}()
		detail := fmt.Sprintf("%s of %s did not move for %s: its disk is not answering", job.jobKind, job.reading,
			stood.Round(time.Second))
		agent.logger.WarnContext(ctx, "a job's reading stalled and is left to end", "jobKind", job.jobKind,
			"jobId", job.jobID, "reading", job.reading, "withoutMovingFor", stood)
		return job.stalledAnswer(nodeproto.Stall{
			Cause: nodeproto.StallStoodStill, Condition: nodeproto.ConditionWaitingOnDisk, IsLeftRunning: true,
			WithoutAdvanceMs: stood.Milliseconds(), Detail: detail,
		}), "", true
	}
}

// notTriedError is an extraction the node did not try: it says nothing of the source's bytes.
type notTriedError struct{ error }

func (refusal notTriedError) Unwrap() error { return refusal.error }

// runExtraction makes a subtitle's text, or says why it could not: that the converter could not,
// which is the source's, or that the node did not try. The text is a video's tracks, a sidecar or a
// fetched subtitle's text, whichever the command names, and only one.
func (agent *Agent) runExtraction(ctx context.Context, command nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
	sourceCount := 0
	for _, isSet := range []bool{command.Video != nil, command.Sidecar != nil, command.FetchedSubRip != nil} {
		if isSet {
			sourceCount++
		}
	}
	var text subtitle.Text
	var err error
	switch {
	case agent.workers.Subtitler == nil:
		err = notTriedError{errors.New("this node does not extract subtitles")}
	case sourceCount != 1:
		err = notTriedError{errors.New("a subtitle names a video's tracks, a sidecar or a fetched text, and only one")}
	case command.Video != nil && len(command.StreamIndexes) == 0:
		err = notTriedError{errors.New("a video's extraction names no track")}
	case command.Video != nil:
		var tracks []nodeproto.TrackText
		if tracks, err = agent.extractTracks(ctx, *command.Video, command.StreamIndexes); err == nil {
			return nodeproto.SubtitleText{Tracks: tracks}
		}
	case command.FetchedSubRip != nil:
		text, err = agent.convertFetched(ctx, command.FetchedSubRip)
	default:
		text, err = agent.convertSidecar(ctx, *command.Sidecar)
	}
	if err != nil {
		return subtitleAnswerOf(err)
	}
	return nodeproto.SubtitleText{ASS: text.ASS, VTT: text.VTT}
}

// subtitleAnswerOf is the answer of an extraction that made nothing: the node did not try, FFmpeg
// stalled, or the converter could not make the text.
func subtitleAnswerOf(err error) nodeproto.SubtitleText {
	if errors.As(err, new(notTriedError)) {
		return nodeproto.SubtitleText{Refusal: err.Error(), RefusalCode: refusalCodeOf(err)}
	}
	if stall := stallOf(err); stall != nil {
		return nodeproto.SubtitleText{Stall: stall}
	}
	return nodeproto.SubtitleText{Failure: err.Error()}
}

// extractTracks extracts tracks of a video checked to be the one the core named (D100), from one
// read of it (D340), each with its text or its own failure.
func (agent *Agent) extractTracks(
	ctx context.Context, video nodeproto.BytesFile, streamIndexes []int,
) ([]nodeproto.TrackText, error) {
	film, closeFilm, err := agent.openFilm(ctx, video)
	if stallOf(err) != nil {
		return nil, err
	}
	if err != nil {
		return nil, notTriedError{err}
	}
	defer closeFilm()
	extracted, err := agent.workers.Subtitler.ExtractTracks(ctx, film, streamIndexes)
	if err != nil {
		return nil, err
	}
	tracks := make([]nodeproto.TrackText, 0, len(extracted))
	for _, track := range extracted {
		answer := nodeproto.TrackText{StreamIndex: track.StreamIndex, ASS: track.Text.ASS, VTT: track.Text.VTT}
		if track.Err != nil {
			answer = nodeproto.TrackText{StreamIndex: track.StreamIndex, Failure: track.Err.Error()}
		}
		tracks = append(tracks, answer)
	}
	return tracks, nil
}

// trackFailures says, for the log, which of a video's tracks could not be made and why.
func trackFailures(tracks []nodeproto.TrackText) string {
	var failures []string
	for _, track := range tracks {
		if track.Failure != "" {
			failures = append(failures, fmt.Sprintf("stream %d: %s", track.StreamIndex, track.Failure))
		}
	}
	return strings.Join(failures, "; ")
}

// convertSidecar converts a subtitle file beside the media, read through the root it is under and
// checked to be the one the core named (D100).
func (agent *Agent) convertSidecar(ctx context.Context, sidecar nodeproto.BytesFile) (subtitle.Text, error) {
	file, _, err := agent.openNamed(ctx, sidecar)
	if stallOf(err) != nil {
		return subtitle.Text{}, err
	}
	if err != nil {
		return subtitle.Text{}, notTriedError{err}
	}
	source, err := bounded.ReadAll(file, subtitle.MaxTextBytes)
	readonly.Close(file)
	if err != nil {
		return subtitle.Text{}, fmt.Errorf("read the sidecar: %w", err)
	}
	return agent.workers.Subtitler.ConvertText(ctx, source)
}

// convertFetched converts a fetched subtitle's text, which the command carries (D224 §5).
func (agent *Agent) convertFetched(ctx context.Context, fetched []byte) (subtitle.Text, error) {
	if len(fetched) > subtitle.MaxTextBytes {
		return subtitle.Text{}, fmt.Errorf("a fetched subtitle larger than %d bytes", subtitle.MaxTextBytes)
	}
	return agent.workers.Subtitler.ConvertText(ctx, fetched)
}
