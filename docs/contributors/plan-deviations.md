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
