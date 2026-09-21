> **ABOUTME:** A stored, re-evaluatable query over library items — smart playlists, smart
> collections, retention policy. One predicate language, three bindings, and a typed field
> registry that is part of the schema.

# Rules engine

- **Status:** PLANNED — researched 2026-09-19 ([rules-engine.md](../research/rules-engine.md), [D75](../../decisions/working-notes.md)). Promoted into Phase 1 by [D20](../../decisions/working-notes.md). Gained a fourth and then a fifth binding the same day — sharing ([D76](../../decisions/working-notes.md)) and profile restriction ([D84](../../decisions/working-notes.md)).
- **Depends on:** —
- **Blocks:** [library-model.md](library-model.md) — the field registry and the tag storage choice are schema

## Why it is urgent rather than merely wanted

It is **schema-shaping**, and `library-model.md` is Phase 1. A rule is a stored query over item
attributes, tags and provider fields, so what a rule can express constrains how all three are
modelled. Designing it after the schema means a migration.

The demand is ~7,000 stars across plugins and companion apps that exist *only* because no server
in this field lets a user express a rule about their library: Kometa (3,416), Maintainerr
(2,271), Janitorr (752), plus Jellyfin's smartlists (363), auto-collections (292), mergeversions
(439), media-cleaner (260) and the official tmdbboxsets (176).

## The shape

**One predicate language, now five bindings** ([D75](../../decisions/working-notes.md),
[D76](../../decisions/working-notes.md), [D84](../../decisions/working-notes.md)). They are the
same stored query at different times with different effects.

| Binding | Effect | Evaluated |
| --- | --- | --- |
| Smart playlist | ordered membership | on read, or off the change journal |
| Smart collection | display grouping | same |
| Retention | **marks** items — never deletes | on a schedule |
| Share ([D76](../../decisions/working-notes.md)) | grants visibility, **fails closed** | on every access path |
| Profile restriction ([D84](../../decisions/working-notes.md)) | limits visibility, **fails closed** | on every access path |

**The last two bind to a *role*, not to a user** ([D130](../../decisions/working-notes.md)).
Both were originally "a grantee plus a predicate"; a role is a named object carrying one
predicate and a closed capability set, and a user is assigned to exactly one. So the two
collapse into one binding evaluated once per role rather than once per person — *"Kids"* is
defined in one place and three children point at it. A share with one grantee is a role with
one member.

The last two were added after the fact and cost nothing, which is the argument for having built
this first. "Rating at or below 12 in the household's system, and not tagged `horror`" is a
kids profile; it is also just a rule.

Retention marking rather than deleting is [D7](../../decisions/working-notes.md) applied:
Maintainerr and Janitorr delete, we produce a state the user reviews and acts on. Same shape as
[D61](../../decisions/working-notes.md)'s quarantine.

## The two artefacts that are schema

### 1. A closed, typed field registry

A map from field name to semantics — type, nullability, multi-valued, and an **`alias`** for
backward-compatible renames. Four properties:

- **Injection defence** — a field resolves or the rule is invalid; no user string becomes a SQL
  identifier.
- **Typed operators** — the legal operator set is a function of the field's type, so bad rules
  fail at parse time. Kometa's per-type modifier table is the model.
- **`alias` is how rules survive schema change**, and there is no other plan that works.
- **Extensible at runtime** — user tags, and **provider-materialised** fields, because a
  vocabulary of local columns only is why Kometa exists outside the server.

**One registry, not two.** Navidrome splits semantics and SQL mapping across two maps with the
same keys — the same drift that cost them a corrective migration in their FTS work. One table,
or a test asserting identical key sets.

Two additions from the library-model review ([D79](../../decisions/working-notes.md)): each
field also records **which level it lives at** (Title, Edition, Version, File), because
`genre = horror AND resolution > 1080` mixes levels and the compiler must know the join. And a
cross-level predicate is **ambiguous without a quantifier** — *any* version over 1080, or
*all*? Both are wanted, so it is explicit rather than defaulted.

### 2. Multi-valued fields and their count operators

"Has genre X" and "has more than three genres" are different operators. Count operators must
exist from the start because **they decide how tags are stored** — which is the
[library-model.md](library-model.md) question this forces.

## Operators

Navidrome's set is the right size:

```
all/and  any/or                              conjunctions
is/eq    isNot                               equality
gt lt    before after  inTheRange            ordering
contains notContains startsWith endsWith     string
inTheLast notInTheLast                       relative time
isMissing isPresent                          nullability
count_gt count_lt …                          multi-valued
matches                                      regex — see below
inRule notInRule                             reference another rule
```

Plus `sort`, `order`, `limit`, `limitPercent`, `offset` on the rule itself — and note that
**the limit has its own selection order**, distinct from display order (Apple: *"limited to 2
hours with songs most recently added"*).

## Things that will bite

**Nullability is declared per field, never inferred.** Coalesce where a LEFT JOIN produces NULL
— play count and rating have no row for a never-played item, so `playcount < 5` without
coalescing *excludes everything never played*. And emptiness is per-field: Navidrome treats the
JSON `[]` as missing for lyrics.

**The compiler has a hard ceiling.** Measured on SQLite 3.46.1: an OR chain fails at **1000
terms**, a join fails at **65 tables**, and they push opposite ways. Use `EXISTS` rather than
joins, batch ORed conditions inside them, and **enforce an explicit complexity bound with a
comprehensible error** rather than emitting SQL the database refuses.

**Rule references are allowed, acyclic, and checked at save.** Apple shipped this and broke it —
since Big Sur a smart playlist depending on others no longer updates. Detect cycles at save and
reject with a named path; failing at evaluation means a rule that silently stops working.

**A rule referencing a missing field is kept, marked invalid and surfaced** with the field
named. A smart collection quietly disappearing after an upgrade is the worst outcome
([principles §7](../../principles/README.md)).

**Live updating is a property of the rule**, as Apple exposes it. Live rules re-evaluate off the
change journal ([D8](../../decisions/working-notes.md)), which already says exactly what
changed, with a debounce.

**Regex is safe here.** SQLite has no built-in `REGEXP` but both pure-Go drivers can register
one; measured ~5× `LIKE` on 50k rows, which is nothing at this scale. And Go's RE2 makes
user-supplied patterns non-catastrophic — `(a+)+$` completes in 16 µs and backreferences are
rejected at compile time — so no ReDoS guard is needed.

## Still open

- **Where the complexity bound goes** — term count, depth, or generated-SQL size. Needs a
  corpus of Kometa-scale rules.
- **Provider-supplied fields.** A field needing a network call cannot be in a `WHERE` clause, so
  providers presumably materialise into columns on refresh. Unspecified.
- **Tags in a join table or JSON.** Navidrome's JSON choice is what created their batching
  problem. A `library-model.md` decision.

## Done when

A rule survives a column rename; a rule referencing a deleted field is visible and named rather
than silently empty; a cycle is rejected at save with the path; a rule too complex to compile
says so in words; a retention rule has never deleted a file; and the same predicate drives a
playlist, a collection and a retention list without three implementations.
