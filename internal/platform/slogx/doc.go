// SPDX-License-Identifier: GPL-3.0-or-later

// Package slogx builds the process's *slog.Logger. It is the one sanctioned way to log:
// cmd/orpheum calls New once, and every other package takes the logger as a constructor
// parameter. There is no global logger.
//
// Secrets never reach a log as text: secrets.Value implements slog.LogValuer and logs as
// "[redacted]" (D194 §5). Bug reports built from logs pseudonymise rather than redact (D74).
//
// # Invariants
//
//   - No package uses the log package, logrus, zap or zerolog. Enforced by the depguard rule
//     one-way-to-do-it.
//   - No package logs through slog's global functions or reads slog.Default. Enforced by
//     sloglint (no-global: all) and the forbidigo rule tagged [ambient].
//   - Every log call has a constant message and camelCase keys. Enforced by sloglint
//     (static-msg, key-naming-case: camel).
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D73: each component reports according to its own visibility.
//   - D74: bug reports pseudonymise rather than redact.
//   - D194: secrets log as "[redacted]"; the canary test checks the logs.
//   - docs/contributors/standards/go.md, Logging.
//
// # Where to change
//
//   - slogx.go: formats and handler options. A new Format needs a case in every switch over
//     Format; exhaustive fails the build until it has one.
package slogx
