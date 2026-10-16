> **ABOUTME:** How data reaches the web client today, measured from the code: every request each
> main screen makes, in order, what it costs and what is cached; the likely causes of Firefox's
> "This page is slowing down Firefox" on nas.example and how to confirm each; what practitioners say
> about client caches, HTTP caching, push, per-screen ("page-shaped") fetches, batching,
> prefetching, windowing, compression and images; what the core should keep in memory; proposed
> principles DD1–DD13 with what would enforce each; findings in order of impact; and the owner's
> questions. Written 2026-10-16, after D303's data-layer research and before the client is looked
> at "hard".

# Data delivery and caching: research, 2026-10-16

The owner, after Firefox on nas.example showed *"This page is slowing down Firefox. To speed up your
browser, stop this page."*:

> *"We have multiple tiers of 'data access speed' … from the client's perspective (which affects
> UX), everything gets fetched over the network. So in addition to client-side caching, we should
> be thinking in terms of optimizing how the data gets to the client. For example, batching the
> data that a client is likely to need soon. Collecting all of the data that a web page is going
> to need and sending it in one chunk rather than multiple requests. Some such optimizations could
> force a shape to match the web page shape … maybe even that's not too bad as long as our data
> batching design remains supple enough? Or maybe page-based single data fetch isn't such a great
> idea? Either way, I want to know."*

This round read the web client (`web/apps/web`, `web/packages/sdk`, `web/packages/player`), the
core's API and its caches, and, online, the sources listed at the end: Firefox's own source for
its hang warning, the HTTP caching RFCs and their practitioners, the client-cache libraries'
authors, the "backend for frontend" and screen-shaped-API literature, and what Jellyfin, Plex and
Navidrome do. Nothing was run against nas.example, and no browser was profiled: **every cost below
is computed from the code or measured on a synthetic payload, and says so; none was measured in a
browser.** Section 2 says how to measure each.

Paths below are under `web/apps/web/src/` unless they start with `internal/`, `cmd/` or
`web/packages/`.

**The short answer to the owner's question.** A screen should get what it needs for its first
paint in **one round trip**, and the way to get there is **not** a bespoke view model per page.
It is a screen endpoint (or a "load" that the client issues in parallel) that **composes the same
resource representations the resource endpoints return**, keyed so that one client cache is
filled by either. At Orpheum's size there is a stronger version of the same idea that is not
page-shaped at all: the whole browse index of a library is about 150–215 KB gzipped, small enough
to be one cached, revision-validated response from which every list screen renders with no
further requests. But the measured problems today are not the number of requests on first paint.
They are **polling that re-reads everything a person has scrolled through**, **whole lists
re-rendered on every change**, **nothing compressed**, and **no cache between screens**. Fix
those first. §3.4 gives the reasoning, §5 the principles and §6 the order.

## 1. Today, measured from the code

### 1.1 The client's data machinery

| Piece | What it does | Where |
| --- | --- | --- |
| `openapi-fetch` client | Typed calls. Middleware adds `Authorization`, rewrites a proxy's error, watches for a 401 and for a new client build. Nothing caches, dedupes or batches. | `web/packages/sdk/src/client.ts:62-80`; `main.tsx:74-79` |
| `useResource(key, load, poller?)` | Loads on mount and on a key change, and keeps the value in component state. With a poller it asks again on D237's backing-off schedule (2 s doubling to 30 s), compares answers by `JSON.stringify`, and sets state only on a change. A new key starts from "loading". | `use-resource.ts:31-83` |
| `usePages(load, poller?)` | A list read a page at a time. Each poll re-reads **every page shown so far**, from the first, one after another, then compares the whole merged list by `JSON.stringify` of before and after. | `use-pages.ts:35-45`, `59-82`, `97-115`, `125-184` |
| `Remembered` / `useRemembered` | A per-tab map of the last 50 title and series pages, shown while the page loads again. It is the only client cache, and it covers no lists. | `remembered.ts:11-50`; `title.tsx:256-266`; `series.tsx:174-189` |
| Polling | `setTimeout` chains per component. No visibility check, so a hidden tab keeps polling. Pollers are not shared between components. | `polling.ts:71-86`; `app.tsx:131-147` |
| Push | None for the library. The only server-to-client channel is Watch Together's session WebSocket. D237 §3 chose polling and left push for later. | `internal/core/api/together_socket.go:258-262`; working-notes D237 §3 |
| Lists | Rendered whole. `content-visibility: auto` on each grid item skips layout and paint off-screen, but React still renders and diffs every item on every change. No item is memoised. | `browse.tsx:38-74`; `browse.css:26-31` |
| Images | One `<img loading="lazy" width=240 height=360>` per poster at `portrait-m` (480×720), with no `srcset`. Content-addressed URLs, served `private, max-age=31536000, immutable`. | `poster.tsx:21-27`; `internal/nodeproto/artwork.go:34-36`; `internal/core/api/media_cookie.go:145`, `art.go:80` |
| Session check | `GET /me` every 60 s while visible, and whenever the tab is shown again. | `signed-out-watch.ts:33`, `49-70`; `main.tsx:77` |
| Path keeper | Looks for a better path to the core every 30 s. | `main.tsx:110`, `146-153` |
| WebRTC path (away, or home through D300) | Each request opens a data channel of its own on one of 4 peer connections, up to 12 under way on each, and sends once the channel opens. Media loads (images, chunks) are fetched by the page for the service worker. | `web/packages/sdk/src/rtc-transport.ts:1-19`, `177-191`, `360`, `453-466`; `sw.ts` |

**Two client caches already exist**, and they are different mechanisms: `useResource`'s
component state, and `Remembered`. Rule 11 ("if there are already two, stop and say so")
applies to any third. §5's DD2 proposes converging on one.

### 1.2 The server side

- **Every JSON response is built fresh** by a huma handler. No handler sets `ETag`,
  `Last-Modified` or `Cache-Control`, so a browser never revalidates JSON and never gets a 304.
  The only `ETag` in the tree is on the node's byte server (`internal/node/byteserver/byteserver.go:126`).
- **Nothing is compressed.** No `Content-Encoding` is set anywhere in `internal/` or `cmd/`.
  The web client's files go through `http.ServeFileFS` with immutable `Cache-Control` on
  `assets/` and `no-cache` on `index.html`, uncompressed (`internal/core/webclient/webclient.go:65-70`).
  Measured on today's `web/apps/web/dist`: the one script bundle is **1,224,295 bytes, 372,700
  gzipped (-9)**, and the stylesheet is 29,500 bytes, 5,791 gzipped.
- **Byte paths are cached well.** Images are immutable for a year. Finished segments and
  subtitles are `private, max-age=86400`, and a growing playlist is `no-cache`
  (`internal/core/api/media_cookie.go:140-155`).
- **HTTP versions.** HTTPS is served by `ServeTLS` (`cmd/orpheum/https.go:163-170`), and Go's
  server "automatically enable[s] HTTP/2 support when using HTTPS" (net/http documentation).
  The LAN door is plain HTTP (`cmd/orpheum/serve.go:59-68`). Browsers do not speak unencrypted
  HTTP/2, so a page on the LAN door is HTTP/1.1 with the browsers' six connections per host.
- **In-memory state in the core:** a session's last use is noted in memory and written in
  batches (`internal/core/auth/auth.go:560`, DL10). **Settings are read from SQLite on every
  read** (`internal/core/settings/settings.go:207-231`; the owner queued "settings held in
  memory" on 2026-10-05). Every list request reads the node roots to name nodes
  (`internal/core/api/destinations.go:42-57`) and builds a collation sorter
  (`internal/core/library/browse.go:226-236`). No query result is cached.
- **The queries are cheap.** performance-targets.md measured a 50-film poster grid at 0.84 ms,
  Continue Watching at 0.055 ms and a detail page at 0.050 ms. nas.example's 3–47 s waits were
  requests queued behind the writer, because authentication wrote; D303 (DL10) removes that. A
  `/titles` page runs about seven statements: the session and its idle-expiry setting, the sort
  locale, the list, the posters, the files' locations and the node names
  (`internal/core/api/browse.go:226-250`, `internal/core/library/readable.go:114-133`).

### 1.3 What each main screen costs

"RTT" counts the dependent steps after the app's script is loaded, on a signed-in, warm-cache
visit. Requests in one step go out together. Sizes are estimates unless marked measured.

**The app shell (every screen, on mount)** (`app.tsx:79-124`, `status-tray.tsx:105-127`):

| Step | Requests |
| --- | --- |
| 1 | `GET /me`, `GET /client-settings`, `GET /together/mine`, and the screen's own (below) |
| 2 | `GET /needs-attention`, for an admin only, because it waits for `/me` to say "admin" |
| 2′ | When `/client-settings` arrives, the poller object changes (`app.tsx:140-146`). Every polled `useResource` on screen restarts and **reads again at once** (`use-resource.ts:72`, `78`), and the first answer is dropped. The home screen thus reads `/home` twice on every load. |

**Films (the home screen)** (`browse.tsx:80-139`, `continue-watching.tsx:167-174`):

| Step | Requests |
| --- | --- |
| 1 | `GET /home` (Continue Watching) and `GET /titles?kind=movie&sort=name&limit=60`, alongside the shell's |
| 2 | Posters for the visible tiles and Continue Watching rows; the rest load lazily as they scroll in. On the HTTP/1.1 LAN door, six at a time. |
| scroll | `GET /titles?cursor=…` each time the end of the list comes into view: one sequential request per 60 titles |
| poll | `GET /home`, plus **every page shown so far, re-read one after another from the first** (`use-pages.ts:35-45`, `69`), at 2 s, doubling to 30 s while nothing changes, back to 2 s on any change |

A `/titles` page of 60 is about **12.8 KB of JSON, 3.3 KB gzipped**. That was measured on a
synthetic page with the `EntryBody` shape of `internal/core/api/browse.go:19-31`: about 213 bytes
an entry, 64 of them the poster hash. Repeated names make the gzip figure optimistic. **3,000
films scrolled to the end is 50 pages, about 630 KB, re-read in 50 sequential requests on every
poll.**

**A series list** is the same with `kind=series`, and has no Continue Watching.

**A film's page** (`title.tsx:256-299`, `watching.ts:120-139`):

| Step | Requests |
| --- | --- |
| 1 | `GET /items/{id}` (a remembered copy shows meanwhile, when this tab saw it before) |
| 2 | Poster and backdrop. `POST /playback`, the warm-up (D258), because it waits for the title to name its first file. |
| 3–5 | The hidden preload's master playlist, media playlist and first chunk, when the film is on its ladder (D260 §3) |
| poll | `GET /items/{id}`, backing off 2 s → 30 s |

**A series' page** (`series.tsx:174-200`): `GET /collections/{id}` (the series with every
episode), then the stills. Choosing a season remounts the page (`app.tsx:167-175`) and reads the
same resource again, showing the remembered copy meanwhile. It is polled.

**The player's start** (`play.tsx:372-406`, `film-player.tsx:455-489`, `529-532`;
`web/packages/sdk/src/ready.ts`):

| Step | Requests |
| --- | --- |
| 1 | The warm decision from the film's page, if it is fresh; otherwise `POST /playback` |
| 2 | `whenReady`: `GET` the playlist, repeated on each 425 after its `Retry-After` |
| 3+ | hls.js: the master playlist, the media playlist, then chunks. `GET /files/{id}/availability` and the subtitle tracks go out alongside. |

The warm-up removes one to four of those steps from the press of Play. That is the right idea
(D258), and the same idea §3.5 calls prefetching on intent.

**From away**, every dependent step above is a round trip of the relayed or WebRTC path
(typically 50–150 ms, more on mobile; network-profiles.md). Each request also opens a data channel
first (`rtc-transport.ts:177-191`). Whether that costs a round trip before the request is sent was
**not measured**.

## 2. Firefox's "This page is slowing down Firefox"

### 2.1 What triggers it

From Firefox's source, not a forum:

- **The words are the selected tab's.** `processHang.selected_tab.label` = *"This page is
  slowing down %1$S. To speed up your browser, stop this page."* An extension's hang reads
  *"“%1$S” is slowing down…"* with the extension's name instead (`browser.properties`). So the
  owner's warning blames **the Orpheum tab's own content script**, not an add-on.
- **The threshold is one continuous script run of `dom.max_script_run_time`, 10 s by default**
  (`StaticPrefList.yaml`). XPConnect's interrupt callback measures the time **"since returning to
  the event loop"**, and only after half the limit has passed twice (`XPCJSContext.cpp`). Many
  short tasks never trigger it, however much they add up to.
- **On desktop it also needs the person to be interacting.**
  `dom.max_script_run_time.require_critical_input` is true on desktop: the warning shows only when
  critical input (a click, a key) is queued behind the running script.

**So the warning means: one uninterrupted main-thread task of about ten seconds or more, in the
Orpheum tab, while the owner clicked or typed.** Ordinary jank (tasks of 50–500 ms) degrades the
feel but never shows this warning.

Two facts matter for what can cause it. React 19 renders an update that is not a transition (a
`setState` in a fetch's `.then`, which is every update here) synchronously, as one task. And
anything synchronous the script triggers, such as a JSON round trip or a forced layout, counts
toward the same task.

### 2.2 The candidates, most likely first

No code path was found that obviously runs for ten seconds. These are the places where one
synchronous task grows with the library or with how far a person scrolled, ranked by how
plausibly they reach ten seconds on nas.example's library.

1. **A long list re-rendered whole when a poll finds a change.**
   - **What happens:** `usePages` replaces the whole merged list (`use-pages.ts:130-136`). `BrowseView` then re-renders every `PosterGrid` item: a `Link`, a `Poster` with its own `useTranslation`, a `StatusTray`, and a `t()` call for the year (`browse.tsx:38-74`, `poster.tsx:10-29`). Each comparison also stringifies the whole list twice (`use-pages.ts:73`).
   - **Why it bites now:** under D304 §4, a library being filed changes on almost every poll, so a grid scrolled deep re-renders in full every 2 s for the length of a filing.
   - **Cost:** the per-item cost was not measured. At a plausible 30–100 µs an item in Firefox, 3,000 items is 0.1–0.3 s a render. Reaching ten seconds needs a far larger list, a slow machine, or one of the multipliers in items 3–4.
   - **Confirm:** a Firefox Profiler capture of the films grid scrolled to the end during a scan; the React DevTools profiler on a profiling build.
2. **A large list mounted at once.**
   - **Needs a match** reads `GET /items?identificationState=…` with **no `LIMIT`** (`internal/store/queries/match.sql:88-104`; `identify.tsx:104-121`), renders every row with a `useRequest` each, and polls. In a fresh library, or a filing under D304, "unidentified" can be most of the library.
   - **Files** loads 500 rows a page (`files.tsx:135-150`) and, polled, re-reads every page shown.
   - **Confirm:** open each with a few thousand rows under the Firefox Profiler.
3. **Several Orpheum tabs, or a hidden tab, adding to the same thread.**
   - **What happens:** polls continue in hidden tabs (`polling.ts:71-86`).
   - **Unverified hypothesis:** with site isolation, tabs of one site may share a content process and so its main thread. If they do, a deep grid open in a background tab works on the same thread as the tab the owner clicks in. This was not checked against Firefox's process-allocation rules.
   - **Confirm:** `about:processes` while it happens, which shows which tabs share the process using the CPU.
4. **Synchronous JSON work on large answers.** `useResource` stringifies every polled answer (`use-resource.ts:46`); `usePages` stringifies the merged list twice per poll. At 630 KB that is milliseconds, not seconds. It only matters as a multiplier of item 1.
5. **Not the data layer.**
   - **Media:** hls.js transmuxes in its worker, and jassub renders ASS in a worker. The player's timers are 0.5–10 s intervals of small work (`web/packages/player/src/abr.ts:49-52`, `164-174`; `link.ts:18`; `report.ts:48`). Voice's level meter writes one CSS property every 100 ms (`together-voice.tsx:171`, `193-213`).
   - **WebRTC path:** each 64 KB message of a film's bytes passes through the page's main thread on its way to the service worker. That costs many small tasks, not one long one.
   - **Verdict:** nothing here is a ten-second candidate. Each is worth a glance in the same profile.

### 2.3 How to confirm, in order of effort

- **Catch it in the act on nas.example.** Use profiler.firefox.com's "Web Developer" preset: Ctrl+Shift+1 starts it, Ctrl+Shift+2 captures. Then reproduce: the films grid scrolled deep while a scan or identification runs; Needs a match; Files. The capture names the function holding the main thread. Its responsiveness track marks the long events in red.
- **Know which screen it was without asking.** A main-thread heartbeat (`setTimeout` or `MessageChannel` every 250 ms, logging gaps over 1 s with the route and the list lengths shown) can be reported through the existing `sendFailuresToCore` path (`main.tsx:79`).
  - **Firefox has no other way to observe it from the page:** it implements neither the Long Tasks API nor Long Animation Frames (MDN compatibility data: both Chrome-only), and a heartbeat works in both browsers.
  - This is DD12, and the owner's standing rule that "digging means missing logs".
- **Reproduce it in CI.**
  - **What the test does:** a Playwright test (`web/e2e` already drives Chrome and Firefox) generates a library of 5,000 titles, scrolls the films grid to the end, makes the server report changes on every poll, and records the longest main-thread gap.
  - **How it measures:** in Chrome, through `PerformanceObserver({type: "long-animation-frame"})` as well; in Firefox, through the heartbeat.
  - It becomes DD8's budget.
- **React-level detail** needs React's profiling build. `<Profiler onRender>` reports `actualDuration` and is "disabled in the production build by default". React 19.2 adds Scheduler tracks to Chrome's performance panel.

## 3. Best practice and expert opinion

### 3.1 Client caching

- **Stale-while-revalidate** began as an HTTP directive: "return a stale response while it
  revalidates it in the background, thereby hiding latency" (RFC 5861). The SWR library and
  TanStack Query made it the default pattern for client data.
- **TanStack Query's design** is the reference for keyed document caches. Its parts:
  - a key per query ("treat the query key like a dependency array");
  - `staleTime`, how long an answer is fresh, which "defaults to 0";
  - `gcTime`, how long an unused answer is kept, 5 minutes;
  - deduplication of identical in-flight requests;
  - refetch on focus and on reconnect;
  - `invalidateQueries`, to mark keys stale after a mutation or a push.

  Its maintainer: the defaults "are geared towards keeping things up-to-date, not to minimize the
  amount of network requests … I like to set [staleTime] to a minimum of 20 seconds to deduplicate
  requests" (TkDodo, *React Query as a State Manager*).
- **Seeding detail from lists.** The cached list can supply the detail screen's first paint
  (TkDodo, *Seeding the Query Cache*). The "pull" approach reads the list's entry as initial
  data; the "push" approach writes each entry under its detail key when the list arrives. Mark
  seeded data with the list's time, so that it is still revalidated. This is how a poster click
  can paint a film's name, year and poster at once.
- **Normalized caches** (Apollo's `InMemoryCache`, Relay's store, urql's Graphcache) store each
  entity once by type and id: "a flat lookup table of objects that can reference each other".
  Every query that includes a film then sees one edit to it. They cost a schema-aware client and
  rules for merging lists. A document cache (TanStack's) stores each answer whole and relies on
  invalidating keys instead.
- **The counter-view.** Dan Abramov, *One Roundtrip Per Navigation* (2025): "client-side caching
  in general is a bit of a red herring … for many navigations — primarily clicking on links —
  the user actually expects to see fresh content". A flash of stale content replaced at once can
  be worse than waiting. Caching also does not fix "N queries for N items" or waterfalls.

**What applies here.**
- **The back button and re-opening a screen should paint from cache at once.** Today a films
  grid starts from "Loading…" at page 1 and loses its scroll position.
- **Leaving a stale cache on screen** is acceptable for this application's data. A poster's
  name or year rarely changes between two looks, and D237 already accepts showing the last
  answer while asking again.
- **A normalized entity cache is more than this needs.** A small keyed cache, seeded from lists,
  invalidated by key, does the job.
- **Whether that cache is ours or TanStack Query is a dependency decision (Q3).**
  - **What exists now:** `openapi-fetch` is allowed and is the only data library (`deps/npm.allow`).
  - **What our own would be:** `useResource` and `Remembered` are most of one already. A store
    keyed by resource path, read through `useSyncExternalStore`, is a few hundred lines with no
    new package.
  - **What TanStack Query would add:** dedupe, focus refetch, garbage collection and devtools,
    already tested, and one more package to audit (D220).

### 3.2 HTTP caching and conditional requests

- **Two patterns** (Jake Archibald, *Caching best practices*):
  - **immutable content with a long `max-age`.** Orpheum's images and built assets already do this.
  - **mutable content, `no-cache`, revalidated every time.** "no-cache doesn't mean 'don't cache',
    it means it must check … with the server". A short `max-age` on mutable content "is a
    baaaad compromise".
- **Validators.** With an `ETag` and `If-None-Match`, an unchanged resource is a 304 with "very
  little data to transfer" (web.dev, *HTTP cache*). RFC 9111 §5.2.2.4 defines `no-cache` as
  "MUST NOT be used … without forwarding it for validation".
- **Browser support.** `immutable` works in Firefox and Safari; Chrome ignores it but honours
  `max-age`. The `stale-while-revalidate` directive is supported by Chrome 75+, Firefox 68+ and
  Safari 14+ (MDN compatibility data). Browsers that do not support it "silently ignore" it.
- **Prior art for images.** Jellyfin and Navidrome both give an image URL that names its content
  `immutable` for a year. Anything else revalidates by `ETag` (Jellyfin's `ImageController`,
  Navidrome's `imghttp/headers.go`). Orpheum's content-addressed art URLs already meet this.

**What applies here.**
- **The cheap and correct step is a validator on every polled JSON resource.** It makes D237's
  polling nearly free: a 304 has no body, and with a revision kept in memory it needs no query
  (§4).
- **Revalidate, never set a TTL.** For JSON this is `Cache-Control: private, no-cache` plus an
  `ETag`. The browser's own HTTP cache then serves `If-None-Match` round trips without any
  client code. A client cache (§3.1) can keep the `ETag` itself to skip even parsing.
- **The WebRTC transport also passes these headers through.** It carries the request's headers
  and the response's status (`rtc-transport.ts:1-9`), so a 304 works on that path too.
  *Inference from the framing; not tested.*

### 3.3 Push invalidation

- TkDodo, *Using WebSockets with React Query*: push **which entities changed**, and invalidate
  their keys. "If we receive an event for an entity that we are not interested in at the moment,
  nothing will happen." Push the data itself only for small partial updates. With every change
  pushed, a long `staleTime` stops polling altogether.
- **This is D166's design applied to screens.** The journal says *what* changed, never what it
  changed to. The client's response is to re-fetch, and "the server may always answer with
  state". A push channel is an accelerator; polling at the longest wait stays as the backstop
  that runs every day (principles §14).
- **The pieces exist.** `github.com/coder/websocket` is allowed and serves Watch Together. A
  `text/event-stream` response needs only the standard library. Either one carries "library
  revision N; changed: titles 12, 40" over the HTTP door and over the WebRTC path, whose
  responses already stream.
  - **The gap:** the client's lint forbids `EventSource` outside the SDK (`web/apps/web/README.md`
    invariants), so it would be an SDK function.
- **D237 §3 chose polling, and the owner decides whether to change that (Q1).** Polling made
  cheap by validators (§3.2) and limited to what is shown (DD5) may be enough at two to five
  people.

### 3.4 Page-shaped fetches: the arguments, the failure modes, and the supple form

**For.**
- **Round trips, not bytes, bound small requests.** "The bottleneck is the network roundtrip
  latency" (Grigorik, *High Performance Browser Networking*). HTTP/2 multiplexing removes the
  cost of connections but not of dependent requests. Abramov: "You can't optimize a
  client/server waterfall if the server doesn't return all the data needed to run requests in
  parallel", and "inherent waterfalls can't be fixed from the client at all — even by
  prefetching".
- **The industry has made this move repeatedly.**
  - **Netflix** collapsed its device calls "into a single request optimized for a given client …
    the device then pays the price of WAN latency once" (Christensen, 2013).
  - **SoundCloud's BFF** made a user profile one resource, after fine-grained endpoints needed "a
    large number of HTTP requests … to render even the simplest experiences" (Calçado, 2015).
  - **Falcor:** "all of the data they need in a single network request".
  - **GraphQL** exists for "fewer roundtrips".
  - **Plex** answers its home screen's hubs from one endpoint (`/hubs`, `/hubs/promoted`).
- **Abramov's *JSX Over The Wire*** argues for screen-shaped endpoints outright
  (`GET /screens/post-details/123`), because a REST resource shaped for every screen becomes "a
  fossilized record of everything they've ever needed". A screen endpoint "gets deleted" with its
  screen.
- **The field's failure is this one.** Jellyfin's web home page makes one request for the user's
  views, then a request per section, including one "Latest" per library
  (`homesections.js`, `recentlyAdded.ts`). Its issues show the per-section endpoints taking 3–30 s
  on large libraries (#15803, #17510, #18369). Its search fired "separate sequential queries for
  each content type" (#6793).

**Against, and the failure modes.**
- **Coupling the API to the UI.** Each screen change is an API change, and the endpoints multiply.
  Sam Newman: "lots of duplication between the BFFs". Merging them into one general aggregating
  API "has proven time and again to lead to highly bloated code". Abramov names the cost himself:
  "a lot more endpoints … How do you reuse code between the endpoints?"
- **Cache granularity.** An aggregate's freshness is its freshest part's: "the aggregated content's
  expiry is as short as the freshest piece of content in the aggregation needs it to be" (Newman).
  A film page that bundles the film with Continue Watching is invalidated by every progress
  report.
- **Over-fetching.** A bundle sends what the screen might show, not what it shows. Re-sent on
  every poll, that is today's deep-list problem with a different shape.
- **Invalidation.** A view model cannot be invalidated by entity. When a film changes, which
  bundles contained it? A normalized client cache, or an aggregate built from resources, answers
  that.
- **Partial failure.** One slow part delays the whole response. Newman advises responses the
  client "can interpret [as] partial".
- **GraphQL's own critics.** Matt Bessey after six years: unbounded queries, N+1s that force
  DataLoader "everywhere", and authorisation per field. For a team that controls its few
  statically typed clients: "You are probably better off exposing an OpenAPI 3.0+ compliant JSON
  REST API". That describes Orpheum.

**The supple form.** Every source that argues for page-shaped fetching also limits it.
- **Relay** composes a screen's query from each component's fragment ("a single network request
  for it and all of its nested fragments").
- **JSON:API** "compound documents" return a primary resource plus `included` related resources,
  each in its normal representation, by an `include` parameter.
- **Remix/React Router loaders** start every route segment's fetch at the route boundary, in
  parallel ("flattened and 3x faster"), instead of letting components fetch as they render.
- **Abramov's own data layer** behind screen endpoints "batch[es] reads and share[s] an in-memory
  cache across different parts of the request".

For Orpheum, that means:
1. **Resources stay the unit of truth.** `/titles`, `/items/{id}`, `/collections/{id}`, `/home`
   and `/me` keep their shapes and their own handlers.
2. **A screen's first paint is one round trip.** Either:
   - **the client starts all of a screen's reads at the route boundary, in parallel**, before
     rendering. The router knows the screen, so it can call its loader. This needs no server
     change, and today's shell already does it for step 1; or
   - **a screen endpoint returns a compound document** of those same representations, keyed by
     their resource paths: `{"/items/42": {…ItemDetail…}, "/home": {…}}`. It is built in-process
     by calling the same functions as the resource handlers, inside **one read transaction (M2)**
     so the parts agree.
3. **The client cache is filled by either.** Each part of a compound response is stored under its
   own key, as if fetched alone. Polling, invalidation and the back button work per resource,
   never per bundle.
4. **The bundle is never a new schema.** A test asserts that every property of a screen response
   is a `$ref` to a resource schema (DD3). The UI's shape decides which resources travel
   together, never what a resource looks like.

**When is the screen endpoint worth having over parallel loaders?**
- **Only when steps are dependent, not parallel.** On HTTP/2 or the WebRTC path, independent
  requests cost one round trip together. The step a bundle removes is a **dependency**:
  - the admin's `/needs-attention` after `/me`;
  - a film page's warm-up after `/items/{id}`;
  - a list's next page after the last.
- **The fix depends on what the dependency is.** Some depend only on the session (who is admin):
  fix those by sending what they need with the first answer (the shell's "boot" read). Others
  depend on data, such as which file a film plays first: a screen endpoint can include those, or
  the server can act on them itself, as D258's warm-up could be started by `GET /items/{id}`.

**The stronger version at Orpheum's size: the catalogue, not the page.**
- **D166 measured the whole browsable state** of the 3,510-row reference library at 847 KB raw,
  **215 KB gzipped, decoded in 8 ms**, and concluded "the server may always answer with state".
  A films grid's data (id, name, year, poster hash, flags) is about 213 bytes an entry. 3,000
  entries is about 630 KB raw and roughly 150–215 KB gzipped, **one response**.
- **What one revision-validated catalogue response per library and person would give:**
  - every list screen, both sorts and search-as-you-type, from memory with **no paging
    requests**;
  - a poll that is one 304 while nothing changed;
  - an instant film page from the seeded entry.
- **Its costs:**
  - **name order must match the server's collation.** The server can send its sort position, or
    the list in name order, so the client never collates;
  - **memory on a phone**, a few megabytes of objects;
  - **reach:** the response is per person's set of libraries (D243 §4);
  - **a 20,000-item household** is 1.2 MB gzipped, about 1 s on 10 Mbit/s. D166 §5 already says
    a screen renders from the server while such a copy fills.
- **This is a change of design, and the owner decides it (Q2).** D166 deferred the journal, not
  this.

**The answer to the owner.** Page-based single fetch is a good idea for **first paint and
dependent steps**, in the composed form above: a screen's reads start together at the route
boundary, a compound response carries resource representations unchanged, and the client cache is
keyed by resource. It is a bad idea as bespoke view models, which couple the API to today's
layout and cannot be invalidated by entity. For Orpheum's list screens the better single fetch is
the library's catalogue, shaped like the data rather than the page.

### 3.5 Batching and prefetching

- **Batching.** DataLoader coalesces the loads "within a single frame of execution". A client
  batch endpoint does the same across requests. On HTTP/2 and the WebRTC path, parallel requests
  already share a round trip, so a batch endpoint buys little over parallel requests. Its only
  gain is per-request overhead: authentication, and opening a channel. A screen endpoint
  (§3.4) is the useful kind of batching.
- **Prefetching on intent.** React Router's `prefetch="intent"` loads "when the user hovers or
  focuses the link", and `"viewport"` is "very useful for mobile". Next.js prefetches links as
  they enter the viewport, in production only. quicklink prefetches in-viewport links "during
  idle time". Speculation Rules target document navigations and "[don't] handle subresource
  prefetches", so they do not fit a single-page app's API reads.
- **For Orpheum:**
  - **hover or focus on a poster:** prefetch `GET /items/{id}` into the cache. The data is
    small, and it saves one round trip on the click, which matters from away.
  - **a series' poster:** prefetch `GET /collections/{id}`.
  - **a list's next page:** fetch it while the person is still a screen above the end, not when
    the end is in view (`end-of-list.tsx:24-35` asks when it intersects).
  - **never prefetch server work.** D258's warm-up (`POST /playback`) starts a preparation and a
    preload, so it stays on the page's opening, as decided.

### 3.6 Windowing, rendering cost, pagination

- **Windowing** "only render[s] what is visible … improves both the rendering and scrolling
  performance" (web.dev, *Virtualize large lists*).
- **`content-visibility: auto`** skips layout and paint off-screen. It is Baseline since
  September 2025, and the web client uses it. **It does not reduce React's work**, which is the
  candidate in §2.2.
- **Three ways to cut React's share:**
  - **stable identities and memoised rows:** reuse the previous entry object when an entry is
    unchanged, and wrap rows in `React.memo`. A poll's change then re-renders only the changed
    rows. This needs no dependency.
  - **windowing:** the poster grid's rows are uniform, so a windowed grid needs no measuring.
    A few dozen lines of our own, or `react-window` or TanStack Virtual (dependencies).
  - **`useDeferredValue` / transitions:** these let React yield, so a long render no longer
    blocks input. That turns one long task into many short ones. It does not shorten the
    work.
- **The React Compiler** memoises automatically. It is a build-time dependency for the owner to
  decide; it does not fix a list replaced wholesale.
- **Keyset pagination** "solves the problem of drifting results" (Winand). The server already
  pages by keyset (sql.md §11). Re-reading every page on each poll throws that benefit away.
- **web-client.md says "Lists are virtualised."** The code does not window. That is a plan
  deviation to record or fix.

### 3.7 Payload size and compression

- **Encodings.** Brotli is in every current browser; zstd is in Chrome 123 and Firefox 126.
  Go's `net/http` compresses nothing on the server side. Its documented transparent gzip is on the
  client's Transport. `compress/gzip` is in the standard library. Brotli and zstd are not:
  `klauspost/compress`'s `gzhttp` would be a dependency, and gzip is enough here.
- **Measured here:**
  - the script bundle is **1.22 MB → 373 KB** with gzip. Assets are immutable, so they can be
    compressed once at startup and kept, or at build time;
  - a 60-title page is **12.8 KB → 3.3 KB**.
- **What it costs today.** From away on a phone link, the first load of a new release moves 3.3×
  the bytes it needs, and every list poll 3.9×.
- **The fix** is gzip `Content-Encoding` with `Vary: Accept-Encoding` on JSON over about 1 KB.
  Leave out byte paths that are already compressed: images, video chunks.

### 3.8 Images

- **Size images to their slot.** MDN: `srcset` with width descriptors, plus `sizes`, lets the
  browser choose an image for the slot and the screen's density.
  - **Today's mismatch:** the grid's tiles are about 108–168 CSS px wide (`tokens.css:83-104`),
    while `portrait-m` is 480 px. At device-pixel-ratio 1 that is about 3–4× the width and 9–16×
    the pixels.
  - **Cost:** each poster decodes to about 1.4 MB of bitmap (480×720×4), whether shown at 120 px
    or not. `portrait-s` (240×360) suits density 1; `-m` suits density 2 and above.
  - *The decode cost was not measured.*
- **Loading:**
  - **lazy-loading** is right for the grid, but not for images "likely to be in-viewport when
    the page loads"; give those `fetchpriority="high"` instead;
  - **`decoding="async"`** keeps the next paint from waiting on a decode;
  - **width and height** avoid layout shift. The posters already have them.
- **The backdrop and stills** have no `loading`, `decoding` or `srcset` (`title.tsx:68-72`,
  `99`). The backdrop is `landscape-l`, 1920 px.

## 4. Server-side tiers: what the core should keep in memory

**SQLite is not the slow tier here.** The queries take well under a millisecond
(performance-targets.md). nas.example's waits were requests queued behind one writer, which D303
removes, and SQLite's page cache plus the OS cache already hold the hot pages. **A cache of
query results would save fractions of a millisecond and add a staleness risk.** Principles §11
and §14 say when that trade is worth it: when the value is expensive to derive, cheap to keep,
and invalidated by a known event.

| Keep in memory | Why | Invalidated by | Worth it |
| --- | --- | --- | --- |
| **Settings** | Read from SQLite on every read today (`internal/core/settings/settings.go:215`), several per request. The owner queued it 2026-10-05. | `Set`/`Reset`, the core being their only writer; their `OnChange` hooks run already | **Yes.** No staleness: the writer is the invalidator. |
| **A library revision**: a counter per library, plus one for the set of libraries | It is the `ETag` of every library-derived resource (DD4). A poll costs a map lookup and a 304, with no query. | A committed write that changes a library's visible data. The writer gate (DL11) is the one place every write passes. A random epoch at start makes old `ETag`s miss after a restart. | **Yes.** It is the mechanism that makes polling cheap. A missed bump leaves a screen stale until the next bump. The fix is the backstop re-read: a full answer at the longest wait, unconditionally (§14: "the path that ignores it is the ordinary one"). |
| **Node names, a node's state** | Read for every list request (`internal/core/api/destinations.go:42-57`) | The node channel's hello, retire and status, events the core already handles | **Yes, small.** |
| **The sort locale and its sorter** | Built per list request (`internal/core/library/browse.go:226-236`) | `sort_locale` changing (a setting) | **Yes, trivial.** |
| **A screen's or the catalogue's encoded, gzipped answer**, per library and reach, at a revision | Saves encoding and compressing 600 KB per poll when the answer changed | The revision changing | **Only if Q2 chooses the catalogue,** and only once measured. Keep it lazy and one at a time per key: a `singleflight`-style guard. `golang.org/x/sync` is not in `deps/go.allow`, so that is a dependency or a few lines of our own. |
| Per-title detail, Continue Watching, search | 0.05 ms queries | — | **No.** Per-person and fast. Bessey: user-specific data "serving stale data is unacceptable". Jellyfin's item LRU (`FastConcurrentLru`, ProcessorCount × 100) exists because its queries are slow; ours are not. |

**The rule:** cache in-process only a value whose invalidating event the core itself causes and
can name, and keep the path without the cache running every day.

## 5. Proposed principles

These are in the style of sql.md §13. Each says what would enforce it. "*(new)*" marks an
enforcement that does not exist yet.

### Round trips and requests

- **DD1 — A screen's first paint costs at most two dependent round trips after the script
  loads**: one for the session's shell, one for the screen, with images excepted. Independent
  reads go out together, started at the route boundary rather than as components render.
  **Enforced by:** a Playwright test per main screen (films, series, a film, a series, play) that
  records request start and end times and fails on a third dependent step *(new)*. Also a
  happy-dom test per screen that counts the requests its mount makes against `test-dom.tsx`'s
  stand-in server *(new)*.
- **DD2 — Screens read through one keyed client cache**:
  - keyed by resource path and query;
  - stale-while-revalidate, so a screen seen before paints at once from the cache;
  - seeded from lists, so a poster's click paints the film's known fields;
  - invalidated by key after a mutation or a push;
  - deduplicating identical requests in flight.

  It replaces `useResource`'s component state and `Remembered` (rule 11: one mechanism).
  **Enforced by:** lint (`no-restricted-syntax` on `api.GET` outside the cache's module and the
  screens' loaders) *(new)*; the cache's own tests *(new)*.
- **DD3 — A response that bundles is composed of resource representations, never a new
  shape.** Every part is the body its own endpoint returns, under that endpoint's key, read in
  one read transaction (M2). **Enforced by:** a `TestArch_` test over the OpenAPI document that
  every property of a screen response is a `$ref` to a resource endpoint's schema *(new)*; the
  cache test that a bundle fills the same keys a direct read does *(new)*.
- **DD4 — Unchanged costs nothing.** Every polled JSON resource carries an `ETag` from the
  in-memory revision, with `Cache-Control: private, no-cache`. A matching `If-None-Match` is a
  304 that runs no query beyond authentication. **Enforced by:** an API test that a second
  conditional `GET` is a 304 and that its `querytally` counts only the session's statements
  *(new)*; a test that a library write changes the `ETag` *(new)*.
- **DD5 — A poll reads what changed, not what was scrolled.**
  - **One check per poll:** at most one conditional request per list per poll, whatever the
    number of pages shown.
  - **On a change:** only the changed entries are replaced, by id. Unchanged entries keep their
    object identity.
  - **Scale:** the client-side twin of DL6 ("nothing a request does grows with the library").

  **Enforced by:** `use-pages.test.ts`, which counts the requests one poll makes with ten pages
  shown *(new)*; a test that an unchanged entry keeps its identity across a poll *(new)*.
- **DD6 — Nobody looking, nobody asking.** A hidden tab does not poll. Shown again, it asks once
  at once and resumes the schedule. Every screen in a tab shares one poll per resource.
  **Enforced by:** `polling.test.ts` with a stubbed `visibilityState` *(new)*.
- **DD7 — Push is an accelerator, never a dependency.** This applies if Q1 adds a change
  channel. The channel carries only what changed (keys and revisions), never data. Polling at the
  longest wait continues beneath it. With the channel down, every screen still converges within
  that wait. **Enforced by:** a test that, with the channel closed, a change reaches the screen
  by the poll *(new, with Q1)*.

### Rendering

- **DD8 — Main-thread work is proportional to what changed and what is on screen.**
  - A list longer than a few screens is windowed, or its rows memoised with stable identities.
  - No main-thread task in the main flows exceeds 200 ms on the reference machine with a
    5,000-title library, scrolled to the end, while a scan changes the list.

  **Enforced by:** a Playwright budget test in Chrome (Long Animation Frames) and Firefox (the
  heartbeat), on the generated library *(new)*.
- **DD9 — Every list the server returns is bounded.** Every list is paged by keyset with a
  `LIMIT`, including Needs a match. **Enforced by:** sql.md §11's plan check extended to every
  query a `GET` handler runs *(new)*; review until then.

### Bytes

- **DD10 — Bytes are compressed and sized to their slot.**
  - **JSON over 1 KB and the built assets** are sent gzipped to a client that accepts it, with
    `Vary: Accept-Encoding`.
  - **Images are chosen by `srcset`/`sizes`** from the sizes D212 makes.
  - **The rest stay as they are:** images and video are never compressed again.

  **Enforced by:** an API test that a `/titles` page with `Accept-Encoding: gzip` comes back
  gzipped *(new)*; a webclient test for an asset *(new)*; a render test that `Poster` names a
  `srcset` *(new)*.
- **DD11 — Prefetch data on intent, never work.** Hover, focus or near-viewport may fetch a
  resource into the cache, a bounded number at a time and cancelled when the person leaves.
  Nothing prefetched starts server work (a playback, a preparation); that remains the page's
  (D258). **Enforced by:** a test that hovering a poster issues exactly one `GET` and no `POST`
  *(new)*.

### Server memory and measurement

- **DD12 — The client reports its own stalls.** A main-thread gap over a second is sent to the
  core with the screen and the sizes of the lists shown, through `sendFailuresToCore`. The next
  "slowing down" names its screen. **Enforced by:** a test of the heartbeat and its report
  *(new)*.
- **DD13 — The core keeps in memory only values whose invalidating event it causes and can
  name** (§4): settings, revisions, node names, the sorter, and optionally the encoded catalogue.
  Each has a test that the event invalidates it. The path without it is the one that runs on every
  start and at every longest-wait poll (principles §11, §14). **Enforced by:** one test per kept
  value *(new)*; review.

### Budgets

| Budget | Value | Checked by |
| --- | --- | --- |
| Dependent round trips to a screen's first paint | ≤ 2 after the script | DD1's Playwright test |
| Requests per poll per list | 1 | DD5's unit test |
| Bytes per unchanged poll | headers only (304) | DD4's API test |
| Longest main-thread task, main flows, 5,000 titles | ≤ 200 ms | DD8's Playwright test |
| A list response's rows | ≤ its `limit` | DD9 |

## 6. Findings, in order of impact on the owner

1. **A list's poll re-reads every page shown, one after another, and re-renders the whole list
   on any change.**
   - **Where:** `use-pages.ts:35-45`, `59-82`, `97-115`, `130-136`; `browse.tsx:118-139`,
     `38-74`; also `files.tsx:135-150`.
   - **Cost:**
     - 3,000 films scrolled to the end is 50 requests and about 630 KB per poll, by the
       synthetic measurement.
     - From away, the 50 sequential round trips alone are about 5 s at 100 ms.
     - While a filing changes the list (every poll under D304), that is a poll every 2 s. That
       is about **1.1 GB an hour per open tab** (630 KB × 1,800), computed, not observed.
     - Each change is one synchronous re-render of every row: the leading candidate for §2's
       warning.
   - **Fix:** DD4, DD5, DD8. A revision `ETag` on the list's first page, so an unchanged poll is
     one 304. Re-read only the visible window on a change, and merge by id with stable
     identities. Memoise rows; window the grid.
2. **Needs a match is unbounded and polled.**
   - **Where:** `internal/store/queries/match.sql:88-104` has no `LIMIT`; `identify.tsx:104-121`
     renders every row and polls.
   - **Cost:** in a fresh or refiling library, thousands of rows mounted and re-rendered: the
     second candidate for §2.
   - **Fix:** DD9 and DD5. Page it by keyset like `/titles`.
3. **No cache between screens.**
   - **Where:** `app.tsx:160-163` keys `Browse` by screen; `usePages` holds its pages in the
     component.
   - **Cost:** back from a film to the grid is "Loading…", page 1, scroll lost. The film page
     shows a remembered copy only because a second mechanism exists (`remembered.ts`).
   - **Fix:** DD2. One keyed cache, list pages included, with the scroll position kept per list
     key.
4. **Nothing is compressed.**
   - **Where:** no `Content-Encoding` anywhere; `internal/core/webclient/webclient.go:65-70`.
   - **Cost:** 1.22 MB of script instead of 373 KB on each new release's first load, and 3.9×
     the bytes on every JSON answer. This matters on a phone away from home.
   - **Fix:** DD10, with `compress/gzip` from the standard library.
5. **Polling never pauses.**
   - **Where:** `polling.ts:71-86`.
   - **Cost:** hidden tabs keep polling, with finding 1's costs, for as long as they are open.
   - **Fix:** DD6.
6. **Cold start reads twice and waits once more than it needs.**
   - **Where:** `app.tsx:131-147` makes a new poller when `/client-settings` arrives, and
     `use-resource.ts:72`, `78` re-read at once, so `/home` is read twice on every load and the
     first answer dropped. `/needs-attention` waits for `/me` (`status-tray.tsx:28-33`,
     `105-110`).
   - **Fix:** DD1. Change the poller's waits without changing its identity, or read
     `/client-settings` before mounting screens. Send the shell's facts (who, admin, waits,
     attention counts) in one read.
7. **Posters are 3–4× wider than their tiles at density 1, with no `srcset`.**
   - **Where:** `poster.tsx:21-27`; `tokens.css:83-104`; `internal/nodeproto/artwork.go:34-36`.
     The backdrop and stills have no `loading` or `decoding` (`title.tsx:68-72`, `99`).
   - **Cost:** decoded-bitmap memory, about 1.4 MB a poster, and bytes on phones away. Not
     measured.
   - **Fix:** DD10's `srcset`/`sizes`.
8. **On the plain-HTTP LAN door, requests share six connections.**
   - **Where:** `cmd/orpheum/serve.go:59-68`.
   - **Cost:** posters queue behind each other and behind polls. The cost is small at LAN
     latency.
   - **Fix:** none needed if the LAN page uses the HTTPS door, which gets HTTP/2. Otherwise
     accept it. Noted so it is not mistaken for a server problem.
9. **Per-request reads that could be in memory.**
   - **Where:** settings (`internal/core/settings/settings.go:215`), node names
     (`internal/core/api/destinations.go:42-57`), the sorter (`internal/core/library/browse.go:226-236`).
   - **Cost:** small per request, multiplied by finding 1's polls.
   - **Fix:** DD13, the owner's queued "settings held in memory" first.
10. **No field measurement of the client.**
    - **Where:** nothing reports a stalled main thread.
    - **Cost:** the owner's warning cannot be tied to a screen without a profile taken in the
      act.
    - **Fix:** DD12.
11. **web-client.md says lists are virtualised; they are not.**
    - **Fix:** record it in plan-deviations.md, or fix it with DD8.

Findings 1, 2 and 5 are the likely substance of the Firefox warning and of most avoidable
traffic. Findings 3 and 6 are what the owner feels on every navigation. Finding 4 is what the
owner feels away from home.

## 7. Questions for the owner

**Q1 — Should the server tell open screens that something changed?**
- **The gap:** today the client keeps asking (D237), because a push channel was more machinery
  than the problem needed. The server always knows when it changed a library: it made the write.
- **The choice:**
  - **Push:** a change channel, carrying only "these keys changed" with polling kept beneath it
    as the backstop (DD7). Screens update within a second, and idle tabs cost nothing.
  - **Keep polling, made cheap:** 304s (DD4), only what is shown (DD5), nothing while hidden
    (DD6). Updates arrive within 2–30 s, and there is no new channel to keep alive across the
    WebRTC path.
- **The cost of push** is a long-lived response per tab, through the relay too.

**Q2 — Should a library's browse index live in the browser?**
- **The gap:** today lists are read from the server a page at a time, and every view is a
  request. At a few thousand titles the whole index is one ~150–215 KB gzipped response (D166's
  measurement).
- **What holding it would give:** every list, both sorts and search-as-you-type answered in the
  browser. One 304 per poll. A film's page painted from its entry at once.
- **What it would cost:** a design change to how lists work. Memory on phones. A per-person
  answer (reach). A slower first look for a 20,000-item household, about 1 s on 10 Mbit/s,
  rendering from the server meanwhile as D166 §5 says.
- **This is the "single fetch" the owner asked about, shaped like the data rather than the
  page.**

**Q3 — The client cache: ours, or TanStack Query?**
- **What it is:** a dependency decision (rule 3).
- **Ours:** grows from `useResource` and `Remembered` (a few hundred lines). No new package, but
  dedupe, focus refetch, garbage collection and devtools are ours to write and test.
- **TanStack Query** (`@tanstack/react-query`) brings all of those, tested by a large community.
  It costs one more package to audit before release (D220).
- **Not needed for either:** neither needs a normalized cache.

**Q4 — Long lists: our own windowing, a library, or memoised rows only?**
- **What it is:** a dependency decision only if a library is chosen.
- **Memoised rows with stable identities** cost nothing and may be enough with
  `content-visibility`.
- **A windowed grid of our own** is small because the rows are uniform.
- **`react-window` or TanStack Virtual** are dependencies that handle the edge cases: keyboard
  focus moving off-window, restoring the scroll position, variable heights.
- **What it depends on:** DD8's measurement, once built, says whether memoising suffices.

## Sources

Fetched pages are cached under `/yoloai/cache/research-data-delivery/`; Jellyfin and
jellyfin-web clones under `/yoloai/cache/repos/`.

**Firefox's warning and profiling**
- Firefox source, `browser/locales/en-US/chrome/browser/browser.properties` (`processHang.*`): https://raw.githubusercontent.com/mozilla-firefox/firefox/main/browser/locales/en-US/chrome/browser/browser.properties
- `modules/libpref/init/StaticPrefList.yaml` (`dom.max_script_run_time`, `…require_critical_input`, `dom.ipc.processHangMonitor`): https://raw.githubusercontent.com/mozilla-firefox/firefox/main/modules/libpref/init/StaticPrefList.yaml
- `js/xpconnect/src/XPCJSContext.cpp` (the interrupt callback): https://raw.githubusercontent.com/mozilla-firefox/firefox/main/js/xpconnect/src/XPCJSContext.cpp
- `browser/modules/ProcessHangMonitor.sys.mjs`: https://raw.githubusercontent.com/mozilla-firefox/firefox/main/browser/modules/ProcessHangMonitor.sys.mjs
- Firefox Profiler docs: https://github.com/firefox-devtools/profiler/blob/main/docs-user/guide-getting-started.md, https://github.com/firefox-devtools/profiler/blob/main/docs-user/guide-ui-tour-timeline.md
- MDN, `PerformanceLongTaskTiming` and its compatibility data: https://developer.mozilla.org/en-US/docs/Web/API/PerformanceLongTaskTiming, https://raw.githubusercontent.com/mdn/browser-compat-data/main/api/PerformanceLongTaskTiming.json, https://raw.githubusercontent.com/mdn/browser-compat-data/main/api/PerformanceLongAnimationFrameTiming.json
- React `<Profiler>`: https://react.dev/reference/react/Profiler; React 19.2: https://react.dev/blog/2025/10/01/react-19-2

**Client caching**
- RFC 5861: https://datatracker.ietf.org/doc/html/rfc5861
- SWR: https://swr.vercel.app/
- TanStack Query, important defaults and caching: https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults, https://tanstack.com/query/latest/docs/framework/react/guides/caching
- TkDodo: https://tkdodo.eu/blog/react-query-as-a-state-manager, https://tkdodo.eu/blog/practical-react-query, https://tkdodo.eu/blog/effective-react-query-keys, https://tkdodo.eu/blog/seeding-the-query-cache, https://tkdodo.eu/blog/using-web-sockets-with-react-query
- Apollo cache: https://www.apollographql.com/docs/react/caching/overview; urql Graphcache: https://github.com/urql-graphql/urql/blob/main/docs/graphcache/normalized-caching.md

**HTTP caching**
- RFC 9111: https://www.rfc-editor.org/rfc/rfc9111.html
- Jake Archibald, *Caching best practices & max-age gotchas* (2016): https://jakearchibald.com/2016/caching-best-practices/
- web.dev, *HTTP cache*: https://web.dev/articles/http-cache; *stale-while-revalidate*: https://web.dev/articles/stale-while-revalidate
- MDN `Cache-Control`, `Vary`, and compatibility data: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control, https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Vary, https://raw.githubusercontent.com/mdn/browser-compat-data/main/http/headers/Cache-Control.json

**Page-shaped and resource-shaped APIs**
- Dan Abramov, *JSX Over The Wire* (2025): https://overreacted.io/jsx-over-the-wire/; *Impossible Components*: https://overreacted.io/impossible-components/; *One Roundtrip Per Navigation*: https://overreacted.io/one-roundtrip-per-navigation/
- Sam Newman, *Backends For Frontends*: https://samnewman.io/patterns/architectural/bff/
- Phil Calçado, *The Back-end for Front-end Pattern*: https://philcalcado.com/2015/09/18/the_back_end_for_front_end_pattern_bff.html
- Ben Christensen, *Optimizing the Netflix API* (2013): http://benjchristensen.com/2013/05/01/optimizing-the-netflix-api/; InfoQ: https://www.infoq.com/news/2013/02/netflix-api-optimization/; Falcor: https://netflix.github.io/falcor/starter/what-is-falcor.html
- GraphQL: https://graphql.org/, https://graphql.org/faq/; Matt Bessey, *Why, after 6 years, I'm over GraphQL* (2024): https://bessey.dev/blog/2024/05/24/why-im-over-graphql/
- JSON:API, compound documents: https://jsonapi.org/format/
- Remix, *Remixing React Router*: https://remix.run/blog/remixing-react-router; React (legacy), Suspense and fetching strategies: https://legacy.reactjs.org/docs/concurrent-mode-suspense.html; Next.js data fetching: https://nextjs.org/docs/app/getting-started/fetching-data
- Relay, *Thinking in Relay*: https://relay.dev/docs/principles-and-architecture/thinking-in-relay/

**Batching, networking, prefetching**
- DataLoader: https://github.com/graphql/dataloader
- Ilya Grigorik, *High Performance Browser Networking*: https://hpbn.co/primer-on-web-performance/, https://hpbn.co/http2/
- React Router `<Link prefetch>`: https://reactrouter.com/api/components/Link; Next.js `<Link>`: https://nextjs.org/docs/app/api-reference/components/link; Speculation Rules: https://developer.mozilla.org/en-US/docs/Web/API/Speculation_Rules_API; quicklink: https://github.com/GoogleChromeLabs/quicklink
- Go `net/http` (HTTP/2 with HTTPS; client-side gzip): https://pkg.go.dev/net/http

**Rendering, pagination, compression, images**
- web.dev, *Virtualize large lists*: https://web.dev/articles/virtualize-long-lists-react-window; *content-visibility*: https://web.dev/articles/content-visibility
- React `useDeferredValue`: https://react.dev/reference/react/useDeferredValue; React Compiler: https://react.dev/learn/react-compiler/introduction
- Markus Winand, *No offset*: https://use-the-index-luke.com/no-offset
- MDN `Content-Encoding` compatibility data: https://raw.githubusercontent.com/mdn/browser-compat-data/main/http/headers/Content-Encoding.json; `gzhttp`: https://github.com/klauspost/compress/tree/master/gzhttp
- MDN `<img>`: https://developer.mozilla.org/en-US/docs/Web/HTML/Element/img; web.dev lazy-loading, fetch priority, CLS: https://web.dev/articles/browser-level-image-lazy-loading, https://web.dev/articles/fetch-priority, https://web.dev/articles/optimize-cls

**Comparable projects**
- Jellyfin `ImageController.cs`, `LibraryManager.cs`: https://github.com/jellyfin/jellyfin/blob/master/Jellyfin.Api/Controllers/ImageController.cs, https://github.com/jellyfin/jellyfin/blob/master/Emby.Server.Implementations/Library/LibraryManager.cs
- jellyfin-web home sections: https://github.com/jellyfin/jellyfin-web/tree/master/src/components/homesections
- Jellyfin PR #15803, issues #17510, #18369, #15742, #15090, #17871; jellyfin-web #6793, PR #7610: https://github.com/jellyfin/jellyfin/pull/15803, https://github.com/jellyfin/jellyfin/issues/17510, https://github.com/jellyfin/jellyfin/issues/18369, https://github.com/jellyfin/jellyfin/issues/15742, https://github.com/jellyfin/jellyfin/issues/15090, https://github.com/jellyfin/jellyfin/issues/17871, https://github.com/jellyfin/jellyfin-web/issues/6793, https://github.com/jellyfin/jellyfin-web/pull/7610
- Plex Media Server API (`/hubs`): https://developer.plex.tv/pms/
- Navidrome `server/imghttp/headers.go`; `singleflight` in `server/jellyfin/api.go`: https://github.com/navidrome/navidrome/blob/master/server/imghttp/headers.go, https://pkg.go.dev/golang.org/x/sync/singleflight
- Phil Karlton via Martin Fowler, *TwoHardThings*: https://martinfowler.com/bliki/TwoHardThings.html

**In this repository**
- [Database access audit](database-access-audit-2026-10-15.md), [data-layer practice](data-layer-practice-2026-10-15.md), [independent audit](data-layer-independent-audit-2026-10-15.md); [sql.md §13](../../standards/sql.md); [principles §11, §14](../../principles/README.md); [performance-targets.md](../plans/performance-targets.md); [web-client.md](../plans/web-client.md); decisions D166, D237, D243, D258, D260, D300, D303, D304 in [working-notes.md](../../decisions/working-notes.md).
