// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// Core is the one core a single-target discovery holds.
type Core struct {
	ID        string               `json:"id"`
	PublicKey []byte               `json:"publicKey"`
	State     discoveryproto.State `json:"state"`
	// ClaimCode is the code its owner claims it by while it is pending, until ClaimExpiresAt (Unix
	// ms).
	ClaimCode      string `json:"claimCode,omitempty"`
	ClaimExpiresAt int64  `json:"claimExpiresAt,omitempty"`
	// Addresses is its last heartbeat, and LastSeenAt when it came (Unix ms); none before the first.
	Addresses  *discoveryproto.AddressesRequest `json:"addresses,omitempty"`
	LastSeenAt int64                            `json:"lastSeenAt,omitempty"`
}

// Record is what a single-target discovery keeps: its core, or none.
type Record struct {
	Core *Core `json:"core,omitempty"`
}

// Store keeps the record. Update changes it under a lock, read and written whole; the server and
// `orpheum discovery claim`, two processes, change it through the same store.
type Store interface {
	Read() (Record, error)
	Update(change func(record *Record) error) error
}

// FileStore keeps the record in one JSON file, mode 0600, under an advisory lock on a file beside it.
type FileStore struct {
	Path string
}

// Read is the record as the file holds it; an absent file is no core.
func (store FileStore) Read() (Record, error) {
	var record Record
	err := store.locked(func() error {
		var readErr error
		record, readErr = store.read()
		return readErr
	})
	return record, err
}

// Update reads the record, changes it, and writes it back whole, the lock held throughout. A change
// that fails writes nothing.
func (store FileStore) Update(change func(record *Record) error) error {
	return store.locked(func() error {
		record, err := store.read()
		if err != nil {
			return err
		}
		if changeErr := change(&record); changeErr != nil {
			return changeErr
		}
		return store.write(record)
	})
}

func (store FileStore) read() (Record, error) {
	var record Record
	contents, err := os.ReadFile(store.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return record, nil
	}
	if err != nil {
		return record, fmt.Errorf("read the discovery's record: %w", err)
	}
	if err = json.Unmarshal(contents, &record); err != nil {
		return record, fmt.Errorf("read the discovery's record %s: %w", store.Path, err)
	}
	return record, nil
}

// write writes record whole, through a temporary file synced and renamed over it, so a crash leaves
// the old record or the whole new one, never one half written.
func (store FileStore) write(record Record) error {
	contents, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("write the discovery's record: %w", err)
	}
	if err = writeWhole(store.Path, contents); err != nil {
		return fmt.Errorf("write the discovery's record: %w", err)
	}
	return nil
}

// writeWhole writes contents to path whole and durably, mode 0600: a temporary file beside it,
// synced, renamed over it, and the directory synced. A copy of internal/platform/atomics' WriteFile,
// which a public package cannot import (the owner, 2026-10-14: duplicate, and watch for more;
// look-alike-watch-list.md).
func writeWhole(path string, contents []byte) (err error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open the record's directory: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	name := filepath.Base(path)
	temporary := "." + name + "." + rand.Text()
	handle, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create a temporary file for %s: %w", name, err)
	}
	defer func() {
		if err != nil {
			if removeErr := root.Remove(temporary); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	if _, err = handle.Write(contents); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", temporary, err), handle.Close())
	}
	if err = handle.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync %s: %w", temporary, err), handle.Close())
	}
	if err = handle.Close(); err != nil {
		return fmt.Errorf("close %s: %w", temporary, err)
	}
	if err = root.Rename(temporary, name); err != nil {
		return fmt.Errorf("move %s into place: %w", name, err)
	}
	directory, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("open the record's directory to sync it: %w", err)
	}
	return errors.Join(directory.Sync(), directory.Close())
}

// locked runs work holding an exclusive lock on the record's lock file.
func (store FileStore) locked(work func() error) error {
	if err := os.MkdirAll(filepath.Dir(store.Path), 0o700); err != nil {
		return fmt.Errorf("make the discovery's directory: %w", err)
	}
	lock, err := os.OpenFile(store.Path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open the discovery's lock: %w", err)
	}
	defer func() { _ = lock.Close() }() //nolint:errcheck // closing releases the lock; nothing was written
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock the discovery's record: %w", err)
	}
	return work()
}
