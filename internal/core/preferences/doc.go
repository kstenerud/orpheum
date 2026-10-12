// SPDX-License-Identifier: AGPL-3.0-or-later

// Package preferences keeps a person's choices, the one place they live (the architecture map's F2
// and F6): the subtitle and audio language last chosen in each work, what was last used in each
// started file, a file's subtitle offset, and the switches that hold in every file, the dialogue
// boost and levelling. A choice is worked
// out against a file's tracks when it plays, never copied between files (D230). Playback lists a
// file's tracks and calls it in the transaction that reads them; the API keeps a person's switches
// through it. A device's sound is a fact about the device, kept on its row by internal/core/auth,
// and no setting is a person's.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/instancedir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - It is the only package that writes the preference table. Enforced by
//     TestArch_EachTableHasOneWritingPackage.
//   - The last subtitle and audio language a person chose in a series, or among a film's copies,
//     is what its unwatched files start with, an episode added later and a file whose tracks changed
//     included; a started file plays again with what was last used in it, which a choice made
//     elsewhere does not change (D230 §2, §4). Enforced by
//     TestSubtitleChoice_TheLastLanguageChosenFollowsTheSeriesUnwatchedEpisodes and
//     TestAudioChoice_TheLastLanguageChosenFollowsTheSeriesUnwatchedEpisodes (internal/core/api).
//   - A choice is a language, not a track: a file plays its track of that language of the same kind,
//     then of the same title; the language is matched exactly first, then by its primary subtag
//     (en-GB finds en, and en finds en-GB); off follows as off; a file with no track in the
//     language plays without subtitles, or its main sound, and that fallback is not kept as a
//     choice (D230 §3, D24). Enforced by TestMatchingTrack_PrefersTheSameKindThenTheSameTitle,
//     TestWorkTrack_MatchesTheLanguageExactlyFirstThenByItsPrimarySubtag and
//     TestSubtitleChoice_AnUnwatchedEpisodeTakesTheSameKindThenTheSameTitle (internal/core/api).
//   - What a started file used is kept by the track and what it is, so a rescan's new stream ids
//     find it again, and one whose track went follows the work again. Enforced by
//     TestSubtitleChoice_IsFoundAgainWhenARescanRewritesTheFilesTracks (internal/core/api).
//   - What a playback used is kept only where the file keeps nothing yet: a choice made in the file
//     stands. Enforced by TestKeepUsed_LeavesAChoiceMadeInTheFile.
//   - The dialogue boost is off until the person turns it on, and levelling on until they turn it
//     off, in every file (D83, D222, D226 §5). Enforced by
//     TestFilePreferences_TheDialogueBoostIsThePersonsInEveryFile and
//     TestLoudness_LevellingIsOnUntilThePersonTurnsItOff (internal/core/api).
//
// # Read before changing
//
//   - D230: the last language chosen in a work follows its unwatched files, worked out at play time.
//   - D24: what a choice is: a language and kind, off distinct from unset, a fallback never kept.
//   - D219 §1: a choice is kept per person and per file (its §2 to §4 are replaced by D230).
//   - D83, D222, D226 §5: the boost and levelling are a person's.
//
// # Where to change
//
//   - preferences.go: what is kept and how a file's starting tracks are worked out from it.
//     keep.go: the writes: a choice, what a started file used, an offset and the switches.
package preferences
