// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ReadAgain is what a scan reads again, changed on disk or not, so its probe is compared with the one
// kept (D392 §2): files and folders relative to the scan's root, wholeRoot for every file. Empty, a
// scan reads only what is new or changed on disk (D240 §3). A scan that reads any again reads every
// NFO and subtitle file it lists again too: they are cheap to read, and an item's NFO is read again
// with its videos (D392 §4).
type ReadAgain []string

// readEverythingAgain is what a full rescan of a whole root reads again: every file it lists.
func readEverythingAgain() ReadAgain { return ReadAgain{wholeRoot} }

// cleaned is the paths as a scan keeps them: each a clean path relative to the root, sorted, once,
// every one dropped that another holds, and only wholeRoot when it is among them.
func (paths ReadAgain) cleaned() ReadAgain {
	var kept ReadAgain
	for _, each := range paths {
		kept = append(kept, cleanScope(each))
	}
	slices.Sort(kept)
	kept = slices.Compact(kept)
	if slices.Contains(kept, wholeRoot) {
		return ReadAgain{wholeRoot}
	}
	return slices.DeleteFunc(kept, func(each string) bool {
		return slices.ContainsFunc(kept, func(other string) bool {
			return other != each && strings.HasPrefix(each, other+"/")
		})
	})
}

// with is the paths of both, as a scan that two asks joined reads again.
func (paths ReadAgain) with(other ReadAgain) ReadAgain {
	return append(slices.Clone(paths), other...).cleaned()
}

// covers is whether a file, by its path relative to the root, is read again: at or under one of the
// paths.
func (paths ReadAgain) covers(relative string) bool {
	return slices.ContainsFunc(paths, func(each string) bool {
		return each == wholeRoot || relative == each || strings.HasPrefix(relative, each+"/")
	})
}

// encoded is the paths as the scan's row keeps them, a JSON list, empty for none.
func (paths ReadAgain) encoded() (*string, error) {
	cleaned := paths.cleaned()
	if cleaned == nil {
		cleaned = ReadAgain{}
	}
	encoded, err := json.Marshal(cleaned)
	if err != nil {
		return nil, fmt.Errorf("encode what a scan reads again: %w", err)
	}
	return new(string(encoded)), nil
}

// decodeReadAgain is what a scan's row says it reads again.
func decodeReadAgain(stored *string) (ReadAgain, error) {
	if stored == nil {
		return nil, nil
	}
	var paths ReadAgain
	if err := json.Unmarshal([]byte(*stored), &paths); err != nil {
		return nil, fmt.Errorf("decode what a scan reads again: %w", err)
	}
	return paths.cleaned(), nil
}

// rebasedOn is the paths, relative to folder, made relative to root, the library folder above it that
// its scan is made within (scanRootOf).
func (paths ReadAgain) rebasedOn(folder string, folders []string) ReadAgain {
	rebased := make(ReadAgain, 0, len(paths))
	for _, each := range paths {
		_, scope := scanRootOf(folder, each, folders)
		rebased = append(rebased, scope)
	}
	return rebased.cleaned()
}
