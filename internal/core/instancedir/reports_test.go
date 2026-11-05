// SPDX-License-Identifier: AGPL-3.0-or-later

package instancedir

import (
	"os"
	"slices"
	"testing"
	"time"
)

// Reports written in the same instant list by name backwards, so a page's cursor resumes in one order.
func TestListReports_ReportsWrittenTogetherListByNameBackwards(t *testing.T) {
	t.Parallel()
	directories := prepareTemporaryDirectories(t)
	names := []string{
		"filing-1-20261105T120000Z.md",
		"filing-2-20261105T120000Z.md",
		"filing-3-20261105T120000Z.md",
	}
	together := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	for _, name := range names {
		path, err := directories.WriteReport(name, []byte("report"))
		if err != nil {
			t.Fatalf("WriteReport %s: %v", name, err)
		}
		if err := os.Chtimes(path, together, together); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := directories.ListReports()
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	got := []string{}
	for _, report := range listed {
		got = append(got, report.Name)
	}
	want := slices.Clone(names)
	slices.Reverse(want)
	if !slices.Equal(got, want) {
		t.Errorf("ListReports = %v, want %v", got, want)
	}
}
