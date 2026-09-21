> **ABOUTME:** The core data model — how a title, its editions, its files and its provenance are
> represented — and the first schema, including the change journal.

# Library model

- **Status:** PLANNED — designed 2026-09-17; **reviewed and corrected 2026-09-19** ([library-model-review.md](../research/library-model-review.md), [D79](../../decisions/working-notes.md)); **nine more schema items added by the audit the same day** ([audit-2026-09-19.md](../research/audit-2026-09-19.md), [D81](../../decisions/working-notes.md)–[D84](../../decisions/working-notes.md), [D90](../../decisions/working-notes.md), [D95](../../decisions/working-notes.md)–[D97](../../decisions/working-notes.md)), plus the sidecar split ([D98](../../decisions/working-notes.md)), a device scope on the preference table ([D103](../../decisions/working-notes.md)), and **the `role` table** ([D130](../../decisions/working-notes.md), 2026-09-21).
- **Depends on:** —

## Scope

The first schema. Models multi-version and multi-part items as first-class rather than bolted
on, keeps provider IDs and field-level provenance in their own tables, and includes the
audit/change-journal tables and sync cursor from D8.

Generic enough that audio can be added without a rewrite (D1): `Library`, `Item`, `MediaFile`
rather than `Movie`/`Episode` everywhere.

## The levels: four, not three

Corrected by [D79](../../decisions/working-notes.md). **EIDR** — the industry identifier
registry — models works as **Abstraction → Edit → Manifestation** plus **Collection** as an
orthogonal container. Ours:

| Level | Holds | EIDR |
| --- | --- | --- |
| **Title** | the work; genres, external ids, invariant facts | Abstraction |
| **Edition** | a **creative cut** — theatrical, director's, extended | Edit |
| **Version** | a **technical representation** — an encoding, a resolution | Manifestation |
| **File** | the bytes; a Version may have several (multi-part) | Item |
| **Collection** | series, season, box set, user collection, smart collection | Collection |

**Why the fourth level is not optional.** *Glass Runner: The Final Cut* held as 1080p in two
parts **and** as a single 4K file is one Title, one Edition, two Versions, three Files. With
`Edition → File` there is nowhere to record that the two parts are one viewing and the 4K is
another — the multi-version and multi-part axes hang off the same parent and cannot be told
apart. That is precisely the combination named below as the field's permanent bug source.

Jellyfin demonstrates the cost of the absence: `Video.cs` carries **four** overlapping
mechanisms — `PrimaryVersionId`, `LinkedAlternateVersions`, `LocalAlternateVersions`,
`AdditionalParts` — two of them bare arrays of *path strings*, so the parts of a multi-part
film are not rows at all. A `Video` is Edit, Manifestation and Item at once, so every
distinction becomes a side-array.

**A Collection is either an explicit member list or a predicate**
([D75](../../decisions/working-notes.md), [D76](../../decisions/working-notes.md)) — which is
what lets one table serve series, seasons, box sets, hand-made collections and smart
collections.

### Two levels that must be declared

- **Watch state attaches at the Edition.** Not File or Version, or upgrading 1080p to 4K loses
  your place. Not Title, because cuts have different runtimes and a resume position across them
  is meaningless. Whether a watched *flag* rolls up to the Title is a UI question.
- **Names attach at Title *and* Edition** — "Glass Runner" and "The Final Cut" are both names
  needing [D45](../../decisions/working-notes.md)'s translation treatment. So the displayed
  title is a **composition of two translated strings**, which the sort key
  ([D47](../../decisions/working-notes.md)) and the FTS row
  ([D48](../../decisions/working-notes.md)) must both account for.

## Requirements added late

Each is schema-shaped, so it lands here rather than being bolted on. The first two come from the
plugin survey ([D20](../../decisions/working-notes.md)), the track-selection pair from
[D24](../../decisions/working-notes.md), and the last three from the internationalisation round
([D45](../../decisions/working-notes.md), [D47](../../decisions/working-notes.md),
[D48](../../decisions/working-notes.md)).

- **A rules engine** ([rules-engine.md](rules-engine.md), now designed —
  [D75](../../decisions/working-notes.md)). Two things it hands this schema: a **closed, typed
  field registry** with an `alias` column, which is how saved rules survive a column rename and
  is the only mechanism that works; and a forced decision on **whether tags live in a join table
  or as JSON**, because multi-valued count operators ("more than three genres") are compiled
  differently in each and Navidrome's JSON choice is what drove them into SQLite's
  expression-depth limit. Smart playlists, smart collections, and retention policy. The field shows
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
  a different language in episode 2 — at user, library, series **and device** scope
  ([D103](../../decisions/working-notes.md); a device *preference* is stored, a device
  *capability* never is), tri-valued so that `off`
  is distinguishable from unset, and with the subtitle selection modelled as a **set** rather
  than a single value so dual subtitles do not later require a migration. These are user data:
  they go in the change journal — but **not** to sidecars, correcting the parenthesis in
  [D24](../../decisions/working-notes.md). They are per-user and the media tree is shared, so
  [D98](../../decisions/working-notes.md) applies: a sidecar carries what is true about the
  work, the database carries what is true about a person.
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

## Nine more schema items, from the audit

Added 2026-09-19 by [audit-2026-09-19.md](../research/audit-2026-09-19.md). Each is schema, so
each lands here rather than being bolted on — the same argument
[D75](../../decisions/working-notes.md) and [D79](../../decisions/working-notes.md) made.

- **Favourites and user ratings are in the play-state family, at the Title**
  ([play-state.md](play-state.md)). Jellyfin keeps them in the same mutable row as the resume
  position, which is why *"Movie favourites are randomly unfavourited"* (33 comments) sits next
  to the resume bugs: one race, three symptoms. They ride the change journal and, like all
  per-user state, are **database-only** ([D98](../../decisions/working-notes.md)).
- **Play state is two structures, not one column**
  ([D81](../../decisions/working-notes.md), [play-state.md](play-state.md)). A **guarded
  position** per `(user, edition)` carrying a monotonic sequence, so a stale device cannot
  rewind a newer one; and an **append-only play-event log** with device, server timestamp and
  the client's own monotonic clock. Jellyfin keeps one mutable row and its tracker carries
  *"Most movies now appear to be 75% watched"* (44 comments) and *"Media playback randomly
  starts over again or jumps back — STILL"* (29). The log is also the only table with an
  unbounded row count, so it carries a retention horizon from the first migration.
  **A position is Edition-relative**, so each File records its offset within the Version's
  timeline — otherwise "12:30 into part 2" has nowhere to live.
- **Media segments, typed, at the Version** ([D82](../../decisions/working-notes.md),
  [media-segments.md](media-segments.md)). `(version, type, start, end, provider, confidence)`
  with a user override. At the **Version** rather than the Item because a timecode indexes an
  encoding: a 4K remux and a web-dl differ in lead-in, and a PAL-speedup rip differs
  throughout. Distinct from chapters, which are authored container metadata and have no type.
- **Loudness per audio track** ([D83](../../decisions/working-notes.md),
  [audio-presentation.md](audio-presentation.md)). Integrated loudness, loudness range and true
  peak, measured by the node with `ebur128` and stored as observed facts. Netflix normalises its
  catalogue at ingest (-27 LKFS dialog-gated); we cannot rewrite files, so we take ReplayGain's
  answer — measure once, apply gain at playback.
- **Certifications are `(item, system, code)`** ([D84](../../decisions/working-notes.md),
  [parental-controls.md](parental-controls.md)), many per item, with an ordering curated **per
  system** — **plus a content-descriptor vector and its provenance**
  ([D131](../../decisions/working-notes.md)). Seven axes, Kijkwijzer's — violence, fear, sex,
  discrimination, drugs, language, dangerous acts — each tri-valued so **unknown is distinct
  from none**, and each carrying *which source judged it*, because a descriptor parsed from a
  `TV-14-SV` string and one inferred from the `horror` genre are not the same quality of fact.
  A single number cannot express "block *Sex and the City* for the tween and *Blair Witch* for
  the teenager", and Jellyfin's `us.json` collapses all fifteen `TV-14-*` descriptor
  combinations to one value, which is that failure shipped. BBFC 15, FSK 16, MPAA R and ACB MA15+ are not points on one scale and TMDB returns
  them keyed by country. Flattening to an integer fails **open**, which is the wrong direction
  for this one field. An item with no certification in the household's system is *unrated*, a
  third state, not permitted.
- **People are entities with provider identity** ([D95](../../decisions/working-notes.md)).
  `Person` with `{provider, provider_id}` in a side table; credits as typed rows — `cast` with
  a character and an order, `crew` with a job. **The display name is never the key**: Jellyfin
  has *Actors with the same name appear as one person* open since 2020-06, and its music twin
  names the cause outright — *"artists with the same name are merged while MBIDs are ignored"*.
  A person's name is also a translatable string ([D45](../../decisions/working-notes.md)).
- **Track-presentation preferences gain two axes**
  ([D90](../../decisions/working-notes.md)): a signed **sync offset** in milliseconds for audio
  and for subtitles — requested on four separate client trackers — and the caption presentation
  set (font, size, colour, background, window, edge) that CVAA and the EU Accessibility Act
  both require. Both ride [D24](../../decisions/working-notes.md)'s existing table at the same
  scopes.
- **An episode's position is `(ordering, season, number)`**
  ([D96](../../decisions/working-notes.md)). Aired, DVD, absolute and a provider's alternate
  orders are orderings of the same set of episodes, so an episode has one position per ordering
  and the series Collection declares which exist and which is the default. This is
  [D79](../../decisions/working-notes.md)'s disease in a different organ, with the same
  signature in Jellyfin's source: one `IndexNumber`, then three nullable escape hatches
  (`AirsBeforeSeasonNumber`, `AirsAfterSeasonNumber`, `AirsBeforeEpisodeNumber`) for the
  specials that do not fit, and a **bare string** `Series.DisplayOrder` compared against magic
  values with two different comparison modes. Anime makes it unavoidable, and
  [play-state.md](play-state.md)'s "Next Up" is a statement *about* an ordering.
- **An extra is a typed relation, not a level** ([D97](../../decisions/working-notes.md)).
  A trailer is not an Edition (not a cut) and not a Version (not an encoding); it is a shorter
  work belonging to the film. So: a typed relation from a Title or Edition to a work, closed
  enumeration, ordered, never in the main listing. It holds its own Versions and Files, so it
  gets subtitles, loudness, segments and play state by construction. `ThemeSong` and
  `ThemeVideo` are members, which is what makes androidtv's *Video Backdrop and Theme song
  playback* (+37) nearly free.

### And one more table, from 2026-09-21

- **A `role` is a named policy object, and a user has exactly one**
  ([D130](../../decisions/working-notes.md)). `role(id, name, capabilities, predicate,
  defaults)` plus a `role_id` on the user — the predicate stored in
  [rules-engine.md](rules-engine.md)'s serialised form over its field registry, the
  capabilities a closed enum. It is schema and it is Phase 1, because the alternative is what
  Jellyfin has: `UserPolicy.cs` carries **44 settable fields per user and no grouping at all**,
  and their own forum records the consequence — *"no option exists to create groups to assign
  permissions more efficiently"*. **The 44 fields exist because there are no roles**: with no
  way to say "these people are the same", every distinction anyone needed became another
  checkbox on every account. So this table is what keeps the capability column short.

  Two consequences for the schema rather than the UI: the rules engine's **share and profile
  bindings now attach to a role**, not a user, so one evaluation serves every member; and a
  per-user **deny** list is permitted alongside because it can only narrow, while anything that
  grants is a role change.

  **And the admin's own tags become load-bearing** ([D132](../../decisions/working-notes.md)).
  When no provider metadata separates an item, the admin tags it and the rules match on that —
  which scales to every role at once, where a per-role list of item ids would not. So a user
  tag is **user-authored data about the work**: it attaches at the **Title**, is keyed on the
  identity that survives a move, rename, re-rip and provider change
  ([D7](../../decisions/working-notes.md)), rides the change journal, is sidecar-able
  ([D98](../../decisions/working-notes.md)), and **a full rescan must not destroy it** — which
  the fixture list below already asserts, and which now has a second consumer where the failure
  is a child seeing something.

**And one policy that is not a table — and the policy is now "we don't."** Deletion is
**deferred** ([D144](../../decisions/working-notes.md)). It appears **0 times in 120
top-reacted Jellyfin issues and 0 times in Plex's yearly top 50**, and the single relevant
mention among 2,205 App Store reviews is a complaint that a server *failed to notice* a
deletion made outside it. [D89](../../decisions/working-notes.md) came from an audit's
"zero occurrences in the tree", not from demand: it asked *what is our policy*, and **"we
don't" answers that question**.

So there is no schema here, and instead **an enforced absence**: no code path removes a file
beneath a media root, asserted by a test.
[D75](../../decisions/working-notes.md)'s retention binding keeps *marking*; acting on the mark
happens in the acquisition layer, which already does upgrade-and-replace properly.

**What does not defer is overwrite safety**, because
[D141](../../decisions/working-notes.md)'s sidecar export writes into the media tree regardless:
writes are **atomic** (temp plus rename, [D39](../../decisions/working-notes.md)'s own
`.partial` pattern) and **the replaced file is retained once, beside it**. That is the trash in
embryo, justified by a feature we are building rather than one we are not.
[D143](../../decisions/working-notes.md) §3's full trash design stays recorded as **the
precondition** if deletion is ever asked for. The tree previously said nothing at all about deleting media, which is how `os.Remove`
ends up in a handler. The list of files and sizes in that confirmation **is** the identity
guard the node enforces ([D100](../../decisions/working-notes.md)):
[D38](../../decisions/working-notes.md)'s command channel is replayed after a node restart, so
a delete keyed only on a path can destroy a different file that has since appeared there.

## Three cross-cuts from the review

- **Store the node's raw observation, not only the interpretation**
  ([D59](../../decisions/working-notes.md), [D79](../../decisions/working-notes.md)). D59's
  argument for parsing filenames in the core was that a parser improvement must be replayable
  over the database *with every node switched off* — which is only true if the input is in the
  database. So the File level keeps the **verbatim filename and the raw probe output**, not
  just the parse. Provenance says where a value came from; this says keep the input.
- **The field registry carries a level, and cross-level predicates carry a quantifier.**
  `genre = horror AND resolution > 1080` mixes Title- and Version-level fields, so the registry
  records each field's level and the compiler knows the join. And the predicate is **ambiguous
  without a quantifier** — *any* version over 1080, or *all*? Both are wanted ("films I have in
  4K" versus "films I have only in 4K"), so it is explicit, like D75's tag count operators.
- **Mark the backup-critical tables in the schema** ([D77](../../decisions/working-notes.md)),
  so backup is a query rather than a hand-maintained list that goes stale.

## Why it is shaped this way

- Jellyfin 12.0's headline change — in 2026 — is turning playlist and collection membership
  into real rows with real foreign keys. Normalise from the start.
- Multi-version items (1080p and 4K of one film, multiple cuts, multi-part files, disc images)
  are a permanent source of long-running bugs in both Jellyfin and Plex because both bolted
  them on — and the structural cause is the missing Manifestation level, above.
- A change journal cannot be retrofitted cheaply, and offline clients need deletes (D8).
- Identity must survive a file move, a rename, a re-rip and a provider change: UUID primary
  keys, `{provider, provider_id}` in a side table.

## Done when

Schema and `sqlc` queries exist, migrations run forward from empty, and a documented test
fixture library round-trips through scan → query → rebuild-from-empty with no user-visible loss.

The fixture includes **one film held as two parts at 1080p and a single file at 4K, in two
different cuts** — the case that breaks a three-level model — and resume position survives
switching between the 1080p and the 4K.

Three more fixture assertions, from the audit:

- **Two devices reporting progress for one edition cannot rewind each other**, and the losing
  report is still in the log ([D81](../../decisions/working-notes.md)).
- **A full rescan destroys no user-authored data** — not a chosen poster, not a playlist, not a
  manual identification, not a preference. Jellyfin's *All Library Scan causes playlists to
  disappear* has 50 comments; we are structurally safe by
  [D7](../../decisions/working-notes.md), which makes this one assertion rather than a design.
- **Two people with the same name from two providers stay two people**
  ([D95](../../decisions/working-notes.md)).
- **An anime series with a season split lists correctly in both aired and absolute order**, and
  a special that airs between two episodes sits between them in the ordering that says so
  ([D96](../../decisions/working-notes.md)).
- **A film's trailer never appears in the film list** and never looks like a cut
  ([D97](../../decisions/working-notes.md)).
