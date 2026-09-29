// SPDX-License-Identifier: GPL-3.0-or-later

package confine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// MediaRoot is a media root opened read-only. It lists, stats, reads links and opens files for
// reading, all through os.Root, so nothing it returns can reach outside the root; it has no
// method that writes, renames or removes (principles §2).
type MediaRoot struct {
	path string
	root *os.Root
}

// OpenMediaRoot opens the directory at path, which must be absolute, as a media root. Its
// path is recorded with symlinks resolved, so a link's target can be matched against it.
func OpenMediaRoot(path string) (*MediaRoot, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("a media root must be an absolute path, not %q", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("media root %s: %w", path, err)
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, fmt.Errorf("media root %s: %w", path, err)
	}
	return &MediaRoot{path: resolved, root: root}, nil
}

// Path is the root's absolute path, symlinks resolved.
func (media *MediaRoot) Path() string { return media.path }

// Close releases the root.
func (media *MediaRoot) Close() error {
	if err := media.root.Close(); err != nil {
		return fmt.Errorf("close media root %s: %w", media.path, err)
	}
	return nil
}

// ReadDir lists the directory at rel ("." for the root) in byte order of name, so a scan
// reports in the same order every time.
func (media *MediaRoot) ReadDir(relativePath string) ([]fs.DirEntry, error) {
	directory, err := media.root.Open(relativePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", relativePath, err)
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return nil, fmt.Errorf("list %s: %w", relativePath, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", relativePath, closeErr)
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

// Lstat describes the entry at rel without following a final symlink.
func (media *MediaRoot) Lstat(relativePath string) (fs.FileInfo, error) {
	info, err := media.root.Lstat(relativePath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", relativePath, err)
	}
	return info, nil
}

// Open opens the regular file at rel for reading. Symlinks that stay inside the root are
// followed; one that leaves it is refused by os.Root.
func (media *MediaRoot) Open(relativePath string) (*os.File, error) {
	file, err := media.root.Open(relativePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", relativePath, err)
	}
	return file, nil
}

// Resolve returns the absolute path the entry at rel names once every symlink is followed,
// wherever that is; the caller decides whether it may go there (D206).
func (media *MediaRoot) Resolve(relativePath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(filepath.Join(media.path, relativePath))
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", relativePath, err)
	}
	return resolved, nil
}

// RelativeIfInside reports whether absolute, a resolved path, is the root or inside it, and the path
// relative to the root if so.
func (media *MediaRoot) RelativeIfInside(absolute string) (string, bool) {
	relativePath, err := filepath.Rel(media.path, absolute)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relativePath, true
}
