> **ABOUTME:** The engineering disposition for this project — the small set of principles that
> decide arguments, each earned from a specific failure observed in the competitive research.

# Principles

Principles explain **why**; the per-technology standards under `../standards/` explain **what**
and **how**. A principle wins over any conflicting standard or design choice.

These are deliberately few. They will grow when a real argument needs settling, not before.

## §1 — Boring technology, and innovation tokens

Prefer technology that is established, widely deployed and well understood. Spend the small
number of "innovation tokens" this project has on the things that are genuinely novel — the
playback negotiation model (D2) and the transcode core (D4) — and take the obvious, dull answer
everywhere else.

**Bites when:** you are about to add a second datastore, a message broker, a second server
language, or a framework that owns your application structure.

## §2 — The user's files are not ours to touch

Media is mounted read-only by default. The database is a rebuildable index, never the authority
(D7). Destructive actions say exactly what they will destroy.

**What the user authored survives us two ways, and they are not interchangeable**
([D98](../decisions/working-notes.md)). The automatic database backup
([D77](../decisions/working-notes.md)) is **durability** — it is on by default and needs no
permission. Sidecars beside the media are **portability**, the part that means "survive *us*",
and writing one needs a writable path this principle does not grant by default. And the split
is by subject, not by convenience: **a sidecar carries what is true about the work; the
database carries what is true about a person**, because the media tree is shared with the
household and often with a grantee.

**Deleting media is deferred, and that is now the stated policy**
([D144](../decisions/working-notes.md)). It appears **0 times in 120 top-reacted Jellyfin
issues and 0 times in Plex's yearly top 50**; [D89](../decisions/working-notes.md) came from an
audit noticing the tree said nothing, not from anyone asking. Its real purpose — *"unstated,
this ends with `os.Remove` in a handler and the read-only promise becomes a comment"* — is
kept as **an enforced absence**: no code path removes a file beneath a media root, asserted by
a test. A retention rule may still only ever *mark*
([D75](../decisions/working-notes.md)). **Running entirely read-only is a supported
configuration**, not a cautious user's workaround.

**And the user's *arrangement* of their files is no more ours to correct than the bytes are**
([D158](../decisions/working-notes.md)). Someone who re-rips a film because the first attempt
broke at 61 minutes, or who hand-deletes the opening scenes of *Wings* because they give the
film away, has made a decision. Our job is to index what is there, not to work out what they
meant and offer to reconcile it. **The concrete rule: we group files for browsing and never
relate their timelines** — if a feature needs to know that a moment in one file corresponds to
a moment in another, it is out of scope. A user-created Edition is their data and a rescan
never overwrites it, exactly as [D141](../decisions/working-notes.md)'s lock markers already
require for any other correction.

**But we do write to the media tree, so this principle still has a real consumer.**
[D141](../decisions/working-notes.md)'s sidecar export needs an **opt-in writable path per
library**, off by default; it happens in the node, never the core
([D33](../decisions/working-notes.md)); writes are **atomic**, and **the file it replaces is
retained**. "Destructive actions say exactly what they will destroy" applies to an overwrite
as much as to a delete.

**And where we do destroy, it is reversible** ([D143](../decisions/working-notes.md)). A
delete goes to a **trash at the root of each library root** — one per root, because
`rename(2)` does not cross filesystems and a central one would make every delete a
copy-and-delete that can fail half way. It never empties itself: automatic expiry would be
the single place this product destroys something nobody asked it to. So the two phases become
**confirm → trash → empty**, and the confirmation can be calmer because the act is undoable.

**Running with the media entirely read-only is a supported configuration, not a cautious
user's workaround.** Someone who never grants a writable path has a complete product, minus
deleting and minus sidecar export. Say so, because the alternative is people assuming they
must loosen permissions to get full function.

**Bites when:** a feature would be easier if the server could just write to the media tree, or
if some state existed only in the database — or when "we don't support deleting" quietly
becomes an `os.Remove` in a handler.

## §3 — Explain the decision to the user

The server constantly decides things on the user's behalf — what to transcode, what a file is,
which provider won a field. Every one of those decisions must be inspectable, and the important
ones must be overridable. "It constantly tries to re-encode video that could be played perfectly
fine" is a complaint about an *unexplained* decision as much as a wrong one.

**Bites when:** you implement an automatic behaviour without a way to see why it fired or to
turn it off for one item.

## §4 — One way to do it

One server language. One formatter. One linter config. One router. One way to write a query.
The value is not aesthetic: the fewer ways there are to express a thing, the more of the
codebase looks like the rest of it, and the better both reviewers and agents perform.

The same holds inside the codebase: a second home-grown mechanism for one job is the commoner
case, and [§19](#19--a-defect-is-a-sample-look-for-its-siblings-before-fixing-it) is its rule.

**Bites when:** a new dependency introduces a second idiom for something we already do.

## §5 — Combinatorial complexity gets an interface, not a branch

Hardware accelerators, containers, codecs, subtitle formats and metadata providers all multiply.
Each family goes behind one interface with one implementation per member and a capability probe.
`EncodingHelper.cs` is 8,042 lines because the alternative was chosen.

**Bites when:** you add an `if` for a specific accelerator, codec or provider outside that
family's own implementation file.

The same reasoning at the scale of one function: **push ifs up, fors down**
([Debasish Ghosh's account](https://debasishg.github.io/blog/push-ifs-up-fors-down/) of the idiom
from TigerBeetle's style guide, made known by matklad; adopted by the owner, 2026-10-08).

- **Ifs up.** A function takes what it needs, not what might be there: the caller decides once,
  and passes the decided value, so the function has no branch for a case it should never see.
  A nil part checked at every use is a decision put off; make it where the thing is built (the
  node exchanges take their unsent-attempt hearing from their sender once, in `NewExchanges`,
  rather than checking for it at each assignment).
- **Fors down, where a boundary is crossed.** A function that crosses one, the database, the
  network, the node protocol, takes the whole batch and loops inside: one query for a hundred rows,
  not a hundred queries. A loop that crosses nothing may stay where it reads best; the article's
  reason, vectorized tight loops, rarely applies to code that waits on disks and links.
- **Not at §4's cost.** Pushing a check up into every caller makes it many checks; when the callers
  multiply, the decision belongs in one constructor or one type, not in each of them.

**Bites when:** a function's first lines branch on whether an argument is usable, or a loop
calls something that crosses a boundary once per item.

## §6 — Untrusted code is sandboxed, or it does not run

Plugins, scrapers and metadata providers run in a WASM sandbox with an explicit capability list
(D12). There is no tier of "trusted" third-party code.

**Bites when:** a plugin API would be so much simpler as an in-process interface.

## §7 — Migrations are a safety feature

Back up before migrating, automatically, and say where. Forward-only, ordered, in-tree,
CI-verified. Detect-and-refuse across a version gap rather than auto-converting. Test against a
realistically-sized database, because hour-long hangs are invisible on a small one.

**Sharpened by [D181](../decisions/working-notes.md):** an *older* database is always
converted — every step is kept forever — and a *newer* one is refused unless it declared itself
compatible, naming the snapshot that leads back. The pre-migration snapshot is **kept after
success**, because that is when people want to go back. The policy is
[plans/migrations.md](../design/plans/migrations.md).

**Bites when:** a schema change seems too small to need a migration test.

## §9 — Sensors report; the core decides

Every component that observes the world — a media node, a webhook, a plugin, a probe — reports
**what it saw**. Interpretation, policy and action live in the core, which is the only place
that holds history, configuration and the user's preferences.

The test for which side of the line something falls on: *could this component determine it from
what it observed, with no memory of previous observations and no knowledge of the library?* If
yes it is a fact and the sensor reports it. If no it is a judgement and belongs in the core.

Three reasons this is not merely tidiness. A sensor that judges needs **history**, which means
duplicating core state and creating a second source of truth. A sensor is often **the component
under suspicion** — a node whose mount just vanished is not who should rule on what that means.
And scattering judgement means **nobody can reason about how the library reached its state**,
nor test that logic without hardware.

**A component reports according to its own visibility, and that bound is a fact rather than an
excuse.** A confined component sees less, not nothing — a relay that cannot read the film still
sees throughput, connection state, latency per leg and every drop. Limited visibility is not
limited usefulness, and it is usually the *unseen* part that would not have helped anyway:
nobody diagnoses stuttering by looking at video frames. Where a component sits in the path of
another's reports, it **pipes them through** rather than summarising them.

**A sensor may be asked to measure across several inputs; it may never choose them.**
"Find the segment repeated across this season" is a measurement a node can make and a *set* it
cannot assemble, because "these twelve files are one season" is a conclusion the core reached
from parses the node is forbidden to make ([D101](../decisions/working-notes.md)). So the core
supplies the list and the node supplies the number. A sensor that picks its own comparison set
has quietly acquired a copy of the library's grouping logic, and it will be wrong differently
from the original.

**The largest operator in this field arrived at the same split.** A Netflix Open Connect
Appliance — hardware inside a network Netflix does not control — does exactly two things:
*"report their status … and what files they have stored on disk"*, and *"serve content via
HTTP/HTTPS when it is requested by a client device"*. Steering, fill behaviour, storage layout
and telemetry interpretation are all in the control plane. That is this principle at planetary
scale, and it is independent evidence rather than a borrowed idea
([node-transport.md](../design/research/node-transport.md) §1).

**Bites when:** a component starts withholding, suppressing or correcting its own output
"helpfully"; when you find yourself giving a sensor a threshold, a memory, a copy of
something the core already knows, or the job of deciding which inputs go together; or when
confinement gets used as a reason a component cannot report. ([D42](../decisions/working-notes.md), [D73](../decisions/working-notes.md),
[D101](../decisions/working-notes.md), and the same reasoning as
[D37](../decisions/working-notes.md).)

## §10 — Give a client only what it may use; a refusal is an alarm, not a branch

If a client is not entitled to a resource, **omit it from the manifest** rather than serving a
URL and rejecting the request. The illegal action should be unreachable, not merely forbidden.

The server-side check still exists — a client is not a trust boundary — but its job changes. A
check that fires during normal operation is not enforcement, it is control flow in a security
costume, and it generates exactly the traffic that would hide a real attempt. A check that never
fires in a correct system is one whose every firing is worth looking at.

**This governs authorisation of our own cooperating client, and nothing else.** Validation of
untrusted input is the opposite case and *should* fire routinely: node reports arrive malformed
([D39](../decisions/working-notes.md), [D42](../decisions/working-notes.md)), and media is
attacker-supplied and FFmpeg's confinement is expected to be load-bearing
([D26](../decisions/working-notes.md)). Conflating the two leaves you with no signal in either,
because the security events drown in the normal ones.

**And it pays for something unexpected.** Because the manifest is *constructed per principal*
rather than filtered after the fact, "show me the library as this role sees it" is that same
construction with a different argument — so an admin can preview a restriction before
committing it, on the enforcement path rather than a simulation of it
([D132](../decisions/working-notes.md)). Neither Plex nor Jellyfin has that, and the reason is
visible in their shape: when entitlement is a check in a handler, there is nothing to run for a
principal who is not making the request.

**Bites when:** an entitlement is easier to express as a 403 in the handler than as a branch in
whatever builds the manifest. ([D58](../decisions/working-notes.md), from
[D57](../decisions/working-notes.md).)

## §8 — Factual accuracy in documentation

Verify claims. Date them. Mark what is unverified as unverified, in place, rather than dropping
it or laundering it into confident prose. Never state a count that a gate does not enforce.

**Bites when:** you are about to write a number, a star count, or "X is faster than Y" from
memory.

## §11 — A value on a hot path is cached against a known event, not computed per request

**The home page computes nothing.** Anything rendered on the screen every user sees at launch
is **computed once and kept until something known changes it**, then read as a lookup.
Computing it per request is the default that keeps being wrong.

**Cache it; do not incrementally maintain it** ([D161](../decisions/working-notes.md)). The
two perform identically on read and fail completely differently: a missed *invalidation*
leaves a value briefly **stale**, while a missed *update path* leaves it **wrong for ever,
with nothing to notice**. Incremental maintenance means enumerating every path that can move
the number — for the pending count that is a scan, a rule edit, a role edit, a tag edit, a
re-identification and a removal — and missing one silently stops a child's library growing.
The invalidation signal already exists: [D166](../decisions/working-notes.md)'s change journal
says what changed.

This is a principle rather than a note because it has now been rediscovered four times
independently, each time by a different route:

| Value | Per request | Kept | Mechanism | Found by |
| --- | --- | --- | --- | --- |
| Recently added | 21.79 ms | 0.027 ms — **807×** | cached result | the 2026-09-20 benchmark |
| FTS `bm25` ranking | 11.66 ms | 0.017 ms — **686×** | **derived column** — the query takes a term, so there is no single result to cache | the same benchmark |
| Pending-items count | 43.6 ms, or **no completion at all** without an index | 0.063 ms — **692×** | cached result | a *feature request*, 2026-09-21 |
| Subtitle-sync check | an audio decode, **~32 s** — and a sleeping node to wake | **3–8 ms** from a stored **1–3 kB** map | **stored artefact** | a *user complaint*, 2026-09-21 |
| A collection's sequence and progress | 14.8 ms at 976 members | 0.01 ms | cached result | the 2026-09-22 benchmark |

Two of those were audit findings, the third arrived attached to a new feature and the fourth
attached to a complaint about somebody else's product, which is what makes it a hazard rather
than a bug list: **every new surface rediscovers it.**

**And the fourth row widens the principle past the home page**, which is where the first three
found it. The speech map ([D148](../decisions/working-notes.md)) is not on any screen; the
hot path is *a user waiting for a subtitle*. The shape is identical — a value that is
expensive to derive, cheap to keep, and invalidated by a known rare event — so read "hot path"
as **anything a person is waiting on**, not as "the home page". The storage argument is the
same one every time, and it is never close: **1–3 kB per film, 3 MB for a thousand-film
library**, against re-deriving it on demand.

**The invalidation is not the hard part, which is why this can be absolute.** These values
change only on rare, known events — a scan completing, a rule or role changing, a tag edit, a
metadata refresh — and [D166](../decisions/working-notes.md)'s change journal already says
exactly what changed, with `rules-engine.md` already consuming it on a debounce. Event-driven
invalidation on a mechanism that exists, not a TTL and not new machinery.

**Three mechanisms, and incremental counters are not among them**
([D161](../decisions/working-notes.md)): a **cached result** is the default; a **derived
column** is for queries that take a parameter, so there is no single result to keep; a
**stored artefact** is for bytes derived from bytes. Read "maintained" anywhere in this tree
as "cached" unless it says otherwise.

**The boundary worth knowing:** per-*library* values are cheap to maintain because there is one
of each; per-*user* values are not automatically so, because there may be many. Continue
Watching measured flat and needs nothing today ([D113](../decisions/working-notes.md)); the
rule bites where it would not.

**Bites when:** you add a count, a badge or a "new since" marker to a screen, and the query
looks harmless at the size of your test library. ([D113](../decisions/working-notes.md),
[D133](../decisions/working-notes.md), [D134](../decisions/working-notes.md).)

## §12 — Defer the work, never the seam

**A feature you have decided not to build still constrains the design, and the constraint is
cheap exactly once.** Rank what to build by demand; decide what to *design around* by
**retrofit cost**, which is a different number and frequently points the other way.

The four costs, in the order they hurt:

| Cost to add later | Example | Pay it |
| --- | --- | --- |
| **free** — additive, usually client-side | shuffle, sleep timer | never; ignore it until there is room |
| **seam** — needs an extension point to exist | a DLNA adapter, a transcoder | when the first *real* consumer arrives ([D19](../decisions/working-notes.md)) |
| **schema** — needs a column or table | external ratings, artwork types | **before the model freezes**, or it is a migration |
| **invariant** — needs a rule *not* to have been assumed | *"every item has a File"* | **now**, because it costs nothing to keep and cannot be recovered |

**The last row is the one that gets missed**, because keeping an invariant open looks like
doing nothing. It is the cheapest work in the project and the only kind that becomes
impossible. "Every item has a File" is one sentence today and a rewrite the day somebody wants
live TV — and *we have declined live TV*, which is exactly why nobody would think to protect
it.

**So a decline is a statement about effort, never about possibility.** Recording *what keeps
the door open* beside each declined feature is what makes changing your mind later a decision
rather than an excavation — which is what [`FEATURES.md`](../../FEATURES.md)'s last column is
for.

**Bites when:** you are freezing a schema, closing an enumeration, or writing "must" in an
interface, while a feature you have just declined quietly depended on the opposite.
([D153](../decisions/working-notes.md).)

## §13 — State attaches to what you observed, never to what you inferred

**A conclusion may be wrong. Data the user cannot afford to lose must not hang off one.**
Store a value against the thing you directly observed; derive the grouping at read time, where
revising it costs nothing.

The distinction is not about confidence, it is about **what happens when the inference
changes**:

| | Attach state to it? |
| --- | --- |
| *this file*, *this device*, *this user reported this timestamp* | **yes** — it is what happened |
| *this is the director's cut*, *these two files are the same cut*, *this item is a horror film* | **no** — it is what we concluded, and we may re-conclude tomorrow |

**This is [§9](#9--sensors-report-the-core-decides) pointed at rows instead of components**, and
it is [D59](../decisions/working-notes.md)'s *"store the observation, not the
interpretation"* promoted out of one decision, because it was violated twice in three days by
the same author who had cited it: [D155](../decisions/working-notes.md) put a playback
position on a Title, [D156](../decisions/working-notes.md) moved it onto an inferred Edition,
and [D157](../decisions/working-notes.md) put it where the player actually was — the File.
Both errors were found by describing an ordinary library, not by review.

**The tell is a migration.** If changing your mind about a classification means *moving* user
data rather than *regrouping* it, the state is attached one level too high. A relabel should
write one column.

**Bites when:** you attach a position, a preference, a rating or a progress counter to an
entity whose identity came from a parser, a provider or a heuristic.

## §14 — A cache may always be wrong, so the path that ignores it is the ordinary one

**Assume every cache breaks, for a reason nobody thought of.** A local copy, a sync cursor, a
delta stream, a memoised result — each is an optimisation that will eventually be stale,
truncated, corrupt, or from a version that no longer exists. Design for that as the normal
case, not as an incident.

The rule that makes it survivable:

> **Nothing may live only in the cache, and the path that does not use it must be one that
> runs every day.**

An escape hatch used once a year is code that has never been tested, invoked at the worst
moment by the person least able to debug it. The fix is not to make the cache more reliable —
that is the hard problem, and network communication makes it harder — it is to make *not
having it* cheap enough to be unremarkable.

**Cheap enough is a measurement, not a hope.** The whole browsable state of the reference
library is **215 KB gzipped and 0.2 s on a 10 Mbit link**
([D166](../decisions/working-notes.md)), which is why the sync design can answer any request
with *"here is everything"* and treat that as an ordinary reply rather than a failure. Where
the fallback turns out to be expensive, that is the thing to fix — not the cache's reliability.

Three consequences that keep showing up:

- **A cache failure must be a performance event, never a correctness one.** If a stale cache
  can produce a *wrong answer* rather than a *slow* one, it is not a cache.
- **Let it be lossy on purpose.** A journal that may be incomplete can be capped, truncated,
  dropped on restart or never written; one that must be authoritative needs durability,
  ordering, retention and compaction. The second is a project; the first is a table.
- **Partial is usable.** Nothing should wait for a cache to be *complete* before a person may
  interact — the copy fills in behind a product that already works.

**This is [§11](#11--a-value-on-a-hot-path-is-cached-against-a-known-event-not-computed-per-request)'s
other half.** §11 says what to cache and how to invalidate it; this says what must remain true
when the invalidation is missed — and [D161](../decisions/working-notes.md)'s reason for
preferring cache-and-invalidate over incremental maintenance is exactly this test applied to
one mechanism: *a missed invalidation is stale, a missed update is wrong.*

**Bites when:** a client cannot start without a sync, an error path exists only for "cache
unavailable", or the answer to "what if this is stale?" is a guarantee rather than a fallback.

## §15 — A default must be safe to be wrong and visible where it acts; otherwise, ask

A default is a decision made on someone's behalf without asking them. That is fine when being
wrong costs nothing and the person can see it happening. It is a trap when it does something
consequential they never chose and only find out about afterwards.

So two tests, and a default must pass both:

- **Safe to be wrong.** If the default turns out not to be what this person wanted, does anything
  get lost, exposed, deleted, overwritten, fetched, or spent? If yes, it is not a default; it is a
  question.
- **Visible where it acts.** The effect is labelled at the point the person meets it — "from NFO",
  "default", "not fetched" — not only on a settings page they have no reason to open.

Where a choice is consequential, **ask at the moment it first matters** (creating a library,
adding a device, the first remote sign-in), state the effect in the question, and show the answer
wherever it acts afterwards. Asking once is cheaper than a support thread, and far cheaper than a
loss.

**The field's examples.** Home Assistant's backup-before-update is **off by default** since
2025.2 — the release notes' own mitigation is to show *"the last time you made a backup"* beside
the switch, which is the visibility half of this rule arriving after the fact. And Plex's
*"Empty trash automatically after every scan"*: harmless while a network share never drops; when
it drops, a scan flags every file on it as deleted and the trash is emptied, taking the entries'
metadata with it — a consequential behaviour a person turns on without the question stating that
effect. *(Whether it was ever on by default is not verified here.)* [D34](../decisions/working-notes.md)
and [D167](../decisions/working-notes.md) — *absence is never evidence* — are this principle's
first application; [D183](../decisions/working-notes.md) states it.

**How this project keeps a default visible.** Since [D200](../decisions/working-notes.md) nothing
has a default in code: every opinion is a setting row seeded with its **starting value**, or a
config key with one, and both are shown beside their starting value, marked when a person chose
them, and listed by `doctor` when they run on a starting value
([plans/settings.md](../design/plans/settings.md)). A starting value must still pass both tests
above; a consequential one is a question, which starts unanswered.

**Bites when:** a setting's default is chosen because it is "what most people want", without
asking what it does to the people who wanted something else.

## §16 — People are responsible for what they play; we never supply, locate, unlock or promote it

**We build a player for media people have the right to use, and we do not police them.** No
filtering, no fingerprinting, no guessing from filenames, no phoning home. That is the owner's
position, and it is also the legally stronger one: what makes a tool's author liable is **what the
author says and bundles**, not what users do unprompted — and monitoring would create the very
knowledge that liability rests on ([D187](../decisions/working-notes.md),
[research/legal.md](../design/research/legal.md)).

So every feature, integration, screenshot, example and sentence of documentation passes one test:

> **Does this supply content, locate it, unlock it, or encourage getting it from somewhere the
> person has no right to?** If so, we do not build it, bundle it, link it or say it.

Canada's s.27(2.4) turns that into a checklist worth running on anything borderline: how is it
**marketed**; what do we **know** of its use; does it have **substantial lawful uses**; could we
**limit** misuse by design; do we **benefit** from misuse; would it be **viable** without it.

**What this is not.** It is not a reason to refuse generic, dual-use compatibility — a webhook
from any tool that writes files, a player that plays any codec. The line is **preconfigured
sources, circumvention and promotion**, not interoperability.

**Bites when:** a feature request says "it would be convenient if it could also fetch…", an example
uses a real film because it was to hand, or a README sentence explains where people "usually" get
their files.

## §17 — A rule that nothing runs is a hope

**Put each rule at the leftmost place that can hold it: the type system, then a generator, then
a lint rule, then a test, then a hook — and prose last, naming the mechanism that enforces it.**
Prose is read; it is not applied. A rule only holds when breaking it makes something fail.

The evidence is unusually direct. yoloAI measured its own guidance: its factual-accuracy
principle *"fired zero times in six"*, and its log of agent failures concludes *"Execution
catches my errors. Reading does not."* Three of its specimens broke a rule that was written in
the very file being edited (A9, A28, A30). This tree has its own: §13 exists because
[D59](../decisions/working-notes.md)'s *"store the observation"* was broken twice in three days
by the author who had cited it ([D155](../decisions/working-notes.md),
[D156](../decisions/working-notes.md)).

**Documentation is where this bites hardest, because nothing executes it.** A yoloAI design doc
said, in bold, "no exceptions". It was false, and the owner and an agent both built on it. So a
load-bearing claim about what the code does — one where, if the sentence were false, a reader
would build the wrong thing — is backed by a test named `TestArch_<Claim>` and cites it by name,
and a gate checks that every cited test exists and every such test is cited
([D190](../decisions/working-notes.md)). *"This is a judgement, not a claim"* is the right answer
for most sentences; only checkable facts about the code need a test.

Where a rule cannot be mechanical, say so: every rule in `../standards/` names its mechanism or
says it is review-only, so nobody mistakes a hope for a gate.

**Bites when:** you write "must", "never" or "always" in a doc, a comment or `AGENTS.md`, and
nothing would fail if the code did the opposite — or you answer a repeated mistake by rewording
the rule.

## §18 — A check you have never seen fail proves nothing

"Denied", "not found", "0 dropped", "skipped" and "passed" are the cheapest results there are:
almost any broken setup produces them. **So before trusting a pass, see the same check fail on a
case where failure is certain** ([D190](../decisions/working-notes.md)). Three forms of it keep
recurring:

- **A missing prerequisite fails; it does not skip.** A yoloAI test tier was gated on an
  environment variable nothing set, and skipped itself for months on the only platform that
  could run it; building the gate that caught it found two security tests that had never run.
  Only a CI step's or lab job's named `ORPHEUM_TEST_UNCONTROLLED` list may be absent, and a test
  that cannot run on a platform is listed as uncovered, never silently passed.
- **A negative result needs a positive sibling, and empty is invalid.** A yoloAI run reported
  *"7 PASS / 0 FAIL"* and concluded the inverse of the truth, because the sandbox lacked the
  capability under test. Its guard for a security invariant passed on a shell parse error
  (A22). Our escape test first opens the forbidden path unconfined and sees it succeed
  ([D112](../decisions/working-notes.md)).
- **A fixture is built the way the real caller builds it.** Six yoloAI tests certified a function
  no backend could reach, because the fake had a combination of capabilities no real backend has
  (A15); another fix passed every test and did nothing, because the tests supplied a dependency
  the real caller lacks (A27). Red-on-revert shows a test is wired to the code, never that the
  code is wired to the program. The conformance suite is the check no fake can satisfy.

**Bites when:** a test passes on a refusal, an absence, a zero or a skip; a fixture hands the code
something its real caller would not have; or a gate has no test that makes it fail.

## §19 — A defect is a sample: look for its siblings before fixing it

**Before fixing a defect, search for its shape** — the same kind of fault on a sibling path,
another implementation, the other half of a pair. If it recurs, the design is generating it and
the instance is a symptom. If a sibling path already solves it, converge on that one and delete
the divergence, rather than adding a second mechanism for one job.

yoloAI found four defects in one workstream with one shape — a capability implemented on some
backends and not their siblings — and one backend was already immune because it did the job
differently. The durable fix was one conformance case every backend must pass, not four patches
(DF150–DF158). Its rule that a failed subprocess's error carries the tool's stderr was *"re-broken
by every new backend path"* that did not copy an existing one, until it became a single helper.
And a proposed fix for one finding would have rebuilt another by hand (A4).

**Naming the pattern is mandatory; acting on it is a decision.** A finding that says "this is the
fourth of its kind" is a complete response when rewriting the design mid-fix is not justified.
§5 is this principle applied in advance to the families we already know will multiply.

**Bites when:** you find yourself writing "a mechanism on one path and not its sibling", or a fix
introduces a helper whose name is close to one that already exists.

## §20 — A gate changes on purpose, never to land a change

The lint configuration, a complexity threshold, a frozen baseline, a golden file, a tolerance and
a test's assertion change **only as the subject of their own change, reviewed by the owner** —
never as the step that turns the current change green. A suppression states why the finding does
not apply here; "the linter complained" is not a reason. Complexity findings are never
suppressed: the answer is a named function.

Loosening the gate is the cheapest route to green, and agents take the cheapest route:
ImpossibleBench (arXiv 2510.20270) documents agents that *"delete failing tests rather than fix
the underlying bug"*. In yoloAI a lint fix
narrowed a test to satisfy a security linter, and on macOS the test silently stopped testing
anything (A19); elsewhere "extract a function" drifted into "silence the linter" until a gate
banned complexity suppressions. [D110](../decisions/working-notes.md) already says re-basing a
baseline is a reviewed act, not a flag somebody runs; this extends it to every gate. The mechanism
is a pre-edit hook that refuses an agent's edit to a gate file and says what to do instead
([cut-a.md](../design/plans/cut-a.md) §8, M0).

**Bites when:** the quickest way to a green check is editing `.golangci.yml`, adding a `//nolint`
or an `eslint-disable`, re-recording a baseline or golden, widening a tolerance, or weakening the
assertion itself.

## §21 — An owner's correction is a specimen, and it is written down

When the owner contradicts a claim, or asks a question whose honest answer turns out to be "I was
wrong", **record it in [`agent-failures.md`](../agent-failures.md) before moving on**: what
happened, what caught it, and whether a gate now exists. The trigger is the correction, which is
an event anyone can see — not a feeling of uncertainty, which a confident mistake never produces.

"Be more careful" is the one conclusion known not to work. yoloAI's log found that almost every
specimen was caught by the owner rather than by the agent or a gate, and the gates it later built
against the agent's own habits exist only because someone wrote the specimen down. The **Caught
by** field is the one that says what to build next.

**Bites when:** the owner asks "what did you run?" or "why does that need X?", and the answer
changes what you told them.

## §22 — Untrusted data is cleaned where it enters; what must stay raw stays inert

**Every value an attacker can influence is cleaned at the boundary where it enters the system**,
so nothing inside ever has to ask whether it is safe: filenames and paths, NFO and provider text,
probe output and track tags, a node's error text, subtitle files, and whatever a person types.
Past the boundary a string is a clean string, and every consumer — the pass, the store, the API,
a log line — may use it as one. Cleaning later, at each place text is shown, means one missed
place is a hole, and the unclean value travels through everything in between.

**When the original must be kept — a filename's bytes to open the file, an NFO's bytes to parse
again — it is kept in a form that cannot be mistaken for text**: `[]byte` in Go, a `BLOB` in the
database, never a `string` that a formatter, a template or a log could print as it stands. The
raw form's only readers are the ones that need the original: opening the file, parsing it
again.

The one sanitiser is `internal/core/textindex` ([D211](../decisions/working-notes.md)); its
cousins are D49's NFC and the node's parse-don't-validate of FFmpeg output.

**Bites when:** a value is "only shown in the admin view", "only logged" or "cleaned where it is
displayed"; or when keeping the original as a `string` is one line shorter.
