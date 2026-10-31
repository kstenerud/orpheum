// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
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

// ForgetPictureStores throws out a film's stores of its picture, copied and at each quality, once the
// facts of its probe they were made from changed (library.DerivedDroppers, D392 §3): their cache.db
// rows, and what is known of them in memory but where a run is on its way. Their directories' names
// hold the old facts (StoreKey), so the node is never asked for their chunks again, and evicts them
// first. It says whether there were any.
func (service *Service) ForgetPictureStores(ctx context.Context, of library.DerivedOf) (bool, error) {
	return service.forgetStores(ctx, of.FileID, []string{pictureStreamName, "q[0-9]*"}, func(stream Stream) bool {
		return !stream.Kind.IsSound()
	})
}

// ForgetSoundStores throws out a film's stores of one audio track, every mix and its copy, once the
// facts of its probe they were made from changed, as ForgetPictureStores does a picture's.
func (service *Service) ForgetSoundStores(ctx context.Context, of library.DerivedOf) (bool, error) {
	glob := "sound" + strconv.Itoa(of.AudioTrack) + "-*"
	return service.forgetStores(ctx, of.FileID, []string{glob}, func(stream Stream) bool {
		return stream.Kind.IsSound() && stream.AudioTrackIndex == of.AudioTrack
	})
}

// forgetStores deletes a film's chunk_store rows of the streams whose names match globs, and forgets
// what the tracker holds of the stores of the streams isOf names, but one making; it says whether
// there were any.
func (service *Service) forgetStores(
	ctx context.Context, fileID int64, globs []string, isOf func(stream Stream) bool,
) (bool, error) {
	var deleted int64
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		deleted = 0
		for _, glob := range globs {
			count, err := queries.DeleteChunkStoresOfStreams(ctx, cachegen.DeleteChunkStoresOfStreamsParams{
				SourceFile: fileID, StreamGlob: glob,
			})
			if err != nil {
				return err
			}
			deleted += count
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("forget file %d's chunk stores: %w", fileID, err)
	}
	forgotten := service.runs.forgetStoresOf(fileID, isOf)
	return deleted > 0 || forgotten > 0, nil
}

// forgetStoresOf forgets the stores of fileID of the streams isOf names that no run of is on its
// way, and says how many.
func (tracker *runTracker) forgetStoresOf(fileID int64, isOf func(stream Stream) bool) int {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	forgotten := 0
	for dir, known := range tracker.storeByDir {
		parts := strings.Split(dir, "/")
		if fileOfStore(dir) != fileID || len(known.runByFirst) > 0 || len(parts) != 3 {
			continue
		}
		if stream, isStream := StreamNamed(strings.TrimPrefix(parts[1], "chunks-")); !isStream || !isOf(stream) {
			continue
		}
		delete(tracker.storeByDir, dir)
		forgotten++
	}
	return forgotten
}
