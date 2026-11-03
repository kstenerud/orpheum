> **ABOUTME:** Checks that only a person with the hardware can do — a film on a television, a
> voice call across two houses. A fix that no automated test can turn red names its item here
> in a `Verified-By: VQ<n>` trailer; until it is ticked it is owed, which revert-red lists (D378).

# Verification queue

[D190](decisions/working-notes.md#d190--the-testing-strategy-devices-report-on-themselves-against-numbers-a-missing-capability-fails-every-check-first-shows-it-can-fail-and-the-lab-is-built-from-what-is-on-the-shelf)
requires every behaviour change to be able to turn a test red when reverted. A few cannot: the
symptom shows only on a device, a network or a room. For those, the change adds an item here,
someone does it, ticks it, and the commit carries `Verified-By: VQ<n>`.
`scripts/check_revert_red.py` refuses the trailer if the item is missing, and lists an unticked one as
owed, not a failure: the owner decides when a batch goes out with checks owed (D378). yoloAI's
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
  to the confined encode (cut-a §6, D225). (`internal/node/encode/encoder.go`, before D344 `internal/node/prepare/encoder.go`,
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
  *Run by the lead on staging, 2026-11-02, Linux Chrome 156, The Gold Rush (file 241).* Passes but for
  one part unseen. A refused login (staging was first given the account's email address, which
  OpenSubtitles answers 400) now says OpenSubtitles refused it ("fix(opensubtitles): a login answered 400 is a refused login"). With the username: "20 of 20
  downloads left today", the note that none is checked, 11 German results; the download played on with
  it chosen as "German · … · downloaded, unverified", its 1,476 cues loaded; played again, it started with
  it; Later and Earlier moved its cues 0.1 s each way; the next search said 19 of 20. Unseen: a copy
  made for this very file first, as none of the results matched this file.
- [x] **VQ12** — On the dev loop, on the Mac, in Chrome and then in Safari: play *file 42*
  (director's cut) from the real films (`/play/42`) and then *file 59* (`/play/59`),
  once both are measured (the job view lists `measure_loudness` jobs until then). Under the video,
  **Level loudness** is on and says file 42 is raised about 13 dB; with the volume untouched, the
  two play at about the same level, and file 42's dialogue is clear with the Mac's volume where it
  is for other films. Turn **Level loudness** off: file 42 is quiet again at once, and the note says
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
  (`internal/node/prepare/arguments.go`, retired with the prepare, D344)
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
  it, on Chrome for Android: a film played over four punched connections on Wi-Fi; Wi-Fi went off
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
  *Run by the lead on staging, 2026-11-02, Linux Chrome 156 and Firefox 155.* The note part fails:
  a 306 s film said "waiting to be measured, about 55 minutes", was measured within 15 s, said "being
  measured, less than a minute" for about two minutes, then "not measured yet, so it plays as it is"
  until the page was opened again, when it said "Lowered 1.2 dB" (the player asks for the level only
  as it starts; the plan's item 8 of 2026-11-02, next batch). The series part cannot be checked as
  written: the job view shows no priority since D329 put tiers in place of numbers, and staging has no
  read-through left queued; it needs restating. The Firefox part needs a Firefox that cannot decode
  HEVC (the Mac's or Windows'): Linux Firefox 155 plays The Gold Rush's HEVC as it is, unprepared.
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
  *Run 2026-11-02 (agent, staging, by adb; not ticked: one stick of those at home, and signed in
  by password, not by a device code):* the Fire TV Stick 4K (AFTSSS), Fire OS 7.7.1.7, Silk 138.20.3
  (Chrome 138), signed in as lead-tests, showed its report and *Sent as NGYYG5QS*: screen 960 × 540 at
  2 device pixels per pixel, page 960 × 480; sound delay 43 ms base, 288 ms output, one output; the
  WebRTC call with itself connected in 160 ms, round trips 21.8, 6.1 and 3.2 ms, its sound arriving;
  H.264, HEVC, AV1, VP9 profile 0, AAC, HE-AAC, Opus and FLAC yes, VP9 10-bit, AC-3 and E-AC-3 no.
  The core logged it at 11:48:23Z (`a lab page sent its results`, `labResultId=NGYYG5QS
  probe=device userId=4 deviceId=110`) with the codecs, the audio latency and the call.
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
  *Run 2026-11-02 (agent, staging, the Fire TV in Silk 138 by adb, as lead-tests; not ticked):*
  every place named was walked, and every control was reached by the arrows with its ring whole and
  clear of the banner, one focus move a press; OK pressed each; Back returned to the page before
  with focus where it was left (Nanook of the North in Films). Two faults, fixed and checked on the stick:
  a series opened from far down the Series list opened scrolled 239 px down, its name hidden
  ("fix(web): a series opens at its top, its row of seasons scrolled sideways alone"); and on the video OK opened the hidden overlay with nothing focused, so a second OK
  paused nothing and no arrow reached its controls ("fix(web): OK on a television's hidden player overlay puts focus on play"). Since the fix OK opens it on Play,
  each further OK plays or pauses, Left and Right seek while it is hidden, and Back hides it, then
  leaves for the film. Still open: Back from a film opened from Films' search shows the whole list
  again, the search and the sort being held in the page, not its address (for the owner); and some
  arrows skip a nearer control for one in line with them, each reached the other way (Down from
  *‹ Series* to the season row past Play, Down from Rename past the game controller's choices, Right
  at a row's end into the banner), filed for the next batch.
- [ ] **VQ62** — On a laptop's Chrome and Firefox with the 8BitDo SN30 Pro in X-input mode, its OK
  chosen as the right button on This device, walk the same pages by the controller alone (H9).
  Passes when the D-pad and the left stick move focus, repeating while held, with focus shown as a
  key's is; A presses; B goes back, closing the banner's menu and a dialog first; Start plays and
  pauses; and B leaves the player's full screen before the page. (`web/apps/web/src/input-pad-poll.ts`,
  `controller-choice.ts`, `player-commands.ts`)
- [ ] **VQ63** — Once staging runs an earlier commit or later, open Orpheum on staging in the owner's four
  browser tabs (Firefox, Chrome, Safari on the Mac; Firefox on Linux), leave them a minute, then sign
  in on the Fire TV stick in Silk. Passes when the TV reaches the server, and the core's log
  (`docker logs orpheum-staging-orpheum-core-1 2>&1 | grep rtcapi`) shows the network holding about
  one connection per open page once each has been idle 30 s (`networkHeld` on the last "a connection
  is held" line), more only while a page loads posters or a film. Then, with the limit lowered on a
  test core or enough pages open, the TV's sign-in must say *This server is turning this device away:
  too many Orpheum pages are open on your network. Close some and try again.*, never *can't be
  reached from this network*, and the log must show one "turned a connection away" line per Try
  again, not one a second. (`web/packages/sdk/src/rtc-transport.ts`, `rtc-connect.ts`,
  `internal/core/rtcapi/verdict.go`, `web/apps/web/src/discovery-gate.tsx`)
- [ ] **VQ64** — Sign a Fire TV stick (Silk, in pointer mode and again in spatial mode), the TCL TV
  (TV Bro) and a phone in as the same person; open any Orpheum page on the TV. On the phone, play a
  film, press **Cast to another device** and choose the TV (D337, D338; plans/cast-to-device.md §4,
  §8 step 3). Passes when the TV is listed by its name with when it was last used, plays the film
  within a few seconds of where the phone stood, filling the screen with the TV's own player controls
  over it and *Controlled from ‹phone› · Back, or OK twice, to take control here*, which fades after
  about five seconds; with sound, or with **Sound on**, which OK starts;
  the phone shows the remote (no film playing on it), and its Play, Pause, Back 10 s, Forward 10 s and
  timeline move the TV within a second or two. (`web/apps/web/src/cast-to.tsx`, `cast-receiver.tsx`,
  `cast-remote.tsx`, `live.tsx`)
- [ ] **VQ65** — With the phone casting to the TV as in VQ64, on the Fire TV in Silk's spatial mode
  press Back once; cast again and press OK twice quickly; in pointer mode, press Back once, and again
  OK twice quickly; on the TCL, the remote's Back once, and OK twice (D338 §6; §6.1). Passes when each
  takes control back at once: the TV plays on as an ordinary player with its own controls, and the
  phone says *‹TV› took control back.* with **Play here from ‹position›**; a single OK, or two half a
  second or more apart, only works the TV's controls (opens them, presses what has focus) and the
  cast goes on. Then cast from the phone, then from a laptop: the
  phone must say another device now controls the TV, and the TV follow the laptop. Lock the phone:
  the TV plays on. (`web/apps/web/src/cast-receiver.tsx`)
- [ ] **VQ66** — With the phone casting to the TV as in VQ64, press the Fire TV remote's play/pause
  key, and in Silk's spatial mode its Left and Right, and OK then (once the controls show) OK on
  play; on the TCL, its remote's play/pause (D337 §7; D338 §5; §6.4). Passes when the TV pauses and
  plays, and seeks 10 s, through its own controls, still controlled, and the phone shows *Paused* or
  *Playing* and the new position within a second or two.
  (`web/apps/web/src/cast-receiver.tsx`, `player-commands.ts`)
- [ ] **VQ67** — Sign the Fire TV stick (Silk, in spatial mode and again in pointer mode), the TCL TV
  (TV Bro) and a laptop's Chrome in as one person whose autoplay is on (Your account, *Playing*).
  Play an episode that is not its season's last, seek to a minute before its end and let it play
  (plans/up-next.md). Passes when, 30 s before the end, *Up next: S1E3 · ‹name›* shows with its still
  in the picture's top corner, leaving the picture's end in view; at the end it counts *Plays in 10 s*
  down and plays the next episode; done again, a press of Down stops the count and leaves the
  offer, OK on it plays it, and Back dismisses it; with the TV's overlay open (OK while it plays),
  Up from play reaches the offer and OK presses it. Then turn autoplay off and play to the end: the
  offer stays with no count, focused, and OK plays it. On the laptop the mouse does the same, and
  its × dismisses it. (`web/apps/web/src/up-next.tsx`, `tv-controls.tsx`)
- [ ] **VQ68** — With the phone casting an episode that is not its season's last to the Fire TV as in
  VQ64, autoplay on, seek the TV from the phone to a minute before the end (plans/up-next.md §8).
  Passes when the TV shows *Up next: ‹place› · ‹name›* 30 s before the end and, at the end, *Plays in
  10 s* counting down, then plays the next episode filling the screen, the phone's remote showing its
  title and length within a few seconds; then on the phone press **Up next**: the TV plays the one
  after. Turn autoplay off and let an episode end: the TV waits with the offer, and the phone's **Up
  next** plays it. Cast the season's last episode and let it end: the TV goes back to the page it
  showed before the cast (D338 §1) and the phone says the film ended. (`web/apps/web/src/up-next.tsx`, `cast-receiver.tsx`, `cast-remote.tsx`,
  `live.tsx`)
- [x] **VQ69** — On the Fire TV stick (Silk, spatial mode) and a laptop's Chrome, signed in with
  autoplay on, play an episode with at least four after it in its season, seeking each to a minute
  before its end and pressing nothing else, and let three play on by themselves (D339). Passes when
  the fourth's end shows *Still watching?* with Continue focused and no count; OK plays the next
  episode; done again, Stop (Right, then OK) leaves the film for the page before. Then, with two
  autoplayed, press any key or tap mid-episode: the next end counts *Plays in 10 s* as usual.
  Each episode must reach `ended`: one whose chunks join with a hole once stood a frame short of its
  end for good (idiosyncrasies.md, "hls.js never ends a stream with a hole behind the playhead";
  `web/packages/player/src/end-of-stream.ts`). (`web/apps/web/src/still-watching.tsx`, `up-next.tsx`)
  - Run 2026-11-02, the Fire TV as lead-tests: 5667→5670 played on by themselves and 5670's end showed
    *Still watching?* with Continue focused and no count; OK played 5671. 5674 then stood at 1299.97 of
    1300.06 s, never ended (the hls.js fault above), until the end was sent by hand.
  - Run again 2026-11-02 on the stick with "fix(player): an episode with a hole behind the playhead reaches its end", "fix(player): a video stalled at its end while seeking is ended too" and "fix(web): Right on a television's Still watching reaches Stop, not a seek" on staging: 5674 ended
    and 5675 played. Opened from the series page, 5683→5686 played on by themselves and 5686's end
    asked *Still watching?* with Continue focused; Right on Continue sought the film instead of going
    to Stop, so OK played on (fixed, "fix(web): Right on a television's Still watching reaches Stop, not a seek"); after the fix Right reached Stop and OK went back to
    the series page. With two autoplayed, Up pressed mid-episode: that end and the next counted
    *Plays in 10 s* as usual. The stick's part passes.
  - Run 2026-11-02 in a desktop Chrome 156 (1280×720, Linux) as lead-tests, opened from the series
    page: 5683→5686 played on, each counting, and 5686's end asked *Still watching?* with Continue
    focused; Enter played 5687. 5687→5690 played on, *Still watching?* again; Right reached Stop and
    Enter went back to the series page. With two autoplayed, Shift pressed mid-episode: the next two
    ends counted and played on. Passes on both; ticked.
- [ ] **VQ70** — Host Watch Together on a laptop's Chrome with a guest on another browser (a phone's
  or Firefox), both with sound, and play an episode that is not its season's last; seek to a minute
  before its end (D339). Passes when the host's *Plays in 10 s* runs out and both screens go to the
  next episode together, the guest's never showing the black stopped screen nor "Karl stopped the
  film" between, the group starting in step; done again with the host pressing Up next before the
  end, the same. (`web/apps/web/src/together-host.tsx`, `up-next.tsx`)
- [ ] **VQ71** — Sign a laptop's Chrome, the Fire TV stick (Silk) and a phone in as one person
  (D338 §1, §2). On the laptop open two Orpheum tabs, then bring a tab of another site to the front;
  on the phone, play a film and open **Cast to another device**. Passes when the laptop is listed
  with *Open, but no Orpheum tab is in front*, and choosing it says *‹laptop› has no Orpheum tab in
  front. Bring an Orpheum tab to the front on it, then cast again.*; bring the second Orpheum tab to
  the front, and within a second or two the list says *Open now*, and a cast plays in that tab, the
  first staying as it was. On the Fire TV open a title's page, cast to it from the phone, then press
  **Stop casting** on the phone: the TV goes back to that title's page, not paused on the film; cast
  to it from its home page, reload the TV's page while it receives, then stop again: it goes back to
  its home page. (`web/apps/web/src/live.tsx`, `cast-to.tsx`, `cast-return.ts`,
  `internal/core/live/registry.go`)
- [x] **VQ72** — On a Fire TV stick in Silk's spatial mode (D349, library-browsing.md §5.5), open the
  Libraries page from the top bar with two or more libraries, one of them an `other` library. Passes
  when Up from the grid's top row reaches the bar, Left and Right move among the library chooser, the
  view chooser (the `other` library only) and the sort chooser; OK on any of them opens its list of
  choices as a focusable overlay with focus on it at once, arrows move within it, OK on a choice
  follows it and closes the overlay, and Back closes it without choosing, returning focus to the
  chooser button; the alphabet rail at the grid's right shows only while sorted by name or file name,
  and Right from the grid's last column reaches it; opening the `other` library's Folders shows its
  subfolders and videos, Up leaves a folder, and one click reaches each from the bar.
  (`web/apps/web/src/library-browse.tsx`, `modal-dialog.tsx`, `focus-navigation.ts`)
  - Run 2026-11-02 on the stick as lead-tests, with Movies, Shows, Musicals and an `other` library
    "Musicals folders" (library 4, made over the musicals root for this, D363). The library chooser
    is now the bar's Libraries menu: OK on it opened its list with the first library focused, and
    one press of OK on a library opened it. Up from the grid's top row reached the bar (from the
    right-hand tiles by way of the page's buttons). Left and Right moved among View (the `other`
    library only), Configure, Search and Sort. OK on View and on Sort opened each list as an
    overlay with focus in it; arrows moved within it, OK on a choice followed it and closed it, and
    Back closed it with focus back on its button. The rail showed under Name and File name sorts in
    the All view and not under Recorded or Released ones, nor in Folders (by design,
    `library-browse.tsx`), and Right from the last tile reached it. Folders listed the six
    subfolders; OK on one listed its video, its "‹ Musicals folders" link above it, and OK on that
    went back up with focus on the folder left. On entering a folder nothing has focus; the
    first arrow goes to the page's first control, View (`startIn`, as on every page). Passes;
    ticked.
- [ ] **VQ73** — On staging, put a decrypted DVD image (`.iso`, a film of your own, made by MakeMKV's
  backup or a DVD ripper that decrypts) in a film library and scan it (D350). Passes when the film
  is identified by its file name, its page shows the main feature's length (not a trailer's) and
  its audio tracks with their languages, it plays from the start, and a seek to the middle plays from
  there with picture and sound in step. An encrypted image is instead listed as a file that could
  not be read, its message saying it is encrypted. (`internal/node/disc`,
  `internal/node/execx/disc.go`) *2026-11-02, by the agent in Chrome, with the musicals library's
  Der Golem concert DVD:* it reads with the main feature's 2:27:45 and its English AC-3 track,
  and plays from the start; it is not identified, its file name (LES_MIS_NTSC_V2) naming no film.
  A seek to the middle failed: the title's clock starts again at 1:28:39 (D404). Re-run the seek,
  to before and after 1:28:39, once D404 is on staging.
  *Re-run 2026-11-02 in Chrome, with D404 and D405 on staging ("fix(disc): a DVD title's clock is checked at every VOBU, with a frame's slack"):* seeks to 1:13:52, 1:50:48
  and 2:27:02 (past the title's last clock restart, D405 §1) each play on, picture and sound in step
  (within 12 ms), 2 to 5 frames dropped. Left: an encrypted image's message, and identifying a film
  by a file name that names one.
- [ ] **VQ74** — As VQ73 with a decrypted Blu-ray image whose film is one clip. Passes as VQ73 does,
  and the picture is copied, not re-encoded, on a browser that plays H.264 (the play's decision
  says remux). (`internal/node/disc/bluray.go`)
- [ ] **VQ75** — As VQ74 with a Blu-ray image whose main playlist joins several clips (a seamless
  branching disc, or one with a multi-angle or obfuscated playlist set). Passes when the right
  playlist is picked (its length matches the film's) and the film plays across the clips' joins;
  report what is wrong when it does not: which clip, at what time, and whether seeking across a join
  works. A DVD title with angles or seamless branching is refused as interleaved, saying so.
  (`internal/node/disc`)
- [x] **VQ76** — Cast a film from a laptop (Firefox on the Mac) to a Fire TV stick in Silk, then to
  Chrome on Linux, each with an Orpheum tab in front that was not pressed since it loaded (D358).
  Passes when each receiver shows "‹Laptop› wants to play ‹Film› here" with Play focused, the
  laptop saying "Asking ‹device›…" and its film playing on; on the Fire TV the remote's OK on Play,
  and on Chrome a click on it, starts the film there **with sound and full screen** (Silk's own bars
  gone), and only then does the laptop become the remote; staging's core log shows
  `logEvent=playedWithSound` and `logEvent=fullScreen answer=entered by=key` (Fire TV) or
  `by=click` (Chrome). Cast again and press Not now: the laptop says "‹device› declined." and plays
  on; cast again and leave it a minute: the prompt closes and the laptop says "‹device› did not
  answer.". Should a receiver still play muted (`logEvent=playedMuted`, whose `page=` says whether
  the page had been pressed), "Press OK to turn the sound on" shows over the film, and one OK (or
  any arrow) turns the sound on without pausing it (`logEvent=unmutedTap by=key`).
  (`web/apps/web/src/cast-asked.tsx`, `cast-to.tsx`, `cast-receiver.tsx`, `live.tsx`)
  Passed 2026-10-27 (the owner, staging): the Fire TV asked, and OK on Play played with sound full
  screen; cast from Firefox on Android to Chrome on the Mac, Not now and a minute unanswered each said
  so on the phone; the log holds `playedWithSound`, `answer=entered` and the declined and unanswered
  lines.
- [x] **VQ77** — As VQ76, a cast played full screen on the Fire TV, then on Chrome on Linux.
  Passes when **one** Back on the Fire TV takes control back (full screen left, the film playing on
  there), the laptop saying it was taken back, and staging's core log shows
  `logEvent=fullScreen answer=left`; on Chrome, Escape does the same. Stop casting from the laptop,
  and the film's end, return each receiver to its page with no "taken back" (the log saying
  `reason=stopped` or `filmEnded`, no `answer=left`). On Chrome, moving the mouse or clicking shows
  the player's controls, which hide again about 5 s after the mouse stops; a double click takes
  control back; with the sound refused, the first click turns the sound on and does nothing else.
  Chrome's receiver goes full screen at Play (D358 §2). On the laptop, after the receiver took
  control back at, say, 16:22, *Play here from 16:22* plays from 16:22, and the core log's
  `a playback started` line says `cause=castOver pageStartsAtMs=982…`, its `play state changed
  kind=started positionMs=` near it. (`web/apps/web/src/cast-receiver.tsx`, `tv-controls.tsx`,
  `cast-remote.tsx`, `play.tsx`)
  Passed 2026-10-27 (the owner, staging): Back on the Fire TV and Escape on Chrome took control
  back, the film playing on; Stop casting and the film's end returned the receiver to the series page
  it was on; a mouse move showed Chrome's controls; each side played on from the right place
  (`cause=castOver pageStartsAtMs=763013` at 14:07). Seen beside it, filed for the next batch: on
  Chrome's receiver neither a click on the video nor Space pauses it; only the pause button does.
- [x] **VQ78** — On Chrome on a computer, play a film on its ordinary Play page, click the video's
  own full-screen button, then press Space. Passes when Space pauses the film and Space again plays
  it, full screen staying as it is; Space on the video after a click on it does the same, and Space
  on a control under the player (a menu, a button) still presses that control. Then cast a film to
  Chrome on Linux and press Play there: a mouse's click on the film pauses it and another plays it,
  the controls opening; Space pauses and plays it with focus anywhere in the player (on the video,
  the overlay's play button or its scrub bar); a double click still takes control back with the film
  playing on; with the sound refused, the first click only turns the sound on.
  (`web/apps/web/src/player-commands.ts`, `cast-receiver.tsx`)
  Passed 2026-10-27 (the owner, staging): on a receiver a click pauses and plays, a double click takes
  control back; on a computer Space pauses after the full-screen button, once focus went back to the
  video after a press on its own controls (Chrome pressed the focused button before the page heard
  Space).
- [x] **VQ79** — On the Fire TV stick in Silk's spatial mode, open a film's Play page so it opens out
  of full screen (as a reload of the Play page does). Passes when the player's overlay offers a
  full-screen button the remote moves to beside Play, whose OK puts the film full screen (the
  button then says Leave full screen); with the overlay showing, moving down to the scrub bar and
  pressing Left and Right seek back and forward 10 s each, focus staying on the bar. Sign the Fire TV
  in as a new device (sign out first, or from a fresh Silk profile): *Your devices* names it "Silk on
  Fire TV". A device signed in before keeps its name until renamed. (`web/apps/web/src/tv-controls.tsx`,
  `full-screen-button.tsx`, `focus-engine.ts`, `device.ts`)
  Seen 2026-10-27 (the owner, staging): the button goes full screen and back; Left and Right on the
  scrub bar seek, in full screen and out. Its last part, the new device's name, moved to VQ84 so this one
  could be ticked for what was seen.
- [x] **VQ80** — On staging, D363's overlap: make a series library over a folder inside the films
  library's folder (or a films library over a parent of it). Passes when both are made, one scan of the
  outer folder lists the place once (staging's core log, at debug, says `a folder inside another library
  folder is scanned within it`), each library's page lists its own kind, *All libraries* and Home show
  each film and series once, and adding to the films library a folder beneath its own is refused as
  already covered. (`internal/core/library/scans.go`, `scanroots.go`, `libraryfolders.go`)
  Passed 2026-10-27 (the lead, on staging, as the owner asked): a films library over Crowd's
  folder inside Films' was made and scanned within it (`a folder inside another library folder is
  scanned within it`, scope the folder); both list Crowd as item 5056, All films lists 325 films
  and no item twice; adding that folder to Films was refused (`the library already covers that
  path`); an other library over the same folder gave the file an other video of its own (5421).
- [ ] **VQ81** — On staging, D363's copies: copy a film byte for byte to a second folder that a second
  films library covers, and scan it. Passes when, after the copies' sweep (the pass after the scan, or
  within `read_through_sweep_every_hours`), the core log says `one file at two places from now on`, the
  film is listed once, the doctor's "files found at more than one place" names both paths, a person
  whose role reaches only the second library sees and plays the film (from the copy), and, the first
  folder's disk unplugged, a person reaching both still plays it, from the copy. A re-encode put
  there instead stays a film's second version, and a file of the same payload with other bytes is
  logged `and other bytes: they stay two files`. (`internal/core/library/copies.go`, `playable.go`)
  Seen 2026-10-27 (the lead, on staging): the copy merged within half a second (`one file at two
  places from now on`, copy 12304 into file 12168), each library lists the sync-test clip as item 5285, the
  doctor names both paths (and found two real pairs: two episodes of an animated series are one file, and
  two Insex videos); a person whose role reached only the copy's library saw the film, no path, and
  played it, and with the copy renamed away was refused (404) though the original stood: only the
  reached place is read. Not seen: the fall-back to the copy. A file renamed away, or its library
  folder made to vanish, is not an unavailable place (only a node away, a root or disk missing or not
  answering is), so a play of the original's place answered 404 rather than reading the copy; and the
  vanished folder's scan was given up without marking the folder unavailable. Filed for the next
  batch. Owed: the owner's unplugged-disk case, on a disk.
  Fixed 2026-10-27, to see again: with the original renamed away, a play by a person reaching both
  places plays the copy (direct and from the ladder), the core log saying `a read found its file not
  at this place` and a scan of its folder following, and a person reaching only the original's place
  is still refused; with a library folder deleted, its scan lists it as unreadable (node log `a
  library folder could not be opened`), the Libraries page shows the folder unavailable, its films
  stay listed as unavailable, and they come back when the folder does and a scan reads it.
  (`internal/core/api/playback.go`, `internal/core/chunks/job.go`, `internal/core/library/absent.go`,
  `internal/node/agent/listing.go`)
  Seen again 2026-10-27 20:41 (the lead, staging, after earlier commits): with the original
  renamed away the admin's play read the copy (206; runs moved to the copy's place, logged), and the
  original's library folder made to vanish scanned as `unreadable`, nothing removed. Still owed: the
  owner's unplugged disk.
- [x] **VQ82** — On nas.example, when the owner next deploys, step 0060 carries production from 0059.
  Passes when the migration completes (no `D363 removes the mixed kind` refusal), every film and
  series is listed as before, the libraries keep their folders, and each person's Continue Watching
  and watched marks are as they were. (`internal/store/migrations/0060_…`)
  Passed 2026-10-27 (the owner, nas.example at an earlier commit): Continue Watching looks the same as before.
- [x] **VQ83** — On staging and on the Fire TV stick, D366: on the Libraries page of a films library,
  the sort chooser offers *Name, A–Z* and *Z–A*, *Newest* and *Oldest released*, *Newest* and *Oldest
  added*, each reachable by the remote. Passes when *Name, Z–A* lists Z first, its address keeps
  `order=descending` across a reload and Back, its alphabet rail runs Z to A and a letter moves the
  list to that letter's first title in that order; *Newest released* lists the newest film first and
  those with no date last, also in *Oldest released*, after a pass has read each film's day (its NFO's
  `premiered` or TMDB's release date); and, on the person's own settings, leaving a library out of
  *Libraries on Home* takes its titles off Home's rows (Continue Watching, the new films and series)
  while Films and the Libraries page keep them. (`web/apps/web/src/library-browse.tsx`,
  `alphabet-rail.tsx`, `left-out-libraries.tsx`, `internal/core/library/lists_released.go`)
  Passed 2026-10-27 (the owner, staging): every sort both ways, release date, the rail's direction and
  Libraries on Home.
- [x] **VQ84** — VQ79's last part: sign the Fire TV stick in as a new device (sign out first, or from a
  fresh Silk profile). Passes when *Your devices* names it "Silk on Fire TV"; a device signed in before
  keeps its name until renamed. (`web/apps/web/src/device.ts`, `tv-device.ts`)
  Failed 2026-10-27 (the owner, nas.example): signed out and in again on the Fire TV, it is still listed as
  "Chrome on Android". It signed in through the discovery's page over WebRTC, whose logged user agent is
  empty: the name likely comes from the discovery's sign-in, not device.ts. Fixed in this batch.
  Found 2026-10-27 (the lead's agent): the name is device.ts's alone, but the device is made by the
  realm sign-in or the code on the discovery's own page, the loader orpheum-connect serves from the web
  client it was packaged with; the bundle live at app.orpheum-app.com that evening named no Silk and no
  Fire TV (its browser list Edge, Firefox, Samsung Internet, Chrome, Safari), so it was packaged before
  An earlier commit. No code changes: to see again once orpheum-connect is packaged and deployed from an
  Orpheum with an earlier commit, signing in from the discovery's page.
  Passed 2026-10-28 (the owner, staging, signed in through staging's discovery once its connectd was
  rebuilt from an Orpheum with an earlier commit): "It now registers as Silk on Fire TV".
- [ ] **VQ85** — D369, the player's own controls on every device, on staging. Passes when: on a
  computer, the Play page shows the app's bar over the film and no browser controls; a mouse's move
  shows it and five still seconds hide it while the film plays; a click on the film pauses
  and plays it, a double click goes full screen and back; Tab and the arrows reach play, full screen,
  mute, the volume and the timeline (Down from the film goes to play), Left and Right on the
  timeline seek 10 s, Space plays and pauses wherever focus is in the player and never presses the
  button focused too, and the subtitle, audio, boost and sound controls under the film still work;
  on a phone (an iPhone and an Android), a tap shows the bar without pausing, a finger anywhere on
  the timeline moves the film there, the volume shows where the phone lets a page set it (not on an
  iPhone, whose mute alone shows), and full screen fills the screen; on the Fire TV, the overlay
  behaves as before with its icons drawn (play, pause, full screen) and its 2.75rem buttons readable
  across the room; and on a cast's remote (a phone casting to the Fire TV or a computer), the remote's
  play, pause, Back 10 s, Forward 10 s and timeline command the receiver, a drag sent once it rests.
  The status tray's signs, now 2.75rem wide, are looked at for how they feel (the owner, 2026-10-27).
  **Seeks shown before they are made (2026-10-29):** on the Fire TV, holding Right (overlay hidden or
  on the scrub bar) runs the marker and time ahead, faster after 2 s and again after 4 s, the film
  playing on where it was, and one seek, to the place shown, follows the release by about half a
  second (the core's log: one `kind=seeked` line, not one per repeat); Left the same backwards;
  pressing Right a few times in a row is one seek too; a click or tap on the timeline (a computer, a
  phone) moves the marker and time there at once and they stay there, not jumping back, until the
  film plays on from there; on a cast's remote, Forward 10 s pressed three times quickly moves the
  remote's time 30 s at once and the receiver seeks once; in Watch Together, a held Right on the
  host's player gives the guests one seek.
  (`web/apps/web/src/page-controls.tsx`, `player-controls.tsx`, `timeline.tsx`, `icon-button.tsx`,
  `tv-controls.tsx`, `cast-remote.tsx`, `seek-ahead.ts`)
  **The lead's check 2026-10-28 (headless Chromium on Linux, computer part only):** the app's bar in
  place of the browser's controls, hidden 5.05 s after a move; a click plays and pauses, a double
  click enters and leaves full screen (emulated); Tab and the arrows reach every control; Left and
  Right seek 10 s; Space plays and pauses from Mute, Full screen or the timeline without pressing
  them. Left for the owner: phones, the Fire TV, a cast's remote, the tray's feel.
  **The iPhone's full screen (2026-11-01):** it filled only the window under Safari's bars, as an
  iPhone has no element full screen; the full-screen button (and a double tap) now opens iOS's own
  player, with the chosen subtitles shown in it, and Done there returns to the Play page where the
  film was. The same in Watch Together's viewer. (`film-fullscreen.ts`)
- [x] **VQ86** — D368, on nas.example or staging, in Chrome on the Mac at home: play a 5.1 film (AAC or
  E-AC-3) and a stereo one. Passes when each starts at the top on its own sound (the tracks note says
  "E-AC-3 5.1 (original)" or "AAC stereo (original)", and the core's `a playback started` line says
  `delivery=ladder`, `ladderQuality=picture`, `ladderSound=sound0-own`); a forced step down to a lower
  quality and back up (DevTools throttling; never "Bypass for network", which skips the service worker the app is loaded through: idiosyncrasies.md) switches the sound to
  the light stereo track and back, the same track and the same mix, with no jump in loudness; and the
  dialogue boost turned on at the top plays the boosted mix, kept as the quality steps. Note whether a
  blip is heard at each switch, and on which device. (`internal/core/chunks/ownsound.go`,
  `internal/node/chunks/arguments.go`, `web/packages/player/src/hls.ts`)
  **The lead's check 2026-10-28 (headless Chromium, Linux):** stereo 5662 and 5.1 5667 start at the
  top on their own sound, "(original)"; throttled to 400 kb/s the playback stepped down to 360p and
  the light stereo track, but had not stepped back up 50 s after the throttle was lifted (once it
  also sat still at 1231.3 s for 50 s; a second run played on: perhaps the throttling's own); the
  boost at the top switches to the boosted mix. Left for the owner: the Mac's Chrome, loudness and
  blips by ear, the boost kept across steps.
  **The owner's run 2026-10-29 (Chrome on the Mac, file 5682, over HTTP):** throttled
  to 3G it stepped down to the lowest quality with no hiccup heard; freed, it climbed one quality a
  minute (11:25:36 to level 1, 11:26:43 to level 2, each shown about 31 s after its step, the forward
  buffer's 30 s below the top), and was paused before reaching the film's own picture: one quality at a
  time after drops, as D352 §3 has it. Faster climbing after a clean recovery is filed with the
  step-up research for the next batch. Left: a 5.1 film's own sound (file 5712), the boost.
  **The owner's run 2026-10-29, file 5712, Chrome on the Mac over HTTP:** the top played the
  film's own 5.1 sound (`ladderSound=sound0-own`); throttled, it stepped down to the light stereo
  (`sound0-96k-ownlevel`) with no stall; freed, it climbed one quality a minute (as with 5682 above);
  the boost played the boosted mix and kept it across a step up, but turning it on froze 2.2 s while
  the server made the boosted mix (fixed in the batch: the loaded sound plays on until the boosted
  mix is ready). Levelling is the player's own gain and switches at once. Left: the boost fix seen.
  **The owner's run 2026-10-29 (Chrome on the Mac, file 5712):** turning the boost on switched
  to `sound0-boost-96k-ownlevel` before the server had made it, and the video froze 2.2 s; the boost
  then played and was kept across a step up. Fixed: also passes when turning the boost on (and off)
  plays on with no freeze, the other mix heard a moment later, and the core's log says `sound <n>
  chosen (boostOn)` then `sound <n> heard (boostOn)` in the playback's link notes.
  **Passed 2026-10-29 (the owner, Chrome on the Mac):** file 5712's own 5.1 sound at the top, the
  step down to the light stereo and back, and, through the discovery over WebRTC (device 30,
  13:55–13:56), the boost turned on and off five times with no freeze: each `sound n chosen (boostOn|
  boostOff)` then `heard`, no stall reported; levelling switched at once.
- [x] **VQ87** — On the Fire TV stick, signed in as the admin through the discovery's page (the core
  deployed, and orpheum-connect packaged, from this batch). Passes when: the banner's menu, opened by
  OK, scrolls within itself as the remote moves down it, in Silk's spatial mode too, every place in
  sight as it is reached, down to Sign out, whose OK signs the TV out; the TV then shows the
  discovery's own sign-in (app.orpheum-app.com), not the server's, and does not go back to the
  household by itself; and, signed in again, the TV removed under *Your devices* on a phone leaves
  its films for that same sign-in within a second or two, with nothing pressed on the TV. (The
  owner, 2026-10-27: no way to sign out on the TV; a removed TV kept its films until pressed; it
  landed on the server's sign-in.) (`web/apps/web/src/app-shell.css`, `live.tsx`,
  `loader-at-client-origin.tsx`, `loader-households.ts`, `internal/core/live/registry.go`)
  **The owner's finding on the Fire TV, 2026-10-29,** fixed and to check again: "When I sign out, it
  goes to the core's sign-in page for a while and then switches to the discovery's sign-in page.
  That would be confusing for a user": Sign out shows only "Signed out. Taking you to sign in…" until
  the discovery's sign-in replaces it, never the server's sign-in between (`web/apps/web/src/app.tsx`).
  **Passed 2026-11-02 (agent, staging, the Fire TV in Silk 138 by adb, as lead-tests, not the
  admin):** the menu opened by OK at its top, and each Down moved the focus one place, the list
  scrolling within itself with the focus ring whole, down to Sign out; OK there and on the question
  signed out to the discovery's sign-in (`orpheum.dev/?signedout=…`), which stayed, once "fix(web): a sign-out with its question open leaves for the discovery's sign-in"
  had fixed the leaving (VQ200). Signed in again and playing The Gold Rush, the TV removed under *Your
  devices* from Chrome on the desktop as lead-tests left its film within about a second, by a link
  straight to that sign-in, with nothing pressed (the page's own events: only the app's home, then
  the sign-in's address).
- [ ] **VQ88** — On nas.example in Chrome, play file 15102 (RealMedia, RealVideo 4 with cook sound) from
  its start, then seek to about the middle and near the end. Passes when it plays with sound from the
  start, in step with the picture, and after each seek; the node's log shows no "a run of chunks did
  not finish" for its sound, and the core's no run of it queued again every few seconds.
  (`internal/node/chunks/arguments.go`, `internal/core/chunks/job.go`, `failures.go`)
- [x] **VQ89** — D370, the watched check and marking by hand, on staging. Passes when: on a computer,
  a phone and the Fire TV, a film played past 90% shows a white check as the last icon of its poster's
  tray in Films, in its library and on Home's rows, readable over pale and busy art and never fading or
  greying the poster; a series whose every episode is watched shows the check on its poster, a partly
  watched one its progress bar and no check; a season's row and each watched episode's row on the
  series' page show the check; a screen reader (VoiceOver on the phone) reads "Watched" after the
  title's name. On the Fire TV with its remote alone, a season's page offers *Mark as watched*, reached
  by the arrows and pressed with OK: every episode's row shows the check at once, the season's row and
  the series' progress follow, and *Undo* puts each back as it was; on a film's page *Mark as watched*
  takes it out of Continue Watching and its next Play starts from the beginning, and *Mark as
  unwatched* leaves it as never watched. Restarting a finished film and stopping early leaves its check,
  and a series' Next Up does not step back. (`web/apps/web/src/watched-mark.tsx`, `status-cues.tsx`,
  `series.tsx`, `title.tsx`; `internal/core/playback/watched.go`, `internal/core/library/watched.go`)
  **Passed 2026-10-28 (the owner, on staging):** the check on posters in Films, a library and Home,
  readable on pale art (a film marked by hand), no fading; a series' check and progress; the
  season's and episodes' rows; Mark as watched / Undo / Mark as unwatched on the Fire TV by the remote;
  the restart and Next Up rules. VoiceOver's "Watched" not tried: the owner, "we'll do a more complete
  voiceover later once the UI is finalized". Seen: the check in a chooser's items looks poor (moot with
  season posters) and belongs over an episode's still (next batch).
- [ ] **VQ90** — D364, casting as a session, on staging: the Mac casting to the Fire TV (Silk), then
  the Mac casting to Chrome on Linux. Passes when: the banner has no Watch Together or Cast item, and
  the tray shows both signs white with nothing on; on a film's Play page on the Mac, Cast's sign leads
  to the devices, and choosing the TV asks "‹Mac› wants to use this device for playback" there, whose
  Use (OK on the remote) moves the film to the TV at its position, full screen with sound, and turns
  Cast's sign blue, gently pulsing, on every page of the Mac (note whether the pulse reads well or
  distracts); browsing to another film on the Mac and pressing Play plays it on the TV with no
  question and no press on the TV, the Mac showing the remote; a film's end with nothing after it, or
  the remote's "Play on this device instead", leaves the TV full screen on "Ready, controlled from
  ‹Mac›", and the next Play on the Mac starts there with sound; the Mac's sign leads back to the
  remote from anywhere, and the Mac loaded again, or woken from sleep, still shows the session;
  taking control back on the TV (Back, OK twice) plays the film on there as its own, or with nothing
  playing goes to its home page, the Mac told; Stop casting on the Mac returns the TV to the page it
  was on before the session and offers "Play here from ‹time›"; the TV switched off for two minutes
  ends the session, the Mac's sign going white. Then with Watch Together hosted on the Mac: its sign
  blue, leading to the session's page, which holds the film's controls, Back to the film and Stop,
  and, while casting too, Stop casting; Cast's sign then leads to that page; the film keeps playing
  for a guest while the host browses away from it; a signed-in guest sees Watch Together's sign blue
  and returns to the session by it. (`web/apps/web/src/status-tray.tsx`, `cast-remote.tsx`,
  `cast-to.tsx`, `cast-ready.tsx`, `cast-receiver.tsx`, `together-plays.tsx`, `live.tsx`,
  `internal/core/live/registry.go`, `internal/core/api/casts.go`)
  **The owner's finding on the Fire TV, 2026-10-29** (staging, device 68): the TV playing file 5664
  as its own, a cast put file 5682 on it; Stop casting on the Mac
  returned the TV to file 5664, and the TV's Back then played file 5682 as its own.
  Fixed, to check again: after Stop casting (or control taken back on the ready page), the remote's
  Back on the TV goes to the page before the one it returned to, never into a film the cast played.
  **The owner's finding, 2026-10-29 ~14:00** (staging, Firefox on the Mac casting a film
  to Chrome on the Mac): turning Watch Together on from Firefox while casting made a second session,
  which the phone joined; the receiver stayed in the cast's own, paused, and the casting page's way to
  Watch Together's page led to that second session, with no film and no controls. Fixed, to check
  again: casting a film, then turning Watch Together on from the sender, the receiver goes on with the
  same film from where it stood, paused if it was, now leading the Watch Together session (no second
  session: the receiver is handed the host's part in it, D364 §11, D372 §8); a guest invited then
  joins and sees that film; Watch Together's page shows the film with its controls, which play and
  pause the receiver, and the casting page leads there; Stop on Watch Together's page and Stop
  casting behave as above. And the reverse order still works: Watch Together on first, then cast,
  the receiver leading the session with the film played.
- [ ] **VQ91** — D367, Back and Up, and a library's Configure, on staging. Passes when, on a computer
  or a phone, no page shows a Back of its own and each page below the top shows its trail
  (`Series › Show › S1E1 Pilot`), each part leading up: (1) Continue Watching on Home → play an
  episode → the browser's (or the gesture's) Back returns Home; played again, the trail's series
  shows the series at that episode's season, and Back from there returns Home, never to the player;
  (2) a series at season 1 → play its last episode and let Up next (or its button) go on into season
  2 → the trail's series shows season 2 with that episode focused, and Back leaves the series for
  the page before it; (3) a series' season picker, a sort or a search on the Libraries page add no
  step to Back; (4) on the Fire TV, reload on a playing film, then press the remote's Back at once,
  before any other key: it goes to the film's page, not out of Silk, and the player's overlay
  shows "‹ ‹film›" and Back, both reached by the arrows, each leaving the player (and, in TV Bro, its
  Back too, since its remote's never reaches the page); (5) a library's Configure on its Libraries
  page, opened by a click and by the remote's OK, shows Show in Films (or Series) for a films (or
  series) library and Show on Home, each changing that page and Home at once; Escape, the remote's
  Back and Close shut it, and the account page no longer lists libraries. On an iPhone's installed
  app, the edge swipe goes back, from the player too. (6) The owner's checks of 2026-10-28: on a
  film's page with a backdrop, on the computer, the phone and the Fire TV, the trail stands whole
  above the backdrop, nothing drawn over its words; playing an episode on the computer and the
  phone, its name, the trail's last part, is a link that leads to the episode's page in one press
  while it plays, and on a film's Play page its name before "Playing" does the same for the film
  (on the Fire TV the overlay's "‹ ‹film›" leads to the film's page); every chooser (the Libraries
  page's library, view and sort, a series' season) shows a down chevron at its button's end.
  (`web/apps/web/src/navigation.ts`, `upward.ts`, `up-trail.tsx`, `series.tsx`, `play-up.tsx`,
  `tv-controls.tsx`, `library-configure.tsx`, `title.css`, `route-chooser.tsx`)
  **The lead's check 2026-10-28 (headless Chromium, desktop and a 390×844 phone size):** (1), (2)
  (staging's test series has seasons 2–4: S2E12 into S3E1), (3) (one Back still leaves the page, though
  the first choice adds a history entry), (5) by click, and (6) on the desktop and the phone size all
  hold. Left for the owner: (4), the remote's OK and Back, the iPhone's swipe, real phones.
  **The owner's findings on the Fire TV (Silk, remote only), 2026-10-29,** fixed and to check again
  there: (7) "I went to [a series]… navigated back into [that series]. The page started scrolled down
  (showing the episode list from ep 9 to 12), so I can't see what button is highlighted": a series
  chosen far down the Series list, and any page a link or OK opens, shows from its top (title,
  backdrop, Next Up's Play, the season picker), the first arrow starting on its first control, even
  after an episode of that series was played from Continue Watching; only Back or Up from one of its
  episodes shows that episode's season with it focused. (8) "Pressing back from the [a series] page
  brought me back to the top of the series list. It should bring me back to the point in the list
  where [that series] is, still highlighted": the remote's Back from a series or film opened far down
  Series, Films or a library's list (past its first 60 titles too) returns to the list with that
  title focused and in view. (9) "The note about the browser asking for permission is persistently
  showing so I've dismissed it", which came back after each reload: on the Fire TV through the
  discovery, the note on the browser's local-network prompt, once dismissed, stays away after a
  reload and on every page (`web/apps/web/src/local-network-note.tsx`).
- [ ] **VQ92** — a remote's commands reach its receiver at once, on staging. Passes when, with the Mac's
  Firefox casting to the Mac's Chrome and Watch Together on (the receiver the host's player), the
  receiver's window behind or beside the remote's: (1) the remote's Pause pauses the receiver's
  sound at once, never seconds later; (2) Play plays it again within a second, and a step forward
  or back moves it at once; (3) the same with Watch Together off; (4) a Watch Together guest on a
  computer whose browser window is behind another follows the host's pause and play at once; (5)
  an iPhone guest whose browser goes to the background still pauses its film, plays nothing
  there, and rejoins the group when shown. In the core's log each player's `groupFollowed` line
  says `heldBy=atOnce` with an `afterChangeMs` of tens of milliseconds, and the iPhone's
  `heldBy=hidden` while it was away. (`web/apps/web/src/handheld.ts`, `together-host.tsx`,
  `together-viewer.tsx`; `web/packages/sdk/src/together-follow.ts`, `together-followed.ts`;
  `internal/core/together/player_log.go`)
- [ ] **VQ93** — a cast to the Fire TV plays smoothly, on staging. Passes when, the Mac casting to the
  Fire TV (Silk) with Watch Together off, a converted film (a ladder) played for five minutes from
  its middle: (1) the picture and sound never stutter or stop after the start; (2) the core's log
  for the cast's session has no `nudged` note and no `sought` note with `why=hold` while it plays,
  and its `followedOnlyPlayer` lines step the group by tens of milliseconds, not hundreds; (3) a
  pause and a play from the remote pause and play it where it was, with no jump; (4) any
  `playerBuffering` line says what held it (`why`, `readyState`, `loadedAheadMs`,
  `sinceRateChangeMs`): note them in the item, as they say whether Silk stalls after a rate
  change on its own. Then (5) with an iPhone guest in a Watch Together session the Fire TV plays
  in, the Fire TV's `nudged` notes and its `playerBuffering` lines are noted too: a stall a few
  hundred ms after each nudge is the platform's, for the idiosyncrasies. (`internal/core/together/group.go`,
  `player_log.go`; `web/packages/sdk/src/together-follow.ts`, `together-refill.ts`)
- [ ] **VQ94** — a Chromium page on WebRTC measures its link, on staging. Passes when, the Mac's
  Chrome and then the Fire TV (Silk, cast to from the Mac) each playing a converted film (a ladder)
  over WebRTC for a minute from its middle: (1) a seek's report in the core's log
  (`a player's seek was ready to play`) carries `linkSamples` of chunks that finished, each of
  128 KB or more, not only a chunk given up, and its `linkKbps` is not 0 once two chunks loaded;
  (2) in a Watch Together session the Fire TV plays in, its first `qualitySwitched` note has no
  `fromQualityKbps` and no `linkEstimateKbps` of 5000, and any later one names the quality it came
  from. Before the fix both showed no finished chunk in `linkSamples` and `linkKbps=0` in every
  report (staging logs, 2026-10-27 and 28). (`web/packages/player/src/cache-hit.ts`, `chunked.ts`,
  `hls.ts`; `internal/core/together/player_log.go`)
- [ ] **VQ96** — a cast to the Fire TV after its first starts with no stall, its sound in step, on
  staging. Passes when, the Mac casting to the Fire TV (Silk) a converted film (a ladder) with
  levelling on, once to teach the device its latency and then three times more, from its start and
  from its middle: (1) the second and later casts start and play on with no wait, and no
  `playerBuffering` line with `loadedAheadMs=0` in the core's log in their first 20 s; (2) the sound
  is in step with the lips from the start; (3) the core's log has the first cast's `soundMoved`
  line, `why=measured` with `flushed=true` and a `latencyMs` of about 330, and for a later cast no
  `soundMoved` line with `flushed=true` unless its `latencyMs` is 40 ms or more from what was kept
  (a later cast's `why=remembered` move, `flushed=false`, is logged only where the player had
  joined its session before it was made); each later cast's `soundMoved` line, where there is one,
  says `rememberedMs=` about 330, never `none`, and no line says `fromWhy=measured` with an
  `earlierFromMs` short of 300. Before the fix every start threw its sound away once or twice, a
  wait of about 2.5 s each (staging, 2026-10-28, 07:18); after the first fix each cast still moved
  its sound back to its latency from a latency read too early, `earlierFromMs` 83 to 259 (staging,
  2026-10-28, 07:47 to 07:51).
  (`web/packages/player/src/gain.ts`, `audio-shift.ts`; `web/apps/web/src/sound-latency.ts`;
  `internal/core/together/player_log.go`)
- [ ] **VQ97** — D367 §10, a remote's Back closes a chooser and the menu without leaving the page, on
  the Fire TV (Silk, spatial navigation on), on staging. Passes when: (1) on a series' page, the
  season picker opened by OK, the remote's Back closes its list, the series' page staying with the
  picker focused, and a second Back then goes back the way the person came; (2) the same for the
  Libraries page's library, view and sort choosers; (3) the banner's menu, opened by OK, closes on
  Back with its icon focused, the page staying; (4) Left or Right in an open chooser or the menu
  closes it, focus on its button; (5) each list's Close, a cross at its top-right that stays put as
  its places scroll under it, is reached by Up and closes it; (6) a season chosen, then Back, goes back past the series as the series' entry was
  replaced, and a library chosen, then Back, returns to the library before. The owner, 2026-10-28:
  "with the chooser widget I can only exit without choosing by pressing the back button, which
  actually brings me back a page." (`web/apps/web/src/navigation.ts`'s `holdBack`,
  `modal-dialog.tsx`, `drop-down.tsx`, `route-chooser.tsx`, `app-shell.tsx`) Changed after the
  owner's check of 2026-10-28 ("it shifted the page up before displaying it", "Left/right didn't
  close it", "the initial selected widget is the close button"): (7) a list opens on its current
  choice, else its first place, never its Close; (8) opening and closing a chooser on a series' page
  scrolled down, and Up to its Close, leave the page where it was, the picker in view after Back; (4)
  holds from the Close too; and should Left or Right leave a list open, the core's log has a client
  problem "a drop-down stayed open at …" saying which layer took the key: note it here. Then, the
  owner the same day ("try to make them overlap the chooser somehow, so that it's obvious what they
  belong to"): (9) on the Fire TV and a computer, a chooser's list opens at its button, its current
  choice over the button, inside the screen, for a chooser near the screen's bottom too; the menu
  at its icon; (10) on a phone, a chooser's list is a pop-up over its button, not the screen's
  middle; (11) the Films and Series pages' sort, and a library's files' Show, are the same chooser.
  **The lead's check 2026-10-28 (headless Chromium):** (7), (8), (10) (a 390×844 phone size), (11),
  and Left, Right, Escape, Close and Up to the Close then Left, each closing with focus on the button
  and no scroll, hold; (9) holds at 1920×1080 and 1280×720, while in a 1920×600 window the season
  list moves up 22 px to stay on screen, its choice no longer over the button's centre. Fixed by the
  owner's rule of 2026-10-29: (9) also passes when, in a 1920×600 window, a season picker near the
  screen's bottom opens with its list cut at the screen's edge, scrolling within itself, its current
  choice over the button; and a chooser so low that the cut list would keep less than half its height
  opens moved up whole, inside the screen. Left for the owner: everything on the Fire TV.
  **The owner's findings on the Fire TV, 2026-10-29,** fixed and to check again there: (12) "The
  menu opens up partially scrolled down (I see from Needs a match to Device tests) rather than
  showing from the top of the list": the banner's menu opens showing its top, its top at its icon,
  its first place focused, on any page, one of its own places too; the choosers still open on
  their current choice. (13) "The X button on the list partially obscures the first item, which
  looks ugly. You can see it partially covering the outline around the first item in the list": in
  the menu and every chooser, on the Fire TV and a computer, the Close covers no part of the first
  place or its focus ring, a first choice opened at a chooser near the screen's top too, and a place
  moved back up to stops below the Close. (14) "As I scroll down the menu, I notice the highlight
  around the current item is slightly cut off at the bottom of the menu canvas": moving down the
  menu and a long chooser to the last place, each place's whole ring shows at the list's bottom.
- [ ] **VQ98** — a cast from a phone to Firefox on a Mac starts playing, on staging. Passes when, the
  phone casting to the Mac's Firefox a converted film (a ladder) from its middle, where it was left
  partway, five times, with Watch Together off: (1) each starts playing within 10 s of Use and plays
  on, never left paused on its first frame; (2) the core's log has, for each, a `groupChanged` line
  with `event=everyoneReady` and `state=Playing` after its `seeked`; (3) any `playerBuffering` line
  with `groupState=Waiting` says what held the wait (`why`, `readyState`, `loadedAheadMs`): note
  them in the item. Before the fix the cast's group waited for its receiver for good, the receiver
  held paused at 1549.673 s with 7 s loaded but a hole of 60 ms in its sound 1.5 s on, saying it
  buffered (staging, 2026-10-28, 07:51). (`web/packages/sdk/src/together-media.ts`,
  `together-buffering.ts`; `internal/core/together/player_log.go`)
- [ ] **VQ99** — a cast to the Fire TV starts playing once, on staging. Passes when, the Mac
  casting to the Fire TV (Silk) a converted film (a ladder) from where it was left partway, five
  times, with Watch Together off: (1) each starts with one play, never a play, a pause and a play,
  no frame or sound moving before the film plays on; (2) the core's log has no `playedWhileHeld`
  line for the cast's session, and its first `playerChanged` line with `playing=true` comes after
  its `groupChanged` line with `state=Playing`; (3) the sound is on from the start, the log's
  `playedWithSound` line coming as it plays, or, where Silk refused it, a `playedMuted` line and the
  "Press OK" line over the film. Then (4), with the cast's Fire TV unplugged from the network just
  after Use, the group plays once its wait's 20 s are up (a `groupChanged` line with
  `event=continuedWithout`), and the film plays from where the Fire TV stands once it is back, its
  `followedOnlyPlayer` lines at debug placing the group where it is. (Its (4) is withdrawn by D371,
  which ended every wait but the leader's: VQ101 checks the cast as D371 built it.) Before the fix the browser's
  autoplay played the film as it loaded, its group still waiting, and the hold paused it again:
  `playedWithSound` at 09:08:06.398, the group's start at 07.207, playing again at 07.981 (staging,
  2026-10-28). (`web/apps/web/src/play.tsx`; `web/packages/sdk/src/together-followed.ts`;
  `internal/core/together/group.go`, `hub.go`, `player_log.go`)
- [ ] **VQ100** — a device whose key its core no longer has lands on the sign-in, not the dead end, on
  production (app.orpheum-app.com, after connect is repackaged with this client). Passes when, the
  Fire TV (Silk) signed in to nas.example by its realm and then removed on the Devices page from another
  device: (1) opening app.orpheum-app.com on the Fire TV ends at the realm sign-in saying "This device
  needs to sign in again", never "Can't reach the server from this network"; (2) its "Copy what
  happened" text names "the offer's proof does not hold"; (3) signing in by the realm there reaches
  nas.example and plays; (4) nas.example's log has the one `an offer's proof does not hold; refused` line
  and the discovery counts no dead end for it. Before the fix the Fire TV showed the network dead end
  after its device was removed (the owner, 2026-10-28, 09:25). (`web/apps/web/src/discovery-gate.tsx`,
  `loader-at-client-origin.tsx`, `loader-at-loader-origin.tsx`; `web/packages/sdk/src/discovery.ts`)
- [x] **VQ101** — a cast alone to the Fire TV plays as the Fire TV plays a film alone (D371 §1, §4,
  §5), on staging. Passes when, the Mac casting to the Fire TV (Silk) with Watch Together off, five
  films one after another, converted ones (a ladder) among them, each played from the Mac: (1) each
  starts with one play, never a play, a pause and a play, and plays on with no stall it would not
  have played alone (the same film played on the Fire TV by itself for comparison); (2) the core's
  log, for the cast's session, has a `leaderChanged` line naming the receiver's device, no `nudged`
  and no `sought` note from it but `why=command` for the remote's seeks, and each `groupChanged`
  line names it as `leaderDeviceId`; (3) the remote's pause, play and a 10 s skip each move the TV
  once, as its own press would; (4) the casting page on the Mac says how the TV plays the film:
  "Plays the file as it is" for a direct one, "Plays it converted · Quality … · Link about …" for a
  converted one, the quality changing as the TV's does. Before D371 every cast of 2026-10-28 was
  steered: a 0.9× nudge loop, a hold's seek back, a run-up's jump, a play-pause-play start.
  (`internal/core/together/group.go`; `web/packages/sdk/src/together-leader.ts`;
  `web/apps/web/src/cast-remote.tsx`)
  **The lead's reading 2026-10-28 (staging's log, runs 11:06–18:20):** (2) holds (the receiver named
  leader on attach, no `nudged`, only `sought why=command`, every `groupChanged` naming it); the
  remote's seeks and pause move the TV once. Found: two of the TV's own 10 s skips were each followed
  ~200 ms later by a seek back to where it had gone, its group's change for its own skip taken as
  another device's; fixed, to be seen again.
  **Passed 2026-10-29 (the owner, staging):** five films cast to the Fire TV each started with one play
  and played on; one quirk filed for the next batch (seeking a 5.1 film: a jump back, then 2-3 s fast with no sound).
- [ ] **VQ102** — in Watch Together the host's screen leads and a guest catches up to it (D371 §2,
  §6), on staging. Passes when the Mac hosts a session, casting to the Fire TV (Silk), and the
  iPhone joins as a guest on mobile data: (1) as the host plays a film, the TV starts at once and
  plays on, never paused or sought for the iPhone, which starts as it can and is nudged or jumps
  into step; (2) with the iPhone's link throttled until it stalls, the TV plays on through each of
  its stalls, and the iPhone catches up to the TV once it has loaded; (3) the host's pause, play and
  a 2-minute jump from the Mac each move the TV once, and the iPhone follows; (4) the core's log
  names the TV's device as `leaderDeviceId` on each `groupChanged` line, and has a
  `followerCaughtUp` line at info for each of the iPhone's catch-ups, by seek or nudge, with how far
  behind it was (`behindMs`), and no `groupChanged` line for an iPhone stall. Before D371 the group
  waited for each player at a start, a play and a seek, and a stalled player was waited for or
  left behind with "continuing without". (`internal/core/together/group.go`, `player_log.go`;
  `web/packages/sdk/src/together-follow.ts`)
  **The owner's run 2026-10-29 (the Mac hosting, cast to the Fire TV, the owner's Samsung S23 in
  Chrome as the guest, on home Wi-Fi):** (1), (3) and (4) hold: one leader (device 17) named on every
  `groupChanged`, the S23 (device 7) caught up by nudges within 0.25 s (`followerCaughtUp`, 21 lines),
  no group change for a follower's stall; the owner saw everything work. Left: (2), the guest's link
  throttled until it stalls (the S23's mobile data is out until 2026-11-01; Chrome's remote debugging
  was too fiddly) — to do then, or with another phone.
  **Again 2026-10-29 23:07 (Mac Chrome casting to the Fire TV, the S23 as guest, pause, play and
  jumps from Chrome and then from the TV's remote):** the S23 stayed in step; one leader (device 64)
  on every `groupChanged`, the S23 (device 67) caught up by a seek at each jump and nudges to within
  0.5 s. (2) still left.
- [x] **VQ103** — a guest casting to a second device follows the group there (D371 §4), on staging.
  Passes when the Mac hosts a session playing a film and a guest, signed in on the iPhone as a
  person of their own, joins it, then casts from the iPhone (the tray's Cast) to a second device of
  theirs (the Fire TV or a computer's browser) and says Use there: (1) the second device plays the
  group's film, in step with the host's screen, and the iPhone's own page stops its film, saying
  "Playing on ‹Device›" with the group's controls its link gives; (2) the host's pause, play and a
  jump each move the second device, which catches up as a follower, never holding the group up; (3)
  the core's log has the second device attach as the guest's member with `role=video`, never named
  `leaderDeviceId`, and `followerCaughtUp` lines from it; (4) Back on the second device ends the cast
  and the iPhone's page plays the film again; Stop casting on the iPhone returns the second device to
  the page it was on. (`internal/core/together/cast.go`; `internal/core/live/registry.go`;
  `web/apps/web/src/cast-ready.tsx`, `together-viewer.tsx`)
- [x] **VQ104** — the lead passes to a guest while the host's screen is gone and comes back to it in
  step (D372 §1–§4, §7), on staging. Passes when the Mac hosts a session playing a film on its own
  player and two guests join, the iPhone first and a computer's browser a minute later: (1) the Mac
  leaving the Play page (to the library) leaves the film playing for both guests with no pause, the
  core's log saying `logEvent=leaderChanged why=leaderGone goneBecause=closed` naming the iPhone's
  device, with its `leaderSeniority` the earlier; (2) the iPhone's own pause is refused it (a link
  without play and pause) and its film plays on, a `sought` note `why=undone` for a refused jump;
  (3) the Mac opening the Play page again catches up as a follower, the guests' films never jumping,
  and the log says `why=hostInStep` with `offGroupMs` within 500 ms a few seconds later; (4) with
  every device's Play page closed, the group is Paused where it stood, "Everyone's screen left;
  paused at …" on the host's session page. (`internal/core/together/leader.go`;
  `web/packages/sdk/src/together-follow.ts`, `together-leader.ts`)
  **Passed 2026-10-29 (the owner: the Mac's Chrome hosting, the Samsung S23 and the Mac's Firefox as
  guests):** "it worked". Found: the host away from the film saw no way back to watching on the session's
  page (the link drawn as the primary action as a stopgap; the page's revamp is next batch).
- [x] **VQ105** — a locked phone's lead passes at once (D372 §2, §5), on staging. Passes when the
  iPhone, a guest, leads a session (the host put the film on, then browsed away from the session's
  page, D372 §6; a guest cannot choose the film) beside a computer's browser, a second guest that
  joined after it: (1) locking the iPhone hands the lead to the browser at once, the browser's film
  playing on without a stop, and the log says `goneBecause=hidden`; (2) unlocking the iPhone catches
  it up as a follower, and closing the browser's page then gives the lead back to the iPhone, its
  `leaderSeniority` the moment it was unlocked; (3) a stall of the leader's (its link throttled)
  holds the group (`waitingFor`) and does not move the lead. (`internal/core/together/leader.go`;
  `web/packages/sdk/src/together-follow.ts`, `together-connection.ts`)
  **Run 2026-10-29 (the owner, staging, sessions 45 and 61):** (1) passed: the S23 leading, locked,
  the lead went to Firefox at once (`goneBecause=hidden`), its film playing on; (2) the lead came back
  to the S23 when Firefox's page closed, and the host's Chrome returning caught up, then led
  (`hostInStep`); but the unlocked S23 sat paused ~20 s, deaf on its old connection until its silence
  limit reconnected it (fix being built); (3) the host's Chrome throttled stalled and held the group
  (`waitingFor`), the lead staying, but after a chunk given up as stuck it never fetched again (fix
  being built). Firefox's DevTools throttling does not slow its WebRTC path; Chrome's does.
  **Criterion (3) replaced by D373 (the owner, 2026-10-29):** no one's stall holds a Watch Together
  group, and a leader stalled past its 3 s grace passes the lead (`goneBecause=stalled`); VQ128
  checks the new rule, and (3) is no longer to pass as written.
  **Passed 2026-10-29 (the owner, staging):** (1) the S23 leading, locked, passed the lead at once (`goneBecause=hidden`); (2) unlocked, it came
  back in step (VQ126's fix); (3) replaced by D373 (VQ128 passed).
- [x] **VQ106** — a person is in one session at most (D372 §8), on staging. Passes when: (1) a
  guest by link alone on the iPhone, in the Mac's session, opens a link to a second session (another
  person hosting it on a computer's browser) in a new tab and is asked "Leave ‹host›'s session to
  join this one?"; Don't join leaves them in the first, Leave and join takes them out of it (the
  Mac's roster drops them) and into the second; (2) the Mac's host, signed in, opening a link to
  the second session is asked "Leave your own session to join this one? It ends for everyone in it."
  and, saying so, their guests' pages say the host ended it; (3) a host pressing Start a session
  while hosting one on another device of theirs (the session lost from this tab) ends the old one,
  its guests told it ended, and the core's log says `a person left a session for another`.
  (`internal/core/together/places.go`; `web/apps/web/src/together-join.tsx`)
  **Passed 2026-10-28 (the owner, on staging):** all three; the core's log has the `in_another_session`
  refusals (a host's own and "you are in dan's … session") and `a person left a session for another`
  for a host and for a guest.
- [x] **VQ110** — a player alone or leading starts, and plays on after a seek, only once what it
  loaded outlasts its next chunk's load, and its dropped frames reach the log (the owner, 2026-10-28),
  on staging. Passes when, the Mac casting to the Fire TV (Silk) with Watch Together off: (1) a
  series, resumed part way where its first chunk ends a second or two past the resume point
  (as at 1798.9 s on 2026-10-28), starts with one play and does not stall in its first 30 s; the
  core's log line "a playback started playing" for it has `loadedAheadMs` above `waitedForAheadMs`,
  and `waitedForAheadMs` about 1.5 times `nextChunkLoadMs`; (2) a 2-minute jump from the remote plays
  on with no stall after it, its "a player's seek was ready to play" line saying the same, and no
  pause told for it ("a player's play state changed" with `kind=paused`); (3) the same on the Mac
  alone, in Chrome, where a start with plenty loaded plays at once (`waitedForAheadMs` absent); (4)
  played for ten minutes, a report whose dropped frames passed 1% gives a line "a player dropped
  frames" with `droppedFrames` and `totalFrames` near when the picture stuttered, if it did, and a
  smooth one none. (`web/packages/sdk/src/start-gate.ts`, `ready-ahead.ts`;
  `web/packages/player/src/next-chunk.ts`, `frames.ts`; `internal/core/api/report.go`)
  **The lead's check 2026-10-28 (headless Chromium):** (3) as worded fails: no start was logged
  without `waitedForAheadMs`, even after the film page preloaded 12 s, though each played 160–300 ms
  after the page asked (whether "absent" is the right test is the owner's to say); the 2-minute seek
  holds (`readyAfterMs=307`, ~115 s loaded, no pause told).
  **Passed 2026-10-29 (the owner, staging):** a series cast to the Fire TV resumed with one play
  (`playingAfterMs=3333 loadedAheadMs=10025 waitedForAheadMs=881`); the jumps played on, the second
  with the fast silent play filed for the next batch (VQ101's quirk).
- [x] **VQ111** — a cast's receiver waits for enough loaded before its first play, whenever its
  session's welcome comes (staging, 2026-10-28: fileId 5664, cast from the Mac, played with
  `loadedAheadMs=0` and no `waitedForAheadMs`), on staging. Passes when, the Mac casting five films in
  a row to the Fire TV (Silk), Watch Together off: every "a playback started playing" line for device
  60 carries `waitedForAheadMs` or a `loadedAheadMs` at or above refillAheadMs (10000), none has
  `loadedAheadMs=0`, and no "a player began holding its group up … why=notReady" follows its group's
  `leaderPlays` within a second; and no `qualitySwitched` note from it jumps to the film's own picture
  while its `linkEstimateKbps` is below that picture's rate (2026-10-28: 1896 → 15635 kb/s on a 1.5
  Mb/s estimate, the leader kept the film's own picture as a follower would). (`web/packages/sdk/src/together-follow.ts`, `together-connection.ts`, `following-a-group.ts`)
  **Passed 2026-10-28 (the owner's casts, read by the lead from staging's log, runs 13:51–18:20):**
  every device-60 `a playback started playing` line for a cast carries `waitedForAheadMs`, none
  `loadedAheadMs=0` (the least 1067 against 980); no switch to the film's own picture above the
  estimate (the one later step up, 1896 → 9018, on 16556 kb/s).
- [x] **VQ112** — a Fire TV's second start says what came before it (staging, 2026-10-28: fileId 5670
  on the TV's own Play page logged `kind=started` twice 0.6 s apart, 9 s loaded, 6 of 21 frames
  dropped, and nothing between), on staging. Passes when, the Fire TV (Silk) playing three films from
  their Play pages, resumed part way: each "a player's play state changed" line with `kind=started`
  carries `startedAfter` (`play` for the first) and `stoodMs`, with `readyState` and both buffers;
  and, where a start comes twice again, the second's `startedAfter` says whether a wait for data
  (`waiting`) or a seek (`seeking`, with `skippedBy` on a `kind=seeked` line if hls.js made it) came
  between, which decides the cause the first commit could not. (`web/apps/web/src/play-state.ts`,
  `internal/core/api/report_context.go`)
  **Passed 2026-10-28 (the owner's runs, read by the lead from staging's log):** every device-60
  `kind=started` line carries `startedAfter`, `stoodMs`, `readyState` and both buffers; second starts
  name a wait (`startedAfter=waiting stoodMs=377`, `1172`) or hls.js stepping over a 40 ms hole in
  the sound (`skippedBy=hole`, then `startedAfter=seeking stoodMs=312`).
- [x] **VQ114** — a player's start rests on a lone first chunk no more (staging, 2026-10-28: device
  60, file 5766, its first chunk at 28384 kb/s gave `nextChunkLoadMs=653`, it played with 1.07 s
  loaded, its next chunk loaded at 9636 kb/s and it ran dry a second in, `startedAfter=waiting`), on
  staging. Passes when, the Fire TV (Silk) resuming three films part way from their Play pages, the
  first after it played another for a minute: the core's "a playback started" line carries
  `rememberedLinkKbps` near the link it measured; each "a playback started playing" line that has
  `waitedForAheadMs` carries `nextChunkLoadFrom`, `remembered` or `firstSample` (`none` for a device
  never measured on the network, with `waitedForAheadMs=10000`), never a start resting on the first
  chunk where that was faster than the remembered link; and no film stalls in its first 30 s
  (`startedAfter=waiting` absent on its second start). (`web/packages/player/src/next-chunk.ts`;
  `internal/core/playback/playback.go`)
  **Passed 2026-10-28 (the owner's casts, read by the lead from staging's log, runs 15:24 and
  18:20):** each start that waited carries `nextChunkLoadFrom` (`confident`, `remembered` or
  `firstSample`, the last only where slower than the remembered link); `rememberedLinkKbps` follows
  the film before; no `startedAfter=waiting`. Seen: the remembered link is 0 on the first start after
  each deploy (it is not kept across one), and one start waited 10 s on a confident estimate of 753
  kb/s (next batch: why the estimate was so low).
- [ ] **VQ115** — the WebRTC media relay carries every load across a deploy and loads chunks as fast
  as the LAN carries them, not one 64 KiB piece for each round trip through a busy page (staging,
  2026-10-28: on `path=direct_ipv4` the Fire TV loaded every chunk at 15–18 Mb/s at most, 1.5–5 Mb/s
  in a playback's first seconds; the first relay that sent ahead spoke only its own protocol, and
  across the deploy both the Fire TV and the Mac showed a black page), on staging. Passes when, on
  the Fire TV (Silk) and the Mac (Chrome), each on WebRTC through the discovery: a tab left open on a
  film's page while staging is redeployed with this change goes on loading (posters, a film played
  from it), then a reload opens the app and plays a film; and, on the Fire TV on the LAN's direct
  path, playing two films from their Play pages, the core's `playerNoted` lines with
  `kind=chunkLoaded` for device 60 carry `loadedKbps` well above 18000 for chunks of a megabyte or
  more, and the Mac seeks twice across a film with no stall and no error.
  (`web/packages/sdk/src/rtc-media.ts`)
  **The lead's reading 2026-10-28 (staging's log):** the Fire TV on the LAN's direct path (`host udp
  192.168.1.150 → 192.168.1.199:8443`) still loaded chunks of a megabyte or more at ~15.6 Mb/s on
  average, as before the change: the relay's credit window did not lift it. This fits VQ117's finding
  that Chromium's own WebRTC path is the limit (the Chrome fixes and the LAN-origin tab, next batch).
- [x] **VQ116** — a Fire TV holds its quality while plenty is loaded, and a change of sound with a
  change of quality throws no loaded sound away (staging, 2026-10-28: device 60, file 5671, one
  slow sample of five, 376471 B in 1200 ms, lowered the ceiling from the film's own picture to
  q700k432p with 37.3 s loaded, `chose started, ceiling 3 to 1`, and the sound's change from the
  copied `sound0-own` to `sound0-96k-ownlevel` left 2.5 s of sound against 38.6 s of picture and
  stalled the TV 1172 ms, `startedAfter=waiting stoodMs=1172`), on staging. Passes when, the Fire TV
  (Silk) playing file 5671 for 2 minutes from where it was: no "chose … ceiling a to b" step
  down while 20 s or more is loaded ahead (a held lowering reads `held at`, with what was loaded);
  and, where the quality does change between the top and a lower one (a later step down below 20 s,
  or a step up), the stall and seek reports show the sound's buffer reaching as far as the picture's
  after the change, no `startedAfter=waiting` follows it, and the sound plays on across it with no
  gap or blip heard. (`web/packages/player/src/abr.ts`, `link-estimate.ts`, `hls.ts`)
  **The lead's reading 2026-10-28 (staging's log):** as written it holds (no lowering with 20 s or
  more loaded; the sound's buffer reaches the picture's after each change; no `startedAfter=waiting`
  after 15:24). But at 15:33:05 file 5671 dropped from 9018 to 696 kb/s 2.3 s into play on an estimate of
  69 kb/s while its picture chunks had loaded at 14–18 Mb/s, and stayed there 75 s: most likely a
  chunk given up, counted as a sample of the few bytes that came over the time waited, which a
  harmonic mean cannot outvote for five chunks. A quality switch's note now says why the controller
  chose it and its samples (`why`, `linkSamples`), so the next one says; how a given-up chunk counts
  is the owner's to decide (D352).
  **Passed 2026-10-29 (the owner, staging):** file 5671 on the Fire TV held its quality through seeks, and dialogue boost on and off played on with no
  hang (on the receiver's own controls, reached after taking control back: the cast's remote has none yet, filed).
- [x] **VQ117** — the Throughput probe measures where the WebRTC path's ceiling is on the owner's
  devices (webrtc-throughput-2026-10-28.md: in Chrome and Firefox here, one connection carried
  25–30 Mb/s at a 50 ms round trip against 240 and more over HTTP, its window cut by the browser's
  own UDP socket overflowing; the Fire TV saw 15–30 Mb/s), on staging with Device tests on. Do: on
  the Fire TV (Silk) and on the Mac (Chrome, then Firefox), each reached through the discovery on
  WebRTC, open Device tests, then Throughput, type the core's LAN address (its LAN name over HTTPS
  as the discovery publishes it, such as `https://192-168-1-199.<core id>.<zone>:8443`), press
  Run and note the three rates. Passes when each device's core log has its `a throughput test ran`
  line with all three legs (or a leg's failure in words) and three `a throughput test's body was
  sent` lines, and the lead has read them: the transport and the relay alike near 15–30 Mb/s on the
  TV with the LAN name several times faster confirms the WebRTC path as the limit (the research's
  §4, fixes 1 and 2); the relay well below the transport puts it in the relay; all three alike puts
  it in the TV's link. (`web/apps/web/src/lab-throughput.tsx`,
  `internal/core/api/lab_throughput.go`)
  **Passed 2026-10-28 (the owner's runs, read by the lead from the core's log):** old Fire TV over WebRTC
  36 / 29 Mb/s (transport / relay), at the LAN name over HTTPS 190–224; Mac Firefox over WebRTC 106 / 102;
  Mac Chrome over WebRTC 23 / 18; Mac Safari at the LAN name over HTTPS 108 / 80. The LAN leg was run from
  the LAN page rather than typed. Only Chromium browsers are slow over WebRTC: the path, not the devices
  (research/webrtc-throughput-review-2026-10-28.md; the fixes in the plan's last sections).
- [ ] **VQ118** — the efficiency audit's cheap fixes make a Fire TV's playback lighter
  (web-client-efficiency-audit-2026-10-28.md, findings 1, 2 and 3), on staging. Do, before the
  batch is deployed and again after, the same way each time: (1) cast from the Mac to the Fire TV
  (Silk) a converted film (a ladder) from where it was left, three times, Watch Together off, and
  note from the core's log each cast's `a player dropped frames` lines in its first two minutes and
  its `a playback started playing` line's `playingAfterMs`; (2) on the Fire TV reached through the
  discovery on WebRTC, open a converted film's page, wait 15 s, press Play, three films. Passes when
  (1) the casts after drop fewer frames than those before (fewer `a player dropped frames` lines,
  or smaller counts on them) and start no later; and (2) each film's `a playback started playing`
  line after has no `preloadSteps`, and the core's log (the requests at debug) has none of the
  film's first chunks asked for twice, once on its page and again after Play, as before it was
  (the preload's ~10 s fetched on the film page and again by the Play page). Note
  `playingAfterMs` before and after for (2) too. (`web/apps/web/src/watching.ts`,
  `frame-timing.ts`, `tv-controls.css`, `status-tray.css`)
  **The lead's reading 2026-10-28 (staging's log):** (1) not met on the data: five casts after the
  batch dropped 38 frames in 6 lines against 31 in 6 before, and started no sooner; the frame
  sampling changed in the same batch, so the counts compare roughly. (2) only one film went from the
  TV's film page to Play, one that starts on its own picture and never preloads, so it cannot show
  the fix.
- [x] **VQ119** — file 5667 plays from its start and from a resumed point (staging,
  2026-10-28: file 5667 on the Mac in Firefox, its own 5.1 sound copied failed chunk 0 twice, "raw
  00000.mp4 holds 0 to 12267 ms of the run, not its chunk's 10176 to 12262 ms", and the film could not
  play, `cannot_convert`; its first keyframe is before its start, discarded, so its grid starts
  10.176 s in), on staging once deployed. Passes when, on the Mac (Firefox or Chrome), file 5667
  plays from its start with its sound, the tracks note naming its own 5.1 sound, for a
  minute with no error, and again from a point resumed past the half hour; and the core's log has no
  `runs of chunks failed at the same chunk again` for `5667/chunks-sound0-own`. The same for one
  other film of the shape (file 5691, or file 5697).
  (`internal/node/chunks/arguments.go`, `maker.go`, `listed.go`; `internal/core/chunks/offer.go`)
  **Passed 2026-10-28 (the lead, headless Firefox on Linux against staging, and the owner's Firefox
  on the Mac):** 5667 played a minute from its start and from 2000 s, and 5691 a minute from its
  start, each on its own 5.1 sound (`ladderQuality=picture ladderSound=sound0-own`, the tracks note
  "AAC 5.1 (original)"); the node made chunk 0 of each own sound from the grid's start (10176 ms for
  5667) and logged no `did not finish`, no `same chunk again` and no `cannot_convert`. On the owner's
  Mac, Firefox refused 5667's own (HEVC) picture (`bufferAppendNoProgress`) and the player fell back
  to 800p with the server's stereo, as designed; why the Mac's Firefox refused it is filed for the
  next batch.
- [ ] **VQ120** — the chunked player's link estimate of 2026-10-29 (D352's revision, P1–P6 of
  research/bandwidth-sampling-2026-10-29.md) holds a Fire TV's quality through a stuck chunk, and a
  start after a deploy rests on the link remembered before it, on staging once deployed. Do: (1) on
  the Fire TV (Silk) over WebRTC, play a converted film (file 5671, or another whose top is
  its own picture) for ten minutes, Watch Together off; (2) let the core restart (the deploy itself
  will do), then start a film on the Fire TV within the hour. Passes when (1) no `ceiling … to …`
  step lowers the quality on a chunk given up without progress (its note says `stuck`) while its
  estimate stays near what its other chunks measured, the given-up chunk's note naming its transport
  and connection, and no switch to the lowest quality follows one; and (2) the start's `a playback
  started` line has a non-zero `rememberedLinkKbps` with `rememberedLinkSamples` 2 (or less, if over
  an hour old) after the restart, and its seek/start note says `nextChunkLoadFrom` `remembered` or
  `confident`, not `none`. (`web/packages/player/src/link-estimate.ts`, `abr.ts`, `next-chunk.ts`;
  `internal/core/playback/speeds.go`, step 0063). Through the discovery's page (its loader
  repackaged with loader minor 3), a stuck chunk's note names `WebRTC connection N`, not `http`,
  on the Fire TV (`web/apps/web/src/client-main.tsx`, `client-reaches.ts`, `kept-path.ts`).
- [ ] **VQ121** — how far ahead the chunked player loads (research/forward-buffer-2026-10-29.md P1,
  P2 and P6, accepted by the owner 2026-10-29): the Fire TV's real limit for a page's picture, and a
  step up shown within about 40 s, on staging once deployed. Do: (1) the report's T4: on the Fire
  TV (Silk, the AFTSSS), play a film whose top is its own picture at 9 Mb/s or more (file
  5671) for three minutes from its start, Watch Together off; then the same on the Chromecast
  receiver (`apps/cast-receiver`); (2) on the Mac in Chrome, play a converted film held below its
  top (DevTools' throttling at about 0.6 Mb/s for a minute), then lift the throttling and play on
  for three minutes. Passes when (1) the play reports' `linkNotes` in the core's log carry the
  device's `device memory … video buffer … assumed` line and a `loads ahead … by …` line at the
  top, and either no `buffer full on video` line, or, where one comes, its seconds behind and ahead
  at its rate give the device's limit (about 31 MB: Chromium's low-end limit, which a
  `deviceMemory` of 1 assumed rightly; about 157 MB: the ordinary one, so the 30 MiB assumed is too
  low and the player loads less than it could), no stall follows it, and the `loads ahead … by cut`
  after it stays in force; and (2) the Mac's notes have a `loads ahead 30.0 s … by below top` line
  while held low, and a `stepped up to N shown after M ms` line with M at most about 40 000 (the
  30 s loaded below the top and a chunk), against the minutes seen before. Also on the Fire TV, at
  the top of a film whose own sound is 5.1 (file 5671's): the `device memory` line says `sound … MB
  assumed`, a `loads ahead … by sound bytes` or `by sound floor` line follows the first chunk of
  sound, and a `buffer full on audio` line, if one still comes, gives the sound's own rate, so its
  seconds give the sound buffer's real limit (about 2 MB: Chromium's low-end; about 12.6 MB: the
  ordinary one). And a skip back on the Fire TV's remote, three times during the film, shows no
  main-thread stall over 1 s: no `main thread stalled` line in the notes, or, where one comes, the
  events it names before it say what froze the page, and the picture plays at its own speed after.
  (`web/packages/player/src/forward-buffer.ts`, `main-thread.ts`, `chunked.ts`, `hls.ts`,
  `step-up.ts`)
- [x] **VQ122** — On the Fire TV, cast a film to it from a phone so its receiver is in controlled
  mode, then hold OK on its remote for two seconds. Passes when the receiver stays controlled (the
  *Controlled from* line does not end the session, the phone still controls it), and OK pressed
  twice quickly afterwards still takes control back. Before this change every repeat of the held
  key counted as another press, and Android's first repeat comes 400–500 ms after the press, inside
  the 500 ms window (`design/research/starts-chunks-remote-2026-10-29.md` §3.3).
  (`web/apps/web/src/cast-receiver.tsx`, `input-commands.ts`)
  **Passed 2026-10-29 (the owner, staging):** a held OK kept the cast controlled, and OK twice
  quickly took it back. The first two runs failed: Silk sends a held OK as unmarked presses with no key up
  (idiosyncrasies.md).
- [x] **VQ123** — In Chrome on the Mac, through staging's discovery, open the home page, then in
  DevTools tick "Bypass for network" (Application → Service workers; untick it again at the end:
  Chrome keeps it ticked for every later session) and open a film you have not
  opened since the page loaded. Passes when, after about a second, the page says "This page could not
  load." with Reload, Copy what happened, and the line that developer tools may be bypassing the
  service worker, never a black page; Reload with the box unticked opens the film's page; and the
  core's log has a "page reported a failure" line naming the screen and the file. Then, with the box
  unticked, open the home page in a tab after staging's connectd was rebuilt since that Chrome's last
  visit, and open a film: its page opens. (`web/apps/web/src/screen-load.tsx`, `sw.ts`)
  **Passed 2026-10-29 (the owner, Chrome on the Mac):** the first load through the discovery after the
  deploy (13:55, over WebRTC) opened a film's page and player with no failed screen.
- [x] **VQ124** — On the owner's Android phone, signed in, join a Watch Together session as a guest
  from a host on another device, and have the host end it. Passes when the phone's page goes to the
  home page (not "This session is over. You can close this page.") with a note at its top, "The host
  ended the Watch Together session.", that the × closes, and that goes on its own once another screen
  is opened. Then the same on the Fire TV signed in as a guest: the host ends the session, the TV
  goes home with the note, and the remote's arrows reach its × and OK closes it. A guest joined by
  link alone, not signed in, still sees the page that says the session is over.
  (`web/apps/web/src/together-viewer.tsx`, `notice-note.tsx`)
  **Passed 2026-10-29 (the owner, staging):** the session ended by the host: the S23 and Firefox went Home with the note.
- [x] **VQ125** — In Chrome on the Mac, over WebRTC through staging's discovery (the core's log says
  `over=webrtc`), play a film; it covers both ways a throttled load stopped playback for good. First,
  the stuck chunk: in DevTools' Network panel set a slow preset until the film stalls, wait half a
  minute, and set throttling back to none. Passes when playback resumes by itself within a few
  seconds of unthrottling, with no reload, and the core's log shows the player's link notes "asked
  again for a chunk of …, time n in a row" (or "failed to load" with the carrying connection) during
  the stall. Second, the load that failed: set Offline and hold it until the film stalls or the
  player's loading fails, then set throttling back to none. Passes when the page shows no failure
  ("The film could not play: …") at any point, only its wait, playback resumes by itself within a few
  seconds of going back online, with no reload, and the core's log shows the link notes "loading
  stopped: networkError … ; status …; carried …; asking again in 8000 ms, time 1 in a row" (then
  16000 ms, time 2 …) while offline and "asked again to load from … s, the browser back online" as
  it comes back; no "the player failed" line, and any that appears for another cause names hls.js's
  error after the words. Then Offline held for two minutes: it resumes within about forty seconds, a
  silent connection replaced. (`web/packages/player/src/abr.ts`, `chunked.ts`,
  `web/packages/sdk/src/rtc-transport.ts`, `rtc-request.ts`)
  **Passed 2026-10-29 (the owner, staging):** a film alone, Wi-Fi off until it stalled, on: it resumed by itself.
- [x] **VQ126** — a follower unlocked catches up at once, on staging. With the S23 (Chrome) following
  a Watch Together session that another device leads and plays, lock the S23 for about 30 s and
  unlock it, pressing nothing. Passes when, within about 3 s of the unlock, the S23's film plays at
  the group's place, not paused where it was locked; and the core's log, for the S23's video device,
  says how its socket was found: a `shownAgain` line (`socket=alive`, its `roundTripMs`), or a
  `playerNoted kind=reconnected … why=shown` line, then `followerCaughtUp`, and no ~20 s of silence
  before a `why=silent` reconnect, as on 2026-10-29. If instead the S23 shows its sound button (a guest's
  Sound back, the host's Sound on) over a paused or muted film and a `playRefused` line says why (with its `page` activation), that
  is acceptable: its tap plays it at the group's place; say which of the two happened. Repeat once
  with the S23's Wi-Fi off during the lock (mobile data) to see the reconnect path.
  (`web/packages/sdk/src/together-connection.ts`, `together-follow.ts`, `together-sound.ts`)
  **Passed 2026-10-29 (the owner, staging):** the S23 follower, locked and unlocked, played on in step
  without a press.
- [x] **VQ127** — a tab left open across a deploy keeps its posters, on staging. In Chrome on the
  Mac, open a household through staging's discovery and leave the tab on the home page; deploy
  staging so that the discovery is rebuilt (a new `sw.js`); then reload the tab once, ordinarily.
  Passes when every poster shows after the reload, none 404 in the Network panel (their remote
  address is never Cloudflare's), and the core's log for that page has no `the service worker sent a
  media load to the network` or `could not load a media file` line. If the log has a
  `no service worker controls the page … reloading the page once` line, the page reloaded itself
  and must show its posters after it. (`web/apps/web/src/sw.ts`, `web/packages/sdk/src/rtc-media.ts`,
  `web/apps/web/src/loader-at-client-origin.tsx`)
  **Passed 2026-10-29 (the owner, staging):** shown by VQ137's run: a tab left open across a deploy loaded its film at once, no 404s.
- [x] **VQ128** — no one's stall holds a Watch Together group (D373), on staging. With the Mac's
  Chrome as the host, leading a session that plays, and the S23 (Chrome) and Firefox following,
  throttle the Mac's Chrome in DevTools' Network panel (a slow preset, or Offline) until its film
  stalls. Passes when: (1) within about 3 s of its stall the lead passes (the core's log says
  `leaderChanged … why=leaderGone goneBecause=stalled … stalledMs=…`), and the S23 and Firefox keep
  playing with no pause and no "Waiting for" (no `waitingFor` in the log); (2) unthrottled, the
  Mac's Chrome catches up to the group as a follower and takes the lead back once in step
  (`why=hostInStep`), and a `stallNotWaitedFor` line says its stall and how long it lasted; (3) a
  follower stalling (the S23 on a slow link, or throttled) pauses no one, its own
  `stallNotWaitedFor` line saying `isLeader=false`. (`internal/core/together/leader.go`,
  `group.go`; `web/packages/sdk/src/together-follow.ts`)
  **Passed 2026-10-29 (the owner, staging):** Chrome leading stalled (Wi-Fi off): the lead passed to the S23 after the grace, the others played on.
- [x] **VQ129** — only a device playing the film leads, a lone player's stall is waited for, and a
  leader recovered within its grace catches up forward (D374), on staging. (1) With the Mac's
  Chrome hosting a Watch Together session that plays, and the S23 (Chrome) following, cast from the
  Mac to the Fire TV. Passes when, from the cast on, no `leaderChanged` line names the Mac's Chrome
  video device as leader while the Fire TV plays: the Mac's lead passes as its Play page is left
  (`goneBecause=closed`, or `noFilm` with `passedOver=<its id>:noFilm`), and the Fire TV takes it as
  the host's player (`why=attached` or `why=hostInStep`); stop the cast and play on the Mac again to
  see it lead (`why=putOn` or `attached`). If the TV can be throttled (Silk's developer tools, or the
  router), a 1–2 s stall of it while the S23 plays makes the TV catch up, below. (2) The Mac's Chrome
  hosting and leading, the S23 following: force a brief stall of the Mac's film, a quality switch
  (DevTools' Network panel, Fast 4G for a moment and back) or 1–2 s Offline. Passes when the Mac's
  player catches up forward, a `stallNotWaitedFor … isLeader=true … catchesUp=true behindGroupMs=…`
  line then `leaderCaughtUp … offGroupMs=… catchingUpMs=…`, keeps the lead, and the S23 never jumps
  back (no `followerCaughtUp` on the S23 with a negative `behindMs` near it, no stretch seen twice).
  (3) Lock the S23 (its page hidden, it contends no more) and stall the Mac's film as in (2), longer
  than 3 s. Passes when the group waits for the Mac (`groupChanged state=Waiting event=waitingFor`),
  the lead does not pass, and the film plays on from where it stalled once the Mac loads, nothing
  skipped. (`internal/core/together/leader.go`, `group.go`;
  `web/packages/sdk/src/together-follow.ts`, `together-leader.ts`;
  `web/apps/web/src/together-viewer.tsx`)
  **Passed 2026-10-29 (the owner, staging, (1)):** cast from Chrome (host) during Watch Together: at 22:56:30
  Chrome's player closed and the S23 led; 22:57:06 the TV took the lead as the host's (`hostInStep`); stopping the
  cast, the S23 led (22:57:40), then Chrome (22:57:47); Chrome never led while the TV played.
- [x] **VQ130** — a page's WebRTC connection comes back after the network does, in every browser, on
  staging. With the Mac's Chrome and Firefox both in one Watch Together session over WebRTC through
  staging's discovery (the core's log says `over=webrtc` for each), and the S23 a third member, play a
  film. (1) Turn the Mac's Wi-Fi off for a minute, then on. Passes when both Mac browsers make a new
  WebRTC connection within about 15 s of Wi-Fi returning (the core's log shows an `rtcapi` offer from
  each, and each reports "a WebRTC connection to the core is back after … ms with none; the last was
  taken as gone because …"), their session sockets reconnect (`playerNoted kind=reconnected`), and both
  catch up with the group, Firefox's Play page with no spinner left. (2) In Chrome alone, Wi-Fi off
  until its film stalls completely, then on: its film plays again from where it stalled, and within
  about 5 s of playing again it is back in step with the S23 (its `kind=reconnected` line comes within
  a few seconds of its "is back after" line, never tens of seconds later).
  (`web/packages/sdk/src/rtc-transport.ts`, `rtc-peer.ts`, `rtc-reopen.ts`, `rtc-connect.ts`,
  `together-connection.ts`;
  `web/apps/web/src/kept-path.ts`)
  **Passed 2026-10-29 (the owner, staging):** Mac Wi-Fi off ~1 min with Chrome and Firefox in the session: both reconnected within a second
  of each other at 19:09:50 once it returned (with VQ131's run).
- [x] **VQ131** — a Watch Together guest who was signed in before joining by the host's link reaches
  the server again after a lost connection, on staging. On the Mac, Firefox signed in as dan at the
  household through staging's discovery (the core's log says `over=webrtc` for it), and Chrome as the
  admin hosting a Watch Together session; the host makes a link, and Firefox opens it and joins
  (`someone joined by a link … name=dan isSignedIn=true`). Play a film. Turn the Mac's Wi-Fi off for
  a minute, then on. Passes when Firefox makes a new WebRTC connection within about 15 s of the Wi-Fi
  returning (its "a WebRTC connection to the core is back after … ms with none" line), its session
  socket reconnects and it catches up with the group, and connectd's log has no "the offer's proof
  does not hold" for the page and the core's none of `an offer's proof does not hold` from the join
  on. (`web/packages/sdk/src/discovery.ts`, `rtc-transport.ts`;
  `web/apps/web/src/discovery-gate.tsx`, `loader-at-client-origin.tsx`, `kept-path.ts`)
  **Passed 2026-10-29 (the owner, staging):** Wi-Fi off ~1 min with both Mac browsers in the session; Firefox
  (`back after 69261 ms`) and Chrome (`58049 ms`) reconnected within a second of each other once it
  returned; the session had paused as everyone had gone (D372 §7, the S23 locked).
- [x] **VQ132** — Show debug tools, and Watch Together's leader debug view, on staging. On the Mac's
  Chrome (signed in as the host) and the S23 (a signed-in guest), turn on Your account → Testing →
  Show debug tools; reload neither. (1) A film on its ladder shows "Play the original file" under the
  player on both; turned off, it is gone, and a person who never turned it on never sees it. (2) The
  host starts a Watch Together session from Chrome, the S23 joins and both play the film. Passes when
  the host's session page, under the people, and the S23's page, under who is here, list each device
  with the one leading marked "▶ leading" and the one the page is on "this device", and a line under
  them says the group's state, the leader by device id and name, why it leads (`putOn`, `attached`…)
  and this device's id, matching the core's `leaderChanged` lines. (3) Join from the Mac's Firefox
  too, as a second guest playing the film, and leave the host's Play page for the session's page, so
  a guest's device leads (the S23's, joined first, `leaderGone:…`). Lock the S23: the mark moves to
  Firefox's device on the host's page and Firefox's, the why saying `leaderGone:hidden`; unlocked,
  the S23 follows. (4) Turn Show debug tools off on one device: the view is gone there,
  and from the other device of the same person after its next read of who they are (a reload).
  (`internal/core/preferences`, `internal/core/api/session.go`, `internal/core/together/leader.go`;
  `web/apps/web/src/debug-tools.tsx`, `together-debug.tsx`, `app.tsx`)
  **Passed 2026-10-29 (the owner, staging):** Show debug tools turned on for admin and dan; the leader view marked the leader and this device,
  the marks moved as the lead passed, and turning it off hid the view (the owner).
- [x] **VQ133** — a Watch Together follower's page offline for a minute neither overflows its stack
  nor comes back a minute ahead, on staging. With the Mac's Chrome following in a Watch Together
  session over WebRTC through its client origin (the loader and the client of one build, the core's
  log saying `over=webrtc`) and the S23 leading, play a film; turn the Mac's Wi-Fi off until Chrome's
  film has stalled, about a minute, then on. Passes when (1) the core's log has no "Maximum call stack
  size exceeded" for Chrome, its link notes naming what carried each chunk that failed ("carried
  WebRTC connection …" or "over …", never an `internalException`), and (2) Chrome comes back within a
  few seconds of the S23's place, never more than a few seconds ahead of it: its
  `followerCaughtUp … why=catchUp` line says `groupAtMs`, `aheadForLoadMs`, `aimedBy` (`fill` or
  `lastLoad`, not `mostAhead` on a fast link), `lastLoadMs` and `loadsFaster`, and the
  `playerChanged` after it a `behindGroupMs` no more negative than `aheadForLoadMs`, which on staging's
  link is a few seconds, never a minute. (`web/packages/sdk/src/rtc-media.ts`,
  `together-load-speed.ts`, `together-follow.ts`; `internal/core/together/player_log.go`)
  **Passed 2026-10-29 (the owner, staging):** Chrome back from a minute's outage near the group, no stack overflow.
- [x] **VQ134** — a Watch Together session left with no player keeps its playing state (D375), on
  staging. On the Mac, Chrome hosting a session and Firefox a guest, both playing the film; the S23
  in it too, then locked. (1) Turn the Mac's Wi-Fi off for about a minute, then on. Passes when the
  core's log has `groupChanged state=Paused event=everyoneLeft … heldPlaying=true` as the last of
  them went, and as the first came back `groupChanged state=Waiting event=resumedOnReturn` at the same
  `positionMs`, then `state=Playing event=leaderPlays`; both browsers play the film on from about
  where it held, no one pressing Play. (2) Again, with the host having paused first. Passes
  when the log says `heldPlaying=false`, and both come back to the film paused where it was, until
  someone presses Play. (`internal/core/together/leader.go`, `group.go`;
  `web/packages/sdk/src/together.ts`, `web/apps/web/src/together-events.ts`)
  **Passed 2026-10-29 (the owner, staging):** held and played on with no press; the owner then asked that
  a returning leader's own position win (D376, built separately).
- [x] **VQ135** — a film's playback through a minute's outage neither restarts from the start nor
  condemns its top quality, on staging. With the Mac's Chrome playing a film alone at its top quality
  (the core's log saying the top level for it), turn the Mac's Wi-Fi off for about a minute, past
  where the film stalls, then on. Passes when (1) the film plays on from where it stood, never from
  0:00, the link notes' `asked again to load from` line, if any, giving that place; (2) the core's
  log has no unplayable quality for the playback, its link notes saying instead `quality <n> not
  judged unplayable, its loads not arriving` or `too little loaded ahead`; and (3) within a few
  minutes it climbs back to its top quality. (`web/packages/player/src/abr-video.ts`, `abr.ts`,
  `resume-position.ts`, `load-again.ts`)
  **Passed 2026-10-29 (the owner, staging):** the outage neither restarted the film nor dropped its top quality.
- [x] **VQ136** — a signed-in guest whose session the server lost goes home, on staging. On the Mac,
  signed in, join a Watch Together session as a guest from the S23 hosting it, and leave the guest's
  page on the session; restart staging's core. Passes when the Mac's page goes to the home page with
  the note "The Watch Together session is over." or "The Watch Together session is no longer
  there." at its top, never "This address has no invitation in it.", and the tray shows no Watch
  Together session; the S23's Watch Together page says "That session is over." with Start a session,
  no error. A guest joined by link alone, not signed in, whose page is reloaded after the restart, is
  told the session is over, not that the address has no invitation.
  (`web/apps/web/src/together-viewer.tsx`, `together-storage.ts`)
  **Passed 2026-10-29 (the owner, staging, core restarted 21:04:25):** Firefox (dan) went Home with "The
  Watch Together session is over."; Chrome (admin, host) played on alone, its Watch Together sign white.
- [x] **VQ137** — a page open across a deploy loads a screen it had not opened, on staging. On the
  Mac, in Chrome through staging's discovery, open the home page, and open no film's page; deploy
  staging with a client change, so its screens' files are renamed; then open a film's page. Passes
  when the page reloads once by itself and shows the film's page, at the film's address, never "This
  page could not load."; and the core's log has a "page reported a failure" line saying "the page
  reloaded for a newer client, once", naming the title screen and its file.
  (`web/apps/web/src/newer-client.ts`, `screen-load.tsx`)
  **Passed 2026-10-29 (the owner, staging, deploy 21:59):** a tab on Home across the deploy opened a
  film at once (22:00:11, playing in 298 ms), no error page. The run before (21:14) took ~1 min on
  "Loading..." with nothing in the log to say why; an earlier commit now reports a slow screen or start.
- [x] **VQ138** — a Watch Together host's Play page left for another site and come back by Back
  attaches its player, on staging. On the Mac, Chrome hosting a session on its Play page, Firefox and
  the S23 guests, the film playing. In Chrome go to google.com, then press Back. Passes when Chrome
  plays the film within a few seconds, in step with the others, and the core's log has a `role=video`
  device connecting for the host's member after its `a playback started … cause=playPage`; never
  "Loading…" for good. If its playback's answer was lost over WebRTC, the core's log has the page's
  `a request failed … nothing came for /api/v1/playback` and a second `a playback started`, then the
  `role=video` device. (`web/packages/sdk/src/rtc-request.ts`, `web/apps/web/src/play.tsx`,
  `reopen.ts`)
  **Passed 2026-10-29 (the owner, staging):** the host's Play page, away to google.com and Back, attached and played in step.
- [x] **VQ139** — who leads a Watch Together session, and from where (D377), on staging. On the Mac,
  Chrome hosting a session and Firefox a guest, the S23 a guest too, all playing the film. (1) Close
  the host's Chrome tab while it leads. Passes when the lead passes to the guest whose run started
  first, the core's `leaderChanged … why=leaderGone` line saying `positionFrom=core`, and no one's film
  jumps. (2) Lock the S23, then unlock it: the log has its `runEnded why=hidden` and, as it plays
  again, a new `runStarted`; it is now behind Firefox in line. (3) With the S23 locked, turn the Mac's
  Wi-Fi off for about a minute, then on. Passes when the log has `groupChanged … event=everyoneLeft …
  heldPlaying=true`, then, as the first browser comes back, `leaderChanged … why=noneLed …
  positionFrom=leader` with that browser's own `positionMs` and `groupChanged state=Waiting
  event=resumedOnReturn` at it: the browser plays on from where it stood, never sought back to where
  the group was held, and the other takes its position. (4) Reopen the host's page with everyone else
  gone: its `leaderChanged` says `positionFrom=core`, and it plays from where the group was held.
  (5) Reopen Chrome's page while Firefox leads: Chrome takes Firefox's position, then the lead in step
  (`why=hostInStep`), no one's film jumping. (`internal/core/together/leader.go`,
  `web/packages/sdk/src/together-follow.ts`, `together-roster.ts`)
  **Passed 2026-10-29 (the owner, staging):** handover moved no one; the host took the lead back in step; with no one left, the first back resumed from its own place, and a pause stayed. Firefox took ~45 s to recover after the outage (Chrome quicker): filed for the next batch.
- [x] **VQ140** — On the Fire TV (Silk), playing a film on its own Play page with its controls shown
  (out of full screen), move down with the remote onto the scrub bar and press Down again. Passes when
  the film stays where it was (no step back a second at each Down) and focus stays on the scrub bar or
  moves below it. Before this change Down at the bottom of the controls fell to the browser, which steps
  a focused slider's value (the owner, staging, 2026-10-29). (`web/apps/web/src/input-commands.ts`)
  **Passed 2026-10-29 (the owner, staging):** Down from the Fire TV's scrub bar left the film where it was.
- [x] **VQ141** — a leading player never drops to the group's common quality at its own jump, on
  staging. Cast file 5671 from the Mac to the Fire TV and let it reach the film's own picture
  (the core's log: `qualitySwitched qualityKbps=9018`); seek with the remote a few times. Passes when
  no `qualitySwitched … why="its group's: the common quality after its group jumped"` line comes from
  the receiver (on 2026-10-29 one took it from 9018 to 1896 kb/s on a link of 18451 kb/s).
  (`web/packages/sdk/src/together-player-events.ts`, `together-follow.ts`)
  **Passed 2026-10-29 (the owner, staging):** file 5671 cast to the Fire TV held 9018 kb/s through six seeks of the remote; no common-quality step.
- [x] **VQ142** — a cast while Watch Together is hosted plays on the TV, and a device receiving a
  cast is no other device's target (D386 §1), on staging. (1) Host a Watch Together session from one
  admin device (the S23's Firefox) with a guest in it; from Chrome on the Mac, with nothing playing,
  cast to the Fire TV (Use on the TV), then play a film on Chrome. Passes when it plays on the TV,
  Chrome's Cast sign stays blue and its Play page shows the remote. (2) While the TV receives the
  S23's cast, Chrome's Cast dialog shows the TV unavailable ("Showing another cast"), its button
  disabled; a cast to it is refused with the message "Fire TV is showing another cast" (raced: open
  Chrome's dialog before the S23 casts); once the S23 stops casting, Chrome may cast to it. Before
  D386 Chrome's cast took the TV over (`replaced=1` in the core's log).
  (`internal/core/live/registry.go`, `web/apps/web/src/cast-to.tsx`)
- [x] **VQ170** — a Watch Together Play page navigated away gives up the lead at once, on staging.
  On the Mac, Chrome hosting a session on its Play page, Firefox a guest, the film playing, Chrome
  leading. In Chrome go to google.com. Passes when the core's log has Chrome's video device detached
  closed (`goneBecause=closed`, not `silent`) within a second or two and Firefox's `leaderChanged`
  right after it, not 45 s later; then press Back: Chrome's page connects again at once (its note
  `kind=reconnected … why=left`), catches up and takes the lead back in step (`why=hostInStep`). On
  the S23, a guest's Play page sent to another site and back the same way. Its live connection too:
  the core's log has the page's live device leaving and coming back.
  (`web/packages/sdk/src/together-connection.ts`, `live-connection.ts`, `shown-page.ts`)
- [x] **VQ171** — a phone's live connection is checked at once as its page is shown again, on
  staging. On the S23 (Chrome), signed in on any page, lock the phone for 35 s or more, then unlock it.
  Passes when the core's log has its live device connected again (or its socket answering) within a
  couple of seconds of the unlock, not the 20 s it took on 2026-10-29 (VQ126's run), and a cast to the
  S23 from the Mac right after the unlock finds it among the devices. (`web/packages/sdk/src/live-connection.ts`,
  `shown-check.ts`, `shown-page.ts`)
- [x] **VQ172** — a Watch Together guest away from the session's page (D372 §9–§10), on staging.
  (1) Signed in as dan on the Mac's Firefox, join Chrome's session by a link. Passes when the guest's
  page has the banner: the tray (Cast and Watch Together's blue sign), the menu with Your account
  (Show debug tools reachable there, and a cast from the tray, unblocking VQ103). (2) With a film
  playing for the group, go home in Firefox and play another film: it plays for Firefox alone, the
  host's page lists dan ", away" and Chrome's "Here:" says "dan (away)"; press the tray's Watch
  Together sign: Firefox is back on the session's page, catches up to the shared film and plays in
  step. (3) A guest by link alone (the S23's Chrome, not signed in): close the tab, open the same link
  again in that browser: back as the same member, no join asked; open it in another browser (Samsung
  Internet): the link is refused. (`web/apps/web/src/app.tsx`, `together-viewer.tsx`,
  `together-viewer-tap.tsx`, `together-people.tsx`, `together-news.tsx`)
- [x] **VQ160** — no local-network note on a Fire TV, on staging. On the Fire TV (Silk), clear the
  site's data (so a dismissal of the note is forgotten), open staging and reload it twice. Passes
  when "Your browser is asking to reach devices on your network" never shows, and the page reaches
  the server as before; on the Mac's Chrome, with the site's local-network permission reset, the note
  still shows while Chrome's prompt is up. Before this change the note showed on the Fire TV, where
  Silk asks nothing (the owner, 2026-10-28). (`web/apps/web/src/local-network-note.tsx`)
- [x] **VQ161** — a title's and a series' rarely used actions under a vertical "…", on staging.
  (1) On the Mac's Chrome as admin, a film's page: Play is its first button, then Mark as watched and
  the "…"; the page shows no "Identified incorrectly", "Rescan this" or "Report filed wrongly" of its
  own. The "…" opens a list with "Identified incorrectly", which opens one dialog in the list's place
  (no list left behind it) offering Rescan, Enter manually and Report this; one click outside it is
  back on the page. (2) A series with more than one ordering (as dan): its "…" holds Order, whose
  dialog changes the ordering and closes. (3) On the Fire TV (Silk), the same film page by the
  remote: arrows reach the "…", OK opens it on "Identified incorrectly", OK opens its dialog, the
  arrows reach Enter manually and the works it finds, and Back closes it with focus on the "…".
  (`web/apps/web/src/more-actions.tsx`, `match-correction.tsx`, `modal-dialog.tsx`)
- [x] **VQ174** — a page says when developer tools bypass its service worker, on staging's client
  origin in Chrome on the Mac. (1) Open the home page with DevTools open and the posters shown; tick
  Application → Service workers → "Bypass for network" **without reloading**, then go to All Series:
  passes when the page shows "Developer tools are bypassing the service worker; media won't load"
  above the screen and the core's log has one client report saying "developer tools bypass the page's
  service worker". (2) Reload with the box still ticked: the client cannot load, and the loader's page
  says so with a line to untick "Bypass for network", then reload. (3) **Untick the box** (it stays
  ticked across DevTools sessions, agent-failures A51) and reload: the line is gone and the posters
  load. A reload with the box ticked never reaches the client, so the note shows only for a box ticked
  after the page loaded (the owner, 2026-10-30). (`web/apps/web/src/worker-bypassed-note.tsx`,
  `loader-client-failed.tsx`, `poster.tsx`)
- [x] **VQ175** — a cast's remote works the receiver's sound, on staging: cast a film with more than
  one audio track and a 5.1 track (so it has a boosted mix) from Chrome on the Mac to the Fire TV, Use
  on the TV. Passes when the casting page shows Dialogue boost, Level loudness and the Audio menu as
  the TV has them, and each change made there is heard on the TV within a couple of seconds (the boost
  and levelling at once; another track after the film is asked for again from where it stood), the
  remote's controls then showing what the TV says; the TV stays controlled throughout. While
  controlled the TV's own controls are play/pause, the scrubber and full screen alone (its sound
  controls hidden, cast-receiver.tsx); Back there leaves controlled mode, the film playing on, and
  its own controls then show the same sound choices (corrected after the owner's run, 2026-10-30).
  (`web/apps/web/src/sound-panel.ts`,
  `internal/core/together/hub.go`)
- [x] **VQ176** — a film starts on the quality it plays, on the Fire TV (Silk), staging: play a film
  whose ladder has nothing between 2.5 and 12.2 Mb/s, then another film, from their start, on the Fire TV alone (no cast). Passes when neither
  shows black frames as it begins, and the core's log has no quality switch within the first seconds
  of either: its first `qualitySwitched` names the rung its first chunk was of (the 12.2 Mb/s rung for
  the first on a link remembered near 12 Mb/s), why "first by the remembered link". Before D381 both showed
  2–3 black frames as a low first rung switched to the top at play (the owner, 2026-10-28).
  (`web/packages/player/src/abr-ceiling.ts`)
- [x] **VQ177** — the debug link caps and drops the tab's connection, on staging's client origin (over
  WebRTC) in Chrome on the Mac, Show debug tools on. (1) Play a film on its ladder and choose a cap of
  1 Mb/s under the player: passes when "Throttled to 1 Mb/s" shows above the screen, the player steps
  down within a minute to a quality within 1 Mb/s, and the core's log has its link estimate near
  1000 kb/s; set No cap and the badge goes and it climbs back. (2) Choose "Drop the connection for 30
  s": passes when the page counts down, the core's log shows the tab's live and Watch Together
  sockets closed and not back for 30 s, the film plays on what it had loaded and waits after, and
  within a few seconds of the end the sockets are back and the film plays on, with nothing reloaded.
  (`web/apps/web/src/link-debug.ts`, `web/packages/player/src/link-shaping.ts`)
- [x] **VQ180** — a series' seasons as a row of posters (D379), on staging. (1) On the Mac's Chrome,
  a series of several seasons with specials (or any with a season 0): above the episodes a
  row of season posters, the specials last, the season shown marked; no season choice box. Each
  season shows its still in its poster's place, or the series' poster where it has none. Choosing a
  poster shows that season's episodes below; the browser's Back then leaves the series, not the
  season. Open an episode of season 2 and come back: season 2 is shown with that episode focused.
  (2) On the Fire TV (Silk), by the remote: Down reaches the row, Left and Right move along it (the row
  scrolling sideways where it is wider than the screen), OK shows the season, and Down from the row
  lands on the season's first episode, not on its Mark as watched. (`web/apps/web/src/series.tsx`)
- [x] **VQ181** — the banner's Libraries icon as a menu of the person's libraries, on staging.
  (1) On the Mac's Chrome as admin (several libraries): the Libraries icon opens a list of the
  libraries by name, "All libraries" last, the one shown marked; choosing one opens its page, whose
  top has no library chooser (its view and sort choosers stay) and names the library. As dan, with
  access to one library only (or a household of one library), the icon goes straight to its page, no
  menu. (2) On the Fire TV (Silk), by the remote: OK on the icon opens the menu from its top; Down
  walks the libraries, the list scrolling within its height where they are more than the screen
  holds; Back closes it, as do Left and Right, focus back on the icon, the page unchanged.
  (`web/apps/web/src/app-shell.tsx`, `library-browse.tsx`)
- [x] **VQ185** — a series' seasons show their real posters (D379 §1), on staging after its deploy,
  which migrates to step 0064. (1) Before deploying, note a TMDB series already matched on staging
  (any with several seasons and specials), whose season cards show stills. Deploy and
  wait for the library's next pass (the start's sweep, or Scan library): the core's log shows
  make_artwork_sizes jobs for the series, and no fetch of its TMDB record (the season posters come
  from the record stored). Passes when its page's season cards show each season's own poster, the
  specials' too where TMDB has one, and the series' head and its card in Series still show the
  series' own poster, not a season's. (2) `sqlite3 persistent.db "SELECT season_number, source_ref
  FROM artwork WHERE item_id = <series id> AND removed_at IS NULL"` lists one row per season with a
  poster, and one with NULL, the series' own. (`internal/core/artwork/images.go`,
  `internal/core/library/art.go`)
- [x] **VQ186** — a season with no poster of its own, on staging. On the Mac's Chrome, a series
  whose TMDB record has a season with no poster (a new season, or a series TMDB lacks posters for) or
  one in a library that does not follow TMDB: passes when that season's card shows its first
  episode's still in its poster's place with no "No poster" caption, a season with neither its own
  poster nor a still shows the series' poster, and "No poster" shows only on a card with none of
  these. Then choose a DVD order on a series that has one: its discs show stills or the series'
  poster, never an aired season's poster. (`web/apps/web/src/series.tsx`, `poster.tsx`)
- [x] **VQ187** — the Fire TV keeps its quality through a chunk the page was too busy to read
  (D382), on staging: play file 5664 from its middle on the Fire TV alone; passes when it plays on
  its top or next rung throughout the first minute, and the core's log shows any give-up excused as
  stuck or busy, never `level 0`; each step as `step:` in its link notes.
  (`web/packages/player/src/abr-ceiling.ts`, `first-piece.ts`, `abr-busy.ts`, `chunked.ts`)
- [x] **VQ188** — a Watch Together host's Play page never dead-ends on a playback answer lost over
  WebRTC, on staging. On the Mac, Chrome hosting a Watch Together session on its Play page: open the
  Watch Together page, then go back to the film by its button, ten times. Passes when it plays each
  time within a few seconds, never "The server could not be reached", and any lost answer shows in
  the core's log as a retried request: the page's `a page reported a failure … SilentRequestError:
  nothing came for /api/v1/playback` (or the playback report's `nothing came for 40000 ms on stream
  <n>`), a second `a playback started` after it, and the core's `rtcapi: a response's channel stayed
  open long after the core's last byte for it … stream=<n>`, whose `bufferedAtClose`,
  `isHandedOverWhole` and `streamClosedBeforeMs` say where its bytes stopped: copy that line into
  the result. Needs a staging connect rebuilt from this commit, as its loader carries the client's
  requests. (`web/packages/sdk/src/rtc-request.ts`, `web/apps/web/src/reopen.ts`,
  `internal/core/rtcapi/answer_watch.go`)
- [x] **VQ189** — Back or Forward never starts a film, and a receiver with no controller goes to the
  film's page (D383), on staging, on the Fire TV (Silk). (1) Play film A, go home, play film B, then
  press the remote's Back twice: passes when each Back lands on a title's page (B's, then A's or the
  page before it), Play first, and no film starts (the core's log shows no `a playback started` for
  either). (2) Cast a film from the Mac's Chrome to the TV, press Use on the TV, then reload the TV's
  page (or redeploy staging): passes when the TV goes to the film's page within a few seconds, not a
  Play page playing with no controller; Back from it never starts the film.
  (`web/apps/web/src/navigation.ts`, `play-up.tsx`, `live.tsx`, `cast-receiver.tsx`)
- [x] **VQ190** — a cast that ends sends its receiver home, and the page cast from stays where it is
  (D384, D385), on staging. (1) The Fire TV receiving a cast from the Mac's Chrome, Use pressed on the
  TV with one OK: passes when the cast holds, never taken back by the OK that said Use (the core's log
  shows no `a cast ended … reason=takenBack` after the `a receiver said Use`); Stop casting on Chrome
  sends the TV to its home page; in a second cast, Back on the TV ends the cast: the TV goes home, and
  Chrome plays the film itself from where the TV stood, no "Play here from" offer; in a third, paused
  on the TV first, Chrome's film waits paused there. (2) The S23, a Watch Together guest on the group's film: Cast from the tray,
  choose the Fire TV: passes when the S23 stays on its page of the group's film, its video's area
  saying "Playing on Fire TV" with the group's controls; Stop casting plays the film on the S23
  again on that page, the TV going home. (3) The Mac's Chrome, a solo Play page: cast to the Fire
  TV and stop: passes when Chrome stays on its Play page throughout, the film playing there again
  from the TV's position. (4) Watch Together's sign opens a dialog over whatever page is shown;
  "Back to the film" goes to the Play page of the group's film; leaving it and coming back never
  shows a separate session page. (5) End the Watch Together session from its dialog: passes when the
  dialog closes and does not open again. (6) With a cast playing on the Fire TV, unplug its network
  (or turn its Wi-Fi off): passes when, within about three minutes (the core notices the TV silent
  after up to 45 s, then waits `ReceiverGoneAfter`'s two minutes, D364 §6), Chrome plays the film
  itself from about where the TV was, and the TV, once back on its network, is on its home page.
  (`web/apps/web/src/navigation.ts`, `cast-receiver.tsx`, `cast-remote.tsx`, `live.tsx`, `play.tsx`,
  `status-tray.tsx`, `watch-together.tsx`)
- [x] **VQ191** — a jump in Watch Together plays, on staging. A Watch Together session on a film,
  the S23's guest casting to the Fire TV, which leads, and the Mac's Chrome as host: from Chrome,
  jump far ahead into film not made yet. Passes when every player plays from there within a few
  seconds, and the core's log has a `queued a run of chunks` whose `firstChunk` is the chunk each
  player asked for after the jump (`waited for a chunk not made yet … isSought=true`), with no
  `planned no run of chunks: no player wants one from there` repeating for it. Repeat it a few
  times, as the jump that hung landed exactly on a chunk's start.
  (`internal/core/playback/interest.go`)
  **Passed 2026-10-30 (the owner, staging), without a cast:** far jumps, playing and paused, both devices played from the new point each time; the cast-led case follows VQ193's fix.
- [x] **VQ192** — a Watch Together session ended under a signed-in guest's film sends them home,
  and the cast remote brings a film back by Stop casting alone (D372 §9–§10, D384 §2, D388), on
  staging. (1) The Mac's Chrome hosts a session on a film; the S23, signed in, joins by its link and
  plays the group's film on its Play page; the host ends the session from its dialog: passes when
  the S23's film stops and its page goes home, in the shell (the top bar shown), with the
  session-ended notice, and reloading the S23 then shows its home page, no film starting (the core's
  log shows no `a playback started … deviceId=<the S23's>` after `the host ended the session`). Run
  it twice: once with the S23 signed in as a person of its own (s23), once as the host's own person
  (admin) on its second device. (2) Cast a film from the Mac's Chrome to the Fire TV: passes when
  the remote on the Play page offers Stop casting and no "Play on this device instead", and Stop
  casting sends the TV home and plays the film on Chrome from where the TV stood.
  (`web/apps/web/src/together-guest-play.tsx`, `cast-remote.tsx`)
  **Passed 2026-10-30 (the owner, staging), part (1):** the S23 as s23 and as admin went home, told, and stayed home on reload; part (2) (Stop casting alone) is checked with VQ193.
- [x] **VQ193** — a Watch Together guest who joins lands on the group's film, and a guest's cast
  follows the group (D364 §11, D371 §4, D384 §2), on staging. (1) The Mac's Chrome starts a session
  with nothing on; the S23, signed in, joins by its link: passes when the S23 shows its own page
  saying it waits for the host to start a film, with no Watch Together dialog open; Chrome starts a
  film: the S23 moves to that film's Play page and plays it in step. Chrome puts on another film:
  the S23 follows to its Play page. Repeat with a session already playing a film: the S23's join
  lands straight on that film's Play page. Repeat once with a private window, not signed in (a guest
  by link alone): its page waits, saying so, and plays each film as it starts. (2) The S23 on the
  group's film casts to the Fire TV from the tray: passes when the TV plays the group's film in step
  with Chrome (the core's log shows the TV's `a device connected sessionId=<the session's>
  … role=video`, not a session of the cast's own, and, for an S23 of a person of its own, no `the
  session's leader changed` naming the TV), the S23's page says "Playing on Fire TV" with the group's controls; pause, play and a jump
  from the S23 and from Chrome each move the TV with the group; Stop casting sends the TV home and
  the S23 plays the film again on its page. Run (2) twice: once with the S23 signed in as a person of
  its own (s23), once as the host's own person (admin). (3) With the S23 casting the group's film to
  the Fire TV, end the session from Chrome's dialog: passes when the S23 goes home, told; the TV goes
  home; and Chrome, the host, goes home too, with no notice; the core's log has `cast: a cast ended …
  reason=sessionEnded` for it (D389 §1).
  (`web/apps/web/src/together-viewer.tsx`, `cast-dialog.tsx`, `internal/core/together/cast.go`,
  `internal/core/assembly/assembly.go`, `together-session-end.ts`)
- [ ] **VQ194** — the first visit after a deploy says nothing of developer tools: after
  the next nas.example deploy, open the production page in Firefox on the Mac (the owner's, where it was
  seen on 2026-10-31) without reloading first, DevTools closed. Passes when no "Developer tools are
  bypassing the service worker" line shows, or one shows for a moment and goes as the new worker
  takes over, and posters load. With DevTools' "Bypass for network" ticked and the page reloaded,
  the line shows (the note still works). (`web/apps/web/src/worker-bypassed-note.tsx`)
  **Failed 2026-10-31 (the owner, production):** the line still showed after the deploy, and went at
  a reload: nothing asks the browser to update its worker, so a tab loaded while connectd restarted
  kept the old one (agent-failures A53). The fix, `registration.update()` before the note speaks, is
  next batch's; this item is checked again with it.
  **Built (11-01 batch):** a probe the network answers first asks `registration.update()`; a newer
  worker on its way is no bypass, and the note asks again as it takes over. Check again as above,
  after the deploy that carries it, in a tab left open across the deploy.
- [ ] **VQ195** — an admin sees a security update's banner on a real deployment
  (security-updates.md Part B): on staging, give the discovery a release record naming the core's
  version as needing a fix (`go run ./cmd/release-record --record <file> --version <newer tag>
  --digest sha256:<64 hex> --security "a test fix"` after a first run for the core's own tag), and
  restart the core. Passes when, signed in as the admin on the Mac's Chrome, every page shows "A
  security update is available (<tag>): a test fix." with the command and Update now leading to How
  to update; Snooze for a day hides it and a reload keeps it hidden; signed in as dan on Firefox,
  no banner shows; the settings page's Updates row names the newer version. Needs the paid
  service's `POST /release` (or Orpheum's own discovery) to serve the record, which the connect
  repository owes. (`web/apps/web/src/security-update-note.tsx`, `internal/core/discovery/release.go`)
- [ ] **VQ196** — a GPU encodes under the node's read-only root (noexec step 1): deploy the next
  image to nas.example (Quick Sync) and run `deploy/hwtest/hwtest.sh <image> --expect h264_qsv` on the
  amd box (`--expect h264_vaapi`). Passes when each run plays all its films with the expected
  encoder, the node's log has no "read-only file system" line, and `docker compose exec orpheum-node
  cat /proc/mounts` shows `/` `ro` and `/tmp` `noexec`. The sandbox has no GPU, so only software
  encoding was tried there. (`internal/deployment/templates/compose*.yaml`)
- [ ] **VQ197** — Firefox's page is back soon after the network, with no `online` event to tell it,
  on staging. On the Mac, Firefox over WebRTC through staging's discovery (the core's log says
  `over=webrtc` for it), playing a film. Turn the Wi-Fi off for about 20 s, then on. Passes when the
  page is back within about 5 s of the Wi-Fi returning: its "a WebRTC connection to the core is back
  after … ms with none" line comes within about 5 s of the Wi-Fi coming back, its tries' last failure
  naming "the offer reached no server", and the film plays on (its `kind=reconnected` line within a
  few seconds of it). Before this change Firefox came back about 45 s after the Wi-Fi (VQ139's run,
  2026-10-29). (`web/packages/sdk/src/rtc-transport.ts`, `rtc-reopen.ts`, `discovery.ts`)
- [ ] **VQ198** — a page's freeze is told to the core's log on any screen, on staging. On the Mac's
  Chrome, on a series' page with no film playing, open DevTools' console and run
  `setTimeout(() => { const t = Date.now(); while (Date.now() - t < 2000) {} }, 0)` (a loop typed
  straight into the console is no long task to Chrome: idiosyncrasies.md, the owner's run of
  2026-11-01). Passes when the core's log has, from
  that page, `the page froze 2.0 s (long task) on screen=series-page filmPlaying=false hidden=false`
  (a few tenths more is fine); run it three times within five seconds: one more line comes about five
  seconds after the first, saying `1 more since`. Then on the Fire TV, the next time its focus seems
  stuck, the log says whether its page froze and on which screen.
  **The Mac part passed (the owner, nas.example, Chrome 156, 2026-11-01):** the core logged the freeze at
  13:39:30 and 13:39:37, then `1 more since, the longest 2.0 s` at 13:39:42, five seconds after; and
  again on staging (19:36:10, then `2 more since` at 19:36:14). The Fire TV part is still owed.
  (`web/apps/web/src/page-freezes.ts`, `client-main.tsx`; `web/packages/player/src/main-thread.ts`)
- [x] **VQ199** — OK on a sign-in's checkbox ticks it and signs nothing in, on a Fire TV, on staging
  (the owner, 2026-10-30, VQ160's run: "As soon as I select the checkbox, it acts as though I'd
  pressed the log in button"). On the Fire TV (Silk), signed out, at the discovery's sign-in: fill in
  the realm, name and password by the remote, arrow to "This is my own device: remember my realm" and
  press OK. Passes when the box ticks and the page stays on the sign-in; OK again unticks it; then OK
  on Sign in signs in. Hold OK on the box for a second too: it ticks once, and nothing is submitted.
  Signed in after, the core's log has a debug line `a page noted … a form's OK on a checkbox input`
  for each OK pressed in the core's own sign-in, if one was used (the discovery's sign-in, at the
  discovery's origin, can tell only its console). (`web/apps/web/src/form-ok.ts`, `loader-main.tsx`,
  `input-commands.ts`)
  **Passed 2026-11-02 (agent, staging, the Fire TV in Silk 138 by adb):** at the discovery's sign-in
  on orpheum.dev, the realm remembered and the box ticked: OK on the box unticked it and ticked it
  again, the page staying; held OK ticked it once and submitted nothing; OK on Sign in signed in as
  lead-tests. The core's own sign-in was not used, so no debug line was looked for.
- [ ] **VQ200** — signing out lands on the discovery's sign-in however the household was reached, on
  staging (the owner, 2026-10-30: the Fire TV went to the discovery's sign-in, the S23 to the core's).
  (1) On the S23 (Chrome), the household reached by a link (your own link, or a fresh sign-in by a
  link), signed in: the menu's Sign out asks "Sign out of this device?"; Cancel keeps you in; Sign out
  goes to the discovery's sign-in, with no core sign-in drawn on the way. (2) The same on the Fire TV,
  by remote: the question takes focus, Back cancels it, OK on Sign out signs out to the discovery's
  sign-in. (`web/apps/web/src/app-shell.tsx`, `loader-at-client-origin.tsx`, `app.tsx`)
  **Run 2026-11-02 (agent, staging, Fire TV in Silk by adb as lead-tests):** (2)'s question took
  focus on Sign out and Back cancelled it, focus back on Menu; but OK on Sign out stayed on "Signed
  out. Taking you to sign in…" for good, as did Chrome 156 on the desktop. The question's held entry
  was stepped off after the page began to leave, which cancelled the leaving (idiosyncrasies.md).
  Fixed by the sign-out waiting for held entries to go (`signed-out-watch.ts`'s
  `signingOutWithNoneHeld`); (1) and (2) to run again on the fix.
  **Run on the fix ("fix(web): a sign-out with its question open leaves for the discovery's sign-in"), 2026-11-02 (agent, staging):** (2) passes on the Fire TV by remote:
  the question took focus on Sign out, and OK reached the discovery's sign-in
  (`?signedout=<core>`) within 4 s. Chrome 156 on the desktop, through orpheum.dev as lead-tests,
  reached it within 1.5 s. (1), on the S23 reached by a link, is the owner's still.
- [x] **VQ201** — the People page's three buttons, dialogs and pages work, on staging (the owner,
  2026-11-01: "The people page is just too messy"; D399). On the Mac's Chrome as admin, People.
  (1) Create Role, Create Account and Invite Someone sit side by side above Invitations, Accounts and
  Roles. (2) Invite Someone: write a note, choose a role, make the link: the invitation's dialog opens
  at once with its details, a QR code, the link and Withdraw at the bottom; Copy says "Copied." and
  pasting gives the link; close it, and the invitation is a row of note, role and date issued, which
  opens the same dialog again; there is no "Send another link" anywhere. (3) Open the link in
  Firefox and register: the row now says who used it, and its dialog shows the details and that
  person with no QR code, link or Withdraw. (4) Withdraw another invitation (it asks again): its row
  is gone. (5) Create Account makes a row of name, note and role and opens nothing; Create Role makes
  a row with 0 people and opens nothing. (6) An account row opens that person's page, where they can
  be changed. (7) A role row opens its role page: tick a library and a capability, reload the page,
  both are kept, and there is no Save button; Delete shows only for a role no one has (give the role
  to someone and it goes; never on Administrator or Can watch). Then on the Fire TV, by remote: the
  three buttons and each dialog (OK opens, Back closes) and the rows can all be reached and used.
  On the S23's Chrome, Copy on a waiting invitation's dialog copies its link.
  (`web/apps/web/src/people.tsx`, `invitations-section.tsx`, `people-dialogs.tsx`,
  `roles-section.tsx`, `role-page.tsx`; `internal/core/auth/invitations.go`)
  **Passed 2026-11-01 (the owner, staging):** "Everything worked as planned." Follow-ups in the
  plan file's 11-04 batch: a redeemed invitation leaves the list, and a role opens in a dialog.
- [ ] **VQ204** — another audio track chosen mid-film switches without stopping the picture, on
  staging, on the Mac's Chrome and on the Fire TV. A film of two or more audio tracks on its ladder
  (the tracks note says "transcoded" or names the top's own sound), playing a minute in. Choose the
  other track in the audio menu, then the first again; on the Fire TV, choose it from its cast's
  remote as well. Passes when the picture never goes black, any wait comes at the choice itself
  and the new language is heard from there with no gap or old language after it (D400, VQ237), the URL and the position stay as they were (no
  reload: the core's log has no second `POST /playback` with cause `audioChosen`), and the player's
  notes say `sound N chosen (trackChosen)` then `sound N heard (trackChosen)`. Note whether the cut
  between a copied 5.1 track and a made stereo mix, or AC-3 and AAC, is clean on each device. In a
  Watch Together session, the leader's choice leaves the guests' films playing undisturbed.
  (`web/apps/web/src/play.tsx`, `film-player.tsx`; `web/packages/player/src/hls.ts`)
  **Passed in part 2026-11-01 (the owner, staging), the Mac's Chrome.** Still owed: the Fire TV and
  the cast's remote.
- [x] **VQ205** — a film started on a track other than its main one plays at that track's own
  loudness, on staging. A file whose second track is much quieter or louder than its first, measured
  (its levelling note gives a number): choose the second track, then reopen the film so it starts on
  it. Passes when the levelling note under the film names the second track's gain, not the first's
  (the core's playback answer's `tracks.sounds[*].levelling` differs by track), and switching to the
  first track in place changes the note, and the loudness, only once the first track is heard.
  (`internal/core/playback/playback.go` `startingLevelling`, `internal/core/chunks/ownsound.go`
  `soundLevelOf`)
  **Passed 2026-11-01 (the owner, staging).**
- [ ] **VQ206** — Safari switches a file played as it is between its own tracks in place, on the
  Mac's Safari. A file of two audio tracks Safari plays as it is (the tracks note says "original"),
  chosen to play as the original. Choose the other track. Passes when the picture plays on and the new
  track is heard at once, the page does not reload (no second `POST /playback`), and the core's log
  has the interest's sound `sound1-own`; then throttle the link until it stalls: the lower quality it
  is offered plays the track chosen, not the first. On Chrome the same choice moves the film onto its
  ladder once, as before. (`web/packages/player/src/own-tracks.ts`, `web/apps/web/src/play.tsx`
  `DirectUnder`)
  **Passed in part 2026-11-01 (the owner, staging):** the switch in place works, with a slight gap in
  the sound at the switch, which the owner accepts. Still owed: the stalled link's lower quality,
  throttled by macOS's Network Link Conditioner (Safari has no throttle like Chrome's DevTools).
- [ ] **VQ207** — file 5667 (MP4, HEVC `hev1`, B-frames with every pts equal to its dts) plays at its
  own frame rate at the top of the ladder in Chrome, on staging (design/plans/
  audio-switch-and-retiming-2026-11-01.md, Part 2; the owner's answers of 2026-10-31). Deploy core and
  node together (node protocol 55), then have file 5667 examined again: a rescan does not probe a file
  unchanged on disk, so it needs a fresh examining (a new library over its folder on staging). First,
  its stored probe record's picture says `"needsRetiming":true`. Then on the Mac's Chrome, play it at
  the top quality for a minute, then jump to its middle and play another minute. Passes when motion is
  smooth throughout, `chrome://media-internals` shows no run of dropped frames, and its decoder is not
  in low-delay mode; and the core's log shows its picture's runs made, none refused. Also the qualities
  below the top play smoothly, with no frame stepped twice at chunk joins.
  (`internal/node/probe/parse.go`, `internal/node/chunks/arguments.go`, `internal/core/chunks/job.go`)
- [ ] **VQ215** — a full rescan corrects file 5667 on staging (D392; with VQ207). Deploy core and node
  together. On the Libraries page, open the library holding file 5667 and press "Full rescan". When it
  ends, the core's log has the line "a file read again differs from what was kept of it; what that
  touched is thrown out" for fileId 5667, its `differed` naming `video.needsRetiming: none → true` on
  its picture's stream and its `thrownOut` naming the picture's chunks, and the scan's totals line
  ("a scan compared the files it read again with what was kept"). Its stored probe record's picture
  says `"needsRetiming":true`. Then on the Mac's Chrome play file 5667 at the top quality for a minute
  and jump to its middle: passes as VQ207 does, smooth, no run of dropped frames, its picture's chunks
  made anew (the core's log shows its runs into a store not used before). Last, press "Full rescan"
  on the same library again: its totals say `"correctedCount":0` and nothing is thrown out.
  (`internal/core/library/derived.go`, `internal/core/chunks/streams.go`, `internal/core/identify/rescan.go`)
  *Run by the lead on staging, 2026-11-02: does not pass, and the item's premise is in doubt.* The
  full rescan of Shows (10:20, 10,568 files read again in 1,448 batches, 8 minutes) ended with
  `comparedCount=4925 correctedCount=0 thrownOutCount=0`; no line for file 5667, and its stored picture
  still lacks `needsRetiming`. File 5667 on staging, as in the old staging's database, is
  `The.Pumbletons.S33E01.iNTERNAL.720p.x265-ARBOR.mkv`: a Matroska file, not the MP4 (`hev1`) the item
  and the plan describe. The node's ffprobe gives its HEVC packets reordered display times (29 of the
  first 40 with pts unlike dts; the first two without a dts), so the probe's rule rightly does not mark
  it. The pts-equals-dts packets and the `hev1` tag were probably read from Orpheum's own MP4 copy, which
  would put the fault in the remux, where a scan-time mark cannot catch it. In Linux Chrome 156 it plays
  now at 24 fps with no frame dropped. Asked of the owner: does it still play choppily on the Mac?
- [ ] **VQ225** — a cast seeked on the Fire TV goes where it was sent, never back, and plays on at its
  own speed with sound (staging; the plan's entry for file 5680, 2026-10-29). Deploy, then cast a film
  from the Mac to the Fire TV (Silk), Watch Together off. Press Right on the Fire TV's remote twice, a
  second apart, then twice again quickly (under a second apart), then press the Mac remote's Forward
  10 s twice quickly. Passes when the film lands where the presses sent it each time and no step back
  follows; the core's log has, for every "a player's play state changed … kind=seeked" line of the
  receiver, a `soughtBy` (`right-key`, `seekForward-click`…) or a `sought` note with `why=command`, and
  no `soughtBy=left-…` or `seekBack-…` you did not press; and no seek plays its picture fast and silent.
  If a step back still comes, its line's `soughtBy` names what asked it: note it in the plan's entry.
  If the fast silent play still comes, repeat with levelling off (the play page's switch) and say
  whether it comes then too (`a playback's sound … isThroughWebAudio` says which path played).
  (`web/packages/sdk/src/together-leader.ts`, `web/apps/web/src/seek-ahead.ts`,
  `web/apps/web/src/play-state-video-events.ts`, `internal/core/api/report_context.go`)
- [ ] **VQ226** — the first film cast to the Fire TV after a deploy starts and plays without freezes
  (staging; the plan's section "A first film after an update freezing on the Fire TV", file 5671). With
  the Fire TV on the discovery's page (under its loader) on the home page, deploy staging, let the Fire
  TV reload onto the new build, leave it on its home page for at least 15 s, then cast file 5671 from
  the Mac. Passes when its first minute plays with no freeze and its first 10 s report drops fewer
  than 5% of its frames ("a player dropped frames"), and the Play page's scripts were fetched before
  the cast: no page failure "the play screen's code took … ms to load" is reported for it, and its
  "a playback started playing" `firstDataAfterMs` is no worse than a cast's after a plain reload. Repeat once leaving the Fire TV on its
  home page for under 5 s before casting, to compare. (`web/apps/web/src/warm-play.ts`,
  `web/apps/web/src/home.tsx`, `web/apps/web/src/cast-ready.tsx`)
- [ ] **VQ230** — a damaged chunk is dropped and made again on staging (D392 §4). Deploy core and node
  together (node protocol 56). Play a film on the Mac's Chrome for a minute, note its file id, and stop.
  In the node's prepared directory (`<instance cache>/prepared/<file id>/chunks-<stream>/<store>/`), pick
  a chunk the minute made, say `00005.m4s` of the stream played, and damage it by hand: overwrite a
  few bytes in its middle (`printf 'xxxx' | dd of=00005.m4s bs=1 seek=1000 conv=notrunc`), and in a
  store of the sound played truncate one (`truncate -s 100 00007.m4s`). Play the film from its start again in a new
  private window, so the browser's day of caching does not answer for the server. Passes
  when playback goes past both without a stall longer than a chunk's making and with no picture or
  sound broken, the node's log has "dropped a chunk that did not match its record; it is made again
  when asked for" for each, at warn, naming the file id, the stream, the chunk and what differed (its
  CRC-32C, its size), and the core's log shows a run of that chunk alone made. Then damage another
  chunk the same way, press "Full rescan" on the film: the core's log has "chunks of a file read again
  were missing or damaged" for its file id, and the scan's totals line says `"chunksDroppedCount":1` and
  a `chunksCheckedCount` of every file of its stores. (`internal/node/prepared/records.go`, `internal/node/byteserver/byteserver.go`,
  `internal/core/chunks/files.go`, `internal/core/library/derived.go`)
- [ ] **VQ231** — the release workflow writes the unsigned release record (D394 §8), at the first
  release after the owner's ceremony. Set the repository's variables `PUBLISHER_PLACE` (the origin
  serving the publisher's published files), `PUBLISHER_ID` and `PUBLISHER_ROOT_VERSION`, then push an
  annotated tag whose message has a line `Security: <a test sentence>`, and approve the run. Passes
  when the run's "Write the release" step names the tag, the digest and the security line, its
  artifact `release-record` downloads, its digest matches the one the image push printed, and
  `discovery-sign sign-release` on the Mac shows the same release and signs it. Without
  `PUBLISHER_PLACE` the two steps are skipped and the image publishes as before.
  (`.github/workflows/release-image.yml`)
- [ ] **VQ232** — Chrome on the Mac reads over WebRTC near Firefox's rate (D397). Deploy the core and
  the web client together. On the Mac, run the Throughput probe over WebRTC in Chrome, then in
  Firefox, from the same network, three times each. Passes when Chrome's figure is at least 70% of
  Firefox's (before: Chrome a fraction of it), and the core's log has one `rtc connection ended` line
  per connection the probe used, with its path, `durationMs`, `bytesSent`, `cwndBytes`, `srttMs`,
  `rwndBytes` and `mtu`. Note the figures, Chrome's and Firefox's, and the end lines' `cwndBytes` and
  `srttMs`, in D397. In Chrome's `chrome://webrtc-internals` the connection shows a video
  transceiver that receives nothing. (`internal/core/rtcapi/pacer.go`,
  `internal/core/rtcapi/video.go`, `internal/core/rtcapi/ended.go`,
  `web/packages/sdk/src/rtc-connect.ts`)
  *2026-11-02, from the sandbox's Linux Chrome 156 and Firefox 155, not the Mac:* Chrome 238–247
  Mb/s both ways, Firefox 193–245 for the app's requests and 75–119 for a film's pieces; one
  `rtc connection ended` line per connection with every field (D397 §7). The Mac's run is still owed.
- [x] **VQ233** — a Fire TV reads over WebRTC faster than before (D397). Deploy the core and the web
  client together. On the Fire TV, run the Throughput probe over WebRTC three times. Passes when its
  figure is above the one noted before this change (record both in D397), playback of a film over
  WebRTC starts and plays as before, and the core's log has its `rtc connection ended` lines, as in
  VQ232. (`internal/core/rtcapi/pacer.go`, `internal/core/rtcapi/video.go`,
  `web/packages/sdk/src/rtc-connect.ts`)
  **Passed 2026-11-02 (agent, staging, the Fire TV in Silk 138 by adb, as lead-tests):** three runs
  over WebRTC: the app's requests 26.1, 36.1 and 33.6 Mb/s, a film's pieces 22.5, 33.3 and 28.6,
  above 15–18 before (figures in D397 §7); The Gold Rush then played over WebRTC, 2 frames dropped in
  10 s; the core logged its `rtc connection ended` lines for user 4's device. Three earlier runs
  were made while the stick was signed in as admin, by mistake (A50).
- [ ] **VQ210** — staging's core enrols with staging's discovery by its issuer and identity (D391 §3,
  D393 §4). Owed first: staging connect publishes its root chain and a signed address document at
  its issuer's `/.well-known/` paths (connect's change, after VQ211's ceremony for staging's keys). On
  staging's System page as admin, Leave the discovery, then type staging's issuer (its name alone) and
  your password, and Connect. Passes when the page shows the discovery's name and an identity `od1-…`
  to compare, which matches the one `discovery-sign` printed for staging's genesis; "They are the
  same: connect" enrols, the page then shows the identity and the root keys' fingerprints, which match
  `ssh-keygen -lf` of the keys; the account key claims it as before; an invitation's link opens at the
  client origin the document names; `trust-discovery` in the core's persistent directory holds the
  chain and the document; and the core's heartbeat names root version 1. Then a wrong password
  refuses with "That isn't your password", and the suggested name refuses saying the server doesn't
  know its keys until `suggested.go` holds the genesis. (`internal/core/discovery/document.go`,
  `internal/core/api/discovery.go`, `web/apps/web/src/system-discovery.tsx`)
- [ ] **VQ211** — the owner's day-one ceremony with the YubiKeys, on the Mac (D393; the root key
  audit's §4). Follow docs/users/discovery-signing.md's "Day one" for staging's chains first. Passes
  when each `ssh-keygen -t ed25519-sk -O resident -O verify-required …` asks for the PIN and a touch;
  each genesis signed by every key `check`s; a signature made with a key whose credential was made
  without `-O verify-required` is refused by `check` ("made without the key's PIN"); each key's drill
  is named by `check --kind drill`; `ssh-keygen -Y verify` on a second machine accepts each genesis;
  `ssh-keygen -K` on that machine brings each key's stub back. Then a rotation: `root --remove
  e1.pub --add-everyday e3.pub`, signed by `e2` and `e3`, `check`s, and signed by `e1` alone is
  refused. (`cmd/discovery-sign`, `discovery/trust`)
- [ ] **VQ220** — a household installs through the bootstrap, which checks the release before running
  anything (D393 §8; the audit's §3.9), on the amd box from nothing. After VQ211 and a signed record
  published with a release: `curl -fsSL <the bootstrap's address> | sh -s -- --media /path/to/films`.
  Passes when it prints the SHA-256 the release page prints for the script (`sha256sum`), says
  "installing Orpheum <latest>, <repository>@sha256:…", the server comes up as with the install script
  alone, and `.env` names the image by that digest; then with the record's `.sig` replaced by another
  key's, it says "the release record's signature does not hold" and pulls nothing (`docker images`
  unchanged). (`cmd/discovery-sign/bootstrap.sh`)
- [ ] **VQ221** — the watcher tells the owner of a signature it did not make (the audit's F4), on
  nas.example. Run `go run ./cmd/discovery-watch --expected ~/ledger/expected --place <staging's issuer>`
  after VQ210: it says every signature is the owner's. Then publish, by hand, a root statement or
  address document signed by a test key outside the ledger at one place. Passes when the next run
  exits non-zero naming that file, the key's fingerprint and its counter, and its timer's alert reaches
  the owner's phone. (`cmd/discovery-watch`)
- [ ] **VQ234** — Firefox's page is live again soon after the network on a core's own address, with
  no `online` event to tell it, on nas.example. On the Mac, Firefox on nas.example's direct HTTPS name, in a
  Watch Together session, then again with `?transport=webrtc` in direct mode (no discovery). Turn the
  Wi-Fi off for 20 s, then on. Passes when, each time, the page is live again within about 5 s of the
  Wi-Fi returning: the session shows connected and a pause by the other device reaches it (its
  `kind=reconnected` line within about 5 s), and on `?transport=webrtc` its tries' last failure names
  "the offer reached no server". (`web/apps/web/src/transport-choice.ts`,
  `web/packages/sdk/src/together-connection.ts`, `live-connection.ts`, `rtc-reopen.ts`)
- [ ] **VQ235** — a page on a network without IPv6 says so when it cannot reach its core (D398 §3).
  Deploy nas.example's core with this build, and its discovery with it (connect, once it logs the reason,
  D398 §2). On a phone on mobile data with no IPv6 (or on a Wi-Fi whose router has IPv6 turned off),
  open a nas.example invitation's link, and sign in by the realm. Passes when each dead end's page says "This network seems to have no IPv6, and your server
  has no IPv4 way in yet, …" (the realm's opening "Your server was found, but"), the discovery's log
  has "a client found no way to the core" with `ipv6=no cause=noIpv6`, the ways tried and
  `seenOver=ipv4`, and no address; then the same link on a Wi-Fi with IPv6 reaches the core, or shows
  today's words if it does not. (`web/apps/web/src/dead-end-reason.ts`,
  `web/apps/web/src/discovery-gate.tsx`, `discovery/single/client.go`)
- [ ] **VQ236** — a timeline seek plays on without a second stop, on staging, on the Mac's Chrome
  (the owner's Nosferatu at 29:04, 2026-11-01: the film started at 1744.0 s, played 1.2 s and waited
  2.1 s with 3.7 s loaded ahead). Play a film a minute or more at its top quality, then click the
  timeline far ahead, three times at different places. Passes when each seek starts and plays on
  with no second wait (no `kind=started … startedAfter=waiting` line soon after the seek's
  `startedAfter=play`). If one does wait, send the seek's and the wait's lines whole: their
  `videoBuffered` and `audioBuffered` (every range, not the first alone) and `steps`, which now say
  where each flush ends (`video flushed 0.00-957.29`, hls.js's trim behind the playhead) and how long
  ago each choice was made (`chose 502 s ago ceiling 0 to 1`); the 2026-11-01 report's step up was
  made 501 s before its seek and its flush was that trim, so neither caused the wait, whose cause is
  still unknown. (`web/packages/player/src/diagnosis.ts`)
- [ ] **VQ237** — another audio track chosen cuts in at the choice, with no gap after it, on staging,
  on the Mac's Chrome (the owner, 2026-11-01: the old language played on 1 to 3 s, then the new one
  cut in with a gap). A film of two or more audio tracks on its ladder, playing a minute in. Choose
  the other track in the audio menu, then the first again, then turn the dialogue boost on and off.
  Passes when each track choice stops the old language at once, the picture waits there for at most
  about the 1 to 3 s the old language used to play, and the new language plays on from there with no
  gap or stop later; the notes say `sound N chosen (trackChosen), cut in at the playhead over sound
  M` then `sound N heard (trackChosen)`. Note whether the boost's switch, which still plays the sound
  loaded on until the boosted mix arrives (D400 §2), has a gap at its seam too: that is the owner's
  next call. (`web/packages/player/src/hls.ts`)
- [ ] **VQ238** — a used invitation leaves the People page and a role opens in the one dialog that makes
  it, on staging, on the Mac's Chrome (the owner, 2026-11-01, after VQ201; D403). As admin, People.
  (1) Invite Someone, make the link, close the dialog; open the link in Firefox and register: reload
  People, the invitation's row is gone from Invitations (the section too, when it was the only one) and
  the new person is in Accounts with the note you wrote. (2) A waiting invitation's row still opens its
  dialog with QR code, link, Copy and Withdraw. (3) Create Role opens a dialog headed New role with the
  name, the capabilities and the libraries; give it a name, tick a library and a capability, Add the
  role: the dialog closes and the role is a row. (4) Click that role's row: the same dialog opens,
  headed with the role's name, with what you ticked ticked and no Add the role; tick another library,
  close and reopen it from the row: it is kept, and there was no Save button. Rename it by editing the
  name and leaving the field. (5) Delete shows only for a role no one has; on Administrator and Can
  watch it does not, and Administrator's capabilities cannot be changed. (6) Open `/roles/3` by
  address: it is not a page any more. (`web/apps/web/src/role-dialog.tsx`, `people.tsx`,
  `invitations-section.tsx`; `internal/store/queries/invitation.sql`)
  *Run by the lead on staging, 2026-11-02, as lead-admin in Linux Chrome 156 (registering in Firefox
  155), scripted.* All six pass: a used invitation's row left Invitations and its account
  (`vq238-…`, Can watch, left on staging) showed the note; the waiting invitation's dialog has two QR
  images, the link, Copy and Withdraw; New role made a row; reopened from its row it was headed by
  its name, kept a library ticked after closing, renamed on leaving the field, had no Add the role and
  no Save; Delete only on a role no one has (not Administrator or Can watch, whose capabilities are
  fixed); `/roles/3` is "There is nothing at this address". Not seen: the section going when its
  only row goes (the owner's invitation "o" stayed). The Mac's Chrome is the owner's to tick it on.

- [ ] **VQ239** — *Quillwick II*, "Money" plays in step from its very start, on staging (the owner,
  2026-11-01: its audio was out of sync; measured, the sound half a second early for its first five
  minutes). After the deploy, play the episode from the start, the sound transcoded: its first run of
  sound chunks, made before the fix, is stored, so evict it or play it under a new sound ladder. Watch
  the lips over the first minute, then seek to about 4:50 and play across 5:00, where the first run
  meets the second. Passes when the speech is on the lips at the start and stays so across 5:00 with
  no jump or repeat. Do the same for any other XviD AVI from the start. (`internal/node/chunks/arguments.go`)

- [ ] **VQ240** — a link opened inside an app on an iPhone says to open it in Safari, with the link to
  copy (plan item 15, the Berlin guest's Telegram, 2026-11-02). On staging, send a Watch Together link
  by Telegram (and, if at hand, WhatsApp or Instagram) to an iPhone and open it there, in the app's
  own browser. Passes when the page is headed "Open this link in Safari", "Copy link" copies the link
  whole (fragment included) or, where the app refuses, shows it to select, and the copied link pasted
  into Safari joins the session. Note each app's user agent from "What happened". The same on Android
  in Telegram's own browser, headed "Open this link in Chrome". (`web/apps/web/src/open-in-browser.tsx`)

- [ ] **VQ241** — a Watch Together guest outside the house hears the host from the start, before
  pressing talk (D406; plan item 15, the Berlin guest's 90 s, 2026-11-02). After the deploy, on
  staging (no relay), the host joins from home and a guest on another network (a phone on mobile data,
  Wi-Fi off) joins by link and does not press talk or grant the microphone. Passes when the guest
  hears the host within a few seconds of joining, and the host's "What happened" shows the call
  connected at the first try, not a run of failures 5 s after "accepted". (`internal/core/turn/credentials.go`)

- [ ] **VQ242** — the button test records what the overlay's keys send (D407 §10; plans/player-overlay.md
  step 1). After the deploy, open Lab › Button test on staging and note its run name each time. On the
  Fire TV in Silk: put the full-screen test's box full screen, then press Menu (or `adb shell input
  keyevent 82`), Rewind (89), Fast forward (90) and Back, and the same with the page not full screen.
  On a Mac: press the ⏪ and ⏩ media keys with the tone on. On a computer: press +, =, − and the
  number pad's + and − on a Swiss German, a Turkish Q and a Japanese JIS layout. Passes when every
  press is a line in the core's log under its run, each key with its modifiers (`shiftKey`,
  `altGraph`, …), `location` and `isComposing` where set, and Menu either as a key or as a
  `contextmenu` line, or absent; note for each what arrived, as the input for steps 2, 9 and 10.
  (`web/apps/web/src/lab-key-table.tsx`)

- [ ] **VQ243** — the player's overlay on a Fire TV walks its rows, seeks only where it should, and
  hides by Back (D407 §4–§7; plan item 17, Right seeking instead of reaching Full screen, 2026-11-02).
  On the Fire TV, in Silk, play a film. Passes when: it opens with the overlay showing, focus on ▶;
  Up and Down walk the rows (‹ and the title; the scrub bar; the buttons), a row returned to keeps the
  button left there; Right walks the buttons to ⤢ without seeking while the overlay shows; with the
  overlay hidden, Right held seeks ahead with the scrub bar alone showing, gone 1.5 s after; OK while
  hidden pauses with a pause sign, which hides 3 s later while the film stays paused, and OK plays
  again with nothing shown; Back with the overlay showing hides it, and the next goes to the film's
  page; the same Back walk with the stage in full screen (Silk then leaves full screen first, D407
  §7). Silk never delivers Rewind or Fast-forward, so those are not tried here. While playing, the
  overlay hides 3 s after the last press, so a slow walk can find it hidden and Right then seeks: that
  is the rule, not a failure. (`web/apps/web/src/player-overlay.tsx`, `player-overlay-state.ts`)

- [ ] **VQ244** — a computer's full screen is the overlay, and Space presses the focused button (D407
  §5–§7; plans/player-overlay.md step 6). On a computer, in Chrome, Firefox and Safari in turn, play a
  film on staging. Passes when: Tab to ⤢ and Space puts the film full screen, without playing or
  pausing it, and the controls are then the overlay (the scrub bar row and the ⏪ ▶ ⏩ … ⤢ row over the
  film); a mouse's move shows the overlay and, the film playing and the mouse still, it hides 3 s
  later; with focus on ▶ while paused, Space plays and focus goes to the video, so the overlay hides 3
  s later; Down from the buttons does the same; with focus on the video, Space plays and pauses;
  Escape leaves full screen and the bar is back over the film, the film where it was; a
  double click on the film goes full screen and back. Note anything that differs by browser.
  (`web/apps/web/src/page-controls.tsx`, `player-commands.ts`, `player-overlay.tsx`)

- [ ] **VQ245** — the overlay's 💬, 🎧 and ⁝ choose a film's subtitles, audio and the rest in full
  screen, and subtitles move up out of the overlay's way (D407 §1, §8; plans/player-overlay.md step 7).
  On the Fire TV, in Silk, play a film with two subtitle tracks and two audio tracks, full screen.
  Passes when: Right walks the buttons ▶ … 💬 🎧 ⁝ ⤢; OK on 💬 opens Off, the tracks and Find
  subtitles, Up and Down walk them, OK chooses one and the subtitles show; 🎧 likewise switches the
  sound; ⁝ holds Start over, the subtitle offset, the sound timing and levelling, and OK on a stepper
  keeps it open; Back closes an open menu and leaves the overlay showing; while a menu is open the
  overlay does not hide; with the overlay showing, the subtitles sit above its rows, and they move back
  down as it hides; Find subtitles opens its dialog, and fetching one closes it with the subtitle on.
  Repeat 💬 and ⁝ on a computer in full screen with a mouse. (`web/apps/web/src/player-menus.tsx`,
  `player-overlay.tsx`, `player-overlay.css`)

- [ ] **VQ246** — Up next and Still watching show alone in the overlay's convenience area, and OK
  takes them (D407 §3; plans/player-overlay.md step 8). On the Fire TV, in Silk, play an episode with
  a next one and let the overlay hide; seek near the end. Passes when: Up next shows at the top right
  with nothing else of the overlay and no scrim; OK takes it wherever focus is; Left and Right seek
  with the scrub bar showing and Up next staying; Up opens the whole overlay, Up next still in its
  corner, and Back then hides the overlay while Up next stays; Back with the overlay hidden dismisses
  Up next. Then let three episodes autoplay with no press: Still watching shows alone the same way, OK
  continues, and Stop is reached by Up into its row. With nothing on offer and the whole overlay
  shown, the corner says when the film ends. (`web/apps/web/src/convenience-area.tsx`,
  `player-overlay.tsx`, `up-next.tsx`)

- [ ] **VQ247** — a computer's media keys and the Mac's ⏪ ⏩ change the episode, and ⏮ ⏭ show only
  within a sequence (D407 §2, §10; plans/player-overlay.md step 9). First read what VQ242 recorded for
  the Mac's ⏪ and ⏩: they are expected to change the episode only if they arrived as
  `MediaTrackPrevious` and `MediaTrackNext` (or as `previoustrack` and `nexttrack`). On staging, in
  Chrome and Firefox on a computer with media keys, and on the Mac, play an episode with one before
  and one after it, and press each key once it has played a moment. Passes when: next plays the
  episode after it and previous the one before, as Up next plays, on each machine whose keys arrive
  so; Chrome's media controls (its toolbar's media button, or the system's) offer previous and next
  track for that episode, and only next for a series' first episode and only previous for its last,
  once the episode has played; a film offers neither. In full screen the overlay shows ⏮ and ⏭ around
  ⏪ ▶ ⏩ for the same episodes, and only the one there is at either end; on the Fire TV, a cast's
  receiver shows ⏭ alone, which moves the cast on. Note what each machine's keys did.
  (`web/apps/web/src/up-next.tsx`, `input-commands.ts`, `player-controls.tsx`)

- [ ] **VQ248** — + and − move the volume, and 🔈 opens it upright, in the overlay (D407 §10, Q11;
  plans/player-overlay.md step 10). On staging, in Chrome and Firefox on a computer, play a film in
  full screen and press + and − (with a US keyboard, = as well, unshifted), then Ctrl and Alt with
  them. Passes when: each press moves the volume 5 % and shows "Volume n %" in the middle of the film
  for 1.5 s after the last, whether the overlay is hidden or shown, and + turns a muted film's sound
  on; Ctrl and Alt with them are left to the browser's zoom. Repeat on a Swiss and a Japanese (JIS)
  layout, noting what the unshifted key and the numpad's + and − do. 🔈 shows first of the buttons' far group
  (🔈 💬 🎧 ⁝ ⤢) and opens the volume standing up, full at its top, with mute under it, both keeping
  it open. On the Fire TV and a phone there is no 🔈. 🎧 names the track playing, the film's first
  where none was chosen. (`web/apps/web/src/player-commands.ts`, `player-menus.tsx`,
  `player-overlay.tsx`)

- [ ] **VQ249** — outside full screen the controls lie under the film, nothing over it (D407 §9;
  plans/player-overlay.md step 11). On staging, on a phone held upright and then on its side, open
  a film's Play page without full screen. Passes when: nothing covers the picture but subtitles;
  under it are the timeline, then ⏪ ▶ ⏩ (⏮ ⏭ around them in a series) and ⤢, then "Ends 9:41" or
  Up next and Still watching as ordinary buttons; a tap on the film neither pauses it nor shows
  anything over it; ⤢ puts it full screen with the overlay. On the Fire TV, play a film (it opens
  full screen with the overlay), then press Back until Silk leaves full screen. Passes when: the
  film then shows with the controls under it, focus on ▶, a ‹ to the film's page among them; Left
  and Right on the film seek, OK plays and pauses, Down reaches ▶; ⤢ returns to full screen; and,
  out of full screen, the next Back goes to the film's page. Note what each Back did.
  (`web/apps/web/src/page-controls.tsx`, `up-next.tsx`, `play.tsx`)
- [ ] **VQ250** — a pop-up is not a dialog: no ✕, sized to what it holds, its list showing context
  (D408 §2–§3). On the Fire TV in Silk, play a film with several subtitle tracks and open the
  overlay's 💬, 🎧, ⁝ and 🔈 in turn, then the banner's menu, a library's sort and a title's ⁝.
  Passes when: none of them draws a ✕, and every dialog (Find subtitles, Identified incorrectly)
  still does; focus opens on the current choice with a choice above and below it in view where the
  list has them, and moving down or up keeps the next one in view; Back, Left or Right closes each
  without choosing, focus back on its button; 🔈's pop-up is no wider than its upright slider, and
  Up and Down move the volume while OK on mute mutes. Note any pop-up that opened on its first
  entry, not its current one.
  (`web/apps/web/src/popover.tsx`, `list-box.tsx`, `player-menus.tsx`)
- [ ] **VQ251** — a pop-up is as wide as its widest entry and keeps clear of the screen's edges
  (plans/toolkit.md items 8 and 9; the owner, 2026-11-03). On the Fire TV in Silk and on the phone,
  open the banner's menu (from the screen's top right), a library's sort and a title's ⁝. Passes
  when: no choice of the menu or the sort wraps where the screen is wide enough to hold it; the
  menu's "Signed in as …" line never widens the menu, wrapping to the width its choices set (the
  owner, 2026-11-03: it "is fine to wrap, since it's just informative content"); none of them covers
  the outermost half rem of the screen at any edge (a sliver of the page shows beyond it), the
  phone's status bar and rounded corners included; and on the phone, a choice wider than half the
  screen widens its pop-up to 16rem, about two thirds of the screen, before it wraps. Note any
  pop-up that wrapped a choice or met the screen's edge.
  (`web/apps/web/src/popover.tsx`, `popover.css`, `drop-down-place.ts`, `tokens.css`)
- [ ] **VQ252** — 🔈's pop-up grows out of its button (plans/toolkit.md item 11; the owner,
  2026-11-03). On a computer, play a film full screen and press 🔈. Passes when: the pop-up's mute
  speaker sits where 🔈 was, the button's size, the slider standing above it, so the button looks to
  have grown upward, focus on that speaker; it shrinks back to 🔈 at a press outside or Escape. Note
  anything that drew a gap, a pointer or a jump between the button and the pop-up. (The Fire TV and
  the phone show no 🔈.) (`web/apps/web/src/popover.tsx`, `drop-down-place.ts`, `player-menus.tsx`)
- [ ] **VQ254** — Cast and Watch Together open at once (plans/toolkit.md item 10). On staging, in a
  fresh tab of a phone and then of the Fire TV in Silk (after a deploy, so nothing is cached), open
  the home page, wait a few seconds, then press the tray's Cast sign; close it and press Watch
  Together's. Passes when: neither sign is ever replaced by "Loading…" and each dialog opens on the
  press. Note how long after the page showed the press came. (`web/apps/web/src/tray-dialog-code.tsx`,
  `screen-load.tsx`'s `preloadableScreen`)
- [ ] **VQ253** — the menus that were native selects work by remote (plans/toolkit.md step 6). On
  the Fire TV in Silk, play a film with two audio tracks and several subtitle tracks out of full
  screen, and reach the Sound and Subtitles choosers under the film with the arrows; then open Find
  subtitles and its Language; then a series with two orderings and its Ordering. Passes when: OK
  opens each as a list on its current choice, an entry either side in view, the arrows move in it,
  OK on another chooses it (the sound or subtitles change, the search asks in that language, the
  series shows in that order), and Back closes it without choosing, focus back on its button.
  (`web/apps/web/src/audio-tracks.tsx`, `subtitles.tsx`, `subtitle-search.tsx`, `series.tsx`)
