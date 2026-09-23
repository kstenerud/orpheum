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

**A lease**, because a prepared rendition that nothing references is garbage and a prepared
rendition that something references must not be reaped. The lease is per device, has an
expiry, and is renewed when the device checks in.

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

- **Deletions** reconcile through [D8](../../decisions/working-notes.md)'s change journal, which
  exists for exactly this and has had no consumer until now — **or, just as validly, through a
  full state fetch, which expresses deletion by absence**
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
non-Latin script; progress made offline merges without losing either side; an item deleted on
the server disappears from the device on next contact; a download can be a smaller rendition
than the original; and a prepared rendition nothing references is eventually reclaimed and says
so.
