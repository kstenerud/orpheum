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

## From the owner's VQ89 check (2026-10-28), next batch

- **The watched check on an episode's row** sits beside it; the owner: "I think the checkmark should be
  over the screenshot" (the episode's still, as on a poster's tray).
- **The check inside a chooser's items** "looks awful"; moot once seasons are chosen by posters (the
  season posters item above), so not fixed in the chooser.
- **The scrub bar:** "Clicking on the scrub bar doesn't move the marker until the movie actually starts
  playing from there, which makes it feel like it didn't work." The marker (and the time shown) should
  move to the pressed place at once and stay there while the seek loads.
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
  1231.3 s for 50 s** (headless Chromium with DevTools throttling; a second run played on).
- **A drop-down in a short window** (1920×600) moves up 22 px to stay on screen, its choice no longer over
  its button's centre.
- **A WARN "a request that only reads wrote to the database"** from `forgetAttempt` when the boost stopped
  an own-sound run (20:57:08), where every other such line is DEBUG with a reason.
- **Pages keep asking every 5 s for a Watch Together session that no longer exists.**
- **A remote opened just as its cast ends says only "The cast is over."** Its hello reaches a session
  already gone ("no such Watch Together session"), so it never hears that the receiver took control
  back, though the core logged `reason=takenBack` the same millisecond (browser tier, 2026-10-29). The
  refusal could carry a recently ended session's reason.
