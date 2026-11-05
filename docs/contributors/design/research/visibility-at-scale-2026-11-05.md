# Visibility at scale (2026-11-05)

> D426 asked for visibility to be measured again at 500k rows, so the owner can decide whether a
> library's visibility stays derived (the step 0077 view `library_place`, and `version_library` on
> top of it) or is stored per item and library. This document reports the numbers and decides
> nothing. It also times a D429 prefix repoint over half the places, lists every column that stores
> a path, and checks R6's claim that refscale's statistics are stale. Measured at schema v0080, on
> main at "docs(decisions): D430 also keeps this batch's steps appended, no fold".

## 1. How it was measured

The harness was a throwaway `integration` test in `package store`. It is not committed. A test
behind its own flag would need a skip, and `check-test-weakening` flags skips. It is kept at
`/yoloai/cache/vis-scale/visibility_scale_test.go` and was copied in only to run:
`go test -tags 'integration visscale' -run TestVisibilityScale ./internal/store`. It opens a
migrated database the way `openMigrated` does. The plans therefore use the committed planner
statistics (`statistics/persistent.tsv`), as production does, not statistics gathered at 500k.

**The data.** One node and 496,000 items:

- **Films:** 70,000. Every 33rd film is under the Kids folder, and every 20th has a second copy
  under `/media/uhd`.
- **Series:** 6,000 series of 70 episodes each, so 420,000 episodes. One series in ten is under
  the Anime folder. The first 4,000 are under the Drama folder, which holds 252,000 places.

That gives 493,500 places, 490,000 files, filings and versions, 420,000 positions and 2,000 play
positions for one person. The database is 696 MB.

**The seven libraries** have overlapping roots:

| Library | Kind | Root |
| --- | --- | --- |
| Films | movie | `/media/films` |
| Kids | movie | `/media/films/kids` |
| UHD | movie | `/media/uhd` |
| Archive | movie | `/media` |
| Series | series | `/media/shows` |
| Anime | series | `/media/shows/anime` |
| Drama | series | `/media/shows/drama` |

Names are three-word phrases drawn from refscale's 35 synthetic words (D187). Each query is
prepared on the reader pool and run 3 times to warm up, then 41 times measured. The tables give
the median and p95 of the 41. The machine is an Intel i3-1315U (8 threads) with the database on
local disk, using modernc.org/sqlite.

**"Restricted" runs** pass a caller who reaches only one small library:

- the film lists use Kids, which holds 3% of the films;
- the series lists use Anime, which holds 10% of the series.

**"Stored"** is a throwaway table holding one row for each library that shows an item:

```sql
CREATE TABLE vis (item_id INTEGER, library_id INTEGER, PRIMARY KEY (item_id, library_id)) WITHOUT ROWID;
CREATE INDEX vis_library ON vis (library_id, item_id);
```

It was filled from the view, with a row for each film, episode and series, for every library that
shows it. Each query's coverage `EXISTS` was swapped for `EXISTS (SELECT 1 FROM vis WHERE
vis.item_id = <item> AND <reach>)`. Search's `searchListedCondition` was swapped the same way.
Nothing else in the queries changed.

## 2. Reads: derived against stored

| Query | Rows | Derived median | Derived p95 | Stored median | Stored p95 |
| --- | ---: | ---: | ---: | ---: | ---: |
| ListFilmsByName, every library | 60 | 0.70 ms | 0.77 | 0.33 | 0.35 |
| ListFilmsByName, Kids only | 60 | 22.6 ms | 23.2 | 4.2 | 4.4 |
| ListFilmsByAdded, every library | 60 | 0.51 ms | 0.96 | 0.20 | 0.23 |
| ListFilmsByAdded, Kids only | 60 | 9.6 ms | 10.0 | 2.1 | 2.3 |
| ListSeriesByName, every library | 60 | 0.88 ms | 1.03 | 0.30 | 0.49 |
| ListSeriesByName, Anime only | 60 | 52.4 ms | 53.7 | 0.70 | 0.73 |
| ListSeriesByAdded, every library | 60 | 0.72 ms | 0.78 | 0.22 | 0.23 |
| ListSeriesByAdded, Anime only | 60 | 104.4 ms | 106.8 | 0.75 | 0.78 |
| ListRecentPlayPositions (Continue Watching), every library | 20 | 30.4 ms | 32.4 | n/a | |
| ListRecentPlayPositions, Kids only | 8 | 23.7 ms | 24.4 | n/a | |
| IsItemReached, a film | 1 | 0.14 ms | 0.21 | 0.03 | 0.03 |
| IsItemReached, a series | 1 | 0.15 ms | 0.18 | 0.03 | 0.04 |
| ListLibrariesOfItem, a film | 4 | 0.17 ms | 0.20 | 0.03 | 0.12 |
| ListLibrariesOfItem, a series | 2 | 0.77 ms | 0.82 | 0.01 | 0.01 |
| ListWorkFiles, a film with a copy | 2 | 0.09 ms | 0.11 | n/a | |
| ListSeriesEpisodeDetails, one series | 70 | 0.54 ms | 0.56 | 0.19 | 0.21 |
| CountOrderingMembers, 60 series | 60 | 21.9 ms | 22.9 | n/a | |
| SearchWords "harbour", every library | 50 | 846 ms | 856 | 272 | 280 |
| SearchWords "harbour", Kids only | 50 | 1107 ms | 1121 | 148 | 153 |
| SearchWords "harbour quiet orchard", every library | 50 | 293 ms | 297 | 132 | 135 |
| SearchWords "harbour quiet orchard", Kids only | 50 | 329 ms | 340 | 99 | 100 |
| MatchWords "harbour" (a list's search), every library | 1000 | 216 ms | 225 | 141 | 145 |
| MatchWords "harbour", Kids only | 575 | 511 ms | 517 | 123 | 127 |
| SearchTrigram "harb", every library | 50 | 303 ms | 308 | 89 | 92 |
| SearchTrigram "harb", Kids only | 50 | 438 ms | 441 | 87 | 92 |
| SearchSubstring "harb", every library | 50 | 334 ms | 342 | 335 | 345 |
| SearchSubstring "harb", Kids only | 50 | 463 ms | 471 | 331 | 336 |

"n/a" marks queries that ask per file, not per item:

- **ListRecentPlayPositions** asks whether this file is shown, and by which place.
- **ListWorkFiles** asks the same per file.
- **CountOrderingMembers** counts the members that have a reached file.

An item-level table does not answer those, so they would stay on `version_library` either way.
They are timed here as the derived cost that remains.

**What the numbers say:**

- **Lists when the caller reaches everything** are under 1 ms either way. Storing saves about
  0.4 ms.
- **Lists when the caller reaches one small library** are where derivation costs most.
  - Films: 10–23 ms, against 2–4 ms stored.
  - Series: 52–104 ms, against under 1 ms stored.
  - Why the series cost so much: the list walks the series in order, and for each one the
    coverage check walks up to its 70 episodes, each through version, filing, place and root.
    With 9 series in 10 outside the caller's reach, most of those walks find nothing.
  - In the stored version, each candidate costs one primary-key probe.
- **Search is the slowest read in both versions.**
  - `ORDER BY bm25` and `ORDER BY name` must check every match before the `LIMIT`. "harbour"
    matches a large share of the 496k names, so the coverage check runs for each match. That
    includes every matching episode, which is why SearchWords is slower than MatchWords:
    MatchWords drops episodes before the check.
  - Storing cuts search by a factor of 1.5 to 7.5. It is still 90–280 ms, because ranking every
    match costs that much on its own.
  - Substring search scans `item_text` in both versions, so storing does not help it.
- **One-item checks** (IsItemReached, ListLibrariesOfItem, episode details) are under 1 ms either
  way.

## 3. Which queries scan

None of the derived plans scans `place`, `file`, `file_filing` or `version`. Every step through
those tables is an index search.

- **`SCAN library_root`** (7 rows) appears where the series check reaches `library_root` before
  it knows the node: in the series lists, IsItemReached and search. The film checks search
  `library_root_node`.
- **`SCAN library`** (7 rows) appears in ListLibrariesOfItem and ListWorkFiles.
- **`SCAN item_text`** appears in substring search, in both versions. It is the only full scan of
  a large table.

The derived cost comes from how many index probes each candidate needs, not from scans. In the
restricted series lists that is up to 70 episodes × 4 probes for each series turned down.

Two representative plans follow. The rest are in the harness's output
(`/yoloai/cache/vis-scale/out.md`).

ListSeriesByAdded, derived:

```
SEARCH item USING INDEX item_kind_found (kind=? AND (found_at,found_file_modified_at)<(?,?))
CORRELATED SCALAR SUBQUERY 3
  SEARCH ordering USING COVERING INDEX sqlite_autoindex_ordering_1 (sequence_id=? AND name=?)
  SEARCH position USING COVERING INDEX sqlite_autoindex_position_1 (ordering_id=?)
  SEARCH version USING INDEX version_work (work_id=?)
  SEARCH file_filing USING INDEX file_filing_version (version_id=?)
  SEARCH place USING INDEX place_file (file_id=?)
  SCAN library_root
  BLOOM FILTER ON library (id=?)
  SEARCH library USING INTEGER PRIMARY KEY (rowid=?)
SEARCH item_text USING INDEX item_text_original (item_id=?) LEFT-JOIN
```

The same, stored:

```
SEARCH item USING INDEX item_kind_found (kind=? AND (found_at,found_file_modified_at)<(?,?))
SEARCH item_text USING INDEX item_text_original (item_id=?) LEFT-JOIN
SEARCH vis EXISTS USING PRIMARY KEY (item_id=?)
```

## 4. What a stored table costs

**Size.** The table has 869,821 rows, about 1.75 per item. By kind:

| Kind | Rows |
| --- | ---: |
| Films | 145,621 |
| Episodes | 714,000 |
| Series | 10,200 |

On disk, from `dbstat`:

| Part | Size |
| --- | ---: |
| The table | 9.8 MB |
| The `(library_id, item_id)` index | 8.6 MB |
| Total | 18.4 MB, 2.6% of the 696 MB database |

Without the second index the total is 9.8 MB. Every query measured here probes by item, so none
used that index. It would serve "list library X" written to drive from the table.

**Building it** from the view takes 6.8 s for all 869,821 rows, without the second index. That is
the cost of a migration step, or of rebuilding after a bug.

**Upkeep.**

- **A scan of 1,000 new files.** The files are 500 films (half of them under Kids) and 500
  episodes of 10 new series under Drama.
  - The scan's own rows (items, works, texts, files, places, versions and filings) take 52 ms.
  - The stored table adds **2,270 rows** for those files:
    - each film in Films and Archive, and the Kids ones in Kids too;
    - each episode in Series and Drama;
    - the 10 series in both.
  - Those rows took **209 ms**. The harness recomputed them from the view for the new versions,
    so this is a ceiling on the writes: four times the scan's own.
- **Adding a library over an existing folder.** A root on the Anime folder means 42,600 rows
  (42,000 episodes and 600 series), written in **1.7 s**.
- **Removing that library** deletes 42,600 rows in 20 ms.
- **What else must keep it up to date.** Every write that changes coverage would have to keep the
  table in step, whether by trigger or in the code that writes:
  - a place found, removed or moved;
  - a filing that changes its version;
  - a version removed;
  - a library root added, retired or moved;
  - a library removed;
  - an episode moved into or out of a series' aired order.

  `item_found` (step 0058's triggers) already does the same for `found_at`, and it is the model
  for doing it by trigger.

## 5. A D429 prefix repoint

The test moved `/media/shows/drama` to `/media/shows/big/drama` on node 1, which covers 252,000 of
the 493,500 places. All of it ran in one writer transaction:

| Statement | Rows | Time | Plan |
| --- | ---: | ---: | --- |
| `place.path`, `reported_path_raw` | 252,000 | 1.12 s | SCAN place |
| `library_root.root_path` | 1 | 0 | SCAN library_root |
| `file_filing.identified_from_path` | 252,000 | 0.59 s | SCAN file_filing, with a place search for each |
| `unavailable_place.path`, `root_path` | 2 | 0 | index `unavailable_place_root` |
| `root_state`, `root_scan`, `root_mount` | 3 | 0 | primary keys |
| **All statements** | | **1.70 s** | |
| The commit | | 0.82 s | |

The writer is held for about **2.5 s**. If the same transaction also rebuilt the stored table for
every item it touched, that would be 511,200 rows deleted and written again, for another
**4.4 s**. A repoint that moves a library's roots together with its places changes no
visibility, though. Code that knew that could skip the rebuild. Code that recomputed the items
under the folder would pay the 4.4 s.

**Notes from the repoint:**

- **No index serves the place rewrite, which scans.** The statement matched `reported_path_raw` by
  range on the node, as `library_place` does. It did not limit itself to live places, because
  removed places' paths have to move too. `place_live_path` is a partial index
  (`WHERE removed_at IS NULL`), so it cannot serve that, and the statement scans `place`.
  Splitting the rewrite into live and removed places would let the live half use the index.
- **`file_filing.identified_from_path` has no node.** To rewrite only "folder X on node A", the
  statement has to join through `place` to find each filing's node. Without the join, the same
  path on another node would be rewritten too.
- **The raw bytes and the NFC text need separate prefixes.** A raw-byte path and its NFC form can
  differ when a folder name is NFD on disk. The statement takes the raw prefix's length from the
  bytes and the text prefix's length from the NFC text.
- **Joining BLOBs in SQL gives TEXT.** `CAST(:y AS BLOB) || substr(reported_path_raw, …)` returns
  TEXT, and the STRICT `place` refuses it: "cannot store TEXT value in BLOB column" (3091). The
  result must be cast back to BLOB. For bytes that are not valid UTF-8, the new raw paths are
  safer built in Go.

**Constraints that trip partway.** No CHECK constraint and no trigger involves a path column.
`place_found_au` fires only on `removed_at` and `modified_at`. The UNIQUE indexes are another
matter:

- **The drama repoint tripped nothing.** Its target held nothing.
- **Moving a folder into its own subfolder fails partway.** The test moved `/nest` to `/nest/sub`
  where both `/nest/a.mkv` and `/nest/sub/a.mkv` exist. It failed with `UNIQUE constraint failed:
  place.node_id, place.reported_path_raw`, though the end state (`/nest/sub/a.mkv`,
  `/nest/sub/sub/a.mkv`) is unique. SQLite checks UNIQUE row by row as an UPDATE runs, not at the
  statement's end.
  - **Workaround:** move to a temporary prefix first, then to the target, both in the same
    transaction.
- **A target that already holds places at the same paths is a real conflict.** It fails on:
  - `place_live_path`;
  - the primary keys of `library_root`, `unavailable_place`, `root_state`, `root_scan` and
    `root_mount`;
  - cache.db's `scan` UNIQUE on `(node_id, root_path)`.

  The operation would have to refuse such a target, or merge it.

## 6. Every column that stores a path (v0080)

### Plain path columns

These columns hold a plain path, so a prefix rewrite reaches them:

| Column | Notes |
| --- | --- |
| `place.path` | NFC text. In `place_errored`. |
| `place.reported_path_raw` | The raw bytes, the place's key. In `place_live_path` (UNIQUE). Rewrite it by byte prefix. |
| `library_root.root_path` | In the primary key. |
| `file_filing.identified_from_path` | No node: join through `place` (§5). |
| `unavailable_place.path`, `root_path` | `path` is in the primary key. |
| `root_mount.root_path`, `mount_path` | `mount_path` is in the primary key. |
| `root_scan.root_path` | In the primary key. |
| `root_state.root_path` | In the primary key. |
| cache.db `scan.root_path` | UNIQUE with the node. |

### Relative paths and file names

These are relative to a root, or hold only the last part of a path. They change only when a move
reaches inside a root, not when a root moves:

| Column | What it holds |
| --- | --- |
| cache.db `scan.scope_path`, `again_scope_path` | A path relative to the root. |
| cache.db `scan.read_again_json`, `again_read_again_json` | JSON lists of paths relative to the root. |
| `other_video.file_name`, `file_name_key` | Only the last part of the path. |

### Paths inside an encoded value

A plain prefix rewrite cannot reach these:

| Column | What is inside |
| --- | --- |
| cache.db `job.destination` | Strings like `root:3:/media/films`, with the node and the root together in one. |
| cache.db `scan_batch.files_json` | The files as the examine command names them: a JSON list. |
| `place.error_detail_json` | The platform's error text, which can quote the path. |
| `place.filing_reason`, `file_filing.filing_reason` | Free text, "the rule verbatim, what was tried", which can name a path. |
| `person.picture_ref` | An NFO's thumb when the source is an NFO, which can be a local path. |
| `file_nfo.body` | The NFO's own bytes, which can name paths. Not ours to rewrite. |

cache.db is class C and can be dropped instead of rewritten. The rest are diagnostic or provider
text that no lookup keys on.

### Checked, and holding no path

- `file_probe.probe_json` is the parsed `nodeproto.Probe`, and has no field for the file's path.
  Its attachment `filename` is a name inside the file.
- `artwork.source_ref` names a local image as `file:<id>:<size>:<mtime>`, an id, not a path.
- Loudness, video timelines and subtitle text are keyed by the payload's hash.
- `setting.value_json` holds no path in v0080's settings.

## 7. R6's claim about the refscale statistics

R6 said refscale's statistics output is stale, that it writes `title_id` and `collection` names
from before step 0072. The file is stale, but not for that reason.

- **The names are current.** The committed `statistics/persistent.tsv` names only v0080's tables
  (`work`, `sequence`, `version`, `place`, `file`, `file_filing`, `position`, `ordering` and the
  rest). It has no `title` or `collection` row.
- **What R6 saw is the filler.** `refscale_test.go`'s filler writes `title`, `collection`,
  `title_id` and `collection_member` by design. It fills at the baseline schema (step 0048) and
  then runs every step, so that the steps are timed at scale. `ANALYZE` runs after the last step.
- **What is stale is two lines.** `go test -tags integration -run TestReferenceScale
  ./internal/store` fails at "docs(decisions): D430 also keeps this batch's steps appended, no fold". It finds the committed file is not what `ANALYZE` of the
  reference library writes now. The difference is two lines:
  - the fresh output has a row for `grp`'s key, `grp sqlite_autoindex_grp_1 1000 5 1`, which the
    committed file lacks (the table arrived with "refactor(store): make a group a row", "make a group a row");
  - `schema_step`'s count is 34 in the fresh output, against the committed 32.

  No other row differs.
- **What a fix would take:** one run of the existing
  `go test -tags integration -run TestReferenceScale ./internal/store -update-statistics`,
  then committing the rewritten `statistics/persistent.tsv`. No code needs to change. The filler
  needs a rewrite only if the baseline moves past step 0072. `statistics/` may be a baseline under
  the gate, so the run and the commit are left to the owner. Until then, check-pr's integration
  tier fails on this test.

## 8. The two options

**Keep it derived** (the view, as now).

- **Writes:** nothing more than now. A D429 repoint rewrites paths only: about 2.5 s for 252k
  places, with the writer held.
- **Reads:**
  - Browse lists are under 1 ms for a caller who reaches every library.
  - For a caller who reaches one small library, film lists take 10–23 ms and series lists
    52–104 ms.
  - Word and trigram search take 0.2–1.1 s on a common word.
  - Continue Watching takes 24–30 ms with 2,000 positions.
- **What it rests on:** the restricted series lists and search grow with the number of episodes
  outside the caller's reach.

**Store it** (an `(item_id, library_id)` table).

- **Space:** 9.8 MB for 870k rows, or 18.4 MB with a by-library index.
- **Building it:** 6.8 s once.
- **Writes:**
  - A 1,000-file scan writes about 2,300 more rows, at up to 209 ms, against the scan's own
    52 ms.
  - A library added over 42k episodes writes 42,600 rows in 1.7 s. Removing it takes 20 ms.
  - A D429 repoint costs nothing more if the code knows visibility did not change, and up to
    4.4 s more if it recomputes.
  - Every coverage-changing write (§4) must keep the table in step.
- **Reads:**
  - Browse lists take under 1 ms, and 2–4 ms for restricted film lists.
  - Search takes 90–280 ms for words and trigrams, and substring search is unchanged at
    330–460 ms.
  - Continue Watching, ListWorkFiles and CountOrderingMembers ask per file. They stay on
    `version_library`, as derived.
