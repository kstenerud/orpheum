> **ABOUTME:** How Plex, Emby, Jellyfin, Kodi, Navidrome, Audiobookshelf, Immich, Home
> Assistant, Syncthing, Gitea/Forgejo, Nextcloud and the *arr apps came to package and update
> their software the way they do now: a dated timeline of the failures behind each change, and
> fourteen lessons for this project.

# Packaging history: how competitors got to their current shape

- **Date:** 2026-09-23. **Question:** *How have competitors packaged their products, and which problems over their lifetimes led them to their current shape?*
- **Scope:** history only. The current state and current complaints are in [audit-2026-09-23.md](audit-2026-09-23.md) §6 ("audit2" below; its full working report is `/yoloai/cache/research/audit2-packaging.md`) and are not repeated here.
- **Evidence:** four sets of per-project notes, each with a full citation on every item: `/yoloai/cache/research/pkg-history-notes/{jellyfin-emby,plex-kodi-nas-stores,immich-ha-navidrome-abs,syncthing-gitea-nextcloud-arr}.md`.
  - Web pages are cached in `/yoloai/cache/http/pkg-history/`. The note files explain how the cached files are named.
  - Clones in `/yoloai/cache/repos/` now have full history: jellyfin, jellyfin-ffmpeg, jellyfin-packaging, docker-jellyfin, Emby, pms-docker, docker-plex, PlexDBRepair, kodi, kodi-tv, immich, ha-core, ha-architecture, ha-supervisor, ha-operating-system, navidrome, audiobookshelf, syncthing, gitea, Sonarr, Radarr, Tautulli, nc-updater, nc-all-in-one, nc-docker and nc-nextcloud-snap.
- **Citation shorthand:**
  - `#N` is an issue or PR in that project's main repo.
  - `t/N` is a thread on that project's Discourse forum (Plex, Syncthing, Nextcloud or Sonarr).
  - Hex strings are commit hashes in the clone.
- **UNVERIFIED** marks anything I could not confirm. WebSearch and the GitHub API quota ran out during the work, so some items rest on a single source.

---

## 1. Timelines

### Plex Media Server: a closed binary, a moving platform floor, and vendor packages

- **2016-05 (0.9.17):** dropped ARMv5 and PowerPC NAS (Synology, QNAP, Netgear, WD, Thecus), 32-bit Unraid, Vista and OS X 10.6/10.7. **Old installers are not offered** (support article "platforms no longer supported").
- **2016-12:** the first commit of `pms-docker` already had tags with no server inside, which download PMS at container start. Five days later `latest` got a server built into the image (a6a7666, b2c4f81). The reason is **UNVERIFIED**; the likely one is that beta downloads need the user's Plex Pass token.
- **2017-12:** the Plex Pass channel became "beta" (7529bef). The same commit window introduced `[ "$PLEX_UPDATE_CHANNEL" > 0 ]`. Inside `[ ]` that `>` is a shell redirect, not a comparison, so the channel override silently did nothing. It was **fixed on 2026-09-08, nine years later** (8438f12 → d6d2684).
- **2018–2020, apt repo:** recurring `NO_PUBKEY` and missing-`Release.gpg` errors (t/234698, t/663368).
- **2019-02 (1.15):** a new build toolchain needed glibc 2.14.1. That dropped CentOS 6 and every 32-bit QNAP (t/375580). Plex Web then **refused servers older than 1.15.2**, which left QTS 4.2 owners with no server they could run (t/635181).
- **2020-11 → 2021-06, Synology DSM 7:** Plex had to rewrite its package.
  - The service user became the system-internal `PlexMediaServer`, which users must **grant read/write on every share**. Staff: *"This change was unavoidable"*.
  - The data migration is one-way.
  - Package Center kept serving the stale 1.21 beta into late 2022 (t/653717, t/719845, t/794410, t/819896).
- **2022-07:**
  - The Linux postinst began **rewriting the user's apt source itself** to keep the key current (t/799912). The rpm key was still SHA-1 signed (t/800755).
- **2023-09:** 1.32.6/1.32.7 were **withdrawn for breaking Linux HW tone mapping**. Fixed builds were tested in the forum before re-release (t/853757).
- **2024-07:** plug-ins removed because Python 2.7 is dead. Phase 1 was **Shield-only**, chosen to hit the *"fewest number of users"* (t/883484).
- **2024-10, DSM 7.2.2:** broke the stable PMS package again; users needed a separate `DSM72` SPK (t/893174).
- **2026-01 (1.43.0):** a new build system, a new `repo.plex.tv` and a new Ed25519 key. The automatic repo migration **broke apt and dnf**, and production was **rolled back to 1.42.2** (t/935847, 395 posts). The new repo carries no betas.
- **2026-03:** 32-bit Windows frozen at 1.42.2 because of *"changes in how we build and deploy"* (t/937610).
- **2026-08:** 1.43's schema changes broke the community PlexDBRepair tool (cf02006). That tool exists because corrupt Plex databases are common (2cb2d51, 2022).
- **linuxserver/docker-plex:**
  - 2015: runtime auto-update, with the version feed hosted by fanart.tv.
  - 2019-02: `VERSION=docker`, meaning no runtime update, became the opt-out and later the default (b10368d).
  - 2024: runtime update disabled for read-only and non-root containers (9e313af, b29a1b8).

### Emby: closing the source was a distribution decision

- From 2015 it shipped vendor NAS packages one at a time: ReadyNAS, Asustor, TerraMaster, WD, QNAP ARM (Emby blog listing, 2017–18).
- May 2018: the media encoding library was already a *"closed source binary"* (#3269).
- **2018-08-08:** Joshua Boniface, later Jellyfin's leader, published an `emby-server` Debian package that **patched out the Premiere payment checks** (jellyfin c0953e51b).
- **2018-09-19:** last public commit, 3.5.3 (1d7c2ab4b). A maintainer confirmed in 2020 that the code went closed (#3647).
- 2018-11/12: hardware acceleration and web Live TV moved behind Emby Premiere, and a 4.0 database restructure followed (Emby blog 443, 447).
- Today Linux installs are **per-URL `dpkg -i`, with no apt repo**, so there are no package-manager updates. The Docker image says *"the in-place update feature [is] gone"* since .NET Core.
- Self-update on Windows/macOS is **UNVERIFIED**.

### Jellyfin: every major packaging change followed a failure

- **2018-12 fork:** *"as a result of Emby's decision to take … 4.x closed-source"*.
  - The first Dockerfile used **distro `apt install ffmpeg`** and PUID/PGID via gosu (206bc7911).
  - PUID/PGID was dropped for `--user` eight days later (def954e8d).
- **2019-01-04:** Docker switched to a static FFmpeg 4.0.3 because it *"fixes chromecast + androidtv"* (2961dc0bd).
- **2019-01-21:** a fix for FFmpeg 4.1 was reverted because it broke HLS segmenting (b2767ecef).
- **2019-02-14:** **jellyfin-ffmpeg created** as a fork of Debian's ffmpeg packaging (JF a91e4632).
  - At first the deb depended on `ffmpeg (<7:4.1) | jellyfin-ffmpeg`.
  - 2019-02-25: *"No longer try to override the system ffmpeg, just put ours somewhere else"* (5054a77dc).
  - The patches then grew into the reason it exists: VAAPI/NVENC (2019), OpenCL and VAAPI tonemapping (2020), CUDA (2021), Dolby Vision (2022) and CPU SIMD tonemapping (2024).
- **2019-01 → 2020-03:** packaging lived in the server repo as ~20 per-distro Docker build directories (399a079dd, 6028bc0f7).
- **2019-04 (#1250):** a 10.3.0 image reached `latest` **before the official release**. Watchtower and Ouroboros users got a broken server, and one reported a wiped `/config`.
- **2020-06 (10.6):** split `jellyfin-server` + `jellyfin-web` + metapackage builds on Azure. The previous system was *"duct-tape-and-coat-hanger scripts"* (blog "packaging updates"). The first EF Core moves came in the same release.
- **2020-04:** the Docker image switched to the jellyfin-ffmpeg deb to *"avoid non-free drivers"* (7b2fca96e).
- **2020-11:** Intel's OpenCL runtime went into the image for tonemapping (b26a5e9e2). The apt key was about to expire, and **there was "no convenient way to update"** it; it was re-issued with no expiry (blog "jellyfin apt key").
- **2021-06:** linuxserver pinned Jellyfin's dependency versions *"to prevent upstream apt repo issues"* (LS 36f190b).
- **2022-05 onward:** the jellyfin-ffmpeg package is renamed each FFmpeg major (`jellyfin-ffmpeg5`/`6`/`7`) so old servers never pull a new FFmpeg (b943381c).
- **2022-06 (#7928):** 10.8.0 through linuxserver `latest` plus Watchtower produced a start loop.
- **2024-03-05, "New CI, New Repo":**
  - The split build *"failed more times than we could count"*. A major release took *"over 20 man-hours"*.
  - Packaging moved to `jellyfin-packaging` on GitHub Actions.
  - Non-LTS Ubuntu dropped, and Jellyfin's own Fedora/CentOS RPMs dropped (*"poorly maintained … broke frequently"*) in favour of RPMFusion. GHCR added.
  - The server repo's packaging was deleted on 2024-03-18 (#11162).
- **2024-05 (10.9):**
  - A migration failed on release day (#11532).
  - Watchtower-driven 10.9.2 breakage (#11715).
  - **X and X.Y tags were added only a month later**, on 2024-06-06 (JP 3831122).
- **10.9.3→10.9.6 (#12035):** a **patch release carried a DB migration**. The official image's healthcheck plus a restart policy looped the migration and left `library.db.bak2…bakN`, one per attempt. The maintainer asked users to remove healthcheck-based kills.
- **2024-10 (10.10):**
  - 32-bit ARM deprecated as *"popular but extremely poor in performance"*.
  - The **server refuses to start without the correct FFmpeg** *"to avoid mis-reported issues"* (#12463).
  - 10.10.2 had a migration write into a read-only library (#13048).
- **2025-06:** armhf removed (JP 499fddf). A `preview` channel was added.
- **2025-10 (10.11):** the library.db→jellyfin.db EF Core migration saga (see `migrations-selfhosted.md`). Proxmox LXC+ZFS SQLite locking was *"amplified"* (#15101).
- **2026-09 (12.0):** the "10." prefix was dropped because 10.11 *"was … a major release"* that *"the version number presented … as a minor update"*.
- Explicit Windows 7 and old-macOS drop notices: **UNVERIFIED** (implied by .NET 8 in 10.9).

### Kodi (client-side counterpoint)

- Add-ons come from in-app repos, one branch per codename, with versioned add-on APIs. Python 3 was announced to add-on developers **three years early** (2018-01).
- Even so, in 2021-03 **Play Store and Windows Store auto-updates pushed users onto Matrix and broke their add-ons**. The official advice was to turn auto-update off (kodi.tv "kodi-broke-my-addons").
- Store history: Amazon refused Kodi in 2015. iOS is sideload-only, because relicensing the GPL code would need every contributor's consent.
- Google Play's target-SDK bump is forced every year (*"We don't know what will break"*, 6a5eaee3e1). Each major raises the platform floor, announced in the alpha notes.

### Navidrome

- **Docker came first** (2020-01), with Alpine's ffmpeg. goreleaser tarballs followed four days later.
- **Packages:**
  - The UI was embedded with go-bindata, then `embed` (v0.41, 2021-03).
  - Non-Docker Linux meant "copy the contrib systemd unit". That unit's own **sandboxing blocked ffmpeg** (a18093e2, 2020-04).
  - deb/rpm/MSI plus `service install` arrived only in **v0.54.1 (2024-10)**.
  - The MSI fetches ffmpeg from a `latest` release URL. A tail of MSI bugs followed: ini overwritten (#3836), no ffprobe (#5326), settings lost on upgrade (#5333, 2026-09).
  - A kardianos library bump broke the systemd template (#5743).
- **Data:**
  - v0.50 made albums look new and lose stars and play counts unless the user opted out.
  - v0.55 (2025-03) forced a full rescan with *"avoid using Navidrome"* until it finished.
- **cgo/TagLib:**
  - TagLib (cgo) was the metadata reader from 2020.
  - A pure-Go/WASM TagLib landed in 2026 (#4902) to *"simplify cross-platform builds and packaging"*.

### Audiobookshelf

- **Channels:**
  - Docker from day one (2021-08).
  - A Debian PPA whose **`preinst` downloaded an unpinned ffmpeg *git* build from a personal site** on every reinstall, amd64 only (bc47dfa3, #633). It was removed in v2.12.0 (2024-07).
  - The Windows tray app came only in 2024-01, after two years of "looking for a .NET developer". RPM is community-maintained.
- **Runtime downloads:**
  - v2.7.2 added a BinaryManager that **downloads ffmpeg 5.1 from ffbinaries.com at server start**.
  - From v2.12 it also fetches a SQLite unicode extension.
  - `SKIP_BINARIES_CHECK` had to be added as an escape hatch (#2741).
- **v2.3.0 (2023-07), JSON → SQLite:**
  - It took three hotfixes in four days, for incomplete migrations and out-of-memory crashes.
  - The fix was *"delete `absdatabase.sqlite` and restart"* (#1913, #1926, #1931, #1938).
  - v2.14 (2024-09) added backup-before-migrate, restore-on-failure and down-migrations on downgrade.

### Immich: Compose only, and a long run of hand edits

- **Compose only since 2022-02-03** (85b83f966). There was never a non-Docker path, and I found no stated rationale (**UNVERIFIED**). The likely reason is the multi-runtime stack: Node, Python ML, Postgres and Redis.
  - Over time the docs only narrowed what they support: NTFS/WSL warned against 2024-05, non-Linux 2024-12, Docker Desktop 2025-01, LXC 2025-01.
- **Compose/.env hand edits a user had to make to upgrade:**

  | Version | Date | Change |
  | --- | --- | --- |
  | 1.48 | 2023-02 | ML container reworked; ML dropped on arm64/armv7 |
  | 1.51 | 2023-03 | Typesense container plus a required key |
  | 1.86 | 2023-11 | compose `name:`, which the notes tell existing users to *ignore* or their volumes are orphaned |
  | 1.88 | 2023-11 | proxy and web containers removed; port change |
  | 1.91 | 2023-12 | Typesense replaced by **pgvecto.rs** (because users' CPUs lacked AVX); a stale `LOG_LEVEL=simple` stops the server from starting |
  | 1.95 | 2024-02 | hand-written extension-upgrade SQL for non-superusers |
  | 1.106.1 | 2024-06 | microservices container removed; environment variables renamed |
  | 1.118 | 2024-10 | internal port change, which broke reverse proxies |
  | 1.133 | 2025-05 | pgvecto.rs → **VectorChord** via Immich's own Postgres image; **upgrades refused from anything older than 1.107.2** |
  | 1.137 | 2025-07 | must have started once on ≥1.132 |

  1.106.0 was skipped while the mobile app waited for store review.
- **Watchtower:** the "take special care with Watchtower" warning was added 2023-01 (#1342) and removed on stable day (#22549).
- **Stable:** v2.0.0 on **2025-10-01**, with semver and "any 2.x app works with any 2.x server". The 2026-02 policy says breaking changes only in majors, a `:v2` tag, and **no downgrade even within a minor**.
- **v3.0.0 arrived only nine months later** (2026-06-30) and removed pgvecto.rs.
- **Unraid:** demoted from official to community on 2026-07-30 (#30323). Unraid's Docker lacked the healthcheck `start_interval` parameter (#10406).

### Home Assistant: five install methods, trimmed with data

- **Early:** pip (2013), Docker (2014), a `curl | bash` Pi installer (2016), and the Hassbian image (2016, **retired 2019-10**: *"Hass.io has surpassed Hassbian"*).
- **2017-07, Hass.io:** ResinOS + Docker, with UI updates, snapshots and add-ons, because *"users … spend a lot of time maintaining their systems"*.
- **2018-07, HassOS:** there was **no over-the-air path from ResinOS, so users had to reflash**.
- **2019-07, ADR-0006:** two image families were collapsed into one because *"our Docker team and resources are limited"*.
- **2020-01:** renamed. The 2025 blog admits the names still confuse people.
- **2020-05:** the Supervised deprecation was **put on hold** because the method was *"more popular than expected"*. At the time they had no numbers.
- **2020-06, ADRs 0012–0016:** "supported" means tested and documented. Supervised got strict host rules (Debian only, no other software, cgroup v1).
  - The Supervisor marks hosts that break the rules "unsupported" and blocks updates on them.
  - The cgroup v1 rule had to be **relaxed in 2024** once distros dropped v1 (supervisor #5419).
- **2019–2025, Python floor:** policy went from "latest two minors" to "a single minor" (ADR-0020, 2023). Core-on-venv users had to *"compile Python from source"*.
- **2021-11:** reached 100k installs in opt-in analytics. Later deprecations cite this data.
- **2025-05-22:** Core, Supervised and 32-bit deprecated using analytics (i386/armhf <0.5% each, armv7 0.95%). The notice was shown for six months, and support ended in 2025.12. Core stays as the developer path.
  - 32-bit builds were cut in 2025-11.
  - The ADR text was formally reverted only in **2026-04**.

### Syncthing: self-upgrade, done carefully

- **2014:**
  - 2014-05: `-upgrade` added.
  - **2014-08: the `noupgrade` packager build tag, only 3.5 months later.**
  - 2014-09: background auto-upgrade **on by default**, every 12 h.
  - 2014-11: `STNOUPGRADE` env var.
  - 2014-12: auto-upgrade no longer crosses breaking versions.
- **2015-08:** only signed upgrades accepted.
- **2016-05 (#3044):** an outside reporter found a **MITM downgrade**, because the signature covered only the binary. The fix signs archive name, version, arch and binary together (d6a7ffe0).
- **2017:** a release-candidate channel where **auto-upgrade and usage reporting are mandatory**: an opt-in canary the project can see. Usage reports double as the install census, e.g. "15% on our Debian packages".
- **2018-02:** Docker image, with `STNOUPGRADE=1`.
- **2020:**
  - A crashing release candidate could not auto-upgrade out of its crash (#6384). The fix was reverted.
  - **v1.5.0 auto-upgraded users who had disabled it** (#6701).
- **2023:**
  - Go 1.21 dropped Windows 7/8. The maintainer called per-OS upgrade gating infeasible (t/19743). A release note naming Windows 7 is **UNVERIFIED**.
  - **The move to GitHub Actions dropped the flag, and Docker images shipped with auto-upgrade on** (d421d66a).
  - Debian's own package was stuck at 1.19. Project line: *"use syncthing's distribution"* (t/20931).
- **apt repo trouble has always been keys and TLS:** the Let's Encrypt root expiry (2021), the `apt-key` → `signed-by` switch breaking Ubuntu 22.04 upgrades (2022), and an unannounced CDN domain move that users read as tampering (2025, t/25281).
- **2025-08, v2.0:**
  - LevelDB → SQLite. Announced three months ahead in v1 release notes.
  - **Per-major apt suites**, and auto-upgrade that no longer jumps majors.
  - **Seven platforms' prebuilt binaries dropped because SQLite (cgo) made cross-compiling hard.**
  - The migration then needed a **temporary API server so healthchecks stop killing it**, plus one-transaction migrations (d776657b, e41d6b9c).

### Gitea / Forgejo

- **Gitea's rootless image (2020-11, #10154)** was *"a breaking solution that users can transition slowly"* toward a new default. It never became the default, and both images still ship.
- Gitea dropped i386 in 2026 when it moved off cgo cross-compiling. Signing moved from GPG (2018) to sigstore (2026).
- **Forgejo** (fork, 2022-12):
  - Mis-set tags made distros ship **v1.19 labelled v1.18**, which could not be downgraded (2023-02).
  - v1.20.3 **refuses unsafe upgrades and all downgrades**, after a point release silently moved storage directories (2023-08).
  - v7 (2024-04) was the first LTS and switched to semver.
  - Gitea 1.22 is the last version that migrates transparently to Forgejo.

### Nextcloud: the web updater, then its escape routes

- **The PHP web updater dies on web-server timeouts**, which is why a CLI mode exists (2016).
  - A stale `.step` file left users stuck on *"Step N is currently in process"* for years; thread t/58371 has 92k views. Real fixes came in 2023 and 2025.
  - Downloads were still failing from a single server with HTTP 429 in 2025.
- **"Don't skip majors"** was documented but **enforced by the snap only in 2020 and by Docker only in 2023**. That was after images refused to start on volumes they could not upgrade (a68c6ef), and FreeBSD packages jumped 26→28.
- **PHP floors stranded distro installs.** Debian 11 tops out at Nextcloud 25, and Debian 12's PHP is too new for 25, so there is no in-place path (t/181410).
- **Snap:** Canonical's author made ~1,000 commits, then largely left after 2023. The snap moved to core20 only after core20's EOL.
- **Docker:** the official image has been *"community volunteers … expert use"* since 2024-02.
- **All-in-One (2021-11):** a mastercontainer holding the **Docker socket**, which creates, updates and backs up (Borg) every other container. It exists so the install *"is not bound to the host system (and its slow updates)"*. It is now "the official installation method".

### *arr apps and Tautulli

- **Update mechanism:** the `package_info` file with `UpdateMechanism=Docker|Apt|…` (2018) disables the built-in updater under packaging. The UI has warned since 2023.
- **Mono → .NET:**
  - Radarr dropped Mono in 2021.
  - Sonarr v4 (beta 2022-11, stable 2023-12-30) dropped Mono, Linux x86 and BSD, **required a manual reinstall**, and v4 wasn't in Sonarr's own apt repo during the beta. v3 databases that were already corrupt broke (t/31078, t/33089).
  - The trigger was Mono's own apt repo shipping broken versions on Ubuntu 22.04, with the forum answer *"I run Sonarr in docker"* (t/32699).
- **Tautulli:**
  - Python 2→3 (2019–2020). It **disabled its updater on Python 2** rather than push an update that wouldn't run.
  - A Windows exe auto-updater lasted two months (v2.6.5→v2.6.7) because **antivirus flagged it**.
  - Its git self-updater needed a "reset git installation" button (2020), and a tar path-traversal fix as late as 2026-08.

### NAS and client stores

- **Synology DSM 7.0 (2021-06)** dropped about 50 packages and removed runtime root and the share-permission helpers from third-party packages (SynoCommunity #4524).
- **TrueNAS:**
  - TrueCharts stopped SCALE releases (2024-05).
  - 24.10 moved apps from k3s to Compose and **auto-migrated only the official apps**.
- **QNAP:** Deadbolt (2022) led to forced firmware auto-install.
- **Amazon disabled the live Jellyfin Android TV app** while an update awaited review (jellyfin-androidtv #2377, 2022-12).
- **Roku** ended private channels (2022).
- **LG:** the store took Jellyfin only for webOS 6+, and developer-mode sideloads expire.
- **Plex** carries old unlocks over only on the same store account (2025).
- Apple rejections of Jellyfin, Swiftfin or Emby apps: **UNVERIFIED/not found**.

---

## 2. Cross-cutting themes (detail is in the lessons)

- **Why Docker won.** Every other channel kept costing money: Jellyfin's RPMs, Mono's apt repo for Sonarr, Python and PHP floors for HA and Nextcloud, Debian's stale Syncthing. Containers fixed the userland. The trouble moved to orchestration: floating tags, Watchtower, healthchecks, and compose files users edit by hand. AIO and the fetch-at-start images were attempts to win back control of updates inside Docker, and each took years to get right.
- **FFmpeg.** Projects that used the system FFmpeg all hit breakage: Jellyfin, Navidrome's unit, Synology's Jellyfin. Projects that *downloaded* it hit a different kind: Audiobookshelf, Navidrome's MSI. Bundling a pinned build won every time. What it cost Jellyfin was a patch set it has to maintain for hardware acceleration.
- **Self-update.** Every in-app updater went the same way: an updater, then an off switch for packagers, then a bug that turned it back on (Syncthing, *arr, linuxserver, Tautulli). The ones that survived either sign version metadata (Syncthing) or run only inside their own appliance (HA OS, AIO).
- **Migrations × packaging.** The worst incidents paired a migration with something else: a floating tag, a healthcheck, a skipped major, a patch release, or no way to re-run it.
- **Hardware-acceleration drivers** live in the image (Jellyfin's Intel OpenCL runtime, 2020) or on the host, never inside the FFmpeg binary. Since 10.9, packages add the service user to `render`/`video`. Plex twice pulled releases over tonemapping.
- **NAS vendors** change privilege models whenever they choose (DSM 7, 7.2.2, TrueNAS twice), and their stores fall behind. The channel that lasts is an OCI image run by the vendor's container runtime.
- **Client stores** can disable a live app (Amazon), push a breaking update nobody chose (Kodi on Play and the Windows Store), freeze releases (Roku), or shut out older TVs (LG). Sideloading is the escape hatch, but it is narrowing.
- **Data drives clean drops.** HA's analytics, linuxserver's arch counts and Syncthing's opt-in reports made their deprecations uncontroversial. HA without data (2020) had to reverse itself. Most drops came as side effects of a toolchain change and were announced late or not at all.

## 3. Lessons (14), with implications

1. **Never float a tag across anything that migrates the database, and don't hide migrations in patch releases.**
   - Incidents: Jellyfin #1250 (a pre-release on `latest`), #7928, #11715, #12035 (a migration in 10.9.6), and the 12.0 renumbering. Immich's Watchtower warning, 2023–25.
   - *Implication:* schema migrations only in `X.Y` or higher, never in `X.Y.Z`. `X.Y.Z` must be migration-free by CI assertion: compare the schema version at the two tags.
   - Confirms audit2 §3.5 (no `latest` across majors). **It goes further:** a user on the `X` tag still receives minor-version migrations. D181's snapshot must therefore fire on every schema change, and the release notes must flag "this release migrates".

2. **A pinned FFmpeg is built and shipped by us, and the server refuses to run with the wrong one.**
   - Incidents: Jellyfin's system FFmpeg broke Cast and HLS (2019), leading to jellyfin-ffmpeg and the #12463 gate. Audiobookshelf's unpinned downloads (#633, BinaryManager). Navidrome's MSI `latest` URL.
   - *Implication:* confirms audit2 §3.2 in full, including never downloading at runtime.
   - **Adds:** version the bundled FFmpeg's package or directory by major, as `jellyfin-ffmpeg7` does, so an older server never receives a newer FFmpeg. Make the hash check a hard gate for the *transcoder* plugin. Keep audit2's `ffmpeg_path` escape hatch as a *visible* override (D183).

3. **Keep the pre-migration backup, make the migration re-runnable, and support a guided downgrade.**
   - Incidents: Audiobookshelf 2.3.0 ("delete the sqlite and restart", out-of-memory crashes) and its fix in 2.14 (backup, restore on failure, down-migrations). Jellyfin deleted its backup on success (audit2). Immich's "no downgrade even within a minor". Forgejo's refusal to downgrade after mis-tagged releases.
   - *Implication:* confirms D181 and audit2 (keep the snapshot; an old binary names `restore --snapshot`). Test resumability by **killing the migration mid-step in CI**. Nextcloud's stale `.step` file is the failure to avoid: a resume marker must be validated before it is trusted.

4. **Health must say "migrating", and every channel's supervisor must respect it.**
   - Incidents: Jellyfin #12035 (the official healthcheck killed the migration and left N backups). Syncthing v2 had to add a temporary API server mid-migration. Unraid lacked `start_interval` (Immich #10406).
   - *Implication:* confirms audit2 (`/health` with a `migrating` state, a generous `start_period`). **Extends it:** the HTTP listener must come up *before* the migration and answer 200 `migrating`, so a runtime without `start_period` (Unraid, Kubernetes) still waits. The systemd units must not use `WatchdogSec`/`TimeoutStartSec` values a long migration can hit; use `Type=notify` with `EXTEND_TIMEOUT_USEC`.

5. **Enforce upgrade paths in code, not docs.**
   - Incidents: Nextcloud's "don't skip majors", broken by FreeBSD and by Docker until 2023. Immich's 1.107.2 and 1.132 floors. Forgejo refusing unsafe upgrades.
   - *Implication:* the binary records the minimum version it migrates from and refuses below it, naming the intermediate release to install. Old releases stay downloadable forever (D108, audit2). **Plex's "we do not provide old versions"** is the counter-example.

6. **Self-update is off in every packaged build, and the off switch is tested in CI.**
   - Incidents: Syncthing `noupgrade` (2014), #6701 (ignored the setting), d421d66a (a CI migration re-enabled it in Docker). *arr `package_info`. linuxserver `VERSION=docker`. pms-docker's broken channel check, silent for nine years. Tautulli's updater flagged by antivirus.
   - *Implication:* confirms audit2 (never self-update automatically; `mediaserver upgrade` for tarballs only). **Add a CI assertion** that the deb and image builds report `upgrade: disabled (package)`. A shell-script bug is exactly the kind of silent regression that goes unseen. Postpone any Windows self-updater until Authenticode signing exists.

7. **Sign the metadata, not just the payload.**
   - Incident: Syncthing #3044 (a MITM downgrade, because the signature covered only the binary).
   - *Implication:* audit2's minisign-signed release feed must bind **version, arch, channel and SHA-256** in the signed bytes. The client must also refuse a feed older than the newest one it has seen (anti-rollback). This confirms and sharpens audit2 §3.5.

8. **Rotate repo keys on purpose and test them against the strictest distro.**
   - Incidents: Jellyfin's apt key nearly expired with no rotation path (2020). Plex's `NO_PUBKEY` years and the 1.43 key and repo move, which broke apt/dnf and forced a rollback (2026). Syncthing's `signed-by` switch (2022) and unannounced CDN move (2025).
   - *Implication:* ship the key in a `mediaserver-archive-keyring` package from day one, so rotation is a package update. Use Ed25519/RSA-4096 with no SHA-1 anywhere. Run a CI job that `apt update`s against the repo on the newest Debian's `sqv`. Announce any domain move in the signed feed first. **Not in audit2; add it.**

9. **Treat a toolchain change as a platform-deprecation event, and publish the platform floor.**
   - Incidents: Plex 1.15 and 1.43, Go 1.21/Windows 7 (Syncthing t/19743, Gitea), Sonarr .NET/linux-x86, Syncthing v2 losing seven platforms to cgo-SQLite, Gitea dropping i386 when leaving cgo.
   - *Implication:* write the supported OS/kernel/arch matrix down (a `PLATFORMS` file). A Go-version bump in `go.mod` is a release-note item. Stay `CGO_ENABLED=0` with the pure-Go SQLite. Syncthing and Navidrome both paid for cgo in portability, which confirms audit2 §3.1. Announce a drop at least one release ahead, as Jellyfin did for armhf and HA did with six months.

10. **Don't support an install method you can't test in CI. Decide the list with data, and keep the developer path.**
    - Incidents: HA's 2020 Supervised reversal, made without data, and the 2025 cut made with data. Jellyfin dropping its own RPMs and non-LTS Ubuntu. Immich narrowing its supported hosts and demoting Unraid. Gitea's rootless image never becoming the default.
    - *Implication:* confirms audit2 (supported = CI-tested; everything else community). **Challenge:** we have no telemetry (D13/D183), so we will lack HA's data when deciding to drop armhf or a distro. Options:
      - Use GitHub issue-form fields as the census, as audit2 did.
      - An **opt-in** anonymous "install method, arch, version" ping, asked at setup and visible (D183).
      - Syncthing-style: opt-in reporting required only for a release-candidate channel.

      Decide before the first public release.

11. **Configuration the binary owns beats configuration the user edits.**
    - Incidents: Immich's ten rounds of compose/.env hand edits (a stale `LOG_LEVEL` stopped the server from starting; ignore `name:` or lose volumes). Navidrome's MSI overwrote the ini. Sonarr v4 required hand-editing `ExecStart`.
    - *Implication:* confirms D182 §3 (the config file holds only pre-database settings; everything else lives in the database and is migrated with it). The documented compose file should have nothing to change between releases except the tag. **Unknown or renamed config keys must warn and continue, not refuse to start.** Mark deb conffiles `noreplace` (audit2) and never rewrite them in postinst. Plex's postinst rewriting the user's apt source is the anti-pattern.

12. **A single dependency-free artifact is the durable NAS channel; vendor packages are a treadmill.**
    - Incidents: DSM 7 took away root, share helpers and about 50 packages. Plex needed a separate DSM72 SPK. Package Center served a stale Plex beta for over a year. TrueCharts disappeared. TrueNAS migrated only official apps.
    - *Implication:* confirms audit2 §3.1 (no SPK; seed TrueNAS, Unraid and Umbrel catalogs from our OCI image). **Adds:** keep catalog entries *in our repo and CI* where the format allows (Audiobookshelf's Unraid template), so a catalog's collapse costs a README, not users.

13. **The build pipeline is part of the product; one pipeline, one artifact set, run on every release.**
    - Incidents: Jellyfin's split server/web builds cost *"over 20 man-hours"* per major until the 2024 rewrite. Forgejo's mis-set tags shipped a non-downgradable build. Syncthing's CI move dropped a build flag. Plex's build-system change broke signing.
    - *Implication:* confirms audit2 §4.6 (goreleaser; one tag → all artefacts, reproducibility diff, install matrix). **Adds:** CI asserts on build *properties*: self-update flag, FFmpeg hash, embedded web version equal to server version, tag equal to embedded version. That catches a flag silently lost in a CI move. Jellyfin's *separate packaging repo* was about their .NET/web split. For a Go single binary, keeping packaging in-repo is fine, so this does **not** challenge audit2.

14. **Clients must never demand a server version the server's platform cannot reach, and stores must not be the only path.**
    - Incidents: Plex Web's "requires 1.15.2" plus no builds for 32-bit QNAP. Kodi Matrix through store auto-update. Amazon disabling the live Jellyfin app. Plex unlocks tied to a store account. LG developer-mode expiry.
    - *Implication:* confirms D108 and audit2 §3.6–3.7 (the server offers its own APK; the Cast receiver serves every server version through the advertised API version). **Adds:** compatibility is decided *by the server's advertised API range*, never by a client-side minimum. A client that cannot talk to a server shows the server's recommended client link, served by that server, rather than refusing.


## 4. Net effect on audit2

- **Confirmed:** most of §3 (L1, L2, L6, L10, L12–14).
- **Sharpened:** L1 (no migrations in patches), L4 (listener before migration; systemd timeouts), L5 (upgrade floor in code), L6 (CI asserts self-update off), L7 (anti-rollback feed), L13 (build-property assertions), L14 (server API range decides).
- **New:** L8 (keyring package; `sqv` test), L9 (published platform floor), L2 (FFmpeg versioned by major), L11 (unknown config keys warn).
- **Challenged:** audit2's "supported" list presumes decisions without usage data (L10).

## 5. Unverified or not found

- Rationale for pms-docker's download-at-start design.
- Emby's desktop self-update.
- Explicit Windows 7 drop notices from Jellyfin, Syncthing or Gitea.
- Immich's stated reason for Compose-only.
- The Immich AIO "removal" (likely false).
- Apple rejections of Jellyfin, Swiftfin or Emby.
- A QNAP advisory naming Plex.
- hotio's image history.
- The Nextcloud VM's origin.
- The exact year of Plex's deb SHA→GPG change.
