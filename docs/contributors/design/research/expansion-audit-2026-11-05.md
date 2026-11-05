# The expansion audit of the API and schema (2026-11-05)

> The owner asked for one more audit of the API (D416, D425) and the schema (D426, steps 0072–0080)
> before the deploy that makes every later schema change an appended migration. The audit checks
> them against expert practice, every planned feature, music, and the unwanted photos and live TV.
> This document proposes and decides nothing. Each finding says what change it proposes now and
> what churn that saves later, or why extending later is cheap. Relocation (D429 §4) is audited
> separately and is not covered here. Read at schema v0080 and `api/openapi.json` on main at
> "docs(plans): defer the API compatibility gate, with how to build it later".

## 1. Findings, ranked by churn saved

| # | Finding | Proposed now | Churn saved later |
| --- | --- | --- | --- |
| 1 | `kind=any` means "every kind", so every "all libraries" list fills with tracks, photos or recordings when they arrive (§3.1) | `kind` takes a list of kinds; `any` goes, or is defined as today's three kinds by name | A change of meaning to a value every deployed client sends; otherwise a new value plus a deprecation |
| 2 | A profile is not yet decided as a `user` row (§5.1) | Record that a profile is a `user`; no code | Re-keying six or more per-person tables, four of them `WITHOUT ROWID` on `user_id` |
| 3 | Ten closed `CHECK` vocabularies must widen for music, collections, photos or DVR, and SQLite cannot alter a `CHECK` (§4.1) | Choose the route now: write down the `writable_schema` widening in sql.md, or widen the central lists in a step 0081 | Up to four rebuilds of `item`, the table with 19 foreign keys and 14 triggers pointing at it |
| 4 | Response enums, the `kind` discriminator and all 272 schemas are closed (`additionalProperties: false`) (§3.2) | Say in the document that responses are open; mark kind and code enums extensible; stop closing response schemas | Each new kind or field becomes a breaking change once native or third-party apps exist |
| 5 | `/libraries/{id}/folders` and `/nodes/{name}/folders` return a whole folder (§6) | A cursor and limit on the library folder, and a limit on the node folder | Adding paging to an unpaged list later truncates old clients silently |
| 6 | `GET /needs-attention` carries two unbounded arrays inside a count response (§6) | Counts only, or a cap plus a count | Small: a change of meaning to two fields |
| 7 | Music's tables fit by rows and kinds, except one file holding many tracks (CUE), deliberately out (§4.2) | Nothing beyond finding 3 | — |
| 8 | Live TV needs a playback without a file; DVR recordings fit as files (§5.3) | Leave it | Making `fileId` optional is a compatible request change |
| 9 | `202` starts return no job id or location (§7) | Leave it | Adding a field is additive |
| 10 | Rows carry `revision` but the API has no ETag or `If-Match` (§7) | Leave it | An optional header is additive |
| 11 | Collections of series or mixed items, playlists with repeats, the watchlist, segments, trickplay (§5.2) | Leave it | Each is a new table |

**Before the deploy, in my view:** findings 1, 2 and 4 cost a parameter, a sentence and a
generator setting now, and a client-facing break later. Finding 3 needs only a choice of route
now. Finding 5 is cheap now and breaks clients later.

## 2. What was read

- The API: `api/openapi.json` (137 paths, 162 operations, 273 schemas) and
  [api-greenfield-2026-11-04.md](api-greenfield-2026-11-04.md), whose §2 already lists what music
  asks of the API.
- The schema: `internal/store/testdata/golden/schema-v0080.sql` and
  [schema.md](../plans/schema.md): Items, the work, series and orderings, play state, and "Not in
  Cut A, and additive".
- Decisions D186 (music's one reshape, made then), D416, D425, D426, D429, D430 and D431.
- [closed-beta.md](../plans/closed-beta.md) B0–B5 and "After the closed beta", and
  [inventory-2026-10-27.md](../plans/inventory-2026-10-27.md) §12 ("Later", "Maybe", "Not
  wanted").
- [music.md](music.md) and D186 for the earlier music research.
- Nine web searches; fetched pages are cached in `/yoloai/cache/expansion-audit/pages/`.

## 3. The API

### 3.1 `kind=any` grows with every new kind

`GET /items`, `GET /items/letters` and `GET /genres` take `kind` as `movie`, `series`, `other` or
`any`. The parameter's own words: "any: every kind, every library's". The web client sends `any`
whenever "All libraries" is chosen (`library-browse.tsx`, `alphabet-rail.tsx`). When music arrives,
"every kind" includes albums and tracks; with photos, every photo. A film browser built against
today's API would show half a million tracks in its "All" view.

**Proposed now:** `kind` takes a list (`kind=movie&kind=series&kind=other`), and `any` is dropped.
Alternatively `any` is defined as today's three kinds by name, and a new kind is never added to it.
**Saves:** a change of meaning to a value that every deployed client sends. Later, the choice is a
silent change for every client or a new value plus a deprecation of `any`.

### 3.2 Closed response enums and closed objects

- **The enums.** `TitleEntryBody.kind`, `TitleBody.kind` and `PlaysBody.kind` are
  `movie|episode|extra|other`. `GroupHitBody.kind` and `LibraryBody.kind` are
  `movie|series|other`. `ProblemBody` is a `oneOf` of 8 codes. In all, 110 enums.
- **The discriminator.** `/items` and `/items/{id}` return a `oneOf` whose `kind` mapping names
  exactly five kinds. A strict generated client fails on `album`.
- **The objects.** All 272 component schemas say `additionalProperties: false`, responses
  included.

Expert practice says responses stay open:

- Zalando: "API formats must not declare additionalProperties to be false", and clients "must
  ignore fields sent by the server they cannot process" (rule "MUST treat OpenAPI specification as
  open for extension by default"). It also says to use open-ended lists for output enums "likely to
  be extended with growing functionality".
- Microsoft's Azure guidelines: "YOU SHOULD use extensible enumerations unless you are positive
  that the symbol set will NEVER change", and "YOU SHOULD define the discriminator field of a
  polymorphic type to be an extensible enum".
- Google AIP-180: enum values "used in response messages … and which are expected to receive new
  values should document this".

Today the only client is our own web client, which does no run-time validation, so nothing breaks
yet. The cost arrives with the "Later" list's native phone apps, store apps for TVs and a
Jellyfin-compatible adapter. From then on, each music, photo or DVR kind is a breaking change for
them.

**Proposed now:** say in `info.description` that response objects and enums are open, so a client
ignores unknown fields and skips items of unknown kinds. Mark the kind and error-code enums
`x-extensible-enum`. Stop emitting `additionalProperties: false` on response schemas, keeping it on
request bodies, which Zalando's same rule lets a server reject. **Saves:** a `/api/v2`, or
per-client aliases, for each expansion once outside clients exist.

### 3.3 Departures that matter little

- **Errors** are `{"error": {"code", "message", "detail"}}`. That is Microsoft's error shape, not
  RFC 9457's `application/problem+json`. Both are recognised practice. Leave it.
- **Paging** is an opaque `cursor` and a `limit` of at most 500, with `nextCursor` absent at the
  end. That is Google AIP-158's page-token pattern. It conforms.
- **Versioning** is `/api/v1` in the path. That is recognised practice. It conforms.
- **Arrays may be `null`** (`["array","null"]`), against Zalando's "SHOULD not use null for empty
  arrays". Tightening a response to `[]` later is compatible. Leave it.
- **Search's groups** are three required keys (`titles`, `people`, `genres`). Music's `albums` and
  `artists` would be new optional keys, which is additive. Leave it.

## 4. The schema and music

### 4.1 Ten closed vocabularies

sql.md's convention is "TEXT with a CHECK (… IN (…))" for a closed vocabulary. These ten close
what an expansion must open:

| Column | Today | Music adds | Photos, DVR, collections add |
| --- | --- | --- | --- |
| `item.kind` | movie, series, episode, extra, other, person | album, track, perhaps artist | collection, playlist, photo, channel |
| `work.kind` | movie, episode, extra, other | track | photo, perhaps recording |
| `sequence.kind` | series | album | collection |
| `library.kind` | movie, series, other | music | photo, perhaps dvr |
| `file_filing.library_kind` | movie, series, other | music | as `library.kind` |
| `item.followed_source` | nfo, tmdb, tvdb, none | musicbrainz, and the files' own tags | a guide's source |
| `item` identification `CHECK` | only `person` is unidentified | perhaps artist | playlist, collection, channel |
| `credit.job` | cast, director, writer | artist, album artist, composer, performer, featured | — |
| `file.sidecar_kind` | nfo, subtitle, image | lyrics (`.lrc`), cue | — |
| `artwork.kind` | poster, backdrop, logo, banner, still, disc | a square cover, perhaps as `poster` | — |

D186 §3 foresaw "two CHECK lists widened". D426 §2 already names "album, artist and track later".
The rebuild since then put a kind on `item` and `CHECK`s on each subtype, so it is now ten lists.

SQLite cannot alter a `CHECK`. Its documentation gives two routes
([lang_altertable.html](https://www.sqlite.org/lang_altertable.html)):

1. **The 12-step rebuild**: create, copy, drop, rename, and recreate indexes, triggers and views.
   For `item`, that means 19 foreign keys referencing `item(id)`, the subtypes' `(id, kind)` keys,
   the 14 triggers that keep `item_found` and the search indexes, and 4 views. Step 0072 did exactly this in 315 lines.
2. **A `writable_schema` edit**: documented for "removing CHECK or FOREIGN KEY or NOT NULL
   constraints", changes "that do not affect the on-disk content in any way". Widening an `IN`
   list only loosens the constraint, so it qualifies. SQLite warns that an error corrupts the
   database, and asks for testing on a copy first.

`credit` is class C, derived by the pass, so dropping and re-deriving it is cheap either way. The
other nine hold identity or a person's choices.

**Proposed now, one of:**

- **(a) Write the route down.** sql.md adds the `writable_schema` widening as the sanctioned way to
  add a value to a closed vocabulary. The migration tests' fresh-against-migrated equality and
  `integrity_check` guard it. This costs nothing now and makes each later widening a few lines.
- **(b) Step 0081 widens the four central lists now** (`item`, `work`, `sequence`, `library`
  together with `file_filing`), with the words D426 §2 already chose. It also rewrites the
  identification `CHECK` as a list of the kinds that are not identified. This costs one rebuild
  in this batch, rehearsed with D430's rehearsal, and leaves values that no code writes yet.
- **(c) Reference tables** (`item_kind(name)` with foreign keys) in place of `IN` lists. Adding a
  kind becomes an `INSERT`. This is the larger change, and the Go constants remain the vocabulary
  either way.

**Saves:** with no route, music, collections, photos and DVR could each cost a rebuild of `item`
in an append-only world.

### 4.2 Music fits by rows

Music maps onto the current model as follows:

| Music needs | Where it goes | Fit |
| --- | --- | --- |
| Album | `item` kind `album`, `sequence` kind `album`; `ordering.name` `'release'` (schema.md already says so) | rows, plus finding 3 |
| Disc and track numbers | `grp` is the disc and `position.number_in_group` the track (D426 §5); a disc-less album has `grp_id` NULL | rows |
| Track | `item` and `work` kind `track` → `version` → `file`, filed through `file_filing` | rows, plus finding 3 |
| Album artist and track artist | `credit` on the album item and on the track item; the composite key `(item_id, job, ordinal)` holds many per job | rows; `credit.job` widened |
| Multi-artist credits, join phrase, "credited as" | two nullable columns on `credit` (`ADD COLUMN`) | additive |
| Compilations, release type, label, barcode | an album side table (music.md §5) | additive |
| Artists as entities | `person` with a MusicBrainz id in `item_provider_id`, or a kind `artist` | open question (below) |
| Several ISRCs per recording | `item_provider_id` (D186 §2.4) | rows |
| Release date with only a year or a month | `year` alone, or `released_on` with zero day/month (the `CHECK` admits 19770600) | fits |
| ReplayGain | `loudness` already holds EBU R128 integrated loudness and true peak per payload and stream, which ReplayGain 2.0 is computed from; tag-supplied gains are nullable columns | additive |
| Gapless | the encoder delay and padding are in `file_probe.probe_json`; `preferredDelivery: direct` exists | API work, no reshape |
| Lyrics | a table keyed on payload hash, as `subtitle_text` is; `.lrc` needs `sidecar_kind` widened | additive, plus finding 3 |
| Audio-only play state, play counts | `play_position` per file, `play_event` and `play_span` per file; `work_watched` per track | rows |
| Scrobbling | `play_span` and `play_event` give Last.fm's "half or four minutes"; the token is a user-scope `secret` | additive |
| Playlists that repeat a song | a new table; `position`'s key `(ordering_id, work_id)` forbids repeats (music.md §5) | additive |
| Queues | server-side, a new resource; casts' `next` is in memory today | additive |
| Retagging without losing state | D179's payload hash survives Picard and beets (D186 §4) | fits |
| 500,000 tracks | `/items` is paged; `sort_key` carries its listing (D186 §2.3) | fits |

**Reshapes:**

- **One file holding many tracks** (a CUE sheet over one FLAC). `file_filing`'s key `(file_id,
  library_kind)` gives a file one version, and `play_position` is per file. music.md §4 excludes
  it deliberately, and Navidrome, the largest OpenSubsonic server, still lists none in its 2025–26
  releases. Leave it.
- **The word `work`.** In MusicBrainz a Work is a composition, and D186 §3 lists "`work` for
  classical". D426 §3 has since given `work` to the playable thing. A classical-composition table
  would need another name. Note it; there is nothing to change.

**Open question for when music comes:** D426 §2 lists `artist` as an item kind, while
api-greenfield §6 recommends artists under `/people`, with the credit naming the job. Neither
touches a current table beyond finding 3.

## 5. Planned and unwanted features

### 5.1 Profiles (B2) are the one planned reshape

The per-person tables key on `user_id`: `play_position`, `play_event`, `play_span`,
`work_watched`, `item_hidden`, `preference`, `fetched_subtitle.fetched_by_user_id`, and the user
scope of `secret`. If a profile were a new entity under a user, each would gain a profile column.
Four of them (`play_position`, `work_watched`, `item_hidden`, `preference`) are `WITHOUT ROWID`
with `user_id` leading the primary key, so each would need a rebuild. The inventory marks profiles
"decided" from D150, but how a profile is stored is not written down.

**Proposed now:** record that a profile is a `user` row, with a household or manager link added as
a nullable column when the picker is built, and the PIN as a credential. Plex Home's managed users
and Jellyfin's users work this way (from knowledge, not checked this round). **Saves:** six or more
re-keyed tables.

### 5.2 The rest extend

| Feature | What it needs | Why extending is cheap |
| --- | --- | --- |
| Next Up, skip intro and credits, countdown (B1) | media segments keyed on payload hash, as `video_timeline` is | new table |
| Trickplay, chapters (B1) | `cache.db` images; chapters from the probe | new table or cache |
| Genres, browse filters (B2) | `item_genre` exists; `/items` takes `genre`, `decade`, `person` | rows, new parameters |
| Watchlist (B2) | a per-user table (dropped in D426 §8, returns with its feature) | new table |
| Episode orderings, missing episodes (B2) | `ordering`, `grp`, `position`; a work with no version is valid (schema.md obligation #1) | rows |
| Disc folders, cd1/cd2 (B2) | `file_filing.part_index` | rows |
| Second factor (B2), OIDC (Maybe) | `grant` kinds, under `/session` and `/me` | additive |
| Language defaults, night mode, caption style (B3) | `preference` axes are free text | rows |
| Collections, box sets, a person's own (Later) | `position.work_id` holds only works, so a collection of series or mixed items needs a member table keyed on `item` (D426 §6 removed `collection_member`); `sequence.kind` widens | new table, plus finding 3 |
| Cast, crew and ratings; parental controls; limits per role (Later) | a certification table, a user or role column, a filter in the reach | additive |
| Metadata editor, NFO write (Later) | `field_override` | rows |
| Play statistics, Trakt (Later, Maybe) | `play_span`, `play_event` | reads |
| Scoped API keys (Later) | capabilities per operation already declared (D425) | rows |
| Offline downloads, native apps (Later) | a lease beside `/playback`; finding 4 | additive |
| Second media node, several cores (Later) | `node`; opaque item ids (D174) | rows |
| Watch Together across episodes, bigger groups | in memory (D119) | none |

### 5.3 Live TV and DVR

Demand does not track cable. In 2026 about 19–20% of US TV homes are antenna-only (Nielsen's July
2026 universe estimates via TVB; MoffettNathanson, Q1 2026), up from 14% in Nielsen's 2018 figure.
Plex's and Jellyfin's DVRs serve antennas through tuners such as HDHomeRun, and IPTV playlists.
Plex's Live TV & DVR forum still shows active tuner threads in September and October 2026. Cable's
decline may therefore raise DVR demand rather than end it.

What it would need:

- **Recordings** are files a node writes into a writable library (`library.is_writable` exists),
  filed as episodes or films. They fit, once finding 3 allows a `dvr` library kind, if one is
  wanted at all.
- **Channels and the guide** are new tables. The guide is refetchable and churns, so it belongs in
  `cache.db`.
- **A recording rule** names a series item. That is a new table.
- **Watching live** has no file. `StartPlaybackInputBody` requires `fileId`. Accepting a channel as
  an alternative is a compatible change to a request.

**Leave it:** no room is needed now beyond findings 1, 3 and 4.

### 5.4 Photos

The nearest model is the other library (D349): folder browsing, a recorded time from a tag, a name
or the file (`other_video.recorded_from`), file-name sort, and no online identification. A photo
is a `work` of a new kind in a library of a new kind (finding 3). EXIF and place are a new table,
and thumbnails are artwork-cache entries. Play state does not apply. The one thing to fix now is
finding 5: a phone's camera folder holds thousands of files, and the folder listing is unpaged.

## 6. The unpaged lists against music, photos and DVR

| List | Bound today | With music, photos, DVR | Flag |
| --- | --- | --- | --- |
| `/backups` | the retention count | unchanged (each backup is larger) | small |
| `/libraries`, `/nodes`, `/roles`, `/secrets` | what an admin made | unchanged | small |
| `/me/devices`, `/users/{id}/devices` | a person's devices | unchanged | small |
| `/now-playing`, `/me/casts`, `/me/together` | live sessions | unchanged | small |
| `/items/{id}/candidates` | a provider's result page | MusicBrainz's page is as bounded | small |
| `/files/{id}/waits` | a file's few jobs | unchanged | small |
| `/system`, `/config` | the settings catalogue | grows with features, not media | small |
| `/items/letters` | a script's index letters | unchanged | small |
| `/home` (`continueWatching`) | `continue_watching_max_works` | unchanged; tracks need excluding by kind, as extras are | small |
| `/needs-attention` | counts, plus `filmsWithoutSound` and `seriesReadWrongly` arrays with no cap (`needs_attention.go`) | photos would count as videos without sound, unless excluded | **flag (finding 6)** |
| `/libraries/{id}/folders` | one folder's subfolders and videos, with no cap (`library/folders.go`) | a camera folder holds thousands; a flat music folder holds thousands | **flag (finding 5)** |
| `/nodes/{name}/folders` | one folder's subfolders, when choosing a root | an artist-per-folder music root has thousands of subfolders | **flag (finding 5)** |

Jellyfin's own roadmap lists "fix client-side enumeration for large libraries". That is this
churn, paid after release.

## 7. Expert practice: the rest

**SQLite:**

- Every table is `STRICT`, and the small keyed tables are `WITHOUT ROWID`. That conforms.
- `AUTOINCREMENT` is used widely. SQLite's documentation asks for it only when needed. It is
  needed here: ids appear in URLs and client caches and must never be reused. That conforms.
- Foreign keys are `RESTRICT`, and rows are tombstoned rather than deleted. A few child columns
  have no index (`item.merged_into`, `extra_relation.extra_work`, `item_hidden.item_id`). SQLite's
  foreign-key page recommends one, but it matters only when a parent is deleted, which tombstones
  avoid. Leave it.
- Views and triggers (`series_member`, `item_found`) carry no data. An album's equivalent is a
  `DROP VIEW` and `CREATE VIEW` away. Leave it.

**The API:**

- **Long-running starts** (`POST /backups`, `/libraries/{id}/scan`, `/items/{id}/refresh`, …)
  answer `202` with a flag but no job id or `Location`. RFC 9110 suggests a 202 point at a status
  monitor, and AIP-151 returns an operation. Adding a `jobId` later is additive. Leave it.
- **Concurrency:** rows people edit carry `revision` (sql.md M1), used inside the server's
  compare-and-set. The API offers no ETag or `If-Match`, so two admins editing a role overwrite
  each other. An optional header is additive later. Leave it.

## 8. The competitor gap, re-checked

The pasted table is dated. These are the points that change weight:

- **Plex** extended its remote-play paywall in 2026 to Fire TV and smart TVs (from 29 April) and to
  third-party API clients. It added a $1.99 a month Remote Watch Pass. Music and photo libraries
  are exempt, which says Plex sees them as what keeps users. Live TV and DVR remain.
  This raises the value of native and TV clients on our free remote path, and so of finding 4.
- **Jellyfin** shipped trickplay and lyrics in 10.9, and media segments and selectable lyrics
  providers in 10.10. In 10.11 (October 2025) it moved to EF Core, one database and built-in
  backups. Intro skip and trickplay are now baseline, which B1 plans; both are additive here.
- **Emby** 4.9 added books, spoiler hiding, mixed film-and-series tabs and an A–Z picker for
  collections and playlists. Books and audiobooks are music's neighbours, and resume per file
  already fits them.
- **Navidrome** (0.55+) supports multiple artists per album and track, and "participants". This
  confirms that credits need a join phrase and several artists per job, which `credit` can take
  additively. It still has no CUE support.

None of this changes the ranking. It strengthens finding 4 (native clients are the market's
pressure point) and finding 3 (music and DVR are live features at all three competitors).

## Sources

- SQLite, [ALTER TABLE](https://www.sqlite.org/lang_altertable.html),
  [AUTOINCREMENT](https://www.sqlite.org/autoinc.html),
  [foreign keys](https://www.sqlite.org/foreignkeys.html).
- Zalando, [RESTful API guidelines](https://opensource.zalando.com/restful-api-guidelines/).
- Microsoft, [Azure REST API guidelines](https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md).
- Google, [AIP-180](https://google.aip.dev/180), [AIP-126](https://google.aip.dev/126),
  [AIP-158](https://google.aip.dev/158), [AIP-151](https://google.aip.dev/151).
- Plex, [Important 2025 Plex updates](https://plex.tv/blog/important-2025-plex-updates/);
  [Changes coming to remote streaming on smart TVs](https://forums.plex.tv/t/changes-coming-to-remote-streaming-on-smart-tvs/937015);
  [Live TV & DVR forum](https://forums.plex.tv/c/livetv-dvr/127).
- Jellyfin, [10.9.0 release](https://jellyfin.org/posts/jellyfin-release-10.9.0/),
  [10.11.0 release](https://github.com/jellyfin/jellyfin/releases/tag/v10.11.0),
  [Jellywatch on 10.11 and the roadmap](https://jellywatch.app/blog/jellyfin-current-version-latest-features-march-2026).
- Emby, [releases](https://github.com/MediaBrowser/Emby.Releases/releases).
- Navidrome, [releases](https://github.com/navidrome/navidrome/releases).
- TVB, [OTA, July 2026](https://www.tvb.org/wp-content/uploads/2026/08/OTA_July_26_LandingLink.pdf).
