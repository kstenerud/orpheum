// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// listQuery is one of the store's queries, as its file writes it.
type listQuery struct {
	name    string
	kind    string // sqlc's command: one, many, exec…
	sql     string
	comment string // its comment lines, which say why it reads or writes as it does
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
			var body, comment []string
			for line := range strings.SplitSeq(string(text[mark[1]:end]), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "--") {
					comment = append(comment, line)
				} else {
					body = append(body, line)
				}
			}
			found = append(found, listQuery{
				name: string(text[mark[2]:mark[3]]), kind: string(text[mark[4]:mark[5]]),
				sql: queryArg.ReplaceAllString(strings.Join(body, "\n"), "?"), comment: strings.Join(comment, "\n"),
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

// searchQueries are search's hand-written statements: those that find hits, and those that find the
// ids a searched list holds.
func searchQueries() []listQuery {
	return []listQuery{
		{name: "SearchWords", sql: searchWordsSQL},
		{name: "SearchTrigram", sql: searchTrigramSQL},
		{name: "SearchSubstring", sql: searchSubstringSQL},
		{name: "MatchWords", sql: matchWordsSQL},
		{name: "MatchTrigram", sql: matchTrigramSQL},
		{name: "MatchSubstring", sql: matchSubstringSQL},
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
		// folders is a read of whether a place is covered, through place_library, version_library or
		// library_place: the library folders on its node, a handful, read whole (D363).
		folders = "a household has a handful of library folders, those on a place's node read whole for " +
			"whether they cover it (D363): "
	)
	return map[string]string{
		// Tables a household keeps small.
		"ListProviderMatchedLibraryIDs": few + "libraries; the first-match sweep lists them",
		"ListLiveLibraryIDs":            few + "libraries; the identification sweep lists them",
		"GetLiveLibraryByName":          few + "libraries; a name is checked when one is made or renamed",
		"ListLibraries":                 few + "libraries",
		"SummarizeResourceUse": "every row kept is what it sums: a minute's row for the core and each node, " +
			"pruned past a month, read when an admin opens the System page",
		"ListWatchedLiveFiles": "every file anyone watched is what it lists, by the covering index of play " +
			"position's files, once a minute, for their work's rung above the background (D324)",
		"ListLibrariesAtOrUnderRoot": few + "roots; a scan finds the libraries under its root once",
		"ListLiveRootPathsOnNode":    few + "roots; a scan's request finds the root it runs under",
		"ListLiveRootsOfNode":        few + "roots; retiring a node reads them",
		"ListCheckedRoots":           few + "roots; the scan schedule reads each one's last scan every 15 minutes",
		"ListUnavailablePlaces": few + "roots and disks in them; the unavailable ones' sweep reads them " +
			"every unavailable_root_rescan_seconds (D328)",
		"ListOfflinePlaces": few + "roots and disks in them; the System page lists those offline (D328)",
		"ListWithheldRoots": few + "roots; the System page lists those whose scans wait for the admin",
		"GetLiveNodeByName": few + "nodes",
		"ListLiveNodes":     few + "nodes",
		"ListRoles":         few + "roles and people; the admin lists them (D243 section 4)",
		"ListRoleLibraries": few + "roles' libraries; the admin lists them (D243 section 4)",
		"DeleteRole":        few + "people, whose roles a removed role's check reads",
		// Removing a person, which is rare, reads these once (D305).
		"ClearSettingsSetByRemovedUser":      few + "settings, a hundred or so",
		"ClearSecretsSetByRemovedUser":       few + "keys",
		"ClearOverridesSetByRemovedUser":     "the overrides people typed, a few hundred at most",
		"DeleteGrantsOfRemovedUser":          "every credential ever made, which retention bounds",
		"DeleteLocatorsOfRemovedUser":        "every credential, for the removed person's sessions' locators",
		"DeleteInvitationLinksOfRemovedUser": "every credential, for the removed person's invitations' links",
		"DeleteGrantsDerivedFromRemovedUser": "every credential, for those derived from the removed person's",
		"DeleteDevicesOfRemovedUser":         few + "devices",
		// The daily retention sweep over every credential, which it bounds (D304 section 6).
		"DeleteLocatorsOfEndedGrants":         "every credential, for the ended ones' locators; a daily sweep",
		"DeleteInvitationLinksOfEndedGrants":  "every credential, for the invitations' ended links; a daily sweep",
		"DeleteWaitingInvitationsWithNoLinks": few + "invitations, for those whose every link went; a daily sweep",
		"DeleteEndedGrants":                   "every credential, for the ended ones; a daily sweep that bounds them",
		// A union of reads each narrowed by an index, read once as it is made (D306 section 8).
		"FoldWatchingOfPurgedFiles":  "the spans and finishes of a batch of deleted files, each found by its file",
		"GetWatchStats":              "a person's spans and their watching of deleted files, each found by the person",
		"DemoteAdministrators":       few + "people and roles; the operator's reset-admin reads them",
		"CountLiveUsers":             few + "people; setup counts them",
		"GetLiveUserByName":          few + "people; a name is checked when one is made or renamed",
		"ListLiveUsers":              few + "people; the admin lists them",
		"ListInvitations":            few + "roles and invitations; the admin lists the invitations",
		"ListLiveInvitationLinks":    few + "invitations, each one's links by their index; the admin lists them",
		"ListAllSealedSecrets":       few + "secrets; doctor lists them",
		"ListSettings":               few + "settings, a fixed registry; the admin lists them",
		"ListSeriesByIdentification": "the needs-a-match list's series, which are few",
		"CountWaitingForAPerson": "it reads title_ambiguous and title_waiting, indexes of the few titles it counts, " +
			"and the series, which are few",
		"SearchSubstring": "a query too short for either full-text index reads every name once, by design " +
			"(D48): a household's names take milliseconds",
		"MatchSubstring": "a list searched by a query too short for either full-text index reads every name " +
			"once, as search does (D48)",
		"DeleteSortKeysOfOtherLocales": "runs once, when the sort locale changes",
		"cache.ListArtServed":          "the artwork ceiling's sweep reads when every image was served, once a day",
		"cache.ListNodeAttempts":       few + "nodes' attempts",
		"cache.ListScans":              few + "scans at a time, one per root",
		"cache.ListRescans":            few + "rescans at a time, one per library or item an admin pressed (D357)",
		"cache.ListRematches": "the rescans' sweep queues every film and series they wait on, which a rescan's " +
			"jobs delete as they run (D357)",
		// Small tables the planner reads whole by the committed statistics (D303, DL15).
		"ListSeriesByName": "by the statistics the planner reads the series, which are few (200 at the " +
			"reference scale), and sorts a page of them",
		"ListFilesOfWork": "the series' orderings, which are few, for the aired one; an admin's report of a " +
			"work's filing, which is rare (D306 section 3)",
		"ListUnnumberedEpisodeFiles": "the series' orderings, which are few (one or two a series), each searched " +
			"for its positions; Needs attention looks for episodes read wrongly (D306 section 6)",
		"ListFilmsWithoutSound": "every film's files are checked for a sound track, by index each; Needs " +
			"attention reads it once a look, and the list stops at its bound",
		"ListUnreadablePaths":   few + "nodes, which the errored files are joined to",
		"ListLibraryRootStates": few + "roots and nodes",
		"ListLiveRealmKeys":     few + "people, whose realm keys a discovery lists",
		"ListLibraryIDsOfRole":  few + "libraries",
		"ListLibrarySeries":     "a library's series, which are few, read whole by the statistics",
		"ListLibraryUnnumberedEpisodePaths": "the series' orderings, which are few, each searched for its " +
			"unnumbered positions; a pass reads it once (D307 section 2)",
		"ListSeriesAwaitingMatch": "the first-match sweep reads the series, which are few",
		"ListTitlesAwaitingMatch": sweep + "the first-match sweep reads the titles never answered",
		"ListLibraryRoots":        few + "roots",
		"RetireRootsOfLibrary":    few + "roots; a library's removal retires them",
		// Whether a library covers a place (D363).
		"CountLibraryFilings":  folders + "the scanner view's counts",
		"CountOrderingMembers": folders + "a page's series' episodes counted",
		"IsArtworkReached":     folders + "whether a person reaches an image",
		"IsFileReached":        folders + "whether a person reaches a file",
		"IsItemReached":        folders + "whether a person reaches an item",
		"ListLibrariesOfItem":  folders + "the libraries a title's or a series' page goes up to (D367)",
		// No index leads from an extra to its film: the relation's few rows (a film's handful of extras)
		// are read once as an extra's page or playing opens (D367 section 2).
		"GetFilmOfExtra": "the extras' relations, few, read as an extra's page or playing opens (D367)",
		"ListArtworkSourceSha256s": folders + "the artwork ceiling's daily sweep, for the images of what no " +
			"library lists",
		"ListEpisodesNotInOrdering":            folders + "a series' episodes an ordering leaves out",
		"ListFileTitles":                       folders + "the scanner view",
		"ListFilesPage":                        folders + "the scanner view",
		"ListFilmsByAdded":                     folders + "a page of films",
		"ListFilmsByName":                      folders + "a page of films",
		"ListFilesToReadThrough":               folders + "the read-through sweep, a range of files at a time",
		"ListFirstEpisodes":                    folders + "a page's series' first episodes",
		"ListIdentifyFiles":                    folders + "the identification pass, once",
		"ListLibrariesOfFile":                  folders + "the libraries a played file is in",
		"ListLibraryNFOBodies":                 folders + "the identification pass's snapshot",
		"ListLibraryRecordedTimes":             folders + "an other library's pass, once",
		"ListLibraryVersionTitles":             folders + "the identification pass's snapshot",
		"ListLiveFileLocationsOfSeries":        folders + "where a page's series' files are",
		"ListLiveFileLocationsOfTitles":        folders + "where a page's titles' files are",
		"ListOrderingMemberFilesWithPositions": folders + "a series' members for Next Up",
		"ListOtherByAdded":                     folders + "a page of other videos",
		"ListOtherByFileName":                  folders + "a page of other videos",
		"ListOtherByName":                      folders + "a page of other videos",
		"ListOtherByRecorded":                  folders + "a page of other videos",
		"ListOtherVideosOfLibrary":             folders + "an other library's folders",
		// Every list's other direction, and films and series by release date (D366), as their twins.
		"ListFilmsByNameDescending":     folders + "a page of films",
		"ListFilmsByAddedAscending":     folders + "a page of films",
		"ListFilmsByReleased":           folders + "a page of films",
		"ListFilmsByReleasedAscending":  folders + "a page of films",
		"ListFilmsUndated":              folders + "a page of films",
		"ListSeriesByNameDescending":    folders + "a page of series",
		"ListSeriesByAddedAscending":    folders + "a page of series",
		"ListSeriesByReleased":          folders + "a page of series",
		"ListSeriesByReleasedAscending": folders + "a page of series",
		"ListSeriesUndated":             folders + "a page of series",
		"ListOtherByNameDescending":     folders + "a page of other videos",
		"ListOtherByAddedAscending":     folders + "a page of other videos",
		"ListOtherByRecordedAscending":  folders + "a page of other videos",
		"ListOtherByFileNameDescending": folders + "a page of other videos",
		"ListPlacesOfFile":              folders + "the places a file is read from",
		"ListRecentPlayPositions":       folders + "Continue Watching",
		"ListSeasonEpisodeDetails":      folders + "a season's episodes",
		"ListSeriesByAdded":             folders + "a page of series",
		"ListSeriesByProviderID":        folders + "a series found by its provider id",
		"ListSeriesEpisodeDetails":      folders + "a series' episodes",
		"ListSubtitleTracksOfVideo":     folders + "the subtitles beside the places a person reaches",
		"ListTitleExtras":               folders + "a title's extras",
		"GetPersonReached":              folders + "whether a person's page has a title its viewer reaches",
		"ListPersonCredits":             folders + "a person's page, their titles its viewer reaches",
		"ListGenresWithCounts": few + "genres, a few dozen from TMDB and the NFOs, each counted once as " +
			"the genre chips open; " + folders + "the titles counted",
		"ListFilteredItemIDs": "a list's genre, person, decade or unwatched filter reads the films and series " +
			"once, as MatchSubstring reads every name for a short query (D48): a household's titles take " +
			"milliseconds",
		"ListTitleFiles":                 folders + "a title's files at the places its person reaches",
		"ListTitlesByProviderID":         folders + "a film found by its provider id",
		"ListTitlesNoLibraryLists":       folders + "a retire's titles no library lists",
		"ListTitlesOfFileNoLibraryLists": folders + "a retired file's titles",
		"ListTitlesToAdopt":              folders + "the identification pass, once",
		"ListUncoveredFilesUnder":        folders + "a retired folder's places no library covers",
		"SearchTrigram":                  folders + "a search's hits",
		"SearchWords":                    folders + "a search's hits",
		"MatchTrigram":                   folders + "the titles a searched list holds",
		"MatchWords":                     folders + "the titles a searched list holds",
		"ListCopyCandidates": sweep + "the copies' sweep reads every live video for one whose payload a video " +
			"found before it has (D363 section 2)",
		// Defects the redesign removes (the access audit and the independent audit).
		"ListTitlesByIdentification": redesign + "counted by fetching every row (access audit A7)",
		"ListRefreshRequests":        "the refresh sweep reads every request a person made, a handful",
		"ListLiveLocators":           few + "signed-in devices, one locator each; a discovery lists them",
		"cache.CountJobsByKind":      "the System page counts the queue, which pruning holds to what ended lately",
		"cache.ListJobs":             "the job view walks the newest jobs by id and stops at its LIMIT",
		// Whole-library sweeps the derivation split makes incremental.
		"ListItemsDueRecheck":                   sweep + "the recheck sweep reads every image",
		"ListItemsWithLiveUnchosenLocalArtwork": sweep + "each pass reads every image",
		"ListResponsesAskedBefore":              sweep + "the rolling refresh reads every provider response",
		"ListSeriesOwingOrderingParts": sweep + "the sweep of what should have been fetched reads every series, " +
			"hourly, for the few owed a part of their orderings (D335)",
		"CountItemsMissingSortKey":       sweep + "start counts the names with no key in the locale",
		"ListNamesWithSortAndSearchKeys": sweep + "the key refresh reads every name, each with its key by item",
		"ListOtherVideoFileNames": sweep + "the key refresh reads every other video's file name with its key, " +
			"as it reads every name (D349)",
	}
}

// The queries that read a small table whole read that one, never the large table they join: the
// refresh requests, not every item; the locators, not every grant ever issued (D303, DL2).
func TestQueryPlans_ASmallTableDrivesItsJoin(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	drivenBy := map[string]string{"ListRefreshRequests": "refresh_request", "ListLiveLocators": "locator"}
	for _, query := range queriesIn(t, "queries") {
		table, isChecked := drivenBy[query.name]
		if !isChecked {
			continue
		}
		for _, read := range wholeReads(t, database.readers, query) {
			if !strings.HasPrefix(read, "SCAN "+table) {
				t.Errorf("%s reads %q whole; want only %s whole", query.name, read, table)
			}
		}
	}
}

// indexSearch is a plan line that narrows by an index: the table, the index (the table's own name
// for a WITHOUT ROWID primary key, "" for a rowid), and the terms it binds.
var indexSearch = regexp.MustCompile(
	`^SEARCH (\w+)(?: AS \w+)? USING (?:COVERING )?(?:INDEX (\w+)|(PRIMARY KEY)|INTEGER PRIMARY KEY) \((.+)\)`)

// rowsPerSearch is how many rows SQLite's statistics say a search reads for each value it binds:
// the stat's figure for as many leading columns as the search binds with =.
func rowsPerSearch(stats map[string]string, table, index, terms string) (rows, total int) {
	stat, isKnown := stats[table+"/"+index]
	if !isKnown {
		return 0, 0
	}
	figures := strings.Fields(stat)
	bound := strings.Count(terms, "=?")
	if bound == 0 || bound >= len(figures) {
		return 0, 0
	}
	total, totalErr := strconv.Atoi(figures[0])
	rows, rowsErr := strconv.Atoi(figures[bound])
	if totalErr != nil || rowsErr != nil {
		return 0, 0
	}
	return rows, total
}

// weakSearchesAllowed are the queries whose search binds only columns with few values, each with
// why. A keyset page, which stops at its LIMIT, needs none.
func weakSearchesAllowed() map[string]string {
	const (
		library = "it reads a library's files or items, all of them, by design: "
		node    = "it reads a node's files, all of them, by design: "
		person  = "it reads a person's history, all of it, by design: "
		// snapshot is the identification pass's read of its library at its start.
		snapshot = "the identification pass's snapshot"
	)
	return map[string]string{
		"DeleteDevicesOfRemovedUser": person + "the check that no play names a device deleted, whose plays " +
			"removing the person deleted first (D305)",
		"ListLiveFilesUnder": library + "a scan's comparison and its removals, of the places under its " +
			"root",
		"ListLiveFilesForAttaching":       library + "a scan's attaching of sidecars",
		"ListFilesToCompare":              library + "a scan's comparison",
		"ListFilesAHeldBackScanDidNotSee": library + "what a withheld scan would remove",
		"ListIdentifyFiles":               library + "the identification pass",
		"ListLibrarySeriesWithFetchedGroups": library + "a pass's last step, its series with an episode group " +
			"fetched, whose episodes it keeps placed (episode-orderings.md)",
		"ListLibraryTitles": library + "the identification pass",
		// What the pass reads of its library at its start, once a pass, so that it runs no
		// statement per file (D303, DL7).
		"ListLibraryTitlesForPass":       library + snapshot,
		"ListLibraryOverrides":           library + snapshot,
		"ListLibraryProviderIDs":         library + snapshot,
		"ListLibraryOriginalTexts":       library + snapshot,
		"ListLibraryAiredPositions":      library + snapshot,
		"ListLibrarySeriesForPass":       library + snapshot,
		"ListLibrarySeriesEpisodePlaces": library + snapshot,
		"ListLibraryProviderResponses":   library + snapshot,
		"ListLibraryOtherVideos":         library + snapshot,
		"ListLibraryLiveEpisodeSources":  library + snapshot,
		"ListLibraryExtraRelations":      library + snapshot,
		"ListLibraryCredits":             library + snapshot,
		"ListLibraryItemGenres":          library + snapshot,
		"ListTitlesNoLibraryLists":       library + "a retire's titles no library lists any more",
		"ListLiveFilesByNode":            node + "the System page's files of a node",
		"DeletePlaySpansOfUser":          person + "their removal",
		"DeletePlayPositionsOfUser":      person + "their removal",
		"DeleteTitlesWatchedOfUser":      person + "their removal",
		"GetWatchStats":                  person + "their watching's totals",
	}
}

// A search by an index narrows to a few rows for each value it binds: one that binds only a column
// with a handful of values (a locale, a kind, a state) reads most of the table, though the plan says
// SEARCH (D303, DL2; the access audit's A1, SEARCH sort_key USING PRIMARY KEY (locale=?)). Measured
// by the committed statistics.
func TestQueryPlans_EverySearchNarrowsToAFewRows(t *testing.T) {
	t.Parallel()
	database := openMigrated(t)
	stats := map[string]string{}
	for _, row := range queryRows(t, database, "SELECT tbl, coalesce(idx, ''), stat FROM sqlite_stat1", 3) {
		stats[row[0]+"/"+row[1]] = row[2]
	}
	allowed := weakSearchesAllowed()
	for _, query := range append(queriesIn(t, "queries"), searchQueries()...) {
		if regexp.MustCompile(`(?is)\bLIMIT\b`).MatchString(query.sql) || insertsValues.MatchString(query.sql) {
			continue // a page, or a first match, stops at its LIMIT; an insert's plan is foreign-key bookkeeping
		}
		reason, isAllowed := allowed[query.name]
		weak := weakSearches(t, database, stats, query)
		switch {
		case len(weak) > 0 && !isAllowed:
			t.Errorf("%s: %v for each value it binds; bind a column with many values first, or list it in "+
				"weakSearchesAllowed with why", query.name, weak)
		case len(weak) == 0 && isAllowed:
			t.Errorf("%s no longer searches weakly; take it off weakSearchesAllowed (%s)", query.name, reason)
		}
	}
}

// The measure sees the access audit's A1: a search of sort_key that binds only the locale, of
// which there is one, reads every key, though the plan says SEARCH.
func TestRowsPerSearch_SeesASearchThatBindsOnlyTheLocale(t *testing.T) {
	t.Parallel()
	stats := map[string]string{"sort_key/sort_key": "20000 20000 10000 1 1"}
	if rows, total := rowsPerSearch(stats, "sort_key", "sort_key", "locale=?"); rows != 20000 || total != 20000 {
		t.Errorf("a search by the locale reads %d of %d rows; want all 20000", rows, total)
	}
	whole := "locale=? AND listing=? AND collation_key=?"
	if rows, _ := rowsPerSearch(stats, "sort_key", "sort_key", whole); rows != 1 {
		t.Errorf("a search by the whole key reads %d rows; want 1", rows)
	}
}

// weakSearches are the lines of a query's plan that read many rows for each value they bind.
func weakSearches(t *testing.T, database *Database, stats map[string]string, query listQuery) []string {
	t.Helper()
	var weak []string
	for _, line := range planOf(t, database.readers, query) {
		match := indexSearch.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		index := match[2]
		if match[3] != "" {
			index = match[1]
		}
		if rows, total := rowsPerSearch(stats, match[1], index, match[4]); rows > 64 && rows*4 > total {
			weak = append(weak, fmt.Sprintf("%q reads about %d of %d rows", line, rows, total))
		}
	}
	return weak
}
