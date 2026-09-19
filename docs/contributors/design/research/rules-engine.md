> **ABOUTME:** Stored, re-evaluatable queries over the library — smart playlists, smart
> collections, retention. What the field registry has to look like, why it is schema-shaping,
> and the database limits a rule compiler actually hits.

# Rules Engine

Researched **2026-09-19**. Sources: Navidrome's `model/criteria` and `persistence/criteria_sql.go`
read locally; Apple's and Plex's user-facing smart-playlist behaviour; Kometa's filter
vocabulary; and **measurement** of SQLite 3.46.1 and Go's `regexp`, flagged as such.

## 0. Why this blocks the schema

A rule is a **stored query over item attributes**, so what a rule can express constrains how
attributes, tags and provider fields are modelled. Designing it after
[library-model.md](../plans/library-model.md) means a migration —
[plans/README.md](../plans/README.md) already says so. The concrete outputs are a **field
registry** and a **type-to-operator table**, both of which are schema.

The demand is ~7,000 stars of plugins and companion apps that exist only because no server in
this field lets a user express a rule ([D20](../../decisions/working-notes.md)).

## 1. Navidrome is the right starting point, and it is small

`model/criteria` is ~1,660 lines including tests; the SQL compiler is another 887.

**The operator set**, complete:

```
all/and   any/or                              conjunctions
is/eq     isNot                               equality
gt  lt    before  after   inTheRange          ordering
contains  notContains  startsWith  endsWith   string
inTheLast notInTheLast                        relative time
inPlaylist  notInPlaylist                     reference another rule
isMissing   isPresent                         nullability
```

And the criteria object carries `Sort`, `Order`, `Limit`, `LimitPercent`, `Offset` and
`RefreshDelay` alongside the expression.

### The field registry is the load-bearing part

Not arbitrary column names — a **closed map** from field name to semantics:

```go
type FieldInfo struct {
    Alias    string // backward-compat alias for another canonical name
    IsTag    bool
    IsRole   bool
    Numeric  bool
    Boolean  bool
    Nullable bool  // isMissing/isPresent allowed here
}
```

Four things this buys, and the last two are easy to miss:

1. **Injection defence.** A field either resolves in the map or the rule is invalid. No user
   string ever reaches SQL as an identifier.
2. **Typed operators.** `Numeric`/`Boolean`/`Nullable` decide which operators are legal, so an
   invalid rule is rejected at parse time rather than failing in the database.
3. **`Alias` is the schema-evolution mechanism.** Rename a column, register the old name as an
   alias, and every saved rule keeps working. Cheap, and there is no other plan that works.
4. **The registry is extensible at runtime** — `AddRoles()`, `AddTagNames()`,
   `AddNumericTags()` pull user-defined tags from `mappings.yaml` into the queryable vocabulary.
   The rule language is not fixed at compile time.

### Nullability is per-field and subtler than it looks

Two mechanisms in the SQL layer, both of which encode a semantic decision:

- **`coalesceDefault`** — `playcount` and `rating` live in an `annotation` table that has no row
  for a never-played track. Without coalescing, `playcount < 5` **excludes** everything never
  played, which is the opposite of what a user means.
- **`emptyValues: ["[]"]`** on `lyrics` — an empty JSON array counts as missing.

And the registry comment states the general rule: *"for numeric/boolean fields, missing means
NULL; for string fields it means NULL or empty string."* Emptiness is declared per field, never
inferred.

### Two registries that must not drift

Semantics live in `model/criteria/fields.go`; the field-to-SQL mapping lives in
`persistence/criteria_sql.go`. **They are separate maps with the same keys.** This is exactly
the shape of the bug that cost Navidrome a corrective migration in the FTS work — one
normalisation implemented twice, drifting ([internationalisation.md](internationalisation.md) §4).
Either one table carries both, or a test asserts the key sets are identical.

## 2. The database limits are reachable, and they shape the compiler — measured

Measured against SQLite 3.46.1:

```
OR chain of  900 terms                      ok
OR chain of 1000 terms   FAIL: Expression tree is too large (maximum depth 1000)
nested parens x1100                         ok   (redundant parens are flattened)
bound parameters x40,000                    ok
join of 64 tables                           ok
join of 65 tables        FAIL: at most 64 tables in a join
```

Both ceilings are reachable by a user with a Kometa-sized rule, and **they push in opposite
directions**:

- A compiler that emits **a JOIN per tag condition** dies at **64 conditions**.
- A compiler that emits **an OR chain** dies at **~1000 terms**.

Which explains something that looks like style in Navidrome and is not: **it uses `EXISTS`
subqueries rather than joins**, and then batches ORed conditions inside them —
`jsonCondBatchSize = 350`, with the comment *"The EXISTS wrapper consumes ~4 levels; each ORed
condition adds 1 level. Empirically, 496 is the maximum."* They also apply De Morgan to merge
negated conditions: `NOT EXISTS(X) AND NOT EXISTS(Y)` becomes `NOT EXISTS(X OR Y)`.

So a rule compiler needs **an explicit complexity bound with a good error**, rather than
emitting SQL the database refuses to parse.

## 3. What the commercial and community tools do

### Apple, and the failure worth noting

- **"Live Updating" is a user-visible toggle.** A smart playlist can be frozen. Re-evaluation is
  a property of the rule, not of the system.
- **`any`/`all` with nesting behind the Option key** — nesting exists and is deliberately
  advanced.
- **The limit has its own selection order**, distinct from display order: *"limited to 2 hours
  with songs most recently added."*
- **Smart playlists referencing other smart playlists broke in Big Sur** — the master playlist
  stopped updating when its dependencies changed. Apple shipped rule-references and could not
  keep them working. Navidrome has `inPlaylist`/`notInPlaylist` and a `ChildPlaylistIds()`
  extractor, which exists precisely to find the references — i.e. for cycle handling.

### Plex

One base `any`/`all`, arbitrary nesting inside it, and smart collections are **always** ordered
by the rule's sort — no manual ordering. Simple and sufficient.

### Kometa, which is where the unmet demand is legible

Six filter categories with **per-type modifiers**:

| Type | Modifiers |
| --- | --- |
| Boolean | presence/absence |
| Date | `.before` `.after` `.not` `.regex` |
| Number | `.gt` `.gte` `.lt` `.lte` `.not` |
| String | `.is` `.isnot` `.begins` `.ends` `.not` `.regex` |
| Tag | `.not` `.regex` `.count_gt` `.count_gte` `.count_lt` `.count_lte` |

Three things to take from this:

1. **Multi-valued fields need count operators.** "More than three genres" is a distinct class
   from "has genre X", and it decides how tags are stored and queried.
2. **Users want regex**, and it appears on every string-ish type.
3. **Users want fields the media server does not hold** — IMDb keywords, TMDB original language
   and vote counts, TVDB status, even after-credits-scene ratings. A vocabulary limited to local
   columns is *why* Kometa exists as an external tool.

And the honest note in their own documentation: *"Filters can be very slow, particularly on
larger libraries,"* with a recommendation to avoid them. **Kometa is slow because it filters
outside the database**, over an API, row by row. That is the whole argument for building this in
rather than leaving it to a companion app: we are the database.

## 4. Regex is safe here in a way it usually is not — measured

SQLite has **no built-in `REGEXP`**; it is an application-defined function, which both pure-Go
drivers can register. Measured on 50,000 rows: `LIKE '%zq%'` 3.3 ms, registered `REGEXP` 17.6 ms
— about 5×, neither index-assisted, and both fine at household scale.

The interesting part is safety. User-supplied regex is normally a denial-of-service surface,
because backtracking engines blow up on patterns like `(a+)+$`. Go's `regexp` is **RE2**, and
measured:

```
(a+)+$    against 40 a's + "X"   → no match in 16 µs
^(a|aa)+$ against the same       → no match in  2 µs
compile (a)\1    → error: invalid escape sequence
compile (?=foo)  → error: invalid or unsupported Perl syntax
```

Linear time, guaranteed, and backreferences and lookahead are rejected at compile time. **So we
can offer user-supplied regex in rules without a ReDoS guard**, which almost no other runtime
could. An unplanned payoff from [D9](../../decisions/working-notes.md).

## 5. Open

- **Where does the complexity bound go** — a term count, a depth limit, or a generated-SQL size
  check? Needs a real corpus of Kometa-scale rules to calibrate.
- **Provider-supplied fields.** Kometa's users want them and our plugin tiers could supply them,
  but a field that requires a network call cannot be in a `WHERE` clause. Probably: providers
  *materialise* fields into the database on refresh, and the registry treats them like any other
  column. Unspecified.
- **Whether tags live in a join table or JSON.** Navidrome uses JSON with `EXISTS`; that choice
  is what forced the batching in §2. A join table has different limits. Undecided, and it is a
  `library-model.md` question.
