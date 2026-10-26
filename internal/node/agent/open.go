// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/namedfile"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// evict keeps the prepared directory within its budget after a run has added to it (D217 §5).
func (agent *Agent) evict(ctx context.Context) {
	if agent.workers.PreparedShelf == nil {
		return
	}
	if _, err := agent.workers.PreparedShelf.Evict(ctx); err != nil {
		agent.logger.WarnContext(ctx, "could not keep the prepared directory within its budget", "err", err)
	}
}

// refusalCodeOf names a refusal the core says apart: a file changed since the scan.
func refusalCodeOf(err error) nodeproto.RefusalCode {
	if errors.Is(err, namedfile.ErrChanged) {
		return nodeproto.RefusedChanged
	}
	return ""
}

// openNamed opens the regular file named, under one of the node's roots, once its payload hash is
// the one named (D100). The caller closes it. It opens through the node's disk gate: an open that has
// not returned within the open stall window, its disk not answering, is a stall on that disk, as an
// FFmpeg's would be (D233 §10), left to end on its own, listed as stuck until it does; and while it is,
// every other open under its root is the same stall at once, touching nothing (diskwatch).
func (agent *Agent) openNamed(ctx context.Context, named nodeproto.BytesFile) (*os.File, fs.FileInfo, error) {
	path := named.RootPath + "/" + string(named.Path)
	if root, known := agent.roots[named.RootPath]; known {
		path = root.Path() + "/" + string(named.Path)
	}
	window := cmp.Or(agent.workers.OpenStallWindow, defaultOpenStallWindow)
	operation := diskwatch.Operation{Root: named.RootPath, Path: path, Window: window}
	opened, err := diskwatch.Run(ctx, agent.diskGate, operation, func() (openedFile, error) {
		file, facts, openErr := agent.openFile(agent.roots, named)
		return openedFile{file: file, facts: facts}, openErr
	}, func(late openedFile) { readonly.Close(late.file) })
	if stalled, isStalled := errors.AsType[*diskwatch.StalledError](err); isStalled {
		agent.logger.WarnContext(ctx, "a file to read did not open: its disk is not answering", "path", path,
			"waitingOn", stalled.Path, "isBehindAnother", stalled.IsBehindAnother)
		return nil, nil, &execx.StalledError{
			Tool: stuckReadTool, Cause: execx.StallNeverMoved, Condition: execx.ConditionWaitingOnDisk,
			WithoutAdvance: stalled.Waited, WaitingOnPath: path, IsLeftRunning: true, StderrTail: stalled.Error(),
		}
	}
	if diskwatch.IsDiskFailure(err) {
		agent.logger.WarnContext(ctx, "a file to read did not open: its disk failed the read", "path", path, "err", err)
		return nil, nil, &execx.StalledError{
			Tool: stuckReadTool, Cause: execx.StallDiskFailed, Condition: execx.ConditionWaitingOnDisk,
			WaitingOnPath: path, StderrTail: err.Error(),
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("open the file named: %w", err)
	}
	return opened.file, opened.facts, nil
}

// openedFile is a file openNamed opened, with its facts as the open found them.
type openedFile struct {
	file  *os.File
	facts fs.FileInfo
}
