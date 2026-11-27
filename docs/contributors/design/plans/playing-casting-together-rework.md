# The rework: playing, casting and Watch Together as one (D555)

**Status:** agreed by the owner (2026-11-25); not built. It plans D555,
from maps of the server, the web client and the Google Cast receiver read on 2026-11-25.

## What the maps found

- **The seams exist already.** Every control row reads one interface, `PlayerSource`
  (`player-controls.tsx`), which the video and the session's remote both implement, and one
  component, `FilmPlayer`, plays on the Play page, the receiver and the guest's page. The browser
  receiver is the Play page. What is duplicated is what was built around them: three sets of
  track menus, three Up nexts, three wrappers of the timeline, two compositions of the player
  (the Play page's and the guest's), and the remote as a page of its own controls.
- **On the server a cast is always a second session,** and a record in the live registry with a
  film, a position and its own ends. In a Watch Together session the receiver also gets a
  credential of the caster's member, so it holds two attachments and the remote works two
  sessions at once. A guest by link's cast is the exception: it is the guest's own part alone,
  as D555 §2 has it, but with no end for silence, an unanswered question or idleness.
- **Nothing on the server can end a cast at 20 s.** Silence is measured on the presence socket
  alone, which takes 45 s to notice, then two minutes more. The playing device reports its state
  on a change and each 60 s.
- **Joining, leaving and switching sessions do not carry the castee.** A host starting another
  session loses the cast; a guest who leaves keeps a cast that follows nothing; one who joins
  has the receiver follow only at its next hand-off. A cast's credential in a session is never
  taken back at the cast's end.
- **The Google Cast receiver is a second receiver,** with its own leading, following, playback
  request and Up next. A regular cast reaches it from the app's regular pages alone, but its
  handler sits in the lab's class (`lab.ts`), and D288's pairing is reached from the lab alone.
- **The Play page hides the top bar on every device today:** its stage fills the window, which
  counts as full screen, so the overlay always draws and the signs sit in its bottom row.
- **The cast screen's key frames are the server's,** asked for at the position the remote
  estimates, not the castee's.

## Phases

Each phase ends with `main` working, the gate passing and staging walkable. Phases 1 and 2 touch
different code and run side by side.

### Phase 0: pin this week's black screen

A test that fails today for a member of a session casting from a page that names no film: the
castee must play the session's film. It passes at the end of phase 1.

### Phase 1: the server's one mechanism (D555 §2, §4–7)

1. **The cast becomes a relation in the hub:** a member's playing device and its remote in one
   session. Alone, the session of one stays. In a Watch Together session the castee attaches to
   that session as the member's playing device and the caster's page as its remote; no cast
   session is opened. A guest by link's cast is the same relation, no longer a path of its own.
2. **The film, position and acceptance leave the live registry** for the hub. The registry keeps
   connections, which tab is in front, and the hand-off to a tab loaded again.
3. **The castee says where it is:** starting, waiting for its OK, or running, with its position,
   each 5 s. The hub ends a cast whose castee has said nothing for 20 s, by a deadline of the
   cast's own. A sender may no longer end a cast as `receiverGone`.
4. **One end, with one list of reasons,** every kind of cast under it: a guest by link's gains the
   ends for silence, an unanswered question and idleness. An end takes the castee's credential
   back.
5. **The castee goes where its member goes.** Joining a session moves the cast into it; leaving,
   or the session's end, moves it to a session of one where the caster has rights to the video,
   and ends it where not. A host starting another session keeps the cast.
6. **The caster's own connection lost ends nothing,** and a caster returning is told its cast.

**Built (2026-11-25):** every step.

- A cast is a relation on its caster's membership: alone a session of one, in Watch Together
  the castee is the member's playing device, and it follows its member on a join, a leave and
  a session's end.
- The server alone decides a castee gone, at 20 s of silence; the castee reports each 5 s; a
  by-link guest's cast ends by the same path.
- The castee says starting, waiting for its OK or running, and the caster is told.
- A signed-in person takes part from one device; Move here, for a host and a guest alike, ends
  that person's cast and cuts the old device off.
- The hub alone holds what a cast plays, where it stands, whether it was taken and when it
  ends; the registry keeps presence and the hand-off.

**Found:**

- A guest cannot press Start while their part is on another device.
- A reloaded castee was handed the position its cast started from; it now gets where the cast
  stands.
- Nothing but its own test calls the hub's `OnSessionEnded` now; it goes in phase 6 unless
  phase 4 finds a use.

### Phase 2: one playing code for every device that plays (D555 §3)

1. **The framework-free parts move to the packages:** leading and following a session, asking
   for a playback, the receiver's commands for sound and picture, Up next's choice. The Play page
   and both receivers use them.
2. **The Google Cast receiver takes a regular cast outside the lab's class,** keeps its log, its
   idle close and its fills for an old engine, and loses what served the lab's sender, D288's
   pairing with it.

**Built (2026-11-25):** the regular cast's own handler on the Google Cast receiver, the lab's
sender and D288's pairing gone from it; and in the SDK one way each to put a leading player's
film on, to ask how a film plays, and to choose Up next.

**Left for after phase 1,** as each changes what the Google Cast receiver sends:

- **Following a session.** The receiver keeps its own follower, which seeks only when more than
  3 s off and plays on while waiting; the SDK's `followGroup` reports buffering, loaded chunks
  and quality besides.
- **Sound and picture commands.** The web receiver's are React hooks; the receiver's own stay.

**Built after phase 1 (2026-11-25):** both. The Google Cast receiver now follows its session by
the SDK's `followGroup`, as every other player does. It keeps no follower of its own. Leading its
cast, it carries out each move its remote makes once it is more than about 150 ms off, not 3 s,
and holds the group while it loads. Following, in its person's Watch Together session, it eases
its rate for small drift, seeks past 3 s, and pauses while the group waits for its leader. It now
tells its session its buffering, loaded chunks, quality switches and delivery as the Play page
does. What a receiver does with its remote's sound and picture choices now sits in the packages,
free of React: `workedByTheRemote` and `toldTheCastHowItPlays` in the SDK, and
`hearFilmChannel` and `chooseFilmChannel` in the player. The web receiver's hooks are thin
wrappers over them. The Google Cast receiver has no sound or picture panel yet, so it calls
neither. If it told a sound, its remote would show the 🎧 menu with a levelling switch it cannot
carry out. Whether it should show the film's volume (D555 §10) is the owner's to say.

**Found:** `idempotent.test.ts` scans the app alone, so the two playback requests, now in the
SDK, are outside its reading. Until phase 5 the lab's Chromecast sender pages get no answer.

### Phase 3: one target under one set of controls (D555 §8, §10)

1. **`PlayerSource` grows into the playback target:** play, pause, seek, sound, and the choices
   the menus make (subtitles, sound track, levelling, timing, picture). Two implementations: the
   video here, and the castee through the session.
2. **One set of menus, one Up next, one timeline wrapper** over the target. A choice with no use
   for the target, or no right for a guest, is not shown.
3. **Key frames are asked for at the castee's reported position.**

**Built (2026-11-25):** the target, `PlaybackTarget` (`web/apps/web/src/playback-target.ts`): the
bar's `PlayerSource` with which device plays, where it last said its film stands with no seek
chosen here counted (`reportedAtMs`), and its menus' choices. Its two implementations,
`useVideoTarget` for the video here and `useCasteeTarget` for the castee, which the cast remote
now uses. The one timeline, `TargetTimeline`, drawn by the bar, the overlay and the remote. The
remote's key frame is asked for at the castee's reported place: while a seek is sent it stays on
the frame it showed, and moves once the castee says it got there. One menus entry point, `TargetMenus`
(`web/apps/web/src/target-menus.tsx`), drawn by the bar, the overlay and the remote from the
target's choices, a castee's with no 🔈 (its level the pad's). One Up next, `TargetUpNext`
(`web/apps/web/src/up-next.tsx`), drawn by the Play page's offers and the remote's, reading where
the target stands and its end.

**Left:**

- **One body for the menus.** `TargetMenus` still delegates to the two bodies, `PlayerMenus` and
  `RemoteMenus`; making them one needs the video here to say its sound and picture as a castee
  does, which the receiver's command core moving into a package gives.
- **One body for Up next.** `TargetUpNext` delegates to `UpNextOffer` over the video and
  `CasteeUpNext` over a castee; they differ as the pages did: here a countdown, Still watching and
  Back to put it away, there no count and a ✕. Making that one is a design choice for the owner.
  How to move on is still the page's (the sequence's next, or the cast moved on), not the target's.
- **A guest's rights.** No choice is hidden for a guest that was not hidden before.

**Found:** the server does not tell a caster the castee's own reports: the castee's report of
its place only moves the group's position on the server (`followLeader`,
`internal/core/together/group.go`), and the roster carries no position. So `reportedAtMs` on a
castee is the group's position as last heard, moved on by the clock, without the remote's seek
aim. A frame at the place the castee reports itself needs the server to pass that report on.

### Phase 4: the one Play page (D555 §5, §6, §8–11)

1. **The top bar shows when the page is not full screen;** the stage fills the window only in
   full screen. The signs sit in the bottom bar only then.
2. **Upright:** the picture (the video, or the castee's key frames) with the controls under it,
   never hidden. **On its side, full screen, a computer:** the overlay as it is, with a larger
   play and pause.
3. **Casting is the same page with the other target.** A cast's end swaps the target back at the
   castee's last position, paused alone, playing in a session. A silent castee shows the
   reconnecting sign from 5 s.
4. **A guest by link uses the same page,** signed in by its membership, under its own top bar.
5. **Removed:** the cast remote and its pad, choices, offers and ends; the guest's player and
   tracks; the guest's cast page; with their tests.

**Built (item 3):** `PlayOrSendOn` mounts `CasteePlay` (`play-castee.tsx`) where it mounted the
cast remote: the way up and "On: ‹TV›" with when the film ends, the castee's key frame where the
video would be with Up next or a skip over it, and the Play page's bar (`page-controls.css`) over
the castee target: `TargetTimeline`, ⏮, the 10 s steps, play or pause, ⏭, `TargetMenus` and the
castee's volume. The cast's session hooks moved, unchanged, to `castee-session.tsx`, so
`castMoved` is still no end. Every end swaps back once, with no note, at this tab's own Stop's
answer, else where the castee last stood, else the session's place as it ended: paused alone,
playing for a Watch Together host or signed-in guest. Where the design was silent: nothing on the
castee (Ready on), an end goes home as before; the host's "unreachable, playing here" notice and
the lost-home notice are dropped (§5: no dialog explains the end); Cast again is gone from the
Play page.

**Left (item 3):** the reconnecting sign shows while the castee is not heard
(`useReceiverHeard`: its device gone from the connected devices, or its leading lost), not at
5 s of silence: the caster is not told the castee's 5 s reports, so a true 5 s sign needs the
server to pass the castee's report or its silence on. The cast remote, its pad and its notes went
with item 5.

**Walked (item 3, staging, 2026-11-25, the Pixel and the Chromecast):** casting, the Play page
shows the castee's key frame and its own bar; Play, Pause and both 10 s steps reach the
Chromecast; Stop casting brings the film back here paused at the Chromecast's place, with no
note.

A Watch Together session started and ended during the cast keeps the page's controls on the
Chromecast (the session's end still sends the page Home, from where the cast's sign leads back).
Stop casting after such a session failed, the page left empty: the server sent the cast's end on
a closed connection and panicked. Fixed (`fix(cast): a device gone since its cast left a session
is sent nothing at the cast's end`) and walked again. Seen once, not looked into: with that Stop
failing, the page showed nothing and Cast's dialog opened; what the page shows when a Stop fails
is for the next batch.

**Built (items 1–2):** the Play page's stage fills the window only while the page is full
screen (`keepFillingWhileFullScreen`, `film-fullscreen.ts`), full screen asked of the page so Next
and Previous keep it; out of it the top bar shows above the film. A television's stage still fills
the window always (D418 §1), so a Fire TV's Back still leaves the film filling it. The signs were
already in the bottom bar only in full screen (`SignsOverTheFilm`). Upright (`src/upright.ts`:
portrait and a coarse pointer) and not full screen, the controls are the bar under the picture;
otherwise the overlay, its play and pause 64 px with a 2rem icon. Where the design was silent: a
full screen the browser refuses still falls back to filling the window; a television's play keeps
the row's size, so a remote's Down from the scrub bar still lands on the row's first button
(D407 §3). VQ405.

**Built (items 1–2 over the castee, 2026-11-26):** the castee's page (`CasteePlay`) lays the
video's overlay (`OverlayOver`, the castee its target) over the key frame on its side, in full
screen and on a computer, its bar under the key frame only upright out of full screen. Full screen
stays with a film on the castee, the key frame filling the screen with "On: ‹TV›" and when it ends
over its top; "Ready on" still leaves it (D440 §5). Nothing in the overlay takes a press while the
castee is not heard, but the signs.

**Walked (the castee, staging, 2026-11-26, the Pixel and the Chromecast):** on its side the
overlay shows over the key frame, and its Play, Forward 10 s and Pause each reach the Chromecast;
Full screen fills the screen, "On: ‹TV›" and when it ends over the top, the signs and Leave full
screen in the bottom row, and leaving it brings the top bar back; upright the bar is under the key
frame; Stop casting swaps the film back to the phone, paused where the Chromecast stood. For the
owner: on its side and not full screen the stage is taller than the room under the top bar, so
Settings and Full screen sit half below the screen's edge until the page is scrolled; upright the
bar has no Full screen and no volume. Not walked: the castee's page on a computer.

**Walked (items 1–2, staging, 2026-11-26, the Pixel):** upright, the top bar, the picture and
the controls under it, all shown; on its side, the top bar and the overlay with the 64 px play;
full screen fills the screen, the signs in its bar, and leaving it brings the top bar back. For
the owner: on its side and not full screen, the top bar, the trail and the title leave the phone
about half its height for the picture; upright, Full screen sits alone on a row of its own.

**Built (item 4):** the guest's film (`together-viewer.tsx`) mounts the Play page's own
controls: `OverTheFilm` on its side and in full screen, `PageControls` under the picture upright,
the stage filling as the Play page's does. What the guest's link allows reaches them through
`CommandRightsContext` (`player-controls.tsx`): without play and pause no play shows and a press
does nothing; without seek no steps show and the timeline is disabled (D555 §10). A guest by link's
own top bar (`GuestTopBar`, `together-guest-controls.tsx`) carries the mark, the session's name,
Cast and Leave; in full screen Cast and Leave move to the session's panel (D555 §9). The old
corner (⏮ ⏭ and Full screen) is gone. Where the design was silent: the session's name is the
"Watching together" heading; the guest's overlay has no way up and no menus, its subtitles, audio
and offset staying under the picture; a signed-in guest gets the same controls, its Cast and Leave
staying in the panel under the shell's bar; AirPlay is offered as before.

**Built (item 5):** the cast remote is gone: its page (`cast-remote.tsx`), its pad, its end notes
(`cast-remote-ends.ts`, the notes in `cast-remote-offer.tsx`), Cast again (`cast-remote-again.ts`),
their rules in `cast.css`, their words and their tests. Each test of a behaviour D555 keeps runs on
the Play page over the castee (the `play-castee-*` tests); the remote's layout, its pad, the end's
notes, Play here and Cast again went untested, as D555 §5 and §8 end them. What the castee's page
still uses keeps its `cast-remote-` name: the offers and the picture, the menus, the heard receiver
and the report. The guest's player, tracks and Cast stay, the guest's page and its top bar mounting
them (D555 §2); the old corner's last word and export went. The pad's keys came back on the Play
page's one key handler (`useStageCommands`, `src/player-commands.ts`), whose answers work a
`PlayerStage`, the video or the castee with its stage as the picture, the overlay's target too:
over the castee Space plays or pauses, Left and Right step 10 s, + and - step its level 10 % and M
mutes it, F puts its stage full screen, under the bar and under the overlay alike; none while the
castee is not heard, typing or in a dialog; F and M, single letters, are the video's keys too
(`src/play-castee-keys.test.tsx`).

**Walked (item 4, staging, 2026-11-26, headless Chromium as host and as guest by link):** before
the host plays, the guest's page has its top bar, Leave, Talk and "Waiting for the host to start
a film"; with the host playing, the guest's film plays in step under the overlay, without menus or
a way up; the guest's Pause, Forward 10 s and Play each move the host's film to the same second; at
a phone's size upright the controls are under the picture. A guest whose link allows nothing has
no play and no steps, its timeline disabled, and its Space and Right arrow move nothing; Volume,
Talk and Full screen stay. Not walked: on the Pixel itself.

**Walked (phase 5, staging, 2026-11-26):** `/lab` and `/lab/sound` say there is nothing at the
address; This device links the sound timing test, and its page offers the clip.

**Walked (a cast's ends, 2026-11-26, Pixel 3a casting to the lead's Fire TV on staging):** the
cast took after OK on the Fire TV and played. Back on the Fire TV, pressed three times, left the film
on it and the phone over it, as the receiver's controlled mode says. Home on the Fire TV left the
cast standing too: Silk lives on behind the launcher, so the phone showed "On: Silk on Fire TV" for
the 72 s watched. Silk stopped outright, the phone had the film on its own Play page 20 s later,
paused at the Fire TV's position, with no dialog. VQ406 holds the ends only the owner can make. Hosting a Watch
Together session with a guest in a headless browser, the phone cast the session's film to the Fire TV
and Silk was stopped: the guest played on throughout, and 20 s later the phone played the film
itself, in step with the guest, with no dialog.

**Walked (keys over the castee, staging, 2026-11-26, Pixel 3a casting to the lead's Fire TV, keys
sent to the phone's page):** Space played and paused the Fire TV's film; Right moved it on 10 s and
Left back 10 s, twice; M muted it and M again gave the sound back; F took the stage into full screen
and out. + and - were pressed and nothing on the page showed whether the level moved. Not walked: a
real keyboard on a computer's page.

**Found by the owner (Firefox on a Mac, with a mouse, 2026-11-26), and built:** a page's arrival,
focused before any key or controller had pressed, showed the focus ring, and so did the picture of
a film the mouse had opened; and the Play page showed under the film what the overlay's menus hold
(Start over, subtitles, sound, levelling, the timing, the dialogue boost), twice on a computer.
Now a focus the page gives before any press, or after a pointer's, shows no ring on a device that
is not a television (`focusAfterNoPress`), the first key moving on from it with the ring; a
television rings it at first load, its remote being its only input. What the menus hold is
hidden, kept mounted, wherever the overlay lies over the picture, and shows under the film only
on a device held upright out of full screen (`OverlayHeld`); the alerts above it stay.

**Where focus starts (the owner, 2026-11-26):** "I'd like the home icon on the top bar to start
out highlighted on TVs, and selected but not showing the ring on desktop. All keyboard movement
happens from there and begins highlighting." Built: as the app opens, before any key or controller
has pressed, focus goes to the top bar's Home mark in place of the page's own arrival, ringed on a
television and held with no ring elsewhere; a page whose arrival is a text field, one drawn under no
top bar and the Play page keep their own, and every page opened later arrives as before. An arrow
now always shows the ring: where it lands, and on the focus it leaves where it cannot move (Right on
a row of one poster showed none). `web/apps/web/src/arrival.browser.test.tsx`.

**The remote's layout put back (the owner, 2026-11-26).** "Yes I meant the one with the
remote-control looking layout (3x3 pad). That should be the layout for portrait and landscape
mode when casting, and the layout in portrait for local playback. The others should have the
overlay on top of the video, and the video frame taking most of the real estate. In phone
landscape mode there's not enough room for that, so it's OK if it has to scroll." And on the
buttons: "The general rules for the buttons are: Only display if they can actually do something.
For the bottom button row, always right-aligned and only displayed if they can actually do
something (for submenus, only if there's at least one submenu item that can do something). Cast
and watch together buttons only show once (top bar if it's visible, bottom bar otherwise). Never
offer the fullscreen mode button when casting. If they do enable fullscreen mode through their
browser's controls while casting, we present a similar display layout to the non-fullscreen
one." Put back: the castee's Play page laid out as the remote was before D555 (the bar with *On:
‹TV›* and the end, the key frame with its offers and centre pill, the timeline with the time
under it, the 3×3 pad, the menus' row end-aligned), one column upright and two on its side or
wide, in and out of full screen, with no overlay and no ⤢, F doing nothing and the page leaving
full screen as it opens (`src/remote-pad.tsx`, `src/play-castee.tsx`); D555's one Play page, its
end and swap back, the reaching sign and its keys stay. Held upright, the video's controls under
the film are the same pad with no volume's row and ⤢ last in the menus' row, and `OverlayHeld` is
gone: the film's choices are the menus' in every layout.

**The remote pad's placements (the owner, 2026-11-26).** "The "Ends" line is placed wrong. It
should be under the backlink at the top. The playback position and total time display should be
directly under the scrub bar on the right-hand side, right up close so that there's just enough
clearance for the position gadget to not overlap. It can be this close because the text display
is non-interactive. In portrait mode, the scrub bar can be much closer to the video frame since
the frame itself is non-interactive." Built: casting, *Ends: ‹time›* sits directly under the way
up at its start, *On: ‹TV›* after it on that line, in every layout (on a phone on its side the
bar is now two lines, as upright); the Play page held upright has its trail and the film's name
instead of a way up, so Ends sits directly under the name, over the picture
(`src/ends-place.tsx`), and a Watch Together guest's keeps it among the offers. The time sits
under the scrub bar's track at its end, as the overlay's does: a half step under the track's
middle and a half step in from its end, clear of the thumb (`src/remote-pad.css`). Upright, the
track's middle sits one step (1 rem) under the picture or the key frame, the slider's touch box
reaching over the frame's foot; the castee's key frame sits at the foot of its cell so the slack
goes above it (`src/play-castee.css`, `src/page-controls.css`). Pinned in both engines by
`src/play-castee-layout.browser.test.tsx` and `src/play-layout.browser.test.tsx`.

**Next batch (found on these walks):**

- The sound timing clip does not play on staging: the Play page says it is no longer there
  (`notFound: no such file to play`). The server made the clip again after a deploy and the scan
  took it for a second place of a file found before (`copiesMerged`); the file's own place is no
  longer read, and a file plays only from its own place (`GetPlayableFile`), so the clip the system
  videos name cannot be played. The page and the route are the lab's unchanged; the fault is the
  server's and older than this rework.
- `failure-reports.test.ts` failed once under the gate's load (three reports where one was
  expected) and passes alone.
- A Fire TV sent Home keeps its cast: Silk plays on unseen behind the launcher and the caster's page
  goes on saying "On: Silk on Fire TV". Whether a receiver whose page is hidden should end its cast
  (or pause and say so) is the owner's to decide.
- A start that cannot be made says nothing (found 2026-11-26, file 1282 on a Chromecast, fixed at
  its cause by the encode's times starting at its first picture): the castee stays black and the
  caster's Play does nothing while the core answers each chunk 422 `cannotConvert`. Whether both
  say "This video cannot be played", and whether sooner than the core's give-up, is the owner's to
  decide; so is whether the 2026-11-03 rule (no step down on a chunk that cannot be made) stands.
- A file whose picture starts after its sound (an MP4 empty edit, 105 ms in file 1282) may now play
  with that much between picture and sound; measure before changing anything.
- What still lies under the stage after D556, each needing the owner's choice of a home: the
  Watch Together host's and guest's panel (news and voice), the castee's Watch Together controls,
  and the lasting alerts (where you are is no longer kept, stopped by the admin, the
  original-format line). The owner, 2026-11-26, gave two of them theirs: every Watch Together
  volume is in the bar's 🔈 (VolumeMenu; over a castee, CasteesVoices), and a guest's own tracks
  and sound timing are in its Subtitles and Sound menus (GuestTracksProvider), each shown only
  where it can do something.
- On Home, Down from the Home mark lands on the first Continue Watching card's more-actions button,
  not its poster; the owner's to say which is wanted.

### Phase 5: the lab goes (D555 §12)

Its pages, routes, words, menu entry and the server's `/lab/` endpoints.

**Built (phase 5).** The lab's pages, routes, words and menu entry (Experimental) are gone from the
web client, and `/lab`, `/lab/sound` and every page under them are not found. The server's `/lab/`
endpoints (the result log, the throughput body) and the client's `/lab/` files and Cast sender
policy go with them, and so does the admin's "Offer Device tests" setting (agreed 4) with its
`deviceTests` capability. The sound timing test stays, as agreed 4 says, but as a page of This
device's at `/this-device/sound-timing`, linked from This device for everyone, not of Settings:
Settings is the admin's alone, and a person times their own device's sound. The owner may still
move it. It plays the server's clip on the Play page as any film. Of the lab's helpers, the
page-made tone moved to `tone-wav.ts` (a start-refused test plays it). The SDK's device report and
loopback call are left; nothing in the apps uses them now. The lab's unwalkable checks (VQ55–57,
VQ59, VQ60, VQ242, VQ256, VQ286, VQ310) are marked withdrawn, VQ269 superseded by VQ275, and
VQ275 walks the kept page.

### Phase 6: close

Package documents and invariants, the amended decisions' status lines, the device walks
(verification-queue.md), one `make check-pr`, staging.

## Walked on staging (2026-11-25, after phase 1)

By the lead, on headless browsers, the Fire TV, the Pixel and the Chromecast, with two test
accounts.

**Passes:** a cast from a page naming no film while in a session; a castee loaded again (it comes
back within 1–3 s of where it stood, headless); the host's Move here ends the cast; the host
ending the session mid-cast; a signed-in guest's "Move it here" on a second device, casting and
not; a plain cast from the Pixel to the Chromecast, and its Stop.

**Failed, fixed and walked again the same day** (both pass on headless browsers):

1. **A signed-in person casting alone who joins a session by a link:** their castee is told
   nothing at the join and plays its own film on. At their leave it is handed twice, the second
   time the session's film from 0:00.
2. **A page left open does not learn its person's part changed elsewhere:** it offers Start a
   session while they are in one on another device, or Move here after the session ended. Nothing
   is pushed, and the dialog asks nothing when it opens.

**Fails phase 4 builds:** a silent castee's cast ends at 20 s as it should, with no reconnecting
sign from 5 s, and its caster stays a remote showing the old unreachable note; Stop casting alone
plays on at the caster where D555 §6 says paused.

**Not walked:** a by-link guest's cast to the Chromecast (the Pixel's Chrome showed no device
picker after its local-network question); a castee switched off at the wall; the Fire TV's reload
after phase 1's last step.

**Next batch, or the phase named:**

- A guest who leaves a session while casting lands on Home, the cast's remote behind the cast
  button (phase 4's landing).
- One change of part sends its "together" message five or six times, and a leave its "ended"
  several times; a "together" message lost while the live socket reconnects is not asked for again.
- The Watch Together dialog names the other device for one who was moved away from, and says
  "another of your devices" on a page that never held the part.

- A signed-in person opening their own session's invitation link is let in as a by-link guest
  under a name, a second place in the session for one person.
- The cast picker lists two devices of one name alike ("Chrome on Linux" twice).
- A session guest who casts has no Stop control within reach on its page (phase 4's page).
- While a guest follows a session, their tray still says they cast their own film (phase 4).
- The Chromecast's remote has back, pause and forward alone (phase 3's one set of controls).
- "Cast again" goes 30–45 s after the unreachable note (phase 4 removes the note).
- HTTP 425 answers repeat in the console while a film plays.

## Walked on staging (2026-11-25, after phase 2)

By the lead, the Pixel casting to the Chromecast, its video read over the Chromecast's devtools,
and a headless by-link guest.

**Passes:** the remote's place runs with the Chromecast's own video within 0.3 s; Pause, Play and
both 10 s steps reach it at once, paused and playing; a by-link guest who joins the caster's
session plays within 0.2 s of the Chromecast, and both follow the caster's pause, steps and play;
the host ending the session leaves the cast playing alone, its remote working; Stop casting ends
it.

**Failed, fixed and walked again the same day:**

1. **A person casting to the Chromecast who starts a session lost the cast 20 s later:** the
   receiver took the cast handed again in the session for the same hand repeated and stayed in the
   session it had left ("fix(cast-receiver): a Chromecast joins the session its cast moves to").

2. The caster's remote said "Casting ended" once they started a session while casting, the cast
   alive and its buttons working; a reload cleared it. The cast's own session of one was let go
   without a word, and the remote took its loss for the cast's end. The server now tells that
   session's devices `castMoved`, and the remote reads its casts again and joins where the cast
   went. Walked again: 45 s into the session the remote shows the cast live, and Pause and Play
   reach the Chromecast.
3. A host whose cast was the session's only player ended the session, and the Chromecast loaded
   its film again paused. The cast read where the session stood after its own leaving had paused
   it. It now reads it before. Walked again: the Chromecast plays on, and the remote's Pause
   reaches it.

**Seen, not faults of this batch's design, for the owner:** each move of a cast between sessions
has the Chromecast load its film again, about 3 s without a picture moving; and ending a session
from the remote takes the caster's page Home, the cast's sign still in the header.

**Not walked:** a small seek (under 3 s) on the device; the Fire TV's reload.

**Next batch:**

- A by-link guest's page is still the viewer with the browser's own controls (phase 4's page).
- The server's log of a loaded chunk gives a film position a thousandth of the real one
  (`chunkLoaded filmMs`), on the Chromecast at least.
- A voice call's step fails again and again between a guest's device and a panel of the host's.

## Tests

About 165 tests walk the cast remote, 57 the receiver, 188 Watch Together and 98 the lab, with
some 60 on the server's casts and 54 in the Google Cast receiver. A test of a behaviour that
stays is carried to the page that now has it, with `Test-Renamed`. A test of a behaviour D555
ends (the remote's notes at an end, Try again and the give-up, Play here, the second session,
the lab) goes, its commit carrying `Test-Weakened:` naming the section of D555. Each phase lists
both kinds in its first commit's body, so the gate's reading and the owner's are the same. The
Play page's own 355 tests stay and grow.

## Agreed by the owner (2026-11-25)

1. **A castee that says it is leaving ends its cast at once** (Back or Home on it, a Chromecast
   stopped); a socket that only closes waits the 20 s, so a page loaded again keeps its cast.
   D555 §5 says so.
2. **The castee reports each 5 s** while it plays.
3. **A session lives on one device for each person in it, that device with its castee where it
   casts** (the owner's words for D442's rule): the caster's page is the remote and the castee
   plays, the two as one.
4. **The sound-timing page stays,** as a page of Settings.
5. **The admin's "offer device tests" setting goes** with the lab.
6. **The television's own controls while it receives stay:** a player without menus.
7. **D288's pairing of a Chromecast through the lab goes.**

Under point 3 (the owner, 2026-11-25): **everyone in a session, host or guest, takes part from
one place, a device with its castee where it casts.** A signed-in person may take their seat
over on another device, by an act of theirs (the host's Move here, offered to a guest too), never
by a page loading. **Taking a seat over ends that person's cast:** a Chromecast started from one
browser cannot be worked from another, and a castee carried over could be left with no remote.
The seat then runs on the new device alone, playing itself. Today a signed-in guest gets a
credential for each of their devices and may be in on several: phase 1 ends that.

## After the close: everything the Chromecast can do (2026-11-26)

The owner, 2026-11-26: "Check what the chromecast is actually capable of. Anything it can do, I
want. I want the volume controls for watch together to be merged into the volume button in the
bottom bar. The tracks and sound timing and such should also be put in their relative submenus,
visible only when it makes sense."

Built for the Chromecast, with no server or protocol change (the core already relays a remote's
sound and picture choices to the device playing): its receiver works the remote's Sound menu as a
browser castee does, the audio track, dialogue boost and levelling (switched in place, each kept
for the person as the Play page keeps it) and the sound timing (for the film alone); its Subtitles
menu, the core's tracks shown as the video's own WebVTT track at the person's timing; and quality,
which it already followed. The pad's volume and mute set the TV's own volume through the caster's
Cast session, and follow the TV's remote. Not offered: the original file (the receiver asks for
no container of a file's own), the film's own channel (the device's volume stands for it), a
file's sound offset, and a subtitle found from the remote until the next film (the receiver's
README and its panels say why); the owner, 2026-11-27: left out until something needs them. After a
page reload the pad has no volume for a cast it did not
start: the Cast session is that page's (idiosyncrasies.md). VQ407 walks it on the device. The
Watch Together volumes and a guest's tracks and timing went into the bar's menus, as the
"Next batch" item above says.

## Next batch, from the 11-26 walk (the owner, 2026-11-26)

0. **A guest's page is the base display, its keys with it** (D557 §6, decided and not built; the
   owner: "OK, follow your recommendation"). A guest's page is its own build of the player and
   binds none of the player's commands (player-commands.ts): no key plays, pauses, seeks, moves
   the volume or goes full screen for a guest. Built on the one base display with only a guest's
   parts shown, each key does what its button does for a guest. First in the batch. **Built**
   (2026-11-26): the guest's page binds the Play page's keys, its stage taking play, pause and seeks
   only as far as the link's rights (`PlayerStage`, `useMovesCommands` for ⏮ and ⏭), the overlay's
   presses too; walked on devices by VQ410.
1. **A Chromecast's volume shows a number on the pad** (the owner: "Yes, show a number. Do this
   in the next batch"). The pad's Volume down, Mute and Volume up set the TV's own volume through
   the caster's Cast session and show none today; the session's `getVolume()` says it.
2. **A guest's cast's remote names the film the session moved to** (the owner: "Fix that in the
   next batch"). A guest's Next moves the host's own cast along; a guest's TV follows the session,
   but that guest's remote may still show the film it was handed. **Item 1 needed no code**
   (2026-11-26): the pad already shows "Volume ‹n› %" from the Cast session, a test now holds it,
   and VQ411 asks for it on a Chromecast. **Item 2 was built and taken back** the same night: the
   remote took the session's film once it differed from the film sent, and a plain cast's second
   film then never played (`cast.spec.ts`, the remote left at "Ready on"): the remote's page opens
   before it hears its session, the session says the second film before the server hands the cast
   again, and the remote showed that film with no length. The rule needs to know the cast is a
   guest's in a Watch Together session, not only that the two films differ. **Built again**
   (2026-11-27), narrowly: the cast's session's welcome already says it. A cast alone is in its own
   session of one, its caster that session's host, and a host's cast is in their Watch Together
   session as its host; a guest's cast takes the guest's part there, so its welcome says it is not
   the host's. Only a cast whose session has welcomed it as no host's (`filmOn`'s
   `isAGuestsCast`, `castee-session.tsx`) shows the session's film where it differs from the one
   sent; every other cast shows the film sent, as before. `play-castee-controls.test.tsx` holds
   both: the guest's remote names the film the session moved to, and a cast of one's own stays on
   the film sent, with its length, while its session says the next film first. No server change.
   **Walked on staging, 2026-11-27:** the Pixel joined a second account's session by link as a
   guest and cast to the Chromecast; at the host's Next and Previous the guest's remote went to
   the other episode each time (its address and its position), the TV with it. Its label did not
   change, the two episodes being of one series.
3. **Mute is not kept between films** (the owner: "No, mute doesn't stick"): as built, nothing to do.

Added by the owner, 2026-11-26, from the same walk's unfixed observations:

4. **A guest's page hung at "Checking your invitation…" until a reload**, three times on the walk.
   **Looked into, 2026-11-27**, nothing built. The likeliest route: the check
   (`POST /together/join/check`) is the guest's page's only request, and on a WebRTC connection
   that has stalled while still saying it is connected its body never leaves the browser; the sdk
   starts timing a request's answer only once the body has left (`rtc-request.ts`), so the check
   waits for ever and nothing replaces the connection. A scratch test showed it: a GET on such a
   connection failed at 40 s, the check was still pending at 400 s. A second route: where no
   connection works at all the transport waits for good and the page says nothing. Chromium closes
   a request's channel when its connection closes, so that is no route there; Firefox was not
   checked. Options for the owner: (a) a limit on the check in the page; (b) a limit in the sdk on
   a request whose body does not leave (the lead recommends it: the cause, for every request);
   (c) a ping on idle connections; (d) a "still trying" note for the guest.
   **Built, 2026-11-27 (D565, option b):** the sdk's silence watch now runs from a request's
   making, so a body that has not all left the browser within `silentAtMostMs` (40 s) fails the
   request with a `SilentRequestError`, as an unanswered one does: the transport replaces a
   connection nothing came over for as long, and the app asks again where it asks an unanswered
   request again. The limit is on the body's whole leaving, not on its progress: the API's bodies
   are small JSON (the core takes at most 1 MiB; a problem report, the largest, some kilobytes).
   After the body has left, the wait for the answer starts anew, as before. The guest's page asks
   its check once and, on any failure, shows the form (`useLinkState`), so it leaves "Checking
   your invitation…" for the form at 40 s, whose Join goes on the new connection. Proved by
   `rtc-request-silence.test.ts`, `rtc-transport-carrier.test.ts` and
   `apps/web/src/together-join-stalled.test.tsx`. The second route (no connection works at all)
   is not changed.
5. **A TV's cast that started late put its own film back on over the group's**, once, before the
   batch's fixes; not checked since.
6. **Stopping a Chromecast's cast left the phone's film paused.**
   **Built (D564 §1), 2026-11-27.** A cast this tab stops (the pad's or the panel's Stop casting,
   Chrome's own Stop for a Chromecast) comes back here, the person alone, at the castee's place and
   as the castee had it: playing where the server's answer to the Stop says it played, paused where
   it was paused, and, the answer failed, as the cast's session last said (`useSwapsBackAtTheEnd`,
   `play-castee.tsx`). Every other end still comes back paused alone, and a Watch Together session
   still plays on. A start the browser refuses is the player's, as at any film's start
   (`start-refused.ts`). Tests: `play-castee-stop.test.tsx`, `play-castee-ends.test.tsx`; the phone's
   own start: VQ415.
7. **A sleeping Fire TV shows as a silent cast waiting for OK.**
   **Built (D564 §2), 2026-11-27.** The receiver already said it: a page holding a cast for its
   press (D468 §2, `cast-start.tsx`) tells the cast's session its cast is `waitingForOK` (D555 §4),
   and the server puts that in the roster as the castee's `castState`. The caster's Play page now
   reads it (`castee-target.ts`'s `isCasteeWaitingForOKIn`: a device of the remote's own member,
   besides the remote, saying it) and shows "Waiting for someone to press OK on ‹TV›" in the offer
   area's centre, where "Reaching ‹TV›…" goes, with no spinner; it goes as the receiver says
   `running` or its holding page leaves. Reaching and the server lost still come first. No server
   change. Not covered: a film playing muted for its sound's press ("Press OK to turn the sound on",
   D468 §4) says nothing to the remote, as the film plays. Test:
   `play-castee-reaching.test.tsx`; on the devices: VQ416.
8. **The Invite dialog's "Make a link" button sits under a phone's keyboard.** **Built** (2026-11-27, D566):
   the page asks the browser to resize it under a keyboard, so every dialog fits the room above it.
9. **The Fire TV's own volume across films is walked on the device**: kept per device by the
   batch, and not yet walked there.
10. **A phone whose network changed could not reach the server for two minutes** (production, the
    owner's phone, 2026-11-26 21:36–21:39Z, Chrome on Android, film page of file 1322). Its four
    connections came direct from a mobile address; the core last heard them about 21:36:42; new
    ones came from a fixed line's IPv6 at 21:36:53 (eleven seconds), and the core last heard those
    about 21:37:16; none came again until 21:39:15. In between the Cast panel said "The server
    could not be reached. Check the connection and try again." and listed no device. The relay
    carried nothing of it. Not known: what changed on the phone the second time (its status bar
    showed a VPN), and whether 21:39:15 was the page's own doing or a reload. To find: how long a
    page takes to see its connections dead and make new ones, and whether the panel should wait
    for that and ask again before it says anything.
    **Looked into, 2026-11-26 (night).** The core's log has no "connection is back" line at
    21:39:15, and the page that connected then says it was opened, not kept: the old page never
    came back by itself in two minutes. The sdk has no state that waits minutes: a test loses a
    transport twice in a row and sees it keep trying (`rtc-network-loss.test.ts`). So either every
    try failed (a VPN leaves over IPv4 alone and production's core is behind DS-Lite, so only the
    relay could carry it, and the relay saw no allocation from the phone: the page reads its relay
    credentials once, at load, and they last 15 minutes), or the page was hidden or frozen while
    the phone's settings were open. Found beside it: a read is asked three times, but all three
    can go to the old connections in the ten seconds before they count as gone, so the Cast panel
    gives up before a new connection exists. Options for the owner, none built: (A) hand a retry
    only to a connection newer than the failure (recommended); (B) fetch relay credentials again
    for a try made long after the last fetch (recommended if the VPN is confirmed); (C) the panel
    says it is reconnecting and keeps its list for a while before saying unreachable (wording and
    wait are the owner's); (D) take a `navigator.connection` change as a sign to check.
    **Built** (2026-11-27): (A) first (a request waits for a connection that can carry it,
    `canCarry`); then, as D564 decided, (B) (§3): a transport holds its ICE servers in
    `RelayCredentials` (`rtc-relay-credentials.ts`), and a rung of a connection's ladder that asks a
    relay first fetches them again from the discovery's `/paths` (`connectThrough`'s
    `iceServersAgain`) where they are within a minute of the expiry their TURN username states, the
    rung without a relay waiting on no fetch; walked on a device by VQ417. And (C) (§4): while the
    live connection is down, the Cast panel says "Reconnecting to the server…" and keeps the
    devices it listed last, disabled, though a read of them fails meanwhile; once it has been down
    `reconnectingSaidForMs` (30 s, `cast-to.tsx`), it says the server could not be reached,
    whether or not a read failed (the clock alone, the owner, 2026-11-27); walked on a device by
    VQ418. And (D) (the owner, 2026-11-27, "Do it"): where the browser has
    `navigator.connection` (Chrome on Android), `onWayBack` (`rtc-connect.ts`) hears its
    `change` events, one burst of them within `networkChangeBurstMs` making one sign
    ("changed"), and the transport's `networkChanged` (`rtc-transport.ts`) has each connection
    judged at once by its own `watchGone` (`rtc-peer.ts`), where one disconnected then or within
    `disconnectedGraceMs` after is gone at once, its grace cut short, and replaced; one still
    connected is left, and tries and waits under way are untouched; walked on a device by VQ419.
    The owner, 2026-11-27, on what is not known: only Tailscale runs on the phone, usually up all
    the time, though it has been known to stick down; whether it was up then, and whether the page
    was reloaded at 21:39, the owner does not remember. The cause of that night stays unknown.
11. **`make check-pr` runs past its budget** (the owner, 2026-11-27: "Look for savings, but do it
    in the next batch"): its median on the sandbox's machine is 316 s against 300 s. Find what can
    be cut before the budget is raised.
12. **Join with no name does nothing and says nothing** (found walking a guest-led film's end,
    2026-11-27): a guest who presses Join on an empty name field gets no message.
13. **A group shows "playing" for about half a minute while nobody plays** (the same walk): the
    host had stalled near the film's end.
14. **A guest's page logs a 425 again and again after a seek** (the same walk); nothing was seen
    wrong on the page.
15. **A cast being prepared leaves the caster's controls live and says nothing of its state** (the
    owner, staging, 2026-11-27: "the playback controls on the caster side are selectable, and
    there's no way to know what the state is or what those buttons will actually do"). The pad is
    disabled with "Reaching ‹TV›…" only until the TV is reached; from then until the TV first shows
    the film it is live and unexplained. Decided as D569: one stated state from
    the choice of the TV until its first picture, the transport disabled through it and only
    "Stop casting" live; a TV standing ready and paused says so, with Play the one action.

    Built (2026-11-27, VQ424). Left by the build, for the owner to weigh: a TV lost and found again
    mid-cast gets none of the start's notices, as they show only to a page that saw the start; the
    volume's number is hidden while a start notice shows; `decideAgainFor` in `play.tsx` still reads
    the video element's pause, not the person's.
16. **A test's stray timer fails a run now and then** (the gate, 2026-11-27, twice in a day): in
    `apps/web/src/together-host-paused.test.tsx` a `StartGate` look-again timer fires after the
    page's teardown ("Failed to execute 'dispatchEvent'"), an unhandled error that fails the whole
    web test run; the file passes alone and the next run passes. The timer is to end with its gate.
