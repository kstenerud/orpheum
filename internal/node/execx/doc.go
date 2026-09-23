// SPDX-License-Identifier: GPL-3.0-or-later

// Package execx is the only place a process starts. Every FFmpeg or ffprobe run re-executes
// orpheum as a shim, which applies confinement (internal/node/confine) and then execs the bundled
// FFmpeg with hardened arguments and an explicit environment. A failure carries the tail of the
// tool's stderr.
//
// # Invariants
//
//   - It is the only package that imports os/exec or calls exec.Command. Enforced by the depguard
//     rule exec-only-in-execx, the forbidigo rule tagged [exec], and
//     TestArch_OnlyExecxImportsOsExec.
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary and
//     TestArch_NodeNeverImportsCore.
//   - Errors from the process are wrapped where they enter. Enforced by wrapcheck, which is scoped
//     to this package.
//
// # Read before changing
//
//   - D26: no FFmpeg in the core; confine it.
//   - D43: confinement is per invocation.
//   - D112: the node probes its confinement and reports it.
//   - D190 §4: the fake ffmpeg fixture asserts the arguments and environment.
//   - docs/contributors/design/plans/cut-a.md §7.
//
// # Where to change
//
//   - The shim entry point and the argument hardening; the stderr tail on failure.
package execx
