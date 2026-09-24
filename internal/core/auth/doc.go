// SPDX-License-Identifier: GPL-3.0-or-later

// Package auth is who may do what: local accounts, argon2id password hashes, sessions, invites,
// and the single grant table from which every credential (session, invite, join, cast) is an
// opaque, revocable, hashed row. There are two roles, admin and user; a Watch Together guest is
// not a principal.
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
//   - A password is stored only as an argon2id PHC string, with RFC 9106's second parameters.
//     Enforced by TestHashPassword_IsAPHCStringThatVerifies and TestCreateUser_StoresOnlyTheHash.
//   - A sign-in under older costs re-hashes the password. Enforced by
//     TestSignIn_RehashesAPasswordMadeUnderOlderCosts.
//   - Failed sign-ins are limited per account and per address, and a failure does not say which
//     part was wrong. Enforced by TestSignIn_RateLimitsRepeatedFailuresBySetting and
//     TestSignIn_SaysNothingAboutWhichPartWasWrong.
//   - A session is stored only as its token's SHA-256 and ends when revoked. Enforced by
//     TestSignIn_IssuesASessionUntilSignedOut.
//   - Only the setup code creates the first admin, and only once. Enforced by
//     TestSetup_TheCodeFromTheLogCreatesTheFirstAdminOnce.
//   - argon2id takes at most 500 ms on devnas: BenchmarkHashPassword_DefaultParameters, run there
//     (verification queue VQ1).
//   - Its opinions — argon2id's costs, the failure limit and window, idle expiry — are settings
//     read when used; a missing one is healed with its starting value, written to the database, not
//     held in code (D200, D202). Enforced by TestSignIn_HealsAMissingSettingWithItsStartingValue.
//   - A session unused for the idle expiry ends. Enforced by
//     TestAuthenticate_EndsASessionUnusedForTheIdleExpiry.
//
// # Read before changing
//
//   - D125: access is role-based with two roles; a Watch Together guest is not a principal.
//   - D154: one grant table, opaque and revocable.
//   - D168: entitlement is resolved at the point of use, never pushed.
//   - D194 §2: argon2id with RFC 9106's second parameters; re-hash on login; rate-limit failures.
//   - docs/contributors/design/plans/auth.md.
//
// # Where to change
//
//   - password.go: argon2id and PHC strings. policy.go: the settings auth reads. limiter.go: the
//     failure counts.
//   - auth.go: users, sign-in, sessions on the grant table. setup.go: the setup code and the first
//     admin. The invite flow of cut-a §5 (Setup and auth) arrives with M4.
package auth
