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

## Requirements added late

Each is schema-shaped, so it lands here rather than being bolted on. The first two come from the
plugin survey ([D20](../../decisions/working-notes.md)), the track-selection pair from
[D24](../../decisions/working-notes.md), and the last three from the internationalisation round
([D45](../../decisions/working-notes.md), [D47](../../decisions/working-notes.md),
[D48](../../decisions/working-notes.md)).

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
  **The embedded-font opt-in is one more axis on this same table**
  ([D57](../../decisions/working-notes.md)) — per user, sticky at series scope, tri-valued, and
  deliberately *without* the per-library default the language axes get, because a library
  default is a standing grant over every file that later lands in the folder.
- **Normalised language tags on every track.** BCP 47, normalised at scan time from whatever the
  container carried, with the original kept for provenance. ISO 639-2's bibliographic and
  terminological codes (`ger`/`deu`, `fre`/`fra`, `chi`/`zho`) are a real source of mismatches —
  FFmpeg carries a 695-entry table for exactly this.
- **Every metadata-bearing entity splits in two** ([D45](../../decisions/working-notes.md)): a
  language-invariant row (kind, dates, runtime, ratings, external ids, and **genres as an
  enumeration** rendered from our own UI catalogue rather than stored as provider text) plus
  translation rows keyed `(item, language)` carrying name, description, tagline, aliases,
  free-text tags **and artwork** — posters have title text in the pixels, so poster language is
  its own axis. The original-language record is distinguished rather than being one translation
  among many: it always exists, so it is both the guaranteed fallback and the meaning of "sort
  by original title". Selection happens per request from `Accept-Language`, and because it
  participates in `ORDER BY` it must be a **total** order or keyset pagination repeats rows.
- **A sort-key column per (item, locale-in-use)** ([D47](../../decisions/working-notes.md)) — an
  indexed `BLOB` of CLDR sort-key bytes, ~140 B each, ordered with SQLite's default `BINARY`
  collation. Not a transliteration of the title, and not a custom SQLite collation, which would
  need one index per locale anyway and would make the file unopenable by the `sqlite3` CLI.
- **Two FTS5 indexes plus a normalised-alternates column**
  ([D48](../../decisions/working-notes.md)): `unicode61 remove_diacritics 2` for Latin word
  search and bm25 ranking, `trigram` for substring search, because neither alone covers CJK,
  Thai and atomic letters like ø/æ/ß. Both indexes for 20k titles measure 1.5 MB. **The
  tokeniser is immutable once the virtual table exists**, so this cannot be deferred, and the
  normalisation function must have exactly one implementation — Navidrome shipped a corrective
  migration in July 2026 because a SQL approximation of it drifted from the Go original.

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
