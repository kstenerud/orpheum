// SPDX-License-Identifier: GPL-3.0-or-later

package loudness

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Settings are the settings loudness reads.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Nodes is what loudness asks of the node channel: nodes.Service.
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

// Parts are what the loudness service is built from.
type Parts struct {
	Database *store.Database
	Queue    Queue
	Nodes    Nodes
	// Exchanges are how a measurement asks its node and takes its answer; the service records its
	// results with them.
	Exchanges *nodes.Exchanges
	Settings  Settings
	Clock     clock.Clock
}

// Service measures how loud each file's sounds are, keeps the measurements by the file's payload
// hash, and says the gain that levels each (D226).
type Service struct {
	database  *store.Database
	queue     Queue
	nodes     Nodes
	exchanges *nodes.Exchanges
	settings  Settings
	clock     clock.Clock
}

// NewService returns the loudness service, recording the nodes' measurements with exchanges.
func NewService(parts Parts) *Service {
	service := &Service{
		database: parts.Database, queue: parts.Queue, nodes: parts.Nodes, exchanges: parts.Exchanges,
		settings: parts.Settings, clock: parts.Clock,
	}
	parts.Exchanges.Record(nodeproto.CommandMeasureLoudness, service.record)
	return service
}

// aim reads the level and the most a sound is raised from their settings.
func (service *Service) aim(ctx context.Context) (decide.LoudnessAim, error) {
	target, err := service.settings.Int(ctx, settings.LoudnessTargetLUFS)
	if err != nil {
		return decide.LoudnessAim{}, fmt.Errorf("read loudness_target_lufs: %w", err)
	}
	maxRaise, err := service.settings.Int(ctx, settings.LoudnessMaxRaiseDB)
	if err != nil {
		return decide.LoudnessAim{}, fmt.Errorf("read loudness_max_raise_db: %w", err)
	}
	return decide.LoudnessAim{TargetLUFS: float64(target), MaxRaiseDB: float64(maxRaise)}, nil
}

// Levels are the gains that level each measured sound of the bytes with payloadHash.
func (service *Service) Levels(ctx context.Context, payloadHash []byte) (decide.Levels, error) {
	aim, err := service.aim(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := service.database.Read().ListLoudnessOfPayload(ctx, payloadHash)
	if err != nil {
		return nil, fmt.Errorf("read the loudness: %w", err)
	}
	levels := make(decide.Levels, len(rows))
	for _, row := range rows {
		measured := decide.Measured{
			IntegratedLUFS: row.IntegratedLUFS, TruePeakDBTP: row.TruePeakDBTP, IsFailure: row.Failure != nil,
		}
		if level, isMeasured := decide.LevellingOf(measured, aim); isMeasured {
			levels[decide.SoundKey{StreamIndex: row.StreamIndex, Mix: row.MeasuredMix}] = level
		}
	}
	return levels, nil
}

// fileIDsPerSweepRead is how many file ids a sweep for files to measure reads at a time: an internal
// read size, which bounds one query's work; any value works.
const fileIDsPerSweepRead = 1000

// FilesToMeasure are the live videos with sound whose bytes have no loudness yet, leaving out a file
// under a root or disk that is unavailable, which is not there to measure (D231 §5, D233 §10).
func (service *Service) FilesToMeasure(ctx context.Context) ([]int64, error) {
	queries := service.database.Read()
	lastFileID, err := queries.GetMaxFileID(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the last file id: %w", err)
	}
	unavailability := library.NewUnavailability(queries)
	var ids []int64
	for after := int64(0); after < lastFileID; after += fileIDsPerSweepRead {
		rows, listErr := queries.ListFilesToMeasure(ctx, sqlcgen.ListFilesToMeasureParams{
			AfterID: after, ThroughID: after + fileIDsPerSweepRead,
		})
		if listErr != nil {
			return nil, fmt.Errorf("list files to measure: %w", listErr)
		}
		for _, row := range rows {
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

// QueueMeasurements queues a measurement for every file FilesToMeasure finds (D226 §2), behind
// everything a person is waiting on, and returns how many it queued.
func (service *Service) QueueMeasurements(ctx context.Context) (int, error) {
	ids, err := service.FilesToMeasure(ctx)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, id := range ids {
		added, enqueueErr := service.queue.Enqueue(ctx, jobs.Request{
			Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: id},
			Reason: "measures how loud it is, so it plays at the level of the others",
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

// HandleMeasureLoudness runs one measurement job (jobs.KindMeasureLoudness): the node that holds the
// file measures its audio tracks, and this waits for its answer and keeps it by the file's payload
// hash. A measurement says nothing until it ends, so its node is waited on for the silence the other
// jobs allow and the time to read the file at the slowest the node's hello says it reads. A file
// whose bytes are already measured, as a second copy's are, is not measured again; FFmpeg's failure
// is kept and not retried, and a node's refusal is not kept and is retried.
func (service *Service) HandleMeasureLoudness(ctx context.Context, job jobs.Job) error {
	queries := service.database.Read()
	file, err := library.PlayableFile(ctx, queries, job.FileID)
	if errors.Is(err, library.ErrNoFile) {
		return jobs.MarkPermanent(err)
	}
	if err != nil {
		return err
	}
	loudnessRows, err := queries.ListLoudnessOfPayload(ctx, file.PayloadHash)
	if err != nil {
		return fmt.Errorf("read the loudness of file %d: %w", file.ID, err)
	}
	if len(loudnessRows) > 0 {
		return nil
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return jobs.MarkPermanent(err)
	}
	command := nodeproto.MeasureLoudness{}
	for _, stream := range decide.AudioStreams(probe) {
		command.Tracks = append(command.Tracks, nodeproto.LoudnessTrack{
			StreamIndex: stream.StreamIndex, ChannelCount: stream.Audio.ChannelCount,
		})
	}
	if len(command.Tracks) == 0 {
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
		return fmt.Errorf("node %d has not said how fast it reads, so a measurement of file %d cannot be "+
			"waited for", file.NodeID, file.ID)
	}
	return service.exchanges.Assign(ctx, file.NodeID, job, nodeproto.Command{
		Work: nodeproto.Work{Type: nodeproto.CommandMeasureLoudness, MeasureLoudness: &command},
	})
}

// Assignee is how a measurement given to a node is resolved; its recorder keeps the measurement, or
// FFmpeg's word, by the file's payload hash (record).
func (service *Service) Assignee(runner nodes.Resolver) nodes.Assignee {
	keepFailure := func(ctx context.Context, attempt nodes.Attempt) func(failure string) error {
		return func(failure string) error {
			measure := attempt.Work.MeasureLoudness
			if measure == nil {
				return nil
			}
			payloadHash, err := hex.DecodeString(measure.Video.PayloadHash)
			if err != nil {
				return fmt.Errorf("measuring a file with payload hash %q: %w", measure.Video.PayloadHash, err)
			}
			return service.keep(ctx, payloadHash, measure.Tracks, nodeproto.LoudnessResult{Failure: failure})
		}
	}
	return nodes.Assignee{
		Runner: runner,
		Answer: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			about := fmt.Sprintf("measuring file %d", job.FileID)
			if result.Loudness == nil {
				return fmt.Errorf("node %d answered %s with something other than loudness", attempt.NodeID, about)
			}
			answer := *result.Loudness
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

// record keeps a file's measurement, or FFmpeg's word that it cannot be measured, by the file's
// payload hash: the measurement's one recording, whether or not a job still waits for it (D238 §2).
func (service *Service) record(ctx context.Context, _ int64, result nodeproto.Result) error {
	answer, command := result.Loudness, result.Work.MeasureLoudness
	if answer == nil || command == nil {
		return fmt.Errorf("a measurement's result carries no loudness, or names no measurement: %w",
			nodes.ErrMalformedResult)
	}
	if answer.Refusal != "" || answer.Stall != nil {
		return nil
	}
	payloadHash, err := hex.DecodeString(command.Video.PayloadHash)
	if err != nil {
		return fmt.Errorf("%w: a payload hash %q: %w", nodes.ErrMalformedResult, command.Video.PayloadHash, err)
	}
	return service.keep(ctx, payloadHash, command.Tracks, *answer)
}

// keep keeps a node's answer by the file's payload hash: each track's sounds, or, when FFmpeg could
// not measure the file, its words, for each track as stereo, so the sweep does not ask again.
func (service *Service) keep(
	ctx context.Context, payloadHash []byte, tracks []nodeproto.LoudnessTrack, answer nodeproto.LoudnessResult,
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
	for _, track := range answer.Tracks {
		rows = append(rows, row(payloadHash, track.StreamIndex, decide.MixStereo, track.Stereo, now))
		if track.Multichannel != nil {
			rows = append(rows, row(payloadHash, track.StreamIndex, decide.MixMultichannel, *track.Multichannel, now))
		}
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

// row is one measured sound as persistent.db keeps it.
func row(
	payloadHash []byte, streamIndex int, mix string, measured nodeproto.Measurement, now int64,
) sqlcgen.UpsertLoudnessParams {
	return sqlcgen.UpsertLoudnessParams{
		PayloadHash: payloadHash, StreamIndex: int64(streamIndex), MeasuredMix: mix,
		IntegratedLUFS: &measured.IntegratedLUFS, RangeLU: &measured.RangeLU, TruePeakDBTP: measured.TruePeakDBTP,
		MeasuredAt: now,
	}
}
