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

