> **ABOUTME:** Every feature this project knows about, ranked by how essential it is and
> annotated with what the architecture must keep true so a deferred one stays addable. The
> owner's list — edit it freely; it is the one document here that is a preference, not a
> finding.

# Feature register

Four other documents answer four other questions. This one answers **"how much do we care, and
what does caring later cost?"**

| Document | Question |
| --- | --- |
| [`ROADMAP.md`](ROADMAP.md) | **when** |
| [`contributors/design/plans/`](contributors/design/plans/) | **how** |
| [`contributors/decisions/working-notes.md`](contributors/decisions/working-notes.md) | **why** |
| **this file** | **whether, and how much** |

## How to read it

Every row has a stable **`F`-number** so it can be referred to and struck without renumbering.
Two columns carry the weight:

- **Rank** — how essential. `0` there is no product without it · `1` its absence reads as
  broken · `2` deferred, but the architecture owes it something **now** · `3` deferred and
  free to add whenever · `4` declined. *(Called **rank**, not *tier*: this tree already uses
  "tier" for plugin tiers and corpus tiers, and they are unrelated.)*
- **Retrofit** — **the column that actually matters.** What it costs to add later:
  **free** (additive, mostly client) · **seam** (needs an extension point to exist) ·
  **schema** (needs a column or table, so a migration) · **invariant** (needs some rule *not*
  to have been assumed) · **protocol** (needs a wire change, which is
  [D108](contributors/decisions/working-notes.md)'s promise broken).

**Rank and retrofit are independent, and that is the point.** A rank-3 feature is safe to
ignore. A rank-2 feature may be something nobody wants for two years and still has to be
designed around this month, because `schema` and `invariant` are paid once and paid early or
paid ten times and painfully. **Rank 2 is the only list the architecture has to read.**

**★** marks where we intend to be visibly better than the field, rather than merely present.

## How to edit it

Change a rank, strike a row, add one — it is yours. Two rules keep it from rotting:

1. **A row links to the decision or plan; it never restates it.** If this file and a decision
   disagree, the decision wins and this file is stale.
2. **Striking a row is not the same as closing its door.** Move it to rank 4 and fill in
   *what keeps it possible* — the declined table has that column precisely so that a decline
   is reversible.

## Cut A — what ships first

**[D178](contributors/decisions/working-notes.md):** the first release is for two people — the owner at home and one person remote,
watching together with subtitles. Ranks say how much we care; the cut says what comes first.
**The obligations below are owed regardless of cut** — and only those that would *preclude* a feature if missed; additive tables arrive with their features ([D180](contributors/decisions/working-notes.md)).

In Cut A: F1–F13, F19, F20, F26, F88, F89, F90; F35 for two with voice; F21's cast receiver; F27's
fetch-on-request half; the minimal part of F33 (the last subtitle and audio language chosen in a series
follows its unwatched episodes, [D230](contributors/decisions/working-notes.md); subtitles start off,
[D223](contributors/decisions/working-notes.md) §3); D83's dialogue boost and loudness levelling
from F24 ([D226](contributors/decisions/working-notes.md)); F14's first form, **prepare** with the `remux` and `h264-8m` profiles
([D193](contributors/decisions/working-notes.md)); F15's reachability for one remote person.
Not F30: her television joins a session as a device of her membership, so pairing waits
([D191](contributors/decisions/working-notes.md)). Everything else follows it.
[ROADMAP.md](ROADMAP.md) has the detail; [plans/cut-a.md](contributors/design/plans/cut-a.md) is
the build contract, and Cut A's design is frozen (D193 §8).

---

## Rank 0 — Essential

There is no product without these.

| # | Feature | Where | Retrofit |
| --- | --- | --- | --- |
| F1 | Scan, observe and index a library; files are the source of truth | [D7](contributors/decisions/working-notes.md), [D59](contributors/decisions/working-notes.md), [scanner.md](contributors/design/plans/scanner.md) | — |
| F2 | Identify: filename and provider metadata to a Title | [identification.md](contributors/design/plans/identification.md) | — |
| F3 | The four-level model: Title → Edition → Version → File | [D79](contributors/decisions/working-notes.md) | — |
| F4 | Direct play, ranged `GET` | [D17](contributors/decisions/working-notes.md) | — |
| F5 | Browse, sort and search | [D47](contributors/decisions/working-notes.md), [D48](contributors/decisions/working-notes.md) | — |
| F6 | Users, authentication, sessions, API keys | [auth.md](contributors/design/plans/auth.md) | — |
| F7 | Play state, resume, Continue Watching | [D81](contributors/decisions/working-notes.md) | — |
| F8 ★ | Subtitles: scan-time extraction, fonts, libass everywhere | [D11](contributors/decisions/working-notes.md), [D50](contributors/decisions/working-notes.md)–[D57](contributors/decisions/working-notes.md) | — |
| F9 | Three-axis track negotiation from a manifest | [D2](contributors/decisions/working-notes.md), [D21](contributors/decisions/working-notes.md), [D22](contributors/decisions/working-notes.md) | — |
| F10 | Artwork: derive, size, serve, cache | [artwork.md](contributors/design/plans/artwork.md) | — |
| F11 | A web client | [web-client.md](contributors/design/plans/web-client.md) | — |
| F12 | Backup and a tested restore | [D77](contributors/decisions/working-notes.md) | — |
| F13 ★ | Media nodes: the core never touches a media file — the node does every media read and FFmpeg run, and the core relays ([D193](contributors/decisions/working-notes.md)) | [D26](contributors/decisions/working-notes.md), [D33](contributors/decisions/working-notes.md), [D99](contributors/decisions/working-notes.md) | — |

## Rank 1 — Expected

Absent, the product reads as unfinished to anyone arriving from Plex or Jellyfin.

| # | Feature | Where | Retrofit |
| --- | --- | --- | --- |
| F14 | Compatibility transcoding, ~~as a plugin~~ **as a node capability** ([D193](contributors/decisions/working-notes.md)); Cut A's prepare job is its first form | [D19](contributors/decisions/working-notes.md), [transcoding.md](contributors/design/plans/transcoding.md) | seam |
| F15 ★ | Remote access for non-technical family | [D14](contributors/decisions/working-notes.md), [D25](contributors/decisions/working-notes.md), [D71](contributors/decisions/working-notes.md) | — |
| F16 ★ | Roles and parental controls, authored as a preview loop | [D130](contributors/decisions/working-notes.md)–[D134](contributors/decisions/working-notes.md) | schema |
| F17 ★ | Collections, explicit and predicate — **and every one produces a sequence**, so Next Up and binge affordances work over a franchise or a composed pair of series exactly as over a season ([D160](contributors/decisions/working-notes.md)). No incumbent does this: they all have series machinery plus a static poster grid | [D75](contributors/decisions/working-notes.md), [D79](contributors/decisions/working-notes.md), [D159](contributors/decisions/working-notes.md), [D160](contributors/decisions/working-notes.md) | schema |
| F18 | Episode orderings: aired, DVD, absolute | [D96](contributors/decisions/working-notes.md) | schema |
| F19 | Extras, trailers, theme songs and backdrops | [D97](contributors/decisions/working-notes.md), [D162](contributors/decisions/working-notes.md) | schema |
| F20 | Multiple versions and editions of one work | [D79](contributors/decisions/working-notes.md) | schema |
| F21 | Casting to a television | [D135](contributors/decisions/working-notes.md)–[D137](contributors/decisions/working-notes.md) | — |
| F22 | Offline download | [D87](contributors/decisions/working-notes.md), [offline-download.md](contributors/design/plans/offline-download.md) | seam |
| F23 | Intro and credit segments, typed and provenanced | [D82](contributors/decisions/working-notes.md), [D115](contributors/decisions/working-notes.md) | schema |
| F24 | Loudness measurement and gain | [D83](contributors/decisions/working-notes.md) | — |
| F25 | Trickplay and thumbnails | [D102](contributors/decisions/working-notes.md), [D114](contributors/decisions/working-notes.md) | — |
| F26 | Manual identify and per-field metadata override — including the chooser for an ambiguous match and the one-click refresh of thin metadata ([D172](contributors/decisions/working-notes.md)) | [identification.md](contributors/design/plans/identification.md), [D98](contributors/decisions/working-notes.md), [D170](contributors/decisions/working-notes.md) | schema |
| F88 ★ | **"What the scanner saw"** — every observed file carries a `filing` with a reason, so the admin can browse what was skipped, errored, filed as an extra or left unidentified, look inside a subtree that was never walked, play it, and say what it is. No incumbent can do this: their skip is a LINQ filter and there is nothing to query ([D163](contributors/decisions/working-notes.md)) | [D40](contributors/decisions/working-notes.md), [D163](contributors/decisions/working-notes.md) | schema |
| F27 ★ | Subtitle acquisition, verified against the audio before it is offered | [D147](contributors/decisions/working-notes.md), [D148](contributors/decisions/working-notes.md) | schema |
| F28 | Caption presentation controls (statutory floor) | [D90](contributors/decisions/working-notes.md), [accessibility.md](contributors/design/plans/accessibility.md) | schema |
| F29 ★ | NFO read, write and export — leaving is supported | [D141](contributors/decisions/working-notes.md)–[D143](contributors/decisions/working-notes.md) | — |
| F30 | Device pairing / signing in on a television — **not in Cut A** ([D191](contributors/decisions/working-notes.md)) | [D93](contributors/decisions/working-notes.md) | — |
| F31 | **Bandwidth and concurrent-stream ceilings, per role** | [D149](contributors/decisions/working-notes.md), [auth.md](contributors/design/plans/auth.md) | **schema** — ~~Phase 1~~ additive, arriving with the feature and before any relay we run ([D180](contributors/decisions/working-notes.md), [D193](contributors/decisions/working-notes.md)); the paid relay has no ceiling without it |
| F32 | Client-side adaptation on a buffer signal | [D86](contributors/decisions/working-notes.md) | protocol |
| F33 | Language and track preferences, sticky per series | [D24](contributors/decisions/working-notes.md), [D45](contributors/decisions/working-notes.md) | schema |
| F34 | A TV client | [tv-client.md](contributors/design/plans/tv-client.md) | — |
| F35 ★ | Watch Together — **two people, remote, with voice, is the priority**: directly after the web client ([D175](contributors/decisions/working-notes.md)) | [D62](contributors/decisions/working-notes.md)–[D72](contributors/decisions/working-notes.md) | see F52 |
| F36 ★ | Scan legibility: named, deferrable, separately reported passes | [D102](contributors/decisions/working-notes.md), [D118](contributors/decisions/working-notes.md) | — |
| F37 ★ | Honest refusals — every no names its reason | [D92](contributors/decisions/working-notes.md), [D152](contributors/decisions/working-notes.md), [principles §3](contributors/principles/README.md) | — |
| F89 | **Now playing** — every live session: who, what, device, where (LAN/remote/relay), direct or transcode with its reason, bitrate, client buffer health; stop with a message. Admin sees all, others their own ([D176](contributors/decisions/working-notes.md)). *Bitrate against the ceiling and lowering a ceiling arrive with F31, after Cut A* | [D176](contributors/decisions/working-notes.md) | — |
| F90 | **Invite someone outside the house** — an account plus a role, redeemed from an invite link, seeing nothing of the household. *A finite download lease and a ceiling arrive with F22 and F31, after Cut A.* What "sharing" means here ([D174](contributors/decisions/working-notes.md)) | [sharing-model.md](contributors/design/plans/sharing-model.md), [D76](contributors/decisions/working-notes.md), [D174](contributors/decisions/working-notes.md) | — |
| F38 | Jellyfin-compatible adapter | [D18](contributors/decisions/working-notes.md), [jellyfin-api-shim.md](contributors/design/plans/jellyfin-api-shim.md) | seam |

---

## Rank 2 — Deferred, but the architecture owes them something now

**This is the list to read before freezing a schema or an interface.** Nothing here is
scheduled; everything here is expensive to retrofit if the obligation is missed.

| # | Feature | Demand | Retrofit | **What must be true now** |
| --- | --- | --- | --- | --- |
| F39 | **Play statistics and watch history** | **9,196★** | schema | [D81](contributors/decisions/working-notes.md)'s play-event log must be **retained**, not truncated — **settled: no horizon, 13.4 MB over five years** ([D165](contributors/decisions/working-notes.md)). The surface is additive. [play-statistics.md](contributors/design/plans/play-statistics.md) |
| F40 | **External critic/audience scores** — built in: TMDB, and OMDb for IMDb, RT critic and Metacritic; shown by a ranked list with vote counts ([D171](contributors/decisions/working-notes.md), [D172](contributors/decisions/working-notes.md)) | basis of most of Kometa's 129 builders | **schema** | A Title-level `(source, kind, score, vote_count, as_of)` table, with **critic and audience separate**. A single `rating` column destroys it. Due before [library-model.md](contributors/design/plans/library-model.md) freezes |
| F41 | **External list import** (IMDb/Trakt/Letterboxd → a Collection) | Kometa **3,414★**, 22 services | **invariant** | Collection membership must not be modelled as *explicit XOR predicate*. There is a **third kind** — authored elsewhere, refreshed on a schedule. [external-lists.md](contributors/design/plans/external-lists.md) |
| F42 | **Admin-curated home rows, per role** | 513★ | schema + invariant | The client must **fetch its home layout from the server**, not compile it in. Row *order* is already user state ([D138](contributors/decisions/working-notes.md)); the row *set* needs a table |
| F43 | **Retention surface** — candidates in a visible collection with a grace period | **3,301★** | free | [D75](contributors/decisions/working-notes.md)'s retention binding already marks. Needs no deletion ([D144](contributors/decisions/working-notes.md)) |
| F44 | **Scored, signed content descriptors** (Common Sense-shaped) | Plex Pass only | **schema** | [D131](contributors/decisions/working-notes.md)'s descriptor vector must carry a **score and a sign**, not mere presence — *"strong language, but cartoonish violence"* is unrepresentable otherwise. The data is licensed ([D16](contributors/decisions/working-notes.md)); the shape is not |
| F45 | **Lyrics** | Jellyfin ships a whole fetcher subsystem | invariant | The track model must not hard-code exactly three axes. A lyric is timed text that is not a subtitle |
| F46 | **Playback speed** — *obligation met by [D90](contributors/decisions/working-notes.md): local unless a Watch Together group shares it* | +25 | **invariant** | Rate is **either** part of Watch Together's shared session state **or** explicitly local ([D66](contributors/decisions/working-notes.md)). There is no third option, and deciding late means a reconciler that fights the user |
| F47 | **Two-factor / passkeys** | 0 measured, and an expectation anyway | seam | The session model must not assume a single factor. Nothing else |
| F48 | **A PIN that selects a profile** | **+56 — the loudest single request found** | invariant | A PIN gates a switch on an **already-authenticated device** and may never authenticate. Jellyfin removed theirs for getting this wrong ([D150](contributors/decisions/working-notes.md), [D152](contributors/decisions/working-notes.md)) |
| F49 | **AirPlay and other receivers** | +11 | invariant | The playback-role abstraction ([D124](contributors/decisions/working-notes.md), [D135](contributors/decisions/working-notes.md)) must stay **protocol-agnostic**. If it hard-codes Cast, every other receiver is a rewrite |
| F50 | **Deletion** — *deferred ([D144](contributors/decisions/working-notes.md)); D89 is the precondition; tombstones for files deleted outside us: [D167](contributors/decisions/working-notes.md), [D173](contributors/decisions/working-notes.md)* | 0/120, 0/50, 16/2,205 | free | Precondition already met: the trash is designed and the export uses it ([D144](contributors/decisions/working-notes.md), [D145](contributors/decisions/working-notes.md)). Policy is an *enforced absence* |
| F51 | **Artwork type enumeration** (poster/backdrop/logo/banner/disc) | Kodi and Plex both model it | schema | A closed enumeration on the image, at the level it belongs to. Cheap now, a migration later |
| F52 | **Watch Together surviving a client rewrite** | #2 in the corpus | **invariant** | [D62](contributors/decisions/working-notes.md)'s **six methods per client is a hard budget**, not a description. Plex withdrew the feature in Feb 2025 because it did not survive one ([D150](contributors/decisions/working-notes.md)) |
| F53 | **Relay gap annotation** — what it saw while the core was away | — | free | Counts and a window, never identities ([D72](contributors/decisions/working-notes.md), [D152](contributors/decisions/working-notes.md)) |
| F91 | **Rebroadcast grant** — a user with `rebroadcast` plugs his account into his own core, which treats our server as a node; his household plays on derived grants only his core can mint; his overlay of tags and rules stays on his side; never transitive ([D174](contributors/decisions/working-notes.md), [D177](contributors/decisions/working-notes.md)) | owner's design | schema + invariant | A grant may have a **parent** (narrower, short, cannot mint, dies with it); a **node need not be a local daemon**; a file may be reached through another server; every item carries its **source** |
| F54 | **Content-safety inspection** | — | seam | [D61](contributors/decisions/working-notes.md) keeps four affordances open; containment now, detection later |

## Rank 3 — Deferred and free

Pure client work or strictly additive. Ignore until there is room; nothing is owed.

| # | Feature | Demand |
| --- | --- | --- |
| F55 | Shuffle and random play | **zero mentions in this tree**, universal elsewhere |
| F56 | Sleep timer | +32 |
| F57 | Picture-in-picture and background audio | +20 |
| F58 | Folder / raw-directory browsing — **the escape hatch of last resort**: each file shows what it is attached to and how (inferred, sidecar, person) ([D176](contributors/decisions/working-notes.md)) | +19 |
| F59 | External player handoff | +63 ([tv-client.md](contributors/design/plans/tv-client.md)) |
| F60 | Bulk edit and multi-select | +3 |
| F61 | Chapters in the player UI | modelled already ([media-segments.md](contributors/design/plans/media-segments.md)) |
| F62 | Deep links, widgets, Siri/Shortcuts | +61 |
| F63 | Android Auto / CarPlay | +9, and audio is already second |
| F64 | Instant mix / radio / party mode | +13 |
| F65 | Favourites surface | +42 |
| F66 | Missing-episode display | Jellyfin has `DisplayMissingEpisodes` |
| F67 | Watchlist as a surface | **+3 — and Plex removed it for lack of use** ([D150](contributors/decisions/working-notes.md)). Storage yes, screens no |

---

## Undecided

| # | Feature | What decides it |
| --- | --- | --- |
| F92 | **Music** (ROADMAP Phase 7, OpenSubsonic) | Competitor and sentiment research, commercial players first. The owner's case to judge it by: *"my music collection playing through the TV"* ([D176](contributors/decisions/working-notes.md)). **Schema cost explored** ([D186](contributors/decisions/working-notes.md), [music.md](contributors/design/research/music.md)): one reshape, made now; the rest additive |

---

## Rank 4 — Declined

**A decline is not a demolition.** The last column is what would make it possible again, and it
is there so that changing your mind is cheap.

| # | Declined | Why | What keeps the door open |
| --- | --- | --- | --- |
| F68 | Photos | [D1](contributors/decisions/working-notes.md) — Immich exists and moves faster | nothing; door shut |
| F69 | Audiobooks and comics | different products; Audiobookshelf has 1,191 open issues | the four-level model is media-agnostic |
| F70 | **Live TV / DVR** | a second product inside the first — Jellyfin's `LiveTv` alone is 38 routes | **an item must never be *required* to have a File.** Cheap to keep true, expensive to undo |
| F71 | Requests (Overseerr-shaped) | **21,728★ — the field's largest gap, and companion-app territory** | [D20](contributors/decisions/working-notes.md)'s public API, plus a writable "wanted" concept if ever |
| F72 | **Outbound notifications** | 8 reactions; a media server has no outcome you wait on ([D151](contributors/decisions/working-notes.md)) | the event sources exist; a sink is additive |
| F73 | DLNA / UPnP | 11 reactions; Jellyfin moved it out of core, plugin is 87★ / 84 issues | [D15](contributors/decisions/working-notes.md)'s out-of-process adapter tier |
| F74 | 3D / stereoscopic | one issue, 23 reactions | it would be a fourth rendition axis; [D22](contributors/decisions/working-notes.md) is written for three |
| F75 | Sonos / multi-room | no demand here | nothing |
| F76 | Games, sports, music videos as library types | Emby shipped Games, Jellyfin dropped it; Plex closed its arcade | see F70 |
| F77 | Multi-server federation | 3 reactions | **narrowed, not closed:** one-hop server-to-server grants are F91 |
| F78 | Poster overlays baked into artwork | destructive to files we do not own ([principles §2](contributors/principles/README.md)) | badges as a **client render** over an untouched image |
| F79 | A content-source plugin tier | **Plex killed it at "less than 2 percent" usage** ([D150](contributors/decisions/working-notes.md)) | the metadata-provider and adapter tiers are different products and stay |
| F80 | Voice assistants | 66 likes | Cast already carries voice-originated load requests |
| F81 | A separate desktop application | the web client is the desktop client | — |
| F82 | A Roku client | BrightScript shares no code and Roku cannot render bitmap subtitles | the cast receiver covers the long tail ([D124](contributors/decisions/working-notes.md)) |
| F83 | DRM | [D3](contributors/decisions/working-notes.md) | nothing; door shut |
| F84 | A device-profile database | [D2](contributors/decisions/working-notes.md), [D21](contributors/decisions/working-notes.md) — the field's largest bug source | the closed output menu must never become a grammar of conditions |
| F85 | A Netflix-style encoding ladder | [D86](contributors/decisions/working-notes.md) | — |
| F86 | A video encoder in the core binary | [D19](contributors/decisions/working-notes.md), [D193](contributors/decisions/working-notes.md) | ~~it is a plugin on the public API~~ it is a media-node capability |
| F93 | **Decrypting protected discs or streams** (DVD CSS, AACS, streaming DRM), bundled or linked | [D187](contributors/decisions/working-notes.md) — §1201 liability that lawful uses do not cure | nothing; door shut. An encrypted disc is refused with its reason |
| F94 | **Built-in content sources** — indexers, trackers, debrid, IPTV playlists, source add-on catalogues, request→download wiring | [D187](contributors/decisions/working-notes.md) — *Filmspeler*, Popcorn Time; the features app stores remove | generic webhooks and the plugin API; third-party repositories added by hand |
| F95 | **Venue, kiosk or public-display mode; payments between users** | [D187](contributors/decisions/working-notes.md) — a public performance in every venue; paid access is resale | nothing |
| F87 | A provider's trailer played from its URL | [D162](contributors/decisions/working-notes.md) — a work-shaped row with no Version, no File and nothing for a rebuild to recover, plus a third-party fetch during playback | the provider record may carry the **link** as metadata; following it is opening a link, not playback |

---

## The obligations, collected

Everything above distils to **fourteen things the architecture has to keep true**. This is the
checklist to read a design against; each links to the row that needs it.

1. **An item must not be required to have a File.** (F70, F76)
2. **The playback path must be addressable by a *File*, not only by a library item.** You
   cannot say what something is without watching ten seconds of it, and an unfiled file has no
   Title. Free today, a rewrite once every endpoint takes an item id. (F88)
3. **No row a user's history or authored data references is ever hard-deleted** — it is
   tombstoned ([D167](contributors/decisions/working-notes.md)). The play log is kept for ever
   and a device back from a trip syncs spans for a file the server no longer has. (F22, F39)
4. **Collection membership is three kinds, not two** — explicit, predicate, and authored
   elsewhere. (F41)
5. **A rating is `(source, kind, score, votes, as_of)`, and critic ≠ audience.** (F40)
6. **A content descriptor carries a score and a sign, not just presence.** (F44)
7. **The track model is not exactly three axes.** (F45)
8. **The playback role is protocol-agnostic, not Cast-shaped.** (F49)
9. **The client fetches its home layout; it does not compile it in.** (F42)
10. ~~**Playback rate is shared session state or explicitly local — decided, not discovered.**~~ **Met** by D90: local unless a group shares it. (F46)
11. **A PIN never authenticates; the session model never assumes one factor.** (F47, F48)
12. **Watch Together costs six methods per client, as a budget.** (F52)
13. **A grant may have a parent, and a node may be another core acting as a user.** (F91)
14. **Every item carries its source**, so "was this given to me?" is a column. (F91)

And one that is not a feature obligation but pays for several of them: **[D81](contributors/decisions/working-notes.md)'s
play-event log must be retained rather than truncated**, because F39, F43 and most of F41's
reconciliation read it. (F39) — **and it is free rather than a trade**
([D165](contributors/decisions/working-notes.md)): once a heartbeat stops being a log row and a
run of them becomes one watched span, five years of a four-person household is **13.4 MB**
against the 470 MB the original shape would have cost. D81 wanted a retention horizon and this
obligation forbade one; neither had to give.
