# @orpheum/web

The web client: the browser and phone interface, served embedded by the core. MPL-2.0.

## Invariants

- **Imports packages by name, never another app.** `app-not-to-other-app` and
  `not-into-another-package-by-path` (`web/.dependency-cruiser.cjs`).
- **Server calls go through `@orpheum/sdk`.** `no-restricted-globals` (`fetch`,
  `WebSocket`, `EventSource`, `XMLHttpRequest`).
- **Every focusable element has an accessible name** (D188). `jsx-a11y-x` strict plus
  `control-has-associated-label`.
- **No user-visible text written inline in JSX** (D51). `no-restricted-syntax` on `JSXText`,
  on a literal with letters in a JSX child expression, and on literal
  `aria-label`/`title`/`alt`/`placeholder`. **Every key named as `t("key")` has English text**:
  `src/locales/catalogue.test.ts`.
- **A failure is told in the viewer's words, chosen by the server's code** (cut-a §5.2): a
  screen shows `problemText`'s sentence, never the server's message; an outside provider's
  refusal is told by its reason (`providerProblemText`), with who can fix it.
  `no-restricted-syntax` on `error.error.message`; `src/screen-refusals.test.tsx`.
- **The shell renders a banner with the product name and the menu, and one `main` landmark;**
  an admin's destinations show to an admin only (the server refuses them to anyone else).
  `src/app-shell.test.tsx`.
- **Every screen is a pure view of its data, tested by rendering it**; the loading around it is
  thin. `src/*.test.tsx`. What a screen does when clicked is tested in happy-dom against a
  stand-in server passed to the SDK as its fetch (`src/test-dom.tsx`), never the network.
- **A device not yet asked how it makes sound is asked before it plays**, and the decision is
  asked for again with the answer; it can be changed under the video and in Your devices, and
  the dialogue boost starts as the person left it (D222). `src/sound.test.tsx`.
- **The session token travels in the Authorization header**; images load through the media
  cookie sign-in sets (D154), by addresses from `artUrl` in `@orpheum/sdk`.
- **Lists have no pagination to click through** (web-client.md): the next page loads as the
  end comes into view, with a button that always asks too. `src/end-of-list.tsx`.
- **About credits TMDB with its logo and exact notice, and names what we are built on**
  (D213 §7). `src/about.test.tsx`.
- **Focus only adds** (D189): ring, lift and modest scale are custom properties in
  `src/tokens.css`. Review-only until the first tile lands; then the geometry and luminance
  tests of [testing-strategy.md](../../../docs/contributors/design/plans/testing-strategy.md) enforce it.

## Read before changing

[D51](../../../docs/contributors/decisions/working-notes.md), [D138](../../../docs/contributors/decisions/working-notes.md),
[D188](../../../docs/contributors/decisions/working-notes.md), [D189](../../../docs/contributors/decisions/working-notes.md),
[D193](../../../docs/contributors/decisions/working-notes.md) §6, [D222](../../../docs/contributors/decisions/working-notes.md);
[web-client.md](../../../docs/contributors/design/plans/web-client.md);
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The frame every screen renders in: `src/app-shell.tsx`; which screen an address names:
  `src/router.ts` and `src/app.tsx`.
- The screens: `browse.tsx` (films and series), `title.tsx`, `series.tsx`, `search.tsx`,
  `identify.tsx` (needs a match), `libraries.tsx` and `create-library.tsx`, `files.tsx` (what
  the scanner saw), `settings.tsx` (keys, passwords and server settings), `jobs.tsx` (the job view), `system.tsx` (the System page: the server's facts, doctor's findings, and each machine's software, sandbox, hardware encoding, disks and recent prepares, D225), `node-confinement.tsx` (a machine's FFmpeg sandbox, on the System page), `about.tsx`,
  `sign-in.tsx`, `play.tsx` (the player), `sound.tsx` (how the device makes sound, the boost),
  `audio-tracks.tsx` (the audio track menu, D230), `subtitles.tsx` (the subtitle menu and offset) and `subtitle-search.tsx` (finding and fetching a
  subtitle a file lacks, from OpenSubtitles, D224).
- Every user-visible string: `src/locales/en.ts`, through react-i18next (D51).
- Colours, sizes, focus effects: `src/tokens.css`, never a component's CSS.
- The mount point: `src/main.tsx` and `index.html`.
