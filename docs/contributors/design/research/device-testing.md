> **ABOUTME:** How player projects, media servers and the big streamers test playback on real
> devices and networks, what yoloAI's test machinery offers a project written the same way, and
> which of it a home lab can copy. Evidence for D190.

# Device testing: competitors, yoloAI, and a home lab

- **Date:** 2026-09-23. Requested by the project owner, with a hardware inventory: *"We also
  need to come up with a testing strategy for this. Do research into our competitors, and also
  take a look at what yoloai does. Grab whatever will be useful to us."*
- **Builds on** [testing-strategy.md](testing-strategy.md) (2026-09-20: Jellyfin, Navidrome,
  FFmpeg FATE, hls.js, VMAF, Apple's HLS validators), which this does not repeat. **Corrects**
  [test-hardware.md](test-hardware.md) (see its correction note).
- **Working notes** (verbatim quotes, URLs; [V] read at source, [S] secondary):
  `/yoloai/cache/research/testing-competitors{,-bigco,-tooling}.md`,
  `/yoloai/cache/research/testing-yoloai.md` (yoloAI at an earlier commit, 2026-08-21),
  `/yoloai/cache/research/labhw.md`. Fetches under `/yoloai/cache/http/{testing2,labhw}/`;
  clones under `/yoloai/cache/repos/`.
- **Not reached:** netflixtechblog.com directly (read through the Wayback Machine where
  archived), YouTube's hosted conformance suite and certification PDFs (partner login), any
  first-party QA description from Plex or Emby, and anyone's method for testing watch-party sync.

## 1. The one practice every mature project shares: the device reports on itself

**Nobody who tests playback at scale drives the UI to find out whether video played.** They make
the player report its own state.

| Who | How | Evidence |
| --- | --- | --- |
| **Shaka Player** | the only open project with a real device lab. The device list is YAML in the repo (desktop browsers, Chrome on Android, three Chromecasts, Tizen on; Chromecast V1, ChromeOS and Xbox off, **each with a written reason**). Devices attach through `generic-webdriver-server`, which only knows "open this URL"; the test suite runs in the device's browser and reports results over HTTP. **No UI automation.** | [V] repo |
| **Netflix** | over 40,000 tests a day on over 600 devices (2015); the primary check is events the app itself emits; each partner lab has a box that acts as the devices' router | [V/S] TechBlog via archive |
| **YouTube** | its device conformance suite is a web page that reads dropped-frame counts and posts them | [V] |
| **BBC** | manufacturers must pass a compliance app; its open-sourced device farm (Hive CI, now dormant) has a **recovery ladder**: relaunch the app, reboot, power-cycle | [V] |

[device-lab.md](../plans/device-lab.md) §3 already chose this shape. The research confirms it
and supplies what it lacked: numbers to pass or fail against, and the operating habits of a lab
that has run for years.

## 2. Pass criteria other people already fixed as numbers

| Source | Criterion | Evidence |
| --- | --- | --- |
| **Media3** (ExoPlayer) playback tests | **≤ 1% dropped frames, and ≤ 10 consecutive**. On failure it retries with only the formats the device is *required* to support, which separates "slow device" from "we asked too much" | [V] source |
| **dash.js** functional tests | playback progresses within **15 s**; stops within **3 s** of pause; a seek lands within **0.5 s** of its target | [V] source |
| **Roku** certification | channel launch **≤ 15 s**; key response **≤ 250 ms**; playback start **≤ 8 s** | [V] docs |
| **Shaka** | subtitle and UI rendering compared against **per-platform screenshot goldens** by image similarity, with a per-test threshold | [V] source |

Media3's retry is the most useful single idea. It is [D21](../../decisions/working-notes.md)'s
mandatory baseline used as a *diagnostic*: if the baseline also drops frames, the device is the
problem; if it does not, our choice of rendition was.

## 3. The rest of the field

- **Jellyfin tests no clients.** It tests its play/transcode decision against recorded client
  profile files (307 cases, many tagged with the issue that found them) and posts an API diff on
  every PR. Web, Android TV, Swiftfin and Roku have little or no automated testing [V].
- **Immich has the clearest tiers** [V]:
  - unit tests with mocked storage
  - "medium" tests against a real Postgres
  - full-stack end-to-end tests in Docker on both x86 and ARM
  - Playwright, both against the full stack and with a mocked network
  - its Android emulator tests are commented out (why is not recorded)
- **hls.js** requires two Chrome-on-Windows configurations on SauceLabs; Safari and Firefox on
  BrowserStack are optional. **dash.js** runs one stream on LambdaTest per PR. It has a small
  webOS/Tizen app that loads the test URL, but its Tizen config hard-codes one developer's TV,
  so TV runs are manual [V].
- **mpv, VLC and Kodi** use fuzzing and tiny synthetic media [V]:
  - mpv fuzzes on every PR through OSS-Fuzz; VLC is enrolled; Kodi is not
  - **VLC's `mock://` access module** takes track counts, duration, `can_seek` and error flags
    from the URL, which gives a player test with no file at all
  - Kodi's test clips are 2–3 KB
  - none of the three tests on devices
- **Plex and Emby:** beta channels and forums. No public device lab [S].
- **Google Cast:** receiver testing *"is currently manual"* [V].
- **Sync and voice:** nobody publishes how they test watch-party sync [V].
  - Syncplay has no unit tests.
  - Jellyfin SyncPlay has about 30 server tests covering group state, not measured drift.
  - WebRTC voice *is* testable: Chrome's fake-media flags feed a `.wav` as the microphone, and
    `getStats` exposes jitter, loss and audio level.
  - Jitsi's lip-sync check (a green flash with a beep) is the pattern to borrow for sync.

## 4. yoloAI: what transfers

yoloAI is this project's template and, like it, is written by agents. Its test machinery is
mostly scar tissue, and each piece names the failure that caused it. Ranked by value here:

1. **Missing capability fails; only a named list may be absent.**
   `docs/contributors/design/plans/mandatory-infra-test-policy.md`,
   `internal/testutil/uncontrolled.go`.
   - A test whose prerequisite could exist on this machine and does not **fails**. A CI step may
     name what is legitimately absent (`…_UNCONTROLLED=vaapi,nvenc,landlock`), and that list is
     printed.
   - A test that cannot run on this platform at all is omitted and **listed as uncovered**.
   - `TestRepoHygiene_TestGates_AreSetBySomething` fails when code reads a test-gate variable
     that nothing sets. A tier gated that way self-skipped for months, and two security tests
     had never run.
2. **A check must first show it can fail.** `principles/testing-principles.md` §11,
   `procedures/verification-rounds.md` §3.
   - The specimen: a run reported *"7 PASS / 0 FAIL"* and concluded the exact inverse of the
     truth, because the sandbox lacked the capability under test.
   - **Our planned escape test ([D112](../../decisions/working-notes.md)) has this defect.** A
     denied open inside Docker may be Docker's seccomp, not our Landlock.
   - **Our device lab has it too.** "0 dropped of 0 frames" reads as smooth.
3. **A fake-binary fixture for the tool we shell out to** (`runtime/apple/build_test.go`). It
   checks the arguments we pass, the environment the worker receives, and that errors carry the
   tail of stderr. For us that tool is FFmpeg (does `-force_idr 1` reach NVENC?).
4. **Revert-and-require-red** (`scripts/check_revert_red.py`). For each changed source file, it
   restores the old version in a throwaway worktree and requires some test to fail. It found 4
   real violations in 5 merged agent commits. Its `Verified-By:` trailer is for fixes that only
   a device can confirm, but it has been used **zero** times, so it only works if something
   checks it.
5. **Architecture as named tests.** A load-bearing design claim cites a `TestArch_` test, and a
   gate checks both directions: every cited test exists, and every such test is cited
   (`scripts/check_claim_citations.py`, `writer_fence_test.go`, `internal_leak_fence_test.go`).
   Where the code starts in violation, a baseline list may only shrink.
6. **A verification queue for what cannot be scripted**
   (`design/plans/mac-verification-queue.md`). Each item has a setup, checks, the expected
   result, and ✅/❌ recorded inline. The agent writes code; the owner runs the queue.
7. **Readable lab failures** (`scripts/smoke_test.py`):
   - known error signatures linked to an idiosyncrasies register
   - a diff of a failed run's events against the last good run
   - a check that the device is running the build under test
   - one job per shared resource
   - a disk and reachability pre-flight
   - uniquely named, self-cleaning test resources
8. **One conformance suite, run per implementation**
   (`runtime/runtimetest/conformance_iface.go`). Each implementation declares what it cannot do,
   with a reason, as an explicit SKIP.
9. **Cheap `make check` items:**
   - build the integration- and lab-tagged tests on every check
   - cross-compile for linux/amd64, linux/arm64 and darwin
   - pin tool versions and fail on a missing one
   - run tests in a hermetic environment (git config, time zone, locale)
   - no gate that runs only on pull requests
   - govulncheck, whose suppressions expire when a fix ships
   - every gate script has tests showing it can fail
   - setup output is logged, never discarded
10. **Agent process:**
    - a Stop hook that runs the checks and blocks the turn on failure
    - an `agent-failures.md` with a "Caught by" field
    - a lint for API whose only callers are tests

**Not worth taking:**
- the rest of the 3,090-line `repo_hygiene_test.go` (our `check_docs.py` has the parts that
  matter)
- harness versions frozen as separate files
- the per-OS linting, multi-Docker-provider matrix and privilege handling, which solve problems
  we do not have
- transcript-parsing provenance hooks
- coverage targets, which yoloAI also rejects
- a monolithic release test needing every host

## 5. Tooling a home lab can actually use

| Need | Use | Avoid |
| --- | --- | --- |
| Control Fire TV and Android | `adb` (Fire OS 6–8 are Android) | Appium drivers for Tizen/webOS/Roku (unmaintained since late 2024 or early 2025, and none of those devices are in the lab) |
| Control the Apple TV HD | `pyatv` from Linux; XCUITest on the MacBook only if needed | — |
| Network faults | `tc netem` on a Linux bridge between VLANs (a Proxmox VM); Toxiproxy for HTTP/WebSocket-level faults | `comcast` (a thin `tc` wrapper), KITE (archived), testRTC (commercial) |
| Voice | Playwright with Chrome's fake-media flags and `getStats` | — |
| Did a frame appear, is A/V in sync | the harness's own report first; a USB HDMI capture card only after checking that HDCP does not black it out | paid stb-tester nodes ($4,260 a year each) |
| Browsers we do not own | nothing yet; SauceLabs/BrowserStack only if a Safari version we lack matters | — |

## 6. The lab's facts, verified

Corrections to [test-hardware.md](test-hardware.md), from primary sources unless marked:

- **The 2015 Apple TV is the Apple TV HD (A1625), and it is not H.264-only.** Apple's spec page
  lists *"HEVC SDR up to 1080p, 30 fps, Main/Main 10 profile"* beside H.264 up to 1080p60 [V].
  - It still cannot do 4K, HDR, HEVC above 30 fps or AV1, so it remains the lab's strictest
    client.
  - It stopped at **tvOS 26.6**: tvOS 27 (2026-09-14) dropped it and the Apple TV 4K 1st gen
    [S, MacRumors; Apple's update page still lists 26.6].
  - A frozen OS makes it the proxy for devices that will never update.
- **"Fire TV Stick 4K Max 3rd gen" is the 2nd gen (2023)**: AFTKRT, MT8696T, Fire OS 8
  (Android 11), with AV1, HEVC and VP9 at 4K60 10-bit, HDR10+ and Dolby Vision [V].
- **Fire TV sticks from 2018 onwards** [V]:
  - 4K 1st gen (2018, Fire OS 6): HEVC 4K and Dolby Vision, no AV1
  - Lite, 3rd gen and HD (2020–24, Fire OS 7): HEVC 1080p60 10-bit with HDR, no AV1
  - 4K Max 1st gen (2021): the first with AV1
  - 4K and 4K Max 2nd gen (2023) and 4K Plus (2025): Fire OS 8, with AV1
  - **4K Select (2025) and the 2026 HD run Vega OS**, which is Linux-based, not Android; `adb`
    and sideloading are not documented there [S]
- **The F2-422 is a Celeron J3455, Apollo Lake**, not Gemini Lake [V]. It decodes HEVC 10-bit,
  encodes HEVC 8-bit only, and does no AV1. 8 GB is its maximum.
- **The F4-424 is an Intel N95** (Alder Lake-N) [V]. It decodes AV1, and encodes H.264, HEVC
  10-bit and VP9, with no AV1 encode.
- **The NUC13** is one of the NUC 13 Pro parts, i3-1315U to i7-1370P [V]. Its media engine is
  the same generation as the F4-424's.
- **The staging host with 64 GB is unexplained.** Every staging host variant documents a maximum of 32 GB
  (Kaby Lake) or 8 GB (Gemini Lake) [V]. Identify it with `dmidecode`.
- **The RTX 3090 has 24 GB** [V].
  - NVDEC decodes AV1.
  - NVENC encodes H.264 and HEVC 10-bit, with **no AV1 encode**.
  - NVIDIA's matrix gives consumer cards **12 concurrent encode sessions** [V]; check it with
    `nvidia-smi` on the installed driver.
  - The Ryzen 5 3600 has no integrated GPU.
  - The box runs Linux Mint 22.3 (Ubuntu 24.04 base), not Debian.
- **AV1 decode is common in the lab.** Test-hardware.md implied otherwise. The 3090, NUC13,
  F4-424, M4 and iPhone 15 Pro and later decode it; the S23 probably does [S]. No machine
  encodes it.
- **iPhone Safari** [V]:
  - MSE has never shipped. ManagedMediaSource arrived in iOS 17.1, gated on offering an AirPlay
    source or disabling remote playback.
  - Element fullscreen is iPad-only; iPhone fullscreen is the native player.
  - iOS 27 keeps iOS 26's floor: iPhone 11 / SE 2nd gen and newer.

### What the network equipment can do

| Device | Can | Cannot |
| --- | --- | --- |
| **GL-MT6000** (4.9.1 = OpenWrt 21.02 base; stock OpenWrt 24.10/25.12 also supports it) | netem and shaping modules in GL's feed; SQM; WireGuard server; VLANs in LuCI [V] | SQM together with hardware offload |
| **GL-MT3000** | the same; as a **"remote household" router** — WAN into a lab VLAN for double NAT, netem with IFB on ingress, a `100.64.0.0/10` WAN address as carrier-grade NAT uses, `--random-fully` for strict NAT [S] | remain the 2.4 GHz repeater at the same time |
| **GL-SFT1200** | plain NAT | SQM [V]; netem unconfirmed; a vendor OpenWrt 18.06 fork with 128 MB RAM, not upstream-supported |
| **Zyxel XGS1250-12** | 8×1G, 3× multi-gig to 10G, 1× 10G SFP+; 802.1Q VLANs, a management VLAN, **port mirroring**, priority QoS, link aggregation, IGMP snooping [V] | shaping or per-port rate limits; SSH or SNMP (web-managed only) |

**The simplest remote-household simulator is a Proxmox VM**: two interfaces on two VLANs, NAT
between them and `tc netem` on both, driven by the test runner. The MT3000 is the physical
alternative when the radio matters. The switch's port mirroring lets a capture box record one
client's traffic without touching the client.
