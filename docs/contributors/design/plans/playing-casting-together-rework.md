# The rework: playing, casting and Watch Together as one (D555)

**Status:** proposed for the owner's review (2026-11-25). Nothing here is built. It plans D555,
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

### Phase 2: one playing code for every device that plays (D555 §3)

1. **The framework-free parts move to the packages:** leading and following a session, asking
   for a playback, the receiver's commands for sound and picture, Up next's choice. The Play page
   and both receivers use them.
2. **The Google Cast receiver takes a regular cast outside the lab's class,** keeps its log, its
   idle close and its fills for an old engine, and loses what served the lab's sender, D288's
   pairing with it.

### Phase 3: one target under one set of controls (D555 §8, §10)

1. **`PlayerSource` grows into the playback target:** play, pause, seek, sound, and the choices
   the menus make (subtitles, sound track, levelling, timing, picture). Two implementations: the
   video here, and the castee through the session.
2. **One set of menus, one Up next, one timeline wrapper** over the target. A choice with no use
   for the target, or no right for a guest, is not shown.
3. **Key frames are asked for at the castee's reported position.**

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

### Phase 5: the lab goes (D555 §12)

Its pages, routes, words, menu entry and the server's `/lab/` endpoints.

### Phase 6: close

Package documents and invariants, the amended decisions' status lines, the device walks
(verification-queue.md), one `make check-pr`, staging.

## Tests

About 165 tests walk the cast remote, 57 the receiver, 188 Watch Together and 98 the lab, with
some 60 on the server's casts and 54 in the Google Cast receiver. A test of a behaviour that
stays is carried to the page that now has it, with `Test-Renamed`. A test of a behaviour D555
ends (the remote's notes at an end, Try again and the give-up, Play here, the second session,
the lab) goes, its commit carrying `Test-Weakened:` naming the section of D555. Each phase lists
both kinds in its first commit's body, so the gate's reading and the owner's are the same. The
Play page's own 355 tests stay and grow.

## For the owner to decide

1. **A page loaded again cannot be told from a page closed.** Both close the socket. D555 §5 ends
   a cast "at once" on a page closed, which would end it on Silk's own reloads too. Proposed:
   "at once" is for a castee that says it is leaving (Back or Home on it, a Chromecast stopped);
   a socket that only closes waits the 20 s.
2. **The castee reports each 5 s** while it plays, for the 20 s to be four reports.
3. **The host's session lives on one device today (D442),** and a host's sign-in elsewhere cuts
   the receiver off. Proposed: one *playing* device, its remote beside it.
4. **The lab's sound-timing page** is what This device links to for setting a device's sound
   delay, with the server's timing clip. Proposed: it stays, as a page of Settings, not of a lab.
5. **The admin's "offer device tests" setting** goes with the lab.
6. **The television's own controls while it receives** stay as they are: a player without menus.
7. **A Chromecast paired by D288 through the lab** would stop being a cast target. None is known
   to be in use; a regular cast needs no pairing.
