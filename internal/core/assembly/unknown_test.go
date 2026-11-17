// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// TestBuild_TriesOnceMoreTheFetchesAnEarlierStartGaveUpForAnUnknownCause is the start's retry wired
// (D511 §4): Build registers fetch_metadata's, which clears the item's own failure so its sweep fetches
// it again, and runs it; the failure stays kept, of an earlier start, until the fetch succeeds.
func TestBuild_TriesOnceMoreTheFetchesAnEarlierStartGaveUpForAnUnknownCause(t *testing.T) {
	t.Parallel()
	parts := testParts(t)
	storetest.ExecRaw(t, parts.Database.Path(),
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movie', 'nfo_then_tmdb', 'manual', 0, 1790000000000)`,
		`INSERT INTO item (id, kind, filing_library_id, added_at, followed_source, identification_state)
		 VALUES (5, 'movie', 1, 1790000000000, 'none', 'unidentified')`,
		`INSERT INTO fetch_failure (item_id, failure, failed_at) VALUES (5, 'not JSON', 1790000000000)`,
		`INSERT INTO unknown_failure (kind, subject, error_text, output, version, first_failed_at, last_failed_at,
		 failure_count, start_id) VALUES ('fetch_metadata', 'item 5', 'not JSON', '', 'v0.8.2', 1790000000000,
		 1790000000000, 1, 'an-earlier-start')`)
	services, err := Build(t.Context(), parts)
	if err != nil {
		t.Fatal(err)
	}
	failures := storetest.QueryRaw(t, parts.Database.Path(), `SELECT item_id FROM fetch_failure`)
	if len(failures) != 0 {
		t.Errorf("fetch failures after the start = %v; want item 5's cleared, to be fetched again", failures)
	}
	if services.UnknownFailures.IsUnavailable(string(jobs.KindFetchMetadata), "item 5") {
		t.Error("item 5's fetch is held unavailable at the start after the one that gave it up")
	}
	report, err := services.UnknownFailures.Report(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	subjects := []string{}
	for _, kind := range report.Kinds {
		for _, failure := range kind.Failures {
			subjects = append(subjects, failure.Subject)
		}
	}
	if !slices.Equal(subjects, []string{"item 5"}) {
		t.Errorf("kept failures = %v; want item 5's, until its fetch succeeds", subjects)
	}
}
