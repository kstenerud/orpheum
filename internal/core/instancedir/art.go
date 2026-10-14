// SPDX-License-Identifier: AGPL-3.0-or-later

package instancedir

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/kstenerud/orpheum/internal/platform/atomics"
	"github.com/kstenerud/orpheum/internal/platform/errs"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// artDirectoryName holds the artwork under the cache directory (D212): one directory per image,
// named by the SHA-256 of its source, holding one file per size and format, and an empty .none file
// for a size its source is too small to make.
const artDirectoryName = "art"

// ErrBadArtName is an artwork source SHA-256 or file name that is not one the artwork store makes.
var ErrBadArtName = errors.New("not an artwork name")

var (
	artSourceSha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	artFileNamePattern        = regexp.MustCompile(`^[a-z]+-[a-z]+\.(webp|jpeg|none)$`)
)

// WriteArt stores one size of one image, atomically, mode 0600. Its names are ours, not
// anyone's text: a SHA-256 in hex, and a size name with its format.
func (directories *Directories) WriteArt(sourceSha256Hex, name string, contents []byte) (err error) {
	imageDirectory, err := directories.openImageDirectory(sourceSha256Hex, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the artwork directory", imageDirectory.Close())) }()
	if !artFileNamePattern.MatchString(name) {
		return fmt.Errorf("%q: %w", name, ErrBadArtName)
	}
	if err := atomics.WriteFile(imageDirectory, name, contents, fileMode); err != nil {
		return fmt.Errorf("write art %s: %w", name, err)
	}
	return nil
}

// ReadArt reads one size of one image; ErrNotFound when it is not there.
func (directories *Directories) ReadArt(sourceSha256Hex, name string) (contents []byte, err error) {
	if !artFileNamePattern.MatchString(name) {
		return nil, fmt.Errorf("%q: %w", name, ErrBadArtName)
	}
	imageDirectory, err := directories.openImageDirectory(sourceSha256Hex, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("art %s: %w", sourceSha256Hex, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the artwork directory", imageDirectory.Close())) }()
	contents, err = imageDirectory.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("art %s/%s: %w", sourceSha256Hex, name, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("read art %s/%s: %w", sourceSha256Hex, name, err)
	}
	return contents, nil
}

// HasArt reports whether one size of one image is stored, without reading it.
func (directories *Directories) HasArt(sourceSha256Hex, name string) (found bool, err error) {
	if !artFileNamePattern.MatchString(name) {
		return false, fmt.Errorf("%q: %w", name, ErrBadArtName)
	}
	imageDirectory, err := directories.openImageDirectory(sourceSha256Hex, false)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the artwork directory", imageDirectory.Close())) }()
	_, err = imageDirectory.Stat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look for art %s/%s: %w", sourceSha256Hex, name, err)
	}
	return true, nil
}

// RemoveArt removes one size of one image; one already gone is not an error.
func (directories *Directories) RemoveArt(sourceSha256Hex, name string) (err error) {
	if !artFileNamePattern.MatchString(name) {
		return fmt.Errorf("%q: %w", name, ErrBadArtName)
	}
	imageDirectory, err := directories.openImageDirectory(sourceSha256Hex, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the artwork directory", imageDirectory.Close())) }()
	return ignoreMissing(errs.WrapIf("remove art "+sourceSha256Hex+"/"+name, imageDirectory.Remove(name)))
}

// ArtFile is one stored file of one image, and its size.
type ArtFile struct {
	SourceSha256Hex string
	Name            string
	SizeBytes       int64
}

// ListArt lists every stored image file with its size: read once when the server starts, so
// the artwork ceiling keeps a running total rather than walking the tree again (D212 §6). A name
// the store does not make is skipped.
func (directories *Directories) ListArt() (files []ArtFile, err error) {
	cache, err := os.OpenRoot(directories.cacheDirectory)
	if err != nil {
		return nil, fmt.Errorf("open the cache directory: %w", err)
	}
	defer readonly.Close(cache)
	images, err := fs.ReadDir(cache.FS(), artDirectoryName)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list the artwork: %w", err)
	}
	for _, image := range images {
		if !image.IsDir() || !artSourceSha256HexPattern.MatchString(image.Name()) {
			continue
		}
		sizes, err := fs.ReadDir(cache.FS(), artDirectoryName+"/"+image.Name())
		if err != nil {
			return nil, fmt.Errorf("list art %s: %w", image.Name(), err)
		}
		for _, sizeFile := range sizes {
			info, infoErr := sizeFile.Info()
			if infoErr != nil || !info.Mode().IsRegular() || !artFileNamePattern.MatchString(sizeFile.Name()) {
				continue
			}
			files = append(files, ArtFile{SourceSha256Hex: image.Name(), Name: sizeFile.Name(), SizeBytes: info.Size()})
		}
	}
	return files, nil
}

// openImageDirectory opens one image's directory under the cache's art directory, creating both when
// asked, through os.Root, so no name can reach outside it.
func (directories *Directories) openImageDirectory(sourceSha256Hex string, shouldCreate bool) (*os.Root, error) {
	if !artSourceSha256HexPattern.MatchString(sourceSha256Hex) {
		return nil, fmt.Errorf("%q: %w", sourceSha256Hex, ErrBadArtName)
	}
	cache, err := os.OpenRoot(directories.cacheDirectory)
	if err != nil {
		return nil, fmt.Errorf("open the cache directory: %w", err)
	}
	defer readonly.Close(cache)
	relative := filepath.Join(artDirectoryName, sourceSha256Hex)
	if shouldCreate {
		if err = cache.MkdirAll(relative, directoryMode); err != nil {
			return nil, fmt.Errorf("create %s: %w", relative, err)
		}
	}
	imageDirectory, err := cache.OpenRoot(relative)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", relative, err)
	}
	return imageDirectory, nil
}
