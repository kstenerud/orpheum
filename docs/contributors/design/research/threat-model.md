> **ABOUTME:** The security model this project builds to — adversaries, the trust boundaries,
> and the containment stance for FFmpeg, which is the sharpest edge in the product and is on the
> normal path rather than an edge case.

# Threat Model

Researched **2026-09-18**. Written because we documented Jellyfin's security failures in detail
and then designed a product whose headline feature is exposing a server to non-technical family
— which means Jellyfin's "put it behind a VPN" posture is not available to us and we need our
own.

## 1. Adversaries

| # | Adversary | Reaches us via | Why they bother |
| --- | --- | --- | --- |
| A1 | The internet at large | Any exposed port; scanners find everything | Commodity: botnet recruitment, crypto mining, ransomware pivot into the LAN |
| A2 | A guest we shared with | A legitimate credential or link | Access beyond what was shared; keeping access after revocation |
| A3 | A household member | A legitimate account | Escalation to admin; reaching other users' data |
| A4 | **A media file** | The library itself | The most interesting one — see §2 |
| A5 | A metadata provider or plugin | Outbound requests we make; installed extensions | Supply chain; a hostile scraper response |
| A6 | Someone on the LAN | Local network | Unauthenticated endpoints, DLNA/SSDP, the setup wizard |

A4 is the one the field systematically under-rates, and it is where the rest of this document
goes.

## 2. Media is attacker-supplied data, and it is on the normal path

Every file in the library is untrusted input parsed by FFmpeg — a large C codebase whose
`libavcodec` contains decoders for hundreds of formats, each parsing attacker-controlled
bitstream into heap buffers. FFmpeg has **roughly 494 published CVE records since 2005**, with
recent years running in the **mid-20s to high-30s annually**, dominated by heap and stack
overflows, integer overflows and invalid writes across demuxers, decoders, protocol handlers and
parsers.

**Two things make 2026 different from 2016, and both point the same way:**

1. **Discovery is accelerating because LLM-driven analysis is now pointed at it.** Published
   2026 work describes an AI agent finding 21 zero-days in FFmpeg, and separate AI-assisted
   static analysis reporting seven memory-safety flaws that traditional tools missed. The rate
   is a function of attacker effort, and attacker effort just got cheaper.
2. **Our threat model puts hostile files in the library.** A user who downloads content from
   places that are not a shop is our *typical* user, not an edge case.

So the working assumption is the right one: **FFmpeg will eventually execute attacker code. Plan
for containment, not prevention.** This is not pessimism about FFmpeg; it is the same assumption
every browser vendor makes about their own media stack, which is why Chrome and Firefox run
codecs in sandboxed processes.

Note the chain this completes with something already recorded: Jellyfin's published CVE for
**FFmpeg argument injection through an unauthenticated `/Videos/{id}/stream` reaching arbitrary
file write** ([competitors-video.md](competitors-video.md)). That is the same surface reached
from A1 rather than A4.

## 3. Containment stance: cheapest effective rung first

The constraint is that this must not consume the development budget. Ranked by cost-to-benefit,
not by strength:

| Rung | Mechanism | Dev cost | Ops cost to the user | Protects against |
| --- | --- | --- | --- | --- |
| 0 | **Separate process** (already true) | none — [D19](../../decisions/working-notes.md) | none | Crash isolation only |
| **1** | **Landlock + seccomp-bpf on the child** | **low — a few hundred lines of Go** | **none** | Filesystem reach, network egress, syscall surface |
| 2 | Dedicated unprivileged user + systemd hardening | low | small (packaging) | Same, coarser, older kernels |
| 3 | Container per invocation | medium | requires a container runtime | Adds namespace isolation |
| 4 | microVM (Firecracker) / gVisor | high | heavy | Kernel-surface isolation |
| 5 | Separate physical machine, wiped | none for us — it is [D19](../../decisions/working-notes.md)'s remote transcoder | high for the user | Everything, including kernel escapes |

**Rung 1 is the recommendation, and it is unusually cheap here.** Modern Linux provides
**Landlock** (filesystem and network access control, kernel 5.13+) and **seccomp-bpf** (syscall
filtering) as *unprivileged, per-process* mechanisms that need **no root, no namespaces and no
cgroups** — a process can be confined about as tightly as a container without a container
runtime being installed. Maintained Go bindings exist for both (`landlock-lsm/go-landlock`,
`elastic/go-seccomp-bpf`).

The policy an FFmpeg child actually needs is small enough to be auditable: read the one input
file, write into one cache directory, and — for the probe, subtitle and thumbnail paths —
**no network at all**. A decoder that gets compromised while confined that way can corrupt one
cache directory and nothing else. That is most of the value for a fraction of the cost.

Rungs 3–5 stay available as **opt-in** for users who want them, and rung 5 is free: it is
already the architecture, because [D19](../../decisions/working-notes.md) lets the transcoder
run on another machine.

## 4. The consequence: the sandbox boundary and the plugin boundary want to be the same boundary

This is the finding that changes the architecture rather than just adding a mitigation.

[D22](../../decisions/working-notes.md) put audio transcoding in the core because the licence
boundary falls there. But **the trust boundary does not follow the licence boundary** — probe,
demux, subtitle extraction, thumbnailing and audio encoding *all* parse hostile input, and all
of them were going to run in the core process.

The resolution is to separate the two concerns cleanly:

- **Packaging** follows the licence: audio encoding ships in the core distribution; video
  encoding ships as the separately-installed plugin (D16, D22 unchanged).
- **Process** follows trust: **no FFmpeg invocation ever happens in the core process.** All of
  it — probe, demux, subtitle extract, thumbnail, audio encode — runs in a confined **media
  worker** child, which is the same shape as the video transcoder and can use the same
  interface.

The core then never parses attacker-controlled bytes at all. It handles HTTP, SQL and JSON, and
delegates every encounter with a media file. That is a much stronger statement than "we
sandboxed the transcoder", it unifies the worker interface from D19 with the trust boundary, and
it costs a process boundary we were already going to build.

## 4a. Fonts are a second attacker-supplied parser, on a second machine

Media is attacker-supplied data (§2) and so is anything extracted from it. Fonts deserve their
own line because the exposure is shaped differently from FFmpeg's: the parser is **FreeType on
the client**, not FFmpeg on the server, so the containment of §3 does not reach it and the
server acts as an **amplifier** — one bad file reaches every screen in the house.

Two FreeType bugs are in CISA's Known Exploited Vulnerabilities catalogue: **CVE-2020-15999**
(CVSS 9.6, heap overflow on PNG images embedded in fonts, part of an exploit chain) and
**CVE-2025-27363** (CVSS 8.1, out-of-bounds write in TrueType GX/variable subglyph parsing,
arbitrary code execution, FreeType ≤ 2.13.0 — which describes the old TV boxes). The second is
from March 2025.

The conclusion is [D56](../../decisions/working-notes.md): embedded fonts are **off by default**
and the client must ask, because only the client knows whether its FreeType is sandboxed (WASM
in the browser) or in-process (libmpv on Android and Apple), and only the client knows its
version. Netflix's subtitle spec allows one generic family and no named fonts, from vetted
studio partners; Jellyfin loads every attachment whose container-declared MIME type looks like a
font, by default. Full evidence in
[internationalisation.md](internationalisation.md) §10.

## 5. The rest of the model, briefly

Each of these is a Jellyfin failure we have already documented, restated as a requirement:

- **Authenticated by default, with a deny-by-default router.** Jellyfin's cluster of
  unauthenticated endpoints (`GetUser`, `GetUsers`, `GetAttachment`, all of `AudioController`,
  the stream controllers, most of `DashboardController`) is what happens when authentication is
  opt-in per handler. Make the default deny and make anonymous access an explicit, greppable
  annotation.
- **No credential in a URL.** Jellyfin's `Copy Stream URL` leaks the session key. Media URLs are
  **signed, scoped and expiring** — which is also required by [D19](../../decisions/working-notes.md)
  for redirecting to a transcoder without creating an open proxy.
- **Scoped, expiring API keys.** Non-expiring dashboard keys with full access are a standing
  Jellyfin complaint (+23 for granular permissions).
- **Never trust a source IP for authorisation.** Jellyfin's unauthenticated `/System/Restart` was
  reachable by spoofing a LAN address. "Is LAN" is not an authorisation decision.
- **The setup wizard is a takeover window.** An uninitialised instance that anyone can claim is
  a real reported failure; first-run must require a locally-obtained secret.
- **Plugins are sandboxed by construction** ([D12](../../decisions/working-notes.md),
  [D15](../../decisions/working-notes.md)) — and note that tier 2 and tier 3 extensions are
  separate processes, so the plugin trust story and the media trust story are the same story.
- **Media is read-only by default** ([D7](../../decisions/working-notes.md)), which caps the
  blast radius of everything above.

## 6. Open

- **Landlock's floor is kernel 5.13**, and network control arrived later still. What happens on
  older kernels, on macOS and on Windows? The answer is probably rung 2 and an honest capability
  report rather than silent absence — the user should be able to see whether their install is
  confined.
- **How is the confinement tested?** A sandbox nobody verified is a comment. This belongs in
  [`testing-strategy.md`](../plans/testing-strategy.md) as an actual escape attempt, not a
  checklist item.
- **Signed URL design** — key rotation, clock skew, revocation on share withdrawal. Couples to
  [`sharing-model.md`](../plans/sharing-model.md).
- **Do we ship a hardened FFmpeg build** (`--disable-` the decoders nobody needs)? Reducing the
  parser surface is free at build time, and the set of formats a home library actually contains
  is much smaller than the set FFmpeg supports. Needs the playback survey's format census to
  answer.
