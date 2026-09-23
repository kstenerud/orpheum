> **ABOUTME:** The core data model — how a title, its editions, its files and its provenance are
> represented — and the first schema, including the change journal.

# Library model

> **The tables themselves are listed in [schema.md](schema.md)** — the Cut A sketch, as SQL that
> runs ([D185](../../decisions/working-notes.md)). This document is the *why*.

- **Status:** PLANNED — designed 2026-09-17; **reviewed and corrected 2026-09-19** ([library-model-review.md](../research/library-model-review.md), [D79](../../decisions/working-notes.md)); **nine more schema items added by the audit the same day** ([audit-2026-09-19.md](../research/audit-2026-09-19.md), [D81](../../decisions/working-notes.md)–[D84](../../decisions/working-notes.md), [D90](../../decisions/working-notes.md), [D95](../../decisions/working-notes.md)–[D97](../../decisions/working-notes.md)), plus the sidecar split ([D98](../../decisions/working-notes.md)), a device scope on the preference table ([D103](../../decisions/working-notes.md)), and **the `role` table** ([D130](../../decisions/working-notes.md), 2026-09-21).
- **Depends on:** —

## Scope

The first schema. Models multi-version and multi-part items as first-class rather than bolted
on, keeps provider IDs and field provenance — which source a title follows, which fields are
manual ([D170](../../decisions/working-notes.md)) — out of the value columns, and includes one
change-journal table that clients may use and must never depend on
([D166](../../decisions/working-notes.md)).

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
| ↳ | **A Version's parent is an Edition, or the Title directly when the edition is unknown** ([D155](../../decisions/working-notes.md)) | `isManifestationOf` may target an Abstraction |
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
collections. **An explicit member list is ordered, and the order may be partial**
([D159](../../decisions/working-notes.md)): the *Machete order* for Metropolis is
**IV, V, II, III, VI** and deliberately **omits Episode I**, so a position is a property of
membership and not every member need have one. **A predicate's sequence is a sort**, which
completes the sentence — `franchise = Friday the 13th` ordered by release date picks up part
12 the day it is ripped, where a hand-authored list would not.

**Every Collection therefore produces a sequence, and that is the only thing anything
downstream consumes** ([D160](../../decisions/working-notes.md)). A series is the Collection
whose members are one work's episodes; a franchise binge is the Collection whose members are
films; **a member may itself be a Collection**, which is what *"Three's Company plus Three's a
Crowd"* is — one thing made of two series, flattened depth-first at read time and staying
correct as new episodes arrive. A Collection may not contain itself transitively; that check
is at write time. **There is no separate "series" concept**, and Next Up is a query over a
sequence rather than over episodes.

### Two levels that must be declared

- **The playback position attaches at the File; completion rolls up**
  ([D157](../../decisions/working-notes.md), superseding this paragraph's original claim of
  the Edition). A File is the only object here with an unambiguous timeline — a baked-in
  studio ident or a PAL transfer's 4.27% speedup makes two Versions of one cut disagree — and
  it is what the player observed ([principles §13](../../principles/README.md)). Completion
  has no timeline, so it rolls up freely to Version, Edition and Title.
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
  expression-depth limit. **Settled 2026-09-22 by
  [D164](../../decisions/working-notes.md): a normalised `tag` + `item_tag`, plus a
  `tag_count` derived column.** Measured at the 3,500-item reference library, the join table
  is 50× faster on a single tag, 30× on a 3-tag AND and 11× on a negation; JSON wins only the
  count operator, and a `tag_count` column wins that outright at **66 µs**, 42× the JSON it
  lost to. Smart playlists, smart collections, and retention policy. The field shows
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
  position** per ~~`(user, edition)`~~ **`(user, file)`** ([D157](../../decisions/working-notes.md)) carrying a monotonic sequence, so a stale device cannot
  rewind a newer one; and an **append-only play-event log** with device, server timestamp and
  the client's own monotonic clock. Jellyfin keeps one mutable row and its tracker carries
  *"Most movies now appear to be 75% watched"* (44 comments) and *"Media playback randomly
  starts over again or jumps back — STILL"* (29). The log is also the only table with an
  unbounded row count, ~~so it carries a retention horizon from the first migration~~ — **and
  keeps everything: spans, not heartbeats, make five years 13.4 MB** ([D165](../../decisions/working-notes.md)).
  **A position is File-relative** ([D157](../../decisions/working-notes.md)); each File records
  its offset within the Version's ordering for *display* only, and nothing above the File
  holds a position.
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
- **What makes something an extra is containment, and a name only types it**
  ([D162](../../decisions/working-notes.md)). Jellyfin carries **34 rules over 12 types** —
  15 directory names, 3 filenames, 18 suffixes — matched wherever a file appears. We admit
  two evidences and both require being *inside a title's own folder*: a subdirectory in the
  closed vocabulary, or a file whose stem is the title's name plus a vocabulary suffix. The
  asymmetry is the argument: an extra shown as a film is clutter, a **film classified as an
  extra vanishes silently**. Nothing at a library root is ever an extra — which is why a
  library of short films works, and why Jellyfin needs the `parent is not UserRootFolder`
  carve-out that we do not. An unmatched type is `Unknown` and the relation still holds:
  *filed, not named*, as for an unknown cut.
- **`Sample` is not one of the types.** It is release debris, it is skipped and reported as
  skipped ([scanner.md](scanner.md)), and Jellyfin's own source is incoherent about it —
  `IgnorePatterns` drops `x.sample.mkv` while `VideoExtraRules` turns `x-sample.mkv` into a
  library item. Eleven types.
- **The scanner attaches an extra to a Title; only a person attaches one to an Edition.**
  `Wings (1986)/deleted scenes/` sits above both cuts and the layout cannot say which owns
  it — the machine groups, the human names ([D158](../../decisions/working-notes.md)). A
  relation the scanner inferred is an index; one a person asserted, or a type they corrected,
  is true about the work and **mirrors to a sidecar**
  ([D98](../../decisions/working-notes.md)).
- **An extra is not always a video.** `ThemeSong` is audio, and trickplay, poster derivation,
  segment detection and the speech map are meaningless on a 90-second `.mp3`, so the pass set
  gates on the item's **media kind** and not only on the library
  ([D102](../../decisions/working-notes.md)).
- **And a provider's trailer URL is not an extra at all** — no Version, no File, no hash,
  nothing for a rebuild to recover, because it was never in the files. It is metadata about
  the work, and following it is opening a link rather than playback.

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
[D141](../../decisions/working-notes.md)'s sidecar export writes into the media tree
regardless — and that gives the trash its first real consumer
([D145](../../decisions/working-notes.md)):

```
<library root>/.trash/2026-09-21T143205Z-export/
  manifest.json
  films/Glass Runner (1982)/Glass Runner (1982).nfo
```

**One directory per event, with the library's own structure mirrored inside**, so restoring an
entire export is `cp -a` rather than 3,400 individual moves — the unit of undo matching the
unit of action. Repeated replacement needs no versioning scheme: the same relative path
appears under several event directories and the chronology is the name. One
`manifest.json` per event carries when, who, which operation, and per-file size and hash;
the path already carries *where*.

Three rules that make it work: **retain only when the bytes actually differ**, or every export
trashes thousands of unchanged files; the write sequence is **temp → copy the old file into
the trash → atomic rename** ([D146](../../decisions/working-notes.md)), because what we retain
is kilobyte sidecars and a hardlink would buy a dependency on `link(2)` — absent on **exFAT**
and variable over **SMB**, both of which are in our path — to save copying nothing; and
**`cp -a` is a guarantee while our own restore trashes the current file first**, because
restoring an old version over a newer one would destroy something that was never retained.

Independently confirmed by two prior arts — the **freedesktop.org Trash Specification**, which
is also per-volume and also records original path plus timestamp, and **Syncthing's
`.stversions`**, which versions *replaced* files exactly as we do. Both are built for one file
at a time, which is why we depart from them.
[D143](../../decisions/working-notes.md) §3's wider trash design stays recorded as **the
precondition** if deletion is ever asked for. The tree previously said nothing at all about deleting media, which is how `os.Remove`
ends up in a handler. The list of files and sizes in that confirmation **is** the identity
guard the node enforces ([D100](../../decisions/working-notes.md)):
[D38](../../decisions/working-notes.md)'s command channel is replayed after a node restart, so
a delete keyed only on a path can destroy a different file that has since appeared there.

### And two more from the subtitle work, 2026-09-21

- **A subtitle track carries provenance and a verification result**
  ([D147](../../decisions/working-notes.md)). `origin` (embedded / sidecar / fetched),
  `provider`, `verified_score`, `applied_offset_ms`, `applied_ratio`, and whether a human
  overrode it. Two of those are load-bearing rather than diagnostic: **a track that was never
  verified must be distinguishable from one that scored 0.6 everywhere it is offered**, because
  the whole point is not to present an unchecked guess as a checked one; and a **user-set
  offset outranks our measurement**, so it has to be a different column from the one we
  computed rather than an overwrite of it. Note this applies to tracks we did **not** fetch —
  an embedded track can be out of sync too, and scoring it costs 8 ms.
- **The speech map is an artefact at the Version**
  ([D148](../../decisions/working-notes.md)). A 10 ms voice-activity bitmap, run-length
  encoded — **measured at 143–331 bytes for a 12–15 minute film, 1–3 kB for a 2-hour one, and
  1.2–3.2 MB for a thousand-film library**, against trickplay's 5.7 GB. It sits with the other
  scan artefacts, is invalidated by the same signal, and is **derived**, so
  [backup-restore.md](backup-restore.md) does not carry it. It exists so that verifying a
  subtitle months after the scan does not require waking a node and decoding a film:
  with it stored, the full offset-and-framerate search is **3–8 ms**
  ([principles §11](../../principles/README.md) — maintained, not computed).

### And three from the 2026-09-21 source sweep

[D149](../../decisions/working-notes.md).

- **External critic and audience scores are missing from the schema.** This document has
  favourites and **user** ratings in the play-state family and no external score anywhere —
  and neither does any other document in the tree. Maintainerr carries **twelve** such
  properties and most of Kometa's 129 collection builders rest on them. So: at the **Title**,
  `(source, kind, score, vote_count, as_of)` — `source` ∈ TMDB and OMDb's IMDb, Rotten Tomatoes and Metacritic ([D172](../../decisions/working-notes.md): easy data only),
  `kind` distinguishing **critic from audience**, which Rotten Tomatoes reports separately and
  which a single column would destroy. The vote count is what makes the score usable, and
  `as_of` is what makes a refresh honest. They are provider data, so they carry provenance
  like everything else and a user override wins. **Each score source is opt-in per library** —
  it is the one place several sources are fetched per title, and OMDb's free key allows 1,000
  calls a day ([D170](../../decisions/working-notes.md)). **Shown by a ranked list** ([D171](../../decisions/working-notes.md)):
  a tile shows the first source with a score, named; the detail screen may show several; each on
  its own scale **with its vote count** — *IMDb 9.6 (11)* ([D172](../../decisions/working-notes.md)); a sort or rule names one
  source and kind, never the fallback.
- **A home row is an object, and it has an audience.** Plex's `ManagedHub` promotes a
  collection to the home screen with `homeVisibility ∈ {none, all, admin, shared}`. We model
  row *order* as user state ([D138](../../decisions/working-notes.md)) and have nowhere to say
  *which rows exist for which role*. That is a small table — a row is a Collection plus a
  role scope plus an order — and it is schema, so it is due here rather than in the client.
- **Artwork has a type.** [artwork.md](artwork.md) specifies sizes, languages and caching and
  never enumerates poster / backdrop / logo / banner / disc. Closed enumeration, at the level
  the image belongs to.

### Episode orderings, from the 2026-09-22 re-read of D96

[D159](../../decisions/working-notes.md).

- **The ordering choice is a [D24](../../decisions/working-notes.md) preference at series
  scope, not a column.** The series declares which orderings **exist** — a fact from
  providers, one row each. The user chooses which they **use**. Two people watching one anime
  in different orders costs nothing and adds no column.
- **An ordering may be user-authored**, and a rescan or provider refresh never overwrites it
  ([D141](../../decisions/working-notes.md)'s lock markers). Same table as a provider's,
  different provenance.
- **A multi-episode file holds one position and an `episode_boundary` segment.** *N* works in
  one file is the inverse of multi-part, and the boundary is a timecode inside that file, so
  per-episode completion is **derived** rather than stored
  ([media-segments.md](media-segments.md)).

### And three from the EIDR re-check, 2026-09-22

[D155](../../decisions/working-notes.md), read from EIDR's published XSDs rather than a
summary of them.

- **Edition is optional, and that is the schema change.** A Version's parent is an Edition
  **or the Title**. Most rips carry no edition marker, so *"Glass Runner, cut unknown"* is the
  ordinary case, not the exotic one — and forcing it through a synthetic *"Unknown Edition"*
  row means that when [identification.md](identification.md) later learns the cut, we must
  **merge a real Edition into a fake one and carry watch state across**. This tree already
  treats unknown as a first-class state everywhere else —
  [D131](../../decisions/working-notes.md)'s tri-valued descriptors,
  [D132](../../decisions/working-notes.md)'s refusal to judge an unknown item,
  [play-state.md](play-state.md)'s refusal to mark an item with unknown runtime as watched —
  and a synthetic row is the structural form of the same mistake. EIDR gets this right by
  construction: `isManifestationOf` may point straight at an Abstraction.
  - **The playback position is stored on the File** ([D157](../../decisions/working-notes.md),
    superseding D156) — the only object here with an unambiguous timeline, since a baked-in
    studio ident or a PAL transfer's 4.27% speedup makes two Versions of one cut disagree.
    **Completion rolls up** to Version, Edition and Title, because it has no timeline. So
    naming a cut later **moves no state at all**, and getting the Edition wrong costs a
    misleading group rather than somebody's place in a film.
  - **An Edition may be created and named by the user, and a rescan never overwrites it**
    ([D158](../../decisions/working-notes.md)). Someone who hand-deletes the opening scenes of
    *Wings* has made a cut that matches no release, no runtime and no provider record — it is
    theirs, and it is user-authored data under the same rule as any other correction
    ([D7](../../decisions/working-notes.md), [D141](../../decisions/working-notes.md)'s lock
    markers). Provider-derived and user-created Editions are the same row with different
    provenance.
  - **Two Versions under one Title with no Edition are not known to be the same viewing**, and
    are presented as **separate playable entries** distinguished by runtime, resolution and
    the verbatim filename — never as a quality menu, and never as a row called *"Unknown
    Edition"*.
  - **The speech map answers the grouping question.** Measured
    ([D156](../../decisions/working-notes.md)): two Versions of the *same* cut score
    **r = 1.000** against each other and nothing else comes close — a removed scene lands at
    0.36–0.71, an added voice-over at 0.19–0.41, a different film at 0.10. So the scanner may
    **propose** a merge at 1.0 and must never propose a *name*: the machine groups, the human
    names.
- **The extras vocabulary is EIDR's**, not ours ([D97](../../decisions/working-notes.md)):
  `isClipOf`, `isPromotionFor`, `isSupplementTo`, `isAlternateContentFor`, `isCompositeOf`,
  `isPackagingOf`. D97 independently chose EIDR's *shape* — a typed relation rather than a
  level — so the names are free.
- **Episode orderings stay ahead of EIDR.** `episodeInfoType` carries one `Parent` and one
  `SequenceInfo`, so **EIDR expresses exactly one ordering**;
  [D96](../../decisions/working-notes.md) keeps several per episode because anime requires it.
  Recorded here so that "align with EIDR" is never read as a reason to undo it — a registry
  needs one canonical sequence, a player needs the one its user expects.

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
- **No row a user's history or authored data references is ever hard-deleted**
  ([D167](../../decisions/working-notes.md)). A File whose media is gone is **tombstoned**,
  because [D165](../../decisions/working-notes.md) keeps the play log for ever and a device
  returning from a trip syncs spans that reference it. Free to keep true now; a data-loss
  migration to recover later, which is [principles §12](../../principles/README.md)'s
  **invariant** row exactly. **Mechanics** ([D173](../../decisions/working-notes.md)): `ON DELETE RESTRICT` on every
  reference from history or authored data; `AUTOINCREMENT` on referenced tables so an ID is
  never reissued; partial unique indexes over live rows; live-only views as the default read;
  `merged_into` instead of deleting a merged Title; purge removes a tombstone *with* its
  references, never alone.
- **A replaced file is a deletion and an addition** ([D173](../../decisions/working-notes.md)). Anything expressed in seconds
  of a file — position, spans, sync offsets, segments, chapters — stays with its tombstone;
  anything about the work — watched, rating, tags, identification — survives at the Title. The
  new file joins no Edition until the user puts it there.

## Why it is shaped this way

- Jellyfin 12.0's headline change — in 2026 — is turning playlist and collection membership
  into real rows with real foreign keys. Normalise from the start.
- Multi-version items (1080p and 4K of one film, multiple cuts, multi-part files, disc images)
  are a permanent source of long-running bugs in both Jellyfin and Plex because both bolted
  them on — and the structural cause is the missing Manifestation level, above.
- A change journal is cheap to have and **must never be depended on**
  ([D166](../../decisions/working-notes.md)): the whole browsable state is 215 KB gzipped, so
  the server may always answer with state and the journal is an optimisation below ~2,600
  changed items. One journal table, permitted to be lossy — not a paired audit table per
  syncable table.
- Identity must survive a file move, a rename, a re-rip and a provider change:
  ~~UUID primary keys~~ **integer keys never reissued**, `{provider, provider_id}` in a side
  table. **The keys are backed up although the index is not**, and a rebuild is an ordinary
  scan matching observations against them by **payload hash**
  ([D179](../../decisions/working-notes.md)).

## Done when

Schema and `sqlc` queries exist, migrations run forward from empty, and a documented test
fixture library round-trips through scan → query → rebuild-from-empty with no user-visible loss.

The fixture includes **one film held as two parts at 1080p and a single file at 4K, in two
different cuts** — the case that breaks a three-level model — ~~and resume position survives
switching between the 1080p and the 4K~~ **and each file keeps its own resume position, while
completing either marks the film watched** ([D157](../../decisions/working-notes.md),
[D158](../../decisions/working-notes.md), [D173](../../decisions/working-notes.md): two files'
timelines are never related).

Three more fixture assertions, from the audit:

- **Two devices reporting progress for one file cannot rewind each other**, and the losing
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
