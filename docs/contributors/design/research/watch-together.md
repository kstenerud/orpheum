> **ABOUTME:** Synchronised remote playback with voice chat — what the commercial services built
> and then deleted, how the sync actually works, why the feature keeps dying, and the two hard
> problems (echo cancellation and per-client cost) that decide whether ours survives.

# Watch Together

Researched **2026-09-19**. Sources: the Jellyfin SyncPlay implementation read locally, Apple's
Group Activities and AVFoundation documentation, the support pages of the services that removed
the feature, and the `react-native-webrtc` capability matrix.

## 0. The finding that reframes everything

**Almost every commercial watch-party feature has been shut down.**

| Service | Status |
| --- | --- |
| Disney+ GroupWatch | **Removed 2023-09-18** |
| Amazon Prime Video Watch Party | **Removed 2024-04-02**, quietly, with no announcement |
| Sling | Removed early 2024 |
| Netflix | **Never shipped one.** "Netflix Party" was always third-party |
| Hulu | Still alive |
| **Plex Watch Together** | **Removed 2025-02-25**, except the web app |

Plex's own note is the most informative, because Plex has no content-licensing constraint at all:

> As we debut our new Plex experience, we are ending support for some features we've grown to
> love, like Watch Together. **While this feature won't be available for most platforms as they
> get the new experience**, you can continue using the feature in our web app for the
> foreseeable future.

That sentence names the real cause, and it is not licensing and not lack of demand: **the
feature had to be implemented in every client, and it did not survive a client rewrite.**

Meanwhile the demand is not in question. **Teleparty** — the browser extension formerly called
Netflix Party, which exists purely because the platforms will not do this — has **10,000,000
users** and 17.5K ratings at 4.1 stars.

So the strategic read is: **a genuinely differentiating feature that the incumbents abandoned
for a reason we can engineer around.** And the engineering requirement falls straight out of the
cause of death — see §4.

## 1. How the synchronisation actually works

### Jellyfin SyncPlay, read locally

7,635 lines server-side, plus a client plugin. The mechanism is worth copying almost wholesale.

**Clock offset** is textbook NTP, in `timeSync/TimeSync.js`: a four-timestamp measurement
(`requestSent`, `requestReceived`, `responseSent`, `responseReceived`), a rolling window of
samples, and — the important part — **the sample with minimum round-trip delay wins**, because
the least-delayed sample carries the least queuing noise. Polling is greedy at 1 s for the first
few pings, then drops to 60 s.

**Drift correction has two strategies**, and the thresholds are stated in the source:

```js
minDelaySpeedToSync = 60.0     // ms — below this, do nothing
maxDelaySpeedToSync = 3000.0   // ms — above this, SpeedToSync is hopeless
minDelaySkipToSync  = 400.0    // ms — above this, seeking is acceptable
```

- **SpeedToSync** — nudge `playbackRate` for about a second to absorb 60–3000 ms of drift.
  Invisible when it works.
- **SkipToSync** — seek, for anything larger. Jarring, and it re-buffers.

The honest part is the `TODO` comments they left in `PlaybackCore.js`:

```
// TODO: SpeedToSync sounds bad on songs.
// TODO: SpeedToSync is failing on Safari (Mojave); even if playbackRate is supported,
//       some delay seems to exist.
// TODO: both SpeedToSync and SpeedToSync seem to have a hard time keeping up on
//       Android Chrome as well.
```

So the approach is right in principle and **flaky per-platform in practice** — which is more
evidence for §4's conclusion that the per-client surface has to be tiny and testable.

**Group state is a four-state machine** (`GroupStateType.cs`):

```
Idle    → nothing loaded
Waiting → paused, will start when users are ready
Paused  → paused, will resume on play
Playing → advancing
```

`Waiting` is the one that matters: a client that stalls drops the group into `Waiting` rather
than letting it run away, unless `IgnoreBuffering` is set on the playing state.

### Apple's design, which is better in one specific way

Apple built a platform-level primitive: `AVPlayerPlaybackCoordinator`, tied to a
`GroupSession`, with *"an entirely new playback-sync protocol with deep integration into the
AVFoundation stack."* We cannot use SharePlay itself — it requires FaceTime and Apple IDs — but
one concept in it is worth more than the rest of the API.

**Suspensions**, with enumerated reasons. A participant temporarily detaches from group
coordination, stating why; while suspended they neither obey nor issue group commands; several
can be active at once; when the last clears, they rejoin. Apple's list:

| Reason | Meaning |
| --- | --- |
| `stallRecovery` | buffering after a stall |
| `audioSessionInterrupted` | the system interrupted the audio session |
| `playingInterstitial` | playing something other than the primary content |
| `userActionRequired` | needs user intervention to resume |
| `coordinatedPlaybackNotPossible` | cannot start or resume coordinated playback |
| **`userIsChangingCurrentTime`** | **a participant is actively scrubbing** |

That last one is the detail that would take months to discover independently, and it is exactly
what a shared-seek design needs: **while you are dragging the scrub bar you are suspended**, so
your intermediate positions do not yank everybody else around the timeline. Without it, "anyone
can seek" is unusable.

A named suspension with a reason is also strictly better than a binary in/out for the UI: "Karl
is buffering" is something a person can wait for.

## 2. Voice chat, and the problem nobody mentions

### Echo cancellation is the hard part, not the plumbing

WebRTC is the obvious transport and the plumbing is well-trodden. The problem is that **the
movie is playing out loud while the microphone is open.**

Acoustic echo cancellation works by subtracting a *reference signal* — what was sent to the
speaker — from what the microphone hears. The constraint is that the AEC only gets a reference
for audio it knows about: **only the official playback paths (`<audio>`, WebRTC receiver tracks)
serve as references, and custom playback through the Web Audio API does not reliably function as
one.** On native clients the film is decoded by libmpv, ExoPlayer or AVPlayer and reaches the
output through a path the WebRTC stack has no reference for at all.

Consequence: **your microphone sends the film back to the other person**, slightly delayed,
mixed with their own copy. That is the failure mode, and it is not a tuning problem.

It gets worse with distance and with wireless output — Bluetooth headphones and soundbars
introduce variable latency, and AEC needs the reference and the echo to be time-aligned.

**So headphones are the supported configuration, and we should say so rather than pretend.** If
both participants wear headphones there is no acoustic path at all and the entire class of
problem disappears. That is also what every voice-chat product in practice assumes. Speakers get
best-effort: aggressive noise gating, and ducking where we can.

### Ducking has a direct conflict with the above

Ducking the film when someone speaks requires routing the film's audio through a mixer we
control. In a browser that means the Web Audio API — **which is precisely what stops it being a
usable AEC reference.**

The resolution is a clean rule rather than a compromise:

- **Headphones (no acoustic path):** route through Web Audio, duck freely, AEC is irrelevant.
- **Speakers:** leave the film on the native playback path so AEC has a reference, and accept
  manual levels instead of automatic ducking.

Per-source gain — the user's explicit request, so one loud participant does not drown the film —
is independent of all this and is simply a gain node per remote stream.

### tvOS and Android TV: measured, and awkward

From the `react-native-webrtc` capability matrix:

| | Android | iOS | **tvOS** | Web |
| --- | --- | --- | --- | --- |
| Audio/Video | ✓ | ✓ | **✓** | ✓ |
| Data Channels | ✓ | ✓ | **—** | ✓ |
| Screen capture | ✓ | ✓ | — | ✓ |

tvOS gets audio/video but **no data channels**, requires a locally built `WebRTC.framework`
(issue #1430), and has an open compatibility issue against tvOS 16 (#1597).

Two conclusions:

1. **Do not put playback sync on a WebRTC data channel.** It does not exist on tvOS, and it
   would make synchronisation depend on peer connectivity. Sync goes over our own WebSocket to
   the server, which is also where concurrent commands get serialised.
2. **On a television, the phone is the voice endpoint.** An Apple TV has no app-accessible
   microphone and neither do most Android TV boxes; and even if they did, a microphone across
   the room from the speakers is the worst possible AEC geometry. Phone-plus-earbuds for voice,
   TV for video, is both the only workable configuration and a familiar one.

## 3. The control model the request implies

"Anyone can play, pause and seek" means **concurrent conflicting commands are normal**, not
exceptional. Two people hitting pause at the same moment, or one seeking while the other
resumes, has to have a defined outcome.

The server is already in the path and is the obvious serialisation point: every command carries
the group-state version it was issued against, the server assigns a monotonic sequence, and
clients reconcile to the server's state rather than to each other. That makes commands
**idempotent and ordered**, which is what stops the two-people-both-pausing case from
oscillating.

Volume is local, as requested, and so is everything else that is a property of *this* room:
subtitle track choice arguably too, and that is a genuine open question — one person may need
subtitles and the other may not.

## 4. Why theirs died, and what that requires of ours

Plex's note says the feature did not survive a client rewrite. Jellyfin's `TODO`s say the
per-platform playback-rate behaviour is inconsistent. Both point at the same thing: **the cost
and fragility live in the per-client code.**

So the design requirement, derived rather than asserted:

> **The client-specific surface must be small enough to reimplement in an afternoon.**

Concretely, everything except these belongs in the shared TypeScript SDK that
[web-client.md](../plans/web-client.md) already says is the real deliverable — clock-offset
estimation, drift policy, group state, suspension handling, command sequencing, reconnection:

```
currentPosition() → time        what the player is at, now
seek(time)                      go there
play() / pause()
setRate(r)                      for SpeedToSync; may be unsupported
isBuffering() → bool            drives the stallRecovery suspension
```

Six methods. A client that can do those six gets watch-together, and a client rewrite cannot
casually drop the feature because there is almost nothing in the client to drop. That is the
one lesson worth taking from four dead implementations.

## 5. What this forces

Watch-together with someone who is travelling needs three things, and we have honestly settled
none of them:

1. **She must reach the media at all.** That is
   [remote-access.md](remote-access.md) and [D25](../../decisions/working-notes.md) — the
   project's stated open product question.
2. **A rendezvous for sync.** This one is free: if she is connected to the server, the server
   *is* the rendezvous, and sync needs no infrastructure beyond whatever solves (1).
3. **A path for voice.** Peer-to-peer where NAT allows — remote-access.md's research puts direct
   connection above 90% with Tailscale-style hole punching — and a **TURN relay** for the rest.

A TURN relay is unambiguously another machine that somebody must run. Which makes this the
cleanest possible test of the promise in the README: *"we only charge for things that genuinely
require another machine."* Watch-together does not make the hosted-service question harder; it
makes it **concrete**, and it supplies the first premium-tier member nobody has to feel bad
about.

## 6. Open

- **Does a stalled participant pause everyone?** Jellyfin makes it a flag (`IgnoreBuffering`).
  For two people the answer is obviously yes; for eight it is obviously no. Probably a group
  setting with a sensible default by size, but unmeasured.
- **Is the subtitle track group state or local state?** One person may need subtitles.
  Argues local, but then "seek to the funny bit" and "turn on subtitles for this line" behave
  inconsistently.
- **Text chat.** Deferred, and the honest note is that on a TV it is nearly useless — there is no
  keyboard, and reading chat while watching splits attention on the one screen. Emoji reactions,
  which is what Disney+ GroupWatch shipped instead, may be the right shape for the TV case and a
  full chat the right shape for the web case.
- **Whether `SpeedToSync` is viable on our clients at all.** Jellyfin reports it failing on
  Safari and struggling on Android Chrome. Needs measuring on the device lab before it is relied
  on; the fallback is seek-only, which is worse but always available.
