// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/internal/atomics"
	"github.com/kstenerud/orpheum/discovery/internal/strictjson"
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
	// HolderInstallationID is the installation that last held its realm, admitted at once when it
	// asks again; empty while none has, or the last was a core before D437 (D437 §5).
	HolderInstallationID string `json:"holderInstallationId,omitempty"`
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
	if err = strictjson.Unmarshal(contents, &record); err != nil {
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

// writeWhole writes contents to path whole and durably, mode 0600, through the module's atomics
// (discovery/internal/atomics, a twin of internal/platform/atomics).
func writeWhole(path string, contents []byte) (err error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open the record's directory: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err = atomics.WriteFile(root, filepath.Base(path), contents, 0o600); err != nil {
		return fmt.Errorf("write the record: %w", err)
	}
	return nil
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
