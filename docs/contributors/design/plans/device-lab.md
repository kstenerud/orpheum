> **ABOUTME:** A repeatable performance and capability harness across the whole collection of
> old phones and TV boxes — what it measures, how it avoids the traps of cold hardware, and how
> its output sets the support floor.

# Device lab

- **Status:** PLANNED — designed 2026-09-18.
- **Depends on:** —

## What it produces

One committed, regenerable table: **per device, what it can decode, what it can decode
*smoothly*, and where it falls over.** That table is the input to three things that are
currently guesses — the support floor, the client performance targets, and the per-client
direct-play rates that [D17](../../decisions/working-notes.md) is bet on.

## Scoping correction, stated first

**Phase 0 must not wait for this.** [`playback-survey.md`](playback-survey.md) is deliberately
days of work, and a twenty-device lab is not. The split:

- **Phase 0 uses the web harness (§3) on about five representative devices** — the weakest TV,
  the weakest phone that boots, a modern phone, the Apple TV, one Fire Stick. That answers D17
  and D22 and stays inside its budget.
- **This plan is the full sweep**, built afterwards and in parallel with Phase 1. It turns a
  one-off measurement into a repeatable regression gate and sets the floor from evidence.

Building the lab first would be a textbook way to spend three months not shipping.

## 0. Three facts this lab owes the cast design

[D136](../../decisions/working-notes.md). For every television in the rig, and they are
cheap to collect once a receiver exists:

1. **Does HDMI-CEC reach the receiver at all?** Many televisions implement CEC on one HDMI
   port only, and some implement it without *Deck Control*, which is the part that carries
   transport commands.
2. **Which commands survive?** Deck Control can in principle carry
   play/pause/stop/rewind/fast-forward; reporting at the 2015 Chromecast launch said only play
   and pause worked. That figure is eleven years old and **unverified against current
   firmware** — which is the sort of stale per-device claim this lab exists to replace.
3. **Can the receiver tell a CEC pause from a sender pause?** This is the one that matters,
   because it decides whether `userPausedLocally` can be *reported* or only *inferred* — and
   an inferred one is indistinguishable from a buffering stall, which makes the reconciler
   fight the user.
4. **Does HLS with CMAF/fMP4 load, or only HLS-TS?**
   ([D137](../../decisions/working-notes.md).) `HlsVideoSegmentFormat` defines both and
   `#EXT-X-MEDIA` is documented, but there are reports of fMP4 failing on some CAF receivers.
   If true it collides with [D22](../../decisions/working-notes.md)'s separate-axis output and
   the cast path needs a muxed fallback — the same question
   [playback-survey.md](playback-survey.md) already asks of Tizen and webOS, so ask it once.

## 0b. Her setup, which decides the television path

[D188](../../decisions/working-notes.md). Before choosing between the TV's own browser, our cast receiver, a
sideloaded prototype and the laptop, find out:

1. **What the television is**: a Samsung set running Tizen, or a Google TV device (Chromecast
   with Google TV, Google TV Streamer) plugged into a Samsung. Model and year.
2. **Its browser, if any**, and whether it plays our fMP4 HLS and Opus/AAC with WebVTT.
3. **Whether it accepts a Cast session from Chrome on her MacBook.**
4. **Her phone's voice path**: iOS version, whether she has earbuds, and whether
   `navigator.audioSession` keeps call audio off the earpiece without them (WebKit #230902).
5. **Her uplink** and Wi-Fi at the television, against the ~8 Mbps prepared rendition.

## 0c. The lab, from the shelf

[D190](../../decisions/working-notes.md) §6, from the owner's inventory as verified in
[device-testing.md](../research/device-testing.md) §6. The roster is regenerated from probe
output once the owner's full enumeration arrives.

| Host | Role |
| --- | --- |
| **The development NAS** — F2-422, J3455, 8 GB | the low-end **server floor**: QSV decodes HEVC 10-bit, encodes HEVC 8-bit only, no AV1 |
| **nas** — F4-424, N95, Proxmox | VMs: CI runner, distro install matrix |
| **nuc13** — NUC 13 Pro, 64 GB, Proxmox | VMs: **the remote-household simulator**, a Windows VM when wanted |
| **The GPU test machine** — RTX 3090 24 GB, Mint 22.3 | NVENC conformance; the 12-session limit |
| **MacBook Air M4** | Safari; `mediastreamvalidator`/`hlsreport`; Apple builds; the Chrome sender for Cast |
| **The staging host** | the owner's desktop — **not a lab host**; its browsers are clients |

**Network.**
- **Zyxel XGS1250-12:** an isolated lab VLAN and a separate management VLAN; port mirroring
  feeds packet capture.
- **GL-MT6000:** a lab SSID on the lab VLAN.
- **The remote household** is a Proxmox VM with two VLANs, NAT between them, `tc netem` on both
  and a `100.64.0.0/10` WAN address. It runs three profiles: a good home link, a congested one
  and a lossy mobile one.
- **GL-MT3000:** the physical equivalent when the radio matters.
- **GL-SFT1200:** plain NAT at most.
- A phone on mobile data remains the real carrier-grade-NAT client.

**Clients, reference tier:**
- Apple TV HD, tvOS 26.6 frozen: the strictest client, with H.264 and HEVC SDR at 1080p30 only
- Fire TV Stick 4K Max 2nd gen: the modern ceiling
- one Fire OS 7 stick: the 1080p HDR rung, no AV1
- Fire TV Stick 4K 1st gen, Fire OS 6: the oldest Android base present
- Samsung S23 with Chrome
- a current iPhone with Safari
- the oldest iPhone on iOS 26/27

**Sweep tier:** everything else. **Archive:** iPhones below the iOS floor, kept as frozen
old-Safari clients. **Vega OS sticks:** web only.

**Gap: no Cast device.** Cut A's cast receiver needs one. Buy a current Google TV device after
her setup is known, the same model if she has one.

## 0a. Benchmark the incumbents, don't admire them

[D139](../../decisions/working-notes.md). The lab has the hardware to measure **click depth**
and **focus latency** on Netflix, Apple TV, Roku and Prime Video as well as on ours, and
without those numbers [D138](../../decisions/working-notes.md)'s targets are opinions.

**Roku is the benchmark**: highest-rated in a 2,205-review App Store sample at **3.88★** with
navigation complaints at **3%**, against Plex's 2.04★. Measure what it actually does — presses
from home to playing a chosen title, and milliseconds from D-pad press to the selection
moving — and set our target from that rather than from a feeling.

## 1. Cold hardware has its own failure modes

Twenty devices that have not been powered in years are not twenty test devices until proven
otherwise. Triage before designing around any of them:

- **Swollen batteries are a fire risk.** Check every device for a lifted screen or bowed back
  *before* charging it. Anything swollen gets disposed of, not tested. This is the one item on
  this list that is not merely inconvenient.
- **Deep discharge.** Cells left for years may not recover, and some will charge but hold
  minutes. A device that only works on mains is still useful for a wired rig — note it and move
  on.
- **Activation Lock and FRP.** iCloud-locked iPhones and Android Factory Reset Protection turn
  devices into bricks. Find out early; this may remove a meaningful fraction of the twenty.
- **Expired root certificates.** Devices that stopped receiving updates carry old CA bundles and
  will fail TLS against a modern server — the Let's Encrypt root transition broke a whole
  generation of Android this way. **This will silently break the rig** and look like a network
  bug. The lab serves over plain HTTP on an isolated network, or installs a private CA on each
  device.
- **Dead clocks.** A device with a flat RTC comes up in 1970 and rejects every certificate on
  date grounds. Set time before anything else.
- **Storage full of old junk** and an OS that will not update. Factory-reset what can be reset.

Record all of this per device. "Device did not boot" is a result; "device booted but cannot do
TLS" is a *different* result and mistaking one for the other wastes a day.

## 2. Tier the fleet — most of it does not belong in CI

Twenty devices permanently powered is a fire-safety, battery-degradation and cable-management
problem in exchange for capacity nobody uses.

| Tier | Devices | Connected | Runs |
| --- | --- | --- | --- |
| **Reference** | 3–4: the oldest supported Android, a modern Android, the Apple TV, one Fire Stick | Always | Every build |
| **Sweep** | Everything else | Rotated through 4–6 slots | On demand, and before a release |
| **Archive** | Bricked, locked, swollen, or below the toolchain floor | Never | Recorded and shelved |

## 3. The design decision that makes this affordable: a self-reporting harness

**Do not build per-platform external instrumentation.** Build **one harness application that
plays a corpus, measures itself, and POSTs its results to the lab server.** Identical protocol
on Android, iOS, Fire TV, Apple TV and the browser; the lab server just collects rows.

Two consequences make this clearly right:

1. **Start with the web version and most of the fleet needs no install at all.** Every device
   here has a browser. `navigator.mediaCapabilities.decodingInfo()` gives `supported`, `smooth`
   and `powerEfficient`; a `<video>` element plus `getVideoPlaybackQuality()` gives dropped and
   total frames. No signing, no `adb`, no store, no provisioning profile. This is a very large
   shortcut and it is why Phase 0 can be days.
2. **It is not throwaway.** The harness is the first consumer of the shared client SDK from
   [`web-client.md`](web-client.md) — the same probe-and-pick logic
   ([D21](../../decisions/working-notes.md)) that every real client will use. Writing it early
   exercises the SDK against twenty real devices before a single screen is designed.

**The honest limitation:** a browser's codec support is not the device's codec support. A phone
that decodes HEVC natively may refuse it in its browser. So the web harness **under-reports**,
and is a fast first pass rather than the answer. Native harnesses follow for the devices whose
web results look worse than their hardware should allow.

### What a result must meet

[D190](../../decisions/working-notes.md) §1. Starting values, each frozen as a baseline once measured here
([D110](../../decisions/working-notes.md)):

| Check | Starting criterion | From |
| --- | --- | --- |
| Direct play starts on the lab LAN | ≤ 8 s | Roku |
| Playback progresses, any path | within 15 s | dash.js |
| Dropped frames | ≤ 1% and ≤ 10 consecutive; on failure re-run the mandatory baseline to tell *device* from *our choice* | Media3 |
| A seek lands | within 0.5 s of target | dash.js |
| Pause takes effect | within 3 s | dash.js |
| Remote key to focus moved | ≤ 250 ms | Roku |
| Subtitle rendering | per-platform screenshot golden, similarity threshold per test | Shaka |

**An empty result is invalid.** "0 of 0 frames dropped" means nothing played. Each probe carries
a **negative control**, a stream this device cannot play, and its passes count only after that
control has been seen to fail.

### How the lab runs

From Shaka's, the BBC's and yoloAI's practice ([D190](../../decisions/working-notes.md) §6):
- **Scheduling:**
  - the device roster lives in the repository, with a written reason for every disabled device
  - a nightly run, plus on demand; runs from a change happen only after review
  - one job per device at a time
- **Retries:** up to three, each one recorded, so the flake rate is a number.
- **Pre-flight:** the device is reachable, has disk, and is running **the build under test**.
- **Recovery ladder:** relaunch, then reboot (`adb`, `pyatv`), then power-cycle (`uhubctl` or a
  switched plug).
- **Failure reading:**
  - failure signatures are matched against a device-idiosyncrasies register
  - each failed run is diffed against the last good one
- **What only a person can check** goes into a verification queue: setup, checks, the expected
  result, and ✅/❌ recorded inline.

### Native harnesses, in cost order

- **Android / Fire TV — cheap.** `adb install`, and wireless debugging (Android 11+) removes
  most of the cabling. External instrumentation supplements the self-report:
  `dumpsys gfxinfo <pkg> framestats` for UI frame timing (the last 120 frames, nanosecond
  timestamps), `dumpsys media.metrics` for codec-level frame statistics,
  `dumpsys thermalservice` for thermal state, and `MediaCodecList` for the authoritative
  capability dump.
- **iOS / tvOS — expensive, and worth knowing before committing.** Sideloading with a free Apple
  ID gives **seven-day provisioning profiles and a three-app limit**, which is unworkable for a
  rig; the paid Developer Program gives year-long profiles and a per-device-type registration
  cap. Older devices may also fall below what current Xcode can target at all. Plan the iOS half
  as **manual and selective**, on two or three devices, not as an automated sweep.

## 4. Thermal throttling is a measurement hazard, not a footnote

Sustained video decode heats a phone, the SoC throttles, and results drift downward through a
run. A rig that ignores this produces numbers that depend on test order.

- Enforce a cooldown between runs, and record `dumpsys thermalservice` state with every result
  so throttled samples can be identified rather than silently averaged in.
- Give the rack real airflow. A shelf of phones in a cupboard measures the cupboard.
- Report both a **cold-start** and a **sustained** figure. They answer different questions: cold
  start is what a user feels; sustained is what a two-hour film actually does.

## 5. Power and physical

- **Powered USB hubs with per-port switching**, driven by `uhubctl`, which controls USB power
  per port on hubs that support it and is used for exactly this in device farms. That gives
  scripted power-cycling of a wedged device and, more importantly, control over charging.
- **Do not hold twenty lithium cells at 100% indefinitely.** Charge to roughly half for storage
  and power up for runs. This is a longevity and safety measure, not fussiness — and per-port
  switching is what makes it automatable.
- Label every device with an immutable id that matches its row in the results table. Twenty
  similar black rectangles are otherwise unidentifiable within a week.

## 6. Network

- **An isolated SSID/VLAN** for the lab, so ancient devices with unpatched stacks are not on the
  home network, and so traffic can be shaped without affecting anything else.
- **Bandwidth shaping** (`tc netem` on the lab server, or an OpenWrt AP) to exercise the
  bandwidth-transcode path and ABR switching under constrained links — one of the two drivers of
  transcoding ([tech-playback.md](../research/tech-playback.md) §2b).
- **Plain HTTP or a private CA**, per §1.
- Note that the handsets double as real symmetric-CGNAT clients over mobile data, which the
  lab network cannot simulate — see [remote-access.md](../research/remote-access.md).

## 7. What gets recorded

One row per (device × media profile), committed to the repository:

- **Device:** stable id, model, SoC, OS version and API level, browser version, battery health.
- **Claimed:** decoder support, hardware vs software, `smooth`, `powerEfficient`, max
  resolution/framerate/profile.
- **Measured:** time to first frame; dropped frames as a fraction of total, cold and sustained;
  UI frame timing during library scroll; thermal state at sample time.
- **Behavioural, one boolean and a note each:** does demuxed HLS play
  ([D22](../../decisions/working-notes.md)); does a mid-playback audio-track switch stall the
  video; does a subtitle switch stall it ([D23](../../decisions/working-notes.md)); do ASS
  subtitles render with embedded fonts; do CEA-608/708 captions survive a transcode.

The gap between **claimed** and **measured** is the most valuable column in the table, because
it is exactly what a server-side device profile can never know and what
[D2](../../decisions/working-notes.md) is built on.

## 7a. The locale axis, which is nearly free once the harness exists

Two jobs for the rig that have nothing to do with performance
([D52](../../decisions/working-notes.md), [D53](../../decisions/working-notes.md)).

**Enumerate the fonts.** On every device in the fleet, list the system font set and the Unicode
scripts it covers. This is a few seconds per device and it settles a question D53 currently
guesses at: the Android CDD mandates **no** CJK, Arabic, Hebrew, Thai or Indic font, so a
compliant TV box may render tofu, and we do not know which of the Fire TV sticks and older
Android TV devices actually do. The user's reading of tofu is "the subtitles are broken".

**Screenshot the locales.** Netflix's Hydra catalogues every unique screen across devices *and*
locales by having the existing UI automation post screenshots with metadata; adding a locale is
one line in a property file. Once §3's self-reporting harness exists, the locale axis costs
almost nothing and is the only way anyone has found to make RTL and text-expansion bugs visible
without employing 26 reviewers. Their known gap is worth pre-empting: nothing detects that a
screen *changed*, so review never becomes incremental.

One RTL locale (Arabic or Hebrew) and one CJK locale belong in the standing set. Jellyfin has
the Arabic layout bugs that come from not having them.

## 8. Choosing the floor — three floors, and the binding one is probably not codecs

The stated purpose of the lab is to set the support floor from evidence rather than guesswork.
There are three distinct floors and they should be reported separately:

1. **Toolchain floor** — the oldest OS the client stack can build for at all. Set by Expo /
   `react-native-tvos` minimums, not by us.
2. **UI-performance floor** — the oldest device that renders the library UI at an acceptable
   frame rate. **This is most likely the binding one.** An old phone usually still decodes
   H.264 perfectly well; what fails is running a modern React Native app smoothly.
3. **Codec floor** — the oldest device that plays the mandatory baseline
   (`h264-sdr` + `aac-stereo`, [D21](../../decisions/working-notes.md)) smoothly. Likely the
   *lowest* of the three, which would be a good result: it would mean the menu's baseline is
   correctly chosen and the constraint is our UI, not the format design.

Deciding deliberately matters because the choice **constrains the client stack rather than
following from it** ([tech-clients.md](../research/tech-clients.md) §3a). And the argument is to
set it low: this audience disproportionately runs hardware that commercial services have
abandoned.

## 9. Watch Together and voice

[D190](../../decisions/working-notes.md) §5. Nobody publishes how they test this, so it is designed here.

- **Sync.** Two clients play a clip with a burned-in frame counter and a flash-and-beep every
  ten seconds.
  - **Automated:** the lab server timestamps each client's position reports on arrival and
    computes the drift, bounded by the LAN round trip.
  - **Ground truth:** a verification-queue item, a phone filming both screens and comparing the
    frame counters. It calibrates the automated number and is re-run when the sync code
    changes.
- **Voice.** Playwright with Chrome's fake-media flags feeds a `.wav` as each microphone.
  `getStats` asserts arrival, jitter and loss. A beep's round trip measures the mouth-to-ear
  latency [D65](../../decisions/working-notes.md)'s offset uses.
- **Both run through the remote-household simulator** under its three netem profiles.

## Done when

The table exists, is regenerable by one command against the reference tier, and the three floors
above have numbers. A decision recording the chosen floor and its justification follows from it.
