// SPDX-License-Identifier: AGPL-3.0-or-later

// Package auth is who may do what: local accounts, argon2id password hashes, sessions, issued secrets,
// and the single grant table from which every credential (session, issued secret, join, cast) is an
// opaque, revocable, hashed row. A person has one role, their whole access (D130, D243 §4): the
// built-in admin and user, and those the admin makes. A Watch Together guest is not a principal.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//
//   - Opens no file and walks no directory; files are internal/core/instancedir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//
//   - A person's name, and a device's name and platform, pass through the one display sanitiser
//     (textindex.Line) where they enter, and a sign-in reads the name it is given the same way
//     (D211 §2). Enforced by TestNames_APersonsNameIsCleanedWhereItEntersAndSignsInAsTyped and
//     TestNames_ADevicesNameAndPlatformAreCleanedWhereTheyEnter.
//
//   - A password is stored only as an argon2id PHC string, with RFC 9106's second parameters.
//     Enforced by TestHashPassword_IsAPHCStringThatVerifies and TestCreateUser_StoresOnlyTheHash.
//
//   - A sign-in under older costs re-hashes the password. Enforced by
//     TestSignIn_RehashesAPasswordMadeUnderOlderCosts.
//
//   - Failed sign-ins are limited for a person on one site, for a site (an IPv4 address or an IPv6
//     /48), and past a looser limit for a person everywhere are slowed rather than refused, a device
//     that proves an earlier sign-in not slowed at all, so no stranger locks a person out by name
//     (the third audit's A13); attempts made at once each count; a failure does not say which part
//     was wrong; at most hashesAtOnce password hashes run at once. Enforced by
//     TestSignIn_RateLimitsRepeatedFailuresBySetting, TestSignIn_ASitesNetworksCountAsOne,
//     TestSignIn_AKnownDeviceIsNotSlowedByTheAccountsFailuresElsewhere,
//     TestSignIn_GuessesMadeAtOnceEachCountAgainstTheLimit,
//     TestSignIn_SaysNothingAboutWhichPartWasWrong and
//     TestHashSlots_RunAtMostSoManyAtOnceAndRefuseOneThatWaitsTooLong.
//
//   - A session is stored only as its token's SHA-256 and ends when revoked, and belongs to the
//     device it was signed in on: the one the client remembers, or a new one the client names
//     (D217 §8, D221). Enforced by TestSignIn_IssuesASessionUntilSignedOut.
//
//   - Authenticating writes nothing: a session's use is remembered and written in a batch by
//     FlushSessionUses, which never moves a stored use back; the idle check and a person's devices
//     read the later of the stored and the remembered use (D303 §4, DL10). Its reads are one read
//     transaction (M2). Enforced by TestAuthenticate_WritesNothingAndTheFlushWritesTheUse,
//     TestFlushSessionUses_NeverMovesAStoredUseBack and
//     TestDevices_ADeviceWasLastUsedWhenItsLatestSessionWasIssuedOrUsed.
//
//   - A session a device code activated carries the device scope: the role without administering
//     and without changing the account's security (CanManageAccount); a password sign-in carries the
//     whole role, wherever it comes from (D292 §4, D294 §4; the security audit's A11). Enforced by
//     TestSession_ByACodeCarriesNoAdminAndNoAccountSecurityAndByAPasswordTheWholeRole.
//
//   - A principal names its session by that digest's hex (SessionKey), so no other package hashes
//     a credential to tell sessions apart. Enforced by
//     TestAuthenticate_NamesTheSessionWithoutItsCredential.
//
//   - Only the setup code creates the first admin, and only once. It is twelve safe32 characters,
//     written and read as a device code is (D223 §2). Enforced by
//     TestSetup_TheCodeFromTheLogCreatesTheFirstAdminOnce and
//     TestSetup_ACodeAnEarlierReleaseStoredInAnotherAlphabetIsReplaced.
//
//   - An account is set up in full by the admin, or made from an invitation: 256 bits, for
//     registration_invitation_life_days (seven to start), once, under a role and with the admin's
//     note, which the account keeps; its person fills in their own name and password, and a name
//     already taken spends nothing. A withdrawn invitation, or one into a role since deleted,
//     admits no one (D246 §2). Enforced by
//     TestInvitation_MakesTheAccountItsPersonFillsInOnceAndSignsTheDeviceIn and
//     TestInvitation_AWithdrawnOneAndOneOfADeletedRoleAdmitNoOne.
//
//   - A registration link sets a forgotten password, once, and only the newest works; the old
//     password works until the new one is set (D218 §5). Enforced by
//     TestIssueRegistrationInvitation_LastsAsLongAsTheSettingSays and
//     TestRegistration_ANewLinkWithdrawsTheOldAndTheOldPasswordWorksUntilANewOneIsSet.
//
//   - A device is activated by a code it asks for and shows: eight safe32 characters, the same each
//     time it asks while no one has entered it, for ten minutes; a person signed in enters it, read
//     as safe32 reads it, for themselves or, an admin, for someone else, and both devices then show a
//     check code; only that person's "they match" activates the device, signed in as the person,
//     resuming the device it was before when it is theirs; a wrong code counts against the person's
//     failed sign-ins; pairings are kept in memory, a new one refused, never an old one dropped, past pairingsAtMost
//     waiting, pairingsPerNetworkAtMost not yet entered from one network or pairingsPerSiteAtMost
//     from one site (D288). Enforced
//     by discovery/safe32code's TestDeviceCode_IsSafe32AndReadsWhatAPersonTypes,
//     TestPairing_ADeviceKeepsItsCodeUntilEnteredAndThenShowsThePersonsCheckCode,
//     TestPairing_OnlyTheMatchSignsTheDeviceInAndItsSessionIsTakenOnce,
//     TestPairing_NoMatchTenMinutesOrSomeoneElsesAnswerActivatesNothing,
//     TestPairing_WrongCodesCountAgainstThePersonEnteringThem,
//     TestPairing_ADeviceSignedInBeforeResumesItselfAndANewOneNeedsAName and
//     TestPairing_ANewCodeIsRefusedPastTheCapsAndNoWaitingOneIsPushedOut.
//
//   - A sign-in, or a device activated by its code, resumes the device the client remembers only when
//     it is the same person's and the client proves it was that device, by the device secret its
//     latest sign-in gave (never a session token, the third audit's A8); else it is a new device; a
//     registration invitation is not held back by the failed-sign-in limit and clears the person
//     from it (D221; the second audit's K1).
//     Enforced by TestDevices_ASignInResumesTheDeviceTheClientRemembersWhenItIsTheSamePersons,
//     TestDevices_ADeviceIsResumedOnlyByAClientThatProvesItWasIt,
//     TestPairing_ADeviceSignedInBeforeResumesItselfAndANewOneNeedsAName and
//     TestRegistration_ALinkIsNotHeldBackByTheLimitAndMakingOneClearsThePerson.
//
//   - A session that no longer works says why: ended, lapsed, or none, so a client can tell the
//     person (cut-a §5.2). Enforced by TestAuthenticate_SaysASessionWasEndedRatherThanUnknown.
//
//   - Names are unique without regard to the case of their ASCII letters, and nothing is keyed by
//     them. Enforced by TestRenameUser_KeepsNamesUnique.
//
//   - Changing a password needs the current one and ends every other session of the person's; a
//     new password set by a registration link ends every session (D218 §6 as amended 2026-10-13); a
//     wrong current one is ErrWrongPassword, not a failed sign-in's ErrBadCredentials. Enforced by
//     TestChangePassword_NeedsTheCurrentOne and TestPassword_ANewPasswordEndsTheOtherSessions.
//
//   - A person's devices are listed, and only they, or an admin, change one, by its name or how
//     it makes sound (D222); a removed device's sessions end, and signing out elsewhere ends
//     every session but the asking one and keeps the devices listed (D218 §4, §5). Enforced by
//     TestDevices_AreListedAndARemovedOneIsSignedOutAtItsNextRequest,
//     TestSetAudioOutput_IsKeptOnTheDeviceAndOnlyItsOwnerSetsIt and
//     TestSignOutElsewhere_EndsEverySessionButTheAskingOneAndKeepsTheDevicesListed.
//
//   - An admin removes a person, never themselves (so an admin remains): every credential of
//     theirs ends, their devices leave the list, their own watching, positions, lists, ratings and
//     preferences are deleted (D173 §4.6), and the row stays as a tombstone whose name is free
//     (S40). Enforced by TestRemoveUser_EndsTheirAccessDeletesTheirOwnRowsAndFreesTheName.
//
//   - argon2id takes at most 500 ms on devnas: BenchmarkHashPassword_DefaultParameters, run there
//     (verification queue VQ1).
//
//   - Its opinions — argon2id's costs, the failure limit and window, idle expiry — are settings
//     read when used; a missing one is healed with its starting value, written to the database, not
//     held in code (D200, D202). Enforced by TestSignIn_HealsAMissingSettingWithItsStartingValue.
//
//   - A session unused for the idle expiry ends. Enforced by
//     TestAuthenticate_EndsASessionUnusedForTheIdleExpiry.
//
//   - What a discovery finds the core by is kept as hashes, never as a secret: a link's locate hash and
//     offer key, and a session's locator's hash; a used invitation or registration link becomes its
//     person's own link, revoking the last, and ends when the person is removed or the admin revokes
//     it, which signs no one out (discovery-core.md K7).
//     TestLiveLinks_AreEveryLiveLinksHashAndAUsedInvitationLeadsItsPersonBack,
//     TestRegister_ARegistrationLinkBecomesItsPersonsOwnLinkAndRevokesTheLast,
//     TestRevokeOwnLink_EndsTheLinkAndTellsTheDiscoveryLeavingThePersonSignedIn.
//
//   - A link's secret is kept only sealed in its grant (secrets.Store.SealBound), so its person is
//     shown their own link again; it never rests as itself, and a link that no longer opens is
//     shown as none. TestOwnLink_IsShownBackFromItsSealedSecretAndNeverKeptPlain.
//
//   - Each session has one device locator, a fresh one on each visit, which ends with the session;
//     a visit renews the person's own link. TestLiveLocators_EachSessionHasOneFreshOnEachVisitEndingWithTheSession,
//     TestFreshLocator_RenewsThePersonsOwnLink.
//
//   - An offer's key is its live session's hash, by its locator, or its live link's offer key; an
//     ended one has none. TestOfferKey_IsALiveSessionsHashByItsLocatorOrALiveLinksKeyByItsHash.
//
// # Read before changing
//
//   - D217 §8: a device is created at sign-in.
//
//   - D125: access is role-based with two roles; a Watch Together guest is not a principal.
//
//   - D154: one grant table, opaque and revocable.
//
//   - D218: accounts by one-time link; a device by a short code. D221: a device keeps its identity.
//
//   - D168: entitlement is resolved at the point of use, never pushed.
//
//   - D194 §2: argon2id with RFC 9106's second parameters; re-hash on login; rate-limit failures.
//
//   - docs/contributors/design/plans/auth.md.
//
//   - A principal's reach, the libraries it may browse and play, is its role's, read as each request
//     is authenticated, so a library given or taken back counts from the next request; a role that
//     administers reaches every library, and a reach never read reaches none (D243 §4).
//     TestReach_FailsClosedAndIsWhatTheRoleLists,
//     TestAuthenticate_ReadsTheRolesLibrariesAtEachRequest.
//
//   - A built-in role keeps its name and capabilities and is never deleted; a role's capabilities
//     come from the closed set (Capabilities); a role someone holds is not deleted.
//     TestRoles_RefuseWhatARoleCannotBe, TestRoles_AnInviteeReachesOnlyTheirRolesLibraries.
//
// # Where to change
//
//   - reach.go: a principal's reach. roles.go: the roles, their capabilities and libraries, and a
//     person's role.
//   - password.go: argon2id and PHC strings. policy.go: the settings auth reads. limiter.go: the
//     failure counts.
//   - auth.go: users, sign-in, sessions on the grant table. setup.go: the setup code and the first
//     admin. accounts.go: accounts, registration invitations and changing a password (D218).
//     pairing.go: a device activated by its code and a check code (D288). devices.go: a person's
//     devices. locators.go: links' hashes, own links and device locators (discovery-core.md K7). A code's safe32 is
//     discovery/safe32code's, which a discovery's claim code shares.
package auth
