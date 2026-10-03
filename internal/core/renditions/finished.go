// SPDX-License-Identifier: GPL-3.0-or-later

package renditions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// FinishedPrepare is one prepare a node finished: its file, its profile, how it ended, and what
// made its video and how fast (D225 §3).
type FinishedPrepare struct {
	SourceFileID int64
	// Path is the file's path, empty when the library no longer knows the file.
	Path    string
	Profile string
	// State is complete or failed.
	State string
	// VideoEncoder is h264_qsv, h264_nvenc, x264 or copy; empty when the node did not say.
	VideoEncoder string
	// RealtimeFactor is seconds of the source per second of running; zero when the node did not say.
	RealtimeFactor float64
	SizeBytes      int64
	Failure        string
	FinishedAt     time.Time
}

// FinishedPrepares are the node's latest finished prepares, newest first, at most limit.
func (service *Service) FinishedPrepares(ctx context.Context, nodeID int64, limit int) ([]FinishedPrepare, error) {
	rows, err := service.cache.Read().ListFinishedRenditions(ctx, cachegen.ListFinishedRenditionsParams{
		NodeID: nodeID, Limit: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list node %d's finished prepares: %w", nodeID, err)
	}
	finished := make([]FinishedPrepare, 0, len(rows))
	for _, row := range rows {
		path, err := store.OrNotFound(service.database.Read().GetFilePath(ctx, row.SourceFile))
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("name file %d: %w", row.SourceFile, err)
		}
		finished = append(finished, FinishedPrepare{
			SourceFileID: row.SourceFile, Path: path, Profile: row.Profile, State: row.State,
			VideoEncoder: ptr.ValueOrZero(row.VideoEncoder), RealtimeFactor: ptr.ValueOrZero(row.RealtimeFactor),
			SizeBytes: ptr.ValueOrZero(row.SizeBytes), Failure: ptr.ValueOrZero(row.Failure),
			FinishedAt: time.UnixMilli(ptr.ValueOrZero(row.FinishedAt)).UTC(),
		})
	}
	return finished, nil
}
