> **ABOUTME:** Synchronised remote playback with shared controls and voice chat — the feature
> five commercial services built and then deleted, and the design constraint that follows from
> why theirs died.

# Watch Together

- **Status:** PLANNED — researched 2026-09-19 ([watch-together.md](../research/watch-together.md), [D62](../../decisions/working-notes.md)–[D64](../../decisions/working-notes.md)).
- **Depends on:** [stream-service.md](stream-service.md), [web-client.md](web-client.md) (the SDK), [remote-access.md](remote-access.md) (for the off-LAN case)

## Scope

Anyone in a group may **play, pause and seek**. Volume is local. **Voice chat** with per-source
gain, so one loud participant does not drown the film. Text chat deferred; emoji reactions are
probably the right shape for a television.

The motivating case is two people in different places, one of them travelling — which means
this is a *remote* feature first and a LAN feature incidentally.

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
- **Two corrections**: nudge `playbackRate` for ~1 s to absorb roughly 60–3000 ms of drift; seek
  beyond that. Rate nudging is **inconsistent across platforms** — Jellyfin's own source records
  it failing on Safari and struggling on Android Chrome — so it is optional and seek-only is the
  always-available fallback. **Measure it in the [device lab](device-lab.md) before relying on
  it.**
- **Group state**: `Idle`, `Waiting`, `Paused`, `Playing`, where `Waiting` is
  paused-until-everyone-is-ready.
- **Suspensions with reasons**, Apple's model: a participant detaches temporarily and says why.
  `stallRecovery`, `audioSessionInterrupted`, `userActionRequired`, and above all
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

**On a television the phone is the voice endpoint.** An Apple TV has no app-accessible
microphone, and a microphone across the room from the speakers is the worst possible AEC
geometry regardless. Phone-and-earbuds for voice, TV for video.

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
  eight. Jellyfin makes it a flag; we probably want a default that varies by group size.
- **Is the subtitle track group state or local?** One person may need subtitles. Local is the
  intuitive answer, but then "seek to the funny bit" and "turn subtitles on" behave
  inconsistently, which is the kind of inconsistency users report as a bug.
- **Text chat**, and whether emoji reactions are the TV-shaped answer.
- **Whether `setRate` works anywhere we care about.** A device-lab question.
- **Real playback-position drift per platform.** The 10–20 minute reconciliation interval is
  derived from a 50 ppm clock assumption; playback-rate error will probably dominate it and may
  be far worse on cheap hardware. Device lab.

## Done when

Two people on different networks watch a film end to end; either can pause, resume and seek;
scrubbing does not drag the other person around; a stall on one side is visible on the other as
a named reason rather than a freeze; voice is intelligible with per-speaker levels; and a third
client platform gains the whole feature by implementing six methods.
