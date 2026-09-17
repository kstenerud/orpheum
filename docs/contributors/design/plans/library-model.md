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

## Two requirements added late, from the plugin survey

Both come from [D20](../../decisions/working-notes.md), and both are schema-shaped, so they must
land here rather than being bolted on:

- **A rules engine.** Smart playlists, smart collections, and retention policy. The field shows
  roughly 7,000 stars of demand across plugins (smartlists, auto-collections, mergeversions,
  media-cleaner, tmdbboxsets) and companion apps (Kometa, Maintainerr, Janitorr), all of which
  exist because no server in this space lets a user express a rule about their library. A rule
  is a stored, re-evaluatable query over item attributes — which constrains how attributes,
  tags and provider fields are modelled.
- **Auth with OIDC/OAuth and LDAP**, in the core. A third-party Jellyfin SSO plugin has 1,458
  stars and Navidrome's two most-requested issues are the same two things. The user/identity
  tables are part of this schema and retrofitting external identity is painful.
- **Track-selection preferences, scoped and re-resolvable** ([D24](../../decisions/working-notes.md)).
  Stored as `{language, kind}` descriptors — never track indices, because track 3 in episode 1 is
  a different language in episode 2 — at user, library and series scope, tri-valued so that `off`
  is distinguishable from unset, and with the subtitle selection modelled as a **set** rather
  than a single value so dual subtitles do not later require a migration. These are user data:
  they go in the change journal and mirror to sidecars.
- **Normalised language tags on every track.** BCP 47, normalised at scan time from whatever the
  container carried, with the original kept for provenance. ISO 639-2's bibliographic and
  terminological codes (`ger`/`deu`, `fre`/`fra`, `chi`/`zho`) are a real source of mismatches —
  FFmpeg carries a 695-entry table for exactly this.

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
