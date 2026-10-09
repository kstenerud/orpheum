// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"errors"
	"path"
	"strings"
)

// wholeRoot is the scope of a scan of a whole root: what it lists, relative to the root.
const wholeRoot = "."

// errScopeNeedsTheRoot is a scan of a folder that found what only a scan of the whole root may
// judge: a folder it could not read, removals that would wait for the admin, or a root not
// complete at its last scan (D272 §9). Nothing it would have removed is removed; the root is
// scanned instead.
var errScopeNeedsTheRoot = errors.New("a scan of a folder found what only a scan of its whole root may judge")

// cleanScope is a scope as a scan keeps it: a clean path relative to the root, or the whole root
// for one that is absolute, empty or leads out of the root.
func cleanScope(scope string) string {
	cleaned := path.Clean(scope)
	if cleaned == "" || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return wholeRoot
	}
	return cleaned
}

// widerScope is the narrowest scope covering both: the folder both are in.
func widerScope(first, second string) string {
	first, second = cleanScope(first), cleanScope(second)
	if first == wholeRoot || second == wholeRoot {
		return wholeRoot
	}
	firstParts, secondParts := strings.Split(first, "/"), strings.Split(second, "/")
	shared := 0
	for shared < len(firstParts) && shared < len(secondParts) && firstParts[shared] == secondParts[shared] {
		shared++
	}
	if shared == 0 {
		return wholeRoot
	}
	return strings.Join(firstParts[:shared], "/")
}

// isScoped reports whether a scan lists a folder under its root rather than the whole root.
func (scan *scanState) isScoped() bool { return scan.scope != wholeRoot && scan.scope != "" }

// covers reports whether a file, by the bytes its node reported, is one this scan can judge absent:
// under its root and, for a scan of a folder, under that folder.
func (scan *scanState) covers(reportedPath []byte) bool {
	if _, under := RelativeToRoot(reportedPath, scan.root); !under {
		return false
	}
	return !scan.isScoped() || isAtOrUnder(reportedPath, path.Join(scan.root, scan.scope))
}
