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
//   - Seeding writes every missing row and never overwrites one. Enforced by
//     TestSeed_WritesEveryStartingValueAndNeverOverwrites.
//   - A missing row is an error naming the key, its starting value and the fix; there is no
//     fallback. Enforced by TestSeed_RefillsARemovedSettingAndTheErrorSaysHowToFixIt.
//   - A change is checked against the registry's kind and bounds, and against what the settings
//     page shows, and applied all or none. Enforced by TestSet_RefusesAndChangesNothing.
//   - Modified means different from the starting value; reset writes it back. Enforced by
//     TestList_SaysWhatWasChangedAndShowsOnlyExposedOnTheSettingsPage and
//     TestReset_WritesTheStartingValueBack.
//
// # Read before changing
//
//   - D200: every opinion is a setting row seeded with its starting value; nothing defaults in code.
//   - D183: an opinion must be visible where it acts (amended by D200).
//   - D184: the defaults audit; update_check is asked, not assumed.
//   - D194 §3: why secrets have their own table.
//   - docs/contributors/design/plans/schema.md.
//
// # Where to change
//
//   - registry.go: every key, its type, bounds, starting value and whether the settings page shows
//     it. A new opinion is a key here; it is seeded on the next start.
//   - settings.go: Seed, List, Int and Bool, Set and Reset. The endpoints (/settings and /config)
//     are in internal/core/api. settingstest seeds for other packages' tests.
package settings
