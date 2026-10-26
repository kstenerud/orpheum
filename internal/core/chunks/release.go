// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
)

// ReleaseWanted says no player of a film waits for its chunks or its timeline any more, as one that
// left its page (D258 §2): each of its stores' runs, and its read-through, falls to background at
// the next demotion. A player still watching the film asks again for what it reads, and keeps it.
func (service *Service) ReleaseWanted(ctx context.Context, fileID int64) error {
	stores, err := service.cache.Read().ListChunkStoresOfFile(ctx, fileID)
	if err != nil {
		return fmt.Errorf("list file %d's chunk stores: %w", fileID, err)
	}
	for _, storeID := range stores {
		err := service.queue.ReleaseWanted(ctx, jobs.KindMakeChunks, jobs.Target{ChunkStoreID: storeID})
		if err != nil {
			return err
		}
	}
	return service.readThrough.ReleaseForPlay(ctx, fileID)
}

// ForgetFile forgets what the run tracker keeps of the film's stores that have no run on its way:
// the chunks known made, an eviction, the grid. A playback service calls it once no playback of
// the film is left; kept, the tracker grew with every film ever played, and its per-request walks
// with it (the audit of 2026-10-25, C5). A player that comes back is answered by the node, which
// keeps the chunks, as after a restart.
func (service *Service) ForgetFile(fileID int64) {
	service.runs.forgetFile(fileID)
}

// forgetFile forgets the stores of fileID that no run of is on its way.
func (tracker *runTracker) forgetFile(fileID int64) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	for dir, known := range tracker.storeByDir {
		if fileOfStore(dir) == fileID && len(known.runByFirst) == 0 {
			delete(tracker.storeByDir, dir)
		}
	}
}
