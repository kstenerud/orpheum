> **ABOUTME:** The server half of offline download — what a downloaded item actually consists
> of, why it is more than a file, and the lease that keeps a prepared rendition alive. The most
> requested client feature in the field.

# Offline download

- **Status:** PLANNED — designed 2026-09-19 ([D87](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §9).
- **Depends on:** stream-service.md, library-model.md, [play-state.md](play-state.md)

## The demand, which is not subtle

| Repo | Issue | Reactions |
| --- | --- | --- |
| Swiftfin | **Local Downloads** | **145** — the most-reacted issue on that tracker |
| findroid | Allow transcoding of multimedia content | 52 |
| findroid | **Transcode downloads to save storage space** | 32 |
| findroid | Downloader suggestion | 25 |
| findroid | Downloading & Deleting Enhancements | 21 |
| findroid | Download storage location | 21 |
| findroid | Batch download a whole season / unwatched episodes | 18 |
| streamyfin | Choosing download location (internal/SD) | 16 |
| streamyfin | Download queue stops when background downloading | 8 |

Our tree already treats the **client** state machine as the hard part
([D29](../../decisions/working-notes.md), [shared-core.md](../research/shared-core.md)) and it
is right about that. This plan is the other half, which was missing entirely.

## A download is a rendition plus a lease

**A rendition**, because two of those requests are a server capability in disguise:
*"transcode downloads to save storage space"* means the download should be **smaller than the
original**. That is the compatibility transcoder ([transcoding.md](transcoding.md) §(a)) used
for a completely different reason, with a different trigger and a different retention policy —
and on a phone it is usually the *only* thing anyone wants.

**A lease**, which is **indefinite by default** and is mostly a **registration**
([D169](../../decisions/working-notes.md)). D87 gave it two jobs under one name and they want
different lifetimes:

| Job | Who it serves | Needs an expiry? |
| --- | --- | --- |
| keep the server's prepared rendition from being reaped | the server's disk | **no** |
| reach a device nobody can contact | revocation | **only where there is something to revoke** |

**The server's prepared rendition is a cache.** Once the bytes are on the device, the server's
copy exists only to make a re-download cheap, so it is governed by disk pressure and may be
evicted whenever — losing it costs a re-transcode, which is
[principles §14](../../principles/README.md) exactly. No reference count, no reaping policy. A
direct-copy download leaves no artefact and never needed one.

**So an expiry is a property of the relationship, not of the download:**

| Whose device | Lease | Why |
| --- | --- | --- |
| the owner's own, on their own library | **indefinite** | there is nothing to revoke |
| a household member under a role | **indefinite by default** | narrowing a child's rules is not usually *"confiscate the phone"*; if it is, the owner says so |
| a **grantee** ([D76](../../decisions/working-notes.md)) | **finite, set with the share** | a share can end, and this is the only thing that reaches them offline |
| a Watch Together guest | **none — no download** | a guest is not a principal ([D125](../../decisions/working-notes.md)) |

What the indefinite lease still does is say **which devices hold what** — for D89's *"2 devices
hold a downloaded copy"* and so the owner can **forget a device** that was lost, sold or wiped,
which is the same act as signing it out. A stale registration is untidy, not dangerous, so a
person clears it rather than a timer.

**And an idled-out session must not orphan a download.** A phone in a drawer for three months
signs in again with its films untouched; tying playback to a session timer would produce the
plane-mode failure [D167](../../decisions/working-notes.md) exists to prevent.

## And it is self-contained, which the font design did not anticipate

[D53](../../decisions/working-notes.md)–[D58](../../decisions/working-notes.md) build an
elegant on-demand font pipeline: Noto served by language, subsets produced at scan time,
embedded fonts fetched only on an explicit opt-in. **Every one of those is an HTTP fetch from
the server.** On a plane, a downloaded film with a Japanese subtitle track renders as tofu.

The same is true of the extracted subtitle tracks, the artwork, the chapter data and
[media-segments.md](media-segments.md)'s segments.

So the **offline manifest enumerates every resource the client must also store**, and a
download is not complete until they are all present. It is one list, produced by the server,
and getting its shape right early is what stops this being a migration:

- the chosen media rendition (and its parts, if the Version is multi-part);
- every subtitle track the user might switch to, **with its font subset**;
- artwork at the sizes the client's offline browse needs;
- chapters and segments;
- enough metadata to render the item without a server.

## Reconciliation

- **Deletions** reconcile through the change journal, which has had no consumer until now —
  **or, just as validly, through a full state fetch, which expresses deletion by absence**
  ([D166](../../decisions/working-notes.md)). The server answers whichever is cheaper; the
  client must handle both, so the fallback is exercised rather than reserved.
- **And the sync cache is never what makes a download playable.** A download is self-contained
  with a lease for exactly this reason — it is the one case where there is no server to ask, so
  it carries everything instead of trusting a cache. If the two ever merge, D166's central
  property is silently lost.
- **Play state** reconciles through [play-state.md](play-state.md)'s event log, which is why
  the log carries the client's own monotonic clock: an offline device's events are ordered
  relative to each other even when its wall clock is wrong.
- **A device that never comes back** loses its lease and its prepared renditions are reaped,
  loudly enough to appear in the job log rather than as a silent disk reclaim.

## What does not need this

A television. [shared-core.md](../research/shared-core.md) already says so, and it is the reason
the offline module is quarantined rather than spread through the clients.

## When the server copy goes away

[D167](../../decisions/working-notes.md), 2026-09-23. **"Deleted on the server" is four
different events and only one of them is about permission:**

| What actually happened | What the user wants |
| --- | --- |
| the owner deliberately deleted the media ([D89](../../decisions/working-notes.md)) | keep my copy; it may be the only one left |
| the file vanished from disk — a drive died, a folder moved | **keep my copy**, urgently |
| a node is offline, a mount is down | keep my copy; nothing happened |
| a mis-scan or a permission change ([D163](../../decisions/working-notes.md)) | keep my copy; *we* were wrong |
| **entitlement was revoked** | remove it, and that is the point |

So the rule is the strong one, and it is
[D34](../../decisions/working-notes.md)'s *"never tombstone on silence"* at the other end of
the wire:

> **Absence is never evidence. Only a positive statement is.** Nothing on a device is destroyed
> because something was missing from a response.

This matters more since [D166](../../decisions/working-notes.md): a full state fetch expresses
deletion by *absence*, and absence cannot tell **deleted** from **unavailable** from **not
yours any more** from **we mis-scanned**. "Absent ⇒ delete my downloads" would make D166's
cheap, exercised-daily fallback a **mass deletion trigger on every client**.

- **Availability gone → the download is `orphaned`**: still there, still playable, and labelled
  *"no longer in the library — this may be the only copy."* The user decides. With indefinite
  leases this is **ordinary operation rather than an edge case**
  ([D169](../../decisions/working-notes.md)) — a download routinely outlives the server's copy
  — so the label reads as a neutral fact, never as a warning.
  [principles §2](../../principles/README.md) governs our own filesystem; a phone is further
  from ours than that.
- **Entitlement gone → the lease is not renewed and the client removes it on expiry** — where
  there is an expiry at all, which is shares ([D169](../../decisions/working-notes.md)).
  Online, entitlement needs no message: it is resolved per request at **6.64 µs**, so
  revocation lands on the next one ([D168](../../decisions/working-notes.md)). **The lease is
  what covers the device nobody can reach**, so the honest statement is the precise one:
  *for your own devices there is no offline revocation guarantee, and there should not be one;
  for a grantee there is, and it is exactly the lease the owner set on the share.*
- **The server distinguishes them, because the client cannot.** The reconciliation reply says
  *"no longer available"* or *"no longer entitled"*. Silence says neither.
- **An orphaned download cannot be repaired.** Partial when the source went means partial
  forever: say so, offer to remove it, and **stop retrying** — a client retrying a fetch that
  can never succeed is a battery bug wearing a progress bar.
- **It may be the last copy, and both ends say so.** D89's confirmation already names every
  file and its size; it gains *"and 2 devices hold a downloaded copy"*.
- **Play state from a removed item still lands.** A phone back from a trip syncs spans for a
  file the server no longer has, and [D165](../../decisions/working-notes.md) keeps the log
  forever — so **no row a user's history references is ever hard-deleted**; it is tombstoned.

Storage is the user's call: *"3 orphaned downloads, 12 GB"* is a screen with a delete button,
and that is the whole reclamation policy for bytes we do not own.

## What the lease actually enforces

[D154](../../decisions/working-notes.md). Worth one honest paragraph, because the alternative
is implying something we do not do. **There is no DRM** ([D3](../../decisions/working-notes.md)),
so **nothing enforces a lease on bytes already sitting on a device.** The grant governs
**acquisition, renewal and re-download** — which is real, and is what makes revoking a
departed housemate's access mean something — and playback of an already-downloaded file is a
client-side courtesy. Say that in the product. A lease presented as enforcement that is not
enforcement is the kind of claim that gets discovered and quoted.

## Done when

A phone downloads a film, goes into aeroplane mode, and plays it with correct subtitles in a
non-Latin script; progress made offline merges without losing either side; **a node going
offline for an afternoon costs a travelling user none of their downloads**, while an item they
are no longer entitled to goes away when its lease lapses; a download can be a smaller rendition
than the original; and a prepared rendition nothing references is eventually reclaimed and says
so.

*(That third clause read "an item deleted on the server disappears from the device on next
contact" until [D167](../../decisions/working-notes.md) — which contradicted the paragraph
above it, conflated availability with entitlement, and would have deleted a traveller's films
because a drive was being replaced at home.)*
