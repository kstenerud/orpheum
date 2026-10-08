> **ABOUTME:** An outside review of Watch Together's synchronisation: how we tell a person's own
> moves from the player's, catch-up, drift correction, the group clock and the iPhone's start-up,
> set against Syncplay, Jellyfin SyncPlay, MediaSync, DVB-CSS, NTP and the smaller open-source
> watch-together projects. For each area: what we do, what others do, whether ours is sound, the
> risks, and ranked recommendations. Gathered 2026-10-08. Feeds the owner's decisions on D250,
> D264 and the M5 player.

# Sync review: is Watch Together's synchronisation on the right track?

Researched **2026-10-08**, after a week of fixes to the follower
(`web/packages/sdk/src/together-follow.ts`, `together-drift.ts`, `together-connection.ts`) and the
hub (`internal/core/together/group.go`). The design under review is
[watch-together.md](../plans/watch-together.md), as D246, D247, D250, D252, D253, D261, D262 and
D264 ([working-notes.md](../../decisions/working-notes.md)) made it, and the Safari and hls.js
entries in [idiosyncrasies.md](../../idiosyncrasies.md).

Sources were read first-hand, from source code where it exists. The clones and fetched pages are
cached under `/yoloai/cache/sync-research/`:

- jellyfin/jellyfin at `284997e` and jellyfin/jellyfin-web at `3f2e88c`;
- Syncplay/syncplay at `2af7ace`;
- hls.js at `3a2e737`;
- Motion Corporation's `mediasync.js`, and the timingsrc and timingobject repositories;
- BBC pydvbcss, and ETSI TS 103 286-2 V1.3.1 (DVB-CSS);
- RFC 5905 (NTP), RFC 7272 (IDMS) and ITU-R BT.1359;
- CyTube (calzoneman/sync), howardchung/watchparty, RblSb/SyncTube and Metastream;
- the GitHub issues and PRs cited below.

Web search was exhausted, so pages were fetched by known URL. **Nothing could be verified about
how the commercial services sync** (Teleparty, Discord, Prime Video Watch Party, Disney+
GroupWatch, Hulu, Twitch): their help pages would not load, and no engineering write-up was found.
Plex appears only through one help-page sentence. Kyoo has no Watch Together: its issue #341 is
still open. Anything below not read in a source is marked as **our inference**.

## Executive summary

**Overall: the architecture is sound and matches the field.** A server-held group timeline, a
follower that corrects drift by rate nudges or seeks, and waiting only at sync points are what
Jellyfin, Syncplay and the research libraries do. Our "jump ahead and wait" catch-up is better
than anything shipped in the open. The trouble is in three places:

1. **We hold a tighter bound than anyone ships, on the browser that fights it.**
   - Our allowance is 150 ms.
   - Jellyfin turned drift correction **off by default** in 2023 ("constant stuttering"; Safari
     "failing").
   - Syncplay and CyTube tolerate 1.5–2 s.
   - MediaSync forces seek-only mode on Safari.
   - Recommendation: a per-browser policy chosen from measured cost, wider on iPhone.
2. **The group clock never forgets its best pong.**
   - NTP, DVB-CSS and Jellyfin keep the best of a recent window, or let old samples age out.
   - Ours keeps the best for the whole timeline, so clock skew between devices goes uncorrected
     over a long film.
   - Our acceptance runs both browsers on one machine, with one clock, so it cannot see this.
3. **Intent is inferred from `<video>` events, with one-shot flags.**
   - Jellyfin takes the person's command at the button and never reads element events as intent.
   - Syncplay compares states rather than counting events.
   - Most of this week's bugs live here.
   - Recommendation: route the host's commands through the M5 controls, with a state comparison
     as the fallback.

Two smaller points:

- Jellyfin starts every player at a scheduled moment. We start each player when the message
  reaches it, then nudge.
- The follower has grown about ten interacting flags. It should become an explicit state machine.

## How the field compares, at a glance

| | Orpheum | Jellyfin SyncPlay | Syncplay | MediaSync (Motion Corp.) | CyTube / SyncTube / watchparty |
| --- | --- | --- | --- | --- | --- |
| Timeline held by | the server, extrapolated per device | the server, scheduled in server time | the server, at the **slowest** watcher | a shared timing object | the server (CyTube, SyncTube); the max or median of members (watchparty) |
| Clock | no shared clock; group placed by the all-time shortest-RTT pong (D264) | NTP-style offset, best of the **last 8** by delay | moving average of RTT/2 | skew, adjusted gradually | none, or one sample (Metastream) |
| Drift left alone | 150 ms | 60 ms, but correction **off by default** | 1.5 s (behind: 1.75 s, then a seek at 5 s) | 25 ms | 2 s (CyTube, SyncTube), 0.5 s (watchparty) |
| Rate correction | ±10% for 1 s, repeated | `1 + diff/1000` for 1 s, 60–3000 ms | slow down only, 0.95× | continuous, gain-scheduled | watchparty: speed-up only, at most 1.1× |
| Seek correction | beyond 3 s | from 400 ms | rewind beyond 4 s; fast-forward beyond 5 s, +0.25 s | beyond 1 s; **Safari: seek only** | the only correction (+0.5 s or +1 s lead) |
| Waiting for a slow member | only at sync points, at most 20 s (D250) | **every** buffering member pauses the group; 30 s timeout | readiness only at start | n/a | SyncTube: at a new item, at most 3 s |
| Start after a wait | each player as the message arrives | **scheduled** at now + max(2 × highest ping, 500 ms) | readiness countdown | n/a | CyTube: 3 s lead-in |
| Person's own move detected by | element events + one-shot "own" flags + 300 ms debounce | UI methods replaced; element events never read as intent | state comparison: differs from both the last player state and the global state by > 1 s | n/a | — |

Sources for each cell are in the sections below.

## 1. Telling a person's moves from the player's and the follower's own

### What we do

The host's player has the browser's native `controls` (`web/apps/web/src/play.tsx`), so a person's
play, pause and seek reach us only as media-element events. `commandOfEvent` turns them into
commands:

- a `play` while Paused is a play;
- a `pause` or `ended` while Playing is a pause;
- a `seeking` more than 3 s from where the group is expected is a seek.

A pause or seek is held for `scrubPauseWindowMs` (300 ms), so a drag becomes one seek. Moves that
are not the person's are excluded by one-shot flags:

- `#isOwnSeekPending`, set by our own seeks and by `orpheumplayermoveditself`, which is hls.js's gap
  jumps and stall nudges, re-dispatched from its non-fatal ERROR events;
- `#isOwnPausePending`, set by holds and waits ahead. This flag was added today, after a host's play
  was stopped 0.4 s later by its own hold's pause (066918fa).

### What others do

- **Jellyfin never reads an element event as intent.** While SyncPlay is on, it replaces the
  playback manager's `playPause`, `pause`, `unpause`, `seek` and similar methods with ones that
  send requests to the server. Remote commands use the saved local methods. Element events only
  feed internal state (`jellyfin-web src/plugins/syncPlay/ui/players/NoActivePlayer.js:26-80`). A
  `waiting` counts as buffering only if no `playing` follows within 3 s
  (`HtmlVideoPlayer.js`, `minBufferingThresholdMillis`).
- **Syncplay compares states; it does not count events.** A pause change is the person's only if
  the new paused state differs from both the player's last state and the group's. A seek is the
  person's only if the new position is more than `SEEK_THRESHOLD` (1 s) from both the player's last
  position and the group's (`syncplay/client.py:218-223`). So a correction the client made, which
  lands on the group's position, can never be echoed back as a person's seek, with no flag to go
  stale. On top of this:
  - each correction skips itself when the change was "set by" this user;
  - an `ignoringOnTheFly` counter drops incoming state until the server has echoed the client's own
    change (`protocols.py:79-80, 283-325`).
- **hls.js writes `currentTime` itself in three places**, each a real `seeking`/`seeked`:
  - `_trySkipBufferHole` (to the next start + 0.1 s);
  - `_tryNudgeBuffer` (+0.1 s per retry, after a stall of 2 s or a `waiting`);
  - `nudgeOnVideoHole` (+0.000001 s);

  all in `src/controller/gap-controller.ts`. Its maintainer on hls.js#7680: "If hls.js didn't seek
  over gaps, playback would stall." Since 1.7.0 the gap controller is replaceable
  (`config.gapController`), already noted in idiosyncrasies.md.

### Is ours sound?

**Partly, and it is the most fragile part of the design.** Inferring intent from events is
unavoidable for an iPhone in native fullscreen, and for media keys. But one-shot flags have two
failure modes we have not yet hit, and will:

- **A flag that is set and never consumed.** For example, a `pause()` whose event the browser
  coalesces, or a seek whose `seeked` never comes (WebKit 326390, below). The flag then swallows
  the person's next real pause or seek.
- **A flag consumed by the wrong event.** For example, the person drags the timeline while our own
  catch-up seek is still loading. The person's `seeking` is taken for ours, and the host's seek is
  lost.

Two smaller points:

- **The 3 s "jump" test drops short seeks.** A host's seek of less than 3 s is not a command, and
  is then pulled back by the drift correction. Syncplay's 1 s, measured against both the player's
  last position and the group's, is tighter without catching hls.js's sub-second moves.
- **The 300 ms debounce is the field's normal answer for scrubbing.** But it also delays every real
  pause by 300 ms, and it holds `#isAwaitingGroup`, during which nothing is applied.

### Recommendations, ranked

1. **Take the host's commands at the controls, not from the element** (Jellyfin's pattern).
   - M5's skinned player (D245) replaces the native `controls` anyway. Its play, pause and seek
     should call the SDK, which sends the command and then moves the local element with its "own"
     mark already set.
   - For the iPhone's native fullscreen, and for lock-screen and media keys, register
     `navigator.mediaSession` action handlers (`play`, `pause`, `seekto`, `seekbackward`,
     `seekforward`), so those also arrive as commands. **Our inference:** not tested on iOS
     fullscreen; device-check it (a VQ item).
   - *Design:* this narrows D62's "per-client surface" in the right direction: a client gives
     commands, not events. It needs no protocol change, and it fits M5.
2. **Replace the one-shot flags with Syncplay's state comparison** for whatever still arrives as
   events.
   - Keep the state the follower last asked for (playing or paused, a target position, and when it
     asked).
   - An event is the person's only if the element's new state differs from **both** that target and
     the group's, by more than a threshold for positions. Our own seeks then land on the target and
     are never a person's; hls.js moves of 0.1–2 s fall under a 1 s threshold anyway.
   - *Design:* none; implementation only.
3. **Until then, bound the flags in time.** An "own" mark should expire, for example after 2 s,
   rather than wait indefinitely for its event, and each expiry should be logged. *Design:* none.
4. **Mark hls.js's moves at the source:** a `gapController` subclass that flags its own
   `currentTime` writes, instead of inferring them from ERROR events. This is the existing "better
   workaround" in idiosyncrasies.md. *Design:* none.

## 2. Catch-up: falling behind, joining late, sync points

### What we do

The group waits (`Waiting`) only at its sync points (D250 §2): a film put on, the host's play after
a pause, and the host's seek.

- At a sync point, it waits for every player that has once been ready, for at most 20 s (D250 §3).
- A player still loading its first picture holds no one up (D246 §6).
- A stall never stops the group. The stalled player catches up alone: it seeks ahead of the group,
  by its last load time plus the time its measured link needs to fill 80% of 30 s, and at most a
  minute (D250 §4). It waits there paused, and plays when the group arrives.
- A player counts as ready after a stall only with 10 s loaded ahead (`refillAheadMs`, D247 §5).

### What others do

- **Jellyfin pauses everyone for anyone.**
  - A `Buffer` request while Playing sends Pause to every ready member, and resumes when all are
    ready (`MediaBrowser.Controller/SyncPlay/GroupStates/WaitingGroupState.cs:356-378`).
  - A late joiner pauses the group too (`PlayingGroupState.SessionJoined`).
  - The resume is scheduled at now + delay when a recovering member is behind by more than
    2 × the highest ping (`WaitingGroupState.cs:500-519`).
  - A ready report whose position is more than `MaxPlaybackOffset` (500 ms) off gets a corrective
    seek for that member alone. This bound was widened to `MaxCatchUpOffset` (60 s) for a member
    still playing, in PR jellyfin/jellyfin#17797 (2026-09). Before it, a stale client was sent a
    pause scheduled "1804 seconds" ahead, which the PR names as the likely cause of the auto-pause
    and jumping loops in #8785, #12422 and #16241.
  - A 30 s group-wait timeout arrived only in PR #17938 (2026-09), after a joiner that never said
    Ready left the host's spinner turning forever (#17864).
  - A per-member timeout (60 s for a first start, 10 s for a re-buffer) is still an open PR, #17349.
- **Syncplay follows the slowest.** The room's position is the minimum of its watchers' positions
  (`syncplay/server.py:597-608`). Readiness gates only the start (`unpauseAction=IfOthersReady`).
- **CyTube, SyncTube and Metastream only seek, with a small lead.**
  - CyTube seeks to the group's position + 1 s "to account for possible buffering", and gives new
    media a 3 s lead-in.
  - SyncTube seeks to its position + 0.5 s, and waits for every client's `VideoLoaded` for at most
    3 s.
  - Metastream's `syncOnBuffer` jumps ahead when more than 1 s behind.
- **Plex:** its FAQ says one member's re-buffering "could cause other members of the session to
  also pause". No mechanism is published.
- **RFC 7272 (IDMS)** names the choice: sync to a reference, "the most lagged client" for example,
  with the others slowing down or pausing.

### Is ours sound?

**Yes, and it is the strongest part of the design.**

- **Never stopping the group for one member avoids Jellyfin's bug class.** Jellyfin's "pause
  everyone, resume when all ready" produced years of auto-pause, jump and deadlock reports, fixed
  only in the last month. Jellyfin's maintainer on jellyfin-web#6210: "until someone is willing to
  essentially rewrite how syncplay works this is unlikely to be fixed."
- **Catching up by jumping ahead and waiting is the CyTube and SyncTube lead, done properly.** It
  is sized from the measured link instead of a fixed 0.5–1 s.

The risks are in the execution, not the idea:

- **The follower has outgrown its shape.** `GroupFollower` carries `#isOwnSeekPending`,
  `#isOwnPausePending`, `#isAwaitingGroup`, `#hasCaughtUp`, `#isRefilling`, `#waitAtMs`,
  `#seekStartedAtMs`, `#pendingCommand`, `#nudgeEndsAtMs`, `#settlesAtMs` and
  `#behindAtReturnMs`. That is about 2¹¹ combinations, of which a handful are meaningful. Each of
  this week's fixes added a guard for one combination, so the next bug is likely another unguarded
  one.
- **The iPhone's ManagedMediaSource can stall a seek ahead.**
  - WebKit 326390, filed 2026-10-05: after `endstreaming` at about 30 s buffered, a seek outside the
    buffer never sets `streaming` back to true, "so hls.js waits forever". Already in
    idiosyncrasies.md.
  - A catch-up jump of up to a minute is exactly that seek.
  - An earlier bug of the same family, WebKit 321919, got an hls.js workaround (#7984, in 1.6.19 and
    1.7.1, which resumes loading on `seeking`). Whether that workaround covers 326390 on our pinned
    1.7.3 is unverified.
- **A sync point's 20 s wait now ends on the server only when a ready player sends a message**
  (`endWait`, run on every message). That is fine while every ready player reports each second, as
  it does today, but it is a dependency to keep in mind. Jellyfin moved to a 1 s server timer for
  the same job (`SyncPlayManager.cs:24`).

### Recommendations, ranked

1. **Make the follower an explicit state machine.** Suggested states: *Following*, *Nudging*,
   *SeekingToHold*, *WaitingAhead*, *Holding*, *Refilling*, *CommandPending*. Each event or tick
   becomes a transition, and each transition gets a test. The behaviour stays the same; only the
   shape changes, which is what would have prevented 066918fa's bug. *Design:* none.
2. **Log the iPhone's ManagedMediaSource `streaming` state** (and `startstreaming` and
   `endstreaming`) in the "what it holds" note. A catch-up seek that has not finished after N
   seconds should be treated as stuck, retried by a small seek, and noted. *Design:* none; it adds
   an idiosyncrasy workaround if confirmed.
3. **Keep the per-sync-point wait, but consider Jellyfin's open idea of two timeouts**: longer for a
   first load, shorter for a re-buffer. D247 §4 once had the same split (60 s and 10 s), which
   D250 §3 replaced with one 20 s wait. *Design:* the owner's (D250 §3).

## 3. Drift correction: nudges, seeks, the allowance, Safari

### What we do

Every second the follower compares the player's position with the group's:

| Drift | Correction |
| --- | --- |
| up to 150 ms | none |
| up to 3 s | a nudge: up to ±10% for 1 s, repeated |
| beyond 3 s | a catch-up seek |

- No nudge is made in the 300 ms after a play, a seek or a rate change, because Safari's
  `currentTime` stands still then.
- Since today, each player learns what its rate going back to 1× costs it: about 220 ms on iPhones
  (WebKit 163433). A nudge then ends past the group by that much (6494dc73).

### What others do

- **Jellyfin's SpeedToSync** is `rate = 1 + diff/1000 ms` for 1 s, between 60 and 3000 ms.
  SkipToSync seeks from 400 ms. Corrections are at least 1.5 s apart, and start 1.5 s after an
  unpause (`PlaybackCore.js:50-69, 547-603`).
  - **`enableSyncCorrection` defaults to false.** It was first switched off on mobile in
    jellyfin-web PR #2946 (2021, "it causes constant stuttering"), then everywhere in PR #5003
    (2023-11, fixing #4972: correction "seems to cause a lot of issues when most people probably
    don't care about super precise syncing").
  - The code still says: "SpeedToSync is failing on Safari (Mojave); even if playbackRate is
    supported, some delay seems to exist" (`PlaybackCore.js:556`).
  - On Samsung TVs even a 400 ms SkipToSync was too aggressive (jellyfin-tizen#71).
- **Syncplay** only slows down, to 0.95×, when more than 1.5 s ahead, and returns to 1× under
  0.1 s. It never speeds up. A player behind waits for the room to be at the slowest member's
  position, or fast-forwards by a seek after 5 s (`constants.py:72-91`, `client.py:434-484`).
- **MediaSync** (Motion Corporation, `mcorp.no/lib/mediasync.js`):
  - **On Safari it switches itself to seek-only mode**
    (`if (navigator.userAgent.indexOf('Safari') != -1 && … 'Chrome' == -1) options.mode = "skip"`).
  - Everywhere it averages 3 position samples, because "currentTime is only a coarse
    representation".
  - It gives up on the rate for good after repeated jumps of more than 0.5 s.
  - In seek mode it **learns its overshoot**: each seek aims past the target by the miss measured
    after the last one. That is the same idea as our learned return cost, applied to seeks.
  - It doubles its target when more than 3 seeks come within 1.5 s ("thrashing").
  - Its 2015 report: "functioning variable playback rate is the single most important factor …
    Seeking is very disruptive", and Safari never reached its 10 ms target by seeking.
- **hls.js maintainer**, on hls.js#7616 (Safari: "nondeterministic rate change and seek latency …
  100ms to 500ms from the ideal"): change `currentTime`, play/pause and `playbackRate` only on
  muted elements, with one element playing sound at 1×.
- **WebKit 163433** ("playbackRate update while playing creates a small pause") is open since 2016.
  A 2024 comment reports 200–300 ms freezes on iOS since iOS 15, still in 17.2. **WebKit 225868**
  (distorted audio when changing the rate) is open.
- **What people notice:**
  - ITU-R BT.1359 puts lip-sync detectability at +45 ms (sound early) and −125 ms (sound late).
    That is within one screen, not between two people.
  - Geerts et al., CHI 2011, "Are we in sync?", found that with voice chat people notice
    differences sooner and are more annoyed than with text. We could not read the full paper, so
    **we have no verified threshold in seconds**.
  - A W3C note puts the browser's own timing jitter at up to 250 ms (`timeupdate` fires every
    15–250 ms, per the HTML standard).

### Is ours sound?

**On Chrome and Firefox, yes**: our numbers sit between Jellyfin's and MediaSync's, and the
acceptance holds them. **On Safari and iPhone, we are fighting the platform at the edge of what it
can measure.**

The 150 ms allowance sits below each of these Safari errors:

- the 250 ms `currentTime` freeze;
- the 220 ms return cost;
- the 100–500 ms landing error of a rate change or seek.

So each correction can create the next drift it corrects. On an audible element, each nudge is two
rate changes, so two stutters or glitches.

The learned return cost is a reasonable patch, and MediaSync's learned overshoot shows it is a known
technique. But:

- **It learns from a noisy, still-frozen position.** It reads `currentTime` once, 300 ms after the
  return, when Safari's position may still be standing still.
- **It cannot tell the return's cost from a dropped frame or a micro-stall in the same window.**
- **It still pays two audible rate changes per correction.**

**Our inference, on the 150 ms target:** a laugh crossing our voice path already arrives up to the
300 ms voice bound (D243 §9) after its moment. So holding the pictures much closer than the voice
path's latency buys little a person can perceive. D65's "offset alignment" is the same observation
from the other side.

### Recommendations, ranked

1. **Choose the correction per browser from what it measures, not from the User-Agent** (the
   owner's "browser-specific policies", feature-detected as D250 §7 requires).
   - A player whose learned return cost, or seek landing error, is above the allowance switches to
     **seek-only with learned overshoot** (MediaSync's skip mode), with a wider allowance of the
     order of 300–500 ms.
   - Any other player keeps nudging.
   - *Design:* a new decision refining D250 §1 and D261 §1. The numbers stay settings.
2. **Measure position better on WebKit.**
   - Average 3 or more samples, as MediaSync does, or read `requestVideoFrameCallback`'s `mediaTime`
     with its `expectedDisplayTime`. These give the presented frame and when it was shown, rather
     than Safari's frozen estimate. rVFC is in Safari from 15.4 (MDN compat data).
   - **Our inference:** whether rVFC fires in the iPhone's native fullscreen is unverified; device-check
     it.
   - *Design:* none.
3. **Reconsider 150 ms for iPhone pairs specifically.**
   - D261 §1 chose 150 ms against a 200 ms / 95% acceptance bound. Every open-source peer that ships
     correction on runs at 0.4–2 s.
   - If the owner keeps 150 ms for Chrome and Firefox, an iPhone row of D263's "bounds per client
     pair" at, say, 400 ms would be honest about the platform.
   - *Design:* the owner's; it changes D243 §9 and D261 §1 for one client pair.
4. **Nudge in one direction only for small drifts** (Syncplay: slow down only). A player ahead
   slows; a player behind is left for the group's next placing or a seek. **Our inference:** this
   halves the corrections, but only matters if nudging is kept on Safari at all. *Design:* none.

## 4. The group clock

### What we do

There is no shared clock (D66). Each follower extrapolates the group's position on its own
monotonic clock:

- from the group message, aged by half the shortest RTT, until a pong arrives;
- then from the pong with the **shortest round trip yet**, aged by half that round trip (D264).

A later pong replaces it only if its round trip is shorter still. Pings go every 15 s, and once at
each new timeline. A group message of the same timeline never re-places it (475c26cc).

### What others do

- **NTP** (RFC 5905 §10) keeps an 8-stage register of samples and takes the offset of the
  lowest-delay one. Every stored sample's dispersion grows with age (15 ppm), and a sample older
  than the latest is never used. So **the minimum is over a sliding window, and stale minima age
  out**.
- **DVB-CSS** (ETSI TS 103 286-2 §C.8.3) picks "the candidate with the lowest dispersion at the
  present time". Dispersion is RTT/2, plus the precision of both sides, plus
  **(φs + φc) × the measurement's age**. Its worked example re-measures within 3 s. BBC's
  `pydvbcss` recommends `LowestDispersionCandidate(repeatSecs=1.0)`, and its JS client assumes
  50 ppm of frequency error.
- **Jellyfin** keeps the **last 8** measurements and uses the one with the smallest delay. It pings
  every 1 s until 3 succeed, then every 60 s (`timeSync/TimeSync.js:11-14, 98-108`).
- **The timing object spec** applies skew changes gradually ("1 ms change per 10 ms"), so a
  re-placing never makes a visible jump.
- **Syncplay** uses an exponential moving average of RTT/2, which is cruder.
- **Metastream** uses one sample, with a TODO to average.

### Is ours sound?

**The shortest-RTT idea is right**: D264 is Cristian's and NTP's method, and the *congested* result
(median 30 ms) shows it. **Keeping the minimum for the whole timeline is not.** Over a film played
without a pause, the group is placed once, in the first seconds, and extrapolated for up to two
hours on each device's own clock.

Clock-rate errors between devices are typically tens of ppm (DVB-CSS's example uses 1000 ppm
combined as a worst case; BBC assumes 50). **Our inference** from those figures: at 50 ppm between
two devices, a 2-hour film opens 360 ms between them; at 20 ppm, 144 ms. Either is near or beyond
the 150 ms allowance. The drift correction then nudges each player toward its own wrong idea of the
group, and the two never agree.

The acceptance cannot see this. Both browsers run in one sandbox and share one clock, so their skew
is zero by construction. The same holds for the owner's tests on one machine. Two real phones
would show it.

A second, smaller point: a pong is placed only when its round trip is shorter than the best so far.
If the route changes for the better, the new best is taken; if it changes for the worse, the old
placing stays, which is correct. But with the minimum never ageing, nothing re-places the group
once skew has accumulated.

### Recommendations, ranked

1. **Let the placing age** (refines D264).
   - Choose the placing pong by **lowest dispersion now**: half the RTT plus an assumed skew, for
     example 50 ppm, × the sample's age. Equivalently, take the shortest RTT among the last 8 pongs.
   - Apply a re-placing gradually, as the timing object does, or let the existing nudge absorb it.
   - Ping every few seconds while a timeline is young (Jellyfin: 1 s × 3), then every 15 s.
   - *Design:* a decision refining D264. Still no shared clock, so D66 stands.
2. **Log every pong's residual first**: where the pong says the group is, against where the
   follower extrapolated it. That measures each device pair's skew from staging logs before
   anything changes, as "digging means missing logs" asks. *Design:* none.
3. **Add a test for clock skew**: a follower whose monotonic clock runs 100 ppm fast, over two
   simulated hours. With the timeline placed once, the test should show the drift. *Design:* none.

## 5. iPhone start-up: the 10 s timeline offset and a partly held first chunk

### What we do

Every chunk's media time is its film time plus `TimelineOffsetMs` (10 s), so that the AAC priming
and the decode before a keyframe are never below zero (`internal/node/chunks/maker.go`). hls.js
maps media time to playlist time from the first fragment it loads.

Today's log showed:

- an iPhone guest that jumped 10 s on its own at the start, then waited a minute for data;
- earlier, a first chunk whose picture was only partly held, then hls.js jumping a hole.

### What others do, and what is known

- **hls.js** jumps a start gap of up to `MAX_START_GAP_JUMP` = 2 s, but only while playing. A
  paused player is never stepped over a gap (`gap-controller.ts`; already in idiosyncrasies.md).
- On Safari's "bad sessions" (hls.js#5274, maintainer's comments):
  - the element emits `seeking` → `seeked` with an empty buffer → `waiting` → `canplay`;
  - after `canplay`, `currentTime` does not advance for at least a quarter second;
  - hls.js therefore deliberately reads `.seeking` and `.currentTime`, not those events.
- WebKit 305712 (iOS 26.2, open): on the first ManagedMediaSource after a cold start, concurrent
  appends to two SourceBuffers fail; sequential appends work. hls.js#7697 added a recovery. This is
  a candidate for "a first chunk only partly held".
- **Not found:** a report of iOS dropping a seek made before the first frame. The 10 s jump has no
  public match either.

### Is ours sound?

**Not enough is known to judge, and this is the area where we are most likely guessing.** A
non-zero media timeline is legitimate, and hls.js maps it. But a 10 s jump equal to our offset is
a strong hint (**our inference**): on that iPhone, some step saw media time where it expected
playlist time. Possible culprits:

- hls.js's mapping, on a start mid-film whose first fragment loaded was the sound's (whose time
  starts earlier by the priming);
- a seek the follower made before the mapping was known.

### Recommendations, ranked

1. **Capture before fixing.** On the first `FRAG_BUFFERED` of each track, log:
   - the fragment's playlist start, its `tfdt`-derived start, and hls.js's `initPTS`;
   - the element's `currentTime` and `buffered`;
   - whether ManagedMediaSource is `streaming`.

   Today's "what it holds" note (066918fa) is half of this. *Design:* none.
2. **Do not seek a player before its first frame is shown.** The follower's hold and catch-up
   seeks should wait for `readyState >= HAVE_CURRENT_DATA`, or the first `requestVideoFrameCallback`,
   on a fresh element. Use hls.js's `startPosition`, as `chunked.ts` already does, for the first
   placement instead. *Design:* none.
3. **Consider a media timeline equal to the playlist's.** Keep `tfdt` at film time and absorb the
   priming with an edit list or by trimming. That removes a whole class of mapping errors, but it
   is node work and changes the chunk format. *Design:* the owner's. Do it only if step 1 shows
   the offset is the cause.

## 6. Two structural points outside the five areas

### Start players at a scheduled moment

Jellyfin schedules every unpause, and the end of every wait, at a future server time:
`now + max(2 × highest ping, 500 ms)` (`PlayingGroupState.cs:67`; `WaitingGroupState.cs:500-519`).
Every client starts at that moment, from that position.

We start each player when the "Playing" message reaches it. So each starts late by:

- its transit;
- its `play()` latency;
- on Safari, a quarter-second freeze.

Each then nudges in its first seconds, the moment a person is most likely to notice. Today's
0.4 s self-pause also lived in this hand-over.

We can do the same without a shared clock: the server announces Playing with "from position P, in
L ms". Each follower starts at receipt + L − its half shortest RTT, and holds its element until
then. **Our inference:** this removes most start-up nudges. *Design:* a protocol change to
cut-a §5.1's group message, for the owner.

### Is the 200 ms / 95% goal the right one?

No open-source peer holds anything that tight with correction on. The one that ships it switched it
off, saying users do not care (jellyfin-web#4972). The research libraries that reach tens of
milliseconds do it on desktop browsers, and give up the rate on Safari. Our target is achievable on
Chrome and Firefox, as the acceptance shows. **Our inference:** on iPhone, the target is what drives
the stutter the owner hears.

A per-client-pair bound (D263 already allows one) would let iPhone pairs trade 200 ms for silence.
That is the owner's call: it is Cut A's acceptance (D243 §9, D261 §2).

## Recommendations across all areas, ranked

| # | Recommendation | Area | Changes frozen design? |
| --- | --- | --- | --- |
| 1 | Let the group placing age (lowest dispersion now, or best of the last 8); log each pong's residual; test with a skewed clock | 4 | refines D264 (owner) |
| 2 | Per-browser correction from measured cost: seek-only with learned overshoot and a wider allowance where rate changes cost more than the allowance | 3 | refines D250 §1 / D261 §1 (owner) |
| 3 | Host commands from the M5 controls and Media Session handlers; element events only as a fallback | 1 | fits M5; narrows D62 (owner to confirm) |
| 4 | Syncplay-style state comparison instead of one-shot "own" flags; until then, flags expire | 1 | none |
| 5 | Follower as an explicit state machine | 1, 2 | none |
| 6 | Scheduled start after a sync point ("in L ms") | 2, 3 | cut-a §5.1 protocol (owner) |
| 7 | Better position reading on WebKit (averaging, or rVFC) | 3, 5 | none |
| 8 | iPhone start-up: log the mapping and ManagedMediaSource streaming state; no seek before the first frame; watch WebKit 326390 on catch-up seeks | 2, 5 | none |
| 9 | Per-client-pair sync bound for iPhone pairs | 3 | D243 §9 / D261 §2 (owner) |

## Sources

**Jellyfin**
- jellyfin-web `src/plugins/syncPlay/core/PlaybackCore.js`, `core/timeSync/TimeSync.js`,
  `ui/players/NoActivePlayer.js` and `ui/players/HtmlVideoPlayer.js`, at `3f2e88c`:
  https://github.com/jellyfin/jellyfin-web/tree/master/src/plugins/syncPlay
- jellyfin `Emby.Server.Implementations/SyncPlay/Group.cs` and
  `MediaBrowser.Controller/SyncPlay/GroupStates/{Waiting,Playing}GroupState.cs`, at `284997e`:
  https://github.com/jellyfin/jellyfin
- Jellyfin pull requests:
  - jellyfin/jellyfin#17797: https://github.com/jellyfin/jellyfin/pull/17797
  - jellyfin/jellyfin#17938: https://github.com/jellyfin/jellyfin/pull/17938
  - jellyfin/jellyfin#17349
  - jellyfin-web#2946: https://github.com/jellyfin/jellyfin-web/pull/2946
  - jellyfin-web#5003
  - jellyfin-web#1011, comment 613055273
- Jellyfin issues:
  - jellyfin/jellyfin#17864, #8785, #12422 and #16241
  - jellyfin-web#4972: https://github.com/jellyfin/jellyfin-web/issues/4972
  - jellyfin-web#6210
  - jellyfin-tizen#71

**Other players and libraries**
- Syncplay `syncplay/constants.py`, `client.py`, `protocols.py` and `server.py`, at `2af7ace`:
  https://github.com/Syncplay/syncplay
- MediaSync: https://mcorp.no/lib/mediasync.js; the MediaSync Report 2015 (Borch and Arntzen,
  linked from the timingsrc docs); timingsrc's book chapter sources:
  https://github.com/webtiming/timingsrc
- Timing Object specification: https://webtiming.github.io/timingobject/
- CyTube: https://github.com/calzoneman/sync
- watchparty: https://github.com/howardchung/watchparty
- SyncTube: https://github.com/RblSb/SyncTube
- Metastream (archive branch): https://github.com/samuelmaddock/metastream
- Kyoo issue #341: https://github.com/zoriya/Kyoo/issues/341

**hls.js**
- `src/config.ts` and `src/controller/gap-controller.ts`: https://github.com/video-dev/hls.js
- Issues #7680, #7616, #5274 and #7984

**WebKit bugs**
- 163433: https://bugs.webkit.org/show_bug.cgi?id=163433
- 225868, 258945, 305712, 321919 and 326390

**Standards and papers**
- ETSI TS 103 286-2 V1.3.1 (DVB-CSS), §8.2.1 and §C.8.3
- BBC pydvbcss: https://github.com/bbc/pydvbcss
- RFC 5905 §8 and §10: https://www.rfc-editor.org/rfc/rfc5905
- RFC 7272: https://www.rfc-editor.org/rfc/rfc7272
- ITU-R BT.1359-1
- W3C Requirements for Media Timed Events: https://w3c.github.io/me-media-timed-events/
- requestVideoFrameCallback: https://wicg.github.io/video-rvfc/
- HTML Living Standard, media elements (`timeupdate`)
- Geerts et al., "Are we in sync?", CHI 2011, doi:10.1145/1978942.1978986 (abstract only)
