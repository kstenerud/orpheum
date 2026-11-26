// SPDX-License-Identifier: AGPL-3.0-or-later

// Package tokenbucket is the one token bucket (AGENTS.md rule 11): it holds up to a second's worth
// of tokens, which flow back in at its rate on the caller's clock. A provider keeps to a host's
// request rate with it, a token a request (internal/core/providers), and the hosted relay keeps a
// household to its rate, a token a byte (internal/relay, D558 §1).
//
// # Invariants
//
//   - A bucket starts full, holds at most a second's worth of its rate (and at least one token),
//     and refills at its rate as the clock passes, never beyond. Enforced by
//     TestBucket_StartsFullAndRefillsAtItsRateUpToASecondsWorth.
//   - A take of more than the bucket holds takes nothing. Enforced by
//     TestBucket_TakeIfHeldTakesAllOrNothing.
//   - A new rate keeps the tokens held within its new capacity. Enforced by
//     TestBucket_SetRateKeepsTheTokensWithinTheNewCapacity.
//   - Times come from the caller, so every rate is testable on a fake clock. This package imports
//     nothing from the module. Enforced by the depguard rule platform-is-a-leaf.
//
// # Read before changing
//
//   - D170, D213: provider rate limits. D558 §1: a household's rate on the hosted relay.
//
// # Where to change
//
//   - tokenbucket.go.
package tokenbucket
