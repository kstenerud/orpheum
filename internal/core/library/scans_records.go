// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// takeListing compares a scan's listing and queues the batches of files it needs read; a scan with
// none to read completes at once.
func (scans *Scans) takeListing(ctx context.Context, scanID int64, records []nodeproto.ScanRecord) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scan, err := store.OrNotFound(scans.cache.Read().GetScan(ctx, scanID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && scan.ListedAt != nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read scan %d: %w", scanID, err)
	}
	rules, perBatch, err := scans.readRules(ctx)
	if err != nil {
		return err
	}
	taken, err := scans.ingester.TakeListing(ctx, scan.NodeID, scan.RootPath, scan.ScopePath, records, rules)
	if err != nil {
		return err
	}
	scans.completions[scan.ID] = taken.Complete
	batchIDs, err := scans.keepBatches(ctx, scan, taken, batchesOf(taken.ToExamine, perBatch))
	if err != nil {
		return err
	}
	scans.logger.InfoContext(ctx, "a node listed a root", "nodeId", scan.NodeID, "rootPath", scan.RootPath,
		"listedCount", taken.ListedCount, "toExamineCount", len(taken.ToExamine), "batchCount", len(batchIDs))
	if len(batchIDs) == 0 {
		return scans.complete(ctx, scan)
	}
	for _, batchID := range batchIDs {
		_, err := scans.jobs.Enqueue(ctx, jobs.Request{
			Kind: jobs.KindExamine, Target: jobs.Target{ScanBatchID: batchID}, Priority: scan.Priority,
			Reason:      "reads what is new or changed under " + scan.RootPath,
			Destination: jobs.NodeDestination(scan.NodeID),
		})
		if err != nil {
			return fmt.Errorf("queue scan batch %d: %w", batchID, err)
		}
	}
	return nil
}

// keepBatches records a scan's listing taken, with its batches, and returns their ids.
func (scans *Scans) keepBatches(
	ctx context.Context, scan cachegen.Scan, taken TakenListing, batches [][]nodeproto.ExamineFile,
) ([]int64, error) {
	batchIDs := make([]int64, 0, len(batches))
	err := scans.cache.Write(ctx, func(queries *cachegen.Queries) error {
		err := queries.SetScanListed(ctx, cachegen.SetScanListedParams{
			ListedAt: new(scans.clock.Now().UnixMilli()), ListedCount: int64(taken.ListedCount),
			ToExamineCount: int64(len(taken.ToExamine)), ID: scan.ID,
		})
		if err != nil {
			return err
		}
		for _, files := range batches {
			encoded, err := json.Marshal(files)
			if err != nil {
				return fmt.Errorf("encode a batch's files: %w", err)
			}
			id, err := queries.InsertScanBatch(ctx, cachegen.InsertScanBatchParams{
				ScanID: scan.ID, FilesJSON: string(encoded), FileCount: int64(len(files)),
			})
			if err != nil {
				return err
			}
			batchIDs = append(batchIDs, id)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("record the listing of %s: %w", scan.RootPath, err)
	}
	return batchIDs, nil
}

// takeExamined ingests what a node read of a batch into its scan.
func (scans *Scans) takeExamined(ctx context.Context, batchID int64, records []nodeproto.ScanRecord) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	opened, isOpen, err := scans.openBatch(ctx, batchID)
	if err != nil || !isOpen {
		return err
	}
	if err := scans.ingester.IngestScan(ctx, opened.scan.NodeID, opened.scan.RootPath, records); err != nil {
		return err
	}
	return scans.finishBatch(ctx, opened)
}

// RecordExamineFailure records a batch the node could not read as given up: each of its files is one
// the scan could not read, so none of them is removed (D40), and the scan goes on to complete.
func (scans *Scans) RecordExamineFailure(ctx context.Context, job jobs.Job, failure string) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	opened, isOpen, err := scans.openBatch(ctx, job.ScanBatchID)
	if err != nil || !isOpen {
		return err
	}
	records := make([]nodeproto.ScanRecord, 0, len(opened.files))
	for _, file := range opened.files {
		records = append(records, nodeproto.ScanRecord{Type: nodeproto.RecordError, Error: &nodeproto.ScanError{
			Path: file.Path, Scope: nodeproto.ScopeEntry, Code: nodeproto.CodeIOError, OccurrenceCount: 1,
			PlatformError: nodeproto.PlatformError{
				Text: "the node did not read it: " + failure, OwnerUID: -1, GroupGID: -1,
			},
		}})
	}
	if err := scans.ingester.IngestScan(ctx, opened.scan.NodeID, opened.scan.RootPath, records); err != nil {
		return err
	}
	return scans.finishBatch(ctx, opened)
}

// RecordScanFailure gives up a scan whose listing failed: nothing it did not see is removed.
func (scans *Scans) RecordScanFailure(ctx context.Context, job jobs.Job, failure string) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scan, err := store.OrNotFound(scans.cache.Read().GetScan(ctx, job.ScanID))
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read scan %d: %w", job.ScanID, err)
	}
	scans.logger.WarnContext(ctx, "a scan was given up: its root could not be listed", "nodeId", scan.NodeID,
		"rootPath", scan.RootPath, "failure", failure)
	return scans.endThenAgain(ctx, scan)
}

// finishBatch counts a batch read, and completes its scan after its last.
func (scans *Scans) finishBatch(ctx context.Context, opened openedBatch) error {
	batch, scan := opened.batch, opened.scan
	var undone int64
	err := scans.cache.Write(ctx, func(queries *cachegen.Queries) error {
		finished, err := queries.SetScanBatchDone(ctx, batch.ID)
		if err != nil || finished == 0 {
			return err
		}
		err = queries.AddScanExamined(ctx, cachegen.AddScanExaminedParams{FileCount: batch.FileCount, ID: scan.ID})
		if err != nil {
			return err
		}
		undone, err = queries.CountUndoneScanBatches(ctx, scan.ID)
		return err
	})
	if err != nil {
		return fmt.Errorf("count batch %d read: %w", batch.ID, err)
	}
	if undone > 0 {
		return nil
	}
	return scans.complete(ctx, scan)
}

// complete ends a scan whose files are all read with its listing's end, applying its removals (D39)
// and handing its library on to identification, then starts the one asked for meanwhile, if one was.
func (scans *Scans) complete(ctx context.Context, scan cachegen.Scan) error {
	complete, isHeld := scans.completions[scan.ID]
	if !isHeld {
		complete = nodeproto.ScanComplete{RootPath: scan.RootPath, IsStoppedByErrors: true}
	}
	err := scans.ingester.IngestScan(ctx, scan.NodeID, scan.RootPath, []nodeproto.ScanRecord{{
		Type: nodeproto.RecordScanComplete, ScanComplete: &complete,
	}})
	if errors.Is(err, errScopeNeedsTheRoot) {
		scans.logger.InfoContext(ctx, "a scan of a folder found what only a scan of its whole root may judge; "+
			"the root is scanned", "nodeId", scan.NodeID, "rootPath", scan.RootPath, "scopePath", scan.ScopePath,
			"why", err.Error())
		if endErr := scans.end(ctx, scan); endErr != nil {
			return endErr
		}
		return scans.request(ctx, scanAsk{
			nodeID: scan.NodeID, root: scan.RootPath, scope: wholeRoot, priority: scan.Priority,
		})
	}
	if err != nil {
		return err
	}
	if counted, readErr := scans.cache.Read().GetScan(ctx, scan.ID); readErr == nil {
		scan = counted
	}
	scans.logger.InfoContext(ctx, "a node finished a scan", "nodeId", scan.NodeID, "rootPath", scan.RootPath,
		"listedCount", scan.ListedCount, "examinedCount", scan.ExaminedCount, "errorCount", complete.ErrorCount,
		"isStoppedByErrors", complete.IsStoppedByErrors)
	return scans.endThenAgain(ctx, scan)
}

// endThenAgain ends a scan, then starts the one asked for while it ran.
func (scans *Scans) endThenAgain(ctx context.Context, scan cachegen.Scan) error {
	if err := scans.end(ctx, scan); err != nil {
		return err
	}
	if scan.IsAnotherScanQueued == 0 {
		return nil
	}
	again := wholeRoot
	if scan.AgainScopePath != nil {
		again = *scan.AgainScopePath
	}
	return scans.request(ctx, scanAsk{nodeID: scan.NodeID, root: scan.RootPath, scope: again, priority: scan.Priority})
}

// end forgets a scan, its batches and what the ingest held of it.
func (scans *Scans) end(ctx context.Context, scan cachegen.Scan) error {
	delete(scans.completions, scan.ID)
	scans.ingester.AbandonScan(scan.NodeID, scan.RootPath)
	err := scans.cache.Write(ctx, func(queries *cachegen.Queries) error {
		return queries.DeleteScan(ctx, scan.ID)
	})
	if err != nil {
		return fmt.Errorf("end the scan of %s: %w", scan.RootPath, err)
	}
	if scans.onEnded != nil {
		scans.onEnded()
	}
	return nil
}

// readRules is the rules a scan reads by, and how many videos a batch holds.
func (scans *Scans) readRules(ctx context.Context) (ScanRules, int, error) {
	perBatch, err := scans.settings.Int(ctx, settings.ScanVideosPerExamineBatch)
	if err != nil {
		return ScanRules{}, 0, fmt.Errorf("read scan_videos_per_examine_batch: %w", err)
	}
	return scanRules(), int(perBatch), nil
}
