// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/renditions"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
)

// ErrGridChanged is a run asked for on a grid the film no longer has: its file changed, or the
// segment length did, and its chunks are another store's.
var ErrGridChanged = errors.New("the film is no longer cut on the grid the run was asked for")

// storedRun is a make_chunks job's run: its store, as the chunk_store row the job targets names it,
// and its chunks.
type storedRun struct {
	fileID int64
	stream Stream
	gridID string
	runArgs
}

// dir is the run's store's directory.
func (run storedRun) dir() string { return StoreDir(run.fileID, run.stream, run.gridID) }

// runOf is the run a make_chunks job makes, failing for good for a job whose store or arguments
// name none.
func (service *Service) runOf(ctx context.Context, job jobs.Job) (storedRun, error) {
	var run storedRun
	if err := json.Unmarshal([]byte(job.ArgsJSON), &run.runArgs); err != nil {
		return run, jobs.MarkPermanent(fmt.Errorf("job %d's arguments: %w", job.ID, err))
	}
	row, err := store.OrNotFound(service.cache.Read().GetChunkStore(ctx, job.ChunkStoreID))
	if errors.Is(err, store.ErrNotFound) {
		return run, jobs.MarkPermanent(fmt.Errorf("no chunk store %d", job.ChunkStoreID))
	}
	if err != nil {
		return run, fmt.Errorf("read chunk store %d: %w", job.ChunkStoreID, err)
	}
	stream, isStream := StreamNamed(row.Stream)
	if !isStream {
		return run, jobs.MarkPermanent(fmt.Errorf("chunk store %d names no stream: %q", row.ID, row.Stream))
	}
	run.fileID, run.stream, run.gridID = row.SourceFile, stream, row.GridID
	return run, nil
}

// HandleMakeChunks runs one make_chunks job (jobs.KindMakeChunks): the node that holds the film
// makes the run of chunks of the store the job targets that its arguments name, and says later how
// it ended (Assignee). A run on a grid the film no longer has, of a stream it cannot have, or of a
// file gone, ends for good.
func (service *Service) HandleMakeChunks(ctx context.Context, job jobs.Job) error {
	run, err := service.runOf(ctx, job)
	if err != nil {
		return err
	}
	film, isCut, err := service.cut(ctx, run.fileID)
	switch {
	case errors.Is(err, library.ErrNoFile), errors.Is(err, ErrCannotBeCut):
		return jobs.MarkPermanent(err)
	case err != nil:
		return err
	case !isCut || film.grid.ID() != run.gridID:
		return jobs.MarkPermanent(fmt.Errorf("file %d, grid %s: %w", run.fileID, run.gridID, ErrGridChanged))
	case run.FirstChunk < 0 || run.LastChunk < run.FirstChunk || run.LastChunk > film.grid.lastChunk():
		return jobs.MarkPermanent(fmt.Errorf("file %d has no chunks %d to %d", run.fileID, run.FirstChunk,
			run.LastChunk))
	}
	queries := service.database.Read()
	located := library.LocatedOf(film.file)
	if parked := library.ParkIfUnavailable(ctx, queries, service.nodes, job, located); parked != nil {
		return parked
	}
	source, err := library.BytesFileOf(ctx, queries, located)
	if err != nil {
		return jobs.MarkPermanent(err)
	}
	command, err := service.makeChunks(ctx, film, run)
	if err != nil {
		return err
	}
	command.Video = source
	service.runs.knowStoreID(command.StoreDir, job.ChunkStoreID)
	service.runs.begin(command.StoreDir, run.FirstChunk, run.LastChunk, job.ID)
	return service.exchanges.Assign(ctx, film.file.NodeID, job, nodeproto.Command{
		Work: nodeproto.Work{Type: nodeproto.CommandMakeChunks, MakeChunks: &command},
	})
}

// makeChunks is the node's command for a run, but its source: the film's main picture, never a
// cover; the stream's recipe; its store; and the grid's bounds from the run's first chunk's start to
// its last's end. A sound past the film's first chunk starts a chunk early, which the node drops
// with its encoder's priming (slow-links.md §10).
func (service *Service) makeChunks(ctx context.Context, film cutFilm, run storedRun) (nodeproto.MakeChunks, error) {
	stream, args := run.stream, run.runArgs
	command := nodeproto.MakeChunks{
		VideoStreamIndex: film.picture.StreamIndex, VideoCodec: film.picture.Codec,
		Stream:     nodeproto.ChunkStream{Kind: stream.Kind},
		StoreDir:   run.dir(),
		FirstChunk: args.FirstChunk,
	}
	for n := args.FirstChunk; n <= args.LastChunk; n++ {
		command.BoundsMs = append(command.BoundsMs, film.grid.StartsMs[n])
	}
	command.BoundsMs = append(command.BoundsMs, film.grid.endMs(args.LastChunk))
	switch stream.Kind {
	case nodeproto.StreamQuality:
		command.Stream.BitRateKbps, command.Stream.Height = stream.Quality.Kbps, stream.Quality.Height
	case nodeproto.StreamSound:
		track, err := service.levelledTrack(ctx, film, stream)
		if err != nil {
			return command, err
		}
		command.Stream.Sound, command.Stream.IsBoosted = &track, stream.IsBoosted
		command.Stream.SoundKbps = stream.SoundKbps
		if args.FirstChunk > 0 {
			command.SoundLeadInMs = film.grid.StartsMs[args.FirstChunk-1]
		}
	case nodeproto.StreamPicture:
	}
	return command, nil
}

// levelledTrack is a sound stream's audio track as a prepare carries it, with the levelling gain
// a prepare puts in it (D226 §4): its chunks are the same mix a prepared copy's are. A track the
// film does not have, or a boosted mix of a track with no centre to raise, ends the job for good.
func (service *Service) levelledTrack(
	ctx context.Context, film cutFilm, stream Stream,
) (nodeproto.PrepareAudio, error) {
	levels, err := service.loudness.Levels(ctx, film.file.PayloadHash)
	if err != nil {
		return nodeproto.PrepareAudio{}, err
	}
	levelled := nodeproto.Prepare{AudioTracks: renditions.PreparedAudio(film.probe)}
	decide.LevelPrepare(&levelled, levels, film.probe)
	if hasSound(renditions.AudioRenditionsOf(levelled.AudioTracks), stream) {
		for _, track := range levelled.AudioTracks {
			if track.AudioTrackIndex == stream.AudioTrackIndex {
				return track, nil
			}
		}
	}
	return nodeproto.PrepareAudio{}, jobs.MarkPermanent(fmt.Errorf("file %d has no sound %s", film.file.ID,
		stream.Name()))
}

// RecordMakeChunksFailure forgets a run whose job failed for good, whatever ended it, so a player
// asking for its chunks queues another: the chunks are made on request, and a run lost is asked
// for again by the next (D231 §4).
func (service *Service) RecordMakeChunksFailure(ctx context.Context, job jobs.Job, _ string) error {
	// A job whose store or arguments name no run has no run to forget.
	if run, err := service.runOf(ctx, job); err == nil {
		service.runs.forget(run.dir(), run.FirstChunk)
	}
	return nil
}

// Assignee is how a run given to a node is resolved: by its node's word on how it ended, by its
// silence or restart, by its progress, chunk by chunk, and by a cancel.
func (service *Service) Assignee(runner nodes.Resolver) nodes.Assignee {
	return nodes.Assignee{
		Runner: runner,
		Answer: func(_ context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			command := attempt.Work.MakeChunks
			if command == nil {
				return jobs.MarkPermanent(fmt.Errorf("%s job %d's attempt names no run", attempt.Kind, attempt.JobID))
			}
			if result.MakeChunks == nil {
				return fmt.Errorf("node %d answered the run of %s with something other than a run's report",
					attempt.NodeID, command.StoreDir)
			}
			return service.takeEnd(job, attempt.NodeID, *command, *result.MakeChunks)
		},
		Unanswered: func(_ context.Context, job jobs.Job, attempt nodes.Attempt, err error) error {
			return nodes.UnansweredOutcome(job, err, func(string) error {
				service.forgetRun(attempt)
				return nil
			})
		},
		Progress: service.takeProgress,
		Cancelled: func(_ context.Context, attempt nodes.Attempt) error {
			service.forgetRun(attempt)
			return nil
		},
	}
}

// forgetRun forgets the run an attempt was, which will make nothing more.
func (service *Service) forgetRun(attempt nodes.Attempt) {
	if command := attempt.Work.MakeChunks; command != nil {
		service.runs.forget(command.StoreDir, command.FirstChunk)
	}
}

// takeProgress takes the node's word on the last chunk a run has made: the run tracker learns its
// chunks made, a core started again learns the run, and the job view its share of the run done.
func (service *Service) takeProgress(ctx context.Context, attempt nodes.Attempt, progress nodeproto.Progress) error {
	command := attempt.Work.MakeChunks
	if command == nil || progress.ChunksMadeThrough == nil || len(command.BoundsMs) < 2 {
		return nil
	}
	last := command.FirstChunk + len(command.BoundsMs) - 2
	madeThrough := min(*progress.ChunksMadeThrough, last)
	service.runs.begin(command.StoreDir, command.FirstChunk, last, attempt.JobID)
	service.runs.madeThrough(command.StoreDir, command.FirstChunk, madeThrough)
	share := float64(madeThrough-command.FirstChunk+1) / float64(last-command.FirstChunk+1)
	return service.queue.SetProgress(ctx, attempt.JobID, max(share, 0))
}

// takeEnd takes the node's word on how a run ended, and says how the job ended: done; FFmpeg
// failing on the source, which fails again; stalled, tried again once; refused for want of room,
// parked until the node's disk has it; or refused otherwise, tried again. Whatever the run made
// before it ended is known made.
func (service *Service) takeEnd(
	job jobs.Job, nodeID int64, command nodeproto.MakeChunks, report nodeproto.MakeChunksReport,
) error {
	if report.LastChunk >= command.FirstChunk {
		service.runs.madeThrough(command.StoreDir, command.FirstChunk, report.LastChunk)
	}
	what := "the run of " + command.StoreDir
	switch report.State {
	case nodeproto.PrepareDone:
		service.runs.forget(command.StoreDir, command.FirstChunk)
		return nil
	case nodeproto.PrepareFailed:
		return jobs.MarkPermanent(errors.New(report.Failure))
	case nodeproto.PrepareStalled:
		return nodes.FFmpegStallOutcome(job, nodeID, what, ptr.ValueOrZero(report.Stall),
			func(string) error {
				service.runs.forget(command.StoreDir, command.FirstChunk)
				return nil
			})
	case nodeproto.PrepareRefused:
		refused := nodes.RefusalError(nodeID, what, report.Refusal, report.RefusalCode)
		if report.RefusalCode == nodeproto.RefusedNoRoom {
			// Its prepared directory's disk is full: the node's runs wait until its status shows
			// room (D231 §5), as its prepares do.
			return jobs.Park(jobs.Parking{
				Destination: jobs.PreparedDirectoryDestination(nodeID), Reason: "its disk has no room",
				AwaitsSignal: true,
			}, refused)
		}
		return refused
	}
	return fmt.Errorf("node %d ended %s in a state this core does not know: %q", nodeID, what, report.State)
}

// hasSound is whether a film whose sounds are those a prepare makes of its audio tracks has the sound
// stream, at whatever bit rate: a track it has, and a boosted mix only of a track a prepare makes one
// of.
func hasSound(sounds []renditions.AudioRendition, stream Stream) bool {
	return slices.ContainsFunc(sounds, func(sound renditions.AudioRendition) bool {
		return soundStream(sound, stream.SoundKbps) == stream
	})
}
