> **ABOUTME:** How a series offers more than one order of its episodes (aired, DVD, absolute,
> production, a provider's alternates, a person's own), how each person chooses the one they
> watch in, and what that choice changes. Read before building closed-beta B2's "Episode
> orderings" item, or before adding a query that names an ordering.

# Episode orderings

- **Status:** SKETCH — 2026-10-25, at the owner's request: per-person choice of a series'
  episode ordering, *aired, DVD, absolute, production, custom*, with every ordering the same
  mechanism ([D160](../../decisions/working-notes.md): a sequence is what a Collection produces,
  and Next Up is a query over any of them). The questions in §7 are the owner's to answer before
  it is built.
- **Depends on:** [play-state.md](play-state.md) (Next Up over an ordering),
  [schema.md](schema.md) (`ordering`, `position`, `group_text`, `preference`),
  [closed-beta.md](closed-beta.md) (B2, "Episode orderings", D96 and D210, size M).
- **Governing decisions:** [D96](../../decisions/working-notes.md) (a position is
  `(ordering, season, number)`, and a series declares its orderings),
  [D159](../../decisions/working-notes.md) §1–§4 (the choice is a
  [D24](../../decisions/working-notes.md) preference at series scope; an ordering may be
  authored; a multi-episode file is a boundary; an ordering is chosen, never detected),
  [D160](../../decisions/working-notes.md), [D209](../../decisions/working-notes.md),
  [D210](../../decisions/working-notes.md) (the on-disk order and the viewing order),
  [D157](../../decisions/working-notes.md) (a position belongs to the file),
  [D170](../../decisions/working-notes.md) and [D213](../../decisions/working-notes.md) (fetched
  once; refreshed on a rolling schedule or by a person).

The decisions already settle the shape. What they leave open is where the orderings come from,
how a provider's lists map onto the tables, and which of today's queries mean "the order this
person watches in" when they say `'aired'`. This plan answers those and names what the owner
still has to choose.

Sources fetched 2026-10-25 are cached under `/yoloai/cache/http/tmdb-episode-groups/` (TMDB's
API reference and public series pages, TheTVDB's v4 swagger, Kodi and Jellyfin source files)
and `/yoloai/cache/git/kodi-tmdb-tv/` (Kodi's TMDB TV scraper, branch `piers`, commit
An earlier commit). No API key was used and nothing was written to any service.

## 1. Two orders, kept apart

[D210](../../decisions/working-notes.md) splits the question in two, and every part of this
plan sits on one side or the other:

| | What it is | Who sets it | Where it lives | Default |
| --- | --- | --- | --- | --- |
| **Files' order** (D210 §1, "on-disk order") | which ordering a library's `S01E05` numbers are in | an admin, for the household, per series | the series (no column yet, §3.5) | aired |
| **Viewing order** (D210 §3, D159 §1) | which ordering a person watches, browses and is offered Next Up in | each person, per series | a `preference` row, scope `work` | the files' order (D210 §5) |

The files' order is used by identification only: it says where to look a file's numbers up. The
viewing order is used by everything a person sees. Once an episode is identified, its provider
id places it in every ordering the provider lists (D210 §3), so the two never need to agree.

## 2. Where orderings come from

### 2.1 TMDB: the season listing, and episode groups

**The aired ordering is TMDB's season listing**, `/tv/{id}/season/{n}`, which is all the code
fetches today (`internal/core/providers/tmdb/works.go:189`, `FetchSeason`; D209 §2).

**Every other TMDB ordering is an "episode group".** Two endpoints, both read-only
([TMDB API reference](https://developer.themoviedb.org/reference/tv-series-episode-groups),
fetched 2026-10-25):

- `GET /3/tv/{series_id}/episode_groups` lists a series' groups. Each result has `id` (a hex
  string such as `5e9077d2e640d600151f32bd`), `name`, `description`, `type`, `episode_count`,
  `group_count` (how many parts) and `network`.
- `GET /3/tv/episode_group/{tv_episode_group_id}` returns one group: its fields above plus
  `groups[]`, the parts. Each part has `id`, `name`, `order` and `episodes[]`. Each episode
  entry is a whole TMDB episode — `id`, `name`, `overview`, `air_date`, `still_path`,
  `runtime`, **its aired `season_number` and `episode_number`** — and an `order` within the
  part. The reference's example numbers episode `order` from 0
  ([episode group details](https://developer.themoviedb.org/reference/tv-episode-group-details)).

**The types, as the reference enumerates them** (verified, 2026-10-25): *"Groups support 7
different types"*:

| `type` | Name | Proposed `ordering.kind` |
| --- | --- | --- |
| 1 | Original air date | `aired_alternate` |
| 2 | Absolute | `absolute` |
| 3 | DVD | `dvd` |
| 4 | Digital | `digital` |
| 5 | Story arc | `story_arc` |
| 6 | Production | `production` |
| 7 | TV | `tv` |

**The group list can come with the series at no extra request.** Jellyfin asks for it as part of
the series fetch (`TvShowMethods.EpisodeGroups` among the `append_to_response` methods,
`MediaBrowser.Providers/Plugins/Tmdb/TmdbClientManager.cs:151`, master, 2026-10-25). Our series
fetch already appends `credits,external_ids` (`works.go:121`). Fetching one group's contents is
one request; the reference accepts a `language` for it, as Jellyfin passes
(`TmdbClientManager.cs:192`).

**Groups are contributed by TMDB's users, a series may have many of one type, and the type is a
contributor's label rather than a meaning.** From the public "Episode Groups" pages on
2026-10-25 (served in German; parts and episodes translated):

| Series | Groups (type) — parts, episodes |
| --- | --- |
| *Emberwing* (1437) | "Intended Order" (Absolute) 2, 14 — *"The proper and intended order of Emberwing episodes"*; "DVD Order" (DVD) 2, 23 |
| *Orbitorama* (615) | "DVD Order" (DVD) 13, 170; "Digital Order" (Digital) 14, 168; "Production Order" (Production) 12, 170 |
| *Metropolis: The Clone Wars* (4194) | "Chronological Order" (Story arc) 1, 133 — *"starwars.com's official chronological ordering"*; "Story Arcs" (Story arc) 50, 134; "Ahsoka Tano Essential Episodes" (Story arc) 1, 16; "Compromise Order" (Story arc) 1, 133 |
| *One Piece* (37854) | Original air date ×3 ("TVDB Order" 24, 1218; "Wikipedia Seasons"; "Openings Order"); Absolute ×2 ("Absolute (No Specials)" 1, 1181; "Absolute (With Specials)" 2, 1220); DVD ×1 ("German DVD Release" 36, 1047); Digital ×7 (Funimation, Netflix, Hulu, BBC iPlayer and three Crunchyroll storefronts); Story arc ×3 ("Story Arc" 55, 1219; "Sagas"; "Season, Arc & Saga Combos") |

What follows from it:

- **There is no "the DVD order" to pick by type.** *One Piece* has seven Digital groups and three
  of Original air date. *Clone Wars*' chronological order is typed Story arc, not a type of its
  own; *Emberwing*'s intended order is typed Absolute. The list response has no "official" or
  "primary" flag (verified against the reference's schema, 2026-10-25). **An ordering is
  therefore one group, named by its id**, and the type only labels it (§7 Q1).
- **A group may be partial.** "Ahsoka Tano Essential Episodes" holds 16 of 133; "Absolute (No
  Specials)" omits the specials. D159 §2 already requires an ordering to be partial.
- **A group may hold more than the aired listing.** *Emberwing*'s DVD group has 23 episodes to the
  intended order's 14, which suggests it lists specials or extras as episodes (UNVERIFIED: the
  group's contents were not fetched).
- **One episode appears at most once in a group.** A contributor's note on *One Piece*'s Hulu
  group: *"As episodes can't be listed twice in the same episode group, the episode is listed in
  this group as part of season 10 instead of as a special."* That matches
  `position`'s `PRIMARY KEY (ordering_id, title_id)`.
- **A group cannot split or merge an episode**: an entry is a whole TMDB episode by id, with no
  field for a part of one (inferred from the schema; UNVERIFIED against a group that tries).

**How the incumbents read a group** (verified in source, 2026-10-25): Kodi's TMDB TV scraper
maps a group's part `order` to the season and an episode's `order + 1` to its number
(`lib/scraper.py:571`, `_apply_episode_grouping`); Jellyfin finds the part whose `Order` is the
season and the entry whose `Order` is the number minus one (`TmdbClientManager.cs:196`, `:267`).
Both use it for the **files' order** — the numbers on disk — chosen once per series: Kodi from a
`themoviedb.org/tv/{id}/episode_group/{group}` URL after the root of `tvshow.nfo`
(`lib/scraper.py:24`, `_NFO_TMDB`), Jellyfin from a series provider id `TmdbEpisodeGroupId`
(`TV/TmdbEpisodeProvider.cs:107`). Neither offers a per-person viewing order.

### 2.2 TheTVDB: one list per kind (note only)

TheTVDB's v4 API returns a series' episodes per **season type**, `default`, `official`, `dvd`,
`absolute`, `alternate`, `regional` (`/series/{id}/episodes/{season-type}`, v4 swagger,
2026-10-25): one curated list per kind, where TMDB has any number per type. **There is no TVDB
provider here**: `internal/core/providers/` holds `tmdb`, `opensubtitles` and `cloudflare`;
`collection.followed_source` admits `'tvdb'`, and `tmdb/works.go` reads a TVDB id only as an
external id. Should one be added, its kinds are orderings named `tvdb:<season-type>` on the same
tables.

### 2.3 NFO and file names

- **An episode NFO's `<season>` and `<episode>` are aired numbers**, and the reader takes them
  (`internal/core/identify/nfo.go:58`–`59`, `:100`; `episodes.go:49`).
- **Kodi's `<displayseason>`, `<displayepisode>` and `<displayafterseason>` place a special among
  the aired episodes**; they are not another ordering. Kodi reads them into
  `m_iSpecialSortSeason` and `m_iSpecialSortEpisode` (`xbmc/video/VideoInfoTag.cpp:1308`,
  master, 2026-10-25); Jellyfin reads them as `AirsBeforeSeasonNumber`,
  `AirsBeforeEpisodeNumber` and `AirsAfterSeasonNumber` (`EpisodeNfoParser.cs:162`–`:182`) —
  D96's three nullable side-columns. Our reader ignores them today. Read, they would be the
  aired ordering's `sort_order` for that special, not a new ordering.
- **Neither Kodi's NFO reader nor Jellyfin's has a DVD or absolute field** (searched for `dvd`,
  `absolute` and `order`, 2026-10-25). Kodi's `tvshow.nfo` names a TMDB group only through the
  scraper URL above, and `<namedseason number="n">` names an aired season
  (`VideoInfoTag.cpp:1454`). Our reader drops what follows the root element (`nfo.go:79`), so
  the URL is not read today.
- **A file name's `S01E05` is in the files' order, which is aired unless an admin says otherwise**
  (D210 §1–§2). A library ripped from *Emberwing*'s DVDs and numbered by disc order looks exactly
  like an aired library; nothing in the name says which it is, and D210 §2 trusts the numbers
  within the order the admin names. A file named by absolute number (`One Piece - 812.mkv`)
  is the same case with one group.

## 3. Mapping onto the schema

The tables (`internal/store/testdata/golden/schema-v0056.sql`) already hold most of this: an
`ordering` per `(collection_id, name)` with a `provider`, a `position` per
`(ordering_id, title_id)` with `group_number`, `number_in_group` and a total `sort_order`, and a
`group_text` for a group's name. What is missing is small and listed in §3.5.

### 3.1 Names

| Ordering | `ordering.name` | `provider` | Proposed `kind` |
| --- | --- | --- | --- |
| TMDB's season listing | `aired` (exists) | `tmdb`, or NULL for one from files or NFO alone | `aired` |
| A TMDB group | `tmdb:<group id>` | `tmdb` | from `type`, §2.1 |
| A TVDB season type, if ever | `tvdb:<season-type>` | `tvdb` | `dvd`, `absolute`, … |
| A person's own | `user:<ordering id>` or a name they type | NULL | `authored` |

`name` is the stable key a preference stores, so it is never a translated label. The group's
TMDB name (*"DVD Order"*, *"Intended Order"*) is display text and goes beside it (§3.5).

### 3.2 Positions

- **Materialising a TMDB group**: for each entry, the series' episode whose TMDB id is the
  entry's `id` gets `position(ordering, title, group_number = part.order, number_in_group =
  entry.order + 1, sort_order = part.order × 10 000 + entry.order + 1)`, the same scheme
  `positionEpisode` uses for aired (`identify/episodes.go:342`) and the same mapping Kodi and
  Jellyfin use (§2.1). The TMDB id is the one `fromEpisode` records as the title's
  `followed_source_id` (`episodes.go:299`); an episode only an NFO speaks for is matched by a
  `tmdb` `item_provider_id` when it has one (UNVERIFIED that episodes are written one today).
- **Group names**: each part's `name` (*"Season 1"*, *"First Cup"*, *"Volume 3"*) is the group's
  text in that ordering, in the language fetched.
- **An episode the group omits has no position in it** (D96: one position *per ordering it
  appears in*). How the page shows it is §7 Q4.
- **An episode the group lists and the library lacks has no row**: a position needs a title.
  Listing it as missing is closed-beta B2's "Missing episodes" (D187), which should read the
  stored group as it reads the season listing.
- **One file holding two episodes** (`S01E01-E02`): today the filename parser reads the run
  (`identify/filename.go:27`, `EpisodeEnd`), but only the filing report uses it
  (`identify/report.go:207`), so the file is one title at E01 (inferred from where `EpisodeEnd`
  is read). In another ordering that title takes E01's place and E02 has none. D159 §3's answer,
  two titles and an `episode_boundary` segment in one file, is not built (there is no segment
  table in v0056); until it is, **a multi-episode file sits at its first episode's place in
  every ordering**. That is a known gap, not a design.
- **Two files of one episode** keep separate play positions (D157, D159's assertion) and share
  the title's one ordering position. Nothing changes here.

### 3.3 A file numbered in a non-aired order

With the files' order set to a group (D210 §1), identification looks a file's
`(season, number)` up in that group: the entry at part `season`, order `number − 1`. The entry
carries the episode's TMDB id **and its aired season and number**, so one stored group places
the file in both: in the group's ordering at the file's own numbers, and in `aired` at the
entry's. Its name, overview, still and runtime come from the entry too, so no season listing
is needed to name it (the entry carries them, §2.1). A number the group does not list stays on
local data, unidentified, saying so — D209 §2's rule with a different listing.

### 3.4 Fetching and refetching

- **The list of a series' groups** comes with the series' own record (append `episode_groups`
  to the series fetch, §2.1): no new request, and no request at all for a series already
  fetched. A series fetched before this lands has no list; it gains one at its next refresh,
  rolling (D213 §3, at most `tmdb_refresh_after_days`, 180 days) or a person's (D213 §2). The
  selector's "Look for other orders" is that refresh (§7 Q5).
- **A group's contents are fetched when it is first needed**: a person chooses it, or an admin
  makes it the files' order. That is a first fetch on a person's act, inside D170. Stored as
  `provider_response` with `part = 'episode_group/<id>'`, beside `season/N`
  (`identify/series.go:53`), and parsed from storage on every pass (D170). Fetching every group
  of every series at scan time is what D170 forbids: *One Piece* alone has sixteen.
- **A refresh of the series refreshes the groups it has fetched**, and re-materialises their
  positions. A group TMDB has since changed moves episodes; play state does not move (D157), and
  a person's Next Up follows the new order — which is what refreshing is (D213 §3).
- **A group TMDB has deleted is kept** with its last positions (D213: nothing is deleted because
  a provider dropped it); the selector says it is no longer listed.
- **A person's own ordering is never touched by a refresh** (D159 §2, D141).

### 3.5 What the schema lacks

| Gap | Why | Proposal |
| --- | --- | --- |
| `ordering` has no kind, no source id, no label | the selector needs *"DVD Order — DVD, 23 episodes"*; `name` must stay a key | add `kind`, `source_id` (the group id), and a label (or an `ordering_text` per language, like `item_text`) |
| `group_text` is keyed by `collection_id` | a group number means a different part in each ordering (Season 1 aired; Disc 1 DVD) | re-key it by `ordering_id`; nothing reads or writes it today (no query names it), so the step is cheap |
| no files' order on the series | D210 §1 | `collection.files_ordering_id` (NULL means `aired`), or a household setting per series |
| `collection_member.position` mirrors aired's `sort_order` (`episodes.go:342`–`:345`) | a second copy of one ordering | leave it: it orders a series as a member of another collection; note it is not the viewing order |

Migrations are live (`internal/store/migrations/CHECKPOINT` is 48; the last step is 0056), so
each is an appended step (D272 §5, standards sql.md), cut by the owner.

## 4. The per-person choice

- **Stored as a preference**: `preference(user_id, scope = 'work', scope_id = <series item id>,
  axis = 'episode_ordering', value_json = '"tmdb:5e90…"')`. `work` is the scope D230 already uses
  for a series' language choices (`internal/core/preferences/preferences.go:111`), so this adds an
  axis, not a scope (D159 §1: *"the preference resolution adds nothing"*).
- **Resolved, in this order**: the person's preference, if it names an ordering the series
  still has; else the series' files' order (D210 §5); else `aired`. The owner's 2026-10-25 note
  says the default is aired; D210 §5 says it follows the files' order. They agree until an admin
  sets a files' order (§7 Q3).
- **Set only by a person, on the series page.** Never inferred from what they watch (D159 §4,
  play-state.md "the ordering is never inferred from behaviour").
- **What switching changes**, all of it derived:
  - **Next Up** — the earliest unwatched member after the latest completed one in that ordering
    (play-state.md; D160). The series page's Next Up being built now takes the ordering as a
    parameter.
  - **The series page's groups** — headings, their names, their order, and which episodes each
    holds. An ordering of one part (absolute, *Clone Wars* chronological) shows its episodes at
    once, as a one-season series does today (`web/apps/web/src/series.tsx:57`).
  - **Episode labels** — the place shown beside an episode on its page, in Continue Watching and
    in Next Up: the group's name and the number in it (§7 Q8).
  - **Continue Watching's rows do not change** — a row is the most recent partway work
    (play-state.md); only its label does.
  - **What a look at the series raises** (D324 §3) — the episodes ahead of the person in that
    ordering, and "the season looked at" is a group of it.
  - **The cast queue** — Next Up extended (play-state.md, D135).
  - **Nothing stored moves.** Every `play_position`, `play_event` and watched mark is
    byte-identical before and after (D159's first assertion; D157).
- **The UI**: a selector on the series page, shown only when the series has more than one
  ordering, listing only the orderings it has: aired first, then each provider ordering by its
  label with its kind and how many of the series' episodes it holds, then the household's own.
  Choosing one not yet fetched fetches it (§3.4) and says so while it does.
- **A composed collection** (*Gruelstone → Lockfast*) follows each inner series' ordering
  for the person (D160 §1). That needs nothing here beyond the resolver being per series.

## 5. Every place that assumes aired today

At `HEAD`. The browse files and `interest/` are being changed by the Next Up work
in flight, which parameterises the member query by ordering name and passes `aired`
(`internal/core/library/next_up.go`, untracked as this is written); line numbers below are
`HEAD`'s.

Each is one of three meanings. **V**, the viewing order: becomes the person's resolved ordering.
**F**, the files' order: becomes the series' files' order (aired until D210 §1 is built).
**M**, membership: means "the episodes of this series" and should not name an ordering at all
(`collection_member`), or may keep `aired`, which every episode is in today.

| Where | What | Meaning | Should |
| --- | --- | --- | --- |
| `internal/store/queries/browse.sql:128`–`134` `GetEpisodePlace` | an episode's series and place | V for labels; M for its callers in identify | take an ordering for labels; identify asks for the series alone |
| `browse.sql:149`–`162` `ListSeriesEpisodeDetails` | a series' episodes, grouped into the page's seasons | V | take the ordering name |
| `browse.sql:165`–`179` `ListSeasonEpisodeDetails` | one season's page of episodes | V | take the ordering; "season" is the group |
| `browse.sql:196`–`205` `ListLiveFileLocationsOfSeries` | whether a series' files can be read | M | read `collection_member` |
| `browse.sql:231`–`247` `ListSeriesEpisodeFilesWithPositions` | where a person is in a series | V | in flight: `ordering_name` argument |
| `internal/store/queries/series.sql:40`–`49` `ListLiveSeriesEpisodeSources` | which season listings to fetch | F | the files' order's listing |
| `series.sql:86`–`102` `ListSeriesAwaitingMatch` | a series with an episode no NFO speaks for | M | `collection_member` |
| `internal/store/queries/identify.sql:70`–`78` `MoveEpisodeToSeries`, `MovePositionToOrdering` | joining two series moves the aired position | every ordering | move each position whose ordering the kept series has; drop the rest; re-materialise provider orderings |
| `identify.sql:200`–`207` `ListSeriesEpisodePlaces` | a series' episodes by number, for filing | F | files' order |
| `identify.sql:330`–`336` `ListLibraryAiredPositions` | each title's place, for a pass | F | files' order |
| `identify.sql:339`–`348` `ListLibrarySeriesForPass` | each series with `aired_ordering_id` | F | the files' ordering id |
| `identify.sql:359`–`363` `ListLibrarySeriesEpisodePlaces` | as `ListSeriesEpisodePlaces`, per library | F | files' order |
| `identify.sql:378`–`383` `ListLibraryLiveEpisodeSources` | as `ListLiveSeriesEpisodeSources`, per library | F | files' order |
| `identify.sql:420`–`426`, `:455`–`465`, `:469`–`473` the unnumbered queries | episodes with no number | F | files' order |
| `identify.sql:436`–`451` `ListFilesOfWork` | a work's files and their numbers, for the filing report | M, and F for the numbers | membership by `collection_member`; numbers in the files' order |
| `internal/core/identify/episodes.go:18`–`19` `airedOrdering`; `:340`–`386` `positionEpisode`, `positionUnnumbered`; `:391`–`404` `airedOrderingOf` | a file's numbers placed in aired | F | place in the files' order; when that is a group, place in aired too from the entry (§3.3) |
| `episodes.go:257`–`288` `fromListing` | look a number up in the aired season listing | F | look up in the files' order's listing |
| `identify/series_return.go:108`–`146`, `:187` | returning an episode to its first series, by aired place | every ordering | as `MoveEpisodeToSeries` |
| `identify/snapshot.go:43`–`52`, `:134`–`188`, `:446`–`457` | a pass's snapshot of places | F | files' order |
| `identify/refresh.go:137` | an episode's series, to refresh it | M | series alone |
| `internal/core/library/detail.go:81`–`83` `episodePlace` | the place shown on a title's page | V | person's ordering |
| `detail.go:117`–`191` `SeriesDetail`, `seasonsOf` | seasons, specials last | V | groups in the ordering's order; "specials last" is aired's rule |
| `detail.go:227`–`250` `SeasonPage` | one season | V | one group |
| `library/home.go:35`, `:86` | Continue Watching's episode label | V | person's ordering |
| `internal/core/api/browse.go:118`–`122` `EpisodePlaceBody` | `seasonNumber`, `episodeNumber` | V | add the group's label; keep the numbers |
| `api/browse.go:165`–`184` `SeasonBody`, `CollectionOutput` | the series' seasons | V | add the orderings offered and the one in use |
| `api/browse.go:201`, `:247` `getSeason` | "in aired order" | V | the person's ordering |
| `api/home.go:24`, `:75` | Continue Watching's episode | V | as `EpisodePlaceBody` |
| `internal/core/interest/raise.go:271`–`279`; `interest/series.go` | episodes ahead, for a look (D324 §3) | V | in flight: moves to the member query; pass the resolved ordering |
| `internal/core/library/next_up.go:12`–`14` `OrderingAired` (in flight) | Next Up's caller's ordering | V | the resolver's answer |
| `web/apps/web/src/series.tsx:36`–`38` `seasonName`; `:55`–`85` | "Season N" / "Specials" headings | V | the server's group label |
| `web/apps/web/src/title.tsx:117`–`131`, `:270`–`277` | the link up to an episode's season | V | the group in the person's ordering |
| `web/apps/web/src/continue-watching.tsx:57`–`71` | the `S01E05` label | V | the group's label and number |
| `web/apps/web/src/router.ts:15`, `:59`–`61`, `:135`; `page-interest.ts:19`–`25` | `?season=` | V | a group number in the ordering in use (§7 Q10) |
| `internal/store/queries/preference.sql:25`–`36` `GetWorkOfFile` | a file's series, as a preference scope | — | unchanged: the scope is the series, whatever its ordering |

## 6. Build, in steps

Each step ends green and carries a test that fails when it is reverted (D190). Tests use
fixtures and the fake provider (`internal/core/identify/identifytest`), never TMDB.

1. **The resolver, with only aired.** A `preference` axis `episode_ordering` at scope `work`, a
   function that resolves a person's ordering for a series (§4), and the **V** queries of §5
   taking an ordering name. Nothing visible changes. *Test:* with a second ordering inserted by
   fixture, a preference for it changes the series page's groups, the season page, the episode
   label and Next Up; every play position and event is byte-identical afterwards (D159's
   assertion, as a `TestArch_` claim if it is cited from a `doc.go`).
2. **The schema step** (§3.5): `ordering` kind, source id and label; `group_text` by
   `ordering_id`. *Test:* store tests and the migration window test.
3. **The selector.** `GET /collections/{id}` lists the orderings the series has and the one in
   use; a `PUT` sets the preference. The series page shows the selector when there is more than
   one. *Test:* API and web tests; a series with one ordering shows none.
4. **TMDB groups.** Append `episode_groups` to the series fetch and parse the list; fetch a
   group's contents when chosen, store it as `episode_group/<id>`, materialise its ordering,
   positions and group names. *Test:* a stored *Emberwing*-shaped fixture group makes an ordering
   whose positions match it; an episode it omits has no position; choosing it twice fetches
   once.
5. **Refresh.** A series' refresh refetches its fetched groups and re-materialises them; a group
   no longer listed is kept and marked. *Test:* a changed fixture group moves positions and no
   play state; a removed one keeps them.
6. **The files' order** (D210 §1). An admin sets a series' files' order; identification looks
   numbers up in it, and places aired from the entries (§3.3); the **F** queries of §5 read it;
   optionally Kodi's `tvshow.nfo` group URL sets it. *Test:* a library numbered in a fixture's
   DVD order is named correctly, and its aired positions are the entries'.
7. **A person's own ordering** (D159 §2): start from a copy of any ordering, then move, renumber
   and omit episodes. *Test:* it survives a full rescan and a provider refresh, name included
   (D159's assertion).

Closed-beta B2's acceptance, *"a series in DVD order plays in that order"*, is steps 1–4.

## 7. Open questions for the owner

Conceptual first; each with a recommendation.

1. **Is an ordering a kind (DVD) or a particular list?** TMDB has any number of lists per type,
   contributed by its users and labelled loosely (§2.1). *Recommend:* a particular list, shown by
   its name with its kind beside it. Choosing by kind would pick arbitrarily among *One Piece*'s
   seven Digital orders and would miss *Clone Wars*' chronological one.
2. **Which lists does a series offer?** All of TMDB's, or a filtered set (no partial ones, no
   story-arc ones)? *Recommend:* all, each showing how many of the series' episodes it holds,
   so a partial one like "Ahsoka Tano Essential Episodes" is visibly partial. Filtering would hide
   exactly the orders an enthusiast wants.
3. **What is the default?** The 2026-10-25 note says aired; D210 §5 says a person follows the
   series' files' order until they choose. *Recommend:* keep D210 §5. It is aired unless an
   admin has said the files are in another order, and then a person who never chose sees the
   episodes as the files are numbered.
4. **Where does an episode the chosen ordering leaves out go?** Hidden, or listed after the
   ordering's groups under "Not in this order"? *Recommend:* listed after, never offered as
   Next Up and not raised by a look. Nothing in the library disappears because of a viewing
   choice.
5. **When are lists fetched?** *Recommend:* the list of a series' orderings with the series'
   record (no new request); a list's contents only when a person chooses it or an admin makes it
   the files' order (D170); for a series fetched before this lands, a "Look for other orders"
   action that refreshes the series (D213 §2).
6. **Is the files' order (D210 §1) part of this build?** *Recommend:* after the viewing order
   (step 6), unless a series in the library is known to be numbered in another order. D210 §4
   records the owner was unsure the library still has one.
7. **Whose is a custom ordering?** *Recommend:* it belongs to the series, visible to the
   household and chosen per person, like any other ordering (D159 §1 puts orderings on the
   series and the choice with the person); only its author edits it.
8. **How is an episode labelled outside the aired order?** *Recommend:* in the person's
   ordering everywhere they see it — the group's name and the number in it (*"Disc 2 · 3"*,
   *"Episode 812"*) — with the aired place as a second line on the episode's own page, because
   that is the number other sites and people use.
9. **A person-wide default** (*"DVD order wherever a series has one"*)? *Recommend:* not now. It
   depends on choosing by kind, which Q1 recommends against.
10. **What does `?season=N` mean in a link?** *Recommend:* a group in the ordering in use;
    switching ordering returns to the series' top page. A link one person sends another opens
    the other's ordering at the series' top if their orderings differ (UNVERIFIED that links are
    shared enough to matter).
11. **A provider list that TMDB's users edit** moves Next Up at the next refresh. *Recommend:*
    accept it: play state does not move (D157), and a person who wants a fixed order copies it
    into their own (step 7).
