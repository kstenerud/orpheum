// SPDX-License-Identifier: AGPL-3.0-or-later

// Package readonly closes what was only read — a file opened to read, a directory root opened to
// read under — and lets its error go: closing such a handle cannot lose data. It is the one helper
// for what had grown as copies of `_ = file.Close() //nolint:errcheck // read-only` (AGENTS.md
// rule 11; audit-2026-09-29 §9 E).
//
// A written file's Close is where a lost write is reported, so it is never closed here: its error
// is returned, joined to the function's own with errs.WrapIf. That is why this is a helper named
// for the case rather than an errcheck exclusion for Close: an exclusion cannot tell a file that
// was read from one that was written, and would silence both.
//
// An HTTP response body and database rows are the exceptions, and keep their direct
// `_ = response.Body.Close()`: bodyclose and sqlclosecheck, which check that every one is closed,
// see only a direct call, and would report each one handed to this helper as never closed.
//
// # Invariants
//
//   - Close closes what it is given, once, and never reports its error.
//     TestClose_ClosesAndLetsTheErrorGo.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - AGENTS.md rule 11: one way to do each job.
//   - internal/platform/errs, for a Close whose error matters.
//
// # Where to change
//
//   - readonly.go.
package readonly
