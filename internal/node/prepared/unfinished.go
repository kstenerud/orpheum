// SPDX-License-Identifier: AGPL-3.0-or-later

package prepared

// EvictUnfinished evicts every run directory that is not a store of chunks with its manifest: a
// store whose making never finished, and anything a whole-film prepare left before it was retired
// (D238 §2, D344 §1). A run being written is left alone. It returns the directories it evicted.
func (shelf *Shelf) EvictUnfinished() ([]string, error) {
	runs, err := shelf.listRuns()
	if err != nil {
		return nil, err
	}
	var evicted []string
	for _, each := range runs {
		if shelf.isWriting(each.runDir) {
			continue
		}
		if _, isStore := shelf.readStoreManifest(each.runDir); isStore {
			continue
		}
		if err := shelf.removeRun(each.runDir); err != nil {
			return evicted, err
		}
		evicted = append(evicted, each.runDir)
	}
	return evicted, nil
}
