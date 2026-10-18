> **ABOUTME:** What went differently from the plan, milestone by milestone: what was planned,
> what happened, why the plan missed it, and what that says about how projects are planned. Kept
> for the owner's process across projects, not only this one.

# Plan deviations

The plan for Cut A was frozen (D193) and still met reality. This log records each place where it
did — not to grade the plan, but to see which *kinds* of thing plans miss, so the next project's
plan can look for them. [agent-failures.md](agent-failures.md) records the agent's mistakes; the
between-milestone audits ([the first](design/research/audit-2026-09-24.md)) record the repairs.
This records the gap between intent and outcome, whoever's it was.

## When to write an entry

At every between-milestone audit, and whenever a decision entry exists because the plan was
silent, wrong or contradicted itself. One entry per deviation, not per fix.

## The entry format

Newest first. IDs are `P<n>`, never reused. **Kind** is one of a closed set, so patterns can be
counted:

| Kind | Meaning |
| --- | --- |
| `environment` | the machines, network or hosting differed from what the plan assumed |
| `tool-behaviour` | a tool or library did something its documentation or reputation did not suggest |
| `plan-gap` | the plan was silent, contradicted itself, or named something out of scope |
| `gate-design` | a check, hook or rule written before the code it governs misjudged that code |
| `requirement-emerged` | a preference or requirement surfaced only once something concrete existed |

```markdown
### P<n> — <what differed, in a phrase> (M<k>, <YYYY-MM-DD>) · <kind>

- **Planned:** what the plan said or assumed.
- **What happened:** what was true, and how it showed up.
- **Why the plan missed it:** the honest reason.
- **Cost:** what it took: decisions, patches, rework, time.
- **Process lesson:** what a future plan, on any project, should do differently.
```

## Log

### P60 — Cut A's next milestones were replaced by the closed beta's, after a stock-take (after M4, 2026-10-18) · plan-gap

- **Planned:** cut-a.md §8's M5, the polished UI (D245), then M6, the remote TV, as what comes
  after M4.
- **What happened:** by 2026-10-18 M0–M4 were built, and so was a track no milestone named (P57).
  The owner's stock-take
  ([milestone-reassessment-2026-10-18.md](design/research/milestone-reassessment-2026-10-18.md))
  found the plan documents behind the code (the roadmap, seven plans' Status lines, m5.md's
  contradictions, this log), and found that the discovery run then matched neither outcome the
  owner wanted: a self-run discovery, and the paid service with accounts. D310–D312 brought the
  paid service's accounts into the closed beta, and D313 replaced M5 and M6 with the closed beta's
  milestones, B0 to B5, the UI revamp among them as B4.
- **Why the plan missed it:** the build contract was frozen (D193 §8) and had no place for work
  the owner chose outside it; each decision was recorded, but no one changed the milestone list
  with it, so its order went on describing work no longer next.
- **Cost:** a day's stock-take and a feature-by-feature comparison with the field; four decisions;
  one pass over the roadmap, cut-a and seven plans.
- **Process lesson:** when the owner moves work outside the milestones, revise the milestone list
  in the same change as the decision, and read each touched plan's Status line at every
  deployment, not only at audits.

### P59 — The polished UI was deferred twice, until the features it must hold are in (M5, 2026-10-18) · requirement-emerged

- **Planned:** Cut A ends with one polished, skinned web UI built on research (M5, D245), the
  remote TV right after it.
- **What happened:** on 2026-10-09 the owner judged M5's picture not clear enough to build and
  put the closed beta first; M5's research and inventory were done, and D271 answered its first
  questions, but no phase was planned. On 2026-10-18 the owner deferred the revamp until the
  features are in (*"doing the ui revamp would risk handcuffing us"*), and D313 §5 made it B4,
  after B0–B3. D251's first tier of the skin stays meanwhile.
- **Why the plan missed it:** M5 was placed before the features it must hold were settled; a
  revamp then would fix a layout around screens that are not there yet.
- **Cost:** M5's research waits, done ahead; m5.md and cut-a stayed out of step with the order of
  work for nine days.
- **Process lesson:** schedule a UI revamp after the feature set it must hold, and say in the
  plan what it waits on.

### P58 — The data layer was rebuilt once a real library ran on it (after M4, 2026-10-15) · plan-gap

- **Planned:** SQLite behind `internal/store` (D31), with queries written as each feature needed
  them, and the first scan ordered so a library is browsable early (D118).
- **What happened:** on nas.example, requests waited 3–47 s behind an identification pass that held
  SQLite's one writer for 25–30 s. An access audit, a practice survey and an independent DBA's
  audit led to D303: written principles (short writes that re-check what they decided, a revision
  on rows people edit), a fresh baseline at 0048 with every table STRICT, committed statistics and
  hourly backups. D304 then filed a library in stages by weight, and D305 settled deletion and
  backup thinning.
- **Why the plan missed it:** the scan's speed was measured, but not what a long write does to the
  requests beside it; the development library was too small to show it, and the data layer had no
  written principles to review against.
- **Cost:** three research documents, three decisions, a fresh baseline and the rewrite of the
  store's writers, over about two days.
- **Process lesson:** write the data layer's principles before its first query, and test early
  against a library of the size the first household has, with requests running beside the
  background work.

### P57 — A closed beta's discovery, transport and loader were built outside Cut A's milestones (after M4, 2026-10-09) · requirement-emerged

- **Planned:** M5, the polished UI, after M4; a standard tunnel to a VPS as Cut A's route in
  (D193 §4, D243 §7); the paid tier and its relay after the release.
- **What happened:** on 2026-10-09 the owner set a closed beta within about two weeks: a household
  reaching its core through a discovery, the owner's paid service at app.orpheum-app.com among
  them. D273 and D274 replaced the tunnel with discovery and WebRTC first; D275–D309 followed:
  the core's side of the protocol, the public `discovery/` packages, the WebRTC transport, device
  codes, realms (D297, D298) and the loader with each core's own client (D308), beside release
  mode on nas.example (D272). nas.example has been reached through the production service since 2026-10-15. M4's
  last acceptance run (VQ24) was left open meanwhile.
- **Why the plan missed it:** Cut A was for two people, and a beta for other households was not
  in its scope; the owner's priority changed once Cut A's core worked.
- **Cost:** thirty-seven decisions (D273–D309), five plans and a second repository (`orpheum-connect`); M4 not
  formally closed; the roadmap and plan Status lines left behind until P60's pass.
- **Process lesson:** a new track gets a milestone in the build contract the day it starts, even
  a provisional one, so status, order and acceptance have somewhere to live.

### P56 — The gate checked the tree, not the commit, and this clone had no git hooks (M4, 2026-10-08) · gate-design

- **Planned:** the pre-commit hook runs the gate on what a commit records; the Stop hook runs it
  at the end of each turn.
- **What happened:** this sandbox clone never ran `make setup`, so no git hook ran. Three commits
  went in with the gate failing or the build broken (agent-failures A26).
- **Why the plan missed it:** the hooks were installed by a one-time step, and nothing checked
  they were there.
- **Cost:** three fix-forward commits, found by reading output and by the audit.
- **Process lesson:** a gate that depends on a setup step checks that the step was taken, before
  the work it guards (the shell hook now refuses a commit without the hooks).

### P55 — Sync was built on one-shot marks and a placing that never aged, and an outside review found it (M4, 2026-10-08) · plan-gap

- **Planned:** D246 and D250's follower, tuned by the owner's tests.
- **What happened:** a week of fixes to the follower, then the sync review against Jellyfin,
  Syncplay, MediaSync and NTP found an allowance tighter than any shipped, a group placing that
  never aged, and intent read from events; D268 changed all three.
- **Why the plan missed it:** the design was not set against the field before it was built.
- **Cost:** a week of patches, each fixing the last.
- **Process lesson:** review a sync or timing design against the field's practice (A14) before
  building it, and again after a second patch to the same mechanism.

### P54 — hls.js's quality choice was patched four times, then replaced (M4, 2026-10-07) · tool-behaviour

- **Planned:** hls.js chooses the quality (chunks.md §4).
- **What happened:** four patches to its controller, then D265's controller of our own.
- **Why the plan missed it:** the library's controller assumed a link and a server unlike ours,
  chunks made on request.
- **Cost:** the four patches.
- **Process lesson:** after the second patch fighting a library's policy, replace the policy
  rather than patch it again.

### P53 — A device said it plays a codec it could not play from that point (M4, 2026-10-07) · tool-behaviour

- **Planned:** the capability probe decides what a player is sent (D217).
- **What happened:** D266: a quality whose video stood still while its chunks arrived was one the
  device could not play.
- **Why the plan missed it:** a capability query was taken for behaviour.
- **Process lesson:** confirm a capability by behaviour, not by the query.

### P52 — The acceptance's network profiles were guesses (M4, 2026-10-07) · plan-gap

- **Planned:** D252's three netem profiles.
- **What happened:** D263 rewrote them from research into four, and BBR was needed on the server.
- **Process lesson:** take network profiles from published measurements, not intuition.

### P51 — Two decisions hours apart contradicted each other on congested voice (M4, 2026-10-07) · plan-gap

- **Planned:** D261 §7 made voice on the congested profile best effort.
- **What happened:** D263 §2 held it for every pair, naming neither D261 §7 nor its revision.
- **Why the plan missed it:** P50's lesson again: a decision changing a bound did not name the
  one it revised.
- **Process lesson:** before recording a decision that sets a bound, search for the decisions
  that set it before, and name each one revised.

### P50 — A tuning number and an acceptance bound were decided days apart, and contradicted (M4, 2026-10-06) · plan-gap

- **Planned:** D243 §9 bounds two clients to 200 ms apart 95% of the time; D250 §1 then set the
  drift a follower leaves alone to 500 ms, the owner's comfort after testing.
- **What happened:** the first netem run left a guest 430 ms off for good, in step by its own
  rules and out of the bound. Both decisions stood; neither named the other.
- **Why the plan missed it:** the bound was written as an acceptance number, the allowance as a
  setting; nothing tied a setting's starting value to the acceptance it must meet.
- **Cost:** a failed acceptance and an owner question (M4's audit, Q1).
- **Process lesson:** when a setting governs a measured bound, write the bound beside the
  setting's description, and let a decision that changes either cite the other.

### P49 — The netem acceptance was planned for a VM and ran first in a container that could not shape its network (M4, 2026-10-06) · environment

- **Planned:** cut-a's M4 row: two clients on D190's remote-household VM under its netem
  profiles; m4.md: the profiles "run in the sandbox's containers for the gate".
- **What happened:** the development container had no `tc` and no right to shape its network.
  D252: the kernel's netem on the container's loopback, one profile per run, in `make
  test-acceptance`, with the VM as VQ24. Its first runs found a follower defect (the message's
  transit) and P50.
- **Why the plan missed it:** "the sandbox's containers" was written without checking what the
  container could do.
- **Cost:** a decision, a development image change, a launcher change.
- **Process lesson:** for every test environment a plan names, check before the milestone starts
  that it can do what the test needs (a capability, a device, a network).

### P48 — Watch Together was rebuilt mid-milestone after the owner used it (M4, 2026-10-05) · requirement-emerged

- **Planned:** D243 §1–§3 and D121's abilities: a shared link with a cap and rotation, guests
  with a per-film cookie, abilities chosen by the host.
- **What happened:** D246 replaced them: one link per person with a note, the host alone in
  control, an ephemeral viewer; D250 then reworked sync points, catch-up and stalls after the
  owner's tests on phones and an airplane's network.
- **Why the plan missed it:** the design came from other products' Watch Together, not from the
  two people who would use it; the first hour of real use changed it.
- **Cost:** Phase E built twice, two decisions, and design documents that lag (M4's audit §4).
- **Process lesson:** put a rough version in the owner's hands before the design is frozen in
  detail, for any feature whose value is how it feels between people.

### P47 — A quality ladder Cut A ruled out arrived because real links were slower than the film (M4, 2026-10-05) · requirement-emerged

- **Planned:** Cut A: no adaptive ladder (D86, D193 §2); a viewer plays the original or one
  prepared copy.
- **What happened:** the owner's tests on a phone's network stalled; D247 and D250 added chunked
  runs at lower qualities, a step down after a stall and a step up when the link allows, and
  capacity eviction on the node.
- **Why the plan missed it:** remote playback was planned at home bandwidths; the remote viewer's
  link was the unknown, and nobody measured one before deciding.
- **Cost:** two decisions, new packages on both sides, three protocol bumps and a migration.
- **Process lesson:** measure the slowest real link a feature must serve before ruling out
  machinery for it.

### P46 — A signed-in guest's history is in M4's acceptance, and its design was deferred (M4, 2026-10-06) · plan-gap

- **Planned:** cut-a's M4 row: "a signed-in guest's history advances".
- **What happened:** D246 §4 deferred recording a signed-in recipient's viewing ("later"), and M4
  ends with the acceptance unmet (M4's audit, Q3).
- **Why the plan missed it:** a later decision changed what a guest is, and the acceptance row was
  not revisited with it.
- **Cost:** an acceptance item open at the milestone's end.
- **Process lesson:** a decision that defers something names every acceptance line it touches,
  and moves or drops each one there and then.

### P45 — The skin was pulled forward from M5 for a conference (M4, 2026-10-06) · requirement-emerged

- **Planned:** D245: the polished skin is M5's, after the research and the UI principles.
- **What happened:** the owner's week of demonstrations brought D251: a dark skin, coral accent,
  the logo, the menu split, in a day, ahead of M5's restructuring.
- **Why the plan missed it:** a demo date is an external event no milestone plan carried.
- **Cost:** a day; M5 now starts from a skin rather than none.
- **Process lesson:** keep a "presentable in a day" path (tokens and plain-element styles) cheap
  at all times, so a demo needs no milestone reordering.

### P44 — Decision and scenario statuses drifted from the code, because only doc.go citations are checked (before M4, 2026-10-03) · gate-design

- **Planned:** each decision's **Status** line and each scenario's status say what is built, and
  the audit checklist's item 19 reads them back at every milestone.
- **What happened:** the pre-M4 audit found eight decisions' Status lines stale (D234, D237, D238,
  D239, D240 and D241 still "decided" or "to be built" though built; D233 claiming a clause that
  is not), scenario statuses still naming gaps closed weeks before, and a scenario-tier list
  naming tests never written. `check_claims` checks test names cited in doc.go and README files,
  so every citation there was true; the same claims in working-notes.md and scenarios.md had no
  check.
- **Why the plan missed it:** the gate was built where the first stale claims were found, and
  the claims spread to documents it does not read.
- **Cost:** an audit section; a batch of documentation fixes.
- **Process lesson:** a check on claims covers every document that makes them, or the documents
  it does not cover rot first; and a status line updated by the commit that builds the thing
  beats one an audit corrects.

### P43 — One package collected five jobs before the plan's layout gave them homes (jobs rebuild, 2026-10-03) · plan-gap

- **Planned:** cut-a §3's layout names one `playback` package for playing a file.
- **What happened:** playback collected play state, prepared copies, subtitles (extraction,
  conversion, fetching on request), loudness and the levelling arithmetic, half of its doc.go
  invariants about node work. The architecture map flagged it (F2); the split waited on the
  owner because it changed the frozen layout, and was made after Phase 6 into `loudness`,
  `subtitles`, `renditions` and `decide/levels.go`.
- **Why the plan missed it:** the layout was written from the user's verbs (play a file), not
  from the kinds of work behind them; each feature found the nearest package.
- **Cost:** five refactor commits and a docs pass.
- **Process lesson:** derive a layout from the kinds of work and their owners (jobs, stores,
  wire), and let a package's doc.go saying "and" a second time be the trigger to split, before a
  milestone builds on it.

### P42 — Where an instance's files live was the deployment's guess, until someone had to back it up (2026-10-03) · requirement-emerged

- **Planned:** cut-a §2: `/var/lib/orpheum` for the state, `/var/cache/orpheum` for the cache,
  `/run/orpheum` for the sockets, each a config key; a backup is persistent.db by VACUUM INTO.
- **What happened:** the owner, looking at the development directory, could not tell what to
  back up. D242 made an instance one directory with a top-level config pointing at persistent,
  cache, backups and run, made a backup a tarball of persistent, gave the node an instance of its
  own, and had its hello say where its byte socket is.
- **Why the plan missed it:** the layout followed Linux's filesystem conventions, decided for a
  packager, and was never looked at from the operator's chair: "what do I copy?".
- **Cost:** a decision, a protocol bump, a dev-loop rewrite with a gate trailer, and a
  regression (the pre-migration snapshot no longer restorable) found by the next audit.
- **Process lesson:** for anything a person must back up or move, design the operator's view
  first ("this directory is the instance; this one is what you keep") and let the distribution
  map it onto conventions.

### P41 — What a play does when its media is gone was unplanned (jobs rebuild, 2026-10-03) · plan-gap

- **Planned:** D231 parked work on a missing root; nothing said what a person who asks to play a
  file there sees, or what happens to a film already playing.
- **What happened:** the owner's questions about nodes going away produced D241: unavailable
  titles stay listed and marked, a play of one fails at once and queues nothing, an admin says
  "gone" per file or per root and can repoint a root, and a play that loses its media says why.
- **Why the plan missed it:** the jobs design looked at the work, not at the person waiting for
  it.
- **Cost:** a decision and Phase 6.
- **Process lesson:** for every way a resource can go away, write what the person using it sees,
  not only what the background work does.

### P40 — A prepare's film shows a growing length, and a seek cannot reach past what is made (after the jobs rebuild's Phase 3, 2026-10-01) · plan-gap

- **Planned:** D193 §2: prepare writes a growing `EVENT` playlist so playback starts at the
  first segments, and "a seek past the prepared point waits". On-demand segment-addressed
  transcoding, D4's path (b), stays unbuilt until a measurement says first-play latency hurts.
- **What happened:** the owner played an XviD film on their phone. The player, the browser's own
  controls, showed the length made so far (1:54, then 3:58, growing as it played), and its bar
  ended there, so there was no seek past the prepared point to wait for. In the sandbox the x264
  encode barely outruns playback.
- **Why the plan missed it:** the decision described the server's side of a seek and never what
  the player shows of a playlist that is still growing; nothing in cut-a or web-client.md says
  what a person sees while a prepare runs.
- **Cost:** none yet. The owner deferred it (2026-10-01). What they want, recorded as F96 and
  F97 in [FEATURES.md](../FEATURES.md): the whole length from the start; a seek that stops the
  run and starts again where it lands, keeping every segment made and making nothing before the
  play point, as D4 (b) describes; a scrub bar that previews frames; controls sized for touch.
- **Process lesson:** when a design streams something still being made, say what the person's
  player shows of it (length, bar, seeking), not only what the server does.

### P39 — "The weekly reminder" assumed a way to tell the admin that does not exist (jobs rebuild Phase 3, 2026-09-30) · plan-gap

- **Planned:** D231 §7 and the jobs rebuild's Phase 3: offline storage never ends without the
  admin, "the System page shows 'offline since', and the admin is reminded after a week".
- **What happened:** the code has no channel to the admin outside the pages they open: no
  notification, message, e-mail or reminder mechanism. "Offline since" was built; the reminder was
  not, since building it means choosing how an admin is told, a design decision.
- **Why the plan missed it:** the decision named an outcome ("is reminded") without the means, and
  the walk of scenarios S22–S24 took the means for granted.
- **Cost:** the reminder and its interval setting waited for the owner.
- **Resolved** (2026-09-30): the owner chose a banner. An admin sees on every page how many library
  folders and disks are offline since when, leading to the System page; dismissing it hides it from
  them for `offline_reminder_interval_days` (starting at 7), kept as their preference, and it comes
  back after that while anything is still offline (jobs-rebuild.md, Phase 3).
- **Process lesson:** when a decision says someone "is told" or "is reminded", check that a channel
  to them exists, and name it in the decision if it does not.

### P38 — Failure classes were decided for one host and one job, and the others inherited none (M3, 2026-09-29) · plan-gap

- **Planned:** standards/go.md §21 asks every call to another host to name its policy for a 404,
  a 429, a 5xx, a timeout and a host that keeps failing. D213 §5 decided that policy, with a
  breaker, for TMDB's images; D224 decided OpenSubtitles' 401, 406 and 429.
- **What happened:** M3's closing audit walked every class for every host. TMDB's metadata fetch
  had no pause for a failing host, and a failed first match is never queued again, so a
  7.5-minute outage loses automatic matching for every title queued in it. A provider's 5xx
  reached the person as `internal`. OpenSubtitles' dropped token (a 500) failed for a day, and
  its login and download link could send the key or a download to any host. A node's refusal to
  prepare, for a full disk, was kept as FFmpeg's permanent failure.
- **Why the plan missed it:** each decision answered the host or job in front of it; nothing asked
  the same question of the next one, and "refused" and "failed" were one field on the node
  protocol until loudness split them.
- **Cost:** six fix commits and four proposals (audit-2026-09-29 §5, §9 A).
- **Process lesson:** when a policy is decided for one caller of a kind (a host, a job, a node
  answer), list every other caller of that kind in the same decision and say whether it applies.

### P37 — The branch gates' default base was days stale, so they checked almost nothing (M3, 2026-09-29) · gate-design

- **Planned:** `make check-pr` compares the branch with `BASE`, `origin/main` by default: the
  commit the branch forked from.
- **What happened:** in the yoloAI sandbox the owner takes commits with `yoloai apply` and nothing
  is pushed, so `origin/main` stayed where the sandbox began, 323 commits back. Against it,
  revert-red saw 285 of 314 files as added, whose revert is a build failure rather than a test,
  and test-weakening skips added test files: it listed 5 weakenings where the range held 60. Every
  run that did not pass `BASE=` by hand passed while checking almost nothing, and the same stale
  ref made agents' worktrees start at M0's tree.
- **Why the plan missed it:** the gates were designed for a pushed branch and a pull request,
  where the fork point is the remote's head; the sandbox's workflow has no push.
- **Cost:** found by the closing M3 audit; a proposal for a local base ref (audit-2026-09-29
  §9 E).
- **Process lesson:** a gate that compares against a reference should print the reference and how
  far back it is, and refuse one it cannot vouch for, rather than pass quietly.
- **Since (2026-09-30):** the gates' base is `refs/orpheum/applied`, which the owner sets with
  `make mark-applied` after `yoloai apply`, then origin/main; `check-pr` prints it and how far
  back it is, and refuses an implicit base more than 150 commits back (`scripts/branch_base.py`).

### P36 — The code outgrew cut-a's layout, lists and counts, and the frozen text still shows the old ones (M3, 2026-09-29) · plan-gap

> **Resolved 2026-10-04, at the pre-M4 audit:** cut-a's §2a, §3, §4, §5 and §5.3 were rewritten
> from the code, as the audit proposed and the owner agreed; what follows is the record of how
> they had drifted.

- **Planned:** cut-a.md, frozen by D193 §8, fixes the repository layout (§3), the config file's
  keys (§2a), the node protocol's messages (§4), the client API's endpoints and error model
  (§5) and the job slots (§5.3). Names D227 renamed are corrected in place; these are not names,
  so the text stays as frozen and this entry lists where the code differs, for M3's closing
  audit.
- **What happened:** a sweep of stale documentation at the end of M3 found:
  - **§3 layout.** Core packages `artwork`, `nodes`, `textindex` and `webclient`; node packages
    `agent`, `artwork`, `hardware`, `loudness`, `namedfile`, `prepared` and `subtitle`;
    `internal/archtest`, `internal/testutil`, and `store/cache`, `cachegen` and `storetest` are
    not in it. `internal/platform/` has eleven packages, not "clock, slogx". The subcommands
    also include `openapi`, `version` and, in dev builds, `dev`. `go.mod` has no `tool`
    directives: sqlc and golangci-lint are pinned in the Makefile. `testdata/`, `assumptions/`
    and `tools/gates/` do not exist (the gates are `scripts/`).
  - **§2a config keys.** "Exactly the keys in `deploy/dev/orpheum.dev.toml`" is true of the file, but the
    list beside it leaves out `server.socket_dir`, `node.name`, `video_extensions`,
    `subtitle_extensions`, `max_errors_per_root`, `probe_timeout_seconds` and
    `status_interval_seconds`.
  - **§4 node protocol.** No message carries a version; the version is the path's (`/node/v1`,
    `/bytes/v1`). The Node → core table lists hello, report and commands; each job also answers
    on its own path (`/node/v1/artwork`, `/prepare`, `/subtitle`, `/loudness`). The hello also
    carries `build`, `host`, `uid`, `gid` and `hardware{gpus, trials, chosenEncoder}`; `prepare`
    also carries `videoStreamIndex`, `videoCodec` and `segmentLengthSeconds`.
  - **§5 API.** A play report is `POST /playback/{token}/report`, by the media token, not the
    file's id. `POST /files/{id}/prepare` ("Get this ready") is not built; it is M5's. The job
    view also shows `attemptCount`. The OpenAPI document is not diffed with `oasdiff`; nothing
    runs it. **§5.2:** the error codes are one list in the server (`internal/core/api/errors.go`),
    but the OpenAPI document types `code` and `failedHop` as plain strings, so the SDK has no
    generated copy of it; and of the five hops only `node` and `provider` exist, the rest waiting
    for M4's reachability.
  - **§5.3 slots.** The node runs one encode and two remuxes at once, and one subtitle extraction
    in a slot of its own, not "two remux or extract jobs".
  - **Search** (D193 §4, standards/sql.md §4) is three static hand-written statements, one per
    route (D48), not a filter builder: M2 needed no filter, so nothing is assembled at run time.
  - **Added 2026-09-30, with D230 and the architecture map's F2 and F6.** §3's layout gains
    `internal/core/preferences`, which keeps a person's choices. §5: `GET/PUT /me/settings` are
    gone, since no setting is a person's; `PUT /files/{id}/preferences` also takes `audioChoice`;
    the playback answer also has `audioTracks` and `startingAudioStreamId`; and the transcode
    reasons also have `audio_track`, a chosen sound other than the one the original plays first.
- **Why the plan missed it:** the plan was written before the packages, keys and paths existed,
  and each milestone added what its features needed without the frozen text following; only the
  names had a decision (D227) that reached back into it.
- **Cost:** none in code. At M3's closing audit the owner chose (2026-09-30: "agreed",
  [audit-2026-09-29.md](design/research/audit-2026-09-29.md) §9 D): cut-a stays the record of
  intent, as its header now says, and this entry is where the code differs from it. A later
  difference of the same kind is added here, not to cut-a. Only cut-a §6's eviction clause was
  amended, to D217 §5's wording.
- **Process lesson:** a frozen plan's enumerations (layouts, key lists, endpoint lists, counts)
  go stale fastest; either generate them from the code or mark them as illustrative when freezing,
  so that only intent is frozen.

### P35 — Revert-red could not see a change only a browser shows (M3, 2026-09-29) · gate-design

- **Planned:** revert-red (D190) reverts each changed file and runs the narrowest suite that
  covers it, widening only for a file that survives: for web source, vitest and then the type
  check.
- **What happened:** a fix to the sound question's layout added a stylesheet that its component
  imports. The browser tier, whose phone test checks the layout, went red without it, but
  revert-red never runs the browser tier, and vitest renders no CSS, so the import could never go
  red there. The owner chose to have revert-red try the browser tier too.
- **Why the plan missed it:** the gate was written when the web client had no browser tier (it came
  with M3, D217 §4), and its idea of a web test was vitest's.
- **Cost:** a `browser-tier` target, `make test-browser` without its install step, which
  revert-red runs last for a web source file that survives the unit tests and the type check:
  about two minutes, and only for such a file.
- **Process lesson:** when a new test tier is added, walk every gate that chooses which tests to
  run and ask whether it should reach the new tier.

### P34 — A film too quiet to hear brought loudness levelling into Cut A (M3, 2026-09-28) · requirement-emerged

- **Planned:** D83 designed loudness measurement and a playback gain; D178 §4 left it out of Cut
  A, and cut-a §1 listed "loudness measurement" under Out. The dialogue boost was Cut A's only
  answer to quiet sound.
- **What happened:** with a real library on the dev loop, the owner found *Wing* too quiet to
  hear with the speakers at maximum. It measured -35.7 LUFS, about 12 dB under the owner's
  typical films, and it is stereo, so the boost, which acts on a downmix, could not touch it. The
  owner brought levelling into Cut A (D226).
- **Why the plan missed it:** the plan's test media were generated at one level, and its
  reasoning about quiet sound was about dialogue in a downmix, the case the design research had
  documented; a whole film mastered quiet was not among the cases considered, and no real library
  was played before M3.
- **Cost:** a measurement job on the node, a table, a gain in the core, a switch per person and
  Web Audio in the player, added to M3.
- **Process lesson:** play a sample of the real library, not only generated fixtures, before
  deciding what a release can leave out; generated media hide the spread real media have.

### P33 — The owner could not tell whether the server was set up right (M3, 2026-09-27) · requirement-emerged

- **Planned:** cut-a §6 named the hardware encoders; `doctor` and the Machines section of
  Libraries reported confinement and prepared storage; nothing showed which encoder a node had or
  used.
- **What happened:** once hardware encoding was built, the owner asked for a single admin page
  that shows every fact about the server and each node, and for M3, enough of it to know that
  hardware encoding works: *"set-and-pray is never fun"* (D225).
- **Why the plan missed it:** the plan listed what the server does, not how its operator learns
  that it is doing it; diagnostics were scattered across the features that produced them.
- **Cost:** a System page, the per-encoder trial and device report, and a record of the encoder
  each prepare used, added to M3.
- **Process lesson:** for each capability that depends on the machine (hardware, permissions,
  kernel features), plan where its operator sees that it works, alongside the capability.

### P32 — A fetched subtitle's credentials, search and conversion were left to the builder (M3, 2026-09-27) · plan-gap

- **Planned:** subtitle-acquisition.md gave each user their own OpenSubtitles credentials and a
  role capability, and ordered candidates by `moviehash`; cut-a named two endpoints.
- **What happened:** building D223 §4 raised five questions the plan did not settle: who may
  fetch, where the API key comes from, how a candidate is found, whose count of downloads is
  shown, and which side parses the fetched text. The owner answered them (D224).
- **Why the plan missed it:** the plan was written for automatic fetching, with verification;
  when Cut A narrowed it to fetching on request, the narrowed form was described, not re-planned.
- **Cost:** a round of questions before the feature's first line.
- **Process lesson:** when scope narrows a planned feature, plan the narrowed form again rather
  than trimming the old plan's text.

### P31 — Decided work reported built with a clause left out, again (M3, 2026-09-27) · plan-gap

- **Planned:** D217 §5 gave the node a budget, eviction by last use, a grace and a disk-full
  limit for prepared renditions; D217 §4 allowed Playwright for M3's timing test; cut-a §6 named
  three hardware encoders in order.
- **What happened:** none was built, and D217's status line said "built in M3". The M3 audit
  found them by reading each clause back (checklist item 19), which the status line had not had.
- **Why the plan missed it:** the milestone's row lists outcomes ("prepare with both profiles"),
  not the mechanisms a decision attached to them; a mechanism with no acceptance item is easy to
  leave out and hard to notice missing.
- **Cost:** three pieces of M3 built after the audit, and a status line corrected.
- **Process lesson:** a milestone's acceptance list should name each mechanism a decision
  attaches to it, or the decision should carry its own "done when"; a status line is written
  only after its clauses are read back.

### P30 — The gate that finds slow queries ran only where no one looked (M3, 2026-09-27) · gate-design

- **Planned:** the reference scale's test (sql.md §11) keeps every listing query off whole tables.
- **What happened:** it lives in the integration tier, which `make check-pr` did not run and CI
  ran only on a push; two M3 queries that read whole tables reached `main`.
- **Why the plan missed it:** the tiers were split by cost, not by what each gate protects.
- **Cost:** a migration, a query rewrite, and `make check-pr` running the integration tier.
- **Process lesson:** every gate that protects `main` runs in the branch gate, however slow.

### P29 — The branch gate's revert check was blind to most of the web client (M3, 2026-09-27) · gate-design

- **Planned:** revert-red proves every behaviour change has a test that fails without it.
- **What happened:** its worktree had no installed web packages, so every TypeScript revert
  "failed" for want of vitest and passed as covered; types, other architectures and generated
  queries were blind spots too. Found only when a report did not match a hand-run.
- **Why the plan missed it:** the gate was first seen to fail on Go, and its web path was
  never watched going red.
- **Cost:** four gate fixes, and screens made testable (happy-dom).
- **Process lesson:** a gate is watched failing on every kind of file it claims, before its
  passes are trusted.

### P28 — The sound question's two sources disagreed (M3, 2026-09-27) · plan-gap

- **Planned:** cut-a §6 played the original while the boost was off; audio-presentation.md's
  "done when" prepared a 5.1 film on TV speakers either way.
- **What happened:** building the question found the contradiction; D222 chose the copy, and
  counted headphones as stereo.
- **Why the plan missed it:** two documents each stated the rule for their own concern.
- **Cost:** one decision.
- **Process lesson:** a rule stated in two documents is one rule; say it once and point at it.

### P27 — Next Up was deferred past M3 (M3, 2026-09-27) · requirement-emerged

- **Planned:** cut-a §1 and `/home` in §5 include Next Up and "recently added".
- **What happened:** the owner deferred Next Up (*"Next Up doesn't have to be in M3"*), and
  "recently added" with it; `/home` has Continue Watching only.
- **Why the plan missed it:** nothing missed; the owner chose to trade it for the milestone's
  other work.
- **Cost:** none yet.
- **Process lesson:** none; recorded so the plan's row and the code agree on what is left.

### P26 — M3's contract had gaps in nine places, found by reading it end to end before building (M3, 2026-09-25) · plan-gap

- **Planned:** cut-a §4–§6 and §8 as the build contract for playing in a browser.
- **What happened:** a review before the first line found nine places where building would
  have meant inventing design. The client's menu had no wire format, and the plan called the
  container an axis where D21 says it is not. The media token was unnamed as a credential or
  not. Three sections put prepared output in different places. The job queue's one-per-file rule
  could not hold two renditions. The subtitle command lacked the payload hash §4 requires of
  every command. The play position had three different guards. Nothing created a device, and
  M3's "accounts" had no content once invites moved to M4. The new player libraries were not on
  the allowlist. The owner settled them in D217 and D218.
- **Why the plan missed it:** sections written at different times by the same design pass,
  each consistent inside itself; nobody read the milestone's path through all of them at once.
- **Cost:** one review, one exchange with the owner, two decisions, one allowlist patch.
- **Process lesson:** before a milestone, walk one request through every section it touches,
  in order, and list each value some step needs that no step produces.

### P25 — The node's byte socket, part of M2, was never built (M3, 2026-09-25) · plan-gap

- **Planned:** cut-a §2 and §4: the node serves `/run/orpheum/node.sock`, and M2's row names
  "both sockets". §4's file route is `GET /bytes/v1/file?path=…&hash=…`.
- **What happened:** M2 read no media bytes through the core (artwork travels in the node's
  answer), so nothing needed the socket and nothing noticed it was missing; `byteserver` held
  only its `doc.go`. The M3 review found it. Building it, the route gained a `root` and carries
  `path` as base64url of the bytes relative to that root, because a path is known relative to its
  root and need not be UTF-8 (D49).
- **Why the plan missed it:** a milestone row named a component that no acceptance test of
  that milestone exercised.
- **Cost:** built at the start of M3.
- **Process lesson:** every component a milestone names needs an acceptance line that fails
  without it; otherwise it is scheduled only by hope.

### P24 — Creating a library needs a node's roots, and nothing lists them (M2, 2026-09-25) · plan-gap

- **Planned:** cut-a §5's `POST /libraries` takes a node's name and roots its node configured
  (D107); the web client's create form asks three questions (D184).
- **What happened:** building the form found no endpoint that lists nodes or their configured
  roots. The form takes them typed; the server refuses a root the node did not name.
- **Why the plan missed it:** the endpoint table was written from the server's side, where the
  node's hello already carries its roots; the form's author is the first who has to know them.
- **Cost:** `GET /nodes`, built after the audit (audit-2026-09-25, proposal 5); the form now
  offers a connected node's roots to tick.
- **Process lesson:** for every write endpoint, check that a client can learn every value it has
  to send.

### P23 — The plan never said how an image beside the media ranks against a provider's (M2, 2026-09-25) · plan-gap

- **Planned:** artwork.md named "images beside the media" as a source, and D183 says NFO files
  come before a provider where the library reads them; nothing said which image shows when a
  film has both its own `poster.jpg` and TMDB's.
- **What happened:** the owner decided (D215): images beside the media are taken in every
  library at once; where the library reads NFO files they win and the provider's of that kind is
  not fetched; in a TMDB-only library the provider's replaces them once fetched. The owner amended it
  mid-build, after seeing that the first version ignored local images in TMDB-only libraries.
- **Why the plan missed it:** artwork was planned as one pipeline from one source; the question
  only exists once there are two sources for the same slot.
- **Cost:** one decision, amended once; the ranking lives in one query.
- **Process lesson:** wherever two sources can fill the same slot, the plan says which wins, and
  what the other one is for.

### P22 — The artwork ceiling keeps each shape's smallest size, and re-cuts from the base (M2, 2026-09-25) · plan-gap

- **Planned:** D212 §6: never evict a live item's poster-wall size; evict bases last, "fetched
  again when next needed"; §2: every size is cut from the base.
- **What happened:** the smallest size of each shape (portrait-s, landscape-s, logo-s) is kept
  too, so a request for an evicted size always has a size to be redirected to; a backdrop or a
  still has no poster-wall size, and without it an evicted one would answer 404. An evicted size
  no larger than the held base is cut again by the node from the base, our own WebP, and nothing
  is fetched; only a larger size, or an image with nothing held, goes back to the provider. The
  node marks at fetch time the sizes the source is too small for, so a missing size with no mark
  is known to have been evicted. The high-water warning is `GET /doctor`'s first finding.
- **Why the plan missed it:** D212 named the poster wall, which is posters only; and "cut from
  the base" did not say by whom once the node is never on the request path.
- **Cost:** the smallest sizes add about 0.6 GiB at the reference scale, mostly episode stills,
  to what is never evicted.
- **Process lesson:** when a policy protects one case, check what serves the cases next to it.

### P21 — A person's refresh endpoint, from D213 (M2, 2026-09-25) · plan-gap

- **Planned:** cut-a §5 had no refresh; D170 allowed a refresh only when the user asks, per
  title or per library, and no scheduled refresh.
- **What happened:** the owner decided D213 after the research into provider etiquette:
  `POST /items/{id}/refresh` for one title or series, as often as a person likes; no
  library-wide refresh; a rolling refresh per provider; a dead image checked once.
- **Why the plan missed it:** TMDB's six-month limit on keeping its data was not known when
  D170 was decided.
- **Cost:** one research round; the refresh, the sweep and two settings.
- **Process lesson:** before designing around an external service, read its terms of use.

### P20 — The media cookie is Secure only over TLS until the HTTPS name (M2, 2026-09-25) · plan-gap

- **Planned:** D154: the media cookie is `Secure`, `HttpOnly`, `SameSite`, scoped to the byte
  paths.
- **What happened:** Cut A has no TLS until M4's HTTPS name, and a browser never sends a
  `Secure` cookie back over plain HTTP, so posters would not show in M2 and M3 anywhere but on
  localhost. The cookie is `Secure` when the request came over TLS. The rest is as D154 says.
- **Why the plan missed it:** D154 was written for the finished product, where every
  connection is TLS; the milestones put TLS after the first client that needs the cookie.
- **Cost:** none; once M4 serves every client over TLS, the cookie is always `Secure`.
- **Process lesson:** when a decision assumes a transport, check the milestone that first
  needs it has that transport.

### P19 — JPEG is cut from the WebP of its size, not from the base (M2, 2026-09-25) · plan-gap

- **Planned:** D212 §4: "JPEG is cut by the core on the first request for it, from the base".
- **What happened:** the core cuts a size's JPEG from the WebP of the same size, which the node
  cut from the source. Both are one lossy step from our own encode; cutting from the base would
  add a resize, and so a resampler, to the one core package that decodes images, for no gain in
  quality. A size with no WebP yet redirects to the nearest size's JPEG.
- **Why the plan missed it:** D212 named where JPEG comes from before the sizes were settled as
  each being made by the node.
- **Cost:** none.
- **Process lesson:** none new; recorded so the decision's text and the code agree.

### P18 — D208's two admin answers had no endpoint (M2, 2026-09-25) · plan-gap

- **Planned:** D208 withholds a scan's removals "and the admin asked", and keeps a missing
  disk's boundary "until the disk returns or the admin says it is gone". cut-a §5 lists every
  endpoint.
- **What happened:** §5 had no endpoint for either answer; building the Libraries API found it.
  The owner approved two: approve a root's removals, and say a disk is gone. Both approve the
  root's *next* scan rather than replaying the withheld one, whose unreadable regions were never
  kept (migration 0008).
- **Why the plan missed it:** D208 was decided after the plan's endpoint table was written, and
  a decision that says "the admin is asked" did not prompt anyone to add where the admin answers.
- **Cost:** one proposal round; a migration.
- **Process lesson:** when a decision puts a person in a loop ("asked", "confirms", "chooses"),
  add the endpoint and the screen for their answer to the plan in the same change.

### P17 — A migration drafted, applied by the dev loop, then edited (M2, 2026-09-25) · gate-design

- **Planned:** migrations are append-only, enforced by the pre-edit hook on committed steps and
  by SHA256SUMS (D181).
- **What happened:** the dev loop rebuilds and migrates on every save, so it applied the first
  draft of step 0007. The agent then added a column to the uncommitted step. The dev database
  stayed at version 7 without the column, and identification failed with "no such column".
- **Why the plan missed it:** append-only was enforced where a step becomes shared (commit), but
  a step becomes *applied* earlier than that — the moment the dev loop sees the file.
- **Cost:** a dev database reset; migration 0009 and a runner check.
- **Process lesson:** enforce an invariant where the state it protects is created, not where it
  is published: the runner now records each applied step's checksum and refuses one that
  changed.

### P16 — Identification was designed per title, and a series is not a title (M2, 2026-09-24) · plan-gap

- **Planned:** D170 and D172 describe matching, fetching and choosing for a *title*; the schema
  gives `title` its source, id and ident and keys `provider_response` on `title_id`; cut-a §5's
  chooser is `/titles/...`. The library model makes a series a Collection (D160, D186).
- **What happened:** building TMDB matching for shows found that providers identify the show,
  not the episode, and nothing gave the series a source, a match, an ambiguity or a stored
  response. The owner decided the series is what is identified (D209); the schema and the
  chooser's paths change to fit.
- **Why the plan missed it:** identification was designed on films, and TV was assumed to be
  "the same, per episode". The two designs met only in code: the library model's shape
  (series as Collection) and the identification model's unit (title) were each right alone.
- **Cost:** two decisions (D209, and D210 for the files' episode ordering the same gap exposed, deferred past Cut A); a migration and an API change, proposed before building.
- **Process lesson:** when one design names a unit ("per title") and another design decomposes
  the domain differently, walk one concrete case through both — here, one TV episode from file
  to metadata — before freezing either.

### P15 — Building the node exposed four things the frozen plan never decided (M2, 2026-09-24) · plan-gap

- **Planned:** cut-a §2–§4 and §7 described the node, its protocol and its confinement.
- **What happened:** building it needed four decisions the plan did not contain: how a symlink
  in a media root is treated; which library normalises filenames and language tags; how `run`
  starts its child when only `execx` may start a process and `execx` ran only FFmpeg; and which
  packages hold the node's loop and the core's side of it (D206, D207). Each was asked, not
  assumed. A fifth — the AppleDouble skip rule — was in the testing plan and the dev library
  but not in scanner.md, and the scanner missed it until the dev loop showed it erroring.
- **Why the plan missed it:** the plan described messages and behaviour; process topology and
  filesystem edge cases only appear against real code, and a rule stated in one plan was not
  cross-referenced into the plan the scanner was built from.
- **Cost:** two decision rounds with the owner; one skip rule added after the fact.
- **Process lesson:** before building a component, collect every rule about it from all the
  plans, not just its own; and ask "what starts this, and what may it touch?" for each process.

### P14 — The standard library reads configuration the plan never listed (M2, 2026-09-24) · plan-gap

- **Planned:** "only `cmd/orpheum` reads the environment" (go.md §12), enforced by banning
  `os.Getenv` and its siblings outside `cmd/`.
- **What happened:** the owner asked for no ambient configuration at the product level (D204).
  Checking showed the ban covered our calls but not the standard library's: Go's default HTTP
  transport reads `HTTP_PROXY`, `time.Now` formats in `TZ`'s zone, SQLite and `os.TempDir`
  follow `TMPDIR`, and every release build still handed `os.LookupEnv` to its subcommands.
- **Why the plan missed it:** the rule was written against the calls in our code, not against
  what the process can be steered by.
- **Cost:** a decision, four small code changes and a lint patch, before M2's first outbound
  request.
- **Process lesson:** state a containment rule as the list of inputs the program may have, then
  hunt for every input it actually has, library defaults included; banning calls finds only
  your own.

### P13 — sqlc fails on a multibyte character in a query comment (M1, 2026-09-24) · tool-behaviour

- **Planned:** sqlc compiles `internal/store/queries/*.sql`; nothing constrained their text.
- **What happened:** a `§` in a comment made sqlc 1.31 reject the whole file with *"edited query
  syntax is invalid"*, far from the cause.
- **Why the plan missed it:** a byte-versus-character bug in a tool is found only by using it.
- **Cost:** a debugging detour; a README rule; a gate proposed (audit §4.3).
- **Process lesson:** when a tool's error is far from its cause, bisect the input before reading
  the tool's documentation; then turn the finding into a check, not a note.

### P12 — An acceptance item needs hardware the development machine is not (M1, 2026-09-24) · environment

- **Planned:** "argon2id takes ≤ 500 ms on devnas" as M1 acceptance.
- **What happened:** development moved to a VM (P1); devnas was not at hand, and the item is
  queued as VQ1 while M1 is otherwise done.
- **Why the plan missed it:** acceptance was written against the machine the plan expected to
  develop on.
- **Cost:** M1 is "done, VQ1 open" rather than done.
- **Process lesson:** split acceptance into what the development loop can prove and what only the
  target hardware can, and schedule the second where the hardware is.

### P11 — The home connection cannot take the plan's first reachability route (M1, 2026-09-24) · environment

- **Planned:** reachability tries a port-forward of 443 first, then IPv6, then a tunnel (cut-a §2).
- **What happened:** the owner's connection is DS-Lite: no public IPv4, so no port-forward; only
  IPv6 or a tunnel can work.
- **Why the plan missed it:** cut-a §9 listed "test the ISP" as an owner task; it had not been done.
- **Cost:** none yet — found before M4, which is the milestone it would have broken.
- **Process lesson:** run the plan's infrastructure checks before the first milestone, not before
  the one that needs them; they are cheap and can reorder the plan.

### P10 — M1's canary acceptance named bug reports, which Cut A does not build (M1, 2026-09-24) · plan-gap

- **Planned:** the canary proves a secret appears in no "API response, log, doctor output, bug
  report or backup".
- **What happened:** bug reports are out of Cut A's scope; the item could not be met as written.
- **Why the plan missed it:** acceptance text was written from D194's full design, not from the
  scope list it sat beside.
- **Cost:** an audit finding and a wording fix.
- **Process lesson:** check each acceptance item's nouns against the milestone's scope list.

### P9 — An M2 acceptance test contradicts the process topology (M1 audit, 2026-09-24) · plan-gap

- **Planned:** "with the media root at mode 000 to the core's user, the core still works".
- **What happened:** the node runs as the core's child under one service user, so a real node
  could not read that root either. Found by the audit, before any M2 code.
- **Why the plan missed it:** the test was written for the property (the core never opens media)
  without checking the topology it runs in.
- **Cost:** a decision (the test uses a replaying node).
- **Process lesson:** for each acceptance test, sketch how it would run on the planned topology
  before freezing the plan.

### P8 — The settings model changed three times in a day (M1, 2026-09-24) · requirement-emerged

- **Planned:** a registry with defaults in code; a row exists only when someone set a value
  (D183).
- **What happened:** seeing it built, the owner wanted no defaults in code (D200); then a stored
  "modified" mark and defaults that follow releases (D201); then healing instead of refusing, and
  no seed command (D202). A `seed-settings` command was built and removed in the same day.
- **Why the plan missed it:** the preference existed ("I worry about defaults", D183) but its
  consequences were only visible once there was behaviour to look at.
- **Cost:** three decisions, a migration, a command built and deleted, two agent failures (A1, A2).
- **Process lesson:** for anything the owner has strong instincts about, build the smallest
  slice that shows the behaviour and ask before building its surroundings.

### P7 — Four points the frozen plan left open appeared in M1's first hour (M1, 2026-09-24) · plan-gap

- **Planned:** Cut A's design frozen and complete enough to build (D193 §8).
- **What happened:** `secret.key`'s location was stated two ways; `cache.db` did not fit sqlc's
  one-schema model; M1's API scope was unstated; the first golden's owner was unclear (D198).
- **Why the plan missed it:** each was visible only against the code's structure (a package
  boundary, a generator's config, a hook).
- **Cost:** one decision round with the owner before code.
- **Process lesson:** before each milestone, read its plan rows against the actual code and
  tools for an hour and list the questions; the between-milestone audit now does this.

### P6 — The shell guard blocked correct work by matching words (M1, 2026-09-24) · gate-design

- **Planned:** a pre-command hook that refuses writes to gate files and new dependencies.
- **What happened:** it refused at least eight harmless commands — a read of the hooks config, a
  `grep` for `go get`, writes to scratch files and to the agent's memory — because it matched gate
  words and write tokens anywhere in a command.
- **Why the plan missed it:** it was tested against the commands it should catch, not against a
  milestone's worth of ordinary commands.
- **Cost:** repeated workarounds, which teach exactly the evasion the guard exists to prevent; a
  rewrite (audit §4.1).
- **Process lesson:** test a guard against a realistic stream of allowed actions as well as the
  forbidden ones, and decide from effects (what is written where), not from words.

### P5 — Three branch gates misjudged their first real milestone (M1, 2026-09-24) · gate-design

- **Planned:** revert-red, test-weakening and generated-code checks, each unit-tested (M0).
- **What happened:** revert-red never ran the dev-tagged suite or the macOS compile; the
  test-weakening check counted a newly added golden as a weakening; check-generated compared with
  git's index, so correct unstaged output failed.
- **Why the plan missed it:** each gate was tested on synthetic cases, not on a milestone's diff.
- **Cost:** three owner-applied patches.
- **Process lesson:** before trusting a gate, run it over the first real milestone and treat its
  first findings as findings about the gate.

### P4 — A library's transitive imports broke an architecture rule (M1, 2026-09-24) · tool-behaviour

- **Planned:** the pure-Go SQLite driver (D31); the core never reaches `os/exec` (D26, D193).
- **What happened:** the driver's C runtime, `modernc.org/libc`, imports `os/exec`; every core
  package using the store failed `TestArch_CoreNeverExecs` (D199).
- **Why the plan missed it:** the driver was chosen for being pure Go, and its imports were never
  checked against the rules it would live under.
- **Cost:** a decision and an exemption in the architecture test.
- **Process lesson:** when choosing a dependency, run the architecture checks against it
  (`go list -deps`) before adopting it.

### P3 — Two M0 gates contradicted each other on generated code (M1, 2026-09-24) · gate-design

- **Planned:** every package has a `doc.go`; generated directories are never hand-edited.
- **What happened:** `internal/store/sqlcgen` is a package and generated: it could not satisfy
  both (D198).
- **Why the plan missed it:** the gates were written before any generated package existed.
- **Cost:** an owner-applied patch.
- **Process lesson:** list every class of file a gate will meet — generated, vendored,
  platform-specific, build-tagged — and decide each one when the gate is written.

### P2 — Compiled Python caches were committed despite `.gitignore` (M0, 2026-09-24) · gate-design

- **Planned:** `.gitignore` excludes `__pycache__/`.
- **What happened:** two `.pyc` files were on `main`, under `scripts/`, a gate path; removing them
  needed the owner's trailer.
- **Why the plan missed it:** they were added before the ignore rule, or forced; nothing checks
  that tracked files obey `.gitignore`.
- **Cost:** a cleanup commit with an owner trailer.
- **Process lesson:** a hygiene check (`git ls-files -ci --exclude-standard` is empty) belongs in
  the first gate set of any project.

### P1 — Development moved to a VM and a sandbox (M1, 2026-09-24) · environment

- **Planned:** development on devnas with a copied corpus (cut-a §2).
- **What happened:** development moved to a Proxmox VM running a yoloAI sandbox: no GitHub access
  from the agent, commits leave by `yoloai apply`, tools installed outside PATH, ports forwarded
  through the sandbox.
- **Why the plan missed it:** the development environment is a working choice, not part of the
  design, and it changed.
- **Cost:** a session of setup; rule 14 amended; memory notes for the sandbox.
- **Process lesson:** write the development environment's assumptions down separately from the
  design, so a change of machine is a checklist, not a discovery.
