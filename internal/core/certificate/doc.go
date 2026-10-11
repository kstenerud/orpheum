// SPDX-License-Identifier: AGPL-3.0-or-later

// Package certificate obtains and renews an HTTPS certificate by ACME DNS-01: the server's public
// name's (D243 §6), its challenge's TXT record set through Cloudflare with the admin's scoped token,
// or the wildcard of the names the core's discovery gives it, the record set through the discovery
// (D273); the certificate kept in the instance's persistent directory, since a CA
// limits how often it issues one for a name; and the current one handed to the HTTPS listener,
// a renewed one served as soon as it is kept.
//
// # Invariants
//
//   - A certificate is obtained by proving the name with a DNS-01 record that is removed again,
//     served at once and kept for the next start.
//     TestObtain_ProvesTheNameByDNSServesTheCertificateAndKeepsIt.
//   - A wildcard is proven by the name under it, as RFC 8555 §7.1.3 says, and each certificate is
//     kept under its own prefix, apart from the others.
//     TestObtain_AWildcardIsProvenByTheNameUnderItAndKeptApart; through a discovery,
//     internal/core/discovery's TestSetTXT_ACertificateForTheCoresNamesIsObtainedThroughTheDiscovery.
//   - A renewal is asked for when there is none, or when it expires within a month, a third of
//     Let's Encrypt's ninety days. TestQueueRenewal_AsksWhenThereIsNoneOrItExpiresWithinAMonth.
//   - A token not set or refused fails the renewal for the admin to fix, and asks the CA nothing.
//     TestHandleRenew_ARefusedTokenFailsForTheAdminToFix.
//   - Every ACME request goes through the outbound client it is given. Review-only: the acme
//     package takes it as its HTTPClient.
//
// # Read before changing
//
//   - D243 §6: the name, DNS-01 through Cloudflare, the record never proxied.
//   - D273, and the discovery spec's §3.7: the wildcard, its record set through the discovery.
//   - RFC 8555: ACME; golang.org/x/crypto/acme speaks it.
//
// # Where to change
//
//   - certificate.go: the service, obtaining, renewing and keeping. certificatetest/: a fake CA.
package certificate
