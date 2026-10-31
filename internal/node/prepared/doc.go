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
//     (<file_id>/<kind>/<id>), and the empty folders they leave; and, inside a store, a file and its
//     record found not to agree (records.go), with a run's raw files. Enforced by
//     TestEvict_RemovesTheLeastRecentlyUsedPastTheirKeptTimeFirstAndNeverOneBeingWritten and
//     TestOpenChecked_DropsAChunkMissingOrDamagedAgainstItsRecord.
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
//   - Each file a store holds but its manifest, its init and every chunk, has its record beside it,
//     <name>.sum, "size=<bytes> crc32c=<8 hex>": written as the file is put, after it, the store's lock
//     held across both, and a check takes the same lock to read the record and open the file, so it
//     never sees the one without the other. The manifest names the records' version (ChunkRecords).
//     CRC-32C, not SHA-256: it guards against accident, a disk's or a person's, not an adversary, who
//     could write the record too; computed in hardware, a 4 MiB chunk's check costs about 1.5–2 ms
//     (BenchmarkOpenChecked_AFourMiBChunk) (D392 §4).
//     TestPutFile_PutsEachFileWithItsRecordAndItIsOpenedOnceItMatches.
//   - A store's file is opened to be served only once its size, from the filesystem, and its CRC-32C,
//     over every byte, are its record's (OpenChecked). One truncated, with a byte changed, whose file is
//     missing or without its record is dropped, file and record, logged at warn with its file id,
//     stream, chunk and what differed, and is as one never made: the byte socket answers 404, and the
//     core has it made again (D392 §4). TestOpenChecked_DropsAChunkMissingOrDamagedAgainstItsRecord.
//   - A store made before its files were recorded, its manifest naming no records' version, cannot be
//     checked: it is dropped whole at its first use, a chunk asked for or a run opening it, never while
//     a run writes it (D392 §4). TestOpenChecked_DropsAStoreMadeBeforeItsFilesWereRecordedAtItsFirstUse.
//   - A full rescan's check of a file's stores (CheckStoresOf) checks every init and chunk of each as a
//     serve does, drops what a serve would, and says how many it checked and each it dropped; no other
//     file's store is touched (D392 §4).
//     TestCheckStoresOf_ChecksEveryFileOfAFilesStoresAndDropsTheMissingAndDamaged.
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
//   - D392 §4: what is kept is checked against what is there; made chunks missing or damaged are
//     dropped.
//   - principles §2: the user's files are not ours to touch; these are ours.
//
// # Where to change
//
//   - shelf.go: the budget, last use, eviction and room.
//   - unfinished.go: the hello's eviction of what never finished.
//   - stores.go: the stores of chunks, their files and their runs' raw files.
//   - records.go: each file's record, and its check before it is served and when a full rescan asks.
//   - sizes.go: the runs' sizes, as the shelf knows them without walking them again.
package prepared
