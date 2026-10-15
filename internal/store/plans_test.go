// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// listQuery is one of the store's queries, as its file writes it.
type listQuery struct {
	name string
	kind string // sqlc's command: one, many, exec…
	sql  string
}

var (
	queryName = regexp.MustCompile(`(?m)^-- name: (\w+) :(\w+)\s*$`)
	queryArg  = regexp.MustCompile(`sqlc\.n?arg\(\w+\)`)
	fullScan  = regexp.MustCompile(`^SCAN (\w+)$`)
	// An INSERT of values, not of a query's rows.
	insertsValues = regexp.MustCompile(`(?is)^\s*INSERT\b[^;]*\bVALUES\b`)
	// A plan line that reads every row of a table, or of an index over all of it, or
	// builds an index for one execution because none serves it (sql.md section 13, DL1).
	wholeRead = regexp.MustCompile(`^(SCAN (\w+)( USING (COVERING )?INDEX \w+)?|.*AUTOMATIC .*INDEX.*)$`)
)

// queriesIn are every query in the .sql files of directory, their arguments made placeholders.
func queriesIn(t *testing.T, directory string) []listQuery {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(directory, "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	var found []listQuery
	for _, path := range paths {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		marks := queryName.FindAllSubmatchIndex(text, -1)
		for index, mark := range marks {
			end := len(text)
			if index+1 < len(marks) {
				end = marks[index+1][0]
			}
			var body []string
			for line := range strings.SplitSeq(string(text[mark[1]:end]), "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "--") {
					body = append(body, line)
				}
			}
			found = append(found, listQuery{
				name: string(text[mark[2]:mark[3]]), kind: string(text[mark[4]:mark[5]]),
				sql: queryArg.ReplaceAllString(strings.Join(body, "\n"), "?"),
			})
		}
	}
	return found
}

// listingQueries are the :many queries named List… in the query files.
func listingQueries(t *testing.T) []listQuery {
	t.Helper()
	return slices.DeleteFunc(queriesIn(t, "queries"), func(query listQuery) bool {
		return query.kind != "many" || !strings.HasPrefix(query.name, "List")
	})
}

// planOf is a query's plan, one line per step, on pool.
func planOf(t *testing.T, pool *sql.DB, query listQuery) []string {
	t.Helper()
	args := make([]any, strings.Count(query.sql, "?"))
	rows, err := pool.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query.sql, args...)
	if err != nil {
		t.Fatalf("%s: %v", query.name, err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck // read-only
	var lines []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

// fullyScannedTables are the tables a query's plan reads whole, without an index.
func fullyScannedTables(t *testing.T, database *Database, query listQuery) []string {
	t.Helper()
	var scanned []string
	for _, line := range planOf(t, database.readers, query) {
		if match := fullScan.FindStringSubmatch(line); match != nil {
			scanned = append(scanned, match[1])
		}
	}
	return scanned
}

// wholeReads are the lines of a query's plan that read a whole table or index, or build an index
// for one execution.
func wholeReads(t *testing.T, pool *sql.DB, query listQuery) []string {
	t.Helper()
	var reads []string
	for _, line := range planOf(t, pool, query) {
		if wholeRead.MatchString(line) {
			reads = append(reads, line)
		}
	}
	return reads
}

// searchQueries are search's three hand-written statements.
func searchQueries() []listQuery {
	return []listQuery{
		{name: "SearchWords", sql: searchWordsSQL},
		{name: "SearchTrigram", sql: searchTrigramSQL},
		{name: "SearchSubstring", sql: searchSubstringSQL},
	}
}

// Every query in both databases, and each of search's statements, reads by an index, or is listed
// in wholeReadsAllowed with why it may read a whole table: the plan check reads them all, not only
// the listing queries (D303, DL1; the access audit's A6 was a Get… that scanned every file). The
// plans are read on empty databases: without statistics SQLite plans the same whatever the rows.
func TestQueryPlans_EveryQueryReadsByAnIndexOrSaysWhy(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	cache := openCacheAt(t, filepath.Join(t.TempDir(), "cache.db"))
	allowed := wholeReadsAllowed()
	check := func(pool *sql.DB, prefix string, queries []listQuery) {
		for _, query := range queries {
			name := prefix + query.name
			if insertsValues.MatchString(query.sql) {
				// The plan of an INSERT … VALUES lists the child tables' foreign-key bookkeeping,
				// which runs only while a deferred violation is outstanding, and none is deferred.
				continue
			}
			reads := wholeReads(t, pool, query)
			reason, isAllowed := allowed[name]
			delete(allowed, name)
			switch {
			case len(reads) > 0 && !isAllowed:
				t.Errorf("%s reads %q; give it an index, or list it in wholeReadsAllowed with why that is right "+
					"(sql.md section 13)", name, reads)
			case len(reads) == 0 && isAllowed:
				t.Errorf("%s no longer reads a whole table; take it off wholeReadsAllowed (%s)", name, reason)
			}
		}
	}
	check(database.readers, "", queriesIn(t, "queries"))
	check(database.readers, "", searchQueries())
	check(cache.database.readers, "cache.", queriesIn(t, filepath.Join("cache", "queries")))
	for name := range allowed {
		t.Errorf("wholeReadsAllowed names %s, which is no query", name)
	}
}

// wholeReadsAllowed are the queries that read a whole table or index, each with why that is right.
// A new one fails until it is listed here, and a listed one that no longer does fails until it is
// taken off. An entry naming D303 is a defect the redesign removes (sql.md section 13).
func wholeReadsAllowed() map[string]string {
	const (
		few      = "a household has a handful: "
		redesign = "a defect D303's fresh baseline removes: "
		sweep    = "a background sweep over the library, which D303 phase C makes incremental: "
	)
	return map[string]string{
		// Tables a household keeps small.
		"ListProviderMatchedLibraryIDs": few + "libraries; the first-match sweep lists them",
		"ListLiveLibraryIDs":            few + "libraries; the identification sweep lists them",
		"GetLiveLibraryByName":          few + "libraries; a name is checked when one is made or renamed",
		"ListLibraries":                 few + "libraries",
		"GetLibraryForRoot":             few + "roots; a scan finds its library once",
		"ListLiveRootsOfNode":           few + "roots; retiring a node reads them",
		"ListCheckedRoots":              few + "roots; the scan schedule reads each one's last scan every 15 minutes",
		"ListOfflineRoots":              few + "roots; the System page lists them",
		"ListOfflineDisks":              few + "roots, whose missing disks the System page lists",
		"ListWithheldRoots":             few + "roots; the System page lists those whose scans wait for the admin",
		"GetLiveNodeByName":             few + "nodes",
		"ListLiveNodes":                 few + "nodes",
		"ListRoles":                     few + "roles and people; the admin lists them (D243 section 4)",
		"ListRoleLibraries":             few + "roles' libraries; the admin lists them (D243 section 4)",
		"DeleteRole":                    few + "people, whose roles a removed role's check reads",
		"DemoteAdministrators":          few + "people and roles; the operator's reset-admin reads them",
		"CountLiveUsers":                few + "people; setup counts them",
		"GetLiveUserByName":             few + "people; a name is checked when one is made or renamed",
		"ListLiveUsers":                 few + "people; the admin lists them",
		"ListLiveInvitations":           few + "people's grants; the admin lists the invitations",
		"ListAllSealedSecrets":          few + "secrets; doctor lists them",
		"ListSettings":                  few + "settings, a fixed registry; the admin lists them",
		"ListSeriesByAdded": "newest first reads the series, which are few (200 at the reference scale), " +
			"and sorts them: walking item_added would pass every film and episode",
		"ListSeriesByIdentification": "the needs-a-match list's series, which are few",
		"ListFilmsByAdded":           "a keyset page walks item_added in order and stops at its LIMIT",
		"SearchSubstring": "a query too short for either full-text index reads every name once, by design " +
			"(D48): a household's names take milliseconds",
		"DeleteSortKeysOfOtherLocales": "runs once, when the sort locale changes",
		"TombstoneLiveFilesOfLibrary":  "runs once, when a library is removed",
		"cache.ListArtServed":          "the artwork ceiling's sweep reads when every image was served, once a day",
		"cache.ListNodeAttempts":       few + "nodes' attempts",
		"cache.ListFinishedRenditions": "read once at start, to adopt what the node kept",
		"cache.ListScans":              few + "scans at a time, one per root",
		// Defects the redesign removes (the access audit and the independent audit).
		"GetLatestTombstoneByPayloadHash": redesign + "no index serves removed files by payload (access audit A6)",
		"ListTitlesByIdentification":      redesign + "counted by fetching every row (access audit A7)",
		"MoveVersionsBetweenTitles":       redesign + "version_title is partial, so moving every version reads all",
		"GetWorkOfFile":                   redesign + "collection_member has no index by member title",
		"ListRefreshRequests":             redesign + "without statistics the planner walks every item (DL15)",
		"ListFileTitles":                  redesign + "file has no index by library",
		"CountLibraryFilings":             redesign + "file has no index by library",
		"ListUnreadablePaths":             redesign + "no index serves the errored files the System page lists",
		"RevokeCredentialsOfDevice":       redesign + "grant has no index by device",
		"RevokeDeviceSecretsOfDevice":     redesign + "grant has no index by device",
		"ListLiveLocators":                redesign + "every grant ever issued is read to find the live sessions",
		"cache.ListEveryOpenJob":          redesign + "the job queue has no index by state (independent audit S4)",
		"cache.ListOpenJobsOfTarget":      redesign + "every enqueue reads the queue (independent audit S4)",
		"cache.SetJobWanted":              redesign + "the job queue has no index by target (independent audit S4)",
		"cache.TakeOverdueAssignedJob":    redesign + "the job queue has no index by state (independent audit S4)",
		"cache.RequeueRunningJobs":        redesign + "the job queue has no index by state (independent audit S4)",
		"cache.DemoteUnwantedJobs":        redesign + "no index by wanted time (independent audit S4)",
		"cache.DeleteFinishedJobs":        redesign + "no index by finish time (independent audit S4)",
		"cache.CountJobsByKind":           redesign + "the System page counts the whole queue (independent audit S4)",
		"cache.ListJobs":                  redesign + "the job view sorts the whole queue (independent audit S4)",
		// Whole-library sweeps the derivation split makes incremental.
		"ListIdentifyFiles":                     sweep + "the pass reads every file of every library",
		"ListItemsDueRecheck":                   sweep + "the recheck sweep reads every image",
		"ListItemsWithLiveUnchosenLocalArtwork": sweep + "each pass reads every image",
		"ListResponsesAskedBefore":              sweep + "the rolling refresh reads every provider response",
		"CountItemsMissingSortKey":              sweep + "start counts the names with no key in the locale",
		"ListNamesWithSortAndSearchKeys":        sweep + "the key refresh reads every name, each with its key by item",
	}
}
