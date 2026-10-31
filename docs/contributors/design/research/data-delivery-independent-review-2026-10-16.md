> **ABOUTME:** An independent review of the data-delivery and caching research of 2026-10-16
> (DD1–DD13), from a web performance engineer's and API designer's seat, with evidence the first
> report did not have: the owner's saved series page (80,270 episodes, 45 s to draw). The
> Firefox warning was a quadratic grouping function in the series page fed an unbounded response,
> not polling or a long grid; the report's ranking of causes is rejected. Verdicts on DD1–DD13;
> nine missing principles (MD1–MD9), the first being that no response and no render grows without
> bound; fourteen findings (F1–F14); an answer on the page-shaped single fetch; an ordered plan;
> four owner questions.

# Independent review of the data-delivery research and its principles DD1–DD13

*2026-10-16. Written from the position of a web performance engineer and API designer who has
shipped data-heavy single-page apps and their backends, and who was not involved in the first
report. Everything was read from the repository at an earlier commit or measured in a scratch directory;
nothing else in the repository was changed. Claims marked **(measured)** were measured here, in
Node 22.23 (V8) for JavaScript and Go for the server's encoding. Firefox's engine will differ in
its constants, not in the shape of the growth. Claims marked **(from memory)** are from practice
and were not re-read for this review; check them before acting on them. **Verify** marks what I
could not settle here. No browser was profiled, and nothing was run against nas.example.*

Inputs read:
- [data-delivery-and-caching-2026-10-16.md](data-delivery-and-caching-2026-10-16.md) (the "first
  report"), and the
  [independent data-layer audit](data-layer-independent-audit-2026-10-15.md) for form;
- D166, D237, D273, D303 and D304 in working-notes;
- the owner's saved page `/yoloai/files/Orpheum-quillwick.html`;
- the client: `series.tsx`, `browse.tsx`, `use-resource.ts`, `use-pages.ts`, `polling.ts`,
  `remembered.ts`, `app.tsx`, `title.tsx`, `poster.tsx`, `end-of-list.tsx`, `identify.tsx`,
  `search.tsx`, `continue-watching.tsx`, `link.tsx`, `services.tsx`, `status-cues.tsx`,
  `transport-choice.ts`, `main.tsx`, `sw.ts`, and the SDK's `client.ts` and `rtc-transport.ts`;
- the server: `internal/core/api/browse.go`, `identification.go`, `jobs.go`, `libraries.go`,
  `destinations.go` and `pacing.go`; `internal/core/library/browse.go`, `detail.go`, `art.go`,
  `readable.go` and `home.go`; `internal/store/open.go`; `internal/core/settings/settings.go`;
  the queries in `browse.sql` and `match.sql`; `cmd/orpheum/https.go` and `serve.go`;
- `orpheum-connect/internal/webclient/webclient.go`, which serves the page at app.orpheum-app.com.

Paths are under `web/apps/web/src/` unless they start with `internal/`, `cmd/`, `web/packages/` or
`orpheum-connect/`.

---

## 1. Verdict

**The first report is a good survey with the wrong diagnosis.** Its reading of Firefox's source is
right, and so is the warning it derives from it: one uninterrupted main-thread task of about ten
seconds while the person is giving input. So are most of its mechanisms: polling re-reads
everything scrolled, nothing is compressed, there is no cache between screens, and no JSON
response has a validator. But it then ranked the causes by **how large a real library is**: 3,000
films, a 5,000-title budget. It concluded that "no code path was found that obviously runs for ten
seconds". That is the error. The question to ask of a ten-second task is **what in the code grows
faster than linearly with the data, and how large the data can become**, real or not.

**The answer is in `series.tsx:26-37`.** `bySeason` groups a series' episodes by copying the
season's whole array for every episode it adds:

```ts
seasons.set(episode.seasonNumber, [...(seasons.get(episode.seasonNumber) ?? []), episode]);
```

That is n²/2 element copies for a season of n episodes. On the owner's page, n was 80,270, all in
"Specials":

- that is about 3.2 × 10⁹ copies through about 26 GB of short-lived arrays;
- **in V8 it takes 45.7 s (measured)**, the owner's "about 45 seconds";
- it runs inside `SeriesView`'s render (`series.tsx:58`), unmemoised, as one synchronous task.

Rendering the 80,270 rows takes a few more seconds on top. The server's part is a tenth of a
second. §2 gives the numbers.

So the incident is **an unbounded response, rendered whole, through a superlinear transform**.
Polling then made it repeat. None of the report's three leading candidates (the deep films grid,
Needs a match, several tabs) was the cause. Its principle that would have prevented it (DD9, every
list bounded) covers list endpoints only, and the series endpoint is not one: its episodes are an
array nested in a detail response.

Smaller verdicts:

- **The page-shaped answer is right in kind and wrong in emphasis.** "Compose resources, never
  invent view models" is correct. The "stronger version", one catalogue response per library, is a
  response that grows with the library by design. It should not be adopted before the bound
  principle (MD1) is, and probably not at all at this size (§6).
- **DD4's validators help only an idle library.** Under D304, a library being filed changes
  continuously for as long as it is being filed. A per-library revision `ETag` then never matches,
  and the deep re-read the report rightly condemns goes on as before. The fix for polling is to
  poll **what is on screen** (amended DD5), not to validate what was scrolled.
- **On the paths the owner uses, the browser's HTTP cache does less than the report assumes.**
  On app.orpheum-app.com every media load, images included, is carried by the page for the service
  worker, and on the WebRTC path nothing reaches the HTTP cache at all (F5). "Byte paths are cached
  well" is true only for a page served by the core itself, over HTTPS.
- **Finding 8 (HTTP/1.1 on the LAN door) does not apply to the owner.** A page from
  app.orpheum-app.com may call only `https://` cores (the connectd page's policy,
  `orpheum-connect/internal/webclient/webclient.go`, `policyOf`). The owner is on HTTP/2 or on
  WebRTC.

## 2. The new evidence: what the series page cost

The owner's saved page has **80,270 numbered entries**, ids 607 to 1,244,212 (measured with `grep`
on the file). 80,246 of them read "not on disk" with no name. That is the filing bug fixed in
Earlier commits. Below, each stage of drawing the page, sized from the code and measured
on a synthetic body of the same shape:

| Stage | Where | Cost at 80,270 | Cost at 751 (another series, if one "season") |
| --- | --- | --- | --- |
| Server: queries | `library/detail.go:127-160`: 7 statements, two of which pass all 80,270 ids as one JSON array | not measured; constant in count, linear in rows | — |
| Server: encode | `api/browse.go:315-352` | **9.4 MB JSON in 98 ms; gzip would be 230 KB in 55 ms (measured, Go)** | 88 KB, 0.8 ms |
| Client: parse | `openapi-fetch` | 28 ms (measured, V8) | 0.2 ms |
| Client: change check | `use-resource.ts:46` `JSON.stringify` of every answer | 31 ms (measured) | 0.5 ms |
| **Client: group by season** | `series.tsx:29`, inside render | **45,722 ms (measured)**; 16.7 ms with a `push` instead of the spread (measured) | 3 ms |
| Client: React render of rows | `series.tsx:131-169` | ≥ 2.6 s for `renderToString` of equivalent rows (measured; a lower bound, since there is no DOM); the client also builds about 400,000 DOM nodes | ≈ 25 ms |

And then it repeats:

- **On every poll that sees a change.** The series page is polled (`series.tsx:180-190`). While
  passes were making a new special each time, every poll saw a change. The next poll is scheduled
  at the 2 s first wait before the render starts (`polling.ts`, `nextWaitMs`), so it fires the
  moment the 45 s task ends. The tab never became responsive while the library was being filed.
- **Twice on every navigation within the series.** Choosing a season, or coming back from an
  episode, remounts `Series`, because the route keys it by season (`app.tsx:167-174`).
  `useRemembered` draws the remembered copy first (bySeason, once). Then the fresh answer arrives.
  A new effect's `lastAnswer` is `undefined`, so the answer counts as changed and the page draws
  again (bySeason, twice) (`use-resource.ts:41-50`, `remembered.ts:36-50`).

Nothing in the client or the API limited any of these. The episode array has no `maxItems`
(`api/browse.go:176`), the query has no `LIMIT` (`browse.sql:149-163`), `EpisodeList` maps every
episode (`series.tsx:135`), and `Remembered` keeps 50 pages by count, not by bytes
(`remembered.ts:11`), here about 9 MB each. A real 751-episode series costs nothing anywhere in
this table. What hurt the owner was data a bug produced, and a sound client survives that.

## 3. Principle by principle, and the report's findings

### Principles

**DD1, a screen's first paint takes at most two dependent round trips. *Agree, amend.***
- **Start the budget once a path is up.** From app.orpheum-app.com a cold start first finds the
  core and races its paths, deliberately for "a few seconds" (D273 §7, amended 2026-10-13). That
  dominates a cold start from away and is not a screen's to spend. Budget the path setup
  separately, and measure it first (DD12).
- **The enforcement is right.** Count requests in a happy-dom test, which is machine-independent,
  in the spirit of D303's counted statements. The Playwright timing test can come later.

**DD2, one keyed client cache. *Agree, amend.***
- **Bound it by bytes as well as entries.** `Remembered`'s count of 50 held about 450 MB of
  the series at worst.
- **Have the cache send `If-None-Match` itself.** On the WebRTC path no browser cache sits
  underneath (F5).
- **Let rule 11's convergence take in `usePages` and `useRequest`**, not only `useResource` and
  `Remembered`. That makes four mechanisms, not two.

**DD3, a bundle is composed of resource representations. *Agree as a rule; do not build bundles
now* (§6).** The `$ref` test is the right guard if bundles are ever built.

**DD4, unchanged costs nothing. *Amend.***
1. **Validate by a hash of the body first, not by a revision.** A hash of the encoded body cannot
   go stale, needs no invalidation event (DD13's own rule) and saves the bytes, the parse and the
   client's change check. The server still runs the query, which costs 0.05–0.84 ms
   (performance-targets.md). A revision `ETag` that skips even the query is an optimisation to
   make once measured, and a missed bump in it is a stale screen.
2. **It helps an idle library only.** During a D304 filing the revision moves on every commit, and
   any validator misses. What bounds a poll's cost during a filing is amended DD5.
3. **The client must handle the 304 itself.** On the WebRTC path the headers pass through
   (`rtc-transport.ts:157-170`), but nothing adds `If-None-Match`, and `openapi-fetch` treats a 304
   as a non-success. Inference from the code; **verify**.

**DD5, a poll reads what changed, not what was scrolled. *Agree with the goal, amend the
mechanism.***
- **Not implementable as written.** "One conditional request per list per poll" plus "replace
  only the changed entries" assumes a way to learn *which* entries changed. The API has none: no
  journal (D166 deferred it), no changes feed. A miss on the first page's validator still leaves
  every shown page to re-read.
- **Amend to: a poll reads the window in view, and at most two pages.** Pages scrolled past are
  kept as they are, and read again when they come back into view. That is bounded whatever the
  library or the filing does, and it needs no server mechanism.
- **Merge by id.** Unchanged entries keep their object identity, and an id seen twice is dropped
  (F7).

**DD6, nobody looking, nobody asking. *Agree.*** Chrome's intensive throttling of chained timers in
tabs hidden for five minutes masks part of this there (from memory). Firefox's 1 s background
clamp does not (from memory).

**DD7, push is an accelerator. *Agree, defer.*** With amended DD5 a poll is bounded, and push buys
latency, not safety. Decide it on DD12's measurements.

**DD8, main-thread work proportional to what changed and what is on screen. *Agree, amend.***
1. **Name complexity.** "A transform of server data on the client is linear (or n log n) in its
   input" is the rule that would have caught `bySeason`. Windowing does not help when the
   transform runs before the window.
2. **Test pathological shapes, not only a big library.** The fixture must include one season of
   100,000 episodes and one list of 100,000 titles, beside the 5,000-title library.
3. **Prefer machine-independent enforcement**, as D303 did with statement counts: the rows
   mounted (≤ the window), the renders per poll, the requests per poll. Keep a generous
   wall-clock ceiling (a test timeout) only as the backstop that catches a quadratic.

**DD9, every list the server returns is bounded. *Amend: too narrow, and its enforcement does not
check bounds.***
- **Every array in every response**, nested ones included, is bounded: episodes, versions, files,
  streams, extras, Needs a match's items. It is paged, or carries a declared ceiling that the
  response states it reached. This is MD1.
- **Enforce it with a test of the API's schema.** sql.md §11's plan check reads access paths, not
  row counts. A `TestArch_` test over the OpenAPI document should require every response array to
  carry `maxItems`, or to sit in a page envelope (`items` + `nextCursor`).

**DD10, bytes compressed and sized to their slot. *Agree, amend.***
1. **The core's assets reach app.orpheum-app.com from connectd**, not from the core
   (`orpheum-connect/internal/webclient/webclient.go`, `Handler`). Compressing in the core does
   nothing for the owner's first load there. Whether anything in front of connectd compresses is
   **unverified**.
2. **The WebRTC path gets none of it.** A `Response` built in JavaScript is not decoded by the
   browser, so compression there would need the transport's own framing and `DecompressionStream`.
   Gzip on 9.4 MB of episodes is 41× (measured). Over the data channels, a large response is the
   case where it matters.
3. **Images are lower priority than stated.** Tiles are `minmax(6.75rem–10.5rem, 1fr)`
   (`browse.css`, `tokens.css`), so 108–200 CSS px. `portrait-m` (480 px) is right from density 2.4
   up, which is most phones. Decoding largely happens off the main thread, and off-screen decoded
   images are discarded (from memory). This is about bytes from away, not the main thread.

**DD11, prefetch data on intent, never work. *Agree, amend.***
- **Prefetch only bounded resources.** Prefetching `/collections/606` on hover would have pulled
  9.4 MB.
- **Resolve a contradiction in the first report.** §3.4 suggests "D258's warm-up could be started
  by `GET /items/{id}`". That contradicts DD11 (a hovered prefetch would start a playback) and
  D303's gate that a GET writes nothing. Strike the suggestion.

**DD12, the client reports its own stalls. *Strongly agree; do it first.*** It would have named the
series page and its 80,270 rows in the first report.
- **Add the server's half:** a response with more than a few thousand array items, or more than a
  few MB, logs a warning naming the endpoint and the count. "Digging means missing logs": the
  server knew it was sending 9.4 MB on every poll.

**DD13, keep in memory only what the core invalidates. *Agree.*** Settings first. They are still
read from SQLite on every read (`internal/core/settings/settings.go:215`), although D303 phase A
listed "settings".

### The report's ranking of causes (§2.2) and its findings (§6)

- **§2.2: reject the ranking.** Item 1 (a deep grid re-rendered) is real jank, and its own
  arithmetic, 0.1–0.3 s at 3,000 items, shows it is not a ten-second task. Item 2 (Needs a match)
  is bounded in practice by "titles with a live file" (`match.sql:84-104`), so the 80,246 empty
  specials never reached it. Item 3 is mostly moot: Firefox desktop gives each site up to 4
  content processes (`dom.ipc.processCount.webIsolated`, 4 off Android; Firefox `all.js`, in the
  cache), so a few Orpheum tabs need not share one. The cause is §2 above.
- **Finding 1 (poll re-reads every page shown): agree, re-rank to F3,** and fix it by amended DD5.
  The "1.1 GB an hour" is an upper bound: each poll waits for its sequential answers before the
  next is scheduled.
- **Finding 2 (Needs a match unbounded): agree.**
- **Finding 3 (no cache between screens): agree.** Add the remount per season (F2).
- **Finding 4 (nothing compressed): agree,** with DD10's amendments.
- **Finding 5 (polling never pauses): agree.**
- **Finding 6 (cold start reads twice): agree.** On the series page it also doubled the 9.4 MB
  download.
- **Finding 7 (posters oversized): agree, lower priority** (DD10 above).
- **Finding 8 (six connections on the LAN door): reject for the owner** (§1). Replace it with F9.
- **Finding 9 (per-request reads that could be in memory): agree, small.**
- **Finding 10 (no field measurement): agree, first.**
- **Finding 11 (web-client.md says lists are virtualised): agree.** web-client.md also says "no
  cap on search", while `/search` caps at 100 (`api/browse.go:188`). Record both in
  plan-deviations.md.
- **Q1–Q4:** answered in §6 and §8.

What the report got right and the plan should keep: its Firefox source work (§2.1); the
heartbeat (DD12); validators over TTLs; seeding a detail screen from a list's entry; the rule that
a bundle is never a new schema; and its caution about normalized caches.

## 4. Missing principles

**MD1. No response and no render grows without bound with data.** Every array a response carries
has a ceiling the API declares, whether it is paged or capped. Every list a screen shows mounts at
most a window of rows. The test of a bound is data a bug can make, not the largest real library:
a season of 100,000 episodes, a title with 10,000 versions. **Enforced by:** the OpenAPI
`TestArch_` test of DD9 *(new)*; a happy-dom test that a series with 100,000 episodes in one
season mounts at most the window's rows *(new)*; a store test that every `:many` query reached
from a GET handler has a `LIMIT` or is listed with its natural bound *(new)*.

**MD2. Client transforms of server data are linear.** No spread or `concat` into an accumulator
inside a loop, and no `find` or `filter` inside a `map` over the same list. Group and index with
`Map` and `push`. **Enforced by:** a `no-restricted-syntax` rule on an array spread whose result
is assigned back inside a loop *(new; a gate change for the owner)*; MD1's 100,000-row test, whose
default timeout catches a quadratic.

**MD3. A render does derived work once.** Grouping, sorting and indexing a response happen once
per response, memoised on its identity (`useMemo`), or once when it arrives, never on every render
of the component. **Enforced by:** review; MD1's test counts the transform's calls across two
renders *(new)*.

**MD4. A screen's data follows its navigation.** A screen asks for what it shows: a series'
seasons with their counts, then one season's episodes; a grid's window. A route that remounts its
screen does not re-read what it already holds. **Enforced by:** DD1's request-count test per
screen, which here includes a season's page *(new)*.

**MD5. A list read in pages is merged by id.** A multi-page read is not a snapshot, and under D304
titles move in name order as they are matched. An id seen twice is kept once, at its latest place.
**Enforced by:** a `use-pages` test in which a title moves from page 1 to page 2 between reads
*(new)*.

**MD6. A multi-statement read is one snapshot (D303's M2, applied to the API).** `Browser.Title`,
`Series` and `List` run 5 to 12 statements each through `Database.Read()` (`library/detail.go:45-75`,
`127-160`; `readable.go:114-176`), each on its own commit, though `Database.ReadTx` now exists
(`internal/store/open.go:191-198`). Its only caller in `internal/core` is
`internal/core/auth/auth.go:552`. **Enforced by:** a test that a read handler's statements share
one transaction (`querytally` can say which connection ran them) *(new)*.

**MD7. Every path to the core is measured on its own.** HTTPS direct and WebRTC behave
differently for caching, compression, concurrency and main-thread cost (F5, F9). A budget or a
claim names its path. **Enforced by:** DD12's reports carrying the path in use, which the path
keeper knows (`main.tsx:117-133`) *(new)*.

**MD8. A long main-thread task stalls the tab's media too, so the main thread is a shared
resource.** On app.orpheum-app.com the page carries every media load for the service worker
(`main.tsx:139-145`; `sw.ts:88-99`), so the 45 s render also stopped every image and every HLS
chunk in that tab. **Enforced by:** DD8's budget, and moving the carrying into a worker if DD12
shows contention *(later)*.

**MD9. A response's size is logged when it is large.** This is the server's half of DD12.
**Enforced by:** a test that a response over the threshold logs its endpoint and item count
*(new)*.

## 5. Findings, by severity

### High

**F1. The series page's grouping is quadratic, inside render, on an unbounded response.**
- **Where:** `series.tsx:26-37`, `58`.
- **Cost:** 45.7 s at 80,270 (measured), 1.4 s at 20,000 and 28 ms at 5,000. This is the Firefox
  warning.
- **Fix:** a `push` into a `Map` (16.7 ms at 80,270, measured), memoised on the response (MD2,
  MD3). This is a few lines with no design question, and should go out first.

**F2. The series endpoint and page are unbounded, polled, and re-read per season.**
- **Where:** `browse.sql:149-163`; `library/detail.go:136-155`; `api/browse.go:315-352`;
  `series.tsx:131-169`, `180-190`; `app.tsx:167-174`.
- **Cost:**
  - each poll and each season click moves the whole series: 9.4 MB at the series' count,
    re-stringified on every answer;
  - two of the seven statements bind every episode id as one JSON array
    (`shownArtSourceSha256s`, `unreadableItems`), about 640 KB of parameter text at 80,270.
- **Fix:** MD1 and MD4. A series read returns the seasons with counts and first stills, as SQL
  counts (DL6); a season read is paged; `EpisodeList` is windowed. This changes the API, so it is
  Q2 for the owner.

**F3. A deep list's poll re-reads everything scrolled, and under D304 it never backs off.**
- **Where:** `use-pages.ts:35-44`, `59-84`.
- **What it is:** the first report's finding 1, kept.
- **What is added:** while a library is filed, every poll changes, so the wait stays at 2 s for
  the length of the filing, and D304 §1 puts the video reads at the slow priority.
- **Fix:** amended DD5.

**F4. No response array in the API declares a bound.**
- **Where:** a search finds `maximum:` only on `/titles` (`limit` ≤ 500), `/search` (≤ 100) and
  `/files` (≤ 1000) (`api/browse.go:75`, `188`; `api/libraries.go:176`). `/jobs` is bounded by
  the setting `job_view_max_jobs` (`api/jobs.go:99-103`).
- **Unbounded today:** `/items` (Needs a match, `match.sql:84-104`); a series' episodes; a title's
  versions, files, streams and extras.
- **Fix:** MD1.

### Medium-high

**F5. On app.orpheum-app.com, media goes through the page, and on WebRTC nothing is HTTP-cached.**
- **Where:**
  - `artUrl` is relative to the page (`web/packages/sdk/src/client.ts:116-122`);
  - the page carries media for the worker on every path (`main.tsx:139-145`);
  - the worker answers from the page (`sw.ts:88-99`) and uses no Cache API;
  - the WebRTC transport builds its `Response` in JavaScript (`rtc-transport.ts:297`).
- **Cost:**
  - **direct HTTPS:** the page's own `fetch` still has the HTTP cache, but every poster passes
    through the main thread twice (a port message each way);
  - **WebRTC:** every poster and every JSON body is fetched again on every page load, because
    `immutable` means nothing there.
  - Inferred from the code and the Fetch and Service Worker specifications; **not measured**.
- **Fix:** a Cache API store in the worker for `/bytes/art/` (content-addressed, so never stale),
  and the client cache's own validators (DD2, DD4).

**F6. A series list page reads every live file of every episode of its series.**
- **Where:** `readable.go:138-176` with `ListLiveFileLocationsOfSeries` (`browse.sql:175-184`),
  called for `kind=series` from `finishPage` (`readable.go:114-136`).
- **Cost:** the rows grow with episodes, not with the page's 60 series. A page holding one series
  alone returns at least 751 rows, and the loop needs only the first readable one per series. The
  same applies on every poll and every page re-read (F3). This breaks DL6.
- **Fix:** one row per series. Either the first readable location, which needs the readability
  check in SQL or per node, or the distinct (series, node, root) triples, of which there are a
  handful.

**F7. A re-read across pages can show a title twice or lose one.**
- **Where:** `loadAgain` reads pages one after another and `appendPage` concatenates them
  (`use-pages.ts:27-44`); keys are by id (`browse.tsx:46`).
- **Cost:** a title renamed between two page reads moves past the cursor. Under D304 that happens
  whenever a "still reading" path name becomes a provider's name, so it will be common. The result
  is a duplicate React key or a gap.
- **Fix:** MD5.

**F8. The bundle is one 1.22 MB script, of which hls.js is a large part.**
- **Where:** `apps/web/dist/assets/index-*.js`, 1,224,295 bytes and 372,700 gzipped (measured). The
  full `hls.js` build is imported statically (`web/packages/player/src/hls.ts:15`, `chunked.ts:3`,
  and others), and `hls.min.js` alone is 619,692 bytes (measured on `node_modules`). There is no
  dynamic `import()` in the app.
- **Cost:** the films grid parses and compiles the player, Watch Together's voice and every admin
  screen. On a phone that is a cost on every new release (not measured).
- **Fix:** `import()` the play and admin routes. Vite splits them with no new dependency.

### Medium

**F9. Over HTTP/2, voice's pacing slows the API too.**
- **Where:** `api/pacing.go:31-48`, which says so: "under HTTP/2 they share it with the film's".
  Go serves HTTP/2 over TLS (`cmd/orpheum/https.go:163-170`).
- **Cost:** while a person's voice is live, the TCP connection is paced near the film's rate and
  marked lowest effort. Posters, polls and a large JSON body queue on the same connection. This is
  small unless a response is large, which F2 made possible.
- **Fix:** none needed once MD1 holds. Note it so that it is not misdiagnosed.

**F10. The end-of-list watcher is rebuilt on every render.**
- **Where:** `usePages`' `loadMore` is a new function on each render (`use-pages.ts:155`), and
  `EndOfList` re-creates its `IntersectionObserver` whenever it changes (`end-of-list.tsx:19-33`).
- **Cost:** every poll's re-render tears the observer down and builds it again. A new observer's
  first callback reports an end already in view, so a re-render can ask for another page.
- **Fix:** a stable `loadMore` (`useCallback` over refs).

**F11. The API's reads are not snapshots.**
- **What it is:** MD6.
- **Cost:** while a pass commits, a title's page can show versions from one commit and art from
  another. This is cosmetic at worst today, but it is the read skew D303 named.

**F12. A title's detail reads streams once per file, and its art in three statements.**
- **Where:** `library/detail.go:100` (`ListStreamsOfVideoFile` per file); `art.go:50-63` (one
  `ListShownArtwork` per kind).
- **Cost:** about 10 statements plus one per file plus one per node. Bounded by a title's files,
  so this is low on its own. Fold it in with MD6.

### Low

**F13. Every grid item calls `useTranslation`.**
- **Where:** `Poster` calls it before it knows whether it needs it (`poster.tsx:11`), and
  `PosterGrid` calls it too.
- **Cost:** each call subscribes its component to i18next's events, so 3,000 posters make
  thousands of subscriptions (from memory of react-i18next). This is small next to F1–F3.
- **Fix:** pass `t` down, or call it only in the blank branch.

**F14. `EmptyList` reads `/me` again** (`browse.tsx:174`), although `SignedIn` holds it
(`app.tsx:85-87`). DD2's de-duplication covers this.

The parts that are sound: keys are stable ids everywhere I looked; the services context is stable
(`main.tsx:94`), so there is no context churn beyond the one-time admin and poller changes; Jobs,
Continue Watching and search are bounded; art URLs are content-addressed and immutable; and keyset
paging on the server is right.

## 6. The page-shaped single fetch

**For it, at its strongest.**
- **From away, round trips are what a person feels.** At 150 ms, a film page's chain (title, then
  the warm-up, then the playlists) is visible. On mobile networks, waterfalls cost more than
  bytes.
- **One response is one place to measure and one transaction to read in.**
- **Industry practice backs it.** Netflix, SoundCloud and Plex all moved to screen-shaped reads
  for their clients, and so do React Router's loaders and RSC.
- **Orpheum controls every client**, so the coupling it brings is internal.

**Against it, at its strongest.**
- **A page's data is exactly what grew without bound here.** A "series page" bundle would have
  been the 9.4 MB series plus everything beside it, impossible to poll cheaply and impossible to
  validate per part.
- **Bundles concentrate unbounded growth, and fix the API to today's layout.** The owner already
  changed this page's layout once (series, then seasons, then episodes; `series.tsx:47-48`).
- **A bundle cannot be invalidated by entity, and fails as a whole.**
- **Over HTTP/2 and the data channels, independent requests already share one round trip.** A
  bundle buys only the *dependent* steps, and there are few of them.
- **The catalogue variant is a response that grows with the library on purpose.** Under a filing
  it changes on every poll, so it needs D166's journal to be cheap. A filing bug like this one,
  aimed at films, would have made it hundreds of megabytes: item ids reached 1,244,212 on
  nas.example.

**My recommendation.** Shape responses to **the viewport and the navigation, not to the page**.
1. **Resources stay the unit**, each bounded (MD1).
2. **A screen starts its independent reads together at the route boundary**, which needs no server
   change.
3. **One session-shaped read, not a page-shaped one**: the shell's facts in one answer: who you
   are, admin or not, the client settings, the attention counts, your Watch Together session. It
   is stable, small, and removes the only dependent step every screen pays (`/needs-attention`
   after `/me`), and the double read when the client settings arrive.
4. **Remove a data dependency** (a film's warm-up after its title) by seeding from the list's entry
   (DD2), not by a GET that starts work.
5. **No compound endpoints and no catalogue until DD12's field data from away says round trips
   dominate.** Then build the first one under DD3's `$ref` rule.

I agree with the first report's rule (compose, never invent), and disagree with its "stronger
version" and with building compound endpoints now.

## 7. Plan

**First, for the owner on nas.example. These are client changes, independent of the data layer, and
small.**
1. **F1.** Make `bySeason` linear and memoise it on the response. Its revert-red test groups
   100,000 episodes within the test's timeout, which the quadratic misses by minutes.
2. **Window or cap the episode list** (MD1, client half). Its test mounts a 100,000-episode season
   and counts the rows mounted.
3. **DD12's heartbeat and MD9's large-response log**, so the next stall names itself.
4. **When the D303 baseline reaches nas.example, its fresh database drops the 80,246 rows** (D303 §2).
   Ship 1–3 anyway: the next bug will find another array.

**In the D303/D304 data-layer work, now.**
5. **MD1 at the API:** the season-scoped series read (owner Q2), `maxItems` on every response
   array, the OpenAPI `TestArch_` test, and Needs a match paged by keyset.
6. **F6:** one answer per series for its readability (DL6).
7. **MD6:** the Browser's reads through `ReadTx`.
8. **DD13:** settings in memory.
9. **Before D304's "still reading" reaches the grid:** amended DD5 (poll the window, merge by id,
   MD5) and DD6 (hidden tabs do not poll). D304 makes every list change for hours, so this belongs
   with it, not after it.

**After it, in the client audit.**
10. One keyed cache (DD2), sending its own validators, with DD4's body-hash `ETag` on JSON GETs.
11. The windowed poster grid (DD8, amended) and its budget tests, pathological shapes included.
12. Split the bundle (F8). Then gzip on the core's JSON and assets, and a word with connectd about
    the page's (DD10).
13. A worker-side art cache for the WebRTC path (F5).
14. The shell's session read (§6); `srcset`; prefetch of bounded resources on intent (DD11).
15. Push (DD7) or the catalogue only if DD12's data asks for them.

## 8. Questions for the owner

**Q1. What should a screen do when the data is absurd?**
- **The kind of gap:** today the client trusts the server's counts. A bug that makes 80,000 of
  something makes 80,000 rows on screen.
- **The concept:** is an implausible count only a rendering problem, to be windowed and survived,
  or also a health signal for the admin?
- **My recommendation:** both. Every list is windowed (MD1). Separately, a series with far more
  episodes than its files and its provider list could account for appears in Needs attention.
  The second is new behaviour, so it is yours to decide.

**Q2. Should the API follow the series → season navigation?**
- **The kind of gap:** the page shows one season at a time (your choice, 2026-10-07). The API
  sends every season's episodes every time, and re-sends them on each season click.
- **The concept:** should a read's shape follow how a person moves through the screens? Here,
  that is a series read with its seasons and their counts, and a season read with its episodes,
  paged.
- **The cost:** an API change, which Cut A's frozen design (rule 1) puts to you. Your standing
  word is that protocol formats may change freely during development.

**Q3. How fresh must the parts of a list that are off-screen be while a library is being filed?**
- **The kind of gap:** D304 accepts eventual consistency. The poller still re-reads every page
  scrolled through every 2 s.
- **The concept:** is the window in view enough to keep current, with the rest refreshed when it
  scrolls back into view?
- **My recommendation:** yes (amended DD5). It bounds a poll whatever the library does.

**Q4. The first report's Q2 (a library's catalogue in the browser) and Q3–Q4 (a cache library, a
windowing library).**
- **My recommendation:** decline the catalogue for now (§6).
- **For the cache and windowing:** our own code. `useResource` is most of a keyed cache already,
  and the grid's rows are uniform.
- **No new dependency is proposed here.** If you prefer TanStack Query or a virtualiser, that is
  a dependency decision under rule 3, to be audited under D220.

## 9. What I could not verify

- **Firefox's own timings.** The timings are V8's. SpiderMonkey's array spread and GC will give a
  different number, of the same order, for 3.2 × 10⁹ copies.
- **Which path the owner's tab used** when the page was saved, direct HTTPS or WebRTC. Both lead to
  the same conclusion.
- **Whether anything in front of connectd compresses** the page's assets.
- **Whether opening a data channel per request costs a round trip.** RFC 8832 lets the opener send
  before the channel's acknowledgement arrives (from memory), and the first report also left this
  unmeasured.
- **How Go's HTTP/2 write scheduler orders** a large JSON body against a film's chunks on one
  connection.
- **That `openapi-fetch` rejects a 304** (DD4 item 3). This was inferred from its treatment of
  non-2xx answers and not tested.
- **The decoded-bitmap and off-main-thread decoding behaviour** of DD10 item 3 (from memory).

## Sources

**In this repository**
- [data-delivery-and-caching-2026-10-16.md](data-delivery-and-caching-2026-10-16.md);
  [data-layer-independent-audit-2026-10-15.md](data-layer-independent-audit-2026-10-15.md);
  [web-client.md](../plans/web-client.md) ("Browsing, settings and errors");
  [performance-targets.md](../plans/performance-targets.md); decisions D166, D237, D258, D262,
  D273, D303 and D304 in [working-notes.md](../../decisions/working-notes.md).
- Commits earlier commits (the filing bug and its invariant).
- The owner's saved page, `/yoloai/files/Orpheum-quillwick.html` (5,296,449 bytes).

**Measurements (scratch scripts, not committed)**
- `bySeason` as written and with `push`, plus `JSON.stringify` and `JSON.parse` of a synthetic
  series body at 751, 5,000, 20,000 and 80,270 episodes: Node 22.23.
- `react-dom/server` `renderToString` of equivalent rows: React 19.3.0 from `web/node_modules`.
- Go `encoding/json` and `compress/gzip` of 80,270 `EpisodeBody` values.

**Firefox** (from the first report's cache, `/yoloai/cache/research-data-delivery/`)
- `modules/libpref/init/StaticPrefList.yaml`: `dom.max_script_run_time` = 10,
  `…require_critical_input`.
- `modules/libpref/init/all.js`: `dom.ipc.processCount.webIsolated` = 4 off Android.

**From memory, not re-read for this review**
- RFC 8832 (WebRTC Data Channel Establishment Protocol), on sending before the acknowledgement.
- Chrome's intensive timer throttling of hidden pages, and Firefox's background timeout clamp.
- react-i18next's per-component subscription in `useTranslation`.
- Browsers' off-main-thread image decoding and their discarding of off-screen decoded images.
