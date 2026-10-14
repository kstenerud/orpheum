// SPDX-License-Identifier: AGPL-3.0-or-later

package atomics

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/kstenerud/orpheum/internal/platform/errs"
)

// WriteFile writes contents to name under root, whole and durably, with mode.
func WriteFile(root *os.Root, name string, contents []byte, mode os.FileMode) error {
	return WriteStream(root, name, mode, func(output io.Writer) error {
		if _, err := output.Write(contents); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		return nil
	})
}

// WriteStream writes name under root as write produces it, through a temporary file beside it,
// synced, then renamed over the name, and the directory synced: a crash leaves the old file or the
// whole new one.
func WriteStream(root *os.Root, name string, mode os.FileMode, write func(io.Writer) error) error {
	return writeThrough(root, name, mode, write, true)
}

// Replace writes contents to name under root whole, but not durably: no reader sees part of it, but
// a crash may lose it. For what can be made again.
func Replace(root *os.Root, name string, contents []byte, mode os.FileMode) error {
	return writeThrough(root, name, mode, func(output io.Writer) error {
		if _, err := output.Write(contents); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		return nil
	}, false)
}

func writeThrough(
	root *os.Root, name string, mode os.FileMode, write func(io.Writer) error, isDurable bool,
) (err error) {
	temporary := "." + name + "." + rand.Text()
	handle, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create a temporary file for %s: %w", name, err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, ignoreMissing(root.Remove(temporary)))
		}
	}()
	if err = write(handle); err != nil {
		return errors.Join(err, handle.Close())
	}
	if isDurable {
		if err = handle.Sync(); err != nil {
			return errors.Join(fmt.Errorf("sync %s: %w", temporary, err), handle.Close())
		}
	}
	if err = handle.Close(); err != nil {
		return fmt.Errorf("close %s: %w", temporary, err)
	}
	if err = root.Rename(temporary, name); err != nil {
		return fmt.Errorf("move %s into place: %w", name, err)
	}
	if !isDurable {
		return nil
	}
	return SyncDirectory(root)
}

// SyncDirectory makes a rename in root's directory durable.
func SyncDirectory(root *os.Root) error {
	handle, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("open %s to sync it: %w", root.Name(), err)
	}
	return errors.Join(
		errs.WrapIf("sync "+root.Name(), handle.Sync()), errs.WrapIf("close "+root.Name(), handle.Close()),
	)
}

func ignoreMissing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
