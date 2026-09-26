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
	go func() {
		select {
		case agent.slots.extract <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-agent.slots.extract }()
		answer := agent.runExtraction(ctx, command)
		if answer.Refused != "" {
			agent.logger.WarnContext(ctx, "refused a subtitle", "job", command.Job, "why", answer.Refused)
		}
		encoded, err := json.Marshal(answer)
		if err != nil {
			agent.logger.ErrorContext(ctx, "could not encode a subtitle", "job", command.Job, "err", err)
			return
		}
		var accepted struct{}
		err = agent.exchange(ctx, http.MethodPost, nodeproto.PathSubtitle, nodeID, bytes.NewReader(encoded), &accepted)
		if err != nil && ctx.Err() == nil {
			agent.logger.WarnContext(ctx, "could not send a subtitle to the core", "job", command.Job, "err", err)
		}
	}()
}

// runExtraction makes one subtitle's text, or says why it could not: a video or a sidecar is
// checked to be the file the core named (D100), and a sidecar is read through the root it is
// under.
func (agent *Agent) runExtraction(ctx context.Context, command nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
	refused := func(err error) nodeproto.SubtitleText {
		return nodeproto.SubtitleText{Job: command.Job, Refused: err.Error()}
	}
	if agent.workers.Subtitles == nil {
		return refused(errors.New("this node does not extract subtitles"))
	}
	var text subtitle.Text
	answer := nodeproto.SubtitleText{Job: command.Job}
	switch {
	case command.Video != nil && command.Sidecar == nil:
		videoPath, err := agent.verifiedSource(*command.Video)
		if err != nil {
			return refused(err)
		}
		if text, err = agent.workers.Subtitles.ExtractTrack(ctx, videoPath, command.StreamIndex); err != nil {
			return refused(err)
		}
	case command.Sidecar != nil && command.Video == nil:
		root, known := agent.roots[command.Sidecar.Root]
		if !known {
			return refused(errors.New("the sidecar is under none of this node's roots"))
		}
		source, err := readBounded(root, string(command.Sidecar.Path), subtitle.MaxTextBytes)
		if err != nil {
			return refused(fmt.Errorf("read the sidecar: %w", err))
		}
		identity, err := hash.Payload(bytes.NewReader(source), int64(len(source)))
		if err != nil {
			return refused(fmt.Errorf("hash the sidecar: %w", err))
		}
		if hex.EncodeToString(identity.Hash[:]) != command.Sidecar.Hash {
			return refused(errors.New("the file at this path is not the one named; it changed since the scan"))
		}
		if text, err = agent.workers.Subtitles.ConvertFile(ctx, source); err != nil {
			return refused(err)
		}
	default:
		return refused(errors.New("a subtitle names a video's track or a sidecar, and only one"))
	}
	answer.ASS, answer.VTT = text.ASS, text.VTT
	return answer
}
