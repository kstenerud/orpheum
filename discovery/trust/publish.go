// SPDX-License-Identifier: AGPL-3.0-or-later

package trust

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WellKnownPrefix is the path every signed file is served under; Publish writes a directory to be
// served as it.
const WellKnownPrefix = "/.well-known/"

// Published is one signed file to publish: its path under WellKnownPrefix, and the signed object.
type Published struct {
	Path   string
	Signed Signed
}

// Publish writes each file into directory, laid out as it is served under WellKnownPrefix, its
// signature beside it at the same path and ".sig": what discovery/single serves, and what any static
// host can (the root key audit's §3.1). Readable by all: nothing signed is secret.
func Publish(directory string, files []Published) error {
	for _, file := range files {
		name, isUnder := strings.CutPrefix(file.Path, WellKnownPrefix)
		if !isUnder || name == "" {
			return fmt.Errorf("publish %s: not under %s", file.Path, WellKnownPrefix)
		}
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // published, read by all
			return fmt.Errorf("publish %s: %w", file.Path, err)
		}
		written := map[string]string{path: file.Signed.Payload, path + ".sig": file.Signed.Signature}
		for name, contents := range written {
			if err := os.WriteFile(name, []byte(contents), 0o644); err != nil { //nolint:gosec // as said
				return fmt.Errorf("publish %s: %w", file.Path, err)
			}
		}
	}
	return nil
}

// ChainFiles are a chain's statements as published under prefix: each at prefix and its version, and
// the last also at prefix and "latest".
func ChainFiles(prefix string, statements []Signed) []Published {
	files := make([]Published, 0, len(statements)+1)
	for _, statement := range statements {
		root, err := ParseRoot([]byte(statement.Payload))
		if err != nil {
			continue
		}
		files = append(files, Published{Path: fmt.Sprintf("%s%d", prefix, root.Version), Signed: statement})
	}
	if len(statements) > 0 {
		files = append(files, Published{Path: prefix + "latest", Signed: statements[len(statements)-1]})
	}
	return files
}
