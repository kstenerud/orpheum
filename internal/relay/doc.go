// SPDX-License-Identifier: AGPL-3.0-or-later

// Package relay is the hosted TURN relay (D558), `orpheum relay`: a TURN server on UDP and TCP
// 3478 and TLS on 443 that carries a household's core and its clients to each other when no direct
// path holds, and nothing else. It admits the credentials the paid discovery makes
// (discovery/relaycred), relays only between a household's two ends, at the household's rate, and
// records what each allocation carried, never what it was.
//
// # Invariants
//
//   - A household's client and core reach each other both ways, over UDP, TCP and TLS. Enforced by
//     TestRelay_CarriesAHouseholdsClientAndCoreBothWays and
//     TestRelay_CarriesAnAllocationMadeOverTCPAndTLS.
//   - A datagram leaves a relayed socket only for the relayed address of a live allocation of the
//     same household and the other role, and enters one only from such an allocation; anything else
//     is dropped and counted (D558 §4). Enforced by
//     TestRelay_ReachesNoOtherHouseholdNoOtherClientAndNothingNotAnAllocation.
//   - A permission is granted for any address, and opens nothing: a browser asks one for each of
//     the core's candidates, and Firefox drops its relayed candidates when one is refused (D560).
//     Enforced by TestRelay_GrantsAPermissionForAnyAddressAndCarriesNothingThere.
//   - An allocation whose owner has sent nothing through it for a minute is ended, so a page that
//     has gone and a browser's spares do not count against their household for pion's ten minutes
//     (D561 §1). Enforced by TestRelay_EndsAnAllocationItsOwnerSendsNothingThroughForAMinute.
//   - An allocation is made only on a credential whole, unexpired and under one of the relay's
//     secrets; its username changed in any part fails. Enforced by
//     TestRelay_AdmitsACredentialUnderAnyOfItsSecretsAndRefusesAnyOther.
//   - The bytes toward a household's clients pass a token bucket at its credential's rate, on the
//     injected clock; what it cannot pass is dropped (D558 §1). Enforced by
//     TestRelay_DropsWhatIsBeyondTheHouseholdsRate.
//   - Past 16 allocations a client credential, 64 a household or 128 a source address, an allocation
//     is refused, with a line naming the limit. Enforced by TestRelay_RefusesPastEachLimit.
//   - Each ended allocation is one line: its household, role, transport, source shortened to its
//     /24 or /48, seconds, bytes each way and its highest ten-second rate toward its client, and no
//     full address and nothing relayed; each household's day is one line at UTC midnight with the
//     day's highest ten-second rate (D558 §11, §14). Enforced by
//     TestRelay_RecordsAnEndedAllocationWithItsSourceShortenedAndItsTenSecondPeak and
//     TestShortened_KeepsAnIPv4sSlash24AndAnIPv6sSlash48.
//   - The TLS certificate is read again when its files change. Enforced by
//     TestRelay_ReadsTheCertificateAgainWhenItsFilesChange.
//   - The relay announces itself to each discovery Config.Announce names (D567 §1): at once and
//     each minute, it makes an allocation on its own public UDP listener under that discovery's
//     secret, for relaycred.SelfTestHousehold, and only when it works POSTs an announcement signed
//     under the same secret; each change of state is logged once, and the self-test's allocation is
//     recorded as no household's and counted against no limit. Enforced by
//     TestAnnounce_TestsItselfAndPostsASignedAnnouncementEachMinute and
//     TestAnnounce_AFailingSelfTestAnnouncesNothing; that a secret no `[[announce]]` names is
//     announced to no one, by cmd/orpheum's TestRelay_AnnouncesOnlyToTheDiscoveriesItsConfigNames.
//   - The relay imports nothing from internal/core: what it shares with the core's voice relay is in
//     internal/platform/turnserver and internal/platform/tokenbucket. Review-only.
//
// # Read before changing
//
//   - D558: the hosted relay, closed to a household's own two ends (§4), its rate (§1), what it
//     records (§11, §14).
//   - docs/contributors/design/plans/hosted-relay.md §1–2: the wire contract and the relay; §6
//     and D567: the announcement.
//
// # Where to change
//
//   - relay.go: the config, the listeners and the server.
//   - secrets.go: which of the relay's secrets a request was made under.
//   - ledger.go: the live allocations, where a datagram may go, the rate, the limits and the records.
//   - certificate.go: the TLS certificate, read again when its files change.
//   - announce.go: the self-test and the announcement to each discovery.
package relay
