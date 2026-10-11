// SPDX-License-Identifier: AGPL-3.0-or-later

// Package namedfile opens the file a command names, by the root it is under, its path there and
// its payload hash, once it is checked to still be that file (D100): the one way the node does
// it, for a prepare, a loudness measurement, a subtitle extraction or conversion, and the byte
// socket, where three copies had grown, one without the regular-file check (AGENTS.md rule 11).
// OpenNamed also finds the root by its path and reads the payload hash's hex, which the agent and
// the byte socket had each written.
//
// # Invariants
//
//   - Only a regular file is opened: a directory, device or pipe at the path is refused
//     (ErrNotRegular) before anything is read. TestOpen_RefusesWhatIsNotARegularFile.
//   - A file whose payload hash is not the one named is refused (ErrChanged), even after its hash
//     was remembered: a file whose inode, size, modification time or ctime changed is hashed
//     again, so a same-size copy that keeps the mtime is caught.
//     TestOpen_RefusesAFileThatIsNoLongerTheOneNamed,
//     TestOpen_RefusesASameSizeReplacementThatKeptItsModificationTime.
//   - A file named under a root the node was not given is refused (ErrNoRoot, D107), as is a name
//     with no path or a payload hash that is not hex (ErrBadName).
//     TestOpenNamed_FindsTheRootByItsPathAndRefusesABadName.
//   - Opens media only through the read-only confine.MediaRoot. Enforced by the forbidigo rule
//     tagged [media-root].
//   - Imports nothing from internal/core. Enforced by the depguard rule node-boundary and
//     TestArch_NodeNeverImportsCore.
//
// # Read before changing
//
//   - D100: a command names its target by identity, not by path.
//   - D179: identity is a hash of the media payload.
//
// # Where to change
//
//   - namedfile.go; inode_<os>.go for the inode and ctime.
package namedfile
