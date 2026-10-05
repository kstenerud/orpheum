// SPDX-License-Identifier: GPL-3.0-or-later

package readthrough

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Nodes is what a read-through asks of the node channel: nodes.Service.
type Nodes interface {
	library.NodeConnections
	// SlowestReadBytesPerSecond is the slowest a node reads a whole file, as its hello said; zero
	// when not known.
	SlowestReadBytesPerSecond(nodeID int64) int64
}

// Queue is the job queue: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
}

// Parts are what the read-through service is built from.
type Parts struct {
	Database *store.Database
	Cache    *store.Cache
	Queue    Queue
	Nodes    Nodes
	// Exchanges are how a read-through asks its node and takes its answer; the service records its
	// results with them.
	Exchanges *nodes.Exchanges
	Clock     clock.Clock
}

// Service has each video read through once, and keeps what the read learned by the file's payload
// hash: its sounds' loudness in persistent.db (D226), its picture's timeline in cache.db (D247 §10).
type Service struct {
	database  *store.Database
	cache     *store.Cache
	queue     Queue
	nodes     Nodes
	exchanges *nodes.Exchanges
	clock     clock.Clock
}

// NewService returns the read-through service, recording the nodes' read-throughs with exchanges.
func NewService(parts Parts) *Service {
	service := &Service{
		database: parts.Database, cache: parts.Cache, queue: parts.Queue, nodes: parts.Nodes,
		exchanges: parts.Exchanges, clock: parts.Clock,
	}
	parts.Exchanges.Record(nodeproto.CommandReadThrough, service.record)
	return service
}

// fileIDsPerSweepRead is how many file ids a sweep for files to read through reads at a time: an
// internal read size. Reading the ids in ranges keeps each query short, so the sweep never holds the
// database's read connection for long while a library of tens of thousands of files is looked at;
// a thousand is a few milliseconds' read. Any value gives the same answer.
const fileIDsPerSweepRead = 1000

// FilesToReadThrough are the live videos whose bytes lack what a read-through learns: the loudness
// of a video with sound, or the timeline of one with a picture. It leaves out a file under a root or
// disk that is unavailable, which is not there to read (D231 §5, D233 §10).
func (service *Service) FilesToReadThrough(ctx context.Context) ([]int64, error) {
	queries := service.database.Read()
	lastFileID, err := queries.GetMaxFileID(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the last file id: %w", err)
	}
	unavailability := library.NewUnavailability(queries)
	var ids []int64
	for after := int64(0); after < lastFileID; after += fileIDsPerSweepRead {
		rows, listErr := queries.ListFilesToReadThrough(ctx, sqlcgen.ListFilesToReadThroughParams{
			AfterID: after, ThroughID: after + fileIDsPerSweepRead,
		})
		if listErr != nil {
			return nil, fmt.Errorf("list files to read through: %w", listErr)
		}
		for _, row := range rows {
			isLacking, lackErr := service.lacksAnything(ctx, row)
			if lackErr != nil {
				return nil, lackErr
			}
			if !isLacking {
				continue
			}
			_, isUnavailable, underErr := unavailability.Under(ctx, row.NodeID, row.ReportedPathRaw)
			if underErr != nil {
				return nil, underErr
			}
			if !isUnavailable {
				ids = append(ids, row.ID)
			}
		}
	}
	return ids, nil
}

// lacksAnything is whether a listed file's bytes lack their loudness, which the listing says, or
// the timeline of their picture, which cache.db says.
func (service *Service) lacksAnything(ctx context.Context, row sqlcgen.ListFilesToReadThroughRow) (bool, error) {
	if row.LacksLoudness != 0 {
		return true, nil
	}
	if row.HasPicture == 0 {
		return false, nil
	}
	timelineCount, err := service.cache.Read().CountVideoTimelinesOfPayload(ctx, row.PayloadHash)
	if err != nil {
		return false, fmt.Errorf("look for the timeline of file %d: %w", row.ID, err)
	}
	return timelineCount == 0, nil
}

// QueueReadThroughs queues a read-through for every file FilesToReadThrough finds (D226 §2), behind
// everything a person is waiting on, and returns how many it queued.
func (service *Service) QueueReadThroughs(ctx context.Context) (int, error) {
	ids, err := service.FilesToReadThrough(ctx)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, id := range ids {
		added, enqueueErr := service.queue.Enqueue(ctx, jobs.Request{
			Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: id},
			Reason: "reads it through, for how loud it is and where its picture can be cut",
		})
		if enqueueErr != nil {
			return queued, enqueueErr
		}
		if added {
			queued++
		}
	}
	return queued, nil
}

// ReadForPlay queues a read-through of the file at play priority, for a person waiting on what it
// learns: a player that stalled on a film whose picture has no timeline yet (D247 §2). A
// read-through already queued for it by a sweep is raised to play priority. It reports whether it
// queued one.
func (service *Service) ReadForPlay(ctx context.Context, fileID int64) (bool, error) {
	added, err := service.queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: fileID}, Priority: jobs.PriorityPlayNow,
		Reason: "a player is waiting for where its picture can be cut",
	})
	if err != nil {
		return false, fmt.Errorf("queue a read-through of file %d: %w", fileID, err)
	}
	return added, nil
}

// HandleReadThrough runs one read-through job (jobs.KindReadThrough): the node that holds the file
// reads it through for what its bytes lack, the loudness of its audio tracks and the timeline of its
// picture, and the answer is kept by the file's payload hash when it arrives. A file whose bytes lack
// neither, as a second copy's do not, is not read again; FFmpeg's failure is kept and not retried,
// and a node's refusal is not kept and is retried.
func (service *Service) HandleReadThrough(ctx context.Context, job jobs.Job) error {
	queries := service.database.Read()
	file, err := library.PlayableFile(ctx, queries, job.FileID)
	if errors.Is(err, library.ErrNoFile) {
		return jobs.MarkPermanent(err)
	}
	if err != nil {
		return err
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return jobs.MarkPermanent(err)
	}
	command, err := service.whatIsLacking(ctx, file, probe)
	if err != nil {
		return err
	}
	if len(command.LoudnessTracks) == 0 && command.VideoTimeline == nil {
		return nil
	}
	located := library.LocatedOf(file)
	if parked := library.ParkIfUnavailable(ctx, queries, service.nodes, job, located); parked != nil {
		return parked
	}
	if command.Video, err = library.BytesFileOf(ctx, queries, located); err != nil {
		return jobs.MarkPermanent(err)
	}
	if service.nodes.SlowestReadBytesPerSecond(file.NodeID) <= 0 {
		return fmt.Errorf("node %d has not said how fast it reads, so a read-through of file %d cannot be "+
			"waited for", file.NodeID, file.ID)
	}
	return service.exchanges.Assign(ctx, file.NodeID, job, nodeproto.Command{
		Work: nodeproto.Work{Type: nodeproto.CommandReadThrough, ReadThrough: &command},
	})
}

// whatIsLacking is the read-through a file's bytes need, without its video: the loudness of every
// audio track when none is kept, and the timeline of its main picture when that has none.
func (service *Service) whatIsLacking(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, probe nodeproto.Probe,
) (nodeproto.ReadThrough, error) {
	command := nodeproto.ReadThrough{DurationMs: probe.Format.DurationMs}
	loudnessRows, err := service.database.Read().ListLoudnessOfPayload(ctx, file.PayloadHash)
	if err != nil {
		return command, fmt.Errorf("read the loudness of file %d: %w", file.ID, err)
	}
	if len(loudnessRows) == 0 {
		for _, stream := range decide.AudioStreams(probe) {
			command.LoudnessTracks = append(command.LoudnessTracks, nodeproto.LoudnessTrack{
				StreamIndex: stream.StreamIndex, ChannelCount: stream.Audio.ChannelCount,
			})
		}
	}
	picture, hasPicture := decide.MainVideo(probe)
	if !hasPicture {
		return command, nil
	}
	_, err = store.OrNotFound(service.cache.Read().GetVideoTimeline(ctx, cachegen.GetVideoTimelineParams{
		PayloadHash: file.PayloadHash, VideoStreamIndex: int64(picture.StreamIndex),
	}))
	switch {
	case errors.Is(err, store.ErrNotFound):
		command.VideoTimeline = &nodeproto.VideoTimelineAsk{VideoStreamIndex: picture.StreamIndex}
	case err != nil:
		return command, fmt.Errorf("read the timeline of file %d: %w", file.ID, err)
	}
	return command, nil
}

// Assignee is how a read-through given to a node is resolved; its recorder keeps what it learned,
// or FFmpeg's word, by the file's payload hash (record).
func (service *Service) Assignee(runner nodes.Resolver) nodes.Assignee {
	keepFailure := func(ctx context.Context, attempt nodes.Attempt) func(failure string) error {
		return func(failure string) error {
			command := attempt.Work.ReadThrough
			if command == nil {
				return nil
			}
			payloadHash, err := hex.DecodeString(command.Video.PayloadHash)
			if err != nil {
				return fmt.Errorf("reading through a file with payload hash %q: %w", command.Video.PayloadHash, err)
			}
			return service.keep(ctx, payloadHash, *command, nodeproto.ReadThroughResult{Failure: failure})
		}
	}
	return nodes.Assignee{
		Runner: runner,
		Answer: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			about := fmt.Sprintf("reading file %d through", job.FileID)
			if result.ReadThrough == nil {
				return fmt.Errorf("node %d answered %s with something other than a read-through", attempt.NodeID,
					about)
			}
			answer := *result.ReadThrough
			switch {
			case answer.Refusal != "":
				// A refusal says nothing of the file's bytes (a full disk, a file changed since the
				// scan): nothing is kept, and the job is tried again on its back-off, as when the node
				// says nothing. Once it gives up, the next sweep asks again, of the file as it is then.
				return nodes.RefusalError(attempt.NodeID, about, answer.Refusal, answer.RefusalCode)
			case answer.Stall != nil:
				return nodes.FFmpegStallOutcome(job, attempt.NodeID, about, *answer.Stall, keepFailure(ctx, attempt))
			case answer.Failure != "":
				return jobs.MarkPermanent(errors.New(answer.Failure))
			}
			return nil
		},
		Unanswered: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, err error) error {
			return nodes.UnansweredOutcome(job, err, keepFailure(ctx, attempt))
		},
	}
}

// record keeps what a read-through learned of a file, or FFmpeg's word that it cannot be read, by
// the file's payload hash: the read-through's one recording, whether or not a job still waits for
// it (D238 §2).
func (service *Service) record(ctx context.Context, _ int64, result nodeproto.Result) error {
	answer, command := result.ReadThrough, result.Work.ReadThrough
	if answer == nil || command == nil {
		return fmt.Errorf("a read-through's result carries no findings, or names no read-through: %w",
			nodes.ErrMalformedResult)
	}
	if answer.Refusal != "" || answer.Stall != nil {
		return nil
	}
	payloadHash, err := hex.DecodeString(command.Video.PayloadHash)
	if err != nil {
		return fmt.Errorf("%w: a payload hash %q: %w", nodes.ErrMalformedResult, command.Video.PayloadHash, err)
	}
	return service.keep(ctx, payloadHash, *command, *answer)
}

// keep keeps a node's answer by the file's payload hash, each part the command asked for: each
// track's sounds and the picture's timeline, or, when FFmpeg could not read the file, its words, for
// each track as stereo and for the picture, so the sweep does not ask again.
func (service *Service) keep(
	ctx context.Context, payloadHash []byte, command nodeproto.ReadThrough, answer nodeproto.ReadThroughResult,
) error {
	if err := service.keepLoudness(ctx, payloadHash, command.LoudnessTracks, answer); err != nil {
		return err
	}
	if command.VideoTimeline == nil {
		return nil
	}
	return service.keepTimeline(ctx, payloadHash, command.VideoTimeline.VideoStreamIndex, answer)
}

// keepLoudness keeps each measured track's sounds, or FFmpeg's word for each asked track as stereo.
func (service *Service) keepLoudness(
	ctx context.Context, payloadHash []byte, tracks []nodeproto.LoudnessTrack, answer nodeproto.ReadThroughResult,
) error {
	now := service.clock.Now().UnixMilli()
	var rows []sqlcgen.UpsertLoudnessParams
	if answer.Failure != "" {
		for _, track := range tracks {
			rows = append(rows, sqlcgen.UpsertLoudnessParams{
				PayloadHash: payloadHash, StreamIndex: int64(track.StreamIndex), MeasuredMix: decide.MixStereo,
				Failure: &answer.Failure, MeasuredAt: now,
			})
		}
	}
	for _, track := range answer.Loudness {
		rows = append(rows, loudnessRow(payloadHash, track.StreamIndex, decide.MixStereo, track.Stereo, now))
		if track.Multichannel != nil {
			rows = append(rows,
				loudnessRow(payloadHash, track.StreamIndex, decide.MixMultichannel, *track.Multichannel, now))
		}
	}
	if len(rows) == 0 {
		return nil
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, each := range rows {
			if err := queries.UpsertLoudness(ctx, each); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("keep the loudness: %w", err)
	}
	return nil
}

// loudnessRow is one measured sound as persistent.db keeps it.
func loudnessRow(
	payloadHash []byte, streamIndex int, mix string, measured nodeproto.Measurement, now int64,
) sqlcgen.UpsertLoudnessParams {
	return sqlcgen.UpsertLoudnessParams{
		PayloadHash: payloadHash, StreamIndex: int64(streamIndex), MeasuredMix: mix,
		IntegratedLUFS: &measured.IntegratedLUFS, RangeLU: &measured.RangeLU, TruePeakDBTP: measured.TruePeakDBTP,
		MeasuredAt: now,
	}
}

// keepTimeline keeps the picture's timeline, or FFmpeg's word that the file could not be read.
// An answer that lacks the timeline it was asked for keeps nothing, and the next sweep asks again.
func (service *Service) keepTimeline(
	ctx context.Context, payloadHash []byte, videoStreamIndex int, answer nodeproto.ReadThroughResult,
) error {
	row := cachegen.UpsertVideoTimelineParams{
		PayloadHash: payloadHash, VideoStreamIndex: int64(videoStreamIndex), KeyframesMsJSON: "[]",
		BytesInEachSecondJSON: "[]", MadeAt: service.clock.Now().UnixMilli(),
	}
	switch {
	case answer.Failure != "":
		row.Failure = &answer.Failure
	case answer.VideoTimeline != nil:
		keyframes, err := json.Marshal(answer.VideoTimeline.KeyframesMs)
		if err != nil {
			return fmt.Errorf("encode the keyframes: %w", err)
		}
		seconds, err := json.Marshal(answer.VideoTimeline.BytesInEachSecond)
		if err != nil {
			return fmt.Errorf("encode the bytes in each second: %w", err)
		}
		row.KeyframesMsJSON, row.BytesInEachSecondJSON = string(keyframes), string(seconds)
	default:
		return nil
	}
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.UpsertVideoTimeline(ctx, row)
	})
	if err != nil {
		return fmt.Errorf("keep the timeline: %w", err)
	}
	return nil
}
