// SPDX-License-Identifier: GPL-3.0-or-later

// Package decide is the playback decision: given a file's streams, the device's playable formats
// (its prioritised list per axis of the menu), and nothing of where it is (D247 §11), it answers
// direct play, or a prepared rendition and its profile, with the transcode reasons.
// Whether that rendition already exists is the caller's. It is a pure function of declared
// capabilities.
//
// # Invariants
//
//   - A gain levels a sound towards loudness_target_lufs (-23 to start) both ways, never lifting
//     its true peak above -1 dBTP nor raising it more than loudness_max_raise_db (20 to start),
//     silence left as it is; a file played as it is levels its main track, as it is past two
//     channels, else as stereo; a prepared copy levels each mix as its track's stereo, and says the
//     gain its making put in (D226 §3, §4; levels.go). Enforced by
//     TestLoudness_TheGainGoesTowardsTheLevelWithinItsLimits,
//     TestLoudness_TheGainFollowsTheLevelAndTheRaiseTheSettingsSay and
//     TestLoudness_APreparedFilmIsLevelledAsItsStereoMixAndItsCopyCarriesTheGain
//     (internal/core/api).
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//
//   - Opens no file and walks no directory; files are internal/core/instancedir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//
//   - Imports no database/sql or SQLite driver. Enforced by the depguard rule sql-only-in-store and
//     TestArch_OnlyStoreImportsDatabaseSQL. It calls no store either: a pure function (review).
//
//   - Cannot see the HTTP request, so it cannot branch on client identity. Enforced by the
//     depguard rule decide-sees-capabilities (no net/http, no internal/core/api).
//
//   - A stream the table cannot place is prepared, never guessed playable; each row of the
//     table has a case. Enforced by TestDecide_SortsEachVideoStreamIntoOneMenuEntry and
//     TestDecide_SortsEachAudioStreamIntoOneMenuEntry.
//
//   - Playable formats without both baselines, or naming something the menu does not have, are
//     refused (D21). Enforced by
//     TestPlayableFormats_RefusesAListWithoutTheBaselinesOrWithAnUnknownName.
//
//   - Direct play is judged on the picture and on the track a player starts with (the default,
//     else the first), never on cover art or a second track. Enforced by
//     TestDecide_JudgesTheTrackAPlayerStartsWith.
//
//   - On stereo output (built-in speakers, headphones), a main sound of more than two channels is always
//     prepared, for dialogue_boost, whether the boost is on or off, so it switches mid-film
//     without a new job; a stereo sound is not (D222). Enforced by
//     TestDecide_TheBoostPreparesOnlyASourceWithACentre.
//
//   - Every switch over a codec, container or reason names every value; a new codec breaks each
//     switch that does not handle it. Enforced by exhaustive (default-signifies-exhaustive:
//     false).
//
// # Read before changing
//
//   - D2: the client decides, from a manifest.
//   - D21: a fixed, named output menu, negotiated by a prioritised client list.
//   - D217 §1: the menu's wire form, the container list, and the table that sorts streams.
//   - D22: video, audio and subtitles negotiate as independent axes.
//   - D103: a device capability is never stored.
//   - D193 §2: prepare is the one path besides direct play.
//   - D222: who folds a surround sound down to stereo, and when that prepares.
//   - docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - menu.go: the menu's names, and PlayableFormats, a client's list of them. classify.go: the
//     table from a file's streams to those names. decide.go: the decision, its reasons and its
//     profile.
package decide
