// SPDX-License-Identifier: AGPL-3.0-or-later

// Package randbytes reads random bytes from the source a service is given, and writes the one
// form a bearer secret takes: the one way to do each, where auth and playback had each grown
// their own (AGENTS.md rule 11). What a caller makes of the bytes otherwise is its own: a device
// or setup code is safe32, a store's grid id is hex.
//
// # Invariants
//
//   - Read gives exactly the bytes asked for, or an error: a short read is a failure, never a
//     shorter secret. TestRead_RefusesAShortRead.
//   - A secret is its bytes in unpadded base64url, so it goes in a URL or a header as it is.
//     TestBase64URLSecret_IsTheBytesInUnpaddedBase64URL.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D154: a session's secret, and a registration invitation's, is 256 random bits.
//
// # Where to change
//
//   - randbytes.go.
package randbytes
