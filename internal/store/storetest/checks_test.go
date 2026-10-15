// SPDX-License-Identifier: AGPL-3.0-or-later

package storetest_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/waiting"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// recordingTB is a test that records its errors and keeps its cleanups for the test to run.
type recordingTB struct {
	testing.TB
	errors   []string
	cleanups []func()
}

func (recording *recordingTB) Errorf(format string, args ...any) {
	recording.errors = append(recording.errors, fmt.Sprintf(format, args...))
}

func (recording *recordingTB) Cleanup(cleanup func()) {
	recording.cleanups = append(recording.cleanups, cleanup)
}

// A database storetest opens fails the test whose reading request wrote, naming who wrote; a
// person's write that is no reader's does not (D303, DL10).
func TestOpenMigrated_FailsATestWhoseReadingRequestWrote(t *testing.T) {
	t.Parallel()
	recording := &recordingTB{TB: t}
	database := storetest.OpenMigrated(recording)
	if err := database.Write(waiting.ForAPerson(t.Context()), func(*sqlcgen.Queries) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := writeAsAReader(t, database.Write); err != nil {
		t.Fatal(err)
	}
	for _, cleanup := range slices.Backward(recording.cleanups) {
		cleanup()
	}
	if len(recording.errors) != 1 || !strings.Contains(recording.errors[0], "writeAsAReader") {
		t.Errorf("the test was failed with %q; want one error, naming the reading request's write", recording.errors)
	}
}

// writeAsAReader writes under a request that only reads.
func writeAsAReader(t *testing.T, write func(context.Context, func(*sqlcgen.Queries) error) error) error {
	t.Helper()
	if err := write(waiting.ForAReader(t.Context()), func(*sqlcgen.Queries) error { return nil }); err != nil {
		return fmt.Errorf("write as a reader: %w", err)
	}
	return nil
}

// A database storetest opens fails the test that leaves it breaking an invariant (D303, M7).
func TestOpenMigrated_FailsATestThatBreaksAnInvariant(t *testing.T) {
	t.Parallel()
	recording := &recordingTB{TB: t}
	database := storetest.OpenMigrated(recording)
	storetest.ExecRaw(t, database.Path(),
		"INSERT INTO item (id, kind, added_at) VALUES (7, 'title', 1)",
		"INSERT INTO sort_key (item_id, locale, listing, collation_key) VALUES (7, 'en', 'movie', x'01')",
		"INSERT INTO sort_key (item_id, locale, listing, collation_key) VALUES (7, 'en', 'series', x'02')")
	for _, cleanup := range slices.Backward(recording.cleanups) {
		cleanup()
	}
	if len(recording.errors) != 1 || !strings.Contains(recording.errors[0], "no item has two sort keys in one locale") {
		t.Errorf("the test was failed with %q; want one error, naming the broken invariant", recording.errors)
	}
}
