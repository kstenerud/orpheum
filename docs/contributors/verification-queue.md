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

- [x] **VQ1** — On devnas (J3455), run
  `go test -run '^$' -bench HashPassword -benchtime 10x ./internal/core/auth` and read
  `BenchmarkHashPassword_DefaultParameters`. Passes at 500 ms/op or less
  ([D194](decisions/working-notes.md#d194--secrets-our-users-passwords-are-hashed-third-party-secrets-are-sealed-with-a-key-that-never-enters-the-database-and-nothing-reads-a-secret-back-out)
  §2); above it, the memory cost comes down to OWASP's 46 MiB, t = 1. For reference, an
  i3-N305 VM measured 50 ms. (M1) — done 2026-10-12 by the owner on devnas, two runs:
  `BenchmarkHashPassword_DefaultParameters-4 10 109870320 ns/op` and `10 118155167 ns/op`, 110 and
  118 ms, within the 500 ms; the parameters stay.
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
- [x] **VQ7** — On the dev loop, as owner on a computer, open **People**, add a person, and send
  the link shown to an Android phone. On the phone, open it: choose a password, and it signs in.
  On the phone, open **Your account**, name a device and make a code. On a television's browser
  (or a second browser), open the server, choose **I have a code**, and type the code in capitals
  with a space for the dash: it signs in as that person. The code does not work a second time.
  Back on the computer, **New link** for the person, and the old link no longer works. On the
  phone, **Your devices** lists the phone and the television; while a code is showing, let the
  phone sleep and wake it: the code is still there. **Remove** the television: its next page
  load asks it to sign in.
  (`web/apps/web/src/people.tsx`, `register.tsx`, `code-sign-in.tsx`, `account.tsx`) — done
  2026-10-12 by the owner; a removed device showed it was signed out only at its next click, which the
  owner wanted at once: since then a shown page asks every minute and as it is shown again
  (`web/apps/web/src/signed-out-watch.ts`).
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
  the iPhone keeps in step after each, and its sound does not stutter beyond the short skips VQ27
  allows. Its nudges are VQ27's, no longer this item's: since D268 §2 an iPhone whose returns to the group's rate cost it more than the drift
  left alone stops nudging and corrects by seeks alone, so it may log no `"nudged"` note after its
  first minute. If the sound skips at each nudge before then, set
  **should_together_nudge_keep_pitch** to off on the Settings page, join again, and say whether the
  skips stop. Only a real iPhone shows whether
  Safari's still position just after a change, and its switch of pitch keeping, are answered
  (D250 §7; safari-versions.md §3, §4). (`web/packages/sdk/src/together-follow.ts`)
- [ ] **VQ23** — The host on an Android phone with headphones plays a film in a session against
  staging in **Chrome** and presses **Talk**, with **should_together_voice_cancel_echo** off and
  **should_together_voice_browser_level_microphone** set on (it starts off). Passes when the film stays at
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
  `should_together_voice_browser_level_microphone` off, the other on an iPhone on its own speaker: before
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
- [x] **VQ28** — Against staging, in a desktop browser and on a phone, as the admin: look at a
  library folder with a problem and open its yellow sign, a role that reaches no library, the
  System page's offline, waiting and unread lists and the reminder above the page, and press
  "Remove…" on a person's page and "This folder is gone for good…" so the red confirmation shows.
  Passes when every problem is the warning yellow `#ffcc00` with its triangle beside what it
  affects, every error message is that yellow and not crimson, each irreversible button is
  outlined in `#ff4d4d` and confirms filled in `#b91c1c` with white words, the consequence is in
  `#ff4d4d`, and none of the three reads as the coral accent. Only an eye can say the colours are
  the ones chosen and read as meant (D278 §4, §6).
  (`web/apps/web/src/tokens.css`, `base.css`, `browse.css`, `credentials-form.css`, `jobs.css`,
  `offline-reminder.css`, `title.css`) — done 2026-10-11 by the owner: *"Looks good."*
- [x] **VQ29** — Against staging, a host in a desktop browser and on a phone opens the Watch
  Together sign in the status tray; then two guests join, one by a link with every right, one by
  a link with both boxes cleared. Passes when the tray's pop-up opens wholly on screen, hanging
  left from the sign, at both widths; the guest with every right has the player's own controls,
  pauses and moves the position for everyone, and enters full screen from them; and the guest
  with none has no player controls but a Full screen button at the film's corner, which fills
  the screen, on an iPhone too. Only a browser lays out a dialog and shows its own full screen
  (D271 §4, D280). (`web/apps/web/src/status-tray.css`, `together-viewer.css`)
  Partly confirmed 2026-10-11 by the owner: the pop-up on screen on a desktop and on an Android
  phone, modal over the page; an iPhone guest with every right had the player's controls and left
  full screen by its X, the film playing on for everyone. Open: a guest with no rights and its Full
  screen button. — done 2026-10-12 by the owner: a Mac host with an iPhone and an Android phone as
  guests, each in turn with every right and with none, then the Android hosting: *"Each guest that
  had no rights (on iphone and android) could only fullscreen (which worked). Any guest with rights
  controlled pause/resume, and if they had rights, jump."* The Android host reloaded played muted
  with no way to turn the sound on, fixed by an earlier commit.
- [x] **VQ30** — Against staging, a host on a computer and a guest on an iPhone, neither pressing
  anything: the host presses Talk and speaks. Passes when the guest hears the host without pressing
  Talk; the guest's Talk is one round microphone button, solid blue while on on every phone, Firefox for
  Android's too, with a ring that grows as they speak: pressed again, it stops only the guest's voice reaching the host,
  the guest still hearing the host, and on the iPhone gives the film's sound back to the speaker; each person's voice slider in the host's pop-up changes only that person's loudness;
  the pop-up's Film volume moves the host's film on a computer; Invite and a person's row each open
  a modal dialog in the middle of the screen, the page darkened behind it and taking no press, as
  is the iPhone's one-line headphones notice with its OK; End the session stands clear below the people; and the
  guest's film title sits above the picture, not over it. Only real devices carry voice and show an
  iPhone's audio session (D281). (`web/packages/sdk/src/together-voice-browser.ts`,
  `web/apps/web/src/watch-together.css`, `together-viewer.css`)
  Partly confirmed 2026-10-11 by the owner, a Firefox for Android or Safari for Mac host with an iPhone
  guest: each heard the other without pressing Talk; Talk one toggle, solid blue, its ring following
  the voice on the iPhone and the Android phone; voice made again by itself after the host's phone
  was locked; the iPhone's headphones notice with its OK; Invite and a person's row in dialogs over
  a darkened page. Open: each person's voice slider, the pop-up's Film volume, and the guest's title
  above the picture.
  2026-10-12, the owner's four-device test (Firefox and Safari on a Mac, an iPhone, an Android
  phone), three microphones on: *"Everything works."* A voice slider set to 0 reached its call
  (the core's log, `partnerGain=0`). Still open: the pop-up's Film volume, and the guest's title
  above the picture. — done 2026-10-12 by the owner: *"On the mac, the extra volume slider for the
  film volume works, and both volume controls remain in sync, updating the other. The title is above
  the video in the guest"*. On an iPhone, where a page cannot set a film's volume, the slider is
  replaced by the phone's buttons (the owner's test, 2026-10-12).
- [x] **VQ31** — On an iPhone, in a Watch Together session: put a finger anywhere on a volume
  slider (a person's voice in the host's pop-up, Film volume, a guest's own volume) and slide it.
  Passes when the slider jumps to where the finger landed and follows it, the page not scrolling,
  without having to hit the slider's knob; on a computer the sliders move by mouse as before. Safari
  on an iPhone moves a slider only by a touch that starts on its knob, which no test browser
  reproduces (the owner, 2026-10-12). (`web/apps/web/src/volume-range.tsx`, `volume-range.css`)
  2026-10-12, the owner on an iPhone: *"the sliders are easy to use."* — done 2026-10-12 by the
  owner: *"the sliders on iphone jump to the position tapped"*; on the Mac the film's two sliders
  moved by mouse, each following the other (VQ30).
- [ ] **VQ32** — Against staging, a host and guests on a computer, an iPhone and an Android phone:
  put a film on from the start, twice in one session, then pause and play. Passes when the first
  start begins about two seconds into the film, the phones running up to it, and every later start
  (and every start in a later session on the same devices) begins with no phone nudged into step
  for seconds after, the core's log showing each player's `startLagMs` and few `nudged` notes after
  `everyoneReady`. Only real phones start late as the owner's did (D282).
  (`web/packages/sdk/src/together-start.ts`, `together-follow.ts`, `internal/core/together/group.go`)
  Partly seen 2026-10-12, the owner: *"Everything seems to work."* The log shows one start, a
  run-up (`startsInMs=2000`), the iPhone and the Mac's Firefox learning 43 ms and 17 ms, and one
  nudge in the whole session. The Android phone had left before the film started. Not yet seen: a
  second start using the learned lags, and the Android phone's start.
- [x] **VQ33** — On staging, host a Watch Together session on a computer and a phone. Passes when the
  status tray's Watch Together sign is the new one, a screen playing with two people before it, in
  the status blue, sharp at the tray's size on both. (`web/apps/web/src/status-tray.tsx`) — done
  2026-10-13 by the owner: *"I've already verified VQ33."*
- [ ] **VQ34** — Against staging: a Watch Together group of a computer, an Android phone and an
  iPhone jumps back and forward by half an hour, twice; then a computer alone jumps a few times.
  Passes when, in the group, the computers and the Android phone on the film's own picture are ready
  within about 4 s (the core's log: `player changed … buffering=false` after `event=seeked`), the
  iPhone no later than before, and no run of one chunk is queued for a run-up; and alone, each
  `a player's seek was ready to play` says where it came from and is not followed by `a player found
  a quality its device cannot play`. Only the owner's link and devices carry the timings (D283).
  (`web/packages/sdk/src/together-refill.ts`, `together-follow.ts`, `web/packages/player/src/hls.ts`)
- [ ] **VQ35** — Against staging, on an iPhone and an Android phone with the screen set to lock after
  30 s: a film plays, alone and in a Watch Together group, untouched for two minutes. Passes when
  neither screen locks while the film plays, and each locks as usual once the film is paused; and
  when an iPhone's screen is locked by its button in a group, the others play on (the core's log:
  no `a leading player sent a command … command=pause` from it). Only real devices lock their
  screens (D287).
  (`web/packages/player/src/wake.ts`, `web/apps/web/src/film-player.tsx`,
  `web/packages/sdk/src/together-follow.ts`)
- [ ] **VQ36** — Against staging, `server_upload_mbps` set low on purpose (3 or 4): a Watch
  Together group of a computer and two phones plays and jumps. Passes when the core's log shows the
  qualities lowered one at a time (`a playback's quality was kept within the server's upload`), each
  player then loading at or under its ceiling with no stall, two players of one film sharing a
  quality where they can; raised again a quality at a time some 30 s after room returns; and, with
  the setting at 1, the newest guest sent away (`sent away the newest member`), their page saying
  "The host's connection can't carry everyone right now." and Join again bringing them back. Only the
  owner's link and devices show whether the steps are right (D286 §4, §5).
  (`internal/core/playback/lowering.go`, `upload.go`, `web/packages/player/src/abr.ts`,
  `web/apps/web/src/together-viewer.tsx`)
- [ ] **VQ37** — Against staging, `server_upload_mbps` at your real upload: a Watch Together group
  of two phones outside the house plays while someone in the house starts a video call or a large
  upload. Passes when, within about half a minute, the core's log says "the films' share of the
  server's upload changed with the house's other traffic" with `isCrowded=true`, the phones' qualities
  step down and play on without stalling, and a minute or two after the call ends the share grows back
  and the qualities rise. Only the owner's house and its traffic show it (D286 §5).
  (`internal/core/playback/crowding.go`)
- [x] **VQ38** — Against staging, on an iPhone with its ring/silent switch set to silent: play a
  film, alone and as a Watch Together guest after its tap for sound; then put Safari in the background
  for a minute, come back, and play again. Passes when the film is heard each time without quitting
  Safari, and the voice of anyone talking in the session too; and the film falls silent, paused, while
  Safari is in the background, a guest's picking up with the group as Safari comes back. Only an iPhone has the switch and the GPU
  process that WebKit bug 323104 strands (design/research/ios-silent-switch-2026-10-12.md).
  (`web/packages/sdk/src/page-audio.ts`)
  Partly confirmed 2026-10-12 by the owner, an iPhone guest with its switch on: sound after its tap,
  silent and paused with Safari in the background, back with the group on its return (after
  An earlier commit): *"ok, that worked"*. — done 2026-10-12 by the owner: a film alone, the switch on, after
  a minute in the background with Safari not quit: *"it worked"*.
- [x] **VQ39** — Against staging: on a television's browser (or a second browser, or a private
  window) not signed in, choose **Get a code**; on a phone signed in, open **Your account** and, under
  **Activate a device**, enter the code with a name. Passes when both screens then show the same check
  code, the phone under the big warning to look at the device's own screen; **Yes, they match** signs
  the television in as you within a couple of seconds and lists it under **Your devices**; a reload of
  the television before that keeps its code; **They don't match** signs nothing in and the television
  shows a new code; and, as admin, entering a code on someone's page under People signs the device in
  as them. Only real devices show the codes are readable across a room and the warning is plain
  (D288). (`web/apps/web/src/code-sign-in.tsx`, `device-activation.tsx`, `internal/core/auth/pairing.go`)
  — done 2026-10-13 by the owner, against staging at an earlier commit: *"I just verified that vq39 worked."*
- [x] **VQ40** — Against staging, on an iPhone: (1) join as a new guest while nothing plays, with no
  other tap; then the host starts a film, someone talks, and later the host switches films. Passes
  when the film and the voices are heard with no tap after Join. (2) Reload the page: it shows only
  "Karl invited you to watch together" and Tap to continue, with no voices heard and no film shown
  until the tap, after which both play with sound. (3) Take a phone call while the film plays (or
  have one rung), and end it: if the sound does not come back on its own, a small "Tap to turn the
  sound back on" sits at the top of the film, and one tap brings back both. Only an iPhone keeps a
  tap's permission per video element, and only it interrupts a page's audio for a call (WebKit's
  `setMutedInternal`). (`web/apps/web/src/together-viewer-tap.tsx`, `film-player.tsx` `KeptVideo`,
  `web/packages/sdk/src/together-voice-browser.ts` `TapWaiting`)
  — done 2026-10-13 by the owner, against staging at an earlier commit, for (1) and (2): a new guest joined by
  Join or by the keyboard's Go with no other tap, and a reload showed only Tap to continue, sound
  after its tap: *"I tested the reload."* (3) not run: the test iPhone has no SIM to take a call;
  the owner: *"this should be good enough for using with my gf though"*.
- [x] **VQ41** — Against staging, on a phone over WebRTC (staging's direct names withdrawn, as for
  the relay test): play a film on Wi-Fi, then turn Wi-Fi off so the phone moves to mobile data.
  Passes when the film goes on after a short stall, at most about ten seconds, with no reload: the
  page replaced the connections the move left behind (plans/webrtc-transport.md W2d).
  — done 2026-10-14 by the owner, against staging at an earlier commit with the offer trimming that followed
  it, on Chrome for Android: Wild Rex 2 played over four punched connections on Wi-Fi; Wi-Fi went off
  about 30 seconds in, and four relayed connections replaced them at 20:55:16–22 UTC with no reload.
  The buffer covered the gap, so no stall was seen: *"It's still playing (1:35) and hasn't suffered
  any hiccups or sound glitches"*.
- [x] **VQ42** — Against staging's `/ui-gallery`, in a desktop browser: the still-reading hourglass
  is on the status blue of the tray's signs, over every poster tone and on its own; and in "On a
  title's page", the warning triangle and the question-mark and hourglass icons beside their words
  look the same size, none smaller. Only a person judges a colour and a size by eye (the owner,
  2026-10-17: "the coral color is not good", "the rest are very small by visual comparison", then
  of the triangle: "visually smaller by a tiny amount").
  (`web/apps/web/src/still-reading.css`, `status-cues.css`)
  — colour done 2026-10-17 by the owner, against staging at an earlier commit: *"Color looks right now."*
  — sizes done 2026-10-17 by the owner, against staging at an earlier commit: *"ok, size looks good now"*

- [x] **VQ43** — On nas.example, from a device at home that has IPv6 (the owner's Mac): sign in at
  `app.orpheum-app.com`, and once the core's log says the connection took `path=direct_ipv6` (or
  `punched_ipv6`) from the house's `/64`, the side menu's admin places show and **Libraries** lists
  the libraries and lets one be made. A connection's own public IPv6 candidate, which only a
  machine with public IPv6 gathers, is what marks its requests as from home, and no test machine
  here has one (the owner's Mac, 2026-10-19: admin menu shown, Libraries refused).
  (`internal/core/rtcapi/own_network.go`, `internal/core/api/home_network.go`)
  — done 2026-10-19 by the owner, on nas.example built from main at an earlier commit, the Mac over
  `path=direct_ipv6` from the house's /64: *"I can add libraries, and am watching them update, so
  admin is fully functional"*

- [x] **VQ44** — On nas.example, built with the home address: the account page shows **Your server at
  home** with a link and a QR code, and the link opens the server at home; System → Remote access
  shows the same address. `/me` reads the address from the live discovery, which only a claimed
  server with its certificate has, and no test machine here is one. (The away-from-home menu's
  line is the page's own, held by app-shell.test.tsx.)
  (`internal/core/api/session.go`, `cmd/orpheum/certificates.go`)
  — done 2026-10-20 by the owner, on nas.example built from main at an earlier commit: *"the URL and QR code
  now show up in both places"*; *"When I clicked the link, it brought me to the login page for the
  server. I entered my name and pw and it showed me the films list"*

- [x] **VQ45** — On nas.example (an Intel GPU, its render node passed through), on the image built
  with Intel's runtime: System → Hardware encoding says **Intel Quick Sync** works, and a film
  prepared then is encoded with `h264_qsv` (the node's log). The portable FFmpeg loads
  `libmfx-gen.so.1.2` and the iHD driver from the system, which only a machine with an Intel GPU
  can try; without a GPU the sandbox shows the same `-9` with the packages and without.
  (`deploy/Containerfile`)
  — done 2026-10-20 by the owner, on nas.example built from main at an earlier commit: *"quick sync worked. I
  can also play films no problem."*

- [x] **VQ46** — With orpheum-connect repackaged from main and nas.example built from main: on a TV's
  browser (or any browser with no household), open `app.orpheum-app.com`, choose **Get a code**;
  on the phone, Your account → Activate a device names `app.orpheum-app.com`; enter the TV's code
  there; both screens show the same check code; **Yes, they match** opens the films on the TV, and
  the TV is listed under Your devices. The code crosses the real service, and the outcome is sealed
  to a key only that browser made, which no test here runs end to end.
  (`web/apps/web/src/loader-device-code.tsx`, `internal/core/auth/devicecodes.go`, orpheum-connect
  `internal/devicecodes/`)
  — done 2026-10-20 by the owner, prod-api repackaged and nas.example built from main at an earlier commit
  (an admin's code-activated device first reached no library, fixed there): *"I have access to the
  libraries and can play videos through a device code linked browser now."*

- [ ] **VQ47** — On a deployment with a VPS tunnel, built from main after the Plex-chain review's
  L2: `docker compose exec orpheum-tunnel ip rule` lists `iif wg0` blackholes for the private
  ranges at preference 20, the tunnel's own subnet at 10 and the local table at 100;
  `docker compose exec orpheum-tunnel ip route get <the tunnel's IPv4> from 192.168.1.5 iif wg0`
  is refused while `… from 203.0.113.9 iif wg0` says `local`; and a film still plays from away
  through the VPS. The test runs the script against a fake `ip`, and the sandbox's network
  namespace (a dummy wg0, 2026-10-20) is no real WireGuard tunnel with the VPS keeping sources.
  (`deploy/staging/tunnel.sh`)

- [ ] **VQ48** — With orpheum-connect repackaged from main and nas.example built from main (D321): on a
  TV's browser (or any browser with no household) at `app.orpheum-app.com`, choose **Get a code**;
  the TV shows a QR code beside its code.
  1. Scan it with a phone signed in to realm `karlnas` by password: the phone opens Activate a
     device on the household's own page with no code to type, names the TV and how long ago it
     asked, shows no check code, and one tap on **Activate** has the TV ask "Sign this TV in to the
     household karlnas as ‹you›?"; **Yes** opens the films.
  2. Ask for another code, and type it under Activate a device instead: after a few seconds both
     screens show the same check code, which is not the one an earlier code showed.
  3. Scan a code with a phone whose browser has no household (a private window): the page offers
     **Copy link** and **Sign in here**, and signing in there lands on Activate a device with the
     code filled in.

  A real TV's browser, a phone's camera and both the paid service and a core from main meet only
  here. (`web/apps/web/src/loader-device-code.tsx`, `web/apps/web/src/loader-activate.tsx`,
  `internal/core/auth/devicecodes.go`, orpheum-connect `internal/devicecodes/`)

- [ ] **VQ49** — In Firefox at home (Local Network Access on), in a private window, sign in to staging
  through `app-staging.orpheum-app.com` and stay on the films for two minutes: the console says
  "requests now go over" once or twice, not again and again, every poster loads, and staging's core logs
  no "refused a request past a connection's limit". Firefox's Local Network Access failing a LAN name's
  preflighted requests while it answers a bare one only shows in Firefox on a real LAN.
  (`web/apps/web/src/kept-path.ts`)

- [ ] **VQ50** — On staging, with a film whose loudness is not measured (most are): open its page
  and play it on a laptop, and open the sound panel under the video. The note says "Its loudness is
  being measured, about N minutes" or "is waiting to be measured, about N minutes", N falls as you
  watch (it asks again every 15 s), and within a few minutes the switch says the film was raised or
  lowered to the level of other films. Then open a series you are partway through: within a minute
  its next three episodes' read-throughs are at priority 100 in Jobs (the System page's job view),
  the one you are partway through first, and the background's are at 0 or 1. Play a film a device
  needs prepared in software (an HEVC film in Firefox): the player says "Preparing this film for this
  device: about N minutes", and while it plays no other prepare runs on the node. A real player's
  reports, a real disk's speed and a real software encode meet only here (D324; I6 of
  `docs/contributors/design/plans/interest-priority.md`). (`internal/core/interest`,
  `internal/core/waits`, `internal/core/nodes/playing.go`, `web/apps/web/src/file-waits.ts`)
- [x] **VQ51** — On a machine with an NVIDIA GPU and its driver: run a node built from this change
  and let it say hello; the System page's hardware lists h264_nvenc's trial with no failure, and an
  encode confined as a prepare is (`orpheum __confined-exec` with `usesGpu`) gives frames identical
  to the same encode run unconfined. CUDA's start meets a real driver only here (D331).
  (`internal/node/confine/landlock_linux.go`, `seccomp_linux.go`, `shim_linux.go`)
  — done 2026-10-22 by Claude on gpuhost.example (RTX 3090, driver 580.178.04, kernel 6.14), the shim
  built from this change and jellyfin-ffmpeg 8.1.2-5 with the ffstreamer patches: h264_nvenc and
  hevc_nvenc trials exited 0; a 10 s h264_nvenc encode of film.mkv, confined, gave a framemd5
  identical to the unconfined run's; the same run without GPU access still failed (exit 171). The
  build before it failed every confined NVENC run with `cuInit(0) failed ->
  CUDA_ERROR_OPERATING_SYSTEM`.
- [x] **VQ52** — On the owner's Intel box with its AMD card: run a node built from main and let it
  say hello; the System page's hardware shows each VAAPI or QSV trial, and any that fails names
  what it was refused. A confined VAAPI encode, if one works unconfined, gives frames as the
  unconfined run does. Mesa's start under confinement meets a real driver only here (D331 §3).
  — partly done 2026-10-23 by Claude on "amd" (Linux Mint 22.3, Radeon RX 6600 XT, kernel 7.0,
  Landlock on), in a Debian trixie container with `mesa-va-drivers` 25.0.7: a 10 s `h264_vaapi`
  encode of film.mkv confined with GPU access (the shim built from main) gave 240 decoded frames
  identical to the unconfined run's, and the same encode confined and reading through ffstreamer,
  given no path to the film, ended DONE OK with the same output; without GPU access it failed. The
  node then had no VAAPI encoder, and the image no AMD VA driver: D332 added both. A node built from
  main, with `confinement_policy = "require"`, in a container of the release image's GPU
  packages, chose `h264_vaapi` (usable: h264_vaapi, x264); Quick Sync's and NVENC's trials failed.
  Seen in the node's log, not on a System page, since no core ran; the page's rendering of a VAAPI
  node is tested (`system.test.tsx`). The same kit on a staging host (Iris Plus 640, Kaby Lake, Gen 9.5)
  found Quick Sync failing in the release image ("Error creating a MFX session: -9") and VAAPI
  working: confined and streamed encodes decoded to the unconfined frames, keyframes where asked.
- [x] **VQ53** — On a machine with an NVIDIA card, its driver and the NVIDIA Container Toolkit under
  rootful Docker, whose Docker has no CDI spec for the card: run the install script from main.
  It reports "an NVIDIA GPU: yes", `compose.override.yaml` reserves the `nvidia` driver with the
  gpu, compute, video and utility capabilities, and the node's log says it chose `h264_nvenc`.
  Docker's nvidia device driver meets a real toolkit only here (D333 §1).
  (`internal/deployment/templates/install.sh`, `internal/deployment/write.go`) The CDI route was
  verified 2026-10-23 by Claude on gpuhost.example (RTX 3090, driver 580.178.04, rootless Docker 29.9.2,
  a spec from `nvidia-ctk cdi generate`): the install chose CDI, and the node, confined under
  `require`, chose `h264_nvenc`.
  — done 2026-10-23 by Claude on gpuhost.example, rootful Docker 29.9.2 with the toolkit 1.20 from
  NVIDIA's apt repository, by `deploy/hwtest/hwtest.sh`: the toolkit wrote its own CDI spec, so the
  install chose CDI (8 of 8 films played, only `h264_nvenc` run); with the node's override switched
  by hand to the `nvidia` driver's reservation and the node recreated, it chose `h264_nvenc` again
  and 8 of 8 played. Docker 29 may satisfy that reservation through the same CDI spec, so a machine
  with the hook and no spec at all (a toolkit before 1.18) is not covered.
- [x] **VQ54** — On a machine with rootless Docker 29 (RootlessKit 3.3 or later): with the
  userland proxy on, the install script reports that it hides each device's address and gives the
  fix; after `modprobe br_netfilter` as root, `"userland-proxy": false` in
  `~/.config/docker/daemon.json` and `systemctl --user restart docker`, it reports the proxy off, and
  a sign-in from another machine at home logs `path="direct from home"` and may administer. Only a
  real rootless daemon shows whose address the core sees.
  (`internal/deployment/templates/install.sh`, `docs/users/setup-problems.md`)
  — done 2026-10-23 by Claude on gpuhost.example (rootless Docker 29.9.2, RootlessKit 3.3.0, the owner's
  `modprobe br_netfilter`), the install script and image from "fix(deployment): the install script names rootless Docker's hidden addresses and their fix": with the proxy on, the
  script reported it hid each address and, the module loaded, gave only the `daemon.json` step; a
  sign-in from the sandbox (192.168.1.199) logged `path="direct from away"` and `GET /users` was
  refused. With the proxy off and the daemon restarted, the script reported it off, the same sign-in
  logged `path="direct from home" isAdmin=true` and `GET /users` answered 200.
- [ ] **VQ55** — On each Fire TV stick at home, in Silk, signed in by a device code, open `/lab/device`
  and press **Run the sound and WebRTC tests** (plans/tv-device-tests.md H1). Passes when the page
  shows its report and **Sent as** a name, and the core's log holds that name's line with
  `probe=device`, the stick's codecs, its audio latency and the WebRTC call; note Fire OS or Vega OS
  (Settings, My Fire TV, About) beside it. (the device lab: `web/apps/web/src/lab-device.tsx`,
  `web/packages/sdk/src/device-report.ts`, `internal/core/api/lab.go`)
- [ ] **VQ56** — On the iPhone, in a Safari tab and then as a home-screen app, and on an Android
  phone in Chrome, run `/lab/remote` with earphones for the runs of plans/tv-device-tests.md H4.
  Passes when each run's log reaches the core every 30 s (`probe=remote`) with its wake-lock,
  sound-flowing and page-stopped counts, whatever they show; what they show is the finding.
  (`web/apps/web/src/lab-remote.tsx`, `web/packages/sdk/src/rtc-loopback.ts`)
- [ ] **VQ57** — On the Windows laptop extending to a TV (Win+K, Win+P Extend), in Chrome, open
  `/lab/extend`, **Find screens**, open the test film on the TV, drive it with play/pause and ±10 s,
  then play the two tones on two outputs (plans/tv-device-tests.md H3, H5). Passes when the test
  film opens on the TV's screen and goes full screen at a press, the laptop's page shows its position
  move with each command, and each tone is heard on the output chosen for it, or the page says
  plainly which API is missing. (`web/apps/web/src/lab-extend.tsx`, `lab-extend-player.tsx`)
- [ ] **VQ58** — With a Chromecast (or the TCL's built-in one) registered for development and our
  receiver registered at `https://<core's LAN name>/cast/` (plans/tv-device-tests.md §3), open
  `/lab/cast` in desktop Chrome at that name, **Cast to our receiver**, **Ask for its report**, and
  **Play the film on it** with the sign-in ticked. Passes when the TV shows the receiver's log with
  *CAF started* and no *blocked by the page's policy* line, the receiver's report reaches the core's
  log (`probe=receiver`), and the film plays on the TV; the report's WebRTC line, stalls and memory
  are the spike's findings. (`web/apps/cast-receiver/src/`, `web/apps/web/src/lab-cast.tsx`, the
  cast policies in `internal/core/webclient/webclient.go`)
- [ ] **VQ59** — On the TCL in TV Bro's direct mode, and on a Fire TV stick in Silk, open
  `/lab/input` and move across the tiles about 20 times in each mode (plans/tv-device-tests.md H2,
  B2). Passes when the page logs the remote's keys and measures each move; D190's line is our
  geometric search's worst key-to-focus at or under 250 ms, which the result records whether or not
  it is met. (`web/apps/web/src/lab-input.tsx`, `focus-engine.ts`)
- [ ] **VQ60** — On a Fire TV stick in Silk's spatial mode and on a laptop's Chrome and Firefox,
  run `/lab/input`'s text field, both full-screen buttons and the 8BitDo SN30 Pro in each of its
  modes (plans/tv-device-tests.md H6–H8). Passes when the core's log holds the run's `fieldIn`,
  `fieldInput` (a length, no text), `fullscreenEnter` or `fullscreenRefused`, keys with
  `inFullscreen: true`, and a `pad` line per mode with its layout; what they show is the finding.
  (`web/apps/web/src/lab-field.tsx`, `lab-fullscreen.tsx`, `lab-pad-input.tsx`, `input-pads.ts`)
- [ ] **VQ61** — On a Fire TV stick in Silk's spatial navigation mode, walk the app by the remote
  alone (plans/tv-device-tests.md H9): Films with Continue Watching, a film, a series and a season,
  search, the banner's menu, This device, Play and Watch Together. Passes when every control is
  reached by the arrows with its focus ring seen whole and the focused element on screen clear of
  the banner, OK presses it, Silk's Back returns to the page before with focus on what was left,
  focus never moves twice for one press (ours and Silk's), and on the video play/pause plays and
  pauses and Left and Right seek. (`web/apps/web/src/input-commands.ts`, `focus-navigation.ts`,
  `focus-memory.ts`, `player-commands.ts`)
- [ ] **VQ62** — On a laptop's Chrome and Firefox with the 8BitDo SN30 Pro in X-input mode, its OK
  chosen as the right button on This device, walk the same pages by the controller alone (H9).
  Passes when the D-pad and the left stick move focus, repeating while held, with focus shown as a
  key's is; A presses; B goes back, closing the banner's menu and a dialog first; Start plays and
  pauses; and B leaves the player's full screen before the page. (`web/apps/web/src/input-pad-poll.ts`,
  `controller-choice.ts`, `player-commands.ts`)
