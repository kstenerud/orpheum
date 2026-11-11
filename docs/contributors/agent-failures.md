> **ABOUTME:** The log of mistakes the code-writing agents made that the owner had to correct:
> what happened, what caught it, and whether a gate now catches it. The raw material from
> which the next lint rule, test or hook gets built.

# Agent failures

This project is written by agents, so **the agent is a component with failure patterns**, like
any dependency whose behaviour we cannot change and can only characterise and guard against.
This file records the specimens. Its model is yoloAI's log of the same name, whose conclusion
was *"Execution catches my errors. Reading does not"*, and whose gates against the agent's own
habits were each built from an entry written here first
([principles §21](principles/21-an-owners-correction.md)).

## When to write an entry

**The trigger is an owner correction.** The owner contradicts a claim, or asks a question
("what did you run?", "why does that need X?") whose honest answer is "I was wrong". Record it
**before moving on**, while the reason the wrong claim felt like knowledge is still
recoverable. Once the answer is known the error looks obviously avoidable, and all that is left
to write is "be more careful", which is the one lesson known not to work.

**The bar is that the false belief reached something durable**, or would have: a commit, a
decision, a plan, a filed defect, a recommendation someone acted on. A wrong guess that the next
command corrected is the process working, and does not belong here. A log of every fumble is
unreadable.

A gate that catches a mistake before the owner does is the system working; it needs no entry
unless the mistake is new in kind.

## The entry format

Newest first. IDs are `A<n>`, allocated as one more than the highest in this file, and never
reused.

```markdown
### A<n> — <what went wrong, in a phrase> (<YYYY-MM-DD>)

- **What happened:** what was claimed or built, and what was true. Name where the false
  belief came from: a search result, a subagent's summary, a comment, an error string, a
  document, a test that could not fail.
- **Caught by:** who or what noticed, and how. This is the field that says what to build: if
  it keeps reading "the owner asked", the mechanisms are absent, not weak.
- **Gate:** what would catch it now — a lint rule, a test, a hook, a check — named precisely,
  or "none" with the reason. "None" is an honest answer and a to-do.
```

**Cite entries by ID, never by count.** A pattern drawn from these entries names the IDs it
rests on, so the claim can be checked against them; a total goes stale with the next entry.

## Log

### A69 — Reported a test's bound as unmeetable and asked for a ruling, when the failure showed the design measured the wrong thing (2026-11-11)

- **What happened:** VQ294 bounded the chunks around a heavy stretch at 5 MiB, the sized grid's
  `largestChunkBytes`. The run found chunks of 13 to 23 MB, one keyframe's run each, and the lead
  wrote the clause up as "cannot be met by cutting at keyframes. Owed: the owner's call on that
  clause", then proposed rewording it to "where the keyframes allow" and ticking the item. The
  failure had shown the grid's premise false: a copied picture cannot always be cut small, and what
  a small device must hold is a chunk and its neighbour together, so a 21 MB chunk would still stall
  it. The belief came from treating the item's clause as the goal rather than the playback it stood
  for.
- **Caught by:** the owner: *"how can we handle a situation where a heavy stretch makes a 21mb
  chunk? I'm not interested in just making VQs pass for the sake of passing them. I want a good user
  experience."*
- **Gate:** none can judge a premise. Principles §29 asks the three questions of every failure; the
  design that followed is the plan file's 11-23 item 4.

### A68 — Twice named the cause of an iPhone's grey band from a guess the measurements did not test (2026-11-11)

- **What happened:** Chrome on the owner's iPhone, turned sideways, drew the Play page short of the
  screen, grey beneath. The lead first held the zoom at 1 as the phone turned, which made it worse
  (the page stuck at 2.16 zoom), then read the page's log of its sizes, 808 by 275 of 896 by 414,
  as a page laid out inside the screen's safe areas, and had the Play page say `viewport-fit=cover`.
  Neither guess was tested by what had been measured: the log never held the safe-area insets, nor
  the window's own outer size. Logged next, with cover in force, the insets were 0 on every side and
  the window itself was 808 by 275: Chrome sizes its web view short of the screen, and nothing the
  page draws reaches outside it.
- **Caught by:** the owner: *"turned it sideways and it didn't fill the whole screen"*, and *"landscape
  still has the grey area underneath"*.
- **Gate:** none can. Nothing general beyond: a diagnosis is logged on the device for the quantity
  the fix changes (here the insets and the window's frame) before the fix is built.

### A67 — Reported a list's blank posters working, measured on a held Down alone, when they held Films' opening 4.4 s (2026-11-11)

- **What happened:** the lead added a blank card for every item still to come (332 on Films) and
  reported them working on the Fire TV from a trace of a held Down through the grid. Opening the
  page was never traced. Finding the page's arrival asked every card whether it was drawn, and each
  card off the screen was laid out alone for the asking, so the screen stayed blank about 4.4 s
  after Films was chosen. The belief came from measuring the path the change was for, and not the
  paths the change made longer: a change that multiplies the page's elements costs everything that
  walks them.
- **Caught by:** the owner: *"When I switch from shows to films, there's a 3-4 second delay before
  anything shows up in the poster area. I worry that our fixes are trading one issue for another."*
- **Gate:** the arrival's search now stops at the first control drawn, pinned by
  `focus-engine-skipped.browser.test.tsx`'s arrival test. Nothing general: a change that adds
  elements is traced opening its page too, not only on the press it was for.

### A66 — Said a scroll redraws every poster in view, when the browser moves what it has already drawn (2026-11-11)

- **What happened:** asked how much of the screen a focus move redraws, the lead told the owner
  that a move which scrolls makes everything in view move and be redrawn, every poster's blurred
  shadow with it, and called that likely the most expensive thing a press does on a weak GPU.
  Chromium, which the Fire TV's Silk is, scrolls a composited scroller by moving the tiles it has
  already rasterized and rasterizes only what comes into view. Nothing in the app's CSS (no
  `backdrop-filter`, no filter over the scroller) was checked to defeat that before the claim. The
  belief came from describing a cost in its worst case, and as the usual one, without a trace.
- **Caught by:** the owner: *"I'm just surprised that entire posters need to be redrawn when the
  browser's DOM handler should be handling scrolling movement automatically."*
- **Gate:** none can check a claim about a cost. Measure first: a trace's paint areas and layers
  say what a scroll repaints. Until one is taken, say what is unmeasured.

### A65 — Said production's machines were out of the sandbox's reach, when the machines notes give a key to each (2026-11-09)

- **What happened:** planning production's migration to v0.3.3's schema, the lead told the owner
  that no credential able to touch production was in the sandbox, so the steps on nas.lan and
  ainas.lan were the owner's to run. The operations repository's machines notes list an `orpheum`
  user on each with the sandbox's key (`~/.ssh/nas_lan`, `~/.ssh/ainas`), and production was
  running on ainas at the time. The belief came from a standing constraint carried in the
  session's summary, older than the failover to ainas, repeated without reading the notes that say
  where production runs and how it is reached.
- **Caught by:** the owner: *"On nas.lan and ainas.lan you have an orpheum user that has
  allowed_keys for you."*
- **Gate:** none can check a claim about access. Before saying a step is out of reach, read the
  machines notes, or try a read-only command on the machine.

### A64 — Cited commit hashes as if they lasted, when every commit leaves the sandbox under a new one (2026-11-08)

- **What happened:** handing over the filtered public repository, the lead warned that hashes
  quoted in the docs "won't match the public repo", as if they matched today. They never match
  outside the sandbox: `yoloai apply` replays each commit with `git am`, which gives it a new
  committer date and so a new hash. 443 commit messages and 165 distinct hashes in the tree's
  Markdown cite sandbox commits that the owner's repository does not have. The belief came from
  habit: inside one repository a hash is the most exact reference there is, and nothing in the
  sandbox showed that the commits are re-made when they leave it.
- **Caught by:** the owner: "wouldn't the hashes be out of date from the start, considering every
  commit pulled out of yoloai would get a different hash?"
- **Gate:** `lint_commits.py` and `check_docs.py` refuse a hash that names a commit of this
  repository and give its subject to cite instead (`commit_hashes.py`, the owner: "all of that",
  2026-11-08). The docs' 210 such citations became subjects. A hash that no longer resolves (a
  commit rebased away) cannot be caught or mapped, and stays.

### A63 — Asked the owner three Watch Together questions the decisions already answer (2026-11-07)

- **What happened:** in the playback redesign's dialogue, the lead asked whether a guest may pause
  and seek, whether a guest may add people, and whether a host may hand the session on. The owner:
  "We've already covered these questions. The host gives permissions for pause and seek. Guests
  can't add others. A host only leaves by ending the session." D271 (guests pause and seek unless
  the host withholds it) and D246 (what a person may do is their role's) settle the first two, and
  ending is the host's only way out.
- **Caught by:** the owner.
- **Gate:** none can check it. Before asking the owner a question in an area, grep the decision
  index for that area's decisions and read the matching ones (AGENTS.md "Start here" step 3).

### A62 — Explained the TCL TV's sound timing backwards: picture late, when the sound was (2026-11-06)

- **What happened:** the sound timing test met at 0 on the owner's TCL TV while the library's sync
  clips needed 200 ms. The lead told the owner the TV's video frames reach the screen 200 ms behind
  the sound, so the sound must be held back, and wrote the same into VQ269 ("docs(verification): VQ268 passed on the TCL TV; VQ269 fails, its drawn flash missing the video's delay"). The owner's
  next test, which the new log lines recorded ("fix(api): say each device's sound offset setting, and each playback's offset and levelling"), set the sound 300 ms *earlier*: the sound
  is the late one, as VQ269's own opening already said ("plays sound some 400 ms late"). The lead
  read "200ms offset" as later without asking which way, and built a mechanism on the guess. That
  the page-drawn test cannot stand for a film holds either way; which path lags is not known.
- **Caught by:** the owner's next measurement, read in the log.
- **Gate:** none. The logs now carry each offset set and each playback's offset and levelling, so a
  direction is read, not guessed.

### A61 — Told the owner a TV signed in as admin was a Watch Together screen, and that cast ignores the sound offset (2026-11-06)

- **What happened:** the owner, on the TCL TV in Berlin, could not reach the sound timing test.
  The lead said the TV was a Watch Together screen with no menu; it was signed in normally as
  admin by device code (the QR code), and the menu said only that managing needs a local
  connection. The lead then filed in the plan that cast receivers ignore the device's sound
  offset, from a search that found `SoundOffsetControl` mounted only in play.tsx, bare-player.tsx
  and player-menus.tsx. The cast receiver renders the Play page (play.tsx, `PlaysAlong` →
  `CastReceiver`), so it applies the offset and has the control in ⁝ and under the film. Both
  beliefs came from where a file name pointed, not from following the render path or asking how
  the owner had signed in.
- **Caught by:** the owner, for the first ("No, I'm logged into the TV normally, as admin"); the
  lead, for the second, on reading cast-receiver.tsx while building the cast remote's control.
- **Gate:** none. The plan note is corrected; what the remote lacked, a way to move the receiver's
  timing, is built ("feat(web): the cast remote moves the receiver's sound timing").

### A60 — Proposed a gate change for a lint "gap" that was a gate never run (2026-11-05)

- **What happened:** check-pr's full web lint failed on a test the lead had just committed (a
  `<video>` without captions), and the lead told the owner that `make check` had missed a lint
  rule check-pr catches, proposing to run the same rules in both. The owner said "ok add the gate
  change". Both lints run the same eslint configuration; they differ only in eslint's cache. The
  test had never met `make check`: the commit's `make spot` runs no web lint, and every turn since
  21:52 the day before had ended while check-pr ran, so the Stop hook deferred its `make check`
  (`.claude/.cache/gate-runs.jsonl` holds no run between). The belief came from inferring the
  cause from which tier reported the failure, without asking which tiers had run.
- **Caught by:** the lead, starting the change: the two lint scripts differ only in `--cache`, and
  `make check`'s lint, run on the bare `<video>`, fails it.
- **Gate:** D424: a failed quick tier now stops check-pr before revert-red, so the next such slip
  costs a minute, not half an hour. Nothing stops the wrong diagnosis itself; the lesson is to ask
  which tiers ran before saying which one missed something.

### A59 — Told the owner a TV is always full screen, from a research summary of today's build (2026-11-02)

- **What happened:** reviewing the owner's overlay design, the lead said the 🔈 and ⤢ buttons were
  dead weight on a TV because "the TV is always full screen". The owner: "The TV is NOT always
  fullscreen. You need to be able to access controls like watch together." The false belief came
  from the research file's finding that a TV opens the player full screen, read as a rule of the
  device rather than as how the build starts.
- **Caught by:** the owner, reading the review.
- **Gate:** none; how the build behaves today is not what the design requires, and a claim about
  what a device always does needs the decision that says so, cited.

### A58 — Named a likely cause of a guest's failure, and recorded it as found, from a reproduction of the symptom alone (2026-11-02)

- **What happened:** a Watch Together guest in Berlin was told her network could not reach the
  server, while the core logged her connection held and closed 100 ms later. A staging run of
  Firefox with service workers off reproduced the same log and message, and from that the plan, an
  idiosyncrasy entry, code comments and a commit message said her browser ran no service worker
  (Firefox in a private window), "found and fixed". The owner asked her: ordinary Safari, which runs
  one. The false belief came from a matching symptom taken for a matching cause, with the one fact
  that would tell them apart (her browser) unknown.
- **Caught by:** the owner, by asking the guest.
- **Gate:** none; a symptom reproduced is not a cause confirmed, and until the person's own device or
  report confirms it, the record says "a cause that fits", never "found".

### A57 — Called revert-red's kept green stale after re-running it against the wrong base (2026-10-31)

- **What happened:** check-pr said reverting `install.sh` turned no test red, from a green kept since
  An earlier commit. The agent re-ran revert-red on the file with `BASE=` set to the merge base with
  `origin/main` (858 commits back), saw it red, and told the owner the cache was stale and a gate
  defect. check-pr's base was `refs/orpheum/checked`, 92 commits back, after the digest pinning;
  reverted only to there, the file loses just the discovery's loader line, which no test checks.
  The kept green was right, and the file did need a test. The belief came from taking "the batch's
  base" to be the remote's, never reading the base check-pr's own log prints on its first line.
- **Caught by:** the owner asking the agent to look into the "stale cache".
- **Gate:** none yet. A targeted re-run (`make check-revert-red ONLY=…`) could default its base to
  check-pr's, as the background probe does (`scripts/background_probe.py`'s `base()`), and refuse
  a different `BASE=` without saying it differs.

### A56 — Said a self-hoster's discovery chain protects nothing, without asking what the one-core discovery is for (2026-10-31)

- **What happened:** asked what a self-hoster's chain is for, the lead answered that a discovery
  inside the core checks its own signature, so a chain protects nothing, and proposed dropping it.
  The one-core discovery exists for the opposite case: a public face on a cheap VPS, the exposed
  machine, where a chain held by the home core is exactly what protects the core. The lead had
  assumed the deployment instead of reading D391 §8's purpose or asking.
- **Caught by:** the owner ("The idea behind the one-core discovery is so that users can host their
  own public internet interface to their core").
- **Gate:** none possible; D394 §2 now states the purpose. The lesson: before judging a design's
  worth, state where it runs and what it is for, and ask when that is not written down.

### A55 — Told the owner the session could not edit gates, without checking its environment (2026-10-31)

- **What happened:** preparing a handoff, the lead deferred the token-use gate patch to the next
  session, saying hooks and scripts can only be edited in a session started with
  `ORPHEUM_ALLOW_GATE_EDIT=1`, and gave the owner a relaunch command adding it. The session already
  had it (`echo` showed `1`). The belief came from memory: a note that yoloai sometimes drops
  environment variables on restart, read as "it is not set", and an earlier refusal of an edit to
  `.claude/standing-orders.md`, which is a commit-trailer rule, not the edit hook.
- **Caught by:** the owner ("You were already started with ORPHEUM_ALLOW_GATE_EDIT=1").
- **Gate:** none needed for this one: a one-line `echo` settles it. The rule: a claim about the
  session's own environment or permissions is checked with a command before it is said.

### A54 — A search for a source file walked the whole disk, the read-only media share with it (2026-10-31)

- **What happened:** an agent building B-frame re-timing looked for FFmpeg's `dts2pts.c` with
  `find / -name dts2pts.c`. The walk from `/` reached `/tank/media`, the owner's media share, which
  the owner's standing rule says never to stat, list or search (a stat of it while the NAS was down
  once froze a turn). Its brief repeated the rule; the agent read it as "do not search the media",
  not as "no command may walk into it". It stopped the walk after about two minutes; nothing was
  printed and nothing froze.
- **Caught by:** the agent itself, reporting it.
- **Gate:** none yet. Briefs now say "never run `find`, `du`, `ls -R` or any walk from `/` or any
  parent of `/tank`; search named directories only". A hook could refuse a shell command naming
  `/tank` or a walk rooted at `/`; it is a gate change, the owner's to accept.

### A53 — A stale service worker fixed for the case that cannot happen without a page load (2026-10-31)

- **What happened:** the owner's Firefox on production said developer tools bypassed its worker
  while media played. The lead named the cause, the last release's worker still in control, and
  built a fix that probes again on `controllerchange`, as the new worker takes over.
  But nothing in the client ever asks the browser to update its worker: Firefox checks only on a
  page load, and one made while connectd restarted for a deploy found nothing. The tab then went on
  under the old worker through every in-app move, and no new worker ever took over, so the fix never
  ran. The belief came from the worker's own `skipWaiting` and `clients.claim`, read as "the new
  worker takes over a moment later" without asking what makes the browser fetch it.
- **Caught by:** the owner, checking VQ194 after the deploy: the note was still there. The core's log
  showed the note's report after the deploy, and the client origin's `sw.js` holding the probe.
- **Gate:** the next fix asks `registration.update()` before the note speaks, tested with a worker
  that takes over only after an update. The lesson is to trace a fix to the event that starts it.

### A52 — A missing top bar blamed on full screen before asking what the screen showed (2026-10-29)

- **What happened:** the owner, a guest in Watch Together on the S23, said the view showed none of
  the top bar, so the Cast tray could not be opened. The lead named a suspect at once: the player
  taking an Android browser for one already full screen (`isBrowserFullscreen`, its window being
  the screen's size) and filling the window. The owner had not entered full screen; the page was all
  there, the banner holding the full logo and nothing else.
- **Caught by:** the owner's description of the screen.
- **Gate:** none. Before naming a cause for a missing element, ask what is on screen in its place;
  "none of the top bar" and "the top bar without its items" lead to different code.

### A51 — A device step ticked a DevTools setting and never said to untick it (2026-10-29)

- **What happened:** VQ123's steps told the owner to tick Chrome DevTools' "Bypass for network" to
  check the page's error, and said only that Reload "with the box unticked" opens the page, never to
  untick it. Chrome keeps the box ticked across DevTools sessions; hours later a tab's posters all
  404'd, and the lead and an agent spent the afternoon on the service worker before the
  owner found the box still ticked. (A48 had earlier blamed the same box without evidence; this
  time it was the cause, and VQ123 the likely way it was set.)
- **Caught by:** the owner, checking the box when asked.
- **Gate:** none. A device step that changes a setting ends with the step that restores it, and
  when a device misbehaves after such a step, ask about that setting first.

### A50 — A reproduction signed in as the owner's account stopped the owner's live session (2026-10-29)

- **What happened:** while the owner tested Watch Together on staging, the lead ran a headless
  Chrome through staging's discovery with a profile signed in as admin, the owner's own account, to
  reproduce a poster fault. Opening the app had the owner's hosted session back into that tab
  (D267 §2), and the script's click on the first "Stop" button, meant to end its own test session,
  pressed "Stop the film for everyone" in the owner's session.
- **Caught by:** the lead, in the core's log (`event=stopped byMemberId=2`).
- **Gate:** none. Reproductions against staging while the owner tests use an account of their own,
  never the owner's or one the owner's devices use, and never press session controls.

### A49 — Wrote device steps from a VQ's summary line, with a guest choosing the film (2026-10-29)

- **What happened:** walking the owner through VQ105, the lead wrote "have the S23 join as a guest
  and play the film first, so the S23 leads", from the item's phrase "the host playing on no device".
  A guest cannot choose what plays; only the host can (D372 §4). The setup D372 §6 means is the host
  putting the film on and then browsing away from the session's page, so the lead passes to the
  longest-present guest.
- **Caught by:** the owner ("A guest can't choose what plays; only a host.").
- **Gate:** none. Before writing a VQ's steps for the owner, read the decision it cites and check
  each step against who may do it.

### A48 — Told the owner his own DevTools setting caused a fault, on a subagent's word (2026-10-29)

- **What happened:** a debugging subagent reported that the owner's black page on staging (the core's
  client scripts answered 404 by the client origin) was caused by Chrome DevTools' "Bypass for
  network" being ticked, and that our own VQ86 text had told him to tick it. The lead repeated this to
  the owner as the cause, and as the lead's fault, without evidence that the setting was on: the
  subagent had only reproduced the same symptom with the bypass on in Playwright. The owner had never
  touched the setting; the fault recurred for the lazily loaded scripts after the next deploy.
- **Caught by:** the owner ("I never changed it from whatever it was").
- **Gate:** none. A reproduction of the same symptom is a hypothesis, not a cause; the lead must ask
  the owner to confirm a device-side condition before stating it as the cause.

### A47 — Read a player's first quality note as a switch up, and told the owner so (2026-10-28)

- **What happened:** the Fire TV stuttered as a cast started. The lead read one log line,
  `qualitySwitched qualityKbps=15635 linkEstimateKbps=5000` 50 ms after the start, and told the owner
  the quality chooser "jumped to the top quality" against a 5 Mb/s link, then briefed an agent to fix
  that jump. The agent found no switch: the core starts a device with no remembered link at the top
  rung (D260 §2), the note was hls.js announcing its first quality, the 5000 was hls.js's default
  rather than a measurement, and the stall came from the sound's buffer being thrown away when the
  Web Audio latency moved it earlier.
- **Caught by:** the agent, reading every chunk the device loaded rather than one note.
- **Gate:** none checks a reading of a log. The rule: a note named "switched" is checked against the
  chunks around it before it is called a switch; and the fix makes the log say it: a first
  quality carries no `fromQualityKbps`, an estimate is the controller's own or none, and a buffer
  thrown away is a step of its own.

### A46 — Blamed a browser for a path the server's own network chose (2026-10-27)

- **What happened:** the owner said staging worked in Brave on the Mac; the lead saw staging's log warn
  that the connection hairpinned through the router (D354) and told the owner Brave's privacy protections
  probably kept the page off the local network, so it fell back to the router. One look at the plays
  before it would have shown Chrome on the same Mac hairpinning too: the cause is staging itself, a core
  in the yoloAI sandbox behind Docker's NAT, which takes no UDP from the LAN, so every WebRTC path to it
  goes out to the router and back.
- **Caught by:** the owner, asking whether Chrome hairpinned too.
- **Gate:** none can check a guess's grounding. The rule: before naming a cause for a log line, compare
  it with the same line for another client or another server (digging-means-missing-logs: a play's
  start line now names its device, and the path line should too, so the comparison is one grep).

### A45 — Took revert-red's browser-mode "red" as coverage when the probe could not pass (2026-10-26)

- **What happened:** the night's check-pr flagged `tokens.css` and `app-shell.css` as untested, and the
  agent ran each alone through `make check-revert-red ONLY=…`, which said "a test fails when it is
  reverted, found by the browser-mode tests". It told the owner the files were covered and the gate's
  grouped probe was wrong. In fact every browser-mode test failed to import in revert-red's worktrees:
  Vite refuses anything under a `.git` directory by default, and the worktrees live in `.git/revert-red/`.
  So that probe came back red for any revert, or none. The false belief came from a probe that could
  not pass, read as a test that noticed.
- **Caught by:** the agent itself, the next morning, wiring the browser-mode tests into the grouped
  stylesheet probe: its run printed "Access denied" for every test file, and the same tests run unreverted
  in the worktree failed too.
- **Gate:** An earlier commit lets the browser-mode config serve the worktrees (keeping `.git`'s objects and
  top-level files refused). The agent then told the owner a whole-batch revert-red found every web file
  covered by a real test; that was false too: the unit-test config had the same deny, so every web
  unit-test probe was also red whatever it reverted. An earlier commit made a red count only if the same kind of
  run passes in that worktree with nothing reverted, which caught the unit probes (PROBE_BROKEN);
  An earlier commit gave the unit config the same deny list. With probes that can pass, check-pr found seven
  changes no test noticed, now tested or claimed (69103984).

### A44 — Told the owner a test checked the alphabet rail when the edit that added the checks never landed (2026-10-25)

- **What happened:** with an earlier commit the agent told the owner the browser test now checked that the
  alphabet rail starts under the banner and that every letter from "#" to "Z" sits on its
  background. The checks were never in the file: the agent's scripted string replacement looked for
  lines the formatter had already reflowed after an earlier edit, matched nothing, and wrote the
  file back unchanged without complaint. The test went green because it tested less than claimed,
  and the agent reported the run, not the file. The false belief came from a test that could not
  fail.
- **Caught by:** the agent itself, two rounds later, writing the next rail test and finding the
  first one's checks absent (an earlier commit adds them and was seen red without its fix).
- **Gate:** none. The lesson: a scripted edit asserts that what it replaces is present, and a test
  claimed to check something is seen red before the claim is made.

### A43 — Concluded a TV showed its picture late from clips whose period hid a whole-period lag (2026-10-24)

- **What happened:** to find why the owner's TCL TV in TV Bro played films out of sync, the agent
  made sync clips with a white flash and a beep **every second**. The owner heard the beep before
  the flash, and the agent concluded the TV showed its *picture* about half a second late, wrote it
  into docs/contributors/idiosyncrasies.md, and built a per-device sound *delay*
  (kept on the core in an earlier commit) that the owner tuned to 600 ms. The sound was in fact
  about half a second *late*: a beep 0.5 s late lands 0.5 s before the next flash, and every reading
  the agent took was only known modulo the clip's one-second period. The owner's first report, the
  films' lateness at a delay of 0, and a film made worse by the 600 ms were each explained away
  rather than taken as contradicting the conclusion. The false belief came from a measuring
  instrument whose ambiguity the agent built in and did not notice.
- **Caught by:** the owner, reporting that a film was further out of sync at 600 ms than at 0; a clip
  with a beep every **four** seconds then read the sound 0.5 s late unambiguously (an earlier commit turned the
  delay into a signed offset; the idiosyncrasies entry was corrected).
- **Gate:** none. The lesson: a periodic test signal measures lag only within half its period;
  make the period much longer than any lag expected, or make each mark distinct (the owner's
  design that followed: four colours, four rising tones, a counter and a filling clock), and treat
  a result that contradicts an earlier observation as a reason to doubt the instrument.

### A42 — Reported a gate failure its own change caused as a flake, from one passing run alone (2026-10-24)

- **What happened:** `make check` failed `TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled`
  while committing an earlier commit, which made slow work wait up to 30 s, later 5 s, for a disk's last
  reader. The test passed alone at both lengths, and the agent reported it to the owner as a flake
  under load, "not caused by tonight's changes". Under load it failed 6 times in 96 with the change,
  and never in 96 without it: the wait widened a race in the job queue, where an enqueue compared a
  request with only one of a target's running jobs and queued a duplicate full read-through (fixed
  in an earlier commit). The false belief came from a single passing run in an idle sandbox, taken as
  evidence about a failure that had needed the full gate's load.
- **Caught by:** the owner, asking for the flaky test to be investigated.
- **Gate:** none. The lesson: a failure first seen beside a change is the change's until the same
  load, with and without it, says otherwise; reproduce under load (several parallel runs beside a
  CPU burn) before calling anything a flake.

### A41 — Paired a second core with the owner's account, told it would sit beside the first (2026-10-23)

- **What happened:** to test D334's pairing end to end, the agent asked the owner for a key to pair
  the amd test box "as a second core in your staging realm", and paired it. The discovery service
  allows one server per account (D312 §15): the claim removed staging's own core from the account
  and gave the amd box the realm, so the owner could no longer sign in to realm staging as admin.
  The false belief came from D298, which describes a subrealm for each further core, read in place
  of the code that decides it (orpheum-connect `internal/cores`, its doc.go invariants), which says
  one.
- **Caught by:** the owner: "I can't log in to realm staging with admin/qqqqqqqq anymore".
- **Gate:** none. The lesson: before an action on a shared service that changes what the owner
  holds, read the owning package's invariants, not a decision about it, and say what the action
  will undo.

### A40 — Held back a design change as "Cut A's design is frozen" after Cut A had shipped (2026-10-21)

- **What happened:** asked to fix how slowly a library root came back after its share returned, the
  agent offered a small reordering and a status-driven bring-back, and said the second "changes how
  a folder comes back (D233 §10, D241). Cut A's design is frozen, so that's your call before I build
  anything", citing AGENTS.md rule 1. Cut A was done: the owner had watched with their partner that
  weekend, and the project was preparing a closed beta, whose focus is making things work properly.
  The false belief came from AGENTS.md, which still says the design is frozen, read without asking
  whether the milestone it names was still the current one.
- **Caught by:** the owner: "We're beyond cut A now."
- **Gate:** none. AGENTS.md rule 1 and its "Start here" still name Cut A as the build contract; the
  owner's to update. The lesson: a rule tied to a milestone is checked against the project's state
  before it is cited.

### A39 — Read the owner's own cable pulls in a NAS's log as a failing network card (2026-10-21)

- **What happened:** diagnosing an NFS share that hung during the dead-share tests, the agent read
  the production NAS's `dmesg`, a run of r8169 "Link is Down … Link is Up" lines, as the RTL8125's known link
  drops, called it "the cause", tied the day's earlier outages to it and recommended turning off EEE,
  ASPM or the in-kernel driver. The drops were the owner unplugging the cable for those same tests,
  earlier that afternoon. The false belief came from a log read without wall-clock times
  (`dmesg` gives seconds since boot) against the session's own history: the agent did not ask
  whether the drops lined up with the tests it knew had pulled the cable, and the newest drop was
  38 minutes before the hang it was explaining.
- **Caught by:** the owner, reading the diagnosis.
- **Gate:** none; it is a reading of a host's log, not the code. The lesson: map an event log to
  wall-clock time (`dmesg -T`) and to what the session itself did before naming a cause from it.

### A38 — Said Firefox fails a LAN request at once while its prompt waits; it holds it (2026-10-20)

- **What happened:** the agent recorded in idiosyncrasies.md, and built the local-network permission
  check on, the claim that while Firefox's Local Network Access prompt waits, each request to the LAN
  name fails at once. Firefox holds the request that raised the prompt until it goes, so the page's
  first race, which waited on it, kept the page on "Connecting to the server…" for as long as the
  prompt was up, and the line explaining the prompt, shown only in the app, never appeared. The false
  belief came from one console log read without timings: "prompt action: (null)" followed by a
  failure looked immediate.
- **Caught by:** the owner, testing the line on staging with the console open.
- **Gate:** `discovery-paths.test.ts`'s "while the browser asks, raises its prompt with the LAN name
  and never waits on the answer", whose LAN request never settles. The lesson: a claim about when a
  browser fails a request needs the timing seen, not inferred from the order of log lines.

### A37 — Put staging's client origins in the household zone that forbids every certificate (2026-10-19)

- **What happened:** to give staging's client origins one wildcard route, the agent recommended
  `<core>-client.orpheum-test.com`, staging's household zone, checked only that a CNAME could not sit
  beside a core's CAA record, and the owner set it up. Cloudflare's edge then refused every handshake:
  the zone's apex holds `CAA 0 issue ";"` and Universal SSL is kept off on purpose, so no CA but a
  core's own Let's Encrypt account issues there (orpheum-connect's spec §7, D292 §1, X21), and a
  Cloudflare-certified name in it would need both undone. The false belief came from reasoning about
  the zone's DNS shape without reading what the spec says the zone is for.
- **Caught by:** the agent, from the TLS alert on the owner's first try, after the owner had already
  made the tunnel route and the wildcard record.
- **Gate:** none checks a deployment recommendation against the spec. The lesson: before naming a
  zone, domain or route for a new use, grep the spec and the runbooks for every rule on that zone
  (`CAA`, `Universal SSL`, what it carries) and say which apply.

### A36 — Proposed warning the mailbox's owner through the mailbox an attacker took (2026-10-19)

- **What happened:** proposing password reset by e-mail for the service's accounts, the agent named
  the threat (whoever takes the mailbox can reset the password and enrol their own core) and as its
  safeguard proposed a pause after a reset in which *"the real owner gets an email and can cancel"*.
  The e-mail goes to the very address the reset came through, so the attacker who holds it reads the
  warning, and nothing is cancelled. The false belief came from listing safeguards by habit (notify
  the user) without tracing each one against the threat it was meant to stop: who receives it, and
  is that who the attacker is not?
- **Caught by:** the owner: *"Wouldn't that be sending the email to the address just used to reset
  the password?"*
- **Gate:** none can check a safeguard's reasoning. The lesson: for each mitigation, name the
  channel it relies on and confirm the attacker in that threat does not hold it; a recovery's
  warning goes through a credential the recovery did not touch (an existing session, a passkey, the
  enrolled core itself).

### A35 — Recorded "revoking a used link ends what it granted" as the owner's rule (2026-10-18)

- **What happened:** the owner described one pattern for invitations, Watch Together links and core
  keys: *"The code can be revoked, which eliminates the entitlements. The code can be regenerated,
  which is basically a revoke + new key."* The agent recorded D311 §3 as "revoking a link ends what
  it granted": the account an invitation made loses what it gave, the members a link let in leave.
  The owner's model was two operations apart: an outstanding code is revoked or regenerated; a
  redeemed one's entitlement is kept and moved, by a new code for it, to whoever redeems that (a
  locked-out person gets their own account back). The false belief came from reading
  "eliminates the entitlements" against the whole lifecycle instead of asking which stage it named,
  while the agent's own question about it ("does revoking a long-used invitation delete the account
  or only remove its access?") sat unanswered beside a decision already committed.
- **Caught by:** the owner: *"I think I may not have described this adequately."*
- **Gate:** none can check a decision's reading of the owner's words. The lesson: a decision whose
  meaning the agent is still asking about is not recorded until the answer comes; the question goes
  first, the record after.

### A34 — Read a clock-sync pong as a player saying it was in step (2026-10-18)

- **What happened:** reading the production NAS's log of a Watch Together session whose guest did not play at
  the start, the agent told the owner her player "said it was in step" for two minutes with no film
  loaded, from `a player's pong said where the group is … aheadOfExpectedMs=0 placing=placed`, and
  proposed a fix to stop a player claiming to be in step before it had a film. The pong says how
  far the group's timeline, as the device placed it by its clock, is from where the pong put it
  (D264): nothing of its video. The false belief came from the line's words, "where the group is"
  and "placed", read as the player's position, and from not opening `together-notes.ts`, which
  defines them, before building a finding on them.
- **Caught by:** the agent itself, reading the note's definition before building the fix, after the
  owner had the recommendation.
- **Gate:** none can check what a log line is taken to mean. The roster now says where each player
  stands (`TestHub_TheRosterSaysWhereEachPlayerStands`), so the next reading has the status in
  words instead of an inference.

### A33 — Read a player's automatic restart as the person restarting it (2026-10-14)

- **What happened:** reading staging's log of a relayed playback that stalled, the agent told the
  owner "you restarted it at about 12 s", and was about to put the stalls down to the phone's weak
  LTE alone. The log said `kind=abandoned` then a new `a playback started` 30 ms later: no person
  restarts a film in 30 ms. The player did it by itself, which is a behaviour to explain, not the
  owner's doing. The false belief came from the log's verbs: "abandoned" and "started" read as a
  person's acts, and the agent did not check the gap between them.
- **Caught by:** the owner: *"I didn't restart playback. Everything happened automatically after I
  pressed play."*
- **Gate:** none yet. The log does not say who ended a playback; the fix for the restart adds
  that, so the next reading cannot mistake it (the agent's own rule: digging means missing logs).

### A32 — Named the sandbox's VM after a retired machine (2026-10-14)

- **What happened:** explaining what a core in a container can see, the agent called
  `192.168.1.199` "devnas" and built its account on that: staging in the sandbox on the development NAS, the development NAS's
  LAN address two layers out. The development NAS runs nothing now; `192.168.1.199` is the VM holding the
  sandbox, as the owner had said earlier in the session. The false belief came from older notes and
  READMEs that name the development NAS as the development host, repeated without checking them against what the
  owner had said that day.
- **Caught by:** the owner: *"The development NAS is no longer running anything... All that runs now is
  192.168.1.199 (the VM containing your sandbox), and the production NAS."*
- **Gate:** none; machines are named in no file a gate reads. The agent's notes now list what runs
  where, and the lesson is A30's: what exists is checked against the owner's latest word.

### A31 — Repeated A29 a day later: the first admin made in a screen that does not exist (2026-10-14)

- **What happened:** with staging redeployed as a new instance, the agent gave the owner the setup
  code and said the sign-in page "should offer to set up the server". It has no such screen, as
  A29 recorded the day before; the owner found name, password and "Get a code" there, nothing that
  takes a setup code. The agent had A29's entry and the READMEs' corrected request available, and
  answered from what a setup flow usually looks like instead of either.
- **Caught by:** the owner: *"On the login page I see name + password, and a button 'Get a code',
  which produces a code rather than accepting one."*
- **Gate:** none, and that is the finding: A29's lesson was written down and not acted on, because
  its fix, a setup screen in the client with a browser-tier test that makes the first admin through
  it, was left as a suggestion. A failure whose fix is a missing feature recurs until the feature is
  built; the agent now proposes that screen to the owner rather than leaving it in this file.

### A30 — Called a live site's repository new and empty (2026-10-13)

- **What happened:** the 2026-10-13 handoff listed `orpheum-site` as "new, private, empty", and
  `orpheum-connect`'s infrastructure plan put the `orpheum-app.com` apex "when the homepage exists".
  The repository has held the landing page since 2026-10-06, and Cloudflare Pages has served it
  at `https://orpheum-app.com/` since. The false belief came from the plans, which describe the
  site as something to create (D292 §8); the agent repeated them without one request to the apex
  or one look at the repository.
- **Caught by:** the owner: *"there's already an orpheum-site repository with a simple landing
  page, which is live on https://orpheum-app.com/"*.
- **Gate:** none; no gate reads a handoff. The lesson: a handoff's statement of what exists is
  checked against the thing itself (a `git log`, a request to the name) before it is written.

### A29 — Told the owner to make the first admin in a page that has no such screen (2026-10-13)

- **What happened:** the NAS walkthrough said to open the web client and make the first admin with
  the setup code, as both deployment READMEs say. The web client has no setup screen: `POST /setup`
  is called by nothing in `web/`, so the owner, signed out with a code in hand, had nowhere to
  enter it, and guessed the D288 device codes had broken it. The false belief came from the
  READMEs, which the agent repeated without checking that the client calls the endpoint they
  describe; staging's first admin had been made long before, by other means.
- **Caught by:** the owner: *"I can't use the code to make an admin account."*
- **Gate:** none yet. The READMEs now give the request to make the admin with; a setup screen in
  the client, with a browser-tier test that makes the first admin through it, would make the
  instructions true and keep them so.

### A28 — Judged a device fix by a phone that never ran it (2026-10-11)

- **What happened:** an iPhone guest saw no player controls. The agent changed the guest's video
  layout, and when the owner said the controls were still missing, it told him the first fix "did
  not help" and changed the layout again. The phone's tab had joined each new session by a pasted
  link, which loads no new page, and a guest's page never reloaded for an update while on it: the
  iPhone very likely ran the morning's client all along. The false belief came from the deploy:
  the server ran the new build, so the agent took the phone to run it too, never checking which
  client the page ran.
- **Caught by:** the owner, seeing the host's new Talk button and the guest's old one: *"on the
  guest side it still looks the old way."*
- **Gate:** `ReloadWhenOutdated` now reloads a guest's page at once while no film shows on it
  (reload-when-outdated.test.tsx). Nothing yet says which client a device ran when it reported:
  a device's hello does not carry its script, so the log cannot answer.

### A27 — Blamed the owner's tool for a fault it had not traced (2026-10-08)

- **What happened:** twice in one day the sandbox's whole working tree turned up staged, with no
  staging command of the agent's to explain it. Having found nothing in the Makefile or `scripts/`
  that writes the real index, the agent told the owner the likely cause was a host-side
  `yoloai diff` run around then. It offered that as its best guess, with no evidence of any host
  command, and it was false: the owner had run none in hours. Labelling a guess as unconfirmed does
  not license offering it: an untraced fault is reported as untraced, with what was ruled out.
- **Caught by:** the owner: *"That's weird, because I'm not at a terminal and have not run any
  yoloai commands in hours."*
- **Gate:** none yet. The cause is still unknown. A watcher in the sandbox now logs the process
  table the moment the index gains staged entries, so the next occurrence names its writer.
  Unstaging and staging by path keeps commits right meanwhile.

### A26 — Committed with the gate failing, three times, in a clone with no git hooks (2026-10-08)

- **What happened:** Earlier commits were committed while `make check` failed, and
  An earlier commit did not build (three new files never staged). Each time the agent chained the commit
  after the gate with `;`, or did not stage what it built, and nothing stopped it: this sandbox clone
  had never run `make setup`, so `core.hooksPath` was unset and the pre-commit hook, which runs the
  gate on what a commit records, never ran. The Stop hook runs the gate only at the end of a turn,
  after the commits.
- **Caught by:** the M4 closing audit, the agent's own reading of its output (the other
  two), and the owner: *"If you have to remember to do the && properly, that's a process failure
  because you're nondeterministic. We need to fix the process"*.
- **Gate:** `.claude/hooks/pre_bash.py` refuses a `git commit` while `core.hooksPath` is not
  `.githooks`, naming `make setup`; with the hooks installed, the pre-commit hook gates every commit.

### A25 — Called a design choice a necessity, and weighed an option by it (2026-10-08)

- **What happened:** setting the decided way to cut a film with no keyframe index (D259 §3)
  against an alternative, I told the owner that a grid that grows "gets a new id as it grows",
  so each growth makes a new store, and counted that against the decided way. It is not
  necessary: the id is a hash of the grid's starts only because a grid was always whole
  (`Grid.ID`, `internal/core/chunks/streams.go`). Under D259 §1's local rule a start, once
  known, never changes as more is read, so a growing grid can keep one id. The claim came from
  reading what the code does as what it must do, at the moment of recommending against it.
- **Caught by:** the owner: "Why does the grid get a new id as it grows? Is that necessary?"
- **Gate:** none. The practice: a cost put to the owner against an option says whether it is
  inherent or only how the code is built now, and an option is not weighed by a cost a small
  change removes.

### A24 — Doubled check-pr's time without noticing, and quoted it as normal (2026-10-08)

- **What happened:** check-pr ran in 2.6–5.3 minutes through 2026-10-07. Two of the agent's commits
  each added time to its serial step. An earlier commit put the home acceptance in check-pr. An earlier commit
  (D263) then added a "voice alone" test that runs under every profile, so under home too, about
  3.2 minutes more. The serial step went from about 2.5 to 6 minutes, and check-pr from about 5 to
  10–14.5. The agent told the owner check-pr would take "about 25 minutes". That figure was never
  measured, and it was wrong too: the runs took 10–14.5 minutes. The agent treated the slowdown as
  normal. The false belief came from having no stated budget: nothing said how long check-pr should
  take, so the agent noticed no change.
- **Caught by:** the owner: "check-pr was supposed to be under 5 minutes. what happened?"
- **Gate:** a warning, not a failure. check-pr prints its elapsed time, and past its budget,
  the Makefile's `CHECK_PR_BUDGET_S` (300 s), says so and asks for the step that grew
  (standards/makefile.md). It still passes. That a step added to check-pr states what it adds is
  the standard's, review-only.

### A23 — Told the owner a tag would not travel, without checking the tool (2026-10-06)

- **What happened:** asked to tag the demo-safe commit so it would not be forgotten, I tagged it
  and said a tag made in the sandbox would not carry over with `yoloai apply`, so the owner should
  re-create it on their side. That is false: `yoloai apply --tags` carries it. The belief came
  from knowing that branches do not travel (AGENTS.md rule 14) and assuming tags were the same,
  never looking at what `yoloai apply` offers.
- **Caught by:** the owner: "point of order: tags do carry over if I use --tags".
- **Gate:** none. The practice: a claim about what a tool does, where it is not this project's
  code, is checked against the tool's own help before it is told to the owner, or said as a guess.

### A22 — A browser blamed for a codec it plays, and a fix shipped on the blame (2026-10-06)

- **What happened:** the owner's Mac Firefox loaded a copied HEVC picture and never played it. I
  concluded Firefox says yes to HEVC and cannot decode it, had the owner confirm only that it
  says yes, and shipped a change that gave Firefox no HEVC (a23361e2). The owner then ran the
  commit before it, which carried my gap fix, and HEVC played perfectly: the decoding
  was never the fault. The belief came from the one fact I asked for, the browser's yes, which
  fits both "it lies" and "it is fine"; and the earlier test that still spun had likely not
  included the gap fix, which I did not ask.
- **Caught by:** the owner: "I checked out the commit before the firefox fix and tried again, and
  it works perfectly playing hevc."
- **Gate:** none. The practice: before a fix that removes a capability, ask for the test that
  separates the hypotheses (the same film played in that browser on the exact build), and ask
  which commit the failing test ran.

### A21 — A research finding led with a cause the owner could check in one command (2026-10-05)

- **What happened:** the slow-links research note's first finding was that the owner's link "was
  probably a Tailscale relay", and I told the owner a direct path would probably help more than
  anything we built. The owner's `tailscale ping` answered directly. The belief came from one
  subagent's source, Tailscale's own 2.2 Mbit/s measurement of its relays, which fitted the
  stalls. I had already given the command that decides it, and committed the claim before the
  owner had run it.
- **Caught by:** the owner: "I didn't see via DERP when I ran tailscale ping".
- **Gate:** none. The practice: a cause that one command on the owner's machine can confirm is a
  question in the reply, not a finding in a committed document.

### A20 — A test device named from a guess, twice, and the diagnosis steered by it (2026-10-05)

- **What happened:** after the owner's first Watch Together test, I diagnosed the host's stuck
  player as a phone's player. I asked whether it was an iPhone's Safari, whose own player would
  have changed where to look. It was a MacBook running Firefox, tethered to the phone. Earlier
  in the same diagnosis I had named it the laptop. The owner had said only "the mobile
  network". I took the network for the device, and each guess read as knowledge because it
  fitted the log. The question that found the cause was the owner's own report: the film
  played alone too, stalling under a link too slow for it.
- **Caught by:** the owner: "I'm actually using my macbook running firefox as 'owner', which is
  tethered to my phone."
- **Gate:** none; it is a fact about the owner's desk. The practice: when a diagnosis turns on
  the device, browser or link, ask for all three in the first question, and assume none.

### A19 — A session's permission stated from a summary, not read from the environment (2026-10-03)

- **What happened:** before building D242, which changes the dev loop's gate files, I told the
  owner that I could not edit gates and recommended restarting the session with
  `ORPHEUM_ALLOW_GATE_EDIT=1`. It was already set. The false belief came from the project rules,
  which say gate edits are refused, and from my own earlier work in the session, none of which
  had needed one. One `env` command would have answered it.
- **Caught by:** the owner: "Isn't ORPHEUM_ALLOW_GATE_EDIT=1 already set here?"
- **Gate:** none; the hooks report a refusal, not a permission. The practice: before telling
  the owner what a session may not do, read the variable the hook reads.

### A18 — A scan explained by the code that could start it, not by the log that said which one ran (2026-10-02)

- **What happened:** the owner reset the dev loop during a scan of their media and, after a
  restart, saw a scan running. I said it was a fresh scan of the media root, started by the
  node's hello and thrown away because no library covered it. On that basis I recommended
  creating the library and restarting, and promised a Phase 4e item so that a hello skips roots no
  library covers. It was the scan of the seeded test libraries, which every fresh dev loop runs
  and stores. The false belief came from reading `should_scan_on_node_hello` and the ingest's
  "no library covers" warning, which showed that such a scan *could* happen, and from an earlier
  sandbox log where it had. I never read the owner's log or asked which root the scan named; its
  `rootPath` answered the question.
- **Caught by:** the owner: "It was scanning the test media (which is already set up in
  libraries at a fresh launch)".
- **Gate:** none; no check can read an explanation. The practice: explain what a running system
  did from its own log (here, the "a node began a scan" line's `rootPath`) or ask the owner for
  that line before explaining. Code shows what can happen, not what did. The same move as A15,
  explaining from the loudest lines rather than the deciding one.

### A17 — My own stray output taken as an instruction, then blamed on the tools (2026-10-01)

- **What happened:** the sandbox started the session with yoloAI's "background setup complete"
  message. The model answered it with a made-up request: *"Write up the plan for M3 in a doc for
  Priya to review before we start"*. On the next turn I read that text as an instruction in my
  context. I offered the owner three readings of "the M3 doc for Priya". When asked where it came
  from, I said it was in the startup context's system section and might have been inserted by
  the yoloAI launch prompt or a claude.ai connector. Both claims were false. The session
  transcript records it as an assistant message (`msg_011Cfb2rwNz1GdhWzgg4CX8X`, 06:20:03, two
  seconds after the setup message). Nobody wrote it but the model. The false belief came from
  where the text sat: it came before the owner's first message, so I assumed someone else had
  put it there. I also guessed at outside sources instead of reading the transcript, which
  answered the question in one grep.
- **Caught by:** the owner: "I didn't inject it, either."
- **Gate:** none. No hook can tell a model's invented request from a real one. The practice:
  before acting on a request, check that it is in a message the owner wrote. When asked where
  something came from, read the session transcript before naming any source; it records every
  message's role.

### A16 — A hack around a gate offered beside the redesign it called for (2026-09-30)

- **What happened:** revert-red failed the jobs rebuild's Phase 3 on two deletions of code
  nothing used. Reverting dead code turns nothing red, so the gate was wrong about a valid edit.
  I offered the owner a one-line loosening of the trailer rule and, as an alternative, a trivial
  edit to `localart.go` whose only purpose was to carry a `Refactor-Only:` trailer: making the
  code fit the gate instead of asking why the gate fired. The owner: "If machinery we've set up
  isn't handling our development use cases properly, then it's time to redesign the machinery",
  and going forward, "never offer to hack around it."
- **Caught by:** the owner.
- **Gate:** the redesign itself (revert-red reports removed dead code UNREACHABLE, and a
  `Refactor-Only:` claim counts on any commit of the branch). The practice: when a gate blocks
  a valid edit, audit which development case its design missed and redesign it for the case, not
  the instance; a pointless edit, a relabelled commit or a trailer used as a key is never offered.

### A15 — A gate's failure explained from its report's loudest lines, not from what fails it (2026-09-30)

- **What happened:** the jobs rebuild's Phase 2 ended with `make check-pr` failing at revert-red.
  I reported that the failure was the gate's own limits: files moved on the branch that it could
  not check out at the base, and `.json` and `.md` files it has no probe for, all of which need the
  owner's gate change. Those rows are UNCHECKED, which the gate lists and does not fail on; it
  fails only on GREEN and NO_RUNNER rows. The real failure was five files my commits changed that
  stayed GREEN when reverted: behaviour with no test that noticed (the System page's providers
  wiring, a job's priority reaching its handler, a prepare and an extraction parking on a missing
  root or an away node) and one tag's documentation. The belief came from the report's closing
  advice, printed after the UNCHECKED list, which I took for an explanation of the failure; I did
  not read the gate's own verdict or its code.
- **Caught by:** the coordinator, running revert-red over the branch.
- **Gate:** revert-red itself was right. The practice: when a gate fails, find the row or line
  that decides failure in its code or its summary before explaining it, and never report a gate
  "wrong" without naming the line of the gate that says so.

### A14 — Research framed from one symptom, so the field's standard concerns went unasked (2026-09-30)

- **What happened:** asked how queues handle work for a destination that is down, I briefed the
  research on that question alone. It answered it well (per-destination state, parking, one
  probe) and D231 was agreed on it. The owner then asked what happens when a node gets stuck, when
  a finished result never arrives, and across restarts, and the code's answers were bad: a hung
  FFmpeg holds its slot for ever, a lost result means redoing the work, and a core restart cancels
  every running encode. Delivery guarantees, leases, idempotent results and reconciliation are
  standard in the same literature (Celery, SQS, Temporal); the brief never asked for them. The
  owner: *"this should have come out of our research on queueing best practices. Is there
  anything else we've missed?"*
- **Caught by:** the owner's questions.
- **Practice:** a research brief first asks for the field's standard list of concerns, with
  sources, and has each marked covered, a gap, or not relevant to us; only then the question that
  prompted it. The second pass on coordinator and worker job systems ran that way. No gate can
  check the scope of a question; the audit checklist's item 16 asks it of every design question
  before a milestone.

### A13 — JSON written out by hand beside the struct that reads it, so a rename missed it (2026-09-29)

- **What happened:** `RefreshJob` built a fetch job's arguments with `fmt.Sprintf` and the JSON
  typed out, beside `refreshArgs`, the struct the job decodes them into; the string was there to
  avoid handling an error `json.Marshal` never returns for two booleans. D227 §6 renamed the
  struct's `retryImages` to `shouldRetryImages` with a type-aware rename, which cannot see a
  string, so every refresh read the flag as false and no refresh asked for missing images again.
  The artwork test that checked the recheck decoded the arguments with its own copy of the struct,
  which kept the old name too, so it agreed with the bug.
- **Caught by:** the identify group's rename agent, reading the code by hand; no test failed.
- **Gate:** `TestArch_NoHandBuiltJSON` fails on any string in shipped code that spells out a JSON
  object's keys, and `RefreshJob` encodes `refreshArgs` itself; `TestRefreshJob_ItsArgumentsReadBackAsTheJobReadsThem`
  decodes with the struct the job reads. The lesson for tests: decode with the production struct,
  not a copy of it.

### A12 — A contract rename called done because the client compiled and its tests passed (2026-09-29)

- **What happened:** I committed D227's API renames as complete with `make check` and
  `make check-pr` green, and four old names were still live in the web client: the sound
  switches sent `isDialogueBoost` and `isLevelling`, which the server now refuses; a browser
  signing in again sent `resumeId`, refused too; the Files screen asked with `?library=`, which
  the server ignores, so it listed every library's files; and the browser tier's config said
  `confinement`, which the server ignores. The belief came from the compiler and the tests. The
  compiler checks no excess property on a body built with a computed key or held in a variable,
  and the stand-in server accepted any body and dropped the query string, so the tests asserted
  the old names on both sides.
- **Caught by:** a second audit the owner asked for, whose reviewer compared each API call
  against `api/openapi.json`.
- **Gate:** the four fixes take their names from the API's own types (`satisfies` and the
  generated schemas), so a rename fails to compile. The stand-in server records the query, and
  a Files test checks `libraryId`. The browser tier fails when its server says a config key is
  read by nothing. `jobs.test.tsx` makes every job kind have words. Still open: the stand-in
  server does not check a body against the schema.

### A11 — A screen's words written from a decision's prose, not from what the code produces (2026-09-27)

- **What happened:** building D222, I wrote the receiver's hint as *"Surround sound goes to it as
  it is; the dialogue boost is not offered"*, and VQ8 said a film on a receiver *"plays as it is,
  with no switch"*. Both hold only when the browser can play the original. A file prepared for
  another reason (an MKV, TrueHD) is a copy whose sound is stereo in M3, standard and boosted,
  and the switch is there and works. The words came from D222 §2's sentence, and I never walked
  the prepare path's output for a receiver.
- **Caught by:** the owner, on the phone: *"when I select sound bar, I still see the dialogue
  boost button, and it boosts the channel"*.
- **Gate:** none can compare prose with behaviour. The practice is A9's: read each clause back
  against the code, including each case the decision's table of outcomes reaches (here: a
  receiver × a file prepared for its container or codec), before writing what the screen says.

### A10 — A gate's pass taken as evidence without once watching it fail (2026-09-27)

- **What happened:** every web `feat` and `fix` this milestone passed `make check-pr`, and
  revert-red listed each TypeScript file as *"a test fails when it is reverted"*. It had run no
  test. Its git worktree has no `node_modules`, `npm run test` stopped at "vitest: not found",
  and the gate read that exit as red. I took the report as coverage, and the commits went in on
  it. The false belief came from the report's wording, which is the same for a real failure and
  a runner that never started.
- **Caught by:** the agent, not the owner. A commit that changed only click handlers in screens
  that no test renders was reported covered. Reverting one by hand left the suite green.
- **Gate:** revert-red now links the installed packages into its worktree, and fails a run that
  cannot start (`NO_RUNNER`); `test_a_suite_that_could_not_start_fails_rather_than_passing_as_red`.
  The general lesson is yoloAI's: execution catches errors, so watch a new gate go red on a
  change it should catch before trusting its green.

### A9 — A decision reported built with one of its clauses left out (2026-09-26)

- **What happened:** I built D218's accounts (registration links, device codes, password change)
  and told the owner *"Accounts by one-time link (D218) are built"*. D218 §4 also says the added
  device is *"listed and revocable like any other"*, and nothing listed or revoked a device. The
  summary came from the list of endpoints I had planned, not from D218's clauses read back
  against what was built.
- **Caught by:** the owner, testing on a phone: *"Should it be showing me a list of devices I
  have associated with my account so that I can revoke it?"*
- **Gate:** none can read a decision's prose. The practice: before calling a decision built,
  walk its numbered clauses one by one against the code and tests, and name any that are not
  built. Proposed for the audit checklist's "At every commit" section when a commit claims to
  finish a decision.

### A8 — Names that made the reader dig, against a standard that already forbade them (2026-09-26)

- **What happened:** building prepare, I named the segment length's setting `hls_segment_s`,
  its constant `HLSSegmentS`, and the variable holding it `segment`; beside them `done_ms` (done
  of what?), `run` (a run of what?), `Dir` and `Silence`. standards/go.md §3 says a name says
  what the thing is, so a reader at a use site needs no trip to the declaration; it is
  review-only, and nothing reviewed it before the owner read the code. Renamed in an earlier commit.
- **Caught by:** the owner, reading the code: *"its actual function is the segment length,
  which I'd never have guessed without digging around a lot."*
- **Gate:** none can be built: the rule is about meaning, and even `hls_segment_s` carried its
  unit; the missing word was "length". The owner: *"the only way to solve this will be to do
  periodic audits to catch them"*. So a naming pass joins every between-milestone audit, a
  codebase-wide one ran at once, and before committing a slice the agent re-reads each name it
  introduced as a reader at the use site would. The codebase-wide audit found 47 unclear names
  and 99 minor ones; those its summary named were renamed in an earlier commit to an earlier commit, but its
  per-item lists were lost when the machine failed, so the next milestone audit runs the naming
  pass over the whole tree again. The owner then made the checks policy at every commit as well
  as every milestone (AGENTS.md rule 17; audit-checklist.md, "At every commit").

### A7 — A cause asserted from a plausible mechanism, and a fix built on it, without a measurement (2026-09-26)

- **What happened:** the owner reported that the dialogue boost took two to three seconds to be
  heard. I said the cause was six-second segments, recommended two-second ones as making the
  switch "about a second", and the owner chose that; a setting to vary the length followed. The
  owner then heard no change: still two to four seconds. The cause was elsewhere. The browser
  plays out the audio it has already decoded ahead, whatever the segment length, and a seek in
  place after the switch fixed it (VQ2). Nothing was lost but a round trip, and the
  shorter segments and the setting are still useful; but a recommendation was offered as a
  prediction it could not back.
- **Caught by:** the owner, listening.
- **Gate:** none; the lesson is the agent's. When a symptom's cause is inferred, say so, and
  read the component that owns the behaviour (here hls.js's switch code, which said plainly it
  does not flush the decoder) before recommending a fix, or offer the fix as a test of the
  hypothesis rather than as the answer.

### A6 — A decided rule left unbuilt behind a comment naming code that never existed (2026-09-25)

- **What happened:** D162's second evidence of an extra (`Film (1999)-trailer.mkv` beside its
  film) and D163's `extra` filing were decided and in the plan, and neither was built: a comment
  in `extras.go` said the suffix case was "settled once the folder's film is known
  (extraBySuffix)", and no `extraBySuffix` existed. The comment read as a pointer to working code,
  so nobody looked; such a file joined the film's title as another version, and every extra was
  filed `listed`. The same shape as A3: a rule built without reading every decision that governs
  it.
- **Caught by:** the M2 acceptance test's 200-file fixture tree, on its first run, which checked
  that a trailer or featurette of a film is filed `extra`. Fixed in an earlier commit.
- **Gate:** the acceptance fixture, now. A check that identifiers named in comments exist was
  proposed and weighed against its noise (audit-2026-09-25 §2.1).

### A5 — A failure policy for another host's answers decided in code (2026-09-25)

- **What happened:** the artwork job recorded an image TMDB's CDN answered 404 for as missing,
  for good, and the summary to the owner said so as settled: "a refusal or a missing image is
  recorded with no hash, so it is not fetched again". A 404 there usually means our stored record
  is stale — TMDB replaced the image — and can be a CDN fault; giving up left a blank forever. The
  belief came from treating a 404 as the answer about the image, not about our copy of the record.
- **Caught by:** the owner: *"If TMDB no longer has an image, shouldn't that be a trigger for us
  to schedule another TMDB scan for that title? … They could also be experiencing technical
  difficulties."* The research that followed (provider-etiquette.md) found TMDB's six-month limit
  on keeping its data too, and D213 settled the policy.
- **Gate:** none can decide policy. Proposed: standards name the failure classes of every call to
  another host, each needing a stated policy before it is built (audit-2026-09-25 §2.2).

### A4 — A derived value's refresh tied to the step that happened to write it (2026-09-25)

- **What happened:** sort keys (D47) were written by the identification pass beside each name,
  so a change of the `sort_locale` setting was described to the owner, and in the setting's own
  description, as "a scan applies a change". But a sort key is a pure function of a stored name
  and the locale: the core can rebuild every key the moment the locale changes, with no scan
  and no node. The false belief came from the code's shape — keys were written where names were
  written, so their refresh looked like the pass's job — rather than from what the key depends
  on.
- **Caught by:** the owner asked: *"I don't understand why a sort_locale change has to wait for a
  scan in order to take effect? The node is only giving us its raw, byte observation. It's the
  core that decides what they mean."*
- **Gate:** none can see it. The question that would have caught it, asked of every derived
  value: *what are its inputs, and does every input's change refresh it?* The between-milestone
  audit now asks it of each derived table.

### A3 — Removal logic built without reading the decisions that make removal safe (2026-09-24)

- **What happened:** the scanner and the ingest (an earlier commit, 2563802) were built from D39
  and D40 — completeness, scopes, tombstones only after a complete scan — and presented as
  removal-safe. D41 and D42, the next two decisions, say the most dangerous case has **no error
  at all**: an unmounted disk leaves an empty directory that lists cleanly, so a complete scan of
  it tombstones the whole library. They require the node to report each root's device id and
  mount facts and to tell "listed and empty" from "not enumerated", and the core to withhold
  removals on implausible shrinkage and ask. None of it was built. The false belief came from
  reading the decision the code cited (D40) and not the ones around it.
- **Caught by:** the owner's request for user-story scenarios; walking "a USB disk is unplugged"
  through the design found D41's section on the silent case.
- **Gate:** `TestScenario_S7_S8_AnUnpluggedDiskRemovesNothingAndItsReturnRestoresEverything`
  fails if a clean scan of an emptied root removes anything (plans/scenarios.md, S7). General lesson: before building
  a mechanism, grep the decision log for every decision that names it, not only the one cited.

### A2 — Two rules that contradicted each other, built side by side without a word (2026-09-24)

- **What happened:** D200 said a missing setting is an error, never a fallback, and in the same
  change the server seeded every missing setting at start, so a later release could add one. The
  agent built both, plus `orpheum seed-settings` to paper over the gap, and presented them as one
  coherent model. The owner saw that "balk at a missing value" and "seed any missing value"
  cannot both be the rule (D202). The false belief came from building each requirement as it
  arrived, and checking each against its own tests, never against the others.
- **Caught by:** the owner, reading the summary.
- **Gate:** none can catch a contradiction between requirements. The between-milestone audit is
  the check, and a decision entry should now list the earlier rules it touches and say how each
  still holds.

### A1 — "Modified" computed from the value, so a choice looked like no choice (2026-09-24)

- **What happened:** the settings model marked a setting *modified* when its value differed from
  its starting value (D200). A person who set a value equal to the default became
  indistinguishable from nobody, so a release that changed the default would have overwritten a
  deliberate choice. The owner's question — *"does that still get set if the user modified the
  value back to its default?"* — had the honest answer "no". The belief came from about:config,
  whose computed "modified" the agent copied without asking what upgrades need of it.
- **Caught by:** the owner's question.
- **Gate:** `TestSet_MarksModifiedEvenAtTheStartingValueAndResetClearsIt` and
  `TestSeed_MovesOnlyUnmodifiedSettingsToAChangedStartingValue` now pin the corrected behaviour
  (D201). Nothing general: the lesson is to test a state model against the transitions a later
  release makes, not only the ones today's code makes.
