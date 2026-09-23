> **ABOUTME:** Decisions that no longer describe the design — wholly superseded, withdrawn or
> absorbed by later entries — moved out of the live log so it reads as the current design. Kept
> for the history; cite the successor named in each entry's archive note instead.

# Working Notes — archive

Entries keep their original numbers, headings and text, so a citation still resolves. Each opens
with a note naming what replaced it. **Only the live log, this archive and `design/research/` may
cite an archived decision**; everything else cites the successor, and `scripts/check_docs.py`
enforces that. Rules for when an entry moves are in [README.md](README.md).

---

## D5 — Database: PostgreSQL only, shipped in the default compose

> **Archived 2026-09-23.** Superseded by [D31](working-notes.md#d31--sqlite-with-a-pure-go-driver-supersedes-d5): SQLite, not PostgreSQL.

**Decided.** PostgreSQL. Not SQLite. Ship it in the default Docker Compose so installation is
still one command.

**Why.** ([tech-storage.md](../design/research/tech-storage.md) §1) SQLite's single writer lock
collides directly with this workload — a library scan writing thousands of rows while
transcodes and progress updates also write. This is not theoretical: Audiobookshelf #5349 (an
unhandled `SQLITE_BUSY` during a scan took the whole server down), Jellyfin #11589 (`database
is locked` after 10.9), and Jellyfin's `"Database Locked" on LXC containers using ZFS` group
(228 comments — note that SQLite's locking is sensitive to the filesystem, and self-hosters run
ZFS, btrfs and NFS).

The asymmetry decides it: adding Postgres later is a project nobody finishes — PhotoPrism's
Postgres request has **189 reactions and is eight years old**; Audiobookshelf's has 66. Nobody
is asking for the reverse. Postgres also supplies FTS, `pg_trgm` fuzzy matching for the
scanner, JSONB, `LISTEN`/`NOTIFY` for job wakeups without a broker, and `pgvector` if ever
needed — each otherwise a separate dependency.

**Rejected:** SQLite (chosen by Navidrome, Stash and Kavita and working fine for them — the
tiebreaker is the scan-while-streaming write pattern and the ZFS/NFS deployment reality, not
throughput).

**Consequences.** All SQL behind a repository layer, so an embedded engine remains addable.
The install story must stay one command or this decision costs us users.

**Status:** **Superseded by [D31](working-notes.md#d31--sqlite-with-a-pure-go-driver-supersedes-d5)**
(2026-09-18). Re-examined as this status line invited: the concurrency argument does not survive
the actual write rate, most of the capability arguments evaporate, and the three cited failures
were respectively an application bug, unattributed, and a driver issue.

---

## D8 — A change journal and delta-sync cursor exist in the first schema

> **Archived 2026-09-23.** Replaced by [D166](working-notes.md#d166--the-journal-is-an-accelerator-never-a-dependency-the-server-may-always-answer-with-state-and-that-is-the-ordinary-path-rather-than-the-error-path): one journal table, permitted to be lossy, and an accelerator rather than a dependency — the server may always answer with state. Cite D166 for the change journal.

**Decided.** Every syncable table gets a paired audit table recording deletions (UUIDv7 primary
key, indexed timestamp), and the API serves a cursor-based "what changed since X" endpoint
including deletes, from day one.

**Why.** Immich's `*_audit` tables plus `sync.service` are exactly this, and they exist because
offline clients need deletes — an `updatedAt` column cannot express one. We will have offline
clients (download-for-travel is a non-negotiable client feature). Retrofitting a change journal
is a migration over the whole schema.

**Status:** **Reshaped 2026-09-23 by [D166](working-notes.md#d166--the-journal-is-an-accelerator-never-a-dependency-the-server-may-always-answer-with-state-and-that-is-the-ordinary-path-rather-than-the-error-path).**
The table survives; the framing does not. ~~Provisional; low risk, and cheap now versus
expensive later.~~ **Both urgency arguments above turn out to be wrong**, and they are left
standing so the correction is visible: *"an `updatedAt` column cannot express a delete"* is
true only of a design with no fallback — a full-state fetch expresses deletion by absence —
and what cannot be retrofitted is not a journal but **a design that assumed the journal was
authoritative**. Measured, the entire browsable state of the reference library is **215 KB
gzipped, 0.2 s on a 10 Mbit link**, so the server may always answer with state and a journal
only pays below ~2,600 changed items. So: **one journal table rather than a paired audit table
per syncable table**, the journal is permitted to be lossy, and "your cursor is too old" is not
an error but an ordinary reply. Note that this design still has **no consumer until a mobile
client exists** — but it can no longer break one.

---

## D64 — Watch Together makes the remote-access question concrete, and supplies the premium tier's first honest member

> **Archived 2026-09-23.** Its premium-tier argument was withdrawn by [D68](working-notes.md#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay): voice needs no separate relay, because the media server is already the relay. Its other two points are restated there. Relay pricing is open in [D187](working-notes.md#d187--users-are-responsible-for-what-they-play-we-never-supply-locate-unlock-or-promote-it--and-the-store-and-legal-constraints-that-follow).

**Decided** (2026-09-19). Watch-together is the use case that forces
[D25](working-notes.md#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)'s open product
question, and it resolves *what the paid tier is for* even though it does not resolve whether we
operate one.

Three things are needed for the actual scenario — one participant at home, one travelling — and
they have very different costs:

1. **She must reach the media at all.** Already the project's stated open question
   ([remote-access.md](../design/research/remote-access.md)). Watch-together adds no new
   requirement here; it just makes the absence conspicuous.
2. **A rendezvous for synchronisation.** **Free.** If she is connected to the server, the server
   *is* the rendezvous, and [D62](working-notes.md#d62--watch-together-is-a-first-class-feature-and-the-per-client-surface-is-six-methods)'s
   sync channel is the connection that already exists. No new infrastructure.
3. **A path for voice.** Peer-to-peer where NAT allows — remote-access.md's research puts direct
   connection above 90% with Tailscale-style hole punching — and a **TURN relay** for the
   remainder.

That third item is the point. A TURN relay is **unambiguously another machine that somebody must
run**, with bandwidth costs that scale with use, and it cannot be self-hosted by the person who
needs it, because the whole reason it exists is that their network cannot be reached.

Which makes it the cleanest possible test of the README's promise: *"we only charge for things
that genuinely require another machine."* Most candidate premium features fail that test on
inspection. This one passes it without argument, and the ~10% figure means the free path is the
common path rather than a crippled teaser.

**What this does not settle:** whether we operate a service at all. That still implies a
company, uptime obligations and an entity that can be pressured, and declining it remains
coherent — the cost is leaving the headline use case to Tailscale, which is a fine answer for
this project's likely early audience and a poor one for the household that motivated it.

**Status:** Provisional, and the least technical entry in this log. What it fixes is the *shape*
of the answer, not the answer. **Corrected by [D68](working-notes.md#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay)**
— voice needs no separate relay, because the media server is already mutually reachable. The
premium tier's justification is remote access itself, not voice.

---

## D104 — A Watch Together invite is a capability URL scoped to one edition; the group is never a grant

> **Archived 2026-09-23.** Superseded piece by piece: the entitlement rule by [D117](working-notes.md#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays) (the link is the grant), the Edition scope by [D122](working-notes.md#d122--the-session-is-the-object-what-plays-is-mutable-state-on-it-and-a-guest-sees-the-stream-rather-than-the-library) (a guest sees the stream, not the library), the credential by [D154](working-notes.md#d154--one-grant-table-opaque-and-revocable-the-session-credential-never-appears-in-a-url) (a `join` grant), and its open question by [D168](working-notes.md#d168--entitlement-is-resolved-at-the-point-of-use-never-pushed-revocation-needs-no-message-and-the-notification-is-a-latency-optimisation-that-may-always-be-lost) §8 (revocation ends the session). Nothing in it is current.

> **"Keyed on an Edition" is retired by
> [D122](working-notes.md#d122--the-session-is-the-object-what-plays-is-mutable-state-on-it-and-a-guest-sees-the-stream-rather-than-the-library)**
> (2026-09-21). The session is the object and what plays is mutable state on it; the bound on
> a leaked link is that a guest sees **the current stream, not the library**, which is a
> stronger guarantee than scoping to an item and does not depend on enumerating the scope.
>
> **The entitlement half is superseded by
> [D117](working-notes.md#d117--the-session-link-is-the-grant-revocation-is-the-control-and-the-creator-owns-what-plays)**
> (2026-09-20). ~~*Every participant resolves the item through their own entitlement*~~ — the
> link **is** the grant, and a griefer who obtains one is handled by invalidating the session
> and re-forming it, not by requiring everyone to hold an account first. The *scope* half of
> this decision survives and matters more under D117, because it is what bounds the blast
> radius of a leaked link.

**Decided** (2026-09-19). Audit: [audit-2026-09-19.md](../design/research/audit-2026-09-19.md) §27.

[watch-together.md](../design/plans/watch-together.md) says *"anyone in a group may play, pause
and seek"* and never says **who may be in the group**. There is no mention of an invite, a
join authorisation, or an entitlement check anywhere in the plan or the research. Its
`Depends on` line names `stream-service`, `web-client` and `remote-access` — and not
[sharing-model.md](../design/plans/sharing-model.md).

**That fails open, and it fails open in the most natural implementation.** The whole point of
the feature is watching with someone who is not in your house, so the obvious build is "send
them a group link". A group link that admits anyone who holds it is an access path that routes
around [D76](working-notes.md#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed)'s share predicate and [D84](working-notes.md#d84--certifications-are-system-code-with-a-per-system-order-a-kids-profile-is-a-grantee-plus-a-predicate)'s maturity
ceiling — both of which [D76](working-notes.md#d76--a-share-is-a-grantee-plus-a-predicate-and-it-fails-closed) requires to be enforced *at every access
path, not only at listing*, for exactly this reason.

**So the rule: the group is not a grant.** Every participant resolves the item through their
**own** entitlement. A user who cannot see the film cannot see the group —
[principles §10](../principles/README.md), omission rather than rejection: there is no join
button to decline.

**And the invite has a good answer already sitting in the tree.**
[remote-access.md](../design/research/remote-access.md) §6 lists four identity mechanisms, of
which the second is *"capability URLs — perfect for one film, unattributable for ongoing
access"*. A Watch Together invite is precisely that shape, and the weakness named there is not
a weakness here:

- **Scoped to one Edition**, not to a library. The invitee gets the film, not the collection.
- **Time-boxed** to the session, which is what makes "unattributable for ongoing access"
  irrelevant — there is no ongoing access.
- **Revocable by ending the group**, and listed while it is live so the owner can see who holds
  one.

So the feature that most needs an identity story turns out to need the **cheapest** of the four
([D25](working-notes.md#d25--remote-access-is-tiered-and-the-promise-changes-from-no-accounts-to-we-only-charge-for-another-machine)'s unresolved hosted-directory question is not on its critical path),
and the motivating use case — a partner travelling, with no account — is answered without
one.

**Two consequences worth stating.**

- **`watch-together.md` depends on `sharing-model.md`**, which it did not declare. The
  capability-URL mechanism and the entitlement check are both that document's.
- **A grantee's participation is bounded by their share, not by the host's.** If the host can
  see the director's cut and the grantee's predicate only matches the theatrical, they are
  watching different Editions and the group is incoherent — so the group is keyed on an Edition
  and a participant who cannot resolve *that* Edition cannot join. Sharper than it sounds: it
  means the entitlement check happens against the group's item, once, rather than being
  re-derived per participant per action.

**Status:** The capability URL's own design is settled by
[D154](working-notes.md#d154--one-grant-table-opaque-and-revocable-the-session-credential-never-appears-in-a-url):
it is a `join` grant — **no subject, because a guest is not a principal** — scoped to one
session, revoked by rotation, ejection or the cap. Also
open: whether a participant who loses entitlement mid-film (a share revoked, a rule
re-evaluated) is ejected or finishes the film. Ejecting mid-sentence is hostile; not ejecting
means a revocation is not immediate. Probably: finish the session, deny the next one, and say
so — but it is a decision, not an oversight to be discovered at implementation time.
