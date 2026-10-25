> **ABOUTME:** Casting to one of a person's own signed-in devices (D337): the list of devices
> connected now, the receiver's full-screen mode, the sender as its remote, taking control back,
> the last cast winning, the receiver's own media keys, and the same inside Watch Together. Read
> before building any of it, or before adding a live connection for signed-in devices.

# Cast to another device

- **Status:** SPECIFIED, BEING BUILT — 2026-10-25. The owner decided the route and its behaviour
  ([D337](../../decisions/working-notes.md#d337--casting-starts-with-a-persons-own-signed-in-devices-one-chosen-from-those-connected-now-plays-the-film-the-one-that-cast-becomes-its-remote-and-either-may-take-control-back)),
  then, the same day, accepted every recommendation in §9, the control channel included (§3: A,
  through Watch Together's hub, with a presence socket for presence and the hand-off). §8's steps
  are being built in order.
- **Depends on:** [closed-beta.md](closed-beta.md) (B5, the living room: TV sign-in by code, voice
  on a second device), [tv-device-tests.md](tv-device-tests.md) (what Silk and TV Bro let a page
  hear).
- **Governing decisions:** D337; [D88](../../decisions/working-notes.md#d88--casting-is-remote-control-of-another-client-which-is-watch-together-with-a-group-of-one)
  (casting is Watch Together with a group of one);
  [D120](../../decisions/working-notes.md#d120--a-participant-is-a-person-devices-attach-to-them-with-roles-and-the-cap-counts-people)
  and [D124](../../decisions/working-notes.md#d124--the-playback-role-may-be-a-cast-receiver-and-for-a-guest-that-is-the-best-path-the-phone-controls-it-does-not-relay)
  (devices with roles; the receiver fetches, the phone controls);
  [D135](../../decisions/working-notes.md#d135--casting-on-your-own-is-a-device-delegation-not-an-invitation-the-credential-is-the-session-and-the-queue-is-next-up)
  and [D136](../../decisions/working-notes.md#d136--control-belongs-to-the-cast-session-not-to-the-sender-and-an-externally-paused-receiver-must-say-so),
  as D337 narrowed them;
  [D154](../../decisions/working-notes.md#d154--one-grant-table-opaque-and-revocable-the-session-credential-never-appears-in-a-url)
  (no credential in a URL);
  [D165](../../decisions/working-notes.md#d165--a-heartbeat-is-not-an-event-the-log-records-state-changes-and-watched-spans-there-is-no-retention-horizon-and-an-offline-device-must-not-be-able-to-rewind-you)
  (an offline device must not rewind a person);
  [D188](../../decisions/working-notes.md#d188--the-interface-is-designed-against-the-fields-recorded-failures-accessibility-shapes-the-design-but-does-not-gate-cut-a-her-television-plays-and-her-phone-talks)
  §3–§4; [D267](../../decisions/working-notes.md#d267--a-watch-together-membership-is-kept-by-the-browser-that-joined-or-by-the-account-of-whoever-is-signed-in-and-one-member-may-be-on-several-devices)
  §2–§3; [D268](../../decisions/working-notes.md#d268--a-watch-together-player-corrects-drift-as-its-device-measures-it-places-its-group-by-recent-pongs-starts-at-a-moment-the-server-names-and-its-hosts-moves-are-taken-at-the-controls)
  (a leader's moves are taken at its controls);
  [D288](../../decisions/working-notes.md#d288--a-device-is-activated-by-the-code-it-shows-entered-on-a-device-already-signed-in-and-a-check-code-both-show)
  and [D321](../../decisions/working-notes.md#d321--a-tv-shows-a-qr-code-carrying-its-keys-hash-which-activates-it-with-one-tap-the-typed-codes-check-code-is-committed)
  (how a TV is signed in).
- **Evidence:** [casting-2026-10-25.md](../research/casting-2026-10-25.md) §4–§6,
  [casting-review-2026-10-25.md](../research/casting-review-2026-10-25.md) §2 (the phone page's
  risk), [watch-together-split-devices-2026-10-24.md](../research/watch-together-split-devices-2026-10-24.md)
  §3–§4 (where the code assumes one tab, and the remote page),
  [idiosyncrasies.md](../../idiosyncrasies.md#silk-on-fire-tv-keeps-the-d-pad-for-its-pointer-unless-its-spatial-navigation-mode-is-on)
  (Silk on Fire TV), [tv-device-tests.md](tv-device-tests.md) §5.
- **Code read at** An earlier commit. Efforts below are **rough, unverified estimates** in focused days,
  by analogy with this tree's history; none was planned in detail.

## 1. What the person sees

1. On a device playing a film (§9 Q1 asks where else), they choose **Cast to another device**.
2. A page lists their **other devices open and connected now**, on any page: each by its name and
   when it was last used. A device signed in but closed is not listed.
3. They choose one. **That device plays the film full screen** from where the sender stood, with a
   brief "Controlled from ‹sender device›" that fades, and nothing else on screen.
4. **The sender becomes a remote**: play and pause, the timeline and seeking, subtitles, sound and
   the dialogue boost, volume. It plays nothing. In Watch Together it keeps the voice call.
5. **On the receiver**, any key or tap shows **Take back control**; one more press takes it, and
   the receiver is an ordinary player again. Back on a Fire TV does the same (§6.1). The receiver's
   own remote, keyboard and media keys work all along (§6.4).
6. **Casting again from another of the person's devices takes over** at once; the earlier sender is
   told. If the sender goes away, the receiver plays on.

## 2. What exists to reuse

| Piece | Where | What it gives | What it lacks for this |
| --- | --- | --- | --- |
| A person's devices | [`internal/core/auth/devices.go`](../../../../internal/core/auth/devices.go) (`Devices`, `Device`), `GET /me/devices` ([`internal/core/api/devices.go`](../../../../internal/core/api/devices.go)), [`your-devices.tsx`](../../../../web/apps/web/src/your-devices.tsx) | each device's id, name, platform, `LastUsedAt` (including uses not yet written), whether it is signed in | anything about *now*: `LastUsedAt` is a request's time, not an open page |
| The device a request is from | `auth.Principal.DeviceID` ([`auth.go`](../../../../internal/core/auth/auth.go)) | which device a session is on; 0 for a session from before devices were made at sign-in | — (a device 0 cannot be listed or cast to) |
| TV sign-in | D288 / D321; [`device-activation.tsx`](../../../../web/apps/web/src/device-activation.tsx), [`code-sign-in.tsx`](../../../../web/apps/web/src/code-sign-in.tsx), the SDK's `device-code.ts` | a TV becomes one of the person's devices, by a code it shows, at home or through the paid service | nothing: D337's receiver is exactly such a device |
| A live connection | Watch Together's socket, [`internal/core/api/together_socket.go`](../../../../internal/core/api/together_socket.go): credentials in the hello, never the URL (D154); pings every 15 s, a device silent for three is gone; carried over WebRTC when away ([`internal/core/rtcapi`](../../../../internal/core/rtcapi/doc.go)'s `MessageConn`); the client's reconnects in the SDK's `together-connection.ts` | the pattern, the constants and the away path for a new socket | it admits only a session's members: a device on the home page has none |
| The hub and device roles | [`internal/core/together`](../../../../internal/core/together/doc.go): `Hub.Open`, `Hub.Attach` with `RolePlayback`, `RoleVoice`, `RoleBoth`; commands `play`, `pause`, `seek`, `rate`, `stop`, per member, not per device; `group` state to every device; `Hub.Mine`, a signed-in person's sessions on any device (D267 §2) | D88's remote-control protocol, already per person, with state the remote can draw a timeline from | a session kind with no party (§3); commands for a person's own choices (subtitles, track, volume) |
| The host's player leading | [`together-host.tsx`](../../../../web/apps/web/src/together-host.tsx) (`HostPlaysTogether`, mounted by `play.tsx`'s `PlayerView`), the SDK's `together-lead.ts` | a Play page's own video leads a session, its local moves becoming the group's commands (D268) | — |
| Playing a film | [`play.tsx`](../../../../web/apps/web/src/play.tsx) (`PlayView`), [`watching.ts`](../../../../web/apps/web/src/watching.ts) (`requestPlayback`: `POST /playback`), [`film-player.tsx`](../../../../web/apps/web/src/film-player.tsx) (the video with its own controls) | the receiver plays by its own decision under its own sign-in (D2, D124) | a receiver composition: no controls, no title, full window |
| Where the video stood | `positionToMoveFrom` in `play.tsx`; `ReopenAt` | the position from the video itself rather than the last report, up to `reportEveryMs` (10 s) old | — |
| Progress | [`play-state.ts`](../../../../web/apps/web/src/play-state.ts) (`POST /playback/{token}/report`), `leavePlayback` in [`interest.ts`](../../../../web/apps/web/src/interest.ts) (`DELETE /playback/{token}`) | the device that plays reports; a playback can be left | — |
| Subtitles, sound, volume | `subtitles.tsx`, `sound.tsx` (`BoostSwitch`), `film-volume.ts`, `levelling.tsx` | the receiver's own choices, as on any Play page | a way for another device to set them |
| Media Session | the player's `watchMediaSession` (`web/packages/player/src/event-context.ts`) | play and pause from the system's media controls, recorded for the playback's events | seeking actions; Fire TV's key 179 (§6.4) |
| An admin's stop | `POST /now-playing/{id}/stop` ([`now_playing.go`](../../../../internal/core/api/now_playing.go)) | a message reaching a player within one report interval | far too slow for a remote: 10 s per command. Not a control channel |
| Prototypes | [`lab-extend-channel.ts`](../../../../web/apps/web/src/lab-extend-channel.ts) (a remote and a player over a `BroadcastChannel`), [`lab-remote.tsx`](../../../../web/apps/web/src/lab-remote.tsx) (the phone's endurance probe, VQ56) | the remote's message shape, tried; what a phone page survives | a channel between two machines |

## 3. The live connection: presence and control

Two jobs, which need not share a mechanism:

- **Presence** (D337 §2): which of a person's devices are open now. Nothing has it. It needs a
  connection held by **every signed-in page**, not only one in a session: a new socket,
  `/me/live` say, built on the together socket's pattern (hello with the session token; pings;
  silence limit; reconnect with back-off; over WebRTC when away). The core keeps, in memory, each
  person's connected devices and pushes the list to that person's pages when it changes. The same
  socket carries the **hand-off**: "receive this film, from here, controlled by that device", "you
  were replaced", "control was taken back".
- **Control**: the remote's play, pause and seek, the receiver's state back, and the person's own
  choices (subtitles, audio track, boost, volume).

For control there are two shapes:

| | **A. Through Watch Together's hub** (D88: a cast is a session of one) | **B. On the presence socket** (a channel of the person's own) |
| --- | --- | --- |
| Solo | the sender opens a session for the person with no links, of a kind no one else sees; the receiver gets its credential through `Hub.Mine` (D267 §2) and leads (`HostPlaysTogether`); the sender attaches as a device without a player and commands it | the sender's commands are relayed to the receiver, which sends its state back; the receiver is an ordinary Play page |
| In Watch Together | the same session: the receiver becomes the person's player and the sender keeps voice. One mechanism for both | the receiver joins the hub as the person's player anyway, so the shared commands go to the hub there, and the presence socket carries them only when solo: two transport paths |
| The receiver's own pause | the leader's local moves already become the group's commands (D268), which the remote sees in `group` | new: the receiver reports its state on the socket |
| New in the core | a session kind without links, discovery locators, the party's roster or a place in the host's Watch Together page; "a host has one session" kept with a cast beside a party | a relay between two devices of one person, and a small state message |
| Cost to first use (unverified) | higher: about 4–6 days for the transport step | lower: about 3–4 days |
| Rule 11 (one way to do each job) | kept | play, pause and seek in two protocols, which D88 was written to prevent |

**Decided (the owner, 2026-10-25, §9 Q7): A** for control, with the presence socket for presence
and the hand-off only. Watch Together needs the hub path regardless (D337 §5), so B saves days now
and leaves two remote-control paths to keep in step, which is the outcome D88 records as the usual
failure. The person's own choices (subtitles, track, boost, volume) are new in either shape; the
split-device sketch's §4.3 relays them only to the same member's player, which A's session gives
for free. If the owner prefers one socket per device, the together socket could later become the
general socket of a signed-in device; that refactor is not needed for the first build.

## 4. The receiver

- **Entering.** The hand-off names the file, the position and the sender's choices (subtitle track,
  audio track, boost, volume). The receiver asks for its own playback (`POST /playback`), so it gets
  the rendition its own browser can play (D2, D124), starts at the sender's position, and takes the
  sender's choices as its own for this playback.
- **The look.** The film fills the window, no controls and no title; "Controlled from ‹sender
  device›" fades after a few seconds. The page composes `PlayView` with its composition's
  `hasControls` off (`film-player.tsx`'s `FilmComposition`).
- **Full screen and sound without a press are not given.** The Fullscreen API needs a recent user
  activation, and a browser may refuse to start sound without one (iPhone Safari does:
  [idiosyncrasies](../../idiosyncrasies.md#iphone-safari-refuses-a-films-sound-that-no-recent-tap-started-and-a-tap-long-before-does-not-count)).
  A page started by a message has neither. The receiver fills the window by layout rather than the
  API (a TV browser's window is the screen already), and if `play()` is refused it shows one line,
  "Press OK to start", whose press starts it. Whether Silk and TV Bro refuse is a device check
  (§8 step 3's VQ).
- **A reload comes back receiving.** The core keeps who receives and who controls, in memory; a
  receiver whose page reloads (Silk reloads on a change of navigation mode, a TV browser killed on
  an input switch and reopened) says hello and is told to receive again.
- **Progress is the receiver's** (recommended, §9 Q2): it plays, so it reports, as any Play page
  does. The sender leaves its own playback at the hand-off (`leavePlayback`), so no stale report from
  it can move the person's position back (D165).

## 5. The sender's remote

- **What it shows:** the film's title and poster, play and pause, the timeline with the position
  and seeking, back and forward by a step, subtitles, the audio track, sound (the boost, D336 §2),
  volume, and **Stop casting**, which ends receiving and offers to play here from the receiver's
  position (§9 Q5).
- **Its state comes from the receiver**, never from its own guess: the timeline is drawn from what
  the receiver last said and moved on by the clock between, as a Watch Together follower's view of
  the group is. A change the receiver made itself (§6.4) shows within a second.
- **It holds no player.** It loads no film and takes no share of the upload (D286 §5's count).
- **The phone page's own risk** (casting-review §2): a locked iPhone, a backgrounded tab. A remote
  that drops must cost nothing but the remote: the receiver plays on (§6.3), and the remote comes
  back by itself on `visibilitychange`, as voice already does (D281 §6). Its endurance is VQ56's.

## 6. Control

### 6.1 Taking control back

- Any key that reaches the page, or a tap or click, shows **Take back control** with the sender's
  name; a second press confirms; a few seconds without one hides it again.
- **Fire TV's Back:** in Silk, Back is never a key: each press steps the page's history once, in
  either navigation mode (lab results `Q5JAAP2N`, `HSH4M2QQ`, `BHJ6AJ7I`; idiosyncrasies, Silk on
  Fire TV). The receiver mode pushes a history entry as it starts and treats a `popstate` as a
  press, pushing the entry again so the confirming Back is caught too. In spatial mode OK arrives
  as `Enter`; in pointer mode no arrow and no OK key reaches the page, and OK presumably clicks
  where the pointer is, which counts as a tap: UNVERIFIED, part of step 6's VQ.
- Taken back, the receiver keeps playing as an ordinary Play page, its controls shown, and the
  sender's remote says "‹receiver› took control back" (§9 Q5 for what it then offers).

### 6.2 The last cast wins

- A receiver has at most one controller. A cast by another of the same person's devices replaces
  the controller at once; the earlier sender's remote is told "Now controlled from ‹device›" and
  ends. A cast to a device already receiving changes the film if the new sender plays another.
- Only the person's own devices can cast to it (D337 §1): the core refuses a hand-off naming a
  device that is not the asker's, as `liveDevice` already refuses another person's device.

### 6.3 The sender gone

The receiver plays on; nothing ends the cast but take-back, a new cast, Stop casting, or the film's
end (§9 Q4). The sender's page coming back finds its cast through the core and is the remote again,
unless another cast replaced it meanwhile.

### 6.4 The receiver's own media controls

They work all the time (D337 §7), and every change is reported upward, never inferred from a
position standing still (D136):

- **Fire TV Silk**: play/pause arrives as a `keydown` of `MediaPlayPause`, keyCode 179, one per
  press, and never as a Media Session action; rewind, fast forward and the volume keys never reach
  the page (tv-device-tests.md §5, H2). The receiver page listens for 179; seeking from the TV's
  own remote is by the D-pad, which reaches the page only in Silk's spatial mode.
- **Elsewhere**: Media Session's `play`, `pause` and, where offered, `seekbackward`, `seekforward`
  and `seekto`; `watchMediaSession` handles only the first two today.
- **A keyboard**: space and the arrows, while no prompt is up; any other key raises the take-back
  prompt.
- **Volume** from the TV's own remote changes the TV's output, which the page cannot see; only the
  page's own film volume is reported.

## 7. With Watch Together

- **Who can cast:** the host and any member who joined signed in to the host's core, since their
  sessions follow them to every device (D267 §2). A guest who came by a link has one browser and no
  other device of the same account, so cannot (D337, consequences); a guest's TV waits for the
  later "takeover for this session only".
- **The receiver becomes the person's player** in the session: a host's receiver leads ("whichever
  device the host plays on leads", D267 §3); a member's follows the group like any player (D250).
  The sender stops playing and keeps its voice device, so the person counts as one player in the
  upload share and one row in the roster (D120).
- **Pacing:** a voice-live flag is per tab today (the SDK's `voice-live.ts`), so the receiver would
  not know its person is talking on the phone and would not make room for voice (D262 §2). The core
  must tell a member's player that the member's voice is live (the split-device sketch's §4.4).
- **Echo:** the film now plays from the TV's speakers while the phone's microphone is open; Talk
  (D281) rather than an open microphone, and earbuds, as the sketch's §4.10 says.
- **Upload overload:** `SendAwayNewest` ends every device of the member it picks, voice included;
  with a cast it should end the player and keep the voice (the sketch's §3, Q7). Not needed for
  the first build; noted.

## 8. Build, in steps

Each step ends green with a test that fails when it is reverted (D190). The browser tier is
Playwright (`make test-browser`), where two or three browser contexts signed in as the same person
are two or three devices, as `web/e2e/together.spec.ts` runs several people. Device checks are VQ
items added to [verification-queue.md](../../verification-queue.md) when the step is built.

1. **Presence.** The core's registry of each person's connected devices, in memory, and the
   `/me/live` socket: hello with the session token, pings, the silence limit, over WebRTC too; the
   web client opens it from the app shell on every signed-in page and reconnects. About 2–3 days.
   *Tests:* Go: a device is listed while its socket is open and gone after the silence limit; a
   person sees only their own devices; a session with no device is never listed; the socket over
   `rtcapitest`'s client. Web: reconnect after a drop.
2. **The "Cast to" page.** The person's connected devices but this one, with names and last use
   from `/me/devices`; the entry where §9 Q1 puts it. About 1–2 days. *Tests:* web unit; Playwright:
   device B opens any page and appears in device A's list, B closes and leaves it.
3. **The hand-off and the receiver mode** (§4). About 3–4 days. *Tests:* web unit for the receiver
   composition and the "Press OK to start" line; Playwright: A casts, B plays the film within a
   second or two of A's position, and A's playback is left (one playback in `/now-playing`).
   *VQ:* on a Fire TV in Silk (pointer and spatial modes) and on the TCL in TV Bro: the receiver
   starts with sound and fills the screen with no press, or shows the line and starts on OK.
4. **The remote and its transport** (§3, §5): play, pause, seek, volume, the receiver's state back.
   About 4–6 days by A, 3–4 by B. *Tests:* Go: by A, a cast session has no links, no locators and
   is not listed as a party, and a host keeps their party beside it; by B, a command reaches only
   the person's receiver. Playwright: A's pause pauses B; a seek on A moves B.
5. **The person's choices** (§5): subtitles, audio track, boost, volume sent to the receiver, which
   reports what it shows. About 2–3 days. *Tests:* Go: a choice reaches only the same person's
   receiver; Playwright: subtitles chosen on A show on B.
6. **Take-back, the last cast, the sender gone** (§6.1–§6.3). About 2 days. *Tests:* web unit: one
   press shows the prompt, a second takes control, a `popstate` counts as a press; Playwright with
   three contexts: C casts to B after A, A is told and B follows C; A closes and B plays on; B takes
   control back and A is told. *VQ:* on the Fire TV, Back raises the prompt and a second Back takes
   control; on the TCL, the remote's Back does the same.
7. **The receiver's media keys** (§6.4). About 1 day. *Tests:* web unit: a `keydown` of keyCode 179
   and Media Session's handlers pause and play the receiver and report it; Playwright: B's key
   shows paused on A. *VQ:* on the Fire TV, the remote's play/pause pauses the receiver and the
   phone shows it; on the TCL, the same.
8. **Watch Together** (§7). About 4–6 days. *Tests:* Go: a host's receiver leads, the sender keeps
   voice and counts no player; Playwright: the host casts mid-session and a guest follows the
   receiver; the member's voice-live reaches the receiver. *VQ:* the phone keeps voice for a whole
   film while the TV plays (with VQ56's endurance runs).

Steps 1–7 are casting alone, about 2.5–4 weeks; step 8 adds about 1. Unverified.

## 9. The owner's questions, answered

The owner accepted every recommendation below on 2026-10-25: the player first and a title's page
next (Q1); the receiver reports (Q2); a sender playing another film sends it on (Q3); at the end,
the next episode or the receiver's home page (Q4); "Play here from ‹position›" (Q5); background
tabs count, no opt-out (Q6); control by A (Q7); the fading line is enough (Q8).

1. **Where is "Cast to another device" offered?** The player only, or also a title's page (cast
   before anything plays), and the home page? *Recommend:* the player first; a title's page next,
   since a TV is where a film is meant to start.
2. **Which device reports the viewing?** *Recommend:* the receiver, since it plays (§4); the sender
   leaves its playback at the hand-off.
3. **Can the remote put on another film,** an episode's next or anything the person browses to on
   the sender, keeping the receiver? *Recommend:* yes: a sender that plays another film while
   casting sends it to its receiver, as a host's next film reaches the session.
4. **How does receiving end at a film's end?** The receiver stays receiving on an end screen for the
   next film; or goes back to its own home page and the remote ends; or plays Next Up when the
   series has one. *Recommend:* the next episode as the sender would play it, else the receiver's
   home page and the remote told.
5. **What does the sender show once control is taken back, or replaced?** A note and the title's
   page; or "Play here from ‹position›". *Recommend:* the note with "Play here".
6. **Does an idle device count?** D337 §2 says any page; settled. Left to say: whether a page in a
   background tab (a laptop's other tab) counts while its socket lives, and whether a device
   should be able to say "do not cast to me". *Recommend:* yes, it counts; no opt-out until asked.
7. **The control channel** (§3): A or B.
8. **Should the receiver show who cast**, beyond the fading line, for a household whose TV is
   signed in as one person and used by several? *Recommend:* no; the line is enough while only one
   person's devices can cast to it.

## 10. Risks and what was not tried

- **Starting without a press** (§4) on Silk and TV Bro is untested; a receiver that needs one press
  each time weakens the route but does not break it.
- **Silk's two navigation modes**: the prompt must work in both. In pointer mode only play/pause
  reaches a page as a key, and Back as a history step; whether OK's click reaches the receiver is
  unverified.
- **A TV browser killed on an input switch** drops off the list, and a cast to it ends; reopened, it
  comes back as a device but not receiving unless the core still holds the cast (§4).
- **A held socket on every page** costs a ping every 15 s per open page; a phone's hidden tab is
  throttled or frozen and drops off the list, which is right for "connected now".
- **The TCL's late sound** (idiosyncrasies, TV Bro) is the receiver's problem as it is any player's
  there; casting does not change it.
- Nothing here was run: no two-device session, no remote, no receiver.

## 11. What was built, and the assumptions it made

Built 2026-10-25, steps 1–4, 6 and 7 for casting alone: presence (`internal/core/live`, the presence
socket in `internal/core/api/live_socket.go`, the SDK's `live-connection.ts`, the web's `live.tsx`),
casts as sessions of one in the hub (`internal/core/together/cast.go`, `POST /me/casts`,
`GET /me/casts`, `POST /me/casts/{id}/end`), Cast to another device on the Play page
and a title's page (§9 Q1), the receiver (`cast-receiver.tsx`), the remote (`cast-remote.tsx`,
`/casting`), and a film played on a device that casts sent on to its receiver (Q3,
`PlayOrSendOn`). Device checks: VQ64–VQ66. Not built: step 5 (the person's choices sent from the
remote), step 8 (Watch Together), and §8's Playwright runs but one: `web/e2e/cast.spec.ts`, two
browsers signed in as one person, casts, pauses and plays from the remote, and takes control back.

Where this plan was silent, the build assumed, each the plan's lean or the smallest step:

1. **The presence socket is the session socket's address with a hello of its own** (`live`), so it
   travels wherever that socket does, directly, over WebRTC and through a discovery's loader, with
   nothing new in the transports (§3 left the address open).
2. **The remote is the cast's voice device**: a device with no player (D120's voice role), which the
   group never waits for, rather than a role of its own; in Watch Together it is the one that talks.
3. **A receive goes to the receiver's newest connection**: of a device's several tabs, one receives;
   another is handed the cast only once that one is gone (§4's reload).
4. **A sender controls one receiver**: casting to another ends its earlier cast, whose receiver is
   told it stopped (D337 §6 says one controller per receiver; the reverse was open).
5. **Stop casting pauses the receiver** and leaves it an ordinary player, rather than sending it home.
6. **The receiver plays at once** even when the sender was paused.
7. **At the film's end the receiver ends the cast and goes home** (§9 Q4's second half): the player
   plays no next episode by itself yet, so "as the sender would" is nothing; once it does, the
   receiver should follow it.
8. **Play here from ‹position›** opens the film's Play page, which starts where the server kept the
   person's position: the receiver's last report, at most a report interval behind what it says.
9. **A replaced remote is told another device controls the receiver**, without naming it.
10. **The remote shows the title and its length, no poster**, from the file the cast plays.
11. **Casting is not offered while the tab hosts Watch Together**, until step 8.
12. **Presses on the receiver:** with no prompt up, play/pause, rewind, fast forward and Left and
    Right are the player's own (§6.4), and any other key, a tap or Back raises the prompt; with it
    up, OK, a tap or Back takes control, and an arrow does nothing.
