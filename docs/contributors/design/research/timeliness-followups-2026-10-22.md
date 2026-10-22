> **ABOUTME:** Four questions the owner left open on the data-timeliness scenarios, each researched
> against competitors, platforms and the literature: metered connections, how many media servers
> have no hardware encoder, who drops a rung when several viewers share too little, and when
> background work should run with quiet hours and sleeping disks. Each part ends with a
> recommendation. It proposes and does not decide.

# Timeliness follow-ups, 2026-10-22

**What prompted it.** The owner's answers to
[data-timeliness.md](../plans/data-timeliness.md) §11 left four questions open:

- *C6 and Q8:* "What do our competitors say and do about this? How much of an issue is it these
  days? How prevalent are metered connections these days? How much do people complain about
  saturating metered connections?"
- *Q3:* "How common is it for a system like plex/jellyfin to run on a system with no GPU these days?
  If it's likely enough, we can start with the assumption of no GPU."
- *Q4:* "All viewers are equal … I'm not sure about playback dropping a rung when multiple viewers
  are there. Has anyone examined this and come up with an answer?"
- *Q7, quiet hours and sleeping disks:* "I'm not quite sure about this yet. Do more research about
  it."

**How it was done.** Four research agents worked in parallel. They searched the web and cached
every page they fetched under `/yoloai/cache/http/metered/`, `gpu-prevalence/`, `multiviewer/` and
`quiet-hours/`. Each says which claims it could not check, and those are marked *unverified* here.
Some papers were read from their abstracts only, and they are marked too.

## 1. Metered connections (C6)

### What competitors do

| Product | Cellular behaviour | Source |
| --- | --- | --- |
| Netflix | "Automatic" is the default, at about 4 h per GB. Save Data, Wi-Fi Only and Maximum Data are the choices. | [Netflix Help](https://help.netflix.com/en/node/87) |
| Disney+ | Separate Wi-Fi and cellular settings. On cellular, Automatic is up to HD at about 2 GB/h, and Save Data about 0.6 GB/h. The default is *unverified*. | [iDownloadBlog](https://www.idownloadblog.com/2020/12/23/disney-plus-data-usage-tutorial/) |
| Spotify | Downloads use Wi-Fi only by default. Data Saver is opt-in. | [Spotify](https://support.spotify.com/us/article/listen-offline/) |
| Plex | "Limit Cellular Data" caps quality on cellular, "(defaults to 0.7 Mbps)". | [Plex](https://support.plex.tv/articles/115007570148-automatically-adjust-quality-when-streaming/) |
| Jellyfin | Nothing for cellular. The request has been open since 2020. | [jellyfin-android #210](https://github.com/jellyfin/jellyfin-android/issues/210) |
| Emby, Infuse | No setting specific to cellular. | [Emby forum](https://emby.media/community/topic/55850-emby-mobile-app-set-mobilewifi-max-bitrate/), [Firecore](https://firecore.com/blog/infuse-85-smarter-streaming) |

**No product documents switching off prefetch or warming on cellular.** The controls that exist are
a quality cap and downloads limited to Wi-Fi.

### What the platforms expose

- **Native apps have reliable signals:**
  - Android: `isActiveNetworkMetered` and Data Saver.
  - iOS: `NWPath.isExpensive` and `isConstrained` (Low Data Mode).
  - Windows: `NetworkCostType`. Microsoft's advice for a metered connection: "Play movies in lower
    resolutions. Delay non-critical downloads."

  Sources: [Android](https://developer.android.com/develop/connectivity/network-ops/data-saver),
  [Apple](https://developer.apple.com/documentation/network/nwpath/isexpensive),
  [Microsoft](https://learn.microsoft.com/en-us/previous-versions/windows/apps/jj835821(v=win.10)).
- **The web client has almost none:**
  - `navigator.connection.saveData` exists in Chromium only. Chrome for Android removed Lite mode in
    2022, and Lite mode was the only thing that set it.
  - `connection.type === "cellular"` exists in Chrome for Android only.
  - Safari, on iOS included, gives nothing.

  Sources: [caniuse](https://caniuse.com/mdn-api_networkinformation_savedata),
  [MDN data](https://raw.githubusercontent.com/mdn/browser-compat-data/main/api/NetworkInformation.json),
  [blink-dev](https://groups.google.com/a/chromium.org/g/blink-dev/c/HR7tWmewbSA/m/9UrOsc9AAAAJ).

### How common metered connections are

- **In Germany, mobile is still mostly capped and the home connection is not.**
  - The average SIM used 8.4 GB a month in 2025, and the average fixed line 376 GB
    ([Bundesnetzagentur](https://www.bundesnetzagentur.de/SharedDocs/Pressemitteilungen/EN/2026/20260605_TK_JB.html)).
  - Unlimited plans were 20% of customers in a 2022 survey
    ([Simon-Kucher](https://www.simon-kucher.com/de/wer-wir-sind/newsroom/mobilfunk-studie-trend-zu-unbegrenztem-internet-provoziert-anbieter)).
    No newer figure was found.
  - An hour of video at full quality is 3 GB or more, so a 10–30 GB plan holds a few hours.
- **In the US, unlimited is the norm**, and Comcast dropped data caps from new plans in 2025
  ([MediaPost](https://www.mediapost.com/publications/article/407031/comcasts-new-plans-scrap-controversial-data-caps.html)).
- **Video is about 75% of all mobile traffic**
  ([Ericsson](https://www.ericsson.com/en/reports-and-papers/mobility-report/dataforecasts/mobile-traffic-update)).

### Complaints

- **Too much data is a real complaint, but a rare one.** A 2026 post found Jellyfin used 3.27 GB of
  one month's mobile data, with no caching
  ([Gardiner Bryant](https://gardinerbryant.com/jellyfin-has-flaws-lets-talk-about-them/)).
  Separately, a 2023 Plex user found its cellular limit did not work
  ([Plex forum](https://forums.plex.tv/t/cellular-data-limitation-is-not-working/830642)).
- **Too low a quality is the more persistent complaint.** Plex on iOS has been stuck at 480p or
  2 Mbit/s on cellular, with the limit off, reported every year since 2019
  ([2025 thread](https://forums.plex.tv/t/2025-jan-ios-app-transcoding-on-cellular-data-limited-to-2mbps-480p/904372)).
- Neither is common. The lesson is that a cap must be visible and easy to lift. Reddit could not be
  searched, so this undercounts.

### Recommendation

- **Warming on the node costs the phone nothing.** Only bytes the client fetches count against a
  plan. C6 is about the client's own prefetch: video ahead of playback, and posters.
- **On a connection the platform says is metered,** the client doesn't prefetch video ahead of
  playback and caps quality near Netflix's and Plex's defaults. The cap is shown in the player and
  is easy to lift.
- **When the signal is unknown** (Safari, Firefox, desktop), the client behaves as usual. Guessing
  "metered" leads straight to the "stuck at low quality" complaint.
- **A per-device setting**, modelled on Netflix's: Automatic, Save data, Unrestricted. It is kept on
  the device, not the account, because the same person has a capped phone and an unmetered TV.
- **For the closed beta:**
  - build no detection;
  - keep the client's video prefetch small and cancellable;
  - ship the per-device toggle only if it is cheap.

  Revisit with native apps, which get the signals.

## 2. Hardware encoders (Q3)

### No one publishes the number

Plex has published no figures on hardware transcoding, Jellyfin collects no telemetry, and the
surveys found ask nothing about GPUs. The indirect evidence is strong, though.

**Hosts that can't hardware-transcode are common and growing:**
- **Every 2025 Synology "plus" model.** The Ryzen R1600 and V1500B models have no integrated GPU, and
  the J4125 models ship without the graphics driver, which Synology withheld over HEVC licensing
  ([NAS Compares](https://nascompares.com/2025/09/24/how-to-get-hardware-transcoding-back-on-your-synology-nas/),
  [WunderTech](https://www.wundertech.net/what-is-the-best-synology-nas-for-plex/)).
- **The Raspberry Pi 5,** which has no hardware encoder
  ([Raspberry Pi](https://www.raspberrypi.com/news/introducing-raspberry-pi-5/)).
- **Rented servers:** 27% of self-hosters in one survey
  ([deployn 2025](https://selfhosted-survey-2025.deployn.de/)).
- **Ryzen desktop CPUs without a "G" suffix, and Intel "F" CPUs**
  ([Jellyfin](https://jellyfin.org/docs/general/administration/hardware-selection/)).

**Containers are the norm, so passthrough matters.**
- 3,932 of 4,081 respondents run containers, and 45% run Proxmox
  ([selfh.st 2025](https://selfh.st/survey/2025-results/)).
- Passing the GPU through to a container takes manual steps, and forum threads show it often fails
  ([Proxmox forum](https://forum.proxmox.com/threads/proxmox-lxc-igpu-passthrough.141381/)).

**Plex still requires Plex Pass for hardware transcoding.**
- A free Plex user transcodes on the CPU whatever the hardware
  ([Plex](https://support.plex.tv/articles/115002178853-using-hardware-accelerated-streaming/)).
- What changed in 2025 was remote playback, which now needs a pass
  ([Plex blog](https://www.plex.tv/blog/important-2025-plex-updates/)).

### What a CPU manages

Plex's rule of thumb, in PassMark points
([Plex](https://support.plex.tv/articles/201774043-what-kind-of-cpu-do-i-need-for-my-server/)):

| Transcode | Points |
| --- | --- |
| 1080p H.264 | 2,000 |
| 4K SDR to 1080p | 12,000 |
| 4K HDR to 1080p | 17,000 |

Applied to typical hosts:

| CPU | PassMark | 1080p transcodes | 4K HDR to 1080p |
| --- | --- | --- | --- |
| Intel N100 | 5,305 | about 2.6 | about 0.3× real time |
| Ryzen V1500B | 4,513 | about 2.2 | about 0.27× |
| Celeron J4125 | 2,926 | about 1.5 | about 0.17× |
| Core i5-6500 | 5,599 | about 2.8 | about 0.33× |

This is arithmetic from the rule, not a measurement, and it leaves nothing over for the server.

**Tone mapping decides it.**
- Jellyfin: tone mapping 4K Dolby Vision in software "requires a Ryzen 9 5950X for faster than
  real-time" (hardware-selection page above).
- With a GPU it becomes easy: an N100's integrated GPU tone-maps 4K HDR to 1080p at about 80–130 fps
  ([benchmark](https://randomnamer.github.io/StaticBlog/2024/03/26/jellyfinTranscodeBench/)).

### Recommendation

- **Assume no hardware encoder.** The node detects one at start and reports it as a bonus.
- **Design for one guaranteed concurrent 1080p software transcode on a 4-core node.** A second one
  is best effort.
- **Direct play and remux are the norm, and a video transcode is the exception.** In order of
  preference:
  1. direct play;
  2. a remux;
  3. an audio-only transcode, which is cheap;
  4. a video transcode.
- **4K HDR to an SDR screen without hardware can't be done in real time.** It needs an explicit
  answer, not a stall: direct play only, a version prepared ahead, or "this film needs a capable
  screen". That is the owner's call.

## 3. Several viewers on too little (Q4)

### The literature

- **When several adaptive players share a link, it goes wrong.** The first player takes the high
  rung, and the second oscillates.
  - Akhshabi et al., NOSSDAV 2012 (abstract only).
  - [Seufert et al., survey §7.1](https://opus.bibliothek.uni-augsburg.de/opus4/frontdoor/deliver/index/docId/108572/file/108572.pdf).
  - [PANDA](https://ar5iv.labs.arxiv.org/html/1305.0510).
- **The fairness target has moved from equal bitrate to equal experience.**
  - FESTIVE (CoNEXT 2012) targets equal bitrate
    ([paper](https://conferences.sigcomm.org/co-next/2012/eproceedings/conext/p97.pdf)).
  - Later work targets equal experience, weighted by screen and content. Minerva (SIGCOMM 2019):
    "A user will have a better experience streaming a given video on a smartphone at 1 Mbit/s than a
    large 4K TV at 1 Mbit/s" ([paper](http://web.cs.ucla.edu/~ravi/CS219_F19/papers/minerva.pdf)).
  - Mansy et al. weigh each player by its screen's pixel density
    ([IFIP 2015](https://dl.ifip.org/db/conf/networking/networking2015/1570066341.pdf)).
  - MPEG's SAND lets a home network element assign shares
    ([DASH-IF](https://dashif.org/docs/SAND-Whitepaper-Dec13-final.pdf)).
- **None of it studies admission.** No paper says whether a newcomer may push down someone already
  watching. They all model players sharing a link all along.

### What a switch costs a viewer

- **A stall costs more than a switch.** One rebuffer causes three times the abandonment of one bitrate
  change ([YouSlow](http://www.cs.columbia.edu/~hn2203/papers/11_infocom2016.pdf)).
- **The number of switches matters little; their size and the time spent high matter a lot.** No
  switching scored a MOS of about 4, and any switching just under 3
  ([Hoßfeld et al., QoMEX 2014](https://opus.bibliothek.uni-augsburg.de/opus4/files/107445/107445.pdf)).
- **Viewers punish a drop more than they reward a rise, and prefer consistent quality**
  ([Rehman and Wang, QoMEX 2013](https://ece.uwaterloo.ca/~z70wang/publications/qomex13.pdf)).
- **Changes slower than one every 2 s beat a constant low quality; faster ones don't** (Ni et al.,
  via Seufert's survey).

### What competitors do

**None of them lowers running streams to admit a newcomer.**

- **Plex:**
  - It caps the total remote upload at 80% of the stated speed, and budgets for peaks.
  - "Once a video starts at original quality, it will stay at original quality"
    ([Plex](https://support.plex.tv/articles/115007570148-automatically-adjust-quality-when-streaming/),
    [limits](https://support.plex.tv/articles/227715247-server-settings-bandwidth-and-transcoding-limits/)).
  - A newcomer with nothing left is refused with "Not Enough Bandwidth", and users dislike the
    bluntness ([forum](https://forums.plex.tv/t/limit-remote-stream-quality-and-not-enough-bandwidth/164229)).
  - Transcodes are capped, and beyond the cap they are refused
    ([forum](https://forums.plex.tv/t/unable-to-play-media-due-to-video-transcoders-limit/941024)).
- **Jellyfin:**
  - It has only a cap per stream, and a running stream never changes
    ([forum](https://forum.jellyfin.org/t-dynamic-user-bandwidth-allocation)).
  - Users have asked since 2022 for one budget that adapts to how many are watching
    ([request 1504](https://features.jellyfin.org/posts/1504/global-bandwidth-limit)).
  - It has no cap on transcodes. Without one, a second encode on a Pi 4 made both streams play a few
    seconds, then pause a few seconds
    ([jellyfin-chromecast #40](https://github.com/jellyfin/jellyfin-chromecast/issues/40)).
- **Emby:** fixed slots and refusal.

### Recommendation

"All viewers are equal" means **one rule for everyone**, not equal bitrate at every moment. The rule:
*once you are watching, nobody's arrival stalls you or lowers your picture.*

1. **Running playbacks keep their rung, and the newcomer takes what is left.**
   - The server paces the newcomer to the remainder. Left to TCP, it would simply take from the
     others.
   - The budget is the uplink less headroom, counting each stream at its peak, not its average.
2. **There is a floor.** Below it, the newcomer chooses between "wait, and we'll say when there's
   room" and "play now at reduced quality".
   - The floor can depend on the screen, since a phone needs less than a TV.
   - When a stream ends, a lowered stream rises once, by one step.
3. **Optionally, and off by default, running *transcodes* drop one rung**, but only if that lifts a
   newcomer from below the floor to it.
4. **A direct play is a fixed reservation at its peak.** It never steps down, because stepping down
   means starting a transcode.
5. **The encoder has strict admission.** An overcommitted encoder stalls every stream it feeds. A
   transcode that doesn't fit is refused or waits, and the newcomer is offered a remux or direct
   play if its device can take one.

## 4. Quiet hours and sleeping disks (A8, Q7)

### What competitors do

- **Plex** runs maintenance in a window, 03:00–06:00 by default
  ([Plex](https://support.plex.tv/articles/201553286-scheduled-tasks/)).
  - Preview thumbnails retried the same 20 failing files every night and woke the disks
    ([thread](https://forums.plex.tv/t/plex-not-respecting-scheduled-task-settings-keeps-spinning-up-disks-help/871848/1)).
    It was reported again in 2026.
  - Whether Plex pauses tasks during playback is *unverified*.
- **Jellyfin:** the default triggers, read from its source, are:
  - trickplay daily at 03:00;
  - chapter images at 02:00;
  - a library scan every 12 h.

  Nothing checks whether someone is playing. A 2025 regression stat-ed files every 5 minutes and
  kept disks from ever sleeping ([#15755](https://github.com/jellyfin/jellyfin/issues/15755)).

### Disks

- **Modern NAS drives are rated for 600,000 load/unload cycles**
  ([WD Red Plus](https://documents.westerndigital.com/content/dam/doc-library/en_us/assets/public/western-digital/product/internal-drives/wd-red-plus-hdd/data-sheet-western-digital-wd-red-plus-hdd.pdf),
  [IronWolf](https://www.seagate.com/www-content/product-content/ironwolf/en-us/docs/100844482e.pdf)).
  Twenty spin-downs a day for five years uses about 6% of that. The 50,000 figure is for old 1 TB
  drives that land their heads on the platter.
- **No data shows that spin-down wears drives.** Backblaze's drives never spin down
  ([Backblaze](https://www.backblaze.com/blog/hard-drive-smart-stats/)).
- **A sleeping drive saves about €9–19 a year at German prices.** Idle draw is 3.4–7.2 W against
  0.25–1 W asleep, at 34.4 ct/kWh ([Verivox](https://www.verivox.de/strom/strompreisentwicklung/)).
  Mostly, a sleeping drive buys silence.
- **Spin-up takes up to 23–30 s for a large drive** (IronWolf 8 TB). That is longer than the
  scenarios assumed.
- **Seeking is what people hear,** at about 4–5 dBA above idle. A sleeping drive is silent.
  - People complain of scrubs and periodic reads at night
    ([TrueNAS forum](https://forums.truenas.com/t/why-do-people-complain-about-hdd-noise/28835)).
  - A read-through reads one file in order, so it should seek less than a scan does. This is
    inferred, not measured.
- **TrueNAS ships with disks always on** ([docs](https://www.truenas.com/docs/scale/24.10/scaleuireference/storage/disksscreen/)).
  Many things keep disks awake whatever Orpheum does: SMART polling, scrubs and other software
  ([Synology KB](https://kb.synology.com/en-global/DSM/tutorial/What_stops_my_Synology_NAS_from_entering_System_Hibernation)).

### Elsewhere

- **Windows** moved from a fixed maintenance hour to opportunistic runs: work runs when the machine
  is idle, is suspended when the user returns, and has deadlines
  ([Microsoft](https://learn.microsoft.com/en-us/windows/win32/taskschd/task-maintenence)). Users
  dislike learned schedules overriding what they set.
- **Android's Doze** holds deferred work and runs it together in a maintenance window, which is the
  closest analogue to waking a disk once
  ([Android](https://developer.android.com/training/monitoring-device-state/doze-standby)).
- **iOS** runs `BGProcessingTask` only when idle, and stops it when the user returns.

### Recommendation

1. **Quiet hours are a veto, and idleness is the trigger.**
   - Plex and Jellyfin choose the night because nobody watches then, and night is exactly when a
     disk near a bedroom is heard.
   - Non-urgent work runs only when nothing is playing, and stops when a playback starts. It never
     runs in quiet hours, on by default, for example 22:00–07:00.
   - Each kind of work has a deadline. Overdue work runs at the next idle time outside quiet hours.
2. **A person's want runs any time:** a new file to show, a subtitle someone asked for, "Scan now".
3. **A sleeping disk is not woken for non-urgent work.** Work is batched per disk. It runs:
   - right after a playback on that disk ends, while the disk still spins; or
   - all at once, when the disk's batch passes a size or its oldest job passes its deadline.
4. **Avoid the known disk-wakers:**
   - remember each failure until its file changes;
   - never poll files on a schedule;
   - keep metadata and artwork off the media disks;
   - over NFS, a scan is polling, so treat it as batched work.
5. **Over a share, the disk's state can't be seen.** A NAS doesn't say whether its disks sleep. The
   node can only avoid being the one that wakes them, so it treats a disk as possibly asleep after a
   while with no I/O from Orpheum.
6. **"Sound-isolated" lifts only the quiet-hours veto.** Night then becomes the preferred time. It
   doesn't lift the rule against waking disks. Ask it as a question about where the storage stands,
   not as a technical switch.
7. **Tell the admin:**
   - the quiet hours;
   - each disk's waiting work and when it is next expected to run;
   - every wake Orpheum caused, and why;
   - that the NAS's own sleep setting decides whether its disks ever sleep.
