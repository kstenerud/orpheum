// SPDX-License-Identifier: AGPL-3.0-or-later

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
	// kind is what its film is cut at, as its playback's is.
	kind     GridKind
	stream   Stream
	storeDir string
	// isInit is whether it is the store's init rather than chunk chunk.
	isInit bool
	chunk  int
	// isSought is whether a player asked for it loading where a seek took it (AsSought).
	isSought bool
	// runFrom is the chunk a run is started from when the node does not have the file yet: the
	// chunk itself, or, for the init, the chunk the playback is at, which a player that changes
	// to the stream asks for next.
	runFrom int
	// averageKbps is its stream's average bit rate (averageKbpsOf).
	averageKbps int64
	// startMs is where its chunk starts in the film; zero for the init.
	startMs int64
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
		fileID:      playback.FileID, kind: film.kind, stream: stream, storeDir: dir, isInit: isInit, chunk: chunk,
		runFrom: runFrom, averageKbps: film.averageKbpsOf(stream), startMs: film.grid.StartsMs[chunk],
	}, nil
}

// averageKbpsOf is a stream's average bit rate, in kbit/s: a quality's own, a sound's, or the film's
// own picture's, zero when its rate is not known.
func (film cutFilm) averageKbpsOf(stream Stream) int64 {
	switch stream.Kind {
	case nodeproto.StreamQuality:
		return int64(stream.Quality.Kbps)
	case nodeproto.StreamSound:
		return int64(stream.SoundKbps)
	case nodeproto.StreamPicture:
		return film.pictureAverageBps() / 1000
	}
	return 0
}

// AverageKbps is the average bit rate of the file's stream, in kbit/s: what a player loading it
// needs of the server's upload (D286 §5); zero when it is not known.
func (file StoreFile) AverageKbps() int64 { return file.averageKbps }

// StartMs is where the file's chunk starts in the film, and whether it is a chunk: an init says
// nothing of where its player is.
func (file StoreFile) StartMs() (int64, bool) { return file.startMs, !file.isInit }

// StreamName is the name of the file's stream, as a playback's master playlist names it.
func (file StoreFile) StreamName() string { return file.stream.Name() }

// IsSound is whether the file is of a sound rather than a picture.
func (file StoreFile) IsSound() bool { return file.stream.Kind == nodeproto.StreamSound }

// Want makes sure a file of a store its node does not have yet will be made (Ensure): from its chunk,
// or, for the init, from the chunk the playback is at. The film's runs no player wants stop first
// (StopUnwanted, D309 §2): its player's interest moved as it asked. A run the server starts for a
// player, as it starts or stalls, stops nothing: its interest is recorded once the run is on its way.
func (service *Service) Want(ctx context.Context, file StoreFile) error {
	// A store's init is made by any run of it: one on its way makes it, and an init asked for from
	// where a playback last reported, the film's start for a guest who has reported nothing, cut the
	// run the guest's first chunk needed, which cut it back, every two seconds, until the node evicted
	// it (the owner's VQ38 run, 2026-10-12).
	if file.isInit && service.runs.hasRun(file.storeDir) {
		return nil
	}
	if err := service.StopUnwanted(ctx, file.fileID); err != nil {
		return err
	}
	return service.askedFor(ctx, file.fileID, file.kind, file.stream, file.runFrom)
}

// WantAlongside makes sure sound is made from where picture is, a file of a picture asked for where
// a seek took its player (AsSought), as if the sound were asked for there too: hls.js asks for a
// seek's sound only once its picture's first chunk has come, and the sound's run, started then, kept
// the player waiting 5 s more (the owner's test, 2026-10-12). A sound of another film or grid is
// left alone.
//
// A picture's init moves no sound: it says nothing of where its player is, only where the playback
// last reported, the film's start for one that jumped before its report, and started the sound there,
// cutting the run the jump needed every two seconds (the owner's test, 2026-10-14).
func (service *Service) WantAlongside(ctx context.Context, picture, sound StoreFile) error {
	if !picture.isSought || picture.isInit || sound.fileID != picture.fileID || sound.kind != picture.kind {
		return nil
	}
	return service.askedFor(ctx, sound.fileID, sound.kind, sound.stream, picture.runFrom)
}

// AsSought is the file asked for by a player loading where a seek took it, its person's or its
// group's, which says so: never a catch-up's, which waits for the run already making it (D250 §5,
// D284 §2).
func (file StoreFile) AsSought() StoreFile {
	file.isSought = true
	return file
}

// CoveringRun is the run of a file's store that will make the chunk the file waits for (Want), as
// far as it has made it now, and whether there is one: a chunk asked for far past what its run has
// made waits for the run to get there.
func (service *Service) CoveringRun(file StoreFile) (RunSpan, bool) {
	return service.runs.covering(file.storeDir, file.runFrom)
}

// StoreDir is the directory of the file's store, <file_id>/chunks-<stream>/<grid_id>: its film,
// stream and grid.
func (file StoreFile) StoreDir() string { return file.storeDir }

// WantedChunk is the chunk a run must make for the file: the chunk itself, or, for the init, the
// chunk the playback is at.
func (file StoreFile) WantedChunk() int { return file.runFrom }

// IsSought is whether a player asked for it loading where a seek took it (AsSought).
func (file StoreFile) IsSought() bool { return file.isSought }

// IsInit is whether the file is its store's init rather than a chunk.
func (file StoreFile) IsInit() bool { return file.isInit }

// Served records a file of a store its node served: a chunk is known made, so no run is queued for
// it, and a store a run is making stays wanted while its player reads it (D234 §3). A chunk whose
// next is known made or on its way, inside a run of this server's, looks a minute ahead as a chunk
// asked for before it was made does (Ensure), so the next run starts before the player reaches it:
// a run made faster than played otherwise made the player wait at each run's start (D247 §10).
// After a restart nothing past a served chunk is known, and nothing the node already has is made
// again.
func (service *Service) Served(ctx context.Context, file StoreFile) error {
	if !file.isInit {
		service.runs.served(file.storeDir, file.chunk)
	}
	if !file.isInit && service.runs.isCovered(file.storeDir, file.chunk+1) {
		return service.askedFor(ctx, file.fileID, file.kind, file.stream, file.chunk)
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
