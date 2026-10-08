> **ABOUTME:** Checks that only a person with the hardware can do — a film on a television, a
> voice call across two houses. A fix that no automated test can turn red names its item here
> in a `Verified-By: VQ<n>` trailer, and the revert-red gate accepts it only once it is ticked.

# Verification queue

[D190](decisions/working-notes.md#d190--the-testing-strategy-devices-report-on-themselves-against-numbers-a-missing-capability-fails-every-check-first-shows-it-can-fail-and-the-lab-is-built-from-what-is-on-the-shelf)
requires every behaviour change to be able to turn a test red when reverted. A few cannot: the
symptom shows only on a device, a network or a room. For those, the change adds an item here,
someone does it, ticks it, and the commit carries `Verified-By: VQ<n>`.
`scripts/check_revert_red.py` refuses the trailer if the item is missing or unticked. yoloAI's
free-text trailer was never used, because nothing checked it.

**Format**, one line per item, IDs never reused:

```
- [ ] **VQ<n>** — <what to do, on which device, and what "passes" means> (<commit or branch>)
- [x] **VQ<n>** — … — done <date> by <who>: <what was seen>
```

An item is ticked only by the person who did it, and only with what they saw.

## Items

- [ ] **VQ1** — On devnas (J3455), run
  `go test -run '^$' -bench HashPassword -benchtime 10x ./internal/core/auth` and read
  `BenchmarkHashPassword_DefaultParameters`. Passes at 500 ms/op or less
  ([D194](decisions/working-notes.md#d194--secrets-our-users-passwords-are-hashed-third-party-secrets-are-sealed-with-a-key-that-never-enters-the-database-and-nothing-reads-a-secret-back-out)
  §2); above it, the memory cost comes down to OWASP's 46 MiB, t = 1. For reference, an
  i3-N305 VM measured 50 ms. (M1)
- [x] **VQ2** — In Firefox on the dev loop, play a prepared film or episode with sound in its
  centre (any dev-library MKV but the stereo ones), and tick and untick **Dialogue boost**
  while it plays. Passes when the change is heard within about a second of the click, with at
  most a moment's pause. Before this change it took two to four seconds whatever the segment
  length. (the seek after an audio switch in `web/packages/player/src/hls.ts`) — done
  2026-09-26 by the owner: *"Yup, works within about a second now."*
- [x] **VQ3** — On an Android phone in Chrome, and in desktop Firefox, on the dev loop, play
  **Remux Film (2021)** and choose **English** under **Subtitles**. Passes when the text
  appears over the picture, styled as its ASS says, inside the picture's area and not under the
  controls, within a few seconds of choosing it. Then press **Later** three times: the offset
  reads +0.3 s and the text moves at once, paused or playing. Leave and play the film again:
  it starts at +0.3 s. (JASSUB in `web/packages/player/src/subtitles.ts`) — done 2026-09-26 by the owner, on an Android phone in Chrome: *"Subtitles render, and I've
  observed the subtitles arriving later and earlier when I adjust the offset"*, and the offset
  was still set after the page reloaded. Desktop Firefox not yet seen.
- [x] **VQ4** — On an Android phone in Chrome, on the dev loop, play **Remux Film (2021)**, let it
  play past 0:20, pause, and leave the page. Play it again: it starts where it was paused, with no
  question asked. Then press **Start over**: it plays from 0:00. On a second device signed in as
  the same person, play it past 0:25 and leave; on the phone, play it again: it starts near 0:25,
  the later watching, not the earlier. (`web/apps/web/src/play-state.ts`) — done 2026-09-26 by the owner, on an
  Android phone in Chrome, after `5da8b69`: *"Yup works correctly with pause and without"*; the
  second device's part not yet seen.
- [x] **VQ5** — On an Android phone in Chrome, on the dev loop, play two films partway and leave
  each. Open the home screen (Films): **Continue watching** shows both, the last watched first,
  each with how far along it is, and touching one plays it from where it was left. Press **Hide**
  on one: it goes, with **Undo**, which brings it back. Hide it again and play it a little: it is
  back by itself. The row scrolls sideways and fits the phone's width.
  (`web/apps/web/src/continue-watching.tsx`) — done 2026-09-26 by the owner, on an
  Android phone in Chrome: *"Browser test on Android worked fine. With 3 partially viewed films
  the row showed a scroll bar that matched the width, so all looks good."*
- [x] **VQ6** — On an Android phone in Chrome, on the dev loop, play **Remux Film (2021)**,
  choose **Spanish** under **Subtitles**, and leave. Play it again: Spanish is chosen and drawn
  from the start. Choose **Off**, leave, and play it again: subtitles are off. Then switch apps for
  a while and come back: after the page reloads, the choice is still there.
  (`web/apps/web/src/subtitles.tsx`) — done 2026-09-26 by the owner, on an
  Android phone in Chrome: *"Verified on the phone."*
- [ ] **VQ7** — On the dev loop, as owner on a computer, open **People**, add a person, and send
  the link shown to an Android phone. On the phone, open it: choose a password, and it signs in.
  On the phone, open **Your account**, name a device and make a code. On a television's browser
  (or a second browser), open the server, choose **I have a code**, and type the code in capitals
  with a space for the dash: it signs in as that person. The code does not work a second time.
  Back on the computer, **New link** for the person, and the old link no longer works. On the
  phone, **Your devices** lists the phone and the television; while a code is showing, let the
  phone sleep and wake it: the code is still there. **Remove** the television: its next page
  load asks it to sign in.
  (`web/apps/web/src/people.tsx`, `register.tsx`, `code-sign-in.tsx`, `account.tsx`)
- [x] **VQ8** — On the dev loop, on the Android phone, play a film: before it plays, it asks how
  the device plays sound. Choose **Headphones**; a film with 5.1 sound then plays through a
  prepared copy, saying the device plays stereo, with the **Dialogue boost** switch. Turn the
  boost on, go back, and play another 5.1 film: the switch starts on, and the voices are raised.
  Under the video, **Change** the sound to **Soundbar or receiver**: a film the browser can
  play plays as it is, with no switch; one prepared for its container or sound (an MKV, TrueHD)
  keeps the switch, and the boost works on it. In **Your account**, the device's sound shows the same and can be changed there
  (D222). (`web/apps/web/src/sound.tsx`, `play.tsx`, `your-devices.tsx`) — done 2026-09-27 by
  the owner, on an Android phone: *"Everything works as expected. when I select sound bar, I
  still see the dialogue boost button, and it boosts the channel"*; the receiver step was
  corrected to that (A11), and the owner kept the switch: *"Yes we keep the switch"*.
- [ ] **VQ9** — On devnas, with Landlock switched on at boot (`cat /sys/kernel/security/lsm` lists
  `landlock`), run `go test ./internal/node/confine/ -run TestEscape -v` from the tree: both
  escape tests pass, and neither is skipped. Then start the node there and open **System**:
  it says FFmpeg runs in its sandbox on devnas, and doctor's confinement
  finding is ok. M3's acceptance asks for the escape test green on devnas (cut-a §8, D112).
  (`internal/node/confine/`)
- [ ] **VQ10** — On devnas, start the node and open **System**, as admin. Under the machine,
  **Hardware encoding** says video is encoded on the GPU with Intel Quick Sync; **What it sees**
  lists `/dev/dri/renderD128` with its driver (i915 or xe) and says the server may open it; the
  Quick Sync trial worked, and doctor's hardware finding is OK. Play a film whose video the
  browser cannot play (HEVC 10-bit on a phone): it prepares and plays, `intel_gpu_top` shows the
  video engine busy meanwhile, and afterwards **Recent prepared copies** lists it made with Intel
  Quick Sync, faster than real time. With Landlock on (VQ9), all of it holds: the GPU is granted
  to the confined encode (cut-a §6, D225). (`internal/node/prepare/encoder.go`,
  `internal/node/hardware/`, `internal/node/confine/landlock_linux.go`,
  `web/apps/web/src/system.tsx`)
- [ ] **VQ11** — On the dev loop, as admin, open **Settings** and set the OpenSubtitles username
  and password. Play a film that has no German subtitles, and under the video choose **Find
  subtitles…**, pick German and **Search OpenSubtitles**: it says how many of the day's downloads
  are left (20 on a free account), says none is checked against the sound, and lists what
  OpenSubtitles has, a copy **made for this very file** first when there is one. **Download** one:
  the film plays again with it chosen, named "downloaded, unverified" in the menu, and the count
  is one lower next time. **Earlier**/**Later** move it. Play the film again: it starts with that
  subtitle. Set a wrong password: the search says OpenSubtitles refused the login.
  (`internal/core/providers/opensubtitles/`, `web/apps/web/src/subtitle-search.tsx`, D224)
- [x] **VQ12** — On the dev loop, on the Mac, in Chrome and then in Safari: play *Wing*
  (director's cut) from the real films (`/play/42`) and then *Back to the Lantern* (`/play/59`),
  once both are measured (the job view lists `measure_loudness` jobs until then). Under the video,
  **Level loudness** is on and says Wing is raised about 13 dB; with the volume untouched, the
  two play at about the same level, and Wing's dialogue is clear with the Mac's volume where it
  is for other films. Turn **Level loudness** off: Wing is quiet again at once, and the note says
  it plays as it was mixed; reload, and it stays off. Turn it back on. Play a film whose sound is
  prepared (one with 5.1 sound, on built-in speakers): it is levelled too, and switching the dialogue
  boost changes the voices, not the whole film's level (D226).
  (`web/packages/player/src/gain.ts`, `web/apps/web/src/levelling.tsx`,
  `internal/core/playback/loudness.go`) — done 2026-09-29 by the owner, on the Mac: *"Loudness
  works fine in chrome and safari"*.
- [ ] **VQ13** — On an iPhone, in Safari: play a measured film that plays directly, with the ring
  switch on silent: it plays with sound, and the note says this device sets its own volume. Play
  a measured film that is prepared after it was measured: it plays levelled (D226 §4).

- [ ] **VQ14** — On an iPhone, in Safari, on the dev loop after `make dev-reset`: play
  **TrueHD Film (2022)** (H.264 with TrueHD, so it is prepared on play), then **Remux Film (2021)**
  and choose **English** under **Subtitles** (its ASS track). Passes when each film's picture
  moves within 8 s of pressing play, the reason it is not played directly is on screen, and the
  ASS text appears over the picture. The dev library has no one file with H.264, TrueHD and ASS
  together; `web/e2e/play-timing.spec.ts` checks that file in Chromium and Firefox, and this item
  is its iPhone Safari half (cut-a §8, M3's acceptance). (`web/packages/player/`)
- [ ] **VQ15** — On the Mac, with Apple's HTTP Live Streaming tools installed: once VQ14 has
  prepared **TrueHD Film (2022)**, copy the rendition's directory (the one holding
  `master.m3u8` under `.dev/node/cache/prepared/`) to the Mac and run
  `mediastreamvalidator master.m3u8` in it. Passes when it reports no errors (cut-a §8, M3's
  acceptance: `mediastreamvalidator` passes on a prepared rendition).
  (`internal/node/prepare/arguments.go`)
- [ ] **VQ16** — On devnas (Debian trixie), as your own user: install Docker Engine from Docker's
  Debian repository and add yourself to the `docker` group, clone the repository, and run `./dev
  make check`. Passes when the first run builds the development image (`deploy/Containerfile`)
  without an error and the gate passes inside it. Then run `ORPHEUM_DEV_PUBLISH=0.0.0.0 ./dev make
  dev` and open `http://<devnas's LAN address>:5173` from another machine's browser: passes when the
  dev library appears and a title plays. Also check that nothing but the clone changed on the host:
  `docker volume ls` shows the `orpheum-dev-*` volumes, and no Go, Node or FFmpeg was installed
  outside Docker. The launcher and the image were written in a sandbox without a container engine
  and have never been built or run
  ([D235](decisions/working-notes.md#d235--development-and-deployment-run-in-containers-one-image-family-one-command-each-built-by-goreleaser)
  §3). (`./dev`, `deploy/Containerfile`)

- [ ] **VQ17** — On devnas, after `./dev --stop` and `ORPHEUM_DEV_PUBLISH=0.0.0.0 ./dev make dev`
  with `ORPHEUM_DEV_MEDIA` on `/tank`: the core's log line `a node said hello` lists its
  `devices`. Passes when the media root's device is `zfs:tank`, its `disks` are the pool's data
  disks, and its `kind` is `spinning` (or `ssd` when every disk of the pool is one), and the
  `.dev` directories' device is the disk the clone is on, of kind `unknown` when it is behind USB
  and udev read no rotation rate from it (seen 2026-10-03: the USB SSD `sdc` was called spinning,
  the kernel's guess through its bridge, before that was set aside). The pool is followed to its disks through
  the host's `/run/udev`, which `./dev` mounts read-only into the container; no ZFS was at hand
  where this was written ([D240](decisions/working-notes.md#d240--the-core-keeps-the-smarts-a-scan-is-a-listing-the-core-compares-the-work-it-implies-is-the-cores-to-schedule-and-the-node-enforces-its-devices-limits-and-pauses-background-work-for-play) §5).
  (`internal/node/hardware/storage.go`, `./dev`)

- [ ] **VQ18** — In a desktop browser and on the iPhone, against the dev loop: start
  **Direct Play Film (2020)** (it plays the original), let it play a few seconds, then make its
  folder unavailable: move `.dev/media/movies` aside, then restart the node, so its hello no longer
  names it, or stop the node altogether. Then do the same with **Remux Film (2021)** (a prepared
  copy). Passes when, each time, the film either keeps playing to the end of what the browser holds
  or stops, and the page then says why in words, "This cannot be played now: the folder … on … is
  missing" (or "… is not connected"), never only a generic playback error. Whether a browser plays
  out its buffer after a failed fetch, above all in direct play, no test here can see
  ([D241](decisions/working-notes.md#d241--what-goes-missing-stays-listed-as-unavailable-until-an-admin-says-it-is-gone-and-a-play-of-it-fails-at-once) §7).
  (`web/apps/web/src/play.tsx`)
- [ ] **VQ19** — With the VPS and tunnel set up as `deploy/vps/README.md` says and `[turn] listen`
  on the tunnel's address, start a Watch Together voice call between the laptop at home and a phone
  on mobile data (not the house's Wi-Fi), with relay forced (`iceTransportPolicy: "relay"`). Passes
  when each hears the other, and the laptop's `chrome://webrtc-internals` shows the selected pair's
  local candidate as `relay` at the VPS's address. That proves the relay's replies and relayed
  media go back through the tunnel, and that the VPS sends a relay's packets to another
  allocation on the same relay back in (D243 §7, §8). (`internal/core/turn/relay.go`, M4 Phase F/G)
- [x] **VQ20** — On the iPhone, and in a desktop browser, against staging: look at the sign-in
  page, Films, a title's page, Watch Together with a link made (its QR code), the joining page a
  scanned link opens, the viewer's page, and Jobs. Passes when every page is dark with coral only
  on its main action and the current place, nothing is cut off or runs past the screen's edge,
  every field and button is the skin's and none the browser's default, and the QR code scans
  from another phone. Only an eye can say a page looks right
  ([D251](decisions/working-notes.md#d251--the-web-clients-first-skin-is-dark-with-one-coral-accent-the-system-typeface-and-slightly-rounded-corners)).
  (`web/apps/web/src/*.css`, `base.css`) — done 2026-10-06 by the owner: *"I've looked at
  everything on a browser and on a phone. I also scanned the QR code to test watch together.
  Everything works."*
- [ ] **VQ21** — Two people, each with headphones, against staging's public https address: the
  host on the laptop at home starts a session and plays a film; the other joins by link on a phone
  on mobile data (not the house's Wi-Fi). Each presses **Talk** and allows the microphone. Passes
  when each hears the other clearly while the film plays, the page says "Voice: connected to
  <name>", Mute silences the one who pressed it, the Partner slider changes only the other's voice
  and not the film, and "Leave voice" ends it on both sides. Then once more with the laptop in
  Firefox with `media.peerconnection.ice.relay_only` set to `true` in `about:config`: passes when
  they still hear each other and `about:webrtc` (or Chrome's `chrome://webrtc-internals` on the
  other end) shows the selected pair's local candidate as `relay`. Only two real browsers on two
  real networks show that the microphone, the calls, Web Audio's per-voice gain and the relay
  work together (D193 §4, D63 tier 1). (`web/packages/sdk/src/together-voice-browser.ts`,
  `web/apps/web/src/together-voice.tsx`)
- [ ] **VQ22** — An iPhone on iOS 17.1 or later follows a group the host plays on a laptop, against
  staging. Passes when, over five minutes of playing, with a pause, a play and a seek by the host,
  the iPhone's sound does not skip or stutter as it keeps in step, and staging's log shows its
  nudges (`"nudged"` notes) no more often than every few seconds, not one after every play or
  seek. If the sound skips at each nudge, set **should_together_nudge_keep_pitch** to off on the
  Settings page, join again, and say whether the skips stop. Only a real iPhone shows whether
  Safari's still position just after a change, and its switch of pitch keeping, are answered
  (D250 §7; safari-versions.md §3, §4). (`web/packages/sdk/src/together-follow.ts`)
- [ ] **VQ23** — The host on an Android phone with headphones plays a film in a session against
  staging in **Chrome** and presses **Talk**, with **should_together_voice_cancel_echo** off and
  **should_together_voice_level_microphone** set on (it starts off). Passes when the film stays at
  its usual loudness, the phone's volume buttons still change it while voice is on, and the other
  person hears the host clearly over their film with their Partner slider at 100%. Firefox on
  Android failed this (the owner's test, 2026-10-07: the volume buttons stopped working), which is
  why it starts off. Only a real Android phone shows that Chrome levels a microphone without echo
  cancelling in software, not as a call (voip research, 2026-10-07, from Chromium's source)
  (`web/packages/sdk/src/together-voice-browser.ts`)
- [ ] **VQ24** — `make test-acceptance` on D190's remote-household VM on nuc13 (device-lab.md), each
  of the two browsers behind its own netem profile rather than one shared loopback: passes when
  `ORPHEUM_ACCEPTANCE_MINUTES=30` holds the two within 200 ms 95% of the time and voice within its
  bounds under each profile, as in the development container (D252 §2; M4's audit).
  (`web/e2e/acceptance.spec.ts`)
- [ ] **VQ25** — Against staging, on an Android phone (Chrome or Firefox) and on an iPhone in
  Safari: play a film with English subtitles chosen, past the first line, then go fullscreen with
  the video's own button (the iPhone's player is fullscreen already). Passes when the subtitles
  still show in fullscreen, plainly styled, keep the offset set below the video, and go back to the
  drawn ones when fullscreen ends. Only a real phone shows that its fullscreen player draws a
  page's text track (D254). (`web/packages/player/src/fullscreen-captions.ts`)
- [ ] **VQ26** — Against staging, two people in a session, one on an Android phone in Firefox with
  `should_together_voice_level_microphone` off, the other on an iPhone on its own speaker: before
  Talk, the iPhone shows the earpiece warning; both press Talk and speak for 20 s at a normal
  distance. Passes when each hears the other clearly at the partner volume's 100%, the log's
  `a voice call measured` lines show `receivedLevel` near 0.1 on both sides, and the Android
  phone's film stays at normal loudness; then the iPhone presses Leave voice and its film's sound
  is back on the speaker within a few seconds, not some 20 s later. Only real phones show a
  microphone's level, Android's call mode and iOS's earpiece (the owner's test, 2026-10-08).
  (`web/packages/sdk/src/together-voice-level.ts`, `together-voice-browser.ts`)
- [ ] **VQ27** — Against staging, a host on a computer and a guest on an iPhone in Safari, both
  with sound, play a film for 5 minutes. Passes when the log shows the iPhone's `a player corrects
  by seeks alone` within its first minute, with `returnCostMs` above 150, and no `nudged` note
  from it after that line; the computer's player never logs it; and the guest hears no stutter
  after the first minute beyond one short skip at most every minute or so. Only a real iPhone
  shows what its rate's returns cost and how far its seeks land (D268 §2).
  (`web/packages/sdk/src/together-follow.ts`, `together-drift.ts`)
