// SPDX-License-Identifier: AGPL-3.0-or-later

package instancedir

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"

	"github.com/kstenerud/orpheum/internal/platform/errs"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
	"github.com/kstenerud/orpheum/pkg/atomics"
)

// trickplayDirectoryName holds the scrub bar's pictures under the cache directory (trickplay.md
// section 4): one WebP per sheet, named by its SHA-256, so a sheet is served content-addressed as
// artwork is, and two videos' sheets of the same bytes are one file.
const trickplayDirectoryName = "trickplay"

// trickplaySheetSuffix ends each sheet's file name.
const trickplaySheetSuffix = ".webp"

// ErrBadTrickplayName is a sheet's name that is not a SHA-256 in lower-case hex.
var ErrBadTrickplayName = errors.New("not a trickplay sheet's name")

var trickplaySheetPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// WriteTrickplaySheet stores one sheet, atomically, mode 0600, by its SHA-256 in hex.
func (directories *Directories) WriteTrickplaySheet(sha256Hex string, contents []byte) (err error) {
	sheets, err := directories.openTrickplayDirectory(sha256Hex, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the trickplay directory", sheets.Close())) }()
	if err := atomics.WriteFile(sheets, sha256Hex+trickplaySheetSuffix, contents, fileMode); err != nil {
		return fmt.Errorf("write trickplay sheet %s: %w", sha256Hex, err)
	}
	return nil
}

// ReadTrickplaySheet reads one sheet; ErrNotFound when it is not there.
func (directories *Directories) ReadTrickplaySheet(sha256Hex string) (contents []byte, err error) {
	sheets, err := directories.openTrickplayDirectory(sha256Hex, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("trickplay sheet %s: %w", sha256Hex, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the trickplay directory", sheets.Close())) }()
	contents, err = sheets.ReadFile(sha256Hex + trickplaySheetSuffix)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("trickplay sheet %s: %w", sha256Hex, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("read trickplay sheet %s: %w", sha256Hex, err)
	}
	return contents, nil
}

// RemoveTrickplaySheet removes one sheet; one already gone is not an error.
func (directories *Directories) RemoveTrickplaySheet(sha256Hex string) (err error) {
	sheets, err := directories.openTrickplayDirectory(sha256Hex, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, errs.WrapIf("close the trickplay directory", sheets.Close())) }()
	return ignoreMissing(errs.WrapIf("remove trickplay sheet "+sha256Hex,
		sheets.Remove(sha256Hex+trickplaySheetSuffix)))
}

// ListTrickplaySheets lists the SHA-256 of every sheet stored; a name the store does not make is
// skipped.
func (directories *Directories) ListTrickplaySheets() (sha256Hexes []string, err error) {
	cache, err := os.OpenRoot(directories.cacheDirectory)
	if err != nil {
		return nil, fmt.Errorf("open the cache directory: %w", err)
	}
	defer readonly.Close(cache)
	entries, err := fs.ReadDir(cache.FS(), trickplayDirectoryName)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list the trickplay sheets: %w", err)
	}
	for _, entry := range entries {
		name, isSheet := strings.CutSuffix(entry.Name(), trickplaySheetSuffix)
		if isSheet && entry.Type().IsRegular() && trickplaySheetPattern.MatchString(name) {
			sha256Hexes = append(sha256Hexes, name)
		}
	}
	return sha256Hexes, nil
}

// openTrickplayDirectory checks a sheet's name and opens the cache's trickplay directory, creating it
// when asked, through os.Root, so no name can reach outside it.
func (directories *Directories) openTrickplayDirectory(sha256Hex string, shouldCreate bool) (*os.Root, error) {
	if !trickplaySheetPattern.MatchString(sha256Hex) {
		return nil, fmt.Errorf("%q: %w", sha256Hex, ErrBadTrickplayName)
	}
	cache, err := os.OpenRoot(directories.cacheDirectory)
	if err != nil {
		return nil, fmt.Errorf("open the cache directory: %w", err)
	}
	defer readonly.Close(cache)
	if shouldCreate {
		if err = cache.MkdirAll(trickplayDirectoryName, directoryMode); err != nil {
			return nil, fmt.Errorf("create %s: %w", trickplayDirectoryName, err)
		}
	}
	sheets, err := cache.OpenRoot(trickplayDirectoryName)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", trickplayDirectoryName, err)
	}
	return sheets, nil
}
