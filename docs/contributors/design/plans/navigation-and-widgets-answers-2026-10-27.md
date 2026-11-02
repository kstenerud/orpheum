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
   drill-down, for example Series / [a series] / s1e1 blah blah." The lead proposed both, and the owner
   agreed: on computers and phones a trail (`Series › Show › S1E1 Pilot`, each part a link, the one
   before last the Up); on a TV only "‹ Show", one stop for focus.

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
  a sitcom whose folder read `Show.Complete.Series.Plus.Christmas.Specials.DVDRip.H264(...)`),
  is taken for a specials folder (`identify.isBonusFolder` matches "Christmas Specials" anywhere in the
  name), so `seriesFromFolders` passes over it and names the series "", and TMDB finds no match for an
  empty name. Fix: a bonus folder is one whose whole name is specials or extras, and a series' name
  never comes out empty while a folder above it has one. The owner adds (2026-10-28): "If a folder
  contains a bunch of subfolders that look like seasons, it must be the parent folder of a series, so
  it shouldn't be subject to the specials naming rule; only its children." The parser reads one path
  at a time, so the path's own evidence is used: a folder holding a season folder ("…/Series 1/…") is
  the series' folder, never its specials; where a scan has the siblings, several season folders under
  one folder say so the more surely. **Built (11-01 batch):** `seriesFromFolders` takes a folder above a
  season folder (or a season pack) for the series' own, and falls back to the outermost specials
  folder's name rather than ""; series 2405 is named at its next scan.
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
  2026-10-28: "unused invitation links expire after 7 days"). **Built 2026-10-31**: each link's
  secret was already kept sealed in its grant (`grant.link_sealed`, secrets.Store's `SealBound`, for
  the person's own link, discovery-core.md K7), so no migration: `ListLiveInvitationLinks` opens each
  waiting invitation's live links for `GET /invitations` (`InvitationBody.links`), the people page
  shows each with Copy (`share.tsx`, so a just-issued link and a Watch Together link have Copy too),
  and withdrawing, or using one link, forgets the others' sealed secrets in the same write
  (`RevokeInvitationLinks`). The used link stays sealed as its person's own link (K7), never shown on
  the invitation. The 7 days were already `registration_invitation_life_days`' default. Tests
  `TestInvitation_ItsLiveLinkIsShownAgainUntilUsed`,
  `TestInvitation_AWithdrawnOrEndedLinkIsNoLongerShown`,
  `TestInvitations_TheListShowsEachLiveLinkAgainUntilOneIsUsed`, `people-invitations.test.tsx`; VQ201.
- **Later (the owner, 2026-10-28, "Some other issues to deal with later"):**
  - "When I first connect with a fire tv, I get a message 'Your browser is asking to reach devices on
    your network', but the browser doesn't surface anything asking for permissions." The local-network
    note (local-network-note.tsx, D354) shows on Silk, which asks nothing: the note should show only
    where the browser will ask (or say what to do on a browser that asks nothing).
  - "Adding a device should be a role privilege." (To settle when taken up: which act it means, signing
    a new device in by a code or a link, and the default for each role.)
  - Translated titles: "We need a way to solve the problem of translated titles. For example [a film]
    (tt0567890) and I Just Never Did (tt0678901)." A file named by an English translation of a
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

- **Next batch (the owner: "something I can live with, but I'd rather it not be there"):** a film
  and another film showed 2–3 black frames near their start on the Fire TV (Silk). Both
  switched to their top quality at the moment playback began (12205 kb/s at +0.02 s; 15635 kb/s at
  0 s), from a low first rung the server chose at 70% of the remembered link (startsOnLadderKbps 2489
  and 2000, rememberedLinkKbps 11923 and 9281: the first film's ladder has nothing between 2.5 and 12.2 Mb/s).
  Films whose switch came before playback began (file 5908 −0.75 s, file 5670 −1 s) showed none: a new
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

- **Next batch, to investigate:** file 5671, the first film cast to the Fire TV 22 s after its
  page reloaded onto a new staging build (18:23:39), froze twice for seconds while its sound played on:
  135 frames shown in a 10 s report where 240 were due, then 25 of 186 dropped; its first data took
  7.4 s (pieces not yet made). Its picture is ordinary (H.264 High, level 4.0, 4 reference frames,
  1.8 Mb/s, 1920×1036). Played again later, and after plain reloads, it dropped 2% or less; earlier
  first films after a deploy (13:22, 15:33) were also the worst of their runs. So a cold start after an
  update — new code to compile, a new service worker installing, the server's pieces cold — not the
  film. Find what the page and service worker do in the minute after an update and defer it while a
  film starts; a start after a plain reload shows small hiccups at most (3% of frames dropped in its
  first 8 s, file 5670, 18:39).
- **Looked into (2026-10-29), nothing removed yet.** The run of 08:13 to 08:36 (file 5671, device 60, the
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
- **Done (2026-10-31), the code-side candidates.** (a) **Warmed:** under the discovery's loader, the
  home page and a cast's ready page import the Play page's code (hls.js with it) once per page load,
  5 s after they show and then at the browser's idle (`web/apps/web/src/warm-play.ts`), so a deploy's
  fetch over the link and its compile come while the person browses, not as the first film starts.
  Not measured here: the browser-mode timing tooling runs no loader nor a deploy, so the effect is
  the owner's device's to show (VQ226). (b) **Web Audio not bypassable in the client alone:** a sound
  goes through Web Audio only while a gain or a later sound is wanted (`gain.ts`), and an element once
  handed to Web Audio stays so for its life (`createMediaElementSource` cannot be undone). File 5680's
  cast (2026-10-29, 22:34:55) shows why it was on with the boost off: it started on the film's own
  picture and its own sound (`ladderSound=sound0-own`, no gain carried), which levelling raises
  through Web Audio; the step to the common quality at 22:35:13 moved it to the levelled sound
  (`sound0-96k-ownlevel`, its gain carried, none to add), but the element stayed routed. **Owner to
  decide:** on a television, with levelling on, start on the ladder's levelled sound rather than the
  film's own, so no gain is ever added and no Web Audio is made (the 96 kb/s AAC for the original's
  sound); or keep it. **The owner (2026-10-31): start on the already-levelled sound whenever
  levelling ("Level loudness") is on**, not only on a television; filed for the batch after token
  use. (c) **Left for the device (VQ226):** the first film after a deploy with the
  warming in, against one cast within 5 s of the home page (before it warms); and the same film from
  the same point with levelling off and on, compared by "a player dropped frames" (a film's own sound
  with levelling off takes no Web Audio).

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
  mean about 20 bit/s for the next five chunks. File 5671 on the Fire TV fell from 9018 to 696 kb/s 2.3 s into
  play on a 69 kb/s estimate and stayed 75 s (staging, 15:33:05). The switch's note now carries the
  controller's `why` and `linkSamples`, so the next case says which sample did it.
- **Owner to decide: a film's first seconds before its second keyframe.** In 17 films on staging (14
  films, 3 episodes) the first keyframe sits before the film's start, discarded, and the chunk grid starts
  at the next one, 1–10 s in, so those seconds never play (VQ119's agent, 2026-10-28).
- **Owner to decide: the series ordering** stays a native select (it saves a preference, with its own
  busy and failure states) or becomes a chooser like the others.
- **The Mac's Firefox refused file 5667's own HEVC picture** (`bufferAppendNoProgress`, nothing held)
  where Firefox on Linux plays it; the fallback to 800p worked. Find why, and whether other HEVC films
  do the same there.
- **The remembered link is lost at each deploy** (done: kept in the store since "feat(player,playback): the link a device remembers is a prior of its estimate, and the core keeps it across restarts"; see the 11-02 batch, item 6) (`rememberedLinkKbps=0` on the first start after one);
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
- **Next batch (the owner, 2026-10-29, VQ101's run):** seeking file 5680 cast to the Fire TV,
  twice the film jumped back and then played 2-3 s at 2-4x speed with no sound before going on normally.
  Log, staging 22:35:23-27 UTC, the receiver (device 21 / app device 68): a seek to 863.8 s, one to
  875.2 s, then 22:35:27.097 `seeked` back to 866.2 s (`fromPositionMs=876522 toPositionMs=866181`), a
  seek the remote did not send; find who sought back (a stale seek-ahead target? the leader's own
  undo, `GroupLeader.undo`?) and why the picture then raced its sound.
  Again in VQ110's run, file 5908: the remote's second jump, 22:38:56 UTC, to 3549.2 s
  (`readyAfterMs=4033 pictureAfterMs=1431 soundAfterMs=150`), then the same fast silent play.
  **Looked into (2026-10-31).** Staging's log of 22:35:08–27 shows every seek of the receiver
  (device 21 / app device 68) made on the receiver itself, as a person's: each was sent to the group
  afterwards as the leader's own command (`leaderCommand`, its seeking unmarked), and none was the
  remote's (no `sought` note, `why=command`, all session long; the Mac's panel sent no seek). The
  four, 858.8 → 851.9 (17.5), 854.0 → 863.7 (23.1), 865.7 → 875.2 (25.4), 876.5 → 866.2 (27.1), are
  each a 10 s step from where the film stood 560–575 ms before its seeking: a key's press, its release
  and seek-ahead's 500 ms rest (`seekBy` → `SeekAhead.step`). So the two jumps back were steps back
  the receiver's input layer was asked for, `left` or `seekBack`: from a key (Left, Rewind), a paired
  controller's arrow, or the media session's `seekbackward` (`hearMediaSeeks`), which nothing logged.
  Ruled out by the log or the code: the leader's undo and its carrying out of a command, the follower's
  corrections and a leader's catch-up (each marks its seek and notes `sought`; none), hls.js (its start
  seek only moves forward; its steps over a hole say `skippedBy`), the timeline's slider (it seeks to
  whole seconds; 866181 is not), a rate (no `nudged` note; nudges are a few percent, not 2–4×).
  **Done:** a seek a page's command asks says what asked it, `soughtBy` (`right-key`, `seekBack-media`,
  `timeline-bar`) on its "a player's play state changed … kind=seeked" line, so the next one names
  its source. **Fixed beside it:** a leader that sent two seeks of its own before the group's change
  for the first came took that change for another device's and jumped back to the first
  (`GroupLeader`, which remembered only its last own seek); it now remembers each. Not this run's
  cause (it would have noted `sought`), but the same jump back. **The fast silent play:** not found in
  the code: no rate is changed (none but the group's 1 here), and the picture's and the sound's
  buffers both held the place sought (`videoBuffered=850.10-896.73 audioBuffered=849.75-900.68`). The
  sound played through Web Audio (`isThroughWebAudio=true`, see the section on a first film after an
  update), where Chromium's element runs its clock by how fast the audio graph pulls its sound
  (createMediaElementSource: an element whose node is not connected does not play); a graph that
  pulls in a burst after a seek would race the picture with the sound not heard. Unverified: VQ225
  asks for the same seek with levelling off.
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

## The week's security report (research/security-week-2026-10-30.md), next batch (the owner agreed 2026-10-31)

- **Done (the owner, 2026-10-31):** `net.mptcp.enabled=0` in `/etc/sysctl.d/` on prod-api, the edge,
  nas.example's Proxmox host and LXC 102, all rebooted; the connect package with connect's own
  `godebug multipathtcp=0` (`connect-production-eb6c283-e39602fe.run`) on prod-api.
- **MPTCP off in every container (report §1).** `sysctls: {net.mptcp.enabled: "0"}` on every service
  of `internal/deployment/templates/compose*.yaml` and connect's compose files, with a test that the
  rendered compose carries it: the setting is per network namespace, and a household's kernel is not
  ours to patch.
- **The SSH keys a sandbox holds work only from the sandbox VM (report §2a), the owner's:**
  `from="192.168.1.199"` on the nas.example and gpuhost keys' `authorized_keys` lines. A hook refusing
  commands that print keys (§2b) is not wanted: the keys' reach is the defence.
- **Claude Code is not pinned (the owner, 2026-10-31):** new versions bring better models and are taken
  as they come.
- **The household image by digest (report §3).** The install script resolves its tag to a digest once
  and writes `repo@sha256:…` to `.env`; `release-image.yml` runs in an `environment: release` the owner
  approves, and a ruleset leaves `v*` tags to the owner alone.
- **No npm install scripts (report §4).** `ignore-scripts=true` in `web/.npmrc` (lfa-ponyfill then keeps
  the `fonts.json` of its tarball, not one fetched at install), the Mac dev loop checked without
  fsevents' script, and a check that fails when the lockfile gains an install-script package not on a
  short list.
- **Plex2Shell's lessons as tests (report §5).** an architecture test that home and this
  machine's port are conditions on a session, never one; a test that FFmpeg's library and output
  paths are set by the node alone, never from a request; every token check validates the token, not
  its presence; `noexec` on the node's writable mounts, to look into.
- **Before the public beta: a security update reaches households (report §7).** The discovery publishes
  the latest version with a security flag, a core's admin sees a banner, and security updates may be
  installed automatically by the admin's choice. Design first.
- **A false "developer tools bypass" note after a deploy (the owner's Mac, Firefox, prod, 2026-10-31).**
  The page probes its worker as it loads; the worker still in control is the previous release's, which
  does not know the probe, so it goes to the network, 404s, and the note says the worker is bypassed
  while media plays. The new worker takes over a moment later (skipWaiting, clients.claim), but the
  page asks again only when a poster fails. Fix: probe again on `controllerchange`.

## The 10-31 batch's security and testing work, built (2026-10-31)

- **Built:** MPTCP off in every compose service, Orpheum's and connect's, and none
  named on a kernel without MPTCP (12e606fc), which Docker would refuse; the household image pinned by
  digest at install; `release-image.yml` in the environment `release`; no npm
  install scripts and a lockfile check for new ones; being at home is never a
  credential (02c55fd1); FFmpeg's environment and paths the node's alone, and every token check
  refusing a wrong token (an earlier commit: no hole found); the false DevTools-bypass note after a
  deploy (VQ194); revert-red's R1–R3, R5 and R6, background probing
 ; security updates designed (design/plans/security-updates.md, the owner's four
  questions owed).
- **Deferred until the repository is public (the owner, 2026-10-31):** the environment `release`'s
  required reviewer and the `v*` tag ruleset. A private repository on GitHub's free plan offers
  neither; until then the environment `release` holds no publish.
- **The owner's, on the machines:** `from="192.168.1.199"` on the nas.example and gpuhost keys'
  `authorized_keys` lines (report §2a).
- **The DevTools-bypass note asks the browser for a new worker first (next batch; A53, VQ194 failed).**
  Nothing in the client calls `registration.update()`: Firefox checks for a new worker only at a
  page load, and one made during connectd's restart for a deploy (the owner's Mac, 07:32,
  2026-10-31) kept the old worker, which does not know the probe, for every in-app move after; a
  reload cleared it. Fix: a failed probe asks `registration.update()`, and probes again if a newer
  worker takes over; only the same worker failing again shows the note. **Built (11-01 batch):**
  `probeWorker`'s `isNewerWorkerComing`; VQ194 again.
- **Security updates (the owner, 2026-10-31, yes to the recommendations), next batch:** Parts A and B
  of design/plans/security-updates.md (the release record, written by the release workflow, and the
  admins' banner); Part C, the opted-in timer, once signing exists.
- **noexec (the owner, 2026-10-31, yes), next batch:** step 1 (the node's root read-only, `/tmp` a
  noexec tmpfs) first; step 2 (noexec volumes for `./node` and `./run`) once tried on nas.example.
  **Step 1 built for the 11-01 batch:** both compose templates give the node `read_only: true` and
  `/tmp` a tmpfs `rw,noexec,nosuid,nodev,size=64m` (TestCompose_TheNodesRootIsReadOnlyAndItsTmpRunsNothing).
  Nothing needed moving: the node writes only its instance (`./node`) and its socket (`./run`), and
  FFmpeg runs with an empty environment and its working directory `/` (execx), so no HOME, cache
  (fontconfig's, Mesa's shader cache) or temporary file is asked for; the drivers load from the
  image. Tried in the sandbox: a deployment written by the new templates, its node in software
  (x264, Landlock and seccomp in force), played all seven of hwplay.py's generated films (2026-10-31;
  its encoder check named `libx264` against the node's `x264`, a naming mismatch, and no film failed),
  the node's log with no read-only error; a write to `/` failed
  read-only and a file in `/tmp` would not run. A GPU's driver under the read-only root (QSV on
  nas.example, VAAPI on the amd box) is VQ196.
- **noexec, as looked into (report §5):** Landlock's EXECUTE right governs
  `execve` alone, never a `dlopen`'s `mmap(PROT_EXEC)`, and confines FFmpeg's runs, not the node. A
  noexec mount blocks both, for every process in the container. Proposed: (1) the node's root
  `read_only: true` with `/tmp` a tmpfs `noexec,nosuid,nodev`, which any Docker takes; (2) `./node` and
  `./run` as local-driver volumes `o: bind,noexec,nosuid,nodev`, Linux hosts alone (a plain bind
  mount cannot take noexec; Docker Desktop and OrbStack may not), tried on nas.example's LXC first; (3) a
  deployment test that the options are there. FFmpeg, its libraries and drivers load from the image's
  read-only paths, so nothing is expected to break.

## check-pr's time (research/test-regime-audit-2026-10-31.md), next batch (the owner agreed 2026-10-31)

- **Done: R4.** test-dom.tsx no longer imports the modules of lazy screens; `vitest related
  film-player.tsx` ran 141 files, now 27 (7 s). home-imports.test.ts keeps the shared helpers off them.
- **Gate changes, the owner's yes given:** R1, a file with a valid claim (`Verified-By:`,
  `Refactor-Only:`) takes only its cheap runs in check-pr, the nightly run probing it whole; R2, a
  stylesheet first runs its own direct browser-mode tests, in parallel, halving kept for the browser
  tier alone; R3, a TypeScript file's probe runs its direct importers, then the type check, then the
  whole related set; R5, the "nothing reverted" run of a browser-mode red reruns only the tests that
  failed, shared across the probe trees.
- **R6, the owner's direction:** revert-red probes each commit in the background as it is made, so
  check-pr mostly reads results kept from the batch's own commits.
- **No mechanical recurrence (the owner, 2026-10-31):** "We don't need audits to recur mechanically.
  I'll say when it's needed."

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
- **A ladder's H.264 rung as costly as its HEVC original (the owner, 2026-10-30, VQ177's run).**
  "When stepping back up, it eventually hit 2.0 Mb/s h264, even though the original HEVC was also at
  the same bitrate." File 5664's ladder as Chrome read it: level 3 (H.264) 3096 kb/s peak, 2096 average;
  level 4 (the HEVC original) 15635 peak, 2224 average. On average the rung costs the link what the
  original does and looks worse; it is chosen because the controller weighs the original by its peak.
  Options for the owner: the ladder leaves out a transcoded rung whose average comes within a margin
  of the original's (the server knows both), or the controller judges a rung by its average with the
  buffer ahead (D265 §4's share), so the original is reached wherever a rung of the same average is.
  Sits beside D382's sequel (the decoder's limit, a simpler codec at the same quality).
  **The owner chose the second (2026-10-30): the controller judges a rung by its average, deferred to
  a later batch** ("It works well enough for an edge case for now").
- **A checkbox on the Fire TV's sign-in submits the form (the owner, 2026-10-30, VQ160's run).** "As
  soon as I select the checkbox, it acts as though I'd pressed the log in button and attempts a
  login." OK on a checkbox is clicked by the input layer (`focus-navigation.ts` `pressFocused`) and
  its keydown default-prevented (`input-commands.ts`), so on a desktop Enter does not submit; Silk
  must do something more (its OK's keyCode 23, or a click of its own on key up). Owed: reproduce on
  the TV with the device lab's remote probe (`lab-remote.tsx`), log the OK's events on a form, then
  fix. Also seen: clearing Silk's site data sends it to bing.com, so VQ160 (the local-network note
  on a fresh visit) needed the address typed by the remote; it passed so (no note shown).
- **No way to mark a guessed film "identified correctly" (the owner, 2026-10-30).** A guessed match
  (D301, D307) can be corrected ("Identified incorrectly") but not confirmed. The store already has
  the step, `ConfirmTitleMatch` (`internal/store/queries/identify.sql`: "A person confirmed the match:
  permanent, and never moved by a later pass", D172), but no API or page calls it. Owed: an
  "Identified correctly" beside "Identified incorrectly" under a guessed title's "…" (D379's menu),
  confirming the match through an endpoint, and the guessed mark gone once confirmed. **Built
  2026-10-31**: `POST /items/{id}/confirm-match` (edit_metadata, reaching the item, as "Identified
  incorrectly"'s own requests), `identify.Chooser.ConfirmGuess` (a film, an episode placed by its name
  or a series; `ConfirmSeriesMatch` beside `ConfirmTitleMatch`), and `identifiedCorrectlyAction` in
  `match-correction.tsx` on the title's and the series' "…" while `isGuessed`; tests
  `TestChooser_AGuessConfirmedIsAPersonsMatch`, `TestIdentification_APersonConfirmsAGuessedMatch`,
  `title.test.tsx`, `identified-correctly.test.tsx`.
- **A source whose B-frames have no display times plays choppily in Chrome (the owner, 2026-10-30,
  after a cast's takeover; not the cast's fault).** File 5667 (MP4, HEVC `hev1`, `has_b_frames=2`):
  every packet's pts equals its dts (`-100099,-100099 | -50050,-50050 | 0,0 …`) where a sound file's
  differ (file 5664: `0,-2002 | 5005,-1001 | 3003,0 …`); our remux copies that, its chunks' `trun` with
  no composition offsets, and Chrome renders in "low delay mode", dropping each reordered frame
  ("Decoded frame … is out of order", freezing ratio 0.60, 14–16 fps). **The owner, option 1, a later
  batch:** detect it at scan (B-frames, yet pts = dts) and rebuild the display times "as if it never
  had one": each frame's display order is in the bitstream itself (HEVC/H.264 picture order count in
  the slice headers), so the remux re-times frames by it; to settle at design: whether FFmpeg gives
  the order without decoding (its parser) or one decoding pass at scan is needed, and whether the
  table is kept with the file's probe or worked out per chunk.
- **Sign out asks first (the owner, 2026-10-30).** "Selecting 'sign out' should open a confirmation":
  the menu's Sign out opens a dialog (the standard modal, `modal-dialog.tsx`) asking to confirm, Sign
  out and Cancel, before the session is signed out (`app-shell.tsx` `MenuPlaces`' onSignOut).
- **Signing out lands on two different sign-in pages (the owner, 2026-10-30).** "When I sign out from
  the fire tv, it goes to the discovery server's login page. When I do so from the s23, it goes to
  the core's login." The two reach the core by different paths: the Fire TV (Silk, local-network
  permission `prompt`) over WebRTC through the discovery's client origin; the S23 (Chrome, permission
  `granted`) over plain https to the core's own LAN name (`a page chose its path … over=http
  isLanNameUsed=true localNetwork=granted`). Sign out sends each to the sign-in of where its page now
  is. One place to land is owed, for the owner to choose: the discovery's sign-in (where a person
  first arrives, and which finds their server again from anywhere) is the lead's recommendation.
  **The owner chose (2026-10-30): the discovery's sign-in, except where there is no discovery**; for
  the closed beta one is assumed. Three operating modes are coming: standalone (no discovery: the
  core's own sign-in), a self-deployed discovery server, and our paid discovery server.

## The owner's answers of 2026-10-31 (the 11-01 batch)

- **Seamless audio switching: agreed** (design/plans/audio-switch-and-retiming-2026-11-01.md, Part 1,
  every recommendation). The owner asked whether a film played as it is (`DirectUnder`) could switch
  between its own tracks in place too. On the ladder every sound is its own rendition already, the
  original's copied tracks as much as transcoded ones, so any track to any track switches in place.
  Off the ladder the browser plays the whole file, and switching its tracks is the element's
  `audioTracks`, which Safari alone ships (Chrome and Firefox keep it behind a flag): there, Safari
  switches in place and the others move onto the ladder once, as a track change does today.
- **Re-timing B-frames: agreed** (Part 2, every recommendation: `dts2pts` per run, encodes too, every
  H.264/HEVC file checked, the mark in the probe record, a fixed 0.5 s margin).
- **Easy addresses: agreed, a clean break now** (the owner: "If there's a best time for a clean break,
  it's now"; sign-ins and passkeys lost are acceptable, two people use production). The owner bought
  orpheum.cc, orpheum.dev and orpheum.mov. The app moves to `orpheum.cc` itself (not a redirect), the
  service's other hosts (api., account.) under it; the website to `orpheum.mov`; `orpheum.dev` redirects
  to the website for now; `orpheum-app.com` redirects to the new homes. `orpheum-core.com` and
  `orpheum-client.com` stay (no one types them; their separation is the isolation). A TV types
  `orpheum.cc`. Owed: the owner's Cloudflare zones, tunnel hostnames and HSTS preload; connect's
  production configuration, the panel's pages and passkeys' relying party; Orpheum's install
  template's discovery address and nas.example's core configuration. **Superseded by D390** (the outside
  audit, the owner's yes): the API stays on `orpheum-app.com` and staging moves to `orpheum.dev`.
- **Built (11-01 batch): Firefox's recovery (5ae59ff9, VQ197)** — desktop Firefox fires no `online`
  (Mozilla bug 654579), and the transport cut its backoff short only on that; an offer that reaches no
  server now retries every 2 s. **Next batch, left open:** a page on a core's direct HTTPS name still
  hears the network's return only from `online` (its session socket may wait 30 s), and the direct
  mode's WebRTC signaller (`transport-choice.ts`) does not mark its failures `Unreached`. Freezes of a
  second or more are told to the core's log on every page (VQ198); they share the page's 20
  failure reports.
- **Built (11-01 batch): B-frame re-timing (VQ207).** The owner approved both departures
  from the design (2026-10-31, "1: ok. 2: ok"): a read 1 s past each run's last boundary, not 0.5 s
  (an H.264 keyframe at a boundary needed about 0.65 s), and an encode shifting the re-timed stream
  (`setts`) as well as `dts2pts`. Files already scanned are marked by D392's full rescan. **Next batch:**
  a run's last chunk is accepted though it runs past its boundary, which hid the 0.5 s fault.

## Token use (the owner, 2026-10-31), next batch

"In the next batch I want to look at token use, and how we can optimize it." The 11-01 batch's agents,
as their reports gave them (tokens, tool calls, minutes): the audio/B-frame design 214k, 94, 11; the
security updates and noexec 397k, 248, 49; the small UI fixes 429k, 341, 70; Firefox's recovery and
freezes 230k, 131, 28; the domains audit 183k, 70, 12; the domain-assumptions audit 207k, 78, 10;
audio switching 365k, 129, 38; B-frame re-timing 271k, 198, 36; "Identified correctly" and invite
links 266k (the part after a login lapse), 11. Agents' own `make check` runs, rereads of large files,
VQ-number clashes between parallel agents and cherry-pick conflicts in verification-queue.md were
visible costs. Look at: brief size and what each agent rereads (AGENTS.md, decisions, plan files),
gate runs per agent, model choice per task, the lead's own context (standing orders and memory
injected every prompt), and file layouts that make parallel agents conflict.

**Measured (the lead, 2026-10-31), from the session transcripts of 10-26 to 10-31** (one lead session,
190 agents; cost weighted as input 1, cache write 1.25, cache read 0.1, output 5):

- **Cache reads are the bill.** Every model call re-reads the whole context, so a session costs about
  (calls × average context). The lead read 1.96 billion cached tokens over 4,888 calls (average
  context about 400k, peak 895k); the agents read 5.8 billion. The agents were 77% of the cost, the
  lead 23%.
- **Contexts were allowed to grow to 1M.** Agents inherit the lead's 1M-context model, so a build
  agent ran 200–640 calls with its context climbing to 400–960k before any compaction. An agent starts
  at about 42k (system prompt, tools, AGENTS.md); everything above that is its own tool output carried
  to every later call.
- **What fills an agent's context:** many small shell reads, not big files. Of the carried cost,
  `cat`/`sed`/`head` 38% (6,048 calls, about 1k each), `grep` 27% (9,740 calls, 0.5k each), whole-file
  reads 23% (mostly one research agent's PDFs), test and gate output only 3% (outputs are already
  short; their cost is in the calls and minutes).
- **Simulated: compacting at 160k** would have cut the lead's input by about 70% and the agents' by
  about 50%; at 300k, 55% and 30%. Compaction loses detail and adds some rereads, so take roughly half
  of that as the real saving.
- **Small fixed costs:** MEMORY.md is 25 KB (about 6k tokens on every lead call); the standing orders
  (3 KB) are injected on every prompt.

**Proposed, in order of effect:**

1. **A compaction window of 200k tokens** for the lead and every agent: `CLAUDE_CODE_AUTO_COMPACT_WINDOW=200000`
   in Claude Code's environment (the user-level `~/.claude/settings.json` `env`, or `.claude/settings.json`,
   a gate file). The biggest single lever; the model stays the same.
2. **Agents sized to about 150 calls**: a build that needs more is split, or done by the lead; a brief
   names the files and the mechanism, so the agent does not search for them.
3. **Sonnet unless the brief says Opus**: `CLAUDE_CODE_SUBAGENT_MODEL=sonnet` makes the cheaper model the
   default, so Opus is a choice made per brief rather than a rule that is forgotten (it was, all batch).
4. **Fewer, fuller calls**: related reads and greps in one shell command; a section read with a range
   rather than reading a file, then grepping, then reading it again.
5. **The lead: a fresh session per phase** with a handoff memory, never a day on one context; agents
   end at `make spot`, the lead runs one `make check` at the merge.
6. **MEMORY.md pruned** to the live entries (old "READ FIRST" handoffs folded into one), about a third
   of its size.

**Agreed with the owner (2026-10-31):**

- **Environment, set by the owner through yoloai** (everything in the sandbox but the work
  directories is ephemeral): `CLAUDE_CODE_AUTO_COMPACT_WINDOW=200000`, `CLAUDE_CODE_SUBAGENT_MODEL=sonnet`,
  `ENABLE_CLAUDEAI_MCP_SERVERS=false`, `BASH_MAX_OUTPUT_LENGTH=10000`,
  `CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS=10000`. The first session after checks each takes effect.
- **Memory moves to the repos.** Operating rules go to `docs/contributors/lead.md` (or the standards
  where they belong), machines and access to `orpheum-ops`, platform quirks to `idiosyncrasies.md`;
  stale handoffs are dropped (the standing orders and this plan carry the state). `MEMORY.md` keeps
  pointers and sandbox-only facts. Always-loaded files hold pointers, never the rules themselves;
  nothing is added to `AGENTS.md`.
- **Enforced, not remembered** (gate edits, drafted by the lead as one patch for the owner): the
  standing orders injected only when they change; a context meter in the prompt hook (above 120k:
  write the handoff, ask for a restart); an agent past about 150 tool calls told to commit, report and
  stop; an agent launch refused without a model or as a fourth running agent; `scripts/token_usage.py`
  (the measurement above) run at each batch's end.
- **After the token work:** the Stop hook's fallback (a tree changed by a shell commit) runs the full
  `make check` without looking at what changed; it should classify the diff since the last pass with
  `scripts/documentation_only.py`. The owner: checks are limited to the kind of change made; a code
  problem is caught by the next code change, or the nightly run.

**The first trial, reviewed (the lead, 2026-10-31, at the owner's request).** The D394 work ran under the
limits above: a lead and two build agents, one Sonnet sharing the main tree and one Opus in a worktree.
The ledger (`token_usage.py --since 15:00`) gave 3.3M weighted tokens over 197 model calls: the Sonnet agent
35% (61 calls, peak context 162k), the lead 34% (peak 155k), the Opus agent 31% (80 tool calls, peak 139k).

- **What worked:** both agents finished well under the 150-call limit (62 and 80); the launch limits held;
  the worktree agent's work cherry-picked cleanly; the ledger answered in one command.
- **What did not:** the agent sharing the tree left half-written files, so the Stop hook's `make check`
  failed on them three times (each failure's output shown twice), followed by about 20 minutes of waiting
  loops. Agents' contexts still peaked at 139–162k. The lead's context refilled to 143k. Four full gate runs
  went to fixing line lengths that a lint of the one package would have found in seconds. After `/compact`
  the prompt hook still reported the context from before it. The ledger weights every model alike, so an Opus
  call and a Sonnet call look the same.

**Accepted by the owner (2026-10-31: "I agree with your recommendations and accept the gate changes"),**
each built with a `Gate-Changed:` trailer:

1. **Code-editing agents run in worktrees, and the lead cherry-picks** (`lead.md` §2 already said so, and it
   was not followed): `pre_agent.py` refuses an editing agent's launch without `isolation: "worktree"`
   (built: An earlier commit).
2. **The Stop hook holds back** with one line while a running agent edits the main tree, instead of gating
   its half-written files. **Dropped (the owner, 2026-10-31: "yes, drop change 2"):** with change 1 no
   agent that can edit shares the main tree, so the only uncommitted work there is the lead's own, which the
   gate should check.

   **The 120k warning, reworded (the owner, 2026-10-31, "yes go ahead"):** after a compaction the context
   restarts near 53k (system prompt, tools, `CLAUDE.md`, the memory index), and re-caching it costs about
   $0.50 against about $0.02 saved per later call, so a compaction pays back only over about 25 calls. The
   prompt hook now asks for one at the next phase boundary rather than at once; the 120k line stays. Big
   reference reads go to a cheap agent that returns only the answer (one 46k read filled half this stretch).
3. **`token_usage.py` weights by model price**, so the ledger shows what each session cost (built:
   An earlier commit; dollars at API prices, from the claude-api skill's table).
4. **The prompt hook reports no context size right after a compaction** (built: An earlier commit).
5. **`make spot`, which every commit runs, golangci-lints the changed Go packages** (built: b93bb2b8). The
   owner pointed out that "lint the one package first" was a habit, not a check.

## D391's Orpheum side, built (2026-10-31)

Built on D391 (and D390 before it), from the domain-assumptions audit's §4–§6. The outside audit of
the root keys and signed objects runs as this is written: every signature and check is behind
`discovery/trust` (OpenDocument, OpenRecord, Advance, Sign), so its corrections land there, and the
root set's rotation rules are left at the plain one until it reports.

- **Root key set and signatures** (`discovery/trust`): SSH signatures (`ssh-keygen -Y sign`, verified
  with `golang.org/x/crypto/ssh`, a vector from ssh-keygen in the tests), keys `sk-ssh-ed25519`,
  `sk-ecdsa-sha2-nistp256` or plain `ssh-ed25519`, any one root key signing. A namespace per kind:
  `orpheum-root-set-v1`, `orpheum-delegation-v1`, `orpheum-address-document-v1`,
  `orpheum-release-record-v1`. A signed object is `{"payload": "<exact bytes>", "signature":
  "-----BEGIN SSH SIGNATURE-----…", "delegation": {…}}`, the delegation only on a document an online
  key signed. A set is `{"version", "issuedAt", "keys": [authorised keys' lines]}`, the first signed
  by one of its own keys, each later one by a key of the one before, taken in order of version. A
  delegation is `{"key", "issuer", "notBefore", "notAfter"}` (Unix ms), for documents alone.
  **Waiting on the audit (the owner's addition):** a removed key never accepted again by that core
  (sticky), two sets of one version both refused and the admin told; today a removed key signs
  nothing only because it is not in the current set, and sets at or below the core's version are
  ignored.
- **The address document** (`discovery/addressdoc`) at `/.well-known/orpheum-discovery`:
  `{"document": <signed {issuer, coreApi, clientApi, loaderOrigin, clientOrigins, accountOrigin,
  homepage, support, locations, issuedAt}>, "rootSets": [<every set's statement from the first>]}`.
  Taken only for exactly the issuer asked, every address https or on this machine, `{core}` a whole
  label. `discovery/single` serves the operator's file (`Parts.AddressDocument`); the development
  discovery signs one with a key of its own at each start.
- **The core enrols by issuer** (`internal/core/discovery`): `POST /system/discovery {issuer,
  password}`; for the suggested issuer the compiled set, for any other the set the document shows, at
  first use, the System page showing its fingerprints; `discovery.json` keeps the issuer, the set and
  the document, and one with no issuer is no discovery (no adoption). Origins come from the document
  alone (the status no longer names them); hourly the core reads the issuer and every location and
  takes the newest document that holds, the last good one kept otherwise. Signatures bind to the
  issuer (`proof.Message`), so a newer document moves the API.
- **One default** (`internal/core/discovery/suggested.go`): `https://orpheum.cc`, root keys an empty
  list the owner fills (enrolment refuses it until then, asking nothing). The System page and `pair`
  take it from the core; `serviceDiscoveryUrl` is gone. `TestSuggested_IsTheOnlyShippedSourceThatNamesTheOwnersDomains`
  scans Go, TypeScript, shell, HTML, YAML and TOML outside tests, the browser tests and docs. The
  install script prints the loader its discovery named (`pair`'s `loader:` line); the lab placeholder
  and comments use reserved names.
- **Signed release records** (`discovery/release`, `cmd/release-record`): `{"issuedAt", "latest",
  "locations": [repositories], "releases": {"v0.3.2": {"digest", "security", "summary", "fixedIn"}}}`,
  signed by a root key only, served at `/.well-known/orpheum-release-record` by every discovery; the
  session's `POST /release` is gone. The core reads it at its issuer and locations, verifies against
  its pinned set, and the update command pulls `<location>@<digest>`. `orpheum verify-release` checks
  an image's digest in Go; the install script runs it on `--release-record <file> [--root-keys
  <file>]`, from the previous image on an update. security-updates.md says so.
- **The signing helper** (`cmd/discovery-sign`): root-set (add and remove), delegation, document,
  record, seal, through the ssh-agent (`ssh-add -K` loads a YubiKey's resident key), or `--key-file`.
  It does not run ssh-keygen: starting a process is `execx`'s alone (forbidigo's exec rule), so a
  hand-made `ssh-keygen -Y sign` signature goes through `seal`. Guide: docs/users/discovery-signing.md.

**Owed by the owner:** the three YubiKeys' public keys in `suggested.go`; the install script's default
check (today only with `--release-record`): `scripts/tests/test_install_script.py`'s fake docker
answering `run … verify-release` (a gate file), then the flag made the default; the release workflow's
step running `release-record` (a gate file). **Owed by connect** (a separate task), all from the
public `discovery/` packages: serve `addressdoc.Served` at the issuer's and each location's
`/.well-known/orpheum-discovery` (`addressdoc.WellKnownPath`), the document signed by an online key
under a root key's delegation (`trust.Sign`, `trust.Delegation`) or by a root key, built from its
names file; serve the signed record at `release.WellKnownPath`; verify a core's enrol and session
signatures with `proof.Verify`/`Nonces.Spend` bound to the issuer (`proof.SignedNonce.Issuer`), not
the API's URL; stop naming `clientOrigin`/`loaderOrigin` in the status (the fields are gone from
`discoveryproto.StatusResponse`) and drop `POST /release`; its tests take keys from
`discovery/trust/trusttest`. VQ210 (staging's core by issuer), VQ211 (signing with a YubiKey).

## D393's root key design, built (2026-10-31)

The audit's §3 replaces the root key set, the JSON envelope, the online key's delegation and the
discovery-signed release record of the section above; the rest of it (enrolment by issuer, one Go
file for the suggestion, the domain-name test, the install's closing line) stands.

- **Formats** (`discovery/trust`): the strict line format (header `orpheum-signed <type> <format>`,
  LF lines, keys in order, unknown lines skipped, `!`-critical ones refused, a format number unknown
  refused), a payload and its detached `.sig` of one or more SSH signatures; namespaces
  `orpheum-discovery-root-v1`, `orpheum-discovery-address-v1`, `orpheum-publisher-root-v1`,
  `orpheum-release-v1`, reserved `orpheum-discovery-freshness-v1` and `orpheum-drill-v1`. A root
  statement: `chain`, `id` (none in the genesis, whose SHA-256 in base32 is the id, `od1-`/`op1-`),
  `version`, `issued`, `key <role> <uv|-> <type> <base64> <label>`*, `threshold <role> <n>`*,
  `removed <fingerprint>`* (cumulative), `location`*; a `delegate` line is reserved and ignored (D393
  §9). The address document and release record are the audit's §3.1 lines.
- **Verification** (§3.2): the namespace from the type expected, SHA-512 or SHA-256, the key byte for
  byte one expected, of three types, and for a `uv` key the user-verified flag, checked by Orpheum.
- **Following a chain** (§3.4): the next version by an everyday key when only the everyday tier
  changes, else by a recovery key; any later version, skipping, by a recovery key alone; removed keys
  stay named; a leaving signer has the new set sign too; two statements of one version refused unless
  one is a recovery key's, a later recovery skip resolving it; a statement differing from one accepted
  is a conflict said. Every row of §3.6 is a test (`TestRotation_*`).
- **The core** (§3.3, §3.5, §3.8): `trust-discovery` and `trust-publisher`, statements as received,
  checked again at each start; enrolment by issuer and identity (`POST /system/discovery/identify`
  shows it, the admin confirms; the suggested one from the compiled genesis); every place used in the
  past year asked hourly, floors `(root version, sequence)`, equivocation and changed digests refused
  and said, expiry warned; `caught-up` per run; the heartbeat's `rootVersion` and `rootHash` (F17);
  signatures bound to the identity (F14). The publisher root's genesis is compiled in
  (`suggested.go`, empty until the ceremony: a release build then reads no record, and says so).
- **Automatic installs wait 24 hours** from first sight (`Release.MayInstallAutomatically`); Part C
  is not built, and security-updates.md's Part C says it must ask the core.
- **The tools:** `cmd/discovery-sign` (genesis, root, address, drill, check, publish, bootstrap; it
  prints the `ssh-keygen -Y sign` command rather than running it, since starting a process is
  `execx`'s alone, and never uses an agent, F16), `cmd/discovery-watch` (every signature at every
  place against the ledger), `cmd/release-record` (the line format, `--id`, `--root-version`,
  `--pull`). The install verifies before it runs anything through the bootstrap `discovery-sign
  bootstrap` writes (§3.9); `orpheum verify-release` and `install.sh --release-record` are gone.

**D394 built (2026-10-31):** the forbidigo exclusion letting `cmd/discovery-sign/keygen.go` alone run
`ssh-keygen`, the release workflow's `release-record` step (VQ231), the guided
`ceremony`, `sign-release` and `test-key`, suspend and resume in the root format,
the guided `suspend`, `resume`, `add` and `remove`, the address document signed again when
its signer stops counting, and the owner's guide in plain words. Staging's own
software chain was made with the tool on 2026-10-31 (issuer `https://orpheum.dev`, identity
`od1-npmxozq3…`): records in `/yoloai/files/orpheum-security/staging`, keys in
`/yoloai/files/orpheum-staging-keys`, published into connect's `.staging/published`. Its address
document names D390's planned staging addresses (`api.orpheum.dev`, `account.orpheum.dev`,
`{core}.orpheum-client-test.com`), which no DNS serves yet: staging cores enrol with it only after
the move to `orpheum.dev` (D390 §5). The owner confirmed those addresses (2026-10-31).

**Owed by the owner:** the day-one ceremony (VQ211) and the genesis statements in `suggested.go`;
`-published=/published` and its read-only mount in production's compose file. On the Mac and a real
YubiKey, unconfirmed: the Secure Enclave provider's flags, `-O user=`'s spelling, `ssh-keygen -K`
getting a stub back. **Owed by connect:** see the batch report.

**Next batch (the owner's Mac, 2026-11-01):** `test-key` failed on the Mac: its `ssh-keygen -t ecdsa-sk -w
/usr/lib/ssh-keychain.dylib` refuses to enrol a key ("requested feature not supported"; idiosyncrasies,
"macOS's ssh-keychain.dylib cannot enrol a key"). An agent on the Mac confirmed the route that works
(`sc_auth create-ctk-identity -k p-256-ne -t bio`, then `ssh-keygen -w … -K`), and that its signatures
carry user presence but **not** the user-verified flag (flags 0x21), with the application always `ssh:`.
So the Mac cannot be a key D393 §3 requires to verify its user: building the `mac` kind on that route
would mean listing it without the requirement, trusting Touch ID no core can check. That is the
owner's decision, asked before anything is built. The ceremony goes ahead with the YubiKeys alone.
The owner, 2026-11-01: no key without a second factor a core can check, so the Mac stays out; a
YubiKey Bio (its fingerprint sets the hardware-signed user-verified flag) is the way to fingerprint
signing, unconfirmed on a real one. The owner holds three YubiKeys, so the ceremony makes one everyday
key (`e1`) and two recovery keys (`r1`, `r2`); a second everyday key is added later with `add`, signed by a
recovery key (below).

**This batch (the owner, 2026-11-01, "I don't want an everyday key to have that power"; "Suspend only"):**
as built, an everyday key could sign a root statement adding, suspending or removing an everyday key. Now
the only change to the set an everyday key may sign is suspending another everyday key; adding, removing,
re-marking, thresholds and locations take a recovery key. **Done: D395** (amending D393 §1, D394 §4
and §7). The rule lives in the code cores run (`discovery/trust`, `judgeAfter` and `Root.onlySuspends`),
not in the statements, so the ceremony need not wait; no deployed core holds D393's code, so it lands
before the deploy. The rotation tests now suspend with the other everyday key and replace with a recovery
key, `TestFollow_TheTiersRulesHold` refuses an everyday key adding, removing, re-marking or moving the
chain, and the guided `add`/`remove` offer only recovery keys (`rootChange.next`, `changeStanding`). The
root key audit (research, a dated record) is left as it was; D395 §1 says how its §3.6 rows 1–3 change.
Still to do: the ceremony still asks whether a Mac is an everyday key, and `test-key` and the `mac` kind
remain (D395 §3 says the Mac is no key): removing them is the next small change, asked of the owner first.
**The production address document (the ceremony, 2026-11-01):** issuer and location
`https://orpheum.cc`, as `suggested.go` compiles them into every core (D391 §7); name `Orpheum`; the
API unmoved at `https://api.orpheum-app.com/core/v1` and `/client/v1` (D390 §4); the loader still
`https://app.orpheum-app.com`, accounts `https://account.orpheum-app.com` and cores
`https://{core}.orpheum-client.com`, as production connect serves them today, a new document (an
everyday key's) moving them when D390's move happens; homepage `https://orpheum.mov` (D390 §6). On
2026-11-01 neither `orpheum.cc` nor `orpheum.mov` resolved, so **before the deploy the owner points
`orpheum.cc` at production connect** (Cloudflare zone and tunnel hostname), which serves the published
directory at `/.well-known/orpheum-discovery`; until then no core can enrol with the suggested issuer.
The ceremony's address defaults (`addressDefaults`, each field the issuer plus a path) fit no real
deployment; offering the paid service's addresses instead is a small follow-up.
**Next batch, today (the owner, 2026-11-01: "I actually want to move to the new URLs now. It can be
next batch, but I want this move to the new URLs done today"):** D390's move, in its §8 order: the
relying-party setting and mail hygiene, staging onto `orpheum.dev`, then production's app at
`orpheum.cc` and accounts at `account.orpheum.cc` (the API stays `api.orpheum-app.com`), mail from
`noreply@orpheum.cc`, the old loader's retirement and the redirects, the website at `orpheum.mov`. Owed
by the owner: the Cloudflare zones and tunnel hostnames, mail DNS. The production address document is
therefore signed with the new addresses (loader `https://orpheum.cc`, accounts
`https://account.orpheum.cc`), so it needs no re-signing after the move, and the deploy follows the move.
The owner asked (2026-11-01) that an everyday key can never suspend itself by a wrong pick in a list. Two
guards hold it already and are kept through the change: the guided tool never offers the key being
suspended as its signer (`chooseSigner`; `guidedchange_test.go`), and a core refuses a statement signed
only by the key it suspends (`ErrNotHeldByNewSet`; `suspend_test.go`).
**The ceremony is done (the owner, 2026-11-01):** run on the Mac with three YubiKeys (`e1`, `r1`, `r2`)
into `~/orpheum-security` and committed there. The sandbox is rebuilt so that `~/orpheum-security` is a
working directory of its own; the staging chain made on 2026-10-31 (`staging/`, an earlier commit) lived in
the shared-files copy and is merged into it, while staging's software keys stay outside every repository
(D394). **Next, in order:** `make check` and one full `make check-pr` on D395 (not yet run);
the genesis statements from `~/orpheum-security`'s ledger pasted into `suggested.go` (the discovery and
the publisher); D390's move above, staging first; then the deploy order (the published directory on the
production host, `-published=/published` in production's compose file, `orpheum.cc` pointed at
connect, connect, then nas.example).

**Left, small (from the agents' notes):** `TestServe_ThisMachinesPortAdministersWhereTheHousesPortDoesNot` can collide on a
port when runs overlap; `test-key` leaves its temporary directory (it says where; removing it would need forbidigo's delete rule to allow it, principles §2); the first release in a record
keeps no security note; each signing attempt's copy stays under `ledger/pending/<name>.parts/`. VQ210, VQ211
(rewritten for D393), VQ220 (the bootstrap), VQ221 (the watcher).

## From the production deploy (2026-11-01), next batch (the owner agreed 2026-11-01)

**Done (2026-11-01):** check-pr passed on "refactor(web): reportReloadForNewerClient is reportReloadNote"; connect re-pinned to it (connect an earlier commit, up.sh's
tests an earlier commit); production serves `orpheum.cc` and `account.orpheum.cc` with the ceremony's published
chain; nas.example runs "refactor(web): reportReloadForNewerClient is reportReloadNote"'s tree, re-enrolled with `orpheum.cc`, realm `karls` (`karl` is refused:
D298 §2 wants 5 to 32 characters). Each household member signs in once with a password on nas.example itself,
which makes the core's key for the renamed realm; until then the realm's sign-in at `orpheum.cc` is
refused.

- **Connect's up.sh checks `published/` as the `connect` user.** The first tarball kept uid 1000 and 750
  directories; up.sh, checking as root, passed it, and connectd crash-looped with "read the discovery
  chain's genesis: open /published/orpheum/root/1: permission denied", every name answering 502.
- **A realm renamed needs one password sign-in on the core**, which nothing says: up.sh's `realm`
  command and the household's docs say so, or the core makes the new realm's key without it (realm keys
  are per master realm: `RealmKeyOfPerson`, `keepRealmKey`).
- **check-pr took 352 s** on "refactor(web): reportReloadForNewerClient is reportReloadNote", over its 300 s budget (a warning).
- **check-pr takes its lock after web-deps**, so the Stop hook's `npm ci` raced it (a gate, the owner's).
- **Revert-red takes a stored hint naming a deleted test for a broken environment** (a gate bug, the
  owner's): the stale main.tsx hint in `.git/revert-red/known-red` had to be removed by hand.
- **`e2e/discovery.spec.ts:92` flaked once**: "route.fulfill: Fetch response has been disposed".
- **An empty Home tells an admin to add a library** (seen on staging before its libraries were made). **Done already** in the 11-02 batch ("feat(web): an empty Home says how it fills", "an empty Home says how it fills"), found 2026-11-01.
- **The core logged HeadlessChrome as `browser=safari`.**
- **nas.example logs warnings for setting rows nothing reads** (`prepare_progress_every_seconds`,
  `scan_*_extensions`, `together_*`, …): remove the rows or the warning.
- **An AbortError goes uncaught in the page** (the owner's console, nas.example, Chrome on the Mac,
  2026-11-01): `Uncaught (in promise) AbortError: signal is aborted without reason at MessagePort`
  in transport-choice's bundle, four times, after a film page's warm-up was dropped. A cancelled
  request's promise is left unhandled; find which and catch the abort.
- **The console calls every line sent to the core's log "a request failed"** (VQ198's run,
  2026-11-01: "Orpheum: a request failed the page froze 2.0 s"): reportFailure's console prefix fits
  failures alone; a freeze report wants its own words. Seen in `page-freezes.ts`, which reports through the SDK's `reportFailure` (`web/packages/sdk/src/problem.ts:133`); the core's own log line reads right.
- **Connect's side of D398** (from the 11-02 batch's item 5, 00393430): at connect's next Orpheum pin,
  `deadEnd` in `internal/clients/offers.go` must pass `asked.PathsRequest` (it does not compile
  otherwise) and log the reason with the source's family taken from CF-Connecting-IP, never the
  address. VQ235 passes on a connect discovery only after that. For the owner: whether connect also
  stores the reason (a migration), whether the System page's count shows causes, and an IPv6-only
  probe name so the page can tell "no IPv6" from "IPv6 got no answer" on more networks.
- **orpheum-ops' README §6a names an old core ID**: nas.example's core on production is
  `f3k5n8g0ktf2f8bjrd6rs6v6qt` (`c2zd1gwvgx1wmp3hsx7b77y81b` is a stale core of 10-15).
- **`sw.browser.test.ts` hangs under load** (make check, 2026-11-01): "carries the load of a page of the release before" timed out at its 5 s `hungAfterMs` with the machine's load average at about 24 (two agents building). Alone it passes in 0.7 s. Look at why a loaded machine stretches one relayed load past 5 s before widening anything (rule 5).
- **A seek's playback stops again 1 s after it starts** (the owner, Chrome on staging, Nosferatu at 29:04, 2026-11-01 14:29 UTC). After the timeline seek, the film started at 1744.0 s once one chunk was loaded (1742.1–1748.9 s), played 1.2 s, then waited 2.1 s with 3.7 s still loaded ahead. The seek's steps show "video flushed from 0.00; audio flushed from 0.00" and "ceiling 0 to 1: stepped up" at the same moment. Likely cause, not yet confirmed: the step up after the seek switched at once, flushing everything, including the chunk that had just started playing. The fix to check: a step up never flushes what is playing.
- **A track switch's cut has a gap a second after the choice** (the owner, same session, at 56:01, 14:31:17 UTC). The 232 ms wait at the choice itself is fine, the owner says, but not a second gap about 1 s later, at the point where the old track hands over to the new one (VQ204's "old language plays on for 1–3 s"). The log shows no stall at the cut, so the gap is in the sound itself at the seam. Options for the owner: cut at once (the gap comes at the choice, which they accept), or keep the delay and make the seam clean.
- **The Stop hook's `make check` keeps no record of how long it ran** (the owner, 2026-11-01: "why is make check taking so long? … we've been waiting over 30 mins this turn"). Only the last log stays, and the hook keeps no start time or length per run, so a slow turn can't be explained after the fact. In that turn, staging was transcoding and the background revert-red probe kept restarting after each of about eight commits (load average 13–24). The proposal: record each run's start, length and load, and say at the start of a run when an FFmpeg job is running ("staging is transcoding; the gate will be slower"), since the owner wants to know so they can stop it. Both are gate changes, the owner's to order.
- **Does the boost's switch have the same gap at its seam as the chosen track had?** (the glitch agent, 2026-11-01, after D400). The boost still switches with `nextAudioTrack`; check by ear on the next device test.
- **A rescan that keeps reading its library as changing says nothing of why** (the stuck rescan, fixed in "fix(identify): an episode its name placed is marked a guess once, not every pass", 2026-11-01: one `MarkGuessedMatch` upsert per pass kept library 2's rescan waiting for hours, logging only "a rescan waits for its library to stop changing"). `filesSayOf` could keep the refused write's error from inside its closure and attach it to `errLibraryKeptChanging`, so the log names the statement and title. No fixture library keeps writing now, so its test needs a seam.
- **A confirmed by-name episode placement is marked a guess again** (the same agent, a question under D307 §2). Confirming one clears its `guessed_match` row, but the next pass places the episode by its name again and marks it a guess again. The owner to say whether a confirmation should stick. **The owner, 2026-11-01: "Agreed on D307"** to the lead's proposal: a confirmation sticks; a pass that places the episode at the place confirmed leaves it unmarked, and only a new place is a new guess (as D172 keeps a confirmed film). Next batch, recorded as a decision amending D307 §2.
- **Titles beginning with "The" are filed under T** (the owner, 2026-11-01: "feels weird"; research the same day, from forum and SDK sources). Plex, Jellyfin and Emby skip leading articles in sorting by default, and Kodi has an option. All keep the full title on screen, and each has a per-title sort name. Their article lists are global, mostly English (Plex adds das, der, el, la). Here the sort key is the raw name (`internal/core/identify/text.go:34`, `internal/core/textindex/sortkey.go`), and the letter rail follows the key (`internal/core/library/letters.go`). The NFO's `<sorttitle>` is parsed (`nfo.go`) but never used. `names.go`'s `isArticle`/`withoutArticle` serve only matching. `data-timeliness.md:458` already plans a rail that skips articles, but no decision records it. Proposal for a decision refining D47: key the name without its leading article, from a list per sort locale (en the/a/an; de der/die/das/ein/eine; fr le/la/les/l'/un/une; es el/la/los/las/un/una); an NFO `<sorttitle>` wins; the shown title is unchanged; a name that is only an article stays whole; every `sort_key` row is rebuilt through `rekeyName`. **The owner, 2026-11-01: "Agreed on sorting"**: build it next batch, recorded as a decision refining D47.
- **An episode in Continue Watching had no poster** (the owner, Dimmerhold's first episode on nas.example, 2026-11-01). Fixed in "fix(library): an episode in Continue Watching shows its series' poster": its series' poster. To deploy with the next batch.

## The 11-02 batch (the owner agreed 2026-11-01): networking, and the beta made solid

The owner's device checks run alongside, in the order of `/yoloai/files/testing-plan-2026-11-01.md`.

1. **The iPhone's full-screen button** (the owner, 2026-11-01: "The full screen button doesn't work on
   iphone (safari)"). An iPhone has no element full screen (`requestFullscreen` is iPad-only, WebKit
   bug 206854), so `film-fullscreen.ts`'s `fillTheScreen` only fills the window under Safari's bars.
   The button opens the video in iOS's own player (`webkitEnterFullscreen`), as
   `together-viewer.tsx` already does, the two sharing one function; Apple's controls show there
   (the owner: "OK to use iOS's own player").
2. **The LAN origin, a decision first:** the lead writes the D-proposal (the plan's "Reaching the core
   at home" item 1); built once the owner decides. **Proposed (2026-11-01):**
   [lan-origin-2026-11-01.md](lan-origin-2026-11-01.md), waiting on the owner.
3. **WebRTC for Chromium:** that section's items 3–5.
4. **Recovery:** the core and its discovery find each other again in either restart order; a Play
   page gives up its sockets on `pagehide` and reconnects on `pageshow`; Firefox's leftovers (a page
   on a core's direct HTTPS name hears the network's return only from `online`; the direct mode's
   signaller marks no failure `Unreached`). **Found done (2026-11-01):** the first two were built in
   the 10-30 batch: a link left open by a restarted discovery is found dead within about 48 s and
   dialled again, whatever the restart order ("fix(discovery): a link left open by a restarted discovery is found dead within a minute and dialled again"), and the session connection closes its socket
   on `pagehide` and connects again on `pageshow` ("fix(together): a Play page navigated away gives up its session's socket at once", VQ170). Firefox's two leftovers remain.
5. **A page that cannot reach its core says why**, to the discovery and in words (the plan's "Reaching
   production from outside" item 2). The edge tunnel (item 1 there) is not this batch's: see the
   owner's question of 2026-11-01 below.
6. **Playback's start:** the remembered link kept across a deploy; the player choosing its first rung
   before loading anything (the Fire TV's black blip). **Found done (2026-11-01):** both were built before this
   batch was written: the core keeps each device's link in the store (`link_memory`, "feat(player,playback): the link a device remembers is a prior of its estimate, and the core keeps it across restarts"), and
   nas.example's held device 2's link of 23.6 h before through the 11-01 restart; its three
   `rememberedLinkKbps=0` starts since were devices 31, 33 and 34 at their first playback, new devices
   after the realm change. The first rung by the remembered link is D381 (35822125), VQ176 ticked.
7. **The production deploy's small items** (the section above): HeadlessChrome's browser name, the
   empty Home's hint to an admin, the setting rows nothing reads, the realm rename's note, connect's
   `up.sh` checking `published/` as the connect user, the ops README's core ID.
8. **Gate fixes, the owner's yes (2026-11-01):** check-pr takes its lock before web-deps; revert-red
   tells a stale known-red hint from a broken environment. Not check-pr's 352 s (the owner).

9. **The People page in three sections** (the owner, 2026-11-01, before testing VQ201: "The people page
   is just too messy"), D399:
   - Above the sections, side by side: *Create Role*, *Create Account* and *Invite Someone* (the owner wrote
     "2 buttons" and named three; built as three), each opening a modal dialog with its controls. A new
     invitation opens its detail dialog at once; a new account or role opens nothing.
   - **Invites:** each row shows its admin note, role and date issued. A row opens a dialog with its
     details, the QR code, the link and *Withdraw* at the bottom. "Send another link" is removed.
   - **Accounts:** each row shows name, admin note and role; a row opens that account's page (the
     existing person page), where it is viewed and changed.
   - **Roles:** each row shows name and how many people have it; a row opens a new role page to change
     its permissions and library access, each change taking effect at once with no Save button, and to
     delete the role while no one has it.

**Not this batch:** the cosmetic UI items (the UX rewrite), the decoder-limit codec drop, the
average-bitrate rung, inventory features.

**The owner's question (2026-11-01):** "I don't understand why this requires me to open a tunnel on
the vps. The vps is the paid discovery server, and we can't be opening tunnels to every customer's
cores." Answered: the edge VPS (`11.22.44.139`, orpheum-ops `edge/`) is not the discovery server
(prod-api, `178.84…`); it is the owner's own forwarder for nas.example, whose DS-Lite line has no IPv4 way
in. No customer's core gets a tunnel (D314: a connection that needs a relay fails, and is counted);
item 5 is the counting. Whether nas.example uses the edge again is the owner's own choice, not product work.

**The owner's idea (2026-11-01), to be designed later, not the 11-02 batch:** a household may run its
own proxy on its own VPS, which the discovery then names as a way to its core: the service still
carries no one's traffic (D314), but a household behind DS-Lite or CGNAT can give its core an IPv4
way in. nas.example's edge VPS (orpheum-ops `edge/`) is the test bed when it is designed. nas.example's own
use of the edge waits until then (the owner: "We'll hold off on nas.example's edge server for now").

## The 11-03 batch (the owner agreed 2026-11-01): playback you feel, small visible things, the beta's setup

The lead's proposal from the next-batch list above (the owner, 2026-11-01: "Go with your
recommendations"). The owner holds off on the testing plan's sound steps until this batch changes them.

1. **A seek's playback stops again 1 s after it starts** (Nosferatu at 29:04). Found 2026-11-01 to be in
   the 11-02 batch already: "fix(player): a seek's report says where each flush ends and how old each choice is" showed the step up came 501 s before the seek and the flushes were
   hls.js trimming the back buffer, so neither caused the wait. The cause is still unknown (the sound
   running out, or hls.js's 2 s wait before nudging over a hole); VQ236's next report, with every
   `audioBuffered` range, tells them apart. Nothing to build until then.
2. **A track switch's second gap, at the old track's seam.** Found 2026-11-01 to be in the 11-02 batch
   already: "fix(player): another audio track chosen cuts in at the choice, not a second later with a gap" (D400) cuts a chosen track in at the choice, waiting on VQ237. The boost still
   switches late and likely has the same seam (hls.js writes the new sound over sound Chrome is
   already decoding, `audio-stream-controller.ts`); cutting it at once froze the picture 2.2 s at the
   top quality (2026-10-29), and a clean seam needs hls.js patched or an audio controller of our own,
   a dependency decision. The owner's call after VQ237.
3. **The uncaught AbortError** after a film page's warm-up is dropped.
4. **Titles sort without their leading article**: D402, refining D47 ("feat(library): titles sort without their leading article (D402)"), as agreed above.
5. **A confirmed episode placement sticks**: D401, amending D307 §2, as agreed above.
6. **An empty Home tells an admin to add a library**: done already, in the 11-02 batch ("feat(web): an empty Home says how it fills").
7. **A page's freeze has its own console words**, not "a request failed".
8. **The Continue Watching poster** ("fix(library): an episode in Continue Watching shows its series' poster", verified on staging by the owner) ships with it.
9. **Connect's up.sh checks `published/` as the `connect` user** (orpheum-connect). **Done already** in connect an earlier commit (found 2026-11-01).
10. **A renamed realm's one password sign-in** is said where it is needed, or made unneeded. It cannot be made unneeded: the key comes from the password, which the core keeps only as a hash (D297 §3). It is now said in up.sh's `realm` usage and orpheum-ops' README §6a (connect an earlier commit), and in `docs/users/setup-problems.md` for the household.

End: one check-pr, the owner's go, nas.example. Quick visible fixes go to staging for the owner as they
land. **Not this batch:** connect's side of D398 (at its next pin), the gate items, the ops and logging
items above.

## The 11-04 batch (the owner, 2026-11-01): from VQ201 and the night's testing

The owner, 2026-11-01, going to bed: "Maybe you should finish this batch and start another one
overnight", with VQ201 passed and these comments. No nas.example release is wanted soon.

1. **A redeemed invitation leaves the People page** (the owner: "When an invitation is redeemed, it
   should be removed from the people page"). Its account is in Accounts already.
2. **A role opens in a dialog, not a page, the same dialog creating and editing it** (the owner: "On
   second thought, the role info should be a popup rather than a page. It should be the same dialog
   for creating and editing a role"). Amends D399's role page.
3. **A series nobody has started plays season 1 first; its specials are optional** (the owner: 12
   Monkeys "shows 'Play S0E0'. It should always start at season 1, leaving the specials optional").
   Built in "fix(library): a series nobody has started plays season 1 first, its specials optional": a special is Next Up only while its person is partway through it, and finishing
   one does not move their place.
4. **Quillwick S2E4's sound is out of sync, on staging and on nas.example** (the owner, 2026-11-01).
   Folded in as seriously hurting use. The file is `Quillwick/Quillwick - 2x4 - Money.avi`: XviD
   with packed B-frames (most video packets carry no pts) and CBR MP3 in AVI, both starting at 0 and
   both lasting 1797 s by their own counts. Not yet known: the offset's size, whether it grows, and
   whether the source is off in other players.

End: one check-pr. Quick visible fixes go to staging for the owner as they land.

## The owner's comments of 2026-11-02

1. **A letter jump puts the letter's first title at the top** (the owner: "When jumping to a letter
   on a listing, it should aspire to jump such that the first entry of that letter is at the top of
   the page"). Built in "fix(web): a letter jump shows the letter's first title at the top, loading what the page needs below it", a quick fix on staging: where the letter's first title was in the
   last page loaded, the page ended too soon below it to scroll it up, and it stayed low (Mad Max 2
   at 693 px of 1080); the jump now loads the next page and shows it at the top again. A title near
   the list's very end still stops where the end allows.
2. **Mister 517 has no sound** (the owner: "I think the file is damaged ... it would be great if
   orpheum could compensate for that"). The file, `Mister 517 (1950) [720p].mp4`, holds no sound at
   all: a fragmented MP4 whose one track is H.264 video, its 1022 fragments all video (579 MB at
   about 860 kb/s), as a video-only DASH download whose sound part was never merged in. Nothing is
   there to recover. It is the only file without sound in the sandbox's copy of the library.
   **Next batch (the owner, 2026-11-02: "We don't need to fold the mister 517 sound thing into this
   batch, but I do agree that videos without sound should be included in the list of problem
   films"):** a video with no sound track is listed among the problem films. The note on the film's
   page and its player, proposed beside it, was not taken up.
3. **The letter rail never scrolls** (the owner: "When I scroll to the bottom of a list, the letter
   rail starts to scroll as well. It should never scroll"). Built in "fix(web): the alphabet rail never scrolls, at the list's end too" and "fix(web): the shell's notes over a list keep clear of its fixed alphabet rail", quick
   fixes on staging: the rail is fixed to the window rather than sticky in the layout, and the
   shell's notes over the list stop short of its gutter.
4. **Pale and Kind is found** (the owner: "Pale and Kind" does not match tt0567890). Built in
   "fix(identify): a name's 'and' is also searched as '&', and its '&' as 'and'": TMDB's search finds "Blind & Ugly" and not "Pale and Kind", and no spelling tried after
   a failed search swapped the two; one now does, both ways.
5. **Wings.mp4 is Cameron's, not a Korean Wings of 2014** (the owner, 2026-11-02). The file, a
   148.8-minute cut against TMDB's 137, contradicted Cameron's runtime, and TMDB knows no runtime of
   the 2014 film, so the unknown ranked over the contradicted. An interim fix in "fix(identify): with no year, an unknown runtime does not lift a film's candidate over the oldest": with no
   year, an unknown runtime ranks with the contradicted. **Next batch (the owner agreed,
   2026-11-02), a decision amending D365:** a name with no year was enough for whoever named it, so
   the well-known title is the likely one. With no year, in order:
   1. A candidate whose original title matches the name wins a tie over one matched only by a
      translated title; it never knocks a candidate out (Blind & Ugly is named in English, its
      original German).
   2. An unknown runtime counts as a contradicting one, with a year too.
   3. TMDB's vote count (not its daily popularity score) decides, but only by a wide margin (10×).
   4. Runtime fit, in tiers: a close fit (a few minutes) over an edition's fit (the file up to about
      30% longer, more where the name says Extended, Director's Cut, Special Edition or Unrated)
      over a contradiction. The close tier keeps remakes of near lengths apart (Sabrina's 1954 at
      113 minutes would otherwise fit the 1995's 127-minute file as a longer cut).
   5. The oldest.

   A series: every season and episode number on disk must exist at the provider (missing ones do
   not count against it); where the files carry episode names, names wildly unlike the provider's
   count strongly against it. A match is not re-ranked when only the vote counts drift, and a close
   call between the top two is flagged for checking. The test with no year, Alcazar (a stand-in for
   The Thief of Bagdad: Disney's 1992 against the 2019 remake), gives the remake 15,000 votes and Disney's none;
   its votes become near each other's, as the real films' are, so the oldest still wins.
6. **A DVD image fails to seek past 59 minutes** (found by the agent running VQ73, 2026-11-02;
   folded into this batch as a bug that seriously hurts using the app). The Der Golem concert
   DVD's title starts its clock again at 0 at 1:28:39, and FFmpeg, seeking a program stream by its
   timestamps, landed in the wrong half or at the stream's end. Built: the node finds where a DVD
   title's clock starts again from its navigation packs and serves the title restamped onto one clock
   (D404, amending D350 §3). Checked on the real disc: seeks to 3000, 4432, 5330 and 8000 s land
   within a frame, and a transcode across the restart runs clean. VQ73's seek is re-run on staging.
   Re-run on staging in Chrome: seeks to 1:13:55 and 1:50:49 play. The owner asked for research on
   what others do (`design/research/dvd-playback-2026-11-02.md`); its gaps, checking every VOBU join
   and the sound at a join, are proposed to the owner, not started.
7. **Fixed in this batch (found by the agent running VQ11, 2026-11-02):** a refused OpenSubtitles
   login said "Something went wrong on the server", as OpenSubtitles answers it 400. VQ11 itself waits
   for the account's username (the one given is an email address, which OpenSubtitles refuses).
8. **Next batch (found by the agent running VQ50, 2026-11-02):** a film measured while it plays says
   "Its loudness is not measured yet" once measuring ends, and levels nothing until it is played again:
   the player learns the level only when it starts, and nothing asks for it again. VQ50 expects the
   switch to say within minutes that the film was raised or lowered. And the wait said "about 55
   minutes" for a read-through that began 15 s later, as the playing film's moved ahead of the 17 queued.
9. **Next batch (found by the agent running VQ238, 2026-11-02):** a role one person holds says
   "1 people" on the People page (`roles.holders` has no singular form).
10. **DVD clocks (the owner, 2026-11-02, "yes go ahead" to the research's proposals):** D405, a DVD
    title's clock checked at every VOBU with a frame's slack, is **done** in this batch. **Next
    batch:** the sound at a join (D405 §4): drop an AC-3 frame that repeats or is cut short where the
    clock was restamped, as FFmpeg's `dvdvideo` demuxer does.
11. **Fixed in this batch (found by the agent running VQ61 on the Fire TV, 2026-11-02):** a series
    opened from far down the Series list opened scrolled down, its name hidden ("fix(web): a series opens at its top, its row of seasons scrolled sideways alone"); and OK on
    the video opened the hidden player overlay with nothing focused, so OK never paused and no arrow
    reached its controls ("fix(web): OK on a television's hidden player overlay puts focus on play").
12. **Fixed in this batch (found running VQ61; the owner, 2026-11-02: "Yes put that in"):** Back
    from a film opened from Films' or Series' search showed the whole list again: the search and the
    sort were held in the page (`browse.tsx`), not in its address as the Libraries page's are, so the
    history entry did not keep them, and on a TV a search typed by the remote was lost after looking
    at one result. Now `/films` and `/series` carry `sort`, `order` and `query` as `/browse` does.
13. **Next batch (found running VQ61):** the arrows' search prefers a control in line with the press
    over a nearer one beside it, so some are reached only the other way: Down from *‹ Series* goes to
    the season row past *Play S5E1* and *Mark all … as watched*; Down from the banner goes past Play
    to *Mark Season 5 as watched*; Down from This device's Rename goes past the game controller's
    choices to the account link; Right at the end of the series page's buttons or of Continue Watching
    jumps into the banner; Up from the banner's Menu moves sideways to *Cast to a device*. And with
    the film paused, the player's first Back does nothing visible, as the whole overlay stays while
    paused; the second leaves.
14. **This batch, fixed (found running VQ69):** an episode stood a frame short of its end for good,
    never ended, and no next episode came: hls.js's end check misreads a hole behind the playhead
    (idiosyncrasies.md, "hls.js never ends a stream with a hole behind the playhead"); the player now
    ends the stream itself ("fix(player): an episode with a hole behind the playhead reaches its end", "fix(player): a video stalled at its end while seeking is ended too"). **Next batch:** the hole was ours. The Pumbletons'
    S33E8 (file 5674) chunk 230, listed from 1280.785 s where 229 ends, loaded from 1280.826 s with
    video from 1280.868 s, and later chunks sat 42–83 ms off the playlist: a chunk that starts a frame
    or two late. Find why in the node's chunking (the segment muxer's stop, dts2pts) and close the
    hole at the join.
15. **A Watch Together guest in Berlin could not reach nas.example** (the owner, 2026-11-02; a test of how
    voice and playback fare on her very bad home connection). nas.example's core log, 14:24–14:26 UTC:
    - **This batch:** from her IPv6 network (`2a0f:5000:8119:5200::/56`, likely her Wi-Fi) the page
      connected twice by a punched IPv6 path (363 and 463 ms), the core held each, and each closed
      about 100 ms later with no request and no sign-in. The path works; the page dropped it. **Not yet
      found.** A first guess, a browser with no service worker (Firefox's private window), was
      wrong: she used Safari in an ordinary tab (the owner, 2026-11-02). That case fails the same
      way and is fixed ("fix(web): a browser with no service worker is told it cannot play, not that its network failed"). Playwright's WebKit joins on staging; next, the owner's iPhone in
      Safari against prod, as a guest.
    - **Her second try, on staging (16:46–16:51 UTC):** she opened the link from Telegram, whose
      built-in browser runs no service worker: the page said "This browser can't play from the
      server", and the core logged the connection held and ended after 92 ms with no request, the
      same signature as the first try's. A cause that fits the first try too, not confirmed. In
      Safari she joined. The voice call failed 13 times over 90 s (each about 5 s after "accepted")
      before it connected at 16:49:15, then held at about 35 ms with 1 packet lost; her core
      connections all ended at 16:48:04 after 100 s, their send windows down to one packet, and the
      page reconnected; her video stopped twice "hidden" (Safari in the background); she left by
      Leave at 16:51:17, the call still healthy (her earphones' battery ran out). **This batch:** the
      no-service-worker page tells a page inside an app to open the link in Safari or Chrome, with
      the link to copy. **Next batch:** why the first voice call takes 90 s to connect; what ended
      her connections at 16:48:04.
    - **Why the voice call took 90 s (found 2026-11-02, not yet fixed):** every failed try was the
      owner's browser failing the connection 5.0 s after "accepted"; the try that connected came 5 s
      after her microphone first opened (16:49:10.020), and connected in 143 ms. Staging and prod
      run with no relay, so `GET /together/{id}/ice` gives no ICE servers, not even STUN. A page with
      no STUN server and no microphone permission offers only an mDNS `.local` name, which no one
      outside its network can resolve. In the sandbox, WebKit, Chromium and Firefox each did
      exactly that, and each added its public address once given `stun:stun.cloudflare.com:3478`.
      So a listener who has not pressed talk cannot be reached; the owner's home firewall drops her
      checks, since nothing went out to her first. **Fix, the owner's choice of STUN server:** the
      core's `/together/{id}/ice` adds STUN servers to its relay's (none) (D281 §1: everyone hears
      from the start).
    - **What ended her connections at 16:48:04 (found 2026-11-02):** her side went quiet, not the
      core. Her last message was at 16:47:24; the core ended both connections, with SCTP's window
      down to one packet, some 30–40 s later, about ICE's consent timeout. The owner's Firefox on
      Android did the same at 16:48:02.8 (hidden), its connections ending at 16:48:33.1. She
      reconnected at 16:48:22 and was hidden 1 s later, so the likeliest cause is Safari in the
      background, frozen before its "hidden" note went out; a network gap of her bad connection
      fits too. The core's "rtc connection ended" names no cause; one that did (ICE failed, the
      browser closed it) would tell these apart.
    - From her IPv4-only network (`194.213.108.4`, likely LTE, no IPv6 candidate) every pair failed
      with nothing sent or received: the house is DS-Lite with no IPv4 way in and the beta has no relay
      (as "Reaching production from outside" above).
    - **Next batch:** the core's port mapping asks Docker's bridge gateway (`pcp: the router at
      172.18.0.1:5351 … connection refused`; `upnp: no UPnP router answered`), never the router, so in
      the published compose setup it cannot map for any household. Find the gateway past the bridge,
      or let the admin name the router.
    - **To design and decide later (the owner: "This will be something to design and decide over
      later"):** an IPv4 way in for houses like this one: (a) a relay on a VPS the user supplies;
      (b) possibly a relay through the paid discovery, which has potential legal issues.
16. **Next batch (the owner, 2026-11-02): the screen locked during a film.** "I was playing a movie on
    firefox in linux mint, fullscreen. It suddenly got very choppy, and when I got out of fullscreen I
    noticed that linux mint had gone to the lock screen while I was watching a movie." The player holds
    a screen wake lock already: `keepScreenAwake` (`packages/player/src/wake.ts`, D287, "fix(together): a phone's screen locking no longer pauses its group, and a playing film keeps the screen awake"), held
    while the video plays on a shown page and asked again when the page is shown. The lead's first
    reading here, that the client asked `navigator.wakeLock` nothing, was wrong: it searched the app
    and not the player package. So the lock was asked for, or the page could not ask: the API exists
    only in a secure context, so a page opened at a plain-http LAN address holds none. Firefox on
    Linux turns both a page's lock and its own "video-playing" lock into one desktop inhibit, and
    that is known to fail: Mozilla bug 1935914 (Linux Mint 21.3, Xfce, Firefox 128, the screen slept
    during a full-screen video until Firefox was restarted), and the labwc and Ubuntu reports of the
    inhibit going to an interface the desktop does not honour. Cinnamon is not named in any of them.
    **To find which:** the address the film was opened at (https or not), and Firefox started from a
    terminal as `MOZ_LOG="LinuxWakeLock:5" firefox`, a film played full screen past the lock's
    timeout: the log says whether a lock was asked and by which interface it went. If Firefox's
    inhibit fails, the page cannot fix it, and it goes into `idiosyncrasies.md` with the way around
    (Cinnamon's own inhibit applet). The choppiness is to be checked once the lock no longer comes,
    as the locker's compositing may be its cause.
17. **Next batch (the owner, 2026-11-02): fast-forward and rewind do nothing in Silk's full screen.**
    "I reopend silk browser on fire tv, which was already on a play page with video player. I
    navigated to the play button, pressed select, then navigated to the fullscreen button and pressed
    select to bring it to full screen. Now when I press fastforward or rewind, they do nothing. Maybe
    because of where the focus is?" To check: which element holds focus once the full-screen button
    is pressed, and whether the remote's media keys reach the page's key handler then.
    **Not reproduced (the lead, 2026-11-02, on staging, Silk 138, The Gold Rush):** the owner's session
    was on prod (file 309, 15:29), whose build has the same key handling, full screen and controls as
    staging's. The lead repeated the owner's steps over adb: Silk reopened on the paused play page, the
    D-pad to Play, Select, to Full screen, Select; then fast forward and rewind (keycodes 90 and 89),
    sent as the remote's sources (keyboard and D-pad: the remote, WOBLE_INPUT_DEVICE, is both). Every
    press moved the film, with focus on the video, on Pause or on "Leave full screen". Two things seen
    on the way, for the next batch: once the film plays, the bar can hide between presses, and the
    D-pad's Right then seeks the film rather than reaching Full screen; and a fast forward sent as a
    gamepad's key left the film paused (a controller, not the remote). **Next:** the owner tries again
    with the remote, and if it happens, leaves the TV as it is and says so: the page's focus and full
    screen can then be read over adb as they are.
