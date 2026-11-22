// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
)

// D535 §10: every table of persistent.db and of cache.db says what a discarding rescan does with it,
// and no table that is gone says anything, so a table added without saying fails here. SQLite's own
// tables and an FTS index's own tables are counted with what they serve. A table kept or discarded
// names the test that shows it, and that test is there.
func TestDiscard_EveryTableSaysWhatADiscardDoesWithIt(t *testing.T) {
	t.Parallel()
	tests := testFunctionsUnderInternal(t)
	for _, database := range []struct {
		name   string
		db     *Database
		tables map[string]TableInADiscard
	}{
		{"persistent.db", openMigrated(t), PersistentTablesInADiscard()},
		{"cache.db", openCacheAt(t, t.TempDir()+"/cache.db").database, CacheTablesInADiscard()},
	} {
		var listed []string
		for _, row := range queryRows(t, database.db, "SELECT name FROM pragma_table_list "+
			"WHERE schema = 'main' AND type IN ('table', 'virtual') AND name NOT LIKE 'sqlite_%'", 1) {
			listed = append(listed, row[0])
		}
		if len(listed) == 0 {
			t.Fatalf("%s lists no table; the test reads nothing", database.name)
		}
		for _, table := range listed {
			if _, isSaid := database.tables[table]; !isSaid {
				t.Errorf("%s's table %s does not say what a discarding rescan does with it", database.name, table)
			}
		}
		for _, table := range slices.Sorted(maps.Keys(database.tables)) {
			problem := discardSaysWrongly(database.tables[table], slices.Contains(listed, table), tests)
			if problem != "" {
				t.Errorf("%s's table %s %s", database.name, table, problem)
			}
		}
	}
}

// discardSaysWrongly is what is wrong with what a table says of a discard, isListed whether its
// database has it; empty for nothing.
func discardSaysWrongly(said TableInADiscard, isListed bool, tests map[string]bool) string {
	isProven := said.Fate == KeptAsAPersonsDoing || said.Fate == DiscardedAsItStarts ||
		said.Fate == DiscardedByTheRead || said.Fate == DiscardedByTheMatchAgain
	switch {
	case !isListed:
		return "is no table, yet says what a discard does with it"
	case said.Why == "":
		return "says no reason"
	case isProven && !tests[said.ProvenBy]:
		return fmt.Sprintf("is %s, proven by %q, which is no test under internal/", said.Fate, said.ProvenBy)
	}
	return ""
}

// testFunctionsUnderInternal are the names of the tests in every package under internal/.
func testFunctionsUnderInternal(t *testing.T) map[string]bool {
	t.Helper()
	internal := os.DirFS("..")
	names := map[string]bool{}
	err := fs.WalkDir(internal, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		text, err := fs.ReadFile(internal, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		for line := range strings.Lines(string(text)) {
			if name, isTest := strings.CutPrefix(line, "func Test"); isTest {
				if end := strings.IndexByte(name, '('); end > 0 {
					names["Test"+name[:end]] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}
