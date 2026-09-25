// SPDX-License-Identifier: GPL-3.0-or-later

// Package artwork is the core's artwork (D212): it fetches the images an item's source names,
// has the node re-encode them into our own WebP, and stores what the node answers under
// content-addressed names in the cache directory, with the item's artwork rows in library.db.
//
// # Invariants
//
//   - The artwork store takes bytes from one place: the node's answer to an artwork command
//     (D212 §8). A download is an Unsanitised value whose bytes only the node command reads.
//     TestService_StoresOnlyTheNodesAnswer.
//   - What the node answers is checked before it is stored: a size that was asked for, in
//     WebP. TestService_StoresOnlyTheNodesAnswer.
//   - An image the node refuses, or the source no longer has, is recorded with no hash, so it is
//     not fetched again and an older image stays shown. TestService_RecordsARefusalAndKeepsTheOlderImage.
//   - A job with no node connected waits without spending an attempt. TestService_WaitsForANode.
//
// # Read before changing
//
//   - D212: the whole design; D99 as D212 §5 narrows it.
//   - principles §22: clean at the boundary.
//   - docs/contributors/design/artwork.md.
//
// # Where to change
//
//   - source.go: the unsanitised download and the fetcher interface; service.go: the queue's
//     jobs, the node command and its answer, and the store.
package artwork
