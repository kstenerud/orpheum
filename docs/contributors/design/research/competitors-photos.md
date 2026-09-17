> **ABOUTME:** Survey of the photo half of the field — Immich, PhotoPrism, Nextcloud Memories
> and LibrePhotos — and of what Google and Apple Photos set as the expectation. Establishes why
> photos is a different product from video, not a third library type.

# Competitors: Photos

Repository facts read from the GitHub API on **2026-09-17**; code observations from local
clones the same day.

| Project | Stars | Open issues | Language | Licence |
| --- | --- | --- | --- | --- |
| immich-app/immich | 114,516 | 738 | TypeScript | AGPL-3.0 |
| photoprism/photoprism | 40,199 | 473 | Go | AGPL-3.0 (+ commercial) |

Immich has **twice Jellyfin's stars** and is the fastest-moving project in this entire survey.
That is the headline finding: the unmet demand in self-hosted photos is larger than in video.

---

## Photos is not a media library. It is a backup product.

The defining difference, and the reason this is a separate decision from audio:

- **The phone is the source of truth, not the disk.** A photo server's primary job is to get
  photos *off* a device reliably, in the background, over flaky networks, without duplicating
  and without losing anything — before it ever displays one. Video and music servers index
  files someone else put there. A photo server must *acquire* them.
- **That requires a real mobile app**, with background upload, OS photo-library permissions,
  battery/network policy, resumable uploads and delta sync. There is no protocol to borrow (no
  OpenSubsonic equivalent) and no way to defer it to third parties. Immich ships a Flutter app
  with a local **drift/SQLite** database precisely for this.
- **Trust requirements are categorically higher.** Losing a transcode is an inconvenience;
  losing the only copy of a photo is unforgivable. Immich shipped a `database-backup.service`
  and an `integrity.service` in-tree.

The cost of "add photos" is therefore roughly *the cost of the whole video project again*, and
almost all of it lands in the mobile client.

---

## Immich — the reference architecture

**Stack (read locally).**

| Component | Stack |
| --- | --- |
| `server/` | **NestJS 12 / TypeScript**, Node, `kysely` (typed SQL builder, not an ORM) over **Postgres**, BullMQ on Redis for jobs, `zod` validation, OpenTelemetry, `sharp` for images, socket.io for live updates |
| `machine-learning/` | **Python 3.11+**, FastAPI + gunicorn/uvicorn, ONNX Runtime, OpenCV, rapidocr, `uv` for deps |
| `web/` | **SvelteKit**, `hls.js`, `maplibre-gl` + pmtiles for maps, `thumbhash` placeholders, WASM justified-layout |
| `mobile/` | **Flutter**, `drift` + `sqlite_async` local DB, `background_downloader`, riverpod/hooks |
| Postgres image | pgvector + VectorChord for embedding search |

**Decisions worth taking.**

1. **Clean layering, mechanically enforced.** `controllers/` → `services/` → `repositories/`,
   one file per concern, with a `.spec.ts` beside almost every service. Every external system
   (filesystem, FFmpeg, ML, email, crypto, process) is reached through a repository interface,
   which is why the test suite can run without any of them. This is the layering Jellyfin never
   achieved and it is directly copyable in any language.
2. **A typed query builder, not an ORM.** `kysely` gives compile-time-checked SQL without
   hiding the SQL. Immich's schema is declared in-tree (`schema/tables/*.table.ts`) with
   generated, ordered, verifiable migrations (`migrations:verify-order` is a CI script). This
   is the precise thing Jellyfin got wrong by adopting a full ORM late and letting it enumerate
   client-side.
3. **An audit-table change journal for delta sync.** Every syncable table has a paired
   `*_audit` table recording deletions with a UUIDv7 primary key and an indexed timestamp.
   Combined with a `sync.service`, a mobile client can ask "what changed since cursor X",
   including deletes — which a plain `updatedAt` column cannot express. Any project with
   offline clients needs this, and needs it **in the schema from day one**; retrofitting it is
   a migration.
4. **Jobs as an explicit, inspectable queue.** BullMQ with per-job-type queues, a `job.service`
   and a `queue.service`. Thumbnails, transcodes, metadata extraction, ML inference and library
   scans are all jobs. The tracker's `Better failed job management` request shows the model is
   right and the surfacing is incomplete.
5. **ML is a separate process in a separate language.** The Python service is isolated behind
   an HTTP repository interface, so the server does not carry a Python or ONNX dependency and
   ML can be scaled, replaced or switched off independently. Worth copying if we ever do face
   or scene recognition — and worth noting as the source of most of Immich's user-visible pain.

**Where it fails users.**

- **Sharing.** `Better sharing in Immich` has **1,020 reactions and 259 comments**, is the
  most-reacted issue in this entire survey, and carries a declared **feature freeze**. Nobody
  in this field has solved multi-user sharing — see PhotoPrism below, where the equivalent issue
  is also #1.
- **The ML service is the top source of breakage.** `ML OCR memory leaks?` (+50, 90 comments),
  `Immich 2.5.2 Machine Learning Broken (OpenVINO / Intel)` (+27, 74 comments), `v2.5.0 to
  v2.5.2 broke ML for NVIDIA GTX 960 — cudaErrorNoKernelImage` (+11, 38 comments). Hardware-
  accelerated ML across consumer GPUs is a support nightmare and it recurs every release.
- **Mobile is the second source.** `[v3.0.0 & 3.0.1] iOS App Crashes on start | APP UNUSABLE`,
  `iOS backup uploads frequently fail with PathNotFoundException`, `Deletes on the mobile app
  are confusing/broken`, `Automatic local URL switching frequently fails` (+23) — the
  local-vs-remote URL switch being a problem *everyone* in self-hosting has.
- **Colour management.** `Previews and thumbnails for HEIC HDR images look flat`, `[Regression]
  sRGB is assumed when generating thumbnails`, `HDR Tone mapping - Dark videos`. Getting HDR
  and wide-gamut right in thumbnails is harder than it looks and will bite us in video too.

---

## PhotoPrism

**Stack (read locally).** Go with **`jinzhu/gorm` v1** (the *deprecated* v1, not GORM v2), Gin,
MariaDB or SQLite, TensorFlow and ONNX for classification/face/NSFW (`internal/ai/{classify,
face,nsfw,onnx,tensorflow,vision}`), frontend is Vue 3 + Vuetify on webpack. AGPL-3.0 with a
commercial licence and membership tiers alongside.

The `package.json` is worth reading as a cautionary artifact: it carries multi-paragraph
comments pinning webpack and Vuetify to exact versions with detailed bug narratives, because
minor upstream bumps broke the production bundle in ways that only appeared minified at
runtime. That is what an unpinned, under-tested frontend dependency graph costs.

**Where it fails users.** The top of the tracker is a museum of things asked for years ago and
not delivered: `Sharing: Multi-user / multi-library support` (**+844**, open since 2019-02),
`UX: Timeline View` (+305, 2019-12), `Support for cloud storage` (+196), **`Config: Add support
for PostgreSQL` (+189, open since 2018-10)**, memories (+167), duplicate handling (+132),
`Security: Add Password for Access to Private Pictures` (+125), hierarchical folders (+103).
Reaction counts of that size against issue ages of five-to-seven years is the signature of a
project whose roadmap has diverged from its users — and is a large part of why Immich overtook
it.

The Postgres issue is directly instructive for us: **the database choice is not reversible on a
reasonable timescale.** PhotoPrism has been unable to add Postgres for eight years.

---

## Nextcloud Memories / Nextcloud Photos

A PHP app inside a general-purpose file-sync platform. The Memories app documents HLS
transcoding and claims testing past a million photos; the platform underneath is the
constraint, and the recurring community complaint is performance and operational weight rather
than features. **UNVERIFIED** — the Reddit threads that carry the detailed performance
complaints were unreachable (see [sentiment.md](sentiment.md) §Method), and web-search
summaries of them were too thin to cite.

The strategic point stands regardless: Nextcloud's advantage is that photos ride on a
general-purpose sync/share/user system that already exists. Its disadvantage is that photos
ride on a general-purpose sync/share/user system.

**LibrePhotos** (Python/Django + React) is the third option; smaller, slower-moving, and
generally reported as heavier to run than Immich for the same result.

---

## Google Photos / Apple Photos — the bar

They are what every user compares against, and they set three expectations that are all
*client-side*:

1. **Instant, infinite, jank-free timeline scroll** over a hundred thousand items, with
   position preserved. (Immich's `Timeline scroll position resets to top after viewing a photo`
   is exactly this bar being missed; its WASM justified-layout and thumbhash placeholders are
   exactly this bar being chased.)
2. **Search that understands content**, not filenames — which means embeddings, which means an
   ML service and a vector index.
3. **Backup you never think about**, including a credible "your photos are safe" story.

Apple additionally sets the expectation that photo libraries are *shared with family* without
a permissions dialog — the thing nobody in the open field has built.

---

## What this survey settles

1. **Photos is a separate product with a separate cost centre (the mobile app), and it should
   be a separate decision.** Recommendation: design the data model so it is not *excluded*, and
   build it last, if at all. Immich already exists, is AGPL, and is moving faster than we
   could.
2. **The reference architecture to copy is Immich's server**: controllers → services →
   repositories, typed SQL builder over Postgres, jobs in an explicit queue, ML out-of-process.
   This is worth copying for **video** regardless of whether we ever ship photos.
3. **Build the change journal (audit tables + sync cursor) into the schema from the start.**
   Offline clients need deletes, and `updatedAt` cannot express them.
4. **Sharing is the unsolved problem in this entire field.** Immich (+1,020, frozen),
   PhotoPrism (+844, seven years), Jellyfin (per-user granular library sharing, requested on
   HN). Whoever solves multi-user sharing of a personal library wins something real. It is also
   why Plex's account system — the thing everyone resents — exists.
5. **Consumer-GPU ML is a support liability.** If we do it, it goes out-of-process, behind an
   interface, off by default.
