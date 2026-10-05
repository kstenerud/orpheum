// SPDX-License-Identifier: GPL-3.0-or-later

// Package certificate obtains and renews the HTTPS certificate of the server's public name by ACME
// DNS-01 (D243 §6): the challenge's TXT record set through the DNS provider, Cloudflare, with the
// admin's scoped token; the certificate kept in the instance's persistent directory, since a CA
// limits how often it issues one for a name; and the current one handed to the HTTPS listener,
// a renewed one served as soon as it is kept.
//
// # Invariants
//
//   - A certificate is obtained by proving the name with a DNS-01 record that is removed again,
//     served at once and kept for the next start.
//     TestObtain_ProvesTheNameByDNSServesTheCertificateAndKeepsIt.
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
//   - RFC 8555: ACME; golang.org/x/crypto/acme speaks it.
//
// # Where to change
//
//   - certificate.go: the service, obtaining, renewing and keeping. certificatetest/: a fake CA.
package certificate
