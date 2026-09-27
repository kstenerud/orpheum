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
//   - A session is stored only as its token's SHA-256 and ends when revoked, and each sign-in
//     makes a new device, named by the client, that the session belongs to (D217 §8). Enforced by
//     TestSignIn_IssuesASessionUntilSignedOut.
//   - Only the setup code creates the first admin, and only once. Enforced by
//     TestSetup_TheCodeFromTheLogCreatesTheFirstAdminOnce.
//   - An account is made with no password and a registration link: 256 bits, seven days, once,
//     and only the newest works; the old password works until the new one is set (D218 §1, §5).
//     Enforced by TestRegistration_ALinkSetsAPasswordOnceAndSignsTheDeviceIn and
//     TestRegistration_ANewLinkWithdrawsTheOldAndTheOldPasswordWorksUntilANewOneIsSet.
//   - A device code is eight safe32 characters, read as safe32 reads them, for ten minutes, once;
//     a wrong one counts against the address's failed sign-ins (D218 §3, §4). Enforced by
//     TestDeviceCode_IsSafe32AndReadsWhatAPersonTypes,
//     TestDeviceCode_SignsOneDeviceInOnceWithinTenMinutesAsTyped and
//     TestDeviceCode_WrongCodesCountAgainstTheSignInLimit.
//   - A sign-in resumes the device the client remembers when it is the same person's; a code can
//     be for a listed device, which it signs back in; one code waits per person at a time; a
//     registration link is not held back by the failed-sign-in limit and clears the person from it
//     (D221). Enforced by TestDevices_ASignInResumesTheDeviceTheClientRemembersWhenItIsTheSamePersons,
//     TestDeviceCode_OneWaitsAtATimeAndACodeForAListedDeviceSignsItBackIn and
//     TestRegistration_ALinkIsNotHeldBackByTheLimitAndMakingOneClearsThePerson.
//   - A session that no longer works says why: ended, lapsed, or none, so a client can tell the
//     person (cut-a §5.2). Enforced by TestAuthenticate_SaysASessionWasEndedRatherThanUnknown.
//   - Names are unique without regard to the case of their ASCII letters, and nothing is keyed by
//     them. Enforced by TestRenameUser_KeepsNamesUnique.
//   - Changing a password needs the current one and signs no one out (D218 §6). Enforced by
//     TestChangePassword_NeedsTheCurrentOneAndKeepsOtherSessions.
//   - A person's devices are listed, and only they, or an admin, change one, by its name or how
//     it makes sound (D222); a removed device's sessions end, and signing out elsewhere ends
//     every session but this device's and keeps the devices listed (D218 §4, §5). Enforced by
//     TestDevices_AreListedAndARemovedOneIsSignedOutAtItsNextRequest,
//     TestSetAudioOutput_IsKeptOnTheDeviceAndOnlyItsOwnerSetsIt and
//     TestSignOutElsewhere_EndsEverySessionButThisDevicesAndKeepsThemListed.
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
//   - D217 §8: a device is created at sign-in.
//   - D125: access is role-based with two roles; a Watch Together guest is not a principal.
//   - D154: one grant table, opaque and revocable.
//   - D218: accounts by one-time link; a device by a short code. D221: a device keeps its identity.
//   - D168: entitlement is resolved at the point of use, never pushed.
//   - D194 §2: argon2id with RFC 9106's second parameters; re-hash on login; rate-limit failures.
//   - docs/contributors/design/plans/auth.md.
//
// # Where to change
//
//   - password.go: argon2id and PHC strings. policy.go: the settings auth reads. limiter.go: the
//     failure counts.
//   - auth.go: users, sign-in, sessions on the grant table. setup.go: the setup code and the first
//     admin. accounts.go: accounts, registration links, device codes and changing a password
//     (D218). devices.go: a person's devices. code.go: a device code's safe32.
package auth
