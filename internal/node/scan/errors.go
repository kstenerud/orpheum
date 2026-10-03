// SPDX-License-Identifier: GPL-3.0-or-later

package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"syscall"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// errProbe marks a failure to look inside a file that could be opened: probe_failed.
var errProbe = errors.New("probe failed")

// errTooManyErrors is returned once a root has produced MaxErrors errors.
var errTooManyErrors = errors.New("too many errors; the node stopped enumerating this root")

// errorLog turns errors into error records: it classifies each into the closed vocabulary,
// collapses repeats of one code and scope in one directory into one record with a count, and
// stops the root after too many (D40).
type errorLog struct {
	emit             Emit
	maxErrors        int
	pending          *nodeproto.ScanError
	pendingDirectory string
	errorCount       int
	isStopped        bool
	// emitErr is the first emit failure; once set, the walk stops.
	emitErr error
}

func newErrorLog(emit Emit, maxErrors int) *errorLog {
	return &errorLog{emit: emit, maxErrors: maxErrors}
}

// record notes one error at reported.
func (log *errorLog) record(reported string, scope nodeproto.ErrorScope, cause error, info fs.FileInfo) {
	if log.emitErr != nil || log.isStopped {
		return
	}
	log.errorCount++
	code := classify(cause)
	directory := path.Dir(reported)
	if log.pending != nil && log.pending.Code == code && log.pending.Scope == scope &&
		log.pendingDirectory == directory {
		log.pending.OccurrenceCount++
	} else {
		log.emitErr = log.flush()
		log.pending = &nodeproto.ScanError{
			Path:            toNodePath(reported),
			Scope:           scope,
			Code:            code,
			OccurrenceCount: 1,
			PlatformError:   platformError(cause, info),
		}
		log.pendingDirectory = directory
	}
	if log.maxErrors > 0 && log.errorCount >= log.maxErrors && scope != nodeproto.ScopeRun {
		if flushErr := log.flush(); log.emitErr == nil {
			log.emitErr = flushErr
		}
		log.isStopped = true
		stop := fmt.Errorf("%w (%d)", errTooManyErrors, log.errorCount)
		log.pending = &nodeproto.ScanError{
			Path: toNodePath("."), Scope: nodeproto.ScopeRoot, Code: code, OccurrenceCount: 1,
			PlatformError: platformError(stop,
				nil),
		}
		if flushErr := log.flush(); log.emitErr == nil {
			log.emitErr = flushErr
		}
	}
}

// flush emits the pending record, if any.
func (log *errorLog) flush() error {
	if log.pending == nil {
		return nil
	}
	pending := log.pending
	log.pending = nil
	return log.emit(nodeproto.ScanRecord{Type: nodeproto.RecordError, Error: pending})
}

// classify maps an error to the project's closed vocabulary (D40).
func classify(cause error) nodeproto.ErrorCode {
	switch {
	case errors.Is(cause, context.Canceled):
		return nodeproto.CodeCancelled
	case errors.Is(cause, context.DeadlineExceeded):
		return nodeproto.CodeTimeout
	case errors.Is(cause, errProbe):
		return nodeproto.CodeProbeFailed
	case errors.Is(cause, fs.ErrPermission):
		return nodeproto.CodePermissionDenied
	case errors.Is(cause, fs.ErrNotExist):
		return nodeproto.CodeNotFound
	}
	return nodeproto.CodeIOError
}

// platformError is the platform's account of an error: its text, and the path's owner, group and mode
// when the node could stat it (D163 §8).
func platformError(cause error, info fs.FileInfo) nodeproto.PlatformError {
	described := nodeproto.PlatformError{Text: cause.Error(), OwnerUID: -1, GroupGID: -1}
	if info == nil {
		return described
	}
	described.ModeText = info.Mode().String()
	if system, ok := info.Sys().(*syscall.Stat_t); ok {
		described.OwnerUID = int(system.Uid)
		described.GroupGID = int(system.Gid)
	}
	return described
}
