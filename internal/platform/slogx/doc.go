// SPDX-License-Identifier: AGPL-3.0-or-later

// Package slogx builds the process's *slog.Logger. It is the one sanctioned way to log:
// cmd/orpheum calls New once, and every other package takes the logger as a constructor
// parameter. There is no global logger.
//
// Secrets never reach a log as text: secrets.Value implements slog.LogValuer and logs as
// "[redacted]" (D194 §5). Bug reports built from logs pseudonymise rather than redact (D74).
//
// A line a test looks for carries its event under EventKey, a short camelCase name the test matches
// (internal/testutil's LineWith and Log.AwaitEvent) while the message's English is free to change.
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
//   - Every line logged with a request's context carries its ID (WithRequestID) as requestId, through a
//     logger's own attributes and groups; a line logged with none carries none (D347). Enforced by
//     TestNew_ALineLoggedWithARequestsContextCarriesItsID.
//   - A logger built at a *slog.LevelVar logs at the level it holds now, which the core's log_level
//     setting changes while it runs (D347). Enforced by TestNew_ALevelVarChangesTheLevelWhileItRuns.
//   - A loop's lasting failure is said as it begins, then once each FailingSaidEvery with how often
//     it failed, then as it ends; each loop logs it with its own constant message. Enforced by
//     TestFailing_SaysAFailureOnceAMinuteAndItsEnd.
//
// # Read before changing
//
//   - D73: each component reports according to its own visibility.
//   - D74: bug reports pseudonymise rather than redact.
//   - D194: secrets log as "[redacted]"; the canary test checks the logs.
//   - D347: the log's level changes at runtime, and every request's lines carry its ID.
//   - docs/contributors/standards/go.md, Logging.
//
// # Where to change
//
//   - slogx.go: formats and handler options. A new Format needs a case in every switch over
//     Format; exhaustive fails the build until it has one. failing.go: Failing, when a loop's
//     lasting failure is said. request.go: a request's ID, on every line it causes.
package slogx
