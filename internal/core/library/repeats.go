// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"cmp"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// The fingerprint detector (D101, D115; plans/media-segments.md): the sound a season's episodes share
// at their opening is their intro, and at their closing their credits. The season is library
// knowledge, so the core names its videos and the node compares them (the findRepeats command). Each
// video's bytes are measured once at repeatsMeasurementVersion, kept in repeat_measurement; a sweep
// queues one find_repeats job for each season, on each node, with a video not measured yet.

// repeatsMeasurementVersion is the measurement's version, kept with each video measured: raising it
// measures every video again, as a better comparison would want.
const repeatsMeasurementVersion = 1

// repeatsOpening and repeatsClosing are how much of each video's start and end is fingerprinted: an
// intro rarely ends past the tenth minute, and credits rarely start more than six from the end (the
// node caps each at the video's length).
const (
	repeatsOpening = 10 * time.Minute
	repeatsClosing = 6 * time.Minute
)

// repeatsVideosPerJob is the most videos one job compares: a bound on one job's run on its node, above
// a season's usual length; a longer season's others are compared by a later job.
const repeatsVideosPerJob = 30

// repeatsLeastVideos is the fewest episodes a season is compared with: a repeat needs two to be heard in.
const repeatsLeastVideos = 2

// The outcomes of a video's measurement, as repeat_measurement.outcome holds them.
const (
	repeatsFound  = "found"
	repeatsNone   = "none"
	repeatsFailed = "failed"
)

// RepeatsParts are what the fingerprint detector is built from.
type RepeatsParts struct {
	Database *store.Database
	Jobs     ScanJobs
	Assigner Assigner
	Nodes    NodeConnections
	Clock    clock.Clock
	Logger   *slog.Logger
}

// Repeats queues and keeps the fingerprint detector's work: the intro and credits a season's videos
// share (D101).
type Repeats struct {
	parts RepeatsParts
}

// NewRepeats returns the fingerprint detector.
func NewRepeats(parts RepeatsParts) *Repeats {
	return &Repeats{parts: parts}
}

// repeatsArgs are a find_repeats job's arguments: the season, a group of an aired ordering.
type repeatsArgs struct {
	GroupID int64 `json:"groupId"`
}

// seasonOnNode is the videos of one season whose bytes are on one node: what one job compares, for
// the node reads only its own.
type seasonOnNode struct {
	groupID, nodeID int64
	videos          []sqlcgen.ListSeasonVideosRow
}

// seasonsOnNodes groups the season videos listed, in their order, by season and node, leaving out a
// video whose sound could not be fingerprinted: a fact about its bytes, which another try would not
// change.
func seasonsOnNodes(rows []sqlcgen.ListSeasonVideosRow) []seasonOnNode {
	var seasons []seasonOnNode
	for _, row := range rows {
		if row.MeasuredOutcome != nil && *row.MeasuredOutcome == repeatsFailed {
			continue
		}
		if last := len(seasons) - 1; last >= 0 && seasons[last].groupID == row.GroupID &&
			seasons[last].nodeID == row.NodeID {
			seasons[last].videos = append(seasons[last].videos, row)
			continue
		}
		seasons = append(seasons, seasonOnNode{
			groupID: row.GroupID, nodeID: row.NodeID,
			videos: []sqlcgen.ListSeasonVideosRow{row},
		})
	}
	return seasons
}

// isMeasured is whether a season video's bytes were measured at this version.
func isMeasured(row sqlcgen.ListSeasonVideosRow) bool { return row.MeasuredOutcome != nil }

// set is what a job compares of a season: one video of each episode, for two versions of one episode
// share all their sound and would be heard as one long repeat; of an episode's, the lowest not measured
// yet, else its lowest. Those not measured come first, by id, the lowest of them the job's target, and
// at most repeatsVideosPerJob.
func (season seasonOnNode) set() []sqlcgen.ListSeasonVideosRow {
	chosen := map[int64]sqlcgen.ListSeasonVideosRow{}
	for _, video := range season.videos { // by file id
		kept, isKept := chosen[video.WorkID]
		if !isKept || (isMeasured(kept) && !isMeasured(video)) {
			chosen[video.WorkID] = video
		}
	}
	set := make([]sqlcgen.ListSeasonVideosRow, 0, len(chosen))
	for _, video := range chosen {
		set = append(set, video)
	}
	slices.SortFunc(set, func(one, other sqlcgen.ListSeasonVideosRow) int {
		return cmp.Or(compareBool(isMeasured(one), isMeasured(other)), cmp.Compare(one.FileID, other.FileID))
	})
	return set[:min(len(set), repeatsVideosPerJob)]
}

// compareBool orders false before true.
func compareBool(one, other bool) int {
	switch {
	case one == other:
		return 0
	case one:
		return 1
	}
	return -1
}

// listSeasonVideos lists the season videos, of every season for group 0.
func (repeats *Repeats) listSeasonVideos(ctx context.Context, groupID int64) ([]sqlcgen.ListSeasonVideosRow, error) {
	rows, err := repeats.parts.Database.Read().ListSeasonVideos(ctx, sqlcgen.ListSeasonVideosParams{
		Version: repeatsMeasurementVersion, GroupID: &groupID,
	})
	if err != nil {
		return nil, fmt.Errorf("list the seasons' videos: %w", err)
	}
	return rows, nil
}

// QueueRepeats is the fingerprint detector's sweep: one job for each season, on each node, of at least
// repeatsLeastVideos episodes, one of whose videos is not measured yet, aimed at the lowest of them. It
// says how many it queued.
func (repeats *Repeats) QueueRepeats(ctx context.Context) (int, error) {
	rows, err := repeats.listSeasonVideos(ctx, 0)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, season := range seasonsOnNodes(rows) {
		set := season.set()
		if len(set) < repeatsLeastVideos || isMeasured(set[0]) {
			continue
		}
		args, err := json.Marshal(repeatsArgs{GroupID: season.groupID})
		if err != nil {
			return queued, fmt.Errorf("encode season %d's arguments: %w", season.groupID, err)
		}
		isQueued, err := repeats.parts.Jobs.Enqueue(ctx, jobs.Request{
			Kind: jobs.KindFindRepeats, Target: jobs.Target{FileID: set[0].FileID}, ArgsJSON: string(args),
			Priority: jobs.PriorityBackground,
			Reason:   "finds the intro and credits a season's episodes share by their sound (D101)",
		})
		if err != nil {
			return queued, fmt.Errorf("queue season %d's repeats: %w", season.groupID, err)
		}
		if isQueued {
			queued++
		}
	}
	return queued, nil
}

// seasonOfJob is the season a job's arguments name.
func seasonOfJob(job jobs.Job) (int64, error) {
	var args repeatsArgs
	if err := strictjson.Unmarshal([]byte(job.ArgsJSON), &args); err != nil || args.GroupID == 0 {
		return 0, jobs.MarkPermanent(fmt.Errorf("job %d's arguments %q name no season", job.ID, job.ArgsJSON))
	}
	return args.GroupID, nil
}

// seasonOf is the season on its node of a job's target, as it is now; false when the target is not in
// it any more, or was measured since.
func (repeats *Repeats) seasonOf(ctx context.Context, job jobs.Job) (seasonOnNode, bool, error) {
	groupID, err := seasonOfJob(job)
	if err != nil {
		return seasonOnNode{}, false, err
	}
	rows, err := repeats.listSeasonVideos(ctx, groupID)
	if err != nil {
		return seasonOnNode{}, false, err
	}
	for _, season := range seasonsOnNodes(rows) {
		for _, video := range season.videos {
			if video.FileID == job.FileID {
				return season, !isMeasured(video), nil
			}
		}
	}
	return seasonOnNode{}, false, nil
}

// HandleFindRepeats runs one find_repeats job (jobs.KindFindRepeats): the node that holds its season's
// videos fingerprints their opening and closing minutes. A target no longer in its season, or measured
// since, ends the job: the next sweep asks for what is owed then.
func (repeats *Repeats) HandleFindRepeats(ctx context.Context, job jobs.Job) error {
	season, isOwed, err := repeats.seasonOf(ctx, job)
	if err != nil || !isOwed {
		return err
	}
	queries := repeats.parts.Database.Read()
	command := nodeproto.FindRepeats{OpeningMs: repeatsOpening.Milliseconds(), ClosingMs: repeatsClosing.Milliseconds()}
	var target Located
	for _, video := range season.set() {
		file, err := PlayableFile(ctx, queries, video.FileID)
		if err != nil {
			continue // not readable now: compared when it is
		}
		probe, err := ProbeOf(file.ProbeJSON)
		if err != nil {
			continue
		}
		sound, hasSound := decide.MainAudio(probe)
		located := LocatedOf(file)
		named, err := BytesFileOf(ctx, queries, located)
		if !hasSound || probe.Format.DurationMs <= 0 || err != nil {
			continue
		}
		if video.FileID == job.FileID {
			target = located
		}
		command.Videos = append(command.Videos, nodeproto.RepeatVideo{
			FileID: video.FileID, Video: named, AudioStreamIndex: sound.StreamIndex,
			DurationMs: probe.Format.DurationMs,
		})
	}
	if target.Subject == "" || len(command.Videos) < repeatsLeastVideos {
		return nil
	}
	if parked := ParkIfUnavailable(ctx, queries, repeats.parts.Nodes, job, target); parked != nil {
		return parked
	}
	return repeats.parts.Assigner.Assign(ctx, season.nodeID, job, nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandFindRepeats, FindRepeats: &command,
	}})
}

// Assignee is how a season's comparison given to a node is resolved: what each video shares with the
// others becomes its intro and credits.
func (repeats *Repeats) Assignee(runner nodes.Resolver) nodes.Assignee {
	return nodes.Assignee{
		Runner: runner,
		Answer: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			about := fmt.Sprintf("finding the repeats of the season of file %d", job.FileID)
			if result.Repeats == nil {
				return fmt.Errorf("node %d answered %s with something other than its repeats", attempt.NodeID, about)
			}
			answer := *result.Repeats
			switch {
			case answer.Refusal != "":
				return nodes.RefusalError(attempt.NodeID, about, answer.Refusal, "")
			case answer.Stall != nil:
				return nodes.StallOutcome(job, nodes.StallError(attempt.NodeID, about, *answer.Stall),
					jobs.StalledTwice, nil)
			}
			return repeats.keep(ctx, job, attempt.Work.FindRepeats, answer)
		},
		Unanswered: func(_ context.Context, job jobs.Job, _ nodes.Attempt, err error) error {
			return nodes.UnansweredOutcome(job, err, nil)
		},
	}
}

// keep keeps what the node found of each video of a season: its repeats as the fingerprint provider's
// segments, an opening's an intro and a closing's the credits, and that its bytes were measured. A
// video the node did not read is left for the next sweep, and so is one compared with no other; an
// answer for a video no longer in the season, or whose bytes are not those sent, is not kept.
func (repeats *Repeats) keep(
	ctx context.Context, job jobs.Job, sent *nodeproto.FindRepeats, answer nodeproto.Repeats,
) error {
	season, err := repeats.seasonVideosByFile(ctx, job)
	if err != nil {
		return err
	}
	sentHashes := sentPayloads(sent)
	compared := comparedVideos(answer)
	now := repeats.parts.Clock.Now().UnixMilli()
	kept := 0
	err = repeats.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, video := range answer.Videos {
			row, isInSeason := season[video.FileID]
			if sentHash, isSent := sentHashes[video.FileID]; !isInSeason ||
				(isSent && hex.EncodeToString(row.PayloadHash) != sentHash) {
				continue
			}
			isKept, keepErr := keepVideoRepeats(ctx, queries, video, row.PayloadHash, compared, now)
			if keepErr != nil {
				return keepErr
			}
			if isKept {
				kept++
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("keep the repeats of the season of file %d: %w", job.FileID, err)
	}
	repeats.parts.Logger.InfoContext(ctx, "compared a season's videos by their sound", slogx.EventKey,
		"seasonRepeatsFound", "fileId", job.FileID, "videos", len(answer.Videos), "compared", compared, "kept", kept)
	return nil
}

// sentPayloads are the payload hashes of the videos a comparison was sent, by file id.
func sentPayloads(sent *nodeproto.FindRepeats) map[int64]string {
	hashes := map[int64]string{}
	if sent != nil {
		for _, video := range sent.Videos {
			hashes[video.FileID] = video.Video.PayloadHash
		}
	}
	return hashes
}

// comparedVideos is how many of an answer's videos were fingerprinted, so compared with each other.
func comparedVideos(answer nodeproto.Repeats) int {
	compared := 0
	for _, video := range answer.Videos {
		if video.Failure == "" && video.Refusal == "" {
			compared++
		}
	}
	return compared
}

// seasonVideosByFile are a job's season's videos as they are now, of every node, by file id.
func (repeats *Repeats) seasonVideosByFile(
	ctx context.Context, job jobs.Job,
) (map[int64]sqlcgen.ListSeasonVideosRow, error) {
	groupID, err := seasonOfJob(job)
	if err != nil {
		return nil, err
	}
	rows, err := repeats.listSeasonVideos(ctx, groupID)
	if err != nil {
		return nil, err
	}
	byFile := make(map[int64]sqlcgen.ListSeasonVideosRow, len(rows))
	for _, row := range rows {
		byFile[row.FileID] = row
	}
	return byFile, nil
}

// keepVideoRepeats keeps what the node found of one video of the season, and says whether it kept
// anything: a refused video is left for the next sweep, changed since the scan or gone, and so is one
// heard alone, compared with nothing; a failure is kept as its bytes' measurement.
func keepVideoRepeats(
	ctx context.Context, queries *sqlcgen.Queries, video nodeproto.VideoRepeats, payloadHash []byte,
	compared int, now int64,
) (bool, error) {
	switch {
	case video.Refusal != "", video.Failure == "" && compared < repeatsLeastVideos:
		return false, nil
	case video.Failure != "":
		return true, upsertMeasurement(ctx, queries, payloadHash, repeatsFailed, &video.Failure, now)
	}
	return true, keepRepeatSegments(ctx, queries, video, payloadHash, now)
}

// keepRepeatSegments replaces a video's fingerprint segments with its repeats, and keeps its bytes as
// measured: found when it has either, none when not. No other provider's segment is touched, a
// person's above all.
func keepRepeatSegments(
	ctx context.Context, queries *sqlcgen.Queries, video nodeproto.VideoRepeats, payloadHash []byte, now int64,
) error {
	err := queries.DeleteProviderSegmentsOfFile(ctx, sqlcgen.DeleteProviderSegmentsOfFileParams{
		FileID: video.FileID, Provider: string(ProviderFingerprint),
	})
	if err != nil {
		return fmt.Errorf("forget file %d's fingerprint segments: %w", video.FileID, err)
	}
	outcome := repeatsNone
	for _, found := range []struct {
		span        *nodeproto.RepeatedSpan
		segmentType SegmentType
	}{{video.Opening, SegmentIntro}, {video.Closing, SegmentCredits}} {
		confidence, isSpan := repeatConfidence(found.span)
		if !isSpan {
			continue
		}
		err := queries.InsertSegment(ctx, sqlcgen.InsertSegmentParams{
			FileID: video.FileID, Type: string(found.segmentType), StartMs: found.span.StartMs, EndMs: found.span.EndMs,
			Provider: string(ProviderFingerprint), Confidence: confidence,
		})
		if err != nil {
			return fmt.Errorf("keep file %d's %s: %w", video.FileID, found.segmentType, err)
		}
		outcome = repeatsFound
	}
	return upsertMeasurement(ctx, queries, payloadHash, outcome, nil, now)
}

// repeatConfidence is how sure a repeat is: the share of the videos it was compared with that it is
// heard in. False for no repeat, or one that marks no range.
func repeatConfidence(span *nodeproto.RepeatedSpan) (float64, bool) {
	if span == nil || span.StartMs < 0 || span.EndMs <= span.StartMs || span.ComparedWith <= 0 || span.HeardIn <= 0 {
		return 0, false
	}
	return min(1, float64(span.HeardIn)/float64(span.ComparedWith)), true
}

// errNoPayload is a measurement of a video with no payload hash, which the season's list never lists.
var errNoPayload = errors.New("a video measured has no payload hash")

// upsertMeasurement keeps a video's bytes as measured at this version.
func upsertMeasurement(
	ctx context.Context, queries *sqlcgen.Queries, payloadHash []byte, outcome string, failure *string, now int64,
) error {
	if len(payloadHash) == 0 {
		return errNoPayload
	}
	err := queries.UpsertRepeatMeasurement(ctx, sqlcgen.UpsertRepeatMeasurementParams{
		PayloadHash: payloadHash, Version: repeatsMeasurementVersion, Outcome: outcome, Failure: failure,
		MeasuredAt: now,
	})
	if err != nil {
		return fmt.Errorf("keep a video as measured: %w", err)
	}
	return nil
}
