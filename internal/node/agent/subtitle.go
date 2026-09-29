// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/node/subtitle"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Subtitler makes a subtitle's ASS and WebVTT; subtitle.Extractor is the real one.
type Subtitler interface {
	ExtractTrack(ctx context.Context, videoPath string, streamIndex int) (subtitle.Text, error)
	ConvertFile(ctx context.Context, source []byte) (subtitle.Text, error)
}

// extractSubtitle starts an extraction beside the node's other work, one at a time, since an
// embedded track means reading the whole video, and posts the answer to the core.
func (agent *Agent) extractSubtitle(ctx context.Context, nodeID int64, command nodeproto.ExtractSubtitle) {
	agent.answerInBackground(ctx, nodeID, background{
		job: command.JobID, slot: agent.slots.extract, path: nodeproto.PathSubtitle, what: "a subtitle",
		work: func(jobCtx context.Context) (any, string) {
			answer := agent.runExtraction(jobCtx, command)
			return answer, answer.Refusal
		},
	})
}

// background is one command worked beside the node's others, whose answer is posted to the core.
type background struct {
	job int64
	// slot bounds how many of its kind run at once.
	slot chan struct{}
	// path is where the answer is posted; what names the work in the log.
	path, what string
	// work makes the answer, and says why nothing was made when it refused.
	work func(ctx context.Context) (answer any, refused string)
}

// answerInBackground runs one command beside the node's other work once its slot is free, and
// posts its answer to the core; a command cancelled meanwhile posts nothing.
func (agent *Agent) answerInBackground(ctx context.Context, nodeID int64, command background) {
	jobCtx, done := agent.working.start(ctx, command.job)
	go func() {
		defer done()
		select {
		case command.slot <- struct{}{}:
		case <-jobCtx.Done():
			return
		}
		defer func() { <-command.slot }()
		answer, refused := command.work(jobCtx)
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "a job was cancelled", "work", command.what, "jobId", command.job)
			return
		}
		if refused != "" {
			agent.logger.WarnContext(ctx, "refused a job", "work", command.what, "jobId", command.job, "why", refused)
		}
		encoded, err := json.Marshal(answer)
		if err != nil {
			agent.logger.ErrorContext(ctx, "could not encode a job's answer", "work", command.what, "jobId",
				command.job,
				"err", err)
			return
		}
		var accepted struct{}
		err = agent.exchange(ctx, http.MethodPost, command.path, nodeID, bytes.NewReader(encoded), &accepted)
		if err != nil && ctx.Err() == nil {
			agent.logger.WarnContext(ctx, "could not send a job's answer to the core", "work", command.what,
				"jobId", command.job, "err", err)
		}
	}()
}

// runExtraction makes one subtitle's text, or says why it could not: a video's track, a sidecar
// or a fetched subtitle's text, whichever the command names, and only one.
func (agent *Agent) runExtraction(ctx context.Context, command nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
	named := 0
	for _, isSet := range []bool{command.Video != nil, command.Sidecar != nil, command.FetchedSubRip != nil} {
		if isSet {
			named++
		}
	}
	var text subtitle.Text
	var err error
	switch {
	case agent.workers.Subtitles == nil:
		err = errors.New("this node does not extract subtitles")
	case named != 1:
		err = errors.New("a subtitle names a video's track, a sidecar or a fetched text, and only one")
	case command.FetchedSubRip != nil:
		text, err = agent.convertFetched(ctx, command.FetchedSubRip)
	case command.Video != nil:
		text, err = agent.extractTrack(ctx, *command.Video, command.StreamIndex)
	default:
		text, err = agent.convertSidecar(ctx, *command.Sidecar)
	}
	if err != nil {
		return nodeproto.SubtitleText{JobID: command.JobID, Refusal: err.Error()}
	}
	return nodeproto.SubtitleText{JobID: command.JobID, ASS: text.ASS, VTT: text.VTT}
}

// extractTrack extracts a track of a video checked to be the one the core named (D100).
func (agent *Agent) extractTrack(
	ctx context.Context, video nodeproto.BytesFile, streamIndex int,
) (subtitle.Text, error) {
	videoPath, err := agent.verifiedSource(video)
	if err != nil {
		return subtitle.Text{}, err
	}
	return agent.workers.Subtitles.ExtractTrack(ctx, videoPath, streamIndex)
}

// convertSidecar converts a subtitle file beside the media, read through the root it is under and
// checked to be the one the core named (D100).
func (agent *Agent) convertSidecar(ctx context.Context, sidecar nodeproto.BytesFile) (subtitle.Text, error) {
	root, known := agent.roots[sidecar.Root]
	if !known {
		return subtitle.Text{}, errors.New("the sidecar is under none of this node's roots")
	}
	source, err := readBounded(root, string(sidecar.Path), subtitle.MaxTextBytes)
	if err != nil {
		return subtitle.Text{}, fmt.Errorf("read the sidecar: %w", err)
	}
	identity, err := hash.Payload(bytes.NewReader(source), int64(len(source)))
	if err != nil {
		return subtitle.Text{}, fmt.Errorf("hash the sidecar: %w", err)
	}
	if hex.EncodeToString(identity.Hash[:]) != sidecar.PayloadHash {
		return subtitle.Text{}, errors.New("the file at this path is not the one named; it changed since the scan")
	}
	return agent.workers.Subtitles.ConvertFile(ctx, source)
}

// convertFetched converts a fetched subtitle's text, which the command carries (D224 §5).
func (agent *Agent) convertFetched(ctx context.Context, fetched []byte) (subtitle.Text, error) {
	if len(fetched) > subtitle.MaxTextBytes {
		return subtitle.Text{}, fmt.Errorf("a fetched subtitle larger than %d bytes", subtitle.MaxTextBytes)
	}
	return agent.workers.Subtitles.ConvertFile(ctx, fetched)
}
