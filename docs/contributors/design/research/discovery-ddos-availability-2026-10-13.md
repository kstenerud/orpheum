> **ABOUTME:** How the paid discovery service at app.orpheum-app.com survives attacks and outages: its
> attack surface (HTTPS API, WebSocket signalling, STUN on UDP, DNS, the static client), what
> protection costs at Hetzner, Cloudflare, Bunny, OVH, AWS and Fastly, what availability a one-person
> service can reach and which parts need it most, and what Plex, Tailscale, Synology, Nabu Casa,
> Emby, ZeroTier, ngrok and Cloudflare Tunnel do and suffered. Ends with a recommendation and costs.

# The discovery service: DDoS and availability

- **Date:** 2026-10-13.
- **Asked by** the owner (the spec's Q20): *"How will we handle DDoS attacks? How do we keep high
  availability? What do our competitors do?"*
- **Reads:** the paid service's specification (draft 4, the shared files'
  `discovery-service-spec.md`, §7 architecture, §7.6 "what an outage breaks", §10 milestones) and
  [discovery-protocol.md](../discovery-protocol.md). Builds on
  [reachability-approaches-2026-10-09.md](reachability-approaches-2026-10-09.md) ("the research").
- **Legend:** **[V]** checked at its source this session (see Sources; pages cached under
  `/yoloai/cache/discovery-audit/`). **[I]** this author's inference or recommendation.
  **[unverified]** seen only in a search excerpt or a forum.

## Summary

**The design already does the hard part.** The service is off the playback path. Names and
certificates outlive it for weeks. A returning device reaches its core on a remembered path (D273
§2, §7; protocol §1, §3.3). So an attack or outage mostly stops *new* things: first visits, claims,
WebRTC set-up. That is what Tailscale and ZeroTier also chose, and it is why their many
control-plane incidents rarely break existing connections **[V]**.

**Three findings change the plan:**

1. **DNS is on the playback path; the service is not.** A returning device reaches its core by a
   name under the service's zone. When the cached answer expires and the zone's name servers are
   down, the remembered path fails. Plex's own `plex.direct` DNS had four incidents between June and
   October 2026, one to three hours each **[V]**. So the zone wants an anycast provider with DDoS
   absorption, not two small self-run nodes. Bunny DNS answers from a script on anycast for free
   and could compute Plex-style answers **[V]**.
2. **Cloudflare Free has a 200-record limit on zones made since September 2024** **[V]**. The
   beta's explicit records (§7.2) fill that at roughly 50–70 cores **[I]**. Pro raises it to 3,500
   for $20–25 a month; Bunny DNS allows 5,000 for free **[V]**.
3. **Safari deletes a site's service worker, cache, IndexedDB and LocalStorage after seven days of
   Safari use without a visit** (WebKit, 2020) **[V]**. The device's locator and remembered paths
   go with it. A Safari user away for a week needs the service up to find their core again,
   and may need a new link **[I]**. §7.6's "returning device rides out an outage" holds for
   Chrome, Firefox and Home Screen apps, not for Safari tabs **[I]**.

**Recommendation for the closed beta** (no WebRTC, no payment, a few dozen households), about
**€15–25 a month** **[I]**:

- **Cloudflare Free in front of `app.`, `account.` and `api.`** (proxied). That gives unmetered
  L3/4/7 DDoS protection, WebSockets on every plan, and hides the origin **[V]**. The origin
  accepts HTTPS only from Cloudflare (a Hetzner firewall rule, or Cloudflare Tunnel with no open
  port). The core zone stays DNS-only.
- **The static client on Cloudflare Pages** (free, 20,000 files) or cached immutably at the edge
  **[V]**, so a first load works while the origin is down.
- **Rate limits in the application**, not at Cloudflare (Free allows one rule) **[V]**: per IP on
  magic links, claims and device codes; per core on address changes and probes.
- **One Hetzner VM; a second small VM at another provider** for the outside probe and as a warm
  spare, running `litestream restore -f` (follow mode) so its copy is about a second behind
  **[V]**. Promotion is manual: about 15–30 minutes **[I]**.
- **Target 99.5 % a month for the API** (about 3.6 hours down) **[I]**. Say so on the status page.
- **Watch the 200-record limit.** Move the zone to Bunny DNS (free) or Cloudflare Pro before about
  50 cores **[I]**.

**Recommendation for later** (WebRTC on, hundreds to thousands of households), about
**€50–90 a month** **[I]**, against $3,000 a month of revenue at 1,000 households:

- **The zone on an anycast provider that computes answers** (Bunny scriptable DNS), with long TTLs,
  since an answer never changes for a name. Self-hosted authoritative DNS only as a second
  provider, never alone **[I]**.
- **Signalling with automatic failover:** Cloudflare Load Balancing (from $5 a month) or a Tunnel
  started on the spare. The SQLite writer moves by promotion **[V price; I design]**.
- **STUN on both VMs, on their own addresses, never on the API's.** List both, plus
  `stun.cloudflare.com` ("free and unlimited") as a third **[V]**. Answer only Binding requests,
  rate-limit per source, keep the response small **[I]**.
- **No Spectrum.** Cloudflare's UDP proxying is an Enterprise add-on **[V]**; the price is
  "contact sales".

## 1. The attack surface, and what is realistic

### 1.1 Who attacks a small service, and how big

- Most attacks are small and short. In H1 2026, 96.62 % of network-layer attacks Cloudflare saw
  were under 500 Mbps, and 90.60 % ended within 10 minutes **[V]**. Cloudflare adds that "most
  Internet properties wouldn't be able to withstand even those small attacks" **[V]**.
- They are cheap. Europol's 2025 booter takedown named services that attacked "for as little as
  EUR 10" **[V]**.
- Victims who knew their attacker most often named competitors (63 % of the 29 % who knew), then
  states, then self-DDoS, extortionists and angry users, 5 % each (Cloudflare, 2025 Q2) **[V]**.
- DNS is the biggest vector now: DNS floods and amplification were 34.3 % of network-layer attacks
  in H1 2026 **[V]**.

For a $3 service of a few thousand households, the realistic threats are **[I]**: a bored or
angry user with a booter; a floods-for-fun attack on the STUN or DNS addresses, which are public
and listed; abuse of the free endpoints (magic links, device codes); and the service harming
itself — every core reconnecting at once after a restart.

### 1.2 By component

| Component | What an attacker can do | Realistic? | Protection |
| --- | --- | --- | --- |
| HTTPS API (`api.`, `account.`) | L7 floods; magic-link e-mail bombing (send links to someone else's address); claim-code and device-code guessing; forcing probes | **Yes** **[I]** | Cloudflare in front; per-IP, per-address and per-core limits in the app; codes long enough to make guessing hopeless |
| WebSocket signalling | open many sockets; slow handshakes; offer spam to a core | Medium **[I]** | Token checked before the upgrade; offers limited per locator and per core (protocol §2.5); core caps at 64 |
| The outside probe | a malicious core reports a victim's address as a candidate, so the service probes it hourly and on every change | **Yes, low volume** **[I]** | Cap candidates per core; probe once per change with backoff; only the given port; log and disable on abuse |
| STUN (UDP 3478) | reflection: 2.32:1 average (NETSCOUT, 2021); 3.98 (IPv4) and 5.85 (IPv6) average in Shadowserver's scans (2022) **[V]**; direct UDP floods on its address | **Yes** **[V]** | Separate address; Binding only; per-source rate limit; Hetzner's L3/4 filter |
| DNS, Cloudflare-hosted | floods | No: Cloudflare absorbs **[V]** | Nothing to do |
| DNS, self-hosted | random-subdomain ("water torture") floods; reflection (ANY, large TXT); plain floods on two small nodes | **Yes** **[V]** | Anycast provider; or RRL plus several nodes on several networks |
| Static client | bandwidth floods | Only if served from the origin **[I]** | CDN |

Notes:

- **STUN's amplification is the server's choice.** Amplified STUN packets are mostly 48 bytes,
  some up to 1,452 **[V]**. A server that answers Binding requests with only `XOR-MAPPED-ADDRESS`
  returns little more than it received **[I]**. NETSCOUT stresses that STUN floods are hard to
  filter without "unacceptable levels of overblocking" **[V]**. Shadowserver reports reachable STUN
  servers to their network owners **[V]**, so expect an abuse ticket from Hetzner one day **[I]**.
- **The core is a reflector risk too.** In 2021 about 37,000 Plex Media Servers were abused as
  amplifiers (4.68:1) because UPnP mapped their discovery port to the internet **[V]**. Orpheum's
  core maps ports by default (protocol §2.4). It must map only TCP 443 and never expose a UDP
  discovery responder **[I]**.
- **Hetzner's own filter can hurt.** On 25 February 2026 Hetzner mitigated a DDoS but "blocked too
  much traffic affecting UDP", and services not using UDP were affected too, for 1.5 hours **[V]**.
  STUN on the API's address would share that fate **[I]**.
- **Behind Cloudflare, the source address is Cloudflare's.** The protocol records a request's source
  address (protocol §2.4). Behind a proxy that must come from `CF-Connecting-IP`, trusted only from
  Cloudflare's ranges **[I]**.
- **Hiding the origin needs care.** Cloudflare's guide: proxy the records, allow only Cloudflare's
  addresses, keep mail and DNS-only records from naming the origin, and rotate the origin's IP
  after onboarding **[V]**. A STUN name on the API's IP gives the origin away **[I]**.

## 2. Protection available, and what it costs

### 2.1 Hetzner (the planned host)

- **Included, free, always on**, for all products: automatic detection and filtering at L3/4
  **[V]**. Hetzner names DNS reflection, NTP reflection, UDP floods, SYN floods and invalid packets
  as handled **[V]**. It uses Nokia Deepfield Defender with BGP Flowspec on its
  routers and scrubbing for harder attacks **[V]**.
- **No user controls, no L7.** A Hetzner tutorial says its own firewall-and-rate-limit advice "might
  help with some minor DDoS attacks" only **[V]**.
- **Null-routing:** Hetzner said in 2018 that it no longer null-routes since its 2016 system
  **[unverified]**; forum reports say long attacks still end in null routes **[unverified]**.
- **Incidents:** 25 Feb 2026, UDP overblocking, 1.5 h **[V]**; two network-wide outages in a week
  from DDoS in May 2022 **[V]** (HN thread).
- **SLA:** since March 2026, 99.9 % a month per Cloud Server, refunded in credit; load balancers,
  firewalls and other cloud services are excluded **[V]**.
- **Failover addresses:** a Floating IP (€3.00 a month IPv4, €1.00 IPv6) moves between servers in
  one network zone — Falkenstein, Nuremberg and Helsinki form `eu-central` **[V]**. It does not
  help when Hetzner's network itself is the problem **[I]**.

### 2.2 Cloudflare

- **Free plan:** "unmetered and unlimited" DDoS protection at layers 3, 4 and 7 on all plans
  **[V]**. WebSockets on all plans; the WAF sees only the upgrade request; Cloudflare's own
  releases restart servers and drop WebSockets, so clients need keep-alives and reconnects **[V]**.
  One rate-limiting rule, counting by IP over 10 seconds **[V]**. Pages: 20,000 files, 25 MiB each
  **[V]**. DNS: 200 records per zone for free zones created on or after 1 September 2024; 3,500 on
  Pro and Business **[V]**.
- **Pro:** $20 a month billed yearly, $25 monthly; Business $200/$250 **[V]**. Two rate-limit
  rules on Pro **[V]**.
- **Load Balancing:** from $5 a month, with health checks and failover **[V]**.
- **Only HTTP(S) is proxied** on ports 80, 443 and a few others. Anything else is DNS-only or
  Spectrum; "Spectrum for all TCP and UDP ports is only available on the Enterprise plan" **[V]**.
  Pro and Business get Spectrum only for one Minecraft or SSH (and on Business one RDP) app
  **[V]**. **So STUN cannot sit behind Cloudflare at any price a one-person business pays.**
- **Tunnel:** free; `cloudflared` keeps four outbound connections to at least two data centres;
  replicas on other hosts take over when one fails; traffic goes to the nearest replica, with no
  steering **[V]**. The origin then has no open port.
- **STUN:** `stun.cloudflare.com:3478` is "free and unlimited" **[V]**. TURN costs $0.05 per GB
  after 1,000 GB free **[V]** (not relevant: no relay, D273 §2).
- **Cloudflare is itself a dependency.** 18 Nov 2025: proxied sites returned 5xx from 11:20 to about
  14:30 UTC, fully recovered 17:06; a bad Bot Management file, first mistaken for a DDoS **[V]**. The
  impact list names the CDN, Turnstile, Workers KV, Access and dashboard log-in, not DNS **[V]**.
  5 Dec 2025: 25 minutes, 28 % of HTTP traffic **[V]**. 12 Jun 2025: Workers KV and products built
  on it, 2 h 28 min; "DNS, Cache, proxy, WAF" not directly affected **[V]**.

### 2.3 Others

- **Bunny:** CDN from $0.01/GB (Europe and North America), $1 a month minimum **[V]**. Bunny Shield's
  Basic tier "includes … full DDoS protection" **[V]**; its price was not on the page fetched
  **[unverified: reportedly free]**. **Bunny DNS:** anycast, dual-stack, L3/4 DDoS mitigation and
  per-IP DNS rate limiting; free for up to 500 zones, no query fees; 5,000 records per zone, raised
  on request **[V]**. **Scriptable DNS:** a JavaScript `handleQuery` receives the queried hostname,
  client IP and location and returns A, AAAA and other records **[V]**. That fits the address-in-name
  scheme (protocol §2.7) with no database **[I]**. Whether one script record can answer a whole
  wildcard subtree is not stated **[unverified]**.
- **OVHcloud:** every product gets its Anti-DDoS infrastructure ("1.3 Tbps" per attack, "17 Tbps"
  global) **[V]**. Game DDoS Protection, the UDP-aware layer, is for Bare Metal Game servers only,
  not VPS, and has profiles for games, not STUN **[V]**.
- **AWS Shield Standard:** free, automatic, L3/4, for CloudFront, Route 53, ELB and others **[V]**.
  Route 53 costs $0.50 per zone a month and $0.40 per million queries **[V]**, so a flood of
  queries is billed **[I]**.
- **Fastly:** 100 GB a month and 100,000 requests free; DDoS protection is a separate product
  **[V]**.

### 2.4 DNS: anycast provider or self-hosted

| | Cloudflare Free | Cloudflare Pro | Bunny DNS | Route 53 | Self-hosted (2–3 VMs) |
| --- | --- | --- | --- | --- | --- |
| Cost a month | €0 | $20–25 | $1 minimum | $0.50 + $0.40/M queries | €10–20 **[I]** |
| Records | 200 **[V]** | 3,500 **[V]** | 5,000, more on request **[V]** | 10,000 per zone **[V]** | unlimited |
| Computed answers (no records) | no | no | **yes, scripted** **[V]** | no | yes |
| Absorbs floods | yes **[V]** | yes **[V]** | yes **[V]** | yes, billed **[V]** | **no** **[I]** |

Self-hosting gives computed answers and independence. It loses the one thing the zone most needs:
staying up under a flood. Random-subdomain floods force the authoritative server to answer every
query, since no resolver can cache a random name **[V]**. Response Rate Limiting cuts reflection
but "cannot identify which source addresses are forged" **[V]**. A computed zone cannot be copied
to a secondary provider by zone transfer, because it has no finite record list **[I]**.

Two things soften a DNS outage **[I]**:

- **Long TTLs.** An address-in-name answer never changes, so it can carry a TTL of days. Resolvers
  cap it: Unbound at one day by default, BIND at one week **[V]**.
- **Serve-stale** (RFC 8767): resolvers may answer from expired cache when the authoritative servers
  are unreachable, capped at 7 days; it was written "to make the DNS more resilient to DoS attacks"
  **[V]**. How many public resolvers enable it was not checked **[unverified]**.

### 2.5 STUN: run our own, or use a public one

| | Own STUN | `stun.cloudflare.com` | Google's `stun.l.google.com:19302` |
| --- | --- | --- | --- |
| Cost | the VM | free, "unlimited" **[V]** | free |
| Promise | ours | no SLA found **[I]** | Google's answer: "no guarantee… deploy your own server for commercial use" **[V]** |
| Privacy | client IPs stay with us | Cloudflare sees them | Google sees them |
| DDoS | ours to absorb; a reflector risk | Cloudflare's | Google's |

Recommendation **[I]**: own STUN on both VMs (two networks), plus `stun.cloudflare.com` as a third
entry. ICE gathers from every listed server, so one failing costs nothing. If `app.` is already
proxied by Cloudflare, the client's address already reaches Cloudflare on every page load, so the
added privacy cost is small; it still belongs in the privacy policy. WebRTC experts advise
against relying on free public STUN for a commercial service (BlogGeek.me) **[V]**.

## 3. Availability

### 3.1 What a small service can target

- Hetzner promises 99.9 % a month per server: 43 minutes **[V]**. A single VM and a single person
  cannot beat its host; deploys, kernel updates and the owner's sleep cost more **[I]**.
- **Target 99.5 % for the API** (3.6 h a month), and close to 100 % for what Cloudflare or Bunny
  serves: the static client and, later, the DNS zone **[I]**. Plex, with a company behind it,
  posted 25 incidents between 25 May and 10 October 2026, plex.tv API and plex.direct among them;
  Tailscale posted 25 between 8 July and 8 October 2026, seven of them coordination-service ones
  **[V]** (each status feed returns its latest 25).

### 3.2 What needs availability most

| Part | Who needs it | How long an outage is tolerable | Today's design | Recommendation |
| --- | --- | --- | --- | --- |
| **The zone's DNS** | **every remote device, every time its resolver's cache expires** | **minutes to hours** | Cloudflare (beta), own servers (later) | Anycast provider always; long TTLs **[I]** |
| Static client | new devices; Safari users after 7 days; cleared caches | hours | served by the origin | CDN (Pages or Bunny) **[I]** |
| `paths` and locate (`api.`) | the same as the client, plus every device whose remembered paths all failed | hours | origin | Cloudflare in front; warm spare **[I]** |
| Signalling (later) | **WebRTC-only households, for every new session** | minutes | origin WebSocket | Cloudflare WebSockets; automatic failover **[I]** |
| STUN (later) | WebRTC set-up | minutes | origin, UDP | two own servers + Cloudflare's **[I]** |
| ACME TXT proxy | renewals | days to weeks | origin | as now; renew early, retry **[I]** |
| Address updates and probes | a core whose address changed | hours to days | origin | as now **[I]** |
| Claims, device codes, sign-in | new households and devices | hours | origin | as now **[I]** |

Two cases deserve care:

- **WebRTC-only households** (carrier NAT, no IPv6 at the client) need the service for every new
  connection. An existing data channel survives an outage; a new visit does not **[I]**. For them
  the service *is* on the path to playing, though not on playback itself.
- **Renewals** start at two-thirds of a certificate's life (§7.6). With 45-day certificates from
  2028 (the research **[V]**), that leaves about 15 days of tolerance, still far more than any
  outage **[I]**.

### 3.3 How to get there cheaply

- **Static client on a CDN.** Content-hashed, immutable paths already (§7.1). Cloudflare Pages costs
  nothing **[V]**. The page then loads during an origin outage, and a returning device's remembered
  paths take over **[I]**.
- **Warm spare.** Litestream replicates every second by default, asynchronously, so a crash loses
  up to about a second; restore replays the last snapshot plus every incremental file since, so
  more frequent snapshots make restores faster; `restore -f` follows the replica continuously
  **[V]**. A spare in follow mode on another provider can be promoted in minutes **[I]**.
- **Switching to the spare.** With Cloudflare in front, the switch is a change of origin (or
  starting `cloudflared` on the spare), not a DNS change users wait on **[I]**. Load Balancing ($5+)
  automates it **[V]**. Keep only one writer: SQLite has one **[I]**.
- **Backups on another provider** as well as Hetzner Object Storage, so one provider's outage never
  holds both the service and its backup **[I]**.
- **Reconnect storms.** Every core reconnecting after a restart or a Cloudflare release is a
  self-inflicted flood. Backoff with jitter on the core's WebSocket and heartbeat (protocol §2.5 says
  backoff; add jitter) **[I]**.
- **Safari's 7-day deletion.** Ask for persistent storage where browsers allow it, and tell Safari
  users to add the app to the Home Screen, which keeps its own counter **[V for the Home Screen
  rule; I for the advice]**. The core can also hand a device a fresh locator on each visit, so a
  device that reached its core directly is never stranded **[I]**.

## 4. What competitors do

| Product | Central part | When it is down | Published incidents |
| --- | --- | --- | --- |
| **Plex** | plex.tv (account, API), `plex.direct` DNS, Relay | plex.tv down: sign-in fails, and since even LAN access needs plex.tv by default, local playback can fail too **[V]** | 14 Jul 2026, plex.tv API, ~2 h, users could not reach their own servers **[V]**; `plex.direct` resolution 9 Jun (2.5 h), 27 Jul (3 h), 10 Sep (1 h), 2 Oct (3 h) 2026 **[V]**; Relay 4 Sep 2026 (2.5 h) **[V]** |
| **Tailscale** | sharded coordination service, DERP relays | "all your existing connections keep working"; new devices, key renewal, ACL changes and revocation stop **[V]** | 25 incidents 8 Jul–8 Oct 2026, seven of the coordination service **[V]**; an Amazon CloudFront failure took the admin console and package server down for 5 h on 16 Jul 2026 **[V]**; "nine periods of (partial) downtime" in one month, Jan 2026 **[V]** |
| **ZeroTier** | Central (control), four root servers | "If Central is unreachable, existing connections continue to function" **[V]** | no status feed found **[I]** |
| **Synology QuickConnect** | QuickConnect server, relay servers | tries LAN, WAN, hole punching, then relay through Synology **[unverified: white paper via search excerpt]** | forum reports only **[unverified]** |
| **Nabu Casa** | SNI proxy (TLS ends on the user's Home Assistant), on DigitalOcean in Frankfurt, New York and Singapore; DNS on Route 53 and Cloudflare; TURN from Cloudflare **[V]** | remote access stops; local access is unaffected **[I]** | 10 Jan 2026, European remote access partial outage, 1 h 46 min **[V]** |
| **Emby Connect** | finds servers and maps one login to many | "free optional"; local address and credentials still work **[V]** | none found **[I]** |
| **ngrok** | tunnels, edge, dashboard | endpoints unreachable | 14 Mar 2025 "Sites unavailable", 5 h **[V]**; 4 Oct 2025 endpoint connections, 4 h **[V]**; 2016 founder's note of a DDoS on its upstream **[unverified]** |
| **Cloudflare Tunnel** | Cloudflare's edge | everything behind it is down with Cloudflare (18 Nov 2025) **[V]** | see §2.2 |

What they show **[I]**:

- **Keep the control plane off the data path.** Tailscale, ZeroTier and Emby keep working when their
  centres fail. Plex's requirement that even LAN playback ask plex.tv is the counter-example, and
  the press calls it the reason users leave for Jellyfin **[V]**. Orpheum already signs household
  users in at the core (D214, protocol §1), so it sits with the first group.
- **The name service is the weak point of the Plex model.** Four `plex.direct` incidents in four
  months, on self-run name servers in two AWS regions (`ns*-plexdirect` resolve to us-east and
  eu-west EC2 addresses) **[V for the addresses; I for the regions]**. Nabu Casa and Tailscale put
  their zones on Route 53 **[V]**.
- **Cache the control state across restarts.** Tailscale's January post names one gap: a client
  that restarts forgets its network map; they were building a cache for it **[V]**. Orpheum's
  remembered paths are that cache; Safari's 7-day rule is where it leaks **[I]**.
- **Everyone depends on someone.** Tailscale on CloudFront, Nabu Casa on DigitalOcean and AWS,
  Plex on AWS and Cloudflare **[V]**. A one-person service depending on Cloudflare and Hetzner is
  in normal company **[I]**.

## Sources

Fetched 2026-10-13; copies in `/yoloai/cache/discovery-audit/`.

- Hetzner: [DDoS protection](https://www.hetzner.com/unternehmen/ddos-schutz/);
  [Technical and organisational measures](https://docs.hetzner.com/general/others/technical-and-organizational-measures);
  [Nokia Deepfield case study](https://www.nokia.com/customer-success/hetzner-strengthens-ddos-protection-across-european-data-centers-with-nokia-deepfield-defender/);
  [game-server DDoS tutorial](https://community.hetzner.com/tutorials/game-server-ddos-protection/);
  [status, 25 Feb 2026](https://status.hetzner.com/incident/5e901309-0873-42af-90c5-84f38304f592);
  [HN, May 2022 outages](https://news.ycombinator.com/item?id=31289676);
  [Cloud SLA](https://docs.hetzner.com/general/company-and-policy/slas-cloud/);
  [Floating IPs FAQ](https://docs.hetzner.com/cloud/floating-ips/faq/),
  [overview (prices)](https://docs.hetzner.com/cloud/floating-ips/overview/);
  [null-route tweet, 2018](https://x.com/Hetzner_Online/status/968085046073622528) (unverified).
- Cloudflare: [DDoS protection](https://developers.cloudflare.com/ddos-protection/about/);
  [WebSockets](https://developers.cloudflare.com/network/websockets/);
  [network ports](https://developers.cloudflare.com/fundamentals/reference/network-ports/);
  [Spectrum protocols per plan](https://developers.cloudflare.com/spectrum/protocols-per-plan/);
  [plans](https://www.cloudflare.com/plans/);
  [rate-limiting rules](https://developers.cloudflare.com/waf/rate-limiting-rules/);
  [DNS record quota](https://developers.cloudflare.com/dns/manage-dns-records/);
  [Pages limits](https://developers.cloudflare.com/pages/platform/limits/);
  [protect your origin](https://developers.cloudflare.com/fundamentals/security/protect-your-origin-server/);
  [Tunnel availability](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/configure-tunnels/tunnel-availability/);
  [TURN and STUN](https://developers.cloudflare.com/realtime/turn/),
  [FAQ](https://developers.cloudflare.com/realtime/turn/faq/);
  [outage 18 Nov 2025](https://blog.cloudflare.com/18-november-2025-outage/);
  [outage 5 Dec 2025](https://blog.cloudflare.com/5-december-2025-outage/);
  [outage 12 Jun 2025](https://blog.cloudflare.com/cloudflare-service-outage-june-12-2025/);
  [DDoS report H1 2026](https://blog.cloudflare.com/ddos-threat-report-2026-h1/);
  [DDoS report 2025 Q2](https://blog.cloudflare.com/ddos-threat-report-for-2025-q2/).
- Bunny: [DNS pricing](https://bunny.net/pricing/dns/); [CDN pricing](https://bunny.net/pricing/cdn/);
  [DNS overview](https://bunny.net/docs/dns/index.md); [DNS limits](https://bunny.net/docs/dns/limits.md);
  [scriptable DNS](https://bunny.net/docs/dns/scriptable/introduction);
  [Shield pricing](https://bunny.net/docs/shield/pricing.md).
- Others: [OVHcloud Anti-DDoS](https://us.ovhcloud.com/security/anti-ddos/);
  [OVHcloud Game DDoS Protection](https://www.ovhcloud.com/en/security/game-ddos-protection/);
  [AWS Shield pricing](https://aws.amazon.com/shield/pricing/);
  [Route 53 pricing](https://aws.amazon.com/route53/pricing/);
  [Fastly pricing](https://www.fastly.com/pricing).
- STUN: [NETSCOUT, STUN reflection](https://www.netscout.com/blog/asert/session-traversal-utilities-nat-stun);
  [Shadowserver, accessible STUN report](https://www.shadowserver.org/what-we-do/network-reporting/accessible-stun-service-report/);
  [discuss-webrtc on Google's STUN](https://groups.google.com/g/discuss-webrtc/c/shcPIaPxwo8);
  [BlogGeek.me, STUN](https://bloggeek.me/webrtcglossary/stun/);
  [NETSCOUT, Plex PMSSDP](https://www.netscout.com/blog/asert/plex-media-ssdp-pmssdp-reflectionamplification-ddos-attack).
- DNS: [Akamai, pseudo-random subdomain attacks](https://www.akamai.com/glossary/what-are-pseudo-random-subdomain-attacks);
  [ISC, Response Rate Limiting](https://kb.isc.org/docs/aa-01000);
  [RFC 8767, serve-stale](https://www.rfc-editor.org/rfc/rfc8767);
  [Unbound `cache-max-ttl`](https://unbound.docs.nlnetlabs.nl/en/latest/manpages/unbound.conf.html);
  [BIND `max-cache-ttl`](https://bind9.readthedocs.io/en/latest/reference.html);
  name-server lookups with `dig`, 2026-10-13 (`dig-2026-10-13.txt`; the sandbox intercepts DNS, so
  TTLs there are not the zones' own).
- Attackers: [Infosecurity Magazine, Europol booter takedown, May 2025](https://www.infosecurity-magazine.com/news/ddos-hire-network-dismantled/).
- Storage: [Litestream tips](https://litestream.io/tips/); [Litestream restore](https://litestream.io/reference/restore/);
  [WebKit, 7-day cap on script-writable storage](https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/).
- Plex: [status incidents feed](https://status.plex.tv/api/v2/incidents.json);
  [How-To Geek, 14 Jul 2026](https://www.howtogeek.com/your-plex-server-is-inaccessible-right-now-even-on-your-home-network/);
  [Relay limits](https://support.plex.tv/articles/216766168-accessing-a-server-through-relay/).
- Tailscale: [coordination server down](https://tailscale.com/kb/1091/what-happens-if-the-coordination-server-is-down);
  [Hypergrowth isn't always easy, Jan 2026](https://tailscale.com/blog/hypergrowth-isnt-always-easy);
  [status incidents feed](https://status.tailscale.com/api/v2/incidents.json).
- ZeroTier: [enterprise deployment](https://docs.zerotier.com/enterprise-deployment/);
  [protocol](https://docs.zerotier.com/protocol/).
- Synology: [QuickConnect white paper](https://kb.synology.com/en-sg/WP/Synology_QuickConnect_White_Paper/4)
  (read through a search excerpt; the page needs JavaScript).
- Nabu Casa: [status components and incidents](https://status.nabucasa.com/api/v2/summary.json);
  [SniTun](https://github.com/NabuCasa/snitun).
- Emby: [Emby Connect](https://github.com/EmbySupport/Emby.Docs/blob/master/Emby-Connect.md).
- ngrok: [status incidents feed](https://status.ngrok.com/api/v2/incidents.json).
