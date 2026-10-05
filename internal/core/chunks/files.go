// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"context"
	"fmt"
	"strconv"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/renditions"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// StoreFile is one file of a stream's store a player asks for, its init or a chunk, and where the
// film's node serves it.
type StoreFile struct {
	NodeID int64
	// Target is the file's path on its node's byte socket (nodeproto.PathBytesChunks).
	Target      string
	ContentType string
	fileID      int64
	stream      Stream
	storeDir    string
	// isInit is whether it is the store's init rather than chunk chunk.
	isInit bool
	chunk  int
	// runFrom is the chunk a run is started from when the node does not have the file yet: the
	// chunk itself, or, for the init, the chunk the playback is at, which a player that changes
	// to the stream asks for next.
	runFrom int
}

// StoreFileOf is the file of a stream's store the playback asks for by its stream's name and the
// file's name, init.mp4 or a chunk's NNNNN.m4s: ErrNoStream for a stream the playback is not
// offered or a file the store has no place for, and ErrNoGrid as MasterPlaylist says. Every name is
// checked before the node is asked for anything.
func (service *Service) StoreFileOf(
	ctx context.Context, playback Playback, streamName, fileName string,
) (StoreFile, error) {
	chunk, isInit, isStoreFile := storeFileNamed(fileName)
	if !isStoreFile {
		return StoreFile{}, fmt.Errorf("%w: a file named %q", ErrNoStream, fileName)
	}
	film, stream, err := service.offeredStream(ctx, playback, streamName)
	if err != nil {
		return StoreFile{}, err
	}
	if chunk > film.grid.lastChunk() {
		return StoreFile{}, fmt.Errorf("%w: chunk %d of a film of %d", ErrNoStream, chunk, film.grid.lastChunk()+1)
	}
	runFrom := chunk
	if isInit {
		runFrom = film.grid.chunkAt(playback.PositionMs)
	}
	dir := StoreDir(playback.FileID, stream, film.grid.ID())
	// The node names a chunk's file in five digits, or more past 99999: a name of more is asked for
	// as the node names it.
	name := chunkName(chunk)
	if isInit {
		name = initName
	}
	return StoreFile{
		NodeID: film.file.NodeID, Target: nodeproto.PathBytesChunks + dir + "/" + name,
		ContentType: renditions.ContentType(name),
		fileID:      playback.FileID, stream: stream, storeDir: dir, isInit: isInit, chunk: chunk, runFrom: runFrom,
	}, nil
}

// Want makes sure a file of a store its node does not have yet will be made (Ensure): from its chunk,
// or, for the init, from the chunk the playback is at.
func (service *Service) Want(ctx context.Context, file StoreFile) error {
	return service.Ensure(ctx, file.fileID, file.stream, file.runFrom)
}

// Served records a file of a store its node served: a chunk is known made, so no run is queued for
// it, and a store a run is making stays wanted while its player reads it (D234 §3).
func (service *Service) Served(ctx context.Context, file StoreFile) error {
	if !file.isInit {
		service.runs.served(file.storeDir, file.chunk)
	}
	storeID, isMaking := service.runs.making(file.storeDir)
	if !isMaking || storeID == 0 {
		return nil
	}
	return service.queue.KeepWanted(ctx, jobs.KindMakeChunks, jobs.Target{ChunkStoreID: storeID})
}

// storeFileNamed is the chunk a store's file name names, or that it names the init; false for a
// name of neither: init.mp4, or a chunk's number in five digits or more, NNNNN.m4s.
func storeFileNamed(name string) (chunk int, isInit, isStoreFile bool) {
	match := storeFileNamePattern.FindStringSubmatch(name)
	switch {
	case match == nil:
		return 0, false, false
	case match[1] == "":
		return 0, true, true
	}
	n, err := strconv.Atoi(match[1])
	return n, false, err == nil
}
