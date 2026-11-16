> **ABOUTME:** A war-game of the cases a stall design must tell apart (network, core, device, group
> and combined), what each side sees and does today, the right response under the owner's direction
> of 2026-11-16, the information each side must send so the deciding side is not guessing, and where
> two controllers would act on one symptom. Read before designing how the core and the player
> handle a stall.

# Stall War-Game: Telling a Slow Core from a Slow Link

- **Date:** 2026-11-16. Desk exercise against the code at the commit "docs(research): stalls and
  load, what servers and players do when the transcoder or the link falls behind"; nothing run,
  nothing measured anew.
- **Asked by** the lead, for the owner's direction of 2026-11-16: a core in trouble eases the
  processor, not the network; a slow core may drop quality, but not for the same reason or in the
  same way as a network stall; "cannot keep up" steps down or leaves behind per the group, and alone
  is a normal network stall; each side tells the other what it knows (CMSD-style one way,
  CMCD-style the other), so neither guesses the other's state from symptoms.
- **Inputs:** [stalls-and-load-2026-11-16.md](stalls-and-load-2026-11-16.md) (other servers and
  players, CMSD and CMCD, measuring speed), [stall-logs-2026-11-16.md](stall-logs-2026-11-16.md)
  (what production and staging show), [morning-questions-2026-11-16.md](morning-questions-2026-11-16.md)
  §1 (what a stall is today). Code read: `web/packages/player/src/stalls.ts`, `chunked.ts`,
  `abr-ceiling.ts`, `abr-on-its-way.ts`, `abr-busy.ts`, `abr-video.ts`, `abr-choice.ts`,
  `standstill.ts`; `web/apps/web/src/play-state.ts`, `moving-down.ts`, `ladder-playback.ts`, the
  locale strings; `internal/core/chunks/capacity.go`, `failures.go`; `internal/core/playback/cpu.go`,
  `chunks.go`; `internal/core/api/pacing.go`, `chunks.go`, `playback.go`;
  `internal/node/encode/encoder.go`; the docs of `internal/node/diskschedule`, `diskwatch`,
  `internal/core/resourceuse`. Decisions D247, D250, D253, D266, D286, D309, D324, D352, D371,
  D382, D457, D488, D493 in [working-notes.md](../../decisions/working-notes.md).
- **The design is the owner's** (AGENTS.md rule 1). Every response, field and recommendation below
  is the author's proposal, marked as one, for the owner to accept, change or drop. Scenario ids
  (N, K, V, G, X, E) are this document's own.

## What acts on a stall today

Nine controllers can change what a viewer gets. Each scenario below names which of them fire.

| Id | Controller | Where | Acts on | Hears the core's waits? |
| --- | --- | --- | --- | --- |
| P1 | The bitrate controller's next-chunk choice and ceiling | `QualityCeiling` (`abr-ceiling.ts`), fed by `tellStalls` (`chunked.ts`) | link estimate; a stall (`stalled()`: lowers the ceiling while starting, drops steadiness, D352 §4) | slow loads: no (`isAwaitingTheCore`); a mid-play rebuffer: **yes**, `waitsWhilePlaying` (`stalls.ts`) checks nothing |
| P2 | D247/D253's move down | `MovingDown` (`moving-down.ts`), fed by `watchForReport` (`play-state.ts`); the core's `offerFor` (`internal/core/playback/report.go`), off-ladder playbacks only | stalls | **yes**: `watchForReport` passes no `LoadWaits` |
| P3 | D266's give-up of a quality the device cannot play | `StandingStill` (`abr-video.ts`), `isUnplayable` (`abr-choice.ts`); kept per device by the core (D493) | position still with ≥ 1 s loaded ahead | no (needs data in hand), but sees a film's end (V4) |
| P4 | D247 §8's "cannot keep up" | `followChunkedPlayback` (`ladder-playback.ts`), `beyondTheLink` (`chunked.ts`) | a stall at the lowest rung, link unmeasured or under 1.5× | **yes**: no `LoadWaits`, and "unmeasured" is the state of a start held by 425s |
| P5 | D488's data cap | `topLevel` / `dataCapTop` (`abr-ceiling.ts`) | the device's setting | n/a |
| C1 | D250 §6's speed watch | `keepRunsFast` (`internal/core/chunks/capacity.go`) | a run under `chunkRunSlowestSpeedPercent` (110 %), **only with two or more runs** | its own |
| C2 | D309 §6's move down for the processor | `MoveOffStream` (`internal/core/playback/cpu.go`): every watching playback of the stream gets `cpuCeilingKbps` for `chunkCPUCeilingKeptSeconds` (120 s) | C1's verdict | its own |
| C3 | D286 §5's upload ceiling | `ceilingKbps`, merged with C2 by the lower of the two in `CeilingOf` (`internal/core/playback/chunks.go`) into one `Orpheum-Ceiling-Kbps` header | the core's counted upload against the admin's figure | n/a |
| C4 | D324 §5's hold-back of other work | `holdBackFor` (`capacity.go`), **software encoders only** | a played encode behind | its own |

Two more paths refuse rather than lower: two runs failing at one chunk give `ErrCannotBeMade`
(`internal/core/chunks/failures.go`), answered 422 `cannotConvert`; an evicted run gives
`ErrNoRoomToMake`, 429 `noRoomToMake` (`internal/core/api/chunks.go`). D457's start-up trial refuses
an encode on a machine too slow for any (`tooSlowToConvert`).

Three facts about the wire that recur below:

- **The ceiling header has no reason.** C2 (processor) and C3 (upload) arrive as one number; the
  player cannot say which, and cannot know when it lifts (C2 lapses on a 120 s timer it never sees).
- **The player reads the ceiling only from a 200** (`requestSetupOf`, `chunked.ts`: `answered`
  checks `status === httpOk`), though `countedMedia` (`pacing.go`) sets it on every chunk answer, so
  a player held by 425s learns nothing of the core until a chunk arrives.
- **A 425 says nothing but `Retry-After: 1`** (`tooEarly`, `internal/core/api/playback.go`): not
  whether the chunk is a second away or queued behind three runs.

## Scenarios

"Observed" is what each side can see today (player; core). "Information needed" names the fields of
the contract below; `mb`, `bl`, `mtp`, `dl`, `su`, `bs` are standard CMSD/CMCD keys, `dev.orpheum-*`
and named fields are Orpheum's own.

| Scenario | Observed | Today | Right response (the author's) | Information needed | Conflation risk |
| --- | --- | --- | --- | --- | --- |
| **N1** Slow link, steady, below the top rung | Player: chunks answered 200 at once, slow transfer, buffer draining. Core: runs ahead, bytes leave slowly | P1 lowers the next chunk and, starting, the ceiling; P2 for an off-ladder film. Works | Player steps down (D247, D352); the core only makes the lower rung | To core: `mtp`, `bl`, ceiling reason `link` (so its log and upload accounting know whose it is) | If the lower rung is an encode, the step adds core load (K9, X2) |
| **N2** A link that dips briefly | Player: one or two slow chunks, buffer holds. Core: nothing | `holdsQualityAheadS` (`abr-ceiling.ts`) keeps the quality while enough is loaded; steadiness grows | Hold; no step on either side | None new | A core that eased on a CMCD `bs` or a low `bl` would answer a link dip by cheapening everyone's picture: the core must never ease on the player's buffer alone |
| **N3** Congested house upload, viewer outside (WebRTC) | Player: slow, jittery transfer. Core: counts only its own bytes (`countedMedia`), not the house's other traffic, so C3 stays off unless the admin's figure is low | Each remote player steps down alone; several may each choose a different lower rung, several encodes | Players step down (it is the network); when two or more remote players step down for the link at once, the core consolidates them onto one rung (D286 §5), as the arbiter of what it shares | To core: `mtp` and ceiling reason per player; to player: `mb` with reason `upload` | One viewer's bad last mile reads like house congestion unless the core sees *several* `mtp` fall together; consolidating on one viewer's evidence lowers everyone |
| **N4** Wi-Fi on a TV | Player: bursty transfer, losses, long tasks, maybe dropped frames. Core: bursty sends | `BusyPage` (`abr-busy.ts`) excuses samples overlapping a long task (D382 §3); P3 may judge a rung unplayable | Link controller on clean transfer samples only; the device's verdict kept apart (V1, V2) | Player-local: long tasks, dropped frames. To core: verdict and link figure, each labelled | A TV browser that reports no long tasks excuses nothing (`abr-busy.ts`): a slow page read is a slow link |
| **N5** Data cap (D488) | Player: chooses under the link. Core: a step that adds an encode (staging, 11-15 08:38Z, film 4443, a copy to `q400k360p`, new 425s on its init and chunk 9) | P5 caps the top; the step's 425 waits are counted by `watchForReport` past 8 s and fed to P2 | The cap is the player's own ceiling, reason `dataCap`; the core admits the encode as any run (K9); the step's waits are the core's | To core: ceiling reason `dataCap`; to player: on the 425, `dev.orpheum-eta` | The step's own first-chunk wait counted as the link's stall: a further move down below the cap |
| **K1** One transcode too slow for its rung, alone (under 110 %) | Player: every chunk 425 longer than it plays, then mid-play rebuffers. Core: one run under 110 % | C1 returns with fewer than two runs: **nothing acts on the core side**. P1 counts each rebuffer (`waitsWhilePlaying` has no core filter): lowers the ceiling while starting, drops steadiness; P2 counts them too | The core eases the recipe first (a faster preset of the same encoder), then a cheaper rung, said as `mb` reason `cpu`; the player's link estimate untouched | Per chunk: `dev.orpheum-mk`, `-rt`, `-enc`, `-st=behind`; `mb` + `-mbr=cpu`. To core: `bl`, `dl` | The player answers a slow encoder with the network's response, and with both sides acting the drop is doubled (X3) |
| **K2** 1080p at 2.3× real time with a 7 s first chunk (ainas, measured) | Player: a start or seek held by 425s 3.6 s at the median, 7.7 s at worst; then steady. Core: run speed fine, first chunk slow | `watchForReport` counted the 21:08Z start as a stall (position 0, steps `fragLoadError`); `tellStalls` did not; the D266 note blamed the link | Not a quality problem: a start or seek wait, told as the core's. Whether to answer it (start on a rung already made, a cheaper first chunk) is open decision 8 | On the 425: `dev.orpheum-eta`, `-fr`, `-q`. To core: `su` (start or seek urgency) so the run is put first | A move down for it lands on a rung that needs its own fresh encoder: as late, or later (stalls-and-load §2 pitfall) |
| **K3** Several transcodes at once | Player: each run's 425s lengthen. Core: two or more runs, one under 110 % | C4 (software only), then C1: cancel a run no one plays, else move the newest's players down (C2, 120 s), else evict it (429) | Ease the recipe of the runs first (the one whose players hold the most buffer first, by `bl`), then refuse a new run at the cap (decision 4), then move onto a rung already made (D286 §4), then evict | Core state across playbacks: run count, cap, per-run speed and recipe. To player: `mb` + reason, refusal code | The newest run is always the victim, and the newest is often a network step-down's (X2, K11) |
| **K4** A seek's first chunk (fresh encoder), any rung | Player: 425s, then fast. Core: first chunk slow (a copy's slowest 4.8 s on ainas) | The first chunk is kept out of the speed window (`capacity.go`) and D353 judges a jump by the store's last first chunk: right. `watchForReport` counts past 8 s | Same as K2: the core's wait, never in any quality decision | `dev.orpheum-fr`, `-eta` | Counted as a slow seek of the link by P2 and P4 |
| **K5** Disk spin-up | Player: one chunk seconds late. Core: the read waited in the disk scheduler (D329), not logged | Nothing tells it apart; its time joins the store's start-up | No quality response, no easing: a one-off. Logged once; kept out of every speed window (stalls-and-load §4 option 3) | `dev.orpheum-rd` on that chunk | Judged as encoder slowness: a preset eased for a disk, or a move down |
| **K6** A hardware encoder failing, falling back to software | Player: 425s, then 422. Core: FFmpeg exits with the device's error | The node uses the encoder its start-up chose (the hello's `chosenEncoder`); no fallback per run was found. Two failures at one chunk give `ErrCannotBeMade`, shown with `cannotConvert`'s text: *"it has no picture, or nothing to cut it into pieces by"*, which is false here | Per the owner, "an easier codec": the next usable encoder (`Choice.Usable`, `encoder.go`), x264 last, with its lower speed counted against the node; a cheaper rung if software cannot keep the rung; the admin warned | Node to core: the failure's kind (encoder or device vs source). To player: `-enc`, `mb` + `-mbr=encoder` | A source fault retried on software takes twice as long to its honest refusal; a GPU fault is told to the viewer as a broken file |
| **K7** NVENC or Quick Sync session limits | Player: as K6. Core: the thirteenth NVENC session on a GeForce fails at once (NVIDIA's matrix, stalls-and-load §1); Quick Sync and VAAPI have no cap, only throughput | As K6: two failures and the film "cannot be converted" | The node's session limit is a hard floor under the admin's cap (decision 4): the run beyond it is queued, or put on software only if software keeps up | Node to core: sessions in use and the limit; failure kind `noSession` | A capacity refusal told as a broken film; or a fallback to software that slows every software run (Emby's pitfall, stalls-and-load §1) |
| **K8** The machine loaded by something else (a backup, a scrub, another container) | Player: as K1 or K3. Core: every run slower than D457's trial predicts while the node's own CPU is modest | C1 treats it as its own overload (cancel, move down, evict); C4 stops Orpheum's own background work, software only; `resourceuse` records the processes' CPU per minute, not the machine's pressure; nothing logs "loaded by something else" | Warn the admin, rate-limited (below), naming what was seen; ease recipes so the viewer keeps playing; refuse no more than the measured room requires | Node: PSI for the machine and its own cgroup (`/proc/pressure/cpu`, `io`; cgroup `cpu.pressure`), expected speed (trial, run count) against measured | Orpheum moves its own viewers down to make room for work it cannot see, and the admin reads "too many viewers"; a scrub loads I/O, which looks like K5, not K8, unless `io` pressure is read |
| **K9** A remux that a step-down turns into a transcode (staging, film 4443) | Player: the step causes 425s. Core: a copy (median 154×) replaced by an encode | Allowed without asking; no admission | The core admits the encode if the node has room; otherwise answers with the lowest rung already made, or says it cannot (decision 13) | To core: the step's reason and target; to player: `-eta`, `mb` | Feeds X2 |
| **K10** Two viewers on one file and rung (a shared encode) | Both see the same 425s. Core: one run, two interests | C2 moves every watching playback of the stream (`MoveOffStream`) | Count runs, not viewers, for every cap; a viewer asking for a rung already being made is never refused for load (stalls-and-load §5); easing a shared run's recipe changes both pictures, which is fair | Core log: viewers per run | Low, until one of them steps off the shared rung (K11) |
| **K11** One viewer's step-down changing another's load | A steps down for their link, a new run starts; B's run falls under 110 % | C1 evicts the newest: A's new run (A moved further down or refused); B is spared only by luck of age | The core arbitrates making: A's step lands on a rung already made (D286 §4), or is admitted within room; B's run never pays for A's link | To core, before loading: the rung A wants and why (`link`), so the core can answer "that one is made" | B's playback eased or moved for A's network: the owner's "not for the same reason" broken across viewers |
| **V1** A TV's decoder that cannot play the rung | Player: stands still with buffer, chunks arriving. Core: chunks sent | P3 gives it up; D493 keeps the give-up on the core for the device | Right as built: the device's verdict | To core: the verdict with its evidence (`chunksLoaded`, `loadedAheadS`, frames) | The verdict is now kept per device, so a wrong one (V2, V4) lasts beyond the playback |
| **V2** JASSUB subtitles eating the CPU | Player: long tasks, dropped frames, maybe standing still with buffer. Core: nothing | `BusyPage` excuses link samples; P3 may give up the rung | Ease the device: a lighter subtitle renderer (morning-questions §3) before any rung change; never told as the link's or the core's | Player-local: renderer in use, long tasks, drop rate; to the report: `whoseWait=device` with cause | P3 gives up the top rung for a subtitle renderer's cost and keeps it per device (D493) |
| **V3** A backgrounded or hidden tab | Player: timers throttled (browser background timer clamping; its exact limits **UNVERIFIED** here); `Retry-After: 1` polled late. Core: asks slow down. Logs: device 34 began playing hidden; device 49's hidden tab had 60 s timeouts | D287 §4 pauses a hidden page's film; a hidden page that plays anyway (as device 34's) has its waits timed by late timers | No stall counted and no step taken on samples from a hidden page; the core lowers the run's priority (D324) | To core: `hidden` beside the pause the wants already carry | A late timer reads as a long load: a stall, a move down when the person comes back |
| **V4** A film's end looking like a standstill | Player: position still, nothing ahead, no chunk arrives. Logs: 8 of 9 "stood still … the link's to bring" notes at episodes' ends (films 1739–1746) | `StandingStill.look` tests `isWaitingToPlay` alone; its `Watched` type has no `ended`; the note blames the link | The end excluded everywhere, by one primitive: asked to play, position not moving, not at the end | None across; player-local `ended` and end of stream | Today only a note; if it ever reaches P2 or P3, a film's end gives up a rung or blames the link |
| **V5** A cast receiver | Receiver: a weak CPU, usually a fast LAN link. Sender: remote control (D371 §4); the leader's cast is the leader | The receiver runs its own player stack; `cannotKeepUp` from it reaches the sender (`cast-receiver.tsx`) | The receiver decides its link and device like any player; the sender shows its labelled state | Receiver to core: the same fields as any player | A receiver's CPU stall (V2-like) told as the link's; whether its browser reports long tasks is **UNVERIFIED** |
| **G1** Watch Together, one slow member | Follower falls behind the leader and seeks to catch up (`fellBehindEvent`). Core: the session's positions | `fellBehindEvent` is a stall to every `watchStalls` listener (P1, P2, P4); D371: no one waits; `together-news.tsx` tells the group "X's connection carries about …" | Find whose it is first: the member's link (the member steps down, or the group's rule applies, decision 10); the member's device (its own verdict); the core making the member's rung (the member left the shared rung, K11) | Per member: stall kind and whose wait, `mtp`, `bl`, device verdict; the core sums them for the session | The group told it is X's connection when it is the core or X's device; or the whole group lowered for one member |
| **G2** The host's core slow for everyone | The leader and everyone on the common rung wait on the same 425s | The leader's waits are counted by `watchForReport` (P2) and, at the lowest rung, P4 says the leader's connection cannot keep up (`cannotKeepUpUnmeasured`) | The core eases the common rung's recipe; the group told the server is converting slower than the film plays, naming no member | To all members: `mb` + `-mbr=cpu`, `-st`; the session's channel carries the server's state | The leader blamed by name for the core |
| **X1** A slow core and a slow link at once | A chunk waits 425, then transfers slowly | `ChunkOnItsWay` waits out the 425 (`isAwaitingTheCore`) and samples the transfer alone: right. A mid-play rebuffer is the link's to P1 whatever caused it | Each side acts on its own resource only: the core on make time, the player on transfer time; neither on the total | Per chunk, separately: `dev.orpheum-mk` (core) and the player's own transfer time | When neither part alone is too slow but the sum is (make 2 s and send 3 s of a 3.8 s chunk), no rule says who acts (unresolved 1) |
| **X2** A step-down for the network that raises core load until the core is the bottleneck | Link step to an encode; the node falls under 110 %; that run, the newest, is moved down again (C2) or evicted; 120 s later C2 lapses and the player may climb | As described: C1, C2's timer, P1's climb (30–240 s hold, D352) | Admission (decision 13): the core decides whether the step's encode fits; a core ceiling lifts when the core's state does, not on a timer | `mb` with reason, and its lifting said; `-eta` so the step's cost is known | A cycle of about 120 s plus the climb's hold, each turn a fresh encoder's wait |
| **X3** The core easing its preset while the player steps down (two controllers on one symptom) | A run behind; the buffer drains | No preset easing exists yet; today the pair is C2 and P1: the same slow run lowers the core's ceiling and, through rebuffers, the player's | One cause, one actor: while a chunk says `-st=behind` or `easing`, the player's link controller holds and does not count the waits; the core's ceiling is a ceiling, never a sample | `dev.orpheum-st` on every chunk and 425 | Overshoot to the lowest rung, and a rung change that costs a fresh encoder, later than the eased run |
| **X4** The upload ceiling and the processor ceiling in one header | The player obeys a number (`obeyServer`, `abr-ceiling.ts`), read only from a 200 | `CeilingOf` takes the lower of C2 and C3 | Separate reasons; the player reads the ceiling from 425s too | `mb` + `-mbr`; read on every answer | The player cannot say "for the server" or "for the upload" to the person, nor know when it lifts |
| **E1** A player reading at the encoder's front (staging, device 139, film 207) | Each chunk asked as the run made it; no stall, no margin | Nothing: no field shows the margin | The core, knowing its lead and the player's buffer, puts that run first before a stall comes | To core: `bl`; to player: `dev.orpheum-ah` | None until a hiccup, then K1 |
| **E2** Duplicate starts (one viewer, two playbacks) | 26 of production's 42 leaves were "replaced" | Interest per playback | Caps count runs, not playbacks or viewers | None | A cap counting viewers counts one person twice |
| **E3** A chunk evicted under disk pressure (ainas at 96 %) and made again | A seek back to a place played before waits 425 | As K4 | As K4 | `dev.orpheum-fr` | A seek back blamed on the link |
| **E4** A network share gone (D233 §10, `diskwatch`) | Every read of the root answered stalled | Not traced on the chunk path here (**UNVERIFIED** whether the player gets 425s for ever or `mediaUnreachable`) | Refuse as `mediaUnreachable`, never "being made" | Refusal code | A dead share waited on as a busy core |

## The hardest to tell apart

Each pair looks the same from at least one side today; the field that separates them is named.

1. **A run too slow for its recipe (K1), a machine loaded by something else (K8), and a disk
   spinning up or scrubbing (K5).** All three make chunks late; the player cannot tell them apart and
   should not try. The core can only with the node's PSI (`cpu` against `io`, machine against its
   own cgroup), the disk scheduler's read waits per chunk (`dev.orpheum-rd`) and the trial's
   expected speed. Without them the core eases a preset for a disk, or moves its viewers down for a
   backup.
2. **A mid-play rebuffer for the core (K1) and one for the link (N1).** Today both are the link's to
   P1 and P2, because `waitsWhilePlaying` (`stalls.ts`) has no `isAwaitingTheCore`. The separating
   field is per chunk: whether the chunk the buffer ran dry on waited for the core (a 425 and
   `dev.orpheum-mk`), against its transfer time.
3. **A device's standstill (V1) against JASSUB's CPU (V2), a hidden page (V3) and a film's end
   (V4).** All stand still; only V1 is the rung's. Player-local only: the subtitle renderer in use and
   long tasks, visibility, `ended`. D493 makes a wrong verdict last.
4. **A hardware encoder failing (K6), a session limit (K7) and a source FFmpeg cannot read.** All
   end at `ErrCannotBeMade` and the `cannotConvert` text. Separating them needs the failure's kind
   from the node, which it does not send today.
5. **In a group, a member's link, a member's device, and the core making the member's rung (G1).**
   The group sees one thing, a member behind. Only the member's typed stall (whose wait) and the
   core's knowledge of which run the member reads from separate them.
6. **A start held by the core (K2, K4) and a start the link cannot carry.** At the lowest rung with
   no link measured yet, `beyondTheLink` says "cannot keep up" for both, and the viewer is told
   their connection is too slow (`cannotKeepUpUnmeasured`). The 425 and `dev.orpheum-eta` separate
   them.

## Two controllers on one symptom

| Where | The two | Symptom | Proposed arbiter (the author's) |
| --- | --- | --- | --- |
| F1 | P1 (`tellStalls`) and P2 (`watchForReport`) | one stall, filtered differently | One typed stall event (morning-questions §1, option 1); each listener takes the kinds that are its own |
| F2 | P1's rebuffer response and C1/C2 | a run behind (K1, K3, X3) | The core: the chunk says `-st=behind`; the player's link controller holds |
| F3 | P1's climb and C2's 120 s timer | the ceiling lapsing (X2) | The core: its ceiling lifts when it says so |
| F4 | C3 and P1's link estimate | the upload (N3) | Both, by the lower of the two, as now, but with C3's reason said |
| F5 | P4, P2 and the core's 429 | a hopeless start | The cause decides the message: the link's (P4), the core's (refusal code) |
| F6 | P1 per member and the group's common rung (D286 §3) | a member behind (G1) | The core, by the group's rule (decision 10) |
| F7 | P5 (data cap) and C1 | a step to an encode (N5, K9) | The core admits the encode; the player keeps its cap |
| F8 | P3 and P1 | standing still (V1, N4) | The player alone (already separated by `linkFailed`, `abr-video.ts`) |

## The arbiter question

**Options.**

1. **One arbiter, the core.** The player sends CMCD state; the core chooses every player's rung.
   It sees all players and every run; but it would decide the link from reports a few seconds old,
   and hls.js's controller would have to be replaced by a follower.
2. **One arbiter, the player.** The core sends CMSD state; each player chooses. Simple, but no
   player sees the others: K11, N3 and G1 need a view across playbacks.
3. **Two arbiters, split by resource, with one rule of composition** (the author's recommendation).

**The proposal (the author's).**

- **The core arbitrates what is shared:** the processor, encoder sessions, the disk, its upload,
  and a Watch Together session's common rung. It acts in the owner's order: the recipe first (a
  faster preset, then the next usable encoder), then admission (the cap, a run refused or queued),
  then a lower rung, onto one already made where there is one (D286 §4), said as a ceiling with its
  reason. It never acts on a player's buffer alone: `bl` and `dl` set *urgency* (which run goes
  first, which is eased first, the disk scheduler's deadlines), never *quality*.
- **The player arbitrates what is its own:** its link, its device's decoder and CPU, its data cap.
  It chooses its rung as the lowest of four ceilings, each with its own owner and its own rule for
  lifting: the link's (D352's climb), the cap's (the setting), the device's (D266, D493), and the
  core's (`mb`, lifted when the core says so).
- **Each side is blind to the other's waits by construction.** The player's link controller takes
  no sample, no stall and no step from a wait the chunk says was the core's; the core takes no
  quality decision from a symptom the player says was its link or device.
- **On a symptom both see** (a buffer draining while a run is behind) **the core moves first**,
  because it knows at the moment it answers the chunk. The player holds its link controller while
  any answer says `-st=behind` or `easing`, and for one speed window after (`chunkRunSpeedWindowSeconds`);
  if the buffer still drains with clean transfer samples below the rung, the link is at fault too,
  and the player steps.
- **In a group, the core arbitrates the group's rung and who is left behind,** by the group's rule;
  a member decides alone only within it. A wait that is the core's is told as the server's, never
  under a member's name.

## The information contract

### Core to player: on every chunk answer, 200 and 425

Carried in `CMSD-Dynamic` (the standard's envelope, which a standards-reading player understands for
`mb`), on both the HTTP and the WebRTC path (the relay keeps response headers; Server-Timing is not
reliable there, stalls-and-load §3). The player must read it from a 425 as well as a 200.

| Key | Standard? | Unit | Meaning | When |
| --- | --- | --- | --- | --- |
| `mb` | CMSD | kb/s | The highest rate the core lets this playback load, its picture's average with its sound's (today's `Orpheum-Ceiling-Kbps`) | while any core ceiling stands |
| `dev.orpheum-mbr` | own | token: `cpu`, `upload`, `encoder`, `group` | Why `mb` stands | with `mb` |
| `dev.orpheum-st` | own | token: `ok`, `behind`, `easing`, `capped` | The state of the run this chunk is from | every answer |
| `dev.orpheum-eta` | own | ms | On a 425: how long until this chunk is expected (the run's front, its speed, the queue); `Retry-After` set from it, at least 1 s. CMSD-Static's `at` (an epoch time) fits less well, needing the player's clock | every 425 |
| `dev.orpheum-mk` | own | ms | Wall time the node took to make this chunk, without queue or disk waits | every 200 of a made chunk |
| `dev.orpheum-q` | own | ms | Time this chunk's run waited before it began (queued, held back) | 200 of a run's first chunk; 425 |
| `dev.orpheum-rd` | own | ms | Time this chunk's source reads waited on the disk scheduler | every 200, when above zero |
| `dev.orpheum-rt` | own | ratio, film seconds per wall second | The run's speed over `chunkRunSpeedWindowSeconds`; absent while unmeasured | every answer |
| `dev.orpheum-fr` | own | flag | First chunk of its run: a fresh encoder's start | when true |
| `dev.orpheum-enc` | own | token: `copy`, `x264-<preset>`, `qsv`, `nvenc-<preset>`, `vaapi` | The recipe this chunk was made with | every 200 |
| `dev.orpheum-ah` | own | s | How far the run has made past this chunk | every answer |
| `du` | CMSD | flag | Duress | **not sent**: it means "go to another server", and there is none (stalls-and-load §3) |
| `etp` | CMSD | kb/s | The server's view of throughput | **not sent**: through a relay it misleads (stalls-and-load §3) |

The core logs the same per chunk at debug, and per run at info (stall-logs, missing fields 5–9).

### Core to player: on the playback channel, when it changes

A refusal's cause code (below); the session's state for a group (the server behind, naming no
member); the lifting of a core ceiling.

### Player to core: on the wants, when they change, and with each stall

Named fields in `PUT /playback/{token}/wants`, which already carries position and pause, rather
than CMCD on every request (stalls-and-load §3: CMCD in the query defeats a URL-keyed cache; headers
need a preflight across origins). Sent on a change beyond a threshold and at least every few
seconds while playing (the threshold and the period are the owner's to set).

| Field | Standard? | Unit | Meaning |
| --- | --- | --- | --- |
| `bl` | CMCD | ms | Picture buffered ahead of the playhead |
| `dl` | CMCD | ms | Deadline of the next chunk: when the buffer runs dry at this rate |
| `mtp` | CMCD | kb/s | The link's estimate from transfer samples only |
| `su` | CMCD | flag | Starting or seeking: the next chunk is urgent |
| `bs`, `bsd` | CMCD (v2 for `bsd`) | flag, ms | The buffer ran dry, and for how long |
| `ceilingWhy` | own | token: `link`, `dataCap`, `device`, `core`, `group` | Why the player stands below its top |
| `wantedRung` | own | stream name | The rung a step is about to ask for, so the core can answer "this one is made" (K11) |
| `hidden` | own | flag | The page is hidden |
| `deviceBusy` | own | ms of long tasks per 10 s; dropped frames per second | The device's own load |
| `subtitleRenderer` | own | token | The renderer drawing subtitles (V2) |

With each stall report: its **kind** (rebuffer, slow start, slow seek, fell behind), **whose wait**
(link, core, device, paused, group), its **length**, the **position being loaded** (not the one before
a resume seek), and the chunk, stream and status waited on (stall-logs, missing fields 1–4, 11).

## Refusals and messages, per cause

| Cause | Code today | What the viewer sees today | Proposed (the author's) |
| --- | --- | --- | --- |
| Too slow to encode at all (D457) | `tooSlowToConvert` | "This server can't convert this film fast enough to play it here. A device that plays the film as it is can." | Keep |
| Behind, run evicted (D250 §6) | `noRoomToMake` (429) | "The server cannot make this quality as fast as it plays while it makes others for other viewers. Try again in a while." | Keep, and offer a rung already made where there is one before refusing |
| At the admin's cap (new) | none | none | A code of its own: the server is converting as many films as its admin allows; it plays when one ends |
| FFmpeg failed twice on the source | `cannotConvert` (422) | "This file can't be converted for this device: it has no picture, or nothing to cut it into pieces by." (false for this cause) | Its own text: the server's converter failed on this film each time it tried |
| Hardware encoder failed | as above | as above | Never shown before the software fallback has been tried; then as "behind" or "too slow" |
| Encoder sessions used up | as above | as above | The cap's message |
| Media unreachable (a share gone) | `mediaUnreachable` exists | not traced (E4) | That code |
| The link cannot carry the lowest rung | P4's `cannotKeepUp` / `cannotKeepUpUnmeasured` | "Your connection carries about …" | Only on a wait that was the link's |
| The core cannot keep even the lowest rung | none: told as the connection's | "Even the lowest quality … is too much for this connection" | Its own: the server converts slower than the film plays here |
| A lower quality for the core | none: `lowerQuality` says "for your connection" | "Playing a lower quality for your connection …" | Said by `mb`'s reason: for the server, for the upload, for your data cap |
| In a group | `together.cannotKeepUp` names the member | "{{name}}'s connection carries about …" | Name the member only for the member's link or device; the server's state otherwise |

## Admin warnings

Each is logged at warn and shown on the System page as a current episode. **Rate limit (the
author's proposal):** one line when an episode starts and one when it ends, with its length and how
often it fired meanwhile; at most one start per cause per node in 15 minutes; the per-chunk detail at
debug.

| Cause | Seen by | Says |
| --- | --- | --- |
| A played run behind | C1 | the run, its recipe, its speed and the others' |
| A recipe eased | the core | from what to what, for which run |
| A viewer refused at the cap | the core | the cap, the runs it counted |
| A hardware encoder failed, software took over | the node | the encoder, FFmpeg's word, the speed now |
| Encoder sessions used up | the node | the limit |
| Loaded by something else | the node's PSI against its own cgroup, speed against the trial's | which pressure (`cpu`, `io`), how far below expected |
| Disk waits over a few seconds | the disk scheduler | the device, the wait |

## Open decisions for the owner

1. **Who arbitrates.** (a) the core alone; (b) the player alone; (c) split by resource: the core what
   is shared, the player its own link and device, composed by the lowest ceiling. **Recommended
   (the author's): (c).**
2. **Do D247's move down (P2) and "cannot keep up" (P4) see the core's waits?** (a) no: typed stalls
   with whose wait, each listener taking its own; (b) yes, as today. **Recommended: (a)**, which
   morning-questions §1 option 1 builds.
3. **The core's order of easing.** (a) recipe, then admission, then a lower rung, then eviction; (b)
   a lower rung, then eviction (today); (c) recipe, then refusal, never a rung for the core.
   **Recommended: (a)**, the lower rung only onto one already made where possible. Cost: chunks of
   one store made with different presets must still splice (same rung, same keyframes).
4. **A cap on encodes at once.** (a) none (today); (b) an admin setting counting encode runs (never
   copies, never viewers), with the node's encoder session limit as a hard floor; (c) weighted by a
   rung's cost; (d) worked out from measured speed. **Recommended: (b)**, its default from D457's
   trial; (c) later if a 1080p and a 360p run counted as one proves wrong.
5. **How long a core ceiling lasts.** (a) a timer, 120 s (today); (b) until the core's state lifts,
   with hysteresis, said to the player. **Recommended: (b)**; it ends X2's cycle.
6. **How the core tells the player.** (a) `CMSD-Dynamic` with `mb` and prefixed keys, per chunk;
   (b) Server-Timing; (c) the playback channel alone. **Recommended: (a) per chunk, (c) for state
   changes and refusals.** A header parser is a few lines or the SVTA library, a new dependency for
   the owner (rule 3).
7. **How the player tells the core.** (a) named fields on the wants; (b) CMCD headers per request,
   which hls.js can send. **Recommended: (a).**
8. **A start or seek into a slow first chunk (K2).** (a) accept it, told as the core's; (b) start or
   seek on a rung already made, or a cheaper first chunk, then climb; (c) a shorter first chunk.
   **Recommended: (a) now; (b) if the owner wants the 7 s gone**, which changes the recipe inside a
   store.
9. **A hardware encoder's failure.** (a) fall back to software for that run, counted against room,
   with a warning; (b) refuse after two (today); (c) fall back only when software's measured speed
   keeps the rung, else a cheaper rung. **Recommended: (c).** It needs the node to tell an encoder's
   failure from the source's.
10. **A slow member in Watch Together.** (a) leave behind always (D371 today); (b) the group steps
    down to a common rung the member carries; (c) the group's own setting, defaulting to (a).
    **Recommended: (c)**, as the owner's "per the group"; the core's own slowness is never a
    member's.
11. **The upload ceiling (D286 §5).** It is the core shaping the network, which the direction
    reserves for the processor. (a) keep it, reason `upload`; (b) drop it and leave the upload to
    each player. **Recommended: (a)**: it is the one place a view across players is needed for a
    network (N3).
12. **"Loaded by something else".** (a) PSI, the machine against the node's cgroup; (b) measured
    speed against the trial's; (c) both. **Recommended: (c)**, warning only, never a refusal on it
    alone.
13. **A step-down that turns a copy into an encode on a busy core (K9, X2).** (a) allow (today); (b)
    the core admits it only with room, else offers the lowest rung already made, else says it cannot
    and the player keeps its rung and its stalls; (c) forbid. **Recommended: (b).**

## Unresolved edge cases

1. **The shared budget (X1).** Make time and transfer time each below a chunk's length, their sum
   above. Neither arbiter is wrong alone; the proposal has no rule for who gives way.
2. **Easing a shared run** changes the picture of every viewer on it, including one whose link and
   device were fine. Fair, but untested with viewers who notice.
3. **Mixed recipes in one store.** A store whose chunks change preset or encoder mid-film (a VAAPI
   chunk beside an x264 one) may change the stream's profile or parameter sets; whether hls.js and
   each browser's MSE switch without a reset is **UNVERIFIED**.
4. **Hidden pages' timers.** How far each browser delays a hidden page's timers, and so how wrong a
   wait's length is, was not measured.
5. **The make time's clock.** Measured on the node and sent by the core; across machines it is a
   duration, so only the node's own monotonic clock may produce it.
6. **Paced downloads** (D262 §2) read as a link at the pace; whether the estimator already discounts
   them was not checked.
7. **A cast receiver's reports** go through its own stack; whether the sender's page shows the reason
   labels the receiver sends is not designed.
8. **Thresholds.** What share of a chunk's length the make time must take for a wait to be the core's,
   and when `-st` turns `behind`, are left to the owner; the logs hold one stall to set them by.
9. **Logs lost at each redeploy** (stall-logs, missing field 12): none of this can be measured over
   days until the logs outlive a redeploy.
10. **A stall at position 0 after a resume** (stall-logs §4) must be fixed before any field above
    joins a stall to a run.

## Not tried

- Nothing was run or measured; every "today" is read from the code at the commit named above, and
  every number is from the two research documents of the same day.
- No cast receiver, Watch Together session, NVENC or Quick Sync machine, or software-only node was
  exercised; K6, K7, G1, G2 and V5 are read from the code alone.
- The chunk path's answer when a share is gone (E4) was not traced.
- CMSD and CMCD units are as stalls-and-load §3 quotes the SVTA library; the standards' own texts
  were not re-read here.
