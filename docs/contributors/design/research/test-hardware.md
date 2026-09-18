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
| TerraMaster F2-422, 8 GB, Debian | **The low-end target** | QSV (Gemini Lake-class): H.264, HEVC 8-bit; no AV1 |

**Verify rather than trust this table.** Per-SKU media-engine capability is exactly the kind of
fact that is wrong from memory. On each box:

```sh
ffmpeg -hide_banner -encoders | grep -E 'nvenc|qsv|vaapi|videotoolbox|amf|v4l2'
ffmpeg -hide_banner -decoders | grep -E 'nvdec|qsv|vaapi|videotoolbox|av1'
vainfo            # Linux/Intel/AMD: the authoritative profile list
nvidia-smi         # driver + NVENC session limits
```

The output of those commands, per machine, is the real matrix and belongs in the repository.

## What this inventory already covers well — better than most projects

- **Three generations of Intel QSV** (Gemini Lake ≈ 2017, Kaby Lake 2017, Alder Lake-N /
  Raptor Lake 2022–23). QSV is the most common hardware path in this field, and a
  three-generation spread across it is genuinely good coverage. Generation-specific QSV bugs are
  a recurring theme in every competitor's tracker.
- **NVENC on a real discrete GPU**, plus the CPU headroom to test software encoding honestly.
- **Apple**: arm64 server build, VideoToolbox, Safari, and the only platform where
  [D3](../../decisions/working-notes.md)'s fMP4 requirement is non-negotiable.
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

### 1. There is not a single television in the inventory — this is the big one

[D10](../../decisions/working-notes.md), [D21](../../decisions/working-notes.md),
[D22](../../decisions/working-notes.md) and [D23](../../decisions/working-notes.md) all rest on
claims about how real TV clients behave, and **none of them can be checked on a desktop**:

- whether a client with a poor capability API can ship a static profile list and be correct (D21);
- whether **demuxed HLS** works on older TV firmware — explicitly recorded as *unverified* and
  as the single assumption D22 rests on;
- whether mid-playback subtitle and audio switching stalls the video (D23);
- whether remote-control focus navigation in React Native is acceptable, which
  [`tv-client.md`](../plans/tv-client.md) says to prototype before writing any screens.

Worse, the project's own conclusion is that **clients are the product** and a great server with
a mediocre TV app loses. We currently cannot test the thing the project will be judged on.

**An Android TV box or stick is the highest-value purchase in the entire project** — roughly the
cost of a takeaway, it is the v1 client target, it sideloads without store approval, and it
unblocks four decisions. Buy this first and it is not close.

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

### 6. No Apple TV

tvOS is a v2 target ([tech-clients.md](tech-clients.md)), so this can wait — but it is the
enthusiast's box and Apple is the platform where fMP4/HLS correctness is least forgiving.

## Recommended order

1. **Android TV device now.** Unblocks D21/D22/D23 verification, the playback survey's client
   half, and the TV focus-navigation prototype that gates the whole client plan.
2. **Nothing else until Phase 0 reports.** The playback survey's format census will say which
   hardware paths actually matter for this library, and that is a better buying guide than a
   capability matrix.
3. **Raspberry Pi** before claiming ARM support.
4. **Apple TV** when the Apple client starts.
5. **AV1-capable GPU** when AV1 output ships, and not before.

## Two things to build with what is already here

- **A per-machine capability report, committed to the repository**, generated by the commands
  above. It is the ground truth the encoder work will be written against, and it turns "does
  QSV do 10-bit HEVC on Kaby Lake?" from a memory into a lookup.
- **The network topology lab on Proxmox**, because
  [`remote-access.md`](remote-access.md) ends with two open questions that only measurement can
  close, and the hardware to close them is already on the desk.
