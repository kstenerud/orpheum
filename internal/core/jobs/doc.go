// SPDX-License-Identifier: GPL-3.0-or-later

// Package jobs is the one work queue: fetches, prepares, scans and backups are rows in cache.db's
// job table. One scheduler picks the highest-priority queued job whose not_before has passed and
// whose runner has a free slot. Node jobs become commands; their progress arrives as job records.
// After a crash, running jobs return to queued and restart from zero.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/datadir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - Every switch over a job kind or state names every value. Enforced by exhaustive.
//   - Time comes from a clock.Clock, so scheduling is testable. Enforced by the forbidigo rule
//     tagged [clock].
//
// # Read before changing
//
//   - D193 §2: scheduling, one encode per node.
//   - D118: the scan is incremental, prioritised and steerable.
//   - D170: provider rate limits and back-off.
//   - docs/contributors/design/plans/schema.md, cache.db: the job table, one queue.
//   - docs/contributors/design/plans/cut-a.md §5.3.
//
// # Where to change
//
//   - The scheduler and its slots; the runners per kind; the job view's queries.
package jobs
