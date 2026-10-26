// SPDX-License-Identifier: AGPL-3.0-or-later

// Package subtitles offers a played video's text subtitle tracks, has their text made by the
// video's node and kept, serves it as ASS or WebVTT, and fetches a subtitle on request (D217 §6,
// D224). Which track a person plays with is internal/core/playback's (choices.go); a playback
// reaches its tracks through playback's media token.
//
// # Invariants
//
//   - Every text subtitle track of a played video, its own and its subtitle files', is extracted,
//     whatever the decision, once per payload hash of the file it is in; a track not yet extracted
//     is 425 with Retry-After, a failed one is prepare_failed and tried again when next played, a
//     node's refusal is not kept and its job is tried again, and a playback reaches only its own
//     file's tracks (D217 §6). Enforced by
//     TestSubtitles_ARefusedExtractionIsNotKeptAndTheTrackKeepsWaiting,
//     TestSubtitles_ASubtitleFileBesideTheVideoIsOfferedAndConvertedWhole,
//     TestSubtitles_ATextTrackIsOfferedAndQueuedWhenFirstPlayedAndIsTooEarlyUntilExtracted,
//     TestSubtitles_AnExtractedTrackIsServedAsASSAndWebVTTAndNotExtractedAgain and
//     TestSubtitles_AFailedExtractionSaysSoAndIsTriedAgainWhenNextPlayed (internal/core/api).
//
//   - A player asking for a text still being made keeps its job wanted, at play's priority, as a
//     player asking for a rendition does (D234 §3).
//     TestSubtitles_APlayerAskingForTheTextKeepsItsExtractionWanted (internal/core/api).
//
//   - An extraction or a conversion is given to its node, not waited for, and its text, or why
//     the node could not make it, is recorded as it arrives, by its source's hash, whether or not
//     its job still waits (D234 §7, D238 §2). A cancelled extraction tells its node to stop.
//     TestJobs_TheViewListsAReadThroughAndACancelStopsItOnTheNodeSoTheNextPlayAsksAgain
//     (internal/core/api).
//
//   - The text a node made of a track, or why it could not, is kept in persistent.db, not cache.db:
//     making it again reads a whole video (the architecture map's F1, D231 §3).
//     TestSubtitles_ExtractedTextIsKeptInPersistentDBWhichOutlivesCacheDB (internal/core/api).
//
//   - A node's word that FFmpeg stalled on an extraction costs its job no attempt, and two in a
//     row are the track's failure; an extraction or a conversion whose job fails is kept as its
//     track's own failure, by payload hash as FFmpeg's is (D231 §4, §10), and shows it failed
//     until the next play asks again.
//     TestSubtitles_AStallIsTriedAgainAndTwoInARowAreTheTracksFailure,
//     TestSubtitles_AnExtractionThatGaveUpSaysSoAndTheNextPlayAsksAgain (internal/core/api).
//
//   - A subtitle fetched on request is found by the work's ids or the series' and place, and the
//     file's moviehash, with a candidate whose hash matches listed first; is fetched by anyone
//     signed in; is kept gzipped in persistent.db; and is offered as a track of the file, which
//     the video's node converts (D224). Playback makes it the person's choice.
//     TestSubtitleCandidates_AreFoundByTheWorksIDsAndTheFilesHashWithTheDownloadsLeft,
//     TestSubtitleCandidates_ForAnEpisodeAskByTheSeriesAndItsPlace,
//     TestSubtitleCandidates_APersonWhoIsNotAnAdminListsAndFetchesThem,
//     TestFetchSubtitle_KeepsItChoosesItAndItPlaysOnceTheNodeConvertsIt (internal/core/api).
//
//   - A fetched subtitle is converted by the video's node while the person waits, by the conversion
//     job at play priority, the one way a node is asked for work (D238), and kept only once
//     converted: nothing unparsed becomes a choice, one the node cannot read or did not convert
//     within the silence limit is kept nowhere, and a node that is away is found before a download
//     is spent (D224 §5). TestFetchSubtitle_OnlyASubtitleItsNodeConvertedIsKeptAndChosen,
//     TestFetchSubtitle_OneItsNodeDidNotConvertInTimeIsKeptNowhere (internal/core/api).
//
//   - Imports nothing from internal/node, never os/exec, no database/sql: the depguard rules
//     core-boundary and sql-only-in-store, TestArch_CoreNeverImportsNode and
//     TestArch_OnlyStoreImportsDatabaseSQL.
//
// # Read before changing
//
//   - D217 §6: subtitles in M3: which tracks are extracted, and when.
//   - D224: fetching a subtitle on request; the provider client is
//     internal/core/providers/opensubtitles.
//   - D231 §3, §4, §10: derived work, and failures as facts.
//   - D234 §7, D238 §2: work given to a node, and results known by what they are.
//
// # Where to change
//
//   - subtitles.go: the text tracks a video offers, their extraction jobs and their text.
//     textwait.go: a fetch waiting for its conversion's text to be kept. record.go: how a node's text
//     is kept. fetch.go: subtitles fetched on request, and the job that converts one.
//     service.go: what the service is built from.
//   - fetchonce.go: one fetch of a subtitle for a file at a time (D244 §4).
package subtitles
