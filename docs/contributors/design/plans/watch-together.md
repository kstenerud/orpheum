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

- **Clock offset** by NTP-style four-timestamp exchange over the existing WebSocket, keeping a
  rolling window and using the **minimum-delay sample**. Greedy at 1 s, then 60 s.
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
- **The server serialises.** Concurrent conflicting commands are normal, not exceptional. Each
  command carries the group-state version it was issued against, the server assigns a monotonic
  sequence, and clients reconcile to the server rather than to each other.
- **Sync never rides a WebRTC data channel** — unsupported on tvOS, and it would make
  synchronisation depend on peer connectivity.

## Voice

WebRTC, peer-to-peer where NAT allows, TURN relay otherwise.

**Headphones are the supported configuration and the UI says so.** The film plays out loud while
the microphone is open, and AEC can only cancel audio it has a reference for — which excludes
anything played by libmpv, ExoPlayer or AVPlayer, and excludes Web Audio output in a browser.
Without headphones your microphone sends the film back to the other person, delayed and doubled.
With headphones at both ends there is no acoustic path and the problem does not exist.

**Ducking conflicts with AEC**, so it is conditional rather than always-on:

| Output | Film audio path | Ducking | AEC |
| --- | --- | --- | --- |
| Headphones | through our mixer | automatic | irrelevant |
| Speakers | native playback path | manual levels only | keeps its reference |

Per-source gain is independent of both — one gain node per remote stream — and is the explicitly
requested control.

**On a television the phone is the voice endpoint.** tvOS has WebRTC audio/video but no data
channels and needs a locally built framework; an Apple TV has no app-accessible microphone; and
a microphone across the room from the speakers is the worst possible AEC geometry regardless.
Phone-and-earbuds for voice, TV for video.

## What this forces

Nothing new for synchronisation — if the remote participant can reach the server, the server is
the rendezvous. But **voice needs a TURN relay for the ~10% of networks hole-punching cannot
traverse**, and that is unambiguously another machine, which cannot be self-hosted by the person
who needs it. See [D64](../../decisions/working-notes.md): it does not settle whether we operate
a service, but it settles what the paid tier would be *for*, and it is the one candidate that
passes the README's promise without argument.

## Open

- **Does one participant stalling pause everyone?** Obviously yes for two, obviously no for
  eight. Jellyfin makes it a flag; we probably want a default that varies by group size.
- **Is the subtitle track group state or local?** One person may need subtitles. Local is the
  intuitive answer, but then "seek to the funny bit" and "turn subtitles on" behave
  inconsistently, which is the kind of inconsistency users report as a bug.
- **Text chat**, and whether emoji reactions are the TV-shaped answer.
- **Whether `setRate` works anywhere we care about.** A device-lab question.

## Done when

Two people on different networks watch a film end to end; either can pause, resume and seek;
scrubbing does not drag the other person around; a stall on one side is visible on the other as
a named reason rather than a freeze; voice is intelligible with per-speaker levels; and a third
client platform gains the whole feature by implementing six methods.
