// SPDX-License-Identifier: AGPL-3.0-or-later

package namedfile

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

var (
	// ErrNotRegular is a path that names a directory, device, pipe or anything but a regular file.
	ErrNotRegular = errors.New("not a regular file")
	// ErrUnhashable is a file whose payload hash could not be computed: it could not be read.
	ErrUnhashable = errors.New("the file cannot be read")
	// ErrChanged is a file whose payload hash is not the one named (D100).
	ErrChanged = errors.New("the file at this path is not the one named; it changed since the scan")
	// ErrNoRoot is a file named under a root this node was not given (D107).
	ErrNoRoot = errors.New("the file is under none of this node's roots")
	// ErrBadName is a file named with no path, or with a payload hash that is not hex.
	ErrBadName = errors.New("a file is named by its root, a path under it and its payload hash in hex")
)

// maxRemembered bounds the payload hashes remembered between opens. The byte socket's range
// requests and the agent's runs come for a few files at a time; the bound only stops a long-running
// node growing.
const maxRemembered = 1024

// Hashes remembers each file's payload hash while its inode, size and times stay the same, so a file
// opened again and again, as a player's range requests and each run of chunks after a seek open it,
// is hashed once. The node keeps one, which its agent and byte socket share.
type Hashes struct {
	mutex         sync.Mutex
	hashByVersion map[fileVersion][]byte // guarded by mutex
	computed      int                    // guarded by mutex
}

// fileVersion is what a file looked like when its payload hash was computed: a file whose inode,
// size, modification time or inode change time (ctime) changed is hashed again. Size and mtime
// alone pass a same-size replacement that keeps its mtime (cp -p, rsync -t); the inode and ctime
// catch it, since nothing short of changing the clock sets a ctime back.
type fileVersion struct {
	root, path string
	sizeBytes  int64
	modifiedAt time.Time
	inode      uint64
	changedAt  time.Time
}

// NewHashes remembers nothing yet.
func NewHashes() *Hashes {
	return &Hashes{hashByVersion: make(map[fileVersion][]byte)}
}

// Computed is how many payload hashes these Hashes have computed, each a read of a file's two ends: a file
// opened again unchanged adds none.
func (hashes *Hashes) Computed() int {
	hashes.mutex.Lock()
	defer hashes.mutex.Unlock()
	return hashes.computed
}

// Open opens the regular file at relativePath under root, once its payload hash is payloadHash.
// The caller closes it.
func Open(root *confine.MediaRoot, relativePath string, payloadHash []byte) (*os.File, fs.FileInfo, error) {
	return open(root, relativePath, payloadHash, nil)
}

// OpenNamed is Open for the file a command names: its root, found among roots by the path it has
// (D107), its path there, and its payload hash in hex.
func OpenNamed(roots map[string]*confine.MediaRoot, named nodeproto.BytesFile) (*os.File, fs.FileInfo, error) {
	return openNamed(roots, named, nil)
}

// OpenNamed is namedfile.OpenNamed, remembering each payload hash it computes.
func (hashes *Hashes) OpenNamed(
	roots map[string]*confine.MediaRoot, named nodeproto.BytesFile,
) (*os.File, fs.FileInfo, error) {
	return openNamed(roots, named, hashes)
}

func openNamed(
	roots map[string]*confine.MediaRoot, named nodeproto.BytesFile, hashes *Hashes,
) (*os.File, fs.FileInfo, error) {
	// The root path is a configured root's, or a library folder's inside one (D359 §1).
	root, folder, known := confine.Holding(roots, named.RootPath)
	if !known {
		return nil, nil, ErrNoRoot
	}
	payloadHash, err := hex.DecodeString(named.PayloadHash)
	if err != nil || len(payloadHash) == 0 || len(named.Path) == 0 {
		return nil, nil, ErrBadName
	}
	if folder == "." {
		return open(root, string(named.Path), payloadHash, hashes)
	}
	// os.Root keeps the open inside the configured root; this keeps it inside the library folder.
	if !filepath.IsLocal(string(named.Path)) {
		return nil, nil, ErrBadName
	}
	return open(root, filepath.Join(folder, string(named.Path)), payloadHash, hashes)
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
			readonly.Close(file)
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", relativePath, err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s: %w", relativePath, ErrNotRegular)
	}
	inode, changedAt := inodeAndChangedAt(info)
	version := fileVersion{
		root: root.Path(), path: relativePath, sizeBytes: info.Size(), modifiedAt: info.ModTime(),
		inode: inode, changedAt: changedAt,
	}
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
		hashes.computed++
		if len(hashes.hashByVersion) >= maxRemembered {
			clear(hashes.hashByVersion)
		}
		hashes.hashByVersion[version] = computed
	}
	return computed, nil
}
