// SPDX-License-Identifier: AGPL-3.0-or-later

// Package hash computes a file's identity: a SHA-256 over the first and last megabyte of its
// media payload and the payload's length (D179), not over the whole file's bytes, so a metadata
// edit keeps a file's identity and a remux gets a new one. The payload is Matroska's clusters
// or MP4's mdat; any other file, or one that cannot be followed, is hashed whole (D116). It
// reads through an io.ReaderAt, so the caller decides how the file is opened.
//
// Measured 2026-09-24 with the pinned FFmpeg and MKVToolNix 102.0: every in-place mkvpropedit
// edit of D179's table kept a mkvmerge-authored file's identity, and an ffmpeg -c copy remux
// changed it. For MP4, a retag through ffmpeg -c copy and a +faststart move of moov both kept
// it: ffmpeg wrote mdat byte for byte, so the payload was unchanged.
//
// # Invariants
//
//   - Imports nothing from internal/core and never touches the database. Enforced by the depguard
//     rule node-boundary and, transitively, TestArch_NodeNeverImportsCore.
//   - Starts no process except through internal/node/execx. Enforced by the depguard rule exec-
//     only-in-execx, the forbidigo rules tagged [exec] and [execve], and
//     TestArch_OnlyExecxImportsOsExec.
//   - Removes, renames and truncates nothing. Enforced by the forbidigo rule tagged [delete].
//   - A tag edit keeps the identity: a same-length or relocated title, appended tags or
//     attachments. TestPayloadIdentity_TagEditsKeepTheIdentity; for MP4, TestPayloadIdentity_MP4IsAnchoredOnMdat.
//   - A remux, or a changed first or last megabyte of payload, is a new identity.
//     TestPayloadIdentity_ARemuxOrAChangedPayloadIsANewIdentity.
//   - Finding the last cluster through the Cues gives the same answer as walking every cluster.
//     TestPayloadIdentity_WithoutCuesTheWalkFindsTheSameSpan.
//   - Any file has an identity: a truncated or unfollowable container is hashed whole, never
//     refused, and no input panics. TestPayloadIdentity_AFileItCannotFollowIsHashedWhole, FuzzPayload.
//   - OpenSubtitles' moviehash is the file's size plus the words of its first and last 64 KiB,
//     written from the provider's description (D224 §3).
//     TestOpenSubtitlesHash_IsTheSizePlusTheWordsOfBothEndsAsSixteenHexDigits.
//
// # Read before changing
//
//   - D116: the content hash is a sampling hash.
//   - D179: identity is a hash of the media payload.
//   - docs/contributors/design/plans/scanner.md.
//
// # Where to change
//
//   - identity.go: the scheme and its version (Scheme); a change to what is read is a new one.
//   - matroska.go and mp4.go: finding the payload. opensubtitles.go: the provider's moviehash.
package hash
