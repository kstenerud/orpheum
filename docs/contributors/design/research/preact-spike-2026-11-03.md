# The Preact spike (2026-11-03)

D408 §5 asked for a one-day spike: build the web client with React resolved to `preact/compat`,
and measure what it saves and what breaks, on the build, under the tests and on the Fire TV stick.
It is a measurement, not a migration. The research that proposed it is
[ui-toolkit-2026-11-03.md](ui-toolkit-2026-11-03.md) §7 ("a one-day Preact spike", about 50 KB
gzip estimated). The spike's commits stay on their branch. Only this note goes to `main`.

**Complete (2026-11-03, the lead, rebased on `main` at "refactor(web): ⁝'s steppers lay out with Cluster; a list's entries take no class").** The first pass measured the
build, the tests and React on the stick. The second deployed the Preact build to staging and measured
it on the Fire TV stick and the Pixel 3a, beside React on the same day, and added
`preact-render-to-string` to the spike branch for the static-markup tests. Sections 2 to 4 hold the
first pass's figures and, after them, the second's; §6 is the result.

## 1. What was built

- **Branch.** The lead's commit adds `preact` 11.1.1 to `web/apps/web` (the lock included). One
  spike commit on top resolves React to `preact/compat` (`apps/web/preact-alias.ts`, used by
  `vite.config.ts`):
  - `react` and `react-dom` go to `preact/compat`, and `react-dom/client` to
    `preact/compat/client`.
  - `react/jsx-runtime` and `react/jsx-dev-runtime` go to Preact's own runtimes, and
    `react-dom/test-utils` to `preact/test-utils`.
  - `use-sync-external-store/shim` goes to `preact/compat`. react-i18next's `useTranslation`
    imports that CommonJS shim, and the shim `require`s React itself. Without this alias, a bundle
    or test run under the alias still loads real React and fails with "useSyncExternalStore of
    null".
- **Tests.** The tsconfig and vitest configurations are gates, so the tests run from two configs of
  the spike's own: `web/vitest-preact.unit.ts` and `web/vitest-preact.browser.ts`. Two shims back
  them:
  - `web/preact-shims/react.ts` adds React 19's `act`, which `preact/compat` lacks. It wraps
    `preact/test-utils`' `act`.
  - `web/preact-shims/react-dom-server.ts` substitutes for `renderToStaticMarkup`.
- **The production build contains no React.** `react.transitional.element` occurs only in the React
  build.

## 2. Payload

The figures come from `npm run build` in `web/apps/web` on `main` ("docs(plans): the owner's comments on the pop-ups, folded into the toolkit batch") and on the spike. Sizes
are in bytes. gzip is level 9, and brotli is Node's default (quality 11).

"First page" means every JS file that Silk on the Fire TV fetched during a cold load of Home, up to
the first poster painted: 19 files, recorded live from React's staging (§4). The Preact column uses
the same chunks matched by name. React's `jsx-runtime` chunk is replaced by Preact's `jsxRuntime`
chunk, which holds Preact's core. Rolldown's runtime chunk has no separate counterpart.

| | React raw | gzip | brotli | Preact raw | gzip | brotli |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Entry (`index-*.js`) | 50,788 | 17,641 | 15,864 | 50,744 | 17,619 | 15,875 |
| `transport-choice` (holds React DOM) | 245,640 | 77,360 | 67,177 | 34,833 | 12,766 | 11,275 |
| `client-main` | 192,062 | 52,963 | 45,491 | 179,830 | 52,960 | n/m |
| `jsx-runtime` / `jsxRuntime` | 13,904 | 5,591 | 4,983 | 25,980 | 10,790 | n/m |
| **First page, all JS** | **822,600** | **262,519** | **229,883** | **604,975** | **201,241** | **177,172** |
| **Saving** | | | | **217,625 (26%)** | **61,278 (23%)** | **52,711 (23%)** |

"n/m": not measured per file; the totals are measured.

- **The entry chunk is unchanged.** It holds the loader, not React.
- **The whole saving is React DOM.** It is about 61 KB gzip, close to the research's estimate of
  about 50 KB.
- **The play chunks are unaffected.** `gain` (583 kB, hls.js and the player) and `router` (177 kB)
  are identical, and React Home prefetches them (`useWarmsPlay`) after the first poster. So the
  saving is 23% of what a first page needs, but a smaller share of what a session fetches: about
  1.7 MB decoded in about 35 files once the warm-up has run.

**On the household's network,** 61 KB of gzip is about 0.05 s at 10 Mbit/s and about 0.25 s at
2 Mbit/s, the slow end of a remote viewer's link. On the home LAN it is negligible. A returning
visit costs nothing either way: the service worker keeps the client's files.

**Rebuilt on today's `main` ("refactor(web): ⁝'s steppers lay out with Cluster; a list's entries take no class").** The JS a cold Home load fetches, gzip: React 517,299,
Preact 454,989, a saving of 62,310 (12%). All the client's JS, gzip: React 584,272, Preact
522,198 (11%); raw 1,895,941 and 1,661,925. The saving is the same 62 KB; the share is smaller
because the first page now counts the loader's and the warm-up's chunks.

## 3. Correctness under compat

### Unit tests (vitest, happy-dom and node)

With React, all 2,160 tests pass. The runs under the alias:

| Stage | Failures | Cause | Fix |
| --- | ---: | --- | --- |
| Alias alone | 825 | `act is not a function` (716): React 19's `act` from `react` is missing in compat. The rest: "Objects are not valid as a React child", because `react-dom/server` is still real React. | The two shims. |
| With the shims | 759 | `useSyncExternalStore` of null: react-i18next pulls in `use-sync-external-store/shim`, which loads real React. | Alias the shim to `preact/compat`. |
| With that alias | 176 | Async settling. React's `act` drains effects started while an awaited scope settles; Preact's flushes once. | The `act` shim runs eight more `act` rounds after an async scope. |
| **Final** | **31 (14 files)** | All static markup (below). | Needs `preact-render-to-string`. |

Every interactive test passes: every `createRoot` plus `act` test, including the ones using
`useSyncExternalStore`, react-i18next, refs as props and `<dialog>`.

All 31 remaining failures use `renderToStaticMarkup`, or `react-dom/static`'s `prerender` in
`lab.test.tsx`. The spike's substitute renders into a happy-dom `div` and reads `innerHTML`, and it
differs from a server renderer:

- It orders attributes differently.
- It writes `value`, `checked` and `selected` as properties, and escapes entities differently.
- It runs effects. In the node environment these throw "addEventListener of undefined" and
  "window is not defined".

These are faults of the harness, not of the client. A real port needs `preact-render-to-string`, a
new dependency and so the owner's decision (AGENTS.md rule 3). It would also need its output checked
against the 29 test files that use static markup.

**Second pass, with `preact-render-to-string` 6.8.2** standing in for `react-dom/server`: 25
failures remain, all the harness's. 21 compare against React's exact serialisation
(`autoComplete` against `autocomplete`, `&#x27;` against `'`, `readOnly=""`, `playsInline=""`,
attribute order); 4 in `lab.test.tsx` use `react-dom/static`'s `prerender`, which has no
counterpart. One more shim was needed: `preact/compat`'s `use(context)` reads the rendering
component's global context, which the server renderer never sets, so every static render of a
component calling `use(SomeContext)` threw ("reading '__cC2'"). The shim answers a context with
`useContext`. A port would carry it, or would replace `use(context)` with `useContext` in the
client.

### Browser tests (vitest browser mode: Chromium, Firefox, WebKit)

With React, 322 of 322 pass. Under the alias the first run could not import 44 files: Vite's
dependency prebundle served real React's `act`-less copy. The fix was to alias `react` to the shim
and exclude React from `optimizeDeps`.

After that, 321 of 322 pass. The one failure was in `input-player.browser.test.tsx` (Firefox: focus
not on Play after leaving full screen). Reruns of that file:

- Under Preact, 6 runs of the file (the full run included): 4 passed. One had the Firefox failure; one had 2 Chromium failures in different tests (Back to the
  film's page; Up to a series' season).
- Under React, 4 runs: all passed.

So this file is flaky under Preact and was not seen failing under React. It tests focus restoration
after full screen and route changes, where Preact's commit and effect timing differs from React's.
The failures were not diagnosed.

**Second pass:** 392 of 392 pass under Preact, the focus test included.

### React 19 features in the research's checklist

| Feature | Result |
| --- | --- |
| `use()`, actions, `useOptimistic` | No failure attributable to them; no test exercising them failed. |
| `<title>` hoisting | No failure attributable to it. |
| Ref as a prop | Passes. |
| `useSyncExternalStore` | Passes once the CommonJS shim is aliasing (above). |
| react-i18next under compat | Passes with the same alias. Without it, real React loads beside Preact. |
| Native `<dialog>` | The dialog tests pass. The subtitles pop-up was not exercised on the device under Preact. |

The desktop browser was not sampled by hand under Preact. The browser tests are the only desktop
evidence.

## 4. Fire TV stick

### Setup

- The device is Silk on the Fire TV stick, driven over CDP and with `adb` key events. The app is
  staging, over HTTPS, signed in as `lead-tests`.
- **Cold load:** the HTTP cache and the origin's Cache Storage are cleared, which drops the files
  the service worker keeps. The worker's registration and the sign-in stay.
- **Measures:**
  - *First poster:* the first Home poster `<img>` that is complete with a natural width.
  - *Interactive:* the later of the first poster and the end of the last long task, after 3 s
    without one.
  - *Heap:* `Runtime.getHeapUsage` after a forced GC.
  - *Key latencies:* from `keydown`'s `timeStamp` to the frame after the signal (rAF then
    `setTimeout` 0). The signals are `focusin` and scroll for the grid, `data-shown` for the
    overlay, and `dialog[open]` for the pop-up.
- **Wake the stick first:** a screensaver stops lazy posters from loading, so each run starts with
  `WAKEUP`.

### React, `main`'s web build (staging served `index-BcdVBGDG.js`, the same as the build in §2)

| Cold load | Run 1 | Run 2 | Run 3 | Run 4 |
| --- | ---: | ---: | ---: | ---: |
| First contentful paint (ms) | 2364 | 2120 | 2300 | 2080 |
| Home rendered (ms) | 4508 | 4378 | 4332 | 4244 |
| First poster (ms) | 5109 | 5136 | 5031 | 5080 |
| Interactive (ms) | 5109 | 5136 | 5180 | 5080 |
| Long tasks before it (ms) | 2071 (12) | 2162 (11) | 2117 (12) | 2123 (11) |
| JS heap used (MB) | 14.9 | 15.3 | 15.6 | n/r |

"n/r": not recorded.

**About 2.1 s of a 5.1 s first poster is long tasks on the main thread**: parsing and running the
JS, and rendering. This is where Preact could matter more than in transfer time, because React DOM
is the largest single parse. It is the measurement still missing.

**React key latencies (trial runs, not counted).** These established the procedure:

- Grid on `/films`, 24 moves (12 Down, 12 Up, 1 s apart): Down median 59 ms, worst 573 ms; Up
  median 75 ms, worst 152 ms.
- Overlay shown by Down while a film plays full screen: the signal arrives 380–930 ms after the
  key, and the frame 550–1450 ms after it. The overlay waits on the player's own work.
- Subtitles pop-up opened by OK, 3 presses: median 329 ms, worst 393 ms. The signal arrives at
  170–245 ms.

### Second pass: React and Preact, the same day

The procedure as above, on `main` at "refactor(web): ⁝'s steppers lay out with Cluster; a list's entries take no class" and the spike rebased on it; medians, 5 cold loads
a run. Staging's loader page is the same older build in every run, so it does not confound them.

| Run (in order) | First poster | Home | FCP | Interactive | Long tasks | Heap |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| React 2 | 4250 | 3528 | 1008 | 5623 | 1455 | 6.9 MB |
| Preact 1 | 4048 | 3483 | 1304 | 5441 | 1708 | 12.3 MB |
| Preact 2 | 4645 | 3965 | 1816 | 5995 | 2184 | 17.2 MB |
| React 3 | 4584 | 3992 | 1660 | 5952 | 2124 | 16.1 MB |
| Preact 3 | 4952 | 4410 | 2288 | 6303 | 2547 | 19.8 MB |

**Run-to-run drift swamps the difference.** Every run on the same Silk session is slower than the
one before, whichever build, and the heap grows with the session. So a last run alternated the
builds with Silk restarted before each block of 5:

| Block (in order, Silk fresh) | First poster (5 loads) | Median | Home | FCP | Interactive | Long tasks | Heap |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Preact A | 4312–4502 | 4461 | 3757 | 1372 | 5890 | 1575 | 6.2 MB |
| React A | 3933–6278 | 4830 | 4135 | 1460 | 6619 | 1902 | 6.9 MB |
| Preact B | 3730–4631 | 3830 | 3108 | 948 | 5134 | 1306 | 6.3 MB |
| React B | 3826–3997 | 3968 | 3277 | 972 | 5299 | 1370 | 6.9 MB |

Within each pair Preact is ahead: 140 to 370 ms on the first poster (3 to 8%), 60 to 330 ms of long
tasks, 0.6 MB of heap. The difference between the pairs (about 600 ms) is larger than within them.

**Keys** (ms; n presses):

| | React 2 | Preact 1 | Preact 3 |
| --- | --- | --- | --- |
| Grid Down | median 58, worst 341 (36) | n/m | median 54, worst 326 (36) |
| Grid Up | median 73, worst 137 (36) | n/m | median 72, worst 153 (36) |
| Overlay (Down, full screen) | median 210, worst 841 (15) | median 162, worst 686 (10) | median 792, worst 2208 (15) |
| Subtitles pop-up (OK) | median 237 (3) | median 215 (2) | median 532 (3) |

The grid is the same. The overlay and the pop-up wait on the player's work while a film plays, and
vary by a factor of 4 between runs of the same build (Preact 1 against 3); they show no difference
that these runs can separate. (The first pass's React overlay, 550–1450 ms on "docs(plans): the owner's comments on the pop-ups, folded into the toolkit batch", was before
today's player changes.)

**The Pixel 3a** (Chrome, the same cold load, 6 loads each): first poster React 1414 ms
(1317–1514), Preact 1363 (1307–1398); Home 1278 and 1202; FCP 392 and 376; long tasks 60 and 51 ms;
heap 7.5 and 5.8 MB.

**A server bug found by the runs, not Preact's.** Both early Preact procedures stopped at the play
step: file 241 stalls for good at 42:13 on staging, whichever client, because FFmpeg cannot cut
its chunk 349 (filed in [toolkit.md](../plans/toolkit.md)'s next batch). The runs resume where the
last stopped, so only the later runs reached it. Moving the resume point back let Preact 3 run.

## 5. To finish it

Nothing remains. The scripts (the scratchpad's `spike-*.mjs`, `abab.sh`, `payload.py`) were the
session's; the branch `spike/preact-on-main` holds the alias, the shims and the second-pass changes.

## 6. Recommendation

**Do not move to Preact now.** The device numbers are in, and they show a small, real gain, not a
clear one.

- **Bytes:** 62 KB gzip, 11 to 12% of the client's JS. A quarter of a second at 2 Mbit/s; nothing
  on the LAN or on a return visit.
- **CPU on the stick:** 3 to 8% off the first poster when the builds alternate on a fresh Silk,
  about 0.6 MB less heap, and the same grid. Silk's own drift between two such pairs is larger than
  the gain. The phone shows the same small lead.
- **The cost is unchanged from the first pass** (a new dependency, a permanent `act` shim and now
  a `use` shim or the client's `use(context)` rewritten, a permanent alias for react-i18next's
  shim, every React-ecosystem package a compat question), less the browser test's flakiness, which
  did not recur on today's `main`.
- **The same 0.2 to 0.4 s is more safely won** by D408's payload work: the first page's chunks, the
  warm-up's timing, and how much of `client-main` and `router` Home needs. Revisit Preact if that
  work is done and the first poster on the stick is still the complaint.

**The owner's decision (2026-11-03):** stay on React, "since the gains are so small". A React
Native app is planned eventually, which React keeps closer at hand than Preact would.
