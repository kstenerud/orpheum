> **ABOUTME:** What is actually irreplaceable, how it is backed up, and how a user gets back to
> a working server — the trust story behind "your files stay yours".

# Backup and restore

- **Status:** UNSPECIFIED.
- **Depends on:** library-model.md

## The gap

[D7](../../decisions/working-notes.md) says the files are the source of truth and the database
is a rebuildable index, which removes most of the fear. But it is not all of it:

- **User-authored data is not derivable** — metadata corrections, watch state, playlists,
  language preferences ([D24](../../decisions/working-notes.md)), rules, users and their
  credentials. D7 says it mirrors to sidecars; nobody has designed that.
- ~~**Postgres is not a file the user can copy.**~~ **This cost is gone** —
  [D31](../../decisions/working-notes.md) moved the database to SQLite, so a backup is copying a
  file (with the usual care about WAL sidecars and using the online-backup API rather than `cp`
  on a live database). One of the reasons D31 was taken.
- **A rebuild is not free.** Re-scanning, re-fetching metadata under provider rate limits, and
  regenerating trickplay for a large library is potentially days of work
  (`performance-targets.md`).

Immich ships a `database-backup.service` and an `integrity.service` in-tree, which is the field's
best answer and worth reading.

## What the research must settle

- **What the sidecar format is**, and whether it is enough to rebuild everything a user would
  mourn. NFO is the Kodi-compatible precedent; it does not cover watch state.
- **Automatic pre-migration backup** — already required by D5's migration discipline, not yet
  specified: where it goes, how many are kept, how the user is told.
- **Restore, tested.** A backup nobody has restored is a hypothesis. This belongs in
  `testing-strategy.md` as a scheduled test, not a documented procedure.
- **Partial disaster**: media disappears (unmounted drive), which must not cause the library to
  delete itself — a real failure mode in this field and the reason Janitorr's users are careful.
