> **ABOUTME:** What is actually irreplaceable, how it is backed up, and how a user gets back to
> a working server — the trust story behind "your files stay yours".

# Backup and restore

- **Status:** PLANNED — researched 2026-09-19 ([backup-restore.md](../research/backup-restore.md), [D77](../../decisions/working-notes.md)); **the database is backed up whole** ([D185](../../decisions/working-notes.md)); **an older backup is migrated, not refused** ([D181](../../decisions/working-notes.md)). In Cut A ([D178](../../decisions/working-notes.md), F12).
- **Depends on:** library-model.md

## The gap

[D7](../../decisions/working-notes.md) says the files are the source of truth and the database
is a rebuildable index, which removes most of the fear. But it is not all of it:

- **User-authored data is not derivable** — metadata corrections, watch state, playlists,
  language preferences ([D24](../../decisions/working-notes.md)), rules, users and their
  credentials. D7 says it mirrors to sidecars; nobody has designed that.
- ~~**Postgres is not a file the user can copy.**~~ **This cost is gone** —
  [D31](../../decisions/working-notes.md) moved the database to SQLite, so a backup is copying a
  file — though **not** with `cp`: see below, where a measured file copy of a live WAL database produced a file with no tables in it. One of the reasons D31 was taken.
- **A rebuild is not free.** Re-scanning, re-fetching metadata under provider rate limits, and
  regenerating trickplay for a large library is potentially days of work
  (`performance-targets.md`).

Immich ships a `database-backup.service` and an `integrity.service` in-tree, which is the field's
best answer and worth reading.

## The method: `VACUUM INTO`

**Never copy the database file.** Measured on SQLite 3.46.1 immediately after committing 20,000
rows, `live.db` was **4,096 bytes** and `live.db-wal` was **333,752** — the whole content was in
the write-ahead log. `cp live.db` produced a file that opens cleanly and reports **`no such
table`**. It fails silently, and only at restore.

Correct methods, all measured working: copy all three files, the backup API,
`wal_checkpoint(TRUNCATE)` then copy, and **`VACUUM INTO`**. We take the last, on properties
rather than correctness — it is plain SQL so it needs no driver-specific code, it emits **one**
file with no sidecars, and it compacts. Measured at **29 ms for 2.8 MB with a writer committing
concurrently**, no errors.

Two guards, taken verbatim from Navidrome's hard-won comments:

- **`os.Stat` the source before restoring.** Their note: *"the driver opens with
  `SQLITE_OPEN_CREATE`, so without this check a typo in the path would create an empty database
  and 'restore' it over the live one."* One line; the most destructive bug available here.
- **Reject paths containing `?`** — *"the driver splits the DSN at '?', so such a path would
  open a different file."*

## What is actually backed up

> **Simplified by [D185](../../decisions/working-notes.md):** the table below still says what is
> *irreplaceable*, but the backup no longer splits the database by it. **`library.db` is backed up
> whole** — at the reference scale 32–46 MB and 130–180 ms by `VACUUM INTO` — which carries the
> identity rows ([D179](../../decisions/working-notes.md)) and the index for free and makes a
> restore a restore rather than a rebuild. What is left out is what is large and disposable:
> `cache/` on disk and the separate `cache.db` of fetched responses and queues
> ([schema.md](schema.md)).

| Regenerable by rescan | Irreplaceable |
| --- | --- |
| library index, probe results | watch state and playback positions |
| thumbnails, trickplay sprites | accounts, shares and their predicates |
| extracted subtitles, subset fonts | rules — playlists, collections, retention |
| FTS indexes, sort keys | track-selection preferences ([D24](../../decisions/working-notes.md)) |
| | ~~change journal and sync cursors~~ — a lossy accelerator ([D166](../../decisions/working-notes.md)) |
| | **identity rows**: every File's id, payload hash, length, mtime and last path; every Title's id and provider ids — **the index is rebuildable, its keys are not** ([D179](../../decisions/working-notes.md)) |

Right column: kilobytes to a few megabytes. Left: gigabytes. So **backup is automatic, frequent
and on by default**, which is a product decision only available because
[D7](../../decisions/working-notes.md) made the database an index. **It is shown at setup and
on the backup page** ([D184](../../decisions/working-notes.md)): where the backups go, how many are kept, and that **a
backup on the same disk does not survive that disk** — with the option to move it.

**The media tree is already a partial second copy** — D7 mirrors metadata corrections to
sidecars, so rescan-plus-sidecars recovers them. Test that as a durability property rather than
asserting it.

**Watch state is the exception and the thing that most needs the backup.** It must *not* go to
sidecars: it is per-user, and writing it beside the media would expose one household member's
viewing to anyone with filesystem access. The sidecar mechanism structurally cannot carry it.

## Restore

A backup that has never been restored is not a backup, so the deliverable is **a test that
restores into an empty instance and asserts equivalence of the irreplaceable set** — not a
checksum, which only proves a copy happened.

- **A backup carries its schema version.** ~~Restore refuses a gap rather than
  auto-converting.~~ **An older backup restores and is then migrated by the ordinary runner; a
  newer one is refused unless it declared itself compatible** ([D181](../../decisions/working-notes.md),
  [migrations.md](migrations.md)).
- **Restore is destructive and says what it will overwrite** before doing it.
- **Migrations declare that they need a backup.** Jellyfin has exactly this as a
  `JellyfinMigrationBackupAttribute`, which is §7 implemented rather than promised.

## The sidecar format question is answered

This plan flagged that the sidecar format was undesigned. It is **NFO**
([D141](../../decisions/working-notes.md)) — the README promises *"a format any other media
server can read"* and that settles it. Relevant here because the two mechanisms are
complementary and the split is now concrete: **corrections about the work leave in the
sidecars, and everything about a person leaves in this backup**
([D98](../../decisions/working-notes.md)). Somebody migrating to Kodi takes the first and not
the second, which is a stated limitation rather than a gap.

## Still open

- **Retention** — how many backups, how long. Unmeasured.
- **Whether `cache/` ever deserves an optional archive.** It is disposable by design, but
  "correct, and now regenerate 100 GB of trickplay" is a long wait.
- **Node-local state should cost a rescan and nothing else**
  ([D59](../../decisions/working-notes.md)) — assert it as a test rather than assume it.

## Done when

A fresh install restores a backup and the user's watch state, rules, shares and preferences are
all present; a restore into the wrong path cannot destroy a live database; a backup from an
older schema ~~is refused with a comprehensible message rather than converted~~ **restores and is
migrated by the ordinary runner, and one from a newer schema is refused with a comprehensible
message naming the way back** ([D181](../../decisions/working-notes.md)); **the whole
`library.db` is in the backup, so a restore is a restore rather than a rebuild**
([D185](../../decisions/working-notes.md)); and the automatic backup has run, with where it goes
and how many are kept shown at setup ([D184](../../decisions/working-notes.md)).
