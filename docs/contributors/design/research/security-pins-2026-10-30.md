> **ABOUTME:** A security audit of every pin Orpheum, orpheum-connect, ffmpeg-fdproxy and
> orpheum-ops carry on 2026-10-30: the Go modules and toolchain, the web lockfile, the container
> images and their OS packages, FFmpeg and the libraries its build pins, and Node; each checked for
> known vulnerabilities fixed in a later version, with whether Orpheum's use reaches it and what to
> do. Report only: no pin changed.

# Security Audit of the Pins, 2026-10-30

- **Date:** 2026-10-30. Asked by the owner ("There have been a lot of scary CVEs lately. Check for
  security updates to our pins.").
- **Method:** `govulncheck -show verbose ./...` (latest, run with `GOTOOLCHAIN=go1.26.10`) in
  orpheum (`main` at 06d6bfb2) and orpheum-connect, which reports whether vulnerable symbols are
  reachable from our code; `npm --prefix web audit --json` (read only); the pinned
  `debian:trixie-slim` digest run with `apt-get update && apt list --upgradable`; OSV.dev's
  `querybatch` for every library jellyfin-ffmpeg's builder pins at v8.1.2-5 (queried by its pinned
  commit), the Debian packages pending, and the Go and npm pins not covered above; FFmpeg's own
  security page; the release indexes of Go, Node, jellyfin-ffmpeg and cloudflared. Responses are
  cached in `/yoloai/cache/osv/`.
- **Limits:** OSV's commit query only finds vulnerabilities in repositories it has git ranges for;
  "none found" for a library it does not cover (lame, from SVN; several Rockchip and Khronos trees)
  means unchecked, not clean. FFmpeg itself is checked against its security page, not OSV.
  MKVToolNix (dev fixtures only) and Playwright's browsers are not in any database checked.

## 1. Summary

**Bump now (1):**

- **`github.com/pion/stun/v3` v3.0.1 → v3.1.5** (GO-2026-6163, GHSA-34rh-wp3j-6cxc,
  CVE-2026-54909). A malformed XOR-MAPPED-ADDRESS attribute panics the parser, and govulncheck
  traces it from our code: `internal/core/turn/relay.go` `turn.Start` → `turn.NewServer` → …
  `stun.XORMappedAddress.GetFromAs`. The embedded TURN relay listens on the public internet through
  the VPS (UDP and TCP), so a crafted packet can panic the core: a remote denial of service of the
  whole server. Same major version; the allowlist already names the module (`deps/go.allow`).

**Bump now if the condition holds, else in the batch:**

- **Vite 8.3.0 → 8.3.5**, if the dev loop runs with `DEV_LISTEN` (Vite then binds the LAN,
  `scripts/dev.py` `web_host`): GHSA-rq7h-w75r-8gcw and GHSA-vfpm-58rq-9qcg let a LAN host read
  files outside `server.fs.allow`, and the sandbox holds SSH keys to nas.example and the test hosts.
- **Mailpit `v1.27` → v1.30.4 or later** (staging only), if staging's Cloudflare tunnel routes a
  hostname to it: 12 advisories, including cross-site WebSocket hijacking that reads every mail
  (invitation links) and unauthenticated crashes. Its compose service publishes no port, so on the
  internal network alone it is batch work.

**Counts per area:**

| Area | Pins checked | Known vulnerabilities in our pins | Reachable or exposed |
| --- | --- | --- | --- |
| Go modules (orpheum, connect) | 38 modules (orpheum), 13 (connect), go1.26.10 stdlib | 9 (orpheum), 1 (connect) | 1 (stun/v3) |
| Go toolchain | go1.26.10 | 0 (latest 1.26.x) | — |
| npm (web) | 358 packages (27 production) | 9 packages: 4 high, 5 moderate | 0 in the shipped bundle; Vite's dev server conditionally |
| Container images, OS packages | 6 images | Debian base: 4 packages pending (13 OpenSSL CVEs, PCRE2, xz); Mailpit 12 | 0 in production; Mailpit conditionally |
| FFmpeg and its libraries | FFmpeg 8.1.2 + 79 pinned libraries | FFmpeg 0; 9 libraries, 26 entries | 0 |
| Node | 22.23.3 | 6 CVEs fixed in 22.23.4 | build and dev only |

## 2. Go modules

govulncheck: orpheum is affected by 1 vulnerability (symbol-level); 8 more are in required modules
our code does not call. orpheum-connect is affected by none; 1 module-level. The go1.26.10 standard
library has no open entry (its HTTP/2 fixes GO-2026-6603/6610/6611/6612/6617 landed in 1.26.9).

| Pin | Current | Fixed | IDs | Severity | Reachable | Recommendation |
| --- | --- | --- | --- | --- | --- | --- |
| github.com/pion/stun/v3 | v3.0.1 | v3.1.5 | GO-2026-6163, GHSA-34rh-wp3j-6cxc, CVE-2026-54909 | CVSS 3.1 5.3 (A:L); in practice a process panic | **Yes**: TURN relay, internet-facing | **Bump now** |
| golang.org/x/net (indirect) | v0.58.0 | v0.60.0 | GO-2026-6603, -6610, -6611, -6612, -6617 (CVE-2026-78659/78660/78669/78663/97032) | HTTP/2 DoS, crash | No: our HTTP/2 is the stdlib's bundled copy, already fixed in go1.26.9 | Bump in the batch |
| golang.org/x/net (indirect) | v0.58.0 | v0.62.0 | GO-2026-6866 (CVE-2026-97045), GO-2026-6860 (CVE-2026-78661) | html package: namespace escape, quadratic | No: `x/net/html` not imported | Bump in the batch (with the above) |
| golang.org/x/crypto | v0.57.0 | none | GO-2026-5932 (openpgp unmaintained) | — | No: `openpgp` not imported (both repos) | No action |
| pion/stun/v4, turn/v4, turn/v5, webrtc/v4, dtls/v3 | as go.mod | — | none found (OSV) | — | — | No action |
| everything else in go.sum (both repos) | as go.sum | — | none found | — | — | No action |

The gate tools (golangci-lint 2.13.2, sqlc 1.31.1): none found in OSV.

## 3. npm (web/package-lock.json)

Every flagged package is a development dependency; none reaches the web client's or cast
receiver's bundle. The production closure (react 19.3.0, react-dom 19.3.0, hls.js 1.7.3 and their
dependencies) has no advisory; hls.js 1.7.4 exists, with none.

| Package | Direct? | Current | Fixed | IDs | Severity | Exposed | Recommendation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| vite | direct | 8.3.0 | 8.3.5 | GHSA-rq7h-w75r-8gcw, GHSA-vfpm-58rq-9qcg, GHSA-9jrq-w75r-8gcw; launch-editor GHSA-587h-7wmj-h5pm, GHSA-r825-w8mm-5pxw, GHSA-698v-4r3x-3vcv (Windows) | high (launch-editor, Windows), moderate | Dev server only; on the LAN when `DEV_LISTEN` is set | Bump now if the dev loop is on the LAN, else batch |
| vitest, @vitest/browser-playwright | direct | 5.0.1 | 5.0.5 | via @vitest/mocker, @vitest/ui, @vitest/browser | moderate | Test runner on loopback | Bump in the batch |
| @vitest/mocker | transitive | 5.0.1 | 5.0.3 | GHSA-qxg6-89mg-rqv8, GHSA-xp26-3454-mjgp | moderate | Test runner | Batch (with vitest) |
| @vitest/ui, @vitest/browser | transitive | 5.0.1 | 5.0.5 | GHSA-p458-7748-mcp3 (`server.fs.deny` misapplied) | moderate | Test runner on loopback | Batch (with vitest) |
| happy-dom | direct | 20.14.5 | 21.0.4 (20.14.6 fixes only the cookie one) | GHSA-834v-8h66-3h79, GHSA-c8p4-j97h-3vg8, GHSA-5fpq-26gf-qwcr | high | Tests parse only our own HTML | Batch; a major bump to 21 |
| source-map-js | transitive (postcss, @eslint/css-tree) | 1.2.1 | 1.2.2 | GHSA-68fv-2mgg-jv7q | high (DoS) | Build tooling on our own sources | Batch (lockfile refresh) |
| zod | transitive (eslint-plugin-react-hooks, @eslint-react) | 4.6.5 | 4.6.6 | GHSA-mrhx-c7xc-m3w5 | high (memory) | Lint on our own sources | Batch (lockfile refresh) |

Playwright 1.63.0: none found. Its browsers are fetched for tests only and browse local pages.

## 4. Container images and OS packages

| Pin | Where | Current | Fixed / newer | IDs | Severity | Exposed | Recommendation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| debian:trixie-slim | deploy/Containerfile, every stage | digest of 2026-09-30 (13.7) | libssl3t64 / openssl 3.5.7-1~deb13u3 | CVE-2026-35189, -35191, -42772, -54872, -54873, -54875, -72897, -75804, -75805, -75806, -77696, -84782, -84784 | up to 7.5 (DTLS, QUIC, TLS server DoS) | No: no runtime process links the system libssl (the binary is Go's TLS, FFmpeg links its own) | Bump the digest in the batch |
| (same) | | | libpcre2-8-0 10.46-1~deb13u3 | CVE-2026-103111 (JIT out-of-bounds write with attacker regex) | high | No: no attacker-supplied regex | Same digest bump |
| (same) | | | liblzma5 5.8.1-1+deb13u2, tzdata 2026e | (Debian security update; no OSV id matched) | — | No | Same digest bump |
| Runtime apt packages (iproute2, wireguard-tools, Intel media driver, libmfx-gen1.2, mesa-va-drivers) | runtime stage | unpinned, current at each image build | — | — | — | Installed fresh per build; the base's own packages are not upgraded (`apt-get install` without `upgrade`) | No action beyond the digest; the owner may want an `apt-get upgrade` in the base stage (a gate change) |
| axllent/mailpit | connect deploy/staging/compose.yaml | v1.27 (floating minor, stays 1.27.x) | 1.30.4 | GHSA-524m-q5m7-79mm (CSWSH, read all mail), GHSA-8v65-47jx-7mfr, GHSA-6jxm-fv7w-rw5j, GHSA-mpf7-p9x7-96r3, GHSA-w4mc-hhc6-xp28 (SSRF), GHSA-54wq-72mp-cq7c, GHSA-fpxj-m5q8-fphw, GHSA-28pq-6qxg-wg5r, GHSA-qx5x-85p8-vg4j, GHSA-w4vj-r5pg-3722, GHSA-75mr-qw9x-3r39, GHSA-w878-pj84-3j5v | up to 7.5 | Staging only; no published port; unknown whether the tunnel routes to it | Batch; now if the tunnel exposes it |
| cloudflare/cloudflared | connect production and staging compose | 2026.10.2 | 2026.10.6 (no security notes) | none found | — | — | No action |
| gcr.io/distroless/static-debian12:nonroot | connectd runtime | floating tag | — | — | — | Static binary; CA bundle and tzdata only | No action |
| busybox:1.37 | connect build stage | floating | — | — | — | Build stage only (`mkdir`) | No action |
| alpine:3.22 | connect staging edge | floating minor | — | — | — | Supported to 2027-05; `apk add` takes current packages | No action |
| MKVToolNix | scripts/fetch_mkvtoolnix.py | 102.0 | — | not checked (no database) | — | Dev image only (test fixtures) | No action |

## 5. FFmpeg and its libraries

Orpheum runs jellyfin-ffmpeg v8.1.2-5 with the fdproxy series (`scripts/fetch_ffmpeg.py`,
ffmpeg-fdproxy v8.1.2-5-2; the macOS dev binary is jellyfin's own v8.1.2-5). What bounds
exposure: every run whitelists only the `file`, `pipe` or `fdproxy` protocol
(`internal/node/execx`, `internal/node/fdproxy`), so no network protocol, TLS library or remote
manifest is reached; no run burns subtitles in or draws text, so libass, FreeType, HarfBuzz and
FriBidi are never called; subtitles converted are text codecs only (`internal/core/decide`
`classify.go`: ass, ssa, subrip, webvtt, mov_text, text), converted by FFmpeg's native ASS and
WebVTT code; encodes are libx264 (and hardware), images libwebp and PNG. The demuxers and decoders
of the media opened are FFmpeg's own (VP9 is decoded natively, not by libvpx; AV1 by dav1d).

FFmpeg itself: 8.1.2 carries the fixes FFmpeg's security page lists for the 8.1 line
(CVE-2026-8461, CVE-2026-30999); no CVE is listed against 8.1.2. FFmpeg 8.1.3 is out
(jellyfin v8.1.3-1, 2026-09-27); point releases carry fuzzer-found parser fixes without CVEs, and
it also moves dav1d, SVT-AV1 and x265 to new commits. Since we open untrusted media, rebase the
series on v8.1.3-1 in the batch.

| Library (builder script) | Pinned commit | Fix commit | IDs | Severity | Reachable | Recommendation |
| --- | --- | --- | --- | --- | --- | --- |
| FFmpeg | 8.1.2 (jellyfin 8.1.2-5) | 8.1.3 (no CVE listed) | — | — | Parses every file | Bump in the batch (v8.1.3-1) |
| zlib (20-zlib) | f9dd6009 | df84af25 | CVE-2026-76844, GHSA-g84c-rxfj-3j2c (gzwrite heap overflow) | CVSS 4.0 high | No: FFmpeg inflates and deflates in memory, never `gzwrite` | No action until jellyfin moves it |
| freetype (25-freetype) | 1803559c | 5a280ecd, f3ca71c9 | CVE-2026-50811 (variable-font OOB read), CVE-2026-95512 (CID DoS) | 6.5, 5.5 | No: no libass or drawtext run; would be reachable through fonts attached to Matroska files if burn-in is ever added | No action now; bump before any burn-in |
| harfbuzz (45-harfbuzz) | 56feae40 | 5e821de8, a71aa86c, 6c2107ac | CVE-2026-113679, CVE-2026-113680 (WebAssembly shaper), OSV-2026-962 (instancer) | high, medium | No: the WebAssembly shaper is not built; the instancer is not used; only libass calls it | No action |
| libxml2 (25-libxml2) | a7511af0 | c2e233fc | CVE-2026-11979; OSV-2021-777, OSV-2024-698 (OSS-Fuzz, no fix range) | low (local) | No: only the DASH and IMF demuxers use it, for manifests we never open | No action |
| libvpx (50-libvpx) | 1024874c | 75d62e84, d5f35ac8 | OSV-2026-55, OSV-2026-97 (encoder) | fuzzer finds | No: encoder paths; we never encode VP8/VP9 | No action |
| zvbi (50-zvbi) | 41477c97 | none yet (through 0.2.45) | CVE-2026-110654 (teletext DRCS OOB write) | CVSS 4.0 medium/high | No: `dvb_teletext` is not a text codec we convert; never decoded | No action; watch for a fix |
| libX11, libXi, libXrender (45-x11) | b10abf6b, 3103b584, f32afe9f | b8d5a704, 9ea632ab, f30d6546 | CVE-2026-88806, -94283, -94284, -94285; CVE-2026-109089, -109090, -93541…-93545, -94281, -94282; CVE-2026-88807 | up to 7.5 | No: parse X server replies; FFmpeg never connects to an X server in the node | No action |
| dav1d, libass, fribidi, openssl 3.6.3, libwebp, libopus, libvorbis, libogg, libtheora, x264, x265, svtav1, libbluray, libudfread, openmpt, srt, chromaprint, fontconfig, zimg, libplacebo, libva, libdrm, libvpl and the rest | as builder | — | none found by OSV | — | — | No action |

OpenSSL 3.6.3, linked into FFmpeg: OSV's commit query found nothing, but the 2026-09-29 OpenSSL
advisories list 3.6.5 as fixed for their 3.6 entries (CVE-2026-35191, -42772, -54873, -54875,
-72897, -75804, -84784). Unreachable here (no network protocol is whitelisted); it moves when
jellyfin moves it.

## 6. Toolchains

| Pin | Current | Newest | IDs | Exposed | Recommendation |
| --- | --- | --- | --- | --- | --- |
| Go (go.mod `toolchain`, both repos) | go1.26.10 | go1.26.10 (go1.27.3 on the next line) | none open | — | No action |
| Node (deploy/Containerfile `NODE_VERSION`) | 22.23.3 | 22.23.4 (security release, 2026-10-20) | CVE-2026-105481, -105482, -106069, -106073, -106074, -111497 (brotli); OpenSSL 3.5.9 | Build, gates, Vite and test servers; not in any runtime image | Bump in the batch (version and both checksums) |
