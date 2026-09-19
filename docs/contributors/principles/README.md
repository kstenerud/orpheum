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
(D7). Anything the user authored is mirrored to disk so it survives us. Destructive actions say
exactly what they will destroy.

**Deleting is the one deliberate relaxation, and it is opt-in.** Users do delete things, and a
principle that pretends otherwise gets bypassed rather than followed. So: a writable path is
opt-in **per library** and off by default; deletion is two-phase and the confirmation **names
every file and its size**; it happens in the node, never the core
([D33](../decisions/working-notes.md)); and a retention rule may only ever *mark*
([D75](../decisions/working-notes.md), [D89](../decisions/working-notes.md)). "Destructive
actions say exactly what they will destroy" is the sentence above; this is its first real
consumer.

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

**Bites when:** a component starts withholding, suppressing or correcting its own output
"helpfully"; when you find yourself giving a sensor a threshold, a memory, or a copy of
something the core already knows; or when confinement gets used as a reason a component cannot
report. ([D42](../decisions/working-notes.md), [D73](../decisions/working-notes.md), and the
same reasoning as [D37](../decisions/working-notes.md).)

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

**Bites when:** an entitlement is easier to express as a 403 in the handler than as a branch in
whatever builds the manifest. ([D58](../decisions/working-notes.md), from
[D57](../decisions/working-notes.md).)

## §8 — Factual accuracy in documentation

Verify claims. Date them. Mark what is unverified as unverified, in place, rather than dropping
it or laundering it into confident prose. Never state a count that a gate does not enforce.

**Bites when:** you are about to write a number, a star count, or "X is faster than Y" from
memory.
