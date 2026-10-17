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

### 1a. The registry carries the content axes, and unknown is not false

[D131](../../decisions/working-notes.md). Seven descriptor axes — violence, fear, sex,
discrimination, drugs, language, dangerous acts — are ordinary registered fields, which is what
lets a role say *"no sex above 2 and no fear above 1"* without new machinery. Two registry
properties they lean on hard, both already specified above: each is **tri-valued**, so
`unknown` never coalesces to `none` (the nullability trap below, in the one binding where
getting it wrong shows a child something); and each carries a **provenance** companion field,
because a descriptor parsed from a `TV-14-SV` certification and one inferred from a genre are
different qualities of fact and a household deciding how much to trust a template needs to see
which it got.

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
([principles §7](../../principles/07-migrations-are-safety.md)).

**Live updating is a property of the rule**, as Apple exposes it. Live rules re-evaluate off the
change journal ([D166](../../decisions/working-notes.md)), which already says exactly what
changed, with a debounce.

**Regex is safe here.** SQLite has no built-in `REGEXP` but both pure-Go drivers can register
one; measured ~5× `LIKE` on 50k rows, which is nothing at this scale. And Go's RE2 makes
user-supplied patterns non-catastrophic — `(a+)+$` completes in 16 µs and backreferences are
rejected at compile time — so no ReDoS guard is needed.

## Coverage, and proposing a rule from examples

[D132](../../decisions/working-notes.md). Two additions, both of which lean on the field
registry rather than adding machinery.

**Coverage is a defined property, not a feeling.** An item is **covered** by a rule when every
field the rule references resolves to a **known** value for it. For the share and
profile-restriction bindings, an uncovered item is **denied and queued** — there is no
"show unknown" toggle, because with one the categorisation loop is never forced. This is the
nullability trap above turned into policy: `unknown` must never coalesce to a value, and in
these two bindings it must never coalesce to *permitted*.

**Every rule shows its current coverage count** ([D134](../../decisions/working-notes.md)).
Cheap, and it answers the two questions an author actually has: *is this rule doing anything?*
and *is it doing far too much?* A rule matching 0 items is usually a typo or a field this
library does not carry; one matching 3,400 of 3,500 is usually not what anyone meant. It pairs
with the preview below without duplicating it — **the preview shows the delta of a change,
the coverage count shows the standing state**. Per
[principles §11](../../principles/11-hot-path-values-are-cached.md) it is a **cached** value, invalidated off
[D166](../../decisions/working-notes.md)'s change journal like the live rules already are — a
cached result, not an incrementally maintained counter
([D161](../../decisions/working-notes.md)).

**Rule suggestion from a selection.** Given a set of items an admin has picked out, rank
candidate terms by how cleanly each separates that set from the rest of the visible set, and
report the collateral both ways:

> `genre = Horror` — excludes 12 of your 14, and also removes 3 you did not select
> `keyword = slasher` — excludes 9 of 14, and removes nothing else

Candidates are drawn **only from registered fields**, which is what keeps this honest: a
suggestion the compiler could not then evaluate would be a demo, and no user string becomes a
SQL identifier. When **nothing** separates the selection, the proposal is to create a **user
tag** and exclude that — the registry is already *"extensible at runtime — user tags"*, so the
escape hatch is a feature this plan already has rather than a second mechanism
([principles §4](../../principles/04-one-way.md)).

## Two things the 2026-09-21 source sweep added

[D149](../../decisions/working-notes.md).

- **External critic and audience scores are registry fields, and they are missing.** There is
  no IMDb, Rotten Tomatoes, TMDb or Trakt score anywhere in this tree — `library-model.md` has
  *user* ratings and nothing external. Maintainerr carries **twelve** such properties (critic
  and audience separately, each with a vote count) and most of Kometa's 129 builders rest on
  them. They are typed registry entries like any other, at the Title level, and they inherit
  the **null problem this document already solved** for play count: an unrated film has no row,
  so `imdb_score < 6` must not silently mean "and also every film nobody scored". A vote count
  is the second field that makes the first usable — `rt_critic < 40` over three reviews is
  noise.
- **Retention wants a surface, and the surface is the product.** **3,301★** across Maintainerr,
  janitorr and media-cleaner. This document's retention binding already *marks*; what those
  tools actually sell is the **grace period** — gather the candidates into a collection
  everybody can see, leave them there while people catch up, and only then act. The
  visible-collection half needs no deletion at all, which is why it does not collide with
  [D144](../../decisions/working-notes.md): it is this engine's existing binding with a page
  on it.

## Three families of binding, not one list

[D164](../../decisions/working-notes.md), 2026-09-22. The five bindings above are not five of
the same thing, and the difference decides three questions below:

| | Binding | Written by | Errors are |
| --- | --- | --- | --- |
| **Authored** | smart playlist, smart collection, retention | a person | the author's problem |
| **Restricting** | share, profile restriction | a person | **a security failure** |
| **Structural** | the library listing (`filing = listed`), Continue Watching, Next Up, the admin inventory view | us | **ours**, on every page |

The third family was never designed for — it accreted, which is the sign the abstraction was
right. But *"a saved query evaluated occasionally"* and *"the `WHERE` clause of every page"*
are not one artefact.

### A restricting rule that cannot compile denies everything, loudly

**The rule above — *kept, marked invalid, surfaced* — is fail-open here.** A predicate that
*limits* visibility, not evaluated, limits nothing: rename a column, forget the alias, and the
`Kids` restriction silently evaporates. So validity is **per family**: an invalid authored rule
stays inert and surfaced; an invalid restricting rule **matches nothing** and its users see an
empty library naming the broken field. An alarming failure beats a silent one.

**And it should never get that far.** A migration that removes or renames a field referenced by
a restricting rule, with no alias registered, **fails the migration**
([principles §7](../../principles/07-migrations-are-safety.md)). The `alias` mechanism exists for exactly this;
this makes forgetting it impossible rather than merely regrettable.

## The complexity budget is one number, and composition spends it

Measured on SQLite **3.53.4** ([D164](../../decisions/working-notes.md),
[research](../research/rules-engine.md)):

| Construct | Maximum | Error |
| --- | --- | --- |
| **`EXISTS` nesting depth** | **29** | *Expression tree is too large (maximum depth 1000)* |
| ANDed `EXISTS` terms | 932 | same |
| OR-chain terms | 997 | same |
| **`IN` list length** | **no failure below 2,097,152** | — |
| tables in a join | 63 | *at most 64 tables in a join* |

Three constructs, **one error**: they all spend from a single expression-tree depth budget of
1000. So:

- **`IN (…)` rather than an OR chain, always** — a factor of two thousand for identical
  semantics.
- **Rule references are bounded at a nesting depth well under 29**, which is the number D75
  asked for and did not have.
- **The budget is enforced on the composed statement**, with structural and restricting
  bindings taking their share first and the authored rule getting the remainder — and the
  error **names which contributor overran**, or the author of a small rule is told their rule
  is too big.
- **The negation form is chosen by whether the query is bounded.** Standalone, `NOT IN` beats
  `NOT EXISTS` (656 µs vs 1.61 ms); inside a page with `ORDER BY … LIMIT 60` it loses by 2.4×,
  because the limit lets the index stop early. Mechanical to apply, and worth more than any
  caching considered here.

## Settled: tags, provider fields, and a cache not worth building

- **Tags live in a normalised `tag` + `item_tag`, with a `tag_count` derived column.** Measured
  against JSON at the reference library: single tag **50×** faster, 3-tag AND **30×**, NOT tag
  **11×**. JSON wins only `count(tags) > n` — and a `tag_count` column wins that outright at
  **66 µs**, 42× the JSON it lost to. Navidrome's JSON choice is what created their batching
  problem and it loses everywhere else too.
- **Provider fields are materialised columns carrying an `as_of`**, flagged as such in the
  registry. No network call is reachable from a predicate, so no rule evaluation depends on a
  third party being up. A never-refreshed value is **unknown**, which the coverage rule above
  already denies in the two restricting bindings.
- **Caching a role's visible set is not worth building.** It bought **7%** (1.227 → 1.142 ms);
  rewriting the negation bought **2.4×** (→ 506 µs). The well-shaped live query beats the
  cached one outright, so this is a table, an invalidation path and a staleness class not
  taken on.

## Done when

A rule survives a column rename; a rule referencing a deleted field is visible and named rather
than silently empty; a cycle is rejected at save with the path; a rule too complex to compile
says so in words; a retention rule has never deleted a file; and the same predicate drives a
playlist, a collection and a retention list without three implementations.
