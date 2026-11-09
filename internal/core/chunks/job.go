// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// ErrGridChanged is a run asked for on a store the film no longer has: its file changed, the
// segment length did, or the facts of its probe the stream is made from (StoreKey), and its chunks
// are another store's.
var ErrGridChanged = errors.New("the film is no longer cut on the grid the run was asked for")

// storedRun is a make_chunks job's run: its store, as the chunk_store row the job targets names it,
// and its chunks.
type storedRun struct {
	fileID   int64
	stream   Stream
	storeKey string
	runArgs
}

// dir is the run's store's directory.
func (run storedRun) dir() string { return StoreDir(run.fileID, run.stream, run.storeKey) }

// runOf is the run a make_chunks job makes, failing for good for a job whose store or arguments
// name none.
func (service *Service) runOf(ctx context.Context, job jobs.Job) (storedRun, error) {
	var run storedRun
	if err := strictjson.Unmarshal([]byte(job.ArgsJSON), &run.runArgs); err != nil {
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
	run.fileID, run.stream, run.storeKey = row.SourceFile, stream, row.StoreKey
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
	film, isCut, err := service.cutForRun(ctx, run)
	switch {
	case errors.Is(err, library.ErrNoFile), errors.Is(err, ErrCannotBeCut):
		return jobs.MarkPermanent(err)
	case err != nil:
		return err
	case !isCut || film.storeKey(run.stream) != run.storeKey:
		return jobs.MarkPermanent(fmt.Errorf("file %d, store %s: %w", run.fileID, run.storeKey, ErrGridChanged))
	case run.FirstChunk < 0 || run.LastChunk < run.FirstChunk || run.LastChunk > film.grid.lastChunk():
		return jobs.MarkPermanent(fmt.Errorf("file %d has no chunks %d to %d", run.fileID, run.FirstChunk,
			run.LastChunk))
	}
	queries := service.database.Read()
	// At the first place on its store's node that can be read now: a copy beside it when the first's
	// disk does not answer (D363 §8).
	located, err := library.PlaceOnNodeToRead(ctx, queries, service.nodes, service.absences, film.file,
		film.file.NodeID)
	if err != nil {
		return jobs.MarkPermanent(err)
	}
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
	service.runs.given(command.StoreDir, run.FirstChunk, run.LastChunk, job.ID, film.file.NodeID,
		service.clock.Now())
	return service.exchanges.Assign(ctx, film.file.NodeID, job, nodeproto.Command{
		Work: nodeproto.Work{Type: nodeproto.CommandMakeChunks, MakeChunks: &command},
	})
}

// cutForRun is the run's film as it is cut on the grid the run was asked on: on its keyframes, or,
// when that is not the run's grid or its keyframes cannot cut it, on time (D269 §3, D344 §2);
// reporting false when the film has neither.
func (service *Service) cutForRun(ctx context.Context, run storedRun) (cutFilm, bool, error) {
	film, isCut, err := service.cut(ctx, run.fileID, OnKeyframes)
	hasUncutPicture := errors.Is(err, ErrCannotBeCut) && film.picture.Codec != ""
	if (err != nil && !hasUncutPicture) || (isCut && film.storeKey(run.stream) == run.storeKey) {
		return film, isCut, err
	}
	timed, isTimed, timeErr := service.cut(ctx, run.fileID, OnTime)
	if timeErr == nil && isTimed && timed.storeKey(run.stream) == run.storeKey {
		return timed, true, nil
	}
	if hasUncutPicture {
		return film, false, err
	}
	return film, isCut, nil
}

// makeChunks is the node's command for a run, but its source: the film's main picture, never a
// cover; the stream's recipe; its store; and the grid's bounds from the run's first chunk's start to
// its last's end. A sound past the film's first chunk starts a chunk early, which the node drops
// with its encoder's priming (slow-links.md §10). A picture, copied or encoded, whose probe says its
// B-frames carry no display times, is re-timed (nodeproto.ProbeVideo.NeedsRetiming).
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
		command.NeedsPictureRetiming = film.picture.Video.NeedsRetiming
	case nodeproto.StreamSound:
		track, err := service.levelledTrack(ctx, film, stream)
		if err != nil {
			return command, err
		}
		command.Stream.Sound, command.Stream.IsBoosted = &track, stream.IsBoosted
		command.Stream.SoundKbps = stream.SoundKbps
		if args.FirstChunk > 0 {
			command.LeadInMs = film.grid.StartsMs[args.FirstChunk-1]
		}
		command.IsSoundInterleaved = isSoundInterleaved(film.probe, track.AudioTrackIndex)
	case nodeproto.StreamSoundCopied:
		track, err := copiedTrack(film, stream)
		if err != nil {
			return command, err
		}
		command.Stream.Sound = &track
		if args.FirstChunk > 0 {
			command.LeadInMs = film.grid.StartsMs[args.FirstChunk-1]
		}
	case nodeproto.StreamPicture:
		command.NeedsPictureRetiming = film.picture.Video.NeedsRetiming
		// Read from the chunk before, dropped, so FFmpeg times the pictures of an open GOP shown before
		// the run's first keyframe as the film does (nodeproto.MakeChunks.LeadInMs).
		startMs := film.grid.StartsMs[args.FirstChunk]
		if args.FirstChunk > 0 {
			command.LeadInMs = film.grid.StartsMs[args.FirstChunk-1]
			startMs = command.LeadInMs
		}
		command.PictureSeekMs = PictureSeekMs(film.keyframesMs, startMs, film.grid.FilmDurationMs)
		if container, _ := decide.ContainerOf(film.probe.Format); container == decide.MPEGTS {
			command.IsPictureSeekBeforeKeyframe = true
			command.PictureSeekMs = PictureSeekBeforeMs(film.keyframesMs, startMs)
		}
	}
	return command, nil
}

// isSoundInterleaved is whether a film's audio track, by its place among them, is interleaved in
// super-blocks, which FFmpeg loses the one of that a seek lands in (idiosyncrasies.md): a RealMedia
// film's cook, ATRAC3, SIPR or RealAudio 28.8, by the demuxer's names (libavformat/rmdec.c: genr,
// Int4 and sipr). RealAudio 14.4 is not interleaved.
func isSoundInterleaved(probe nodeproto.Probe, audioTrackIndex int) bool {
	tracks := decide.AudioStreams(probe)
	if !slices.Contains(probe.Format.FormatNames, "rm") || audioTrackIndex < 0 || audioTrackIndex >= len(tracks) {
		return false
	}
	switch tracks[audioTrackIndex].Codec {
	case "cook", "atrac3", "sipr", "ra_288":
		return true
	}
	return false
}

// seekMovedEarlierMs is how much earlier FFmpeg moves an input seek into a file it does not seek by
// shown time, Matroska among them, when a picture has B-frames: 3/23 s, rounded up
// (fftools/ffmpeg_demux.c; docs/contributors/idiosyncrasies.md).
const seekMovedEarlierMs = 131

// PictureSeekMs is where a copied picture's run starting at the keyframe at startMs seeks, so
// FFmpeg lands on that keyframe and not the one before it: midway to the next keyframe, or to the
// film's end at durationMs, and at least seekMovedEarlierMs past the start, but always before the
// next keyframe (Kyoo seeks midway for the same reason; idiosyncrasies.md). With keyframes closer
// than that, a Matroska run still lands one early, and the node refuses its chunks.
func PictureSeekMs(keyframesMs []int64, startMs, durationMs int64) int64 {
	nextMs := durationMs
	after, _ := slices.BinarySearch(keyframesMs, startMs+1)
	if after < len(keyframesMs) && keyframesMs[after] < nextMs {
		nextMs = keyframesMs[after]
	}
	seekMs := max(startMs+(nextMs-startMs)/2, startMs+seekMovedEarlierMs)
	return min(seekMs, nextMs-1)
}

// PictureSeekBeforeMs is where a copied picture's run starting at the keyframe at startMs seeks in
// MPEG-TS, which FFmpeg seeks to the frame at or before a time, not to a keyframe, so a copy from
// a seek past the keyframe starts at the next one, and a run of the film's last keyframe has none
// (idiosyncrasies.md): midway back to the keyframe before it, and at least seekMovedEarlierMs past
// that one, as FFmpeg moves the seek so much earlier, but never past the start; with no keyframe
// before it, the film's start. A copy drops what comes before its first keyframe, so it starts at
// the run's.
func PictureSeekBeforeMs(keyframesMs []int64, startMs int64) int64 {
	at, _ := slices.BinarySearch(keyframesMs, startMs)
	if at == 0 {
		return 0
	}
	previousMs := keyframesMs[at-1]
	seekMs := max(previousMs+(startMs-previousMs)/2, previousMs+seekMovedEarlierMs)
	return min(seekMs, startMs)
}

// copiedTrack is a sound copied's audio track, as its chunks are copied of it: its place, language
// and channels, carrying no gain, which a copy cannot. A track the film does not have ends the job for
// good.
func copiedTrack(film cutFilm, stream Stream) (nodeproto.SoundTrack, error) {
	for _, track := range SoundTracksOf(film.probe) {
		if track.AudioTrackIndex == stream.AudioTrackIndex {
			return track, nil
		}
	}
	return nodeproto.SoundTrack{}, jobs.MarkPermanent(fmt.Errorf("file %d has no sound %s", film.file.ID,
		stream.Name()))
}

// levelledTrack is a sound stream's audio track as its chunks are made of it, with the levelling gain
// they carry (D226 §4), or, made as loud as the track's own sound, the gain that brings its mix there
// (D368 §1). A track the film does not have, or a boosted mix of a track with no centre to raise,
// ends the job for good.
func (service *Service) levelledTrack(
	ctx context.Context, film cutFilm, stream Stream,
) (nodeproto.SoundTrack, error) {
	tracks := SoundTracksOf(film.probe)
	if !stream.IsUnlevelled {
		levels, err := service.loudness.Levels(ctx, film.file.PayloadHash)
		if err != nil {
			return nodeproto.SoundTrack{}, err
		}
		decide.LevelSounds(tracks, levels, film.probe)
		if stream.IsAtOwnLevel {
			for index := range tracks {
				tracks[index].CarriedGainDB = decide.OwnLevelGainDB(levels, film.probe, tracks[index].AudioTrackIndex)
			}
		}
	}
	if hasSound(SoundsOf(tracks), stream) {
		for _, track := range tracks {
			if track.AudioTrackIndex == stream.AudioTrackIndex {
				return track, nil
			}
		}
	}
	return nodeproto.SoundTrack{}, jobs.MarkPermanent(fmt.Errorf("file %d has no sound %s", film.file.ID,
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
		Answer: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			command := attempt.Work.MakeChunks
			if command == nil {
				return jobs.MarkPermanent(fmt.Errorf("%s job %d's attempt names no run", attempt.Kind, attempt.JobID))
			}
			if result.MakeChunks == nil {
				return fmt.Errorf("node %d answered the run of %s with something other than a run's report",
					attempt.NodeID, command.StoreDir)
			}
			return service.takeEnd(ctx, job, attempt.NodeID, *command, *result.MakeChunks)
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

// forgetRun forgets the run an attempt was, which will make nothing more, and that its job was
// cancelled, which its node will say nothing more of.
func (service *Service) forgetRun(attempt nodes.Attempt) {
	service.runs.ended(attempt.JobID)
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
	now := service.clock.Now()
	service.runs.begin(command.StoreDir, command.FirstChunk, last, attempt.JobID, attempt.NodeID, now)
	service.runs.madeThrough(command.StoreDir, command.FirstChunk, madeThrough)
	if madeThrough >= command.FirstChunk {
		madeEndMs := command.BoundsMs[madeThrough-command.FirstChunk+1]
		service.runs.sample(command.StoreDir, command.FirstChunk, now, madeEndMs)
	}
	share := float64(madeThrough-command.FirstChunk+1) / float64(last-command.FirstChunk+1)
	if err := service.queue.SetProgress(ctx, attempt.JobID, max(share, 0)); err != nil {
		return err
	}
	// Each step of a run looks again at who wants its film's runs: an interest lapses unsaid (D309 §3).
	if err := service.StopUnwanted(ctx, fileOfStore(command.StoreDir)); err != nil {
		return err
	}
	return service.keepRunsFast(ctx, attempt.NodeID)
}

// takeEnd takes the node's word on how a run ended, and says how the job ended: done; FFmpeg
// failing on the source, which fails again; stalled, tried again once; refused for want of room,
// parked until the node's disk has it; refused for its film not at the place read, tried again at
// once at a copy beside it (readElsewhere); or refused otherwise, tried again. Whatever the run made
// before it ended is known made.
func (service *Service) takeEnd(
	ctx context.Context, job jobs.Job, nodeID int64, command nodeproto.MakeChunks, report nodeproto.MakeChunksReport,
) error {
	service.runs.ended(job.ID)
	if report.LastChunk >= command.FirstChunk {
		service.runs.madeThrough(command.StoreDir, command.FirstChunk, report.LastChunk)
	}
	what := "the run of " + command.StoreDir
	switch report.State {
	case nodeproto.RunDone:
		service.runs.forget(command.StoreDir, command.FirstChunk)
		return nil
	case nodeproto.RunFailed:
		service.recordFailure(ctx, command.StoreDir, max(report.LastChunk+1, command.FirstChunk), report.Failure)
		return jobs.MarkPermanent(errors.New(report.Failure))
	case nodeproto.RunStalled:
		return nodes.FFmpegStallOutcome(job, nodeID, what, ptr.ValueOrZero(report.Stall),
			func(string) error {
				service.runs.forget(command.StoreDir, command.FirstChunk)
				return nil
			})
	case nodeproto.RunRefused:
		refused := nodes.RefusalError(nodeID, what, report.Refusal, report.RefusalCode)
		if report.RefusalCode == nodeproto.RefusedNoRoom {
			// Its prepared directory's disk is full: the node's runs wait until its status shows
			// room (D231 §5).
			return jobs.Park(jobs.Parking{
				Destination: jobs.PreparedDirectoryDestination(nodeID), Reason: "its disk has no room",
				AwaitsSignal: true,
			}, refused)
		}
		if report.RefusalCode == nodeproto.RefusedGone || report.RefusalCode == nodeproto.RefusedChanged {
			return service.readElsewhere(ctx, nodeID, command, refused)
		}
		return refused
	}
	return fmt.Errorf("node %d ended %s in a state this core does not know: %q", nodeID, what, report.State)
}

// hasSound is whether a film whose sounds are those of its audio tracks (SoundsOf) has the made sound
// stream, at whatever bit rate, levelled, unlevelled or as loud as its track's own: a track it has,
// and a boosted mix only of a track with a centre to raise.
func hasSound(sounds []Sound, stream Stream) bool {
	if stream.Kind != nodeproto.StreamSound || stream.IsUnlevelled && stream.IsAtOwnLevel {
		return false
	}
	return slices.ContainsFunc(sounds, func(sound Sound) bool {
		return sound.AudioTrackIndex == stream.AudioTrackIndex && sound.IsBoosted == stream.IsBoosted
	})
}

// readElsewhere answers a run its node refused because its film is not at the place it read, or is
// not the file named there (D100): the place is noted absent, so the film is read at another of its
// places on the node until a scan sees it there again, and with one the run is tried again at once,
// at no cost of an attempt; with none it is the refusal, tried again on the job's back-off (D363 §8).
// A play whose first place was renamed away waited on runs refused there though a copy stood beside
// it (staging, 2026-10-27, VQ81).
func (service *Service) readElsewhere(
	ctx context.Context, nodeID int64, command nodeproto.MakeChunks, refused error,
) error {
	read := command.Video
	place := []byte(strings.TrimSuffix(read.RootPath, "/") + "/" + string(read.Path))
	service.absences.Note(ctx, nodeID, place, refused.Error())
	queries := service.database.Read()
	file, err := library.PlayableFile(ctx, queries, fileOfStore(command.StoreDir))
	if err != nil {
		return errors.Join(refused, err)
	}
	next, err := library.PlaceOnNodeToRead(ctx, queries, service.nodes, service.absences, file, nodeID)
	if err != nil || service.absences.IsAbsent(nodeID, next.ReportedPathRaw) {
		return refused
	}
	service.logger.InfoContext(ctx, "a run of chunks found its film not at a place; it reads another place "+
		"of it on the node", "nodeId", nodeID, "storeDir", command.StoreDir, "from", string(place),
		"to", string(next.ReportedPathRaw))
	return jobs.Bump(refused)
}
