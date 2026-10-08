> **ABOUTME:** Watch Together as designed and built: the host's session, who may join and do
> what, how the players keep in step, voice, and what each person sees. The feature five
> commercial services built and then withdrew, so it stays small in every client.

# Watch Together

- **Status:** two people, remote, in browsers, built in M4 ([m4.md](m4.md) Phases E, F and H).
  The design is [D246](../../decisions/working-notes.md#d246--what-a-person-may-do-is-their-roles-an-invite-names-who-it-is-for-and-a-watch-together-viewer-is-an-ephemeral-account-that-sees-only-the-hosts-playback)
  (who may do what) and
  [D250](../../decisions/working-notes.md#d250--a-watch-together-group-stops-only-at-its-own-sync-points-a-members-trouble-is-theirs-to-catch-up-on-from-what-is-already-being-made)
  (when the group stops), on the earlier decisions they revise.
- **Depends on:** [stream-service.md](stream-service.md) and [chunks.md](chunks.md) (what each
  player plays), [web-client.md](web-client.md) (the SDK), [remote-access.md](remote-access.md)
  (the public name and the relay's ports). The wire messages are
  [cut-a.md §5.1](cut-a.md#51-the-watch-together-messages).
- **History:** rewritten on 2026-10-07, per
  [D261](../../decisions/working-notes.md#d261--the-m4-audits-questions-answered) §6, to say the
  design as D246 and D250 made it. The earlier version, with abilities, link rotation, a member
  cap and the group pausing for a stall, is in git history. The research behind the feature is
  [research/watch-together.md](../research/watch-together.md).

## What it is

A host plays a film from their own library, and the people they invited watch it with them, in
step, from elsewhere, and can talk. The motivating case is two people in different places
([D69](../../decisions/working-notes.md#d69--group-size-is-a-priority-order-not-a-capability-two-then-four-or-five-then-a-party):
two first, then four or five, then a party on one network; a party across the internet is not
planned). Cut A is two households on web browsers; the remote television comes right after it
([D245](../../decisions/working-notes.md#d245--cut-a-ends-with-a-polished-skinned-web-ui-built-on-research-and-the-remote-tv-comes-right-after-it)).

**The session is the host's** (D246 §5,
[D122](../../decisions/working-notes.md#d122--the-session-is-the-object-what-plays-is-mutable-state-on-it-and-a-guest-sees-the-stream-rather-than-the-library)).
The host starts and ends it. While it is on, the host browses as usual and the viewers see none of
it; whatever the host plays plays on every viewer's screen, and when the host stops it, or leaves
the player, every screen goes black. A film ending, another film, the next episode: all are a
change of what plays, never a new session.

**It stays small in every client.** Plex and others withdrew the feature because it lived in every
client and did not survive a rewrite
([D150](../../decisions/working-notes.md#d150--read-the-withdrawals-not-just-the-requests-five-features-the-field-built-and-took-away)).
So everything but driving the player is in the shared SDK and the server: the per-client surface
is a player's position, buffering, play, pause, seek and rate
([D62](../../decisions/working-notes.md#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)).

## Who may join, and who may do what

- **Each person is invited by a link of their own** (D246 §3). The host writes a note saying who
  the link is for, which only the host sees (`hostNote`, D261 §4). The secret travels after `#`
  and leaves the address bar once read
  ([D243](../../decisions/working-notes.md#d243--m4s-nine-questions-answered-a-guests-cookie-covers-one-film-invites-are-registration-links-under-roles-that-choose-libraries-the-names-certificate-comes-by-dns-01-through-cloudflare-and-a-vps-carries-every-route-in)
  §3). A link admits one person once, and is spent by joining, not by opening it, so a reload before
  joining still works
  ([D252](../../decisions/working-notes.md#d252--m4s-acceptance-shapes-each-clients-network-with-the-kernels-netem-in-a-run-of-its-own-at-the-milestones-end)
  §5). The host may withdraw a link: it admits no one from then on, and a person who already
  joined by it stays.
- **A membership follows its person** ([D267](../../decisions/working-notes.md#d267--a-watch-together-membership-is-kept-by-the-browser-that-joined-or-by-the-account-of-whoever-is-signed-in-and-one-member-may-be-on-several-devices)). A guest's is kept by the browser that
  joined, with the link it came by, so closing the page loses nothing and the same link opened
  again in that browser comes back as the same member; no other browser can use it. Someone
  signed in, the host or a guest who joined signed in, has their session back on any device they
  sign in on (`GET /together/mine`), each device with a credential of its own, so one device
  having it back cuts off no other. Someone not signed in moves to another device by a **New
  link** from the host, which removes them, cutting off every device they came in on, and admits
  whoever opens it under the same note. The host plays in one place at a time; whichever device
  they play on leads. The host can **Show link** again for any link, waiting or used, after a
  reload or for a guest to come back by on the browser that joined (§5).
- **A viewer is an ephemeral account** (D246 §3): a hidden, built-in role that cannot be edited,
  restricted to the playback screen. No browsing, no library, no control of playback. The viewer
  picks the name the others see; someone signed in finds their own name filled in. The membership
  lasts as long as the session, or until they leave or are removed.
- **A viewer plays only what is on, while they are in.** Their player plays by their membership's
  credential, and the cookie it gets reaches only the film the session plays now: it moves when the
  film changes, and removal or the session's end revokes it at the next request (D243 §1). The
  host can put on only a film their own role reaches (D243 §4).
- **Only the host controls playback** in Cut A (D246 §8): play, pause, seek, what plays, stop.
  Only the host removes a member, and never themselves, or ends the session. A removed member's
  credential admits nothing.
- **There is no cap and no link rotation.** One single-use link per person, each withdrawable, does
  the jobs a cap and rotation did for a shared link
  ([D119](../../decisions/working-notes.md#d119--a-join-link-admits-membership-is-what-persists-rotation-is-seamless-ejection-is-owner-only-and-a-cap-bounds-the-leak),
  as D246 revised it).
- **Signing in records nothing yet.** A device that says hello with a signed-in person's session
  token makes the membership theirs (D243 §2), but recording their viewing as their own is
  deferred (D246 §4, D261 §3). An anonymous viewer accrues nothing.
- **A session lives in the server's memory.** A host has one; reopening the host's page gives it
  back. A restart ends every session.

## Sync

**The group** has a state, `Idle`, `Waiting`, `Paused` or `Playing`, a position, a rate and what
plays. The server holds it, applies the host's commands in arrival order, and tells every device
each change with a sequence number. A seek made against a group another seek has since changed is
refused as stale. Everything rides the session WebSocket, one per device; never a WebRTC data
channel.

**The group stops only at its own sync points** (D250 §2): a film put on or started over, the
host's play after a pause, and the host's seek. There it is `Waiting` until every player that has
once been ready is ready again, those catching up included. A player still loading its first
picture, or whose person is still choosing their speakers, holds no one up (D246 §6). A player is
ready after a seek or a stall only with 10 s loaded ahead (`refillAheadMs`,
[D247](../../decisions/working-notes.md#d247--a-slow-link-gets-a-lower-version-made-at-its-first-stall-and-played-from-its-second-and-a-group-waits-for-a-stalled-viewer-once-then-lets-them-catch-up-alone)
§5). A sync point waits at most `together_sync_wait_seconds` (20 s), then plays without whoever is
not ready, but never to no one (D250 §3). The host's pause is immediate.

**The group never stops for one member** (D250 §2). A stall, a slow link or a dropped connection
is that member's alone, the host's player included: the group plays on, everyone is told who it
continues without, and that player catches up by itself. **Catching up** is a jump ahead of the
group to wait there, paused and loading: far enough that, at the speed it measured itself
loading, it has `together_catch_up_buffer_percent` (80%) of what it loads ahead filled when the
group arrives, and at most a minute ahead (D250 §4). Catching up never starts an encode; the
player fills from the run already making its quality (D250 §5).

**Drift is corrected gently** (D250 §1). A following player works out where the group is now from
where it was last placed, the time since, and the transit of what placed it
([D66](../../decisions/working-notes.md#d66--no-clock-synchronisation-monotonic-local-time-and-bounded-round-trips):
no shared clock, each device's own monotonic time). Each pong says where the group is as the
server answers, and the pong that is off by least now places it: half its round trip, and, as a
placing ages, 50 parts per million of its age, so the two clocks' skew never builds up over a film;
until one does, the group message that announced it, half the shortest round trip measured
([D264](../../decisions/working-notes.md#d264--a-follower-places-its-group-by-the-pong-with-the-shortest-round-trip-which-carries-where-the-group-is),
D268 §1). A new timeline is pinged for each second three times, then every 15 s, and each pong is
noted with where it said the group is against where it was expected. Then, checking each second:

| Drift | Correction | Setting, default |
| --- | --- | --- |
| up to 150 ms | none | `together_drift_left_alone_ms`, 150 (D261 §1) |
| up to 3 s | play up to 10% faster or slower than the group for about a second, repeated | `together_nudge_most_percent`, 10 |
| beyond 3 s | seek to the group | `together_seek_drift_beyond_ms`, 3000 |

**A player corrects as its device measures it** (D268 §2). Each player learns what its rate going
back to the group's costs it, about 220 ms on an iPhone (WebKit 163433), and nudges on past the
group by that much. Once two returns have cost it more than the drift left alone, it stops nudging
and corrects by seeks alone: beyond `together_seek_only_drift_left_alone_ms` (400 ms), a small seek
past the group by what its seeks have learned to lose. A pair with such a player is held to 500 ms
apart 95% of the time; D243 §9's 200 ms holds for players that nudge. It is never chosen by the
browser's name.

A player's position is not read for drift for `together_settle_after_change_ms` (300 ms) after it
starts playing, seeks or changes rate, since Safari's stands still for a moment then. A nudge keeps
the sound's pitch (`should_together_nudge_keep_pitch`). Every value is a setting sent to each device
in its welcome, so it is tuned while testing without a new client. No sync tuning is shown to the
people watching.

**The host's own player leads**: its play, pause and seek become the group's commands. A drag of
the timeline is one seek, to where it ended, and a move the player makes itself (hls.js stepping
over a gap) is not sent.

**Also in the protocol, not used by the web client yet:** a group rate, accepted only when every
playing device can set one
([D90](../../decisions/working-notes.md#d90--playback-presentation-is-a-persisted-per-user-preference-set-and-captions-have-a-statutory-floor));
and suspensions with a reason (`stallRecovery`, `audioSessionInterrupted`, `userActionRequired`,
`userPausedLocally`, `userIsChangingCurrentTime`), during which a device is not ready.

## Unequal links: the group shares a position, not a stream

Each player plays the quality its own link carries, at the group's position, and nothing in the
sync mentions bitrate
([D123](../../decisions/working-notes.md#d123--bandwidth-is-per-participant-the-group-shares-a-position-not-a-stream)).
How a player chooses is the solo player's, the same in a group:

- **A film that can be cut plays from its ladder** of qualities from the start, at the top unless
  its device's link on that network is remembered slower, and hls.js steps down and back up as it
  plays ([D260](../../decisions/working-notes.md#d260--a-film-plays-from-its-ladder-of-qualities-from-the-start-filled-on-its-page-and-its-player-keeps-choosing-as-it-plays),
  [D257](../../decisions/working-notes.md#d257--a-player-starts-on-the-version-its-link-can-carry-chosen-before-play-from-the-speed-remembered-for-that-device-on-that-network)).
- **A film not cut yet** plays its original or a prepared copy; the first stall starts a lower
  version, offered in the stall's answer, and the player moves to it once it is ready if it still
  cannot keep up
  ([D253](../../decisions/working-notes.md#d253--a-stalled-player-is-told-of-its-lower-version-at-its-first-stall-and-moves-to-it-once-it-is-ready-if-it-still-cannot-keep-up)).
  A version another viewer's stall started is shared when it fits (D247 §7).
- **A run of the encoder must stay faster than playback.** When one more run would bring any below
  1.1×, a run is evicted, and whoever loses their chunks is told why (D250 §6).
- **At the bottom, the person is told it will not work**, with their link's speed and what the
  lowest quality needs, and the others see it in the roster (D247 §8).
- **Nothing detects home or away** (D247 §11).
- **The browser must run hls.js**: iOS and iPadOS 17.1 or later on Apple devices. A browser that
  cannot is told so before it joins or hosts (D250 §7).

## Voice

**Browser WebRTC through a TURN relay embedded in the server**, with signalling on the session
WebSocket ([D68](../../decisions/working-notes.md#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay),
[D193](../../decisions/working-notes.md#d193--the-audits-proposals-are-accepted-the-node-does-all-media-work-prepare-is-the-one-path-besides-direct-play-the-machinery-is-cut-and-cut-as-design-freezes)
§4). Standard ICE picks the path once: direct where it connects, relayed otherwise, with no
upgrade ladder. The relay (D243 §7–§8) listens on 3478, UDP and TCP, relays through ports
49160–49199 at the VPS's public address, and relays only to the public internet, never into the
house. Each member gets credentials of their own (`GET /together/{id}/ice`) that end with the
membership or after a day; a server that relays nothing hands out none, and calls go direct only.

**Talk** opens the microphone and attaches a second device of the membership in the `voice` role,
which keeps one call with every other person's voice device. Then **Mute**, **Leave voice**, and
the others' volume, from silent to four times as loud as they came, apart from the film's volume.
A microphone that will not open, or a call that cannot connect or drops, turns voice off with the
reason, the browser's own words beside a refused microphone; Talk tries again.

**Echo.** The film plays out loud while the microphone is open, and a browser cannot cancel audio
it has no reference for. **Headphones on at least one side are the supported setup**
([D63](../../decisions/working-notes.md#d63--voice-chat-is-webrtc-and-headphones-are-the-supported-configuration)),
and the voice controls say so. Content-referenced echo cancellation was deleted (D193 §4). The
browser's echo cancellation, gain control and noise suppression on the microphone are settings,
off by default (`should_together_voice_cancel_echo`, `should_together_voice_level_microphone`):
on Android they made a call of it, turning the film down and taking the volume buttons.

**On a congested link voice is best effort** (D261 §7): D243 §9's voice bounds hold for the home
and mobile profiles, not the congested one.

## What each person sees

From [D188](../../decisions/working-notes.md#d188--the-interface-is-designed-against-the-fields-recorded-failures-accessibility-shapes-the-design-but-does-not-gate-cut-a-her-television-plays-and-her-phone-talks)
§1, as built:

- **The host's Watch Together screen**: start a session, or have back the one they host; make a
  link with its note, shown as text and as a QR code; each link, waiting or used, and each member,
  with the host's notes; withdraw a link, remove a member, end the session.
- **The viewer's page**, from the link: a name and one **Join**, which is also the gesture the
  browser's autoplay needs. Then the speaker question, unless this browser already knows the
  answer, which no one waits for (D246 §6). Then only the playback, black while nothing plays, with
  their own film volume, voice, and **Leave**.
- **For everyone**: each change as a named event in the reader's own words, *"Karl paused at
  41:12"*, a seek saying where it jumped from; a drop shown as *reconnecting*, not as an ending;
  who is here, by the names they chose, never the host's notes; and anyone whose link cannot carry
  even the lowest quality, with both numbers.

## Later, by decision

- **Rights per invite link**: play and pause, seek, next and previous, each given per link, the
  last link's rights the next one's defaults (D246 §8).
- **A signed-in recipient's viewing recorded as their own**, details filled in; avatars; inviting
  registered members in the app, without a link (D246 §4, D261 §3).
- **The remote television** (D245): our cast receiver, the television's own browser, or a
  prototype Android TV app, with her phone carrying voice (D188 §3).
- **Subtitles, the audio track and levelling for viewers**, chosen per person (D188); the viewer's
  player shows none yet ([m4.md](m4.md) Phase E).
- **Undo for a seek** and **text chat** (D188).

## Prepared for, not built

The owner, 2026-10-07: native apps and other ways of delivering the film are coming, *"we could
have voice going to a mobile app or to a mobile web browser, and the video itself going to a TV in
the same house as the guest"*; *"we prepare for this eventuality, but we don't need to build it
yet"*. What is designed from now on keeps these open:

- **A person's devices are separate, by role** ([D120](../../decisions/working-notes.md#d120--a-participant-is-a-person-devices-attach-to-them-with-roles-and-the-cap-counts-people)):
  voice on a phone's browser or app, the film on a television, as the protocol's playback, voice
  and both roles already allow. Nothing new may assume that voice and the film share a tab, a
  browser or a device. The missing piece is a second device joining as the same person (Open,
  below).
- **The film gives way to voice per person, not per tab.** D262 §2's pacing is asked by the
  player, from its own tab's voice; with voice on a phone and the film on the television in the
  same house, they share the household's link but not a tab. The core knows both are one person's,
  so it will pace that person's playback devices while any of their voice devices is live.
- **Bounds and behaviour are per client.** Browsers never reach parity (the owner, the same day:
  *"I'm okay with browser-specific policies"*), and a native app can do what no browser can: cap its
  jitter buffer, as Signal's 500 ms, use Opus's deep redundancy (DRED), or carry media its own way.
  Each capability is detected, never read from the User-Agent, and each acceptance bound says the
  client pair it holds for.

## Open

Undecided, or where the code and a decision disagree. Each needs the owner.

- **Voice on a second device.** D245 §1 says Cut A's voice is still tested as the companion: her
  phone's browser carries voice while her computer's browser plays
  ([D120](../../decisions/working-notes.md#d120--a-participant-is-a-person-devices-attach-to-them-with-roles-and-the-cap-counts-people)).
  The code attaches the voice device from the same browser tab. A link admits one person once and
  the credential stays in the tab, so a second physical device has no way to join the same
  membership; opened on her phone, a second link makes a second member.
- **Voice on bad links.** How Signal and WhatsApp keep voice working where ours failed was
  researched (D261 §7) in [voice-bad-links.md](../research/voice-bad-links.md), and D262 acted on
  it: it finds the delay comes
  mostly from the jitter buffer covering delay variation, made by the film's download sharing the
  bottleneck queue with voice and worsened by how the netem profile is applied to the loopback.
- **Offset alignment** ([D65](../../decisions/working-notes.md#d65--playback-offset-is-per-participant-and-it-is-an-echo-control)):
  the speaker-side player running ahead by the voice latency, so a leak arrives in step. Not built;
  the group has no per-member offset. Whether it is still wanted.
- **The per-client surface.** The SDK's follower drives a slice of the browser's media element,
  not a six-method interface, and there is no second client to check D62's budget against.
- **Suspensions and the group rate** are in the protocol and unused by the web client: whether
  the host's scrubbing, now a debounce, should be `userIsChangingCurrentTime`, and whether a group
  rate is wanted.
- **The others' volume is one level for all of them**, which is per-source gain at two. The
  owner, 2026-10-07: on the play page everyone, not only the host, gets a volume for each other
  person and one for the film; the device's own buttons stay the master volume. Not yet decided
  or built: it waits for the UI work, which records it as a decision.
- **Above two.** D250 §2 answers what happens when one member struggles; whether anything else
  changes at four or five (D69) is not designed.

## Done when

**Cut A, two people** (cut-a's M4 row, D243 §9, D252, D261):

- Two browsers on different networks stay within 200 ms of each other 95% of the time for 30
  minutes under each netem profile: *home* (20 ms), *mobile* (80 ± 30 ms, 1% loss) and
  *congested* (150 ± 50 ms, 3% loss, 5 Mbit/s). The *home* run is in every branch check (D261 §2);
  the full run is `make test-acceptance`, at a milestone's end.
- Voice arrives one-way under 300 ms with under 2% audible loss under *home* and *mobile*;
  *congested* is best effort (D261 §7).
- A link works once and only once, after a reload too; no credential appears in any address of a
  HAR capture; a viewer plays only the film on; an anonymous viewer's history does not advance.
- A stall or a drop on one side never stops the other; the group waits only at its sync points,
  and at most the sync wait.
- The host changes the film mid-session and the session carries on; a film ending leaves it
  alive.
- Voice runs on her phone while her computer plays (open above).

**After Cut A**: her television plays while her phone talks (D245), and a second client gains
the feature by driving its player, not by reimplementing the sync.
