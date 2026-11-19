// SPDX-License-Identifier: AGPL-3.0-or-later

// Package problemreport turns a gathered problem report into Markdown, in the form the household's
// admin downloads it: shareable, every identifying value replaced by a tag, or full, everything as
// gathered with the table of what each tag stands for (D74, D524 §4). It is pure: the caller
// gathers the report (the person's words, the device's facts, the playback, the core's and each
// node's recent log, the values it knows identify) and passes in the time and the report's salt;
// this package touches no store, no network and no clock.
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
package problemreport
