// SPDX-License-Identifier: AGPL-3.0-or-later

package storesources

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Nothing in the program runs ANALYZE or PRAGMA optimize, which would replace the committed
// statistics with what one household's library holds, and hold the writer while it measured; the
// loader's ANALYZE sqlite_schema only reloads them (D303 §3, DL15). Until 2026-10-25 the walk
// skipped its own root, named "..", as a hidden directory, and read nothing: it fails a walk that
// reads no source now.
func TestStatistics_NothingElseAnalyzes(t *testing.T) {
	t.Parallel()
	// Statements are strings, quoted or raw; a comment may name either, as this package's doc does.
	analyzes := regexp.MustCompile("(?i)[\"`][^\"`\n]*\\b(ANALYZE|PRAGMA\\s+optimize)\\b[^\"`\n]*[\"`]")
	read := 0
	err := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if isSkippedDirectory(path, entry) {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, filepath.Join("store", "statistics.go")) {
			return nil
		}
		text, readErr := os.ReadFile(path) //nolint:gosec // the module's own files
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		read++
		if analyzes.Match(text) {
			t.Errorf("%s runs ANALYZE or PRAGMA optimize; the statistics are committed (D303 §3)", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read < 100 {
		t.Fatalf("read %d of the module's Go sources; the check would pass vacuously", read)
	}
}
