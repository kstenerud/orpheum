// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/atomics"
	"github.com/kstenerud/orpheum/internal/platform/strictjson"
)

// A store holds one stream of a film cut into chunks (D247 §10), at <file_id>/chunks-<stream>/<grid_id>
// in the prepared directory, beside the runs and in the same budget. Each run of chunks
// writes FFmpeg's raw files in a raw directory of its own inside the store, and takes them out as
// it makes them into chunks.

// rawPrefix begins each run's raw directory's name inside its store.
const rawPrefix = "raw-"

// OpenStore makes the store, with its manifest when it has none, and a raw directory for one run's
// FFmpeg, returning its absolute path. The manifest is written whole or not at all, and says its
// files carry records (chunkRecordsVersion); a store made before they did is dropped first, as one
// that cannot be checked (D392 §4).
func (shelf *Shelf) OpenStore(ctx context.Context, manifest nodeproto.StoreManifest) (string, error) {
	storeDir := manifest.StoreDir
	storePath := filepath.Join(shelf.directory, filepath.FromSlash(storeDir))
	lock := shelf.storeLock(storeDir)
	lock.Lock()
	defer lock.Unlock()
	kept, found := shelf.readStoreManifest(storeDir)
	if found && kept.ChunkRecords != chunkRecordsVersion {
		if err := shelf.dropUnrecordedStore(ctx, storeDir); err != nil {
			return "", err
		}
		found = false
	}
	if err := os.MkdirAll(storePath, 0o750); err != nil {
		return "", fmt.Errorf("make the store %s: %w", storeDir, err)
	}
	shelf.knowStore(storeDir)
	if !found {
		manifest.ChunkRecords = chunkRecordsVersion
		encoded, err := json.Marshal(manifest)
		if err != nil {
			return "", fmt.Errorf("encode %s's manifest: %w", storeDir, err)
		}
		if err := shelf.putWhole(storeDir, nodeproto.StoreManifestName, encoded); err != nil {
			return "", err
		}
	}
	rawPath, err := os.MkdirTemp(storePath, rawPrefix)
	if err != nil {
		return "", fmt.Errorf("make a raw directory in %s: %w", manifest.StoreDir, err)
	}
	shelf.sizes.opened(manifest.StoreDir, rawPath)
	return rawPath, nil
}

// HasFile is whether the store holds name.
func (shelf *Shelf) HasFile(storeDir, name string) bool {
	info, err := os.Stat(filepath.Join(shelf.directory, filepath.FromSlash(storeDir), name))
	return err == nil && info.Mode().IsRegular()
}

// PutFile writes name, the store's init or a chunk, into the store whole, so no reader ever sees part
// of it, and then its record, its size and CRC-32C, beside it (recordName), the store's lock held
// across both, so a check never sees the one without the other (D392 §4). Not durably: a chunk is
// made again when a crash lost it, or its record, and a sync for each would slow the making.
func (shelf *Shelf) PutFile(storeDir, name string, contents []byte) error {
	lock := shelf.storeLock(storeDir)
	lock.Lock()
	defer lock.Unlock()
	if err := shelf.putWhole(storeDir, name, contents); err != nil {
		return err
	}
	return shelf.putWhole(storeDir, recordName(name), recordOf(contents).encoded())
}

// putWhole writes name into the store whole. The store's size grows by what it adds, the file it
// replaces apart.
func (shelf *Shelf) putWhole(storeDir, name string, contents []byte) error {
	storePath := filepath.Join(shelf.directory, filepath.FromSlash(storeDir))
	shelf.knowStore(storeDir)
	var replacedBytes int64
	if info, err := os.Lstat(filepath.Join(storePath, name)); err == nil && info.Mode().IsRegular() {
		replacedBytes = info.Size()
	}
	if err := replaceIn(storePath, name, contents); err != nil {
		shelf.sizes.forget(storeDir) // what it left is not known: walked again when next found
		return err
	}
	shelf.sizes.grew(storeDir, int64(len(contents))-replacedBytes)
	return nil
}

// knowStore makes sure the shelf knows the store's size before a file is put in it, so the put adds
// itself to it once (runSizes.walking). A store that cannot be measured stays unknown, and is
// walked, the file in it, when next found.
func (shelf *Shelf) knowStore(storeDir string) {
	storePath := filepath.Join(shelf.directory, filepath.FromSlash(storeDir))
	_, _ = shelf.knownSize(storeDir, storePath) //nolint:errcheck // see above
}

// replaceIn writes name in directory whole (atomics.Replace).
func replaceIn(directory, name string, contents []byte) (err error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("write %s: %w", filepath.Join(directory, name), err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if err = atomics.Replace(root, name, contents, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Join(directory, name), err)
	}
	return nil
}

// openInStore opens one of a store's files to read, never through a link out of the prepared
// directory.
func (shelf *Shelf) openInStore(storeDir, name string) (*os.File, error) {
	return os.OpenInRoot(shelf.directory, filepath.Join(filepath.FromSlash(storeDir), name))
}

// errNotRaw is a raw directory or file outside the shelf's stores.
var errNotRaw = errors.New("not a run's raw directory in a store")

// rawDirIn is rawPath, once it is a run's raw directory inside a store on this shelf.
func (shelf *Shelf) rawDirIn(rawPath string) (string, error) {
	relative, err := filepath.Rel(shelf.directory, rawPath)
	if err != nil || !strings.HasPrefix(filepath.Base(rawPath), rawPrefix) ||
		strings.Count(filepath.ToSlash(relative), "/") != 3 || strings.HasPrefix(relative, "..") {
		return "", fmt.Errorf("%w: %s", errNotRaw, rawPath)
	}
	return rawPath, nil
}

// DiscardRaw removes one of a run's raw files, once it is made into a chunk or dropped.
func (shelf *Shelf) DiscardRaw(rawPath, name string) error {
	dir, err := shelf.rawDirIn(rawPath)
	if err != nil {
		return err
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("%w: %s", errNotRaw, name)
	}
	if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("discard %s: %w", name, err)
	}
	return nil
}

// CloseRun removes a run's raw directory and what is left in it, once the run has ended.
func (shelf *Shelf) CloseRun(rawPath string) error {
	dir, err := shelf.rawDirIn(rawPath)
	if err != nil {
		return err
	}
	storeDir, err := filepath.Rel(shelf.directory, filepath.Dir(dir))
	if err != nil {
		return fmt.Errorf("close the run %s: %w", rawPath, err)
	}
	removeErr := os.RemoveAll(dir)
	shelf.sizes.closed(filepath.ToSlash(storeDir), dir)
	if removeErr != nil {
		shelf.sizes.forget(filepath.ToSlash(storeDir)) // what it left is not known: walked again
		return fmt.Errorf("close the run %s: %w", rawPath, removeErr)
	}
	return nil
}

// readStoreManifest is a store's manifest, and whether it has one that names it.
func (shelf *Shelf) readStoreManifest(storeDir string) (nodeproto.StoreManifest, bool) {
	var manifest nodeproto.StoreManifest
	path := filepath.Join(shelf.directory, filepath.FromSlash(storeDir), nodeproto.StoreManifestName)
	encoded, err := os.ReadFile(path) //nolint:gosec // the node's own prepared directory
	if err != nil || strictjson.Unmarshal(encoded, &manifest) != nil {
		return manifest, false
	}
	return manifest, manifest.StoreDir == storeDir
}
