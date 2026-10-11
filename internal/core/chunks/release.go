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
