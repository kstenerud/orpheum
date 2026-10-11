// SPDX-License-Identifier: AGPL-3.0-or-later

// Package cloudflare sets and removes the DNS TXT record an ACME DNS-01 challenge asks for, through
// Cloudflare's API with the admin's scoped token (D243 §6): the two calls the server's certificate
// needs, written on the standard library through internal/platform/outbound. The token is the
// dns_provider_token secret, scoped to Zone → DNS → Edit on the name's zone.
//
// # Invariants
//
//   - It is the only package that reveals the DNS provider's token, and only into the request's
//     Authorization header. Enforced by the forbidigo rule tagged [reveal], whose path allowlist
//     holds internal/core/providers.
//   - Every call goes through outbound, and every answer is read to a bound. Enforced by the
//     forbidigo rule tagged [outbound]; TestClient_SetsAndRemovesATXTRecordInTheNamesZone.
//   - Its failure policy (go.md §21): no token, or one refused (401, 403), is ErrTokenRefused, for
//     the admin to fix, and asks no more; 429 is outbound's RateLimitedError, waited out; 5xx, a
//     timeout or no answer is outbound's ErrUnavailable, tried again on the job's back-off; a name
//     no zone the token reaches holds is ErrNoZone; a 200 that is not Cloudflare's shape is refused
//     as malformed. TestClient_SaysWhyItCannotSetARecord.
//
// # Read before changing
//
//   - D243 §6: the certificate by DNS-01 through Cloudflare, the record DNS only, never proxied.
//   - docs/contributors/standards/go.md §21: calls to other hosts.
//
// # Where to change
//
//   - client.go: the client, its two calls, and the zone a name is in.
package cloudflare
