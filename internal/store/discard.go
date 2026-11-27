// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import "maps"

// A rescan that discards what was made is the escape hatch from a cache gone wrong (D535 §10): within
// its scope, a library, every library, or one video page's item and its files, every row the app made,
// measured, fetched, guessed or gave up on goes, failures and successes alike, and only what a person
// did stays: what they chose, edited, entered and watched, and what they put on disk, which the rescan
// reads again. PersistentTablesInADiscard and CacheTablesInADiscard say which of every table, and how,
// and TestDiscard_EveryTableSaysWhatADiscardDoesWithIt fails on a table that says nothing, so a new
// table cannot be missed.

// InADiscard is what a discarding rescan does with a table's rows in its scope.
type InADiscard string

// What a discarding rescan does with a table's rows.
const (
	// KeptAsAPersonsDoing is a person's: what they chose, edited, entered or watched.
	KeptAsAPersonsDoing InADiscard = "kept"
	// DiscardedAsItStarts goes as the rescan starts, in its scope and one write
	// (identify.Rescans.forgetGivenUp: the §8 forgetters and the §10 MadeDiscarders).
	DiscardedAsItStarts InADiscard = "discarded as it starts"
	// DiscardedByTheRead goes as the rescan reads each file again, by the one way a difference in its
	// probe throws it out (library.DerivedDroppers).
	DiscardedByTheRead InADiscard = "discarded by the read"
	// DiscardedByTheMatchAgain goes as the rescan matches its films and series again
	// (identify.Matcher.discardMade).
	DiscardedByTheMatchAgain InADiscard = "discarded by the match again"
	// RewrittenByTheRescan is what the rescan's own read, pass or match writes whole again from what
	// its files, NFOs and providers say now: nothing of it outlives the rescan unless written again.
	RewrittenByTheRescan InADiscard = "rewritten by the rescan"
	// OutsideADiscard is of no video in a rescan's scope: an account, a configuration, a root's state,
	// the work in progress.
	OutsideADiscard InADiscard = "outside the scope"
)

// TableInADiscard is what a discarding rescan does with one table.
type TableInADiscard struct {
	Fate InADiscard
	// Kept is what of a table the app otherwise makes is a person's and stays; empty for none.
	Kept string
	// Why is the reason, in a line.
	Why string
	// ProvenBy names the test that shows it, for a table kept or discarded.
	ProvenBy string
}

// The tests that prove what a discarding rescan does with each table.
const (
	provenByTheRescansStart = "TestRescan_ADiscardingRescanThrowsOutWhatTheAppMadeAndKeepsWhatAPersonDid"
	provenByTheRead         = "TestScans_ADiscardingRescanThrowsOutEverythingMadeOfAnUnchangedFile"
	provenByTheMatchAgain   = "TestRescan_ADiscardingRescanThrowsOutWhatWasFetchedButKeepsAPersonsDoing"
	provenBySegments        = "TestForgetMeasurements_ThrowsOutTheSegmentsItsRepeatsWereKeptAs"
)

// PersistentTablesInADiscard is what a discarding rescan does with each table of persistent.db, an
// FTS index's own tables counted with it.
func PersistentTablesInADiscard() map[string]TableInADiscard {
	tables := map[string]TableInADiscard{}
	maps.Copy(tables, tablesAPersonDid())
	maps.Copy(tables, tablesDiscardedAsItStarts())
	maps.Copy(tables, tablesDiscardedByTheReadOrMatch())
	maps.Copy(tables, tablesRewrittenByTheRescan())
	maps.Copy(tables, tablesOutsideADiscard())
	return tables
}

// tablesAPersonDid are the tables of what a person did.
func tablesAPersonDid() map[string]TableInADiscard {
	return map[string]TableInADiscard{
		"field_override": {KeptAsAPersonsDoing, "", "a person's edit of a field (D183)", provenByTheRescansStart},
		"file_sound_offset": {
			KeptAsAPersonsDoing, "", "an admin entered a file's sound offset for every person (D537)",
			provenByTheRescansStart,
		},
		"item_hidden":   {KeptAsAPersonsDoing, "", "a person hid the item", provenByTheRescansStart},
		"play_event":    {KeptAsAPersonsDoing, "", "a person's playing (D165)", provenByTheRescansStart},
		"play_position": {KeptAsAPersonsDoing, "", "where a person stopped (D81, D157)", provenByTheRescansStart},
		"play_span":     {KeptAsAPersonsDoing, "", "what a person played (D165)", provenByTheRescansStart},
		"preference":    {KeptAsAPersonsDoing, "", "a person's choices", provenByTheRescansStart},
		"purged_watching": {
			KeptAsAPersonsDoing, "", "a person's watching kept past a purge (D306 §8)",
			provenByTheRescansStart,
		},
		"work_watched": {KeptAsAPersonsDoing, "", "a person marked or watched it (D370)", provenByTheRescansStart},
	}
}

// tablesDiscardedAsItStarts are the tables the app made, gave up on or guessed, thrown out as a
// discarding rescan starts.
func tablesDiscardedAsItStarts() map[string]TableInADiscard {
	return map[string]TableInADiscard{
		"fetch_failure": {
			DiscardedAsItStarts, "", "a provider's failure to fetch, forgotten by every rescan (§8)",
			provenByTheRescansStart,
		},
		"unknown_failure": {
			DiscardedAsItStarts, "", "a failure of unknown cause, forgotten by every rescan (§8, D511)",
			provenByTheRescansStart,
		},
		"whole_file_hash": {
			DiscardedAsItStarts, "", "a whole read of a file, made or failed: read whole again",
			provenByTheRescansStart,
		},
		"guessed_match": {
			DiscardedAsItStarts, "",
			"the app's guess that a match is a guess (D301): guessed again by the match again or the pass",
			provenByTheRescansStart,
		},
		"own_picture_given_up": {
			DiscardedAsItStarts, "", "what the app learned of a device's player (D493): learned again",
			provenByTheRescansStart,
		},
	}
}

// tablesDiscardedByTheReadOrMatch are the tables the app made of a file or fetched for a film or
// series, thrown out as each is read or matched again.
func tablesDiscardedByTheReadOrMatch() map[string]TableInADiscard {
	return map[string]TableInADiscard{
		"loudness":       {DiscardedByTheRead, "", "measured by the node", provenByTheRead},
		"video_timeline": {DiscardedByTheRead, "", "read by the node (D247 §10)", provenByTheRead},
		"repeat_measurement": {
			DiscardedByTheRead, "", "measured by the node; failures forgotten by every rescan (§8)",
			provenByTheRead,
		},
		"subtitle_text": {
			DiscardedByTheRead, "", "made by the node; a fetched subtitle's by the match again (DiscardFetched)",
			provenByTheRead,
		},
		"media_segment": {
			DiscardedByTheRead, "a person's segments, provider 'person'",
			"a detector's: the fingerprint's go with the repeats' measurement, the chapters' are synced by the read",
			provenBySegments,
		},

		// What was fetched or made for a film or series, thrown out as it is matched again.
		"artwork": {
			DiscardedByTheMatchAgain, "a person's chosen image (is_chosen, D357 §3)",
			"fetched or made: retired as never stored, fetched or made again; frame stills by the read",
			provenByTheMatchAgain,
		},
		"fetched_subtitle": {
			DiscardedByTheMatchAgain, "a library's discard keeps the downloads, quota-limited, and converts them again",
			"fetched: a video page's discard throws them out (D535 §6)",
			"TestDiscardFetched_AVideoPagesThrowsOutTheSubtitlesAndTheirText",
		},
	}
}

// tablesRewrittenByTheRescan are the tables a discarding rescan writes whole again.
func tablesRewrittenByTheRescan() map[string]TableInADiscard {
	return map[string]TableInADiscard{
		"file":        {RewrittenByTheRescan, "", "identity, and what the read says of it (D426 §4)", ""},
		"file_filing": {RewrittenByTheRescan, "", "where the read files it", ""},
		"file_nfo":    {RewrittenByTheRescan, "the NFO's match, a person's on disk", "the node sends it again", ""},
		"file_probe":  {RewrittenByTheRescan, "", "the node probes it again (D59)", ""},
		"stream":      {RewrittenByTheRescan, "", "from the probe (D186)", ""},
		"place":       {RewrittenByTheRescan, "", "where the node lists the file (D363 §2)", ""},
		"item": {
			RewrittenByTheRescan, "a person's chosen or confirmed match (D357 §3)",
			"identity, and its match, matched again as never matched (D357 §1), its not-found mark and rules " +
				"version with it",
			"",
		},
		"item_provider_id":  {RewrittenByTheRescan, "", "a provider's ids, fetched again", ""},
		"item_text":         {RewrittenByTheRescan, "", "fetched or read, resolved again (D45)", ""},
		"item_fts":          {RewrittenByTheRescan, "", "an index of item_text (D48)", ""},
		"item_fts_tri":      {RewrittenByTheRescan, "", "an index of item_text (D48)", ""},
		"sort_key":          {RewrittenByTheRescan, "", "derived of the titles (D47)", ""},
		"credit":            {RewrittenByTheRescan, "", "derived by the pass", ""},
		"genre":             {RewrittenByTheRescan, "", "fetched; a dictionary every item shares", ""},
		"item_genre":        {RewrittenByTheRescan, "", "derived by the pass", ""},
		"person":            {RewrittenByTheRescan, "", "fetched; shared by every item a person is credited in", ""},
		"provider_response": {RewrittenByTheRescan, "", "fetched again by the match again's refresh (D170)", ""},
		"extra_relation": {
			RewrittenByTheRescan, "a person's, asserted_by 'user'", "the scanner's, of the files' names",
			"",
		},
		"other_video": {RewrittenByTheRescan, "a person's, recorded_from 'user'", "recorded of its file (D349 §4)", ""},
		"grp":         {RewrittenByTheRescan, "", "a season or disc, of the files and the provider (D426 §5)", ""},
		"group_text":  {RewrittenByTheRescan, "", "a season's or disc's name, fetched again", ""},
		"ordering": {
			RewrittenByTheRescan, "a person's ordering, no provider", "a provider's, fetched again (D96, D159)", "",
		},
		"position": {RewrittenByTheRescan, "", "an episode's place in an ordering, fetched again", ""},
		"sequence": {RewrittenByTheRescan, "a person's series, created_by 'user'", "the scanner's or provider's", ""},
		"version":  {RewrittenByTheRescan, "", "identity: a work's cut, as the files file it", ""},
		"work":     {RewrittenByTheRescan, "", "identity, and its runtime resolved again", ""},
	}
}

// tablesOutsideADiscard are the tables of no video in a rescan's scope.
func tablesOutsideADiscard() map[string]TableInADiscard {
	return map[string]TableInADiscard{
		"device":                {OutsideADiscard, "", "an account's device", ""},
		"device_reader_limit":   {OutsideADiscard, "", "an admin's limit (D240 §5)", ""},
		"grant":                 {OutsideADiscard, "", "an account's grant (D154)", ""},
		"invitation":            {OutsideADiscard, "", "an account's (D246 §2)", ""},
		"invitation_link":       {OutsideADiscard, "", "an account's (D318)", ""},
		"locator":               {OutsideADiscard, "", "the server's address", ""},
		"realm_key":             {OutsideADiscard, "", "a key (D297)", ""},
		"person_key":            {OutsideADiscard, "", "a key (D527)", ""},
		"role":                  {OutsideADiscard, "", "an account's (D130)", ""},
		"role_library":          {OutsideADiscard, "", "an account's (D243 §4)", ""},
		"secret":                {OutsideADiscard, "", "configuration, sealed (D194)", ""},
		"setting":               {OutsideADiscard, "", "the server's configuration (D200)", ""},
		"user":                  {OutsideADiscard, "", "an account", ""},
		"library":               {OutsideADiscard, "", "an admin's configuration: the scope itself", ""},
		"library_root":          {OutsideADiscard, "", "an admin's configuration, as the node reports it (D107)", ""},
		"system_library":        {OutsideADiscard, "", "the server's own (D435)", ""},
		"node":                  {OutsideADiscard, "", "an enrolled node", ""},
		"node_unconfined_leave": {OutsideADiscard, "", "an admin's acceptance (D450)", ""},
		"root_mount":            {OutsideADiscard, "", "a root's mount, of no file", ""},
		"root_scan":             {OutsideADiscard, "", "a root's scan state, which the rescan's scan rewrites", ""},
		"root_state":            {OutsideADiscard, "", "a root's state, and an admin's approval of its removals", ""},
		"unavailable_place":     {OutsideADiscard, "", "a place that cannot be read now (D328), listed again", ""},
		"link_memory":           {OutsideADiscard, "", "a player's network (D257 §1), of no file", ""},
		"problem_report": {
			OutsideADiscard, "", "a person's report (D524), of no item; a file's sound offset it suggests stays (D537)",
			"",
		},
		"refresh_request": {OutsideADiscard, "", "a refresh asked for and pending", ""},
		"schema_meta":     {OutsideADiscard, "", "the schema's own", ""},
		"schema_step":     {OutsideADiscard, "", "the migration runner's record", ""},
	}
}

// CacheTablesInADiscard is what a discarding rescan does with each table of cache.db.
func CacheTablesInADiscard() map[string]TableInADiscard {
	return map[string]TableInADiscard{
		"chunk_store": {DiscardedByTheRead, "", "a stream's chunks, made again (D247 §10)", provenByTheRead},
		"trickplay": {
			DiscardedByTheRead, "", "the scrub bar's pictures' run, made again by the sweep (trickplay.md)",
			provenByTheRead,
		},
		"trickplay_sheet": {
			DiscardedByTheRead, "", "the scrub bar's sheets, made again by the sweep, their images removed",
			provenByTheRead,
		},
		"match_candidate": {
			DiscardedByTheMatchAgain, "", "a provider's candidates (D172), fetched again",
			provenByTheMatchAgain,
		},
		"job":             {OutsideADiscard, "", "the work queue (D193): the rescan's own work is queued in it", ""},
		"scan":            {OutsideADiscard, "", "a scan in progress (D240), the rescan's own", ""},
		"scan_batch":      {OutsideADiscard, "", "a scan's batch in progress (D240 §8)", ""},
		"rescan":          {OutsideADiscard, "", "the rescan itself (D357)", ""},
		"rescan_given_up": {OutsideADiscard, "", "a library's rescan given up, until one finishes", ""},
		"rematch":         {OutsideADiscard, "", "the rescan's own list to match again (D357)", ""},
		"node_attempt":    {OutsideADiscard, "", "work given to a node, in progress (D234 §7)", ""},
		"resource_use":    {OutsideADiscard, "", "telemetry, of no video", ""},
		"art_served":      {OutsideADiscard, "", "when an image was last served (D212 §6), for its eviction", ""},
	}
}
