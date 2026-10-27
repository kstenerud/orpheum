// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Nodes is what a read-through asks of the node channel: nodes.Service.
type Nodes interface {
	library.NodeConnections
	// SlowestReadBytesPerSecond is the slowest a node reads a whole file, as its hello said; zero
	// when not known.
	SlowestReadBytesPerSecond(nodeID int64) int64
	// ParkUnlessConnected parks a job on a node that is not connected, until its hello.
	ParkUnlessConnected(ctx context.Context, nodeID int64) error
}

// Queue is the job queue: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
	EnqueueBatch(ctx context.Context, batch []jobs.Request) (int, error)
	ReleaseWanted(ctx context.Context, kind jobs.Kind, target jobs.Target) error
	SetProgress(ctx context.Context, id int64, fraction float64) error
}

// Parts are what the read-through service is built from.
type Parts struct {
	Database *store.Database
	Queue    Queue
	Nodes    Nodes
	// Exchanges are how a read-through asks its node and takes its answer; the service records its
	// results with them.
	Exchanges *nodes.Exchanges
	Clock     clock.Clock
}

// Timings are told each node's full read-throughs: the bytes read and how long their reading took
// once it ran: waits.Estimator.
type Timings interface {
	ReadThroughFinished(nodeID, bytes int64, took time.Duration)
}

// Service has each video read through once, and keeps what the read learned by the file's payload
// hash in persistent.db: its sounds' loudness (D226) and its picture's timeline (D247 §10).
type Service struct {
	database  *store.Database
	queue     Queue
	nodes     Nodes
	exchanges *nodes.Exchanges
	clock     clock.Clock
	// timings are told how long each full read took, for how long the next will (D324 §6); nil tells
	// nothing (TimeReadsBy).
	timings Timings
	// noIndex are the pictures whose files a read from the index found none in, by payload hash and
	// stream index, in memory: a restart asks the node again, which answers at once (D269 §2).
	noIndex noIndexes
}

// noIndexes are the pictures whose files have no index of their keyframes.
type noIndexes struct {
	mutex sync.Mutex
	byKey map[string]bool // guarded by mutex
}

// noIndexKey names one picture of a file's bytes.
func noIndexKey(payloadHash []byte, videoStreamIndex int) string {
	return hex.EncodeToString(payloadHash) + "/" + strconv.Itoa(videoStreamIndex)
}

// NewService returns the read-through service, recording the nodes' read-throughs with exchanges.
func NewService(parts Parts) *Service {
	service := &Service{
		database: parts.Database, queue: parts.Queue, nodes: parts.Nodes,
		exchanges: parts.Exchanges, clock: parts.Clock, noIndex: noIndexes{byKey: map[string]bool{}},
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
	files, err := service.filesToReadThrough(ctx)
	ids := make([]int64, 0, len(files))
	for _, file := range files {
		ids = append(ids, file.id)
	}
	return ids, err
}

// sweptFile is a file FilesToReadThrough finds, by its id, its path as its node reported it, and
// whether its bytes have any timeline yet.
type sweptFile struct {
	id          int64
	path        []byte
	hasTimeline bool
}

// filesToReadThrough is FilesToReadThrough, each file with its path.
func (service *Service) filesToReadThrough(ctx context.Context) ([]sweptFile, error) {
	queries := service.database.Read()
	lastFileID, err := queries.GetMaxFileID(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the last file id: %w", err)
	}
	unavailability := library.NewUnavailability(queries)
	var files []sweptFile
	for after := int64(0); after < lastFileID; after += fileIDsPerSweepRead {
		rows, listErr := queries.ListFilesToReadThrough(ctx, sqlcgen.ListFilesToReadThroughParams{
			AfterID: after, ThroughID: after + fileIDsPerSweepRead,
		})
		if listErr != nil {
			return nil, fmt.Errorf("list files to read through: %w", listErr)
		}
		for _, row := range rows {
			_, isUnavailable, underErr := unavailability.Under(ctx, row.NodeID, row.ReportedPathRaw)
			if underErr != nil {
				return nil, underErr
			}
			if !isUnavailable {
				swept := sweptFile{id: row.ID, path: row.ReportedPathRaw, hasTimeline: row.HasTimeline != 0}
				files = append(files, swept)
			}
		}
	}
	return files, nil
}

// readThroughArgs are a read-through job's arguments: IsFromIndexOnly asks for its picture's
// keyframes from its file's index alone, never a read through (D269 §5); none reads it through.
type readThroughArgs struct {
	IsFromIndexOnly bool `json:"isFromIndexOnly,omitempty"`
}

// argsJSON is the arguments as a job carries them: "{}" for a read through.
func (args readThroughArgs) argsJSON() string {
	encoded, err := json.Marshal(args)
	if err != nil {
		return "{}" // a struct of one bool always encodes
	}
	return string(encoded)
}

// argsOf are a read-through job's arguments; a job whose arguments do not read as them reads through.
func argsOf(job jobs.Job) readThroughArgs {
	args, err := decodeArgs(job.ArgsJSON)
	if err != nil {
		return readThroughArgs{}
	}
	return args
}

// sweptRead is the read-through the sweep asks of a file, in tiers (D269 §4, §5): one with no
// timeline whose container has an index the node reads is read from that index alone first,
// milliseconds, and read through after it (followIndexRead); one whose container has none, ahead
// of the rest; the rest in the background.
func sweptRead(file sweptFile) jobs.Request {
	request := jobs.Request{
		Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: file.id}, Priority: jobs.PriorityBackground,
		Reason: "reads it through, for how loud it is and where its picture can be cut",
	}
	switch {
	case !hasIndexReader(file.path):
		request.Priority = jobs.PriorityAheadOfSweep // its keyframes are known no other way
	case !file.hasTimeline:
		request.Priority, request.ArgsJSON = jobs.PriorityIndexFirst, readThroughArgs{IsFromIndexOnly: true}.argsJSON()
		request.Reason = "reads its index, for where its picture can be cut"
	}
	return request
}

// isIndexReadJob is whether a read-through job asks for the picture's keyframes from its file's
// index alone: a player's, waiting on them, or the sweep's fast tier (D259 §2, D269 §5). Its
// arguments say so, never its priority, which a person's interest raises a full read to (D324).
func isIndexReadJob(job jobs.Job) bool {
	return argsOf(job).IsFromIndexOnly
}

// ArgsMerge is how a repeat request for a file's read-through folds into the one open (D231 §8): a
// player waiting on the picture's keyframes makes a queued full read an index read, which its full
// read follows (followIndexRead). A running read covers only a request with its own arguments, so a
// player's beside a running full read is queued to answer in a fraction of a second.
func ArgsMerge() jobs.ArgsMerge {
	return jobs.ArgsMerge{
		Merge: func(queued, requested string) (string, error) {
			queuedArgs, err := decodeArgs(queued)
			if err != nil {
				return "", err
			}
			requestedArgs, err := decodeArgs(requested)
			if err != nil {
				return "", err
			}
			isFromIndexOnly := queuedArgs.IsFromIndexOnly || requestedArgs.IsFromIndexOnly
			return readThroughArgs{IsFromIndexOnly: isFromIndexOnly}.argsJSON(), nil
		},
		Covers: func(running, requested string) (bool, error) {
			runningArgs, err := decodeArgs(running)
			if err != nil {
				return false, err
			}
			requestedArgs, err := decodeArgs(requested)
			return runningArgs == requestedArgs, err
		},
	}
}

// decodeArgs reads a read-through's arguments as a job or request carries them; "" is none.
func decodeArgs(encoded string) (readThroughArgs, error) {
	var args readThroughArgs
	if encoded == "" {
		return args, nil
	}
	if err := json.Unmarshal([]byte(encoded), &args); err != nil {
		return readThroughArgs{}, fmt.Errorf("read a read-through's arguments: %w", err)
	}
	return args, nil
}

// hasIndexReader is whether the file at path, as its node reported it, is of a container the node
// reads an index of keyframes from, by its extension: Matroska's cues and the sync samples of an
// MP4 or one of its ISO family, QuickTime, Flash's F4V and 3GPP's (internal/node/container, which
// knows them by their ftyp box). Any other, AVI, MPEG-TS and the rest, is read through ahead of the
// sweep (D269 §4); a Matroska file without cues is found out at its first play.
func hasIndexReader(path []byte) bool {
	switch strings.ToLower(filepath.Ext(string(path))) {
	case ".mkv", ".mk3d", ".webm", ".mp4", ".m4v", ".mov", ".qt", ".f4v", ".3gp", ".3g2":
		return true
	}
	return false
}

// QueueReadThroughs queues a read-through for every file FilesToReadThrough finds (D226 §2), behind
// everything a person is waiting on, a file of a container with no index the node reads ahead of the
// others (D269 §4), and returns how many it queued.
func (service *Service) QueueReadThroughs(ctx context.Context) (int, error) {
	files, err := service.filesToReadThrough(ctx)
	if err != nil {
		return 0, err
	}
	reads := make([]jobs.Request, 0, len(files))
	for _, file := range files {
		reads = append(reads, sweptRead(file))
	}
	queued, err := service.queue.EnqueueBatch(ctx, reads) // a batch to each write, not a write each (S4)
	if err != nil {
		return queued, fmt.Errorf("queue the swept read-throughs: %w", err)
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
		ArgsJSON: readThroughArgs{IsFromIndexOnly: true}.argsJSON(),
		Reason:   "a player is waiting for where its picture can be cut",
	})
	if err != nil {
		return false, fmt.Errorf("queue a read-through of file %d: %w", fileID, err)
	}
	return added, nil
}

// HasNoIndex is whether a read of the picture's keyframes from its file's index found none: its film
// is cut on time until it is read through (D269 §2).
func (service *Service) HasNoIndex(payloadHash []byte, videoStreamIndex int) bool {
	service.noIndex.mutex.Lock()
	defer service.noIndex.mutex.Unlock()
	return service.noIndex.byKey[noIndexKey(payloadHash, videoStreamIndex)]
}

// ReadFirst queues a read-through of the file ahead of the sweep, for the keyframes its index lacks
// (D269 §4): a person played it on time, and the next to play it gets its picture copied. It reports
// whether it queued one.
func (service *Service) ReadFirst(ctx context.Context, fileID int64) (bool, error) {
	added, err := service.queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: fileID}, Priority: jobs.PriorityAheadOfSweep,
		Reason: "reads it through for its keyframes, which its file has no index of",
	})
	if err != nil {
		return false, fmt.Errorf("queue a read-through of file %d: %w", fileID, err)
	}
	return added, nil
}

// ReleaseForPlay says no player waits for the file's read-through any more, as one that left its
// page (D258 §2): it falls to background at the next demotion, unless a player asks again.
func (service *Service) ReleaseForPlay(ctx context.Context, fileID int64) error {
	if err := service.queue.ReleaseWanted(ctx, jobs.KindReadThrough, jobs.Target{FileID: fileID}); err != nil {
		return fmt.Errorf("release file %d's read-through: %w", fileID, err)
	}
	return nil
}

// RetryFailedForPlay tries again, for a person who pressed play at before, a read-through of the
// file's picture that failed before then: its kept failure goes, and the file is read through at
// play priority. It reports whether there was such a failure. A failure kept since then, on the
// attempt the play made, stands: it is the one the person sees (D248 §4).
func (service *Service) RetryFailedForPlay(
	ctx context.Context, fileID int64, payloadHash []byte, videoStreamIndex int, before time.Time,
) (bool, error) {
	var forgotten int64
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var deleteErr error
		forgotten, deleteErr = queries.DeleteVideoTimelineFailedBefore(ctx,
			sqlcgen.DeleteVideoTimelineFailedBeforeParams{
				PayloadHash: payloadHash, VideoStreamIndex: int64(videoStreamIndex), Before: before.UnixMilli(),
			})
		return deleteErr
	})
	if err != nil {
		return false, fmt.Errorf("forget file %d's failed read-through: %w", fileID, err)
	}
	if forgotten == 0 {
		return false, nil
	}
	if _, err = service.ReadForPlay(ctx, fileID); err != nil {
		return false, err
	}
	return true, nil
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
	if isIndexReadJob(job) && command.VideoTimeline != nil {
		// A player waits on where the picture can be cut: asked for that alone, from the file's index,
		// the node answers in a fraction of a second, or says the file has none, where reading the
		// loudness too takes as long as reading the film (D259 §2, D269 §2). The sweep reads the
		// loudness and the bytes in each second after.
		command.LoudnessTracks = nil
		command.VideoTimeline.IsFromIndexOnly = true
	}
	if argsOf(job).IsFromIndexOnly && command.VideoTimeline == nil {
		return nil // its timeline is kept since it was queued: the full read is the sweep's
	}
	if len(command.LoudnessTracks) == 0 && command.VideoTimeline == nil {
		return nil
	}
	located, err := service.locate(ctx, queries, job, file, &command)
	if err != nil {
		return err
	}
	// A node not yet heard from since the server started has not said how fast it reads: the job
	// waits for its hello, and is not failed, which each restart of the server had done once
	// until it failed for good (the owner's log, 2026-10-05).
	if err := service.nodes.ParkUnlessConnected(ctx, located.NodeID); err != nil {
		return err
	}
	if service.nodes.SlowestReadBytesPerSecond(located.NodeID) <= 0 {
		return fmt.Errorf("node %d has not said how fast it reads, so a read-through of file %d cannot be "+
			"waited for", located.NodeID, file.ID)
	}
	return service.exchanges.Assign(ctx, located.NodeID, job, nodeproto.Command{
		Work: nodeproto.Work{Type: nodeproto.CommandReadThrough, ReadThrough: &command},
	})
}

// locate is where a read-through reads its file, named in command: the first of its places that can be
// read now, a copy when its first's disk does not answer (D363 §2), for what it learns is of the bytes,
// which are the same. A place that cannot be read parks the job.
func (service *Service) locate(
	ctx context.Context, queries *sqlcgen.Queries, job jobs.Job, file sqlcgen.GetPlayableFileRow,
	command *nodeproto.ReadThrough,
) (library.Located, error) {
	located, err := library.PlaceToRead(ctx, queries, service.nodes, nil, auth.EveryLibrary(), file)
	if err != nil {
		return located, jobs.MarkPermanent(err)
	}
	if parked := library.ParkIfUnavailable(ctx, queries, service.nodes, job, located); parked != nil {
		return located, parked
	}
	if command.Video, err = library.BytesFileOf(ctx, queries, located); err != nil {
		return located, jobs.MarkPermanent(err)
	}
	return located, nil
}

// whatIsLacking is the read-through a file's bytes need, without its video: the loudness of every
// audio track when none is kept, and the timeline of its main picture when that has none, or has only
// what its file's index said.
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
	timeline, err := store.OrNotFound(service.database.Read().GetVideoTimeline(ctx, sqlcgen.GetVideoTimelineParams{
		PayloadHash: file.PayloadHash, VideoStreamIndex: int64(picture.StreamIndex),
	}))
	switch {
	case errors.Is(err, store.ErrNotFound), err == nil && isFromIndexAlone(timeline):
		command.VideoTimeline = &nodeproto.VideoTimelineAsk{VideoStreamIndex: picture.StreamIndex}
	case err != nil:
		return command, fmt.Errorf("read the timeline of file %d: %w", file.ID, err)
	}
	return command, nil
}

// isFromIndexAlone is whether a timeline was read from its file's index: its keyframes without its
// bytes in each second, which a read through adds (D259 §2).
func isFromIndexAlone(timeline sqlcgen.VideoTimeline) bool {
	return timeline.Failure == nil && timeline.BytesInEachSecondJSON == "[]"
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
		Runner:   runner,
		Progress: service.takeProgress,
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
			service.timeRead(ctx, job, attempt)
			return service.followIndexRead(ctx, job, attempt.Work.ReadThrough, answer)
		},
		Unanswered: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, err error) error {
			return nodes.UnansweredOutcome(job, err, keepFailure(ctx, attempt))
		},
	}
}

// takeProgress keeps how far into its film a running read-through is, as its job's progress, for
// how long the rest will take (D324 §6).
func (service *Service) takeProgress(ctx context.Context, attempt nodes.Attempt, progress nodeproto.Progress) error {
	command := attempt.Work.ReadThrough
	if progress.State != nodeproto.ProgressRunning || command == nil || command.DurationMs <= 0 ||
		progress.ReadThroughMs <= 0 {
		return nil
	}
	fraction := min(float64(progress.ReadThroughMs)/float64(command.DurationMs), 1)
	if err := service.queue.SetProgress(ctx, attempt.JobID, fraction); err != nil {
		return fmt.Errorf("keep read-through %d's progress: %w", attempt.JobID, err)
	}
	return nil
}

// TimeReadsBy names what is told how long each full read took. Set it once, before the server
// serves.
func (service *Service) TimeReadsBy(timings Timings) { service.timings = timings }

// timeRead tells the timings how long a full read of a file took, from when its node said it ran:
// an index read says nothing of how fast the node reads.
func (service *Service) timeRead(ctx context.Context, job jobs.Job, attempt nodes.Attempt) {
	command := attempt.Work.ReadThrough
	if service.timings == nil || command == nil || command.IsFromIndexOnly() || attempt.RunningSince.IsZero() {
		return
	}
	file, err := library.PlayableFile(ctx, service.database.Read(), job.FileID)
	if err != nil || file.SizeBytes == nil {
		return // a file gone since says nothing of the node's speed
	}
	service.timings.ReadThroughFinished(attempt.NodeID, *file.SizeBytes, service.clock.Now().Sub(attempt.RunningSince))
}

// followIndexRead queues a file's full read once its index was read (D269 §5): for its loudness
// and its bytes in each second, ahead of the others when it has no index, its keyframes known no
// other way, else in the background. The job its index was read by is running still, so this one,
// with other arguments, is queued beside it rather than taken for it.
func (service *Service) followIndexRead(
	ctx context.Context, job jobs.Job, command *nodeproto.ReadThrough, answer nodeproto.ReadThroughResult,
) error {
	if command == nil || command.VideoTimeline == nil || !command.VideoTimeline.IsFromIndexOnly {
		return nil
	}
	priority := jobs.PriorityBackground
	if answer.HasNoIndex {
		priority = jobs.PriorityAheadOfSweep
	}
	_, err := service.queue.Enqueue(ctx, jobs.Request{
		Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: job.FileID}, Priority: priority,
		Reason: "reads it through, for how loud it is and its bytes in each second",
	})
	if err != nil {
		return fmt.Errorf("queue file %d's read-through after its index: %w", job.FileID, err)
	}
	return nil
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
	if answer.HasNoIndex && command.VideoTimeline != nil {
		service.noIndex.mutex.Lock()
		service.noIndex.byKey[noIndexKey(payloadHash, command.VideoTimeline.VideoStreamIndex)] = true
		service.noIndex.mutex.Unlock()
		return nil
	}
	return service.keep(ctx, payloadHash, *command, *answer)
}

// keep keeps a node's answer by the file's payload hash, each part the command asked for, in one
// write: each track's sounds and the picture's timeline, or, when FFmpeg could not read the file, its
// words, for each track as stereo and for the picture, so the sweep does not ask again.
func (service *Service) keep(
	ctx context.Context, payloadHash []byte, command nodeproto.ReadThrough, answer nodeproto.ReadThroughResult,
) error {
	now := service.clock.Now().UnixMilli()
	loudnessRows := loudnessRows(payloadHash, command.LoudnessTracks, answer, now)
	var timeline *sqlcgen.UpsertVideoTimelineParams
	if command.VideoTimeline != nil {
		var err error
		if timeline, err = timelineRow(payloadHash, command.VideoTimeline.VideoStreamIndex, answer, now); err != nil {
			return err
		}
	}
	if len(loudnessRows) == 0 && timeline == nil {
		return nil
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, each := range loudnessRows {
			if err := queries.UpsertLoudness(ctx, each); err != nil {
				return err
			}
		}
		if timeline != nil {
			return queries.UpsertVideoTimeline(ctx, *timeline)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("keep what the read-through found: %w", err)
	}
	return nil
}

// loudnessRows are each measured track's sounds, or FFmpeg's word for each asked track as stereo.
func loudnessRows(
	payloadHash []byte, tracks []nodeproto.LoudnessTrack, answer nodeproto.ReadThroughResult, now int64,
) []sqlcgen.UpsertLoudnessParams {
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
	return rows
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

// timelineRow is the picture's timeline as persistent.db keeps it, or FFmpeg's word that the file
// could not be read; nil for an answer that lacks the timeline it was asked for, which keeps
// nothing, so the next sweep asks again.
func timelineRow(
	payloadHash []byte, videoStreamIndex int, answer nodeproto.ReadThroughResult, now int64,
) (*sqlcgen.UpsertVideoTimelineParams, error) {
	row := sqlcgen.UpsertVideoTimelineParams{
		PayloadHash: payloadHash, VideoStreamIndex: int64(videoStreamIndex), KeyframesMsJSON: "[]",
		BytesInEachSecondJSON: "[]", MadeAt: now,
	}
	switch {
	case answer.Failure != "":
		row.Failure = &answer.Failure
	case answer.VideoTimeline != nil:
		keyframes, err := json.Marshal(answer.VideoTimeline.KeyframesMs)
		if err != nil {
			return nil, fmt.Errorf("encode the keyframes: %w", err)
		}
		// A timeline read from its file's index has no bytes in each second: kept as none, "[]", which a
		// read through fills in (D259 §2).
		bytesInEachSecond := answer.VideoTimeline.BytesInEachSecond
		if bytesInEachSecond == nil {
			bytesInEachSecond = []int64{}
		}
		seconds, err := json.Marshal(bytesInEachSecond)
		if err != nil {
			return nil, fmt.Errorf("encode the bytes in each second: %w", err)
		}
		row.KeyframesMsJSON, row.BytesInEachSecondJSON = string(keyframes), string(seconds)
	default:
		return nil, nil //nolint:nilnil // no timeline to keep is not an error
	}
	return &row, nil
}

// IsIndexRead is whether a read-through job's arguments ask for its picture's keyframes from its
// file's index alone, which takes a fraction of a second, rather than a read through.
func IsIndexRead(argsJSON string) bool {
	args, err := decodeArgs(argsJSON)
	return err == nil && args.IsFromIndexOnly
}
