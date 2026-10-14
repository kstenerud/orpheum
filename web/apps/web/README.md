# @orpheum/web

The web client: the browser and phone interface, served embedded by the core. MPL-2.0.

## Invariants

- **What cannot be undone is a `ConfirmButton`** (`src/confirm-button.tsx`, D278 §4): outlined in
  the irreversible red, its first press says what will happen and sends nothing, a second, filled red,
  acts, and "Keep it" puts things back. Deleting, removing, withdrawing, ending, signing out other
  devices and a new link that stops the old one all go through it (the owner, 2026-10-12: "red button +
  confirm action is a general pattern that will occur often"). `confirmed-actions.test.tsx`,
  `your-devices.test.tsx`, `system-waiting.test.tsx`, `watch-together.test.tsx`.
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
- **A Watch Together viewer needs no sign-in and sees none of the app** (D246 §3, §5, §6): their
  page is only the host's playback, black while nothing plays, the speaker question as they join
  unless this browser has the answer, voice, their own volume and Leave; the link's secret leaves
  the address once used. `src/together-viewer.test.tsx`.
- **Voice starts only from Talk**, the press the microphone needs, on the viewer's page and with
  the host's player; then mute, Leave voice and the partner's volume, apart from the film's; a
  microphone that cannot open says why. `src/together-voice.test.tsx`.
- **The host's player leads their session while this tab hosts one, and is as before without
  one**: the film goes on for everyone, and stops as the host leaves the player.
  `src/watch-together.test.tsx`.
- **On a discovery's origin the page reaches its core through the discovery** (webrtc-transport.md
  W4c): the discovery names its client API in the page (`<meta name="orpheum-discovery-client-api">`;
  the client reads no environment, D204); a link is spent by its click, never the page's loading; the
  connection's offers are proven and its answers taken only when the core signed them by the key the
  link's fingerprint names; each core's credentials are kept under its id. `discovery-gate.test.tsx`,
  `session.test.ts`, and the SDK's `discovery.test.ts`.
- **The session token travels in the Authorization header**; images load through the media
  cookie sign-in sets (D154), by addresses from `artUrl` in `@orpheum/sdk`.
- **A screen showing what the server changes in the background asks again** — the lists, a title,
  *Needs a match*, the libraries, the System page, the jobs and the home rows — after 2 s, backing
  off while nothing changes to 30 s, and keeps what it shows meanwhile (D237); the waits are the
  server's, from `/client-settings`, through `useBackgroundChanges`. One way to poll:
  `pollUntilStopped` in `src/polling.ts`, through `useResource` and `usePages`.
  `src/polling.test.ts`, `src/use-resource.test.ts`, `src/use-pages.test.ts`.
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
[D193](../../../docs/contributors/decisions/working-notes.md) §6, [D222](../../../docs/contributors/decisions/working-notes.md),
[D246](../../../docs/contributors/decisions/working-notes.md);
[web-client.md](../../../docs/contributors/design/plans/web-client.md);
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The frame every screen renders in: `src/app-shell.tsx`; which screen an address names:
  `src/router.ts` and `src/app.tsx`.
- The screens: `browse.tsx` (films and series), `title.tsx`, `series.tsx`, `search.tsx`,
  `identify.tsx` (needs a match), `libraries.tsx` and `create-library.tsx` (retiring a folder and deleting a library, each confirmed by `confirm-button.tsx`), `files.tsx` (what
  the scanner saw), `settings.tsx` (keys, passwords and server settings), `jobs.tsx` (the job view), `system.tsx` (the System page: the server's facts, doctor's findings, and each machine's software, sandbox, hardware encoding, disks and recent prepares, D225), `node-confinement.tsx` (a machine's FFmpeg sandbox, on the System page), `about.tsx`,
  `sign-in.tsx`, `play.tsx` (the player), `watching.ts` (the film a tab watches: its page's warm-up
  and letting go of the last film's playbacks, D258), `sound.tsx` (how the device makes sound, the boost),
  `audio-tracks.tsx` (the audio track menu, D230), `watch-together.tsx` (the host's Watch Together
  screen: links with notes, members, ending it), `together-viewer.tsx` (a viewer's page, joined by
  their link), `together-host.tsx` (the host's player in their session), `together-news.tsx` and
  `together-events.ts` (who is here and the named events), `use-together.ts` (the session's
  connection), `together-voice.tsx` (voice's controls), `volume-range.tsx` (every volume slider, which a touch anywhere moves), `together-storage.ts` (the memberships this tab keeps and a viewer's speakers),
  `subtitles.tsx` (the subtitle menu and offset) and `subtitle-search.tsx` (finding and fetching a
  subtitle a file lacks, from OpenSubtitles, D224), `this-device.tsx` (the This device page: its
  name and how it makes sound), `now-playing.tsx` (who is playing what, and stopping it),
  `roles-section.tsx` (roles and their libraries), `system-reachability.tsx` and `system-work.tsx`
  (the System page's reachability check and its queue's health), and `busy-group.tsx` (controls
  held while a request is answered).
- The look of plain elements everywhere: `src/base.css`; the skin's tokens: `src/tokens.css`
  (D251).
- Every user-visible string: `src/locales/en.ts` and its parts (`en-together.ts`,
  `en-devices.ts`), through react-i18next (D51).
- Colours, sizes, focus effects: `src/tokens.css`, never a component's CSS.
- The mount point: `src/main.tsx` and `index.html`.
