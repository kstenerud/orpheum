# The owner's answers on navigation and widgets (2026-10-27)

The owner's answers to the questions of two reports,
[navigation-back-up-2026-10-27.md](../research/navigation-back-up-2026-10-27.md) and
[ui-widgets-audit-2026-10-27.md](../research/ui-widgets-audit-2026-10-27.md), kept here until the
decisions they feed are written (the Up decision's revision; the widget set's).

## Widgets

1. **Icon buttons' sizes:** the owner read it as a question of the icons' format: "The icons should be
   SVGs so that they can be sized without quality loss." They are (the `Icon` component). The question
   was of the button around an icon (its touch target, 2.75rem; the status tray's narrower 2.25rem); put
   plainly, the owner answered: "Let's try sizing them the same as the others and see how that feels."
   One size, 2.75rem, the tray's too, to be looked at on devices.
2. **Every top-bar pop-up is modal**: "This makes it easier to get out of a menu by clicking anywhere,
   without accidentally activating something else." And: "maybe watch together and cast should be their
   own screens rather than a popup. Research the implications. The warning sign should also take you to
   its own page."
3. (Answered in 2.) The warning sign leads to a page of its own.
4. **"Approve removals"** ("Remove what the last scan did not find", D208) was explained to the owner;
   the lead recommends it ask first, since one press removes many files (nothing deleted under D363).
5. **Play is a button like every other**, "a special color since it's the most important button on the
   page".

## Navigation

- Silk's fake history after a reload exists already (so Back does not ask to quit); it works on the
  owner's Fire TV.
1. **One series page with a season picker: yes**, "ideally when coming back it preselects the season of
   the episode you're coming back from."
2. **Choosing a season replaces the history entry.**
3. **Leaving Watch Together's page by Back keeps the session, if signed in**, the tray leading back. "If
   you're just a transient participant, there's nowhere for you to go. I wonder if it's a good idea to
   allow a transient guest who's followed a link or QR code to come back in via that same code? Only the
   browser that redeemed it would be able to." (To research.) Then: "Remember the threat model here. The
   stakes aren't very high": the design keeps rejoin simple, its protections proportionate.
4. **Up is named by the parent's name.** And: "don't some competitors show where you are in the
   drill-down, for example Series / Copper Kettle / s1e1 blah blah." The lead proposed both, and the owner
   agreed: on computers and phones a trail (`Series › Copper Kettle › S1E1 Pilot`, each part a link, the one
   before last the Up); on a TV only "‹ Copper Kettle", one stop for focus.

The owner agreed every other answer and recommendation above (2026-10-27).

## Screens, casting and Watch Together (answers to screens-and-guest-rejoin-2026-10-27.md §5)

1. **Leaving the film no longer stops it for the group:** correct; Stop, or the host playing another
   film, ends it.
2. **No mini player and no picture-in-picture**, "without a very compelling reason".
3. **What Watch Together and Cast lead to, by what is on** (the owner's table):

   | Watch Together | Casting | Watch Together leads to | Casting leads to |
   | --- | --- | --- | --- |
   | off | off | the session setup page | the casting setup page: the devices one can cast to |
   | on | off | the session's page | the casting setup page; once a cast is set up, the session's page (as on/on) |
   | off | on | the session setup page | the casting page: playback controls and a way to end the session |
   | on | on | the session's page, with an extra button to stop casting | the same: the session's page |

4. **The Watch Together icon shows for anyone in a session, host or guest, unless a transient guest**;
   a signed-in guest browses freely and returns by the icon. **No Watch Together or Cast items in the
   top bar:** the two icons are always shown, white when off, blue while their function is on (perhaps
   pulsing), and they are the entry points.
5. **The uncertain-match page explains nothing of why**: "The user won't care. All they want is options
   for making it certain": "Yes, this is right", or a choice of the candidates that would be right.
6. **The guest's words: agreed** (the owner, shown the full picture). New or changed:
   - A used link opened in another browser: "This link has already been used, maybe in another app's
     browser. Open it there, or ask ‹host› for a new one." (The server says "used" apart from the other
     refusals; never by whom.)
   - Under Leave: "To come back, open your link again in this browser."
   - Leave asks first: "Leave the session? You'll need a new link from ‹host› to come back." [Leave], the
     irreversible red, [Stay].
   - After leaving: "You left the session. To join again, ask ‹host› for a new link."
   Every other word the guest sees stays as it is (web/apps/web/src/locales/en-together.ts).
7. **A guest away from the session's page** (D372 §9–§10, the owner, 2026-10-28): a signed-in guest stays
   a member while elsewhere, even playing another film only they see, shown away in the roster, and
   catches up to the shared film on coming back; a guest by link alone may leave the page and come back
   while the session lasts and they are not removed. Builds on item 4; D372 §1–§8 are this batch's.
   Evidence (the owner, 2026-10-29): a signed-in guest's page has no banner, so the guest cannot reach
   Your account even to turn Show debug tools on; a second tab was the way round. Again for VQ103 (2026-10-29): the guest's page has no tray, so a signed-in guest cannot cast from it (D371 §4) but from a second tab. Its device check, with it (the owner, 2026-10-29): a guest by link alone, not signed in, closes the
   tab and opens the same link again in that browser, and is back as the same member; from another
   browser the spent link is refused.

## Seen on staging (the owner, 2026-10-28)

- Fixed in the batch: the banner covers half the trail; from the Play page no one press reaches the info
  page ("the last part of the trail should also be selectable to get back to the info page"); the choice
  boxes look like plain buttons.
- Then, revising D367 §2: "it always leaves a trail that goes back to the info page before going any
  further back ... because you go through an info page to get to the playback page in all cases,
  including episodes. ... The only exception is direct-play links, such as continue watching. These
  leave a different 'back' trail that goes directly back to the continue watching page." A Play page's
  parent is what plays' own page on every device; Back stays the history. Built in the batch.
- **Next batch:** "I'm not sure I like having the season selection done as a choice box. Jellyfin actually
  shows posters for the seasons in the season page" (revises D367 §9's picker). And "these
  unlikely-to-be-used UI items should be under a vertical '…' selector so that they don't clutter the UI"
  (Identified incorrectly, Rescan this, ordering, …); "when selecting an item from a choice box (which is
  modal), if that selection also opens another modal, the new modal should not stack on top of the other
  modal; they should swap in/out, such that a single click outside of any modal view dismisses and returns
  the user to normal functionality" (stacking today, if found, is a bug for this batch).
- **Research now:** the playback UI across competitors and experts, desktop, touch and d-pad; "the volume
  widget should be vertically oriented" (playback-ui-2026-10-28.md).

## The player's controls (answers to playback-ui-2026-10-28.md, the owner, 2026-10-28)

1. **On a computer, the volume is a vertical pop-up** from the sound button.
2. **No volume slider on touch devices** (as Jellyfin); the device's own buttons.
3. **On a computer, Up and Down change the volume**, as the others do; Tab enters the bar.
4. **On a TV with the bar hidden, OK pauses**, as the others do.
5. **One 10 s step** everywhere.
6. **The two panels** (asked what they are; the lead: "Audio & subtitles" — the audio track, the subtitle
   track and language — and "Settings" — dialogue boost, levelling, the sound and subtitle offsets — opened
   from the player's own bar, so a remote reaches them in full screen): **yes, move them** (the owner).
7. **Thumbnails while seeking: eventually**, on the wanted-features list; earlier "if it can be pulled out
   as part of the revamped single video file scan".
8. **Holding Left or Right speeds up by growing steps** (Kodi's), not by rates.
- **Next batch (the owner, 2026-10-29, VQ116's run):** a cast's remote on the sender has no sound
  controls: dialogue boost, levelling and the sound track can be changed only on the receiver itself,
  which a Fire TV's remote reaches with difficulty. The sender's remote gets them, sent to the receiver
  as commands. (Related: the item below on what the controller side should show.) While the receiver
  is controlled its own controls are hidden too, so today the sound changes only after taking control
  back (the owner, VQ116's run).
- **Next batch:** "when playing back via remote control: The controller side should get information
  about playback, such as the transcoding and bandwidth choices" (the receiver's decision, quality and
  link, shown on the controlling device).
- **Next batch (a bug, found 2026-10-28):** a series with no name or year on staging (series 2405,
  "Some Fathers Do Mind 'Em"): its folder, `Some.Fathers.Do.Mind.Em.Complete.Series.Plus.Christmas.Specials.DVDRip.H264(...)`,
  is taken for a specials folder (`identify.isBonusFolder` matches "Christmas Specials" anywhere in the
  name), so `seriesFromFolders` passes over it and names the series "", and TMDB finds no match for an
  empty name. Fix: a bonus folder is one whose whole name is specials or extras, and a series' name
  never comes out empty while a folder above it has one. The owner adds (2026-10-28): "If a folder
  contains a bunch of subfolders that look like seasons, it must be the parent folder of a series, so
  it shouldn't be subject to the specials naming rule; only its children." The parser reads one path
  at a time, so the path's own evidence is used: a folder holding a season folder ("…/Series 1/…") is
  the series' folder, never its specials; where a scan has the siblings, several season folders under
  one folder say so the more surely.
- **Next batch (the owner, 2026-10-28, "yes, file it"):** the banner's Libraries icon opens a menu
  listing the person's libraries, rather than going to the Libraries page with one chosen: "make the
  libraries icon a menu that lists the libraries under it". Built on the shared chooser/menu widget of
  this batch. As agreed: (1) with one library, the icon goes straight to its page, no one-item menu;
  (2) "All libraries" stays as the menu's last item; (3) the library chooser at the top of a library's
  page goes (its view and sort choosers stay), changing library being the icon's, the trail leading
  back to the library's list; (4) on a TV it closes as any chooser does (Back, Left/Right) and scrolls
  within its height cap. A household with many libraries (15 or more) may later want a grid page instead.
  **Built 2026-10-30**: `web/apps/web/src/app-shell.tsx`'s `LibrariesMenu` over `drop-down.tsx`
  (`libraries-reached.ts` the one read of the person's libraries, shared with the library's page), the
  library's page's chooser removed and its heading the library shown (`library-browse.tsx`); tests
  `libraries-menu.test.tsx`, `library-browse.test.tsx`, `input-pages.browser.test.tsx`; on a TV, VQ181.
- **Next batch (the owner, 2026-10-28):** "When I send out an invite link for someone to create an
  account on my core, it only shows the invite link once ... it would be nice to be able to see the
  currently issued link." Chosen: "Keep invite links like Watch Together links" (D267 §5): the core
  keeps each unused invitation link (today only its SHA-256, `invitation_link`), and the people page
  shows it again with a Copy button until it is used or withdrawn. A copy of the database can then use
  an unused link; withdrawing or using it ends that. Unused links expire after 7 days (the owner,
  2026-10-28: "unused invitation links expire after 7 days").
- **Later (the owner, 2026-10-28, "Some other issues to deal with later"):**
  - "When I first connect with a fire tv, I get a message 'Your browser is asking to reach devices on
    your network', but the browser doesn't surface anything asking for permissions." The local-network
    note (local-network-note.tsx, D354) shows on Silk, which asks nothing: the note should show only
    where the browser will ask (or say what to do on a browser that asks nothing).
  - "Adding a device should be a role privilege." (To settle when taken up: which act it means, signing
    a new device in by a code or a link, and the default for each role.)
  - Translated titles: "We need a way to solve the problem of translated titles. For example Blind and
    Ugly (tt0567890) and I Just Never Did (tt0678901)." A file named by an English translation of a
    foreign film's title finds no match by its original title: search TMDB's alternative and translated
    titles too, or the English title.
  - "Leaving KT (tt0890123) is an interesting problem because the scan's extracted title puts a space
    between the D and C." A dotted name's initials ("Leaving K.T.") come out spaced; a run of single
    letters should join back ("D C" → "DC", as "S.H.I.E.L.D." would).
  - "For the name scanner, we should as a last resort allow them to manually enter in an ID, for example
    tt0789012. The system brings up the quick data for verification and the user confirms." Entering an
    id or a TMDB/IMDb address exists since 2026-10-21 (candidate-chooser.tsx's PastedLink,
    `POST /items/{id}/match-by-link`), but it matches at once. Add the check: the id looked up first,
    its card shown (poster, name, year, kind, a line of overview, as a candidate is), and the match
    made only on the person's confirm.

## Reaching production from outside (the owner, 2026-10-28)

- **Next batch (the owner: "It's fine for now"):** a phone in Australia, on mobile data, opening an
  invitation to nas.example was told it could not reach the network; on Wi-Fi it got in. Production has no
  IPv4 way in from outside: its config has no tunnel (`domain = ""`), so no relay either (D314 §4), and
  the home is DS-Lite, so a network without IPv6 cannot reach it. Two items: (1) put nas.example on the edge
  VPS's tunnel (orpheum-ops edge runbook; peer 10.77.0.2 was kept for it; the VPS step is the owner's,
  over his laptop's WireGuard), after this batch's nas.example deploy; (2) a page that cannot reach its core
  reports why to the discovery (its network, whether it has IPv6, the ways tried) so the failure is seen
  on prod-api, and tells the person in words ("this network has no IPv6, and the server has no IPv4 way
  in yet"). Nothing reached nas.example's log from the failed attempt; the core cannot log what never arrives.

## A black blip at the start of a film on the Fire TV (the owner, 2026-10-28)

- **Next batch (the owner: "something I can live with, but I'd rather it not be there"):** A View to a
  Kill and The Kid showed 2–3 black frames near their start on the Fire TV (Silk). Both
  switched to their top quality at the moment playback began (12205 kb/s at +0.02 s; 15635 kb/s at
  0 s), from a low first rung the server chose at 70% of the remembered link (startsOnLadderKbps 2489
  and 2000, rememberedLinkKbps 11923 and 9281: Bond's ladder has nothing between 2.5 and 12.2 Mb/s).
  Films whose switch came before playback began (Marrowgate −0.75 s, file 5670 −1 s) showed none: a new
  init segment resets Silk's decoder. Fix: the player, which now receives `rememberedLinkKbps`,
  chooses its first rung itself before loading anything (D257's start choice moved or matched on the
  page), so the first chunk is of the rung it plays and no switch lands at the start.

## Reaching the core at home, and WebRTC's cost (the owner, 2026-10-28: "go with your recommendations on all four")

From research/network-stack-audit-2026-10-28.md and research/webrtc-throughput-review-2026-10-28.md
(the old Fire TV: 15–30 Mb/s over data channels, 190–224 Mb/s over HTTPS at the LAN name):
1. **Next batch, a decision first:** at home, the whole tab moves to the core's LAN origin, the sign-in
   carried over (D308's hand-off, a variant taking the signed-in session), asked before the first move
   (a router blocking public names for LAN addresses shows the browser's error), WebRTC kept as the
   fallback when the LAN name does not answer. The owner is "not against" it; write the D-proposal.
2. **This batch:** the efficiency audit's cheap fixes (research/web-client-efficiency-audit-2026-10-28.md
   findings 1, 2, 3, 4-defer-up-next, 5, 7).
3. **Next batch:** WebRTC tuning for Chromium browsers, tested on the Mac in Chrome (target: Firefox's
   figure on the same Mac) and the Fire TV: pacing in the core with larger per-channel queue limits, the
   window-growth step for direct paths, and a log line of each connection's SCTP figures at its end.
4. **Next batch too** (the owner, 2026-10-28, "yes", revising "only if the LAN origin is not enough"):
   asking for an unused video stream so Chrome enlarges the data channel's receive buffer (2.5–3× on a
   desktop in the review).
5. **Next batch, to look at:** in Safari the relay's leg ran at 80 Mb/s against 108 for the page's own
   requests, its first byte after about a second: whether Safari needs the relay at all on the LAN name.

The devices' own Throughput runs (staging, 2026-10-28, 32 MiB, Mb/s, app requests / film pieces):
Mac Safari over HTTPS at the LAN name 108 / 80; Mac Firefox over WebRTC 106 / 102; Mac Chrome over
WebRTC 23 / 18; old Fire TV (Silk) over WebRTC 36 / 29, over HTTPS at the LAN name 190 / 204. Only the
Chromium browsers are slow over WebRTC, on a fast Mac as on the stick: Chrome's data channel (its fixed
64 KiB receive socket buffer with pion's unpaced bursts), not the device's processor, which the review
had leaned on.

## Example text in text boxes (the owner, 2026-10-28)

- **Fixed in the batch (the owner: "fold it into this batch"):** "The example text in text boxes is too bright. At first I mistook it for a pre-filled
  field and tried to delete its contents." Placeholders are drawn in `--color-muted` (#a3a3ad on the
  field's #1c1c20, web/apps/web/src/base.css `input::placeholder`), the same grey as secondary text.
  Give them their own token, clearly dimmer than any real value (and perhaps italic), checked on the
  TV at a distance as well as on a computer.

## A first film after an update freezing on the Fire TV (the owner, 2026-10-28)

- **Next batch, to investigate:** Nosferatu (file 5671), the first film cast to the Fire TV 22 s after its
  page reloaded onto a new staging build (18:23:39), froze twice for seconds while its sound played on:
  135 frames shown in a 10 s report where 240 were due, then 25 of 186 dropped; its first data took
  7.4 s (pieces not yet made). Its picture is ordinary (H.264 High, level 4.0, 4 reference frames,
  1.8 Mb/s, 1920×1036). Played again later, and after plain reloads, it dropped 2% or less; earlier
  first films after a deploy (13:22, 15:33) were also the worst of their runs. So a cold start after an
  update — new code to compile, a new service worker installing, the server's pieces cold — not the
  film. Find what the page and service worker do in the minute after an update and defer it while a
  film starts; a start after a plain reload shows small hiccups at most (3% of frames dropped in its
  first 8 s, file 5670, 18:39).
- **Looked into (2026-10-29), nothing removed yet.** The run of 08:13 to 08:36 (Nosferatu, device 60, the
  discovery's page on WebRTC, the first film after the 02:43 deploy) dropped frames in 11 of its 138
  reports, 3 to 7 of about 240 each, eight of them between 08:19:29 and 08:21:18 and none in the 15
  minutes after; the drops follow neither the loading (08:13:58 to 08:14:48 loaded 5 to 7 MB per 10 s,
  about 5 Mb/s, with no drop; the rest about 2 MB per 10 s, the film's own rate) nor any other
  request in the core's log. What the page does while a film plays on the TV, from the code, after
  the audit's fixes: the frame timing samples 48 frames at each `playing` only; the bar reads no
  `timeupdate` while hidden; nothing else renders on a tick (the quality poll's `setState` gets the
  same hls.js level object and bails; Up next's `useEnding` runs only for an episode); the timers
  left are the controller's 500 ms look, the standstill and link meters' 1 s, the report's 10 s and
  `GET /me`'s 60 s (code-debt.md), each a few reads; the console logs nothing per chunk; no endless
  animation runs on the Play page; and the service worker caches nothing ahead (its `install` only
  skips waiting): under a loader it fetches each client file through the page's WebRTC and hashes it
  at its first use, so after a deploy the film page's and the Play page's scripts (hls.js's 577 KiB
  among them) come over the link and are compiled afresh at the first film's page and Play, which
  can explain a first film's first seconds, not its twentieth minute. What does run for the whole
  film and is not cheap: **its sound through Web Audio** (`isThroughWebAudio=true`, levelling on, as
  dialogue boost was off), a second audio path on the stick for the film's length (audit, "Not
  covered"); **hls.js's demuxing on the main thread** (audit finding 6); and the WebRTC relay, each
  chunk's bytes crossing the main thread twice. Next: on the Fire TV, the same film from the same
  point with levelling off and on, compared by `a player dropped frames`; then finding 6's worker,
  measured the same way. Warming the Play page's scripts while the person browses (an idle
  `import()` under a loader) would move a deploy's fetch and compile out of the first start, if the
  first seconds are what the owner sees.

## From the owner's VQ89 check (2026-10-28), next batch

- **The watched check on an episode's row** sits beside it; the owner: "I think the checkmark should be
  over the screenshot" (the episode's still, as on a poster's tray).
- **The check inside a chooser's items** "looks awful"; moot once seasons are chosen by posters (the
  season posters item above), so not fixed in the chooser.
- **Fixed in the batch (the owner: "yes, fold it in", 2026-10-29): the scrub bar:** "Clicking on the
  scrub bar doesn't move the marker until the movie actually starts playing from there, which makes it
  feel like it didn't work." The marker (and the time shown) should move to the pressed place at once
  and stay there while the seek loads. With it the owner's Fire TV report of 2026-10-29: a held Right
  seeked the film at each of the key's repeats (~150 seeks in a minute), the marker moving only as
  each landed, so "I have to guess how long to hold left or right down". Now a seek is shown before it
  is made (`web/apps/web/src/seek-ahead.ts`): a held or repeated arrow moves the marker and time
  alone, 10 s steps, then 30 s from 2 s on and 60 s from 4 s, and the film seeks once, there, half a
  second after the presses rest (a key's after its release); a press on the timeline shows its place
  at once; the place shown stays until the seek lands; a cast remote's drags and Back 10 s / Forward
  10 s presses in a row are one command the same way, so a leading player's or a remote's seek is the
  one final seek. VQ85 on the Fire TV.
- **A full VoiceOver pass** once the UI is final (the owner, 2026-10-28), VQ89's "Watched" among it.

## From the lead's overnight checks (2026-10-28), next batch unless the owner folds one

- **Owner to decide: a given-up chunk's sample (D352).** A chunk given up counts as a sample of the bytes
  that came over the time waited, at least 1 B; one such sample among four at 16 Mb/s makes the harmonic
  mean about 20 bit/s for the next five chunks. Nosferatu on the Fire TV fell from 9018 to 696 kb/s 2.3 s into
  play on a 69 kb/s estimate and stayed 75 s (staging, 15:33:05). The switch's note now carries the
  controller's `why` and `linkSamples`, so the next case says which sample did it.
- **Owner to decide: a film's first seconds before its second keyframe.** In 17 films on staging (14
  films, 3 episodes) the first keyframe sits before the film's start, discarded, and the chunk grid starts
  at the next one, 1–10 s in, so those seconds never play (VQ119's agent, 2026-10-28).
- **Owner to decide: the series ordering** stays a native select (it saves a preference, with its own
  busy and failure states) or becomes a chooser like the others.
- **The Mac's Firefox refused A Woman of Paris's own HEVC picture** (`bufferAppendNoProgress`, nothing held)
  where Firefox on Linux plays it; the fallback to 800p worked. Find why, and whether other HEVC films
  do the same there.
- **The remembered link is lost at each deploy** (`rememberedLinkKbps=0` on the first start after one);
  and a start waited 10 s on a confident estimate of 753 kb/s (18:24:01) where the film before had
  measured 17 Mb/s.
- **After a throttle is lifted, a desktop playback stayed at 360p for 50 s and once stood still at
  1231.3 s for 50 s** (headless Chromium with DevTools throttling; a second run played on). Partly
  fixed in the batch (2026-10-29): held low after a drop, it had loaded 450 s of 360p ahead, as hls.js
  loads ahead by bytes; a playback now loads at most 60 s ahead at any quality (the 60 s for the owner
  to confirm). Its step up after a drop waits D352's hold, one quality at a time.
- **Fixed in the batch (the owner agreed the rule, 2026-10-29):** a drop-down in a short window (1920×600)
  moved up to stay on screen, its choice no longer over its button's centre, with nothing to scroll.
  Where the screen's edge would cut a list opened at its button, it is now cut there and scrolls within
  itself, as a select's list, its choice staying over the button, while the cut list keeps at least half
  its own height; past that it is moved to stay on screen as before (`drop-down-place.ts`; VQ97 (9)).
- **Fixed in the batch (the owner: "fold in whatever fixes will help", 2026-10-29):** a WARN "a request
  that only reads wrote to the database" from `forgetAttempt` when the boost stopped an own-sound run
  (20:57:08), where every other such line is DEBUG with a reason. The store's list allows it, with why:
  it is the cancel's hook, as the cancel is.
- **Fixed in the batch (the owner: "fold in whatever fixes will help", 2026-10-29):** pages kept asking
  every 5 s for a Watch Together session that no longer exists (`GET /together/73`, 404, staging
  2026-10-28): the host's screen said the session was over but kept the tab's membership, so its
  background check asked again. It now forgets the membership once the server says the session is over.
- **Fixed in the batch (the owner: "fold in whatever fixes will help", 2026-10-29):** a remote opened
  just as its cast ends said only "The cast is over." Its hello reached a session already gone ("no such Watch Together session"), so it never hears that the receiver took control
  back, though the core logged `reason=takenBack` the same millisecond (browser tier, 2026-10-29). The
  refusal could carry a recently ended session's reason. It does now: for a minute after a cast ends,
  a hello with one of its credentials is sent the same `ended` message, with its reason, before the
  socket closes as for no session.

## The SWAG audit's research, next batch (the owner agreed the recommendations, 2026-10-29)

- **Starts, stalls, chunks and the remote** (research/starts-chunks-remote-2026-10-29.md): the slow-start
  rule at 8 s counting only waits on the link; Watch Together's refill after a stall sized to about a
  minute of play; merging small back-to-back chunks into one link sample in place of the 128 KiB floor;
  the held seek moving by time held, scaled to the film's length; the seek rest at 750 ms; "Still
  watching?" with Netflix's 90-minute floor on TVs; WebRTC started 250 ms in when no direct address has
  answered (Happy Eyeballs), each address's time logged. The owner agreed the report (2026-10-29). Two numbers want the owner at the TV: the Fire
  TV's key-repeat rate and a deliberate double press's gap (the lab's button test). Its three bugs are
  folded into this batch.
- **Sharing the upload and the ladder** (research/upload-and-ladder-2026-10-29.md): the "slow" line at
  110%, the crowding controller's step at 20 s, sending a member away only after 60 s and only past the
  whole upload, the presets (x264 `faster`, Quick Sync `medium` after a speed check on nas.example, NVENC
  `p6`); the simulation and the VMAF plan on gpuhost. The owner's answers (2026-10-29): (1) yes, the
  house's other traffic gets some sharing priority, and may send a Watch Together member away; (2) no
  fixed reserve (the admin sets the upload Orpheum uses), but detect contention on the line and give way
  for a while, as a setting that can be turned off; (3) yes, a TV keeps its quality ahead of a phone when
  lowering; (4) Quick Sync's settings were all accidental: choose by picture quality and UX (the report's
  `medium` after the speed check on nas.example).
- **Climbing back after a recovered link** (VQ86's run, 2026-10-29): one quality a minute after drops;
  with clean samples after a throttle is lifted, climb as far as they agree (D352 §3 allows it).
- **"Play the original file" as a debug tool** (the owner, 2026-10-29: "I'm not even sure I want to
  expose that option to the users tbh. It's useful as a debug tool though."): to decide whether it moves
  behind an admin or Diagnostics switch (diagnostics-design) rather than sitting under every player. It
  keeps reloading the source when used; the player's common switching path leaves it out. **Decided
  (the owner, 2026-10-29), next batch:** a "Show debug tools" option that shows every control on any
  screen marked as a debug tool, hidden otherwise; "Play the original file" is the first so marked.
  Candidates to mark with it: the lab's device tests, the picture-timing and sound-offset lines, the
  tracks note's technical detail.
  **Pulled into this batch (the owner, 2026-10-29)** as a per-person option, with Watch Together's
  leader view (who leads, this device) as its first readout.
- **Choosing another audio track (language) in place** (the boost agent, 2026-10-29): it still asks the
  server for a new decision, which restarts playback; a switch in place needs the API to give each
  sound its own carried gain and levelling (D226, D368), the owner's call. The boost now goes through
  the player's common switch.
- **Decided (2026-10-29): D373, no one's stall holds a Watch Together group** (a leader stalled past a
  short grace passes the lead; the group waits only to start). This batch (the owner, 2026-10-29).
- **Next batch (the owner, 2026-10-29, VQ101's run):** seeking Thimbleford (file 5680) cast to the Fire TV,
  twice the film jumped back and then played 2-3 s at 2-4x speed with no sound before going on normally.
  Log, staging 22:35:23-27 UTC, the receiver (device 21 / app device 68): a seek to 863.8 s, one to
  875.2 s, then 22:35:27.097 `seeked` back to 866.2 s (`fromPositionMs=876522 toPositionMs=866181`), a
  seek the remote did not send; find who sought back (a stale seek-ahead target? the leader's own
  undo, `GroupLeader.undo`?) and why the picture then raced its sound.
  Again in VQ110's run, The Marrowgate Family (file 5908): the remote's second jump, 22:38:56 UTC, to 3549.2 s
  (`readyAfterMs=4033 pictureAfterMs=1431 soundAfterMs=150`), then the same fast silent play.
- **Next batch (the owner, 2026-10-29):** the core and its discovery must find each other again whatever
  order they restart in ("It should be able to figure itself out either way"). Staging, 22:19:56: the
  core restarted, then connectd; the core's signalling said it connected (22:19:58), yet every offer then
  timed out at connectd ("wait for the core's answer: context deadline exceeded") until the core was
  restarted again (22:21:38). A signalling link that answers nothing must be noticed and redialled.
- **Next batch (the owner, 2026-10-29, VQ139's run):** Firefox took ~45 s to come back after the Mac's
  Wi-Fi returned; Chrome far less. Find why (Firefox's ICE state machine, D-numbers in idiosyncrasies.md's
  Firefox entry; the re-offer's backoff) and bring it in line.
- **Next batch (the owner, 2026-10-29):** a Play page navigated away to another site should give up the
  lead at once, as closing the tab does: on `pagehide`, close the session's sockets (the page may sit in
  the back/forward cache with them open, and the core waits out its silence limit), and reconnect on
  `pageshow`.
- **Next batch (the owner, 2026-10-29, cosmetic):** the notice that a Watch Together session ended is a
  modal popup (the shared ModalDialog), not the top-of-page note.
- **Next batch (the owner, 2026-10-29, cosmetic): a title page's misidentification actions, one entry.**
  "Identified incorrectly", "Rescan this" and "Report filed wrongly" all stem from a wrongly identified
  file: keep only "Identified incorrectly", which opens a modal (the shared ModalDialog) offering
  Rescan, Enter manually, and Report this. And Play is the page's first button, not its last.
- **Next batch (the owner agreed 2026-10-29): a page says when developer tools bypass its worker.**
  With DevTools' "Bypass for network" ticked, a client script's failure already says so
  (`loader-client-failed.tsx`), but posters and films 404 silently (idiosyncrasies.md, the Bypass
  entry; A51). At start, the page asks the worker for a probe address only the worker answers; a reply
  from the network means it is bypassed, and the page shows one line: "Developer tools are bypassing
  the service worker; media won't load." Reported once to the core's log.
- **Next batch (found 2026-10-29, VQ126's fix; the owner agreed):** the `live:` socket (`web/apps/web/src/live-connection.ts`)
  has its own ping and reconnect, separate from the session connection's, and so still waits out its
  silence limit after a phone is unlocked (the S23: live socket reconnected 20 s after the unlock).
  Route it through the session connection's shown-again check (`web/packages/sdk/src/shown-check.ts`),
  one mechanism for both (the owner: one kind of decision, one path).
- **Not scheduled — the major UX rewrite** (the owner, 2026-10-29): deferred until enough features are in,
  so it is done once, coherently; until then only small UI changes, and fixes where things get too ugly.
  The Watch Together page's way back to watching (VQ104: the host missed it) is one of its inputs.
  Another (the owner, 2026-10-29, "beginning to think"): Watch Together and Cast as pop-up modals over
  wherever the person is, not pages to navigate to, being "functions of playback circumstances, so you
  shouldn't have to navigate away from where you are just to activate their controls". Would replace
  D364 §13's sign-to-page table; a signed-in guest's own page and the cast receiver are unaffected.
  The modal still needs "Back to the playing film" when Watch Together plays on this device (no
  cast): someone browsing other films while the shared one plays has no easy way back otherwise (the
  owner, 2026-10-29).

## Security (the owner, 2026-10-30)

- **Done:** pion/stun v3.1.5 (the TURN relay's remote panic), on staging and nas.example; check-vulns first
  in check-pr and the nightly run; every Action pinned by commit (research/security-pins-2026-10-30.md,
  research/github-integrity-2026-10-30.md).
- **This batch, the owner's part first:** signed tags (github-integrity recommendation 3). The owner
  makes an SSH signing key on a FIDO hardware key (`ssh-keygen -t ed25519-sk`), with a second key as a
  spare; then the lead adds `allowed_signers` on nas.example and prod-api, outside the clone, and
  `deploy/release/build.sh` runs `git verify-tag` before it builds.
- **This batch, to the owner for a yes:** the audit's "in the batch" bumps: Vite 8.3.5, vitest 5.0.5,
  happy-dom 21, x/net v0.62.0, Node 22.23.4, a fresh Debian digest, a lockfile refresh, FFmpeg 8.1.3
  (jellyfin v8.1.3-1), and staging's Mailpit 1.30.4.
- **Before the public release:** each release's image digest signed with the owner's key and published
  off GitHub, with a bootstrap install on orpheum-app.com that checks it and pulls by digest (the owner
  to choose ssh-keygen or cosign, and whether the household install command changes; recommendation 4);
  a second remote the owner controls, compared with GitHub's refs, which nas.example clones from
  (recommendation 5); the connect package signed; the image and FFmpeg reproducible, rebuilt on gpuhost
  before signing. The account's own hardening (hardware keys as the only 2FA, old keys and tokens
  pruned, no force-pushes, `v*` tags for the owner alone) is the owner's to do in GitHub's settings.
- **Done (2026-10-30): CVE-2026-98432** (a Linux TCP use-after-free that Go's default MPTCP listeners
  expose to remote code execution; golang-announce): `godebug multipathtcp=0` in Orpheum's and
  connect's go.mod, tested (`TestSockets_UseNoMultipathTCP`), on staging and nas.example; the prod connect
  package `connect-production-6f0ec54-276ef290.run` waits for the owner. The fix proper is the hosts'
  kernels: nas.example's Proxmox host (7.0.14-25-pve), prod-api, the edge VPS, the owner's to update. The
  Docker daemon's own embedded DNS listens with MPTCP too (seen in staging's container).
- **Once in beta (the owner, 2026-10-30):** a way to ship a security fix in hours while a batch is
  half built: a fix branch from what production runs (nas.example's commit, the last connect package), its
  own narrow gate (the fix's tests, check-vulns, the build), deployed, then merged into the batch.
  Tonight's pion/stun and MPTCP fixes were made on main with the batch's other work in it.
- **If the restart-order fault comes back (the owner agreed 2026-10-30):** the core's link to connectd now
  finds a dead connection within about 48 s, but the staging fault's cause was not seen; if
  a proxy answered the core's pings for connectd, only a ping connectd itself answers would find it: a
  `discovery/single` protocol change, with connect's re-pin and a deploy order.
- **Later (the owner agreed 2026-10-30): D380's cap, everywhere.** The debug link cap does not reach a
  film played as its own file (the browser loads it) nor the film page's warm-up before a Play page has
  mounted in the tab; and its drop leaves the loader's WebRTC connection open (refusing what is sent),
  as closing it needs the loader's interface.

## From the owner's device round (2026-10-30), next batch

- **Who am I signed in as: folded into the 2026-10-30 batch at the owner's word ("so that it's
  easier for me to test").** "There's nothing anywhere to tell me who I'm logged in as. We should at
  least have this on the account page" (the owner, Mac Firefox as dan, VQ181). The account page now
  says "Signed in as ‹name›" under its heading. The menu's top line already holds the bare name, in
  small grey type, and went unnoticed; whether it should say "Signed in as" too is the owner's.
- **VQ181's one-library case has no account to show it.** Every staging person (dan, s23,
  lead-tests) has four libraries and admin five, so "the icon goes straight to its page" was
  checked by its tests only; a device check needs a role with one library.
- **The decoder's own limit (D382's sequel; the owner, 2026-10-30).** File 5664 (HEVC, 15.6 Mb/s) on
  the Fire TV dropped 6 of its first 101 frames, H.264 file 5766 2 of 185. Design, after a one-off
  probe page on the Fire TV that reads MediaCapabilities `decodingInfo` (smooth, powerEfficient) and
  Compute Pressure for each of our rungs: a decode ceiling beside the link's (a rung dropping more
  than a share of its frames is capped for that device and codec, remembered by the server as the
  link and steadiness are), and, the owner's: "consider dropping to a simpler codec without changing
  quality, even if it increases the bandwidth requirement (provided we are confident the pipe can
  handle it)", e.g. HEVC to H.264 at the same resolution. Amends D352.
- **The Fire TV's focus stuck once on a series' season row (the owner, 2026-10-30, VQ180's run).**
  The first time a series of many seasons was opened that day, season 1 highlighted and no arrow moved it, until after
  many presses it moved on; not seen again. The core's log has the TV's live connection silent from
  about 16:22:45 UTC and dropped at 16:23:28, which fits a frozen page (many season posters decoding
  on a 1 GB device is a guess), but the owner cannot place the time. The owner: pass, and watch for
  it. Owed so the next one explains itself (digging means missing logs): a page's main-thread
  freezes of a second or more noted on every page, not only while a film plays
  (`web/packages/player/src/main-thread.ts`), with what the page was doing.
- **Changing the audio track restarts the whole film (the owner, 2026-10-30, VQ175's run).** "When I
  switch audio tracks, the whole video stops and restarts. Is there a way to make this more
  seamless?" Each playback carries one sound rendition (`ladderSound`), so another track is a new
  playback from where it stood. Proposal: the playlist lists the film's sound renditions as
  alternate audio (EXT-X-MEDIA), and a track change switches the sound alone (hls.js's `audioTrack`),
  the picture playing on and the new track's chunks made from where it stands; the boost and
  levelling likewise where they are renditions. A design for the owner (core playlist, player).
- **Changing the audio track restarts the whole film (the owner, 2026-10-30, VQ175's run).** "When I
  switch audio tracks, the whole video stops and restarts. Is there a way to make this more
  seamless?" Each playback carries one sound rendition (`ladderSound`), so another track is a new
  playback from where it stood. Proposal: the playlist lists the film's sound renditions as
  alternate audio (EXT-X-MEDIA), and a track change switches the sound alone (hls.js's `audioTrack`),
  the picture playing on and the new track's chunks made from where it stands. A design for the
  owner (core playlist, player).
- **A link refused in another browser should say so before the join form (the owner, 2026-10-30,
  VQ172(3)).** "This works, but ideally it should tell them right off the bat rather than letting
  them try to fill out the form uselessly first."
- **D384 §3: no device in the same Watch Together session is a cast target** (the owner, 2026-10-30,
  after casting from the S23 to the Mac hosting the session ended the cast).
