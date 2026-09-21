> **ABOUTME:** Synchronised remote playback with shared controls and voice chat — the feature
> five commercial services built and then deleted, and the design constraint that follows from
> why theirs died.

# Watch Together

- **Status:** PLANNED — researched 2026-09-19 ([watch-together.md](../research/watch-together.md), [D62](../../decisions/working-notes.md)–[D69](../../decisions/working-notes.md)); **membership, abilities and the cast path settled 2026-09-20/21 by the project owner** ([D117](../../decisions/working-notes.md)–[D124](../../decisions/working-notes.md)).
- **Depends on:** [stream-service.md](stream-service.md), [web-client.md](web-client.md) (the SDK, and the cast receiver — [D124](../../decisions/working-notes.md)), [remote-access.md](remote-access.md) (for the off-LAN case). **Not** [sharing-model.md](sharing-model.md): [D117](../../decisions/working-notes.md) made the session link the grant, so this is the one access path that is deliberately not predicate-governed.

## Scope, in priority order

The host creates a session and puts something on. Guests join by link. **Volume is local.**
**Voice chat** with per-source gain, so one loud participant does not drown the film. Text chat
deferred; emoji reactions are probably the right shape for a television.

**The session is the object, and what plays is mutable state on it**
([D122](../../decisions/working-notes.md)). The host may change the video to anything in his
library at any time, mid-video included, and **a video ending does not end the session** — the
group returns to `Idle` and waits, which is what that state was always for.

**What a guest gets is the current playback. Not a library, not a browse view, not an item
they can address by id.** That is the bound on a leaked link, and it is stronger than scoping
the session to one work because it does not depend on enumerating the scope.

The motivating case is two people in different places, one of them travelling — a *remote*
feature first and a LAN feature incidentally.

**Group size is a priority order, not a capability** ([D69](../../decisions/working-notes.md)):

| Priority | Size | Notes |
| --- | --- | --- |
| 1 | **Two** | Where the design is *best*, not merely simplest — see below |
| 2 | **Four or five** | The honest ceiling for a household uplink |
| 3 | **LAN party** | Mostly a UI problem: roster, joining, control defaults |
| — | **Remote party** | **Not on this list.** Gated on adaptive bitrate in [transcoding.md](transcoding.md) |

Two is the best case rather than the degenerate one:
[D65](../../decisions/working-notes.md)'s echo alignment is *fully* solvable at two — one person
on headphones, one on speakers, offset `d = L`, echo gone — and only approximable above it,
because with three or more the pairwise-optimal offsets are over-determined. "Does one stall
pause everyone?" has an obvious answer at two and none at five. And the server relay is free at
two, merely cheap at five.

### Unequal links: the group shares a position, not a stream

[D123](../../decisions/working-notes.md). One guest on hotel wifi and another on fibre is the
normal case, and it is already answered by decisions taken for other reasons:
[D2](../../decisions/working-notes.md) puts rendition choice on the client and
[D86](../../decisions/working-notes.md) makes the client re-pick on its own buffer. So **each
participant streams at whatever rendition their own link supports**, at the same timestamp,
unaware of anyone else's choice. Nothing in the six methods mentions bitrate.

**No lowest-common-denominator.** Degrading everyone to the worst link punishes four people
for one person's hotel, and it is the option that feels fair and is not.

**And N guests do not mean N transcodes.** The output menu is a closed enumeration of about
eleven named profiles ([D21](../../decisions/working-notes.md)) and the transcoder shares one
encoder process between viewers of the same rendition
([transcoding.md](transcoding.md)) — cost is bounded by the menu, not by the roster. Five
guests collapse onto two or three renditions, usually one.

**What *is* shared is stalls**, and that is what the group stall policy below is for.
"Continue without them" lets a struggling guest fall behind and resynchronise rather than
holding four people at a spinner; it is kinder than ejection and it is the first resort, not
the last.

**Per-guest health is in the roster**, because the stall policy is unusable without it — and
it is nearly free, since [D73](../../decisions/working-notes.md) already routes every client's
observations to the core and [D91](../../decisions/working-notes.md) already collects them.
Buffer health and stall count per participant does three jobs: it tells the host *why*
playback keeps pausing ([principles §3](../../principles/README.md) applied to a group), it
makes the stall policy a comprehensible choice rather than a mystery toggle, and it lets the
struggling guest see their own meter and bow out before anyone has to ask. Ejection stays
host-only ([D119](../../decisions/working-notes.md)); what the meter adds is that the decision
is informed.

**Until the transcoder exists** ([transcoding.md](transcoding.md) §(b), which
[D86](../../decisions/working-notes.md) already put on remote access's critical path) a guest
whose link cannot carry the original is out of luck — and the honest thing is to say so on the
join screen rather than let them find out at minute four.

### Where the ceiling actually is

Not the voice. Opus with RTP/SRTP/UDP/IP overhead is ~64 kbps per stream, and because
per-source gain means the server **forwards rather than mixes**, it is `N(N−1)` outbound:

| N | 2 | 5 | 10 | 20 |
| --- | --- | --- | --- | --- |
| Voice out | 0.13 Mbps | 1.3 Mbps | 5.8 Mbps | 24 Mbps |

**The film is the constraint long before that.** A 1080p web-dl direct-plays at 5–10 Mbps and a
remux at 20–30, against a household upload of 10–40 Mbps on fibre and much less elsewhere. One
remote viewer on a remux can saturate an uplink. So "party mode" splits in two: a **LAN party**
costs no uplink at all and is nearly free, while a **remote party** is bounded by physics and
belongs to the transcoding plan, not this one.

### Openings to leave, none of which is work today

1. **Membership is a list with per-member state**, not a pair — and per
   [D119](../../decisions/working-notes.md) each member's state is a **first-class object with
   its own identity**, issued at join. That is what makes ejection and seamless link rotation
   possible; without it a participant is only identifiable by a link several people hold.
2. ~~**Control is a per-participant capability**, defaulting to everyone~~ — **taken**
   ([D121](../../decisions/working-notes.md)): it is now a creator-set policy, chosen at
   session creation, over a **closed table of abilities** rather than one flag — see the
   abilities table below.
3. **The client handles a *set* of remote audio streams** with per-source gain, even when the
   set has one member. Free, since per-source gain was already required — and it is what makes
   the voice companion below cost nothing extra.
4. **Transport is a server-chosen policy** — mesh, relay, or eventually an SFU — not a client
   assumption.
5. **The stall policy is a group setting** with a size-dependent default — and per
   [D123](../../decisions/working-notes.md) it is the real control for unequal links, with the
   per-guest health meter as the surface that makes it choosable.

Not building: an SFU, active-speaker detection, moderation tooling, roles beyond (2), text chat.

If a participant's networks are routinely hostile — hotels, conference wifi, CGNAT — the answer
is not more traversal cleverness but the **`relay` role** on a cheap VPS that the core dials out
to ([D71](../../decisions/working-notes.md)). It fixes reachability, voice and the uplink in one
deployment, and it is useful at two participants, not just at party scale.

If remote party viewing ever happens, the shape is **fan-out** — the household uploads one
stream to a distribution node and N participants pull from it, making the home uplink cost
independent of N ([D70](../../decisions/working-notes.md)). It is speculative and much later,
and the minimum viable version is an HTTP caching proxy, because synchronised viewing is the
ideal caching workload: everyone wants the same segment at the same moment. The **one thing
that must not be closed by accident** is segment-URL cacheability — see
[stream-service.md](stream-service.md).

## Who may join, and who may do what

[D117](../../decisions/working-notes.md). **The session link is the grant.** Holding a valid
one admits you — no account, no share predicate evaluated per person, which is the point: the
motivating case is a partner travelling who does not have an account.

**A link admits; a membership persists** ([D119](../../decisions/working-notes.md)). That
distinction is what makes the rest cheap: a join link is a credential for *admission only*,
and once someone is in, their membership is an object of its own with its own identity.

**"Persists" means for the session and not one second longer**
([D125](../../decisions/working-notes.md)). A guest is **not a principal**: no user row, no
password, no recovery, no predicate, and **no play state, history or statistics** — during a
session it is *the host's* position that advances, because it is the host's library and the
host's choice of what plays ([D122](../../decisions/working-notes.md)). This is worth stating
because the opposite is the natural implementation — a membership looks enough like a user to
get a row — and **a guest who acquires a durable identity is a griefer who survived
rotation**.

So recovering from a leak never disturbs anyone:

1. **The owner rotates the join link.** The old one **stops admitting new clients and keeps
   serving every client already admitted** — no interruption, no re-join, no re-formed
   session. Position, roster and voice are untouched.
2. **The owner ejects the intruder**, which is possible at all only because memberships have
   identity. **Ejection is owner-only and never delegable** — a participant with transport
   control still cannot remove anyone, or a griefer with control rights could eject the host.
3. **Ejection and rotation are paired by a prompt, not by coupling**, because booting someone
   who still holds a working link is a revolving door — but rotating when you did not need to
   has its own victim. Removing someone raises:

   > *If you're booting this person due to abuse, would you also like to rotate the join link
   > so they can't come back?*

   It asks about the **reason**, not the mechanism: the owner is mid-incident and should not
   need to know what a join link is. It is **not a default**, because the cost of a needless
   rotation falls on someone invisible — a legitimate invitee holding the old link who has not
   joined yet; everyone already in is undisturbed either way. And it **never gets a "don't ask
   again"**: a housemate going to bed and a stranger who found the link are the same button
   with opposite answers, so remembering the last choice would be a bug. A batch ejection asks
   once.
4. **The owner sees the roster live**, or cannot know to do any of this.

**And the cap is the control that works before the owner notices.** At session creation the
owner sets a **maximum number of simultaneous participants, with a small safe default** —
**participants, meaning people, not connected clients** ([D120](../../decisions/working-notes.md)
corrects this decision's original wording; two people with voice companions are four clients
and must not trip a cap of two). Small, because [D69](../../decisions/working-notes.md) says
the design is best at two and the motivating case is two people; visible at creation rather
than buried, and raisable in one action at any time. It bounds the flood from a leaked link to
one griefer rather than thirty, and it is simultaneously a **cost** control — though the thing
the uplink and the relay actually pay for is *streams*, which is why the cap is the bound and
the per-guest health meter is the feedback.

**A refused join is a signal.** "At capacity" never fires in a correctly-sized session, so
several in a row is the earliest available evidence that a link has leaked — arriving before
a stranger shows up in the roster. Surface it to the owner live, with the rotate action next
to it ([principles §10](../../principles/README.md)).

**And what the link reaches is the session, not a library**
([D122](../../decisions/working-notes.md)). That is what keeps the blast radius small enough
for revocation to be a sensible control, and it is a **stronger** bound than scoping the
session to one work — which is what this plan used to say, and which stopped being true when
the host gained the right to change what plays mid-session. A link holder gets the current
playback: no browse view, no item addressable by id, nothing enumerable. The bound does not
depend on knowing the scope in advance.

### Abilities: a closed table, each one `group` or `host only`

[D121](../../decisions/working-notes.md). The host picks at session creation. That is all
there is to it.

| Ability | Grant | Default |
| --- | --- | --- |
| Transport — play, pause, seek | `group` \| `host only` | `group` |
| Choose what plays | **`host only`, not a setting** | — |
| Eject, rotate the link, change the cap | **`host only`, never configurable** | — |
| *Change playback within a play set* | `group` \| `host only` | *later* |

Two rows are deliberately not settings. **What plays** is the host's library and the host's
alone. **Ejection** is never delegable, or a participant granted transport could eject the
host.

**Keeping this a table rather than two booleans is the whole of "leave the door open".** Finer
grain almost certainly is not needed; a closed enumeration costs nothing now and makes a later
ability a row rather than a redesign.

**Play sets — and they arrive earlier than this plan assumed.** The host selects a set of
videos — a season, a trilogy — and grants the group the ability to move within it.
Binge-watching without handing over the library.

This was deferred here and is **needed now by solo casting**
([D135](../../decisions/working-notes.md)), where a cast queue is simply
[play-state.md](play-state.md)'s **Next Up** extended past one item over
[D96](../../decisions/working-notes.md)'s ordering-in-use. So the concept ships for casting and
this plan inherits it. That is the **second** time a Watch Together "later" turned out to be a
casting "now" — the first was [D124](../../decisions/working-notes.md)'s receiver — which is
worth noticing as a pattern rather than a coincidence: casting is the same surface with the
hard parts removed, so it reaches things first.

## The constraint that decides whether this survives

Five implementations have been withdrawn: Disney+ GroupWatch (2023-09-18), Amazon Prime Video
Watch Party (2024-04-02, unannounced), Sling (early 2024), and **Plex Watch Together
(2025-02-25, all platforms but web)**. Netflix never shipped one. Plex's note gives the cause:
the feature *"won't be available for most platforms as they get the new experience."*

**It died because it lived in every client.**

So the design requirement, and it should be treated as a hard one:

> **The client-specific surface is six methods.**
>
> ```
> currentPosition() → time      seek(time)      play()
> isBuffering() → bool          setRate(r)      pause()
> ```
>
> `setRate` is optional. Everything else — clock offset, drift policy, group state,
> suspensions, command ordering, reconnection — is in the shared SDK.

If that holds, a client rewrite cannot casually drop the feature, because there is nearly
nothing in the client to drop. If a seventh method starts to look necessary, that is the signal
to stop and rethink.

Demand is not the risk: **Teleparty has 10,000,000 users** and exists only because the platforms
will not do this.

## Sync

Taken from Jellyfin's SyncPlay, which works ([research §1](../research/watch-together.md)):

- **No clock synchronisation** ([D66](../../decisions/working-notes.md)). Each client keeps a
  **monotonic** local time and reports "my position was `P` when my clock read `M`"; the server
  bounds the uncertainty with a round trip and keeps the **minimum-RTT sample**. No shared time
  base, no dependence on device wall clocks — which would be worse, since an NTP daemon
  stepping a phone's clock mid-film produces a spurious jump of arbitrary size.
- **Reconciliation is two mechanisms.** Periodic, in **minutes** — 50 ppm of clock drift is
  30 ms over ten minutes, so second-by-second polling is ~40× more than drift requires — and
  **event-driven** on buffering, seek completion and resume, which is when position actually
  jumps by 100 ms to 2 s. The event-driven half does the real work and is free, because those
  events are already in the six-method contract.
- **A user-chosen playback rate and the drift nudge are the same knob**, so they compose rather
  than compete ([D90](../../decisions/working-notes.md)): the effective rate is the **group
  rate** times the nudge. A user's 1.25× is therefore group state, not a local setting — a
  participant playing faster than everyone else is not drifting, and a reconciler that treats
  it as drift spends the film fighting them. Two corollaries: `setRate` is listed as optional
  above, so **a group's rate stays 1.0 unless every participant can honour it**; and a client
  that cannot set a rate can still join a 1.0× group, which is all of them.
- **Two corrections**: nudge `playbackRate` for ~1 s to absorb roughly 60–3000 ms of drift; seek
  beyond that. Rate nudging is **inconsistent across platforms** — Jellyfin's own source records
  it failing on Safari and struggling on Android Chrome — so it is optional and seek-only is the
  always-available fallback. **Measure it in the [device lab](device-lab.md) before relying on
  it.**
- **Group state**: `Idle`, `Waiting`, `Paused`, `Playing`, where `Waiting` is
  paused-until-everyone-is-ready.
- **Suspensions with reasons**, Apple's model: a participant detaches temporarily and says why.
  `stallRecovery`, `audioSessionInterrupted`, `userActionRequired`, **`userPausedLocally`**
  ([D136](../../decisions/working-notes.md) — a television paused by its own remote over
  HDMI-CEC, which the receiver is the only thing that knows about and must report, or the group
  state says `Playing` over a paused picture), and above all
  **`userIsChangingCurrentTime`** — while you drag the scrub bar you are suspended, so your
  intermediate positions do not drag everyone else across the timeline. Shared seek is unusable
  without it.
- **Position is per-participant: `P + offset_i`** ([D65](../../decisions/working-notes.md)),
  default zero. A trivial generalisation on day one and an awkward retrofit later, since it
  touches the group record, the drift target and every in-sync comparison.
- **The server serialises.** Concurrent conflicting commands are normal, not exceptional. Each
  command carries the group-state version it was issued against, the server assigns a monotonic
  sequence, and clients reconcile to the server rather than to each other.
- **Sync never rides a WebRTC data channel** — unsupported on tvOS, and it would make
  synchronisation depend on peer connectivity.

## Voice

WebRTC, peer-to-peer where NAT allows, TURN relay otherwise.

The film plays out loud while the microphone is open, and the platform's AEC can only cancel
audio it has a reference for — which excludes anything played by libmpv, ExoPlayer or AVPlayer,
and excludes Web Audio output in a browser. So without help, your microphone sends the film
back to the other person, delayed and doubled against their own copy.

**Three tiers** ([D63](../../decisions/working-notes.md), [D65](../../decisions/working-notes.md)):

| Tier | Mechanism | Cost | Ships |
| --- | --- | --- | --- |
| 1 | **Headphones** — no acoustic path | none | always |
| 2 | **Offset alignment** — the leaking participant runs *ahead* by the one-way voice latency, so their leaked audio arrives aligned with the listener's copy | a field in the group state | with tier 1 |
| 3 | **Content-referenced AEC** — feed the decoded film audio to `speexdsp` or `webrtc-audio-processing` as the echo reference | real work | later |

Tier 2 is the field-discovered one and is worth understanding: at zero offset — what every
other implementation targets — **both** participants suffer the full one-way latency of
misalignment, which at 150–300 ms is a distinct slapback. Below ~30 ms the precedence effect
fuses the copies into mild coloration. When only one side leaks it is *fully* solvable; when
both do it is a transfer, so tier 3 is the real answer there.

Tier 3 is available to us and not to the OS: **we are the player**, so we know the film's audio
sample for sample, which is a better echo reference than the operating system's view of the
mixed output.

**Ducking conflicts with the platform AEC**, so it is conditional:

| Output | Film audio path | Ducking | Platform AEC |
| --- | --- | --- | --- |
| Headphones | through our mixer | automatic | irrelevant |
| Speakers | native playback path | manual levels only | keeps its reference |

Per-source gain is independent of both — one gain node per remote stream — and is the explicitly
requested control.

### Voice companion: a participant is a person, and devices attach to them

**On a television the phone is the voice endpoint.** An Apple TV has no app-accessible
microphone, and a microphone across the room from the speakers is the worst possible AEC
geometry regardless. Phone-and-earbuds for voice, TV for video.

[D120](../../decisions/working-notes.md) makes that a model rather than an observation. **A
participant is a person; a person attaches one or more devices, each with a role:**

| Role | Does | Implements |
| --- | --- | --- |
| `playback` | plays the video, reports position, accepts transport | the six methods |
| `voice` | microphone and per-source mix, nothing else | none of them |
| `control` | a remote: roster and transport, no playback | none of them |

**"Voice companion" is that, and nothing more** — a phone attached with the `voice` role while
the television holds `playback`. It needs no player, no rendition negotiation and no sync
participation. A voice-only client implements **none** of the six methods, which is a
confirmation that the surface is the right size rather than a strain on it.

**And the `playback` device need not be our television client**
([D124](../../decisions/working-notes.md)):

| | `playback` | `voice` | `control` |
| --- | --- | --- | --- |
| Our TV client + phone | Android TV app | phone | phone |
| **Cast** | **cast receiver (a web app)** | phone | phone |
| Phone alone | phone | phone | phone |

**For a guest this is the best path, and it is the strongest argument in the feature.** We
choose what our own users install; we do not choose what their friends own, and the motivating
case is somebody else's household. A cast receiver is *"cheap because it is a web app"*
([tech-clients.md](../research/tech-clients.md)) and it is mostly the Phase 2 web player in a
different shell — so a guest with any Cast-capable television joins without us shipping a
client for their platform.

**The receiver holds the membership, not the phone.** Cast's sender hands the receiver a URL
and commands; **the receiver fetches the media itself**. So the receiver speaks the six methods
directly and the phone holds `voice` and `control` — which means a pocketed, sleeping phone
drops *voice*, not playback. That is the correct failure, and it is why this arrangement beats
the phone holding the stream.

**The receiver probes and picks; the sender must not pick for it.** A cast receiver has a
narrower decoder set than the phone driving it, so a phone negotiating on its behalf is a
device-capability database with one row, arriving by the back door —
[D2](../../decisions/working-notes.md) exactly. The receiver is our web app; it probes itself,
and Cast's advertised capabilities are a hint rather than an answer
([D92](../../decisions/working-notes.md)). On Cast that probe is a documented API,
**`CastReceiverContext.canDisplayType()`** ([D137](../../decisions/working-notes.md)) — this is
the platform where D2 is easiest, not hardest.

**What it costs:** a sync hop — `currentPosition()` now carries Cast's status latency inside
it, against a 60–3000 ms budget ([D66](../../decisions/working-notes.md)) — which is
**unmeasured and decides whether this is a first-class path or a documented fallback**. Also
three protocols rather than one (Google Cast, AirPlay, ours; **Fire TV has no Cast**), a Cast
developer account and $5 for a custom receiver, and a session credential that now lives on a
third-party device — acceptable because the link was already the grant and rotation is already
the answer ([D117](../../decisions/working-notes.md),
[D119](../../decisions/working-notes.md)).

**The phone is not a fan-out point.** It has no bytes to fan out — the receiver pulls — and
making it one would mean a caching proxy holding the session credential on the least reliable
device in the chain. Several viewers in one guest household each join as participants up to
the cap, and fan-out happens at the relay, where
[D70](../../decisions/working-notes.md) and [D71](../../decisions/working-notes.md) already
put it.

**A second device attaches with [D93](../../decisions/working-notes.md)'s code flow**: the
first shows a short code, the second enters it and joins the *same membership* rather than
creating a new one. Same mechanism as signing in on a television, pointed at a session — which
is why "one link, several devices per person" needs no account at either end.

**And the cap counts people, not clients.** Two people with voice companions are four clients;
a cap phrased in clients would refuse the second person's phone
([D120](../../decisions/working-notes.md) corrects
[D119](../../decisions/working-notes.md)'s wording).

**Film audio on the phone with video on the television is out of scope**, and the number says
why. Splitting audio and video across two devices is **lip sync**: ITU-R BT.1359 puts
detectability at audio leading by **45 ms** or lagging by **125 ms**, and EBU R37's production
window is **+40 / −60 ms**. Our sync budget ([D66](../../decisions/working-notes.md)) nudges
out **60–3000 ms** and reconciles periodically **in minutes** — our finest correction is
already at the detectability threshold. These are not the same problem and reusing one
mechanism for the other yields something subtly wrong all the time. The want underneath is
private listening, and the answer is Bluetooth headphones paired to the television.

**tvOS does not gate any of this** ([D67](../../decisions/working-notes.md)). It has WebRTC
audio/video but no data channels, needs a locally built framework, and carries an open tvOS 16
issue. Where it cannot keep up it does without, and Swiftfin or Infuse via the
[Jellyfin adapter](jellyfin-api-shim.md) is the fallback for Apple TV users.

## What this forces: less than it first appeared

Nothing new, as it turns out ([D68](../../decisions/working-notes.md)).

Synchronisation needs no infrastructure: if the remote participant can reach the server, the
server is the rendezvous. And **voice needs no TURN service either**, because the same thing is
true of it — by the time voice matters, both parties are already connected to the media server
by construction, since otherwise there is no film to watch together. So **the server relays the
voice**, at ~24–64 kbps of Opus against a film already crossing the same uplink, and with almost
no latency penalty because the relay sits **at one endpoint** rather than in a datacentre
between them.

**Relay first, upgrade later.** Start on the server relay — instant, always works — and migrate
to a direct path if hole punching succeeds. This is Tailscale's trick and it is most of why
their relay rate is under 10% while WebRTC's is 20–25%: ICE must connect within a second or two
and so gives up and relays, whereas a background upgrade has unlimited time. It also means the
relayed path must be good rather than a degraded mode, since everyone uses it for the first
second.

Only two situations make a relay genuinely unavoidable
([remote-access.md](../research/remote-access.md) §3): **both** ends behind hard/symmetric NAT —
a single hard NAT is beaten by port prediction in about two seconds, two of them need ~170,000
probes per side — or UDP blocked outright, where TCP/443 with TLS is the only path.

The consequence for [D64](../../decisions/working-notes.md): the premium tier's justification is
**remote access itself**, not voice. Voice rides along.

## Open

- **Does one participant stalling pause everyone?** Obviously yes for two, obviously no for
  eight. Jellyfin makes it a flag; we probably want a default that varies by group size — which
  is opening (5) above, so the question can stay open without blocking anything.
- **Is the subtitle track group state or local?** One person may need subtitles. Local is the
  intuitive answer, but then "seek to the funny bit" and "turn subtitles on" behave
  inconsistently, which is the kind of inconsistency users report as a bug.
- **Text chat**, and whether emoji reactions are the TV-shaped answer.
- **Whether `setRate` works anywhere we care about.** A device-lab question.
- **Real playback-position drift per platform.** The 10–20 minute reconciliation interval is
  derived from a 50 ppm clock assumption; playback-rate error will probably dominate it and may
  be far worse on cheap hardware. Device lab.

## Two things the audit attached to this surface

- **Casting is this, with a group of one** ([D88](../../decisions/working-notes.md)). The
  six-method per-client surface — join, leave, report position, accept play/pause/seek, report
  readiness, report buffering — is exactly what one device driving another device's playback
  needs: group size one, a remote controller, on the LAN, voice off. Saying so prevents the
  usual outcome, a second parallel remote-control protocol that then diverges. It also makes a
  Chromecast receiver an ordinary [D2](../../decisions/working-notes.md) client rather than a
  special case, and AirPlay free wherever the Apple client uses AVPlayer. The claim to verify
  is that six methods are still enough for a case they were not designed against — if a seventh
  is needed, better to know before the surface is frozen.

  **Solo casting is specified in [D135](../../decisions/working-notes.md)**, and the difference
  from this plan is the principal rather than the protocol: a session link admits *a person
  with no account*, while a cast URL delegates to *a device on behalf of someone already logged
  in*. So it needs no invitation, no roster and no revocation UI — its credential is the cast
  session itself, ending on stop, supersession, idle or an absolute cap.

  **And the verification this decision asked for has come back yes**
  ([D136](../../decisions/working-notes.md)). The case six methods were not designed against —
  a television pausing itself over HDMI-CEC, with no sender involved — needs **no seventh
  method**: it is a suspension with a reason, which the list above already provides for. What
  it does need is that the receiver *reports* it, because the receiver is the only thing that
  saw it ([principles §9](../../principles/README.md)).
- **Playback rate is either shared state or explicitly local** — there is no third option
  ([D90](../../decisions/working-notes.md)). A participant at 1.25× is not drifting, and
  [D66](../../decisions/working-notes.md)'s reconciler must not spend the film fighting them.

## Done when

**Phase 1 — two participants.** Two people on different networks watch a film end to end;
either can pause, resume and seek **if the host granted `group` transport**, and only the host
can otherwise; scrubbing does not drag the other person around; a stall on one side is visible
on the other as a named reason rather than a freeze; voice is intelligible with per-speaker
levels and no echo with one side on speakers; **one of them runs voice on a phone while a
television plays the video, and the roster shows one person rather than two**; **the host
changes the film mid-session and nobody's player is torn down**; a film ending leaves the
session alive; and a third client platform gains the whole feature by implementing six
methods.

**Phase 2 — four or five.** The same, with a roster, and **without the audio path or the
group-state record changing shape**. If either has to change, opening (1) or (3) was not left
properly.

**And the recovery path is tested, because it is the path nobody exercises until they need
it** ([D117](../../decisions/working-notes.md), [D119](../../decisions/working-notes.md)): an
ejected participant is gone within one reconciliation interval; **rotating the join link is
invisible to everyone already in the session** and the old link admits nobody; and a client
mid-join when the link rotates either completes or is refused, never landing half-joined.
Nothing is re-formed — that was D117's conflation, and D119 removed the need for it.
