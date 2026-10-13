// SPDX-License-Identifier: AGPL-3.0-or-later

// Package backoff is the one doubling, jittered wait between retries (AGENTS.md rule 11): the job
// queue's retries of a failed job (D231 §10) and the core's calls to its discovery (the discovery
// protocol's §2; C7) both wait by it, so that retries begun together do not stay together.
//
// # Invariants
//
//   - A wait doubles from the first to the most and stays there, never overflowing however many
//     doublings are asked. TestDoubled_DoublesToTheMostAndStaysThere.
//   - Jitter moves a wait by at most its fraction either way and never past the most; a failed
//     random source leaves it unmoved, so a retry still happens. TestJittered_MovesAWaitWithinItsFraction.
//   - This package imports nothing from the module but other platform packages. Enforced by the
//     depguard rule platform-is-a-leaf.
//
// # Read before changing
//
//   - D231 §10: why retries are jittered.
//
// # Where to change
//
//   - backoff.go.
package backoff
