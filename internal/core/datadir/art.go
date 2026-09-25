// SPDX-License-Identifier: GPL-3.0-or-later

package datadir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// artDirectory holds the artwork under the cache directory (D212): one directory per image,
// named by the hash of its source, holding one file per size and format, and an empty .none file
// for a size its source is too small to make.
const artDirectory = "art"

// ErrBadArtName is an artwork hash or file name that is not one the artwork store makes.
var ErrBadArtName = errors.New("not an artwork name")

var (
	artHash = regexp.MustCompile(`^[0-9a-f]{64}$`)
	artFile = regexp.MustCompile(`^[a-z]+-[a-z]+\.(webp|jpeg|none)$`)
)

// WriteArt stores one size of one image, atomically, mode 0600. Its names are ours, not
// anyone's text: a SHA-256 in hex, and a size name with its format.
func (directories *Directories) WriteArt(hash, name string, contents []byte) (err error) {
	image, err := directories.openArt(hash, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, wrapIf("close the artwork directory", image.Close())) }()
	if !artFile.MatchString(name) {
		return fmt.Errorf("%q: %w", name, ErrBadArtName)
	}
	return writeAtomically(image, name, contents)
}

// ReadArt reads one size of one image; ErrNotFound when it is not there.
func (directories *Directories) ReadArt(hash, name string) (contents []byte, err error) {
	if !artFile.MatchString(name) {
		return nil, fmt.Errorf("%q: %w", name, ErrBadArtName)
	}
	image, err := directories.openArt(hash, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("art %s: %w", hash, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, wrapIf("close the artwork directory", image.Close())) }()
	contents, err = image.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("art %s/%s: %w", hash, name, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("read art %s/%s: %w", hash, name, err)
	}
	return contents, nil
}

// HasArt reports whether one size of one image is stored, without reading it.
func (directories *Directories) HasArt(hash, name string) (found bool, err error) {
	if !artFile.MatchString(name) {
		return false, fmt.Errorf("%q: %w", name, ErrBadArtName)
	}
	image, err := directories.openArt(hash, false)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, wrapIf("close the artwork directory", image.Close())) }()
	_, err = image.Stat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look for art %s/%s: %w", hash, name, err)
	}
	return true, nil
}

// RemoveArt removes one size of one image; one already gone is not an error.
func (directories *Directories) RemoveArt(hash, name string) (err error) {
	if !artFile.MatchString(name) {
		return fmt.Errorf("%q: %w", name, ErrBadArtName)
	}
	image, err := directories.openArt(hash, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, wrapIf("close the artwork directory", image.Close())) }()
	return ignoreMissing(wrapIf("remove art "+hash+"/"+name, image.Remove(name)))
}

// openArt opens one image's directory under the cache's art directory, creating both when
// asked, through os.Root, so no name can reach outside it.
func (directories *Directories) openArt(hash string, create bool) (*os.Root, error) {
	if !artHash.MatchString(hash) {
		return nil, fmt.Errorf("%q: %w", hash, ErrBadArtName)
	}
	cache, err := os.OpenRoot(directories.cache)
	if err != nil {
		return nil, fmt.Errorf("open the cache directory: %w", err)
	}
	defer func() { _ = cache.Close() }() //nolint:errcheck // the image's own root is what is used
	relative := filepath.Join(artDirectory, hash)
	if create {
		if err = cache.MkdirAll(relative, directoryMode); err != nil {
			return nil, fmt.Errorf("create %s: %w", relative, err)
		}
	}
	image, err := cache.OpenRoot(relative)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", relative, err)
	}
	return image, nil
}
