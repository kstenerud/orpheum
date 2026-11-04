# Searching by more than the title (2026-11-04)

> The owner, on the UI audit (ui-audit-2026-11-04.md, "The owner's answers"): revamp the search
> dialog now to search by the other things, genre and the like; research what competitors offer and
> what people use or want. Desk research by a subagent; the repo facts checked by the lead.

## 1. What there is now

- **Reached by** a magnifying glass in a Films or Series list's tools (`listing-tools.tsx`): a
  dialog with one field, Search and Show all. It searches on submit only; no suggestions, no recent
  searches. The query narrows the same list (`query` on `GET /titles`; search is a criterion of the
  listing, the owner, 2026-10-30).
- **Matched** (`internal/store/search.go`, `internal/core/textindex/query.go`): a folded query to
  one of three routes: substring under 3 characters; FTS5 words (name, alternates, overview, each
  word as itself or a prefix, all required, bm25) for Latin text; trigram for other scripts.
- **Searched:** titles in every language, and the overview on the words route. Nothing else.
- **Held and searchable later:** year, runtime, original language, audio and subtitle languages,
  tags, collections, editions, watched state, date added.
- **Not held at all (checked: no code names a genre or a cast):** genres, cast and crew, studio or
  network, content rating, community rating, keywords. Resolution and HDR only inside the probe's
  JSON. **Searching by genre or person needs those read from TMDB and the NFOs and kept first**: the
  larger part of the work.

## 2. Competitors

| Product | Typed search | Filters on a list |
| --- | --- | --- |
| Plex | Titles, people, genres, grouped | Genre, actor, director, decade, year, content rating, country, studio, resolution, unwatched, collection; a cast name opens their titles |
| Jellyfin | Titles and people; results capped at 24 (a top complaint) | Genre (OR only), year, rating, played; a Genres page |
| Emby | As Jellyfin, capped at 30 | Genre, network, actor |
| Infuse | Titles | By genre, alphabetical only; actor search asked for since 2017 |
| Kodi | Titles | Smart playlists: genre, year, rating, actor, director, studio, MPAA, watched, combined |
| Netflix | Results at each keystroke; titles, people, genres; AI mood search in beta (2025) | Genre rows, no filter panel |
| Apple TV | Categorised; Siri by genre, actor, director | Genre shelves |

Disney+, Prime Video and Max were not researched. On every product, typed search finds things and
filters narrow a list: two places.

## 3. What people use and want

- Netflix's study (arXiv 1903.04638, the only numbers found): about 75% of searches are for a known
  title; the rest look for a person or explore a broad term ("horror movies"); at least 13% are for
  titles not in the catalogue. Most viewing is on TVs, where the median query is **3 characters**,
  and results at each keystroke raised success and cut the time to find.
- Requests across Plex, Jellyfin, Infuse and Emby, most asked first: **people by name** (actor,
  director), **genre** (Jellyfin's asks AND/OR/NOT), then year, studio, rating; moods last. Vote
  counts not obtained.
- Baymard (shops, not media): autocomplete helps by steering to better queries; bad suggestions
  mislead.

## 4. Recommendation

1. **Keep genres and people** from TMDB and the NFOs (genres; cast and directors, with their order),
   and index them. Without this, nothing below but year and the held fields is possible.
2. **One search, grouped results**: Titles, then People, then Genres, at each keystroke from the
   first letter. A person or genre opens the list filtered by it.
3. **Filters on the list, not in the search**: Genre, Decade, Unwatched as chips; then language,
   collections and tags, which are held already.
4. **People from a film's page**: its cast and director tappable, their titles one press away.
5. **Recent searches.**
6. **Not now:** moods, our own voice search (a TV's own keyboard has one), AI search. No result cap.

```
Phone                             TV
+-----------------------------+    Search: h e_
| (search) heli_          [x] |    A B C D E F G H I J K L M ...
+-----------------------------+    ------------------------------
| Titles                      |    [Action] [Comedy] [Drama] ...
|  Balloon Heist (2019)    |    [1980s] [1990s] [Unwatched]
|  Sparrows (2013)                |    ------------------------------
| People                      |    Titles: [poster][poster][poster]
|  Winifred Ash               |    People: [face][face]
| Recent: noir  Nolan  1980s  |
+-----------------------------+
```

On a TV the genre and decade chips are reachable with no typing, two presses from arrival.

## Sources

arxiv.org 1903.04638 (Netflix); help.netflix.com node 47765; eWeek and MacRumors 2025-04-18 (Netflix
AI search); forums.plex.tv 315111, 745733, 692779; kometa.wiki Plex search builder;
forum.jellyfin.org (genre filter; search scope); emby.media community 98049, 125686;
community.firecore.com 14587; support.apple.com atvb2f98a04a; kodi.wiki smart playlists; Baymard
filtering benchmark and autocomplete examples; ui-competitors.md and ui-sentiment.md (search 23% of
Jellyfin's complaint votes, 4.2% of Plex's).
