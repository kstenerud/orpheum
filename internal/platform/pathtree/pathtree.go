// SPDX-License-Identifier: AGPL-3.0-or-later

package pathtree

import (
	"path"
	"strings"
)

// IsAtOrUnder is whether the absolute path p is directory, or under it at a separator.
func IsAtOrUnder(p, directory string) bool {
	directory = strings.TrimSuffix(directory, "/")
	return p == directory || directory == "" || strings.HasPrefix(p, directory+"/")
}

// IsClean is whether p is an absolute path as path.Clean leaves it: no "." or ".." element, and no
// doubled or trailing separator, so it names one place however it is read (D359 §1).
func IsClean(p string) bool {
	return strings.HasPrefix(p, "/") && path.Clean(p) == p
}

// DeepestHolding is the deepest of directories that p is at or under, and whether one is: the
// configured root that holds a library folder (D359 §1). A p that is not clean (IsClean) is held by
// none, since a ".." in it could climb out of the directory its text starts with.
func DeepestHolding(p string, directories []string) (string, bool) {
	if !IsClean(p) {
		return "", false
	}
	holding, isHeld := "", false
	for _, directory := range directories {
		if IsAtOrUnder(p, directory) && (!isHeld || len(directory) > len(holding)) {
			holding, isHeld = directory, true
		}
	}
	return holding, isHeld
}
