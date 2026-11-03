// SPDX-License-Identifier: AGPL-3.0-or-later

// Package resourceuse records, once a minute, what the core and each connected node use of their
// machines: CPU time, memory held and cached, and the most held, as internal/platform/selfuse reads
// it, the node's in its status. Beside each minute's figures it keeps what the process was doing: on
// the core's, the playbacks playing; on a node's, the work it runs by kind. The System page shows each
// process's summary over the month kept and its hours of the last day, so that what a server needs
// can be judged from what one used (the owner, 2026-11-03; toolkit.md).
//
// # Invariants
//
//   - A minute's CPU time is counted from the process's previous reading: none on its first, none
//     when its counter went back (it restarted) or its scope changed, and a node with no reading
//     since the last minute has no row. Enforced by
//     TestRecord_CountsEachProcesssCPUTimeSinceItsLastReading and
//     TestRecord_CountsNoCPUTimeAcrossARestartOrANodeThatSaidNothingNew.
//   - A core that cannot read its own use still records the nodes'.
//     TestRecord_RecordsTheNodesWhenTheCoreCannotReadItsOwn.
//   - Rows older than KeptFor are forgotten at each record, so the table stays bounded.
//     TestRecord_ForgetsRowsOlderThanAMonth.
//   - Only an admin reads the figures. TestResourceUse_AnAdminSeesWhatTheCoreAndEachNodeUsed.
//   - Imports nothing from internal/node, never os/exec, and no SQL driver; it calls
//     internal/store. Enforced by the depguard rules core-boundary and sql-only-in-store,
//     TestArch_CoreNeverImportsNode and TestArch_OnlyStoreImportsDatabaseSQL.
//
// # Read before changing
//
//   - toolkit.md, "Core and node record their own CPU and memory use".
//
// # Where to change
//
//   - recorder.go: reading and recording a minute; report.go: the summaries and hours.
package resourceuse
