// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Shelve writes a made rendition's manifest beside it (D238 §2), stamped with when it was made: a
// run directory without one is a making that never finished. It is written whole or not at all.
func (shelf *Shelf) Shelve(manifest nodeproto.RenditionManifest) error {
	manifest.MadeAtMs = shelf.clock.Now().UnixMilli()
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode %s's manifest: %w", manifest.Prepare.RenditionDir, err)
	}
	// Not durably: a manifest a crash lost, or left unreadable, is a making that never finished,
	// which the inventory passes over and the next prepare makes again.
	runPath := filepath.Join(shelf.directory, filepath.FromSlash(manifest.Prepare.RenditionDir))
	if err := replaceIn(runPath, nodeproto.ManifestName, encoded); err != nil {
		return fmt.Errorf("write %s's manifest: %w", manifest.Prepare.RenditionDir, err)
	}
	return nil
}

// Inventory is every made rendition on the shelf, by its manifest, read from the disk alone: what
// the node's hello lists. A run directory whose manifest is missing or unreadable is not one.
func (shelf *Shelf) Inventory() ([]nodeproto.RenditionManifest, error) {
	runs, err := shelf.listRuns()
	if err != nil {
		return nil, err
	}
	manifests := make([]nodeproto.RenditionManifest, 0, len(runs))
	for _, each := range runs {
		if manifest, found := shelf.readManifest(each.renditionDir); found {
			manifests = append(manifests, manifest)
		}
	}
	return manifests, nil
}

// Reconcile evicts what the core does not keep: each of the listed renditions, by directory, that
// kept does not name, and every run directory without a manifest, a making that never finished,
// unless it is being written. A rendition made after the listing, which the core has not seen, is
// left for the next. It returns the directories it evicted.
func (shelf *Shelf) Reconcile(listed, kept []string) ([]string, error) {
	runs, err := shelf.listRuns()
	if err != nil {
		return nil, err
	}
	var evicted []string
	for _, each := range runs {
		if shelf.isWriting(each.renditionDir) {
			continue
		}
		_, hasManifest := shelf.readManifest(each.renditionDir)
		if _, isStore := shelf.readStoreManifest(each.renditionDir); isStore {
			hasManifest = true // a store is the node's own to keep: the core lists renditions alone
		}
		isDropped := slices.Contains(listed, each.renditionDir) && !slices.Contains(kept, each.renditionDir)
		if hasManifest && !isDropped {
			continue
		}
		if err := shelf.removeRun(each.renditionDir); err != nil {
			return evicted, err
		}
		evicted = append(evicted, each.renditionDir)
	}
	return evicted, nil
}

// readManifest is a run directory's manifest, and whether it has one that names it.
func (shelf *Shelf) readManifest(renditionDir string) (nodeproto.RenditionManifest, bool) {
	var manifest nodeproto.RenditionManifest
	path := filepath.Join(shelf.directory, filepath.FromSlash(renditionDir), nodeproto.ManifestName)
	encoded, err := os.ReadFile(path) //nolint:gosec // the node's own prepared directory
	if err != nil || json.Unmarshal(encoded, &manifest) != nil {
		return manifest, false
	}
	return manifest, manifest.Prepare.RenditionDir == renditionDir
}
