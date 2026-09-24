// SPDX-License-Identifier: GPL-3.0-or-later

// Package confine applies per-invocation confinement in the shim: argument hardening, a seccomp
// denylist, and Landlock where the kernel has it, then reports per mechanism what it got,
// translating a Landlock errno into a remedy. On macOS and Windows it compiles to a no-op that
// reports unconfined. It also owns MediaRoot, the read-only wrapper over os.Root through which the
// node opens media.
//
// # Invariants
//
//   - Only this package opens a media root, and MediaRoot exposes no method that writes, renames
//     or removes. Enforced by the forbidigo rules tagged [media-root] and [delete].
//   - MediaRoot reads through os.Root, so nothing it opens is outside the root; following a
//     link into another root is the scanner's decision, made with Resolve and Contains (D206).
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary and
//     TestArch_NodeNeverImportsCore.
//
// # Read before changing
//
//   - D26: confine FFmpeg with Landlock and seccomp.
//   - D112: the escape test asserts what is in force.
//   - D190 §3: the escape test first sees the unconfined read succeed.
//   - D193 §4: the confinement ranking.
//   - principles §2: the user's files are not ours to touch.
//   - docs/contributors/design/plans/cut-a.md §7.
//
// # Where to change
//
//   - mediaroot.go: MediaRoot. The seccomp denylist, the Landlock ruleset and the confinement
//     report come in M3.
package confine
