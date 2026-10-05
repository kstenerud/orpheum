> **ABOUTME:** Granular sharing — giving one person access to part of a library — which nobody
> in this field has solved and which is worth solving.

# Sharing model

- **Status:** PLANNED — researched 2026-09-19 ([sharing-model.md](../research/sharing-model.md), [D76](../../decisions/working-notes.md)).
- **Depends on:** [rules-engine.md](rules-engine.md), library-model.md

## The evidence that it is unsolved

- Immich: `Better sharing in Immich` — **1,020 reactions, 259 comments**, under a declared
  feature freeze. The most-reacted issue in the entire competitive survey.
- PhotoPrism: `Sharing: Multi-user / multi-library support` — **844 reactions, open since
  2019-02**.
- Jellyfin: users symlink subsets of their media into separate directories and register each as
  its own library, per user, to fake it. Named on HN as the #1 missing feature.
- Plex: solved it, via the account system everyone resents. That is not a coincidence — the
  account exists *because* sharing needs an identity for the other person.

## The answer: a share is a grantee plus a predicate

Plex is the only working answer in the field and its mechanism is a **filter** — restrict a
shared user by content-rating ceiling and by **label** allow/exclude, both applying together.
Not a list, not a directory: a predicate that is re-evaluated. (Plex Pass only, which is its own
comment on demand.)

[rules-engine.md](rules-engine.md) already specifies a typed predicate language over a closed
field registry, so **sharing is a fourth binding of it** ([D76](../../decisions/working-notes.md))
alongside smart playlists, smart collections and retention — and
[parental-controls.md](parental-controls.md) became the fifth on the same argument
([D84](../../decisions/working-notes.md)), which is the clearest evidence that the shape is
right. "A library", "a collection",
"everything except label `private`" and "this one film" are all predicates.

**And the predicate hangs off a role, not off a person**
([D130](../../decisions/working-notes.md)). **A share is a role with one member** — which
matters as soon as there are two: sharing the same view with three relatives should be one
policy and three assignments, not three predicates that drift apart. The UI need not say
"role" for a one-off share; the mechanism underneath is one.

Two of the questions this document used to pose answer themselves:

- **A shared item that moves stays shared.** The predicate re-evaluates, and item identity
  already survives a move ([D7](../../decisions/working-notes.md)).
- **A deleted item leaves the share**, because it leaves the result set. An item list would have
  needed cleanup.

## The two things to get right

### Shares fail closed

A rule whose field no longer resolves is kept, marked invalid and surfaced —
[D75](../../decisions/working-notes.md)'s rule, correct for a playlist. **For a share, invalid
must grant nothing.** A migration must never widen access. The same predicate object does both
jobs and the safe default is opposite in each, so the binding declares it.

### Enforce at every access path, not only at listing

The classic bypass is filtering the library view but not the fetch-by-id endpoint. Needs **both
halves**, composing with [principles §10](../../principles/README.md): **omit** from the
manifest so a correct client never asks, **and deny** at the endpoint, because a grantee is a
different trust domain from the owner. A grantee's client asking outside its share is a bug or a
probe, and §10 says both are worth surfacing.

## What "sharing" is here, and the rebroadcast grant

[D174](../../decisions/working-notes.md), 2026-09-23, as [D246](../../decisions/working-notes.md)
§1 revised it. **Sharing is inviting someone**: an account, a role, and an invite link. There is
no outsider kind of account; what D174 set apart for one (seeing the household, a finite
download lease, a ceiling) is a capability of a role, added with the feature it governs, so the
admin chooses the granularity.

**Deferred: the rebroadcast grant** ([D177](../../decisions/working-notes.md)). The grantee is
an ordinary user with a `rebroadcast` capability. **His core signs in as him and treats our
server as a node**, our metadata arriving as assertions he may override with his own tags and
rules for his children. **His household plays on derived grants** that only his core can mint:
within his scope, minutes long, unable to mint further, counted against his ceilings, and dead
when he is revoked. His household's play state lives on his server. What it owes now: a grant
may have a parent; a node need not be a local daemon; every item carries its source.

## The law, and what the sharing page shows

[D187](../../decisions/working-notes.md), [research/legal.md](../research/legal.md). Household remote access is private;
**a few invited people are probably private** — the relationship decides, not a number. So the
sharing page shows the owner **how many people outside the household can play the library** — a
display, never an enforced threshold. A **rebroadcast grant** is accepted with terms (personal,
non-commercial, never in a public place, never resold), shows **how many people** it serves, and
stays **non-transitive** — which is legally valuable, not just tidy. No venue mode, no payments
between users, no open sign-up.

## Identity

Already researched — [remote-access.md](../research/remote-access.md) §6. Four options, not
mutually exclusive: server-local accounts (always available, no cross-server identity),
capability URLs (perfect for one film, unattributable for ongoing access), bring-your-own OIDC
(for the technical), and an optional first-party directory (the premium tier). 1 and 2 need
nothing from us and ship first.

## What we deliberately do not build

**No household verification, device counting or IP checks.** Netflix uses IP, device IDs and
Wi-Fi network — *"the main TV defines the official household network, and phones, tablets and
laptops must occasionally connect to the same Wi-Fi"* — with a code emailed to the account
owner, and $7.99/month per extra member since May 2023. It was a commercial success: 19 million
new subscribers after.

We can decline all of it because **Netflix licenses content per household and we license
nothing** — the user owns the files. The constraint that forces the apparatus does not exist
here.

## Sharing is intent, not enforcement

Anyone who can play a file can re-transmit it, and the alternative is DRM, which is ruled out.
So **no transitive re-sharing by default** — a guard rail against accident, not a control
against a determined person. Say so plainly rather than implying a guarantee.

## Playback state

Lives **with the media, keyed by the grantee** — the owner's server is the only party that can
know. **Do not merge across servers**; a client aggregating several shows several.

## What a shared link grants

[D154](../../decisions/working-notes.md), answering half of what was open below. A share is
**a role with one member**, so a shared *account* carries an ordinary `session` grant with
that role's predicate — nothing special. A Watch Together **join link** is different and
deliberately so: a `join` grant with **no subject at all**, because
[D125](../../decisions/working-notes.md) makes a guest a non-principal. It is scoped to one
session and revoked by rotation, ejection or the cap
([D119](../../decisions/working-notes.md)).

What that settles is *"can it be revoked"* — yes, in one row update, and that is the control
rather than expiry. What it does not settle is the entitlement questions below.

## Still open

- **Whether a grantee may build their own rules** over the filtered view. Natural, and cheap
  only if designed in, since every predicate then evaluates composed with the share predicate.
- ~~**The default share.**~~ **Answered by [D184](../../decisions/working-notes.md): there is none.** What an invited person
  can see, and their ceilings, are **asked when the invite is made** and shown on their page —
  exposure is never a default ([principles §15](../../principles/README.md)).
- **Whether a grantee sees that the library is filtered.** Honest, and it tells them what to
  ask for. **Now shared with [D132](../../decisions/working-notes.md)**, which raises the same
  question for a child — *"three new things are waiting for Dad"* is useful, and is the same
  disclosure with a friendlier face. One question, one answer, likely a per-role setting;
  decide it in both places at once or they will diverge.
- **What a grantee may do besides watch.** Presumably never delete
  ([D89](../../decisions/working-notes.md)) and never change metadata, but "presumably" is not
  a design, and the grantee's own play state, preferences and downloads
  ([D87](../../decisions/working-notes.md)) all need a stated owner.
- **The Jellyfin adapter is a sixth access path** ([D105](../../decisions/working-notes.md)) and
  it enforces nothing itself: it acts as the user, so the core resolves entitlement once. The
  rule that makes that safe — an adapter is a login proxy, never a principal — has to hold for
  every tier-2 surface, including the OpenSubsonic one in Phase 7.
- **Watch Together is a fifth access path, and it is deliberately *not* predicate-governed**
  ([D117](../../decisions/working-notes.md)). The session link **is** the
  grant — no account, no per-person predicate — because the motivating case is someone who has
  neither. What keeps that safe is **scope plus revocation**, and both have since been
  sharpened: the link reaches *the current playback*, not a library and nothing addressable by
  id ([D122](../../decisions/working-notes.md)); a **cap on participants** bounds a leak before
  anyone notices; and the owner **rotates the join link** — the old one stops admitting and
  keeps serving everyone already in — rather than tearing the session down
  ([D119](../../decisions/working-notes.md)). Worth recording here rather than only there,
  because it is the one access path in the project that does not compose with this document,
  and that is a choice rather than an oversight.
- **A bandwidth ceiling per share.** [remote-access.md](remote-access.md)'s economics assume
  nobody is pushing a 30 Mbps remux to five houses; the ceiling is the honest alternative to
  discovering the bill.

## Done when

An owner shares a subset with one person in one action; a new item matching the predicate
appears in that share without intervention; a broken predicate grants nothing rather than
everything; a grantee cannot fetch an item outside the share by id, and the attempt is visible;
and nobody has been asked to connect a phone to a particular Wi-Fi network.
