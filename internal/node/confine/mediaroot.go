// SPDX-License-Identifier: AGPL-3.0-or-later

package confine

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/kstenerud/orpheum/internal/platform/bounded"
	"github.com/kstenerud/orpheum/internal/platform/pathtree"
)

// MediaRoot is a media root opened read-only. It lists, stats, reads links and opens files for
// reading, all through os.Root, so nothing it returns can reach outside the root; it has no
// method that writes, renames or removes (principles §2).
type MediaRoot struct {
	path string
	root *os.Root
	// configured is the path of the root the node was configured with that this one is, or is a
	// library folder inside (Folder).
	configured string
}

// ErrNotARealFolder is a library folder whose path passes through a link, or names what is not a
// directory: a library folder is named by its real path (D359 §1).
var ErrNotARealFolder = errors.New("not a folder reached without a link")

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
	return &MediaRoot{path: resolved, root: root, configured: resolved}, nil
}

// Path is the root's absolute path, symlinks resolved.
func (media *MediaRoot) Path() string { return media.path }

// ConfiguredPath is the path of the root the node was configured with that holds this one: its own
// path, or for a library folder inside a configured root (Folder), that root's. The node's disk gate
// keys its readings by it, so a stuck share holds one reading however many library folders it holds.
func (media *MediaRoot) ConfiguredPath() string { return media.configured }

// Folder opens a library folder inside the root, relativePath below it, as a media root of its own,
// read through os.Root as the root is, so nothing it opens is outside the folder: what a library
// folder narrower than a configured root is listed and examined through (D359 §1). Every element of
// relativePath must be a directory and none a link, so the folder's path is its real one, as a
// configured root's is (ErrNotARealFolder). The caller closes it.
func (media *MediaRoot) Folder(relativePath string) (*MediaRoot, error) {
	if err := media.checkRealFolder(relativePath); err != nil {
		return nil, err
	}
	folder, err := media.root.OpenRoot(relativePath)
	if err != nil {
		return nil, fmt.Errorf("open folder %s: %w", relativePath, err)
	}
	return &MediaRoot{path: filepath.Join(media.path, relativePath), root: folder, configured: media.configured}, nil
}

// checkRealFolder is nil when every element of relativePath, a local path below the root, is a
// directory and none a link; "." is the root itself.
func (media *MediaRoot) checkRealFolder(relativePath string) error {
	if relativePath == "." {
		return nil
	}
	if !filepath.IsLocal(relativePath) || filepath.Clean(relativePath) != relativePath {
		return fmt.Errorf("%s: %w", relativePath, ErrNotARealFolder)
	}
	reached := ""
	for element := range strings.SplitSeq(relativePath, string(filepath.Separator)) {
		reached = filepath.Join(reached, element)
		info, err := media.root.Lstat(reached)
		if err != nil {
			return fmt.Errorf("stat %s: %w", reached, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s: %w", reached, ErrNotARealFolder)
		}
	}
	return nil
}

// Subfolders are the names of the directories in the folder at relativePath ("." for the root), in
// byte order, for an admin choosing a library folder (D359 §2): a link is not one, even to a
// directory, since a library folder is named by its real path, and a hidden folder, its name starting
// with ".", is left out. The folder itself must be reached without a link (ErrNotARealFolder).
func (media *MediaRoot) Subfolders(relativePath string) ([]string, error) {
	if err := media.checkRealFolder(relativePath); err != nil {
		return nil, err
	}
	entries, err := media.ReadDir(relativePath)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// Holding is the root of roots, the node's configured roots by path, that holds path, and path
// relative to it, "." for the root itself: a library folder's root, the folder at or under it by a
// clean path (D359 §1). False when no root holds it, a path with ".." among them.
func Holding(roots map[string]*MediaRoot, path string) (*MediaRoot, string, bool) {
	if root, isRoot := roots[path]; isRoot {
		return root, ".", true
	}
	holding, isHeld := pathtree.DeepestHolding(path, slices.Collect(maps.Keys(roots)))
	if !isHeld {
		return nil, "", false
	}
	root := roots[holding]
	relativePath, isInside := root.RelativeIfInside(path)
	if !isInside {
		return nil, "", false
	}
	return root, relativePath, true
}

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

// Open opens the file at rel for reading. Symlinks that stay inside the root are followed; one
// that leaves it is refused by os.Root. It never waits: a pipe opens without waiting for a writer,
// so a caller that checks for a regular file can refuse one.
func (media *MediaRoot) Open(relativePath string) (*os.File, error) {
	file, err := media.root.OpenFile(relativePath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", relativePath, err)
	}
	return file, nil
}

// ReadFile reads the file at rel whole, as Open opens it, refusing one past maxBytes
// (bounded.ErrTooLarge): the one way the node takes in a small file beside the media, an NFO, an
// image or a subtitle file.
func (media *MediaRoot) ReadFile(relativePath string, maxBytes int64) ([]byte, error) {
	file, err := media.Open(relativePath)
	if err != nil {
		return nil, err
	}
	contents, readErr := bounded.ReadAll(file, maxBytes)
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read %s: %w", relativePath, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", relativePath, closeErr)
	}
	return contents, nil
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
