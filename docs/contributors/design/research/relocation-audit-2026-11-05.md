# The relocation audit of the schema (2026-11-05)

> The owner asked that the schema let a library move without losing anything hung on it, and D429 §4
> asks that every stored path can be rewritten by prefix. This document checks persistent.db at
> v0080 and cache.db at version 48 against five questions, and gives a verdict for each. It decides
> nothing: where something does not hold, it names the smallest appended step (0081 or later) that
> would fix it and the churn that step saves, and leaves the choice to the owner. Read at main
> "docs(plans): defer the API compatibility gate, with how to build it later".

## 1. How it was checked

The path columns come from the list in [visibility-at-scale-2026-11-05.md](visibility-at-scale-2026-11-05.md)
§6, which was verified against `schema-v0080.sql` and `cache/schema.sql` and found complete for plain
paths. Each table was then read for a key, a UNIQUE, a CHECK, a foreign key or a trigger that touches a
path, a node, a hash or a device and inode. The Go and SQL that write the columns were read where the
schema alone could not say how a column is used. Nothing was run, and nothing was changed.

The code that exists today is `library.RepointRoot` (`internal/core/library/move.go`, D241 §6), which
moves one library root and the live places under it. D429 replaces it with a prefix rewrite over every
library, so the audit judges the schema for that, and names where today's code differs.

## 2. Check 1: every stored path can be rewritten by prefix in one transaction

**Verdict: holds for the schema, with four things the rewrite must do and one it should not.**

**What holds.**

- Every path is a plain TEXT or BLOB column: `place.path`, `place.reported_path_raw`,
  `library_root.root_path`, `file_filing.identified_from_path`, `unavailable_place.path` and
  `root_path`, `root_mount.root_path` and `mount_path`, `root_scan.root_path`, `root_state.root_path`.
- No path sits inside a hash, an id or a JSON value in persistent.db. `artwork.source_ref` holds a file
  id, not a path, `file_probe.probe_json` has no path field, and `setting.value_json` and
  `preference.value_json` hold none (a track choice names a stream or a fetched-subtitle id, and a
  description of the track).
- No CHECK and no trigger involves a path column. `place_found_au` fires only on `removed_at` and
  `modified_at`, and the one foreign key to a path-bearing row, `place.node_id`, is on the node's id.
- The only constraints a rewrite meets are UNIQUE indexes and primary keys over paths: `place_live_path`
  (`node_id`, `reported_path_raw`, live places only), and the primary keys of `library_root`,
  `unavailable_place`, `root_mount`, `root_scan` and `root_state`.

**What a rewrite trips over, beyond the known nested case.**

- **The nested move `/nest` to `/nest/sub`** fails partway on `place_live_path` and needs a temporary
  prefix, as the visibility document measured. The same holds for every primary key above, so the
  temporary prefix must be applied to all of them, not only to `place`.
- **A move to another node** changes `place.node_id` together with the path, and the unique key is
  `(node_id, path)`. A target that already holds the paths is a real conflict, and the operation must
  refuse or merge, as the visibility document says.
- **Raw bytes and NFC text are two columns, and nothing in the schema keeps them in step.** SQLite
  cannot compute NFC, so no CHECK could. The rewrite takes two prefixes (raw bytes, NFC text), and the
  guarantee has to be a Go test over the rewrite, not a constraint.
- **`library_root.root_path` is one spelling** (the text the node reported, D107), and the view
  `library_place` compares it as bytes with `place.reported_path_raw`. The rewrite must write a root in
  the spelling the node reports, or the view stops covering the places under it.
- **`file_filing.identified_from_path` has no node and is compared with the place's path.** The
  identification pass treats a file whose `identified_from_path` differs from its place's `path` as
  moved and identifies it again unless a person confirmed the match (`hasMoved` in
  `internal/core/identify/pass.go`, D208 §1). A rewrite that misses this column makes every file under
  the folder look renamed. Today's `RepointFile` (`retire.sql`) rewrites only `place`, so a repoint now
  does exactly that; a repoint under D429 must rewrite this column too, reached through the file's own
  place (the place whose id is the file's, which is what `SetFileVersionAndIdentifiedPath` wrote it
  from).
- **`RepointFile` also sets `modified_at` to NULL**, to force a read. That fires `place_found_au` for
  every place, which recomputes `item.found_at` and `found_file_modified_at` of each item of the file,
  and it sets `found_file_modified_at` to 0 until the next scan. The visibility document's 2.5 s did not
  include this, so it is unmeasured. It trips nothing, but it is the one write in a repoint that
  fires triggers.

**Do not rewrite** the places that are tombstoned and past their keeping: they are history. They are
not in the unique index (it is partial on `removed_at IS NULL`), and a returning payload revives one at
its new path anyway (`reviveOrInsert`, `observe.go`).

**cache.db** holds `scan.root_path` (UNIQUE with the node), `scan.scope_path` and the JSON lists of
relative paths, which a root's move does not change. Everything else is covered in §6.

**Proposed step (optional): a column that says "read this place again".** Instead of NULLing
`place.modified_at`, a step would add `place.is_unconfirmed INTEGER NOT NULL DEFAULT 0 CHECK (is_unconfirmed IN (0, 1))`,
which a repoint sets and the scan clears once the hash matches. It saves firing the found-at triggers
for every moved place and keeps `found_file_modified_at` (so "recently added" does not reorder during a
move). It costs code in the scanner, so it is only worth it if the trigger cost proves large when
measured.

## 3. Check 2: nothing people own keys on a path, a node's name or a device and inode

**Verdict: holds.** What people own keys on ids that a repoint does not change:

| What | Keyed on |
| --- | --- |
| Watched state (`work_watched`), hidden items (`item_hidden`) | user and work or item id |
| Resume positions (`play_position`), spans and events | user and `file.id` |
| Preferences (`preference`) | user, scope, `scope_id` (a file id, item id, library id or device id) and axis |
| Choice of track | a `preference` row at scope `file`, naming a stream id and a description of the track |
| Fetched subtitles (`fetched_subtitle`) | `video_file`, a `file.id` |
| Overrides (`field_override`) | item id, field, language |
| Identification (`item_provider_id`, `identification_state`, `followed_source`) | item id |
| Artwork picks (`artwork.is_chosen`) | the artwork row, whose item is an id |
| Loudness, video timelines, subtitle text | `payload_hash`, which a move does not change |
| Purged watching (`purged_watching`) | user and version id |

No unique or primary key in persistent.db includes `place.filesystem_device` or `place.inode`. They are
plain columns with no index, and the scanner uses them in code to see a same-disk move (D304 §5). That
is correct: they mean nothing on another machine, so no key may rest on them.

**Two soft spots, neither a schema fault.**

- **A track choice names a stream id, and a re-read writes streams anew under new ids.** The choice
  carries a description that finds the same track again (`preferences.go`, D223 §8), so it survives.
- **A local image's `artwork.source_ref` is `file:<id>:<size>:<mtime>`** (`artwork/images.go`). If a
  move to another machine does not keep modification times, the ref no longer matches and the image is
  owed again. Whether a person's pick on the old artwork row then survives was not traced. It is worth
  a test before the repoint ships; the pick also has `source_sha256`, which does not change.

**One deadline.** `ListFilesToPurge` deletes a removed place past its keeping, and a file goes with its
last place, taking `play_position`, spans, events, fetched subtitles and file-scope preferences with
it (only a version-level count survives, in `purged_watching`). A repoint that leaves files "not found
at Y" starts that clock for them. This is the one way a move can lose people's state, and it is code
and policy, not schema.

## 4. Check 3: a node can be renamed or replaced

**Verdict: holds for the rows, does not hold for how a machine is matched to its node.**

**What keys on the node's id.** `place.node_id`, `library_root`, `root_mount`, `root_scan`,
`root_state`, `unavailable_place`, `device_reader_limit` and cache.db's `scan` and `node_attempt` all
hold `node.id`. Renaming a node is one `UPDATE node SET name`, and nothing else keys on the name.

**What keys on the name.** Only enrolment: `GetLiveNodeByName` finds the node a hello belongs to, and
`RemoveNode` says a removed node's name enrols a new one. cache.db `resource_use.node_name` is a
statistic and starts a new series after a rename, harmlessly. Two soft ones:

- **`device_reader_limit.device_key`** (`zfs:tank`, `disk:sdc`) is a name for a device on that machine.
  Another machine in the node's place keeps the rows and they name devices that may not exist. It is an
  admin setting, so it is stale, not lost.
- **No schema rule keeps live node names unique.** `node` has no unique index on `name`. Two hellos at
  once could enrol two live nodes of one name.

**What does not hold.** A replacement machine that says hello under a new name enrols a new node with
no libraries, and there is no way to say "this hello is node 3" in the schema or the code. The only
ways today are to give the new machine the old name, or `RepointRoot`, which moves the roots and the
live places to the new node and leaves anything on the old node id behind. The visibility document's
observation ("a moved database finds its libraries only if the new machine's node keeps the old node's
name") is this.

**Proposed step: the node's name becomes a label, and the node gets its own identity.** A step would
add `UNIQUE INDEX node_live_name ON node(name) WHERE removed_at IS NULL`, so a rename cannot collide,
and a rename operation (`UPDATE node SET name`) becomes safe by construction. Matching a new machine
to an old node needs an enrolment decision (an admin adopting a hello onto a node id), which is an API
and not a column. The index saves a duplicate-node clean-up later; the adoption saves repointing every
place of a node by hand each time a machine is replaced.

## 5. Check 4: a file found at a new place joins its existing `file` row

**Verdict: the table shape allows it; the queries, two views and a trigger assume otherwise, so it does
not hold for a new place row.**

**What allows it.** Step 0078 made `place.file_id` point at `file.id`. `file.payload_hash` is indexed
(`file_payload`) and not unique, so many places can hold one file. Watching, resume positions, filings,
fetched subtitles, probes, streams and preferences all hang on `file.id`, so a place that holds the
existing file keeps all of them. Nothing in a UNIQUE, CHECK or foreign key stops a second place, or a
place on another node, holding it.

**What blocks it.** Everything outside the table shape assumes a file's first place has the file's own
id (`place.id = file.id`):

- **Two derived views and a trigger.** `item_found` joins `place.id = file_filing.file_id`, and
  `place_found_au` selects `file_filing.file_id = new.id`. A file whose first place is removed and which
  is now held by a new place row has no live place in these terms, so `item.found_at` becomes 0 and the
  item drops out of "recently added" ordering, though the file is live and listed. `library_place` and
  `version_library` are fine, they use `place.file_id`.
- **28 query sites** join `place.id = file.id`, `place.id = file_filing.file_id` or
  `place.id = s.bytes_file`, for example `GetLatestTombstoneByPayloadHash` and the copy-candidate query in `file.sql`, and
  reads in `identify.sql` and `browse.sql`. For a file whose own place is gone and
  whose copy lives, they find no live place.
- **Matching by hash is per node and revives only a tombstone.** `ListLiveFilesByPayloadHash` and
  `GetLatestTombstoneByPayloadHash` both take a `node_id`, and revival clears `removed_at` on the file's
  own place and updates its path. A file seen on another node gets a new file row; a file seen at a new
  path of a live place gets a duplicate. The comment above `ingestObserved` says so ("its history does
  not follow it").
- **A sidecar is matched by path only.** `recordSidecar` finds a place by path and otherwise inserts a
  new file; a hash match is never tried, and a sidecar's hash is optional (`file.payload_hash` is NULL
  where the node gave no identity).
- **A file still unread has no hash.** `payload_hash` is NULL until it is read (D304 §1), so a file
  never read has nothing to match by except its size and `opensubtitles_hash`.

None of this loses data if the repoint rewrites the place row in place (D429 §2): the id, the file and
everything on it stay. It matters when the plan is to match files found at the new root by hash and
create new places, as the plan's "Repointing a library" describes.

**Proposed step: make "the file's own place" something the schema says.** Smallest: a step that
recreates `item_found` and `place_found_au` over `place.file_id` and takes the first-found time from the
earliest `first_seen_at` among all places of the file, so that a second place does not reset it. Then
the 28 query sites can be moved off `place.id = file.id` one by one, with no schema change. It saves
the rewrite of every consumer when music, photos or a second node first make a file's first place go
away while the file stays. Rewriting rows in place avoids the need until then.

## 6. Check 5: the unreachable paths

| Column | Matters? | Why |
| --- | --- | --- |
| cache.db `job.destination` (`root:3:/media/films`) | No | Parked work waiting for a root. `RepointRoot` already cancels the old destination's work (`CancelRetired`) and the sweeps derive it again. cache.db is rebuilt by discard, so a stale value costs a re-derived job. |
| cache.db `scan_batch.files_json` | No, if the scan is ended first | It is the files of a scan in flight, named by path. A repoint must cancel the root's open scan, and its batches go with it, in the same operation. cache.db is another file, so this is two commits, not one transaction; that is fine because a lost scan is asked for again. |
| `place.error_detail_json` | No | The platform's text for an unreadable place, which may quote the old path. A rescan of the new place writes it afresh (only a still-errored place has one). |
| `place.filing_reason`, `file_filing.filing_reason` | No | Diagnostic text, "the rule verbatim, what was tried". Nothing looks up by it. It may name an old path until the next identification writes it again. |
| `person.picture_ref` | Maybe | A TMDB profile path, or an NFO's thumb, which can be a local path or a URL. Not fetched (the column's comment). A relative thumb survives a move; an absolute local one would be a stale reference to a picture not shown. No schema change helps, since the NFO owns the value. |
| `file_nfo.body` | No | The NFO's own bytes. It is not ours to rewrite, and a rescan sends it again (class O). |

cache.db may stay as it is: `chunk_store` is keyed by `source_file` (a file id) and stream, and
`match_candidate`'s `provider_poster_path` is a provider's path, not a local one. Neither needs a
rewrite.

**Is a column split for `job.destination` worth it?** A step to cache.db (a version bump that discards
it) could split the string into a kind, a node id and a root path, so a prefix rewrite could reach it.
It is cheap because cache.db is discarded on a version bump. It saves nothing a repoint needs, since
parked work is derived again; it is only worth it if some later feature wants parked work to survive a
move.

## 7. Summary of the proposed steps

Each is a proposal for the owner, not a decision, and none is written.

| For | What the appended step would do | What it saves later |
| --- | --- | --- |
| Check 1, optional | Add `place.is_unconfirmed` and set it in a repoint instead of NULLing `modified_at`. | The found-at triggers firing for every moved place, and the transient 0 in `found_file_modified_at`. |
| Check 3 | Add a partial UNIQUE index on `node(name)` where `removed_at IS NULL`. | A duplicate live node after two hellos, and a safe rename. |
| Check 4 | Recreate `item_found` and `place_found_au` over `place.file_id`, taking found-at from the file's earliest place. | Reworking every consumer when a file's first place goes and another keeps it. |

Three things need code or a decision rather than a column:

- the repoint must rewrite `file_filing.identified_from_path` through each file's own place, or every
  file under the folder is identified again (a bug in today's `RepointFile` too);
- the node adoption flow for a replaced machine (check 3);
- the purge clock for files not found at the new root (check 2).
