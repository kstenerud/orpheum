// SPDX-License-Identifier: AGPL-3.0-or-later

// Package selfuse reads what a process uses of its machine: its CPU time and its memory, held and
// cached, and the most it has held. The core and each node read their own once a minute, and the
// core keeps the figures, so that what a server needs can be judged from what one used (the owner,
// 2026-11-03; toolkit.md). It is the one way to ask (AGENTS.md rule 11).
//
// # Invariants
//
//   - A process in a cgroup v2 of its own, a container's or a service's, is measured by its group,
//     so the FFmpeg runs a node starts count as the node's.
//     TestReadFrom_MeasuresTheProcesssCgroupWhenItHasOne.
//   - A process in the machine's root group, which counts every process on it, or with no cgroup
//     v2, is measured alone, from /proc/self. TestReadFrom_MeasuresTheProcessAloneInTheRootGroup.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - toolkit.md, "Core and node record their own CPU and memory use".
//
// # Where to change
//
//   - selfuse.go.
package selfuse
