// SPDX-License-Identifier: AGPL-3.0-or-later

// Package atomics writes a file whole or not at all: the one way to do it (AGENTS.md rule 11), where
// the core's instance directory and the node's prepared directory had each grown their own. A reader
// sees the old file or the new one, never part of either.
//
// # Invariants
//
//   - A write that fails leaves the name as it was, and no temporary file behind.
//     TestWriteStream_AFailedWriteLeavesTheOldFileAndNoTemporary.
//   - WriteFile and WriteStream are durable: the file is synced before it takes the name, and the
//     directory after, so a power cut leaves the old file or the whole new one. Replace is atomic
//     but not durable, for what can be made again (a store's chunks), where a sync per file
//     would slow the making and a crash may lose it. TestWriteFile_ReplacesTheFileWholeWithItsMode,
//     TestReplace_ReplacesTheFileWhole.
//   - A write destroys nothing a plain write would not: it renames only its own temporary over the
//     name it writes, and removes only that temporary. Review; the lint rule that keeps renaming and
//     removing to a few packages lets this one do both for that reason alone (.golangci.yml).
//   - It is public, for the public discovery packages as for the module, and imports nothing of
//     the module's. TestArch_PublicPackagesImportNothingInternal; the depguard rule pkg-is-a-leaf.
//
// # Read before changing
//
//   - D194: the core's persistent files, which a crash must never leave half written.
//
// # Where to change
//
//   - atomics.go.
package atomics
