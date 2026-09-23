> **ABOUTME:** What hardware this project needs to test against, measured against the inventory
> actually available — the coverage it already gives, the gaps that matter, and what is worth
> buying in what order.

# Test Hardware

Written **2026-09-18** against a stated inventory. Feeds
[`testing-strategy.md`](../plans/testing-strategy.md) and
[`playback-survey.md`](../plans/playback-survey.md).

> **Corrected 2026-09-23** against the owner's exact inventory and primary sources
> ([device-testing.md](device-testing.md) §6). The table below is kept as written; where it
> disagrees, §6 is right:
> - **The Apple TV is the Apple TV HD (A1625)**, which decodes **HEVC SDR 1080p30** as well as
>   H.264, so it is not the H.264-only client described here. It is frozen at tvOS 26.6.
> - **The F2-422 is Apollo Lake** (J3455), not Gemini Lake.
> - **The staging host reportedly has 64 GB**, more than any staging host documents, so identify it.
> - **The Ryzen box runs Mint, with a 24 GB RTX 3090.**
> - **The Fire TV ladder starts at 2018**, has a 1080p HDR rung, and its newest sticks may run
>   Vega OS.
> - **AV1 decode is available on five machines**, not on none.
>
> The lab built from the corrected inventory is in
> [device-lab.md](../plans/device-lab.md) §0c.

## Available inventory

| Machine | Role it plays | Expected media engine |
| --- | --- | --- |
| MacBook Air M4, 32 GB | Apple client + arm64 server + Safari/AVPlayer | VideoToolbox: H.264 + HEVC encode; AV1 **decode** only |
| Ryzen 5 3600 + RTX 3090, 32 GB, Debian | Beefy server; NVENC/NVDEC | NVENC (Ampere): H.264 + HEVC incl. 10-bit; **no AV1 encode**; NVDEC AV1 decode |
| staging host, 32 GB, Ubuntu | Old Intel; the QSV floor | QSV (Kaby Lake): H.264 + HEVC encode; no AV1 |
| NUC13, 64 GB, Proxmox | Modern Intel; VM lab | QSV (Raptor Lake, Xe-LP): H.264 + HEVC; AV1 **decode**; no AV1 encode |
| TerraMaster F4-424, 32 GB, Proxmox | Realistic NAS; VM lab | QSV (Alder Lake-N): H.264 + HEVC; AV1 decode; no AV1 encode |
| TerraMaster F2-422, 8 GB, Debian | **The low-end server target** | QSV (Gemini Lake-class): H.264, HEVC 8-bit; no AV1 |
| Apple TV with touchpad Siri Remote | **tvOS client, at the conservative floor** | Either A1625 (Apple TV HD, A8, **H.264 only — no HEVC**) or A1842 (4K 1st gen, A10X, HEVC 4K HDR). **Identify it** — the two test very different things. |
| 5–6 Fire TV Sticks, several generations | **Android-family TV clients, as a capability ladder** | Spans H.264-1080p-only (gen 1) → HEVC 1080p30 8-bit (gen 2) → HEVC 4K HDR (4K) → **AV1 4K60 10-bit** (4K Max, MT8696) |
| A collection of old phones — iPhones of various vintages; Pixel 3a, a contemporary Motorola, ruggedised Androids, Samsung S23 | **Handset clients across a decade, and the only devices that move between networks** | HEVC decode on iPhone from A9 (2015) and on essentially all modern Android; AV1 decode only on the newest of each (S23-class Android; A17 Pro and later on iPhone) |

**Verify rather than trust this table.** Per-SKU media-engine capability is exactly the kind of
fact that is wrong from memory. On each box:

```sh
ffmpeg -hide_banner -encoders | grep -E 'nvenc|qsv|vaapi|videotoolbox|amf|v4l2'
ffmpeg -hide_banner -decoders | grep -E 'nvdec|qsv|vaapi|videotoolbox|av1'
vainfo            # Linux/Intel/AMD: the authoritative profile list
nvidia-smi         # driver + NVENC session limits
```

The output of those commands, per machine, is the real matrix and belongs in the repository.

## The television half — better than buying something

The Apple TV and the Fire TV sticks close what was ranked as the project's largest hardware gap,
and they do it better than a single new device would, because **what makes them valuable is
that they are old**.

### The Fire TV sticks are a complete capability ladder

Across generations, Fire TV hardware spans almost exactly our output menu
([D21](../../decisions/working-notes.md)):

| Generation | Video decode |
| --- | --- |
| Stick (1st gen) | H.264 only, 1080p max — no HEVC, no VP9 |
| Stick (2nd gen) | HEVC to 1080p30, Main Profile L4.0, **8-bit only** |
| Stick 4K (2018) | HEVC 4K, HDR10 / Dolby Vision |
| **Stick 4K Max** (MT8696) | **AV1 to 4K60, Main L5.1, 8- and 10-bit, HDR10/HDR10+/HLG**, plus HEVC Main10 L5.1 |

Five or six of these across the years very likely covers every rung. That means the menu can be
validated end to end on real silicon — including `av1-sdr`/`av1-hdr10` **decode**, which no
other machine in the inventory can do, so AV1 output can be proven with a software encoder long
before an AV1 encoder is bought.

They are also memory- and CPU-starved, which makes a Fire Stick the **client-side equivalent of
the F2-422**: the box where a bloated UI falls over. And they are a primary target rather than a
compromise — cheap streaming sticks are the community's standard answer to bad smart-TV
platforms.

One more thing worth noticing: Netflix has been dropping support for older Fire TV devices. That
is our audience's situation exactly — working hardware abandoned by the services — and a server
that still plays to a 2016 stick is a real differentiator, not just a test case.

### The Apple TV's value depends on which one it is, and both are useful

Check Settings → General → About, or the model number on the base:

- **A1625 — Apple TV HD (4th gen, 2015, A8).** The A8 has **no HEVC hardware decode**: H.264,
  MPEG-4 and MJPEG only, 1080p. This is the single most useful client in the whole inventory for
  validating [D21](../../decisions/working-notes.md), because it is a real, current-OS device
  that can play *only the mandatory baseline*. If the menu and its negotiation are wrong, this
  device finds out immediately.
- **A1842 — Apple TV 4K (1st gen, 2017, A10X).** HEVC including 4K HDR and Dolby Vision. Its
  last major OS was **tvOS 26 (2025)**, so it is now frozen — which makes it a good proxy for
  the long tail of devices that will never receive another update.

Either way this is the platform where [D3](../../decisions/working-notes.md)'s fMP4 requirement
is least forgiving, and both carry the first-generation touchpad Siri Remote — genuinely
unpleasant to navigate with, which makes it an honest test of the focus design rather than a
flattering one.

### The caveat: Fire OS is not Android TV

Fire OS is an Android fork, so most of the client works and sideloading over ADB is
straightforward. What it does **not** give us:

- Google Play Services, and therefore the Google Cast sender/receiver integration;
- the Android TV / Google TV home-screen channels and recommendations APIs (Fire TV has its own,
  different ones — Streamyfin ships a `tv-recommendations` native module that would not apply);
- Play Store distribution paths.

`react-native-tvos` itself should be fine, since it is plain Android underneath. So a Google TV
device is still worth having **eventually**, for platform integration rather than playback — but
it has moved from "the highest-value purchase in the project" to "nice to have".

## The phone collection tests four things nothing else can

Deliberately-kept old handsets are an unusual asset, and their value here is not mostly about
codecs — the TVs already give a capability ladder. It is about the things that are **only true
of a device that moves**.

### 1. The audio path is dynamic, and a phone is where that is routine

[D22](../../decisions/working-notes.md) claims audio capability changes mid-session in a way
video capability does not — the same device answers differently for HDMI/ARC into a receiver,
TV speakers, or Bluetooth. On a TV that is an occasional event. On a phone it is constant:
speaker → wired → Bluetooth → car, several times a day, mid-playback.

**A phone is therefore the best available test of D22's dynamic-audio claim**, and of whether a
mid-session audio-rendition change is genuinely cheap. Android exposes this through
`AudioManager` / `AudioDeviceInfo`; iOS through `AVAudioSession` route-change notifications.

### 2. A phone on mobile data is a free, real CGNAT client

[remote-access.md](remote-access.md) ends by saying CGNAT prevalence needs primary measurement
and that no NAT-traversal stack is chosen. **Mobile carriers deploy carrier-grade NAT
universally**, and typically symmetric NAT at that — which is exactly the hard case where
hole-punching fails and a relay is required.

So a handset on cellular, talking to a server at home, is the canonical hard topology, available
for nothing and without simulating anything. Combined with the Proxmox lab for the *server* side,
that is a complete remote-access test rig.

### 3. Local-versus-remote switching, which is a real and common bug

Immich carries `Automatic local URL switching frequently fails` at +23 reactions. It is a
phone-shaped problem by construction: the device is on the LAN, then it is not, and something
must notice and switch endpoints without interrupting playback. It cannot be tested on a
machine that never leaves the desk.

### 4. Offline download, and therefore the change journal

[D10](../../decisions/working-notes.md) makes offline download a non-negotiable client feature,
and [D8](../../decisions/working-notes-archive.md) put an audit-table change journal in the first schema
specifically so offline clients can reconcile deletes. **Neither has a consumer until a mobile
client exists**, and both are exactly the kind of design that looks fine until a real device
with intermittent connectivity and full storage disagrees. Immich's architecture carries audit
tables for this reason.

Older handsets also make storage pressure and background-download interruption easy to
reproduce, which is where these features actually fail.

### Twenty devices is a lab, and a lab is a project

With roughly twenty handsets, most unpowered for years, this stops being an inventory and
becomes an engineering task with its own failure modes — swollen cells, activation locks, dead
clocks, and **expired root certificates that will break TLS to a modern server and look like a
network bug**. See [`device-lab.md`](../plans/device-lab.md) for the design, the triage
procedure and the tiering.

Two things from it are worth stating here because they change how the rest of this document
should be read. **The full sweep must not gate Phase 0** — the playback survey uses a web
harness on about five representative devices and stays days-long. And **the harness is a
self-reporting app, not per-platform external instrumentation**, which means most of the fleet
needs no install at all: every one of these devices has a browser, and
`mediaCapabilities.decodingInfo()` plus `getVideoPlaybackQuality()` covers the first pass.

### The constraint to establish first: what is our deployment floor?

Old phones only test what they can run. Both toolchains have minimums — Expo and
`react-native-tvos` carry their own iOS and Android API-level floors — and some of the
collection may be below them.

**Enumerate the OS versions before planning around the devices**, because the answer sets a
product decision nobody has made yet: *how old a handset do we support?* That choice constrains
the client stack ([D10](../../decisions/working-notes.md)) rather than following from it, and it
is better made deliberately than discovered when a build refuses to install.

There are really **three** floors and they should be reported separately: the **toolchain**
floor (what Expo / `react-native-tvos` can build for at all), the **UI-performance** floor (the
oldest device that renders the library at an acceptable frame rate), and the **codec** floor
(the oldest device that plays the D21 baseline smoothly). The binding one is most likely the
UI-performance floor — an old phone usually still decodes H.264 fine; what fails is running a
modern React Native app on it.

- Android: `adb shell getprop ro.build.version.release` and `ro.build.version.sdk`; codec truth
  from `adb shell cmd media.codec` or a `MediaCodecList` dump.
- iOS: the Settings → General → About version, and `VTIsHardwareDecodeSupported()` from a small
  test app for the codec truth — there is no `adb` equivalent.

## What this inventory already covers well — better than most projects

- **Three generations of Intel QSV** (Gemini Lake ≈ 2017, Kaby Lake 2017, Alder Lake-N /
  Raptor Lake 2022–23). QSV is the most common hardware path in this field, and a
  three-generation spread across it is genuinely good coverage. Generation-specific QSV bugs are
  a recurring theme in every competitor's tracker.
- **NVENC on a real discrete GPU**, plus the CPU headroom to test software encoding honestly.
- **Apple**: arm64 server build, VideoToolbox, Safari, and the only platform where
  [D3](../../decisions/working-notes.md)'s fMP4 requirement is non-negotiable.
- **Handset clients across roughly a decade**, covered above — including the only devices in the
  inventory that change networks and audio routes while playing.
- **A television capability ladder**, covered above — and the unglamorous ends of it (an
  H.264-only Apple TV, a 1080p-only Fire Stick) are worth more here than a current flagship.
- **Two Proxmox hosts.** Under-rated: this is a **network topology lab**. The remote-access
  research ends by saying CGNAT prevalence needs primary measurement and that no
  NAT-traversal stack is chosen — both are testable here by standing up VMs behind simulated
  symmetric NAT, double NAT, and IPv6-only, which is otherwise very hard to arrange.
- **The F2-422 with 8 GB is the most valuable machine in the list for this project.** An old
  Celeron with 8 GB is the honest low end of the audience, and it is what will catch the memory
  ceilings and query regressions that every other box hides. Jellyfin shipped 30–50 second home
  pages partly because nothing in the loop was slow enough to notice
  ([`performance-targets.md`](../plans/performance-targets.md)). Most projects do not have one
  of these. Treat it as a gate, not a curiosity.

## The gaps, ranked by what they block

### 1. Google TV / Android TV proper — platform integration only

Downgraded from blocking. Fire TV covers the playback and UI half; what is missing is Play
Services, Google Cast, and the Android TV home-screen channel APIs. Worth buying when the cast
receiver or home-screen integration is actually being built, not before.

### 2. No AV1 hardware encoder anywhere

Every machine listed predates AV1 encode: Ampere is one generation short (Ada/RTX 40+), and all
the Intel parts predate Arc/Meteor Lake. Apple silicon has never had one.

This matters only when we actually ship the `av1-sdr` / `av1-hdr10` menu entries, which is
Phase 4 at the earliest, and software SVT-AV1 covers correctness in the meantime — slowly.
**Not urgent; do not buy hardware for it yet.**

### 3. No AMD GPU → AMF untested

A real gap in the hardware matrix, but AMD discrete GPUs are less common in this audience than
Intel iGPUs and NVIDIA cards. Defer.

### 4. No ARM SBC → V4L2 / RKMPP untested

A Raspberry Pi is a very common host for this class of software and exercises a completely
different media path. Cheap. Worth having before claiming ARM support, not before then — note
the M4 already covers arm64 *compilation*, just not ARM SBC media paths.

### 5. No Windows

Blocks the desktop client and Windows-hosted server testing. A Proxmox VM covers most of it
without buying anything.

## Recommended order

**Buy nothing yet**, and the case is now stronger than it was: server hardware across three
QSV generations plus NVENC and Apple silicon, a television capability ladder from H.264-1080p to
AV1-4K, handsets across a decade including a real CGNAT client, and an 8 GB Celeron and a Fire
Stick as the two honest floors. The inventory covers every decision currently blocked. The
playback survey's format census is a better buying guide than any capability matrix, so let
Phase 0 report first.

After that, in order: **Google TV device** when Cast or home-screen integration is built;
**Raspberry Pi** before claiming ARM support; **AV1-capable GPU** when AV1 *output* ships, and
not before — the 4K Max already lets us prove AV1 decode against a software encoder.

## One measurement-design consequence, from the Apple TV

[D17](../../decisions/working-notes.md) bets that direct play covers the common case and video
transcoding can be deferred. An A8 Apple TV — H.264 only, against a library that is
substantially HEVC — is the counter-example living in the house.

It does not overturn D17, because the core still ships direct-play plus audio transcoding. But
it sharpens what Phase 0 must report: **the direct-play rate is per-client, and the plan has to
survive the worst client in the household, not the mean.** One old device means video
transcoding matters, and its priority is set by the *oldest* screen someone actually watches on.
The survey must therefore report per-client rates and name the worst, not average them.

## Two things to build with what is already here

- **A per-machine capability report, committed to the repository**, generated by the commands
  above. It is the ground truth the encoder work will be written against, and it turns "does
  QSV do 10-bit HEVC on Kaby Lake?" from a memory into a lookup.
- **The network topology lab on Proxmox**, because
  [`remote-access.md`](remote-access.md) ends with two open questions that only measurement can
  close, and the hardware to close them is already on the desk.
