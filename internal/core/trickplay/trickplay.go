// SPDX-License-Identifier: AGPL-3.0-or-later

package trickplay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/kstenerud/orpheum/internal/core/auth"
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

// The trickplay version's pictures (trickplay.md section 1): one every 10 s, 320 px wide, in sheets
// of 10 × 10, WebP at quality 60, which phase F0 measured.
const (
	IntervalMs   = 10_000
	PictureWidth = 320
	SheetColumns = 10
	SheetRows    = 10
	WebPQuality  = 60
)

// Version is the trickplay version the constants above are: raise it when one changes, and the sweep
// makes every video's pictures again, asking nothing of a migration (trickplay.md section 1).
const Version = 1

// StoredVersion is the version a video's pictures are kept at: Version and the node's way of making
// them (nodeproto.TrickplayRecipe), so a change to either makes them again.
func StoredVersion() int64 {
	return int64(Version)*1000 + int64(nodeproto.TrickplayRecipe)
}

// maxSheetIndex bounds a sheet's index: a sheet is 16 min 40 s of film, so ten thousand of them are
// more than a hundred days of it.
const maxSheetIndex = 10_000

// Nodes is what a trickplay run asks of the node channel: nodes.Service.
type Nodes interface {
	library.NodeConnections
	// ParkUnlessConnected parks a job on a node that is not connected, until its hello.
	ParkUnlessConnected(ctx context.Context, nodeID int64) error
}

// Queue is the job queue: jobs.Queue.
type Queue interface {
	EnqueueBatch(ctx context.Context, batch []jobs.Request) (int, error)
}

// Sheets are where the sheets' images are kept, content-addressed in the cache directory:
// instancedir.Directories.
type Sheets interface {
	WriteTrickplaySheet(sha256Hex string, contents []byte) error
	RemoveTrickplaySheet(sha256Hex string) error
	ListTrickplaySheets() ([]string, error)
}

// Parts are what the trickplay service is built from.
type Parts struct {
	Database *store.Database
	Cache    *store.Cache
	Sheets   Sheets
	Queue    Queue
	Nodes    Nodes
	// Exchanges are how a run is given to its node and its answers taken; the service records its
	// results and keeps its sheets with them.
	Exchanges *nodes.Exchanges
	Clock     clock.Clock
}

// Service has each live video's scrub-bar pictures made once per payload hash and version, and keeps
// them in cache.db and the cache directory (trickplay.md section 4).
type Service struct {
	database  *store.Database
	cache     *store.Cache
	sheets    Sheets
	queue     Queue
	nodes     Nodes
	exchanges *nodes.Exchanges
	clock     clock.Clock
	// files is held from a sheet's image written to its row kept, and while images are removed, so a
	// removal never takes an image whose row is on its way.
	files sync.Mutex
}

// NewService returns the trickplay service, recording the nodes' runs and keeping their sheets with
// exchanges.
func NewService(parts Parts) *Service {
	service := &Service{
		database: parts.Database, cache: parts.Cache, sheets: parts.Sheets, queue: parts.Queue, nodes: parts.Nodes,
		exchanges: parts.Exchanges, clock: parts.Clock,
	}
	parts.Exchanges.Record(nodeproto.CommandMakeTrickplay, service.record)
	parts.Exchanges.TakeSheetsBy(service.takeSheet)
	return service
}

// HandleTrickplay runs one trickplay job (jobs.KindTrickplay): the node that holds the file makes the
// pictures of its main picture, posting each sheet as it is made, from the first sheet not already
// kept. A file with no picture but covers has none made; bytes whose pictures are made, or whose
// making FFmpeg failed, are not asked again.
func (service *Service) HandleTrickplay(ctx context.Context, job jobs.Job) error {
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
	picture, hasPicture := decide.MainVideo(probe)
	if !hasPicture || len(file.PayloadHash) == 0 {
		return nil
	}
	isEnded, sheetsMade, err := service.made(ctx, file.PayloadHash)
	if err != nil || isEnded {
		return err
	}
	command := nodeproto.MakeTrickplay{
		VideoStreamIndex: picture.StreamIndex, IntervalMs: IntervalMs, PictureWidth: PictureWidth,
		SheetColumns: SheetColumns, SheetRows: SheetRows, WebPQuality: WebPQuality, SheetsMade: sheetsMade,
	}
	located, err := library.PlaceToRead(ctx, queries, service.nodes, nil, auth.EveryLibrary(), file)
	if err != nil {
		return jobs.MarkPermanent(err)
	}
	if parked := library.ParkIfUnavailable(ctx, queries, service.nodes, job, located); parked != nil {
		return parked
	}
	if command.Video, err = library.BytesFileOf(ctx, queries, located); err != nil {
		return jobs.MarkPermanent(err)
	}
	if err := service.nodes.ParkUnlessConnected(ctx, located.NodeID); err != nil {
		return err
	}
	return service.exchanges.Assign(ctx, located.NodeID, job, nodeproto.Command{
		Work: nodeproto.Work{Type: nodeproto.CommandMakeTrickplay, MakeTrickplay: &command},
	})
}

// made is whether the bytes' pictures at the version are made or failed, and the sheets of them kept.
func (service *Service) made(ctx context.Context, payloadHash []byte) (bool, []int, error) {
	queries := service.cache.Read()
	row, err := store.OrNotFound(queries.GetTrickplay(ctx, cachegen.GetTrickplayParams{
		PayloadHash: payloadHash, Version: StoredVersion(),
	}))
	switch {
	case err == nil && (row.DoneAt != nil || row.Failure != nil):
		return true, nil, nil
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return false, nil, fmt.Errorf("read the trickplay of %s: %w", hex.EncodeToString(payloadHash), err)
	}
	sheets, err := queries.ListTrickplaySheets(ctx, cachegen.ListTrickplaySheetsParams{
		PayloadHash: payloadHash, Version: StoredVersion(),
	})
	if err != nil {
		return false, nil, fmt.Errorf("read the trickplay sheets of %s: %w", hex.EncodeToString(payloadHash), err)
	}
	var made []int
	for _, sheet := range sheets {
		made = append(made, int(sheet.SheetIndex))
	}
	return false, made, nil
}

// takeSheet keeps one sheet a node posted for a run the core holds: its image by its SHA-256 in the
// cache directory, then its row. A sheet posted again, as a node does whose answer was lost, changes
// nothing; one that is no image at all, or past any film's length, is refused for what it is.
func (service *Service) takeSheet(ctx context.Context, attempt nodes.Attempt, sheet nodeproto.TrickplaySheet) error {
	command := attempt.Work.MakeTrickplay
	payloadHash, err := hex.DecodeString(command.Video.PayloadHash)
	if err != nil || len(payloadHash) == 0 {
		return fmt.Errorf("%w: a payload hash %q", nodes.ErrMalformedResult, command.Video.PayloadHash)
	}
	if len(sheet.WebP) == 0 || sheet.Index < 0 || sheet.Index >= maxSheetIndex {
		return fmt.Errorf("%w: sheet %d of %d bytes", nodes.ErrMalformedResult, sheet.Index, len(sheet.WebP))
	}
	sum := sha256.Sum256(sheet.WebP)
	sha256Hex := hex.EncodeToString(sum[:])
	// The image first: a row never names an image not there.
	service.files.Lock()
	defer service.files.Unlock()
	if writeErr := service.sheets.WriteTrickplaySheet(sha256Hex, sheet.WebP); writeErr != nil {
		return fmt.Errorf("keep trickplay sheet %d of job %d: %w", sheet.Index, sheet.JobID, writeErr)
	}
	err = service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.UpsertTrickplaySheet(ctx, cachegen.UpsertTrickplaySheetParams{
			PayloadHash: payloadHash, Version: StoredVersion(), SheetIndex: int64(sheet.Index),
			SheetSha256Hex: sha256Hex, SizeBytes: int64(len(sheet.WebP)), MadeAt: service.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("keep trickplay sheet %d of job %d: %w", sheet.Index, sheet.JobID, err)
	}
	return nil
}

// record keeps how a node's run ended, whether or not its job still waits: made, or FFmpeg's word
// that it cannot, a fact about the bytes kept so they are not asked again (D231 §4, D244 §1). A
// refusal or a stall keeps nothing; the sheets posted before it stay, and the next run resumes after
// them.
func (service *Service) record(ctx context.Context, _ int64, result nodeproto.Result) error {
	answer, command := result.Trickplay, result.Work.MakeTrickplay
	if answer == nil || command == nil {
		return fmt.Errorf("a trickplay's result carries no answer, or names no trickplay: %w", nodes.ErrMalformedResult)
	}
	if answer.Refusal != "" || answer.Stall != nil {
		return nil
	}
	return service.keepEnd(ctx, *command, answer.Failure)
}

// keepEnd keeps a run's end: made, or failed with failure.
func (service *Service) keepEnd(ctx context.Context, command nodeproto.MakeTrickplay, failure string) error {
	payloadHash, err := hex.DecodeString(command.Video.PayloadHash)
	if err != nil {
		return fmt.Errorf("%w: a payload hash %q: %w", nodes.ErrMalformedResult, command.Video.PayloadHash, err)
	}
	row := cachegen.UpsertTrickplayParams{
		PayloadHash: payloadHash, Version: StoredVersion(), VideoStreamIndex: int64(command.VideoStreamIndex),
	}
	if failure != "" {
		row.Failure = &failure
	} else {
		now := service.clock.Now().UnixMilli()
		row.DoneAt = &now
	}
	if err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.UpsertTrickplay(ctx, row)
	}); err != nil {
		return fmt.Errorf("keep how the trickplay of %s ended: %w", command.Video.PayloadHash, err)
	}
	return nil
}

// Assignee is how a trickplay job given to a node is resolved, with runner, its kind's (D234 §7), as
// a read-through's is: a refusal is the node's and tried again, FFmpeg's failure is the file's and
// kept, and two stalls in a row, or two runs that stopped moving, are the file's failure, kept.
func (service *Service) Assignee(runner nodes.Resolver) nodes.Assignee {
	keepFailure := func(ctx context.Context, attempt nodes.Attempt) func(failure string) error {
		return func(failure string) error {
			if attempt.Work.MakeTrickplay == nil {
				return nil
			}
			return service.keepEnd(ctx, *attempt.Work.MakeTrickplay, failure)
		}
	}
	return nodes.Assignee{
		Runner: runner,
		Answer: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			about := fmt.Sprintf("making file %d's trickplay pictures", job.FileID)
			if result.Trickplay == nil {
				return fmt.Errorf("node %d answered %s with something other than a trickplay", attempt.NodeID, about)
			}
			answer := *result.Trickplay
			switch {
			case answer.Refusal != "":
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

// fileIDsPerSweepRead is how many file ids the sweep reads at a time: an internal read size, as the
// read-through's sweep reads (readthrough's fileIDsPerSweepRead). Any value gives the same answer.
const fileIDsPerSweepRead = 1000

// FilesToMake are the live videos with a picture whose bytes' pictures at the version are neither
// made nor failed, leaving out a file under a root or disk that is unavailable, which is not there
// to read (D231 §5).
func (service *Service) FilesToMake(ctx context.Context) ([]int64, error) {
	ended, err := service.cache.Read().ListTrickplaysEnded(ctx, StoredVersion())
	if err != nil {
		return nil, fmt.Errorf("list the trickplays made: %w", err)
	}
	isEnded := make(map[string]bool, len(ended))
	for _, payloadHash := range ended {
		isEnded[string(payloadHash)] = true
	}
	queries := service.database.Read()
	lastFileID, err := queries.GetMaxFileID(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the last file id: %w", err)
	}
	unavailability := library.NewUnavailability(queries)
	var ids []int64
	for after := int64(0); after < lastFileID; after += fileIDsPerSweepRead {
		rows, listErr := queries.ListVideosForTrickplay(ctx, sqlcgen.ListVideosForTrickplayParams{
			AfterID: after, ThroughID: after + fileIDsPerSweepRead,
		})
		if listErr != nil {
			return nil, fmt.Errorf("list the videos for trickplay: %w", listErr)
		}
		for _, row := range rows {
			if isEnded[string(row.PayloadHash)] {
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

// QueueTrickplays queues a trickplay for every file FilesToMake finds, at background priority, behind
// every cheaper pass in the queue's order (trickplay.md section 4, D102), and returns how many it
// queued. Interest raises a film's job like any other of its jobs (D324).
func (service *Service) QueueTrickplays(ctx context.Context) (int, error) {
	ids, err := service.FilesToMake(ctx)
	if err != nil {
		return 0, err
	}
	requests := make([]jobs.Request, 0, len(ids))
	for _, id := range ids {
		requests = append(requests, jobs.Request{
			Kind: jobs.KindTrickplay, Target: jobs.Target{FileID: id}, Priority: jobs.PriorityBackground,
			Reason: "makes the scrub bar's pictures of it",
		})
	}
	queued, err := service.queue.EnqueueBatch(ctx, requests) // a batch to each write, not a write each (S4)
	if err != nil {
		return queued, fmt.Errorf("queue the swept trickplays: %w", err)
	}
	return queued, nil
}
