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
server to pass the castee's report or its silence on. `cast-remote.tsx`, its pad, its notes
(`cast-remote-ends.ts`, `cast-remote-again.ts`, the notes in `cast-remote-offer.tsx`) and their
tests now have no user outside tests; they go with item 5, their tests moved to the page that
keeps each behaviour.

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
castee is not heard, but the signs. Left: the walk on the Pixel and a computer.

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

**Left (item 4):** to walk on a phone and a computer as a guest by link.

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

