> **ABOUTME:** What a music library would need from this project's data model — how the
> commercial platforms, MusicBrainz and the self-hosted servers model music, what their users
> complain about, and our schema measured at music scale. Establishes what adding music costs and
> what is cheap to change now.

# Music: what it would take

- **Date:** 2026-09-23. Requested by the project owner: *"we haven't looked into the audio stuff
  yet… I'm fine with it requiring a migration later, but I'd like to have a good idea of what we'd
  be in for (and whether there are cheap changes we can make now)."*
- **Decision:** [D186](../../decisions/working-notes.md). Music itself remains **undecided**
  (F92); this is its schema cost, not a decision to build it.
- **Working notes** (quotes, file paths, Navidrome's dumped schema, every fetched page):
  `/yoloai/cache/research/music-{models,sentiment}.md`, `/yoloai/cache/http/music*/`; benchmarks in
  `/yoloai/cache/bench/{music,hash}/`. Not in the repo.

## 1. How music is modelled

**The unit everyone plays is a track on an album, not a recording.** Apple, Spotify, Plex,
Navidrome, gonic, Jellyfin and beets all make the album track the row. Only **MusicBrainz**
separates a *track* (a position on a medium of a release, with a text `number`) from a *recording*
(the performance, which carries the ISRC), and above both an **artist credit** — an ordered list
of artists with **join phrases** (" feat. ", " & ") and a credited-as name. Spotify is candid about
the consequence of the simpler model: *"duplicate tracks… are rated independently"*.

| Concept | MusicBrainz | Navidrome (Go, SQLite) | Jellyfin | Mapping here |
| --- | --- | --- | --- | --- |
| song | track → recording | `media_file` row | `Audio` item | a **Title**, its encodings **Versions**, its bytes **Files** |
| album | release (of a release group) | `album`, **built by aggregating files** under a configurable key | `MusicAlbum` | a **Collection**; its tracklist an **ordering** with (disc, track) |
| disc | medium | `disc_number` column | index | the ordering's group number, with optional text |
| artist | artist, with type (person, group) | `artist`, id a hash of the name | `MusicArtist`; *"artist names are strings"* | a **person item** — D95's planned table |
| credits | artist credit, join phrases | `participants`, by role | string lists | a credit table keyed on **item** |
| genre, mood | tags (folksonomy) | **join tables since Aug 2026, after JSON** | string lists | observed join tables beside the authored `tag` |
| ReplayGain | — | nullable track and album columns | columns | stream columns and an album column (additive) |
| lyrics | — | tag or `.lrc` sidecar | sidecar | `attached` sidecars; fetched lyrics like fetched subtitles |
| work / movement | work entity | tags read at query time | — | a `work` table, later |

**Navidrome's return from JSON to join tables** for genres, in August 2026, is independent
confirmation of [D164](../../decisions/working-notes.md).

**Two traps worth knowing now.** "Compilation" means *Various Artists* in iTunes, Picard and
OpenSubsonic, and *a best-of* in MusicBrainz. And Picard's `MUSICBRAINZ_TRACKID` tag holds the
**recording** id.

## 2. What users complain about

Vote scales differ tenfold between communities, so compare within one.

| Rank | What goes wrong | Evidence |
| --- | --- | --- |
| 1 | **Artists are strings, not entities** — multi-artist and featured tracks, split on the wrong separator | Plex *albums and tracks with multiple artists*, **519 votes**, open since 2015; Navidrome #238 (+51) took **five years and a rewrite** (0.55, 2025); Jellyfin 2025 proposal: *"Artists don't really exist in the database… Artist names are strings"*. Discogs has **200,000+** artist names containing `&`, only 12 containing `;` |
| 2 | **What defines an album** — split multi-disc sets, merged editions | Jellyfin #5605 (71 comments, open since 2021); Emby: one album became *"28 different albums"*; Navidrome's key change: *"Artist favourites and artist ratings will be lost after the upgrade"*; *"when the MusicBrainz album tag is missing, everything falls apart"* |
| 3 | **Per-user state lost** on rescans, retags, format upgrades | Jellyfin #14981 (+25): favourites overwritten by a progress write; Navidrome: *replace a missing file keeping its stats* (16 + 11) |
| 4 | **Multi-valued tags** — genre only at album level | Plex, 128 votes |
| 5 | **Classical**: composer, work, movement, performers | Plex *Tag support for ROBUST music library organization*, **866 votes**, 10+ years; Navidrome #275: *"no other streaming server does classical reasonably well"* |
| 6 | **Remote providers overriding tags** | Jellyfin #9406 (+204, its top music issue): Ogg Vorbis files showed photos of the keyboardist **Jacques Ogg** |
| 7 | playlists, gapless, lyrics | Plex *Better Playlists* 1,425; Jellyfin gapless 646 (a player problem); Plex lyrics 260 |
| 8 | scale | Navidrome search **5–20 s at 130k tracks** until an FTS index; scans at 10–16 GB RAM |

**What users praise** — Lyrion/LMS: an artist table, track–artist and album–artist joins typed by
role, and *Works* identified by composer plus title. The pattern across the praised servers:
**tags first, MusicBrainz ids when present, a deterministic fallback key, and the user chooses
the rule** — which is this project's layer stack ([D183](../../decisions/working-notes.md)) and
§15 ([D184](../../decisions/working-notes.md)) already.

## 3. Scale

Navidrome's opt-out telemetry, 154,673 installs: **58% have 5,000 tracks or fewer, 84% 20,000 or
fewer, 97.7% 100,000 or fewer**; 190 installs exceed 500,000. Reported extremes: 323k songs
(Navidrome), 3M tracks (gonic).

Our schema with the [D186](../../decisions/working-notes.md) changes, one Title per track, laptop
CPU, median of 100:

| | 50,000 tracks | 500,000 tracks |
| --- | --- | --- |
| database | 46 MB | **480 MB** |
| insert rate (database side of a scan) | 12,600 tracks/s | 18,000 tracks/s |
| FTS rebuild | 65 ms | 584 ms |
| album grid, 50 per page | 0.71 ms | **0.35 ms** |
| album track list | 0.31 ms | 0.16 ms |
| artist's albums | 0.04 ms | 0.04 ms |
| search, top 50 | 0.73 ms | 7.1 ms *(generator makes every word common — a worst case)* |
| `VACUUM INTO` backup | 0.2 s | 1.4–3.4 s |

Two measured corrections are already in the schema. **At 500,000 tracks the album grid took
9.4 ms** because one sort key served songs, artists and albums together and a page walked all of
them; putting the **listing** first in the key made it 0.35 ms. **The track list took 3.9 ms** for
lack of an index on the ordering's collection — which the video sketch already had.

What 500,000 tracks changes elsewhere: **a whole-database backup is ~460 MB**, so retention count
matters ([D185](../../decisions/working-notes.md)), and a 12-step rebuild of a 500,000-row table
approaches the migration budget of 10 s ([D181](../../decisions/working-notes.md)).

## 4. Retagging is routine, and identity must survive it

Music libraries are retagged in bulk — Picard, beets — far more often than films. Measured with
mutagen on FLAC and MP3, the same method as [D179](../../decisions/working-notes.md):

| Edit | FLAC | MP3 |
| --- | --- | --- |
| set a title | head changes, size same | head **and size** change |
| Picard-style set (artist, album, MBIDs, multi-valued artists and genres) | head changes | head and size change |
| embed a 400 KB cover | head and size change | head and size change |
| **payload-anchored hash** (FLAC after the metadata blocks; MP3 between ID3v2 and ID3v1) | **same, 3 of 3** | **same, 3 of 3** |

A whole-file hash would have sent a library's play history to tombstones on its first Picard run.
The payload anchor holds; **Ogg is the untested case** — a comment block that outgrows its pages
renumbers every page after it, so Ogg's anchor must hash packet data rather than page bytes.

## 5. What this means

The schema survives music with **one reshape avoided now and the rest additive**:

- **Avoided now** — satellite tables keyed on `title` would have left albums, artists, and already
  *series and box sets*, without artwork, sort, search, tags, overrides or stars. They now key on a
  shared `item`.
- **Additive later** — person and credit tables, an album side table (release type, compilation
  flag, label, catalogue number, barcode, dates with precision, album gain), observed genre and mood
  joins, `work`, playlist entries (a playlist may repeat a song, which `collection_member` forbids),
  ReplayGain columns, lyrics, `recording` and `album` added to two `CHECK` lists.
- **Deliberately not** — CUE sheets (one file, many songs by time range); no surveyed server ships
  them.
