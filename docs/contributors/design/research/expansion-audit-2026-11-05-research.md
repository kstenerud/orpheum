# The expansion audit of the API and schema, second round (2026-11-05)

> The owner asked for one more audit of the API (D416, D425) and the schema (D426) as built, once
> the API and database batches were done. It judges them against expert practice, against every
> planned feature (B0–B5, the queued items, "Later", "Maybe" and music), and against the unwanted
> live TV, DVR and photos. It also runs the relocation checks of D429 §4 again
> (navigation-and-widgets-answers-2026-10-27.md, "The expansion audit of the API and database" and
> "Repointing a library"). The first round
> ([expansion-audit-2026-11-05.md](expansion-audit-2026-11-05.md),
> [relocation-audit-2026-11-05.md](relocation-audit-2026-11-05.md)) was answered by D432. This round
> checks what D432 built and looks for what that round missed. Read at schema v0083
> (`internal/store/testdata/golden/schema-v0083.sql`) and `api/openapi.json` (137 paths) at
> "docs(plans): the 11-09 batch, the expansion audit and two test caches". This document proposes and decides nothing (AGENTS.md rule 1).

## 1. Summary for the owner

**The premise has moved.** The request speaks of the greenfield deploy as the last cheap point.
D430 dropped the fold and the regeneration: steps 0073–0083 migrate in place, staging has already
run them, and the production NAS takes them with `batch-2026-11-05-api`. So every schema change from here on is
an appended step 0084+, before the deploy or after it. Two things are still cheaper before the production NAS
and production take this batch:

- **API breaks.** No outside client exists yet. The compatibility check starts once the API lands,
  and after that a break costs a release of aliases (D425 §3).
- **Reshapes of data that real use will fill.** For example, how a node is named or how a file
  rejoins its row. Each is cheaper while two households' history is small.

**D432 is built as it says.** The kind list (§1), open responses (§2), the profile as a user (§4),
the paging (§5) and steps 0081–0083 (§6) are all in place (§4 below). The music reshape of D186
also holds up under MusicBrainz's model (§3).

**What I propose, ranked by churn saved:**

| # | Finding | Proposed now | Cost now | Churn saved | Before the deploy? |
| --- | --- | --- | --- | --- | --- |
| 1 | The API addresses a node only by its name, yet a node must be renamable and replaceable (§2.1) | Expose the node's id; address `/nodes/{id}`; bodies take `nodeId` | Small: admin endpoints and the web client's admin pages | A path change on every node endpoint once clients outside exist | **Yes**: it is an API break later |
| 2 | A file that turns up on another node, or under a new root without a repoint, gets a new `file` row and loses the state hung on the old one (§2.2) | Write down the rule (a gone place is revived by payload hash across nodes, not only on its own node) and build it with the repoint batch | Medium: one query and its tests | Lost positions, fetched subtitles, file preferences and play events for every moved file; repair after the fact is guesswork | Ideally; the code can follow |
| 3 | Every place's id equals its file's id: a convention, not a constraint (§2.3) | Make it a stated store invariant with a test, or drop the equality before anything inserts a second place for a file | Small | A primary-key collision with the next file's id the first time a file gains a second place | Yes, before the repoint batch |
| 4 | The route for widening a closed `CHECK` list (sql.md §5) is still review-only and untried (§2.4) | Prove it with a test on the real driver now, with no list widened | Small | Music, collections, photos and DVR all lean on it; finding out it fails then means rebuilding `item` | Not tied to the deploy, but before music |
| 5 | Three closed lists that the first round missed: `library.metadata_source`, `person.kind` and `artwork.kind` (§2.5) | Add them to D432 §3's list; widen when needed | None now | A surprise at music time | No |
| 6 | Music: a track work should be the MusicBrainz *track* (per release), with the recording kept as a provider id (§3.2) | Record the choice when music is designed; nothing in code | None | Compilations and editions placed wrongly in `position` | No |
| 7 | Music: an artist is a person (D416 §4), but D426 §2 lists `artist` as a later item kind, and D425 §7 keeps persons out of `/items` (§3.4) | Settle which, so music's browse has a home | A sentence | A second artist concept beside `person` | No |
| 8 | Two views know only video: `series_member` names the ordering `'aired'`, and `item_found` dates only series by their members (§3.3) | Leave it; a view is replaced without a rebuild | — | — | No |

**In my view, finding 1 must go in before the deploy.** It is the only one that becomes a
client-facing break. Findings 2 and 3 belong in the repoint batch already planned
(navigation-and-widgets-answers-2026-10-27.md, 11-09 batch item 1), and 3 must come first. Finding 4
is a test that should exist before anyone relies on the route.

**Music is the heaviest of the planned features, and it fits.** The rows carry it
(album = sequence item, release = ordering, disc = group, track = position). The kinds now widen by
name. Its open questions are decisions: the album's identity, the artist's home and the track's
identity. None of them is a table rebuild. The one structural gap is the CUE sheet (one file, many
tracks), which D186 leaves out on purpose.

## 2. Changes proposed

### 2.1 A node has no id in the API

**Found.** `node(id, name, enrolled_at, removed_at)` has an integer id, and `node_live_name` makes
the name unique among live nodes (step 0082). The API never shows that id:

- every node path takes the name: `/nodes/{name}/folders`, `/nodes/{name}/reader-limit` and
  `/nodes/{name}/retire`;
- `NodeBody` carries `isConnected`, `name` and `rootPaths` only;
- `RepointInputBody` takes `nodeName`, `path`, `newNodeName` and `newPath`, and
  `AddLibraryRootInputBody`, `RootActionInputBody` and `RootBody` name the node by `nodeName`.

D416 §7 chose `/nodes/{name}/retire`.

**Why it matters.** D429 §4 asks that a node can be renamed or replaced. The repoint plan
("Repointing a library") moves folders from node A to node B, and node adoption (relocation audit)
gives a new node an old one's place. Under names alone:

- a rename changes every URL and every stored client reference;
- a replacement under the same name cannot be told apart from the old node in a request or a log;
- a retired node's name may be taken again by a new node, because the unique index covers live rows
  only.

Expert practice keeps the identifier immutable and the human name a mutable display field (Google
AIP-122 and AIP-148). Every other resource in this API already follows that practice: libraries,
items, users and files are addressed by integer ids. "A second media node" is on the Later list,
and when it comes, this endpoint family grows.

**Proposed now.** Add `id` to `NodeBody`. Address node paths as `/nodes/{id}`, keeping the name as
a field. Take `nodeId` (and `newNodeId`) in the root and repoint bodies. Show `nodeId` in
`RootBody` beside the name. The node protocol between core and node is not affected: a node still
enrols by its key.

**Cost now.** Small. These are admin-only endpoints and the web client's admin pages, with no
outside client and no compatibility promise yet.

**Churn saved.** After the API lands, this is a path change on a whole family, with a release of
aliases (D425 §3). Renaming a node later would also change the URLs saved in admin bookmarks and
scripts.

### 2.2 A moved file rejoins its row only on its own node

**Found.** A file that disappears and reappears has two ways back to its `file` row:

- **Revival.** `GetLatestTombstoneByPayloadHash` (`internal/store/queries/file.sql:112`) looks for a
  gone place with the same payload hash `WHERE place.node_id = ?`. It searches on the same node
  only. `ListLiveFilesByPayloadHash` is also per node.
- **Copies.** `ListCopyCandidates` (`file.sql:478`) works across nodes. But it joins a new place to
  an existing file only when the earlier place is still live and the whole-file SHA-256 matches.

The repoint (`retire.sql`: `RepointLibraryRoot`, `RepointFile`, `RepointFileFilings`) moves the
rows themselves when the owner says where a folder went. It also fixes `identified_from_path`
(the first relocation round's bug) and marks the place `is_unconfirmed` (step 0081).

**Why it matters.** This is relocation check 4: "a file at a new place joins its existing `file` row
by hash, keeping watching, choices and fetched subtitles". It holds only when the owner repoints, or
when the move stays on one node. It does not hold in these cases:

- **The files move to another node without a repoint.** For example, the owner mounts the disk on
  the second media node, or replaces the NAS and enrols the new node before retiring the old one.
- **The files are copied to a new disk and the old one is unplugged.** The earlier place is no
  longer live, so the copy route does not apply.

In both cases the scanner makes a new `file` row. What hangs on `file_id` is then orphaned on the
old row and purged with it:

- positions;
- fetched subtitles;
- file-scoped preferences (`preference.scope = 'file'`);
- the file side of play events.

Item-level state (watched, stars, the watchlist) survives, because identification finds the same
item again. A second node is on the Later list, and replacing a NAS is ordinary.

For music, whole-file confirmation never matches a retagged file. D186 already rests music's
identity on the payload hash, which survives retags (D179), so revival by payload hash is the right
key there too.

**Proposed now.** Record the rule: a new place whose payload hash matches a file with no live place,
on any node of the account, revives that file. Ambiguity is decided as revival decides it today.
Build it in the repoint batch, beside node adoption. The schema needs nothing: `file` and `place`
already allow it.

**Cost now.** One query widened from one node to the account, the purge clock checked against it,
and tests.

**Churn saved.** State lost for real households. That cannot be rebuilt afterwards except by
guessing from names.

**Unverified.** I did not trace the scanner end to end to confirm that no other path joins across
nodes. I read only the queries named above.

### 2.3 Every place's id is its file's id, by convention

**Found.** `InsertPlace`, the only insert into `place`, sets `id = file_id`. The `file.id` comment in
the golden schema reads "the id of the place it was first found at", and `MakeFileACopy`
(`file.sql:541`) still joins on `file.id = place.id`. Copies avoid a second place per file by making
a throwaway file row first.

**Why it matters.** Two planned changes insert a place for a file that already has one:

- the repoint's matching by hash;
- the cross-node revival of §2.2.

If either writes `id = file_id` while the old place row still exists, it collides on `place`'s
primary key. If it takes a fresh id instead, that id can later equal a new file's id under
`AUTOINCREMENT`, and every query that still joins `file.id = place.id` then reads the wrong row.
Nothing enforces the convention, and nothing flags code that leans on it.

**Proposed now.** Choose one of these:

- (a) state the invariant in the store's `doc.go` with a `TestArch_` or store test that every place
  of a file shares its id, and keep using the throwaway-file route for every second place;
- (b) remove the `file.id = place.id` joins so that place ids are independent.

Either way, decide before the repoint batch writes code that inserts a place.

**Cost now.** Small for (a). For (b), a review of the joins that use the equality.

**Churn saved.** A data-corrupting bug that shows up only on a second place, which is to say only
after a move.

### 2.4 The route for widening a closed list is untried

**Found.** D432 §3 chose to widen a closed `CHECK` list in place through `writable_schema`, as
sql.md §5 (lines 153–172) describes. sql.md says: "The first step that widens a list adds that to
the runner … No list has been widened so far. Review-only until then."

The driver is modernc.org/sqlite v1.59.0. SQLite's defensive mode, which forbids writes to
`sqlite_schema`, is opt-in there (`_defensive`), and `internal/store` does not turn it on. So the
route should work, but nothing has run it.

**Why it matters.** This route is how music (`item.kind`, `work.kind`, `library.kind`, and
`metadata_source`), collections, photos and DVR enter the schema without rebuilding `item`. `item`
is the table with 19 foreign keys and 14 triggers pointing at it (first round §4.1). If the route
fails at music time, because of a driver option, an integrity check or a STRICT-table subtlety, the
fallback is that rebuild under time pressure.

**Proposed now.** A store test that widens a copy of one list (say `library.kind`) through the
runner's planned path on the real driver. It then runs `PRAGMA integrity_check` and
`quick_check`, and re-opens the database. No list is widened in the real schema. sql.md §5 is the
owner's procedure, so whether to prove it ahead of its first use is the owner's call.

**Cost now.** Small.

**Churn saved.** A failed route found at the worst time.

**Unverified.** I did not run SQLite to confirm that the edit passes `integrity_check` on STRICT
tables. The SQLite documentation (ALTER TABLE, "making other kinds of table schema changes")
describes the procedure, but it does not discuss STRICT.

### 2.5 Three closed lists the first round missed

**Found.** The first round counted ten closed `CHECK` lists. Three more stand in the way of the
planned features:

- `library.metadata_source IN ('nfo_then_tmdb','tmdb','nfo')`, together with
  `CHECK (kind <> 'other' OR metadata_source = 'nfo')`. Music reads tags and MusicBrainz, and photos
  would read EXIF. Neither is a value of this list, and the second check ties a fourth library kind
  to nothing.
- `person.kind = 'person'`. If an artist or a band is a person (D416 §4), a band is not a person in
  TMDb's sense, and a `group` or `artist` value would be wanted.
- `artwork.kind IN ('poster','backdrop','logo','banner','still','disc')`. A square album cover is
  not a poster, and the web client sizes a poster as 2:3.

**Proposed now.** Add these three to the inventory of widenable lists that D432 §3 relies on.
Nothing needs widening yet.

**Cost now.** None.

**Churn saved.** A surprise at music time. Each is the same widening as the others.

## 3. Music, weighed hardest

The owner expects music to cause churn if it is not planned for. D186 reshaped the schema for it,
and music is to be revisited when the closed beta ends (closed-beta.md). The OpenSubsonic research
(music-opensubsonic-2026-10-18.md) left its questions to the owner. This section checks the built
schema against MusicBrainz's model, which the competitors use, and against how Navidrome and Plex
handle the hard cases today.

### 3.1 The rows fit

MusicBrainz's hierarchy runs release group → release → medium → track → recording. A release group
is the abstract album. A release is one product of it. A medium is a disc or a side. A track places
a recording at a position on a medium. (MusicBrainz database schema documentation.)

Orpheum's built shape maps onto it without a new table:

| MusicBrainz | Orpheum (v0083) |
| --- | --- |
| release group (the album) | a `sequence` item, kind `album` once widened |
| release (an edition) | an `ordering` of that sequence; the schema's comment already says "'release' for an album" |
| medium (disc) | a `grp` of the ordering (`UNIQUE (ordering_id, number)`) |
| track | a `position` (`PK (ordering_id, work_id)`) |
| the audio | a `work` with its files, kind `track` once widened |
| MBIDs | `item_provider_id (item_id, provider, provider_id)` |

### 3.2 A track should be MusicBrainz's track, not its recording

`position` has the key `(ordering_id, work_id)`, so a work appears at most once per release.

- **If a work is a recording**, a compilation that holds the same recording twice, or a deluxe
  release that repeats a track in a bonus disc, cannot place it twice.
- **If a work is a track** (one per file, per release), it can.

The recording MBID stays a provider id, so "other releases of this song" remains a query.

Navidrome made the same choice: since 0.55 its default track identity is the MusicBrainz *track*
id, and its album identity the release id. When those are absent it falls back to album and disc,
track number and title. It lets the owner choose the album identity (for instance, by folder). When
the identity changes, it keeps stars, ratings, play counts and playlists (Navidrome's "Persistent
IDs" documentation).

That is D186 §3's point that album state must follow its member files when the album's definition
changes. It is also the question D186 left open: what defines an album is asked, not defaulted.

Two editions of one album can be either two album items or two orderings of one. Plex still
treats editions as one album, and its users have asked for a version picker since 2022 (Plex
forums, unverified for the current release). Navidrome added an "album version" tag to tell
editions apart.

Either way fits Orpheum's rows. Two orderings per album need the edition named per ordering, and
`ordering.name` is free text under `UNIQUE (sequence_id, name)`, so no table change is needed.

**Proposed.** When music is designed, record that a track work is a track (per release) with the
recording as a provider id, and ask the album-identity question that D186 left open. Nothing to
build now.

### 3.3 Two views know only video

- `series_member` hard-codes `ordering.name = 'aired'`. An album has no aired ordering.
- `item_found` computes `found_at` from members for kind `series` only. An album would have
  `found_at` 0, and "recently added albums" would be empty.

Both are views. Step 0083 replaced `file_place` in place, and these can be replaced the same way
when music lands, with no table rebuilt. **Sound to leave.**

### 3.4 Where an artist lives

D416 §4 puts artists under `/people`: an artist is a `person`. D426 §2 lists `artist` among the
later item kinds. Meanwhile D425 §7 keeps persons out of `/items` (`/items/{id}` answers 404 for a
person), and there is `/people/{id}` but no `/people` list.

Music's first screen is a list of artists, sorted, paged and filtered by library. That needs one of:

- a `/people` list with a role filter (credited as an artist, in a music library);
- persons admitted to `/items` under a named kind (additive since D432 §1);
- a separate `artist` item kind, which means a second identity beside `person` for the same human,
  with a composer credited on a film and on an album twice over.

Credits fit already: `credit.job` widens with the other lists, and `credit.ordinal` orders
"A feat. B". Navidrome's 0.55 "participants" model (artist, album artist, composer, performer and
the rest) is the shape the tags give.

**Proposed.** Settle D416 §4 against D426 §2 when music is designed. In my view, keep one `person`
and add a `/people` list. Every option is additive, so this costs nothing now. It is listed
because two decisions currently disagree.

### 3.5 What else music needs, all additive

- **Per-person state.** Watched stays video-only. Music wants a play count, last played, a star and
  a rating, each a new column or table. No rename of `watched` to "played" is needed.
- **Album and artist stars must survive regrouping** (D186 §3). `item.merged_into` (D173) already
  carries an item's people-owned state to its survivor. The cases to test then are the split and
  the regroup.
- **Playback.** An audio queue and audio transcoding are new endpoints or new values in
  `playableFormats`. `StartPlaybackInputBody` is a request (closed), so new fields are optional
  additions.
- **Names.** `stream.video_file`, `fetched_subtitle.video_file` and the API's `TitleBody`,
  `TitleEntryBody`, `SeriesBody` and `PlaysBody` read as video-only. These are cosmetic. Renaming a
  component does not change the wire.
- **Scale.** A music library holds tens or hundreds of thousands of tracks. D431's revisit trigger
  for computed visibility becomes likely, and storing visibility later is additive.
  **Unverified:** I did not measure the triggers or `item_found` at 500k works.
- **CUE sheets** (one file, many tracks) stay out (D186). This is the one structural gap. It would
  need a position to name a time range within a file.
- **The OpenSubsonic route** (music-opensubsonic-2026-10-18.md) needs stable string ids per album,
  artist and track. Orpheum's opaque integer ids (D174) serve as long as they stay stable through
  regrouping, which is §3.2's point again.

## 4. Sound as it is

Verified against the code at "docs(plans): the 11-09 batch, the expansion audit and two test caches":

- **D432 §1.** `/items` takes `kind` as a list with the fixed default `movie,series,other`
  (`internal/core/api/item_list.go:145`). `any` is gone. New kinds join only when named.
- **D432 §2.** `info.description` says that responses are open and requests closed. Kind and code
  enums in responses are `x-extensible-enum`, and only request enums (`CreateLibraryInputBody`,
  `EventBody`) are closed. A new kind in a request is a new allowed value, which is compatible.
- **D432 §4.** A profile is a `user` row. Per-person tables key on `user_id`.
- **D432 §5.** The library and node folder listings are paged, and `/needs-attention` is capped.
- **D432 §6.** Step 0081 (`place.is_unconfirmed`), step 0082 (`node_live_name`) and step 0083 (the
  `file_place` view, `item_found`, and triggers keyed on `place.file_id`) are in place.
- **The repoint keeps identification.** `RepointFileFilings` rewrites `identified_from_path`. That
  was the first relocation round's bug, and it is fixed.
- **Relocation check 1.** Every stored path that people or identification depend on is rewritable
  by prefix in one transaction through `retire.sql`. The paths the plan names as not reachable are
  the following, and none of them is a key:
  - cache.db `job.destination`;
  - `scan_batch.files_json`;
  - `place.error_detail_json`;
  - the filing reasons;
  - `person.picture_ref`;
  - `file_nfo.body`.
- **Relocation check 2.** Nothing people own and nothing identification found keys on a path, a
  node name or device+inode:
  - watching, stars, the watchlist and preferences key on item, file and user ids;
  - identification keys on `file_filing (file_id, library_kind)`;
  - `place_live_path` is a uniqueness guard, not a key that anything references.
- **Relocation check 3, schema side.** A node can be renamed: the schema's references use
  `node_id`. The gap is the API (§2.1) and adoption (relocation audit).
- **Ids.** Integers, opaque to clients (D174), and stable through merges via `merged_into` (D173).
- **Search.** It is grouped (titles, people, genres) with a cursor per group. Adding an albums or
  artists group is additive, because `group` is a request enum that widens compatibly.
- **Groups addressed through the person's ordering.** `/items/{id}/groups/{group}` takes the group
  number in the ordering that person watches in. An explicit `ordering` parameter later is additive.
- **Live TV and DVR.** As in the first round: a recording is a file in a writable library
  (`library.is_writable` exists). A live channel needs a playback without a file, and making
  `fileId` optional is a compatible request change. The guide belongs in cache.db.
- **Photos.** As in the first round: a library kind, a work kind and an EXIF metadata source (now
  including `metadata_source`, §2.5). Albums of photos fit as sequences.
- **Collections, playlists, the watchlist, segments and trickplay.** Each is a new table or the
  existing unused tables. All are additive.
- **API keys.** `grant.kind` already includes `api_key`. Scoped keys are on the Later list and add
  rows, not shape.
- **Expert practice, otherwise.** The first round's comparison with Zalando's, Azure's and Google's
  guidelines still holds, with its findings 1, 2, 4, 5 and 6 now done. Two items remain open:
  no ETag or `If-Match`, and no job id from `202`. Both are additive.

## 5. The feature gap, re-checked

The owner's pasted comparison is dated. I re-checked only where the gap touches the API or schema:

- **Music.** Navidrome 0.55 rebuilt its model around configurable persistent ids and participant
  roles (its documentation). Jellyfin 10.11's web client added editing of album artists and showed
  every album artist on cards (its release notes). Jellyfin still groups some artists by tag rather
  than by MusicBrainz id (its forum, 2024; current state unverified). Plex groups by MusicBrainz
  release types and still has no edition picker (its forums).
  The gap is a whole feature, not a field. It is weighed in §3, and the schema is ready for it.
- **Live TV, DVR and photos.** Unchanged from the first round. All three competitors have them.
  §4 says what Orpheum would need.
- I did not re-check prices, client lists or the B-tier video features: none of them changes the
  API's or schema's shape.

## Sources

- Google AIP-122, resource names: https://google.aip.dev/122
- Google AIP-148, standard fields: https://google.aip.dev/148
- Navidrome, persistent ids: https://navidrome.org/docs/usage/configuration/persistent-ids/
- MusicBrainz database schema: https://musicbrainz.readthedocs.io/en/latest/musicbrainz_database/schema/_schema.html
- Jellyfin web 10.11.0 release notes: https://newreleases.io/project/github/jellyfin/jellyfin-web/release/v10.11.0
- Jellyfin forum, artist grouping: https://forum.jellyfin.org/printthread.php?tid=5480
- Plex forums, album editions: https://forums.plex.tv/t/album-editions/815645
- Plex forums, album release types: https://forums.plex.tv/t/new-feature-albums-single-eps-live-albums/735790
- SQLite ALTER TABLE: https://www.sqlite.org/lang_altertable.html
