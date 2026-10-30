// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/kstenerud/orpheum/internal/node/artwork"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// artworkWork re-encodes an artwork job's images, each in turn, and posts one answer for all — each
// image's sizes, or why there are none — to the core, whose artwork store takes images from nowhere
// else (D212 §8).
func (agent *Agent) artworkWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.ArtworkBatch,
) heldWork {
	return heldWork{run: func(jobCtx context.Context) {
		agent.sayProgress(ctx, nodeID, held.progress(nodeproto.ProgressRunning))
		results := make([]nodeproto.ArtworkResult, 0, len(command.Images))
		for _, image := range command.Images {
			results = append(results, agent.reEncode(jobCtx, held.jobID, image))
			if jobCtx.Err() != nil {
				break
			}
		}
		if jobCtx.Err() != nil && ctx.Err() == nil {
			agent.logger.InfoContext(ctx, "an artwork re-encode was cancelled", "jobId", held.jobID)
			return
		}
		for _, result := range results {
			if result.Refusal != "" {
				agent.logger.WarnContext(ctx, "refused an image", "jobId", held.jobID, "key", result.Key,
					"refusal", result.Refusal)
			}
		}
		agent.postResult(ctx, nodeID, "an artwork re-encode",
			held.named(nodeproto.Result{Artwork: &nodeproto.ArtworkBatchResult{Images: results}}))
	}}
}

// reEncode re-encodes one image, read from beside the media when it names its path there, or taken
// from a video it names (D362), and names the image's key in its answer.
func (agent *Agent) reEncode(ctx context.Context, jobID int64, image nodeproto.Artwork) nodeproto.ArtworkResult {
	sourceHash := ""
	var source []byte
	var err error
	switch {
	case image.Video != nil:
		source, err = agent.takeStill(ctx, jobID, image)
	case image.AbsolutePath != nil:
		source, err = agent.readImage(*image.AbsolutePath)
	}
	if errors.Is(err, artwork.ErrNoFrame) {
		return nodeproto.ArtworkResult{Key: image.Key, Refusal: err.Error(), HasNoFrame: true}
	}
	if err != nil {
		return nodeproto.ArtworkResult{Key: image.Key, Refusal: err.Error()}
	}
	if source != nil {
		sum := sha256.Sum256(source)
		image.SourceImage, image.AbsolutePath, image.Video, sourceHash = source, nil, nil, hex.EncodeToString(sum[:])
	}
	result := agent.workers.Artworker.Derive(ctx, image)
	result.Key, result.SourceSha256 = image.Key, sourceHash
	return result
}

// takeStill takes a frame still from the video an artwork command names, once it is the file the core
// named (D100), read as every run reads its film (openFilm, D330), and logs it made or why not (D362 §5).
func (agent *Agent) takeStill(ctx context.Context, jobID int64, image nodeproto.Artwork) ([]byte, error) {
	if agent.workers.StillTaker == nil {
		return nil, errors.New("this node does not take frame stills")
	}
	path := image.Video.RootPath + "/" + string(image.Video.Path)
	film, closeFilm, err := agent.openFilm(ctx, *image.Video)
	if err != nil {
		agent.logger.WarnContext(ctx, "could not open a video to take a still from", "jobId", jobID, "path", path,
			"err", err)
		return nil, err
	}
	defer closeFilm()
	started := agent.clock.Now()
	still, err := agent.workers.StillTaker.Still(ctx, film, image.FrameStreamIndex,
		time.Duration(image.FrameAtMs)*time.Millisecond)
	said := []any{
		"jobId", jobID, "path", path, "atMs", image.FrameAtMs,
		"tookMs", agent.clock.Now().Sub(started).Milliseconds(),
	}
	switch {
	case errors.Is(err, artwork.ErrNoFrame):
		agent.logger.InfoContext(ctx, "a video has no frame to take a still from", said...)
	case err != nil:
		agent.logger.WarnContext(ctx, "could not take a still from a video", append(said, "err", err)...)
	default:
		agent.logger.InfoContext(ctx, "took a still from a video", append(said, "stillBytes", len(still))...)
	}
	return still, err
}

// maxImageBytes bounds an image beside the media the node reads: posters are megabytes.
const maxImageBytes = 64 << 20

// readImage reads an image beside the media through the root it is under, so no path the core
// names can reach outside the node's roots (D107).
func (agent *Agent) readImage(named nodeproto.Path) ([]byte, error) {
	absolute := named.Text
	if named.Raw != nil {
		absolute = string(named.Raw)
	}
	var holder *confine.MediaRoot
	holderRelativePath := ""
	for _, root := range agent.roots {
		relativePath, inside := root.RelativeIfInside(absolute)
		// The deepest root holds it when roots nest, whatever order the map gives them in.
		if inside && relativePath != "." && (holder == nil || len(relativePath) < len(holderRelativePath)) {
			holder, holderRelativePath = root, relativePath
		}
	}
	if holder == nil {
		return nil, errors.New("the image is under none of this node's roots")
	}
	return holder.ReadFile(holderRelativePath, maxImageBytes)
}
