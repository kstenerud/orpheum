> **ABOUTME:** Age ratings and who can watch what — why a certification is not a number, and
> why a kids profile turns out to be a share with a different name.

# Parental controls

- **Status:** PLANNED — designed 2026-09-19 ([D84](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §6).
- **Depends on:** [rules-engine.md](rules-engine.md), library-model.md
- **Blocks:** [library-model.md](library-model.md) — certifications are schema

## The demand

`jellyfin-androidtv`'s **third-most-reacted issue** is *PIN protection for users* (+56), behind
only the playback rewrite and SyncPlay. This project's whole positioning is a **household** —
"a household of people who are not going to read documentation" — and a household with children
in it cannot use a media server that has no answer here.

## Netflix's model, including the part everyone forgets

Two levels, and the second is the one homegrown implementations miss:

- **Per profile:** a profile-lock PIN, a maturity-rating ceiling, and explicitly blocked titles.
- **Per account:** **a PIN required to create a profile.** Without it a child makes an
  unrestricted profile and every other control is theatre.

Copy both — **but not their vocabulary** ([D125](../../decisions/working-notes.md)). We have no
`Profile` separate from a `User`: Netflix needs one because a profile is created from inside a
shared paying account, and we license nothing. So the two rules land differently and the second
lands harder:

- the profile-lock PIN becomes a **user-level** credential — the thing that stops a child
  picking the parent's row on the television;
- *"a PIN required to create a profile"* becomes **only an admin creates a user**, which is
  strictly stronger and costs nothing, because here there is no self-service creation act to
  gate.

Read "profile" below as "user with a predicate".

## A restriction is a vector, not a ceiling

[D131](../../decisions/working-notes.md). Four real household requirements, and **only one of
them is a number**:

| Who | Must not see | Axis |
| --- | --- | --- |
| Toddler with a rugged tablet | anything but cartoons | **an allowlist**, not a ceiling |
| Tween | *Sex and the City* | **sex** |
| Early teenager | *The Blair Witch Project* | **fear / violence** |
| Relatives | the household's private material | **a tag**, no age at all |

A maturity slider ranks the tween's film against the teenager's, and the household's answer
does not sort. So a role's restriction speaks about four things: an **age ceiling per system**,
**per-axis ceilings**, **genre/keyword/tag predicates**, and **explicit allow and deny lists**.

**The axes are Kijkwijzer's seven** — violence, fear, sex, discrimination,
drugs/alcohol/smoking, coarse language, dangerous imitable acts — because it is the
best-specified system of its kind, PEGI derives from it, and its classifier scores each axis
independently *and then takes the maximum*. **The single number was always a lossy projection
of a vector somebody already computed.**

**The relatives case needs none of the rating machinery**, which is worth noticing: *"not
tagged `private`"* is a pure tag predicate, so it is the only one of the four that will work
perfectly — and the one with the strongest privacy claim, because "private" in a family media
server means home video of the children.

### The incumbent has this data and discards it

US TV ratings carry the axes in the string — **D**ialogue, **L**anguage, **S**ex,
**V**iolence. From Jellyfin's own `Ratings/us.json`:

```json
["TV-14-D","TV-14-L","TV-14-S","TV-14-V","TV-14-DL", … ,"TV-14-DLSV"]
   → { "score": 14, "subScore": 1 }
```

**All fifteen combinations collapse to one value**, so `TV-14-S` and `TV-14-V` are
indistinguishable after parsing — the tween and the teenager get the same answer from a string
that contained the distinction. And their cross-system fallback ends in
`foreach (var dictionary in _allParentalRatings.Values)`: the first match from any of 45
national systems, **in dictionary iteration order**. Ours evaluates every system the item
carries and takes the **most restrictive** — deterministic, and fails closed.

### What is obtainable, and where the design is honestly weak

TMDB's `release_dates` carries certifications per country and a `descriptors` array that is
**untyped in the schema and empty in their own documented example**. US TV strings give the
axes free. Everything else is genre, keyword and our own tags.

So: **use descriptors where they exist, fall back to genre where they do not, and record which
of the two judged each item**, so the household can see where the judgement is weak.
**Unknown is a third state per axis** — "no violence descriptor" means unknown, never none —
which is [D84](../../decisions/working-notes.md)'s unrated rule applied one level down.

## Templates, so the admin does not start from nothing

[D131](../../decisions/working-notes.md). A small shipped set — `Young child`, `Pre-teen`,
`Teenager`, `Guest` — compiled against whatever systems the household's metadata actually
carries. Three rules:

- **A template is a seed, copied into the role and then owned by the household — never a live
  link.** A template refresh that silently widens what a child may see is the worst failure
  available here and would arrive through a routine update. Same reasoning as
  [D7](../../decisions/working-notes.md)'s refusal to let a refresh overwrite a correction.
- **It reports what it hid**: *"hides 412 of your 3,500 items; 190 carry no rating in your
  system and are currently hidden; 47 were judged by genre because no descriptor was
  available."* This is not foolproof and everyone knows it — the product's job is to make the
  approximation **legible** rather than to repeat the warning
  ([principles §3](../../principles/03-explain-the-decision.md)).
- **The youngest gets a different predicate *shape*, not a lower number.** `Young child` seeds
  an **allowlist** — "these six shows" — because "everything rated U" is not what anyone means
  by a toddler's tablet, and an allowlist fails closed by construction. The person least able
  to report a mistake gets the shape that cannot leak.

## Authoring a role is a preview loop

[D132](../../decisions/working-notes.md). Nobody hand-writes a predicate. The admin supplies
**examples**, which is what a person is good at, and the system finds the rule.

1. **Browse the library as the role** — the actual grid, not a report.
2. **Select what is still wrong.**
3. **The system proposes rules** that would exclude them, each with its cost.
4. **Preview the change**, in both directions, before committing.
5. **Or tag the item**, when no existing metadata separates it.

**The preview is nearly free, and it must share the enforcement path.** A preview on its own
code path is worse than none, because it is trusted and it will diverge. It is cheap because
[principles §10](../../principles/10-only-what-a-client-may-use.md) already requires the manifest to be built **per
principal** — "browse as this role" is that construction with a different argument.

**It must not show that person's data.** An admin authoring `Kids` needs to see *what is
visible*, not *what was watched*. Neither Plex nor Jellyfin has preview-as-user at all; the
documented advice is to create a test account and log into it, which exposes everything that
account holds.

**Proposals come only from registered fields** ([D75](../../decisions/working-notes.md)), so no
user string becomes a SQL identifier and every suggestion is expressible in the language that
will enforce it:

> `genre = Horror` — excludes **12 of your 14**, and also removes **3 you did not select**
> `keyword = slasher` — excludes **9 of 14**, and removes **nothing else**

**Preview emphasises what *enters*.** A rule edit that accidentally reveals is the failure this
subsystem exists to prevent, and it is the one a satisfied admin will not look for. Composed
with [D130](../../decisions/working-notes.md)'s rule that editing a role changes every member,
the confirmation has two halves: **whom** it affects and **what** it changes.

### The escape hatch is a tag, not a list of names

When nothing separates the offenders, the admin **adds metadata to the item** rather than
naming it in a rule — *"better than putting bare names in the rules because it will
immediately scale to all rules for all roles"*. Tag a film `scary` once and every role that
excludes `scary` picks it up, including roles that do not exist yet.

It costs no new machinery: [D75](../../decisions/working-notes.md)'s registry is already
*"extensible at runtime — user tags"*. And it is **user-authored data about the work**, so by
[D98](../../decisions/working-notes.md) it is sidecar-able, rides the change journal, and must
survive a rescan — **a parent's classification of their own library should outlive this
project.**

One safeguard: a free-text field produces `scary`, `Scary` and `scarey`, and then a rule
silently under-matches, which fails **open**. So the admin **picks from existing tags by
default**, and creating a new one is a deliberate, distinct act.

**What does not become a tag is anything true about a person.** *"Alice had a nightmare about
this one"* is not a property of the film and must not be written beside it in a shared media
tree; that stays the per-user deny from [D130](../../decisions/working-notes.md), which may
only narrow. The line is [D98](../../decisions/working-notes.md)'s, unmodified.

### Anything the rules cannot judge is hidden, not shown

> An item is **covered** for a role when every field that role's predicate references resolves
> to a **known** value for it. If any referenced field is unknown, the item is not covered —
> and for a restricted role it is **denied and queued**.

This removes the "show unrated items?" toggle for restricted roles, and removing it is the
point: with the toggle, the categorisation loop is never forced and the subsystem decays into
the thing it replaced. **The remedy for an uncategorised item is to tag it**, and that
judgement then applies to every role at once.

**Rules do the bulk; the queue holds the residue.** *"Rating at or below 12 in our system"*
admits thousands in one statement — nobody approves three thousand films one at a time. A
large queue is itself information: the rules are not expressed over fields this library
carries.

**This is the exact inverse of the incumbent's default.** Jellyfin's `UserPolicy()` constructor
sets `EnableAllFolders`, `EnableAllChannels`, `EnableAllDevices`, `EnableRemoteAccess`,
`EnableContentDownloading` and `EnablePublicSharing` all `true` — so a new user sees every
library, and **a newly added library becomes visible to everyone automatically**.

### The queue drains because the blocked person feels it

**The failure mode to design against is a quarantine nobody drains.** A dashboard metric
reaches the admin who is already looking, which is the wrong person —
[D133](../../decisions/working-notes.md) puts an alert on the **launch screen every user
sees**, because the one with the incentive is the one being blocked:

| Role | Message | Action |
| --- | --- | --- |
| Unrestricted / admin | *"28 items need rating"* | rate them; housekeeping |
| Restricted | *"28 items are waiting to be rated"* | **ask** |

**No ask button** ([D134](../../decisions/working-notes.md)). *"Daaaaad! Rate the movie so I
can watch it!"* **is** the mechanism, and it already works — families have communication
mediums and we are not going to beat WhatsApp. Not building it also removes the question of
what a restricted user may put in a message to an admin.

**The alert counts items pending categorisation and never items a rule deliberately excluded.**
*"28 waiting to be rated"* is a queue state; *"28 hidden from you"* is a filter disclosure. For
a child they are nearly the same; **for a grantee they are not**, because the
deliberately-excluded set is the household's private material and announcing its existence
invites the conversation the exclusion existed to prevent.

**And the count must never become a list** ([D134](../../decisions/working-notes.md)). The
privacy argument is the obvious one; the real argument is worse:

> **An unrated item is one we cannot vouch for, and that includes its title.**

A title is derived from a filename, or from a provider match on a filename
([identification.md](identification.md)). For an item nobody has classified the title is
**itself unvetted** — it may be mislabelled, and it may be explicit in the name alone.
Rendering it on a child's home screen shows them the thing the rating would have blocked,
**through the surface built to protect them**.

So: the alert is **a number and nothing else**; a pending item is **absent from that role's
manifest entirely** — not listed, not searchable, not in Recently Added, no artwork, no title,
not addressable by id ([principles §10](../../principles/10-only-what-a-client-may-use.md)); and there is no "show me
what's waiting" affordance, because there is nothing safe to show. The admin's queue does show
titles, which is fine — the admin can already see those items.

**Suppressed during a root's first scan**, where everything is uncategorised and the alert
would read *"3,500 items are waiting"* — noise that trains the household to ignore the surface
permanently. Bulk categorisation is a setup step, not an alert.

**And the count is a maintained counter, not a query.** Measured at 100,000 titles with 7.7%
uncovered: the anti-join with no supporting index **did not complete in 30 s**; with a covering
index it is **43.6 ms**, which already misses the 50 ms home-page target at a single reader
before [D113](../../decisions/working-notes.md)'s "p99 at 5 concurrent" is applied; a
maintained counter is **0.063 ms**, or **692×**. A stale queue stays reportable —
*"14 items have been waiting 30 days"* should surface before a child notices.

## A certification is `(system, code)`, not a number

Netflix's own help text: *"Maturity ratings are either set by Netflix or by a local standards
organization... Different countries use different rating systems."*

BBFC 15, FSK 16, MPAA R, ACB MA15+ and PEGI 18 are not points on one scale, and TMDB returns
certifications as a **list keyed by country**. So:

- a certification row is `(item, system, code)`, many per item;
- **ordering is defined per system**, as a curated data table — there is no authoritative
  machine-readable source covering every national system, so it starts with the handful the
  providers actually return and grows;
- the household declares which system it uses, which is a locale-shaped choice sitting beside
  [D45](../../decisions/working-notes.md)'s language handling rather than a new axis.

**Flattening this fails open**, which is the direction that matters. An MPAA "R" mapped onto a
single integer scale and compared against an FSK ceiling produces a number that is wrong in an
unpredictable direction — and a wrong rating ceiling shows a child something, rather than
hiding something from an adult.

**An item with no certification in the household's system is unrated, not permitted.** Unrated
is a third state and whether to show it is an explicit choice, not a fall-through.

## The access half is a predicate

A kids profile is **a role** — a named policy object carrying one predicate and a closed
capability set ([D130](../../decisions/working-notes.md)) — and therefore a **fifth binding**
of [rules-engine.md](rules-engine.md) alongside smart playlists, smart collections, retention
and sharing. "Rating at or below X in system Y, and not tagged `horror`" is a rule.

**The role is the whole ergonomic point, and it is not a detail.** A predicate attached to each
*child* means three children are three copies of one policy; raise the ceiling in two places
and forget the third and the failure is **open**, silently, in the one subsystem where that is
unacceptable. So: **define `Kids` once, assign three children to it.** Jellyfin is the warning
— `UserPolicy.cs` has 44 per-user fields and no grouping, and their forum records exactly this
complaint.

**And a kids role has `host_watch_together` off.** A Watch Together link admits anyone holding
it ([D117](../../decisions/working-notes.md), [D125](../../decisions/working-notes.md)); that
is a reasonable thing for an adult to choose and not a thing a nine-year-old should be able to
mint. A parental-control design that gates *what a child watches* and not *whom a child can
invite into the library* has covered the easier half.

If one child needs different rules, that is a **different role**, not an override — the one
per-user exception is a **deny** list, because it can only ever narrow.

It inherits D76's two rules, both correct here:

- **It fails closed.** An unresolvable predicate grants nothing. A migration must never widen
  what a child can see.
- **Enforce at every access path, not only at listing** — omit from the manifest so a correct
  client never asks ([principles §10](../../principles/10-only-what-a-client-may-use.md)), *and* deny by id, because a
  restricted profile is a different trust domain from the account owner.

## Two additions from the 2026-09-21 source sweep

[D149](../../decisions/working-notes.md), [D150](../../decisions/working-notes.md).

- **The PIN, resolved.** This plan already notes that *PIN protection for users* is
  `jellyfin-androidtv`'s third-most-reacted issue (+56) — and it is in fact the highest-scoring
  single issue across the whole 932-issue sweep. Jellyfin **removed** its own PIN in 10.9 as
  *"a big security risk especially for administrator accounts"*, and it was right to: theirs
  was a short password that could log an administrator in. The resolution is that these are
  two different objects. **A PIN gates a switch between profiles on a device that is already
  authenticated** — the living-room case of a child tapping a parent's tile — and is never a
  credential. Full rule in [auth.md](auth.md).
- **Common Sense Media is a better parental layer than ours, and it is licensed.** Plex
  exposes `parentalAdvisoryTopics` — per-topic label, a five-point rating, and a **`positive`
  flag so a topic can be a reason *to* watch** — alongside `parentsNeedToKnow`,
  `talkingPoints` and an `ageRatings` list carrying `ageGroup` and a crowd star rating, behind
  Plex Pass. That is editorial per-title guidance where [D131](../../decisions/working-notes.md)
  has national-system descriptors, and the shape is worth copying even where the data is not
  available: **a per-topic score with a sign** is strictly more expressive than a descriptor
  that is merely present, and it is what would let a template say *"strong language, but the
  violence is cartoonish"*. The data itself is licensed, which puts it where
  [D16](../../decisions/working-notes.md) puts every provider: the user's key, or nothing.

## What it is not

Not content filtering, not scene skipping, not scanning. It is a visibility predicate and a PIN.
[content-safety.md](content-safety.md) is a different subject entirely.

## Done when

A child's user shows only what the predicate allows; the same items are unreachable by id;
a new film appears in or out of it according to its certification without intervention; an item
with no certification in the household's system is handled by a stated rule rather than by
accident; **a non-admin cannot create a user** ([D125](../../decisions/working-notes.md)); and
the owner can see exactly why a title is hidden.
