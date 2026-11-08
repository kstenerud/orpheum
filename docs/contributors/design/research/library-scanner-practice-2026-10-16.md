> **ABOUTME:** How Jellyfin, Kodi, Plex, Sonarr, Radarr, guessit, FileBot and tinyMediaManager
> read file names and folders, from their source where it is open; what their users most often
> complain about; which of their test corpora (and of other sources: parse-torrent-name, PTT,
> anitomy, Medusa, SickChill, tvnamer, the Internet Archive, pre-databases) Orpheum could copy names
> from, under which licence; what each offers for "why was this filed so?"; ~1,900 of their test
> names run through a scratch copy of our parser, and the categories it gets wrong; and
> recommendations for the beta with the owner's questions. Written 2026-10-16, after the owner's
> NAS showed one parse failure making 80,246 empty specials.

# Library scanner practice: research, 2026-10-16

The owner:

> *"I expect that once we open this up for beta, many scanner bugs will come to light given the
> myriad different ways people name their files and arrange their libraries. We should be prepared
> for that. Maybe also look at our competitors to get some ideas."*

What prompted it, the same day: in a series library, `Show/Show - 1x4 - Born to be
King.avi` was not read as S01E04, because the season-x-episode pattern wanted two episode digits.
All 24 episodes became unnumbered specials, and a second bug made each pass give each special a
new title, leaving 80,246 empty "not on disk" specials under one series (both fixed in 0f1a4829
and 055b929d). A randomized test written that day
([random_test.go](../../../../internal/core/identify/random_test.go)) also found that a deleted NFO
failed a library's whole pass, and that some passes were not idempotent.

**Method.** Each competitor's repository was cloned (`--depth 1`, 2026-10-14 to 16) into
`/yoloai/cache/research-scanners/` and read there; file links below point at the upstream
default branch. The parser in [filename.go](../../../../internal/core/identify/filename.go) was
copied, unchanged except for its package name and three constants, into a scratch module outside
the repository, and fed 1,871 names extracted from the competitors' tests (section 5). Issue
trackers and forums were searched online (section 2). **Nothing here was run against the production NAS,
and no competitor was installed or run**: what a competitor does is read from its code or its
documentation, and says which.

---

## 1. How the competitors do it

### 1.1 The strategies, side by side

| | Strategy | What it cannot parse | Corpus (count, licence) |
| --- | --- | --- | --- |
| **Jellyfin** (`Emby.Naming`) | Pure parse: an ordered list of 26 episode expressions, first match wins, with flags (`IsNamed`, `IsOptimistic`, `IsByDate`, `SupportsAbsoluteEpisodeNumbers`); folder layout decides series and season. The provider is asked per episode by number or date. | Stays an episode with no number, in a virtual "Season Unknown" when no season is known. | `tests/Jellyfin.Naming.Tests`: 575 `InlineData` rows (375 TV, 105 video). Repo `LICENSE` is GPL-2.0; every `.csproj` declares `GPL-3.0-only`. |
| **Kodi** (`VideoInfoScanner`) | Parse with 8 default `tvshowmatching` expressions (user-overridable in `advancedsettings.xml`), then **match against the scraper's episode guide**: by (season, episode), then air date, then exact title, then a fuzzy title match ≥ 0.8 over every episode. | Skipped and logged at debug level ("Could not enumerate file"). | Little: `xbmc/video/test` has stack and filename-attribute tests. GPL-2.0-or-later. |
| **Plex** (legacy "Plex Series Scanner", Python) | Parse with 4 `episode_regexps`, 2 date, 2 "standalone" and 6 "just episode" expressions, folder as series/season. The newer scanner is closed. | Not shown in the library. | Not published. Mirrored copies of the legacy scanner say "Copyright (c) 2010 Plex Development Team. All rights reserved." |
| **Absolute Series Scanner** (Plex, third party) | Parse with absolute numbering, AniDB and TVDB mappings downloaded at scan time, `.plexignore`, per-series scan logs. | Logged to a per-series `.scanner.log`. | None. No licence file: all rights reserved by default. |
| **Sonarr** | Parse a release title with **110** `ReportTitleRegex` expressions (144 `Regex` in `Parser.cs`), then match the parsed series title against its own database of series it already monitors, with scene-name mappings (TheXEM). A file whose name fails is parsed from its folder; a special is matched by its title against season 0. | "Unable to parse": left for **Manual Import**, which shows what was parsed and lets a person choose. | `NzbDrone.Core.Test/ParserTests`: 1,656 `TestCase` rows, of them ~690 about episodes (190 single, 80 multi, 172 absolute, 58 daily, 71 season, 34 path, 14 mini-series). GPL-3.0. |
| **Radarr** | Sonarr's design for films: title, year, edition, release group, quality. | As Sonarr. | 1,055 `TestCase` rows; 115 titles, 49 editions, the rest quality and language. GPL-3.0. |
| **guessit** (Python) | A rule engine (rebulk) of properties (title, season, episode, date, part, cd, bonus, film, edition, …) that conflict and are resolved; no provider. | Returns what it found; the caller decides. | YAML: 622 episode, 252 film, 208 various and ~1,100 per-property entries (2,171 `?` keys); plus `cross_parser/` with 943 entries re-expressed from five MIT/MPL parsers. LGPL-3.0. |
| **FileBot** | **Match against the provider's episode list**, not parse alone: it fetches the series' listing and pairs each file with an episode; "strict" (the CLI default) skips a file whose match is not certain, "opportunistic" (the GUI default) always picks the best, even a wrong one (*documented*). | Left for a person to pair. | Commercial ("a license must be purchased for continued use"); old releases were GPL-2.0; the licence of the present source was not verified. |
| **tinyMediaManager** (Java) | Parse (`TvShowEpisodeAndSeasonParser`, 27 patterns incl. anime prepend/append, roman "Part", unicode `⁄`), folders per show, then the scraper. | Not traced in its source this round. | `TvShowTest.java`: 205 `detectEpisode` assertions. **Apache-2.0.** |

### 1.2 Per case

Read from the code unless marked *documented*.

- **Specials.** All take a `Specials` (Plex/ASS also `SPs`, `映像特典`) or `Season 0` folder and
  `S00E..`. Kodi alone also has a `foo.special.mkv` expression matched by title. Sonarr
  ([ParsingService.cs](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/Parser/ParsingService.cs),
  `ParseSpecialEpisodeTitle` → `FindEpisodeByTitle(series, 0, title)`) and Kodi
  ([VideoInfoScanner.cpp](https://github.com/xbmc/xbmc/blob/master/xbmc/video/VideoInfoScanner.cpp),
  around "fuzzy title match") number a file by its title against the provider's listing; **Kodi
  searches every season, Sonarr and we search only season 0** (our `specialNamed`, in
  [episodes.go](../../../../internal/core/identify/episodes.go)). Had we searched every season,
  "Born to be King" would have been placed as S01E04 despite the parse failing.
- **Multi-episode.** Jellyfin has 10 `MultipleEpisodeExpressions` (`1x01-02`, `1x01x02`,
  `S01E01-E02-E03`, `S01E01 - S01E02`) and refuses an end below the start or one followed by
  `p`/`i` (so `S09E14-1080p` is not E14–E108). Sonarr captures repeated episodes (`S01E05E06E07`,
  `S01E05.S01E06`, `103.104`). tinyMediaManager returns a list of every episode.
- **Multi-part (stacking).** Jellyfin
  ([NamingOptions.cs](https://github.com/jellyfin/jellyfin/blob/master/Emby.Naming/Common/NamingOptions.cs),
  `VideoFileStackingRules`): `cd|dvd|part|pt|disc|disk` + a number or `a–d`; Kodi the same plus
  `file`, and a folder stack (`Movie/CD1/`, `Movie/CD2/`). Both play the parts as one item.
- **Anime absolute numbering.** Sonarr's largest single area (42 of its 110 expressions are commented "Anime"; 172
  tests), resolved against TheXEM scene mappings. Jellyfin marks expressions
  `SupportsAbsoluteEpisodeNumbers` and tries them in a second round. ASS maps absolute numbers to
  TVDB seasons with downloaded tables. None parse it reliably without a provider listing.
- **Date-based (daily shows).** Jellyfin: `yyyy.mm.dd` and `dd.mm.yyyy` (`IsByDate`) and the
  provider matched by air date; Kodi the same, matched by `cDate`; Sonarr ~10 expressions,
  including Plex DVR's and short month names; Plex legacy 2.
- **Multiple versions.** Jellyfin groups files in one folder sharing the folder's name plus
  ` - <label>` as alternate versions (`VideoListResolver`); Plex *documented* the same `Movie
  (Year) - 1080p.ext`; Plex's `{edition-…}` tag *documented*.
- **DVD/Blu-ray folders and ISO.** Jellyfin
  ([BaseVideoResolver.cs](https://github.com/jellyfin/jellyfin/blob/master/Emby.Server.Implementations/Library/Resolvers/BaseVideoResolver.cs))
  treats a `VIDEO_TS` folder (with `VIDEO_TS.IFO`) or `BDMV` folder as **one video**, and opens an
  `.iso` with a UDF reader to tell DVD from Blu-ray. Kodi plays both. tinyMediaManager's test
  `Dexter S01E01 S01E02 S01E03/VIDEO_TS/VIDEO_TS.VOB` expects the folder's markers to number it.
- **Samples and system folders.** Jellyfin's
  [IgnorePatterns.cs](https://github.com/jellyfin/jellyfin/blob/master/Emby.Server.Implementations/Library/IgnorePatterns.cs):
  `sample` (and Hungarian `minta`) as file, suffix or folder; **NAS and OS folders**: `@eaDir`,
  `#recycle`, `@Recycle`, `.@__thumb`, `$RECYCLE.BIN`, `System Volume Information`,
  `lost+found`, `.snapshot(s)`, `.zfs`, `metadata`, `extrafanart`, every dot-folder; plus
  `.ignore` files. Kodi excludes `@eaDir`, `.AppleDouble`, `.DS_Store`, iTunes extras, `-trailer`,
  `sample`, `proof`, `subs`. ASS adds `Network Trash Folder`, `Temporary Items`, `_`-prefixed files,
  recaps, OST/soundtrack.

### 1.3 What the strategies say

1. **Everyone who parses alone keeps growing a regex list**, and each list carries a guard born
   of a bug: Jellyfin refuses seasons 200–1927 and above 2500 so `(1920x1080)` is not S1920E1080;
   Sonarr has a "CrapParserFixture" of 30 names it must *not* parse; Kodi warns that its last
   expression "is greedy and will match years in show names. It should always be last."
2. **The ones users call reliable for odd naming match against the listing** (Kodi's episode
   guide fallback, Sonarr's series database and scene mappings, FileBot's whole design). A parse
   proposes; the provider's list of what exists disposes. We already have the listing (D209 fetches
   one per season) and use it only for specials' names.
3. **What happens to the unparseable is the user-visible part.** Plex and Kodi drop it (the
   owner's D163 complaint); Jellyfin keeps it unnumbered; Sonarr parks it for
   a person with the parse shown. We do Jellyfin's, and D163 makes it visible.

---

## 2. Their known failure modes

Searched by the research agent on 2026-10-16; raw results are cached under
`/yoloai/cache/research-scanners/web/`. **The counts are keyword matches on issue and topic
titles, not classified issues**, and the ranking is judgement across them: "JF" is Jellyfin's
GitHub issues, "PlexF" and "SonarrF" the Plex and Sonarr forums (title search, capped at 300),
and "Mirror" the 122 Radarr issues that each port one Sonarr "Fixed: Parsing …" commit, split by
keyword, which is a fair proxy for which shapes break a regex parser. Reddit returned 403 and
gave no evidence; the Jellyfin and Kodi forums gave threads but no counts.

| Rank | Category | Evidence (counts) | Examples |
| --- | --- | --- | --- |
| 1 | **Anime and absolute numbering** | Sonarr 138 issues; SonarrF 222; Mirror 27 of 122 (the largest share); guessit 21; JF 19; Plex's ASS thread 843 posts | [JF #2446](https://github.com/jellyfin/jellyfin/issues/2446) anime with several seasons; [JF #16406](https://github.com/jellyfin/jellyfin/issues/16406) every episode over 99 in season one; [Radarr #4779](https://github.com/Radarr/Radarr/issues/4779) absolute numbers over 999 |
| 2 | **Episode order** (aired, DVD, absolute; TMDB against TVDB) | PlexF 111 + 52; Sonarr 37 | [Plex thread](https://forums.plex.tv/t/support-alternate-order-flexible-seasons-tvdb-implemented-and-episode-groups-tmdb-open/537737) (199 posts); [Sonarr #255](https://github.com/Sonarr/Sonarr/issues/255); [JF #10937](https://github.com/jellyfin/jellyfin/issues/10937) |
| 3 | **Multi-episode files** | PlexF 138 + 15; Sonarr 33; Mirror 10; JF 7 | [JF #18081](https://github.com/jellyfin/jellyfin/issues/18081) multi-episode files merged as versions of their first episode; [Sonarr #8878](https://github.com/Sonarr/Sonarr/issues/8878) spaced season and episode parsed as a pack |
| 4 | **Specials and season 0** | PlexF 199; Sonarr 52; SonarrF 60; JF 14 + 5 "Season Unknown" | [JF #12202](https://github.com/jellyfin/jellyfin/issues/12202); [JF #14804](https://github.com/jellyfin/jellyfin/issues/14804) "Season Unknown" beside a Specials folder |
| 5 | **Wrong work matched** (same name, remakes, years, aliases) | PlexF "wrong show" 163, "wrong movie" 124; Radarr 111; Sonarr 53 | [Plex thread](https://forums.plex.tv/t/metadata-always-wrong-when-movie-has-same-name-but-different-year/539231); [Radarr #7331](https://github.com/Radarr/Radarr/issues/7331); [Sonarr #8768](https://github.com/Sonarr/Sonarr/issues/8768) alternate titles |
| 6 | **Numbers in a name read as season or episode** (1080p, years, titles with numbers) | JF 26 | [JF #3669](https://github.com/jellyfin/jellyfin/issues/3669) 1080 as the episode; [JF #1180](https://github.com/jellyfin/jellyfin/issues/1180) episodes over 100 as S01E00; [JF #6928](https://github.com/jellyfin/jellyfin/issues/6928); guessit #774 "2049" as S20E49 |
| 7 | **Dependence on folder layout** (flat folders, mixed libraries) | — | [JF #15778](https://github.com/jellyfin/jellyfin/issues/15778); [JF #3074](https://github.com/jellyfin/jellyfin/issues/3074) a film ending in a number taken for a show; [JF #921](https://github.com/jellyfin/jellyfin/issues/921) |
| 8 | **Daily, date-numbered shows** | Sonarr 83; SonarrF 52; PlexF 37; JF 11 | [JF #10253](https://github.com/jellyfin/jellyfin/issues/10253) "2023 09 22" as season 20; [JF #1398](https://github.com/jellyfin/jellyfin/issues/1398) |
| 9 | **Versions merged wrongly, or not merged** | JF 74 (some about software versions) | [JF #718](https://github.com/jellyfin/jellyfin/issues/718); [JF #17885](https://github.com/jellyfin/jellyfin/issues/17885) episodes merged as versions |
| 10 | **Extras taken for films or episodes** | JF 24 | [JF #6630](https://github.com/jellyfin/jellyfin/issues/6630); [JF #8641](https://github.com/jellyfin/jellyfin/issues/8641) a show called "Extras" not added |
| 11 | **Split episodes and stacked parts** | — | [JF #6493](https://github.com/jellyfin/jellyfin/issues/6493); [JF #8750](https://github.com/jellyfin/jellyfin/issues/8750); [JF #10493](https://github.com/jellyfin/jellyfin/issues/10493) |
| 12 | **Samples, both ways** | — | [JF #10656](https://github.com/jellyfin/jellyfin/issues/10656) files with "Sample" in the name vanish; [JF #17961](https://github.com/jellyfin/jellyfin/issues/17961); [JF #5819](https://github.com/jellyfin/jellyfin/issues/5819) samples shown as duplicates |
| 13 | **Non-English and non-Latin names** | Mirror 6 for Chinese anime alone | [Radarr #2355](https://github.com/Radarr/Radarr/issues/2355); [JF #3272](https://github.com/jellyfin/jellyfin/issues/3272) "Le cœur a ses raisons" |
| 14 | **Disc folders and images** | JF ~60 titles, mostly playback | Plex excludes them outright; [JF #10975](https://github.com/jellyfin/jellyfin/issues/10975) BDMV support broken in 10.9.0 |
| 15 | **"1x01" style** | JF 14 (full text) | rare as a complaint: every scanner has an expression for it, as we did until "1x4" ([JF #2171](https://github.com/jellyfin/jellyfin/issues/2171)) |

What the vendors document about the unparseable: Plex, in
["Why is some of my content not found?"](https://support.plex.tv/articles/201543057-why-is-some-of-my-content-not-found/),
*"The movie scanners will skip content that is named like a television episode … the TV
scanners will skip content that is not named like a television episode"*, and disc images and
archives are excluded: silently skipped, not shown. Plex's
[`.plexmatch`](https://support.plex.tv/articles/plexmatch/) file (PMS 1.25.9+) lets a person pin a
show's title, year and ids and map single files (`ep: SP01: file`); since 1.30.0 also a
`pattern:` with `{s}`/`{e}` tokens. Jellyfin's
[shows docs](https://jellyfin.org/docs/general/server/media/shows) say a multi-episode file
appears as one entry and recommend splitting it, and advise against naming an unknown special
`S00Exy`.

**Read against ours.** Ranks 1, 3, 4, 6, 8, 11 and 15 are parser shapes that section 5 measures
in our parser too; 2 and 5 are matching (D96, D172, D301), not parsing; 7, 9 and 10 are folder
rules that D162, D208 and the owner's "a series library never makes a film" address; 14 is a
scope question (Q5). Rank 12's "files with Sample in the name vanish" is the false positive our
`isSampleFile` avoids by matching only `sample`, `.sample` and `-sample` stems.

---

## 3. Can we reuse a corpus?

### 3.1 The facts, per source

Measured in the clones on 2026-10-16. "Names" counts test inputs with an expected answer.

| Source | Names | What it covers | Licence |
| --- | --- | --- | --- |
| [tvnamer](https://github.com/dbr/tvnamer) `tests/test_files.py` | 63 (plus a few in other tests) | `[04x19]`, multi-episode, anime, date-based, "x of y", no series name, unicode | **Unlicense** (public domain dedication) |
| [parse-torrent-name](https://github.com/divijbindlish/parse-torrent-name) `tests/files` | 76 | release names, films and episodes | **MIT** |
| [parse-torrent-title](https://github.com/platelminto/parse-torrent-title) (PTN fork) `tests/files` | 407 | release names | **MIT** |
| [PTT / parsett](https://github.com/dreulavelle/PTT) `tests/*.py` | ~1,040 parametrized cases (182 episodes, 103 seasons, the rest quality, language, dates, anime, extras) | release names | **MIT** |
| [anitomy](https://github.com/erengy/anitomy) `test/data.json` | 251 | fansub anime names, each with its MyAnimeList id | **MPL-2.0** |
| [tinyMediaManager](https://gitlab.com/tinyMediaManager/tinyMediaManager) `TvShowTest.java` | 205 | episodes in folders, anime, unicode slashes, VIDEO_TS, multi-episode, "Part II" | **Apache-2.0** |
| [guessit](https://github.com/guessit-io/guessit) `guessit/test/*.yml` | 2,171 keys (622 episodes, 252 films, 208 various, ~1,100 per property) | the broadest: paths with folders, films, episodes, bonus, cd, part, dates, many languages | **LGPL-3.0** |
| guessit `test/cross_parser/*.json` | 943 | PTT 453, anitomy 180, thcolin 147, go-ptn 87, PTN 76, re-expressed | each upstream's own: MIT, or MPL-2.0 for anitomy (its NOTICE.md) |
| [Jellyfin](https://github.com/jellyfin/jellyfin) `tests/Jellyfin.Naming.Tests` | 575 rows | folder layouts, seasons, specials, multi-episode, daily, absolute, stacking, extras | repo `LICENSE` **GPL-2.0**; `.csproj` files say **GPL-3.0-only** |
| [Sonarr](https://github.com/Sonarr/Sonarr) `ParserTests` | ~690 episode rows of 1,656 | release names and some paths; anime, daily, multi, mini-series | **GPL-3.0** |
| [Radarr](https://github.com/Radarr/Radarr) `ParserTests` | ~165 title/edition rows of 1,055 | film release names | **GPL-3.0** |
| [Medusa](https://github.com/pymedusa/Medusa) `tests/test_guessit.yml`, `tests/name_parser` | 402 YAML keys; ~50 parser cases | release names, anime, air-by-date | **GPL-3.0** |
| [SickChill](https://github.com/SickChill/SickChill) `tests/test_name_parser.py` | 84 `ParseResult`s | release names | **GPL-3.0** |
| Kodi | negligible | — | GPL-2.0-or-later |
| Plex, Absolute Series Scanner, FileBot | none published, or no licence | — | not usable |

### 3.2 What the licences mean for copying names into AGPL-3.0 tests

Stated as facts for the owner's decision; **this is not legal advice, and none of it was put to a
lawyer.**

- **Are test file names copyrightable at all?** A single name ("Show.S01E02.720p.mkv") is very
  probably not a protected work; a *curated collection* of names with expected answers may be
  protected as a compilation or database (the EU database right exists separately from
  copyright). Copying a whole fixture file is the case the licence governs; writing new names of
  the same *shapes* is not copying.
- **Public domain, MIT, Apache-2.0:** may be copied into AGPL-3.0 tests. MIT needs its notice
  kept; Apache-2.0 needs its notice and a NOTICE file kept if there is one (Apache-2.0 is
  compatible with GPL-3.0 and so with AGPL-3.0 one way, per the FSF).
- **MPL-2.0 (anitomy):** file-level copyleft. Kept as a separate data file with its notice, it can
  sit in an AGPL project (MPL-2.0 §3.3 allows combination with GPL-family licences unless the file
  says "Incompatible With Secondary Licenses"; no file in anitomy's clone carries that notice,
  only the licence text's own template). Orpheum's
  own `web/` is MPL-2.0 already.
- **LGPL-3.0 (guessit):** LGPL-3.0 is GPL-3.0 plus extra permissions; material under it can be
  used under GPL-3.0, and GPL-3.0 §13 allows combining with AGPL-3.0. Copying the YAML into
  Orpheum's core tests is possible with attribution, the copied file staying under LGPL-3.0.
- **GPL-3.0 (Sonarr, Radarr, Medusa, SickChill):** GPL-3.0 §13 and AGPL-3.0 §13 each allow
  combining a GPL-3.0 work with an AGPL-3.0 one; the combination is conveyed with each part under
  its own licence. So copied fixtures are possible, kept in their own file with the notice.
  guessit's maintainers excluded Sonarr's and Radarr's fixtures for the opposite reason (GPL-3.0
  into LGPL-3.0 is not allowed); that does not apply to an AGPL-3.0 project.
- **GPL-2.0 (Jellyfin's `LICENSE`) is not compatible with (A)GPL-3.0** unless "or later". The
  repository's `.csproj` files declare `GPL-3.0-only`, which contradicts the `LICENSE` file; which
  governs `tests/` is unclear. **Treat Jellyfin's fixtures as not copyable** until that is
  settled, and write our own names of the same shapes.
- **A decision the owner has not been asked:** whether Orpheum's core should ever hold third-party
  copyleft test data at all, given the project's licensing choices
  ([tech-licensing.md](tech-licensing.md)). MIT/Apache/Unlicense sources avoid the question.

### 3.3 The Internet Archive

Checked with a small sample (two searches, six items, cached under
`/yoloai/cache/research-scanners/ia/`):

- **It works without an account.** `advancedsearch.php?q=collection:classic_tv AND
  mediatype:movies&fl[]=identifier&output=json` reported 11,025 items in `classic_tv` and 28,518
  in `feature_films`; `https://archive.org/metadata/<identifier>` returns each item's files as
  JSON, with `source: original` marking the uploaded file (the rest are derivatives).
- **Limits.** Sorted paged search stops at 10,000 results; the cursor-based
  `/services/search/v1/scrape` API is for deeper walks
  ([aboutsearch.htm](https://archive.org/help/aboutsearch.htm)). No read rate limit is published;
  practitioners report 503s under fast automated search and pace requests (0.3–0.5 s). **The
  terms page (archive.org/about/terms) is rendered by JavaScript and was not read**: the owner
  should read it before a harvest.
- **What the names are like.** Uploaders' own, and close to home naming: `Lockfast (1989) SE7 - EP5
  Piper on the Pier.mp4`, `Belle Starling TV 1954 colorized s01e02 Belle Trusts a Stranger.mp4`,
  `Lamplight Hour (October 13, 1978).mp4`, `Lanternless 2011 dvdrip.avi`, a season as one item of
  `Ned Wicket - S01E01 - New Leaf.avi` … Most items hold one video, with no folder structure, and
  the item's metadata (title, date, collection) is a rough expected answer that a person must
  check.
- **A one-off harvest** would be a script outside the repository (it is network access, which
  tests never do, rule 16): page the search for a few collections, fetch each item's metadata
  at a polite pace, keep the original video file names and the item title, and commit a few
  hundred hand-checked rows as a fixture with the source identifier per row. The names are facts
  about public items; the item metadata is mostly CC0-like but varies per item, so keep only the
  names and our own expected answers.

### 3.4 Pre-databases

[predb.ovh's API](https://predbdotovh.github.io/pre-api/) (the site did not resolve from this
sandbox; its documentation did) holds scene release names with team, category, size and file
count. It states it was *"built for my personal usage"*, *"does NOT host any content, only
metadata"*, limits search to 30 requests a minute and 1,000 results a query, and states **no
licence and no terms for reuse**. predb.net was not checked. The names are the most standardised
there are: they test release tags and group suffixes, which our parser cuts by a word list, not
home naming or folder layout.

### 3.5 Home naming is a different population

The owner's point: *"the office s2 ep3 FINAL.mp4"* is not a release name. The sources differ in
what they cover:

| Population | Example | Who covers it |
| --- | --- | --- |
| Scene/P2P release names | `Show.S02E03.720p.WEB-DL.x264-GRP.mkv` | Sonarr, Radarr, PTT, PTN, predb: thousands |
| Fansub anime | `[Group] Title - 05 [1080p][ABCD1234].mkv` | anitomy, Sonarr absolute, guessit |
| Library layouts (folders) | `Show (2005)/Season 1/…`, `Movie (1999)/Featurettes/…` | Jellyfin, guessit's paths, tinyMediaManager, our own corpus |
| Home naming | `the office s2 ep3 FINAL.mp4`, `Flatmates of Ferris Row season 1 episode 3.mp4`, `Fennimore - 3.04 - The Pen.mkv`, `Metropolis (1995) (1).mkv`, `IMG_1234.MOV` | almost no one: a few in tvnamer and tinyMediaManager; the Internet Archive's uploads come closest |
| Disc rips | `Movie/VIDEO_TS/VTS_01_1.VOB`, `BDMV/STREAM/00001.m2ts`, `.iso` | Jellyfin's resolver tests, tinyMediaManager's two cases |

**The beta's own reports are the only large source of home naming**, which is the case for
section 4's report.

---

## 4. Preparing for beta: making a misfiling cheap to find and fix

### 4.1 What the competitors offer

| Product | Explain why a file was filed so | Test a name | Fix one file |
| --- | --- | --- | --- |
| **Sonarr / Radarr** | Trace logs name the expression that matched and the result ([wiki](https://wiki.servarr.com/sonarr/troubleshooting)); Manual/Interactive Import shows the parsed series, season, episode and rejection reasons | **Yes**: `GET /api/v3/parse?title=…&path=…` ([ParseController.cs](https://github.com/Sonarr/Sonarr/blob/develop/src/Sonarr.Api.V3/Parse/ParseController.cs)); a "Test parsing" pop-up in v4 (release notes only; Radarr's `frontend/src/Parse/Parse.tsx` confirmed) | Manual Import: choose series, season, episode |
| **Jellyfin** | Nothing per item; debug logging only by hand-editing `logging.json` ([troubleshooting](https://jellyfin.org/docs/general/administration/troubleshooting)) | No | Identify; NFO; ids in folder names |
| **Plex** | `Plex Media Scanner.log`, which users say often fails to name the file ([forum](https://forums.plex.tv/t/plex-media-scanner-log-more-detail-required/304541)) | No | Fix Match / Unmatch; `.plexmatch`; the community's last resort is the ["Plex Dance"](https://forums.plex.tv/t/the-plex-dance/197064): move the files out, scan, empty trash, clean bundles, move them back, scan |
| **Absolute Series Scanner** | Per-series `.filelist.log` and `.scanner.log`, naming each ignored file and the rule that removed it (read from its source) | No | Id files; `.plexignore` |
| **Kodi** | Debug log with per-component extras ([wiki](https://kodi.wiki/view/Log_file/Advanced)); that it names the matching expression is a forum statement, not checked | No | Editable `tvshowmatching` expressions; NFO |
| **FileBot** | The pairing, shown before renaming; "strict" skips unsure pairs, "opportunistic" always picks one ([forum](https://www.filebot.net/forums/viewtopic.php?t=4695)) | The GUI is the test | Manual pairing |
| **tinyMediaManager** | No dedicated view found in its [docs](https://www.tinymediamanager.org/docs/tvshows/settings) | No | Edit the episode; `.tmmignore` |

Sonarr's is the model worth copying: the parse is a pure function exposed to a person, and its
result is shown where a person decides. No competitor stores *why* a file is where it is; D163
already goes further than all of them for what was passed on, and 4.3 extends it to what was
filed.

### 4.2 What we already have

- **D163:** every file the node lists has one filing from a closed set with a reason, and the
  admin's files view ([files.tsx](../../../../web/apps/web/src/files.tsx)) groups files by filing and
  reason. **But a file filed `listed` carries no reason** (titles.go writes `""` for it): an
  episode filed as an unnumbered special, or as S01E05 when the file said season 3, is `listed`
  and says nothing about why. That failure was invisible there.
- **D301:** a guessed match is marked, and "Identified incorrectly" opens the chooser — for a
  film's or series' match, not for an episode's number or a file's kind.
- **D208 §1, D172:** a person's confirmation wins over every later pass, so a fix sticks.
- **Invariants and the randomized walk** ([random_test.go](../../../../internal/core/identify/random_test.go)):
  re-running writes nothing; titles never outnumber files seen.

### 4.3 What would make a misfiling cheap

1. **A reading on every filing (an "explain").** Store, beside the filing, what the parser read
   and which rule placed it: `path → parse {kind, name, year, season, episode, end, which
   expression, from file or folder} → placement {series by folder X / tvshow.nfo, season by
   folder "Season 2", episode by marker "1x4", or unnumbered: no marker found}`. The pass already
   computes all of it; keeping it costs a column, and a pure parser can recompute it on demand
   instead (D182: re-derivation reads only stored input), so it could be computed when shown and
   never stored. Shown on the title page (admin), and as the files view's reason for `listed`.
   This is Sonarr's Manual Import column of parse results, and Absolute Series Scanner's
   per-series log, built into the one surface D163 already gives.
2. **"This is filed wrongly" from the title or episode page**, beside D301's "Identified
   incorrectly": it opens the explain view with the fix the person can make here (number this
   episode, this is a film, this is an extra of …, ignore this folder), and offers to make a
   **report**. The report is a file the admin downloads or copies (we have no telemetry, D13): the
   relative path, the parse, the placement, the library kind, Orpheum's version, and optionally
   the folder's sibling names (which often decide the parse). **Paths are personal** (names of
   home videos, of people), so the report is shown in full before it leaves, with a "replace
   names with shapes" option that keeps digits, separators and known tokens and replaces other
   words with placeholders (`Xxxxx Xxxxx - 1x4 - Xxxx.avi`), which keeps what a parser test
   needs.
3. **Every report becomes a corpus row.** A table-driven test over a fixture file of reported
   paths with the expected parse, each row citing its report; the fix and the row land in one
   commit, and rule 4's revert-red already guarantees the row fails without the fix.
4. **Corpus breadth before beta**, from section 3's copyable sources, run as a *survey* test that
   reports a pass rate and lists failures without failing the gate, alongside the hand-checked
   corpus that does fail it. The survey tells us where we stand; the corpus holds what we fixed.
5. **Property tests on the parser**, beside the pass-level walk: for generated names in the shapes
   above, (a) an `S01E04` name and its `1x4`, `1x04`, `S1E4`, `s01.e04` spellings parse alike;
   (b) adding release tags, a group suffix, a site prefix or a `[group]` prefix changes no field
   but nothing; (c) the parse is the same whether the marker is in the file or only in its single
   episode folder; (d) a season folder's number and the file's marker, where both exist, agree or
   the case is flagged.
6. **Growth and shape guards in the pass**, run as invariants and as warnings at runtime:
   unnumbered specials in one series exceeding its numbered episodes (one series: 24 of 24); one
   series holding more than *n* times its files' count of titles (80,246 for 24 files); a season
   with episodes numbered far beyond its provider listing; many files in one folder filed as
   distinct films. Each is a cheap query, and each would have flagged today's bugs on the first
   pass rather than the thousandth.
7. **Diagnostics without telemetry:** a per-library "scan report" an admin can open (counts by
   filing and by placement rule, the growth guards' warnings, the slowest files), and the logs
   saying, at the debug level the owner wants controllable at runtime, each file's parse and
   placement, so a support thread can ask for one library's report instead of a database.

---

## 5. Findings against our parser

### 5.1 How it was run

A scratch copy of `filename.go` (package renamed; `library.TitleKindEpisode`,
`parseNonNegativeInt`, `isSeasonFolder`, `extraTypeOfFolder` and the extra constants copied
beside it) in a module under the session's scratch directory, **not in the repository**. 1,871
cases were extracted from tinyMediaManager (199), guessit (episodes 621, films 251), Sonarr
(single 185, absolute 164, multi 77, daily 37, path 33, mini-series 11), Radarr (47) and
Jellyfin (epnum 77, multi 63, season 43, simple 41, no-season 10, daily 5, absolute 7), each
with its project's expected season, episode, end, title or year; a name with no video extension
was given `.mkv`. Each was run through `ParsePath` (a film or mixed library) and
`ParseEpisodePath` (a series library). About 80 more were hand-written for the categories below,
and 20 for home naming. The extraction is approximate (a regex over test source): some expected
answers were misread, and those are excluded from the examples below.

**Headline:** of 1,402 cases where a competitor expects a season or episode number, our series
library parse **disagrees on 745 (53%)**: **631 become unnumbered specials** (visible, at least)
and **114 get a wrong number or season** (silent). This overstates the damage to real libraries,
because most of these corpora are release names with no folders and our parser leans on folders;
it does not overstate the categories.

### 5.2 Categories we get wrong, by count

| Count | Category | Examples (our result) |
| --- | --- | --- |
| 163 | **Absolute numbering and fansub layout**: `Title - 05`, `Title 05 [hash]`, `[Group] Title - 1085` | `One Piece/One Piece - 1001.mkv` → special; `[SubsPlease] One Piece - 1085 (1080p)` → special; `Attack on Titan/Season 3/[HorribleSubs] Shingeki no Kyojin S3 - 50` → special |
| 95 | **Season and episode as separate words** | `the office s2 ep3 FINAL.mp4` → **S1E3**; `Lockfast (1989) SE7 - EP5 …` → **S1E5**; `Show S03 EP05 Name` → **S1E5**; `Series Title S02 01` → special; `Staffel 2 Folge 5` → special |
| 94 | **Season and episode packed into digits** | `Show - 203 - Name` in `Season 02` → **S2E203**; `Series.Title.103.720p` → special; `lzbp101.avi` → special |
| 66 | **Multi-episode beyond `S01E01-E02`** | `S01E01-03` → end 0; `S01E01E02E03` → E1–E2; `1x01-1x02`, `S01E01.S01E02`, `1x01_1x02` → single episode |
| 65 | **A bare episode number, leading or trailing** | `Season 1/01 Pilot.mkv`, `Season 1/1. Pilot.mkv`, `Season 2/02.avi` → special; `Show/Show - 02.avi` → special (`dashNumberPattern` wants a dash *after* the number too) |
| 64 | **Four-digit seasons** (year-numbered shows) | `Running Man S2017E368`, `Doctor Who S2023E01`, `Season 2009/2009x02` → special |
| 40 | **`NxN` variants** | `S01x02`, `S01xE02`, `1x01x02`, `02x03x04x15` → special; `Show 1×02` (U+00D7) → special |
| 42 (37+5) | **Date-based episodes** | `The Nightly Lantern 2024-03-05 Guest.mkv`, `Last.Week.Tonight…2023.10.08` → special; `Lamplight Hour (October 13, 1978).mp4` → special named "Lamplight Hour (October 13" |
| 12 | **"N of M"** | `Chernobyl (1 of 5)`, `Braunschlag (1⁄8)` → special |
| 8 | **Marker only in the release folder** (obfuscated file) | `Series.Title.S01E01.720p.HDTV/ajifajjj_eppj.avi`, `showname S01E01/ijfi38js.mkv` → special |
| 8 | **"Part N" mini-series** | `Band of Brothers - Part 3 - Carentan`, `Band.of.Brothers.Part.III` → special |
| — | **Non-English words** (inside the 95 and the "other" group) | `Saison 2 Épisode 5`, `Stagione 6 … Episodio 13`, `Сезон 3 Эпизод 21`, `第3話`, `T02E22` → special or season 1 |

### 5.3 Defects found that are not categories of naming

These are bugs in code that exists, each shown by a name a competitor tests:

1. **A leading `[group]` is not removed**, though `cleanTitle`'s comment says bracketed groups go:
   `[Judas] Series Title (2024) - S01E14` names the series `Judas] Series Title`;
   `[KESTREL.EX] Movie Name (2019) 1080p` names the film `KESTREL.EX] Movie Name`, which no provider
   search will find. In a series library the folder usually names the series instead; in a film
   library, or a loose file, it does not.
2. **A title that is a year, followed by a year, loses its year when separated by one character**:
   `1917.2019.1080p.mkv` → name `1917 2019`, no year; `2012.2009.720p…` the same.
   `yearPattern.FindAllStringSubmatchIndex` consumes the separator after `1917`, so `2019` has none
   before it and is not found. `1917 (2019)` works.
3. **Years before 1900 are not years**: `Leaving Movie by Movie (1897)` keeps `(1897)` in the name.
4. **A season said in words or as `S03`/`SE7`/`s2` beside a separate episode marker falls back to
   season 1 silently** when no season folder exists (section 5.2's 95). This is worse than a
   special: it is wrong and looks right.
5. **NAS and OS system folders are read as media**: the node's skips are only the root's `.trash`,
   `sample`, AppleDouble, extension, symlinks out of roots and non-regular files
   ([report.go](../../../../internal/nodeproto/report.go)). `#recycle/Movies/Old Film (1950).mkv`
   (Synology's recycle bin) is a film; in a series library `#recycle` and `.snapshot` become
   series. Jellyfin, Kodi and ASS all skip `@eaDir`, `#recycle`, `@Recycle`, `$RECYCLE.BIN`,
   `.snapshot`, `lost+found`, `System Volume Information`.
6. **Disc structures are filed file by file**: `Movie Name (1979)/VIDEO_TS/VTS_01_1.VOB` is a film named
   "VTS 01 1" (the folder fallback looks only at `VIDEO_TS`), and each `.VOB` another;
   `BDMV/STREAM/00001.m2ts` a film named "00001", with a Blu-ray's dozens of clips each a title.
   `vob` and `m2ts` are in the default extensions. `.iso` is not, so an ISO library is all
   "skipped: extension" — visible, at least.
7. **Multi-part films become versions**: `The Big Parade (2003) - cd1.avi` and `- cd2.avi` parse alike,
   so they are filed as two versions of one film; a person picking one plays half. D79's model
   has room for parts ("the multi-part-plus-multi-version case has somewhere to go"); the parser
   and pass do not use it.
8. **Site prefixes miss common TLDs**: `www.Tamilblasters.party - The Loom of Hours (2021)`,
   `www.5MovieRulz.tc - …` keep the site in the name (`sitePrefixPattern` lists 14 TLDs).
9. **A bare IMDb id**, `[tt1234567]`, is not read as a provider id (only `[imdbid-tt…]` and
   `{imdb-tt…}` are).

What we get right that others test for: S01E02 forms with any separator, `S01E03-E04`, season
packs and per-episode folders (the owner's NAS cases), unicode names (D49), provider ids in
`{tmdb-…}`/`[imdbid-…]`, `Season N`/`Staffel`/`Saison`/`Temporada` folders, specials folders,
D162's extras folders and suffixes, `sample` files and folders, `7th Heaven (2004)` staying a film in a
film library, and a series library never making a film.

---

## 6. Recommendations, by value for the beta

1. **Explain every filing, and make it reachable from the title page** (4.3 §1). It is the
   cheapest fix for the most expensive thing about scanner bugs, which is finding out which rule
   did it. It is also what every later report needs. Value: highest; cost: small (the pass knows
   it all already).
2. **Number unnumbered episodes by name against every season's listing**, not only season 0's
   (Kodi's fallback). It turns a whole class of parse failures (`01 Pilot`, `Part 3 - Carentan`,
   a series' bare episode number) into correct placements whenever the file carries the episode's title, and costs
   nothing new: the listings are already fetched (D209). Needs the owner (Q2).
3. **Never default the season to 1 when the name says a season** (5.3 §4): parse `S03 EP05`,
   `SE7 - EP5`, `s2 ep3`, `Season 2 Episode 5`, `Series 5 - Episode 1` and the foreign words in
   `seasonFolderPattern`'s list, or else leave the file unnumbered rather than placing it in
   season 1.
4. **Skip NAS and OS system folders at the node** (5.3 §5), as D163 skip rules with their own
   reasons so they stay visible: `@eaDir`, `#recycle`, `@Recycle`, `#snapshot`, `.snapshot`,
   `.zfs`, `$RECYCLE.BIN`, `System Volume Information`, `lost+found`, `.@__thumb`, and dot-folders.
   Beta users on Synology and QNAP will meet it on day one.
5. **Fix the small parser defects** (5.3 §1–3, §8, §9): leading `[group]`, the `1917.2019` year,
   pre-1900 years, more site TLDs, bare `tt` ids. Each is a corpus row and a few lines.
6. **Widen episode numbering to the common shapes**, each with corpus rows: `Title - 05` with
   nothing after; leading numbers in a season folder (`01 Pilot`, `1. Pilot`, `02.avi`);
   `S01x02`/`1x01x02`/`×`; four-digit seasons with Jellyfin's 200–1927 guard; multi-episode
   ranges and repeats (with Jellyfin's "end ≥ start, not followed by p/i" guard); a marker in the
   single episode's folder when the file has none; `N of M` and `Part N` in a series library.
   Absolute numbering and packed digits (`103`) are ambiguous by nature and are better settled by
   recommendation 2's listing match than by more expressions.
7. **Build the beta corpus and the report path** (4.3 §2–§4): a fixture of hand-checked rows from
   copyable sources (tinyMediaManager, tvnamer, PTT/PTN, anitomy; others per Q1), a survey test
   over the larger sets that reports without failing, and the "filed wrongly" report that adds
   rows from the field.
8. **Add the growth and shape guards** (4.3 §6), as invariants in the randomized walk and as
   runtime warnings in the scan report.
9. **Date-based episodes**: parse `yyyy-mm-dd`, `yyyy.mm.dd`, `dd.mm.yyyy` (and written dates)
   in a series library and place them by air date against the listing. Needs the owner (Q4).
10. **Disc structures and multi-part films**: a `VIDEO_TS`/`BDMV` folder as one video, and
    `cdN`/`partN` as parts of one version. Both touch playback and the model, not just the
    parser. Needs the owner (Q5).

No recommendation adds a dependency. A ported guessit or a Go release-name parser would be one;
none was found worth proposing over our own corpus-driven parser, and none was evaluated.

---

## 7. Questions for the owner

**Q1. May the core's tests hold third-party test data, and under which licences?** *The kind of
gap:* a policy about what enters the repository, not about the parser. The copyable sources fall
into three groups: permissive (Unlicense, MIT, Apache-2.0: tvnamer, PTN, PTT, tinyMediaManager),
weak or file-level copyleft (MPL-2.0 anitomy, LGPL-3.0 guessit), and GPL-3.0 (Sonarr, Radarr,
Medusa, SickChill), with Jellyfin's GPL-2.0/3.0 contradiction unusable for now. Options: only
permissive, kept in a fixture file with its notices; permissive plus copyleft data files under
their own licences; or no copied data, writing our own names of the same shapes.

**Q2. Should the series' listing decide an episode's number when the name has none?** *The kind
of gap:* who has authority over placement. Today the path places an episode and the listing only
names it (D209), except for specials. Matching an unnumbered file by its title against every
season would place many more files correctly, but it is a provider answer deciding a position,
and a fuzzy one (Kodi uses ≥ 0.8 similarity): it can place a file wrongly where today it would be
a visible special. Options: never; only on an exact identity-fold match (D232); or fuzzy with a
threshold, marked as a guess the way D301 marks a guessed match.

**Q3. What may a "filed wrongly" report carry, and how does it leave?** *The kind of gap:* privacy
and the no-telemetry promise (D13). The report helps only if it reaches us. Options: a file the
admin downloads and attaches to an issue; a copyable text block; or nothing leaves, and the
explain view is for the admin alone. And: is the "replace names with shapes" anonymiser the
default, an option, or not offered?

**Q4. Are daily and date-numbered shows in scope for the beta?** *The kind of gap:* scope. Talk
shows, news and soap operas are numbered by date, and today every one is a special. Supporting
them means a date on an episode's position and matching the listing by air date (D96 allows
orderings; D159's position is (ordering, season, number)).

**Q5. Disc folders, ISOs and multi-part films: in the beta, or declared unsupported?** *The kind
of gap:* scope, and the model's version/part distinction (D79, D155). Today they produce junk
titles (VIDEO_TS, BDMV) or half-films (cd1/cd2). Options: build them; skip such folders with a
D163 reason saying "not supported yet", which is honest and cheap; or leave as is.

**Q6. Should the parser's shape guards also be runtime alarms?** *The kind of gap:* what the
running server does when an invariant a test checks is broken in the field. E.g. a series with
more specials than numbered episodes, or more titles than files: log, show in the scan report,
or also stop filing that series until an admin looks.

---

## Sources

Repositories (cloned 2026-10-14 to 16, in `/yoloai/cache/research-scanners/`):
[jellyfin/jellyfin](https://github.com/jellyfin/jellyfin) (aec42b3),
[xbmc/xbmc](https://github.com/xbmc/xbmc) (4e68196),
[Sonarr/Sonarr](https://github.com/Sonarr/Sonarr) (a2b6054),
[Radarr/Radarr](https://github.com/Radarr/Radarr) (df2faa6),
[guessit-io/guessit](https://github.com/guessit-io/guessit) (3c73072),
[tinyMediaManager](https://gitlab.com/tinyMediaManager/tinyMediaManager) (5749d47),
[ZeroQI/Absolute-Series-Scanner](https://github.com/ZeroQI/Absolute-Series-Scanner) (b6bfa30),
[doublerebel/plex-series-scanner-bdmv](https://github.com/doublerebel/plex-series-scanner-bdmv)
(a copy of Plex's legacy scanner),
[dbr/tvnamer](https://github.com/dbr/tvnamer),
[divijbindlish/parse-torrent-name](https://github.com/divijbindlish/parse-torrent-name),
[platelminto/parse-torrent-title](https://github.com/platelminto/parse-torrent-title),
[dreulavelle/PTT](https://github.com/dreulavelle/PTT),
[erengy/anitomy](https://github.com/erengy/anitomy),
[pymedusa/Medusa](https://github.com/pymedusa/Medusa),
[SickChill/SickChill](https://github.com/SickChill/SickChill).

Files read: Jellyfin `Emby.Naming/Common/NamingOptions.cs`, `Emby.Naming/TV/EpisodePathParser.cs`,
`Emby.Server.Implementations/Library/Resolvers/TV/EpisodeResolver.cs`, `…/BaseVideoResolver.cs`,
`…/Library/IgnorePatterns.cs`, `MediaBrowser.Providers/TV/SeriesMetadataService.cs`,
`MediaBrowser.Providers/Plugins/Tmdb/TV/TmdbEpisodeProvider.cs`; Kodi
`xbmc/settings/AdvancedSettings.cpp`, `xbmc/video/VideoInfoScanner.cpp`; Sonarr and Radarr
`src/NzbDrone.Core/Parser/Parser.cs`, `ParsingService.cs` and `NzbDrone.Core.Test/ParserTests/`;
tinyMediaManager `TvShowEpisodeAndSeasonParser.java`, `TvShowTest.java`; ASS
`Scanners/Series/Absolute Series Scanner.py` and `README.md`; guessit `guessit/test/` and
`cross_parser/NOTICE.md`.

Online: [Internet Archive search help](https://archive.org/help/aboutsearch.htm),
[predb.ovh API](https://predbdotovh.github.io/pre-api/), and the issue and forum links in
section 2.
