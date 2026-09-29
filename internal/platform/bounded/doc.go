// SPDX-License-Identifier: GPL-3.0-or-later

// Package bounded reads at most a bound of bytes and refuses anything larger: the one way to take
// in a body whose size another party decides, a provider's answer or a file beside the media,
// where seven copies had grown (AGENTS.md rule 11). Gzip and Gunzip are the one way to keep such a
// body compressed and read it back, under the same bound.
//
// # Invariants
//
//   - Past the bound is refused with ErrTooLarge, never cut short: a truncated answer can still
//     parse, as something other than what was sent. TestReadAll_RefusesPastTheBoundRatherThanCutIt.
//   - It reads at most one byte past the bound, however much the reader holds.
//     TestReadAll_RefusesPastTheBoundRatherThanCutIt.
//   - A kept body expands only up to its bound: Gunzip refuses past it with ErrTooLarge.
//     TestGunzip_GivesBackWhatGzipCompressedAndRefusesPastTheBound,
//     TestPass_AStoredAnswerThatExpandsPastItsBoundIsRefused.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - AGENTS.md rule 11: one way to do each job.
//
// # Where to change
//
//   - bounded.go; gzip.go: a kept body, compressed.
package bounded
