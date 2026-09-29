// SPDX-License-Identifier: GPL-3.0-or-later

package namedfile

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/hash"
)

var (
	// ErrNotRegular is a path that names a directory, device, pipe or anything but a regular file.
	ErrNotRegular = errors.New("not a regular file")
	// ErrUnhashable is a file whose payload hash could not be computed: it could not be read.
	ErrUnhashable = errors.New("the file cannot be read")
	// ErrChanged is a file whose payload hash is not the one named (D100).
	ErrChanged = errors.New("the file at this path is not the one named; it changed since the scan")
)

// maxRemembered bounds the payload hashes remembered between opens. The byte socket's range
// requests come for one file at a time; the bound only stops a long-running node growing.
const maxRemembered = 1024

// Hashes remembers each file's payload hash while its size and time stay the same, so a file
// opened again and again, as a player's range requests open it, is hashed once.
type Hashes struct {
	mutex         sync.Mutex
	hashByVersion map[fileVersion][]byte // guarded by mutex
}

// fileVersion is what a file looked like when its payload hash was computed: a file whose size or
// time changed is hashed again.
type fileVersion struct {
	root, path string
	sizeBytes  int64
	modifiedAt time.Time
}

// NewHashes remembers nothing yet.
func NewHashes() *Hashes {
	return &Hashes{hashByVersion: make(map[fileVersion][]byte)}
}

// Open opens the regular file at relativePath under root, once its payload hash is payloadHash.
// The caller closes it.
func Open(root *confine.MediaRoot, relativePath string, payloadHash []byte) (*os.File, fs.FileInfo, error) {
	return open(root, relativePath, payloadHash, nil)
}

// Open is namedfile.Open, remembering each payload hash it computes.
func (hashes *Hashes) Open(
	root *confine.MediaRoot, relativePath string, payloadHash []byte,
) (*os.File, fs.FileInfo, error) {
	return open(root, relativePath, payloadHash, hashes)
}

func open(
	root *confine.MediaRoot, relativePath string, payloadHash []byte, hashes *Hashes,
) (_ *os.File, _ fs.FileInfo, err error) {
	file, err := root.Open(relativePath)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			_ = file.Close() //nolint:errcheck // read-only, and already failed
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", relativePath, err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s: %w", relativePath, ErrNotRegular)
	}
	version := fileVersion{root.Path(), relativePath, info.Size(), info.ModTime()}
	found, err := hashes.payloadHashOf(file, version)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w: %w", relativePath, ErrUnhashable, err)
	}
	if !bytes.Equal(found, payloadHash) {
		return nil, nil, ErrChanged
	}
	return file, info, nil
}

// payloadHashOf is file's payload hash, remembered while its version stays the same; a nil
// Hashes remembers nothing.
func (hashes *Hashes) payloadHashOf(file *os.File, version fileVersion) ([]byte, error) {
	if hashes != nil {
		hashes.mutex.Lock()
		known, found := hashes.hashByVersion[version]
		hashes.mutex.Unlock()
		if found {
			return known, nil
		}
	}
	identity, err := hash.PayloadIdentity(file, version.sizeBytes)
	if err != nil {
		return nil, err
	}
	computed := identity.PayloadHash[:]
	if hashes != nil {
		hashes.mutex.Lock()
		defer hashes.mutex.Unlock()
		if len(hashes.hashByVersion) >= maxRemembered {
			clear(hashes.hashByVersion)
		}
		hashes.hashByVersion[version] = computed
	}
	return computed, nil
}
