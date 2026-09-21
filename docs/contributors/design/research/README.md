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
| [commercial-streaming.md](commercial-streaming.md) | Netflix, Apple, Amazon, YouTube — what the big players do, and which of it transfers to a home server. Extended 2026-09-19 with buffer-based rate adaptation (their SIGCOMM'14 paper), ingest-time loudness normalisation, and how Skip Intro is actually produced. |
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
| [tech-licensing.md](tech-licensing.md) | FFmpeg's licence ladder, codec patents, and provider terms under an open-source release. Where the landmines are; not legal advice. §5 (2026-09-19) is the one that bites: a whole-project GPLv3 cannot ship through the Apple App Store, so the licence splits by artefact. Backs D80. |
| [tech-metadata.md](tech-metadata.md) | TMDB, TheTVDB, MusicBrainz, AcoustID, OpenSubtitles — capability, licensing, and the traps. |
| [library-model-review.md](library-model-review.md) | A review of the library model against EIDR's industry hierarchy and against every schema-touching decision since it was written. Finds it one level short, and short where the field's permanent bugs come from. Backs D79. |
| [subtitle-rendering.md](subtitle-rendering.md) | Whether to render subtitles server-side as timed image overlays — **measured** at 8.7 KB/event palettised, why it moves the hostile font parse into the jail, and why it dominates burn-in. Backs D78. |
| [backup-restore.md](backup-restore.md) | What is irreplaceable when the database is a rebuildable index, and the **measured** fact that copying a live WAL database's `.db` file yields a file with no tables in it. Backs D77. |
| [sharing-model.md](sharing-model.md) | Granular sharing: Plex's label filters (the only working answer, and paywalled), Netflix's household-enforcement apparatus and why we need none of it, and why a share is a predicate. Backs D76. |
| [rules-engine.md](rules-engine.md) | Stored queries over the library: Navidrome's criteria package read locally, Apple's and Plex's smart-playlist behaviour, Kometa's vocabulary — and **measured** SQLite limits (OR chains die at 1000 terms, joins at 65 tables) that shape the compiler. Backs D75. |
| [watch-together.md](watch-together.md) | Synchronised remote playback and voice chat: the five commercial implementations that were withdrawn and why, Jellyfin's sync mechanism read locally, Apple's suspension model, and the echo-cancellation problem that decides the voice design. Backs D62–D64. |
| [testing-strategy.md](testing-strategy.md) | How media software is actually tested, from four suites read locally. The corpus question had a false premise: Jellyfin's tests contain **no media**, Navidrome's **13 files and 512 KB**, FFmpeg's 1.29 GiB with a **median file of 58.7 KB**. Measured here: a generated corpus is 8.9 s and 5.9 MB, a VMAF gate is 1.7× realtime, and Landlock is **unavailable** in a stock container. Backs D109–D112. |
| [performance-targets.md](performance-targets.md) | The workload, measured: the four-level schema at 10k and 100k titles, the node's passes on real video, walk and report throughput, SQLite under mixed concurrency. **Ten of twelve queries are flat; the two that scale linearly are both on the home page.** The passes are two populations a thousand-fold apart. Six thresholds other decisions were guessing at become numbers. Backs D113–D116. |
| [content-rating-systems.md](content-rating-systems.md) | What age-rating and content-descriptor data exists and which of it reaches us. Kijkwijzer's seven axes as the vocabulary worth copying; **Jellyfin's `us.json` read locally, collapsing all fifteen `TV-14-*` descriptor combinations to one value**; their cross-system fallback returning the first match from 45 national tables in map-iteration order; and TMDB's `descriptors` array, which exists in the schema and is empty in their own example. Backs D131. |
| [node-transport.md](node-transport.md) | How the core and a media node should talk. Netflix's Open Connect overview read as the primary source — an appliance that does exactly two things, report what files it holds and serve them over HTTP, with the control plane steering clients **via URL**. Tdarr, Immich and Kyoo read for the open field; GitHub's runners and AWS's SSM agent for the convergence on *the agent dials out*. **Measured**: the reports are not the payload — a snapshot is 51 MB and the derived artefacts are 48 GB. Backs D125–D128. |
| [audit-2026-09-19.md](audit-2026-09-19.md) | An independent adversarial audit of the whole tree against industry practice, the large operators and the complaint record. Ranked findings, of which the headline ones are the App Store/GPL one-way door, play state as a log, typed media segments, loudness and downmix, per-country certifications, API skew, the missing adaptation on the one link we charge for, offline download's server half, deletion, the statutory caption-control floor, and People — plus a second batch found by reading the decisions against each other rather than against the outside world, three of which **fail open**. Backs D80–D108. |
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
- **Vendor forums are often Discourse, and Discourse has a JSON API.** `forums.plex.tv`
  answers `/top.json?period=yearly`, `/search.json?q=` and `/t/<id>.json` without
  authentication, which is first-hand user sentiment in structured form. It was not tried in
  the first round, and when it was (2026-09-19) it **changed a conclusion** — see
  [sentiment.md](sentiment.md) §Plex's own forum. Check for one before accepting that a
  community is unreachable.
- **Reddit could not be read.** Every route was blocked: `reddit.com`/`old.reddit.com`/
  `api.reddit.com` JSON returns 403 to this network, the WebFetch tool refuses the domain, the
  search tool refuses `reddit.com` as an allowed domain, public Redlib front-ends are behind
  proof-of-work, and a text-extraction proxy returned Reddit's own block page. The gap is
  covered from HN, GitHub, Lemmy and forums instead, and where a search engine summarised a
  Reddit thread that is marked as second-hand. See [sentiment.md](sentiment.md) §Method.
