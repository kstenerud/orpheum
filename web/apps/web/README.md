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
  tests, among the places for watching, shows to everyone while the server offers them
  (`should_offer_device_tests`). The menu's Sign out asks first, in the shared modal dialog, Sign out
  or Cancel, and only its Sign out signs out (the owner, 2026-10-30). `src/app-shell.test.tsx`,
  `src/app-shell.browser.test.tsx`, `src/lab.test.tsx`.
- **Watch Together and Cast are their status tray signs, never top-bar places** (D364 §12–§14):
  both always in the tray, Cast's to everyone and Watch Together's to a person who may host or a
  signed-in guest in a session, white while their function is off and blue, pulsing gently unless
  less motion is asked for or a film's player is on the page, while it is on. **Each opens its dialog
  over the page, which never moves** (D384 §2; `src/tray-dialog.ts`, one open at a time, closed as
  the page moves to another address): Watch Together's (`src/together-dialog.tsx`) starts a session
  or runs it, what plays with the way back to its film's Play page and Stop for everyone, voice and
  the volumes, the people and Invite, and ending it, which closes the dialog, nothing opening it
  again (D385's Also), or, for a signed-in guest, the way back to the group's film; Cast's (`src/cast-dialog.tsx`) the devices to cast to, taking the film of the Play
  page it is pressed on, but a guest's group's film, which it leaves to the group: the cast carries
  no film and says the guest's membership, the receiver following the group in the guest's part
  (D371 §4; `src/together-viewer-cast.test.tsx`; VQ193), or, with a cast on, what plays where, the way back to its film's page and
  Stop casting; it closes once a cast started from it is taken. There is no Watch Together page nor
  casting page: an old `/watch-together` address goes to the Play page of the group's film, else
  home with the dialog open (a guest's `/together` never opens it), and `/casting` to the Play page of the cast's film, else home
  (`src/old-addresses.tsx`). Each dialog's code is a chunk of its own, loaded ahead once the first
  page has shown and the browser is idle (`src/tray-dialog-code.tsx`, through `screen-load.tsx`'s
  `preloadableScreen`), so a first press opens it at once, never "Loading…" in the sign's place
  (the owner, 2026-11-03; VQ254). `src/status-tray.test.tsx`, `src/app-shell.test.tsx`, `src/app.test.tsx`,
  `src/ui-gallery.test.tsx`, `src/player-widgets.browser.test.tsx`.
- **Every page's freezes of a second or more are told to the core's log** (`src/page-freezes.ts`,
  started once in `client-main.tsx`), found the one way the player finds a playback's (the SDK's
  `watchFreezes`, which loads no player: the browser's long tasks, else a timer's lateness on a shown page), with the
  screen's route name, whether a film plays and whether the page was hidden, at most one report each
  `pageFreezeReportEveryMs` (5 s), those within it told together at its end. A Fire TV's focus stuck
  on a series' season row for many presses and nothing said whether the page had frozen (the owner,
  staging, 2026-10-30; VQ198). `src/page-freezes.test.ts`.
- **A security fix the server's version lacks is a banner above every page, an admin's alone**
  (security-updates.md Part B; `src/security-update-note.tsx`): what it fixes, the command that
  installs it and the way to How to update (`/update`). It is never dismissed while pending; Snooze
  for a day hides that fix's banner in this browser for a day (`update.ts`, its storage wrapped so a
  browser that refuses it keeps the snooze for the page's life), and a new fix shows at once.
  Nobody else is shown it, nor is the server asked for them. A newer release with no security fix
  is no banner, only the settings page's Updates row. `src/security-update-note.test.tsx`.
- **A debug tool shows only to a person who shows them, and is marked one way** (the owner,
  2026-10-29): a control or readout for testing is wrapped in `DebugTool` (`src/debug-tools.tsx`),
  which renders it only while the signed-in person's own Show debug tools is on, off until they turn
  it on in Your account, kept on the server (`PUT /me/preferences`) and read with who they are
  (`GET /me`'s `showsDebugTools`), so it follows them to every device; a guest by link alone never
  sees one. Marked so: Play the original file, Watch Together's leader view
  (`src/together-debug.tsx`) on the host's session page and a signed-in guest's, and the debug link
  on the player (D380, `src/link-debug-tools.tsx`): a cap on the tab's download rate, applied in the
  player's chunk fetch so every carrier is capped alike, and a drop of its connection for 10, 30 or
  60 s, which closes the tab's sockets and refuses its requests, sockets and chunk loads, then says
  the browser is back online (`src/link-debug.ts`); for the tab alone, its cap said above every
  screen the whole time it is on ("Throttled to 1 Mb/s"), shown or not. `src/link-debug.test.ts`,
  `src/link-debug-tools.test.tsx`, `src/debug-tools.test.tsx`,
  `src/play-original.test.tsx`, `src/together-debug.test.tsx`, `src/watch-together.test.tsx`,
  `src/together-viewer-cast.test.tsx`.
- **Nothing of the page is drawn under a player's stage that fills the window** (a cast's receiver, a
  refused full screen), but the receiver's lines over the film and a dialog: the rest is hidden, not
  removed, and shows as it was when the stage closes (the efficiency audit's finding 3).
  `src/player-widgets.browser.test.tsx`.
- **A screen whose code loads as it is first shown never leaves a black page** (`later-screens.tsx`,
  through `screen-load.tsx`'s `lazyScreen` and `LaterScreen`, the one way): its code is asked for
  once more after a second, and then the page says it could not load, with Reload and what happened
  to copy, under a loader whose page no worker controls that developer tools may bypass the worker,
  and the core's log is told the screen and why (the owner's Chrome, staging, 2026-10-29). Where
  the cause is a newer client served (its answers name another, or under a loader the worker's
  client files no longer name the file), the page reloads into it at the same address instead, once
  per build in a tab, a reload that did not help showing that page, and the page after tells the
  core's log why (`src/newer-client.ts`; the owner's Mac, staging, 2026-10-29). Timed from the
  first ask, once a screen's code takes `screenLoadSlowAfterMs` (3 s) or more to load, loaded or
  not, the core's log is told the route, the total, each attempt's time and outcome, and, under a
  loader, what the browser's own record of the load says of how the worker answered the chunk
  (`workerAnswerOf`, a best effort): a tab open across a deploy clicked a Continue-watching tile and
  showed "Loading…" for near a minute with nothing told the core's log at all (VQ137, 2026-10-29).
  `src/screen-load.test.tsx`, `src/newer-client.test.tsx`; on a device, VQ123 and VQ137.
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
- **A film the page plays at once is played by its video's start gate, never the autoplay
  attribute**, at each of its loads, once what it loaded outlasts its next chunk's load (the
  player's README; the owner, 2026-10-28), by the server's `readyAheadOfNextChunkPercent`, which the
  page sets from its client settings; its start's and its seeks' reports say what it waited for
  (`src/seek-timing.ts`). `src/start-gate-play.test.ts`, `src/app.test.tsx`.
- **A playback the page asks for again goes on from where its video stands** (D230 §5, D336 §3):
  another audio track, a fetched subtitle, the original chosen or the boost turned on asks with its
  cause and where the video is, which the page starts from in place of the resume position
  answered, and which the request carries for the server's log (`startsAtMs`). A film whose track
  was changed 2:46 in started again at 0:00, the log saying only resumePositionMs=0 (staging,
  2026-10-26). `src/sound.test.tsx`.
- **Another audio track chosen on a film's ladder switches in place, the dialogue boost's way**
  (`playTrackChosen`, `src/ladder-sound.ts`): the picture plays on and the old sound until the new one is
  heard, the boost kept as the person left it where the track has a boosted mix; the gain changes as
  the new sound is heard, to its own track's levelling as the decision's tracks say it, never the
  starting track's (`ladderSoundLevelOf`, `chooseSoundInPlace`). It asks for
  the film again only where the player cannot switch in place: the element's own HLS, or a ladder a
  stalled player moved to. Off the ladder, a track switches in place where the element lists its
  file's tracks (Safari), levelled by its own and said to the server as the sound playing, which a
  stall's offer then starts; elsewhere the film moves onto its ladder once. A Watch Together leader's
  choice reloads no one's player, and a cast's receiver follows its remote's by the same path
  (audio-switch-and-retiming-2026-11-01.md §1.2; the owner's answers of 2026-10-31).
  `src/audio-switch.test.tsx`.
- **A Play page's playback the core answered nothing for is asked for again** (`askWhileUnanswered`),
  each silence told to the core's log: the page draws its player, and with it a host's Watch
  Together player and its session connection, only once its playback is answered, and one answer lost
  over WebRTC left the host's page on "Loading…" for good, its player never attached (the owner's
  Chrome back from another site, staging, 2026-10-29). The silence is known by its name too, as one
  under a discovery's loader fails in the loader's own copy of the SDK, of another build across a
  deploy (`isUnanswered`); a body that never came fails with it too, which the browser had read as
  "TypeError: Failed to fetch", and the page said "The server could not be reached" (the owner's Mac
  Chrome back from the Watch Together page, staging, 2026-10-30). `src/play-unanswered.test.tsx`,
  `src/reopen.test.ts`; VQ138, VQ188. Every
  attempt of the decision (`watching.ts`'s `decide`, `reopen.ts`'s `askUntilAnswered` and
  `askWhileUnanswered`) is timed from the first ask (`play-start-timing.ts`'s `openPlayback`), and
  once the total reaches `playbackStartSlowAfterMs` (3 s), opened or not, the core's log is told
  each attempt's time and outcome and what carries the request now (`mediaCarrierOf`): a
  Continue-watching tile's click showed "Loading…" for near a minute with nothing told the core's
  log at all, until the playback started (VQ137, 2026-10-29). `src/play-start-timing.test.ts`,
  `src/play-unanswered.test.tsx`; VQ137.
- **A Watch Together viewer needs no sign-in and, by link alone, sees none of the app** (D246 §3, §5,
  §6): their page is only the host's playback, black while nothing plays, saying it waits for the
  host to start a film, with nothing asked of their
  speakers (D336 §4), voice, their own volume and Leave; the link's secret leaves
  the address once used. **A signed-in guest's film is the group's on its Play page, the one page
  every film plays on** (D372 §9, D384 §2; `src/together-guest-play.tsx`): a Play page whose film
  their session plays shows it followed, as a screen in the shell, with who is here, voice, their own
  volume and Leave (`GroupFilmPage`, `ViewerPage`), moving to the Play page of each next film the group
  plays; another film plays as their own, still a member; they come back by the tray's Watch
  Together dialog's way back to the film, connecting again by the membership their browser keeps and
  catching up. Their way in, `/together`, where their join lands, goes to that Play page once
  joined, or, while the group plays nothing, waits on that page, saying so, for the host to start a
  film, then moves to its Play page; no dialog opens but from the tray's sign (`ToTheGroupsFilm`;
  the owner, 2026-10-30; VQ193). A member with no device on the session's page is said away, in
  who is here and on the host's list of people. A guest by link alone comes back by the same link in
  the same browser (D267 §1). `src/together-viewer-shell.test.tsx`; on devices, VQ172. Their session ended, a guest by link alone is told so on that page; a
  signed-in guest is taken home and told why there, in the shell's one notice (`src/notice.ts`, a
  pop-up, the shared modal dialog, closed by its OK, a press outside it or Back, or gone at the next
  screen; the owner, 2026-10-29), but for one sent away for the host's connection, who
  keeps Join again (the owner, 2026-10-29). A session the server no longer has, as after its
  restart, ends so too: a signed-in guest with none once the server was asked goes home told it is no
  longer there, and a guest by link alone whose page loads again after theirs ended is told it is
  over, never that the address has no invitation (staging, 2026-10-29; VQ136). A Play page that
  asks a guest's session what it plays and finds it over, loaded again after it ended or on a device
  that hosted it too, the guest's own person, ends it so as well, the film never played as their own
  (`WhoseFilm`; the owner's S23, staging, 2026-10-30; VQ192).
  `src/together-viewer.test.tsx`, `src/together-guest-play.test.tsx`, `src/together-viewer-shell.test.tsx`,
  `src/together-viewer-over.test.tsx`,
  `src/notice-note.test.tsx`.
- **Casting is a session with a device: whatever the person plays goes there while it lasts, and
  the receiver keeps a television's controls** (D364, revising D337, D338 and D358;
  plans/cast-to-device.md): every signed-in page holds a live connection (`src/live.tsx`) that says
  whether its tab is in front and is told the cast its device sends. Cast's dialog (the tray's Cast)
  lists the person's other devices open now, by name and last use, saying of one with no Orpheum tab
  in front that one must be brought to the front, and showing one receiving a cast unavailable, its
  button disabled, a cast to it refused as "‹Device› is showing another cast" (D386 §1: no device
  takes a receiving one over, the same sender's included), and listing none in the Watch Together
  session the person is in (D384 §3, the server's `isInYourWatchTogether`); one chosen starts a session, moving the film of
  the Play page under it at its position (`src/playing-here.ts`). There is no casting page (D384 §2):
  an old `/casting` address goes to the Play page of the film the cast plays, else home
  (`src/old-addresses.tsx`).
  The receiver asks once, "‹Device› wants to use this device for playback" (`src/cast-asked.tsx`),
  Use focused for a remote's OK: Use's press asks for the page full screen, tells the server, and
  lets every video of the session play with sound; Not now, or a minute's silence, ends the session,
  the sender saying so. Every film the sender then plays (`PlayOrSendOn`) is handed to the receiver
  in the same cast, which opens its Play page, noting the page the session took the tab over from;
  between videos the receiver waits full screen on "Ready, controlled from ‹Device›"
  (`/cast-ready`, `src/cast-ready.tsx`), at a film's end with nothing after it too; the same cast
  handed again is received at once; should the sound still be refused, the first key, click or tap
  turns it on, and does nothing else. The page leads the cast's session from where the sender stood,
  or, while its person hosts Watch Together, that session as their player, playing as the device
  plays a film alone, nothing steering it, each of the remote's commands carried out as its own press
  (D371 §1, §5); while its person is a signed-in guest in a session, the ready page follows that
  session's film in their part, just another follower, and the guest's own page leaves the film to it,
  holding the group's controls its rights give (D371 §4). The remote shows how the receiver plays
  the film: the file as it is or converted, its quality and its link (D371 §4); and its sound, the
  dialogue boost where the film has a boosted mix, levelling and the audio track, each choice sent to
  the receiver, whose own choice carries it out as its press would, drawn or not (`src/player-tracks.tsx`), kept for the person, the
  remote showing what the receiver says after (`src/sound-panel.ts`; a Fire TV's controls are hidden
  while controlled, so its sound could be changed only after taking control back, the owner, VQ116's
  run). It fills the window and
  shows a television's player controls, which a mouse's move, a click or a tap opens as well as a
  remote's keys, a mouse's click on its video pausing or playing it as on an ordinary page, and a
  fading *Controlled from* line; its controls, media keys and seeks stay its own. Back (a Fire TV's
  history step too, the person's alone: never a step the app makes itself, as the Use question's
  close, navigation's `onPersonsStep`, D384, `src/cast-receiver-steps.test.tsx`; and full screen left by the browser, not the page, told to the log), or OK or a
  click or tap twice within 500 ms, a held OK's key repeats no second press, takes control back, which
  ends the cast like any other end (D385 §1, amending D338 §6), the film left as it stands until the
  end has reached the sender. **A cast that ends sends its receiver home** (D384 §1): control taken
  back there, Stop casting on the sender, the session's end, its controller gone (its receiver gone
  too long), in the page's place, out of full screen; a Play page of the cast's that Back reaches after shows its
  title's page, never playing (D383 §1). A page loaded again while its tab received a cast (a reload,
  a deploy) holds its film until the server says whether it hands the cast again (the live
  connection's `isReceiveKnown`, the tab's mark in `src/cast-receiving-mark.ts`), and, handed none
  or one not taken, goes home too. `src/cast-uncontrolled.test.tsx`, `src/cast-receiver.test.tsx`,
  `src/cast-ready.test.tsx`, `src/cast-to.test.tsx`; on a device, VQ189 and VQ190.
  **The page cast from stays where it is** (D384 §2): a Play page whose film goes to the receiver,
  sent on or taken by a cast started over it, shows in its video's place the cast's remote
  (`src/cast-remote.tsx`), which holds no player: with a film playing, *Playing on ‹Device›*, its
  controls (`src/session-remote.tsx`), working the session the tab hosts while Watch Together is on
  too, Up next and Stop casting, the one way to bring the film back (D388); with
  none, *Ready on ‹Device›* and Stop casting. Stopped, here or from Cast's dialog, its control taken
  back on the receiver, or the receiver gone (D364 §6), the film plays on the same page again, asked
  for with cause castOver and that start: stopped, from where the server says the receiver stood,
  playing; taken back or gone, from where the receiver last stood while it led the session the remote
  works (the hosted one while Watch Together is on, the device then the member's player again),
  playing or paused as it was (D385 §2, §3; the route's `paused` for a cast moved on to another film);
  a cast the server no longer knows (not among the device's casts, or its session's credential
  refused, as after a restart) sends the page home, the shared notice's pop-up saying so there
  (`homeTold`, `src/notice.ts`; D386 §2), as it does the receiver; *Play here from* is gone; a film's warm-up is let go of while the page
  is a remote. `src/cast-to.test.tsx`,
  `src/cast-asked.test.tsx`, `src/cast-receiver.test.tsx`, `src/cast-receiver-sound.test.tsx`,
  `src/cast-ready.test.tsx`,
  `src/cast-receiving-mark.test.ts`, `src/cast-remote.test.tsx`, `src/player-overlay.test.tsx`,
  `src/watching.test.ts`, `src/together-viewer-cast.test.tsx`; on devices, VQ64–VQ66, VQ68, VQ71, VQ76,
  VQ77, VQ90, VQ99, VQ101 and VQ103.
- **Voice starts only from Talk**, the press the microphone needs, on the viewer's page and with
  the host's player; then mute, Leave voice and the partner's volume, apart from the film's; a
  microphone that cannot open says why. `src/together-voice.test.tsx`.
- **The host's player leads their session while this tab hosts one, and is as before without
  one**: the film goes on for everyone, the host's player playing as it would alone and the guests
  catching up to it (D371 §1, §6), and goes on as the host leaves the player (D364 §15, D246
  §5's stop on leaving withdrawn); Watch Together's dialog holds the way back to the film, whose Play
  page holds its controls, and Stop for everyone, and Stop casting while the host casts too.
  `src/watch-together.test.tsx`.
- **On a discovery's origin the page reaches its core through the discovery** (webrtc-transport.md
  W4c): the discovery names its client API in the page (`<meta name="orpheum-discovery-client-api">`;
  the client reads no environment, D204); a link is spent by its click, never the page's loading; the
  connection's offers are proven and its answers taken only when the core signed them by the key the
  link's fingerprint names; each core's credentials are kept under its id. A core that turns the
  device away for its network's share (rtcapi's `Verdict`) is said so, with a way to try again,
  never as a network that cannot reach it, and a remembered one stays remembered. A core that
  refuses the key the device remembered for it (its offer's proof does not hold, as once the device
  was removed) is no dead end either: what the device held is forgotten and its person signs in
  again, at the discovery's realm sign-in, which says why, and what happened names the refusal.
  Every WebRTC
  transport a page used is closed with its path, when its hand-off fails, and when the page leaves
  for good. `discovery-gate.test.tsx`, `kept-path.test.ts`, `loader-pages.test.tsx`,
  `session.test.ts`, and the SDK's `discovery.test.ts`. Once signed in, the page tells the core's log
  how it chose its first path and why, over WebRTC with its pairs as the browser sees them (D354 §5).
  `path-choice.test.ts`, and the SDK's `rtc-pairs.test.ts`. The page hears that the way to its core
  may work again from the path in use alone, the WebRTC transport's `onBack` or, on a core's name, the
  browser's `online` event (`onPathBack`, the loader's since minor 4, the browser's `online` under an
  older loader), and its Watch Together connection connects again at once on it (VQ130).
  `kept-path.test.ts`, `client-reaches.test.ts`. A connection's later offers are proven by what the
  device holds once anything it holds changed since the target was found, its membership too
  (`holdingOf`), so a person signed in before joining by a link leaves the spent link's key behind;
  one refused all the same is sent once more under what it holds now, and a transport refused for
  good shows its person, over the client, a reload and a way to sign in again (VQ131).
  `discovery-later-offers.test.ts`, `loader-path-refused.test.tsx`.
- **At a discovery the page is its loader, which runs the core's own client** (D308): `app.` keeps
  no core's session, hands a link on to its core's client origin, and hands a realm sign-in on by a
  single-use code; a client origin is bound to the core its name names and opens nothing for
  another; the core's client runs only once its manifest holds under the key the link, the hand-off
  or this origin named, its version is not one the discovery refuses and its page is the one it
  signed; and the loader clears every cookie it can see. A person signed out at a client origin
  lands on the discovery's sign-in wherever it has realms, however the core was reached, through a
  realm or by a link, and the client draws none of the core's own meanwhile (the owner, 2026-10-30;
  `willLeaveWhenSignedOut`, the loader interface's minor 5). A client's file comes only over the page's
  path, never as the discovery's 404, and a client's script that does not load shows what happened to
  copy, never a black page; a worker reads the list of the client's files again before it takes
  a file for the loader's own. Under a loader a media load never goes to the network: it goes
  through a page carrying its path, waited for patiently, or is answered 502 saying none does; a
  worker keeps a page's answer on its path, never its silence, and tells the page, which tells the
  core's log, where it sent its media and why; a page no worker controls once it found its core
  reloads once in its tab. A page a worker controls under a loader asks it, once, for an address
  only the worker answers (`workerProbePath`); answered by the network, or not at all, developer
  tools bypass the worker, and the page says so above every screen, "Developer tools are bypassing
  the service worker; media won't load", and tells the core's log once (`worker-bypassed-note.tsx`;
  posters 404'd for an afternoon with nothing said, agent-failures A51).
  `loader-config.test.ts`, `loader-households.test.ts`,
  `loader-pages.test.tsx`, `loader-run-client.test.ts`, `loader-client-failed.test.tsx`,
  `loader-sweep.test.ts`, `sw.test.ts`, `worker-bypassed-note.test.tsx`, and the SDK's
  `discovery.test.ts`, `rtc-media.test.ts` and `worker-control.test.ts`.
- **A TV's code is activated by its QR code with one tap, or typed with a committed check code**
  (D321): the TV shows a QR code of the loader's `/activate` carrying its key's hash; `/activate`
  goes to the one realm's Activate a device, asks which of several, or offers the link to copy or a
  sign-in that carries on there; a scanned code is entered at once and answered by one tap, with no
  check code; a typed one's check code is made of the key, the nonce the TV committed to and the
  core's. `loader-device-code.test.tsx`, `loader-pages.test.tsx`, `device-activation.test.tsx`, and
  the SDK's `device-code.test.ts`.
- **An admin rescans from the Libraries pages and from a film's or series' page** (D357): "Rescan
  everything" on the list, and "Rescan" on the own page of a library of films or of series alone,
  never a mixed or other one; the list and the library's page show how far its rescan is, scanning
  first, then the films and series matched again; and "Rescan this" beside "Identified incorrectly",
  for an admin alone. Each says whether it started, or one already runs. `libraries.test.tsx`,
  `rescan.test.tsx`, `library-pages.test.tsx`. Beside each is its full rescan (D392): "Full rescan
  of everything", and "Full rescan" on a library's page and in "Identified incorrectly", which ask
  for the same rescan with every file read again first. `rescan.test.tsx`.
- **The admin's libraries are a list, each with a page of its own, and creating one is a page too**
  (D361): `/libraries` lists each library at a glance with "Manage this library"; `/libraries/<id>`
  holds everything that changes it, deleting it (confirmed first) going back to the list;
  `/libraries/new` creates one, replaced by the new library's page once made. The folder browser is
  a modal dialog (`FolderBrowser`, over `modal-dialog.tsx`) wherever a folder is chosen: creating,
  adding a folder, saying one moved. `src/library-pages.test.tsx`, `src/retire.test.tsx`,
  `src/folder-picker.test.tsx`, `src/folder-picker.browser.test.tsx`, `src/router.test.ts`.
- **A modal dialog gives focus back to what had it once it closes** (D361 §3): the browser does not
  for a dialog the page removes (idiosyncrasies.md). `src/folder-picker.browser.test.tsx`.
- **The session token travels in the Authorization header**; images load through the media
  cookie sign-in sets (D154), by addresses from `artUrl` in `@orpheum/sdk`.
- **A poster or still whose image does not load shows its blank and asks for it again** on the
  background changes' schedule (D237), each time by a new address, until it loads or leaves the page,
  so art the server makes after the page was drawn shows with no reload (the owner, 2026-10-25).
  `src/poster.test.tsx`.
- **A film or series with no poster shows its still in the top half of the poster's blank**, the
  blank's own mark below it, and its poster alone once it has one (D362 §2). `src/poster.test.tsx`,
  `src/browse.test.tsx`.
- **A grid's tiles in view say so** (`PUT /tiles/interest`) once they settle, again while the tab is
  visible, and that none is shown when the grid goes, so the work their pictures need comes first
  (D362 §4, `src/tile-interest.ts`). `src/tile-interest.test.tsx`.
- **A screen showing what the server changes in the background asks again** — the lists, a title,
  *Needs a match*, the libraries, the System page, the jobs and the home rows — after 2 s, backing
  off while nothing changes to 30 s, and keeps what it shows meanwhile (D237); the waits are the
  server's, from `/client-settings`, through `useBackgroundChanges`. An ask with no answer within
  `askAnswersWithinMs` (60 s) is given up as failed, aborted by its signal and reported, and the poll
  asks again after the failed wait, so one request that never answers cannot stop it for good (the
  owner, 2026-10-24). One way to poll:
  `pollUntilStopped` in `src/polling.ts`, through `useResource` and `usePages`.
  `src/polling.test.ts`, `src/use-resource.test.ts`, `src/use-pages.test.ts`.
- **Back is the history; Up goes to the page's parent and never adds to it** (D367): no page shows a
  Back of its own on a computer or a phone, the browser's, the gesture's and the remote's being the
  history's; a television's player overlay keeps one, beside its Up (TV Bro keeps its remote's Back
  from the page). Up (`navigation.up`) goes back to the nearest entry behind showing the parent,
  however far, so Back after it never returns down, else puts the parent in the page's place; each
  entry keeps the addresses of this app's entries behind it (`below`) to find it. Where a page sits is
  `src/upward.ts`: an episode, and its playing, under its series at its season under its library; a
  film's playing under the film, an extra and its playing under its film (the server's
  `filmOfExtra`); a film or a series under the library the person came through, else the first of
  those listing it by name (the server's `libraries`); an other video under its library's list at its
  folder, a folder under the one above; Home, Films, Series, a library's list and the account's and
  admin's tops have no parent. It shows as a trail on a computer or a phone, `Series › Show ›
  S1E1 Pilot`, each part an Up, a Play page's last part, what plays, a link to its own page, and as
  one "‹ Show" on a television (`src/up-trail.tsx`), nothing drawn over it, a title's backdrop
  below it. A
  remote's Back with nothing of the app behind goes to the parent, as does a television's Back at the
  base laid under a page it opened on. `src/navigation.test.ts`, `src/upward.test.ts` (the research's
  scenarios over the history), `src/up-trail.test.tsx`, `src/play.test.tsx`,
  `src/input-player.browser.test.tsx`, `src/up-trail.browser.test.tsx`.
- **Back or Forward never starts a film** (D383 §1): a Play page the browser's history returns to,
  by a popstate or by loading the page again (`isLoadedByHistory`), shows in its entry's place its
  title's page, which the Play page noted on its entry once its playback named it
  (`navigation.noteInPlace`), else home; one place reads it, `navigation.ts`, never each screen. A
  film starts only by a deliberate act: Play, a cast, Up next, rejoining Watch Together, Up's own
  step back to the page a stopped cast took over, or a reload of the Play page the person is on.
  Seen on the owner's Fire TV: Back from a page reloaded mid-cast started a film watched earlier
  (2026-10-30). `src/navigation.test.ts`, `src/play-back.test.tsx`; on a device, VQ189.
- **A series is one page; its season, a sort, a view and a search are views of a page, not steps**
  (D367 §9): choosing one replaces the history's entry (`Link`'s `isReplacing`, `RouteChooser`'s
  `isAView`); a series' page shows the season chosen, else that of the person's next episode, else
  the first, chosen from a row of its seasons' posters above the episodes (D379, `SeasonRow`, one
  focus row a remote moves along by Left and Right and leaves by Down into the episodes; each card
  its season's own poster, else its still, uncaptioned, else the series' poster, "No poster" only
  where there is none of them); returned
  to after one of its episodes' page or playing
  (`src/episode-reached.ts`), it shows that episode's season with the episode focused; opened by a
  step forward, as from a list, it shows from its top, every episode reached let go at each step
  forward, and any page a step forward opens starts at its top (`src/input-layer.tsx`; the owner's
  Fire TV, 2026-10-29). A list Back returns to (Films, Series, a library's) loads its pages until the
  entry it was left from is drawn, which then takes focus in view (`useListLoadsToFocusNoted`).
  `src/series-page.test.tsx`, `src/input-layer.test.tsx`, `src/input-walk.browser.test.tsx`, `src/series.test.tsx`, `src/library-browse.test.tsx`,
  `src/input-rows.browser.test.tsx`,
  `src/input-player.browser.test.tsx`.
- **A series' own page offers its person's next episode at once** (the owner, 2026-10-25): the
  server's `nextUp`, played by *Continue S1E3* where they are partway through it, which resumes, else
  *Play S1E4*; where it cannot be played now, why, and no Play, as a title's page does; at every
  season, none when there is no next one. `src/series.test.tsx`.
- **What a person watched shows a white check, the last of a poster's tray of status cues** (D370
  §2, §3), drawn by `Icon` in `--color-watched` on the tray's dark patch and read "Watched": on films,
  series, other videos, Continue Watching's rewatches and a season's and an episode's rows, never on
  an extra, and nothing fades or greys what it marks; a series or season once its person watched every
  episode. A title's page says it beside the check, among its status notes. `src/status-cues.test.tsx`,
  `src/watched-mark.test.tsx`, `src/ui-gallery.test.tsx`; on devices, VQ89.
- **A person marks a video, a season or a whole series watched or unwatched on its page** (D370 §4):
  *Mark as watched*, or *Mark as unwatched* once it is, a button like any other that a remote reaches,
  never a control on a poster; it changes at once and offers Undo, which puts every mark back as it
  was, never a question. `src/watched-mark.test.tsx`; on devices, VQ89.
- **A series is shown in the ordering its person chose** (D159 §1, episode-orderings.md §4): a series
  with more than one offers them, aired first, each by its label with its kind and how many episodes
  it holds; a choice is kept on the server and shows the series at its default season in place of
  the entry, where `?season=N` is a group of the ordering in use (§7 Q10). Its seasons are that ordering's groups,
  headed by their names ("Disc 1", else "Part 2"); Next Up, an episode's page and Continue Watching
  label an episode there ("Disc 1 · 2", "Episode 812"), its page with its aired place on a second
  line; what the ordering leaves out is listed under *Not in this order* (§7 Q4, Q8). Labels are
  `src/episode-place.ts`'s. `src/episode-orderings.test.tsx`.
- **A playing episode offers what plays next, and the person's autoplay plays it** (plans/up-next.md):
  the server's `GET /files/{id}/up-next`, asked once the film has played so it does not compete with
  its first chunks (the efficiency audit's finding 4), the next episode in the person's ordering, shows as *Up
  next: S1E3 · ‹name›* with its still over the film from `upNextLeadMs` (30 s) before its end and on
  the end screen, a button a mouse, a remote's OK or a controller presses; with autoplay on (the
  person's own switch, on until turned off, on their account) it counts `upNextCountdownMs` (10 s)
  down from the end and plays it; any press stops the count, Back, Escape or its × dismisses it; it
  takes focus as the film ends, and a television's overlay leaves OK on it to it. A film, the last
  episode and one that cannot be read now offer nothing. `src/up-next.test.tsx`,
  `src/autoplay-setting.test.tsx`, `src/player-overlay.test.tsx`; on devices, VQ67.
- **A run of autoplays asks before it goes on, and a host's moves their group** (D339): after
  `stillWatchingAfterAutoplays` (3) episodes autoplayed in a row with no press on the page, the next
  film's end shows *Still watching?*, Continue (focused) or Stop (leaves the film), and counts
  nothing; any press starts the run again. A host's player moving on to another film's player, by Up
  next or its autoplay, does not stop their Watch Together session: the next player puts its film on
  for everyone. `src/up-next.test.tsx`, `src/watch-together.test.tsx`; on devices, VQ69 and VQ70.
- **A Watch Together invitation is short-term** (D346): the host's list says until when each
  waiting link admits; a guest whose link no longer works is told invitations are short-lived and
  to ask for another, and one whose session the server closed as idle is told so. The join's form
  asks first whether its link admits (`POST /together/join/check`, which tells the link's holder
  nothing a join's refusal would not), and one refused, as a link used already in another browser,
  is said in the form's place (the owner, 2026-10-30, VQ172(3)).
  `src/watch-together.test.tsx`, `src/together-join.test.tsx`, `src/together-viewer.test.tsx`.
- **Lists have no pagination to click through** (web-client.md): the next page loads as the
  end comes into view, with a button that always asks too. `src/end-of-list.tsx`.
- **A list by name's alphabet rail moves it to a letter** (the owner, 2026-10-25): the server names
  the first title at or after the letter in its own order (`/titles?from=`), # the first of all; the
  list loads its pages up to it as scrolling would, and its poster takes focus at the view's top,
  where no title comes after the letter the last. The rail (`src/alphabet-rail.tsx`,
  `useAlphabetRailLetters`) is given the letters a list has titles under (`GET /titles/letters`),
  refreshed with the list's own background poll (D237), and shows only those: Films, Series and
  the Libraries page all ask it this one way, Films and Series shown only sorted by name. A list Z
  to A lists its letters Z to A, and a letter starts it at the letter's first title in that order,
  asking from the letter after it (`fromForLetter`, D366 §4). `src/alphabet-rail.test.tsx`.
- **Every sort runs either way** (D366 §2): Films, Series and the Libraries page offer each sort
  its own way and the other (`src/sort-choices.ts`), names A–Z or Z–A, dates newest or oldest
  first; films and series sort by release date too (§1); the Libraries page keeps the way in its
  address (`order`, named only when not the sort's own). `src/library-browse.test.tsx`,
  `src/alphabet-rail.test.tsx`, `src/router.test.ts`.
- **A person configures each library for themselves on its own list** (the owner, 2026-10-27): a
  library's Configure on the Libraries page, for everyone, opens a dialog of their own choices, kept
  on their account, the one place they are made: Show on Home (D366 §5), every library on Home until
  left out (`leftOutOfHomeLibraryIds`), Home's newest rows asking `GET /titles?home=true` and the
  server drawing Continue Watching the same way; and Show in Films or Series (D349 §8), a films or
  series library in those pages until left out (`leftOutLibraryIds`, `GET`/`PUT /me/preferences`),
  staying on the Libraries page regardless. Escape or a remote's Back closes it.
  `src/library-configure.tsx`; `src/library-configure.test.tsx`, `src/library-browse.test.tsx`,
  `src/home.test.tsx`.
- **Libraries replaces Search in the top bar** (D349): one page for every library the person
  reaches, the first chosen by name on a fresh visit and the chosen library, the view, the folder,
  the sort and a search query all held in its address (`/browse`), so Back and a reload return to
  it; a film library lists its films, a series library its series, a mixed one both, *All libraries*
  everything, each by name or newest first, a film or series library by release date too, each
  either way; an other library lists its videos by recorded date (the
  default), name, file name or newest added, with a view of *All* or *Folders*, a 16:9 tile and its
  recorded date in place of a poster and a year; its folders are only a view, each leading into its
  own, with a trail back out (D367); the view and the sort are each a focusable
  drop-down (`src/route-chooser.tsx`, over `drop-down.tsx`), since a native `<select>` is
  unreliable on a television's browser, its button marked by a down chevron as opening a list. The
  page has no library chooser, its heading the library shown: **the banner's Libraries icon is a
  menu of the person's libraries** (the owner, 2026-10-28; `app-shell.tsx`'s `LibrariesMenu`, over
  `drop-down.tsx`), each by name, *All libraries* last, the one shown marked; with one library the
  icon goes straight to its page, and with none, or before they are read, to `/browse`.
  `src/library-browse.test.tsx`, `src/route-chooser.test.tsx`, `src/libraries-menu.test.tsx`,
  `src/library-browse.browser.test.tsx`, `src/input-pages.browser.test.tsx`; on devices, VQ72,
  VQ181.
- **A chooser and the banner's menu are one drop-down** (`src/drop-down.tsx`, the owner, 2026-10-28):
  a button, the chooser's with its chevron and the menu's its icon alone, that opens a pop-up
  (`popover.tsx`, modal by `modal-layer.tsx` as a dialog is) holding a list of places
  (`list-box.tsx`), with no Close (the owner, 2026-11-02; D408 §2); a press outside it closes it and
  presses nothing under it; it closes on a place pressed, Escape, Back of either kind, Left or
  Right, wherever focus is in it, focus back on its button. It opens at its button
  (`drop-down-place.ts`): on a big screen its current choice over the button, as a select's list;
  on a phone's width a pop-up over the button; inside the screen either way, less a little room at
  each edge and the device's safe area that no pop-up covers (`tokens.css`'s `--screen-edge-room-*`;
  the owner, 2026-11-03), a big screen's list cut at that room's edge and scrolling within itself,
  its choice still over the button, while it keeps half its height, else moved (the owner,
  2026-10-29). A pop-up is as wide as its widest choice or control unwrapped, wherever its button
  is, but no wider than half the screen nor capped under 16rem (`--popover-widest`), a longer one
  wrapping (the owner, 2026-11-03); a line that only informs (`ListNote`) wraps to that width and
  never widens it (D408 §7); 🔈's grows out of its button, its speaker laid over it and the slider
  above, where there is room above (Popover's `partOverOpener`); focused on its current
  place, a place either side of it in view where it has them, and a place of context kept beyond
  the one focused as it moves (D408 §3); and its opening and closing never move the page. The Films and Series
  pages' sort and a library's files' Show are choosers too (`ActionChooser`), and so, where a
  native `<select>` was, which a television's browser cannot work (the 2026-10-27 widget audit;
  plans/toolkit.md step 6), are the audio and subtitle menus under the film, Find subtitles' Language and a series' Order (`src/audio-tracks.tsx`,
  `src/subtitles.tsx`, `src/subtitle-search.tsx`, `src/series.tsx`; `src/subtitles.test.tsx`, `src/play.test.tsx`, `src/sound.test.tsx`, `src/subtitle-search.test.tsx`, `src/episode-orderings.test.tsx`; VQ253). **Every modal dialog closes on the browser's own Back** (D367
  §10), a Fire TV's remote Back in Silk being a step of the history: an entry held over the page's
  while it is open (`navigation.ts`'s `holdBack`), which a close another way takes away and a choice
  that moves the page takes away first, so the history is as it would be with nothing open.
  `src/navigation.test.ts`, `src/route-chooser.test.tsx`, `src/app-shell.test.tsx`,
  `src/series-page.test.tsx`, `src/input-pages.browser.test.tsx`,
  `src/app-shell.browser.test.tsx`, `src/drop-down.browser.test.tsx`, `src/drop-down.test.tsx`,
  `src/modal-dialog.browser.test.tsx`, `src/player-menus.browser.test.tsx`,
  `src/popover.browser.test.tsx`; on devices, VQ97, VQ250, VQ251, VQ252.
- **A page's rarely used actions are under its vertical "…", and a dialog a drop-down's place opens
  takes the list's place** (`src/more-actions.tsx`, the owner, 2026-10-29): the one drop-down with
  no chevron, after a page's Play and Mark as watched, none where the person has nothing there: a
  title's and a series' "Identified incorrectly" (an admin's, one dialog offering Rescan, Enter
  manually and Report this, `src/match-correction.tsx`) and a series' Order. The list closes as its
  place's dialog opens, never stacked under it, so one press outside the dialog is back on the page,
  and focus goes back to the "…" (`modal-dialog.tsx`'s chain of what each dialog gives focus back
  to). `src/more-actions.browser.test.tsx`, `src/title.test.tsx`, `src/episode-orderings.test.tsx`,
  `src/rescan.test.tsx`, `src/input-pages.browser.test.tsx`; on devices, VQ161.
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
  rather than going back by it. Each key goes with the modifiers held (AltGr by
  `getModifierState`), its place on the keyboard and whether an input method composed it, shown in
  its key table, and each context menu the page is asked for is sent with whether it was full
  screen, for the volume keys' layouts and a TV's Menu key (`src/lab-keys.ts`,
  `src/lab-key-table.tsx`; D407 §10). `src/lab.test.tsx`,
  `src/lab-input.test.tsx`, `src/lab-input-field.test.tsx`, `src/lab-pad-input.test.tsx`,
  `src/input-pads.test.ts`, `src/lab-events.test.ts`, `src/lab-input.browser.test.tsx`,
  `src/lab-buttons.test.ts`, `src/input-buttons.test.ts`, `src/router.test.ts`, `src/focus-engine.test.ts`,
  `src/lab-timing.test.ts`, `src/lab-keys.test.ts`,
  `src/lab-extend-channel.test.ts`, `src/lab-tone.test.ts`; on devices, VQ55–VQ60 and VQ242.
- **Every page is driven by a remote, a keyboard or a game controller**
  (controller-input-2026-10-25.md §6.1; the owner, 2026-10-25): one input layer
  (`src/input-commands.ts`, run around every screen by `src/input-layer.tsx`) turns a key (by its
  `key`, then the platforms' keyCodes), a controller's buttons, D-pad and left stick
  (`src/input-pad-poll.ts`, its OK by its make or as chosen on This device) and the browser's media
  seeking into commands; pages hear commands, never keys, and single-letter shortcuts have none. A
  command is offered first at what has focus, so an open menu or dialog answers Back before
  anything else; one no one answers moves real DOM focus by the geometric engine
  (`src/focus-navigation.ts`, `src/focus-engine.ts`), searching the marked container around focus
  before the page (`data-focus-container`: the banner, a row or grid, the player, within an open dialog alone), presses
  what has focus (a list's choices opened by a key, stepped through by a controller's OK, which
  cannot work a browser's own list), or goes back as the page's own Back does
  (`src/back-button.tsx`). OK on a form's checkbox or radio ticks it and never submits the form,
  whatever the browser sends after (`src/form-ok.ts`, on the loader's pages too, which run no input
  layer; a browser takes Enter on a checkbox for the form's submission, idiosyncrasies.md), each OK in
  a form noted for the core's log at its debug level (`noteForTheLog`; the owner's Fire TV,
  2026-10-30; `src/form-ok.test.ts`, `src/realm-sign-in.browser.test.tsx`; VQ199). An arrow inside a
  text field's value or a range's is the field's; one with nowhere to go is the browser's. The
  first arrow starts in the main content, not the banner; an arrow into a container or the main
  content goes back to what last had focus there; the focused element is scrolled into view no
  further than it must be, clear of the sticky banner, and Up or Down over more than half a screen
  of text scrolls through it a step at a time before the control past it takes focus; a
  controller's move is shown as a key's is (`:focus-visible`), a pointer's never; Back returns focus to what had it on the page left, noted
  on its history entry as the page was left and found once the page draws it (`src/focus-memory.ts`),
  Silk's Back too. The player answers with its own controls on every device, never the browser's
  (D369 §2; `src/player-commands.ts`): play/pause wherever focus is, OK on the video, Space on the
  video, on nothing or on the player's own timeline, while on one of the player's buttons Space
  presses it, as OK does (D407 §5), the cast receiver's Space still playing with nothing focused, a 10 s seek by the seeking buttons, the
  browser's media controls and Left and Right while the video, nothing or the player's timeline has
  focus, + and − moving the volume 5 % wherever focus is but a text field, + turning a muted film's
  sound on (D407 §10), and Back leaving full screen before the page; every control over and under the video is
  reached by arrows. A seek is shown before it is made (`src/seek-ahead.ts`): a held or repeated
  arrow or seeking button moves the timeline's marker and time alone, by 10 s steps, 30 s from 2 s
  on and 60 s from 4 s, and the film seeks once, there, as the presses rest (half a second, a key's
  after its release); a press on the timeline shows its place at once; and the place shown stays
  until the seek lands. A remote's drags and steps are one command once they rest the same way
  (the owner, Fire TV, 2026-10-29). In full screen, the browser's or a stage filling the window, every
  device's controls are the overlay (`src/player-overlay.tsx`, D407 §1, §5): its rows, Up and the
  convenience area (Up next, Still watching, when the film ends), the timeline, whose arrows are
  commands (`data-focus-arrows-are-commands`), and the buttons, ⏮ ⏪ ▶ ⏩ ⏭ and 🔈 💬 🎧 ⁝ ⤢, with
  the subtitle, audio and more menus (`src/player-menus.tsx`, D407 §8); a press of play that plays or
  Down from the buttons puts focus on the video so the overlay hides (D407 §6), and + and − show the
  volume they leave a moment (D407 §10). A television's player opens full screen. Outside full
  screen, on every device, nothing lies over the film (D407 §9, `src/page-controls.tsx`): the
  controls are under it, never hidden, the timeline, the same buttons but the menus, which stay under
  them on the page, and the offers as ordinary buttons after a television's ‹; Down on the video goes
  to play, and as full screen ends unasked (a Fire TV's Back) focus goes to play, so the next Back
  leaves the page. The player's own controls name the video they command
  (`markPlayersControls`), as the bar under the film is outside its stage. A mouse's click on a
  computer's film plays or pauses it and a double click puts it full screen. `src/page-controls.test.tsx`,
  `src/seek-ahead.test.ts`, `src/timeline.test.tsx`, `src/cast-remote.test.tsx`,
  `src/player-commands.test.tsx`, `src/player-overlay.test.tsx`, `src/input-player.browser.test.tsx`,
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
- **Every icon is drawn by `Icon` and coloured by where it is drawn** (the owner, 2026-10-27):
  `src/icon.tsx` writes each icon's SVG, the QR code's alone apart, as a scanner needs black on
  white. Its marks are the text colour, `currentColor`; a two-tone cue's ground behind them is
  `--icon-ground`; the region it is drawn in sets both, a status cue's its own class from its tokens.
  `src/icon.test.tsx`; the cues' colours, `src/app-shell.browser.test.tsx`; each icon together at
  `/ui-gallery`, `src/ui-gallery.test.tsx`.
- **Every colour is a token** (D138 §4), what is drawn over a film too (`--color-on-video`,
  `--color-video-scrim`, `--color-video-card`, `--color-video-control`, D369 §1): a stylesheet writes
  none out but `src/tokens.css` and the gallery's made-up posters. `src/colour-tokens.test.ts`.
- **A layout primitive's gap is a step of the space scale, by name** (the 2026-10-05 architecture
  research's R2; plans/toolkit.md step 5): `Stack`, its children in a column, and `Cluster`, in a
  row that wraps, lined up across it (`src/layout.tsx`), take `space` as `"1"` to `"7"`, tokens.css's
  `--space-1` to `--space-7`, never a length, which `tsc` refuses. Each lays out its direct
  children alone, shrinks below its content where it is a flex item (`min-width: 0`), so a screen
  never needs the overflow fix, and answers to its container, never the viewport.
  `src/layout.browser.test.tsx`. ⁝'s steppers lay out their parts with `Cluster`; the screens'
  hand-written gapped rows and columns move to the primitives as they are touched.
- **A list's entries take no class from their user** (R5): a look one list needs is a named variant
  of the component, which any list may use (`ListEntry`'s `isInsetAsChoices`, the steppers'
  `isInAList`), or the user's own element inside it (the banner's who is signed in, inside its
  `ListNote`); no stylesheet reaches into another component's classes from outside it.
  `src/list-box.test.tsx`, `src/player-menus.browser.test.tsx`, `src/popover.browser.test.tsx`.
  `IconButton` takes none: its looks are `isOverVideo`, `isOnCard` and `mark` (`"running"`, `"applied"`), and
  `ModalDialog` still takes a `className`; it loses it as the work reaches it.
- **The player is drawn from one set of widgets** (D369 §1): a button that is an icon alone is an
  `IconButton` (`src/icon-button.tsx`), one size, 2.75rem, the status tray's too; a film's timeline
  is a `Timeline` (`src/timeline.tsx`), in milliseconds, touched anywhere; and the controls over a
  film or for one elsewhere are one bar, `PlayerControls` (`src/player-controls.tsx`), fed by the
  video here (`useVideoSource`) or by a cast's remote, the TV overlay, a computer's and a phone's
  controls and the cast remote alike. `src/icon-button.test.tsx`, `src/timeline.test.tsx`,
  `src/player-controls.test.tsx`, `src/cast-remote.test.tsx`.

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
- The screens: `browse.tsx` (films and series), `title.tsx`, `series.tsx`, `library-browse.tsx`
  (the Libraries page, D349, with `alphabet-rail.tsx`; `search.tsx` holds its search results, by
  `QueryResults`), `identify.tsx` (needs a match), `libraries.tsx` (the admin's list of libraries and "Rescan everything", D361 §1, D357), `library-manage.tsx` (one library's page: its folders, scans, its "Rescan", retiring a folder and deleting the library, each confirmed by `confirm-button.tsx`, D361 §2), `create-library.tsx` (the page creating one), `folder-picker.tsx` (the folder browser and its dialog, D359, D361 §3), `match-correction.tsx` (an admin's "Identified incorrectly" on a film's, an episode's or a series' page, and its dialog of Rescan, Enter manually and Report this, D301 §2, D357, D306 §3), `more-actions.tsx` (a page's "…" of rarely used actions), `files.tsx` (what
  the scanner saw), `settings.tsx` (keys, passwords and server settings), `jobs.tsx` (the job view), `system.tsx` (the System page: the server's facts, doctor's findings, and each machine's software, sandbox, hardware encoding, disks and recent prepares, D225), `node-confinement.tsx` (a machine's FFmpeg sandbox, on the System page), `about.tsx`,
  `sign-in.tsx`, `play.tsx` (the player, with `play-opened.ts`, how it opens its playback,
  `player-tracks.tsx`, the track and sound choices every piece over or under the film reads,
  `player-commands.ts`, its answers to a remote, and
  `up-next.tsx`, what plays next; its controls are `player-controls.tsx`, the buttons, in
  `player-overlay.tsx` over the film in full screen and `page-controls.tsx` under it otherwise, with
  `player-menus.tsx` (the overlay's 🔈 and its subtitle, audio and more menus),
  `convenience-area.tsx` (the offers and when the film ends), `play-up.tsx` (Up and the trail),
  `timeline.tsx`, `icon-button.tsx` and `player-icons.tsx`), `autoplay-setting.tsx` (the person's autoplay, on their account), `library-configure.tsx` (a library's Configure: the person's own Show in Films or Series, D349 §8, and Show on Home, D366 §5), `upward.ts` (where each page sits, D367), `up-trail.tsx` (the trail and Up), `episode-reached.ts` (the episode a series' page returns to), `route-chooser.tsx` (a drop-down of routes, or of choices a page keeps), `drop-down-place.ts` (where a drop-down's list opens), `phone-width.ts` (a phone's width, the stylesheets' breakpoint), `drop-down.tsx` (the one drop-down: a chooser's and the menu's, a pop-up holding a list), `popover.tsx` (a pop-up at its button, as wide as what it holds, with no Close, D408 §2), `list-box.tsx` (a pop-up's list and its options, a place of context either side of the one focused, D408 §3), `layout.tsx` (the layout primitives, `Stack` and `Cluster`, their gaps from the space scale), `modal-layer.tsx` (what a dialog and a pop-up share: the browser's modal, its closes, focus given back), `sort-choices.ts` (each sort either way, D366 §2), `watched-mark.tsx` (the watched check, and marking by hand with its Undo, D370), `watching.ts` (the film a tab watches: its page's warm-up
  and letting go of the last film's playbacks, D258), `sound.tsx` (the switches of the person's own choices: the boost, levelling),
  `audio-tracks.tsx` (the audio track menu, D230), `ladder-sound.ts` (each sound by its place and level, and a track or the boost switched in place), `watch-together.tsx` (the host's Watch Together
  controls, in the tray's dialog: links with notes, members, ending it), `together-viewer.tsx` (a viewer's page, joined by
  their link), `together-what-plays.tsx` (asking a guest's session what it plays),
  `together-leave.tsx` (a guest's Leave), `together-host.tsx` (the host's player in their session), `together-news.tsx` and
  `together-events.ts` (who is here and the named events), `use-together.ts` (the session's
  connection), `together-voice.tsx` (voice's controls), `volume-range.tsx` (every volume slider, which a touch anywhere moves), `together-storage.ts` (the memberships this tab keeps),
  `subtitles.tsx` (the subtitle menu and offset) and `subtitle-search.tsx` (finding and fetching a
  subtitle a file lacks, from OpenSubtitles, D224), `this-device.tsx` (the This device page: its
  name), `now-playing.tsx` (who is playing what, and stopping it),
  `people.tsx` (the People page, D399: three buttons, three sections), `people-dialogs.tsx` (the
  dialogs that make an account or an invitation), `invitations-section.tsx` (the invitations and
  their dialog), `roles-section.tsx` (the roles' rows),
  `role-dialog.tsx` (the one dialog that makes a role and shows one, each change saved at once, D403), `system-reachability.tsx` and `system-work.tsx`
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
  chips, `lab-keys.ts` and `lab-key-table.tsx`, its keys, `lab-events.ts`, its events on their way to the log, `lab-field.tsx`, its text field, `lab-fullscreen.tsx`, its full-screen test, and `lab-pad-input.tsx`, its controllers), `lab-remote.tsx`, `lab-extend.tsx` and `lab-extend-player.tsx` (with
  `lab-extend-channel.ts`), `lab-cast.tsx` (with `lab-cast-sdk.ts`, Google's sender SDK),
  `lab-send.ts`, `lab-timing.ts`, `lab-tone.ts`, `lab-offered.ts` (whether the server offers it); its words in `src/locales/en-lab.ts`.
- The look of plain elements everywhere: `src/base.css`; the skin's tokens: `src/tokens.css`
  (D251).
- Every user-visible string: `src/locales/en.ts` and its parts (`en-together.ts`,
  `en-devices.ts`), through react-i18next (D51).
- Colours, sizes, focus effects: `src/tokens.css`, never a component's CSS.
- The mount point: `src/main.tsx`, which loads the loader (`src/loader-main.tsx`) or the client
  (`src/client-main.tsx`) by what the page says it is, and `index.html`.
