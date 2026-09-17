> **ABOUTME:** Survey of the music and spoken-audio half of the field — Navidrome, gonic,
> Audiobookshelf, Plexamp and Spotify — and of the Subsonic/OpenSubsonic API that most of the
> open ones speak. Establishes why audio is a cheaper addition than it looks.

# Competitors: Audio

Repository facts read from the GitHub API on **2026-09-17**; code observations from local
clones the same day.

| Project | Stars | Open issues | Language | Licence |
| --- | --- | --- | --- | --- |
| navidrome/navidrome | 23,631 | 276 | Go | GPL-3.0 |
| advplyr/audiobookshelf | 14,360 | 1,191 | JavaScript | GPL-3.0 |
| sentriz/gonic | — | — | Go | GPL-3.0 |

---

## The Subsonic / OpenSubsonic API — the reason audio is cheap

Music self-hosting converged on a **shared server API** in a way video never did. Subsonic's
HTTP API became the de-facto standard; when Subsonic itself stagnated, the community formed
**OpenSubsonic**, a versioned extension process over the same surface. Navidrome, gonic,
Airsonic, Ampache, Astiga and others implement it.

The consequence is decisive: **an audio server that speaks OpenSubsonic inherits a mature
third-party client ecosystem on day one** — Symfonium and Tempo (Android), Amperfy and play:Sub
(iOS), Feishin and Sonixd (desktop), plus car and watch integrations — at zero client cost.
There is nothing equivalent for video. Video clients are per-server, and that is why the video
half of this project is expensive and the audio half is not.

Navidrome's tracker shows the extension process working in the open (`Tracking OpenSubsonic
extensions development`, +27) and also its limits — the protocol is folder-and-tag shaped, and
issues like `getIndexes and getMusicDirectory return artist/metadata entries instead of
folders, breaking folder-based clients` are the seams showing.

---

## Navidrome — the best-engineered server in this survey

**Stack (read locally).** Go 1.27, `go-chi/chi` v5 router, Squirrel query builder over
**SQLite** (`mattn/go-sqlite3`), `go-chi/jwtauth` for auth, Wire for compile-time DI. UI is
React with `react-admin` and Material-UI. Single static binary; no external services.

**Decisions worth taking.**

1. **Speak other people's APIs.** Navidrome serves `server/subsonic/` **and
   `server/jellyfin/`** — a full Jellyfin-compatible surface (items, browsing, playlists,
   sessions, images, lyrics, streaming, websocket) so that Jellyfin music clients work against
   it unchanged. Implementing a rival's API to borrow its clients is a cheap, high-leverage
   move, and it is directly applicable to this project in the opposite direction.
2. **A phased, resumable scanner.** `scanner/phase_1_folders.go` → `phase_2_missing_tracks.go`
   → `phase_3_refresh_albums.go` → `phase_4_playlists.go`, with a filesystem watcher
   (`watcher.go`) and `.ndignore` handling. Explicit phases with names beat a single recursive
   walk that does everything: each phase is separately testable, separately resumable, and
   separately loggable. Contrast: "logging around library scanning being pretty difficult to
   interpret" is a live complaint about Jellyfin on HN.
3. **WASM plugins via Extism.** Metadata agents, scrobblers, lyrics providers and similarity
   engines run as sandboxed WebAssembly with an explicit host-capability list (HTTP, scheduler,
   cache, KV store, storage, artwork, config, and a Subsonic-API callback). Plugins can be
   written in Go, Rust, Python or TypeScript. This is the correct answer to "plugins" — a
   capability-scoped sandbox instead of Jellyfin's in-process .NET assemblies or Stash's
   in-process JS interpreter.
4. **Tags stay in the files.** The library is a projection of on-disk tags, not a separate
   authority. Rescan rebuilds it. This is why Navidrome has no migration horror stories.

**Where it fails users.** Almost entirely *absent features*, not defects — the top of the
tracker is `Add LDAP Support` (+148), `Improve SSO support` (+64), `Implement podcast subsonic
api` (+42), `Artist Detail Page` (+31), shared listening (+30). The bug half is small and
shallow: `.lrc` lyrics not picked up, artwork regressions, MediaSession state, playback
skipping on tab switch. An HN commenter's line — "Navidrome actually could run on my raspberry
pi with my library" — is the reputational summary: light, correct, unambitious.

**The one thing it does not have: Plexamp.** Repeatedly on HN: "I can never give up plexamp.
I've tried navidrome and various jellyfin clients but I keep going back." Plexamp's edge is
client-side — gapless playback, crossfade, loudness normalisation, offline sync, a genuinely
good mobile UX, and server-side "sonic analysis" for similarity. Navidrome's newer `core/sonic`
package and the `SonicSimilarity` plugin capability are aimed at that gap.

**gonic** is the minimal counterpoint — Go, SQLite, Subsonic-compatible, deliberately tiny.
Useful as proof of how small a competent audio server can be.

---

## Audiobookshelf — the cautionary tale on scope

**Stack (read locally).** Node/Express with `sequelize` over `sqlite3`, `socket.io`, Passport +
`openid-client`; the client is **Nuxt 2 / Vue 2** (end-of-life) with `hls.js` and `epubjs`.
Packaged with `pkg` for Windows/Linux binaries.

**Where it fails users.** **1,191 open issues**, and the top thirty by reactions are
*exclusively* feature requests — Kobo sync (+265), OPDS (+71), **PostgreSQL support (+66)**,
EPUB 3 (+63), Whisper transcription (+57), metadata-driven file moves (+51), Goodreads
ratings, LDAP, DLNA, SyncPlay. This is what an open-ended feature surface with a single
maintainer looks like. Note also the SQLite consequence that shows up here concretely: an
unhandled `SQLITE_BUSY` during a library scan running alongside FFmpeg metadata-embedding jobs
took the whole server down (issue #5349) — the single-writer lock is not theoretical. See
[tech-storage.md](tech-storage.md).

**The lesson we take:** audiobooks and podcasts are a *different product* wearing the same
clothes as music — progress tracking to the second, chapter navigation, series and narrator
metadata, sleep timers, variable speed. Adding "audio" to a media server does not
automatically add audiobooks, and pretending otherwise generates exactly this backlog.

---

## Plex / Plexamp

Closed. What matters is the shape of the win: **Plexamp is a separate, focused, first-party
client** rather than a music tab in the video app. It is the most-cited reason people stay
with Plex on HN music threads, and it is a client-side achievement, not a server-side one.
Jellyfin's own music story is repeatedly described as adequate-but-unloved; the two projects
that people name as good music experiences (Plexamp, Symfonium) are both clients.

---

## Spotify

Not a competitor — a **UX benchmark** and a source of churn. The useful facts are what people
leave it *for*: catalogue removals ("YouTube Music took down some albums I listened to
religiously"), audio quality on good equipment, artist compensation, and bad first-party
clients on secondary devices (watch, car).

Its published architecture is a large-scale microservice estate on Java/Scala with Kafka, and
is **not applicable** to a single-household server; anything claiming otherwise is
extrapolation. Do not copy Spotify's backend. Do copy its client behaviours: instant start,
gapless, crossfade, offline sync that works, and a queue model users can reason about.

---

## What this survey settles

1. **Audio is the cheapest of the three media types to ship well**, on one condition: speak
   **OpenSubsonic**. That buys a working client ecosystem for free and makes "do we build a
   music client?" a question we can defer indefinitely.
2. **Scan as named phases**, not one recursive walk (Navidrome).
3. **Tags in files, database as cache.** It removes an entire class of migration and
   data-loss failure.
4. **Plugins are a sandbox problem.** Extism/WASM with an explicit host-capability list is the
   demonstrated answer.
5. **Audiobooks are out of scope until video and music are done.** Audiobookshelf's tracker is
   the evidence for what happens otherwise.
