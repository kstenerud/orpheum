> **ABOUTME:** An early design for browsing one library at a time, and for a third kind of
> library, home movies and other videos, which fits under neither films nor series: what a
> library is to a viewer, the API and schema it needs, where it sits in the UI, and the owner's
> open questions.

# Library browsing

- **Status:** BUILT, specified by the owner's answers of 2026-10-26
  ([D349](../../decisions/working-notes.md)), which §9 sets out and which replace §5 where they
  differ. Written 2026-10-25 at the owner's request: *"I want to have the ability to browse
  individual libraries (so for example if I have two movie libraries, and want to look in only
  one). This will be important when we add support for other kinds of videos, like home movies,
  which won't fit under series or films. Come up with an early design for this and where it might
  fit into the UI."* The schema, the API (§9.2), the identification pass, the Libraries page itself
  (§9.1: the top bar, the chooser, the grid, the alphabet rail, folders, the TV-remote handling),
  the same alphabet-rail filtering now on Films and Series too, and the person's own setting of
  which libraries Films and Series gather (D349 §8) are built (2026-10-26, §7); a frame as an
  other video's own picture is not (§9.3).
- **Depends on:** [library-model.md](library-model.md) (titles, collections, kinds),
  [web-client.md](web-client.md) (curated home rows, folder browsing),
  [closed-beta.md](closed-beta.md) (B2's "Browse filters and more sorts").
- **Governing decisions:** [D243](../../decisions/working-notes.md) §4 (a role lists the
  libraries its members may browse and play), [D160](../../decisions/working-notes.md) (there is
  no series concept: a sequence is what a Collection produces),
  [D163](../../decisions/working-notes.md) (the library browser with the filter off),
  [D184](../../decisions/working-notes.md) (a library's source is asked at creation, never
  defaulted), [D215](../../decisions/working-notes.md) (images beside the media),
  [D138](../../decisions/working-notes.md) (click depth and focus latency; the row order is the
  person's), [D149](../../decisions/working-notes.md) §5 and §7 (the row set is the owner's;
  folder browsing is missing).

## 1. What exists

| Piece | Today |
| --- | --- |
| `library` table | `name`, `kind IN ('movies','shows','mixed')`, `metadata_source IN ('nfo_then_tmdb','tmdb','nfo')`; every item carries its `library_id` |
| `title.kind` | `movie`, `episode`, `extra`; a series is a `collection` of kind `series` |
| `GET /titles` | `kind=movie\|series`, `sort=name\|added`, cursor, limit. No library parameter |
| Library scoping | already in every list query: `is_every_library` / `library_ids_json` from the caller's `auth.Reach` (D243 §4) |
| `GET /libraries` | any signed-in person, filtered by their reach; an admin also sees roots and filing counts |
| Web | top bar: Films, Series, Search, Watch Together; the menu holds Libraries (admin). Films and Series are one `Browse` page each, with a sort choice and the alphabet rail (a prototype). Home: Continue Watching, Newest films, Newest series |

So the person already browses "films from every library I reach". Narrowing to one library is the
same query with a smaller reach. The gap is not in the data; it is that **a library has no
presence a viewer can see**.

## 2. What a library is to a viewer

Two models compete, and the incumbents split on them:

- **A library is a place.** Plex, Jellyfin and Emby put each library at the top level: a sidebar
  entry or a "My Media" tile per library, a page per library with its own tabs, and home rows
  per library ("Recently added in Kids' Films"). Two film libraries are two places; there is no
  page that holds every film.
- **A kind is a place, and the library is where the files live.** Kodi merges every source into
  one Films and one TV Shows view; Infuse does the same, with Movies, TV Shows and **Other**.
  Neither offers "only this source" without hand-built nodes or favourites.

Orpheum today is the second, and that is the right default for the common household (one film
library, one series library): the kind is what a person thinks in. The owner's ask adds a third
need, and it is narrower than the first model:

> **Kinds stay the places; a library is a filter on a kind's list. A library whose videos have no
> kind — home movies — is a place of its own, because there is no kind to put it under.**

That gives one Films page and one Series page however many libraries there are, a chooser on each
that appears only when the person reaches two or more libraries holding that kind, and a top-bar
entry for each other-videos library under the name the admin gave it.

## 3. Prior art, briefly

| Product | Per-library browsing | Home and other videos |
| --- | --- | --- |
| **Plex** | every library in the sidebar, pinnable; home hubs per library | library type **Other Videos** with the Personal Media agent: no online match; defaults to chronology, with a **Folder** view ([Shorouk's guide](https://blog.shorouk.dev/2020/07/organize-your-home-movies-with-plex-to-make-your-memories-shine/), [Plex forum](https://forums.plex.tv/t/in-other-videos-is-there-a-way-to-add-related-videos-to-a-folder-so-that-the-folder-shows-on-main/818442)) |
| **Jellyfin** | "My Media" tiles; each library its own page and its own "Latest" row | collection type `homevideos` ("Home Videos and Photos"), whose page opens on **Folders** (jellyfin-web `libraryRoutes.ts`, cached in `/yoloai/cache/git/jellyfin-web`) |
| **Emby** | as Jellyfin | "Home videos & photos", folder-first |
| **Infuse** | none: Movies, TV Shows, Other, merged across sources | **Other** is what did not match; per folder, "Use Local Metadata" stops online matching ([Firecore forum](https://community.firecore.com/t/how-to-manage-home-videos-concerts-with-infuse-xml-file-etc/15402)) |
| **Kodi** | none by default; custom video nodes per source | the Files view of a source, with thumbnails and media info only ([Kodi wiki](https://kodi.wiki/view/Video_nodes), [Kodi forum](https://forum.kodi.tv/showthread.php?tid=267861)) |

What they agree on for home videos: **never matched online, ordered by date, browsable by
folder**. Infuse's "Other" as a bin for failed matches is the thing to avoid: an unidentified
film is a film (D163 shows why it was not matched), not an "other video".

## 4. Data and API

### 4.1 One library's list: no schema change

- `GET /titles` takes an optional **`library`** (one id). The handler narrows the caller's reach
  to that library: `Reach.Narrowed(libraryID)`, an intersection, so a library outside the role
  lists nothing (and says so, a 404, rather than an empty page that looks like an empty library).
  The queries do not change: they already take `library_ids_json`. The rule that every list
  passes through the reach stays one rule.
- `GET /libraries` gains, for every caller, **what each library holds by kind**:
  `{ "movie": 812, "series": 40, "other": 0 }`, counted over listed titles. The client shows the
  chooser and the top-bar entries from it; a `mixed` library with films appears in the Films
  chooser and the Series chooser both. The counts are a maintained value, not a page-load count
  (D149 §5's reason, D134).
- The alphabet rail, when it works, asks for its letters with the same `library`, so the letters
  are the narrowed list's.

### 4.2 A kind for home and other videos

| Change | Shape |
| --- | --- |
| Library kind | `library.kind` gains **`other`** (§8 Q3 for the name), offered at creation as "Home movies and other videos" |
| Title kind | `title.kind` gains **`other`**: a video that is not a film, an episode or an extra. Every video in an `other` library is filed as one; nothing in a `movies` or `shows` library ever is |
| Identification | none, ever. An `other` library asks no metadata source (D184's question is not asked: there is one answer), its titles are `followed_source = 'none'`, and nothing about them leaves the server. A household's home video is its most private material ([parental-controls.md](parental-controls.md)); a file name sent to TMDB is a leak |
| Name | the file's name without its extension, the NFO's title if one is beside it, editable by an admin |
| Date | **`title.recorded_at`**, class C: the container's `creation_time` tag where the node's probe finds one, else a date in the file or folder name (`2019-07-14`, `2019-07`), else the file's modified time. Kept with where it came from, so a wrong date can be seen and corrected |
| Picture | an image beside the video (D215's names) where there is one; else a frame the node takes from the video (the scrub-thumbnail work in B1 makes the frame). A 16:9 tile, not a 2:3 poster |
| Listing | `sort_key.listing = 'other'` for name order; `title.found_at` (step 0058) already gives newest-added; a new `ListOtherByRecorded` index gives newest-recorded |
| API | `GET /titles?kind=other&sort=recorded\|added\|name&library=…` |

**Folders, not collections.** A folder of a birthday's clips is an event, and it reads like a
collection (D160: a sequence ordered by date). It is not stored as one in the first build: a
folder view is the file tree the node already reports, listed with the library browser's own
machinery (D163), which D149 §7 names as missing anyway as the escape hatch for every library.
Promoting a folder to a collection with a name and a picture is a later step (§7, step 5).

Schema changes append steps after the checkpoint (AGENTS.md rule 8; `CHECKPOINT` is 48).

## 5. Where it fits in the UI

### 5.1 The top bar

Films and Series stay. Each `other` library the person reaches gets an entry under its own name,
after Series. With more than two, they fold into one **Libraries ▾** entry, so the bar keeps its
one row from a tablet up.

```
 ┌──────────────────────────────────────────────────────────────────────────────┐
 │ [logo]  Films   Series   Home movies   Search   Watch Together          [≡] │
 └──────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 The Films and Series pages: a library chooser

Beside the sort, shown only when the person reaches two or more libraries holding that kind.
The choice is in the address (`/films?library=3`), so Back, a bookmark and a reload keep it.

```
 Films                         Library [ All libraries ▾ ]   Sort [ By name ▾ ]
 ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐                                  #
 │    │ │    │ │    │ │    │ │    │ │    │                                  A
 └────┘ └────┘ └────┘ └────┘ └────┘ └────┘                                  B
 Wing   Fièvre  ...                                                         C
                                                                             …
```

A chosen library changes the heading too ("Films · Kids"), so a person who comes back to the page
sees at once that it is narrowed. Changing the library returns the grid to its top, as changing
the sort does today, and the alphabet rail shows only the letters the narrowed list has.

### 5.3 An other-videos library's page

Newest recorded first by default, a year rail in place of the alphabet rail (the same component,
its marks years), and a Folders view beside it.

```
 Home movies                    View [ By date ▾ | Folders ]
 2024 ─────────────────────────────────────────────────────           2024
 ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐                   2023
 │  frame   │ │  frame   │ │  frame   │ │  frame   │                   2022
 └──────────┘ └──────────┘ └──────────┘ └──────────┘                   …
 Beach day    Mia's 5th    Snow 1      Snow 2
 14 Jul 2024  2 Jun 2024   12 Jan 2024 12 Jan 2024
```

### 5.4 Home

Home keeps its kind rows (Newest films, Newest series): a household with two film libraries does
not want two film rows by default. Each `other` library gets its own row, "New in Home movies",
after them. Rows per library for films and series are a later choice, the person's to turn on and
order (D138), within the set the owner allows (D149 §5).

### 5.5 A TV remote

- The chooser is a button in the page's bar, in the same focus container as the sort: Up from the
  grid's top row reaches the bar, Left and Right move between Library and Sort, OK opens a list
  of libraries as a focusable overlay (a native `<select>` on a TV's browser is unreliable; the
  sort's select will need the same treatment). Back closes it without choosing.
- One click from the bar to a library; no extra page between them (D138's click depth).
- The rail stays to the grid's right, as today; Right from the grid's last column enters it.

## 6. How it meets what exists

- **Search** searches every library the person reaches, `other` titles included, each hit saying
  its library when the person reaches more than one of that kind.
- **Continue Watching** includes other videos; play state is the same per file (D157).
- **Watch Together** plays one as it plays a film.
- **Roles** are untouched: a role lists libraries (D243 §4), and the chooser and the top bar show
  only what the reach holds. The kids' role simply never sees the Home movies entry unless given it.
- **Identify** (admin) never lists `other` titles: there is nothing to match.
- **A mixed library** contributes to Films and Series as today and appears in both choosers.

## 7. Phased steps

1. **The chooser: built** (2026-10-26). §9 replaces this step's own design (a chooser on Films and
   Series, held-kind counts on `GET /libraries`) with a single Libraries page (§9.1): the API side
   is `library` narrowing `GET /titles` and `GET /search`'s reach (a library outside it is a 404),
   `GET /titles/letters`, and `leftOutLibraryIds` on `GET`/`PUT /me/preferences`, which Films and
   Series already read through the same `library` parameter's absence; the web side is the
   Libraries page itself (`library-browse.tsx`): the top bar's entry (replacing Search,
   `app-shell.tsx`), the `/browse` route holding the library, the view, the folder, the sort and the
   query (`router.ts`), the chooser, the sort and (an other library's) the view, each a focusable
   drop-down (`RouteChooser`, reusing `modal-dialog.tsx`'s drop-down, library-browsing.md §5.5), and
   the alphabet rail filtered to the letters `GET /titles/letters` names
   (`useAlphabetRailLetters`, `alphabet-rail.tsx`), one mechanism shared with Films and Series,
   whose own rail is filtered the same way, asked for again with the list's background poll, and
   shown only sorted by name (2026-10-26, `BrowseAlphabetRail`, `browse.tsx`). The person's own
   choice of which libraries Films and Series gather (D349 §8) is a setting on their account
   (`left-out-libraries.tsx`'s `LeftOutLibrariesSetting`, offered on `account.tsx` beside
   `AutoplaySetting`): every `movies`, `shows` or `mixed` library they reach, checked until they
   leave it out; Films and Series already honoured `leftOutLibraryIds` server-side
   (`listReach`, `internal/core/api/library_browsing.go`), confirmed by
   `TestBrowse_ALibraryLeftOutOfFilmsAndSeries`, so only the setting itself was missing. Not built:
   the held-kind counts `GET /libraries` was to gain are no longer needed, since no page offers a
   chooser per kind; a home-page row for an other library.
2. **The other kind: built** (2026-10-26). The library and title kinds, the migration
   (`0059_other_videos.sql`), filing every video of such a library as `other` with no
   identification, its name and `recorded_at` (the container's tag, a date in its name, or its
   file's time, each correctable), its file's name and sort key, and the admin's library-creation
   form offering it (no metadata source asked). The frame as its picture (§9.3) is not: a library's
   picture is the image beside it (browse.tsx's `EntryPoster`, a 16:9 tile for an `other` entry), or
   none.
3. **Its page and its place: built** (2026-10-26, `library-browse.tsx`). Size S to M.
4. **Folders: built** (2026-10-26). The API (`GET /libraries/{id}/folders`,
   `ListOtherVideosOfLibrary`) and the folder view (`library-browse.tsx`'s `FolderView`: its
   subfolders, each leading into its own, its videos, and Up); folder browsing as every other
   library's escape hatch (D149 §7) is built for an `other` library, not yet for a `movies`,
   `shows` or `mixed` one, which still has none.
5. **Later**: a folder promoted to a named collection; per-library home rows as a person's
   setting; a library's own page for a mixed library; a frame taken from the video as an other
   video's picture (§9.3).

## 8. Open questions for the owner

1. **What is a library to a viewer: a place, or a filter on a kind?** This is the shape of the
   whole UI. Plex and Jellyfin make each library a place (no all-films page); Kodi and Infuse
   merge them (no one-library page). *Recommended:* kinds stay the places and a library is a
   filter on them (§2), except other-videos libraries, which are places because they have no kind.
2. **Does a chosen library stick?** A gap of memory: when a person narrows Films to "Kids" and
   comes back tomorrow, should Films open narrowed? Options: the address only (opens at All);
   remembered per device; remembered per person. *Recommended:* the address only for now; a
   remembered choice hides a narrowed list from the person who forgot they chose it.
3. **What is the third kind called, and what belongs in it?** A naming and scope gap. Home movies
   are the case; concerts, lectures, downloaded clips and screen recordings fit the same way (no
   online match, ordered by date). Options: one kind, `other`, shown as "Home movies and other
   videos", the library's own name saying what it holds; or a `home_videos` kind now and others as
   they come. *Recommended:* one kind, `other`; the name the admin gives the library carries the
   meaning, and one mechanism covers them all (AGENTS.md rule 11).
4. **How are other videos ordered by default?** Options: newest recorded first; by folder; by
   name. *Recommended:* newest recorded first, with Folders one click away, as Plex does; a date
   is what a person remembers a home video by.
5. **Where does a home video's date come from, and may an admin correct it?** *Recommended:* the
   container's recorded time, else a date in its name or folder, else the file's time, shown with
   its source; an admin may override it as any field is overridden (D170).
6. **Is a folder an event?** A gap between file layout and meaning: a folder of a day's clips
   reads as one thing. Options: folders are only a view (step 4); or every folder is a collection
   with a name and a picture. *Recommended:* only a view first; promoting one to a collection
   later, by hand.
7. **Do other videos join the shared surfaces?** Continue Watching, search, Watch Together, the
   newest rows. *Recommended:* all of them, each other-videos library as its own home row; a role
   that should not see them is given no such library.
8. **Does a library with films in it also get home rows of its own?** Two film libraries could
   each have a "Newest" row. *Recommended:* not by default; a later person's setting (D138), the
   row set the owner's (D149 §5).
9. **What happens to the top bar with many libraries?** *Recommended:* up to two other-videos
   libraries by name, more fold into one "Libraries ▾" entry.
10. **Privacy of other videos beyond identification.** Should they also be kept out of anything
    that leaves the server (bug-report attachments, discovery's listings) and out of guest and
    shared links unless chosen? *Recommended:* yes: nothing about an `other` title leaves the
    server, and the roles' library list is the only way in.

## 9. As decided (D349)

The owner's answers of 2026-10-26, quoted in D349, and the shapes they give. Where §2, §5 and §7
recommend otherwise, this section holds.

### 9.1 The page

- **The top bar** is Films, Series, **Libraries**, Watch Together: the Libraries page replaces
  Search (Q1, Q9). Films and Series gather every library holding their kind, less the libraries the
  person left out of them on their own settings (Q8).
- **The Libraries page** has one top row: the library chooser (the libraries the person reaches,
  in alphabetical order, then *All libraries*; the first library is chosen on arrival), the view
  and the sort the library offers, and a search field, which searches the library chosen or, with
  *All libraries*, every one. Under it, the grid, with the alphabet rail when the sort is by a
  name.
- **Its address holds what it shows** (`/browse?library=3&view=folders&folder=…&sort=…&query=…`),
  so the top bar's link opens it at its start and Back returns to where it was (Q2).
- **What a library offers.** A film library lists its films, a series library its series, a mixed
  one both, *All libraries* everything, each by name or newest first. An other library lists its
  videos by recorded date (newest first, the default), by name, by file name or newest added, with
  a View of *All* or *Folders* (Q5); no genre.
- **The alphabet rail** shows only while the sort is by name or by file name, follows that order,
  and offers only the letters some title in the list begins with in it, which the server names and
  the rail asks for again with the list's background poll.

### 9.2 Data and API

| Piece | Shape |
| --- | --- |
| Library kind | `library.kind` gains `other`; such a library takes NFO files only (`metadata_source = 'nfo'`), which the schema checks, so nothing of it is ever matched online (Q10) |
| Title kind | `title.kind` gains `other`: every video of an other library, one title a file, named by its file's name without its extension, or by the NFO file beside it; never an extra, an episode or another file's version; identified from its own data, never owed a match |
| `other_video` | one row an other title, class C, the pass's: `recorded_at` and `recorded_from` (`tag`, `name`, `file`, `person`), its file's name and that name's sort key in the sort locale |
| Recorded date | the probe's `creation_time` tag (the node reports it as `format.recordedAt`), else a date in the file's or a folder's name (`2019-07-14`, `20190714`, `2019-07`), else the file's modified time; an admin's correction is the override `recorded_at` (D170), `PUT /items/{id}/recorded-at` |
| `GET /titles` | `kind=movie\|series\|other\|any`, `library` (one, narrowing the caller's reach; a library they do not reach is a 404), `sort=name\|added\|recorded\|file_name` (the last two for `other`), `from` by name or file name. Each entry says its `kind`; an other video its recorded date and where it came from. With no `library`, Films and Series leave out the person's left-out libraries |
| `GET /titles/letters` | the same `kind`, `library` and `sort`: the rail's letters the list has titles under, `#` for those before A |
| `GET /libraries/{id}/folders` | an other library's folder (`folder`, `` at its roots): the folders in it, each with how many videos it holds, and its own videos in the sort asked for |
| `GET/PUT /me/preferences` | `leftOutLibraryIds`: the libraries the person leaves out of Films and Series |
| `GET /search` | takes `library`, as `/titles` does; its hits may be `other` |
| Filing report | `POST /items/{id}/filing-report` takes `includeOtherVideos`, off by default: a report on an other video then withholds its name and paths, and says so |

### 9.3 Not yet

- A frame taken from the video as an other video's picture: the node's artwork command reads only
  images today. Until then an image beside the video (D215's names) is its picture, and a tile
  with none shows its name and date.
- Genres, and a search field on Films and Series (the owner's *"eventually"*).
