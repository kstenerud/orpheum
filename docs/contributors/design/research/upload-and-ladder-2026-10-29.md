> **ABOUTME:** Research for the SWAG audit's groups 2 and 3: the eleven values that share a home's
> upload between players (crowding, raising, sending a Watch Together member away), and the
> quality ladder and encoder presets. What Plex, Jellyfin, Emby, Channels DVR, Apple, Netflix,
> YouTube, Bitmovin, Mux and the research on players sharing a bottleneck say; a recommended value,
> a confidence and a test for each; a simulation and shaped-upload test plan; and a VMAF
> measurement plan to run later on the GPU test machine. Proposals only. 2026-10-29.

# Sharing the upload, and the ladder and its encoder

- **Date:** 2026-10-29, from the code at an earlier commit.
- **Asked by** the owner: *"Go ahead and schedule the research so it can run while we test."* It
  follows [swag-values-audit-2026-10-29.md](swag-values-audit-2026-10-29.md) §2 and §3.
- **Read first:** [upload-budget.md](../plans/upload-budget.md) (D286 §4, §5), D265, D250 §6,
  D247 §6, D323 §3, D285 §3, D344, D368, [slow-links.md](slow-links.md) §4 and §5.
- **Nothing was run.** No encode, no shaped link and no simulation: the owner is testing on
  staging, which shares this machine. §6 and §7 are plans to run later.
- **Films are named only by file id** throughout, and the measurement plan records only ids.
- **Sources** were opened and read unless marked *(secondary)*, which means a search summary or a
  blog repeating someone else's numbers. Notes and cached copies are in
  `/yoloai/cache/research-groups-2-3/` and `/yoloai/cache/abr-research/`.

## Summary

**Sharing the upload (group 2).** The design matches the best research result: the server works
out each player's quality and tells it a ceiling. The research calls this *bitrate guidance*, and
Cofano et al. (MMSys 2016) found it the fairest and steadiest way to share a bottleneck. No
competitor does as much. Plex, Jellyfin and Emby give a per-stream bitrate cap (Plex adds a total
upload figure), and none of them shares a total between streams or sends anyone away. The values
around our design are the weak part, and reading the code beside the research turned up three
problems that matter more than any single number:

1. **A healthy player counts as "slow".** A player keeps a quality while it takes up to 95% of its
   link (`linkShareWhenAhead`), so a player with its buffer full can sit at a link of 1.05 to 1.5
   times its need. That is under the 150% line (`upload_crowded_slow_under_percent`). Two remote
   viewers on modest links with full buffers can make the house look crowded and lower everyone.
   **Recommend 110%**, and later a test that the slowdowns happen *together*.
2. **The house's other traffic can send a Watch Together member away.** The send-away check
   compares against the share as crowding has lowered it, not the share the owner set. A backup
   or a video call that crowds the upload lowers the share by a fifth every 10 s down to 10%. Once
   the players at their lowest qualities no longer fit, the newest member is removed 20 s later,
   and another every 50 s after that. That goes beyond D286 §5, which was about the films' own
   need. **Recommend:** only the setting's share can send anyone away; crowding only lowers
   qualities. This is the owner's to decide.
3. **The crowding controller can act on a lowering before its effect shows.** It may lower every
   10 s. But a player reports its link every 10 s, smooths it with 3 s and 9 s half-lives, and
   obeys a new ceiling only at its next chunk (4–10 s). So a second lowering can come before the
   first one's effect is reported. **Recommend 20 s**, or better, lower again only once every
   player has reported since the last lowering.

**Sending a member away after 20 s** is the severest value, and nothing in the field backs it.
Players hold 30 s ahead, so being over the 85% share does not yet overfill the line, and a player
that has stopped still counts for 30 s (`askedWithin`). **Recommend 60 s, and only when the need
is over the whole upload rather than the films' share.** The other values are close enough to keep
until the simulation (§6) has run. The 85% films' share gets one proposal: a fixed minimum left
over for the rest of the house. 15% of a 5 Mbit/s line is 0.75 Mbit/s, which is less than Zoom
says a 720p call needs (1.2 Mbit/s up).

**The ladder (group 3).** The rungs are Apple's, nearly exactly. The 400k@360p rung, which the
audit filed as measured once, matches Apple's 365k@360p. The 1200k@480p rung sits between Apple's
1100k@432p and its 2000k@540p. Apple's table has not changed since 2018-04-09, and Apple calls it
"initial bit rate recommendations that you can evaluate against your content". The gap is the
encoder:

1. **x264 `veryfast` costs about a third more bits than `medium` for the same picture.** Ozer's
   24-file study (NAB 2025) gives the bit rate each preset needs to match `veryslow` at about
   VMAF 95: veryfast 151%, faster 123%, medium 112%. The time each takes, as a share of
   `veryslow`'s: 16%, 19% and 31%. **Recommend `faster`**: about 19% fewer bits than `veryfast`
   for the same quality, at about 1.2 times its time. Take `medium` instead if the measurement
   shows a software node keeps up.
2. **Quick Sync runs at its fastest, lowest-quality setting by accident.** `QualityArguments`
   overrides the codec, the pixel format and the extra flags for Quick Sync but keeps
   `-preset veryfast`. For `h264_qsv` that is target usage 7, which Intel describes as "fastest
   setting, biggest quality drop". This is the production NAS's encoder, so production plays every encoded
   picture at TU7. Quick Sync also already needs 10–25% more rate than x264 for the same quality
   (slow-links.md §4). **Recommend `medium` (TU4)**, after a speed check on the N305.
3. **NVENC `p4`:** NVIDIA advises high-quality tuning and the slower presets for video on demand.
   **Recommend `p6`**, measured on the GPU test machine's own NVENC.
4. **The peak (1.5×) and buffer (2×)** sit within Apple's limit (a VOD peak "no more than 200% of
   the average"). Ozer found 200% constrained VBR the best of what he tested, but with two passes.
   **Keep them**, and add one row at 2×/2× to the measurement.

Rung changes wait for the measurement in §7: VMAF for each rung at `veryfast`, `faster` and
`medium` on five films (dark, grain, animation, sport, talk), plus NVENC p4 and p6, on the GPU test machine,
with the exact commands. A second, optional phase finds each film's best picture height at each
rate (the convex hull, as Netflix's per-title encoding does) to settle whether 1200k belongs at
480p or 540p, and whether the software ladder needs Apple's 4500k@720p.

**Confidence:** high on the three code findings (read from the code) and on what the competitors
document. Moderate on the presets: Ozer's numbers are at 1080p near VMAF 95, while our rungs sit
lower, where faster presets usually lose more. Low on every controller timing until the simulation
has run.

## 1. What the field does

### 1.1 Home servers

| Server | What it offers | Shares a total between streams? | Sends a viewer away? |
| --- | --- | --- | --- |
| Plex | "Internet upload speed" (the total for remote streaming) and "Limit remote stream bitrate", per stream: *"you might decide that you don't want any individual remote stream to go over 4 Mbps … thus allowing multiple streams"*. Downloads give way to playback. Since 2017 the client lowers its own quality ("Automatically adjust quality") | Not documented: the article names a total, but not how it is split | No |
| Jellyfin | `RemoteClientBitrateLimit`, per user or for the server, applied to each playback from outside the local network: `MediaInfoHelper.GetMaxBitrate` takes the smaller of the client's maximum and the limit (read at an earlier commit, 2026-10-25) | No | No |
| Emby | "Internet streaming bitrate limit": *"This limit is per stream"*, off by default, with a per-user override. It only affects transcoding decisions; *"network requests are not throttled"* | No | No |
| Channels DVR | Quality is chosen on the client; the community's advice for remote viewing is "480p@2Mbps or 720p@3Mbps" *(forum)* | No | No |

**What transfers.** All four leave sharing to the players and the transport, with a cap per stream
at most. Orpheum already goes further: it counts what each player needs, lowers the most demanding,
merges players onto qualities already being made, and gives starts what is left (upload-budget.md
§4–§4b). **No one sends a viewer away.** D286 §5 is a choice of our own, made on the owner's word,
so its timing has to stand on its own reasoning.

### 1.2 Research on players sharing a bottleneck

- **Akhshabi et al., NOSSDAV 2012** (read, cached). The players' ON-OFF pattern in steady state is
  "the main root cause" of instability, unfairness and an underused link, and not TCP. A player
  measures nothing while it is OFF, so when two players' ON periods do not overlap, each sees the
  whole link and overestimates its fair share by up to twice. Instability is worst when the fair
  share falls halfway between two rungs.
  *For us:* a player's link report overstates its share whenever others are idle, and understates
  it while they all load at once. The server's ceiling is what keeps the players from chasing that
  error.
- **FESTIVE, Jiang, Sekar and Zhang, CoNEXT 2012** (read). Fairness, efficiency and stability
  improved together by: a harmonic mean of the last 20 chunks' throughput; stepping up one level at
  a time, and from level k only after k chunks; stepping down at once; and randomising the moment
  each player requests. *For us:* raise slowly and lower at once, which our controller already
  does in shape.
- **Huang et al., IMC 2012, "Confused, timid and unstable"** (read). A player with a competing flow
  spiralled down from 1750 to 235 kbit/s in 91% of runs. Each OFF period of more than an RTO reset
  TCP's window, so chunks ended before the window grew back. *For us:* a slow player is not always
  short of link; it may be the transport. The Linux sysctl `net.ipv4.tcp_slow_start_after_idle`
  (on by default) does this on the core's HTTP path. The WebRTC path has its own window losses
  (webrtc-throughput-2026-10-28.md).
- **Cofano et al., MMSys 2016, "network-assisted control strategies for HTTP adaptive streaming"**
  (read). A central control plane solves max-min fairness of picture quality. *Bitrate guidance*,
  where the network tells each player its rate, gave the best fairness and halved the switching
  (under 0.005 Hz against about 0.01). A control period of 10, 30 or 60 s was tried: fairness
  worsened as the period grew, and "sampling times less than the video segment size is not
  meaningful". *For us:* our 5 s look is right for a controller that sets ceilings. Its effect
  arrives only one chunk later.
- **Mansy, Fayed and Ammar, IFIP Networking 2015** (cited by Cofano). Fairness by *quality*, which
  weighs screen size and viewing distance, differs from fairness by bit rate. *For us:* lowering
  the highest need first is fair in bit rate, not in what each viewer sees. A TV loses more from a
  rung down than a phone does. Recorded as an open question, not a value.
- **Google's ladder from playback statistics** (Chen et al., about 2017, read). Choosing the rungs
  from a million recorded bandwidth traces cut the average bit rate by 14% and switches by 13%.
  *For us:* the players' link reports are already logged, and could choose the rungs too (§4).
- **Congestion control.** BBR (v1, and v3 from 2023) takes most of a shallow buffer from CUBIC
  flows (Hock, Bless and Zitterbart, ICNP 2017; PAM 2024's study of BBRv3). WebRTC's Google
  Congestion Control backs off on delay, with an adaptive threshold added so that loss-based flows
  do not starve it (Carlucci et al., MMSys 2016), but the call still pays the queueing delay those
  flows build. *For us:* (a) do not move the core's sockets to BBR to make films faster: it would
  take the house's share. (b) When films crowd a call, the call loses latency and rate *without
  any player slowing*. §4c's crowding detector only sees the films losing, so the films' share
  (85%) is the only protection the house has.
- **What else a house sends.** Zoom asks for 1.2 Mbit/s up for a 720p call between two people, 2.6
  for a 720p group call, and 3.8 for 1080p (Zoom support KB0060748). A cloud backup takes whatever
  TCP gives it.

## 2. The eleven values, one by one

The current values are in `internal/core/settings/registry_sweep.go` and the code in
`internal/core/playback/{upload,crowding,lowering,upload_tuning}.go`. "Test" names a scenario in
§6.

| Value | Now | Evidence | Recommended | Confidence | Test |
| --- | --- | --- | --- | --- | --- |
| `upload_films_share_percent` | 85 | No measurement of what a house sends. Zoom's 720p calls need 1.2–2.6 Mbit/s up; 15% of a 5 Mbit/s line is 0.75 | **Keep 85, and propose leaving at least 1.5 Mbit/s** over (a share of `min(85%, upload − 1.5 Mbit/s)`). That is a design change, so the owner's | low | S1, T2: the call's round trip and loss while three films play |
| `upload_crowded_slow_under_percent` | 150 | Our player keeps a quality up to 95% of its link with a full buffer (`abr-choice.ts` `linkShareWhenAhead`), so healthy players report 105–150% | **110**. Later: also require each slow player's link to have *fallen* (say by a quarter against its own last minute). Falling together is the signature of a shared bottleneck (RFC 8382 uses the same idea, from delay statistics) | moderate (from our own constants) | S2: two remote players on their own thin links, the house idle; the share must not move |
| `upload_crowded_slow_players_at_least` | 2 | One slow player is its own link (the owner, 2026-10-12). Players inside the house count too, so a weak Wi-Fi in the house can trip it (upload-budget.md "Later") | **Keep 2**; leave players inside the house out once the core knows them | moderate | S2, S3 |
| `upload_crowded_keeps_percent` | 80 | CUBIC keeps 70% at a loss (RFC 9438), AIMD 50% (Chiu and Jain, 1989). Our rungs are 1.5–1.75× apart, so one 20% cut often takes one player down a rung | **Keep 80** | low | S3: time to settle and overshoot |
| `upload_crowded_lower_every_seconds` | 10 | The effect of a lowering shows only after a chunk (4–10 s) and the next report (every 10 s, smoothed over 3 s and 9 s); Cofano: no shorter than a segment | **20 s**, or better: lower again only once every slow player has reported since the last lowering | moderate | S3: count lowerings made before the previous one showed |
| `upload_crowded_floor_percent` | 10 | At 10% of a 5 Mbit/s line (425 kbit/s), not even one player at its lowest quality (about 500 kbit/s with sound) fits; below that, a lower share lowers no one further and only reaches the send-away (§3) | **25**, and moot once crowding cannot send anyone away (§3) | low | S4 |
| `upload_uncrowded_for_seconds` | 60 | With no memory of where crowding began, the share climbs back up into it: a sawtooth (Akhshabi's oscillation, made by the server) | **Keep 60**, and propose remembering the share at which crowding was last seen, so that growth slows on approaching it (CUBIC's *W_max*) | low | S3: changes per 10 minutes while a call goes on |
| `upload_uncrowded_grows_percent` | 10 | From 64% (two cuts) it takes 5 steps (2.5 min) to grow back; from the floor, 24 steps (12 min) | **Keep 10** | low | S3 |
| `upload_uncrowded_raise_every_seconds` | 30 | As above. The player's own step-up waits 30/60/120/240 s after failures (D352) | **Keep 30** | low | S3 |
| `upload_raise_under_percent` | 75 | A raise of one rung adds 50–75% of that player's rate; at 75% of the share, the headroom covers one such raise for a player using up to a third of the share | **Keep 75** | moderate | S1: ceilings raised and lowered again within 2 min |
| `upload_raise_room_seconds` | 30 | FESTIVE: raise slowly, and more slowly the higher the level. D352 doubles the player's own wait after each failed step-up | **Keep 30**, and propose doubling it for a playback lowered again within 2 minutes of a raise, as D352 does for the player | low | S1 |
| `upload_send_away_after_seconds` | 20 | §3 | **60**, and only when the need is over the **whole** upload, never because of crowding | moderate on the condition, low on 60 | S4, S5 |

## 3. Sending a member away

Things that bear on it, in order of weight:

1. **No other server does it.** §1.1.
2. **The share is 85% of the upload.** A need over the share by up to 18% (100/85) is not yet over
   the line, and no one stalls from it.
3. **Players hold 30 s ahead** below the top quality (`loadsAheadBelowTopS`). Even a real overload
   empties a buffer slowly. Over the line by 20%, each player gets 1/1.2 of its rate and loses 0.17
   s of buffer each second: about three minutes from 30 s of buffer to a stall.
4. **A player that stopped still counts for 30 s** (`askedWithin`), so a member who leaves can keep
   the need high for longer than the 20 s wait. Leaving and closing may forget it sooner (not
   checked here).
5. **Crowding reaches it** (Summary, finding 2): `KeepUnderShare` passes the crowded share to
   `lookAtUpload`, whose over-at-lowest result starts the send-away clock.

**Proposal:** a member is sent away only when the need at every lowest quality has been over the
**setting's whole upload**, not the films' share and not the crowded share, for **60 s**. Under
that, the players stay at their lowest and the log says so. The 60 s is two `askedWithin` windows
and still well inside a 30 s buffer's three minutes. **Confidence:** moderate that the condition is
right (it follows from the numbers above), low on 60 s itself. **Test:** S4, S5 and T4.

## 4. The ladder

### 4.1 Where the rungs come from

| Ours (`chunk_qualities`) | Apple, H.264, 16:9 | Netflix's old fixed ladder | Mux *(secondary)* | Note |
| --- | --- | --- | --- | --- |
| 6000k@1080p (`_hardware` only) | 6000 and 7800 @1080p | 4300, 5800 @1080p | 6000 @1080p | |
| 3000k@720p | 3000 and 4500 @720p | 2350, 3000 @720p | 2500–3000 @720p | Apple's 4500k@720p is missing from the software ladder |
| 2000k@540p | 2000 @540p | — | 1500 @540p | Apple's default variant (1.32) |
| 1200k@480p | 1100 @432p | 1750 @480p (720×480) | — | between Apple's rungs; 1.24× the pixels of 432p |
| 700k@432p | 730 @432p | 750 @384p | — | |
| 400k@360p | 365 @360p | 375 @288p | 800 @360p | Apple's own rung (the audit had it as measured once) |
| — | 145 @234p | 235 @240p | — | Apple: not for tvOS |

- **Apple's HLS authoring specification** (read at its 2025-06-26 revision, cached). The H.264 table
  was last changed 2018-04-09 ("Updated the variant bit rate table"). Later revisions added HEVC,
  HDR, spatial video and AV1 containers, not new H.264 rates. Apple names *"specific encoder
  implementation and features used"* and *"complexity of the content"* among the factors, and calls
  the table *"initial bit rate recommendations that you can evaluate against your content,
  constraints and encoding workflow"*. Keyframes every 2 s, 6 s segments, a VOD peak at most 200%
  of the average (1.30), and the 2000k variant as the default (1.32).
- **Netflix, per-title encoding** (2015-12-14, read). Bit rate needs differ by a factor of three or
  more between titles. Some reach a PSNR of 45 dB at 2500 kbit/s, others need 8000 for 38 dB.
  Grain stays blocky even at 5800, while a cartoon plays at 1080p from 1540. *"Adjacent bitrates
  should be perceptually spaced … just below one JND."* Netflix also makes ladders capped at 720p
  for devices limited to 720p.
- **Netflix, the dynamic optimizer** (2018-03-05, read). Per-shot choice of resolution and quantiser
  saves 28–38% (BD-rate) for x264, x265 and VP9 over fixed-QP encoding, scored by VMAF on frames
  upscaled to 1920×1080. *For us:* our chunks are made on demand, a minute ahead, so a per-shot
  search is out of reach. Per-title, or a ladder per kind of film, is not.
- **YouTube** publishes only *upload* recommendations (720p at 5 Mbit/s, 480p at 2.5, 360p at 1, for
  SDR H.264 up to 30 fps). It delivers lower than that, mostly in VP9 and AV1. Its delivery rates
  are not published, so they are not used here.
- **Bitmovin** recommends Apple's table as a starting point, then per-title encoding *(secondary)*.
  **Mux** publishes a typical ladder (360p at 800k, 540p at 1.5M, 720p at 3M), and per-title for
  its paid tiers *(secondary)*.

**Reading.** Our rungs are not a guess. They are the industry's default starting ladder, which
every source says to tune per content and per encoder. Apple does not say which encoder its rates
assume. The question is whether *our encoders at our presets* reach acceptable quality at those
rates, which only the measurement in §7 answers.

### 4.2 The rung values

| Value | Now | Evidence | Recommended | Confidence | Test |
| --- | --- | --- | --- | --- | --- |
| `chunk_qualities` | 3000k@720p, 2000k@540p, 1200k@480p, 700k@432p, 400k@360p | §4.1: Apple's starting ladder within 10%, untested on our encoder | **Keep the heights and rates until §7 has run.** Then: drop or move any rung that is not at least about 6 VMAF (one JND) from its neighbours, or that a lower height beats by more than 2 VMAF at the same rate; add 4500k@720p only if 3000k@720p falls under VMAF 90 on two or more of the five films | moderate (keep), the rest waits on data | §7 phases A and B |
| `chunk_qualities_hardware` | 6000k@1080p above the same | Apple's 1080p rung | **Keep**; re-check at the Quick Sync preset chosen in §5 | moderate | §7 phase C |
| `chunk_sound_kbps` | 96 | Within Apple's stereo AAC range (§3 of the audit) | Keep; a listening check is enough | moderate | — |

## 5. The encoder

| Value | Now | Evidence | Recommended | Confidence | Test |
| --- | --- | --- | --- | --- | --- |
| x264 preset | `veryfast` | Ozer, NAB 2025, 24 files, 1080p, about VMAF 95 target. Bit rate to match `veryslow`: veryfast 151%, faster 123%, fast 122%, medium 112%; time as a share of `veryslow`'s: 16%, 19%, 26%, 31%. So `faster` saves about 19% of the bits for about 1.2× the time; `medium` saves 26% for about 1.9×. D285 §3: a 720p 10 s chunk took 4.5–5.8 s at any preset, on a 10-bit AV1 source whose *decoding* dominated; an H.264 source leaves the encode dominant | **`faster`**; `medium` if §7 shows the weakest software node keeps a 720p run at 1.5× real time or better with two runs at once | moderate (the numbers are 1080p near VMAF 95; at our lower rates the gap is usually wider, which favours the change) | §7 phase A |
| Quick Sync preset | `veryfast` (TU7) **by accident** | `QualityArguments` sets the preset once for x264 and overrides it only for NVENC. Intel: veryfast is TU7, *"fastest setting, biggest quality drop"*, and medium is TU4. Quick Sync needs 10–25% more rate than x264 already (goughlui.com, 2023). No published VMAF per target usage | **`medium` (TU4)**, if the N305 keeps two 1080p runs at 4× real time or better | moderate | §7 phase C, on the production NAS with the owner's OK |
| NVENC preset | `p4` | NVIDIA: video on demand → high-quality tuning; P1 is the fastest and P7 the best. Blogs put NVENC p7 5–10% behind x264 medium *(secondary)* | **`p6`** | low (few households have NVENC) | §7 phase A on the GPU test machine |
| VAAPI | no preset | `h264_vaapi` takes `-compression_level`; untested | Leave; measure on the AMD test host if VAAPI households appear | — | — |
| Peak, buffer | `-maxrate` 1.5×, `-bufsize` 2× | Apple 1.30: a VOD peak of at most 200%. Ozer: 200% constrained 2-pass VBR was best, and CBR (1×) had poor low frames. Ours is one pass. A lower peak keeps chunk sizes even on thin links, which the upload accounting assumes | **Keep**, and measure one 2×/2× row | moderate | §7 phase A, extra row |
| Rate control | one-pass ABR (`-b:v`) | Capped CRF saves bits on easy content (Ozer), and the need accounting would then overcount, which errs safe. Two-pass is impossible a minute ahead | Leave; an optional capped-CRF row in §7 shows the size of the gain | low | §7 phase A, optional |

**Finding to fix whatever the measurement says:** the Quick Sync preset should be chosen on
purpose, as NVENC's is. The comment beside the switch names the IDR flag but not the preset, and
nothing records whether TU7 was chosen.

## 6. Test plan for sharing the upload

### 6.1 Simulation first (no device, minutes per run)

The controller's parts are already pure functions under a clock: `crowding.look`,
`uploadControl.look` (lowering.go), `lastResort.isDue`. A later test-only harness, after the
owner's word, would drive them with a small model of the bottleneck:

- **The link:** upload capacity `C(t)`; the films' traffic and the house's traffic share it. An
  inelastic call takes its rate first. An elastic backup takes a per-flow fair share, so with n
  players' connections and one backup each gets `C/(n+1)` while all are active.
- **Each player:** the ladder's rungs; ON-OFF loading with a 30 s target buffer; a link estimate
  from its ON periods, with 3 s and 9 s half-lives, reported every 10 s; picks the highest rung
  under `min(ceiling, share × estimate)`, with the share 0.7 rising to 0.95 as the buffer fills;
  stalls when the buffer empties. Its own last-mile is a recorded trace where one exists (the
  players' link reports are logged), else a constant.
- **Measures** (Akhshabi's and FESTIVE's): instability (the share of chunk requests that change
  rung), unfairness (the mean spread of rates), use of the link, seconds stalled, minutes falsely
  crowded, members sent away, time to settle after the house's traffic starts and stops, the
  films' and the call's rate, and lowerings made before the previous one could show.

| # | Scenario | Pass |
| --- | --- | --- |
| S1 | 5 Mbit/s up, setting 5, three remote players on fast links; a 720p call (1.2 Mbit/s, inelastic) from minute 2 to minute 10 | The films leave the call its rate. No ceiling changes more than twice in 10 minutes. No stall |
| S2 | As S1 with no call, but two players on their own 3 Mbit/s links (phones), buffers full | The share never moves (fails today at 150%) |
| S3 | As S1, with a backup (elastic) from minute 2 to minute 15 | Settles within 60 s; under 4 share changes per 10 minutes while the backup runs (a sawtooth fails); grows back within 3 minutes of the end |
| S4 | A group of three at their lowest (about 1.5 Mbit/s) on a 2 Mbit/s setting (a share of 1.7); then a backup | No one is sent away for the backup. Today: one lowering (to 1.36) leaves them over at their lowest, so the newest goes about 20–30 s into the backup, then another every 50 s |
| S5 | A group of three at their lowest just over the share; one member leaves at minute 3 | No one is sent away for the leaver's ghost need (`askedWithin`) |
| S6 | Sweep: keeps {70, 80, 90} × lower every {10, 20, 30} × slow under {105, 110, 150} × grows {5, 10, 20} × raise every {15, 30, 60} over S1–S5 | Choose the values that pass every scenario with the least instability; report how sensitive each result is to each value |

### 6.2 Then a shaped upload with real players

Run when the owner is not testing on staging (staging shares this machine). Use the method of
[webrtc-throughput-2026-10-28.md](webrtc-throughput-2026-10-28.md): the core alone in a network
namespace joined to the sandbox by a veth pair, so that shaping its egress shapes only its traffic.
The players are the pinned Playwright Chromium, three of them, two in one Watch Together group.
Set `server_upload_mbps` to 5.

```sh
# The core's side of the veth ($IF): a 5 Mbit/s upload with a home router's deep queue.
sudo tc qdisc add dev "$IF" root handle 1: htb default 10
sudo tc class add dev "$IF" parent 1: classid 1:10 htb rate 5mbit ceil 5mbit
sudo tc qdisc add dev "$IF" parent 1:10 handle 10: pfifo limit 1000
# Repeat each run with a good router's queue instead:
#   sudo tc qdisc replace dev "$IF" parent 1:10 handle 10: fq_codel
# The house's other traffic, sent from inside the core's namespace so it shares the shaped egress
# (iperf3 -s on the sandbox side first):
sudo ip netns exec core iperf3 -c "$PEER" -u -b 1.2M -t 480     # a 720p call, inelastic
sudo ip netns exec core iperf3 -c "$PEER" -t 780                # a backup, elastic TCP
sudo ip netns exec core ping -i 0.2 "$PEER" > call-rtt.log      # the call's latency, under load
```

| # | Run | Look for |
| --- | --- | --- |
| T1 | Three players, no other traffic | Starts split the share (§4a, §4b); the log's need beside sent bytes (every 20 s); no ceiling churn |
| T2 | T1 plus the call, from minute 2 | RTT under pfifo against fq_codel; whether any player slows (the films may simply win); the films' share left for the call |
| T3 | T1 plus the backup | Crowding detected, settled, grown back; ceiling changes per player |
| T4 | Setting lowered to 1.5 Mbit/s mid-film | Qualities fall, merge, reach their lowest; the warning; whether and when the newest member is sent away |
| T5 | T3 with the recommended values (§2) | The same measures, against T3 |

## 7. Measurement plan: VMAF on the GPU test machine (run later)

**Do not run while the owner tests on staging.** The GPU test machine is a separate machine, but its wake and
copies pass through the NAS. The x264 and NVENC phases run on the GPU test machine; the Quick Sync phase runs on
the production NAS (production), only with the owner's OK and while nobody watches.

### 7.0 Setup

```sh
# wake the GPU test machine first: it sleeps when idle (orpheum-ops holds the script)
ssh orpheum@gpu.example
# Encode with the node's own FFmpeg (the pinned build), so the encoders are production's:
FF=/path/to/node/ffmpeg
"$FF" -hide_banner -encoders | grep -E 'libx264|h264_nvenc'
# Score with any FFmpeg that has libvmaf (a separate binary is fine, the scores do not depend on it):
VF=ffmpeg; "$VF" -hide_banner -filters | grep libvmaf
```

**Five films**, one of each kind, chosen by the owner and recorded **by file id only**: dark (night
scenes), grain (film grain), animation, sport or fast motion, talk (static, faces). All SDR
(HDR needs tone mapping first and is a later round), 1080p or 4K sources. For each: its id, kind,
codec, height and average rate, and a 120 s excerpt starting a quarter of the way in (past the
openings, before the credits).

```sh
# One lossless 8-bit reference per film: the excerpt at its own size up to 1080p (a 4K source is
# brought to 1080p here, a deviation from production, which scales from 4K).
ID=1234; SRC=/path/of/file; START=$(( $(ffprobe -v error -show_entries format=duration -of csv=p=0 "$SRC" | cut -d. -f1) / 4 ))
"$FF" -hide_banner -ss "$START" -t 120 -i "$SRC" -map 0:v:0 -an -sn \
  -vf "scale=-2:'min(1080,ih)':flags=lanczos,format=yuv420p" -c:v ffv1 -level 3 "mez-$ID.mkv"
read W H < <(ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=s=' ':p=0 "mez-$ID.mkv")
```

Each reference is about 2–4 GB; five fit in 20 GB.

### 7.1 Phase A: x264 presets and NVENC, each rung

The arguments are `QualityArguments`' own, except one: keyframes come every 6 s, standing in for the
source's keyframes (production forces them where the source has its own, 4–10 s apart).

```sh
RUNGS="3000:720 2000:540 1200:480 700:432 400:360"
KF='expr:gte(t,n_forced*6)'
for P in veryfast faster medium; do
  for R in $RUNGS; do K=${R%%:*}; HT=${R##*:}
    /usr/bin/time -f "%e s %P cpu" "$FF" -hide_banner -y -i "mez-$ID.mkv" -an \
      -c:v libx264 -preset "$P" -vf "scale=-2:$HT" -pix_fmt yuv420p -profile:v high \
      -b:v "${K}k" -maxrate "$((K*3/2))k" -bufsize "$((K*2))k" \
      -g 100000 -force_key_frames "$KF" -sc_threshold 0 "x264-$P-$K-$ID.mp4" 2>> "time-$ID.log"
  done
done
# NVENC, the hardware ladder (with 1080p), p4 now and p6 proposed:
for P in p4 p6; do
  for R in 6000:1080 $RUNGS; do K=${R%%:*}; HT=${R##*:}
    "$FF" -hide_banner -y -i "mez-$ID.mkv" -an -c:v h264_nvenc -preset "$P" -vf "scale=-2:$HT" \
      -pix_fmt yuv420p -profile:v high -b:v "${K}k" -maxrate "$((K*3/2))k" -bufsize "$((K*2))k" \
      -g 100000 -force_key_frames "$KF" -forced-idr 1 "nvenc-$P-$K-$ID.mp4"
  done
done
# Extra rows, 720p and 432p only, at the chosen preset: peak 2x/2x, and capped CRF 23.
#   ... -b:v ${K}k -maxrate $((K*2))k -bufsize $((K*2))k ...
#   ... -crf 23 -maxrate $((K*3/2))k -bufsize $((K*2))k ...   (also record its average rate)
```

**Speed** as well as quality. Record the wall time of each encode. Then repeat the 720p and 432p
rungs at each preset under `taskset -c 0-3 … -threads 4`, standing in for a four-core NAS; the
*ratio* between presets carries over to other machines, the absolute times do not.

### 7.2 Scoring

Each encode is scaled back to the reference's own size (bicubic, as Netflix scores) and scored with
the default model and the phone model in one pass:

```sh
score() {  # $1 = encode
  "$VF" -hide_banner -i "$1" -i "mez-$ID.mkv" -lavfi \
    "[0:v]scale=$W:$H:flags=bicubic,setpts=PTS-STARTPTS[d];[1:v]setpts=PTS-STARTPTS[r];\
[d][r]libvmaf=model=version=vmaf_v0.6.1\\:name=tv|version=vmaf_v0.6.1\\:name=phone\\:enable_transform=true:\
n_threads=16:log_fmt=json:log_path=${1%.mp4}.vmaf.json" -f null -
}
for f in x264-*-"$ID".mp4 nvenc-*-"$ID".mp4; do score "$f"; done
```

Check the filter's syntax against `"$VF" -h filter=libvmaf` before the batch: the multi-model
syntax changed between libvmaf 1 and 2. From each JSON, keep the mean, the harmonic mean and the
1st percentile of the frame scores (low frames), for both models, beside the encode's actual
average rate (`ffprobe … format=bit_rate`) and its time.

**Cost:** 5 films × (15 x264 + 12 NVENC + 4 extra) = 155 encodes of 120 s, and as many scorings of
about 2,900 frames. That is roughly 2–3 hours on the GPU test machine, unattended.

### 7.3 Phase B (optional): the best height at each rate

At the preset chosen in phase A, for each film: heights {360, 432, 480, 540, 720, 1080} × rates
{400, 700, 1100, 1600, 2000, 3000, 4500} kbit/s, 42 encodes. For each rate, the height with the
highest VMAF is the film's convex hull (Netflix's per-title method). About 3 hours.

### 7.4 Phase C: Quick Sync on the production NAS (owner's OK, nobody watching)

The same five references (copied by file id), the hardware ladder, `-c:v h264_qsv -preset
{veryfast,medium} -pix_fmt nv12 -forced_idr 1`, with production's other arguments. Record the
speed of two runs at once (two 1080p encodes in parallel), which is what decides whether `medium`
fits. Score on the GPU test machine.

### 7.5 How the results decide

- **Preset:** take the slowest of `veryfast`, `faster` and `medium` that keeps a 720p run at 1.5×
  real time or better on the four-core stand-in, with two runs at once (the eviction line is 1.1×,
  D250 §6). Take it only if it gains at least 3 VMAF (half a JND) at the 700k–2000k rungs, where
  viewers on thin links are.
- **Rungs:** neighbours at least about 6 VMAF apart (one JND, Netflix's spacing) and 1.5–2× apart
  in rate. The lowest rung at phone-model VMAF 60 or better on four of the five films. The top
  software rung at VMAF 90 or better on four of the five. A rung that a lower height beats by more
  than 2 VMAF at the same rate (phase B) moves to that height.
- **Quick Sync:** `medium` if two 1080p runs at once stay at 4× real time or better on the N305.
- **Per kind of film:** if the animation and talk films clear every threshold at about two thirds
  of the rate while grain falls short, propose ladders by kind of film (Netflix's lesson) as a
  design question for the owner, rather than one global retune.

## 8. Open questions for the owner

1. **May the house's other traffic send a Watch Together member away?** (§3; today it can.)
2. **A fixed minimum left for the rest of the house** beside the 85% share (§2)?
3. **Fairness by quality or by rate** when lowering: should a TV keep its rung ahead of a phone
   (Mansy et al.)?
4. **Quick Sync's preset:** was TU7 intended? (§5)

## Sources

Opened and read, 2026-10-29, unless marked:
[Plex: Remote Access](https://support.plex.tv/articles/200289506-remote-access/);
[Emby: Network setup](https://github.com/EmbySupport/Emby.Docs/blob/master/Hosting-Settings.md);
[Emby forum: bitrate limit](https://emby.media/community/index.php?/topic/80855-internet-streaming-bitrate-limit/) *(secondary)*;
Jellyfin `Jellyfin.Api/Helpers/MediaInfoHelper.cs` at ca6dcf7 (local clone);
[Channels community: streaming quality](https://community.getchannels.com/t/streaming-quality-settings-help/34156) *(secondary)*;
[Apple HLS authoring specification](https://developer.apple.com/documentation/http-live-streaming/hls-authoring-specification-for-apple-devices) (2025-06-26 revision);
[Netflix: per-title encode optimization](https://netflixtechblog.com/per-title-encode-optimization-7e99442b62a2) (2015-12-14);
[Netflix: dynamic optimizer](https://netflixtechblog.com/dynamic-optimizer-a-perceptual-video-encoding-optimization-framework-e19f1e3a277f) (2018-03-05);
[YouTube upload settings](https://support.google.com/youtube/answer/1722171) *(secondary)*;
[Bitmovin: video bitrate for HLS and DASH](https://bitmovin.com/blog/video-bitrate-streaming-hls-dash/) *(secondary)*;
[Mux: instant per-title encoding](https://www.mux.com/blog/instant-per-title-encoding) *(secondary)*;
[Ozer, NAB 2025: best practices for H.264 and HEVC](http://nabstreamingsummit.com/wp-content/uploads/2025/04/Ozer-NAB-2025.pdf);
[Ozer: choosing an x264 preset, part 2](https://streaminglearningcenter.com/articles/the-correct-way-to-choose-an-x264-preset-part-2-your-audience-changes-the-answer.html) (2026-06-29);
[Ozer: finding the JND with VMAF](https://streaminglearningcenter.com/codecs/finding-the-just-noticeable-difference-with-netflix-vmaf.html) *(secondary)*;
[Intel: Quick Sync and FFmpeg](https://www.intel.com/content/dam/www/public/us/en/documents/white-papers/cloud-computing-quicksync-video-ffmpeg-white-paper.pdf) *(secondary, via search)*;
[goughlui.com: h264_qsv round-up](https://goughlui.com/2023/12/28/video-codec-round-up-2023-part-6-h264_qsv-h-264-intel-quick-sync-video/) (via slow-links.md §4);
[NVIDIA NVENC programming guide](https://docs.nvidia.com/video-technologies/video-codec-sdk/13.0/nvenc-video-encoder-api-prog-guide/index.html);
[Netflix/vmaf#987: the phone model in libvmaf 2](https://github.com/Netflix/vmaf/issues/987);
Akhshabi et al., NOSSDAV 2012; Jiang, Sekar and Zhang, CoNEXT 2012 (FESTIVE); Huang et al., IMC
2012; Chen et al., Google, about 2017 (all cached in `abr-research/`);
[Cofano et al., MMSys 2016](https://2016.acmmmsys.org/papers/MMSYS/a3-cofano.pdf);
[RFC 8382: shared bottleneck detection](https://www.rfc-editor.org/info/rfc8382/) *(abstract)*;
[Hock, Bless and Zitterbart, ICNP 2017](https://telematics.tm.kit.edu/publications/Files/595/2017-kit-icnp-bbr-authors-copy.pdf) *(secondary)*;
[BBRv3, PAM 2024](https://pam2024.cs.northwestern.edu/pdfs/paper-59.pdf) *(secondary)*;
[Carlucci et al., MMSys 2016](https://dl.acm.org/doi/10.1145/2910017.2910605) *(abstract)*;
[Zoom: bandwidth requirements](https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0060748).
