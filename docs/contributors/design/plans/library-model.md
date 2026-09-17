> **ABOUTME:** The core data model — how a title, its editions, its files and its provenance are
> represented — and the first schema, including the change journal.

# Library model

- **Status:** PLANNED — designed 2026-09-17.
- **Depends on:** —

## Scope

The first schema. Gets `Title → Edition → MediaFile` right, models multi-version and multi-part
items as first-class rather than bolted on, keeps provider IDs and field-level provenance in
their own tables, and includes the audit/change-journal tables and sync cursor from D8.

Generic enough that audio can be added without a rewrite (D1): `Library`, `Item`, `MediaFile`
rather than `Movie`/`Episode` everywhere.

## Why it is shaped this way

- Jellyfin 12.0's headline change — in 2026 — is turning playlist and collection membership
  into real rows with real foreign keys. Normalise from the start.
- Multi-version items (1080p and 4K of one film, multiple cuts, multi-part files, disc images)
  are a permanent source of long-running bugs in both Jellyfin and Plex because both bolted
  them on.
- A change journal cannot be retrofitted cheaply, and offline clients need deletes (D8).
- Identity must survive a file move, a rename, a re-rip and a provider change: UUID primary
  keys, `{provider, provider_id}` in a side table.

## Done when

Schema and `sqlc` queries exist, migrations run forward from empty, and a documented test
fixture library round-trips through scan → query → rebuild-from-empty with no user-visible loss.
