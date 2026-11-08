> **ABOUTME:** What a hosted relay for the paid discovery would cost: how often a connection
> between a client and a core needs one, how many bytes an average household would push through
> it in a worst case, what those bytes cost at each kind of provider, at 100, 1,000 and 10,000
> households, how to cut it, the risks, and the owner's open questions.

# Relay costs: what relaying the connections that fail would cost the paid discovery

- **Date:** 2026-11-06. Asked by the owner: *"This also ties into relay service in general. I may
  reconsider the 'no relay' rule for the paid discovery server, so I'd also like some research
  into what my costs would likely be considering the average household in a worst-case scenario
  for direct connections."* Video chat in Watch Together is researched separately; the owner wants
  its media off any paid service, so it appears here only as a line showing what it would add.
- **Scope:** reading and arithmetic. No code changed, nothing in production queried. Fetched
  pages and notes are in `/yoloai/cache/research-relay-costs/` (`notes.md` lists every figure
  with its URL).
- **Marks:** **[source]** a page read today, 2026-11-06, linked; **[repo]** a fact from this
  repository or `orpheum-ops`, with its path; **[unverified]** seen only in a search summary, a
  forum or a third-party aggregator; **[est]** my estimate, arithmetic shown. Unmarked text is
  analysis. Prices exclude VAT. 1 GB = 1,000 MB, 1 TB = 1,000 GB.

## TL;DR

- **How often a relay is needed is not known for us, and the beta is built to find out**
  ([D274](../../decisions/working-notes.md#d274--a-core-is-reached-first-by-what-needs-nothing-from-its-owner-webrtc-through-the-discovery-direct-paths-are-upgrades-and-a-relay-waits-for-demand) §3,
  [D314](../../decisions/working-notes.md#d314--a-core-is-installed-from-a-public-image-on-any-machine-that-runs-docker-with-the-hardware-it-needs-and-with-no-tunnel-a-connection-that-needs-a-relay-fails-and-is-counted) §4).
  Published WebRTC figures put TURN at **15–25% of consumer sessions**, from practitioners and
  vendors; no primary, current measurement from Google, Meta, Twilio, Jitsi or Cloudflare was
  found. Our case is kinder than a call between two phones (the core sits on a home router, maps
  its own ports, and races IPv6), so **I estimate 5–20% of a household's remote bytes, 10% as
  the central figure [est]**. A minority of households (a core on fixed-wireless 5G or a locked
  ISP router with no usable IPv6) would relay **most** of their remote viewing.
- **Traffic per household, all of it relayed (the worst case):** low **13.5 GB/month**, typical
  **108 GB**, heavy **1,134 GB** (§2). A mixed population of half low, two fifths typical and a
  tenth heavy averages **163 GB per household**; at 10% relayed, **16 GB**.
- **What that costs per household per month (mixed population, §3.4):**

  | Provider kind | Everything relayed | 10% relayed |
  | --- | --- | --- |
  | Hetzner Cloud, our own TURN, 20 TB included per €5.99 server | **€0.05–0.12** | **€0.01–0.12** (the floor of two servers dominates) |
  | Cloudflare Realtime TURN ($0.05/GB, 1 TB a month free) | **$7.67–8.16** | **$0.32–0.81** |
  | AWS egress (tiered from $0.09/GB) | **$8.55–14.39** | **$1.20–1.44** |
  | Twilio TURN ($0.40/GB) | **$65.34** | **$6.53** |

  Against the paid tier's planned **$3 a month**
  ([D273](../../decisions/working-notes.md#d273--clients-reach-a-core-through-apporpheum-appcom-or-the-owners-own-discovery-by-whatever-methods-work-with-no-traffic-relayed-by-us-a-node-without-landlock-runs-and-warns) §3):
  **a relay run on included-traffic hosting costs cents per household even in the worst case;
  any metered relay (Cloudflare, the hyperscalers, managed TURN) costs more than the
  subscription in the worst case**, and Cloudflare's 10% case is a quarter of it.
- **The bytes are not the cost; operating it is.** Abuse handling, the conduit's legal duties
  ([legal.md](legal.md) §4), monitoring and on-call outweigh €60 of servers.
- **The cheapest cuts:** play a relayed connection from the 720p rung (`3000k@720p`), which takes
  the mixed average from 163 to 57 GB (−65%); keep upgrading to a direct path after the relay
  answers; publish a per-household allowance.
- **Recommendation (§6):** keep the beta's count running until it has a denominator and a cause
  per failure; if the owner then builds a relay, build it as TURN with per-session credentials
  issued by the discovery, run on Hetzner Cloud (EU) with included traffic, relayed playback
  capped at the 720p rung, a stated monthly allowance, and Cloudflare's TURN only as a bootstrap
  while the beta is small (its free terabyte covers about 60 households at 10% relayed).

---

## 1. How often a relay is needed

### 1.1 What the project already established

- **Only two situations make a relay unavoidable for a UDP-capable client**: both ends behind
  hard (endpoint-dependent-mapping, "symmetric") NAT, or UDP blocked at either end
  ([D68](../../decisions/working-notes.md#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay))
  [repo].
- **Since D274 every client reaches the core by WebRTC through the discovery**, so browsers now
  punch too; [direct-reachability.md](direct-reachability.md)'s point that "hole-punching figures
  do not apply to browsers" was written before that and no longer holds for the web client [repo].
- **Wherever a third party is needed, the household's own VPS is enough**
  ([direct-reachability.md](direct-reachability.md) §7); a hosted relay is the answer only for
  owners who will not run one
  ([D214](../../decisions/working-notes.md#d214--a-later-commercial-service-connects-clients-to-a-core-without-carrying-content-the-hosted-relay-stays-deferred) §2) [repo].
- **The core's embedded TURN (`internal/core/turn`, pion/turn) relays Watch Together voice**, and
  serves TCP beside UDP [repo, `internal/core/turn/doc.go`]. It helps only when the core itself
  is reachable: a core behind carrier NAT cannot be its own members' relay. D68's "the media
  server is already the relay" holds for a reachable core and fails exactly in the case this
  report is about.

### 1.2 Published shares of WebRTC sessions that relay

| Source | Figure | Standing |
| --- | --- | --- |
| callstats.io, 2015–16, via webrtcHacks | *"22% of the conferences need some kind of TURN relay"*, 9% needed TCP | [repo: direct-reachability.md §4] |
| Tsahi Levent-Levi, 2018, discuss-webrtc | *"Most large vendors … report 20% TURN relay traffic. Some reported over 30%"* | [repo] |
| Tsahi Levent-Levi, [bloggeek.me/webrtc-turn](https://bloggeek.me/webrtc-turn/), updated 2025-11-10 | *"around 80% of all connections can be resolved by either using the local IP address or by use of STUN"*; in a reply, *"I've seen anything between 0-50% of TURN relay"* | [source] |
| Philipp Hancke, 2017, Chrome-era data | about 12% TURN/UDP, 5% TURN/TCP, under 0.5% TURN/TLS | [unverified: search summary; the Medium page refused the fetch] |
| ZeroTier | *"anywhere from 4% (our numbers) to 8% (an older number from Google)"* | [repo] |
| Tailscale, 2025-10-15 | *"well north of 90%"* direct; two phones on different carriers *"have a good chance"* of needing DERP | [repo] |
| libp2p/IPFS, 4.4 M attempts ([arXiv 2604.12484](https://arxiv.org/abs/2604.12484)) | 70% ± 7.1% hole-punch success once relay and address discovery worked | [repo] |
| Vendors' blogs, 2024–2026 | 15–25% for consumer WebRTC, 25–35% for mobile-heavy apps, 60–70% in corporate networks | [unverified: unsourced vendor figures] |

**No current primary figure was found** from Google (Meet, Chrome UMA), Meta, Twilio, Jitsi or
Cloudflare. Chrome records the selected pair's type in UMA (`IceCandidatePairType` in WebRTC's
`api/uma_metrics.h`) but publishes no share [unverified: search summary].

### 1.3 Why our share should be lower than a call's

A video call puts two arbitrary consumer devices on each end. Ours always has a core on one end,
and the core's end is the better one:

- **The core sits behind a home router**, whose NAT is usually endpoint-independent:
  Syncthing's 308,160 devices were 13.9% symmetric, 55.5% port-restricted cone and 0.9% not
  behind NAT [repo: direct-reachability.md §4, measured 2026-09-25]. A symmetric client and a
  cone core punch through unless the core's NAT filters by port too; that last case is what
  fails.
- **The core maps its own ports** by PCP, NAT-PMP and UPnP IGD, and opens its IPv6 addresses by
  UPnP IGD 2 (`internal/core/portmap`, D273 §7) [repo]. A mapped port accepts a symmetric
  client's packets, so a successful mapping removes the hard case. How often mapping works is
  old data: UPnP returned the router's external address in 40% of Netalyzr sessions in 2016, and
  carrier NATs offer no PCP [repo: direct-reachability.md §1.3].
- **IPv6 removes translation**: punching between two IPv6 ends almost always works (D274 §4).
  Google saw IPv6 on 46–47% of weekday and about 50% of weekend traffic in September 2026, 75% in
  Germany, 86% in France, 13% in Spain [repo: direct-reachability.md §2.1]. Both ends need it.
- **Mobile networks are the client side's weak spot**: over 92% of cellular networks ran carrier
  NAT in 2016, about 40% of them symmetric (Richter et al., IMC 2016) [repo]; but most large
  mobile carriers give phones IPv6 (T-Mobile US, Verizon, AT&T, Jio, Telstra, EE), and the UK's
  O2, Vodafone and Three do not [repo: direct-reachability.md §2.2]. A phone on a UK carrier
  reaching a core with no IPv6 is the textbook relay case.
- **Firewalled networks** (hotels, offices, schools) block UDP for some users: 5% of Google's
  QUIC connections had UDP impaired in 2016 [repo]. These need TURN over TCP or TLS on 443.

**The core's side is the worse problem when it fails, because it fails for every client.** A core
behind carrier NAT with no usable IPv6 inbound (T-Mobile Home Internet, a Starlink router not in
bypass mode, a locked ISP router; rows R3 and R4 of direct-reachability.md §6) can still punch to
clients whose NAT is cone-like, but every symmetric or UDP-blocked client fails. The client's
side, when it fails, fails for one person on one network, and the dead-end page's advice (try
mobile data, or Wi-Fi) often fixes it (discovery-protocol.md §3.6).

### 1.4 The estimate [est]

- **Per remote session:** about **5–15%** need a relay. Inputs: WebRTC's 15–25% for two arbitrary
  ends, lowered for a cone-NAT core that maps its ports and races IPv6, raised for TV browsers and
  UK mobile clients.
- **Per byte, which is what costs:** about the same, **10% as the central figure, 5–20% as the
  range**, with one caveat: relayed bytes concentrate in a few households. A core in the R3 row
  would relay perhaps **30–100%** of its remote viewing (every mobile client without IPv6, every
  hotel). For the fleet's cost the average is what matters; for one household's allowance the
  tail is.
- **This is the number the beta's count exists to replace** (§1.5).

### 1.5 What the dead-end telemetry tells us, and what it does not

**What is recorded [repo]:**

- When a page finds no path, it reports a dead end to its discovery, which keeps a weekly count
  per core with no device identifier (D293 §7). Connect stores, for each report, whether the
  page's network has IPv6 (`yes`/`no`/`unknown`), the cause the page told its person (`noIpv6`, or
  none), the ways it tried as `kind:ended` (`webrtc:failed`, `ipv6:unreachable`, …), and the
  family the report itself came over (`orpheum-connect/internal/clients/offers.go`, `deadEnd`;
  D398 §7). The core reads `deadEndsLastWeek` and `deadEndCausesLastWeek` with its heartbeat's
  answer and the System page shows them (discovery-protocol.md §2.6).
- The core counts every connection's path since it started: `direct_ipv6`, `direct_ipv4`,
  `punched_ipv6`, `punched_ipv4`, `hairpinned_ipv4`, `relayed`, `failed`
  (`internal/core/rtcapi/path.go`, `PathCounts`; D274 §3).

**What that can answer:** how many failures, per household, per week; whether the failing
networks lacked IPv6; which paths were attempted. Read across households, it says whether
failures cluster on a few cores (the R3 row: a relay for those households, or the advice to fix
the line) or spread thinly over clients (hotels and UK mobile: a relay for everyone).

**What it cannot answer yet:**

1. **The denominator.** The discovery sees failures, not successes; the core's `PathCounts` stay
   on the core. A failure rate needs both. Sending the core's path counts (seven integers a week)
   in its heartbeat would give the fleet's share of punched, direct, relayed and failed
   connections.
2. **Whether a relay would have saved it.** A failure from a symmetric NAT is saved by UDP TURN;
   one from blocked UDP only by TURN over TCP/TLS 443; one from an allowlist network by nothing.
   The client already gets two STUN servers, the core's and the discovery's (D406), so it can
   compare its two server-reflexive ports and report "mapping changes per destination" (yes/no),
   and whether any UDP answered at all.
3. **Bytes.** Relay cost is bytes, not sessions. How long the people who failed would have
   watched is unknowable; a proxy is the remote viewing hours of households that never fail.
4. **People who never report.** A person who gives up before the page decides (or whose page
   never loads) is not counted.

**Numbers recorded so far:** none in the repository's documents. The handoff notes record that
D398's stored causes shipped to Connect on 2026-11-03; no week of production counts is written
down, and this report did not query production. With two households on prod (the production NAS and the
owner's own), the count says little until the beta grows.

## 2. Traffic per household

### 2.1 The bitrates

The core cuts a film's lower qualities at `3000k@720p, 2000k@540p, 1200k@480p, 700k@432p,
400k@360p` in software, with `6000k@1080p` above them where the node encodes on a GPU, average
rates with peaks capped at 1.5× (`chunk_qualities`, `chunk_qualities_hardware`,
`internal/core/settings/registry.go`; D247 §6, D323 §3) [repo]. The top of the ladder is the
film's own picture copied, which for a 1080p web release is typically 5–15 Mb/s and for a
Blu-ray remux 20–40 Mb/s [est]. A remote viewer gets at most what the household's upload allows
(`server_upload_mbps`, `upload_films_share_percent`).

**Bytes per hour** = rate × 3,600 s ÷ 8 bits ÷ 1,000 → **0.45 GB per hour per Mb/s**:

| Rate | GB per hour |
| --- | --- |
| 3 Mb/s (720p rung) | 3 × 0.45 = **1.35** |
| 6 Mb/s (1080p rung) | 6 × 0.45 = **2.70** |
| 12 Mb/s (a typical copied 1080p film) | 12 × 0.45 = **5.40** |
| Voice, Opus, ~50 kb/s | 0.05 × 0.45 = 0.0225 |

TURN, DTLS and SCTP framing add a few per cent [est: ~5%]; it is left out of the tables below.

### 2.2 How much a household watches remotely

No published figure covers remote viewing from a home media server. For context only: US
households averaged **3.5 hours a day on Netflix** in June 2026 (Comscore via
[eMarketer](https://www.emarketer.com/content/households-giving-netflix-more-time-than-any-other-streamer))
and internet households **43.5 hours of video a week** on all devices (Parks Associates)
[unverified: search summaries]. Most of a household's viewing is at home, on the LAN, and never
touches a relay. D72's own sketch assumed **60 remote hours a month** [repo].

**Profiles [est]** (a *stream-hour* is one device playing for one hour; a Watch Together night
with N remote members is N streams, since each member's player fetches its own chunks
([watch-together.md](../plans/watch-together.md)) [repo]):

| Profile | Who | Solo remote hours | Watch Together | Stream-hours | Rate |
| --- | --- | --- | --- | --- | --- |
| **Low** | the owner away now and then | 10 | none | **10** | 3 Mb/s |
| **Typical** | one remote member, or a partner who travels | 32 | 2 nights × 2 h × 2 remote members = 8 | **40** | 6 Mb/s |
| **Heavy** | family in two other homes watching most evenings, a weekly group | 150 | 8 nights × 2.5 h × 3 remote members = 60 | **210** | 12 Mb/s |

### 2.3 GB per month, everything relayed (the worst case)

| Profile | Arithmetic | GB/month |
| --- | --- | --- |
| Low | 10 h × 1.35 GB | **13.5** |
| Typical | 40 h × 2.70 GB | **108** |
| Heavy | 210 h × 5.40 GB | **1,134** |
| **Mixed population** (50% low, 40% typical, 10% heavy) | 0.5 × 13.5 + 0.4 × 108 + 0.1 × 1,134 = 6.75 + 43.2 + 113.4 | **163.35** |
| Mixed, 10% relayed | 163.35 × 0.10 | **16.3** |

**What video chat would add if relayed** (for the separate video-chat research, not counted
above): a mesh of N people sends N × (N − 1) streams; at 1.5 Mb/s each, three people are 6 × 1.5 =
9 Mb/s (4.05 GB/h) and four are 12 × 1.5 = 18 Mb/s (8.1 GB/h). The typical profile's 4 hours with
three people adds 4 × 4.05 = **16.2 GB**, the heavy profile's 20 hours with four adds 20 × 8.1 =
**162 GB**. Voice is negligible: four people's 12 streams × 50 kb/s = 0.6 Mb/s, 0.27 GB/h.

### 2.4 Peak rate, which sizes self-run servers

The mixed population's 163.35 GB a month averages 163.35 × 10⁹ × 8 ÷ 2,592,000 s = **0.50 Mb/s
per household**. Viewing bunches into the evening; with a peak four times the mean [est], a
household's peak share is **2.0 Mb/s** fully relayed, **0.2 Mb/s** at 10%.

| Households | Peak, all relayed | Peak, 10% relayed |
| --- | --- | --- |
| 100 | 0.2 Gb/s | 0.02 Gb/s |
| 1,000 | 2.0 Gb/s | 0.2 Gb/s |
| 10,000 | 20 Gb/s | 2.0 Gb/s |

## 3. What relaying costs

### 3.1 Where the project is hosted now

The task mentioned Scaleway; the project is on **Hetzner** [repo: `orpheum-ops/machines.md`]:

- `prod-api`, the production discovery (`connectd`), a Hetzner Cloud VPS in Nuremberg behind a
  Cloudflare Tunnel, inbound TCP 22 only (`orpheum-ops/production/README.md` §2.1, "about €4–8 a
  month").
- The **edge VPS**, `edge-ubuntu-1gb-nbg1-1`, which forwards `media.orpheum-app.com` to the production NAS
  over WireGuard and carries staging's STUN. It already relays the owner's own household's
  content: it is a household's own relay in D214 §2's sense, not a service's.

### 3.2 Price per GB, verified today

| Provider | Price | Included / free | Notes | Standing |
| --- | --- | --- | --- | --- |
| **Hetzner Cloud** (EU) | CX23 €5.99/month (2 vCPU, 4 GB); overage **€1.00/TB** | **at least 20 TB/month** per server in the EU | US: 1–5 TB, Singapore 0.5–5 TB; plans shown "currently unavailable" today | [source: [cost-optimized](https://www.hetzner.com/cloud/cost-optimized/), [traffic](https://docs.hetzner.com/robot/general/traffic/)] |
| **Hetzner dedicated** | AX41 €59/month, no setup | **unlimited** on 1 Gbit/s: *"There is no bandwidth limitation"* | not for the 10G add-on | [source: [AX matrix](https://www.hetzner.com/dedicated-rootserver/matrix-ax/)] |
| **OVHcloud VPS** | VPS-1 $4.54/month, 500 Mbit/s | *"Unlimited traffic"* (EU, NA) | a 2015 contract let OVH cut excessive use to 1 Mbit/s; Asia-Pacific plans 1–4 TB then 10 Mbit/s | [source: [VPS](https://www.ovhcloud.com/en/vps/)]; caps [unverified] |
| **Scaleway** Instances | hourly per type, IPv4 billed apart | no per-GB internet egress named in its docs; speed capped per type (DEV1-S 200 Mbit/s, PLAY2-PICO 100, PRO2-XXS 350) | scaleway.com refused automated fetches; read from its docs' source on GitHub | [source: [docs-content](https://github.com/scaleway/docs-content/blob/main/pages/instances/reference-content/instances-bandwidth-overview.mdx)]; "unmetered" [unverified] |
| **DigitalOcean** | $4/month droplet | 500 GiB outbound | overage $0.01/GiB | included [source: [droplets](https://www.digitalocean.com/pricing/droplets)]; overage [unverified] |
| **Vultr** | overage **$0.01/GB** | per plan | inbound unmetered | [unverified: [Vultr docs](https://docs.vultr.com/support/platform/billing/what-is-the-bandwidth-overage-rate) via search] |
| **Akamai (Linode)** | overage **$0.005/GB** | 1–20 TB pooled | | [unverified: third party] |
| **Fly.io** | **$0.02/GB** NA and EU; $0.04 APAC, Oceania, SA; $0.12 Africa, India | none named | | [source: [pricing](https://docs.fly.io/about/pricing)] |
| **AWS** | **$0.09/GB** first 10 TB, $0.085 next 40, $0.07 next 100, $0.05 over 150 TB | **100 GB/month** | | free tier [source: [EC2](https://aws.amazon.com/ec2/pricing/on-demand/)]; rates [unverified: third parties agree] |
| **Google Cloud** | Premium **$0.12/GB** first TB, $0.11 to 10 TB, $0.08 above; Standard ~$0.085 | ~1 GiB | | [unverified: third parties] |
| **Azure** | **$0.087/GB** first 10 TB (Zone 1), $0.083 next 40, $0.07 next 100, $0.05 above | 100 GB/month | | [unverified: third parties] |
| **Cloudflare Realtime TURN** | **$0.05/GB** egress | **first 1,000 GB a month free**, shared with its SFU; free entirely when used with the SFU | per allocation 50–100 Mbit/s and 5–10 kpps before drops; TLS on 5349 or **443**; anycast; not in China | [source: [TURN](https://developers.cloudflare.com/realtime/turn/), [pricing](https://developers.cloudflare.com/realtime/pricing/)] |
| **Twilio NTS** | **$0.40/GB** US and Germany; $0.60 Singapore, Mumbai, Tokyo; $0.80 Sydney, São Paulo | none | | [source: [pricing](https://www.twilio.com/en-us/stun-turn/pricing)] |
| **Metered** | $99/150 GB (+$0.40), $199/500 GB (+$0.20), $499/2 TB (+$0.10) | as plan | | [source: [metered.ca](https://www.metered.ca/stun-turn)] |
| **Xirsys** | $39/50 GB (+$0.50), $99/150 GB (+$0.40), $199/500 GB (+$0.20), $499/2 TB (+$0.09) | as plan | free tier stops at 500 MB | [source: [xirsys.com](https://xirsys.com/pricing/)] |

**Software.** coturn claims *"thousands simultaneous calls per CPU"* and publishes no Gb/s per
core [source: [coturn](https://github.com/coturn/coturn/)]. pion/turn is already in our
dependencies and embedded in the core. eturnal was not examined. Relaying a few hundred Mb/s of
large UDP packets is within one small VPS's processor; the network port is the limit [est].

### 3.3 Per household per month, by profile

Cost = GB × price; free allowances ignored here (they are fleet-wide, §3.4).

**Everything relayed:**

| Price per GB | Low (13.5 GB) | Typical (108 GB) | Heavy (1,134 GB) |
| --- | --- | --- | --- |
| Hetzner overage €0.001 | €0.01 | €0.11 | €1.13 |
| Linode $0.005 | $0.07 | $0.54 | $5.67 |
| Vultr / DigitalOcean overage $0.01 | $0.14 | $1.08 | $11.34 |
| Fly.io $0.02 | $0.27 | $2.16 | $22.68 |
| Cloudflare TURN $0.05 | $0.68 | $5.40 | $56.70 |
| Azure $0.087 | $1.17 | $9.40 | $98.66 |
| AWS $0.09 | $1.22 | $9.72 | $102.06 |
| GCP Premium $0.12 | $1.62 | $12.96 | $136.08 |
| Twilio $0.40 | $5.40 | $43.20 | $453.60 |

**10% relayed:** divide every cell by ten (Cloudflare: $0.07, $0.54, $5.67; Twilio: $0.54, $4.32,
$45.36).

### 3.4 For the fleet: 100, 1,000 and 10,000 households

Mixed population (§2.3): 163.35 GB per household fully relayed, 16.335 GB at 10%.

| Households | TB/month, all relayed | TB/month, 10% |
| --- | --- | --- |
| 100 | 16.3 | 1.63 |
| 1,000 | 163.4 | 16.3 |
| 10,000 | 1,633.5 | 163.4 |

**Cloudflare TURN** (first 1,000 GB free, then $0.05):

| Households | All relayed | per household | 10% relayed | per household |
| --- | --- | --- | --- | --- |
| 100 | (16,335 − 1,000) × 0.05 = $766.75 | $7.67 | (1,633.5 − 1,000) × 0.05 = $31.68 | $0.32 |
| 1,000 | (163,350 − 1,000) × 0.05 = $8,117.50 | $8.12 | (16,335 − 1,000) × 0.05 = $766.75 | $0.77 |
| 10,000 | (1,633,500 − 1,000) × 0.05 = $81,625 | $8.16 | (163,350 − 1,000) × 0.05 = $8,117.50 | $0.81 |

**AWS** (tiers of §3.2; the 100 GB free counted only at 100 households, 10%):

| Households | All relayed | per household | 10% relayed | per household |
| --- | --- | --- | --- | --- |
| 100 | 10,000 × 0.09 + 6,335 × 0.085 = $1,438.48 | $14.38 | (1,633.5 − 100) × 0.09 = $138.02 | $1.38 |
| 1,000 | 900 + 40,000 × 0.085 + 100,000 × 0.07 + 13,350 × 0.05 = 900 + 3,400 + 7,000 + 667.50 = $11,967.50 | $11.97 | $1,438.48 (as 100, all relayed) | $1.44 |
| 10,000 | 900 + 3,400 + 7,000 + 1,483,500 × 0.05 = 11,300 + 74,175 = $85,475 | $8.55 | $11,967.50 | $1.20 |

**Twilio** ($0.40 flat): 163.35 × 0.40 = **$65.34** per household all relayed, **$6.53** at 10%,
at any size ($653,400 a month at 10,000 households, all relayed).

**Self-run TURN on Hetzner Cloud CX23** (€5.99, 20 TB included each, overage €1/TB). Servers =
TB ÷ 20 rounded up, plus one spare, at least two. A 20 TB server averages 20 × 10¹² × 8 ÷
2,592,000 = 61.7 Mb/s and peaks near 250 Mb/s at a peak of four times the mean; whether a CX23's
port sustains that is not stated by Hetzner [unverified; measure before relying on it].

| Households | All relayed | per household | 10% relayed | per household |
| --- | --- | --- | --- | --- |
| 100 | 16.3 TB → 1, floor 2: 2 × €5.99 = €11.98 | €0.12 | 2 → €11.98 | €0.12 |
| 1,000 | 163.4 TB → 9 + 1 = 10: €59.90 | €0.06 | 16.3 TB → 1, floor 2: €11.98 | €0.012 |
| 10,000 | 1,633.5 TB → 82 + 8 spares = 90: €539.10 | €0.054 | 163.4 TB → 10: €59.90 | €0.006 |

**Self-run on dedicated Hetzner AX41** (€59, 1 Gbit/s, unlimited). Usable 0.7 Gb/s each [est];
boxes = peak ÷ 0.7 rounded up, plus one spare, at least two. 10,000 households all relayed: 20 ÷
0.7 = 29 + 3 spares = 32 × €59 = **€1,888 (€0.19 per household)**; 1,000 households all relayed:
2.0 ÷ 0.7 = 3 + 1 = 4 × €59 = €236 (€0.24); at 10%: 2 boxes, €118. Per terabyte the cloud's
included traffic is cheaper (€5.99 ÷ 20 TB = €0.30/TB, against €59 ÷ (0.7 Gb/s ÷ 4 peak factor ×
2,592,000 s ÷ 8 = 56.7 TB) = €1.04/TB); dedicated wins on a guaranteed port and on not running
ninety machines.

**Regions add a floor.** Hetzner Cloud's 20 TB is EU only; its US servers include 1–5 TB. A US
relay pair would be OVH's unmetered VPS (2 × $4.54) or Vultr at $0.01/GB. So a three-region
footprint (EU, US East, US West) starts near **€30 a month** before any traffic [est].

### 3.5 What these numbers mean against $3 a month

- **Included-traffic hosting (Hetzner, OVH):** cents per household even in the worst case. The
  bytes are not a reason to refuse a relay.
- **Cloudflare TURN:** at 10% relayed it takes $0.32–0.81 of $3 (11–27%); fully relayed it is
  $8, nearly three subscriptions. Its free terabyte covers 1,000 ÷ 16.3 ≈ **61 households** at 10%,
  or 1,000 ÷ 163 ≈ 6 fully relayed. Cheap to start, wrong to grow on.
- **Hyperscalers and managed TURN:** above the subscription in the worst case, and Twilio above
  it even at 10%. Excluded.
- **One heavy household relaying everything** costs $56.70 at Cloudflare and about €1.13 on
  Hetzner overage. Without a stated allowance, the metered options are a free tier whose cost
  scales with use: the thing D72 forbids.

## 4. Ways to cut it

| Way | Effect | Cost and limits |
| --- | --- | --- |
| **Relayed playback starts at the 720p rung** (`3000k@720p`) | mixed average 163.35 → 0.5 × 13.5 + 0.4 × (40 × 1.35) + 0.1 × (210 × 1.35) = 6.75 + 21.6 + 28.35 = **56.7 GB (−65%)**; heavy 1,134 → 283.5 GB | a visible quality drop for the relayed few; the core already lowers players by its upload budget (`upload_films_share_percent`), so this is one more bound, told to the viewer rather than hidden (D72: never throttle silently) |
| **Relay first, keep upgrading** | ICE keeps checking; a direct pair found later carries the rest (D68's "relay first, upgrade later") | only bytes before the upgrade are relayed; needs the client to re-race paths during playback |
| **Relay only control and voice** | voice is 0.27 GB/h for four people | does not let a blocked member watch: solves only part of the problem |
| **Only one side allocates** | TURN's relayed bytes are the stream once whichever side allocates; with both sides on the same relay and both allocating, a provider may count them twice | the client allocates (it is the side behind a hotel firewall or TCP-only); the core allocates only when the client cannot reach a relayed address [est] |
| **A friend's core as relay** (D71's "the relay is whatever mutually reachable node already exists") | costs us nothing | spends the friend's upload; puts the friend in the conduit position; needs trust and consent between households; hard to make dependable |
| **IPv6 first** (D274 §4) | removes the core's NAT; the System page already says when a core has none | both ends need it; UK mobile, Spain and Italy lag |
| **Port mapping** (PCP, NAT-PMP, UPnP; built) | makes the core reachable by symmetric clients | 40% UPnP success in 2016 data; no PCP on carrier NAT |
| **TURN over TLS on 443** | the only path through firewalls passing just HTTPS | some TCP overhead; needs a certificate on the relay's name; Cloudflare offers 443 |
| **Regional placement** | near the core, per D71: the core-to-relay leg carries everything | one region per market; each adds a floor of two servers |
| **Per-household allowance** | caps the tail | e.g. 100 GB a month included (at Cloudflare's price, $5; on Hetzner, €0.10), stated, with a warning near it (D72) |
| **A paid add-on** | the relay's buyers pay for it | splits the product; the people who need it most (a core on carrier NAT) learn of it only by failing |
| **Fix the line instead** | many CGNAT lines have a free opt-out (Free, Orange, most of Australia) or a cheap one (£4–8 in UK altnets) (direct-reachability.md §1.3) | the dead-end System page can say which applies |

## 5. Risks

### 5.1 Abuse

An open TURN server forwards arbitrary UDP and TCP to arbitrary addresses: a proxy for attacks,
scanning and evasion. Hetzner's cloud agreement forbids *"the scanning of foreign networks or
foreign IP addresses"* and *"the use of fake source IPs"* [source:
[cloud-server](https://www.hetzner.com/legal/cloud-server/)], so a customer's misuse of our relay
becomes our account's problem. The protocol already plans the defences (discovery-protocol.md
§1, §2.4): credentials per session issued by the discovery and short-lived (a quarter of an
hour, as the core's relay already does), peers limited to the core's own candidates, no relaying
to private ranges or the relay's own networks (the core's relay enforces this:
`TestRelay_ServesTCPAndRelaysOnlyToThePublicInternet`). Payment is the abuse control (D72 §4).

### 5.2 Legal and terms of service

- **The content is encrypted end to end, and the claim holds for the relay path.** A client's
  data channels run DTLS from browser to core; the core signs its answer and the client checks
  the answer's single sha-256 fingerprint against the key it remembers, so a party that only
  relays signalling cannot terminate DTLS (discovery-protocol.md §2.5, A7, A9) [repo]. A TURN
  relay sees only DTLS records, and Watch Together voice is SRTP keyed by DTLS between the
  members. **The caveat:** the service that serves the web client's JavaScript is trusted by
  every household that uses it (discovery-protocol.md §3.7, RFC 8826 §4.3). app.orpheum-app.com
  serves it, so "we cannot see the film" is true of the relay and of the signalling, and holds for
  the service only as long as the JavaScript it serves is honest. A relay adds no new exposure
  beyond that one.
- **Conduit status.** In the US, 17 U.S.C. § 512(a) shields a provider of *"transitory digital
  network communications"* when someone else starts the transmission, it is carried by an
  automatic process, the provider selects no recipients, keeps no copy accessible to others
  longer than needed, and modifies nothing; § 512(i)(1)(A) still requires a published, enforced
  repeat-infringer policy [source: [law.cornell.edu](https://www.law.cornell.edu/uscode/text/17/512)].
  A TURN relay fits § 512(a) as written. The EU's equivalent is the Digital Services Act's
  Article 4, *mere conduit* (EUR-Lex returned an empty page; the article's conditions mirror
  § 512(a): not initiating, not selecting the receiver, not selecting or modifying the
  information) [unverified: not read today]. This is a reading of the statutes, not legal advice;
  D214 §2 already names the duties (legal.md §4) and asks for a lawyer.
- **Hosting providers.** Copyright complaints reach a hosting provider through its abuse desk;
  Hetzner works notice-and-take-down through abuse.hetzner.com [source:
  [DSA page](https://www.hetzner.com/legal/digital-services-act) via search]; a 24-hour answer is
  expected according to a forum [unverified]. Monitoring firms find infringement by joining
  public swarms and listing public URLs; an encrypted, credentialed relay offers neither, so
  complaints are unlikely, though not impossible [est].
- **Cloudflare.** Its CDN terms require a paid service such as Stream *"to serve video and other
  large files via the CDN"* [source: [service-specific terms](https://www.cloudflare.com/service-specific-terms-application-services/)].
  The terms name nothing for Realtime or TURN, which are paid per byte; whether relaying films
  through its TURN is welcome is unstated. Ask before relying on it.

### 5.3 Privacy

A relay sees both ends' addresses, when they connect, for how long, and how many bytes: from
which a viewing pattern can be read, though not what was watched. Keep only aggregate counts and
short-lived logs (D214 §3), as the dead-end count already does.

### 5.4 Operations

Relays are cattle (D72 §1): no data, stateless, replaceable. Still: a fleet to patch and watch,
credentials to rotate, capacity to plan for the evening peak, and abuse tickets to answer within
a day. At 100 households this is the owner's own time, and costs more than the servers.

## 6. Recommendation

1. **Do not decide on today's data.** The beta's dead-end count is built; add its denominator
   (the core's weekly path counts in the heartbeat) and whether the failing client's NAT maps per
   destination or blocks UDP (§1.5). A month of that across the beta says whether failures sit on
   a few cores or spread over clients, and how many a relay would save.
2. **If failures cluster on a few cores,** tell those households what fixes their line (the
   System page already names a public IPv4 from the ISP, or their own VPS); a relay is a product
   for the handful who cannot.
3. **If a relay is built,** build the shape D274 §2 left open: TURN (pion/turn, already ours),
   credentials per session from the discovery, peers limited to the core's candidates, UDP plus
   TLS on 443. Host it on **Hetzner Cloud in the EU** (included traffic), one pair per region
   served, OVH or Vultr for the US. Play relayed connections from the **720p rung**, keep trying
   to upgrade, and **state an allowance** (100 GB a month is generous at the 720p rung: 74 hours).
4. **Cloudflare TURN as a bootstrap only**, while the beta is under about 60 households, after
   asking Cloudflare whether relaying films suits its terms.
5. **Keep video chat off it** as the owner wants: a relayed four-person video mesh is 8.1 GB an
   hour, more than a 1080p film.

## 7. Open questions for the owner

1. **Who is the relay for**: every household whose connection fails, or only those whose own line
   is the cause and who cannot fix it? The answer decides between "included" and "add-on".
2. **Included in the $3, or sold apart?** On Hetzner the worst case costs cents; the price is the
   support and the legal duties, not the bytes.
3. **The allowance**: a monthly figure per household (100 GB?), and what happens past it (720p
   only, then stop, said on screen)?
4. **Relayed quality**: is the 720p rung acceptable for a relayed viewer, or should a heavy
   household be able to pay for more?
5. **The legal position**: a lawyer on § 512(a) / DSA Article 4 for an encrypted relay, a
   repeat-infringer policy, and whether a DMCA agent is needed for a pure conduit (legal.md §4).
6. **Regions**: EU only at first, or the US from the start (Hetzner's US servers include little
   traffic, so another provider)?
7. **The denominator**: may the core send its weekly path counts (seven integers, no addresses)
   in its heartbeat, so the beta measures a rate and not only a count?
8. **Cloudflare**: worth asking it whether relaying video through Realtime TURN is within its
   terms, before using its free terabyte?

## 8. Gaps

- No current primary measurement of WebRTC relay share (§1.2); every recent figure is a vendor's.
- No current per-country share of home lines behind carrier NAT; Cloudflare's 2025 study labels
  over 200,000 CGNAT addresses but gives no share
  ([The Register, 2025-11-03](https://www.theregister.com/2025/11/03/cloudflare_cgnat_bias_research/)) [source].
- No measurement of how often PCP, NAT-PMP or UPnP succeed today; the core's own mapping results
  across the beta would be the first.
- No figure for remote viewing from home media servers; §2.2's profiles are assumptions.
- Hetzner Cloud's per-server sustained throughput, Scaleway's egress terms and OVH's current
  fair-use terms were not confirmed from the providers' own pages.

## Addendum (2026-11-06): with a forced low bitrate, at 100, 1,000 and 10,000 households

The owner, 2026-11-06: "I'd want to do whatever we can to keep costs down should we implement this.
The question is whether this is tenable at 100 households, 1000 households, 10000 households... I'm
fine with a forced reduced bandwidth like what plex does, but I worry that if we allow this at scale,
we'll be forced to raise prices to plex's level."

A cap on the relayed bitrate bounds the cost per household, whatever the household does. At a cap
of 2 Mb/s per relayed stream (0.9 GB an hour; Plex's relay has long been capped at about 1–2 Mb/s
[lead's recollection, unverified this session]) and 4 Mb/s per household in all:

- **The worst single household**, relaying at its whole cap all month: 4 Mb/s × 2,592,000 s ÷ 8 =
  1,296 GB, €1.30 at Hetzner Cloud's €1/TB overage, $65 at Cloudflare's $0.05/GB.
- **The mixed population (§2), all relayed, capped:** low 10 h × 0.9 = 9 GB, typical 40 × 0.9 = 36 GB,
  heavy 210 × 0.9 = 189 GB; 0.5 × 9 + 0.4 × 36 + 0.1 × 189 = **37.8 GB a month**, against 163.35
  uncapped (−77%). At 10% relayed, 3.8 GB.

| Households | Capped, all relayed | Hetzner Cloud (20 TB a server, + 10% spares, floor 2) | per household | Cloudflare TURN | per household |
| --- | --- | --- | --- | --- | --- |
| 100 | 3.8 TB | 2 × €5.99 = €11.98 | €0.12 | (3,780 − 1,000) × 0.05 = $139 | $1.39 |
| 1,000 | 37.8 TB | 2 + 1 = 3 × €5.99 = €17.97 | €0.018 | (37,800 − 1,000) × 0.05 = $1,840 | $1.84 |
| 10,000 | 378 TB | 19 + 2 = 21 × €5.99 = €125.79 | €0.013 | (378,000 − 1,000) × 0.05 = $18,850 | $1.89 |

**Reading:** with relays the project runs on included-traffic hosting, the bytes cost cents per
household at every scale, and even the worst household stays near €1; bandwidth would not force the
price up. A metered relay (Cloudflare, the hyperscalers, Twilio) costs about $1.90 per household
fully relayed even capped, most of a $3 price, so it suits only a bootstrap. What does grow with
scale is the work: abuse handling, the conduit's legal duties (§5), and watching a fleet (21 small
servers at 10,000 households, or 8 dedicated AX41s: a peak of 4 × 37.8 GB × 8 ÷ 2,592,000 s = 0.47
Mb/s per household, 4.7 Gb/s in all, ÷ 0.7 = 7 + 1 spare = €472, €0.05 per household). Unverified
and to measure before relying on it: a Hetzner Cloud server's sustained throughput, and its
included 20 TB staying as it is.
