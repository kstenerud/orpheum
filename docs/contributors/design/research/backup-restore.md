> **ABOUTME:** What actually needs backing up when the database is a rebuildable index, and the
> measured fact that the obvious way to back up a WAL database produces a file with no tables
> in it.

# Backup and Restore

Researched **2026-09-19**. Measurements against SQLite 3.46.1 are flagged as such; the
implementation evidence is Navidrome's `db/backup.go` and Jellyfin's `FullSystemBackup`, read
locally.

## 1. The obvious backup is catastrophically wrong — measured

A WAL database is three files, and recent commits live in the `-wal`, not the `.db`. Measured
immediately after committing 20,000 rows:

```
live.db = 4,096 bytes      live.db-wal = 333,752 bytes      live.db-shm = 32,768 bytes
```

The entire content is in the write-ahead log. So:

| Method | Result |
| --- | --- |
| `cp live.db` | **BROKEN: no such table: t** |
| `cp` all three files | 20,000 rows, `integrity_check` ok |
| `VACUUM INTO` | 20,000 rows, ok — **single file, no sidecars**, 324 KB |
| backup API | 20,000 rows, ok |
| `wal_checkpoint(TRUNCATE)` then `cp` | 20,000 rows, ok |
| `cp` all three mid-transaction | 25,000 rows, ok — uncommitted row correctly absent |

**`cp library.db backups/` produces a valid SQLite file containing nothing.** It opens without
error. It fails silently and only at restore, which is the worst possible time. Any
documentation that suggests copying the database file is actively dangerous.

## 2. `VACUUM INTO` is the right answer for us — measured

It is plain SQL, so it works on any driver; it emits **one** file with no sidecars; it compacts;
and at our scale it is free. Measured on a 2.8 MB database with a writer committing
concurrently:

```
VACUUM INTO 'v.db'   →  29 ms, 200,000 rows, concurrent writer committed 5 rows, no errors
```

Navidrome instead uses the **backup API** via `mattn/go-sqlite3`, and leaves a caution in the
code: *"-1 means that sqlite will hold a read lock until the operation finishes. This will lock
out other writes that could happen at the same time."* Both pure-Go drivers do expose a backup
API (`ncruces` has `Backup`/`BackupInit`/`BackupStep`), so either is available — but `VACUUM
INTO` needs no driver-specific code at all.

## 3. Two footguns Navidrome guards against, both worth stealing

From the comments in their `backupOrRestore`:

> *"The driver opens with `SQLITE_OPEN_CREATE`, so without this check a typo in the path would
> create an empty database and 'restore' it over the live one."*

A mistyped restore path silently creates an empty database and restores **nothing over your live
data**. They `os.Stat` the source first. This is the single most destructive bug available in
this area and it is one line to prevent.

> *"The driver splits the DSN at '?', so such a path would open a different file."*

A path containing `?` opens something other than what you named. They reject it.

Jellyfin corroborates the shape from the other direction: a full `BackupService` with a manifest
and restore, plus a `JellyfinMigrationBackupAttribute` — **migrations declare that they require
a backup**, which is [principles §7](../../principles/07-migrations-are-safety.md) implemented rather than
promised.

## 4. Our backup is unusually small, and that changes the design

[D7](../../decisions/working-notes.md) and [D13](../../decisions/working-notes.md) say the
database is a **rebuildable index** and the media is the source of truth. So most of what a
media server stores is not backup material:

| Regenerable by rescan | Irreplaceable |
| --- | --- |
| the library index, probe results | watch state and playback positions |
| thumbnails, trickplay sprites | user accounts, shares and their predicates |
| extracted subtitles, subset fonts | rules — playlists, collections, retention |
| FTS indexes, sort keys | track-selection preferences ([D24](../../decisions/working-notes.md)) |
| | the change journal and sync cursors ([D8](../../decisions/working-notes-archive.md)) |
| | metadata corrections — *and see below* |

The right column is **kilobytes to a few megabytes**; the left is gigabytes. Which means
**backup can be automatic, frequent and on by default**, because it costs nothing. That is a
different product decision from the one a project with a monolithic database can make.

### The media tree is already a partial second copy

[D7](../../decisions/working-notes.md) writes metadata corrections back to **sidecars** beside
the media. So a restore from nothing — rescan, read sidecars — recovers the corrections
already. That is a real durability property, not a slogan, and it is worth testing as one.

**Watch state is the exception, and deliberately so.** It is the genuinely irreplaceable thing,
and it must **not** be mirrored to sidecars: it is per-user, and writing it beside the media
would expose one household member's viewing history to everyone with filesystem access. So
watch state is the item that most needs the database backup, precisely because the sidecar
mechanism cannot carry it.

## 5. Restore is the untested half

A backup that has never been restored is not a backup. The concrete requirement is a test that
restores into an empty instance and asserts equivalence of the irreplaceable set — not a
checksum of the file, which proves only that a copy happened.

Two further properties worth designing in:

- **Restore must refuse a version gap** rather than auto-converting, which is
  [principles §7](../../principles/07-migrations-are-safety.md)'s detect-and-refuse rule applied to backups. A
  backup carries the schema version it was taken at.
- **Restore is destructive and should say what it is about to overwrite**, listing the live data
  it replaces, before doing it.

## 6. Open

- **Retention policy for backups** — how many, how long. Navidrome names them
  `navidrome_backup_<timestamp>.db` and prunes; a count plus an age bound is probably right and
  is unmeasured.
- **Whether the `cache/` tree is ever worth backing up.** It is explicitly disposable, but a
  100 GB trickplay regeneration is a long wait after a restore. Possibly a separate, optional,
  off-by-default archive.
- **Whether a node's local state needs any of this.** It should not —
  [D59](../../decisions/working-notes.md) says node-local state may change cost, never content,
  so losing it costs a rescan and nothing else. Worth asserting as a test.
