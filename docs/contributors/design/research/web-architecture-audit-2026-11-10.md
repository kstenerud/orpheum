# The web client's architecture audit, 2026-11-10

The owner ordered it on 2026-11-10: the TypeScript code had had no architecture audit, the web app
had gained weight, and the Fire TV upstairs felt very sluggish. Three read-only audits, one each for
startup, the cost of each press, and playback, followed the
[efficiency audit of 2026-10-28](web-client-efficiency-audit-2026-10-28.md). Their findings are
ranked below by the expected gain on a slow TV. The costs are estimates from reading the code; none
was measured on the stick, and the device lab should confirm them.

The build measured that night:

- A client start loads 32 chunks before its first screen, about 747 KB of uncompressed script.
- The loader's start loads about 568 KB.
- The largest eager chunks:
  - one shared by the loader and the client (284 KB), which holds react-dom, the WebRTC and
    Watch Together SDK, the modal and input layers, and the QR code;
  - React with every English string (188 KB);
  - `client-main`, every screen not already lazy (108 KB).
- hls.js and the player (584 KB) load with the title page.

Status: **done** is committed, with the commit's subject; **owner** waits on the owner's call;
otherwise the finding is open.

## The cost of each press (interaction)

1. **Done** ("perf(web): an arrow asks whether a control can take focus of the nearest alone").
   - **Before:** each arrow press asked every control in its container whether it could take focus.
     That is several walks up the tree and a style check each, over a thousand tiles once a library
     is scrolled. Where the grid held nothing that way, the press read the grid a second time.
   - **Now:** the press reads boxes alone, and asks only the nearest whether it can take focus. A
     widened search leaves out the scope already searched. Each picks the same control as before.
   - **Open part:** scanning a row or grid outward from the focused tile would leave the off-screen
     rows unread, so `content-visibility` could skip them. It assumes the container is in reading
     order, so it needs an opt-in mark.
2. **Owner: held keys queue up.**
   - **Why:** each repeated keydown runs a whole move, so on a slow device focus keeps moving after
     the key is let go.
   - **The proposal:** skip a repeat while the last move has not yet been drawn.
   - **Why it's the owner's:** it changes how far a held key goes. Finding 1 may make it unneeded.
3. **Done** ("perf(web): the watch for focus as a page draws looks once a frame"). The watch that
   gives focus as a page draws used to look at each change to the tree, for up to 10 s.
4. **Posters are fetched about three times too large.**
   - **Now:** `poster.tsx` asks for `portrait-m` (480 × 720) for 136 px tiles, and has no
     `decoding="async"`. Episode stills are `landscape-s` and give no size.
   - **Proposal:** a `srcSet` of `portrait-s 240w, portrait-m 480w` with a `sizes` that fits the
     tile, `decoding="async"`, and a width and height on stills.
5. **The whole grid re-renders when the range in view changes.** `tile-interest.ts`'s range is
   state in `PosterGrid` (`browse.tsx:76`). Move it into a child.
6. **Episode lists are laid out whole.**
   - **Proposal:** give them `content-visibility: auto`, as the grid has.
   - **Risk:** the focus ring was clipped this way before (Silk, 2026-10-25). Do it with finding 1's
     scan.
7. **Owner: CSS on a weak GPU.**
   - Blurred shadows on every poster (`tokens.css` `--shadow-raised`).
   - A mask over the title page's backdrop.
   - The tray icon's endless pulse on idle pages (D364 §12).
8. **Small items.**
   - `poster-row.tsx` measures its ends for arrows that show only with a mouse.
   - Polls do not pause in a hidden tab.

## Startup

1. **All English strings load on every start.** That is about 130 KB minified. `settings`, `system`,
   `libraries`, `people` and most of `play` and `together` could arrive with their screens through
   `addWords` (`i18n.ts`), about 90 KB. Add a test that eager modules use only eager namespaces.
2. **The loader statically imports both of its roles** (`loader-main.tsx:14-15`). A TV at a client
   origin parses the loader-origin role's ~178 KB of source, which it never runs.
3. **Screens static in `app.tsx` that could be lazy** (`lazyScreen`):
   - About, Account, CreditedPerson, LibraryBrowse, CastingLeft, TogetherLeft, Register, SignIn
     (preload it when signed out), and ThisDevice;
   - with them the QR code and its library. That is about 200 KB of source.
4. **The host's voice loads for everyone** (`together-voice.tsx`). Only a host renders
   `HostVoiceOf`. Load it lazily and keep the provider static (D281 §5).
5. **The shell entry pulls in the discovery and WebRTC SDK** (`main.tsx` → `part-load.ts` →
   `loader-config.ts` → `discovery-start.ts`). `app-update.ts` and `client-reaches.ts` import it for
   a one-line meta read. Under the loader the shell is parsed twice.
6. **The opt-in WebRTC path loads eagerly** (`client-main.tsx:53`). Import it inside the
   `wantsWebrtc` branch (D273 §8).
7. **The loader checks the client in sequence** (`loader-run-client.ts:174`). It fetches the
   manifest, then the refused versions, then the page. The three fetches could start together, with
   the checks kept in their order (D308).
8. **The service worker re-fetches every hashed `/assets/` file in the background on each start**
   (`sw.ts:451`). Those names never change content.
9. **The core's page could preload `client-main`'s chunks** (`modulepreload`), saving a round trip.
10. **`/libraries` is fetched and polled twice** (`app-shell.tsx:511`, `home.tsx:57`).

## Playback

The October audit's fixes are all in place. While a film plays, nothing large costs per frame.

1. **Play from Continue Watching, a series page or Up next decides late.**
   - **Now:** only the title page warms a film. The other three ways in send `POST /playback` only
     after the route, the Play chunk and its first render.
   - **Proposal:** warm on the press, not on focus (D258 §2).
2. **The title page waits for hls.js.**
   - **Why:** `watching.ts` statically imports `@orpheum/player`, so the title and series pages and
     four small helpers' users depend on the 584 KB chunk. The page draws only once hls.js has
     loaded and compiled.
   - **Proposal:** import the player dynamically, and split hls.js from the player's light helpers.
     Keep the preload before Play (`warm-play.ts`, 2026-10-28).
3. **Play is a `lazyScreen`, which always suspends once**, and outside the loader nothing warms it.
   Make it a `preloadableScreen`, preloaded from the title and series pages.
4. **hls.js transmuxes on the main thread.** No `workerPath` is set (the October audit's finding 6).
   It needs the worker served by hash under the loader (D308).
5. **Play renders its whole tree before hls.js asks for the playlist** (`film-player.tsx:703`).
   - **Proposal:** mount the hidden controls (D407 §8) after the start.
   - **Constraint:** keep the start gate's wait.
6. **Play waits up to 3 s for the live connection's casts** before it decides (`cast-to.tsx:479`).
   This is D364 §3 and D383 §2.
7. **Owner: subtitles on a TV.**
   - A `<track>` is added that a TV never shows (D418 §1).
   - JASSUB is used for plain tracks (D217 §4, D254).
8. **Up next re-renders every second for the whole episode** (`up-next.tsx:602`).

Memory: no leaks were found.
