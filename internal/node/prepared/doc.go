// SPDX-License-Identifier: GPL-3.0-or-later

// Package prepared keeps a node's prepared renditions within their budget (D217 §5): the node
// alone deletes in its prepared directory, and this is where it does. A rendition's run
// directory's modification time is its last use: the byte socket sets it each time it serves one
// of its files, so it survives a restart. Eviction removes the least recently used first: those
// past their kept time after their last play before any within it, never one being written, until the
// directory is within its budget and its filesystem within its fullness limit. A prepare that
// would find the disk past that limit with nothing left to evict is refused rather than fill it.
//
// # Invariants
//
//   - Nothing but this package and the planned trash writer (internal/node/trash, not yet
//     written) deletes on the node. Enforced by the forbidigo rules tagged [delete].
//   - It deletes only whole run directories under its own directory, three levels down
//     (<file_id>/<profile>/<run_id>), and the empty folders they leave. Enforced by
//     TestEvict_RemovesTheLeastRecentlyUsedPastTheirKeptTimeFirstAndNeverOneBeingWritten.
//   - A rendition within its kept time is evicted only when the budget or the disk still needs it,
//     after every rendition past its kept time; one being written, never.
//     TestEvict_RemovesTheLeastRecentlyUsedPastTheirKeptTimeFirstAndNeverOneBeingWritten.
//   - One being watched, served within node.prepared_watched_minutes, is never evicted either,
//     whatever the disk says; a prepare the disk then has no room for is refused instead, so a film
//     playing does not break (the owner, 2026-10-06). TestEvict_NeverRemovesOneBeingWatched.
//   - A prepare the disk has no room for is refused, with why.
//     TestMakeRoom_RefusesWhenTheDiskIsPastItsLimitWithNothingToEvict.
//   - Each made rendition has a manifest beside it (nodeproto.ManifestName), so a node holding no
//     database knows what its disk holds after a restart, and its hello lists them by it (D238 §2).
//     TestInventory_ListsTheShelvedRenditionsByTheirManifests.
//   - At each hello the node evicts the renditions it listed that the core does not keep, and every
//     run directory without a manifest, whatever their kept time; never one being written, nor one
//     made after the listing (the owner, 2026-10-02).
//     TestReconcile_EvictsWhatTheCoreDoesNotKeepAndUnfinishedMakingsButNeverOneBeingWrittenOrMadeSince.
//   - A store of chunks (D247 §10), at <file_id>/chunks-<stream>/<grid_id>, is kept in the same
//     budget, with a manifest of its own (nodeproto.StoreManifestName) that the hello does not list
//     and the core's reconciling does not evict; a chunk is put in it whole, and a run's raw files
//     are discarded only inside its own raw directory.
//     TestStore_IsMadeWithItsManifestAndTakesAChunkWhole, TestStore_DiscardsOnlyItsRunsRawFiles,
//     TestReconcile_KeepsAStoreWithItsManifestAndEvictsOneWithout.
//   - The disk keeps its floor of free space, node.prepared_disk_min_free_mib, whatever its percentage says:
//     below it renditions are evicted, and a prepare with nothing left to evict is refused (the
//     owner, 2026-10-02). TestMakeRoom_KeepsTheDisksFloorOfFreeSpace.
//
// # Read before changing
//
//   - D217 §5: a node owns its prepared renditions, their budget, kept time and disk limit.
//   - principles §2: the user's files are not ours to touch; these are ours.
//
// # Where to change
//
//   - shelf.go: the budget, last use, eviction and room.
//   - manifest.go: the manifests, the inventory and the hello's eviction.
//   - stores.go: the stores of chunks, their files and their runs' raw files.
package prepared
