// SPDX-License-Identifier: GPL-3.0-or-later

package prepared

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A store holds one stream of a film cut into chunks (D247 §10), at <file_id>/chunks-<stream>/<grid_id>
// in the prepared directory, beside the renditions and in the same budget. Each run of chunks
// writes FFmpeg's raw files in a raw directory of its own inside the store, and takes them out as
// it makes them into chunks.

// rawPrefix begins each run's raw directory's name inside its store.
const rawPrefix = "raw-"

// OpenStore makes the store, with its manifest when it has none, and a raw directory for one run's
// FFmpeg, returning its absolute path. The manifest is written whole or not at all.
func (shelf *Shelf) OpenStore(manifest nodeproto.StoreManifest) (string, error) {
	storePath := filepath.Join(shelf.directory, filepath.FromSlash(manifest.StoreDir))
	if err := os.MkdirAll(storePath, 0o750); err != nil {
		return "", fmt.Errorf("make the store %s: %w", manifest.StoreDir, err)
	}
	if _, found := shelf.readStoreManifest(manifest.StoreDir); !found {
		encoded, err := json.Marshal(manifest)
		if err != nil {
			return "", fmt.Errorf("encode %s's manifest: %w", manifest.StoreDir, err)
		}
		if err := shelf.PutFile(manifest.StoreDir, nodeproto.StoreManifestName, encoded); err != nil {
			return "", err
		}
	}
	rawPath, err := os.MkdirTemp(storePath, rawPrefix)
	if err != nil {
		return "", fmt.Errorf("make a raw directory in %s: %w", manifest.StoreDir, err)
	}
	return rawPath, nil
}

// HasFile is whether the store holds name.
func (shelf *Shelf) HasFile(storeDir, name string) bool {
	info, err := os.Stat(filepath.Join(shelf.directory, filepath.FromSlash(storeDir), name))
	return err == nil && info.Mode().IsRegular()
}

// PutFile writes name into the store whole: to a part file first, then renamed into place, so no
// reader ever sees part of it.
func (shelf *Shelf) PutFile(storeDir, name string, contents []byte) error {
	path := filepath.Join(shelf.directory, filepath.FromSlash(storeDir), name)
	partPath := path + ".part"
	if err := os.WriteFile(partPath, contents, 0o600); err != nil {
		return fmt.Errorf("write %s/%s: %w", storeDir, name, err)
	}
	if err := os.Rename(partPath, path); err != nil {
		return fmt.Errorf("write %s/%s: %w", storeDir, name, err)
	}
	return nil
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
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("close the run %s: %w", rawPath, err)
	}
	return nil
}

// readStoreManifest is a store's manifest, and whether it has one that names it.
func (shelf *Shelf) readStoreManifest(storeDir string) (nodeproto.StoreManifest, bool) {
	var manifest nodeproto.StoreManifest
	path := filepath.Join(shelf.directory, filepath.FromSlash(storeDir), nodeproto.StoreManifestName)
	encoded, err := os.ReadFile(path) //nolint:gosec // the node's own prepared directory
	if err != nil || json.Unmarshal(encoded, &manifest) != nil {
		return manifest, false
	}
	return manifest, manifest.StoreDir == storeDir
}
