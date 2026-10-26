// SPDX-License-Identifier: AGPL-3.0-or-later

// Package prepared keeps a node's stores of chunks within their budget (D217 §5, D247 §10): the node
// alone deletes in its prepared directory, and this is where it does. A store's directory's
// modification time is its last use: the byte socket sets it each time it serves one of its files,
// so it survives a restart. Eviction removes the stores by how strongly the core's players want them
// kept, and the least recently used first, never one being written, until the directory is within its
// budget and its filesystem within its fullness limit. A run of chunks that would find the disk past
// that limit with nothing left to evict is refused rather than fill it. The whole-film prepares it
// once kept beside the stores are retired (D344); what one left is evicted at the next hello.
//
// # Invariants
//
//   - Nothing but this package and the planned trash writer (internal/node/trash, not yet
//     written) deletes on the node. Enforced by the forbidigo rules tagged [delete].
//   - It deletes only whole run directories under its own directory, three levels down
//     (<file_id>/<kind>/<id>), and the empty folders they leave. Enforced by
//     TestEvict_RemovesTheLeastRecentlyUsedPastTheirKeptTimeFirstAndNeverOneBeingWritten.
//   - A run within its kept time is evicted only when the budget or the disk still needs it, after
//     every run past its kept time; one being written, never.
//     TestEvict_RemovesTheLeastRecentlyUsedPastTheirKeptTimeFirstAndNeverOneBeingWritten.
//   - A chunk store is evicted by how strongly the core's players want it kept (D309 §8), the tier
//     the core's last poll answer named: one it named none first of all; then maybe, likely and in
//     use, the last after everything else; the least lately used first within a tier. Until the
//     core names any, the stores go by their kept time.
//     TestEvict_RemovesChunkStoresByTheirPlayersTierFirst.
//   - One being watched, served within node.prepared_watched_minutes, is never evicted either,
//     whatever the disk says; a run the disk then has no room for is refused instead, so a film
//     playing does not break (the owner, 2026-10-06). TestEvict_NeverRemovesOneBeingWatched.
//   - A run the disk has no room for is refused, with why.
//     TestMakeRoom_RefusesWhenTheDiskIsPastItsLimitWithNothingToEvict.
//   - A store of chunks, at <file_id>/chunks-<stream>/<grid_id>, has a manifest of its own
//     (nodeproto.StoreManifestName), so a node holding no database knows what its disk holds after a
//     restart (D238 §2); a chunk is put in it whole, and a run's raw files are discarded only inside
//     its own raw directory. TestStore_IsMadeWithItsManifestAndTakesAChunkWhole,
//     TestStore_DiscardsOnlyItsRunsRawFiles.
//   - At each hello the node evicts every run directory that is not a store with its manifest, a
//     making that never finished or what a whole-film prepare left, whatever its kept time; never
//     one being written (the owner, 2026-10-02; D344 §1).
//     TestEvictUnfinished_EvictsAllButTheStoresWithTheirManifestsAndOnesBeingWritten.
//   - The disk keeps its floor of free space, node.prepared_disk_min_free_mib, whatever its percentage says:
//     below it runs are evicted, and a run of chunks with nothing left to evict is refused (the
//     owner, 2026-10-02). TestMakeRoom_KeepsTheDisksFloorOfFreeSpace.
//   - A run's size is walked once, when the shelf first finds it, never at every run start: a store's
//     moves as the shelf puts files in it, and its open runs' raw directories count as they are; one
//     evicted is forgotten (audit 2026-10-25 N1). TestEvict_WalksARunOnceAndForgetsItWhenEvicted,
//     TestStore_ItsSizeFollowsWhatIsPutInItAndItsOpenRunsRawFiles.
//
// # Read before changing
//
//   - D217 §5: a node owns its prepared directory, its budget, kept time and disk limit.
//   - D344: the whole-film prepare is retired; the directory holds stores of chunks.
//   - principles §2: the user's files are not ours to touch; these are ours.
//
// # Where to change
//
//   - shelf.go: the budget, last use, eviction and room.
//   - unfinished.go: the hello's eviction of what never finished.
//   - stores.go: the stores of chunks, their files and their runs' raw files.
//   - sizes.go: the runs' sizes, as the shelf knows them without walking them again.
package prepared
