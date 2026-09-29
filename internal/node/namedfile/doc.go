// SPDX-License-Identifier: GPL-3.0-or-later

// Package namedfile opens the file a command names, by the root it is under, its path there and
// its payload hash, once it is checked to still be that file (D100): the one way the node does
// it, for a prepare, a loudness measurement, a subtitle extraction or conversion, and the byte
// socket, where three copies had grown, one without the regular-file check (AGENTS.md rule 11).
//
// # Invariants
//
//   - Only a regular file is opened: a directory, device or pipe at the path is refused
//     (ErrNotRegular) before anything is read. TestOpen_RefusesWhatIsNotARegularFile.
//   - A file whose payload hash is not the one named is refused (ErrChanged), even after its hash
//     was remembered: a file whose size or time changed is hashed again.
//     TestOpen_RefusesAFileThatIsNoLongerTheOneNamed.
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
//   - namedfile.go.
package namedfile
