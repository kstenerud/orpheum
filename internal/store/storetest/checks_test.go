// SPDX-License-Identifier: AGPL-3.0-or-later

package storetest_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/waiting"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/rowstest"
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
		rowstest.Node(rowstest.NodeSpec{ID: 1}),
		rowstest.Library(rowstest.LibrarySpec{ID: 1}),
		rowstest.Title(rowstest.ItemSpec{ID: 7, RemovedAt: 2, IdentificationState: "unidentified"}),
		rowstest.Version(rowstest.VersionSpec{ID: 5, TitleID: 7}),
		rowstest.File(rowstest.FileSpec{ID: 1, NodeID: 1, Path: "/m/f.mkv", LibraryKind: "movie", VersionID: 5}))
	for _, cleanup := range slices.Backward(recording.cleanups) {
		cleanup()
	}
	broken := "no live version belongs to a removed title"
	if len(recording.errors) != 1 || !strings.Contains(recording.errors[0], broken) {
		t.Errorf("the test was failed with %q; want one error, naming the broken invariant", recording.errors)
	}
}

// A cache.db storetest opens fails the test whose reading request wrote to it too (D303, DL10).
func TestOpenCache_FailsATestWhoseReadingRequestWrote(t *testing.T) {
	t.Parallel()
	recording := &recordingTB{TB: t}
	cache := storetest.OpenCache(recording)
	err := cache.Write(waiting.ForAReader(t.Context()), func(queries *cachegen.Queries) error {
		return queries.MarkArtServed(t.Context(), cachegen.MarkArtServedParams{
			SourceSha256Hex: "ab", LastServedEpochDay: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, cleanup := range slices.Backward(recording.cleanups) {
		cleanup()
	}
	if len(recording.errors) != 1 || !strings.Contains(recording.errors[0], "only reads wrote") {
		t.Errorf("the test was failed with %q; want one error, naming the reading request's write", recording.errors)
	}
}
