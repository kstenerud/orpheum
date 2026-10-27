// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A file found at a second place (D363 §2): a video read there whose payload hash a live video found
// before it at another place has is a candidate copy of it. D116's whole-file pass, run for the two
// alone, reads each through for the SHA-256 of its every byte; the same bytes make the second the
// first's place from then on, and other bytes leave them two files, which is logged. Each candidate is
// derived from persistent.db by a sweep (D231 §3), and its reads are hash_whole_file jobs.

// copyCandidatesPerSweep is how many candidates one sweep looks at: a justified constant, a bound on
// the sweep's write and its queued reads, which the next sweep continues; a household has few copies.
const copyCandidatesPerSweep = 100

// CopyFold moves what another package holds of a file found to be a copy onto the file it is a copy
// of, in the caller's transaction: playback's watching, a person's choices, the subtitles fetched for
// it (D363 §2).
type CopyFold func(ctx context.Context, queries *sqlcgen.Queries, fromFileID, toFileID int64) error

// CopiesParts are what the copies are confirmed with.
type CopiesParts struct {
	Database *store.Database
	Jobs     ScanJobs
	Assigner Assigner
	Nodes    NodeConnections
	// Work cancels the jobs of a file that became a copy, whose work is its file's from then on.
	Work RetiredWorkCanceller
	// RetireVersion retires the versions a copy leaves into its file's titles (identify's
	// RetireVersionInto).
	RetireVersion VersionRetirer
	// Folds move what other packages hold of a copy onto its file.
	Folds []CopyFold
	// Passes asks for a library's identification pass, once a copy's titles were merged into its file's.
	Passes func(ctx context.Context, libraryID int64)
	Clock  clock.Clock
	Logger *slog.Logger
}

// Copies confirms the files found at a second place that are copies of one found before (D363 §2).
type Copies struct {
	parts CopiesParts
}

// NewCopies returns the copies' confirmation.
func NewCopies(parts CopiesParts) *Copies {
	return &Copies{parts: parts}
}

// QueueCopyChecks is the copies' sweep: of the candidates, a pair both of whose whole-file hashes are
// current is settled, one copy when they match; the read of a file whose hash is missing, or older
// than its size and modified time, is queued. It says how many it queued or settled.
func (copies *Copies) QueueCopyChecks(ctx context.Context) (int, error) {
	candidates, err := copies.parts.Database.Read().ListCopyCandidates(ctx, copyCandidatesPerSweep)
	if err != nil {
		return 0, fmt.Errorf("list the files found at a second place: %w", err)
	}
	done := 0
	for _, candidate := range candidates {
		firstCurrent := isHashCurrent(candidate.FirstSha256, candidate.FirstFailure, candidate.FirstHashedSizeBytes,
			candidate.FirstHashedModifiedAt, candidate.FirstSizeBytes, candidate.FirstModifiedAt)
		candidateCurrent := isHashCurrent(candidate.CandidateSha256, candidate.CandidateFailure,
			candidate.CandidateHashedSizeBytes, candidate.CandidateHashedModifiedAt, candidate.SizeBytes,
			candidate.ModifiedAt)
		if firstCurrent && candidateCurrent {
			if isSame(candidate.FirstSha256, candidate.CandidateSha256) {
				if err := copies.confirm(ctx, candidate.ID, candidate.FirstID); err != nil {
					return done, err
				}
				done++
			}
			continue // compared: a difference stays two files, logged when it was found
		}
		for _, owed := range []struct {
			fileID    int64
			isCurrent bool
		}{{candidate.FirstID, firstCurrent}, {candidate.ID, candidateCurrent}} {
			if owed.isCurrent {
				continue
			}
			if _, err := copies.parts.Jobs.Enqueue(ctx, hashRequest(owed.fileID)); err != nil {
				return done, fmt.Errorf("queue the whole read of file %d: %w", owed.fileID, err)
			}
			done++
		}
	}
	return done, nil
}

// hashRequest is the background job that reads a file whole for its hash.
func hashRequest(fileID int64) jobs.Request {
	return jobs.Request{
		Kind: jobs.KindHashWholeFile, Target: jobs.Target{FileID: fileID}, Priority: jobs.PriorityBackground,
		Reason: "reads it whole, to know whether a file found at a second place is a copy of it (D363)",
	}
}

// isHashCurrent is whether a file's whole-file hash, or its failure, was made at the size and modified
// time the file has now.
func isHashCurrent(sha256 []byte, failure *string, hashedSize, hashedModified, size, modified *int64) bool {
	if sha256 == nil && failure == nil {
		return false
	}
	return ptr.ValueOrZero(hashedSize) == ptr.ValueOrZero(size) &&
		ptr.ValueOrZero(hashedModified) == ptr.ValueOrZero(modified)
}

// isSame is whether two whole-file hashes are both there and the same.
func isSame(first, second []byte) bool {
	return first != nil && second != nil && bytes.Equal(first, second)
}

// HandleHashWhole runs one whole-file read (jobs.KindHashWholeFile): the node that holds the file, at
// the place its row is, reads it whole. A file gone, or no video, ends the job.
func (copies *Copies) HandleHashWhole(ctx context.Context, job jobs.Job) error {
	queries := copies.parts.Database.Read()
	file, err := store.OrNotFound(queries.GetFileToHashWhole(ctx, job.FileID))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.MarkPermanent(fmt.Errorf("file %d: %w", job.FileID, ErrNoFile))
	}
	if err != nil {
		return fmt.Errorf("read file %d: %w", job.FileID, err)
	}
	located := Located{
		Subject: fmt.Sprintf("file %d", file.ID), NodeID: file.NodeID, ReportedPathRaw: file.ReportedPathRaw,
		PayloadHash: file.PayloadHash,
	}
	if parked := ParkIfUnavailable(ctx, queries, copies.parts.Nodes, job, located); parked != nil {
		return parked
	}
	named, err := BytesFileOf(ctx, queries, located)
	if err != nil {
		return jobs.MarkPermanent(err)
	}
	return copies.parts.Assigner.Assign(ctx, file.NodeID, job, nodeproto.Command{Work: nodeproto.Work{
		Type: nodeproto.CommandHashWhole, HashWhole: &nodeproto.HashWhole{File: named},
	}})
}

// Assignee is how a whole-file read given to a node is resolved: its hash, or the failure that is the
// file's, is kept at the size and modified time the read found, and the candidates it settles are
// settled at once.
func (copies *Copies) Assignee(runner nodes.Resolver) nodes.Assignee {
	return nodes.Assignee{
		Runner: runner,
		Answer: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			about := fmt.Sprintf("reading file %d whole", job.FileID)
			if result.WholeHash == nil {
				return fmt.Errorf("node %d answered %s with something other than a hash", attempt.NodeID, about)
			}
			answer := *result.WholeHash
			switch {
			case answer.Refusal != "":
				return nodes.RefusalError(attempt.NodeID, about, answer.Refusal, answer.RefusalCode)
			case answer.Stall != nil:
				return nodes.StallOutcome(job, nodes.StallError(attempt.NodeID, about, *answer.Stall),
					jobs.StalledTwice, nil)
			}
			return copies.take(ctx, job.FileID, answer)
		},
		Unanswered: func(_ context.Context, job jobs.Job, _ nodes.Attempt, err error) error {
			return nodes.UnansweredOutcome(job, err, nil)
		},
	}
}

// take keeps a file's whole-file hash, or why it could not be read through, and settles each candidate
// it is half of whose other half is hashed too: one copy when they match, and a difference logged.
func (copies *Copies) take(ctx context.Context, fileID int64, answer nodeproto.WholeHash) error {
	var sha256 []byte
	if answer.SHA256 != "" {
		decoded, err := hex.DecodeString(answer.SHA256)
		if err != nil {
			return jobs.MarkPermanent(fmt.Errorf("file %d's whole hash %q is not hex: %w", fileID, answer.SHA256, err))
		}
		sha256 = decoded
	}
	// The pairs it is half of, listed before its hash is kept: a pair settled as two is listed no more.
	candidates, err := copies.parts.Database.Read().ListCopyCandidates(ctx, copyCandidatesPerSweep)
	if err != nil {
		return fmt.Errorf("list the files found at a second place: %w", err)
	}
	err = copies.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.UpsertWholeFileHash(ctx, sqlcgen.UpsertWholeFileHashParams{
			FileID: fileID, Sha256: sha256, SizeBytes: &answer.SizeBytes, ModifiedAt: &answer.ModifiedAt,
			Failure: ptr.NilIfZero(answer.Failure), HashedAt: copies.parts.Clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("keep file %d's whole hash: %w", fileID, err)
	}
	if answer.Failure != "" {
		copies.parts.Logger.WarnContext(ctx, "a file could not be read whole; whether a copy of it is the same "+
			"file is not known, and they stay two", "fileId", fileID, "failure", answer.Failure)
		return nil
	}
	return copies.settle(ctx, fileID, sha256, candidates)
}

// settle settles the candidates fileID is half of, now that its hash, sha256, is kept: the same bytes
// make the candidate a copy, and other bytes leave two files, logged once, here.
func (copies *Copies) settle(
	ctx context.Context, fileID int64, sha256 []byte, candidates []sqlcgen.ListCopyCandidatesRow,
) error {
	for _, candidate := range candidates {
		var other []byte
		switch fileID {
		case candidate.ID:
			if !isHashCurrent(candidate.FirstSha256, candidate.FirstFailure, candidate.FirstHashedSizeBytes,
				candidate.FirstHashedModifiedAt, candidate.FirstSizeBytes, candidate.FirstModifiedAt) {
				continue // the other half is not read yet, or has changed since
			}
			other = candidate.FirstSha256
		case candidate.FirstID:
			if !isHashCurrent(candidate.CandidateSha256, candidate.CandidateFailure,
				candidate.CandidateHashedSizeBytes, candidate.CandidateHashedModifiedAt, candidate.SizeBytes,
				candidate.ModifiedAt) {
				continue
			}
			other = candidate.CandidateSha256
		default:
			continue
		}
		if other == nil {
			continue // the other half could not be read, which was logged then: they stay two
		}
		if !isSame(sha256, other) {
			copies.parts.Logger.InfoContext(ctx, "a file found at a second place has the payload of one found "+
				"before, and other bytes: they stay two files", "fileId", candidate.ID,
				"firstFileId", candidate.FirstID, "nodeId", candidate.NodeID, "firstNodeId", candidate.FirstNodeID,
				"path", PathText(candidate.ReportedPathRaw), "firstPath", PathText(candidate.FirstReportedPathRaw))
			continue
		}
		if err := copies.confirm(ctx, candidate.ID, candidate.FirstID); err != nil {
			return err
		}
	}
	return nil
}

// confirm makes copyID, confirmed byte for byte the same as fileID, fileID's place from now on (D363
// §2), in one transaction: it is filed a copy; its own tracks and probe go, its file's being the same;
// the tracks of the subtitles beside it are its file's; what each kind of library filed it as is its
// file's (retireFilingsInto); and what other packages hold of it is moved onto its file (Folds). Its
// jobs are cancelled after, and the libraries covering it are passed again.
func (copies *Copies) confirm(ctx context.Context, copyID, fileID int64) error {
	var merged mergedCopy
	err := copies.parts.Database.Write(ctx, func(queries *sqlcgen.Queries) error {
		var mergeErr error
		merged, mergeErr = copies.merge(ctx, queries, copyID, fileID)
		return mergeErr
	})
	streamIDs, libraryIDs := merged.streamIDs, merged.libraryIDs
	if errors.Is(err, errCopyNotMade) {
		return nil // no longer a video, or gone, since it was read
	}
	if err != nil {
		return fmt.Errorf("make file %d a copy of file %d: %w", copyID, fileID, err)
	}
	copies.parts.Logger.InfoContext(ctx, "a file found at a second place is byte for byte one found before: "+
		"one file at two places from now on", "copyId", copyID, "fileId", fileID)
	if copies.parts.Work != nil {
		work := jobs.RetiredWork{FileIDs: []int64{copyID}, StreamIDs: streamIDs}
		if _, err := copies.parts.Work.CancelRetired(ctx, work); err != nil {
			return fmt.Errorf("cancel copy %d's work: %w", copyID, err)
		}
	}
	if copies.parts.Passes != nil {
		for _, libraryID := range libraryIDs {
			copies.parts.Passes(ctx, libraryID)
		}
	}
	return nil
}

// mergedCopy is what a copy merged into its file leaves to do after: the copy's own tracks, whose work
// is cancelled, and the libraries covering the file, passed again.
type mergedCopy struct {
	streamIDs  []int64
	libraryIDs []int64
}

// merge is confirm's transaction: copyID filed a copy of fileID, and all it held its file's.
func (copies *Copies) merge(ctx context.Context, queries *sqlcgen.Queries, copyID, fileID int64) (mergedCopy, error) {
	var merged mergedCopy
	made, err := queries.MakeFileACopy(ctx, sqlcgen.MakeFileACopyParams{
		FilingReason: new("byte for byte the file found before it, at another place"), CopyOf: &fileID, ID: copyID,
	})
	if err != nil || made == 0 {
		return merged, errors.Join(err, errCopyNotMade)
	}
	encoded, err := json.Marshal([]int64{copyID})
	if err != nil {
		return merged, fmt.Errorf("encode file %d: %w", copyID, err)
	}
	if merged.streamIDs, err = queries.ListStreamIDsOfFiles(ctx, string(encoded)); err != nil {
		return merged, fmt.Errorf("list file %d's tracks: %w", copyID, err)
	}
	if moveErr := moveCopysOwn(ctx, queries, copyID, fileID); moveErr != nil {
		return merged, moveErr
	}
	nowMs := copies.parts.Clock.Now().UnixMilli()
	retireErr := retireFilingsInto(ctx, queries, copies.parts.RetireVersion, copyID, fileID, nowMs)
	if retireErr != nil {
		return merged, retireErr
	}
	for _, fold := range copies.parts.Folds {
		if foldErr := fold(ctx, queries, copyID, fileID); foldErr != nil {
			return merged, foldErr
		}
	}
	if merged.libraryIDs, err = queries.ListLibrariesOfFile(ctx, fileID); err != nil {
		return merged, fmt.Errorf("list the libraries of file %d: %w", fileID, err)
	}
	return merged, nil
}

// errCopyNotMade is a candidate no longer one to make a copy of: gone, or not a video, since it was read.
var errCopyNotMade = errors.New("the file is no longer a video to make a copy")

// moveCopysOwn drops a copy's own tracks and probe, its file's being the same, and makes the tracks of
// the subtitles beside it its file's, the sidecars staying at the copy's place (D363 §3).
func moveCopysOwn(ctx context.Context, queries *sqlcgen.Queries, copyID, fileID int64) error {
	if err := queries.DeleteOwnStreamsOfFile(ctx, copyID); err != nil {
		return fmt.Errorf("drop copy %d's own tracks: %w", copyID, err)
	}
	err := queries.MoveSidecarTracksToFile(ctx, sqlcgen.MoveSidecarTracksToFileParams{ToFile: fileID, FromFile: copyID})
	if err != nil {
		return fmt.Errorf("give copy %d's subtitles to file %d: %w", copyID, fileID, err)
	}
	if err := queries.DeleteFileProbe(ctx, copyID); err != nil {
		return fmt.Errorf("drop copy %d's probe: %w", copyID, err)
	}
	return nil
}
