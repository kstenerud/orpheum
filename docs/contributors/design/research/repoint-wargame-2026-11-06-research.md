# Wargaming the repoint plan (2026-11-06)

> The owner asked for "more wargaming beyond this to look for holes in our plan"
> ([repoint-library.md](../plans/repoint-library.md), "The owner's answers (2026-11-06)"). This
> document runs the owner's three scenarios and eighteen more against the code as it is (main
> "docs(plans): the owner's answers on repointing, and what cache.db's paths are") and against the plan as answered: D429's one folder-level operation, a typed path, a
> privilege of its own, and moves detected and then confirmed by the admin. It also weighs the
> owner's later question, "an admin mounts a new fs somewhere and then wants to use it. How do we
> decide what's a sane mount point and what's not?", which the owner then settled (2026-11-06, through
> the lead): a repoint's new folder may be anywhere beneath any root of the target node (§5). It
> proposes and decides nothing else (AGENTS.md rule 1). Statements about filesystems that come from general knowledge, not from this code or a
> test run here, are marked **(general, not tested here)**. Nothing was run.

## 1. Summary for the owner

### The holes, ranked by harm

| # | Hole | Harm | Likely | Section |
| --- | --- | --- | --- | --- |
| 1 | The repoint's reach: one confirmation rewrites every library under a folder, across nodes. A wrong or malicious X→Y (two folders swapped, a node's whole tree pointed at a backup, a move onto a node that holds other files) scrambles many libraries at once, and nothing records the operation so it can be undone | Every library under X shows the wrong files or none; repair is a second repoint, by hand, from memory | Low by accident with a dry run, real under a stolen admin session | §4.15, §4.16, §5 |
| 2 | Scans act before the admin does: a watched folder is rescanned within the watcher's quiet spell, and a removal under half of a root's files is not withheld, so a moved subfolder's places are **tombstoned** minutes after the move, before any repoint or detection | The films drop out of every list (gone, not "unavailable"); getting them back costs a read of every file; across nodes they come back as new `file` rows and their positions, fetched subtitles and file preferences are purged after `removed_files_kept_days` (90) | High for scenario 2 | §3.2, §4.1 |
| 3 | Today's `RepointRoot` moves **no files** when another library covers the old folder (movies2 inside movies1): it moves the root first, then moves only places no library still covers | The library-page repoint is wrong for nested libraries now; a shortcut that keeps calling it stays wrong | Certain in the owner's layout | §3.2 |
| 4 | Confirming a move re-reads everything: `RepointFile` sets `is_unconfirmed`, and an unconfirmed place is never "stored as listed", so every moved file is probed and hashed again | About 2 MB of reads plus a probe per file: 500,000 files is about 1 TB of reads and 500,000 FFmpeg probes, where D429 §3 asks for device and inode on the same disk and size, time and a sample of hashes elsewhere | Certain | §3.3, §4.17 |
| 5 | Scenario 1 is a failover, not a move. With no fallback (question 4) it is two repoints (to the backup and back), each a full re-read, and a file the backup lacks or holds re-encoded loses its file-level state | State lost for real files; a day of disk work per round trip | Certain when it happens | §3.1 |
| 6 | Files not found at Y "show missing" (question 3), but the first complete scan of Y tombstones them unless more than half go, which starts the 90-day purge | Silent loss of positions and fetched subtitles 90 days after a partial move | High for partial moves | §4.1 |
| 7 | A move into another library changes **who can see** the films: roles reach libraries (D243), so a folder moved from a library a child's role reaches into one it does not, or the reverse, changes access | A privacy or parental-control change nobody chose | Medium | §4.5 |
| 8 | Device numbers are not stable on ZFS, btrfs and network mounts across a reboot or remount **(general, not tested here)**, and the scanner's same-disk match (D304 §5) keys on device and inode | The cheap confirmation silently degrades to a full read; detection keyed on the device misses moves | Medium | §4.11 |
| 9 | The node opens each configured root once at start and keeps the handle (`openRootsThrough`, `os.OpenRoot`): a renamed root keeps being read at its new place under its old name until the node restarts, and a filesystem mounted on a root's path after the start is not seen **(the second not verified)** | What the node reports is not what is on disk; detection reasons from stale facts | Medium | §3.3, §4.9 |
| 10 | Detection can propose a wrong match: season folders, two near-identical trees, a copy that is not a move | A confirmed wrong repoint rewrites many places at once | Low with the guards proposed, high without | §4.12 |
| 11 | There is no "manage libraries" capability to stand beside: everything a library does is `capabilityAdmin` (one `administer` capability in D130's closed set), and an admin edits roles | A new privilege an admin can grant to itself protects nothing | Certain (design gap) | §4.16 |

Smaller holes (copy versus move, the old disk returning, case-only renames, symlinks, moves during a
scan or playback, a machine replaced, scale) are in §4 with what each needs.

### Decisions the owner is asked for (recommendation first)

1. **Settled by the owner (2026-11-06), recorded here, not asked:** a repoint's new folder may be
   anywhere beneath any root of the target node, the roots set on the machine (values.toml
   `[media] roots`, D107 §2), which today's `libraryFolderOn` already checks (§5). It replaces the
   earlier wording ("any path the process can reach, except system-only paths") and should be
   written into the plan or a decision.
2. **Who holds the new privilege, and what it guards** (holes 1 and 11). Its threat is its reach
   (many libraries, cross-node moves, scrambling), not filesystem access.
   - **(Recommended)** A capability only the server's owner holds or grants (the account that set up
     the server), so an admin session alone cannot rewrite many libraries; and every repoint recorded
     (who, when, X, Y, counts) so it can be reversed by the same operation with X and Y swapped.
   - Or a new capability in D130's set that any admin may grant, which is a label more than a
     boundary.
3. **Scenario 1: a backup standing in for a down machine** (hole 5).
   - **(Recommended)** Model it as a **second place** of the same files (D363's copies), so a film
     plays from whichever place answers and nothing moves back when caspar returns. This needs copy
     confirmation by payload hash and size, as D429 §3 has it, not the whole-file SHA-256 that
     `ListCopyCandidates` uses today.
   - Or a repoint there and a repoint back, accepting two confirmations.
4. **Removals that look like a move wait** (holes 2 and 6).
   - **(Recommended)** A scan that would remove every file under some directory while the rest of
     its root reads withholds those removals as "possibly moved" and starts detection; and a file a
     confirmed move did not find stays missing (D241 §5), not tombstoned, until the admin says gone.
   - Or keep today's rule (only more than half and more than twenty waits) and rely on revival.
5. **What confirms a move** (hole 4).
   - **(Recommended)** The listing alone, as D429 §3 says: on one filesystem, inode with size and
     modified time; elsewhere, relative path, size and modified time, with a sample of payload
     hashes. Only what does not match is read again.
   - Or keep `is_unconfirmed` for every moved file, as built.
6. **What detection may look at** (§6).
   - **(Recommended)** Only the declared roots of the node the files were on, by a listing (stat,
     no reads), triggered by the cases in §6.2, never auto-committing, one prompt per moved folder.
   - Or also other nodes (no inode helps there; matching is by name and size only).
7. **What the confirmation shows** (hole 7).
   - **(Recommended)** Per library, how many films it gains and loses, and per role, what it can see
     that it could not, before Move is pressed.
   - Or counts of files only, as question 2's dry run has it.

## 2. What the code does today, in brief

### 2.1 Where a path may be

- **A node's roots are its own configuration** (`[media] roots` / `root_paths`, D107 §2). The node
  opens each at start through `os.OpenRoot` (`cmd/orpheum/node.go` `openRootsThrough`), leaves out
  one that does not open, and names in its hello only the ones that opened
  (`internal/node/agent/agent.go`).
- **Every read goes through `confine.Holding`**: the listing (`agent/listing.go`), the byte server,
  named files and the watcher refuse a path no configured root holds. A library folder is any clean
  path under a configured root (D359 §1), and `libraryFolderOn` refuses anything else in the core.
- **Containers.** The production NAS's node is an LXC that gets `/tank/media/video` read-only at the same path
  through `mp0`, and its roots are everything the container mounts (deploy/release/README.md).
  Staging's is a Docker container with no network and the media read-only (deploy/staging/README.md).
  The generated compose file mounts each root read-only at its own path (`internal/deployment/write.go`
  `mediaOverride`), so the container sees nothing of the media but its roots.
  A filesystem mounted on the host later is invisible to the node until the container's mounts
  change, a host step.

### 2.2 What a scan records and matches

- **Files:** the listing gives every entry's size, modified time, device and inode (`nodeproto.Listed`,
  directories included). The core keeps device and inode on each place (`place.filesystem_device`,
  `place.inode`) and, within one listing of one root, takes a new path with a stored video's device,
  inode, size and time as that video moved, without reading it (`concludeInodeMoves`, D304 §5).
  It never looks outside the root being listed.
- **Directories:** their inodes are listed but not kept. A root's device and whether it is a mount
  point are kept (`root_state`, from `ScanBegin`); its own inode is not.
- **Identity:** the payload hash is a SHA-256 over the first and last megabyte of the media payload
  and its length (D116, D179). A returning payload revives its tombstone **on the same node only**
  (`GetLatestTombstoneByPayloadHash`, `file.sql:112`); across nodes a new `file` row is made
  (expansion audit §2.2).
- **Removals:** a complete scan tombstones what it did not see, except outside regions it could not
  read; it withholds when it saw none of the root's files, when the root's device or mount changed,
  or when it would remove more than half and more than twenty (`withholdReason`, D41, D208 §3).
  A scoped scan from the watcher (D272 §9) removes under the same rules. Tombstoned files are purged
  after `removed_files_kept_days`, default 90, taking positions, fetched subtitles and file
  preferences with them.
- **Unavailable:** a root the hello does not name, a listing that fails, or a disk not answering
  makes places unavailable, never removed (D241 §2, D233 §10).

### 2.3 What `RepointRoot` does

`library.RepointRoot` (`move.go`) moves one library's one root to a folder under the new node's
configured roots, then, in the same transaction, moves the **live** places under the old path **that
no live library still covers** (`ListUncoveredFilesUnder`, run after the root has moved), rewrites
their `identified_from_path`, marks them `is_unconfirmed`, forgets the old root's state when no
library folder is left there, cancels work parked on the old destination and asks for a scan of the
new folder. It is `capabilityAdmin`, like every library operation. Tombstoned places are not moved.

## 3. The owner's three scenarios

Roots `/tank/media` and `/mnt/caspar/video`; libraries movies1 `/tank/media/video/movies`, movies2
`/tank/media/video/movies/disney`, movies3 `/mnt/caspar/video/movies`.

### 3.1 Caspar goes down; `/mnt/balthasar/movies`, a backup of caspar's movies, is mounted

**Facts that decide it.** Caspar is another machine, so `/mnt/caspar/video` is a network mount.
The backup is a copy on another filesystem: new inodes, and new modified times unless the copy
preserved them (`rsync -a` and `cp -a` do, plain `cp` does not) **(general, not tested here)**.
`/mnt/balthasar` is under no configured root, so no scan can see it.

**Today, step by step.**

1. Caspar goes down. Depending on the mount options the share stalls (hard mount), fails reads
   (soft mount), or, if unmounted, leaves `/mnt/caspar` empty so `/mnt/caspar/video` does not exist.
   The status reports the disk not answering, or the next listing fails: movies3's places are
   **unavailable**, nothing is removed (D233 §10, D241 §2). Its films stay listed, marked
   unavailable; a play fails at once with the reason (D241 §4). This part holds.
2. The admin mounts the backup on the host. The node does not see it: in a container it is not
   mounted in; outside one it is under no configured root, so `confine.Holding` refuses it.
3. To use it today the operator must declare `/mnt/balthasar` (or `/mnt/balthasar/movies`) as a root
   on the node (and, on the production NAS, add a mount point to the LXC), restart the node, and then either
   repoint movies3 to it or add it as a second folder of movies3.
   - **Repoint:** live places move, all marked unconfirmed; the scan reads every file again. Files
     whose payload matches are confirmed. Files the backup lacks are not listed, so the complete scan
     tombstones them unless more than half are missing. Files re-encoded in the backup have a new
     payload hash: the old row is tombstoned, a new row made; the item is found again by
     identification, so watched state survives, but file-level state (positions, fetched subtitles,
     file-scope preferences) stays on the tombstone and is purged in 90 days.
   - **Second folder:** the backup's files are new places; a copy joins its original only when the
     original's place is live and the whole-file SHA-256 matches (`ListCopyCandidates`), which reads
     every byte of both, and the originals are unreadable while caspar is down. So in practice the
     backup's files become new rows, duplicates of the unavailable ones.
4. Caspar comes back. With a repoint, nothing covers `/mnt/caspar/video/movies` any more; going back
   is a second repoint and a second full read.

**With the plan (manual repoint and detection).** The repoint can be typed, but the target must be
beneath a root of the node, so the backup must be declared on the machine first (§5). Detection could
propose "movies3 is now at `/mnt/balthasar/movies`" when the new root appears in the hello and its
listing matches movies3's unavailable files by relative path and size (§6). It could confirm by
listing, not by a full read, if the owner takes decision 5.

**What it cannot do.** It cannot see the backup before the host and the node's config expose it. It
cannot tell a temporary stand-in from a permanent move; question 4's "no fallback" makes every
failover a move. It cannot match a re-encoded file by any key the schema has, except by name and
identification. **This scenario is better served as a second place** (decision 3): D363 already lets
one file have several places and plays from a readable one (`PlaceToRead`); what is missing is copy
confirmation by payload hash rather than whole-file SHA-256, and copies joining an original whose
place is unavailable rather than live.

### 3.2 `/tank/media/video/movies/disney` moves to `/tank/media/video/kids-movies/disney`

**Facts that decide it.** If `kids-movies` is on the same filesystem (same ZFS dataset), `mv` is a
`rename(2)`: the directory and every file beneath keep their inodes and modified times **(general,
not tested here)**. If either folder is its own dataset, `mv` copies and deletes (EXDEV): new inodes,
times preserved by GNU `mv` **(general, not tested here)**. The new place is under the configured
root `/tank/media` but under **no library**, so no scan lists it. movies2 is nested in movies1
(D363 lets libraries overlap); one place row serves both.

**Today, step by step.**

1. The move happens. movies1's watcher sees the `disney` folder moved out of its tree (the move-in
   side lands in an unwatched folder) and, after `node.watch_quiet_seconds`, notices the folder
   holding the change, `/tank/media/video/movies`, for a scoped scan. movies2's watcher loses its
   folder.
2. The scoped scan of movies1 lists no Disney files. Unless they are more than half of movies1's
   files and more than twenty, the removal is **not withheld**: the places are tombstoned within
   minutes. They are the same places movies2 lists, so movies2 loses them too. The films leave every
   list; they are not "unavailable", they are gone.
3. movies2's folder no longer exists: its next scan's listing fails, nothing more is removed.
4. The admin repoints movies2 from its library page. `RepointRoot` moves the root to
   `/tank/media/video/kids-movies/disney`, then moves the live places under the old path that no
   library still covers. The places are already tombstoned, so it moves nothing. **Had they still
   been live** (a move made while the node was down, say), it would still move nothing, because
   movies1 covers the old path: `ListUncoveredFilesUnder` runs after the root has moved, so every
   Disney place is "still covered by another library" and stays (hole 3).
5. The scan of the new folder finds the files at new paths. It has no stored files there, so the
   same-disk inode match cannot apply (it only looks within the root's own stored files). Each file
   is read; its payload hash revives its tombstone on this node (`reviveOrInsert`), keeping its
   history. So the history survives on one node, at the cost of a read of every file and a window in
   which the films were gone.

**With the plan.** D429's operation, "folder X is now folder Y" with X the old Disney folder, rewrites
every place under X (live and removed), and movies2's root, which is at X. movies1's root is above X
and stays, so the films leave movies1, which is what happened on disk; the confirmation must say so
(decision 7). Detection (§6) can see this cheaply: the scan that would tombstone a whole directory
withholds it, the node lists the configured root `/tank/media`, and the stored inodes turn up under
`kids-movies/disney` with the same sizes and times. The match is exact and needs no reads.

**What it cannot do.** If the scan tombstones before detection runs, the operation must also revive
the places it finds at Y (decision 4 avoids this). If the move crossed datasets, inodes do not match;
detection falls back to relative path, size and modified time, which still works when times were
preserved. If `kids-movies` is outside every configured root, no detection is possible.

### 3.3 `/tank/media` moves to `/tank/large-files/media`

**Facts that decide it.** `/tank/media` is a **configured root**: the core cannot rename it (D107
§2). If done with `zfs rename tank/media tank/large-files/media`, the dataset and its inodes stay,
and its mountpoint changes; the device number may change on remount **(general, not tested here)**.
If done with `mv` inside one dataset, inodes stay. The node holds an open handle on the root from its
start (`os.OpenRoot`).

**Today, step by step.**

1. The move happens while the node runs. The node's open handle follows the directory, so it may go
   on listing and reading the moved tree under the old name until it restarts **(not verified:
   whether `zfs rename` can unmount a dataset the node holds open without `-f`)**. Plays keep working;
   nothing is noticed.
2. The node restarts. `/tank/media` does not open; the node logs an error and leaves it out of its
   hello. Every library folder under it (movies1, movies2, and any other) becomes **unavailable**
   (D241 §2). Nothing is removed.
3. The operator edits the node's roots to `/tank/large-files/media` and restarts it. The hello names a
   root the core has never seen and lacks one it had.
4. The admin repoints each library root under `/tank/media` one by one (today's operation is per
   library root); each repoint marks its places unconfirmed and costs a full read. movies2 inside
   movies1 again moves nothing if movies1 is repointed after it, or moves only what movies2 does not
   cover, depending on order (hole 3).

**With the plan.** One D429 operation, X `/tank/media`, Y `/tank/large-files/media`, carries every
library root and every place. Detection has a strong, cheap signal: the hello's set of roots changed,
one gone and one new, and a listing of the new root finds the stored inodes with their sizes and
times. It can propose the move the moment the node says hello.

**What it cannot do.** It cannot spare the operator the node's config edit (and on the production NAS, the LXC's
mount point). In a container there is a cheaper route the docs should name: change the host side of
the bind mount and keep the path inside the container, so Orpheum sees no move at all (the root's
device may change, and a scan that would remove nothing completes on a new device:
`TestScenario_ARootOnANewDeviceWithEveryFileCompletesAndKeepsTheNewDevice`).

## 4. More scenarios: the holes

Each says what goes wrong, how likely, what the plan should add, and the cost.

### 4.1 A partial move (half the files moved)

- **Goes wrong:** the dry run reports, say, 55% found; question 2's floor of half lets it through.
  Question 3 rewrites the other 45% to Y, where they are not; the first complete scan of Y does not
  see them and, since 45% is under the half threshold, tombstones them. The purge clock starts.
- **Likely:** high; people move in batches and stop halfway.
- **Add:** a moved place not found at Y stays missing under D241 §5 (the admin's word makes it gone),
  or it stays at X, not rewritten. Either way the scan must not tombstone it on absence alone.
  Prefer splitting: rewrite what was found, leave the rest at X, and let the admin run the move again
  later for the rest.
- **Cost:** a rule in the removal judgement, keyed on a moved place's state; small.

### 4.2 A copy, not a move (both places live)

- **Goes wrong:** detection sees Y's files match X's and proposes a move, but X still reads.
  Confirming it would point the libraries at the copy and leave the original unlisted.
- **Likely:** medium (a backup made inside a configured root).
- **Add:** detection proposes a move only when X is gone or unreadable; when both read, it is a copy
  (D363) and at most a "copies found" note.
- **Cost:** none beyond the check.

### 4.3 A backup with a different file set, or re-encoded files

- **Goes wrong:** the matching sees few matches; re-encoded files never match by payload hash
  (D179 makes a remux a new identity).
- **Likely:** medium for scenario 1.
- **Add:** report three groups: matched, present by name but different (re-encoded), and absent.
  The second group joins its item by identification (watched state survives) but not its file;
  the confirmation says so.
- **Cost:** the dry-run report; small.

### 4.4 The old disk returns after a repoint

- **Goes wrong:** X reads again. If no library covers X, nothing happens (fine). If a parent library
  covers X (movies1 over the old Disney folder), its scan finds the files: by payload hash they match
  the live files at Y, so they become copies or duplicates and the films show in movies1 again.
- **Likely:** medium after scenario 1.
- **Add:** under decision 3 this is the intended state (two places). Under a repoint, the return of X
  should be a detection event too ("the folder you moved from is back: keep both, or move back?").
- **Cost:** a trigger in §6; small.

### 4.5 A moved folder lands inside another library

- **Goes wrong:** the rewrite puts the places under library "kids", out of movies1. Roles reach
  libraries (D243), so who may see the films changes; so does which library's identification
  settings apply.
- **Likely:** medium; reorganising is the reason people move folders.
- **Add:** the dry run lists each library's gains and losses and each role's change of reach
  (decision 7).
- **Cost:** two reads over `library_place` before and after; small.

### 4.6 Two libraries sharing files after a move

- **Goes wrong:** nothing structural: one place, several libraries (D363). But a move that takes a
  folder under a second library's root without leaving the first makes the films listed twice in
  "All libraries"? D363 says each list shows an item once; this holds.
- **Likely:** low harm.
- **Add:** nothing; the per-library report covers it.

### 4.7 A move during a scan

- **Goes wrong:** a running scan of X holds batches of files to examine by path (`scan_batch.files_json`)
  and its listing in memory; its completion would tombstone what moved.
- **Likely:** medium; a large move takes long enough for the periodic scan to start.
- **Add:** as question 6 says, the operation cancels the open scans of every root at or under X (and
  of Y), in its own step before the rewrite, and refuses while a scan's completion is being written.
  A scan that completes after the move but started before it must not judge removals under X.
- **Cost:** a check on the scan's start time against the move's; small.

### 4.8 A move during playback

- **Goes wrong:** a direct play reading through the node (fdproxy, D330) holds an open file and keeps
  playing; a play reading by path, or the next chunk job naming the old path, fails.
- **Likely:** low.
- **Add:** question 6's answer holds; the next request after the rewrite takes the new path. Chunk
  stores are keyed by file id (`chunk_store.source_file`), so nothing made is lost.
- **Cost:** none.

### 4.9 A node down, a folder gone, or a mount that failed

- **Goes wrong:** detection must not treat a mount that failed (the empty mount point lists cleanly)
  as "folder vanished, look for it elsewhere". The core already tells a changed device or mount from
  a deletion (D41, D208 §3, `changedFactsReason`). A node that is down is "not connected", not gone.
  A filesystem mounted on a configured root's path **after** the node started may not be seen at all,
  because the node holds a handle on the directory it opened at start **(not verified)**.
- **Likely:** medium on a NAS whose pools import late.
- **Add:** detection runs only when the root's device and mount facts are as the last complete scan
  had them, and the node connected; a node should reopen a root whose device changed **(needs a
  check of the node's behaviour first)**.
- **Cost:** a guard; the reopening is node work of unknown size.

### 4.10 Case-only renames, NFC and NFD

- **Goes wrong:** `Disney` to `disney` on a case-insensitive SMB share, or a Mac writing NFD names:
  byte prefixes differ while the text looks the same. The relocation audit and the visibility test
  already require two prefixes (raw bytes and NFC text).
- **Likely:** low.
- **Add:** inode matching handles a same-filesystem rename regardless of spelling; name matching in
  detection compares NFC case-folded names but rewrites by the bytes the node reports (D107's
  spelling rule for `library_root.root_path`).
- **Cost:** a test.

### 4.11 Device numbers that change

- **Goes wrong:** Linux gives ZFS datasets, btrfs subvolumes and NFS mounts device numbers that can
  change across a reboot, import or remount **(general, not tested here)**. The same-disk match keys on
  device and inode, so after a reboot it fails and files are read again; detection keyed on device
  misses the move.
- **Likely:** medium.
- **Add:** record a stable filesystem identity with each root and mount (the mount's source and
  filesystem type from `/proc/self/mountinfo`, or `statfs`'s `f_fsid`), and match inodes within a
  filesystem identity rather than a device number.
- **Cost:** a field in `ScanBegin` and `Mount`, a column in `root_state` and `root_mount` (an appended
  step), and the comparison; medium.

### 4.12 Detection proposing a wrong match

- **Goes wrong:** two trees with the same shape (`Season 01` under every series; two editions of a
  collection) match by name and size; a confirmed wrong move rewrites hundreds of places.
- **Likely:** high with name matching alone; low with inodes.
- **Add:** propose only where the match is unique (no second candidate Y with similar coverage), the
  coverage is high, and, without inodes, the sampled payload hashes agree. Show five example paths
  side by side. Never commit without the admin.
- **Cost:** in the matcher; small.

### 4.13 Symlinks and bind mounts

- **Goes wrong:** a user moves a folder and leaves a symlink at the old path. A library folder reached
  through a link is refused (D359 §1); a link inside a root is followed only into a media root, and a
  directory is walked once however many links reach it (D206). Which path the files are listed at is
  then whichever the walk met first **(not verified)**, so the files may flip between X and Y.
  A bind mount shows the same files at two paths with the same inode on one device.
- **Likely:** low.
- **Add:** detection ignores a match whose Y is reached through a link; the dry run says when X is a
  link to Y ("this is the same folder").
- **Cost:** small.

### 4.14 Network shares with unstable inodes

- **Goes wrong:** NFS file ids are usually the server's inodes and stable; SMB mounts make up inode
  numbers unless the server supplies them (`serverino`) **(general, not tested here)**. The watcher
  does not watch a network filesystem (D27), so a move there is seen only by the periodic scan.
- **Likely:** medium for caspar-like shares.
- **Add:** a node reports per root whether inodes can be trusted (filesystem type), and detection uses
  name, size and time there.
- **Cost:** small.

### 4.15 A malicious or mistaken repoint (secrets, and scrambling)

- **Secrets: closed by the settled rule.** Had the target been "any reachable path", a repoint onto
  the instance directory (`secret.key`), `/root` or a secrets mount would have made the node list it
  (names, sizes and times shown in the UI), send `.nfo` bodies beside a video to the core, serve
  `.srt`, `.ass`, `.ssa` and `.vtt` files as subtitles and images as artwork: what D107 §2 was written
  against. Beneath the node's roots, which `mediaOverride` (`internal/deployment/write.go`) mounts
  read-only into the container at their own paths and `confine.MediaRoot` confines every read to, a
  repoint reaches nothing a library folder could not already reach. One residue: the node should
  never serve its own instance directory if an operator declares a root above it **(not verified
  whether it refuses that today)**.
- **Scrambling: the real threat.** One confirmation rewrites every library under X: two folders
  swapped (two operations, or one with the temporary-prefix step), a whole root pointed at a backup,
  a move onto node B where other files already sit. Under a stolen session it is vandalism of the
  whole catalogue in a few clicks; by mistake, a dry run with a mistyped Y.
- **Likely:** low by accident with question 2's dry run and floor; real under a stolen session.
- **Add:** decision 2's privilege; a record of each operation so it can be reversed by the same
  operation; the dry run refuses a Y where fewer than half are found (question 2), shows conflicts
  (§4.21), and marks a cross-node move as one.
- **Cost:** a small table for the record; the rest is in the plan.

### 4.16 The privilege has nothing to stand beside

- **Goes wrong:** the owner's answer separates "manage libraries" from "repoint". There is no such
  split: every library operation, and `RepointRoot`, is `callableBy(capabilityAdmin)`, and roles hold
  capabilities from D130's closed set (`administer`, `delete_media`, `download`, `edit_metadata`,
  `host_watch_together`, `remote_access`). An admin can edit roles, so a new capability an admin can
  grant is a label, not a boundary.
- **Likely:** certain.
- **Add:** decision 2. The settled target rule bounds the privilege's filesystem reach to the node's
  roots; what it guards is "rewrites many libraries at once, across nodes" (§4.15).
- **Cost:** a capability, the check, and the UI that shows it; small to medium.

### 4.17 Scale: 500,000 files

- **Goes wrong:** the rewrite is cheap (252,000 places in about 2.5 s, visibility document §5). The
  confirmation is not: with `is_unconfirmed` on every moved place, a full read (hole 4). Detection's
  listing of a configured root is one `stat` per entry, minutes on spinning disks with cold metadata
  **(estimate, not measured)**.
- **Likely:** certain for the production NAS's size.
- **Add:** decision 5; detection triggered (§6.2), never periodic over everything; a directory-only
  first pass when directory inodes are kept.
- **Cost:** see §6.

### 4.18 What the admin is shown, and when

- **Goes wrong:** one prompt per file, or per library, buries the decision; a prompt raised by a scan
  that then tombstones anyway arrives too late.
- **Add:** one proposal per moved folder (the topmost X; nested moves folded in), raised in the
  System page's Storage area and in needs-attention, while the removals wait.
- **Cost:** the proposal row and its page; medium.

### 4.19 A machine replaced under another node name

- **Goes wrong:** the new machine's node enrols as a new node with no libraries; the old node's
  libraries go unavailable. Question 5's adoption is the answer; the expansion audit's finding 1
  (nodes addressed by name in the API, `RepointInputBody` taking `nodeName`) should be settled first
  so an adopted or renamed node keeps its URLs.
- **Likely:** the development NAS case.
- **Add:** adoption before the folder move; detection does not try to match across nodes.
- **Cost:** as the relocation audit §4 says.

### 4.20 A move to another node without a repoint

- **Goes wrong:** the files turn up on node B under a declared root; B's scan makes new `file` rows
  (revival is per node), and node A's places stay unavailable. File-level state is split.
- **Likely:** low until a second node exists.
- **Add:** the expansion audit's cross-node revival by payload hash (finding 2), and the place-id
  question (finding 3, `MakeFileACopy` at `file.sql:541` joining `file.id = place.id`) decided before
  any code inserts a second place for a file.
- **Cost:** as that audit says.

### 4.21 A target that already holds places

- **Goes wrong:** Y already has live places at the same paths (a half-finished earlier copy that was
  scanned). The rewrite fails on `place_live_path` and the path primary keys (visibility §5).
- **Add:** the dry run finds the conflicts and offers "merge": a place at Y with the same payload
  keeps X's file (its history), and the place at Y's own file goes, as `MakeFileACopy` does today.
- **Cost:** medium.

## 5. The owner's mount-point question, and the signal a changed root list gives

The owner asked: "An admin mounts a new fs somewhere and then wants to use it. How do we decide
what's a sane mount point and what's not?"

**Settled (the owner, 2026-11-06, through the lead): anywhere beneath any root of the target node.**
The roots are values.toml's `[media] roots`. `mediaOverride` (`internal/deployment/write.go`) mounts
each read-only into the node's container at its own path, and `confine.MediaRoot` confines every read
beneath them. Adding or removing a root is: edit values.toml, re-run `build.sh`, `docker compose up -d`,
and on the production NAS an LXC `mp` line. A newly mounted filesystem is used by declaring it as a root; that is
the sane-mount-point test, and it is made on the machine (D107 §2).

**Dropped, one line each:** the node's `/proc/self/mountinfo`, because inside the container it shows
only the declared roots anyway and on a bare machine it is everything (`/`, `/home`, `/var`); a
denylist of system paths, because it can never be complete (`/srv/orpheum`, `/root`, `/run/secrets`,
another app's data) and differs in every container.

**The signal: after a restart, a root vanished and a new one appeared.** Changing the roots recreates
the container, so the node restarts and its next hello names the new set. How the core treats it
today (`internal/core/nodes/enrol.go`):

- **A root that left the hello:** `markRootsNotNamed` takes each live library folder no named root
  holds and calls `Scans.MarkRootNotNamed`, which makes it unavailable from that moment (cause "not
  named"): plays fail at once and say why, nothing is removed, and a hello that names it again asks
  for its scan, which brings it back (D241 §2,
  `TestScans_ARootTheHelloNoLongerNamesIsUnavailableUntilAScanReadsIt`). The node also leaves out of
  its hello a root that does not open at start (`openRootsThrough`), so a renamed root, or one whose
  disk did not mount, looks the same.
- **A root that joined the hello:** nothing. Scans are asked only for the library folders a named root
  holds (`coveredRoots`: a scan of a root no library covers stores nothing), so a new root that no
  library uses is never listed.
- **The core keeps no list of a node's past roots**, only its library folders. Both halves can still
  be derived at the hello: library folders held by no named root are gone; named roots holding no
  library folder are new.

So for scenarios 1 and 3 the core already has both halves of "did X become Y?" at one hello, and does
nothing with the second. Detection (§6.2, trigger 3) would list each new root at that hello (stat
only), compare it with the places of the folders just marked "not named", and raise one proposal.
Because a changed root list always means a restart, a node reporting its mounts in its status would
add nothing over the hello.

## 6. A detection design sketch (proposal, not decided)

### 6.1 What is recorded

- **Already:** each place's device, inode, size and modified time; each root's device and mount
  facts.
- **Added (proposal):** a stable filesystem identity per root and per mount inside one (§4.11); the
  inode of each library folder and of each directory that directly holds a place, kept per node in a
  small table (`directory(node_id, filesystem_id, inode, reported_path_raw)`), written by complete
  scans. That makes "where did this directory go?" a directory-only question.
- **Per proposal:** a row naming node, X, Y, how it was matched, counts and its state (open, accepted,
  declined, lapsed), so the admin's answer is kept and a declined pair is not proposed again.

### 6.2 When it runs

Only on a trigger, never as a sweep:

1. A scan would remove every file under some directory while the rest of its root reads: those
   removals wait as "possibly moved" (decision 4), and detection runs.
2. A library folder's listing fails because the folder is gone while its configured root reads.
3. The hello's roots change (§5): library folders no named root holds are marked "not named", and a
   named root holds no library folder. The new root is listed for the comparison, which today never
   happens (`coveredRoots` asks scans of covered roots only). Scenario 3, and scenario 1 once the
   backup is declared.
4. A scan finds new unread videos whose size and time match places that are unavailable or waiting as
   "possibly moved".
5. The admin asks ("Find moved files", D241 §8's "rescan to find missing media").

### 6.3 What it does

1. Asks the node for a listing (stat, no reads) of its declared roots, or of the one holding the gap;
   directories first when directory inodes are kept.
2. Matches, in order: same filesystem identity and inode, with size and time; then relative path
   below the moved directory, size and time; ties and low coverage are dropped.
3. Groups the matches by their longest common directory pair into X→Y proposals, and for each counts
   matched, different (same name, other size), and absent; finds conflicts at Y; computes each
   library's gains and losses and each role's change of reach.
4. Where matching was not by inode, reads a sample of payload hashes (say 20) at Y and compares.

### 6.4 How the admin confirms

- One proposal per X→Y, in the System page's Storage area and in needs-attention: "The folder
  `/tank/media/video/movies/disney` seems to be `/tank/media/video/kids-movies/disney` now: 143 of 143
  films found by disk identity. movies1 would no longer list them; movies2 follows." Five example
  paths side by side.
- **Move** runs D429's operation with the matched places marked confirmed (no read), the different
  ones unconfirmed, the absent ones left missing (decision 4); **Not a move** lets the waiting
  removals go (as `ApproveRemovals` does); **Later** keeps them waiting and the films unavailable.
- The proposal lapses if X reads again (then it was a mount problem or a copy). Nothing commits
  without the admin; the privilege of decision 2 is needed to press Move.

### 6.5 What it cannot do

It sees only the declared roots of one node. It cannot match a re-encoded file, a copy made without
preserving times on another filesystem (only by name, then a sampled hash), or a move to another
node; those stay manual repoints (and cross-node revival).

## Sources

- [repoint-library.md](../plans/repoint-library.md), D429, D432, D107, D241, D359, D363 (decisions
  index).
- [relocation-audit-2026-11-05.md](relocation-audit-2026-11-05.md),
  [visibility-at-scale-2026-11-05.md](visibility-at-scale-2026-11-05.md),
  [expansion-audit-2026-11-05-research.md](expansion-audit-2026-11-05-research.md) §2.1–2.3.
- Code at "docs(plans): the owner's answers on repointing, and what cache.db's paths are": `internal/core/library/move.go`, `libraryfolders.go`, `compare.go`
  (`concludeInodeMoves`), `complete.go` (`withholdReason`), `purge.go`; `internal/core/assembly/jobs.go`
  (`removed_files_kept_days`); `internal/store/queries/file.sql`, `retire.sql`;
  `internal/nodeproto/hello.go`, `report.go`, `listing.go`; `internal/node/confine` (`Holding`,
  `MediaRoot`), `internal/node/scan`, `internal/node/watch`, `internal/node/hash`;
  `cmd/orpheum/node.go` (`openRootsThrough`); `internal/core/api/access.go`, `libraries.go`;
  `internal/core/auth/roles.go`; deploy/release/README.md, deploy/staging/README.md.
- Not consulted: the development NAS rehoming tests' notes of 2026-11-06, which the plan names as input for
  questions 3, 5 and 6.
