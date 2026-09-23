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

**Bites when:** a new dependency introduces a second idiom for something we already do.

## §5 — Combinatorial complexity gets an interface, not a branch

Hardware accelerators, containers, codecs, subtitle formats and metadata providers all multiply.
Each family goes behind one interface with one implementation per member and a capability probe.
`EncodingHelper.cs` is 8,042 lines because the alternative was chosen.

**Bites when:** you add an `if` for a specific accelerator, codec or provider outside that
family's own implementation file.

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
The invalidation signal already exists: [D8](../decisions/working-notes.md)'s change journal
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
metadata refresh — and [D8](../decisions/working-notes.md)'s change journal already says
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

**Bites when:** a setting's default is chosen because it is "what most people want", without
asking what it does to the people who wanted something else.
