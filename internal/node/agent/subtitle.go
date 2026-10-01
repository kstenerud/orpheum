// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/node/subtitle"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// Subtitler makes a subtitle's ASS and WebVTT; subtitle.Extractor is the real one.
type Subtitler interface {
	ExtractTrack(ctx context.Context, videoPath string, streamIndex int) (subtitle.Text, error)
	ConvertText(ctx context.Context, source []byte) (subtitle.Text, error)
}

// extractSubtitle starts an extraction beside the node's other work, one at a time, since an
// embedded track means reading the whole video, and posts the answer to the core.
func (agent *Agent) extractSubtitle(ctx context.Context, nodeID, jobID int64, command nodeproto.ExtractSubtitle) {
	agent.answerInBackground(ctx, nodeID, backgroundJob{
		jobID: jobID, slot: agent.slots.extract, jobKind: "a subtitle",
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			answer := agent.runExtraction(jobCtx, command)
			return nodeproto.Result{Subtitle: &answer}, answer.Refusal + answer.Failure + stallDetail(answer.Stall)
		},
	})
}

// backgroundJob is one command worked beside the node's others, whose answer is posted to the core.
type backgroundJob struct {
	jobID int64
	// slot bounds how many of its kind run at once.
	slot chan struct{}
	// jobKind names the work in the log.
	jobKind string
	// makeAnswer makes the result, its job id left for answerInBackground to fill, and says why
	// nothing was made: a refusal or a failure.
	makeAnswer func(ctx context.Context) (result nodeproto.Result, refusalOrFailure string)
}

// answerInBackground runs one command beside the node's other work once its slot is free, and
// posts its answer to the core; a command cancelled meanwhile posts nothing.
func (agent *Agent) answerInBackground(ctx context.Context, nodeID int64, job backgroundJob) {
	jobCtx, finishJob := agent.jobs.startJob(ctx, job.jobID)
	go func() {
		defer finishJob()
		select {
		case job.slot <- struct{}{}:
		case <-jobCtx.Done():
			return
		}
		defer func() { <-job.slot }()
		result, refusalOrFailure := job.makeAnswer(jobCtx)
		result.JobID = job.jobID
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "a job was cancelled", "jobKind", job.jobKind, "jobId", job.jobID)
			return
		}
		if refusalOrFailure != "" {
			agent.logger.WarnContext(ctx, "a job was refused or failed", "jobKind", job.jobKind, "jobId", job.jobID,
				"refusalOrFailure", refusalOrFailure)
		}
		agent.postResult(ctx, nodeID, job.jobKind, result)
	}()
}

// notTriedError is an extraction the node did not try: it says nothing of the source's bytes.
type notTriedError struct{ error }

func (refusal notTriedError) Unwrap() error { return refusal.error }

// runExtraction makes one subtitle's text, or says why it could not: that the converter could
// not, which is the source's, or that the node did not try. The text is a video's track, a
// sidecar or a fetched subtitle's text, whichever the command names, and only one.
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
		err = notTriedError{errors.New("a subtitle names a video's track, a sidecar or a fetched text, and only one")}
	case command.FetchedSubRip != nil:
		text, err = agent.convertFetched(ctx, command.FetchedSubRip)
	case command.Video != nil:
		text, err = agent.extractTrack(ctx, *command.Video, command.StreamIndex)
	default:
		text, err = agent.convertSidecar(ctx, *command.Sidecar)
	}
	if errors.As(err, new(notTriedError)) {
		return nodeproto.SubtitleText{Refusal: err.Error()}
	}
	if stall := stallOf(err); stall != nil {
		return nodeproto.SubtitleText{Stall: stall}
	}
	if err != nil {
		return nodeproto.SubtitleText{Failure: err.Error()}
	}
	return nodeproto.SubtitleText{ASS: text.ASS, VTT: text.VTT}
}

// extractTrack extracts a track of a video checked to be the one the core named (D100).
func (agent *Agent) extractTrack(
	ctx context.Context, video nodeproto.BytesFile, streamIndex int,
) (subtitle.Text, error) {
	videoPath, err := agent.verifiedSourcePath(video)
	if err != nil {
		return subtitle.Text{}, notTriedError{err}
	}
	return agent.workers.Subtitler.ExtractTrack(ctx, videoPath, streamIndex)
}

// convertSidecar converts a subtitle file beside the media, read through the root it is under and
// checked to be the one the core named (D100).
func (agent *Agent) convertSidecar(ctx context.Context, sidecar nodeproto.BytesFile) (subtitle.Text, error) {
	file, err := agent.openNamed(sidecar)
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
