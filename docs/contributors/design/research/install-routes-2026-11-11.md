# How competitors are installed, which routes matter, and how Orpheum can support them

Research for the owner, 2026-11-11. The question was: *"Look at all the ways that our competitors are installed, which ones are the most important, and how we can support them as painlessly as possible (similar to our recent pass for omv and synology)."*

Scope: Plex, Jellyfin, Emby, plus a few routes that matter to self-hosters in general. **Evidence** marks measured or published facts, with their source. **Guess** marks my judgement. Anything that would be a design decision is marked **Proposal (owner)**.

---

## 1. Bottom line

- **Docker won, and so did the NAS panels that run compose.** Among self-hosters, 92% of container users run Docker, and 88% of one survey's respondents use Docker Compose. Every competitor's NAS story now ends in "run our image" (Jellyfin's Synology page installs the image in Container Manager; TrueNAS's Jellyfin app is a compose file). Orpheum's single image plus generated compose files is the right base. Most of the remaining work reuses it.
- **By user share, the biggest uncovered routes are Proxmox, TrueNAS and Unraid.** In the 4,081-response selfh.st 2025 survey: Proxmox 45%, TrueNAS 19%, Unraid 10%, Synology DSM 10%, OpenMediaVault 5%. Orpheum has a documented route for DSM and OMV, but none for the three larger ones.
- **Orpheum's three-service layout rules some stores out.** The core runs on host networking. The node has no network and talks to the core through a socket in a shared folder. An init container sets ownership, and start order depends on health. Any store that runs compose (TrueNAS, CasaOS/ZimaOS, Umbrel, Synology's package worker, Portainer, QNAP, UGREEN) can take this. A store whose entries are **single containers** cannot: Unraid's XML templates are the main one, and so is the Proxmox helper-script project, which refuses Docker altogether.
- **Recommended order, least pain first:**
  1. Proxmox: a documented "Docker in a VM or LXC, then the install script" route.
  2. Unraid: a route through its Compose Manager plugin, using a generated file with Unraid's own defaults.
  3. A TrueNAS community-train app.
  4. A prototype Synology package (SPK) that installs the existing compose project, which could remove the SSH `id` step.
  5. Submit the CasaOS and Portainer entries we already have.

  Skip native Windows/macOS installers, Flatpak/Snap, YunoHost, Home Assistant add-ons and Helm for now.

## 2. Summary table

Importance draws on §4: H = high, M = medium, L = low. Effort is for Orpheum: S = days, M = a week or two plus upkeep, L = new artifact type or a design change. "Reuse" means the generated compose file and image (D462 §10.2) carry over.

| Route | Plex | Jellyfin | Emby | Importance | Orpheum today | Orpheum effort | Reuse |
|---|---|---|---|---|---|---|---|
| Docker image + compose (any Linux) | official `plexinc/pms-docker`, lsio, hotio | official `jellyfin/jellyfin` (+ghcr), lsio, hotio | official `emby/embyserver`, lsio | **H** | done (`orpheum-docker.yaml`, install script) | — | — |
| Portainer (template / stack) | community templates | community templates | community templates | **M–H** (33% of all respondents use Portainer) | template built, not submitted | S (submit to a community list) | full |
| Proxmox VE (helper scripts / LXC / VM) | community-scripts LXC (native) | community-scripts LXC (native .deb) | community-scripts LXC | **H** (45%) | nothing written | S (docs + one test) for Docker-in-LXC/VM; L for a helper script (needs a native package) | full / none |
| TrueNAS SCALE app catalog | **stable** train (iX-maintained) | community train (maintainer: truenas) | **stable** train | **H** (19%) | not built | M (app.yaml, questions.yaml, Jinja template) | partial: same image and flags, their template |
| Unraid | listed by Plex; CA templates | CA templates (several maintainers) | listed by Emby | **M–H** (10%, media-heavy users) | nothing | S via Compose Manager plugin; L for a CA XML template | full (plugin) / none (XML) |
| Synology DSM | **Package Center (official SPK)**, Docker | Container Manager (docs); SynoCommunity SPK | Package Center SPK, Docker | **H** for households (33% of NAS owners) | done (Container Manager project) | M for an SPK wrapping the project (docker-project worker) | full: the SPK carries the compose file |
| OpenMediaVault | compose plugin | compose plugin (omv-extras guide) | listed by Emby | M (5%; a beta tester) | done | — | — |
| QNAP | App Center QPKG (official) | community QPKGs; Container Station | QPKG (official) | L–M (7% of NAS owners) | nothing | S (docs: Container Station application); M for a QPKG | full |
| UGREEN UGOS Pro | Docker (guides) | UGREEN's own Jellyfin guide; Docker project | — | L–M (3.8%, growing) | nothing | S (docs) | full |
| Asustor / TerraMaster | official packages (both) | Asustor App Central; Docker | official packages (both) | L (4.1% / 2.9%) | nothing | S (docs, if asked) | full |
| CasaOS / ZimaOS | store entry (+ NVIDIA variant) | store entry (+ NVIDIA variant) | store entry | L–M | entry built, not submitted | S (submit PR) | full |
| Umbrel | store entry | store entry (lsio image) | store entry | L | nothing | M (manifest + `app_proxy` vs host networking) | partial |
| Kubernetes / Helm | official chart (`plexinc/pms-docker/charts`) | official chart (`jellyfin/jellyfin-helm`) | community | L–M (10–23% of technical self-hosters) | nothing | M | partial |
| Native Linux packages (deb/rpm/AUR/Nix) | official deb + rpm repos | official deb repo; Arch, Alpine, Gentoo, NixOS, RPM Fusion | official deb/rpm | M (bare metal 4%; enables Proxmox LXC scripts) | none (D235: image only) | L | none |
| Windows / macOS installers | official | official | official | M for Plex's mainstream users; low for self-hosters (9% / 5%) | none | L (the node sandbox is Linux-only) | none |
| Flatpak / Snap | — | Flathub (Jellyfin says it "may be deprecated") | Flatpak, Snap | L | — | skip | — |
| Raspberry Pi | arm64 deb, Docker | arm64 deb, Docker | arm64 | M (24% run Pi OS) | arm64 image + install script | S (a Pi section in install.md) | full |
| YunoHost, Cloudron, HA OS add-ons | — / — / community | YunoHost package / — / community | — / Cloudron / — | L | — | skip | — |

## 3. Orpheum's starting point, and what it means for app stores

From `deploy/`, `docs/users/install.md`, D455, D461 and D462:

- **One image**, `ghcr.io/kstenerud/orpheum`, for amd64 and arm64 (`.github/workflows/release-image.yml`). It is on GitHub's registry only: `hub.docker.com/v2/repositories/kstenerud/orpheum` returns 404. Hardware encoding (Quick Sync, VAAPI) works on amd64 only.
- **Three services per install** (`deploy/docker/compose.yaml`):
  - `orpheum-folders`: a run-once root init that does `chown`, with `CHOWN`/`DAC_OVERRIDE` capabilities only.
  - `orpheum-core`: `network_mode: host`. It takes 8440, 8441 on loopback, and 8444 TCP+UDP itself, and it has a healthcheck.
  - `orpheum-node`: `network_mode: none`, read-only root, media mounted read-only **at the host's own path**. It depends on the core being healthy and reaches it through a socket in `run/`. `/dev/dri` and `group_add` are left commented out.
- **The file is thin** (D462 §1–2). It holds the user, the data folder, up to four media folders, two ports and a time zone. Everything else is set in the app, which suits store forms well: a form needs to ask only for folders and, optionally, a user.
- **Each route names itself to `start`** (`--compose-file omv|synology|portainer|casaos|docker`, D462 §9), and the Update page shows that route's steps. An unknown name is refused. **A new route therefore needs a new name accepted by the image, a new generated file, a frozen copy (D462 §6) and its own update steps.** That is the main ongoing cost per route, so each route added should earn its keep.
- **D461 §2 (changes additive)** protects files that third parties render: a TrueNAS or Umbrel template that passes today's flags keeps working.
- **Install script** (`docker run … install-script | sh`): one command on any Linux host with Docker. This is the natural route for Proxmox, Pi and plain-VM users.

**Where the two-part layout bites:**

- *Unraid XML templates* (one `<Repository>`, one container) and the *Proxmox helper scripts* ("We do NOT use Docker for our installation scripts") cannot express it.
- *Umbrel* routes each app's web UI through its `app_proxy` on a bridge network. Orpheum serves its own HTTPS and WebRTC on host networking. Whether Umbrel accepts a host-network app and passes UDP needs checking (**unverified**).
- *Single-container stores* would need Orpheum's old all-roles supervisor. D233 §2 and D235 §4 ended it (`cmd/orpheum/doc.go`, D43/D44), so bringing it back is an owner decision (§7).

## 4. Which routes matter: the evidence

### 4.1 Surveys (technical self-hosters; biased toward them)

**selfh.st 2025 Self-Host User Survey**: 4,081 responses, published 2025-11-21; raw JSON at `github.com/selfhst/cdn/.../2025-results.json`, cached as `selfhst-2025-results.json`. 81% of respondents work in technology, so this measures hobbyists, not households.

- **Operating systems (multi-select):** Linux 81%, Proxmox 45%, Home Assistant OS 29%, Raspberry Pi OS 24%, TrueNAS 19%, Unraid 10%, Synology DSM 10%, Windows desktop 9%, Windows Server 6%, BSD 6%, OpenMediaVault 5%, macOS 5%.
- **Containers:**
  - 96% deploy containers; 69% prefer containers to bare metal (4% bare metal).
  - Platforms: Docker 92%, LXC 32%, Podman 11%.
  - Orchestration: 23% use it.
  - Management tools: Portainer is used by 1,336 people, 66% of those who use one and 33% of all respondents. Dockge 425, Komodo 308.
- **NAS:** 68% deploy one.
- **Media servers:** Jellyfin 61%, Plex 30%, Emby 4%, Kodi 6%.

**deployn.de Self-Hosted Survey 2025** (about 850 respondents):

- **Primary host OS:** Proxmox 30.1%, Debian 29.3%, Ubuntu 26.2%, TrueNAS SCALE 8.2%, Unraid 6.2%. OMV is 4.8% (multi-select).
- **Deployment:** Docker Compose 87.6%, Docker 73.6%, LXC 30.1%, Kubernetes 9.6%, Podman 7.3%. Portainer 40.8%.
- **NAS brand:** custom-built 50.4%, Synology 33.1%, QNAP 7.2%, ASUSTOR 4.1%, UGREEN 3.8%, TerraMaster 2.9%, WD 1.2%.
- **Single-board computers:** used by 40.6% (falling from 48% in 2023). 94% of those are Raspberry Pi; Pi 4 65%, Pi 5 30%.

### 4.2 Docker Hub pulls (queried 2026-11-11; cached `hub_*.json`)

| Image | Pulls | Last push |
|---|---|---|
| `plexinc/pms-docker` | 925.7 M | 2026-11-05 |
| `linuxserver/plex` | 910.5 M | 2026-11-09 |
| `jellyfin/jellyfin` | 463.0 M | 2026-11-09 |
| `emby/embyserver` | 265.8 M | 2026-10-31 |
| `linuxserver/jellyfin` | 165.2 M | 2026-11-10 |
| `linuxserver/emby` | 46.1 M | 2026-11-06 |

These count Docker Hub only; pulls through `lscr.io` or ghcr are excluded, so linuxserver is understated. Pull counts include automated re-pulls (Watchtower and similar), so they show relative scale, not users. What they do show:

- For Plex, the community image is as popular as the official one.
- For Jellyfin and Emby, the official image leads.
- An official, well-documented image is enough. Nobody needed a linuxserver.io build to succeed.

### 4.3 What the vendors themselves offer (evidence of where they see users)

- **Plex** (`plex.tv/api/downloads/5.json`, cached):
  - Computers: Windows, macOS, Linux (Debian/RedHat; x86, x86_64, aarch64, armv7), FreeBSD.
  - NAS: Synology in three variants (DSM 6, DSM 7, DSM 7.2.2+), Netgear, QNAP, Unraid, ASUSTOR, TerraMaster (TOS 4 and TOS 5/6), Thecus, Seagate, WD OS3 and OS5.
  - Plex maintains a native package for every NAS vendor. That is the most expensive possible strategy, and it fits a commercial company. Plex's own forum threads asking for its platform split go unanswered: **no public data**.
- **Emby** (`emby.media/download.html`): 4 computer platforms; 15 NAS entries (ASUSTOR, FreeNAS, Netgear, OMV, QNAP, Synology DSM 6/7/7.2+, TerraMaster, Thecus, TrueNAS, TrueNAS SCALE, Unraid, WD ×2); plus Docker, Flatpak, Snap, Cloudron and NVIDIA Shield.
- **Jellyfin** (`jellyfin.org/docs/general/installation/`):
  - Official: Windows, macOS, Debian/Ubuntu repository, container (Docker/Podman), Synology (via Container Manager and the official image), TrueNAS (an app Jellyfin does not maintain), and a portable tarball.
  - Community: Arch, Alpine, Gentoo, NixOS, RPM Fusion ("official RPMs stopped at 10.9"), Flathub (which Jellyfin says "may be deprecated at any time… recommends container images instead"), FreeBSD (unsupported).
  - **Not mentioned at all: QNAP and Unraid.** Jellyfin, the closest analogue to us (open source, volunteers), cut native packaging back to Debian/Ubuntu and points everything else at the image.

### 4.4 NAS market

No trustworthy published share figures turned up. Market-research reports disagree by 2× on the market's size alone. Two points from the surveys:

- The deployn split above (Synology 33% of NAS owners, QNAP 7%, the rest under 5%) is the best figure available, and it comes from technical users.
- **Guess:** among non-technical households, who are Orpheum's target, Synology's share is higher still and custom builds far lower.

UGREEN entered the NAS market in 2024 and already ties Asustor in the deployn survey.

### 4.5 Separating evidence from guesswork

- **Evidence:**
  - Docker and Compose dominate.
  - Proxmox, TrueNAS, Unraid, Synology and OMV are the named NAS/host OSes, in that order, among hobbyists.
  - Synology is the dominant bought NAS.
  - Jellyfin, the volunteer project, supports the image plus Debian packages, and lets TrueNAS maintain the TrueNAS app.
- **Guess:**
  - Plex's real user base, which is larger and less technical, runs mostly on Windows PCs and Synology/QNAP packages. No data was found either way.
  - For Orpheum's five beta households, OMV and Synology are covered, so nothing here blocks the closed beta (D455). The work below is for the open beta.

## 5. Each competitor's routes, briefly

- **Plex:** native installers for Windows/macOS/Linux/FreeBSD, and official NAS packages for about 10 vendors, including Synology Package Center and the QNAP App Center. The QNAP store copy lags; Plex's forum has "PMS 1.41.0.8930 QNAP package broken" threads. Docker: `plexinc/pms-docker`, with an official Helm chart in the same repository. Also linuxserver and hotio images; TrueNAS stable train; Unraid; CasaOS (plus an NVIDIA variant); Umbrel; Proxmox community-scripts LXC.
- **Jellyfin:** Windows installer (with tray), macOS, an official Debian/Ubuntu repository (its own `jellyfin-ffmpeg`), distro packages, and the official image on Docker Hub and ghcr. linuxserver and hotio images. Official Helm chart (`jellyfin/jellyfin-helm`, active 2026-11). TrueNAS community-train app (maintainer field: truenas). SynoCommunity SPK (native; 32-bit dropped at 10.8.9; last listed build 10.10.5 in 2025-02). Community QNAP QPKGs, which wrap Container Station and need "allow unsigned". UGREEN's own install guide (Docker project). CasaOS, Umbrel (`linuxserver/jellyfin`, pinned by digest), YunoHost (`jellyfin_ynh`), and a Proxmox community-scripts LXC that installs the native .deb.
- **Emby:** the widest official list (§4.3), including Synology/QNAP/Asustor/TerraMaster/WD packages, Docker, Flatpak, Snap and Cloudron. TrueNAS stable train. A separate `emby/embyserver_arm64v8` image (1.1 M pulls) shows an older approach to multiple architectures.

## 6. Route by route: what Orpheum would need

### 6.1 Proxmox VE (importance H; effort S for docs)

- **What it is.** 45% of selfh.st respondents run Proxmox, and 30% in the deployn survey run it as the primary OS. The community-scripts project (about 30k GitHub stars) installs Jellyfin, Plex and the rest as **native** packages in unprivileged LXCs, with GPU passthrough. Its contributing guide says "We do NOT use Docker for our installation scripts", though it ships a "Docker LXC" script.
- **For Orpheum now:** a documented route, "Proxmox: make a Debian VM or the community Docker LXC, pass the media folder and `/dev/dri` in, then run the install script". It reuses everything.
- **Risks to test once, on a real Proxmox host:**
  - Whether the node's sandbox (Landlock, seccomp, `network_mode: none`, read-only root) works in Docker nested in an unprivileged LXC. That needs `nesting=1`, and AppArmor may get in the way. **Untested.** A VM avoids the question.
  - Bind-mount UID mapping in unprivileged LXCs: host UID 1000 appears as 101000. That trips every app, and the install docs would need to say so.
- **A real helper script** needs a native Orpheum package (§6.10). Skip unless the owner takes that on.
- **Route name:** the install script already reports `script`. Nothing new.

### 6.2 Unraid (importance M–H; effort S via the plugin)

- **Template format.** Unraid's Community Applications (CA) templates are one XML file per container (`<Repository>`, `<Config>` entries, `<ExtraParams>`). They cannot express three services that depend on each other. CA's submission help mentions a coming "Compose package" type, building "an immutable Compose archive for Unraid 8". Its contract page returns 404 today, so it is **unverified and not yet usable**. Watch for it.
- **What works now:** Compose Manager, the de facto community plugin (newer ones: Compose Manager Plus, and StaXX, which needs 7.2+). It takes a compose file as is.
- **For Orpheum:**
  - A generated `deploy/unraid/compose.yaml` with Unraid's defaults: appdata `/mnt/user/appdata`, media under `/mnt/user/…`, user `99:100` (nobody:users), which is what Unraid apps use.
  - A route name `unraid`, its update steps (the plugin's Update/Pull then Up), and an install.md section modelled on the OMV one.
  - Unraid's Docker supports host networking and `/dev/dri` passthrough.
  - This is a near copy of the OMV pass.
- **A CA XML template** would need the single-container mode (§7, decision A).

### 6.3 TrueNAS SCALE / Community Edition (importance H; effort M)

- **How an app is built** (`truenas/apps`, `CONTRIBUTIONS.md` cached). Each app is `ix-dev/community/<app>/` holding `app.yaml` (metadata, `run_as_context`, min version), `questions.yaml` (the install form), `ix_values.yaml`, a Jinja `templates/docker-compose.yaml` that uses iX's library, test values, and a README. CI renders the app, deploys it and waits for healthy. All new apps go to the community train; stable (where Plex and Emby sit) is iX's own.
- **The library already has what Orpheum's file needs:**
  - multiple containers;
  - `depends.add_dependency(…, "service_completed_successfully" | "service_healthy")`;
  - a built-in **permissions container** (`tpl.deps.perms`), which replaces `orpheum-folders`;
  - `host_network`, device passthrough, and host-path or ixVolume storage;
  - a portal link.
- **The Jellyfin app is about 60 template lines.**
- **What Orpheum needs:**
  - A template for core plus node: core on host network; node with `network_mode: none`, read-only root and the tmpfs.
  - Media mounted at the host path. TrueNAS host paths are `/mnt/<pool>/…`, which keeps D449 §4's "the server shows the name this machine gives it".
  - Run as TrueNAS's `apps` user, 568:568.
  - A `--compose-file truenas` route; updates come from TrueNAS's own Update button.
- **Upkeep:**
  - The app's version bumps follow our releases.
  - The template is TrueNAS's file, not one of ours, so D461's frozen-file test does not cover it. D461 §2 (additive flags) is what keeps it working. **Proposal (owner):** keep a copy of the rendered TrueNAS compose among the frozen fixtures, so the gate also tests the flags it passes.
  - TrueNAS runs on amd64 only, so arm64 does not matter here.
- **Stopgap for users now:** TrueNAS "Install via YAML" (custom app) takes `orpheum-docker.yaml` with values filled in. Document it as an interim route.

### 6.4 Synology (importance H for households; effort M for a package)

- **Current route works** (install.md): a Container Manager project, the `# Yours` block, and DS218-class models through the community `ContainerManager_for_all_armv8` script. Container Manager officially reached some ARMv8 models (DS223, DS420j) with DSM 7.2. Its **weakest step** is step 3: turn on SSH, `ssh`, type `id`, read the uid/gid.
- **Lower-pain option:** Synology's developer guide documents a **Docker Project Worker** for third-party packages:
  - needs ContainerManager 1432+, DSM 7.2.1+;
  - in the package's `resource` file: `"docker-project": {"preload-image": …, "projects": [{"name", "path", "build_params"}]}`;
  - the worker creates and builds the compose project when the package is installed, and starts and stops it with the package;
  - the example has two services.

  So an **Orpheum SPK** can carry the same generated compose file. A DSM install wizard plus postinst script could ask for the media share in DSM's own folder picker and fill `# Yours`. DSM 7's package user and share-permission resources might then remove the uid/gid step. **Unverified:** whether host networking, `network_mode: none` and an init container are accepted by the worker (the page does not say), and whether a package's user can own the project's containers. Prototype it before promising anything.
- **Distribution, without Synology's partnership:** "Manual Install" of the .spk, or a package source URL (as SynoCommunity does). Official Package Center listing (Plex, Emby) needs Synology's agreement.
- **Cost and reuse:**
  - It reuses the image and compose file.
  - New pieces: an SPK build (INFO, resource, wizard JSON, scripts) and its own route name (`synology-package`), with "update from Package Center" steps.
  - The D461 promise holds: the package carries its compose file, so an update replaces both together, which is simpler than the pasted file.
- **Proposal (owner):** worth a prototype because it serves a beta tester's platform. Keep the pasted-project route for DSM models where the package cannot install.

### 6.5 Portainer and CasaOS/ZimaOS (importance M; effort S, already built)

- Both entries exist in `deploy/catalogue/` and need only the owner's submission:
  - Portainer: to a community list such as `Lissy93/portainer-templates` (2.9k stars, active).
  - CasaOS: a PR to `IceWhaleTech/CasaOS-AppStore`, which lists Jellyfin, Plex and Emby with NVIDIA variants.
- Portainer is the most-used container manager among survey users (33% overall).
- ZimaOS uses the CasaOS app format.
- Nothing new to build.

### 6.6 QNAP, UGREEN, Asustor, TerraMaster (importance L–M; effort S each for docs)

- All four now run compose from their own UI:
  - QNAP: Container Station "Applications".
  - UGREEN: Docker → Project → Create, per UGREEN's own Jellyfin guide.
  - Asustor: Portainer / Docker Engine from App Central.
  - TerraMaster: Docker Manager on TOS 6.
- `orpheum-docker.yaml` with values typed in should work on each. **Guess:** each needs a short install.md section (paths, how to find the uid, and the GPU's render group), not a new route.
- **Proposal (owner):** add these as `docker`-route doc sections, not new route names, unless the Update page's steps differ.
- QNAP QPKGs (Plex and Emby are official; Jellyfin's are community wrappers of Container Station, and they need "allow apps without a valid signature"): skip at 7% share.

### 6.7 Umbrel (importance L; effort M)

- **How an entry is built:** a compose file plus `umbrel-app.yml`, with folders chosen through `folderAccess` and mounted at fixed targets. Every app sits behind `app_proxy` (`APP_HOST`, `APP_PORT`). Umbrel's Jellyfin entry uses `linuxserver/jellyfin`, pinned by digest, `PUID=1000`.
- **Why it is awkward for Orpheum:**
  - Orpheum's own HTTPS, WebRTC (UDP) and LAN learning do not fit a proxy-fronted bridge app.
  - Media at fixed targets (`/media`) loses the host path, which D449 §4 wanted.
- **Recommendation:** skip until there is demand. The survey does not list umbrelOS separately.

### 6.8 Kubernetes / Helm (importance L–M; effort M)

- Both Jellyfin and Plex ship official charts.
- Orpheum would fit one Pod: an init container, core with `hostNetwork: true`, node with no network, and an `emptyDir` for `run/`. The Pod would need `hostNetwork` and a shared socket volume.
- Only 10–23% of technical self-hosters use orchestration, and they often write their own manifests.
- **Recommendation:** skip, or accept a community chart without taking on its maintenance.

### 6.9 Docker Hub mirror (cross-cutting; effort S, gate file)

- Synology's Container Manager registry tab, QNAP Container Station and Unraid's search all default to Docker Hub. Pull counts there are also the public measure of popularity (§4.2). Jellyfin publishes to both registries.
- **Proposal (owner):** also push `docker.io/<owner>/orpheum` from the release workflow, and keep ghcr as the name in our files. The workflow is a gate file (rule 2), so this is the owner's change. Docker Hub's anonymous-pull limits are a reason to keep ghcr as the primary.

### 6.10 Native packages, Windows/macOS (importance M; effort L)

- **What competitors do:** all three ship them. Jellyfin keeps only Debian/Ubuntu official and lets distros do the rest.
- **Orpheum today:** image only (D235). The node's sandbox (Landlock, seccomp) is Linux-only, so Windows and macOS mean either running without it or running in a VM.
- **What a native package would buy:**
  - **Debian package with two systemd units** (core, and node with `PrivateNetwork=`, `ProtectSystem=` and the rest; this is D44's first preference): unlocks the Proxmox helper scripts, LXC without Docker, bare-metal Pi, and `apt upgrade`.
  - **Costs:** a signed apt repository and its key rotation (packaging-history lesson 8), FFmpeg shipped beside the binary, and a second update path for the Update page.
- **Proposal (owner):** not now. Revisit if the Proxmox doc route proves painful in testing.

### 6.11 Skip

- Flatpak and Snap: Jellyfin itself steers away from Flatpak.
- YunoHost and Cloudron: tiny audiences.
- Home Assistant OS add-ons: 29% run HA OS, but for home automation; media add-ons there are community-run.
- FreeBSD / TrueNAS CORE: Jellyfin doesn't support it either.
- Windows tray app.

## 7. Ranked recommendation

**Do, in this order (least pain for most reach):**

1. **Proxmox doc route** (S). An install.md section: a Debian VM, or the community Docker LXC, then the install script. Cover the GPU and bind-mount UID notes. Test once on real Proxmox, VM and LXC; the nested-sandbox question decides which the docs recommend. No new route name.
2. **Unraid via Compose Manager** (S). A generated `deploy/unraid/compose.yaml` (`/mnt/user/appdata`, 99:100), route `unraid`, update steps, and an install.md section. The same shape as the OMV pass.
3. **Submit the existing CasaOS and Portainer entries** (S, owner's action).
4. **TrueNAS community-train app** (M). Write it against iX's library, with the permissions container in place of `orpheum-folders`; route `truenas`. Until it is accepted, document "Install via YAML" with `orpheum-docker.yaml`. Plus the frozen-fixture proposal in §6.3.
5. **Synology package prototype** (M). An SPK using the Docker Project Worker plus a folder-picker wizard, to remove the SSH/`id` step. Test it on a DS218 with the community Container Manager, and on an x86 model.
6. **Docs only, when a user asks:** QNAP Container Station, UGREEN, Asustor, TerraMaster, Raspberry Pi OS, all under the `docker` route.

**Skip for now:** Unraid CA XML templates, Umbrel, Helm, native deb/rpm, Windows/macOS, Flatpak/Snap, YunoHost, HA OS add-ons, QPKG.

**Needs an owner decision:**

- **A. Single-container mode?** An all-roles container (the old supervisor of D43/D44) would unlock Unraid CA templates and simplify every store form. It reverses D233 §2 / D235 §4 and weakens the node's isolation (its own network namespace and read-only root). **My view:** no. Most stores now take compose, and Unraid 8's compose packages may close the last gap.
- **B. A native Debian package with systemd units** (§6.10): only if the Proxmox doc route fails in testing.
- **C. A Docker Hub mirror** (§6.9): a change to the release workflow, which is a gate file.
- **D. How many route names to carry.** Each one costs a generated file, a frozen fixture and update steps (D462 §9, §6). The proposal: `unraid`, `truenas` and possibly `synology-package` as new names; the QNAP/UGREEN/Asustor/TerraMaster/Proxmox/Pi docs stay under `docker` or `script`.
- **E. Who maintains third-party catalogue entries** (TrueNAS, CasaOS, Portainer lists), and whether their rendered files join the frozen fixtures (§6.3).
- **F. Timing.** Nothing here blocks the closed beta: both testers' platforms (OMV, DS218) are covered. Should this wait for the open beta?

## 8. Sources

All fetched 2026-11-11 unless dated otherwise.

- selfh.st 2025 survey results, published 2025-11-21: https://selfh.st/survey/2025-results/ ; data: https://github.com/selfhst/cdn/blob/main/assets/surveys/annual/2025-results.json
- deployn Self-Hosted Survey 2025: https://selfhosted-survey-2025.deployn.de
- Docker Hub repository API: `https://hub.docker.com/v2/repositories/<repo>/` for plexinc/pms-docker, linuxserver/plex, jellyfin/jellyfin, emby/embyserver, linuxserver/jellyfin, linuxserver/emby, emby/embyserver_arm64v8, kstenerud/orpheum (404)
- Plex downloads: https://plex.tv/api/downloads/5.json
- Emby downloads: https://emby.media/download.html
- Jellyfin installation: https://jellyfin.org/docs/general/installation/ ; container: https://jellyfin.org/docs/general/installation/container ; Synology: https://jellyfin.org/docs/general/installation/advanced/synology ; community: https://jellyfin.org/docs/general/installation/advanced/community
- TrueNAS apps: https://github.com/truenas/apps (CONTRIBUTIONS.md; `ix-dev/community/jellyfin/`; `ix-dev/stable/` lists plex and emby); https://apps.truenas.com/catalog/
- Unraid CA submission help: https://ca.unraid.net/submit/help ; https://ca.unraid.net/submit/help/xml-field-reference ; compose plugins: https://forums.unraid.net/topic/200462-staxx-new-docker-compose-management-plugin/ , https://forums.unraid.net/topic/197334-plugin-compose-manager-plus
- Proxmox community-scripts: https://github.com/community-scripts/ProxmoxVE (`install/jellyfin-install.sh`, `ct/jellyfin.sh`, CONTRIBUTING.md); contribution guide quoting "We do NOT use Docker": https://git.community-scripts.org/community-scripts/ProxmoxVE/raw/tag/2026-02-21/docs/contribution/AI.md
- Synology Docker Project Worker: https://help.synology.com/developer-guide/resource_acquisition/docker-project.html ; Container Manager release notes (ARMv8 DS223/DS420j): https://www.synology.com/en-uk/releaseNote/ContainerManager ; https://github.com/007revad/ContainerManager_for_all_armv8 ; SynoCommunity Jellyfin: https://synocommunity.com/package/jellyfin , 32-bit drop: https://github.com/SynoCommunity/spksrc/issues/5432
- QNAP: https://github.com/kajain99/Jellyfin-QPKG , https://github.com/pdulvp/jellyfin-qnap , https://forums.plex.tv/t/pms-1-41-0-8930-qnap-package-broken/886526
- UGREEN: https://nas.ugreen.com/blogs/how-to/install-jellyfin-setup-step-by-step , https://nas.ugreen.com/blogs/knowledge/docker-docker-compose-ugreen-nas
- Umbrel: https://github.com/getumbrel/umbrel-apps (`jellyfin/docker-compose.yml`, `jellyfin/umbrel-app.yml`)
- CasaOS store: https://github.com/IceWhaleTech/CasaOS-AppStore (Apps/)
- Helm: https://github.com/jellyfin/jellyfin-helm ; https://github.com/plexinc/pms-docker (charts/plex-media-server)
- Portainer community templates: https://github.com/Lissy93/portainer-templates
- YunoHost: https://github.com/YunoHost-Apps/jellyfin_ynh
- linuxserver.io armhf deprecation: https://www.linuxserver.io/blog/end-of-an-arch , https://info.linuxserver.io/issues/2023-07-01-armhf
- Docker Desktop host networking (4.34, 2024-09): https://www.docker.com/blog/docker-desktop-4-34/
- NAS market (no reliable figures): https://jlw-newsletter.pages.dev/jlw-2026-02-02-home-nas.html (unverified split, not relied on)
- Internal: docs/users/install.md; deploy/{docker,omv,synology,portainer,catalogue}/; .github/workflows/release-image.yml; D455, D461, D462, D43/D44 in docs/contributors/decisions/working-notes.md; docs/contributors/design/research/{deployment-audit-2026-11-08,packaging-history}.md; cmd/orpheum/doc.go.