> **ABOUTME:** Index of the design research: what was examined, what was verified, and which
> design question each document feeds. Start here to find the evidence trail behind a choice.

# Research

Research backing the design. Each file covers one topic area with verified facts, dated, and
states the design implication it feeds.

All of it was gathered **2026-09-17** unless a document says otherwise.

| File | Topics |
| --- | --- |
| [competitors-video.md](competitors-video.md) | Plex, Emby, Jellyfin, Kyoo, Stash, Kavita — stacks, architecture, where each breaks. Source read locally for the open ones. |
| [competitors-audio.md](competitors-audio.md) | Navidrome, gonic, Audiobookshelf, Plexamp, the Subsonic/OpenSubsonic API, Spotify. |
| [competitors-photos.md](competitors-photos.md) | Immich, PhotoPrism, Nextcloud Memories, LibrePhotos, Google/Apple Photos. |
| [commercial-streaming.md](commercial-streaming.md) | Netflix, Apple, Amazon, YouTube — what the big players do, and which of it transfers to a home server. |
| [sentiment.md](sentiment.md) | What users actually complain about, with sources and a stated method. Includes what could **not** be gathered and why. |
| [plugins-and-workarounds.md](plugins-and-workarounds.md) | What people extend and hack around in existing systems — plugin catalogues by stars, the companion-app ecosystem, and the workarounds that exist only because a seam was missing. The requirements list for our extension design. |
| [remote-access.md](remote-access.md) | NAT topology, the mechanisms and what each costs, the TLS-certificate collision, relay economics, and identity — which is also the hard half of sharing. §3 (2026-09-19) covers NAT-traversal state of the art: the two cases where a relay is unavoidable, why published relay rates vary 8×, and why our media server is already the relay. Backs D25, D68. |
| [threat-model.md](threat-model.md) | Adversaries, trust boundaries, and the FFmpeg containment stance. Backs D26. |
| [change-detection.md](change-detection.md) | How the server learns the library changed, across the filesystems people actually use; evaluates the NAS-side index agent. Backs D27. |
| [test-hardware.md](test-hardware.md) | What hardware the test matrix needs, measured against what is available; the gaps and the buying order. |
| [tech-playback.md](tech-playback.md) | The core problem: transcode-vs-direct-play, HLS/DASH, the device-profile trap, subtitles, hardware acceleration. |
| [tech-backend.md](tech-backend.md) | Language, runtime, framework, job scheduling, API shape. |
| [sqlite-reconsidered.md](sqlite-reconsidered.md) | Re-examination of the database choice against the real workload — corrects a misreading of the original evidence and reverses D5. Backs D31. |
| [tech-storage.md](tech-storage.md) | Database, schema stance, search, migrations, on-disk layout, backup. |
| [client-framework.md](client-framework.md) | A deliberate re-examination of the React Native decision against 2026 evidence, including Shopify's reversal to native — with corrections to the evidence D10 originally cited, and a falsifiable bail-out criterion. |
| [tvos.md](tvos.md) | Why tvOS breaks every cross-platform option, what works there, who skips it — and the measurement showing it is ~3% of an Apple client rather than a second app. Backs D30. |
| [shared-core.md](shared-core.md) | Whether a native client can reuse the TypeScript SDK or must reimplement — decomposes what "the shared SDK" actually contains, and evaluates JS-embedding, gomobile, Rust/UniFFI and KMP. Backs D29. |
| [tech-clients.md](tech-clients.md) | Player engines and the platform matrix — where the real cost of this project is. |
| [tech-licensing.md](tech-licensing.md) | FFmpeg's licence ladder, codec patents, and provider terms under an open-source release. Where the landmines are; not legal advice. |
| [tech-metadata.md](tech-metadata.md) | TMDB, TheTVDB, MusicBrainz, AcoustID, OpenSubtitles — capability, licensing, and the traps. |
| [backup-restore.md](backup-restore.md) | What is irreplaceable when the database is a rebuildable index, and the **measured** fact that copying a live WAL database's `.db` file yields a file with no tables in it. Backs D77. |
| [sharing-model.md](sharing-model.md) | Granular sharing: Plex's label filters (the only working answer, and paywalled), Netflix's household-enforcement apparatus and why we need none of it, and why a share is a predicate. Backs D76. |
| [rules-engine.md](rules-engine.md) | Stored queries over the library: Navidrome's criteria package read locally, Apple's and Plex's smart-playlist behaviour, Kometa's vocabulary — and **measured** SQLite limits (OR chains die at 1000 terms, joins at 65 tables) that shape the compiler. Backs D75. |
| [watch-together.md](watch-together.md) | Synchronised remote playback and voice chat: the five commercial implementations that were withdrawn and why, Jellyfin's sync mechanism read locally, Apple's suspension model, and the echo-cancellation problem that decides the voice design. Backs D62–D64. |
| [internationalisation.md](internationalisation.md) | UI strings, collation, FTS5 tokenisers, the multi-language metadata model, libass bidi and line breaking, font coverage, and the Unicode traps in Go and in our own node pipeline. Mostly **measured**, not cited. §8 covers how Netflix does it and what transfers; §9 settles the fallback font. Backs D45–D54. |

## Method and its limits

- **The large commercial operators get their own section, every time.** Netflix, Amazon, Apple,
  Google, Spotify and their peers have far more cumulative operational experience at far higher
  user counts than anything in the open-source field, so a design informed only by open-source
  practice risks adopting something the industry already learned its way out of. Prefer their
  **primary** sources — engineering blogs, published partner and delivery specifications,
  compatibility documents — over secondary summaries, and always state which of their practices
  transfer to household scale, which are artefacts of planetary scale, and where a problem is
  *harder* for us than for them. That last category is the important one: it marks the places
  their experience does not cover us.
- **Source code** for open competitors was cloned at depth 1 into the local cache and read
  directly. Claims about a codebase's shape (file sizes, dependency lists, layering) come from
  that reading, not from documentation.
- **Issue data** comes from the GitHub search API sorted by reaction count. Reaction count is a
  decent proxy for "how many people want this fixed" and a poor proxy for severity. Open-issue
  totals are the API's `total_count` at fetch time.
- **Community sentiment** comes from Hacker News (Algolia API, full comment trees) and from web
  search summaries of discussion threads.
- **Reddit could not be read.** Every route was blocked: `reddit.com`/`old.reddit.com`/
  `api.reddit.com` JSON returns 403 to this network, the WebFetch tool refuses the domain, the
  search tool refuses `reddit.com` as an allowed domain, public Redlib front-ends are behind
  proof-of-work, and a text-extraction proxy returned Reddit's own block page. The gap is
  covered from HN, GitHub, Lemmy and forums instead, and where a search engine summarised a
  Reddit thread that is marked as second-hand. See [sentiment.md](sentiment.md) §Method.
