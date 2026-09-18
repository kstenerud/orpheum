> **ABOUTME:** What hardware this project needs to test against, measured against the inventory
> actually available — the coverage it already gives, the gaps that matter, and what is worth
> buying in what order.

# Test Hardware

Written **2026-09-18** against a stated inventory. Feeds
[`testing-strategy.md`](../plans/testing-strategy.md) and
[`playback-survey.md`](../plans/playback-survey.md).

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

## What this inventory already covers well — better than most projects

- **Three generations of Intel QSV** (Gemini Lake ≈ 2017, Kaby Lake 2017, Alder Lake-N /
  Raptor Lake 2022–23). QSV is the most common hardware path in this field, and a
  three-generation spread across it is genuinely good coverage. Generation-specific QSV bugs are
  a recurring theme in every competitor's tracker.
- **NVENC on a real discrete GPU**, plus the CPU headroom to test software encoding honestly.
- **Apple**: arm64 server build, VideoToolbox, Safari, and the only platform where
  [D3](../../decisions/working-notes.md)'s fMP4 requirement is non-negotiable.
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

**Buy nothing yet.** The inventory now covers every decision that is currently blocked. The
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
