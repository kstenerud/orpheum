// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"path"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/platform/pathtree"
)

// Libraries may overlap (D363 §1): a library's folder may be at, above or below another library's on
// its node. Each place is scanned and watched once whatever libraries cover it (§2), so a folder is
// scanned within the outermost live library folder on its node at or above it, of whichever library:
// that folder is its scan's root, and a scan's root is a folder no other is above.

// scanRootOf is the root a library folder's scan runs under, of the live library folders on its node,
// and the scope within it the folder's scope becomes: the outermost of folders at or above folder, or
// folder itself when none is.
func scanRootOf(folder, scope string, folders []string) (root, rootScope string) {
	root = folder
	for _, other := range folders {
		if len(other) < len(root) && pathtree.IsAtOrUnder(folder, other) {
			root = other
		}
	}
	if root == folder {
		return folder, scope
	}
	relative := strings.TrimPrefix(strings.TrimPrefix(folder, strings.TrimSuffix(root, "/")), "/")
	if cleanScope(scope) == wholeRoot {
		return root, relative
	}
	return root, path.Join(relative, cleanScope(scope))
}

// outermostFolders are the folders no other of them is above: those a node watches, each place under
// them once (D363 §2). Equal folders are one.
func outermostFolders(folders []string) []string {
	outermost := []string{}
	for _, folder := range folders {
		isHeld := false
		for _, other := range folders {
			if other != folder && pathtree.IsAtOrUnder(folder, other) {
				isHeld = true
				break
			}
		}
		if !isHeld && !slices.Contains(outermost, folder) {
			outermost = append(outermost, folder)
		}
	}
	return outermost
}
