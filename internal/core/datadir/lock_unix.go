// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package datadir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/kstenerud/orpheum/internal/platform/errs"
)

// lockFileName is the state directory's lock, held by a running server for its whole life.
const lockFileName = "server.lock"

// ErrInUse is returned when another process holds the state directory's lock: a running server.
var ErrInUse = errors.New("the state directory is in use by a running server; stop it first")

// Lock is an exclusive hold on the state directory.
type Lock struct {
	file *os.File
}

// LockState takes the state directory's lock without waiting. A running server holds it, so
// `orpheum restore`, which must run with the server stopped, cannot replace a database in use.
// The kernel drops the lock when the process ends, however it ends.
func (directories *Directories) LockState() (*Lock, error) {
	path := filepath.Join(directories.stateDirectory, lockFileName)
	root, err := directories.openStateRoot()
	if err != nil {
		return nil, err
	}
	file, err := root.OpenFile(lockFileName, os.O_RDWR|os.O_CREATE, fileMode)
	err = errors.Join(errs.WrapIf("open "+path, err), errs.WrapIf("close the state directory", root.Close()))
	if err != nil {
		if file != nil {
			err = errors.Join(err, file.Close())
		}
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		closeErr := file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.Join(ErrInUse, closeErr)
		}
		return nil, errors.Join(fmt.Errorf("lock %s: %w", path, err), closeErr)
	}
	return &Lock{file: file}, nil
}

// Release gives the lock up.
func (lock *Lock) Release() error {
	if err := lock.file.Close(); err != nil {
		return fmt.Errorf("release the state lock: %w", err)
	}
	return nil
}
