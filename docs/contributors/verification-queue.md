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
- [ ] **VQ69** — On the Fire TV stick (Silk, spatial mode) and a laptop's Chrome, signed in with
  autoplay on, play an episode with at least four after it in its season, seeking each to a minute
  before its end and pressing nothing else, and let three play on by themselves (D339). Passes when
  the fourth's end shows *Still watching?* with Continue focused and no count; OK plays the next
  episode; done again, Stop (Right, then OK) leaves the film for the page before. Then, with two
  autoplayed, press any key or tap mid-episode: the next end counts *Plays in 10 s* as usual.
  (`web/apps/web/src/still-watching.tsx`, `up-next.tsx`)
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
- [ ] **VQ72** — On a Fire TV stick in Silk's spatial mode (D349, library-browsing.md §5.5), open the
  Libraries page from the top bar with two or more libraries, one of them an `other` library. Passes
  when Up from the grid's top row reaches the bar, Left and Right move among the library chooser, the
  view chooser (the `other` library only) and the sort chooser; OK on any of them opens its list of
  choices as a focusable overlay with focus on it at once, arrows move within it, OK on a choice
  follows it and closes the overlay, and Back closes it without choosing, returning focus to the
  chooser button; the alphabet rail at the grid's right shows only while sorted by name or file name,
  and Right from the grid's last column reaches it; opening the `other` library's Folders shows its
  subfolders and videos, Up leaves a folder, and one click reaches each from the bar.
  (`web/apps/web/src/library-browse.tsx`, `modal-dialog.tsx`, `focus-navigation.ts`)
- [ ] **VQ73** — On staging, put a decrypted DVD image (`.iso`, a film of your own, made by MakeMKV's
  backup or a DVD ripper that decrypts) in a film library and scan it (D350). Passes when the film
  is identified by its file name, its page shows the main feature's length (not a trailer's) and
  its audio tracks with their languages, it plays from the start, and a seek to the middle plays from
  there with picture and sound in step. An encrypted image is instead listed as a file that could
  not be read, its message saying it is encrypted. (`internal/node/disc`,
  `internal/node/execx/disc.go`)
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
  places from now on`, copy 12304 into file 12168), each library lists Sync Fig as item 5285, the
  doctor names both paths (and found two real pairs: The Pumbletons S28E14 and S28E15 are one file, and
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
  (`web/apps/web/src/page-controls.tsx`, `player-controls.tsx`, `timeline.tsx`, `icon-button.tsx`,
  `tv-controls.tsx`, `cast-remote.tsx`)
  **The lead's check 2026-10-28 (headless Chromium on Linux, computer part only):** the app's bar in
  place of the browser's controls, hidden 5.05 s after a move; a click plays and pauses, a double
  click enters and leaves full screen (emulated); Tab and the arrows reach every control; Left and
  Right seek 10 s; Space plays and pauses from Mute, Full screen or the timeline without pressing
  them. Left for the owner: phones, the Fire TV, a cast's remote, the tray's feel.
- [ ] **VQ86** — D368, on nas.example or staging, in Chrome on the Mac at home: play a 5.1 film (AAC or
  E-AC-3) and a stereo one. Passes when each starts at the top on its own sound (the tracks note says
  "E-AC-3 5.1 (original)" or "AAC stereo (original)", and the core's `a playback started` line says
  `delivery=ladder`, `ladderQuality=picture`, `ladderSound=sound0-own`); a forced step down to a lower
  quality and back up (DevTools throttling, with "Bypass for network" on: idiosyncrasies.md) switches the sound to
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
- [ ] **VQ87** — On the Fire TV stick, signed in as the admin through the discovery's page (the core
  deployed, and orpheum-connect packaged, from this batch). Passes when: the banner's menu, opened by
  OK, scrolls within itself as the remote moves down it, in Silk's spatial mode too, every place in
  sight as it is reached, down to Sign out, whose OK signs the TV out; the TV then shows the
  discovery's own sign-in (app.orpheum-app.com), not the server's, and does not go back to the
  household by itself; and, signed in again, the TV removed under *Your devices* on a phone leaves
  its films for that same sign-in within a second or two, with nothing pressed on the TV. (The
  owner, 2026-10-27: no way to sign out on the TV; a removed TV kept its films until pressed; it
  landed on the server's sign-in.) (`web/apps/web/src/app-shell.css`, `live.tsx`,
  `loader-at-client-origin.tsx`, `loader-households.ts`, `internal/core/live/registry.go`)
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
  readable on pale art (A Woman of Paris marked by hand), no fading; a series' check and progress; the
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
- [ ] **VQ91** — D367, Back and Up, and a library's Configure, on staging. Passes when, on a computer
  or a phone, no page shows a Back of its own and each page below the top shows its trail
  (`Series › Copper Kettle › S1E1 Pilot`), each part leading up: (1) Continue Watching on Home → play an
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
  (staging's Copper Kettle has seasons 2–4: S2E12 into S3E1), (3) (one Back still leaves the page, though
  the first choice adds a history entry), (5) by click, and (6) on the desktop and the phone size all
  hold. Left for the owner: (4), the remote's OK and Back, the iPhone's swipe, real phones.
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
  list moves up 22 px to stay on screen, its choice no longer over the button's centre. Left for the
  owner: everything on the Fire TV.
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
- [ ] **VQ101** — a cast alone to the Fire TV plays as the Fire TV plays a film alone (D371 §1, §4,
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
- [ ] **VQ103** — a guest casting to a second device follows the group there (D371 §4), on staging.
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
- [ ] **VQ104** — the lead passes to a guest while the host's screen is gone and comes back to it in
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
- [ ] **VQ105** — a locked phone's lead passes at once (D372 §2, §5), on staging. Passes when the
  iPhone, a guest, leads a session (the host playing on no device) beside a computer's browser, a
  second guest: (1) locking the iPhone hands the lead to the browser at once, the browser's film
  playing on without a stop, and the log says `goneBecause=hidden`; (2) unlocking the iPhone catches
  it up as a follower, and closing the browser's page then gives the lead back to the iPhone, its
  `leaderSeniority` the moment it was unlocked; (3) a stall of the leader's (its link throttled)
  holds the group (`waitingFor`) and does not move the lead. (`internal/core/together/leader.go`;
  `web/packages/sdk/src/together-follow.ts`, `together-connection.ts`)
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
- [ ] **VQ110** — a player alone or leading starts, and plays on after a seek, only once what it
  loaded outlasts its next chunk's load, and its dropped frames reach the log (the owner, 2026-10-28),
  on staging. Passes when, the Mac casting to the Fire TV (Silk) with Watch Together off: (1) The
  Marrowgate Family, resumed part way where its first chunk ends a second or two past the resume point
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
- [ ] **VQ116** — a Fire TV holds its quality while plenty is loaded, and a change of sound with a
  change of quality throws no loaded sound away (staging, 2026-10-28: device 60, file 5671 Nosferatu, one
  slow sample of five, 376471 B in 1200 ms, lowered the ceiling from the film's own picture to
  q700k432p with 37.3 s loaded, `chose started, ceiling 3 to 1`, and the sound's change from the
  copied `sound0-own` to `sound0-96k-ownlevel` left 2.5 s of sound against 38.6 s of picture and
  stalled the TV 1172 ms, `startedAfter=waiting stoodMs=1172`), on staging. Passes when, the Fire TV
  (Silk) playing Nosferatu (file 5671) for 2 minutes from where it was: no "chose … ceiling a to b" step
  down while 20 s or more is loaded ahead (a held lowering reads `held at`, with what was loaded);
  and, where the quality does change between the top and a lower one (a later step down below 20 s,
  or a step up), the stall and seek reports show the sound's buffer reaching as far as the picture's
  after the change, no `startedAfter=waiting` follows it, and the sound plays on across it with no
  gap or blip heard. (`web/packages/player/src/abr.ts`, `link-estimate.ts`, `hls.ts`)
  **The lead's reading 2026-10-28 (staging's log):** as written it holds (no lowering with 20 s or
  more loaded; the sound's buffer reaches the picture's after each change; no `startedAfter=waiting`
  after 15:24). But at 15:33:05 Nosferatu dropped from 9018 to 696 kb/s 2.3 s into play on an estimate of
  69 kb/s while its picture chunks had loaded at 14–18 Mb/s, and stayed there 75 s: most likely a
  chunk given up, counted as a sample of the few bytes that came over the time waited, which a
  harmonic mean cannot outvote for five chunks. A quality switch's note now says why the controller
  chose it and its samples (`why`, `linkSamples`), so the next one says; how a given-up chunk counts
  is the owner's to decide (D352).
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
- [x] **VQ119** — A Woman of Paris plays from its start and from a resumed point (staging,
  2026-10-28: file 5667 on the Mac in Firefox, its own 5.1 sound copied failed chunk 0 twice, "raw
  00000.mp4 holds 0 to 12267 ms of the run, not its chunk's 10176 to 12262 ms", and the film could not
  play, `cannot_convert`; its first keyframe is before its start, discarded, so its grid starts
  10.176 s in), on staging once deployed. Passes when, on the Mac (Firefox or Chrome), A Woman of Paris
  (file 5667) plays from its start with its sound, the tracks note naming its own 5.1 sound, for a
  minute with no error, and again from a point resumed past the half hour; and the core's log has no
  `runs of chunks failed at the same chunk again` for `5667/chunks-sound0-own`. The same for one
  other film of the shape (Glass Runner Final Cut, file 5691, or The Gold Rush, file 5697).
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
  the Fire TV (Silk) over WebRTC, play a converted film (Nosferatu, file 5671, or another whose top is
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
