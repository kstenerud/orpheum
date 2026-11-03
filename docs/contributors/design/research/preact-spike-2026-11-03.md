# The Preact spike (2026-11-03)

D408 §5 asked for a one-day spike: build the web client with React resolved to `preact/compat`,
and measure what it saves and what breaks, on the build, under the tests and on the Fire TV stick.
It is a measurement, not a migration. The research that proposed it is
[ui-toolkit-2026-11-03.md](ui-toolkit-2026-11-03.md) §7 ("a one-day Preact spike", about 50 KB
gzip estimated). The spike's commits stay on their branch. Only this note goes to `main`.

**Incomplete.** The build, payload and test parts are done. On the device, only React's cold loads
were measured, 4 of them. The Preact build was never deployed to staging, so no Preact device
numbers exist. §5 gives the procedure and scripts for finishing it.

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

### Preact

Not measured. The spike build was never deployed.

## 5. To finish it

The scripts are in the session's scratchpad, not the repo.

| Script | What it does |
| --- | --- |
| `spike-all.mjs LABEL` | The whole procedure: 5 cold loads; 3 rounds of the overlay (5 presses) and the subtitles pop-up (3 presses); 3 rounds of 24 grid moves. It writes `ftv-LABEL.jsonl`. |
| `spike-play.mjs` | Opens The Gold Rush full screen from its title page, as a remote does. |
| `spike-load.mjs` | One cold Home load. |
| `spike-keys.mjs` | One key-latency series. |
| `payload.py` and `map.py` | The tables in §2. |

Steps:

1. Run `spike-all.mjs react` against `main`'s staging. It was running at the time of writing.
2. Deploy the spike branch with `./staging quick`, and run `spike-all.mjs preact`.
3. Put `main` back on staging.

## 6. Recommendation

**Do not move to Preact now.** Revisit it only if the Preact device numbers show a clear gain in
main-thread time on the stick.

- **The transfer saving is real but small.** It is about 61 KB gzip, 23% of a first page's JS. That
  is a quarter of a second at 2 Mbit/s and nothing on the LAN or on a return visit.
- **The cost is not small:**
  - A new test dependency (`preact-render-to-string`), and 29 static-markup test files checked
    against a different serialiser.
  - A permanent `act` shim, because React 19's `act` semantics are not Preact's. The spike needed
    eight extra settle rounds to make 176 tests pass, which says effect timing differs.
  - A flaky focus-restoration browser test under Preact that is not flaky under React. Focus on a
    TV is the product's hardest part.
  - A permanent alias, because react-i18next's CommonJS shim must also be aliased or real React
    loads silently.
  - Every future React-ecosystem package becomes a compat question.
- **What could change this is CPU time, not bytes.** On the stick a cold Home spends about 2.1 s in
  long tasks before its first poster. If Preact cuts that by several hundred milliseconds, the case
  is different. The payload budget and the code-splitting work in D408 attack the same 5 s more
  safely: the warm-up prefetch of the 583 kB play chunk, and how much of `client-main` and `router`
  Home needs.
