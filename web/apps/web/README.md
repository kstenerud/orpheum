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
  an admin's destinations show to an admin only (the server refuses them to anyone else); Device
  tests, beside Watch Together, shows to everyone while the server offers them
  (`should_offer_device_tests`). `src/app-shell.test.tsx`, `src/lab.test.tsx`.
- **Every screen is a pure view of its data, tested by rendering it**; the loading around it is
  thin. `src/*.test.tsx`. What a screen does when clicked is tested in happy-dom against a
  stand-in server passed to the SDK as its fetch (`src/test-dom.tsx`), never the network.
- **A list's poll reads at most two pages, those in view** (D306 §7): pages scrolled past keep what
  they showed until they are in view again, each page is read again by its own cursor, an entry read
  twice shows once, and an unchanged entry stays the object shown, so a memoised row is not drawn
  again. A series sends its seasons and a season its episodes a page at a time (D306 §9).
  `src/use-pages.test.ts`, `src/items-in-view.test.ts`, `src/series.test.tsx`.
- **No one is asked how their device makes sound** (D336 §4): a film plays at once, and the
  dialogue boost starts as the person left it and switches the mix at once, under a prepared copy
  and under a film on its ladder alike (D222); over the original, where its track has a centre, it
  starts off, and turned on it is kept and the film asked for again from where the video stands,
  with the cause boostChosen (D336 §3). `src/sound.test.tsx`.
- **A Watch Together viewer needs no sign-in and sees none of the app** (D246 §3, §5, §6): their
  page is only the host's playback, black while nothing plays, with nothing asked of their speakers
  (D336 §4), voice, their own volume and Leave; the link's secret leaves
  the address once used. `src/together-viewer.test.tsx`.
- **A person casts only to their own devices open now, and the receiver shows the film and nothing
  else** (D337; plans/cast-to-device.md): every signed-in page holds a live connection
  (`src/live.tsx`), and a cast handed to it opens the film's Play page, which leads the cast's session
  of one from where the sender stood, fills the window and shows only a fading *Controlled from*
  line; its media keys and seeks stay its own; any other key, a tap or Back (a Fire TV's history
  step) shows *Take back control*, and a second takes it; stopped by the remote it pauses, replaced it
  goes on, at the film's end it goes home. *Cast to another device* lists the person's other devices
  open now, by name and last use, and is not offered while the tab hosts Watch Together; the remote
  (`/casting`) holds no player, commands the receiver, and once the cast is over says why with *Play
  here from*. `src/cast-to.test.tsx`, `src/cast-receiver.test.tsx`, `src/cast-remote.test.tsx`; on
  devices, VQ64–VQ66.
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
  link's fingerprint names; each core's credentials are kept under its id. A core that turns the
  device away for its network's share (rtcapi's `Verdict`) is said so, with a way to try again,
  never as a network that cannot reach it, and a remembered one stays remembered. Every WebRTC
  transport a page used is closed with its path, when its hand-off fails, and when the page leaves
  for good. `discovery-gate.test.tsx`, `kept-path.test.ts`, `loader-pages.test.tsx`,
  `session.test.ts`, and the SDK's `discovery.test.ts`.
- **At a discovery the page is its loader, which runs the core's own client** (D308): `app.` keeps
  no core's session, hands a link on to its core's client origin, and hands a realm sign-in on by a
  single-use code; a client origin is bound to the core its name names and opens nothing for
  another; the core's client runs only once its manifest holds under the key the link, the hand-off
  or this origin named, its version is not one the discovery refuses and its page is the one it
  signed; and the loader clears every cookie it can see. `loader-config.test.ts`,
  `loader-households.test.ts`, `loader-pages.test.tsx`, `loader-run-client.test.ts`,
  `loader-sweep.test.ts`, and the SDK's `discovery.test.ts`.
- **A TV's code is activated by its QR code with one tap, or typed with a committed check code**
  (D321): the TV shows a QR code of the loader's `/activate` carrying its key's hash; `/activate`
  goes to the one realm's Activate a device, asks which of several, or offers the link to copy or a
  sign-in that carries on there; a scanned code is entered at once and answered by one tap, with no
  check code; a typed one's check code is made of the key, the nonce the TV committed to and the
  core's. `loader-device-code.test.tsx`, `loader-pages.test.tsx`, `device-activation.test.tsx`, and
  the SDK's `device-code.test.ts`.
- **The session token travels in the Authorization header**; images load through the media
  cookie sign-in sets (D154), by addresses from `artUrl` in `@orpheum/sdk`.
- **A poster or still whose image does not load shows its blank and asks for it again** on the
  background changes' schedule (D237), each time by a new address, until it loads or leaves the page,
  so art the server makes after the page was drawn shows with no reload (the owner, 2026-10-25).
  `src/poster.test.tsx`.
- **A screen showing what the server changes in the background asks again** — the lists, a title,
  *Needs a match*, the libraries, the System page, the jobs and the home rows — after 2 s, backing
  off while nothing changes to 30 s, and keeps what it shows meanwhile (D237); the waits are the
  server's, from `/client-settings`, through `useBackgroundChanges`. An ask with no answer within
  `askAnswersWithinMs` (60 s) is given up as failed, aborted by its signal and reported, and the poll
  asks again after the failed wait, so one request that never answers cannot stop it for good (the
  owner, 2026-10-24). One way to poll:
  `pollUntilStopped` in `src/polling.ts`, through `useResource` and `usePages`.
  `src/polling.test.ts`, `src/use-resource.test.ts`, `src/use-pages.test.ts`.
- **A title's and a series' page have their own Back at the top** (the owner, 2026-10-24: a TV's
  browser keeps its back out of the remote's reach): the page before when it was this app's, else
  up in place of the current one: an episode to its season, a season to its series, a film, an
  extra or a series to its list. `src/back-button.test.tsx`, `src/navigation.test.ts`.
- **A series' own page offers its person's next episode at once** (the owner, 2026-10-25): the
  server's `nextUp`, played by *Continue S1E3* where they are partway through it, which resumes, else
  *Play S1E4*; where it cannot be played now, why, and no Play, as a title's page does; none on a
  season's page or when there is no next one. `src/series.test.tsx`.
- **A series is shown in the ordering its person chose** (D159 §1, episode-orderings.md §4): a series
  with more than one offers them on its top page, aired first, each by its label with its kind and
  how many episodes it holds; a choice is kept on the server and goes to the series' top page, where
  `?season=N` is a group of the ordering in use (§7 Q10). Its seasons are that ordering's groups,
  headed by their names ("Disc 1", else "Part 2"); Next Up, an episode's page and Continue Watching
  label an episode there ("Disc 1 · 2", "Episode 812"), its page with its aired place on a second
  line; what the ordering leaves out is listed under *Not in this order* (§7 Q4, Q8). Labels are
  `src/episode-place.ts`'s. `src/episode-orderings.test.tsx`.
- **Lists have no pagination to click through** (web-client.md): the next page loads as the
  end comes into view, with a button that always asks too. `src/end-of-list.tsx`.
- **A list by name's alphabet rail moves it to a letter** (the owner, 2026-10-25): the server names
  the first title at or after the letter in its own order (`/titles?from=`), # the first of all; the
  list loads its pages up to it as scrolling would, and its poster takes focus at the view's top,
  where no title comes after the letter the last. `src/alphabet-rail.test.tsx`.
- **About credits TMDB with its logo and exact notice, and names what we are built on**
  (D213 §7). `src/about.test.tsx`.
- **The device lab is at `/lab`, each probe a page of its own** (plans/tv-device-tests.md), offered
  only while the server's `should_offer_device_tests` is on (the owner, 2026-10-25): off, every lab
  page says so in place of its probe, the test film excepted; a menu
  of big links a D-pad's own browser navigation reaches; each probe shows what it found in large
  type and sends it to the core's log through the SDK, showing the name the log keeps it by (the
  button test as it goes, below); the
  cast probe opens by its own load, the one page the core lets load Google's Cast SDK; the extend
  probe's test film needs no sign-in. The input probe, the button test, lights a chip for each
  button heard, whether as a key, a step back, a controller's button or stick or a Media Session
  action (`src/input-buttons.ts`), and fits a Fire TV's 960 × 540 in the shell unscrolled, a
  controller of unknown layout connected too (the owner, 2026-10-25); it has no Send: each press (a
  key going down, not up), each move of focus with its latency and each change of who moves it goes
  to the core's log as it happens, one request on its way at most and what came meanwhile sent
  together after it, what waits sent kept alive as the page hides or leaves, under the run's name
  the page makes as it loads and shows (`src/lab-events.ts`; the owner, 2026-10-25); its quiet
  tone, for a keyboard's media keys alone, is off until switched on in its details; its focus
  engine sits behind `FocusEngine` (`src/focus-engine.ts`), so another can be measured beside ours,
  and ours moves across the whole app, the shell's title bar included, among what is laid out, and
  leaves an arrow to a text field but at the value's end it points to (Up and Down always leave one
  line) or while it composes. Its text field sends its focus, input and composition with the
  value's length, never its text, and a character typed in it as `Character` (`src/lab-field.tsx`,
  H6); its full-screen test puts a video drawn in the page full screen by its box or bare, leaves
  at Back or Escape and sends what happened (`src/lab-fullscreen.tsx`, H7); its controller line
  shows the first pad's name, mapping and the layout its id says (`src/input-pads.ts`: by vendor, else
  by name; Nintendo's OK is the right face button, Xbox's and PlayStation's the bottom one), offers
  the choice of OK, kept as the This device page keeps it, and sends a pad with no standard
  mapping's buttons as numbers and its axes as they cross half way (`src/lab-pad-input.tsx`, H8);
  the app's navigation moves by a standard pad there as on every page, and the test keeps Back
  rather than going back by it. `src/lab.test.tsx`,
  `src/lab-input.test.tsx`, `src/lab-input-field.test.tsx`, `src/lab-pad-input.test.tsx`,
  `src/input-pads.test.ts`, `src/lab-events.test.ts`, `src/lab-input.browser.test.tsx`,
  `src/lab-buttons.test.ts`, `src/input-buttons.test.ts`, `src/router.test.ts`, `src/focus-engine.test.ts`,
  `src/lab-timing.test.ts`, `src/lab-keys.test.ts`,
  `src/lab-extend-channel.test.ts`, `src/lab-tone.test.ts`; on devices, VQ55–VQ60.
- **Every page is driven by a remote, a keyboard or a game controller**
  (controller-input-2026-10-25.md §6.1; the owner, 2026-10-25): one input layer
  (`src/input-commands.ts`, run around every screen by `src/input-layer.tsx`) turns a key (by its
  `key`, then the platforms' keyCodes), a controller's buttons, D-pad and left stick
  (`src/input-pad-poll.ts`, its OK by its make or as chosen on This device) and the browser's media
  seeking into commands; pages hear commands, never keys, and single-letter shortcuts have none. A
  command is offered first at what has focus, so an open menu or dialog answers Back before
  anything else; one no one answers moves real DOM focus by the geometric engine
  (`src/focus-navigation.ts`, `src/focus-engine.ts`), searching the marked container around focus
  before the page (`data-focus-container`: the banner, its menu, a row or grid, the player), presses
  what has focus (a list's choices opened by a key, stepped through by a controller's OK, which
  cannot work a browser's own list), or goes back as the page's own Back does
  (`src/back-button.tsx`). An arrow inside a
  text field's value or a range's is the field's; one with nowhere to go is the browser's. The
  first arrow starts in the main content, not the banner; an arrow into a container or the main
  content goes back to what last had focus there; the focused element is scrolled into view no
  further than it must be, clear of the sticky banner, and Up or Down over more than half a screen
  of text scrolls through it a step at a time before the control past it takes focus; a
  controller's move is shown as a key's is (`:focus-visible`), a pointer's never; Back returns focus to what had it on the page left, noted
  on its history entry as the page was left and found once the page draws it (`src/focus-memory.ts`),
  Silk's Back too. The player answers over the browser's own controls (`src/player-commands.ts`):
  play/pause wherever focus is, OK on the video, a 10 s seek by the seeking buttons, the browser's
  media controls and Left and Right while the video or nothing has focus, and Back leaving full
  screen before the page; every control under the video is reached by arrows.
  `src/player-commands.test.tsx`, `src/input-player.browser.test.tsx`,
  `src/input-commands.test.tsx`, `src/input-layer.test.tsx`,
  `src/focus-memory.test.ts`, `src/focus-engine.test.ts`, `src/input-buttons.test.ts`,
  `src/input-pads.test.ts`, `src/navigation.test.ts`, `src/app-shell.test.tsx`,
  `src/modal-dialog.test.tsx`, `src/modal-dialog.browser.test.tsx`, and the walks of every page
  in Chromium and Firefox at Silk's 960 × 480 and a desktop's size, `src/input-walk.browser.test.tsx`,
  `src/input-rows.browser.test.tsx`, `src/input-pages.browser.test.tsx`,
  `src/focus-navigation.browser.test.tsx`; on devices, VQ61 and VQ62.
  A row that scrolls sideways keeps room for the focus ring (`--focus-ring-room` in
  `src/tokens.css`), which it would clip.
- **Focus only adds** (D189): ring, lift and modest scale are custom properties in
  `src/tokens.css`. Review-only until the first tile lands; then the geometry and luminance
  tests of [testing-strategy.md](../../../docs/contributors/design/plans/testing-strategy.md) enforce it.

## Read before changing

[D51](../../../docs/contributors/decisions/working-notes.md), [D138](../../../docs/contributors/decisions/working-notes.md),
[D188](../../../docs/contributors/decisions/working-notes.md), [D189](../../../docs/contributors/decisions/working-notes.md),
[D193](../../../docs/contributors/decisions/working-notes.md) §6, [D222](../../../docs/contributors/decisions/working-notes.md),
[D246](../../../docs/contributors/decisions/working-notes.md), [D336](../../../docs/contributors/decisions/working-notes.md);
[web-client.md](../../../docs/contributors/design/plans/web-client.md);
[standards/typescript.md](../../../docs/contributors/standards/typescript.md).

## Where to change

- The frame every screen renders in: `src/app-shell.tsx`; which screen an address names:
  `src/router.ts` and `src/app.tsx`.
- The screens: `browse.tsx` (films and series), `title.tsx`, `series.tsx`, `search.tsx`,
  `identify.tsx` (needs a match), `libraries.tsx` and `create-library.tsx` (retiring a folder and deleting a library, each confirmed by `confirm-button.tsx`), `files.tsx` (what
  the scanner saw), `settings.tsx` (keys, passwords and server settings), `jobs.tsx` (the job view), `system.tsx` (the System page: the server's facts, doctor's findings, and each machine's software, sandbox, hardware encoding, disks and recent prepares, D225), `node-confinement.tsx` (a machine's FFmpeg sandbox, on the System page), `about.tsx`,
  `sign-in.tsx`, `play.tsx` (the player, with `player-commands.ts`, its answers to a remote), `watching.ts` (the film a tab watches: its page's warm-up
  and letting go of the last film's playbacks, D258), `sound.tsx` (the switches of the person's own choices: the boost, levelling),
  `audio-tracks.tsx` (the audio track menu, D230), `watch-together.tsx` (the host's Watch Together
  screen: links with notes, members, ending it), `together-viewer.tsx` (a viewer's page, joined by
  their link), `together-host.tsx` (the host's player in their session), `together-news.tsx` and
  `together-events.ts` (who is here and the named events), `use-together.ts` (the session's
  connection), `together-voice.tsx` (voice's controls), `volume-range.tsx` (every volume slider, which a touch anywhere moves), `together-storage.ts` (the memberships this tab keeps),
  `subtitles.tsx` (the subtitle menu and offset) and `subtitle-search.tsx` (finding and fetching a
  subtitle a file lacks, from OpenSubtitles, D224), `this-device.tsx` (the This device page: its
  name), `now-playing.tsx` (who is playing what, and stopping it),
  `roles-section.tsx` (roles and their libraries), `system-reachability.tsx` and `system-work.tsx`
  (the System page's reachability check and its queue's health), and `busy-group.tsx` (controls
  held while a request is answered).
- A remote's, a keyboard's and a controller's input (controller-input-2026-10-25.md §6.1):
  `input-buttons.ts` (which button and command a press is), `input-commands.ts` (keys and pads to
  commands, and what a command no one answers does), `input-layer.tsx` (the layer around every
  screen: Back's target, focus kept on history entries, `useCommands` for a component),
  `input-pads.ts` (a controller's layout and stick), `input-pad-poll.ts` (the pads polled once for
  every listener), `controller-choice.ts` (the OK chosen on This device), `focus-engine.ts` (where an
  arrow moves focus, within containers), `focus-navigation.ts` (moving, starting, scrolling,
  pressing) and `focus-memory.ts` (what each container and history entry remembers).
- The device lab (plans/tv-device-tests.md): `lab.tsx` (its menu), `lab-frame.tsx` (a probe's
  page and its Send), `lab-device.tsx`, `lab-input.tsx` (the button test, with `lab-buttons.ts`, its
  chips, `lab-keys.ts`, `lab-events.ts`, its events on their way to the log, `lab-field.tsx`, its text field, `lab-fullscreen.tsx`, its full-screen test, and `lab-pad-input.tsx`, its controllers), `lab-remote.tsx`, `lab-extend.tsx` and `lab-extend-player.tsx` (with
  `lab-extend-channel.ts`), `lab-cast.tsx` (with `lab-cast-sdk.ts`, Google's sender SDK),
  `lab-send.ts`, `lab-timing.ts`, `lab-tone.ts`, `lab-offered.ts` (whether the server offers it); its words in `src/locales/en-lab.ts`.
- The look of plain elements everywhere: `src/base.css`; the skin's tokens: `src/tokens.css`
  (D251).
- Every user-visible string: `src/locales/en.ts` and its parts (`en-together.ts`,
  `en-devices.ts`), through react-i18next (D51).
- Colours, sizes, focus effects: `src/tokens.css`, never a component's CSS.
- The mount point: `src/main.tsx`, which loads the loader (`src/loader-main.tsx`) or the client
  (`src/client-main.tsx`) by what the page says it is, and `index.html`.
