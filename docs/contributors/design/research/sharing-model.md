> **ABOUTME:** Giving one person access to part of a library — what Plex solved and paywalled,
> what Netflix built in the opposite direction, and why the rules engine turns out to be the
> mechanism.

# Sharing Model

Researched **2026-09-19**. The identity half was already covered in
[remote-access.md](remote-access.md) §6; this is the access-control half.

## 0. The state of the field

| | Granularity | Notes |
| --- | --- | --- |
| **Plex** | library + **label** allow/exclude + content-rating ceiling | Solved, and **Plex Pass only** |
| **Jellyfin** | library only | Users symlink subsets into fake libraries to fake finer grain |
| **Immich** | albums + a partner | *Better sharing in Immich* — **1,020 reactions**, under a feature freeze |
| **PhotoPrism** | none | *Multi-user / multi-library* — **844 reactions, open since 2019** |
| **Netflix** | — | Spent the decade building machinery to **prevent** it |

## 1. Plex has the only working answer, and it is a filter

Plex Pass subscribers can restrict a share by **content rating** and by **label**: apply labels
to items, then allow or exclude a shared user by label. If both a rating ceiling and labels are
set, **both apply** — and with no rating set, only the labels do. Managed users (household,
no separate account) take a preset profile; Friends (their own Plex account) take the same
restrictions.

The structural observation is the useful one: **a Plex share restriction is a predicate over
items.** Not a list, not a directory — a filter that is re-evaluated.

Which means we already have the mechanism. [D75](../../decisions/working-notes.md) just
specified a typed predicate language with a closed field registry. **A share is a grantee plus
a predicate**, and "a library", "a collection", "everything except the label `private`" and
"this one film" are all predicates. Granular sharing stops being a feature and becomes a
binding of something already built.

That also answers, for free, two of the questions
[the plan](../plans/sharing-model.md) posed:

- **What happens when a shared item moves?** Nothing — a predicate re-evaluates, and item
  identity already survives a move ([D7](../../decisions/working-notes.md)).
- **When it is deleted?** It leaves the share, because it leaves the result set. An item *list*
  would have required cleanup; a predicate does not.

## 2. Netflix built the exact opposite, and it worked commercially

Worth recording precisely, because it is the thing our users are running from and because it is
a genuine commercial success rather than a cautionary failure.

Netflix's household enforcement uses **IP address**, **device IDs**, and **Wi-Fi network**:
*"the main TV defines the official household network, and phones, tablets and laptops must
occasionally connect to the same Wi-Fi to remain verified."* Stream from outside and a
four-digit code is emailed **to the account owner**, with fifteen minutes to enter it. Sharing
outside the household costs **$7.99/month per extra member slot**, introduced May 2023.

And it worked: **19 million new subscribers** in the months after.

Two conclusions, and they point the same way:

- **They must do this because they license content per household.** We do not license anything
  — the user owns the files. So the constraint that forces the entire apparatus simply does not
  exist for us, and we can give away for free the thing Netflix charges $7.99/month to permit.
- **The apparatus itself is the grievance.** "Your phone must occasionally connect to the
  living-room Wi-Fi" is a requirement imposed on someone's *life*. We should state plainly that
  we never verify households, never count devices and never check IP addresses for entitlement
  — not as a promise of leniency, but because we have no reason to.

## 3. Sharing is intent, not enforcement, and saying so is load-bearing

Anyone who can play a file can re-transmit it. No access-control design changes that, and the
alternative is DRM, which [the overview](../plans/../overview.md) rules out.

So a share expresses **what the owner meant**, and the honest framing is a stated one rather
than a technical claim. Practically: **no transitive re-sharing by default** — a grantee cannot
grant onward — but this is a guard rail against accident and drift, not a control against a
determined person. Plex takes the same position.

## 4. Two places the implementation goes wrong

### Share predicates must fail *closed*

[D75](../../decisions/working-notes.md) says a rule referencing a field that no longer resolves
is kept, marked invalid and surfaced — correct for a smart playlist, where "invalid" means the
user sees an error instead of a list.

**For a share, invalid must mean grants nothing.** A migration that breaks a predicate must
never widen access. This asymmetry has to be explicit in the binding, because the same
predicate object is doing both jobs and the safe default differs.

### Enforce at every access path, not just at listing

The classic bypass: the share filter is applied when building the library view and not when
fetching an item by id. The grantee sees a filtered list and can still stream anything whose id
they can guess or remember from before the share narrowed.

This composes with [principles §10](../../principles/README.md) rather than contradicting it —
you need **both** halves. Omit from the manifest so a correct client never asks, *and* deny at
the endpoint because a shared user is a different trust domain from the owner. And §10's
reading of the denial is even more valuable here: a shared user's client requesting something
outside its share is either a bug or a probe, and both are worth surfacing.

## 5. Playback state

It lives **with the media, keyed by the grantee** — the owner's server records that this person
watched to 00:42:11. Simple, and it is the only place that can know.

The temptation is to merge state across servers when someone uses two, and the recommendation is
not to. A client aggregating several servers shows several sources; trying to reconcile
"watched" across libraries that contain different copies of the same film is a correctness
problem nobody needs.

## 6. Open

- **Whether a grantee may have their own rules**, or only consume the owner's shares. Letting
  them build smart playlists over a filtered view is natural and means every predicate is
  evaluated composed with the share predicate — cheap if designed in, awkward later.
- **The default share.** "Everything except items labelled private" is probably what most people
  mean, and defaults here are consequential in a way playlist defaults are not.
- **Whether shares are visible to the grantee as shares.** Knowing you are seeing a filtered
  library is honest and also tells you what to ask for.
