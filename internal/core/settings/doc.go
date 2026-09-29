// SPDX-License-Identifier: GPL-3.0-or-later

// Package settings is the setting registry and the setting table: every opinionated value the
// server acts on, each a row seeded with its starting value on first start and read from the
// database ever after, with no default in code (D200). A question starts unanswered. Secrets are
// not settings; they live in internal/core/secrets.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/datadir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - No setting holds a secret. Enforced by the canary test in cmd/orpheum, which reads every
//     settings response with every secret set.
//   - Every starting value is a valid value of its setting, or null for a question. Enforced by
//     TestRegistry_EveryStartingValueIsAValidValueOrAQuestion.
//   - Seeding writes every missing row and never overwrites one a person set. Enforced by
//     TestSeed_WritesEveryStartingValueAndNeverOverwrites.
//   - A manual set marks a setting modified, even at its starting value; set-to-default clears the
//     mark; an unmodified row follows a changed starting value (D201). Enforced by
//     TestSet_MarksModifiedEvenAtTheStartingValueAndResetClearsIt and
//     TestSeed_MovesOnlyUnmodifiedSettingsToAChangedStartingValue.
//   - Rows nothing reads are listed, and only they can be deleted, only by a person; a retired key
//     says when and why (D202). Enforced by TestUnused_ListsRowsNothingReadsAndDeletesOnlyThem and
//     TestRetiredKeys_AreNotInUseAndSayWhen.
//   - A missing row is healed: its starting value is written to the database and the heal logged,
//     never held only in memory (D202). Enforced by TestRead_HealsARemovedSettingAndSaysSo.
//   - A change is checked against the registry's kind and bounds, and against what the settings
//     page shows, and applied all or none. Enforced by TestSet_RefusesAndChangesNothing.
//   - Reset writes the starting value back. Enforced by
//     TestList_SaysWhatWasChangedAndShowsOnlyExposedOnTheSettingsPage and
//     TestReset_WritesTheStartingValueBack.
//   - What is derived from a server setting follows a set or reset of it at once, through
//     OnChange, not at some later event (agent-failures A4); a string setting with a pattern
//     refuses what does not match it. TestOnChange_ReactsToASetAndAResetOfItsKeyOnly.
//
// # Read before changing
//
//   - D200: every opinion is a setting row seeded with its starting value; nothing defaults in code.
//   - D201: modified is a person's mark; unmodified rows follow a changed starting value.
//   - D202: missing rows heal; unused rows are listed and deleted by a person, never automatically.
//   - D183: an opinion must be visible where it acts (amended by D200).
//   - D184: the defaults audit; should_check_for_updates is asked, not assumed.
//   - D194 §3: why secrets have their own table.
//   - docs/contributors/design/plans/schema.md.
//
// # Where to change
//
//   - registry.go: every key, its type, bounds, starting value and whether the settings page shows
//     it. A new opinion is a key here; it is seeded on the next start.
//   - settings.go: List, Int and Bool (which heal a missing row), Set and Reset. seed.go: the sweep
//     at every start. unused.go: unused rows, retired keys and deleting. The endpoints (/settings and /config)
//     are in internal/core/api. settingstest seeds for other packages' tests.
package settings
