// SPDX-License-Identifier: AGPL-3.0-or-later

// Package httpgzip reads whether a request's Accept-Encoding takes gzip: the one way the core's
// web client files (internal/core/webclient) and its API answers (internal/core/api, D342) decide
// to send gzip (AGENTS.md rule 11).
//
// # Invariants
//
//   - gzip is taken when named, or by "*", with a weight above zero; a weight of zero refuses it.
//     TestAccepts_TakesGzipByNameOrStarUnlessItsWeightIsZero.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D129: gzip on the client channel. D342: which API answers are gzipped.
//
// # Where to change
//
//   - httpgzip.go.
package httpgzip
