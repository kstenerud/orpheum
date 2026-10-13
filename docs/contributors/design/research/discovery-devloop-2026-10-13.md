> **ABOUTME:** A development loop for building discovery across Orpheum and the closed service
> without slowing work down: where today's loop spends its time (measured), how the two
> repositories are built and tested together, a fake for every external system, the test tiers and
> their budgets, a whole-system stack in one command, the live staging path in the sandbox, and the
> numbers to hold. Gathered 2026-10-13.

# A development loop for discovery

Written **2026-10-13** at an earlier commit, from the repository, its gate logs and timings, and the
tools' own sources and documentation (cloned under `/yoloai/cache/discovery-devloop/`). The owner:
*"I also want a tight dev loop so that it doesn't slow down development (this has traditionally been
our biggest source of development slowdown before we optimized it)."*

**[V]** means confirmed here, by a measurement, the code, or a tool's source. **[I]** means inferred
and not run. Nothing here changes a gate. Every gate change and dependency it proposes is listed for
the owner in §10.

Rules that bound the answers:

- **Rule 16:** no test touches the network, the real media library or the real clock. A network
  namespace with no route off the machine is not "the network" (§4.3 keeps it that way).
- **Rule 6:** a missing capability fails. A tier that needs root, miniupnpd or Pebble fails where
  they are absent. It does not skip.
- **Rule 4 / D190:** every behaviour change needs a test that goes red when it is reverted. That
  makes revert-red's cost per file the number that matters most (§1.3).
- **D279 / principles §23:** the spot check takes 30 s, `make check` about 90 s, `make check-pr`
  300 s.

## 0. The answer in brief

1. **Today's loop is fast where it is in-process, and slow where it starts processes per file.** A
   change to discovery code runs its six packages' tests in **1.2 s uncached** [V], and the dev
   binary rebuilds in **3.0 s** [V]. `make check-pr` took **2,498 s** on 2026-10-13, against a
   300 s budget. **2,265 s of that was revert-red** [V]. In revert-red, a Go probe of one package
   costs **6.6 s** on average, a browser-tier probe **95 s**, and a `vitest related` probe
   **38 s** [V]. Discovery will add many files on both the Go and web sides. **Whether its loop is
   tight comes down to one thing: each discovery behaviour must be turned red by an in-process
   package test, so revert-red settles it at about 6 s, not 95 s.**
2. **Two repositories, one step.** The service's `go.mod` holds `replace github.com/kstenerud/orpheum
   => ../orpheum`, so it builds against Orpheum's working tree. A pin file records the Orpheum
   commit a release was built from. A `go.work` file alone cannot do this job [V: golang/go#50750].
   Go's test cache then re-runs only the service packages that import a changed discovery package.
   **A gap found:** the agent's hooks are Orpheum's, so an edit in `../orpheum-discovery` triggers
   no gate at all [V].
3. **Every external system has one in-process fake for `make check`, and one real-software
   counterpart in a slower tier that keeps the fake honest.** The repository already has the
   pattern: a fake ACME CA, a fake Cloudflare API, fake routers that speak PCP, NAT-PMP and UPnP,
   `discovery/single`, and `clocktest`. The new pieces are pion's virtual network for NAT behaviour
   in-process (already in the module cache, but IPv4 and UDP only [V]); Pebble, which runs
   DNS-PERSIST-01, ARI and profiles [V]; miniupnpd; coturn; and a **netlab** of network namespaces.
   A four-namespace NAT44 and IPv6 home builds in **0.4 s** and tears down in **47 ms** [V], but
   only on the sandbox host as root. It cannot run inside `./dev` [V].
4. **Five tiers:** spot, `make check` and `make check-pr` stay in-process apart from one browser
   test and a few Pebble tests. A new **netlab tier** (minutes, on demand, required before a batch
   that changed networking code) runs real kernels, browsers, miniupnpd, coturn and Pebble across
   six home-network shapes. A **live tier** (VQ items) covers real DNS, Let's Encrypt's staging, the
   probe VM, routers and phones.
5. **The whole system in one command** comes from the service's repository, which knows both
   repositories while Orpheum must not know the closed one. It runs the service, two cores, a fake
   DNS and mail, and Vite serving the client on the `app.` origin, with Go rebuild-on-save and
   hot module reload. Named **scenarios** start it already enrolled, claimed and linked.
6. **Live staging runs in the sandbox, beside Orpheum's**, reached through Cloudflare (by a tunnel)
   and the VPS (for STUN's UDP). It uses a staging subzone and Let's Encrypt's staging CA, and is
   deployed per batch, never from a gate.

## 1. How the loop works today, and where its time goes

### 1.1 The tiers as built

| Run | What | Budget | Measured | Source |
| --- | --- | --- | --- | --- |
| `make spot` | changed Go packages and their direct importers; vitest's direct importers of changed web files; the Python gates' tests if a script changed; the docs gate for Markdown (`scripts/spot_check.py`, two jobs) | 30 s | reports its own overrun | testing.md §10 |
| `make check` | every gate needing only the toolchain, in parallel after `check-tools`, `web-deps`, `go-warm`; web tests, depcruise and the gates' tests skipped when nothing they read changed (`area_stamp.py`) | ~90 s | unchanged tree **17 s** (2026-10-08); the web suite alone **58.9 s** for 123 files and 633 tests in today's gate log, up from 35 s and 95 files on 2026-10-08 [V] | test-audit-2026-10-08.md; `.git/gate-logs/check-verbose.log` |
| `make check-pr` | integration tier, browser tier, uncached eslint, then the home acceptance alone, then revert-red, test weakening, commit lint | 300 s | **2,498 s** on 2026-10-13 [V] | `.git/gate-logs/check-pr-verbose.log` |
| `./staging` | runtime image from the checkout, `write-deployment`, compose up | — | the copy step **11.4 s**, `make build` **11.8 s**, the rest a few seconds when cached [V] | `/yoloai/cache/staging13.log` |
| `make dev` | core + node + Vite, rebuilding on any `.go` change (`scripts/dev.py`, a 1 s poll) | — | `-race -tags dev` rebuild after a change to `discoveryproto` **3.0 s**; the first one this session 21 s; without race 2.4 s [V] | measured in `./dev` |

### 1.2 The last check-pr, step by step (2026-10-13)

| Step | Seconds |
| --- | --- |
| check-revert-red | **2,265** |
| test-browser (21 tests, 1 worker; the four voice tests take about 20 s each) | 189 |
| web-lint-full | 110 |
| test-acceptance-home | 44 |
| test-integration | 36 |
| check-test-weakening, check-decision-status, lint-commits | 17 |

### 1.3 Revert-red: the step that decides discovery's loop

From `.git/revert-red/last-timings` (167 files, 4 worktrees, **2,262 s wall, 8,334 s of probing,
3,332 s of it waiting for the one browser tier at a time**) [V]:

| Kind of run | Runs | Seconds | Per run |
| --- | --- | --- | --- |
| vitest related | 54 | 2,033 | **37.6** |
| browser tier | 14 | 1,331 (+3,332 waiting) | **95** |
| Go, whole suite | 7 | 385 | 55 |
| Go, one package | 53 | 348 | **6.6** |
| npx (build, related of added files) | 23 | 289 | 12.6 |
| vitest, whole suite | 3 | 208 | 69 |
| Go, extra tags | 3 | 166 | 55 |
| Python tests | 7 | 128 | 18 |

Commit an earlier commit (the same day) takes new stylesheets out of the browser batch and turns on
vitest's transform cache. The cost structure does not change: **a file costs what the narrowest
test that turns it red costs, times the number of files.** That is mechanism 5 of
[test-weight.md](../../test-weight.md). Discovery will be about as large as Watch Together and
voice together, spread across `discovery/`, `internal/core/{discovery,portmap,certificate,api}`,
`web/packages/sdk`, `web/packages/player` and `web/apps/web`. If its behaviours are only visible in
a browser or in a whole-suite Go test, each file costs 55–95 s, and a 40-file batch alone costs more
than the whole budget.

### 1.4 What already works in discovery's favour

- **The discovery code is in-process and fast.** `discovery/{proof,safe32code,single}` and
  `internal/core/{discovery,portmap,certificate}` run in **1.2 s** together, uncached [V]. They run
  against `single`, the fake CA (`certificate/certificatetest`, with DNS-01 checked against a fake
  DNS), the fake Cloudflare API (`providers/cloudflare/cloudflaretest`), fake routers speaking PCP,
  NAT-PMP and UPnP (`portmap_test.go`), and `clocktest`.
- **The public packages are light.** `go list -deps ./discovery/...` outside the standard library is
  `github.com/coder/websocket` and nothing else [V]. The service gains one module by importing them.
- **The browser tier already runs WebRTC** (`web/e2e/webrtc.spec.ts`, 1.7–5.3 s per test) and
  fake microphones. **The acceptance already shapes links with netem** on the dev container's
  loopback (D252, D263), and `./dev` grants `NET_ADMIN` for it.

### 1.5 Gaps found

1. **No gate runs for the service's files.** `.claude/hooks/post_edit.py` records an edit only when
   `gate_paths.relative()` places it inside Orpheum. For `../orpheum-discovery` that function
   returns `None` [V]. A session rooted in Orpheum that edits only the service ends its turn with no
   gate.
2. **`./dev` cannot build network namespaces.** Inside it the user is uid 1000 with `NET_ADMIN`
   only. `ip netns add` fails ("mkdir /run/netns failed: Permission denied"), `unshare -rn` fails
   ("Operation not permitted"), and `nft` and `iptables` are not installed [V]. The sandbox host can
   build them: it is privileged, has `sudo`, `ip`, `nft` and `iptables`, and its kernel makes
   `ip6tnl` (DS-Lite's tunnel) and `wireguard` links in a namespace [V].
3. **Production timers in the code paths a dev loop restarts.** C3 waits a flat 30 s between
   signalling dials. C4 waits a fixed 30 s for the TXT record (discovery-core.md, "Phases built").
   Every restart of the dev service would then leave each core offline for up to 30 s. That is
   mechanism 1 of test-weight.md, moved into the interactive loop.
4. **The dev loop runs one core** on fixed ports (8432, 5173, `.dev/core`). Discovery needs at
   least two cores: one account's core and another household's.
5. **The machine:** 8 CPUs, 15 GB of memory, **23 GB of disk free** (85% used) [V]. Images for the
   netlab and the service's staging must stay small and be pruned.

## 2. Principles for discovery's loop

1. **Red in-process.** Every behaviour of discovery has a package-level test, with fakes, that goes
   red when it is reverted. A browser or netlab test confirms the whole. It is never the only test
   that sees a behaviour. This keeps revert-red at about 6 s per Go file. On the web side, the path
   chooser and the cross-origin client live in SDK modules imported by narrow paths, never through
   `index.ts`, because a barrel makes "related" two-thirds of the suite (test-weight.md §5).
2. **One fake per external system, in one place,** where both repositories can reach it: a public
   `discovery/…test` package when the service needs it too. An `internal/` fake cannot be imported
   by the service.
3. **Real software confirms the fakes, in few tests, outside `make check`.** Pebble confirms
   `certificatetest`, miniupnpd confirms the fake routers, the kernel's NAT confirms pion's virtual
   NAT, and coturn confirms the TURN credentials. When the two disagree, the fake is fixed first.
4. **Time is injected everywhere.** Each production interval (dial backoff, the TXT wait, the
   heartbeat, claim-code life, lapse states) is a config value with a production default. Tests
   and the dev stack set milliseconds or seconds. A test whose time is near a round config number
   is the warning sign.
5. **Nothing leaves the machine except the live tier,** and that tier is not a test run: it is VQ
   items and the staging deployment.

## 3. Developing the two repositories together

### 3.1 Module layout

- **Orpheum stays one module.** `discovery/` stays inside it, as D275 §1 and the service plan say.
  A nested `discovery/go.mod` would need Orpheum itself to `replace` its own subdirectory and to tag
  releases with a prefix. It would buy only a smaller dependency graph, and that graph is already
  one module (§1.4).
- **The service's `go.mod`:**

  ```
  module github.com/kstenerud/orpheum-discovery   // [I: the private path the owner picks]
  require github.com/kstenerud/orpheum v0.0.0-00010101000000-000000000000
  replace github.com/kstenerud/orpheum => ../orpheum
  ```

  **Why `replace` and not a workspace alone:** `go mod tidy` ignores `go.work` and must resolve every
  requirement on its own. The Go team closed that request as not planned (golang/go#50750, May
  2025) [V]. While Orpheum is private and the sandbox has no GitHub, nothing else resolves it.
  Once Orpheum is public (D192), the service requires a real tagged version and keeps a
  git-ignored `go.work` for the sibling loop.
- **The pin.** The service holds `ORPHEUM_PIN`: the Orpheum commit, and the tree hash of
  `discovery/` at that commit (`git rev-parse <commit>:discovery`).
  - **While working,** the service builds against Orpheum's working tree, uncommitted changes
    included. That is what makes a spanning change one step.
  - **At the service's check-pr,** a step fails when the working tree's `discovery/` differs from
    the pinned commit's, which means an Orpheum change is uncommitted or the pin is stale.
    `make pin-orpheum` rewrites the pin to Orpheum's `HEAD` once Orpheum's own gate has passed
    there.
  - **The service's image** takes Orpheum's files from the pin (`git -C ../orpheum archive <pin>
    discovery go.mod go.sum`), never from the working tree, so a release is reproducible however far
    `../orpheum` has moved.
- **Versioning the public packages:** no separate semver while Orpheum is private. The pin is the
  code's contract. The wire's contract is the protocol's `major.minor` header (protocol §5), which
  the service's CI checks against the newest and oldest supported cores (the spec's §9.3). Breaking
  the Go API of `discovery/` is allowed, since the service follows by pin in the same session
  ("break early").

### 3.2 A change across both, tested in one step

- **`make spot-both`**, in the service's repository: Orpheum's `make spot` in `../orpheum` and the
  service's own spot, at once. Go's test cache keys on the replaced sources, so the service re-runs
  exactly the packages that import a changed `discovery/` package and nothing else [I: standard
  `go test` caching; the replace is a local directory].
- **The conformance suite** is exported from Orpheum as `discovery/discoverytest` [I: the plan's
  "exported conformance suite"]. It is a function that takes an `http.Handler` and a clock and runs
  the protocol's cases. `single`'s tests run it, and the service's handler tests run the same
  function. A protocol change in Orpheum turns the service red at its next spot, with no copied
  test vectors.
- **The gate for the service's edits (gap 1).** Orpheum's committed hooks must not name a closed
  repository. So **a sandbox-local Stop hook**, in `.claude/settings.local.json` (git-ignored, the
  owner's to write, since `.claude/` is a gate), runs `make check` in `../orpheum-discovery` when its
  `git status` shows a change, beside Orpheum's own. The service copies Orpheum's hook scripts for
  sessions rooted in it.

### 3.3 The service's own gates

The service plan's P1 says the service copies Orpheum's standards and gates. **Copy them once, at
M-S1, with the origin commit recorded, trimmed to what a smaller codebase needs.** Keep: `make
check` (fmt, lint, vet, tests, `check-generated` for sqlc, `check-deps`), `make spot`, `make
check-pr` (revert-red, test weakening, commit lint). Leave out: the browser tier, which stays in
Orpheum because the client is Orpheum's; the integration tier until the service has one; the docs
gates, beyond links. Sharing the scripts by import would tie the service's gate to Orpheum's
working tree, which is the coupling the pin exists to bound.

## 4. Fakes and real counterparts, one per external system

### 4.1 The table

| External system | In-process fake: spot and `make check` | Real software: check-pr or netlab | Live tier |
| --- | --- | --- | --- |
| **ACME CA** (core K9; service P5) | `certificatetest` **extended**: ARI's `renewalInfo` with a settable window, a new order's `replaces`, `dns-persist-01`, any validity, an injected `now` | **Pebble**, pinned binary (§4.2): DNS-PERSIST-01, ARI, `replaces`, profiles, against our DNS fake. A few tests in the integration tier | Let's Encrypt **staging**: CAA, rate limits, propagation across its vantage points (VQ in K9) |
| **Bunny DNS API** (records: CAA, `_validation-persist`, `_acme-challenge` TXT) | `bunnytest` in the service: an `httptest` fake of the record calls with a token check, the shape of `cloudflaretest` | — | the staging subzone (§7) |
| **Bunny's computed answers** (a JavaScript `handleQuery(query)` returning `ARecord` and the like [V: Bunny's docs]) | The name-to-address rule is written once, in Go (`discoveryproto`'s name encoding), with exported **test vectors** (JSON). The JS script runs in **Node** (in the dev image) against the same vectors through a ~30-line shim of the record classes. No npm dependency | the netlab's authoritative DNS serves the same rule from Go | Bunny's script editor's test fields, then `dig` against Bunny; whether one script record answers a whole wildcard subtree is the owner's open question 3 |
| **Authoritative DNS / resolvers** (service: "every authoritative server serves the record"; core: "the record resolves before the CA is asked") | a fake authoritative server in the test, on `golang.org/x/net/dns/dnsmessage` (§10, item 6) or hand-rolled as port mapping was; resolvers given by `net.Resolver{Dial: …}` | the same server as a process in the netlab, with `/etc/netns/<ns>/resolv.conf` pointing each namespace at it; or `pebble-challtestsrv`, whose management API sets A, AAAA, CAA, TXT and SERVFAIL answers [V] | Bunny and Cloudflare |
| **Cloudflare in front** (`CF-Connecting-IP` trusted only from Cloudflare's ranges, P7) | a test reverse proxy that sets the header; the ranges injected; a request from outside them has its header ignored | — | staging behind Cloudflare (§7) |
| **Cloudflare DNS** (the core's own name, D243 §6) | `cloudflaretest`, exists | — | staging, exists |
| **STUN** (W4b.3; K14 needs two servers) | `single`'s STUN on `pion/stun/v3` (allowed); two servers in one test for K14 | the service's STUN in the netlab's "cloud" namespace | the service's STUN and the probe machine's |
| **NAT behaviour for ICE** | **pion's `vnet`** (§4.3): a virtual router with NAT types, core and client pion peers through it, in-process. Covers ICE between pion peers across mapping and filtering types | the **netlab's kernel NAT**, browsers included | real networks |
| **TURN** | the core's own relay, `internal/core/turn` (pion), exists | **coturn 4.6.1** (Debian trixie [V]) in the netlab, so K13's REST-shaped credentials work against the server owners actually run | an owner's TURN, later |
| **Port mapping** (K5) | the fake routers in `portmap_test.go`, extended as K5 lists: epoch, ANNOUNCE, conflicts, pinholes | **miniupnpd** 2.3.9 (Debian trixie: `miniupnpd-nftables` [V]) as the netlab's router: UPnP IGD v1/v2, NAT-PMP and PCP [V: its source], IPv6 pinholes when built `--ipv6 --igd2` [I: Debian's build flags not checked], real nft rules, so the probe really reaches the core through the mapping | FRITZ!Box and OpenWrt (the K5 VQ) |
| **The probe** (P8, K14) | its refusal rules (private, shared, documentation, cloud-internal and the service's own addresses) as pure functions; its dialer injected; the core's tests against a fake probe answering verdicts | the real probe binary in the netlab's "outside" namespace | the probe VM |
| **The core's addresses** (K4, netlink) | recorded netlink messages, as the plan says | the netlab's namespaces with temporary, deprecated and bridge addresses | the owner's machines |
| **Mail** (sign-in links, P2, P3) | a sender interface; the fake keeps messages | the dev stack shows them on a page and in the log | the EU provider |
| **Geolocation** (P9, after the beta) | a fixture behind an interface | — | the database, once the owner chooses it |
| **Time** | `clocktest`; the service's own clock package (copied standard); public packages take `now` | Pebble runs on the real clock (`time.Now` [V]): renewal is tested with a short profile and ARI's override, never by waiting | — |
| **The browser's clock** (W2c's ~3 s race window, D291 §3's demotion) | vitest fake timers in the SDK | Playwright's `page.clock` in the browser tier [I: available since 1.45; the repo pins 1.63] | — |
| **Client integrity** (SRI, CSP, `Integrity-Policy`, W4c, P10) | a test over the built `dist/`: no inline script, an `integrity` on every script | the browser tier serves the build with the headers | Cloudflare Pages |

### 4.2 Pebble, verified from its source (an earlier commit, 2026-09-28)

- **Has:** `dns-persist-01` (`acme/common.go`), validated against `_validation-persist.<name>` with
  `issuer-domain-name`, `policy` and `persistUntil` (`va/va.go`). ARI's `renewal-info` endpoint,
  plus a management call, `POST /set-renewal-info/`, that sets any certificate's answer, malformed
  ones included (`wfe/wfe.go`). A new order's `replaces`. Certificate **profiles** with their own
  `validityPeriod`. `PEBBLE_VA_NOSLEEP` and `PEBBLE_VA_ALWAYS_VALID`, and `-dnsserver` to send its
  lookups to our fake.
- **Lacks:** rate limits, and pre-authorization (README, "Limitations"). Its validation authority
  never mentions CAA, so **CAA is not checked** [V: grep]. It uses the real clock.
- **Watch for:** by default it rejects 5% of valid nonces (`PEBBLE_WFE_NONCEREJECT`), so the core's
  ACME client must retry `badNonce`, as Let's Encrypt also requires. A Pebble test without that
  retry is flaky by design. Set it to 0 in tests that are not about nonces, and keep one test at 100
  that checks the retry.
- **How it arrives:** as a pinned binary fetched and hash-checked like FFmpeg (`scripts/fetch_*.py`),
  or built from the pinned module in the dev image. It is a tool, not a Go dependency of Orpheum, but
  the pin and the fetch script are gates (§10, item 7).

### 4.3 Simulated networks: two levels

**In-process: pion's `vnet`** (in the module cache as `pion/transport/{v3,v4,v5}`) [V from its
source]:

- `NATType{Mode, MappingBehavior, FilteringBehavior, …}` gives endpoint-independent and dependent
  mapping and filtering, and 1:1 NAT. Routers nest to form a tree, and virtual STUN and TURN servers
  live in it.
- **Limits:** IPv4 only (the router's subnet is parsed as `ipv4Net`), and **UDP only**: `DialTCP`
  returns `ErrNotSupported` in v5. It covers ICE between pion peers (the core and `rtcapitest`'s
  client), not browsers, TCP, IPv6 or DS-Lite.
- `webrtc.SettingEngine.SetNet` takes a `pion/transport/v5` `Net` [V], so the core's `rtcapi` needs a
  seam to accept one. That is a small code change, with a test.
- **Cost:** in-process, so seconds for a matrix of a dozen pairs [I: to measure once written].
  It belongs in `make check`.

**Kernel: the netlab,** network namespaces on the sandbox host, run as root:

- **Measured:** four namespaces (a "wan" bridge, a router doing NAT44 with nftables and an IPv6
  firewall that admits only established traffic, a "home" core, an "outside" host), built in
  **433 ms** including the nft rules and torn down in **47 ms**. From the core, IPv4 and IPv6 went
  out. From outside, IPv4 was blocked (no mapping) and IPv6 was blocked (the home firewall), as a
  real home is [V].
- **Topologies:**
  1. a home on dual stack: NAT44 plus an IPv6 firewall, miniupnpd on the router;
  2. CGNAT: the home router's WAN in 100.64/10, behind a second NAT;
  3. **DS-Lite:** native IPv6 plus an `ip6tnl` (`ipip6`) tunnel to an AFTR namespace doing NAT44
     (the owner's own network; `ip6tnl` creation verified [V]);
  4. **symmetric NAT:** nft `masquerade random,fully-random`, an endpoint-dependent mapping, so
     WebRTC needs TURN [I: to confirm with K14's two-STUN measurement in the netlab];
  5. **UDP blocked**, with only TCP 443 open (hotel or corporate), so TURN over TCP and the
     direct HTTPS paths are what is left;
  6. **a phone on IPv6 only** [I: NAT64 needs Jool or TAYGA, neither verified here; leave it out
     until a test needs it].
- **Actors per run:** a core and its node in "home"; the service, its STUN, Pebble and the fake
  authoritative DNS in "cloud"; the probe in "probe"; Playwright's Chromium and Firefox in "away"
  and in "home".
- **Kept off the network (rule 16):** no namespace has a route to the sandbox's uplink. The setup
  asserts that a public address is unreachable before any test runs, and fails otherwise.
- **Where it runs:** not in `./dev` (gap 2). Either a host-side script (as `./staging` is, stdlib
  Python) runs a **netlab container** from the dev image plus `nftables`, `miniupnpd`, `coturn`
  and Pebble, with `--privileged` or `CAP_SYS_ADMIN`, and makes the namespaces inside it; or it runs
  directly on the sandbox host with `sudo`. **The container is the better choice:** one image pins
  the tools, it is torn down whole, and staging's networking stays untouched.

## 5. The tiers

| Tier | Runs | Discovery's share | Budget | When |
| --- | --- | --- | --- | --- |
| **Spot** | changed packages and their importers, in each repository; `make spot-both` for both | in-process tests only; the conformance suite through `single` and the service's handlers | **30 s** total; discovery's packages today take 1.2 s | while working |
| **`make check`** (each repository) | as today; plus, in the service, its unit tests, the Bunny script's vectors in Node, and a build against `../orpheum` | every fake of §4.1's first column; the vnet ICE matrix; the conformance suite | **90 s**; discovery's added Go test time **≤ 10 s** wall | each commit (hooks) |
| **`make check-pr`** (each repository) | as today; plus in Orpheum: **one** discovery browser test (W4b.6: link → locate → WebRTC through `single` → sign-in → first chunks), Chromium only, Firefox only for what differs; **Pebble** integration tests; in the service: the pin check (§3.1) | revert-red settles every discovery file at package level | **300 s**; discovery's added wall **≤ 45 s**: browser test ≤ 20 s with the film cache, Pebble ≤ 15 s, the rest in the parallel phase | a batch, before the owner applies |
| **Netlab** | §4.3's topologies × the path race, port mapping, the probe's verdict, the NAT measurement, Pebble issuance and renewal through the fake DNS, WebRTC via coturn when UDP is blocked, an ICE restart on a network change | the whole system on real kernels and browsers | **≤ 10 min** for the matrix; **≤ 90 s** for one topology while fixing it | on demand; **required before applying a batch that changed networking code** (a `netlab_needed.py` in the shape of `browser_tier_needed.py`); at each milestone's end |
| **Live** | Let's Encrypt staging, the Bunny zone, Cloudflare, the probe VM, a FRITZ!Box and OpenWrt, phones on mobile data, an iPhone's home-screen app | VQ items (K5, K9, W2c, W4c already name them) | — | before M-S7's steps; after a change a VQ names |

**Check-pr must be back in its budget before discovery adds to it.** Today it is at 8× its budget,
and the overrun is not discovery's. The two levers in §1.3 are the 54 `vitest related` runs at
38 s each and the browser runs that revert-red queues one at a time. With those still in place, even
a fully in-process discovery batch rides on a 40-minute gate. This is the owner's call (§10, item 4).

## 6. The whole system in one command

It lives in **the service's repository** (`make dev`), because it starts the closed service and
Orpheum must not name it. It drives Orpheum through Orpheum's own make targets.

| Piece | How | Reload |
| --- | --- | --- |
| The service | its binary with a dev config: fake mail (links on a `/dev/mail` page and in the log), the fake authoritative DNS answering computed names for the dev zone, the fake Bunny, the fake CA or Pebble | rebuilt and restarted on save, by a loop in the shape of `scripts/dev.py` |
| Two cores | Orpheum's dev loop run twice with an **instance index**: ports 8432+10·n, `.dev/core-n`, `.dev/node-n` (a change to `scripts/dev.py`, a gate, §10 item 9) | rebuilt on save (3.0 s measured); cores reconnect to the service at once |
| The client on `app.` | Vite on a port of its own (5174), in W4c's build mode, its discovery origin the service's | Vite's hot module reload |
| A browser | the owner's, over the LAN (`devnas.example`, as today), or the agent's headless Playwright | — |

- **Scenarios** take away the clicking: `make dev SCENARIO=claimed` starts with an account, core 1
  enrolled and claimed, core 2 pending, a registration link issued, and prints the URLs to open.
  Others: `fresh`, `two-households`, `lapsed`, `device-code`. A scenario is a seed file like
  `deploy/dev/seed.toml`, applied through the APIs, so it tests them too.
- **Reconnect timers (gap 3):** K2's backoff starts at about 1 s with jitter, and the dev config
  caps it at 1 s, so restarting the service brings every core back within about 2 s. That rule
  belongs in K2's change [I: within the plan's "Sessions, retries and instances"].
- **Names on loopback:** the zone never answers loopback (D290 §7, D291 §2). The dev stack needs
  names that do. **A dev-build-tag-only zone setting** allows 127/8 for the dev zone, and an
  architecture test proves the release build lacks it, as other `dev`-tag code is kept out. The
  alternative, the stack inside the netlab with real private addresses, cannot be reached by the
  owner's own browser.
- **Start time:** **≤ 20 s warm** from the command to a usable page (two core builds are cached;
  the service builds in about 3 s [I]); **≤ 5 s** from a save to the restarted process.
- **The netlab version:** `make dev NETWORK=ds-lite` runs the same stack inside a netlab topology,
  driven by the agent's Playwright. It is for reproducing a path-race or NAT report interactively.

## 7. The live staging path in the sandbox

The owner: *the live access points go to what will become our new staging server inside this
sandbox.*

```
browser ─► app./account./api.<staging names> ─► Cloudflare ══ Cloudflare Tunnel ══► service (sandbox)
browser ─► <core-name>.<staging zone> (Bunny, computed) ─► VPS ═ WireGuard ═► staging core (sandbox)
core/browser ─► STUN :3479/udp ─► VPS ═ WireGuard ═► service's STUN (sandbox)
service ─► probe VM (Hetzner, its own) ─► the core's published addresses
```

- **The service's staging** is the service's own `./staging` (the pattern of Orpheum's). It runs
  compose services with their data in the service's `.staging/`, from an image built from the
  service's checkout and Orpheum's pinned files.
- **HTTPS ingress by Cloudflare Tunnel** (`cloudflared` as a staging service: outbound only, no open
  port). The VPS already forwards TCP 443 to the core by DNAT, keeping the client's address
  (`deploy/vps/README.md`), so the service cannot share that port. The spec's §7.3 names a tunnel as
  a production option, so staging tests it. The other choice, a second VPS port and a Cloudflare
  Origin Rule, changes the VPS's nft rules and depends on what Cloudflare's free plan allows [I].
- **STUN's UDP** cannot go through Cloudflare (the spec's §7.3 [V there]). One more forward on the
  VPS: UDP 3479 into the tunnel. That is the owner's machine and needs a Hetzner firewall rule
  (README §1's table).
- **The staging core enrols through the public `api.` name**, not the docker bridge, so the
  service sees the house's real address through `CF-Connecting-IP`, and the `lan` path rule (D290
  §2) is tested as in production.
- **DNS and certificates:** a **staging subzone** on Bunny, and Let's Encrypt's staging directory
  (the core's `https.acme_directory` already takes it, D261 §5). Production's zone and quota are
  never touched.
- **The probe** runs on its own small VM from the start, holding no secret but its posting token
  (D291 §1, S10). The VPS holds the tunnel's keys, so a probe there would be a stranger-directed
  dialer beside a secret.
- **Keeping it off the loop:**
  - Nothing in spot, check or check-pr talks to staging.
  - A staging deploy follows a batch's check-pr, as Orpheum's does. Target **≤ 60 s** warm per
    repository.
  - The memory rules carry over: no redeploy while someone is viewing, and no gates while the owner
    tests live.
  - Two staging stacks share 8 CPUs. The service's is light (one Go process, SQLite), so its CPU
    cost is small. Its image must be pruned (23 GB free).

## 8. Numbers: targets, and what to measure

| What | Target | Today | How to measure |
| --- | --- | --- | --- |
| spot, a discovery change in one repository | ≤ 30 s | 1.2 s of test time for the area [V] | `make spot`'s own report |
| spot-both, a change spanning both | ≤ 30 s | — | the same, in the service |
| `make check`, a discovery-only change | ≤ 90 s; discovery's Go tests ≤ 10 s wall | an unchanged tree 17 s; the web suite 59 s | `go test -json` per package; the gate's verbose log |
| revert-red per discovery Go file | ≤ 10 s (package level) | 6.6 s mean for a package probe [V] | `.git/revert-red/last-timings`, by kind; **a discovery file settled only by the browser tier or the whole suite is a defect** |
| revert-red per discovery web file | ≤ 15 s | 37.6 s mean, `vitest related` [V] | same; watch the size of each file's related set |
| `make check-pr` | ≤ 300 s; discovery adds ≤ 45 s | 2,498 s [V] | `check_pr_steps.py durations` |
| one netlab topology | ≤ 90 s; setup ≤ 1 s | setup 0.43 s, teardown 0.05 s [V] | the tier's own per-topology timing |
| the netlab matrix | ≤ 10 min | — | the same |
| `make dev` (stack), cold to usable | ≤ 20 s warm, ≤ 3 min cold | one core: 21 s first race build | timestamps in the stack's log |
| save → restarted process | ≤ 5 s | 3.0 s build [V] + the restart | the dev loop prints each build's time |
| service restart → every core back on signalling | ≤ 2 s | up to 30 s (C3's flat wait) | the service's count of connected cores, logged |
| staging deploy, warm | ≤ 60 s each | the core about 30 s [V] | `./staging`'s timestamps |

**Record each tier's wall time and CPU time** (test-weight.md: "look at CPU time, not just wall
time"). Add a test-weight.md entry when a discovery mechanism turns up, such as a reconnect wait in
a test's setup or a netlab topology rebuilt per test instead of per file.

## 9. What this proposes to build, in order

1. Before discovery's code, the owner's choice on check-pr's budget (§10, item 4).
2. With K1: `ORPHEUM_PIN`, the `replace`, `make spot-both`, the sandbox-local Stop hook, and the
   service's copied gates (M-S1).
3. With K1/W4b.7: `discovery/discoverytest`, the conformance suite.
4. With K2/K3: configurable backoff; the `rtcapi` seam for `SetNet`; the vnet ICE matrix.
5. With K5/K9/K14: the extended fakes (routers, CA, two STUN servers), then Pebble in the
   integration tier.
6. With W4b.6: the one browser test.
7. With M-S2: the dev stack and its scenarios; `dev.py`'s instance index.
8. With M-S4: the netlab and its topologies.
9. With M-S3/M-S7: the service's staging, the tunnel, the subzone, the probe VM.

## 10. Decisions for the owner

Gate changes (G) and dependencies (Dep) are marked. Each has a recommendation.

1. **How the service imports Orpheum.** `replace => ../orpheum`, plus a pinned commit and tree hash
   checked at the service's check-pr, and the image built from the pin. *Recommended.* A `go.work`
   alone cannot satisfy `go mod tidy`.
2. **The gate for edits in the service's repository (G, `.claude/`).** A sandbox-local Stop hook in
   `.claude/settings.local.json`, not committed, that runs the service's `make check` when its tree
   changed. *Recommended:* without it, a turn spent in the service runs no gate.
3. **The service's gates:** copied once at M-S1 from Orpheum's, origin commit recorded, trimmed
   (§3.3). *Recommended* over importing Orpheum's scripts.
4. **Bring check-pr back to 300 s before discovery's batches (G).** Restructure the
   revert-red web probes (`vitest related` at 38 s × 54) and its one-at-a-time browser runs, as
   D279 allows. *Recommended:* the first piece of work, since every discovery batch would otherwise
   wait 40 minutes.
5. **pion's `vnet` used directly (Dep: `github.com/pion/transport/v5` moves from indirect to
   direct, a new reason in `deps/go.allow`).** *Recommended:* it is already in the build, and it is
   the only in-process NAT simulation for ICE.
6. **`golang.org/x/net/dns/dnsmessage` used directly (Dep: a reason change in `deps/go.allow`)** for
   the fake authoritative DNS and for production's "the record resolves" and "every authoritative
   server serves it" checks. *Recommended*, over hand-rolling DNS encoding. It is the Go team's and
   already in the build.
7. **Pebble as a pinned tool (G: a fetch script and its hash, or the Containerfile)** for a handful
   of integration-tier tests. *Recommended:* it is the only local CA with DNS-PERSIST-01 and ARI,
   and it costs about 15 s of check-pr.
8. **The netlab (G: a host-side script and a netlab image with nftables, miniupnpd, coturn and
   Pebble, run privileged on the sandbox host)** as a tier required before applying a networking
   batch and at milestones, not in check-pr. *Recommended.* `./dev` cannot make namespaces, and
   giving the dev container `SYS_ADMIN` would widen every dev command's power for one tier.
9. **`scripts/dev.py` gets an instance index (G)** so two cores run side by side. *Recommended.*
10. **A dev-build-only allowance for loopback names** in the dev zone, proven absent from release by
    an architecture test. *Recommended*, so the owner's browser can use the local stack.
11. **Staging's ingress:** Cloudflare Tunnel (`cloudflared`, a new tool in the service's staging)
    for HTTPS, and one VPS UDP forward (3479) for STUN. *Recommended* over a second TCP port on the
    VPS with an Origin Rule.
12. **Staging's names:** a staging subzone on Bunny and Let's Encrypt's staging CA, the production
    zone untouched. *Recommended.*
13. **The probe on its own VM from staging on**, not on the VPS that holds the tunnel's keys.
    *Recommended*, at about the cost of the smallest Hetzner plan [I].
14. **Testing the Bunny script with a Node shim and Go-generated vectors**, without
    `@bunny.net/scriptable-dns-types` (Dep, npm, the service's). *Recommended:* no dependency.
15. **K2's reconnect backoff starts at about 1 s, and the dev config caps it**, replacing C3's flat
    30 s. *Recommended*, within K2's plan, or the dev loop waits 30 s on every service restart.

## Sources

- The repository at an earlier commit: `Makefile`, `scripts/spot_check.py`, `scripts/check_revert_red.py`,
  `scripts/dev.py`, `dev`, `staging`, `.claude/hooks/post_edit.py`, `scripts/gate_paths.py`,
  `deploy/staging/README.md`, `deploy/vps/README.md`, `deps/go.allow`,
  [test-weight.md](../../test-weight.md),
  [makefile.md](../../standards/makefile.md), [testing.md](../../standards/testing.md),
  [principles](../../principles/README.md) §23, D279,
  [test-audit-2026-10-08.md](test-audit-2026-10-08.md),
  [discovery-core.md](../plans/discovery-core.md), [discovery-mode.md](../plans/discovery-mode.md),
  [webrtc-transport.md](../plans/webrtc-transport.md),
  [discovery-protocol.md](../discovery-protocol.md).
- The gate's records: `.git/gate-logs/check-pr-verbose.log` and `check-verbose.log` (2026-10-13),
  `.git/revert-red/last-timings`, `/yoloai/cache/check*.log`, `checkpr*.log`, `staging13.log`;
  Earlier commits.
- The service's `docs/spec.md` (draft 5) and `docs/plans/service.md`, in `/home/karl/orpheum-discovery`.
- Measured in this sandbox, 2026-10-13: builds and tests in `./dev`; namespaces on the host.
- Pebble, `github.com/letsencrypt/pebble` at `1fcb30c`: `README.md`, `acme/common.go`, `va/va.go`,
  `wfe/wfe.go`, `cmd/pebble-challtestsrv/README.md`, `test/config/pebble-config.json`.
- miniupnpd, `github.com/miniupnp/miniupnp` at `f8c4f4c` (version 2.3.11): `miniupnpd/configure`,
  `miniupnpd.conf`, `pcpserver.c`, `natpmp.c`.
- pion: `transport/v4@v4.0.1` and `v5@v5.1.1` `vnet/` (README, `nat.go`, `net.go`, `router.go`);
  `webrtc/v4@v4.2.23` `settingengine.go`; from the dev image's module cache.
- Debian trixie packages: https://packages.debian.org/search?keywords=miniupnpd&suite=trixie
  (2.3.9-2+deb13u1: `miniupnpd`, `-iptables`, `-nftables`) and
  https://packages.debian.org/search?keywords=coturn&suite=trixie (4.6.1-2).
- Go workspaces and `go mod tidy`: https://github.com/golang/go/issues/50750 (closed as not
  planned).
- Bunny's scriptable DNS: https://bunny.net/docs/dns/scriptable/introduction
