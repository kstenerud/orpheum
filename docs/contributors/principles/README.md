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

**Bites when:** a feature would be easier if the server could just write to the media tree, or
if some state existed only in the database.

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

## §8 — Factual accuracy in documentation

Verify claims. Date them. Mark what is unverified as unverified, in place, rather than dropping
it or laundering it into confident prose. Never state a count that a gate does not enforce.

**Bites when:** you are about to write a number, a star count, or "X is faster than Y" from
memory.
