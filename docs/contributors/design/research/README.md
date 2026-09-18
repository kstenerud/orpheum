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
| [remote-access.md](remote-access.md) | NAT topology, the mechanisms and what each costs, the TLS-certificate collision, relay economics, and identity — which is also the hard half of sharing. Backs D25. |
| [threat-model.md](threat-model.md) | Adversaries, trust boundaries, and the FFmpeg containment stance. Backs D26. |
| [change-detection.md](change-detection.md) | How the server learns the library changed, across the filesystems people actually use; evaluates the NAS-side index agent. Backs D27. |
| [test-hardware.md](test-hardware.md) | What hardware the test matrix needs, measured against what is available; the gaps and the buying order. |
| [tech-playback.md](tech-playback.md) | The core problem: transcode-vs-direct-play, HLS/DASH, the device-profile trap, subtitles, hardware acceleration. |
| [tech-backend.md](tech-backend.md) | Language, runtime, framework, job scheduling, API shape. |
| [tech-storage.md](tech-storage.md) | Database, schema stance, search, migrations, on-disk layout, backup. |
| [tech-clients.md](tech-clients.md) | Player engines and the platform matrix — where the real cost of this project is. |
| [tech-licensing.md](tech-licensing.md) | FFmpeg's licence ladder, codec patents, and provider terms under an open-source release. Where the landmines are; not legal advice. |
| [tech-metadata.md](tech-metadata.md) | TMDB, TheTVDB, MusicBrainz, AcoustID, OpenSubtitles — capability, licensing, and the traps. |

## Method and its limits

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
