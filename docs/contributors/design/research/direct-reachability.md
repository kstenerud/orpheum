> **ABOUTME:** When can a client really not reach a home server directly, given that the server is on
> residential broadband? Separates impossible from inconvenient from "the user does something once".

# Direct reachability: when is a hosted relay actually necessary?

The owner asked for this study: *"What are the actual situations where it's simply impossible to get
a direct connection from the client to the core? Remember that the core side will almost always be
residential Internet, not a locked down hotel connection."* The hosted relay
([D71](../../decisions/working-notes.md#d71--one-relay-artefact-three-jobs-and-the-core-dials-out-to-it),
[D72](../../decisions/working-notes.md#d72--the-hosted-relay-has-no-free-tier-holds-no-data-and-has-no-accounts))
is our largest legal and operating exposure ([legal.md](legal.md) §4). The question is how often it
is necessary, as opposed to merely convenient. [remote-access.md](remote-access.md) (2026-09-18/19)
is the background and is not repeated here. Date 2026-09-25.

**[source]** marks a published page (linked). **[measured]** marks something queried live today.
**[est]** marks an estimate of mine, with its arithmetic shown. **[unverified]** marks a claim seen
only in a search snippet, a forum or a secondary source. Anything unmarked is analysis. §1–§5 are
fact. §6 onwards is analysis.

Local copies: `/yoloai/cache/http/*.summary.md`. The Syncthing query output is in
`/yoloai/cache/http/mon.syncthing.net_relay_nat_queries_2026-09-25.txt`. No code was read or run.
The web-search budget ran out partway through the server-side research, so the gaps listed in §9
are real gaps, not gaps nobody tried to fill.

---

## TL;DR

- **Hole-punching statistics do not apply to most of our clients.** Tailscale's ">90% direct"
  figure is about apps that can send raw UDP. A browser playing HLS, a smart-TV browser and a Cast
  receiver can only open ordinary outbound connections. For them, "direct" means one thing: **the
  server accepts an inbound TCP connection at an address the client can reach.** So the
  question comes down to two facts: does the server have inbound IPv4, and does the client have
  IPv6?
- **Direct access is impossible without a third party in exactly one topology:** the server
  has no usable inbound IPv4 **and** (the client has no working IPv6 **or** the server's inbound
  IPv6 is blocked by a router the user cannot configure). Every other combination can be reached
  directly, sometimes after a one-time step.
- **Some third party is always enough, and the user's own VPS is always one.** The core can always
  dial out on TCP 443. A €0–6/month VPS with a public IPv4 then serves every client type, including
  TVs and Cast receivers. **No combination of networks makes a hosted relay the only technical
  answer.** A hosted relay is the only answer when the owner won't run a VPS. That is a product
  question, not a physics one.
- **Servers without inbound IPv4 are common and growing.** In 2016, CGNAT was on 17–18% of
  non-cellular eyeball networks, counted by network rather than by subscriber [source]. The share
  is higher now in Germany, France, the UK altnets, Australia, India, and on all fixed-wireless and
  Starlink lines. But **almost all of these lines have native IPv6.** DS-Lite, MAP-E and Starlink
  have it by design. The lines with no IPv6 at all (Virgin Media, TalkTalk, TPG, Odido) mostly
  still give out a public IPv4.
- **The client is the weak side.** Google sees IPv6 on only **46–47% of weekday traffic** and
  about **50% at weekends**. The weekend lift is the office networks dropping out. Germany is at
  75% and France at 86%, but Spain is at 13% and Italy at 18%. Big UK mobile networks (O2,
  Vodafone, Three) have no IPv6. Vodafone Germany's Android APN defaults to IPv4-only. Hotel,
  corporate and in-flight networks are unmeasured, but the evidence points to IPv4-only.
  **TVs are the worst case:** a Jellyfin Cast receiver failed against an IPv6-only server, and
  only some Roku models are dual-stack.
- **Many servers can be fixed with one user action.** Free and Orange (FR) give a full public IPv4
  for free. Most CGNAT ISPs in Australia offer a free opt-out. UK altnets charge £4–8/month.
  Vodafone Germany cable grants dual-stack only sometimes, as goodwill. Others have **no fix
  at all**: Deutsche Glasfaser private lines, T-Mobile Home Internet, Starlink Residential, Jio,
  and Japanese MAP-E (no ports 80/443).
- **Estimate [est]:** about **5–20%** of remote viewing situations need a third party before any
  user action. About **2–10%** still need one after the one-time fixes (ISP opt-out, FRITZ!Box
  IPv6 port sharing). For every one of those, a user-run VPS is enough.
- **The owner's own line is the textbook case.** DS-Lite plus a FRITZ!Box can open inbound IPv6
  per device, so remote clients with IPv6 connect directly. The remote person's TV on an IPv4-only
  network, a hotel, an office, or UK mobile data will all need the VPS tunnel. **Plan the VPS for
  M4 rather than treating it as the last resort.**
- **Suggestion (§8):** IPv6-first, plus a documented one-command own-VPS tunnel using the
  existing `relay` mode run by the user (D71's software, the user's machine), plus a reachability
  check that tells the user which one-time step applies. **Defer the hosted relay indefinitely.**
  §8 lists what would have to become true to revisit that.

---

## 1. The server side: residential lines

### 1.1 How many home lines lack a public IPv4

**No current per-country share of households behind CGNAT exists.** Netalyzr shut down around
2019 and nothing replaced it. The measurements that do exist:

- **Richter et al., IMC 2016** ([arXiv 1605.05606](https://arxiv.org/abs/1605.05606)) [source]:
  *"13.3% of all non-cellular ASes use CGNs. However, the penetration jumps to 17–18% when
  considering only non-cellular eyeball ASes. In cellular networks the use is over 92%."*
  - Deployment is *"particularly high … in Asia and Europe"*.
  - Some ISPs multiplex *"up to 128 subscribers per public IP address"*.
  - 11% of fixed-line CGNs use symmetric mapping.
  - These are counts of ASes, not of subscribers.
- **Revelio, 2017** ([arXiv 1704.01296](https://arxiv.org/abs/1704.01296)): 6 of 64 fixed-line ISPs
  in the EU and US had some CGN [unverified, snippet].
- **Hsu, Pearce, Li, Berger, Richter, 2026** ([arXiv 2608.06517](https://arxiv.org/abs/2608.06517)):
  *"rates of massively shared traffic are increasing over time"* [source]. The paper has no
  residential-only figure.

Per country and ISP. Most of these come from ISP help pages or forums, so treat each as marked:

| Country | No public IPv4 by default | Public IPv4 by default |
| --- | --- | --- |
| **DE** | Vodafone cable: *"an Neukunden nur noch DS-Lite-Anschlüsse"* ([helpdesk wiki](https://helpdesk.vodafonekabelforum.de/wiki/Internet_Protocol), unofficial). 1&1 DSL/FTTH DS-Lite [unverified]. O2 cable DS-Lite [unverified]. Deutsche Glasfaser dual-stack with CGNAT IPv4 in 100.64/10 ([forum](https://www.glasfaserforum.de/forum/thread/736-deutsche-glasfaser-und-oeffentliche-ipv4-wan-adresse/)). Telekom on third-party "Regio" fibre ([Telekom hilft](https://telekomhilft.telekom.de/conversations/festnetz-internet/dynamische-%C3%B6ffentliche-ipv4-f%C3%BCr-glasfaser-privatkunden/67eb91a1a0fa136e02886aca), staff) | Telekom own lines, dual-stack [unverified, community]. O2 DSL/fibre [unverified] |
| **FR** | Free (4rd, *"4 voisins se partagent une seule adresse IPv4"*, [Clubic 2016](https://www.clubic.com/connexion-internet/fai-free-box-freebox/actualite-808988-freebox-ipv4-dediee-4rd.html)). SFR, Bouygues. Orange since January 2025 ([MacGeneration](https://www.macg.co/ailleurs/2025/01/orange-partage-son-tour-par-defaut-les-ipv4-pour-les-abonnes-adsl-et-fibre-148513)) | — (all four share by default, but see §1.3) |
| **UK** | Community Fibre for new or renewed customers since 2026-06-04 ([help page](https://help.communityfibre.co.uk/troubleshooting/ip-address/ip-address-what-are-the-different-types-offered-by-community-fibre-on-the-different-speed-packages)). brsk, toob and YouFibre ([BroadbandSwitch, 2026-08-16](https://broadbandswitch.uk/insights/cgnat-by-provider-uk/)) | Virgin Media (public dynamic IPv4, but no IPv6) [unverified]. BT, Sky and TalkTalk *"publish no residential CGNAT or public IP information"* (same source) |
| **US** | T-Mobile Home Internet and Verizon 5G Home (fixed wireless) [unverified] | Comcast, AT&T, Charter [unverified, general consensus] |
| **NL** | Ziggo DS-Lite [unverified]. DELTA [unverified] | KPN [unverified]. Odido (*"doesn't use CGNAT"*, [community](https://community.odido.nl/bekabeld-internet-492/cgnat-issue-on-fiber-internet-383378)) |
| **JP** | MAP-E (v6plus and the like): 240 fixed ports in 4096–65535, **so 80 and 443 cannot be forwarded** ([isecj](https://isecj.jp/tools/portcheck-map-e/), [tabikumo](https://tabikumo.com/v6plus-nat/), secondary). transix DS-Lite: no inbound at all without the fixed-IP option ([mfeed](https://www.mfeed.ad.jp/transix/dslite/)) | PPPoE-based plans [unverified] |
| **AU** | AGL, Aussie Broadband, Dodo, Superloop, Exetel and others, CGNAT on by default ([WhistleOut, 2026-01-29](https://www.whistleout.com.au/Broadband/Guides/what-is-CGNAT-and-how-to-opt-out)) | Telstra, Optus, TPG, iiNet (same source) |
| **IN** | Jio Fiber and Airtel Xstream IPv4 [unverified, forums] | — |
| **Satellite and FWA** | Starlink Residential: 100.64/10, *"blocks all inbound ports"* ([Starlink help](https://starlink.com/support/article/1192f3ef-2a17-31d9-261a-a59d215629f4)). T-Mobile Home Internet. Vodafone GigaCube and mobile hotspots: no data, but >92% of cellular networks run CGN (Richter) | Starlink Priority plans only |

### 1.2 IPv6 on those same lines

APNIC's per-ISP IPv6 capability, 2026-09-24 ([stats.labs.apnic.net/ipv6](https://stats.labs.apnic.net/ipv6/)) [source]:

- **DE:** Telekom 81.9%, Vodafone 71.9%, Telefónica 88.2%, 1&1 Versatel 88.6%, Deutsche Glasfaser 86.3%.
- **FR:** Orange 92.1%, Free 92.8%, Bouygues 89.5%, SFR 80.3%.
- **UK:** BT 89.7%, Sky 93.5%. **Virgin Media 0.27%, TalkTalk 0.29%, Plusnet 0.38%.**
- **US:** Comcast 85.1%, AT&T 85.3%, T-Mobile 94.5%, **Starlink 87.8%**.
- **AU:** Telstra 82.1%, **TPG 12.6%**, Aussie Broadband 28.9%.
- **NL:** KPN 83.8%, VodafoneZiggo 69.3%, **Odido 0.3%**.
- **JP:** KDDI 89.6%, J:COM 21.8%.
- **IN:** Jio 96.5%, Airtel broadband 76.9%.

These figures mix fixed and mobile users within one AS. The pattern still holds: **the ISPs that
took IPv4 away are the ones with IPv6.** DS-Lite and MAP-E cannot work without IPv6. The ISPs with
no IPv6 are, as far as the sources show, the ones that still give out public IPv4.

### 1.3 Whether the user can fix it once

**Getting a public IPv4:**

- **Free (FR):** free and self-service. Its help page says it *"vous donne accès à l'ensemble des
  ports"* (gives you access to all ports) ([assistance.free.fr/articles/1758](https://assistance.free.fr/articles/1758)) [source].
- **Orange (FR):** free, a checkbox in the Livebox (MacGeneration, above) [source].
- **Australia:** a free CGNAT opt-out at most of the RSPs above. A static IP is typically A$5/month (WhistleOut) [source].
- **UK altnets:** a public IPv4 for £4 (Community Fibre) to £8 (toob) a month. Zen includes one (BroadbandSwitch) [source].
- **Vodafone cable (DE):** dual-stack is granted as goodwill on request, especially on gigabit
  tariffs. It is *"vertraglich nicht garantiert"* (not contractually guaranteed) and reported as
  no longer possible for some people ([Vodafone forum](https://forum.vodafone.de/t5/Kabel-Tarife-Rechnung/Umstellung-von-DS-Lite-auf-Dual-Stack/m-p/2863789)) [unverified]. **For Vodafone DSL, which is the owner's line, no data was found.**
- **1&1:** dual-stack on request [unverified].
- **No fix available:**
  - **Deutsche Glasfaser private lines** (*"Bei uns nicht"*, 2020). A third party tunnels a fixed IPv4 for €11.90/month, capped at about 100 Mbit/s ([EDV Kossmann](https://www.edv-kossmann.de/festeip/)).
  - **T-Mobile Home Internet.**
  - **Starlink Residential.** A public IP is *"available to Priority, Mobile Priority, and Maritime customers"* (Starlink help) [source].
  - **Jio** [unverified].
  - **Japanese MAP-E** without a fixed-IP option.

**Port-mapping protocols (UPnP, NAT-PMP, PCP):**

- **FRITZ!Box:** UPnP port changes are a per-device permission ("Selbstständige Portfreigaben"),
  off by default [unverified, forum].
- **Netalyzr, 2016:** UPnP returned the router's external address in only 40% of sessions (Richter).
- **PCP on carrier NATs:** effectively undeployed. Vendors such as MikroTik lack it
  ([Swer 2021](https://www.daryllswer.com/shortcomings-of-cgnat-and-potential-work-arounds/)). Juniper
  supports it for DS-Lite, but **no ISP was found to enable it** [source].

**Inbound IPv6, which is the fix that matters on DS-Lite:**

| Router | Unsolicited inbound IPv6 | Configurable? |
| --- | --- | --- |
| **FRITZ!Box** | blocked | **Yes.** Per-device "IPv6-Freigaben" ([AVM KB 845](https://avm.de/service/wissensdatenbank/dok/FRITZ-Box-3390/845_IPv6-Freigaben-in-FRITZ-Box-einrichten/)) [unverified, JS page read via snippet]. MyFRITZ! gives each shared device a name that follows the prefix ([heise](https://www.heise.de/ratgeber/IPv6-Freigaben-mit-Namensdienst-auf-Fritzboxen-nutzen-6234026.html)) |
| **BT Smart Hub** | blocked | Yes, via "pinholes", with reported bugs ([BT community](https://community.bt.com/t5/Home-setup-Wi-Fi-network/Smart-Hub-Bug-IPv6-Pinholes/td-p/1749600)) [unverified] |
| **Vodafone Station** | blocked | Reports conflict: *"Die IPv6 Firewall der Vodafone Station ist leider nicht konfigurierbar"* (the IPv6 firewall cannot be configured). Bridge mode plus your own router is the usual fix ([forum](https://forum.vodafone.de/t5/Ger%C3%A4te/Vodafone-Station-macht-nur-Probleme-mit-IPv6-und-Firewall/td-p/3100398)) [unverified] |
| **Comcast xFi** | ? | *"xFi does not currently allow you to set up port forward for devices that have an IPv6 address"* ([forum](https://forums.xfinity.com/conversations/internet/answered-how-to-set-up-port-forwarding-on-your-xfinity-gateway/602da6cdc5375f08cdd20bc2)) [unverified]. This matters little, because Comcast gives a public IPv4 |
| **T-Mobile Home Internet** | **blocked** | **No.** *"T-Mobile blocks all unsolicited inbound traffic for IPv6"* (community snippet) [unverified] |
| **Starlink router** | blocked | No. Bypass mode plus your own router receives the /56 ([cellstream](https://www.cellstream.com/2025/05/06/does-starlink-support-ipv6/), [packetville](https://www.packetville.net/docs/starlink/starlink-ipv6-self-hosting/)) [unverified] |

**Prefix stability.** RIPE-690 says *"non-persistent prefixes are considered harmful in IPv6"*, and
that users running services *"typically need to manually configure the addresses of those, so using
non-persistent prefixes is not an option"* ([RIPE-690](https://www.ripe.net/publications/docs/ripe-690/)) [source].

- **Telekom:** a new prefix on every new session. The daily reconnect is a FRITZ!Box default, not
  an ISP one [unverified].
- **Starlink:** keeps the prefix across reboots, but can change it on relocation [unverified].
- **Vodafone:** no data.

A changing prefix means dynamic DNS for the AAAA record: MyFRITZ!, or deSEC, which can update the
prefix alone ([deSEC docs](https://desec.readthedocs.io/en/latest/dyndns/update-api.html)) [source].

### 1.4 Port blocking and "no servers" clauses

- **Comcast** blocks 25, 135–139, 445, 1900 and a few more. **It does not block 80 or 443**
  ([Xfinity](https://www.xfinity.com/support/articles/list-of-blocked-ports)) [source]. Its AUP both
  forbids *"servers … that provide network content or any other services to anyone outside of your
  Premises"* and permits equipment for *"personal and non-commercial residential use"*
  ([AUP](https://www.xfinity.com/corporate/customers/policies/highspeedinternetaup)) [source].
  The two clauses sit uneasily together.
- **Verizon Fios and Cox** reportedly forbid servers. Cox reportedly blocks inbound 80
  [unverified]. Enforcement is reported as aimed at abuse [unverified].
- **No EU example was found.**

**Double NAT** (the ISP's router plus the user's own router): no prevalence data was found. It is
fixed once, by bridge or bypass mode or by forwarding on both routers. It turns into CGNAT only
when the outer router's WAN side is itself shared.

## 2. The client side

### 2.1 How many clients have IPv6

**Google.** From its raw data file ([adoption.js](https://www.google.com/intl/en_ALL/ipv6/statistics/data/adoption.js)) [measured]:

- The global share first passed 50% on **Saturday 2026-03-28 (50.10%)**.
- **Weekdays are still below 50%.** In the week of 2026-09-14: Monday 45.87%, Wednesday 46.02%,
  Saturday 50.47%. From June to September, weekdays averaged 47.1% and weekends 49.7%.
- RIPE Labs reads the Saturday-to-Monday gap as *"home users appearing to have an edge over
  business users"* ([RIPE Labs](https://labs.ripe.net/author/wilhelm/ipv6-10-years-out-an-analysis-in-users-tables-and-traffic/)) [source].

**Per country** (Google as of 2026-09-20, and APNIC's 30-day average to 2026-09-24):

| | FR | DE | IN | JP | US | UK | NL | AU | IT | ES | CN |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Google | 86.3 | 75.4 | 68.5 | 56.9 | 55.5 | 52.4 | 52.0 | 37.9 | 18.1 | 13.2 | 8.1 |
| APNIC | 85.2 | 73.2 | 78.2 | 61.3 | 61.4 | 54.3 | 52.1 | 46.9 | 18.7 | 10.5 | 51.0 |

- APNIC's world figure is **43.8%** capable [source].
- Cloudflare Radar's 2025 review found 29% of requests from dual-stack clients went over IPv6
  ([blog](https://blog.cloudflare.com/radar-2025-year-in-review/)). The Register quotes Radar at 40.1%
  for 2026 [source]. The two figures measure different things and are not reconciled here.

### 2.2 Mobile data

**Largely IPv6, often IPv6-only with 464XLAT:**

- T-Mobile US: 92% IPv6 in 2022 ([World IPv6 Launch](https://www.worldipv6launch.org/apps/ipv6week/measurement/timeline-nets.html)).
- Verizon Wireless: 84%. AT&T Wireless: 85%. Jio: 93%, and Jio 5G is IPv6-only.
- Telstra: IPv6-only since 2020 ([SIDN](https://www.sidn.nl/en/news-and-blogs/australias-telstra-switches-mobile-users-to-ipv6-only)).
- EE: 464XLAT since 2018.
- Telekom DE: dual-stack since 2015 [source].

**No IPv6, or off by default:**

- **Vodafone DE:** dual-stack, but *the Android default APN is IPv4* ([thomas--schaefer.de](https://www.thomas--schaefer.de/apn-ipv4-ipv6.html)) [unverified, well-kept private page].
- **O2 DE:** dual-stack, but no NAT64 and no IPv6 on 2G (same source).
- **UK O2, Vodafone UK, Three and the MVNOs:** no IPv6 as of 2025–2026 ([calum.org](https://calum.org/posts/uk-mobile-phone-networks-supporting-ipv6)) [unverified, community].

**Carrier NAT mapping:**

- Richter found cellular CGNs about *"40% symmetric"*, and *"major cellular networks in the US
  deploying CGNs with symmetric mapping types"* [source].
- Tailscale: two phones on different carriers *"have a good chance"* of needing DERP, its relay
  ([2025-10-15](https://tailscale.com/blog/nat-traversal-improvements-pt-1)) [source].
- This matters only for UDP peer-to-peer (§4). A phone on mobile data opening HTTPS to a public
  address is unaffected.

### 2.3 Hotels, cafés, airports and aircraft

- **No measurement of IPv6 on public Wi-Fi was found**, and none for in-flight Wi-Fi.
- Only anecdote for outbound restrictions: *"only allow port 80 and 443"* policies on captive
  portals ([NANOG](https://diswww.mit.edu/charon/nanog/195315)), and a hotel blocking WireGuard
  handshakes on every port ([Arch forum](https://bbs.archlinux.org/viewtopic.php?id=281038)) [unverified].
- Treat these networks as IPv4-only, TCP 443 only, and UDP unreliable. That is an assumption, not a
  measurement.

### 2.4 Corporate and school networks

- **UDP:** in Google's QUIC paper (2016 data), 5% of connections had UDP impaired, 4.5% of them
  blocked close to the user ([SIGCOMM 2017](https://dl.acm.org/doi/10.1145/3098822.3098842)) [source].
  Zoom and Teams both keep a TCP 443 fallback for this reason [source].
- **Category filtering is the corporate trap that matters for us.** Palo Alto's best practice lists
  **dynamic-dns** and **newly-registered-domain** (under 32 days old) among the categories to block
  ([Palo Alto](https://docs.paloaltonetworks.com/advanced-url-filtering/administration/configuring-url-filtering/url-filtering-best-practices)) [source].
  A home server on a DDNS name or a fresh domain is likely to be refused on such a network whatever
  its IP family.
- **Enterprise IPv6:** only vendor-blog estimates of 20–30% [unverified]. Some universities have
  none (*"There is no IPv6 support at the present time"*, [Cambridge eduroam](https://help.uis.cam.ac.uk/service/wi-fi/it-staff/eduroam-technical)) [source].
- **Explicit proxies and TLS inspection:** prevalence not found.

### 2.5 TVs and Cast receivers

- **Chromecast:** a Jellyfin Cast receiver issue reports *"When trying to connect to an IPv6-only
  server, the connection fails"*. It was closed as not planned
  ([jellyfin-chromecast#404](https://github.com/jellyfin/jellyfin-chromecast/issues/404)) [source]. It is
  unclear whether the fault is the device's or the app's.
- **Roku:** lists the specific models that are dual-stack-capable
  ([Roku, 2026-06-19](https://support.roku.com/article/21332729601687)) [source].
- **Fire TV:** a 2017 report says it needed IPv4 alongside IPv6 [unverified].
- **LG webOS:** IPv6 is a user toggle [unverified].
- **Samsung Tizen:** no data.
- **Apple:** App Store apps must work on IPv6-only networks ([Apple](https://developer.apple.com/support/ipv6)).
  That this covers tvOS is inferred.
- **The TV is also on the remote person's home network.** If that network is Virgin Media,
  TalkTalk, TPG, Odido, a router with IPv6 switched off (TP-Link's guide says to *"turn on the IPv6
  toggle"* [source]) or a building's shared Wi-Fi, the TV has no IPv6 whatever the device supports.

### 2.6 Filtering countries

- **China:** the GFW lets TLS through when it looks like TLS
  ([USENIX Security 2023](https://gfw.report/publications/usenixsecurity23/en/)). It blocked all TCP 443
  to the outside for 74 minutes in August 2025 [source].
- **Russia:** since 2025 it throttles Cloudflare-hosted sites, and reportedly Hetzner, DigitalOcean
  and OVH too. Regional mobile "whitelist" modes allow only approved services
  ([The Record](https://therecord.media/cloudflare-russia-restricting-access-crackdown)) [source].
- **Iran:** a 53-day-plus blackout in 2026, then a tiered whitelist
  ([Rest of World](https://restofworld.org/2026/iran-blackout-tiered-internet/)) [source].
- **UAE:** blocks unlicensed VoIP [unverified]. Whether WebRTC voice is affected is unknown.
- **No relay of any kind helps against an allowlist.** A hosted relay on a hosting range is
  throttled exactly like a user's VPS on one.

## 3. What each client can reach

| Client network | (a) IPv4 :443 | (b) IPv6 :443 | (c) other TCP port | (d) UDP (TURN, WebRTC) |
| --- | --- | --- | --- | --- |
| Home broadband, IPv6 | yes | yes | yes | yes |
| Home broadband, IPv4-only | yes | **no** | yes | yes |
| Mobile, IPv6 carrier | yes (via NAT64/464XLAT) | yes | yes | yes, NAT often symmetric |
| Mobile, IPv4-only carrier (UK, Vodafone DE on Android default) | yes | **no** | yes | yes |
| Hotel, café, airport | yes | **usually no** [est] | often blocked [unverified] | often blocked [unverified] |
| Corporate, school | yes, unless filtered by domain category | **usually no** [est] | often blocked | often blocked (5% of QUIC in 2016) |
| TV or Cast receiver on an IPv4-only network | yes | **no** | yes | n/a (Cast: unknown) |

**Column (a) is the only one every network allows.** So anything that must work everywhere ends
at a public IPv4 on port 443 with an established domain name. Holding such an endpoint is either
the home line's job (public IPv4 plus port-forward) or a third party's.

## 4. NAT traversal evidence, and why most of it does not apply

**What is published** [source unless marked]:

| Source | Relayed share |
| --- | --- |
| Tailscale, [2025-10-15](https://tailscale.com/blog/nat-traversal-improvements-pt-1) | *"well north of 90%"* direct, from *"internal metrics"*. The 2020 post's 90% was an estimate. No raw DERP share is published |
| ZeroTier, [State of NAT traversal](https://www.zerotier.com/blog/the-state-of-nat-traversal/) | *"anywhere from 4% (our numbers) to 8% (an older number from Google)"* |
| callstats.io, 2015–16 ([webrtcHacks](https://webrtchacks.com/usage-stats/)) | *"22% of the conferences need some kind of TURN relay"*, and 9% needed TCP |
| Levent-Levi, 2018 ([discuss-webrtc](https://groups.google.com/d/topic/discuss-webrtc/5d_EJwM6iJM)) | *"Most large vendors … report 20% TURN relay traffic. Some reported over 30%"* |
| libp2p/IPFS, 4.4 M attempts in 167 countries ([arXiv 2604.12484](https://arxiv.org/abs/2604.12484)) | **70% ± 7.1%** hole-punch success once the relay and address discovery had worked. TCP and QUIC were about equal |
| Ford, Srisuresh & Kegel, 2005 ([arXiv](https://arxiv.org/abs/cs/0603074)) | 82% of NATs punchable for UDP, 64% for TCP |

- **Syncthing** [measured, 2026-09-25, [mon.syncthing.net](https://mon.syncthing.net)]:
  - Of 308,160 reporting devices, **13.9% are behind symmetric NAT**. 55.5% are port-restricted,
    20.9% unknown, and 0.9% not behind NAT at all.
  - The volunteer relay pool (992 relays) was carrying about 161,568 sessions at about 4.9 Gbps.
  - Syncthing publishes no share of connections that are relayed.
- **Plex** publishes no share of streams on Relay. Relay is capped at *"2 Mbps maximum for streams
  (this applies to both Plex Pass and Remote Watch Pass subscribers)"*
  ([Plex, 2025-07-14](https://support.plex.tv/articles/216766168-accessing-a-server-through-relay/)).

**Why this does not transfer.** Every figure above is for software that can send UDP from both
ends and punch through: Tailscale, ZeroTier, WebRTC, libp2p, Syncthing.

- **A web page cannot.** It is limited to HTTP, WebSocket and WebRTC
  ([Chrome Direct Sockets](https://developer.chrome.com/docs/iwa/direct-sockets)) [source].
  A `<video>` element or hls.js fetching segments over HTTPS needs a listening server at a reachable
  address. There is nothing to punch.
- **The only browser way through NAT is a WebRTC data channel feeding MSE** (the
  [p2p-media-loader](https://github.com/Novage/p2p-media-loader) pattern). That still needs a
  signalling server both sides can reach.
- **Cast receivers have no documented WebRTC data-channel support.** One developer report had video
  but no audio ([discuss-webrtc](https://groups.google.com/g/discuss-webrtc/c/niJ0WwdbKl4)).
  **TV browsers: no data.**
- **Tailscale's own browser client** tunnels over WebSockets to DERP, so its traffic is always
  relayed ([Tailscale SSH console](https://tailscale.com/blog/ssh-console)).

So **the 90% is a figure for native apps.** For the web client, the TV and the Cast receiver, the
direct rate is simply the share of (server, client) pairs where an inbound address matches. §6
estimates that.

UDP traversal does still matter for one thing: Watch Together voice through the embedded TURN
([D68](../../decisions/working-notes.md#d68--turn-is-not-an-independent-requirement-the-media-server-is-already-the-relay),
[D193](../../decisions/working-notes.md#d193--the-audits-proposals-are-accepted-the-node-does-all-media-work-prepare-is-the-one-path-besides-direct-play-the-machinery-is-cut-and-cut-as-design-freezes)).
The TURN server sits on the same reachability as the web server, and it needs a TCP/TLS 443
fallback for networks that block UDP.

## 5. Ways to reach the server without a hosted relay

| Way | Needs from the user | Serves browsers | Native apps | TV browser, Samsung/LG | Cast receiver | Cost |
| --- | --- | --- | --- | --- | --- | --- |
| **Public IPv4 + port-forward 443** | a line with public IPv4, one router rule, a name | yes | yes | yes | yes | €0 |
| **Ask the ISP for public IPv4 / dual-stack** | one support call or checkbox (§1.3) | then as above | | | | €0 (FR, most of AU) to £4–8/month (UK altnets). Unavailable on DG, Starlink Residential, T-Mobile Home Internet |
| **Inbound IPv6** | one router rule (FRITZ!Box, BT), or bridge/bypass mode (Vodafone Station, Starlink), plus an AAAA record kept current | **only clients with IPv6** | same | same, if the device and its network do IPv6 | doubtful (§2.5) | €0 |
| **Own VPS tunnel** (rathole, frp, WireGuard + SNI passthrough, or our `relay` mode on the user's box) | rent a VPS, run one service, point DNS at it | yes | yes | yes | yes | **€0** (Oracle Always Free: 10 TB/month egress, 2 OCPU/12 GB, reclaimed if idle ([Oracle](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm))). **~€1.5–2/month** (Netcup, IONOS [unverified]). **€5.99 + €0.50 IPv4** (Hetzner CAX11 since 2026-06-15, 20 TB included, [Hetzner](https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/); cheap plans were sold out in early September [unverified]) |
| **Tailscale / Headscale / ZeroTier / NetBird** | every *viewer* installs and enrols an app | **no** (browser client is DERP-only) | yes (iOS, Android) | Apple TV, Android/Google TV and Fire TV have Tailscale apps. **Samsung, LG, Roku: none** ([Tailscale docs](https://tailscale.com/docs/install/appletv)) | **no** | €0 |
| **Tailscale Funnel** | a tailnet node | yes | yes | yes | yes | €0, but *"subject to non-configurable bandwidth limits"* ([docs](https://tailscale.com/kb/1223/funnel)). Staff: *"it's a funnel, not a hose"* and advise against it for media ([HN](https://news.ycombinator.com/item?id=35375794)). **Not for video** |
| **Cloudflare Tunnel / proxy** | a Cloudflare account and domain | yes | yes | yes | yes | €0, but the CDN terms: *"Cloudflare reserves the right to disable or limit … if you use or are suspected of using the CDN without such Paid Services to serve video"* ([terms, 2026-06-02](https://www.cloudflare.com/service-specific-terms-application-services/)). Whether this covers Tunnel is not stated. TLS terminates at Cloudflare. 100 MB upload cap |
| **Public IPv4→IPv6 proxy services** | a domain | yes | yes | yes | yes | Mythic Beasts proxies SNI for its own customers ([Mythic Beasts](https://www.mythic-beasts.com/support/topics/proxy)). withfallback.com was down today. **No dependable free service exists** |
| **Paid IPv4 tunnel** (e.g. EDV Kossmann for DG) | subscription | yes | yes | yes | yes | €11.90/month, about 100 Mbit/s |

## 6. The matrix

**Rows** are the server's situation, **columns** the client's. *Direct* means no machine other
than the two endpoints and DNS. *1×* means direct after a one-time step by the owner. *VPS* means
the owner's own VPS is enough. *Mesh* means an enrolled app on the viewing device is also enough.
*Hosted* means our relay would also work, but is never the only answer.

| Server ↓ / client → | Home, IPv6 | Home, IPv4-only | Mobile data | Hotel / café | Corporate / school | TV / Cast on IPv4-only network |
| --- | --- | --- | --- | --- | --- | --- |
| **R1. Public IPv4, port-forward works** | Direct (v4 or v6) | Direct | Direct | Direct on 443. A non-standard port may be blocked | Direct on 443 with an established domain. DDNS or new domains may be category-blocked. Voice needs TURN over TCP/TLS 443 | Direct |
| **R2. CGNAT/DS-Lite, configurable inbound IPv6** (owner's FRITZ!Box) | **1×** (open IPv6 per device, AAAA via DDNS) | **Impossible without third party.** VPS; Mesh on phone, laptop or Apple/Android TV | 1× if the carrier has IPv6 (T-Mobile US, Telekom, EE, Jio, Telstra). Otherwise VPS or Mesh (UK O2/Vodafone/Three; Vodafone DE on the Android default) | **Usually impossible** [est]: VPS. Mesh only if UDP or TCP 443 to DERP passes | **Usually impossible** [est]: VPS on 443 with an established domain. Mesh often blocked or forbidden | **Impossible:** VPS. Mesh only on Apple TV, Android TV or Fire TV, **never on a Cast receiver, Samsung or LG** |
| **R3. No inbound path at all** (CGNAT with no IPv6, or an unconfigurable IPv6 firewall: T-Mobile Home Internet, a Starlink router not in bypass, a locked ISP router) | VPS / Mesh | VPS / Mesh | VPS / Mesh | VPS | VPS | VPS |
| **R4a. Starlink Residential** | 1× (bypass mode, own router), then as R2 | as R2 | as R2 | as R2 | as R2 | as R2 |
| **R4b. Fixed-wireless 5G** (T-Mobile/Verizon Home, GigaCube, hotspot) | as R3 (T-Mobile; Verizon and GigaCube unknown) | R3 | R3 | R3 | R3 | R3 |
| **R5. Double NAT** (ISP router + user router) | 1× (bridge mode or forward on both), then as R1 if the outer WAN is public, else R2/R3 | same | same | same | same | same |

**How to read it.**

- **Every "impossible" cell is in rows R2–R4**, and in R2 only in columns where the client lacks
  IPv6.
- **Every such cell is solved by the owner's own VPS.**
- A mesh VPN solves the phone and laptop cells, and none of the Cast, Samsung/LG or browser cells.
- **No cell requires a *hosted* relay.**

## 7. Where a hosted relay is truly the only answer

By topology: **nowhere.** A hosted relay is only a machine with a public IPv4 on port 443 that the
core dials out to, and a user's VPS is the same machine. The hosted relay is the only answer in
these narrower cases:

1. **The owner is in a "VPS" cell and will not or cannot run a VPS.** This is the whole of the
   hosted relay's market. It is D71's own conclusion: *"convenience for people who will not rent a
   VPS"*.
2. **Domain reputation on corporate or school filters.** A long-lived vendor domain passes
   "newly-registered" and "dynamic-dns" category blocks that a fresh user domain or a DDNS name may
   not. The same applies to "known streaming or relay service" categories, in the other direction.
   Narrow, and **an aged domain of the user's own does equally well.**
3. **Nothing else found.** Allowlist regimes (Iran, Russia's regional mobile whitelists, allowlist-
   only corporate networks) defeat hosted and self-hosted relays alike. Networks that block UDP
   are handled by TCP 443, which either kind of relay offers.

**Rough share of situations [est]. The arithmetic is shown because every input is soft.**

- **Servers with no inbound IPv4:**
  - In Germany, the new Vodafone cable, 1&1, O2 cable, Deutsche Glasfaser and Regio-fibre lines
    against Telekom's own dual-stack lines suggest roughly **30–50%** [est].
  - In France, all four ISPs share by default: **most lines**, but free self-service fixes it on Free (about 30 minutes, irreversible) and Orange.
  - In the UK and US the incumbents give public IPv4, so **10–20%**, driven by altnets and fixed wireless [est].
  - A Western-European and US blend is roughly **20–35% of servers** [est].
- **Of those, servers with no fix by asking the ISP** (Deutsche Glasfaser, Starlink Residential,
  T-Mobile Home Internet, Vodafone where goodwill fails, Jio, MAP-E): **perhaps a third to a half**,
  so **7–15% of servers** [est].
- **Of those, servers where inbound IPv6 can be opened** (FRITZ!Box, Starlink in bypass,
  Deutsche Glasfaser with an ordinary router): **most**, perhaps two-thirds [est]. The rest
  (T-Mobile Home Internet, locked ISP routers) are R3.
- **Remote clients without IPv6:**
  - Weekday traffic worldwide: about **53%**. Germany: about 25%. Spain and Italy: over 80%.
  - Hotels and offices: mostly [est].
  - For one remote household using a TV plus a phone, the chance that *some* device in a typical
    week lacks IPv6 is **well over half** [est].
- **Combined:**
  - About **5–20%** of (server, remote-session) pairs need a third party before any user action.
  - About **2–10%** still do after the one-time steps.
  - Measured *per household* rather than per session, the share of owners who will sooner or later
    need a third party for someone is higher: **roughly 7–15% of households**, essentially the
    no-fix CGNAT lines [est].
- **The share for which a hosted relay is the *only* answer is that 7–15% of households, times the
  fraction of those owners who will not run a VPS.** No data exists for that fraction. For a
  self-hosting audience it is plausibly small. For the "non-technical family" it is plausibly most
  of them. The number is set by who we are building for, not by the network.

## 8. What this suggests for Orpheum

**The owner's two users, concretely.**

- **The owner's line is R2.** A FRITZ!Box can open inbound IPv6 per device, and MyFRITZ! or deSEC
  keep the name current. Port-forwarding IPv4, which cut-a.md §2 puts first, cannot work on DS-Lite
  (it was already recorded that way on 2026-09-24).
- **The remote person's TV** is either on IPv6 or in a VPS cell, and Cast's IPv6 behaviour is
  unverified (§2.5). So **the VPS tunnel is the likely M4/M5 path, not the last resort.**
- **One question for the owner, not a decision made here:** whether cut-a.md §2's order should
  read "IPv6, then the VPS tunnel" for this deployment. A secondary check: ask Vodafone whether DSL
  dual-stack is available. If so, R2 becomes R1 for free.
- **TURN through a tunnel** needs the relayed address to be the VPS's public address, plus UDP
  forwarding of the TURN ports. frp and WireGuard carry UDP. This is a detail for M4 to confirm,
  not a design gap [unverified].

**Options, in increasing exposure:**

| | Option | What it costs us | What it costs the owner | Who it leaves out |
| --- | --- | --- | --- | --- |
| A | **IPv6-first, plus a documented own-VPS tunnel** (cut-a.md as frozen, reordered per line) | docs | IPv6: one router rule. VPS: €0–6/month and about an hour | owners who won't touch a VPS |
| B | **A plus our `relay` mode, run by the user** (D71's software on the user's VPS, with a one-command install such as a cloud-init file). **No hosted service** | the relay role, which D193 deferred | minutes, and €0–6/month | owners who won't open a cloud account |
| C | **B plus a reachability check that names the fix** ([D152](../../decisions/working-notes.md#d152--reachability-is-three-questions-with-three-observers-and-all-three-are-already-in-place)'s admin view): "no public IPv4; your router can open IPv6; clients without IPv6 need a tunnel; your ISP offers dual-stack on request" | a probe from outside, which needs *some* outside vantage point: the user's VPS, or a public test service | nothing | as B |
| D | **A hosted rendezvous only**: WebRTC signalling, no media bytes | a small service. Carries no content, so it is **not a conduit** in [legal.md](legal.md) §4's sense (analysis, not advice) | nothing | TVs, Cast receivers, and plain HLS in a browser (§4). Only a data-channel web player benefits |
| E | **Hosted relay** (D71/D72) | legal.md §4's whole list: repeat-infringer policy, DMCA agent, DSA contacts, logs, abuse handling, billing | a subscription | nobody |

**Can the hosted relay be dropped or deferred? The evidence says it can be deferred indefinitely.**
It is never technically necessary. What it buys is convenience for the share of owners on
no-fix CGNAT lines who won't run a VPS, and that share is unmeasured. Option B already delivers
the same property that made D71 attractive ("the core dials out; works behind CGNAT; no port
forwarding"), without us in the path. Since the software is the same, E can be added later
without redesign.

**What would need to be true to revisit it:**

1. **A public release targets non-technical owners.** Cut A's two users are not that audience.
2. **Measured, not estimated:** a meaningful share of beta households fail both IPv6 and "ask your
   ISP", *and* abandon at the VPS step. The C-style reachability check can count this on the user's
   own machine without telemetry, if owners choose to report it.
3. **An operating entity willing to carry legal.md §4's obligations.** Under D72 that also means
   payment, and a stated allowance rather than a silent throttle.
4. **No acceptable third party exists.** Today Cloudflare's terms exclude video, Funnel is
   bandwidth-limited, and no dependable free IPv4→IPv6 proxy exists. If one appears with terms
   that allow personal video, pointing owners at it may beat running our own.

## 9. What could not be verified

- **Per-country share of households behind CGNAT.** No current measurement exists. The 2016
  AS-level figure is the best available.
- **Vodafone Germany DSL:** whether dual-stack is available on request, and its prefix-rotation
  behaviour. This is the owner's own line.
- **Router details:** the FRITZ!Box IPv6-sharing specifics (the AVM page renders with JavaScript),
  whether the Vodafone Station's IPv6 firewall is configurable, the T-Mobile Home Internet and
  Starlink router behaviour, and Sky Hub (not researched: search limit).
- **Unmeasured networks:** IPv6 on hotel, café, airport and in-flight Wi-Fi, and how often hotels
  block non-standard ports. No measurements exist.
- **Enterprise:** IPv6 share, and how common explicit proxies and TLS inspection are.
- **Devices:** whether current Chromecast/Google TV Cast receivers fetch media over IPv6, WebRTC
  data-channel support on Cast and TV browsers, and Samsung Tizen IPv6.
- **Relay usage:** the share of streams on Plex Relay, Synology QuickConnect relay or Nabu Casa.
  None is published. Tailscale's DERP share is given only as "under 10%". Google's "8% relay"
  figure has no primary source.
- **Terms and prices:**
  - Whether Cloudflare's CDN video clause covers Tunnel. Cloudflare has not said, and the community
    threads could not be read.
  - Starlink's price for a public IP.
  - Netcup and IONOS prices (secondary sources).
  - The current availability of Hetzner's cheap plans.
- **Server-side gaps:** any EU ISP with a "no servers" clause, and the prevalence of double NAT.
- **The input the estimates in §7 rest on most:** what share of owners on no-fix CGNAT lines would
  run a VPS.
