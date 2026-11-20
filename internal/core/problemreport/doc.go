// SPDX-License-Identifier: AGPL-3.0-or-later

// Package problemreport turns a gathered problem report into Markdown, in the form the household's
// admin downloads it: shareable, every identifying value replaced by a tag, or full, everything as
// gathered with the table of what each tag stands for (D74, D524 §4), and keeps the reports.
//
// Render is pure: it is given the report (the person's words, the device's facts, the playback,
// the core's and each node's recent log, the values known to identify) with the time and the
// report's salt, and touches no store, no network and no clock.
//
// The Keeper gathers and keeps (keeper.go, gathered.go). Make draws the salt, takes the core's
// recent log and asks each connected node for its own, all at once, each within NodeAnswerWithin;
// a node not connected, or that does not answer, is named absent with why and when it was last
// heard (D524 §3). It adds the played file's path, size and kept probe, and stores one row in
// problem_report: who, when, which version, the words, and the rest as gathered_json, nothing made
// safe. Download renders the row in the form asked for, looking up then every name the database
// holds that can say whose the report is (titles, roots, libraries, people, devices, nodes, the
// house's name), so a name added since is hidden too. A report past KeptFor is neither listed nor
// downloaded; Expiring's sweep deletes it, as the core starts and each hour after, logging
// EventExpired. The API (internal/core/api/problem_reports.go) makes a report for anyone signed in
// and lists, downloads and deletes them for the admin; the assembly hands the keeper the process's
// recent log and the node byte socket's client as its NodeLogs. `orpheum bugreport` (cmd/orpheum,
// D524 §5) gathers a report itself for a core that does not start, and renders it the same way,
// with the same names looked up (AddKnown) and the command that made it in place of the page.
//
// A tag is `kind:xxxxxx`, a salted hash's first hex digits, lengthened where two values would share
// one; one salt per report, so the core's and the nodes' lines name one file by one tag, and every
// report's tags differ. A path's tag tags each component and keeps its depth and its file's
// extension: /media/a/b.mkv is three tags under slashes, the last ending in .mkv.
//
// A log field's string is named whole (default deny) unless its key's last dotted segment is on
// shapeKeys, a short list of keys whose values the code chooses from a few (the event, a request's
// ID, a kind, a state, an outcome...); under a free-text key (err, why, reason...) or when it holds
// a URL it is scrubbed instead. Numbers, durations, times and booleans are kept. Free text is
// scrubbed, in order: URLs (host and path named, user, query and fragment dropped in both forms),
// e-mail addresses, known paths, any other absolute path, IP addresses (not loopback), then every
// known value and every value named elsewhere in the report, the longest first and as whole words
// (a value under five letters only in its own case; one under two not at all).
//
// # Invariants
//
//   - The shareable form shows no title, path, person's, device's, node's or house's name, or
//     address the report knows or finds, in fields or free text, and keeps the file's shape.
//     Enforced by TestRender_TheShareableFormNamesNoTitlePathPersonOrAddress and
//     TestRender_ScrubsFreeTextByWholeWordsAndEachKindOfName.
//   - One value has one tag throughout a report: in fields, free text, the core's lines and the
//     nodes'; another salt gives other tags; no two values share one. Enforced by
//     TestRender_OneValueHasOneTagInFieldsFreeTextAndEveryComponent,
//     TestRender_AnotherSaltGivesOtherTags and TestNames_NoTwoValuesShareATag.
//   - The full form names everything, carries the do-not-share banner, and its table says what
//     every tag of the shareable form stands for; the shareable form says to review it, that it is
//     best-effort, and not to share a credential problem. Enforced by
//     TestRender_TheFullFormNamesEverythingAndMapsEachTag.
//   - An other library's names stay tags, out of the full form's table, unless the download asks
//     for them, and the report says which it did (D349 §10). Enforced by
//     TestRender_AnOtherLibrarysNamesStayTagsUnlessTheDownloadAsks.
//   - A field's string is named unless its key is shape; numbers, durations and booleans are kept.
//     Enforced by TestRender_AFieldsStringIsNamedUnlessItsKeyIsShape.
//   - No secret reaches either form: a secrets.Value is already "[redacted]", and a URL's user,
//     query and fragment are dropped in both (D194 §5). Enforced by
//     TestRender_NoSecretReachesEitherForm.
//   - The summary is at most SummaryLimit bytes, shows the newest lines, says how many it leaves
//     out; a component with no log is listed as absent with its reason. Enforced by
//     TestRender_TheSummaryFitsAnIssueAndSaysWhatItLeavesOut.
//   - The form has no default: an empty or unknown one is an error. Enforced by
//     TestRender_TheFormIsRequired.
//   - Free text looks a known value up by its first two words, so a common first word ("the")
//     does not compare every title it starts at each of its uses: a download of a day's log with a
//     household's titles takes a fraction of a second. Enforced by
//     TestScrub_LooksUpATitleByItsFirstTwoWords.
//   - Nothing gathered is read as HTML: a person's words cannot end the report's `<details>`.
//     Enforced by TestRender_AWordCannotEndTheReportsHTML.
//   - A report holds the core's recent log at every level and each connected node's; a node that
//     gave none is named absent with why and when it was last heard. Enforced by
//     TestMake_KeepsTheCoresAndEachNodesLogAndNamesANodeThatGaveNoneAbsent.
//   - The shareable download hides every name the database holds that says whose the report is,
//     in the person's words too. Enforced by
//     TestDownload_TheShareableFormHidesTheNamesTheDatabaseHolds.
//   - One person makes at most PerPersonPerHour reports in an hour. Enforced by
//     TestMake_RefusesAPersonPastTheirReportsInAnHour.
//   - A report is kept KeptFor (60 days), then neither listed nor downloaded and deleted by the
//     sweep; Delete takes one at once (D524 §4). Enforced by TestList_AReportGoesAfterSixtyDays and
//     TestDelete_TakesOneReportAtOnce.
//   - A played file's shape keeps codecs, depths, sizes, rates, layouts and languages, and no
//     container's or track's title (D74). Enforced by
//     TestShape_KeepsTheFilesShapeAndLeavesItsTitlesBehind.
//   - A report made on the command line says the command that made it, and its facts are the
//     machine's, not a page's device's. Enforced by
//     TestRender_AReportFromTheCommandLineSaysTheCommandThatMadeIt.
//
// # Read before changing
//
//   - D74: pseudonymise rather than redact; keep the shape, drop the identity; the outer
//     `<details>`, the 64,000-byte summary, absent components named with a reason.
//   - D524: the report from inside the app, its two downloads, and how a log's values are named.
//   - D349 §10: nothing about an other video leaves the server unless the admin asks.
//   - D194: secrets log as "[redacted]"; the canary test covers the report.
//
// # Where to change
//
//   - fields.go: which log keys are shape (shapeKeys) and which free text (freeTextKeys). A key
//     goes on shapeKeys only if no caller can log a title, path, name, address, URL or free text
//     under it.
//   - scrub.go: what free text is scrubbed of, and in what order. names.go: tags.
//   - markdown.go: the report's sections and the summary's cut.
//   - keeper.go: what is gathered, the rate limit and how long a report is kept; gathered.go: the
//     stored shape (gathered_json), which a report already kept must still read.
package problemreport
