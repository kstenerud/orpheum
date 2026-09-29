// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"bytes"
	"path"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/kstenerud/orpheum/internal/core/textindex"
)

// RelativeToRoot is a file's path below a root: its reported path, the bytes its node reported
// (D211), with the root's path and the separator after it cut off. It reports false when the file
// is not below the root. The bytes are compared, not the text: a file's text path is kept in NFC
// and a root's as its node named it, so as text a root whose name is not NFC holds nothing. A
// root of "/" is its own separator.
func RelativeToRoot(reportedPath []byte, rootPath string) ([]byte, bool) {
	prefix := strings.TrimSuffix(path.Clean(rootPath), "/") + "/"
	relative, under := bytes.CutPrefix(reportedPath, []byte(prefix))
	if !under || len(relative) == 0 {
		return nil, false
	}
	return relative, true
}

// RootHolding is the root among rootPaths that a file is below, the deepest when roots nest, and
// the file's path below it (RelativeToRoot).
func RootHolding(reportedPath []byte, rootPaths []string) (rootPath string, relative []byte, found bool) {
	for _, candidate := range rootPaths {
		below, under := RelativeToRoot(reportedPath, candidate)
		if under && (!found || len(below) < len(relative)) {
			rootPath, relative, found = candidate, below, true
		}
	}
	return rootPath, relative, found
}

// PathText is a path's bytes as the library shows and relates the path: valid UTF-8, in NFC for
// D49's equality, and cleaned (principles §22). Cut at a separator, the text of each part is the
// text of the whole cut there.
func PathText(raw []byte) string {
	return textindex.Path(norm.NFC.String(strings.ToValidUTF8(string(raw), "\uFFFD")))
}

// isAtOrUnder reports whether a file, by the bytes its node reported, is directory or below it.
func isAtOrUnder(reportedPath []byte, directory string) bool {
	_, under := RelativeToRoot(reportedPath, directory)
	return under || string(reportedPath) == path.Clean(directory)
}
