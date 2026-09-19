> **ABOUTME:** A review of the library model against the industry's identifier hierarchy and
> against everything decided since it was written — finding that it is one level short, and
> short at exactly the place the field's permanent bugs come from.

# Library Model Review

Reviewed **2026-09-19**. `library-model.md` was designed 2026-09-17, before roughly thirty
decisions that touch the schema. This checks it against both.

## 1. The industry has a four-level answer and we have three

**EIDR** — the Entertainment Identifier Registry, used across the studios and by Netflix — is
the canonical identifier scheme for audiovisual works, and its object model is a tree of four
record types:

| EIDR level | Meaning | Example |
| --- | --- | --- |
| **Abstraction** | the work in its most general form | *Glass Runner* |
| **Edit** | a **creative cut** — theatrical, director's, a censored broadcast version | *Glass Runner: The Final Cut* |
| **Manifestation** | a **technical representation** — an encoding, a resolution, a language version | that cut, 2160p HDR10 |
| **Collection** | a grouping container — **Series, Season, Compilation** | *Brass Arena*, season 2, a box set |

This is FRBR's Work / Expression / Manifestation / Item applied to film, and it is not
academic: it is the model the industry actually registers against.

Our plan says `Title → Edition → MediaFile`. Mapped across:

```
Abstraction  →  Title
Edit         →  Edition
Manifestation ─┐
Item          ─┴→  MediaFile      ← two levels collapsed into one
Collection   →  (unstated)
```

### Where the collapse breaks, concretely

A user has *Glass Runner: The Final Cut* as **1080p in two parts** (`CD1.avi`, `CD2.avi`) and
also as **a single 4K file**. That is one Title, one Edition, **two manifestations**, three
files.

With `Edition → MediaFile` there is nowhere to record that CD1 and CD2 are one viewing and the
4K is another. Both the multi-version axis and the multi-part axis hang off the same parent and
cannot be told apart — which is **exactly the combination the plan already names as "a permanent
source of long-running bugs in both Jellyfin and Plex."**

### And Jellyfin shows what happens without that level

Read locally, `MediaBrowser.Controller/Entities/Video.cs` carries **four overlapping
mechanisms** for this one problem:

```csharp
public Guid?         PrimaryVersionId        { get; set; }   // back-pointer to the "primary"
public LinkedChild[] LinkedAlternateVersions { get; set; }   // versions in other folders
public string[]      LocalAlternateVersions  { get; set; }   // versions in the same folder
public string[]      AdditionalParts         { get; set; }   // multi-part — a separate axis
public bool IsStacked => AdditionalParts.Length > 0;
```

Two of those are bare arrays of **path strings**, so the parts of a multi-part film are not
rows at all. The cause is structural rather than careless: a `Video` is simultaneously the Edit,
the Manifestation and the Item, so every distinction has to become a side-array on that one
entity. **We are better than this and still one level short of not repeating it.**

### Collections are orthogonal, and they unify more than expected

EIDR keeps Series, Season and Compilation as **Collections**, outside the
Abstraction→Edit→Manifestation chain, rather than as abstractions with parent pointers. That is
worth copying, and it unifies five things we were otherwise going to model separately:

series, season, box set, hand-made user collection, **and smart collection** — where the last
is a Collection whose membership is a predicate rather than a list
([D75](../../decisions/working-notes.md), [D76](../../decisions/working-notes.md)). *A
Collection is either an explicit member list or a predicate*, and everything else about it is
the same.

## 2. Cross-check against the decisions taken since

| Decision | Does the model account for it? |
| --- | --- |
| [D45](../../decisions/working-notes.md) translation split | Yes, but **at which level?** A cut has its own name — see §3 |
| [D47](../../decisions/working-notes.md) sort keys | Title level. Fine |
| [D48](../../decisions/working-notes.md) two FTS indexes | Yes |
| [D49](../../decisions/working-notes.md) NFC path pair | File level. Fine, but the *level* must exist to hang it on |
| [D57](../../decisions/working-notes.md) font opt-in axis | Yes, folded into the track-preference table |
| [D59](../../decisions/working-notes.md) observe vs interpret | **No** — see §4 |
| [D75](../../decisions/working-notes.md) field registry | **Incomplete** — see §5 |
| [D76](../../decisions/working-notes.md) shares as predicates | Follows from D75 |
| [D77](../../decisions/working-notes.md) what is irreplaceable | **Not marked in the schema** — see §6 |
| [D78](../../decisions/working-notes.md) overlay renditions | Cache; nothing to model |

## 3. Watch state and names need a declared level, and the answers are not obvious

**Watch state belongs at the Edition.** Not at the File or the Manifestation — upgrading from
1080p to 4K must not lose your place, and it would. Not at the Title — the theatrical cut and
the director's cut have different runtimes, so a resume position is meaningless across them.
Edition is the level at which "the same content" is true.

(The rollup question is separate and deliberately left: having watched *a* cut arguably means
you have seen the film. Resume position cannot roll up; a watched *flag* could. Decide in the
UI, not the schema.)

**Names attach at Title *and* Edition**, because "Glass Runner" and "The Final Cut" are both
names and both need [D45](../../decisions/working-notes.md)'s translation treatment. So the
displayed title is a **composition** of two translated strings, which affects the sort key
([D47](../../decisions/working-notes.md)) and the FTS row
([D48](../../decisions/working-notes.md)) — both of which currently assume one name per item.

## 4. The core must store the node's raw observation, not only its interpretation

[D59](../../decisions/working-notes.md)'s decisive argument for parsing filenames in the core
was that **a parser improvement must be replayable over the database with every node switched
off**. That is only true if the raw observation is *in* the database.

So the schema must persist, at the File level, the verbatim filename and the raw probe output —
not merely the parsed series/season/episode and the normalised tracks. The plan mentions
"field-level provenance in their own tables", which is adjacent but not the same claim:
provenance says *where a value came from*, and this says *keep the input*.

## 5. The field registry needs a level, and cross-level predicates need a quantifier

[D75](../../decisions/working-notes.md)'s registry carries type, nullability and an alias. With
four levels it also needs **the level each field lives at**, because a rule like

```
genre = horror  AND  resolution > 1080
```

mixes a Title-level field with a Manifestation-level one, and the compiler has to know how to
join them.

Worse, and this is the part that would be discovered late: **the cross-level predicate is
ambiguous.** Does `resolution > 1080` on a Title mean *any* manifestation exceeds 1080, or
*all* of them? Both are wanted — "films I have in 4K" versus "films I have *only* in 4K" — so
the language needs an **explicit quantifier**, the same shape as the tag count operators D75
already requires. Defaulting silently to `any` would be a bug generator.

## 6. Mark the backup-critical tables in the schema

[D77](../../decisions/working-notes.md) divides the database into regenerable and
irreplaceable, and the irreplaceable set is small. Annotating it **in the schema** rather than
maintaining a list in the backup code makes the backup a query and stops the list going stale
the first time someone adds a table.

## 7. What holds up well

Worth saying, since most of this document is corrections. UUID primary keys with
`{provider, provider_id}` in a side table; the change journal and sync cursor from day one;
`Library`/`Item`/`MediaFile` naming rather than `Movie`/`Episode`; membership as real rows with
foreign keys rather than Jellyfin's serialised lists; and the late additions from the i18n round
are all correct and unaffected by the level change.
