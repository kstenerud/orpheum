// SPDX-License-Identifier: AGPL-3.0-or-later

package webclient

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Manifest is the client's manifest, which the core signs and a discovery's loader checks before it
// runs any of the client (D308 §6): the build it is, its own script, and every file's path below the
// client's root with its size and SHA-256.
type Manifest struct {
	// Version is the core's build, which a discovery's floor of refused versions names.
	Version string `json:"version"`
	// Script is the client's own script, as ScriptName names it.
	Script string         `json:"script"`
	Files  []ManifestFile `json:"files"`
}

// ManifestFile is one file of the client: its path, "assets/index-DCLi77_b.js", its size, and its
// SHA-256 in unpadded base64url.
type ManifestFile struct {
	Path      string `json:"path"`
	SizeBytes int    `json:"size"`
	SHA256    string `json:"sha256"`
}

// ManifestOf is the manifest of files, the client of the build version, as the bytes the core
// signs: its files in path order, so the same client is always the same bytes.
func ManifestOf(files fs.FS, version string) ([]byte, error) {
	manifest := Manifest{Version: version, Script: ScriptName(files), Files: []ManifestFile{}}
	err := fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || !entry.Type().IsRegular() {
			return walkErr
		}
		contents, err := fs.ReadFile(files, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		digest := sha256.Sum256(contents)
		manifest.Files = append(manifest.Files, ManifestFile{
			Path: path, SizeBytes: len(contents), SHA256: base64.RawURLEncoding.EncodeToString(digest[:]),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list the client's files: %w", err)
	}
	slices.SortFunc(manifest.Files, func(first, second ManifestFile) int {
		return strings.Compare(first.Path, second.Path)
	})
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode the client's manifest: %w", err)
	}
	return encoded, nil
}
