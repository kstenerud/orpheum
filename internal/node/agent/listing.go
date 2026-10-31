// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// errNotGiven is a root path no root the node was given holds (D107).
var errNotGiven = errors.New("this node was not given the root")

// mediaRootOf is the media root a command's root path names: one of the node's configured roots, or a
// library folder inside one, reached without a link and opened as a root of its own, so nothing
// outside it is read (D359 §1); done closes what it opened. A path no root holds, ".." among them, is
// refused, whatever the core asks (D107).
func (agent *Agent) mediaRootOf(rootPath string) (*confine.MediaRoot, func(), error) {
	root, relative, isHeld := confine.Holding(agent.roots, rootPath)
	if !isHeld {
		return nil, nil, fmt.Errorf("%w %s", errNotGiven, rootPath)
	}
	if relative == "." {
		return root, func() {}, nil
	}
	folder, err := root.Folder(relative)
	if err != nil {
		return nil, nil, fmt.Errorf("the library folder %s: %w", rootPath, err)
	}
	return folder, func() { _ = folder.Close() }, nil //nolint:errcheck // read-only
}

// gateRootOf is the configured root holding path, by which the disk gate keys a reading of it, so
// one stuck share holds one reading however many library folders it holds; path itself when no
// root holds it.
func (agent *Agent) gateRootOf(path string) string {
	if root, _, isHeld := confine.Holding(agent.roots, path); isHeld {
		return root.Path()
	}
	return path
}

// listWork is a listing held until its device has a reader free, which posts what it found to the
// core (D240 §2). Its start and end are logged with how long each wait took: where the minute a root
// took to come back went was not in the log (staging, 2026-10-21).
func (agent *Agent) listWork(ctx context.Context, nodeID int64, held attempt, command nodeproto.List) heldWork {
	heldAt := agent.clock.Now()
	return agent.answeringWork(ctx, nodeID, answeringJob{
		attempt: held, jobKind: "a listing", reading: agent.gateRootOf(command.RootPath),
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			startedAt := agent.clock.Now()
			agent.logger.InfoContext(ctx, "a listing started", "jobId", held.jobID, "rootPath", command.RootPath,
				"path", command.Path, "waitedMs", startedAt.Sub(heldAt).Milliseconds())
			listing := agent.runList(jobCtx, command)
			agent.logger.InfoContext(ctx, "a listing ended", "jobId", held.jobID, "rootPath", command.RootPath,
				"recordCount", len(listing.Records), "tookMs", agent.clock.Now().Sub(startedAt).Milliseconds(),
				"refusal", listing.Refusal)
			return nodeproto.Result{Listing: &listing}, listing.Refusal
		},
		stalledAnswer: func(stall nodeproto.Stall) nodeproto.Result {
			return nodeproto.Result{Listing: &nodeproto.Listing{Stall: &stall}}
		},
	})
}

// runList lists under one of the node's roots, or a library folder inside one. A root the node was
// not given is refused, whatever the core asks (D107), and so is a folder reached through a link; a
// library folder that cannot be opened, gone or unreadable, is listed as a root that could not be
// read (scan.FolderNotOpened); a listing stopped partway says so with its records, and never that it
// is complete.
func (agent *Agent) runList(ctx context.Context, command nodeproto.List) nodeproto.Listing {
	root, done, err := agent.mediaRootOf(command.RootPath)
	if err != nil && !errors.Is(err, errNotGiven) && !errors.Is(err, confine.ErrNotARealFolder) {
		agent.logger.WarnContext(ctx, "a library folder could not be opened; it is listed as unreadable",
			"rootPath", command.RootPath, "err", err)
		return nodeproto.Listing{Records: scan.FolderNotOpened(command.RootPath, os.Geteuid(), os.Getegid(), err)}
	}
	if err != nil {
		agent.logger.WarnContext(ctx, "refused a listing", "rootPath", command.RootPath, "err", err)
		return nodeproto.Listing{Refusal: err.Error()}
	}
	defer done()
	records, err := agent.workers.Scanner.List(ctx, root, command)
	if err != nil && ctx.Err() == nil {
		agent.logger.WarnContext(ctx, "a listing ended early", "rootPath", root.Path(), "err", err)
	}
	return nodeproto.Listing{Records: records}
}

// examineWork is an examining held until its device has a reader free, which posts what it read to
// the core (D240 §4). Between one file and the next it waits while it is paused, as a run of FFmpeg
// is stopped, for play-driven work on its device (D240 §6).
func (agent *Agent) examineWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.Examine,
) heldWork {
	return agent.answeringWork(ctx, nodeID, answeringJob{
		attempt: held, jobKind: "an examining", reading: agent.gateRootOf(command.RootPath),
		stalledAnswer: func(stall nodeproto.Stall) nodeproto.Result {
			return nodeproto.Result{Examined: &nodeproto.Examined{Stall: &stall}}
		},
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			root, done, err := agent.mediaRootOf(command.RootPath)
			if err != nil {
				agent.logger.WarnContext(ctx, "refused an examining", "rootPath", command.RootPath, "err", err)
				return nodeproto.Result{Examined: &nodeproto.Examined{Refusal: err.Error()}}, err.Error()
			}
			defer done()
			between := func() error { return agent.work.waitWhilePaused(jobCtx, held.token) }
			records, err := agent.workers.Scanner.Examine(jobCtx, root, command.Files, between)
			if err != nil && jobCtx.Err() == nil {
				agent.logger.WarnContext(ctx, "an examining ended early", "rootPath", root.Path(), "err", err)
			}
			if err == nil {
				records = append(records, agent.checkChunksOf(jobCtx, command.Files)...)
			}
			return nodeproto.Result{Examined: &nodeproto.Examined{Records: records}}, ""
		},
	})
}

// checkChunksOf checks the stores of chunks of each file examined that a full rescan asked it to
// (nodeproto.ExamineFile.ChecksChunksOf) against their records, dropping what is missing, damaged or
// unrecorded, and says what it found of each in a chunks_checked record (D392 §4). A node with no
// prepared directory checks nothing.
func (agent *Agent) checkChunksOf(ctx context.Context, files []nodeproto.ExamineFile) []nodeproto.ScanRecord {
	if agent.workers.PreparedShelf == nil {
		return nil
	}
	var records []nodeproto.ScanRecord
	for _, file := range files {
		if file.ChecksChunksOf == 0 || ctx.Err() != nil {
			continue
		}
		checked := agent.workers.PreparedShelf.CheckStoresOf(ctx, file.ChecksChunksOf)
		records = append(records, nodeproto.ScanRecord{Type: nodeproto.RecordChunksChecked, ChunksChecked: &checked})
	}
	return records
}
