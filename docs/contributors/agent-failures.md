> **ABOUTME:** The log of mistakes the code-writing agents made that the owner had to correct:
> what happened, what caught it, and whether a gate now catches it. The raw material from
> which the next lint rule, test or hook gets built.

# Agent failures

This project is written by agents, so **the agent is a component with failure patterns**, like
any dependency whose behaviour we cannot change and can only characterise and guard against.
This file records the specimens. Its model is yoloAI's log of the same name, whose conclusion
was *"Execution catches my errors. Reading does not"*, and whose gates against the agent's own
habits were each built from an entry written here first
([principles §21](principles/README.md#21--an-owners-correction-is-a-specimen-and-it-is-written-down)).

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
