> **ABOUTME:** An audit of how the core uses SQLite, made after nas.example's requests waited 3–47 s
> behind the identification pass: what holds the one writer, why, the query plans of every
> sqlc query on a library of 7,500 files, and the fixes in order of what they buy.

# Database access audit, 2026-10-15

## What nas.example showed

`v0.2.0-nas.6` logs every write that waits for persistent.db's writer, or holds it, for over a
second. In ten minutes on nas.example, two callers held it, again and again:

| Holder | Held | How often |
| --- | --- | --- |
| `identify.(*Pass).IdentifyLibrary` | 25–30 s | back to back, about every 30 s |
| `identify.(*Pass).RefreshSortAndSearchKeys` | 3–4.7 s | after every pass |

Everything else that writes waited behind them, for up to 47 s: `auth.Authenticate` (a session's
last-used time, on the request path), `SetAdminAway`, `nodes.takeStatus`, `readthrough.keep`,
`artwork.Take`, the matcher's own writes. So `/me`, `/home` and `/titles` took 3–47 s, though
they only read, because authenticating the request writes.

## The method

A throwaway test filed a synthetic library of 1,500 films and 150 series of 40 episodes (7,500
files) and timed the pass under the CPU profiler; the database was then saved and every one of
the 297 sqlc queries run through `EXPLAIN QUERY PLAN`, with the suspicious ones timed with
CPython's SQLite (C; our pure-Go driver is several times slower). The library's titles are
unmatched, so the provider-data paths of a matched library are lighter here than on nas.example.

The pass took **15 s** a run on this sandbox, all of it inside one write transaction, the
RefreshSortAndSearchKeys after it included.

## Findings, worst first

### 1. `sort_key` has no index by item: two queries are quadratic

`sort_key`'s primary key is `(locale, listing, collation_key, item_id)`, right for listing a
library by name and useless for finding one item's key. Nothing else indexes it. So:

- `DeleteItemSortKeys` (`WHERE item_id = ? AND locale = ?`) reads every key in the locale, and
  the pass runs it for every item it names: O(items²). About 5 s of the 15 s pass.
- `ListNamesWithSortAndSearchKeys` left-joins `sort_key` on `item_id`, again a scan of the
  locale per item: O(items²). All of RefreshSortAndSearchKeys' 3–4 s on nas.example.
- `CountItemsMissingSortKey`, the same shape.

On the saved database an index `sort_key(item_id, locale)` took the list from **2,893 ms to
17 ms** and one item's delete from 0.30 ms to under 0.01 ms.

The plan reads `SEARCH sort_key USING PRIMARY KEY (locale=?)`, which looks like an index use;
it is a scan of every key in the one locale there is. A check for plans that only `SCAN` misses
it.

### 2. The pass rewrites everything it reads, changed or not

Every pass, for every item: `UpsertOriginalText` (an `ON CONFLICT DO UPDATE` with no "only if
different"), then delete and insert its sort key, then `UpdateTitleResolved`. A no-op update
still writes the row, and `item_text`'s update trigger deletes and re-inserts the row in both
full-text indexes, the trigram one included: four FTS writes per item, every 30 s on nas.example.
On a library that has not changed, a pass should write nothing.

### 3. One write transaction holds the whole library

`IdentifyLibrary` loads the library, files every video and writes the results inside one
`Write`. However fast the pass becomes, its time grows with the library, and every request
waits for all of it. Reading should happen outside the writer (WAL readers see a snapshot), with
only the changes written, in short transactions. SQLite allows one writer; a design that lets
one job hold it for seconds is the defect, not the size of the library.

### 4. A matched series re-runs the whole library's pass

`recordSeriesAndRederive` (a series' match, its refresh, a season listing stored) and
`keepGoneCopy` call `IdentifyLibrary` inline, and the pass's after-hook queues the next fetches.
So one series costs one full pass per fetch: O(series × library). After an upgrade that asks
the providers again, nas.example works through its series one 30 s pass at a time, for hours. A
fetch should ask for a pass (the `identification passes` sweep exists for this) and many
fetches should share one.

### 5. A request waits for the writer to record that it was made

`Authenticate` writes the session's `last_used_at` (once per `useWriteInterval`) before the
request runs. A reading request then waits for whatever holds the writer. The time is only for
idle expiry, which is measured in days; it can be recorded after the response, coalesced, and
dropped if the writer is busy. Five requests that arrive together also each decide to write it.

### 6. A revived file's lookup scans every file, once per file

`GetLatestTombstoneByPayloadHash` (`WHERE library_id = ? AND payload_hash = ? AND removed_at IS
NOT NULL`) cannot use `file_payload`, which is partial on `removed_at IS NULL`: `SCAN file` and
a sort, for each file the pass files. About 1 s a pass here, O(files²). It needs an index on
the removed files' payload hash.

### 7. Counting by fetching every row

`WaitingForAPerson` (the Needs-attention count and the tray's warning) fetches every
unidentified title, thousands of episodes among them, into Go to count the few that are not
episodes. `ListTitlesByIdentification` scans `title` (no index on `identification_state`). It
took 1.6 s on nas.example while the pass ran. It should be one `COUNT` over an index.

### 8. The planner never gets statistics

Nothing runs `ANALYZE` or `PRAGMA optimize`, so SQLite plans every query without
`sqlite_stat1`. `PRAGMA optimize` at close, and periodically (SQLite recommends after many
changes), is the standard practice. The connection uses the default 2 MB page cache; a
`cache_size` of tens of MB is cheap at our database's size.

### What is fine

- The browse and search reads: `listFilmsByName` and `listSeriesByName` walk `sort_key`'s
  primary key in order; the three search routes ran in 3–7 ms on the 7,500-file database.
- WAL, `synchronous=NORMAL`, one writer connection with a pool of readers, `temp_store=MEMORY`,
  and pragmas read back at open.
- The other 70 plans that scan or sort scan small tables (users, roles, nodes, roots, settings,
  secrets) or sort a handful of rows.

## Fixes, in the order of what they buy

1. **The two missing indexes** (`sort_key(item_id, locale)`, the removed files' payload hash):
   a migration step. Removes the quadratic work; RefreshSortAndSearchKeys becomes milliseconds.
2. **Write only what changed** in the pass: compare before writing text, sort keys and
   resolution, so a pass over an unchanged library writes nothing and its FTS triggers do not
   fire.
3. **Authentication never waits for the writer**: the last-used time is recorded off the
   request path.
4. **One pass for many fetches**: the matcher asks for a pass instead of running one, and the
   passes sweep runs once its burst of fetches is done.
5. **The pass reads outside the writer** and writes its changes in short transactions. A
   larger change, which 1, 2 and 4 make less urgent; it is what keeps the writer free as
   libraries grow.
6. **A count query** for what waits for a person, and an index on `identification_state`.
7. **`PRAGMA optimize`** at open and after passes, and a larger page cache.

A gate is worth having too: a test that runs the pass on a large generated library and fails
when it holds the writer past a bound, and a check that flags `SEARCH … (locale=?)`-style plans
that use only a column with a handful of values.
