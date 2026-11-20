// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"errors"
	"syscall"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// IsStorageFailure is whether err is the disk under a database failing, for want of space or
// with an I/O error, rather than anything about what was written: a destination that is down
// (D231 §5), which parks the work that writes until a write succeeds again.
func IsStorageFailure(err error) bool {
	if err == nil {
		return false
	}
	if failure, isSQLite := errors.AsType[*sqlite.Error](err); isSQLite {
		// The primary code is the low byte of an extended one (SQLITE_IOERR_WRITE, ...).
		switch failure.Code() & 0xff {
		case sqlite3.SQLITE_FULL, sqlite3.SQLITE_IOERR:
			return true
		}
	}
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EIO) || errors.Is(err, syscall.EDQUOT)
}

// codedFailure is an error carrying SQLite's extended result code, as the driver's *sqlite.Error
// does; it is matched by its method rather than its type, since that type cannot be made outside
// the driver and a test stands one in.
type codedFailure interface {
	error
	Code() int
}

// isSharedMemoryRefused is whether err is SQLite's SQLITE_IOERR_SHMMAP (5386): the filesystem
// under the database refused the shared memory map of its -shm file that WAL needs, as a mergerfs
// pool mounted cache.files=off does (idiosyncrasies.md, "mergerfs mounted cache.files=off refuses
// the shared memory map SQLite's WAL needs").
func isSharedMemoryRefused(err error) bool {
	failure, isCoded := errors.AsType[codedFailure](err)
	return isCoded && failure.Code() == sqlite3.SQLITE_IOERR_SHMMAP
}
