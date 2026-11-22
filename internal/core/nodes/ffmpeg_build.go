// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// FFmpegFailures forgets the attempts FFmpeg gave up on of a node's files, each by the package that
// keeps them (identify.GivenUpForgetters, as a rescan forgets them, D535 §8).
type FFmpegFailures interface {
	// ForgetFFmpegSubtitleFailures forgets, in a write of its own, why the files' subtitle texts could
	// not be made.
	ForgetFFmpegSubtitleFailures(ctx context.Context, fileIDs []int64) error
	// ForgetFFmpegFailuresOfFiles forgets, in the write that keeps the node's new build, the failed
	// loudness measurements, picture timelines and repeat measurements of the files named, a JSON array
	// of ids, and says how many.
	ForgetFFmpegFailuresOfFiles(ctx context.Context, queries *sqlcgen.Queries, fileIDsJSON string) (int64, error)
}

// SetFFmpegFailures names what a node's hello with another FFmpeg build forgets the failures FFmpeg
// made of its files by, and what is told once it has, to ask the background passes to try each again
// (D535 §9). Set it before the channel serves; with none set, a hello keeps its build and forgets
// nothing.
func (service *Service) SetFFmpegFailures(failures FFmpegFailures, forgotten func()) {
	service.ffmpegFailures, service.ffmpegFailuresForgotten = failures, forgotten
}

// takeFFmpegBuild keeps the FFmpeg build a node's hello says it runs, the SHA-256 of its binary, and,
// when the node last said another, forgets the failures FFmpeg made of the files at a live place on
// it, so the background passes try each again with the new build (D535 §9): what succeeded stays, and
// what is not FFmpeg's (a provider's failure to fetch, a whole-file hash) is untouched. A node's first
// build is kept and forgets nothing, as does a hello that says none. The subtitle texts are forgotten
// first, in their own write, and the rest in the one that keeps the build, so a stop between the two
// leaves the old build kept and the next hello forgets again.
func (service *Service) takeFFmpegBuild(ctx context.Context, nodeID int64, sha256 string) error {
	if sha256 == "" || service.ffmpegFailures == nil {
		return nil
	}
	kept, err := service.database.Read().GetNodeFFmpegSha256(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("read node %d's FFmpeg build: %w", nodeID, err)
	}
	if kept != nil && *kept == sha256 {
		return nil
	}
	isAnother := kept != nil
	var files []int64
	if isAnother {
		if files, err = service.database.Read().ListLiveFileIDsOfNode(ctx, nodeID); err != nil {
			return fmt.Errorf("list node %d's files: %w", nodeID, err)
		}
		if subtitleErr := service.ffmpegFailures.ForgetFFmpegSubtitleFailures(ctx, files); subtitleErr != nil {
			return fmt.Errorf("forget the subtitle texts FFmpeg failed at of node %d's files: %w", nodeID,
				subtitleErr)
		}
	}
	filesJSON, err := json.Marshal(files)
	if err != nil {
		return fmt.Errorf("name node %d's files: %w", nodeID, err)
	}
	var forgotten int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if isAnother {
			var forgetErr error
			if forgotten, forgetErr = service.ffmpegFailures.ForgetFFmpegFailuresOfFiles(
				ctx, queries, string(filesJSON)); forgetErr != nil {
				return forgetErr
			}
		}
		return queries.SetNodeFFmpegSha256(ctx, sqlcgen.SetNodeFFmpegSha256Params{
			FfmpegSha256: &sha256, ID: nodeID,
		})
	})
	if err != nil {
		return fmt.Errorf("keep node %d's FFmpeg build: %w", nodeID, err)
	}
	if !isAnother {
		return nil
	}
	service.logger.InfoContext(ctx, "a node runs another FFmpeg; the failures FFmpeg made of its files are forgotten",
		"nodeId", nodeID, "ffmpegSha256", sha256, "fileCount", len(files), "failures", forgotten)
	if service.ffmpegFailuresForgotten != nil {
		service.ffmpegFailuresForgotten()
	}
	return nil
}
