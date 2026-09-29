// SPDX-License-Identifier: GPL-3.0-or-later

// Package confine confines each FFmpeg run (cut-a §7). orpheum re-executes itself as the shim
// (Shim), which sets no_new_privs, restricts itself with Landlock to the run's Access where the
// kernel has it, installs a seccomp denylist, and then becomes the tool. MechanismStates says, without
// confining anything, which mechanisms this machine can put in force, translating a Landlock errno
// into a cause and a remedy (D112). On macOS and Windows the shim is a no-op that reports
// unconfined. It also owns MediaRoot, the read-only wrapper over os.Root through which the node
// opens media.
//
// # Invariants
//
//   - Only this package opens a media root, and MediaRoot exposes no method that writes, renames
//     or removes. Enforced by the forbidigo rules tagged [media-root] and [delete].
//   - MediaRoot reads through os.Root, so nothing it opens is outside the root; following a
//     link into another root is the scanner's decision, made with Resolve and RelativeIfInside (D206).
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary and
//     TestArch_NodeNeverImportsCore.
//   - A confined run reads and writes only what its Access names, beside its tool and the
//     system's libraries, and opens no internet socket; each refusal is first shown to succeed
//     unconfined (D112, D190 §3). Enforced by
//     TestEscape_AConfinedToolReadsAndWritesOnlyWhatItWasGivenAndOpensNoNetwork, and for a
//     dynamically linked tool, as FFmpeg is, by
//     TestEscape_ADynamicallyLinkedToolStartsConfinedAndReadsOnlyWhatItWasGiven. Both fail on a
//     machine without a mechanism unless the job lists it in ORPHEUM_TEST_UNCONTROLLED.
//   - A mechanism the machine offers but the shim could not put in force stops the run
//     (ErrNotConfined), so no run has less than the node reported.
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
//   - mediaroot.go: MediaRoot. spec.go: a run's Access and the shim's entry. landlock_linux.go:
//     the ruleset and what it grants; seccomp_linux.go: the denylist; arch_linux_*.go: each
//     architecture's filter details; shim_linux.go and shim_other.go: MechanismStates and the shim.
package confine
