# Security week, 2026-10-23 to 2026-10-30: what it teaches Orpheum

- **Date:** 2026-10-30. A web survey of the week's incidents, read against Orpheum's repositories
  (orpheum `main` at 10f7ba01, orpheum-connect, orpheum-ops and ffmpeg-fdproxy beside it).
- **Method:** about 45 web searches, with primary sources read where they could be reached. Notes
  and fetch summaries are cached in `/yoloai/cache/security-week/notes.md`. Nothing in any
  repository was changed.
- **Already done today, not repeated here:** `research/security-pins-2026-10-30.md` (the pin audit:
  pion/stun, Vite, Node 22.23.4, the Debian digest and FFmpeg 8.1.3 are done or queued),
  `research/github-integrity-2026-10-30.md` (signed tags, signed image digests, a second remote,
  immutable releases, account hardening), and `godebug multipathtcp=0` in both go.mods (an earlier commit,
  `TestSockets_UseNoMultipathTCP`). The plan is the "Security (the owner, 2026-10-30)" section of
  `design/plans/navigation-and-widgets-answers-2026-10-27.md`. This report adds only what the week
  shows beyond those.
- **Confirmation key:** **[P]** means read in a primary source (vendor, project, CVE or tracker
  page, or the researcher's own post). **[S]** means secondary reporting only, not confirmed from a
  primary source.

## 1. The incidents

### 1.1 Exploited vulnerabilities and kernel

**CVE-2026-98432: Linux IPv4 TCP use-after-free, remote root on MPTCP listeners.**
- **Dates:** published 2026-10-15. Anthropic's exploit write-up came out on 2026-10-27, and the
  Go team's guidance on 2026-10-30.
- **What happened:** a retransmitted SYN can build a SYN-ACK from IP options (`ireq_opt`) that
  another CPU has already freed (CVSS 9.8). Anthropic described a working remote root exploit
  against servers with an MPTCP listener. It took 48 minutes to leak a kernel pointer and 5 more to
  get a shell.
- **Why Go programs are exposed:** since Go 1.24, Go programs listen with MPTCP by default on
  Linux. The Go team's mitigations are `GODEBUG=multipathtcp=0`, a `godebug` line in go.mod, or
  `SetMultipathTCP(false)`. The team calls per-program mitigation "fragile": one MPTCP listener
  left anywhere still exposes the host. A system-wide `sysctl net.mptcp.enabled=0` covers new
  sockets only, so services must restart after it.
- **Patch status:** Debian fixed it on 2026-10-16 (DSA 6568-1). On 2026-10-30, Ubuntu 22.04, 24.04
  and 26.04 were still "work in progress", and RHEL was "affected".
- **Root cause:** a race in the kernel's TCP connection setup. The exploit's way in is MPTCP, which
  Go enables by default.
- **Sources:** [P] [Ubuntu CVE page](https://ubuntu.com/security/CVE-2026-98432);
  [P] [oss-security / golang-announce](https://www.openwall.com/lists/oss-security/2026/10/30/1);
  [P] [the follow-up on the system-wide sysctl](https://www.openwall.com/lists/oss-security/2026/10/30/6);
  [P] [Anthropic research](https://www.anthropic.com/research/discovering-vulnerabilities-with-claude).
  The claim that dropping IPv4 TCP packets with IP options also blocks it is [S]
  ([mitigation scripts](https://github.com/typhoon1217/cve-2026-98432-mitigation)).

**Plex2Shell: four chained Plex Media Server CVEs (CVE-2026-96652, -96654, -96655, -96656).**
- **Dates:** fixed in 1.43.3.10861 on 2026-08-03. Exploited in October, and added to CISA's KEV
  list on 2026-10-21, just before the window.
- **The chain:**
  1. Unauthenticated SSRF through `/player/timeline`, which checks that an `X-Plex-Token` header is
     present, not that it is valid.
  2. A loopback-trust bypass, plus an injection in `searchOne`.
  3. An owner-authenticated transcoder SSRF with an arbitrary-path file write (`dump-yuv`).
  4. Code execution by `dlopen` of an unsigned codec from the Codecs directory.
- **What attackers did:** installed miners and residential-proxy backdoors on servers that had
  missed the August update.
- **Root cause:** a token was checked for presence, not validity. Loopback requests were trusted.
  The transcoder took a path from the request. Libraries were loaded from a writable directory.
- **Sources:** [P] [researcher's write-up](https://github.com/code-zm/plex2shell);
  [S] [KEV addition (The Hacker News)](https://thehackernews.com/2026/10/cisa-orders-federal-agencies-to-patch.html);
  [S] [Field Effect on exploitation](https://fieldeffect.com/blog/active-exploitation-plex2shell-vulnerabilities).

**Synology C3RB3R/L0CK3D ransomware.**
- **Dates:** Synology warned on 2026-10-29. The flaws used, CVE-2026-13639 and -13684, were fixed
  in SA-26:13 on 2026-09-18. CVE-2024-10441 (2024) was also exploited.
- **What happened:** NAS devices that skipped the September update were encrypted.
- **Root cause:** internet-exposed DSM that was not updated.
- **Sources:** [S] [NAS Compares](https://nascompares.com/2026/10/30/synology-lock3d-c3rb3r-ransomware-attack-everything-you-need-to-know/).

**PHP2Shell (CVE-2026-109618, -109619).**
- **Dates:** BSI warned on 2026-10-21. Fixes shipped on 2026-10-22 (8.5.12, 8.4.27, 8.3.36,
  8.2.35). In KEV from 2026-10-26.
- **What happened:** pre-auth RCE that leaked before the patch. watchTowr saw exploitation
  attempts. QNAP had no patch date. Hetzner ran a 2-day incident for its PHP hosting.
- **Root cause:** memory-safety bugs in the Zend Engine.
- **Sources:** [S] [Notebookcheck](https://www.notebookcheck.net/PHP2Shell-What-QNAP-Nextcloud-and-OPNsense-users-should-check.1431047.0.html).
  PHP's advisories (GHSA-w8hw-w9fx-j79w, GHSA-6wmf-4rv5-p3rp) were cited there but not read.

**GitLab CVE-2026-97229 at Alpine Linux.**
- **Dates:** attack attempts on 2026-10-20 and 2026-10-21. Added to KEV on 2026-10-29.
- **What happened:** an integer overflow in regex compilation from CI configuration (CVSS 10). Alpine
  found that upgrading GitLab did not fix it, because the bug is in Ruby's Onigmo and Alpine's
  image ran its own Ruby. Its signing keys had never been on the GitLab server, so nothing was
  signed by the attacker.
- **Root cause:** the fix was in a bundled runtime that the image replaced with its own.
- **Sources:** [P] (search summary of the page, not read in full)
  [Alpine's post](https://alpinelinux.org/posts/GitLab-security-incident-CVE-2026-97229.html);
  [S] [The Hacker News on the KEV addition](https://thehackernews.com/2026/10/cisa-adds-actively-exploited-gitlab-rce.html).

**CISA KEV additions on 2026-10-28.**
- CVE-2026-46300 (Linux kernel, missing initialisation) and CVE-2026-41709 (VMware ESX).
- **Source:** [P] [CISA alert](https://www.cisa.gov/news-events/alerts/2026/10/28/cisa-adds-two-known-exploited-vulnerabilities-catalog).

**Proxmox VE advisories PSA-2026-00075-1 to -00079-1.**
- **Date:** 2026-10-30.
- **What they cover:** the worst is a termproxy parameter that is not shell-quoted, so a crafted
  URL opened by a logged-in `root@pam` runs commands as root (fixed in pve-manager ≥ 9.2.24). The
  others are xterm.js link POSTs, a Pool.Audit ACL gap, network-config injection and a MOTD XSS.
- **Source:** [S] (forum and blog summaries; the PSAs themselves were not read)
  [Proxmox forum](https://forum.proxmox.com/threads/cve-2026-74580-v-ghost-%E2%80%93-affected-proxmox-ve-versions.187037/),
  [summary](https://zaidan.web.id/2026/10/30/temuan-celah-baru-di-proxmox-dari-xss-sampai-eskalasi-root/).

### 1.2 Supply chain

**LiteLLM 1.107.0 on PyPI, Docker Hub and GHCR.**
- **Date:** 2026-10-21.
- **What happened:** seven wheels went to PyPI and stayed live about 2 hours; the `v1.107.0`,
  `latest`, `main-latest` and `main-stable` image tags were live about 2.5 hours. The stealer ran
  at every Python start-up and took cloud, LLM, SSH and GPG keys, cookies and shell history.
- **Root cause chain:**
  1. A master key was committed to a public repository in May and never revoked [S, The Register].
  2. On 2026-10-10 that key opened an internal, publicly exposed LiteLLM deployment.
  3. That deployment's logs held developers' GitHub CLI OAuth tokens. LiteLLM writes: "we have
     observed specific situations in which Claude Code will embed a user's GitHub CLI OAuth token
     into a response body, and in situations where response bodies are logged, these tokens may
     appear in LiteLLM logs" [P].
  4. An org-owner token overrode branch protection. The modified workflow then built from a
     look-alike repository (`Berri-AI/litellm`) and published.
- **Sources:** [P] [LiteLLM incident post](https://docs.litellm.ai/blog/security-incident-oct-21-2026);
  [S] [The Register](https://www.theregister.com/security/2026/10/26/litellm-supply-chain-attack-started-with-an-exposed-master-key/5305544).
  No statement from Anthropic was found.

**OVHcloud CLI.**
- **Date:** 2026-10-19, 12:47–13:12 UTC.
- **What happened:** the binaries of five releases already published (v0.11.0–v0.15.0) **and
  their `checksums.txt`** were replaced with a secret stealer. They were live 94 minutes and
  downloaded about 250 times.
- **Root cause:** a contributor's GitHub account, compromised since 2025-10. Checksums that sit
  beside the artifacts verified the malware.
- **Sources:** [S] [StepSecurity](https://www.stepsecurity.io/blog/ovhcloud-cli-github-release-swap-glassworm),
  [IT-Connect](https://www.it-connect.tech/ovhcloud-cli-binaires-on-github-were-tampered-with-by-an-infostealer/).

**Gheist (Wiz).**
- **Date:** disclosed 2026-10-25.
- **What happened:** stolen GitHub tokens, from infostealer logs and earlier supply-chain thefts,
  planted `ci-sync-*` branches carrying `on: push` workflows. The workflows printed every secret as
  base32, which GitHub's log masking misses, downloaded the logs and deleted the runs. More than
  850 accounts and 13,500 branches were affected.
- **Why private repositories were exposed:** GitHub's hold on suspicious runs (from 2026-07-28)
  covers **public repositories only**, not private ones or GHES.
- **Wiz's advice:** required reviewers on environments with secrets; OIDC scoped to protected refs;
  short-lived fine-grained tokens or Apps in place of PATs.
- **Sources:** [P] [Wiz](https://www.wiz.io/blog/gheist-attempted-scaled-abuse-of-stolen-github-tokens-stumbles);
  [S] [Help Net Security](https://www.helpnetsecurity.com/2026/10/27/gheist-github-token-abuse/).

**A wave of malicious npm and PyPI packages.**
- **Dates:** 106 packages (96 npm, 10 PyPI) in GitHub advisories between 2026-10-26 and -28.
- **What they did:** install-time execution through postinstall or `setup.py`; dependency
  confusion against internal scopes; miners; a PyPI stealer named `claudeopusfile`; a package that
  republished itself under the installer's npm identity; a loader aimed at AI-assistant
  configuration files.
- **Root cause:** install scripts run by default, and highest-version resolution.
- **Sources:** [S] [Safeguard's summary](https://safeguard.sh/resources/blog/106-malicious-npm-and-pypi-packages-october-2026-what-they-did-and-how-to-block-the-next-batch);
  primary: the [GitHub malware advisories, Oct 26–28](https://github.com/advisories?query=type%3Amalware+published%3A2026-10-26..2026-10-28)
  (not read one by one).

**Trojanized "Claude Code" extension 2.1.313 in Cursor's extension marketplace.**
- **Date:** 2026-10-29, about 09:27–20:35 UTC.
- **What it did:** added an attacker key (`svc-telemetry`) to `~/.ssh/authorized_keys`, kept an
  `autossh` reverse tunnel open, and collected the names of environment variables matching
  KEY, TOKEN, SECRET or PASS.
- **Root cause:** how it reached the marketplace was not disclosed.
- **Sources:** [S] [Rescana](https://www.rescana.com/post/claude-code-extension-2-1-313-cursor-marketplace-trojanized-ssh-backdoor);
  Cursor's own notice was not read.

**Background from before the window, still relevant.**
- **Node.js security releases, 2026-10-21:** CVE-2026-111497 (Brotli out-of-bounds write → RCE),
  TLS hostname bypasses through non-canonical IPv4 forms, and removal of the eMudhra CA. Fixed in
  22.23.4, which Orpheum now pins. [P]
  [Node.js](https://nodejs.org/en/blog/vulnerability/october-2026-security-releases)
- **Go 1.26.10, 2026-10-19/20:** crypto/x509 distrusts the eMudhra intermediates. Orpheum pins it.
  [P] [oss-security](https://www.openwall.com/lists/oss-security/2026/10/20/29)
- **npm, 2026-10-02:** unvalidated trusted-publishing configurations now expire, and
  `issue_comment` events are rejected. [P]
  [GitHub changelog](https://github.blog/changelog/2026-10-02-unvalidated-npm-trusted-publishing-configurations-now-expire/)

### 1.3 Attacks on AI coding agents

**AGENTS.md dropped into `~/Downloads` (Forever Security).**
- **Date:** reported 2026-10-25 to 2026-10-27.
- **What happened:** a web page silently downloads an `AGENTS.md` or `CLAUDE.md` into Downloads.
  An agent started in that folder loads it as project instructions. It then rewrites its own
  configuration, installs npm packages, and steals OAuth tokens. In lab reproductions it worked
  about 90% of the time on Codex and about 50% on Claude Code. It was seen in the wild.
- **Root cause:** agents trust instruction files in their working directory, and the browser can
  write that directory.
- **Sources:** [S] [BleepingComputer](https://www.bleepingcomputer.com/news/security/ai-coding-agents-hijacked-using-agentsmd-file-in-downloads-folder/),
  [Dark Reading](https://www.darkreading.com/application-security/in-the-wild-attack-ai-agents-trust-markdown).
  The researcher's own post (forever.security) was not read.

**Background from September.**
- **GitSpawn (Manifold, 2026-09-02):** `core.fsmonitor` in a repository's `.git/config` runs code
  the moment an agent calls `git status`. Fixed in Claude Code 2.1.196. The interim mitigation is
  `git config --global core.fsmonitor false`. [S]
  [summary](https://vortx.ch/gitspawn-malicious-git-configs-hijack-ai-coding-agents/)
- **Plugin4Shell (Air, 2026-09-18):** a branch named like the pinned SHA shadows the commit on
  `git checkout <sha>`. Fixed in Claude Code 2.1.179. [P]
  [Air Security](https://www.air.security/blog-posts/plugin4shell)

### 1.4 Browsers, WebRTC and TURN

- **Firefox 159 (MFSA 2026-118), 2026-10-27:** 126 CVEs, including use-after-frees in WebRTC
  signalling and networking (CVE-2026-118005, -118006, -117892) and a Service Workers
  site-isolation flaw (CVE-2026-117959). None exploited. [P]
  [Mozilla advisory](https://www.mozilla.org/en-US/security/advisories/mfsa2026-118/)
- **Chrome 156.0.8078.73, 2026-10-27:** 52 security fixes, 4 critical. All four critical bugs were
  reported by one researcher, two of them crediting Anthropic and Claude. [S]
  [Malwarebytes](https://www.malwarebytes.com/blog/bugs/2026/10/update-chrome-google-fixes-37-security-flaws-including-seven-critical-bugs);
  Google's release blog was not read.
- **Coturn:** CVE-2026-104074 (2026-10-07: an unauthenticated Allocate leaks stack memory) and
  CVE-2026-73215 (authenticated users drain the relay port pool). These are reminders of the
  TURN-as-proxy and allocation-exhaustion classes. [S]
  [SentinelOne](https://www.sentinelone.com/vulnerability-database/cve-2026-73215/)

### 1.5 Libraries Orpheum links or calls

- **SQLite CVE-2026-119373 to -119379 and -119591.** Two FTS5 heap overflows
  (`fts5MergeRowidLists`, `fts5DoSecureDelete`), WAL information disclosure, R-Tree
  use-after-free, changeset overflow, and stack exhaustion. Reported "fixed in 3.54.0" by VulDB on
  2026-10-30. **Not confirmed:** SQLite's 3.54.0 release log lists no security fixes, and no CVE or
  OSV entry was read. modernc.org/sqlite v1.59.0 carries SQLite **3.53.4**. [S]
  [VulDB](https://vuldb.com/vuln/429019)
- **FFmpeg's 2026-10-08 batch.** CVE-2026-107676 leaves up to 3 bytes uninitialised in HDR10+
  T.35 side data, which reach the output. CVE-2026-107678 is a stack exhaustion from MP4 files
  with tens of thousands of tiny `pssh` boxes. Both are reported against FFmpeg through 9.0.2, with
  upstream fix commits but no release. **Not confirmed:** neither is on
  [ffmpeg.org/security](https://ffmpeg.org/security.html). [S]
  [ffmpeg-micro](https://www.ffmpeg-micro.com/blog/ffmpeg-cve-october-2026-no-patch-exists-contain-them-instead)

### 1.6 Breaches, cloud and Cloudflare

- **So-net (Sony), disclosed 2026-10-27:** 2.58 million customer records and 54,401 email
  passwords. [S] [Help Net Security](https://www.helpnetsecurity.com/2026/10/28/sony-network-communications-data-breach-so-net/)
- **DHS staff directory leak, 2026-10-28:** 41,000 records. [S]
- **Railway, 2026-10-21:** environment-variable store accessed, some values decrypted. [S]
  [tracker](https://agentbeam.com/blog/october-2026-software-supply-chain-incidents-tracker)
- **Cloudflare, 2026-10-20 to -27:** five operational incidents (Zero Trust and the dashboard,
  regional congestion, Workflows, AI Security for Apps). None was a security incident. [S]
  [host-tracker](https://www.host-tracker.com/en/is-cloudflare-down/incidents/2026-10-27-1631)
- **Hetzner:** a network outage on 2026-10-20, DHCP degradation in FSN1 and NBG1 on 2026-10-28, and
  a notice on the EDK II UEFI flaw CVE-2026-97631. No security incident. [S]
  [status tracker](https://pulsetic.com/status/hetzner/)
- **Docker Hub "GhostPat" (CVE-2026-105445):** personal access tokens survived account conversions,
  from 2024-08 until it was fixed on 2026-09-30. Docker found no exploitation. [S]
  [BleepingComputer](https://www.bleepingcomputer.com/news/security/docker-hub-flaw-let-attackers-impersonate-users-tamper-with-images/)
- **AI-run intrusions:**
  - CrowdStrike OverWatch reports three times as many agentic intrusions in the second half of
    2026 as in the first.
  - "CyberStrikeAI" ran 44 agents against exposed VPNs and WebLogic servers.
  - The "Cairn" panels showed more than 1,200 websites attacked autonomously.
  - Source for all three: [S]
    [The Hacker News](https://thehackernews.com/2026/10/cyberstrikeai-uses-44-ai-agents-to.html),
    [SecurityBrief](https://securitybrief.asia/story/crowdstrike-finds-ai-agents-running-autonomous-cyberattacks).

## 2. Lessons for Orpheum, ranked by risk × cost

Proposals only. Each one touching a gate (`.github/`, `scripts/`, `.claude/`, configuration), a
host or a design is the owner's decision. None assumes a new dependency.

### Cheap and worth doing now

**1. Close CVE-2026-98432 at the network namespace, not just in our binaries.**
- **Risk:** remote root on an unpatched Ubuntu kernel.
- **Covered:**
  - Both go.mods carry `godebug multipathtcp=0`, tested by `TestSockets_UseNoMultipathTCP`.
  - The plan already notes that Docker's embedded DNS listens with MPTCP and that the host kernels
    are the owner's to update.
- **Gaps:**
  - Ubuntu 24.04, which prod-api and the edge run (`orpheum-ops/production/README.md`), has no
    fixed kernel yet.
  - The Go team calls per-program mitigation fragile. docker-proxy and cloudflared are Go programs
    we did not build.
  - `net.mptcp.enabled` is set per network namespace (my understanding of the kernel; verify), so
    the host's value does not reach the containers.
  - Households will run our compose files on their own VPS kernels.
- **Proposal:**
  1. Run `sysctl -w net.mptcp.enabled=0`, persisted in `/etc/sysctl.d/`, on prod-api, the edge and
     the production NAS Proxmox host. Then restart Docker and each service.
  2. Add `sysctls: {net.mptcp.enabled: "0"}` to every service in
     `internal/deployment/templates/compose*.yaml` and connect's compose files. That carries the fix
     to households, whose kernels we do not control.
  3. Verify with `ss -Mltn` (lists MPTCP listeners) on each host and inside each container. A
     deployment test can assert the sysctl is in the rendered compose.
  4. Optionally, add an nftables rule in `orpheum-ops/edge/edge-filter.nft` that drops IPv4 TCP
     carrying IP options (no legitimate client sends them).
  5. Reboot into the fixed kernel the day Ubuntu and Proxmox ship it.
- **Cost:** about an hour.

**2. Keep secrets out of what agents print, and out of their reach.**
- **Risk:** the LiteLLM chain began with a token that an agent session put into a logged body. The
  AGENTS.md attack and the trojanized extension both aim at an agent machine's tokens and keys.
- **Covered:**
  - Sandboxes, with no `gh` login in this one (`~/.config/gh` is absent).
  - Pre-edit and pre-bash hooks guard gates and dependencies.
  - D194 (`secrets.Value`; only providers `Reveal()`).
  - D296: staging's DNS token is confined to orpheum-test.com.
  - Every agent starts in the repository, never in Downloads.
- **Gaps:**
  - This sandbox holds SSH keys to production and to a test machine. Any session, or a prompt
    injection from a fetched web page, can read them.
  - Whatever a command prints lands in the model request and in the session transcripts under
    `~/.claude/projects`.
  - Nothing refuses `cat ~/.ssh/*`, `printenv`, or `gh auth token`.
- **Proposal:**
  - **Server side:** restrict those keys to the sandbox's own address (and
    `restrict` where a forced command suffices) in `authorized_keys`. A stolen key then does not
    work from elsewhere.
  - **Sandbox image:** set `git config --global core.fsmonitor false` (GitSpawn's interim fix).
  - **pre_bash.py (owner gate change):** refuse commands that print private keys or tokens.
  - **Agent tooling:** pin the Claude Code version in the sandbox image and update it on purpose,
    as any other dependency.
  - **Research input is untrusted:** treat `/yoloai/cache` clones and `/yoloai/files` drops as
    data, and never start an agent with its working directory there.
- **Cost:** about an hour, mostly the owner's.

**3. Pin the household's image by digest at install, before signing exists.**
- **Risk:** LiteLLM's attackers re-pushed `latest` and the version tag. Gheist shows how a stolen
  token gets push rights. OVHcloud shows that checksums published beside the artifact prove
  nothing.
- **Covered:**
  - Actions are pinned by SHA with `persist-credentials: false`.
  - `fetch_ffmpeg.py` pins sha256 values held in our own repository, which is exactly what
    OVHcloud's users lacked.
  - github-integrity recommendations 3 and 4 (signed tags, a signed digest, a bootstrap install
    that pulls by digest) are in the plan before the public release.
- **Gap until then:**
  - `cmd/orpheum/deployment.go` `installImage` returns `ghcr.io/kstenerud/orpheum:<tag>` (or
    `:latest`).
  - `install.sh` does `docker pull "$image"`.
  - `ORPHEUM_IMAGE=` in `.env` keeps the tag. Any later pull or recreate follows whatever the tag
    points to by then.
- **Proposal:**
  - **Install script:** resolve the tag to its digest once
    (`docker image inspect --format '{{index .RepoDigests 0}}'`) and write `repo@sha256:…` into
    `.env`. From then on a household never silently moves to a re-pushed tag. No new tool needed.
  - **`release-image.yml`:** add `environment: release` with the owner as required reviewer, and a
    ruleset that lets only the owner create `v*` tags (Wiz's first recommendation). The repository
    is private, so GitHub's suspicious-run hold does not apply to it.
- **Cost:** half a day.

**4. Stop install scripts running in the web build.**
- **Risk:** this week's 96 malicious npm packages acted almost entirely through `postinstall`.
- **Covered:** `deps/npm.allow` gates direct dependencies; `save-exact=true`; `npm ci` from the
  lockfile.
- **Found:**
  - Only two packages in `web/package-lock.json` have install scripts: `fsevents` (dev, macOS
    only) and **`lfa-ponyfill`**, which `jassub` pulls into the client.
  - lfa-ponyfill's `postinstall` fetches `https://fonts.google.com/metadata/fonts` and overwrites
    its `fonts.json`, which its `index.js` imports.
  - So the shipped client can contain bytes fetched at install time. The lockfile's integrity hash
    does not cover them, and the build is not hermetic. That is harmless today, but it is exactly
    the shape D220 wants to rule out.
- **Proposal:**
  - Add `ignore-scripts=true` to `web/.npmrc`. lfa-ponyfill then keeps the `fonts.json` from its
    tarball.
  - Confirm the Mac dev loop still works without fsevents' script.
  - Add a check that fails when the lockfile gains a `hasInstallScript` package not on a short
    list.
  - npm 12 makes this the default, but we are on npm 10.9.9.
- **Cost:** under an hour.

**5. Take Plex2Shell's four mistakes as audit items for the core and node.**
- **Risk:** this is the nearest analogue to Orpheum: a self-hosted media server with a transcoder,
  exposed to the internet.
- **Covered:**
  - The TURN relay admits public peers only (`internal/core/turn/relay.go` `mayRelayTo`).
  - OpenSubtitles download links are host-checked (`opensubtitles/requests.go` `fetchLink`).
  - Admin "from home" is an extra condition on a signed-in admin, not a credential
    (`internal/core/api/home_network.go`).
  - FFmpeg runs under Landlock and seccomp with a protocol whitelist (`internal/node/confine`,
    `execx`). Written trees carry no EXECUTE right.
- **Gaps:**
  - No `TestArch_` claim pins "home and this-machine's port never stand in for a session".
    `SetThisMachinePort` trusts anything arriving on a loopback-published port, and Plex's step 2
    was exactly such a trust.
  - Landlock's EXECUTE right governs `execve`, not a `dlopen`'s `mmap`. Plex's step 4 was a library
    loaded from a writable directory.
- **Proposal:**
  - Add `TestArch_HomeIsNeverACredential`.
  - Add a test that FFmpeg's environment (`LD_LIBRARY_PATH`, `LIBVA_DRIVERS_PATH`) and its output
    paths are only ever set by the node, never from a request.
  - Consider `noexec` on the node's writable mounts.
  - Check that every token check validates the token rather than testing that it is present (an
    archtest over the auth middleware, if one does not already exist).
- **Cost:** half a day.

**6. Owner housekeeping from this week.**
- Update the production NAS's Proxmox host: pve-manager ≥ 9.2.24 for the termproxy PSA, and the kernel.
- Don't open links while logged in to the Proxmox UI as `root@pam`.
- Rotate any credential ever pasted into an agent session.

### For public-beta prep

**7. Make "households run the current version" a security control.**
- **Risk:** Plex2Shell, Synology and PHP2Shell were each patched weeks or months before the
  break-ins, which landed on servers that had not updated. That is the dominant way home servers
  fall.
- **Covered:** the plan's "security release path" (fix branch → narrow gate → deploy in hours).
- **Gap:** that path reaches the production NAS and prod-api only. There is no update notice, no
  minimum-version policy and no auto-update for households (no such mechanism exists in
  `internal/` or `web/`).
- **Proposal (design is the owner's):**
  - The discovery publishes the latest version and a "security" flag, signed together with
    recommendation 3's digest.
  - The core shows its admin a banner when it is behind.
  - An opt-in automatic security update.
  - The paid discovery may warn on, or eventually refuse, cores below a minimum security version,
    since we already hold that lever.

**8. D220 needs a fast lane and a slow lane.**
- **Risk:** the week pulls both ways. Malicious releases were removed within hours (LiteLLM 2 h,
  OVHcloud 94 min), while exploits now follow disclosure within days, and for this kernel bug
  within hours.
- **Proposal:**
  - **Slow lane:** a cooling-off rule for routine bumps. Adopt no release younger than 7 days, and
    take none whose source commit is not an ancestor of the recorded one (D220 §3; this also
    defeats Plugin4Shell-style ref shadowing if the check resolves commits by full SHA with
    `^{commit}`).
  - **Fast lane:** a security bump in the same major line, for code we can reach, gets an audit of
    just the fix's diff, then ships through the security release path. The full history audit
    follows within the week.

**9. A standing check of what production runs.**
- **Covered:** check-vulns runs in check-pr and nightly on `main`.
- **Gap:** production runs older commits (the production NAS's commit and the last connect package), which
  `main`'s check does not cover.
- **Proposal:** a daily govulncheck and `npm audit` on those commits, plus a watch on the kernel
  for Ubuntu and Proxmox. The owner is told the same day when something reachable appears.

**10. Library watch items (low risk, no action yet).**
- **SQLite FTS5 CVEs (unconfirmed):** our build carries 3.53.4 and FTS5 is in use (D48). Bump
  modernc when it ships SQLite 3.54.x and OSV lists the entries; check-vulns will catch it.
- **FFmpeg CVE-2026-107678 (`pssh` recursion):** costs one run (a crash) at worst.
- **FFmpeg CVE-2026-107676:** leaks at most 3 bytes of the confined FFmpeg process's memory, which
  holds only media.
- Take both in the next jellyfin rebase.

**11. Browsers on TV-class devices.**
- This week's Firefox fixes are in WebRTC and Service Workers, the two features Orpheum's client is
  built on.
- Our peers are the core and members only, which is low exposure. But TV browsers and receivers lag
  months behind.
- **Proposal:** record a minimum browser version for the receiver and TV paths, and warn below it
  (D-level design, the owner's).

**12. TURN allocation caps.**
- **Gap:** a member's credentials could drain `RelayPortMin..Max` (Coturn CVE-2026-73215's class).
- **Proposal:** check whether pion/turn caps allocations per username. If not, cap them in
  `authHandler` or the event handler.

## 3. The trend: AI-speed exploitation, and what it means for our cadence and dependency policy

**Finding is now cheap, on both sides.**
- In one week, Anthropic described a model that built a remote kernel root exploit in under an hour
  (CVE-2026-98432) and found 62 Chrome CVEs.
- Chrome's 2026-10-27 critical fixes were AI-assisted reports.
- CrowdStrike sees three times as many agentic intrusions as in the first half-year.
- Secondary roundups of M-Trends 2026 put mean time-to-exploit at minus seven days (exploited
  before a patch exists), against a median 43-day patch time in organisations (Verizon DBIR 2026).
  These figures are [S], quoted from summaries.

**Exposure is ordinary software, not exotic attacks.** Bug classes that were rare to weaponise
(races in TCP setup, a parser panic in pion/stun) now get weaponised. Everything Orpheum exposes to
the internet is that kind of code:
- the TURN relay's STUN/DTLS parsing;
- the WebRTC data-channel request reader (`internal/core/rtcapi/channel.go`);
- connectd;
- the VPS kernels.

**What it means for patch cadence:**
- A batch rhythm (days to weeks, one check-pr per batch) is right for features, wrong for these
  components.
- A reachable, internet-facing advisory should reach prod-api, the edge and the production NAS within
  24–72 hours, through the security release path the owner approved tonight.
- It should reach households through item 7, which does not yet exist.
- The VPS kernels need an owner on duty: unattended upgrades and a reboot policy, because
  unattended-upgrades does not reboot.

**What it means for dependency policy:**
- The allowlists (`deps/*.allow`) and the small dependency tree are the best defence we have. Every
  module not taken is one fewer parser for an attacker's model to read.
- D220's audit-on-bump stays, split into the two lanes of item 8: slow and cooled-off for new
  versions, fast and diff-scoped for security fixes.
- Malicious releases are usually pulled within hours, so a 7-day cooling-off costs us little and
  avoids most of them. A security fix, by contrast, cannot wait 7 days, so it skips the cooling-off
  and gets the narrow audit instead.

**Point the same tools at our own code.**
- The node's FFmpeg parses hostile media under Landlock. The core's network parsers have no such
  confinement.
- Fuzz the internet-facing parsers (the data-channel request framing and the TURN auth path) the
  way ffmpeg-fdproxy is fuzzed weekly.
- Give an agent an adversarial audit pass of the core's unauthenticated surface at each milestone
  audit (AGENTS.md rule 17), so the next Plex2Shell-shaped chain is found by us first.
