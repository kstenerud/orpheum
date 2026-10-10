> **ABOUTME:** The network situations customers' homelabs will put a core in (carrier NAT, DS-Lite,
> double NAT, IPv6 quirks, NAT types, containers, VPNs, DNS rebinding, blocked ports, router
> families, isolated Wi-Fi), what each breaks for Orpheum's ways in, how the core can detect it,
> and what to do. Ends with a situation × approach matrix, a prioritised build list, and the steps
> for the owner's own FRITZ!Box + GL-MT6000 network.

# Homelab networks: the complications, and how to support the most setups

- **Date:** 2026-10-10.
- **Asked by** the owner: *"Considering we're likely to encounter all sorts of network situations in
  the many homelabs of our customers, do a research round to build a mapping of what kinds of
  complications that will bring, and how we can support as many setups as possible."*
- **Builds on, and does not repeat:** [reachability-approaches-2026-10-09.md](reachability-approaches-2026-10-09.md)
  (the layers and methods, D273), [direct-reachability.md](direct-reachability.md) (per-ISP CGNAT
  and IPv6 shares, client-side IPv6, prefix stability), [remote-access.md](remote-access.md) (NAT
  taxonomy, relay rates), [self-hosting-ways-2026-10-09.md](self-hosting-ways-2026-10-09.md) (how
  hosters run and expose servers, the platform shares), and the plans
  [discovery-core.md](../plans/discovery-core.md) and [webrtc-transport.md](../plans/webrtc-transport.md).
- **What is built** (the baseline every section measures against): a core enrols with a discovery
  that publishes `*.<coreId>.<zone>` names encoding its addresses and gets it a wildcard
  certificate by DNS-01 (C2–C4); the core reports interface, mapped and configured candidates; the
  working tree's `internal/core/portmap` maps the HTTPS port by PCP, NAT-PMP or UPnP IGD and asks
  for an IPv6 pinhole by UPnP's `WANIPv6FirewallControl` (C5, uncommitted); clients come in by
  HTTPS or over WebRTC data channels (W1–W3), signalled through the discovery (W4, not built);
  voice is WebRTC between people with TURN on the core (D243 §8). Nothing is relayed by us (D273 §2).
- **Legend:** **[V]** checked at its source this session (see Sources) or in the research it builds
  on; **[I]** this author's inference; **[unverified]** seen only in a forum, snippet or secondary
  source. Untagged is engineering common knowledge.

## 1. The short answer

1. **Two facts decide most households:** does the core's router have a public IPv4 that the core
   can get a port on, and is the core reachable on IPv6 from outside? Everything else (NAT types,
   containers, DNS) decides whether the *automatic* path works or the owner must do something.
2. **Germany is mostly the IPv6 case.** Vodafone (cable and DSL), 1&1 and O2 cable give new
   customers DS-Lite: no public IPv4, native IPv6 [V, direct-reachability §1.1; unverified for the
   DSL detail]. For them, the fix is opening the IPv6 firewall (one FRITZ!Box checkbox per device),
   not port forwarding. The US is mostly the public-IPv4 case, except fixed wireless (T-Mobile,
   Verizon) and Starlink, where neither IPv4 nor (with the stock router) IPv6 can be opened.
3. **Four findings in our own code, from reading it against the router software** (§3.2, §3.3,
   §3.5): (a) miniupnpd, the UPnP daemon in OpenWrt, GL.iNet, pfSense and OPNsense, opens an
   IPv6 pinhole only when the request comes *over IPv6 from the very address being opened*, and
   only for internal ports of 1024 and up [V, source]; our UPnP request goes over IPv4, for port
   443, so it is refused on every miniupnpd router. (b) miniupnpd's stock OpenWrt rules allow only
   ports 1024–65535 for IPv4 mappings too [V], so mapping 443 fails there as well. (c) miniupnpd
   switches IPv4 port mapping off entirely when its own WAN address is private, which is every
   double-NAT inner router [V]. (d) The candidates treat 100.64.0.0/10 (carrier NAT, and every
   Tailscale address) as public, and include IPv6 temporary addresses that rotate daily.
4. **Double NAT** (an ISP router plus the owner's own, the owner's case) defeats automatic IPv4
   mapping by design: the inner router can map, but only onto a private address. It is fixed once
   by the owner (bridge mode, access-point mode, or forwarding on both routers), and for IPv6 by
   prefix delegation plus opening the outer router's firewall for the inner router's prefix.
5. **Containers on Docker's default bridge** break every automatic step: the core sees container
   addresses, its "default gateway" is the Docker host, so port mapping asks the wrong machine, and
   WebRTC's host candidates are unreachable. Host networking fixes all of it; bridge mode needs the
   owner to publish ports and tell the core its address. Docker is used by 89% of self-hosters
   [V, self-hosting-ways §1], so this is not an edge case: the default compose file decides it.
6. **DNS rebinding protection** (on by default in FRITZ!Box and OpenWrt/GL.iNet dnsmasq, Unifi's
   content filter, pfSense) drops names that resolve to *private* addresses, so LAN names fail at
   home. It does not drop global IPv6 addresses [V, dnsmasq man page], so an IPv6 name works at home
   where a 192-168-… name does not.
7. **WebRTC hole punching fails** when both ends map per destination (symmetric NAT: many mobile
   carriers, pfSense/OPNsense by default) or UDP is blocked; only a relay helps then, which is the
   owner's own TURN/VPS or nothing (D273 §2). About 70% of punches succeed in the 2026 measurement
   [V, reachability-approaches §4].
8. **The single most useful new build is a reachability diagnosis on the System page** that names
   the situation (double NAT, carrier NAT, DS-Lite, no IPv6, container, host VPN, symmetric NAT,
   rebinding) and gives the one fix that applies, the way Plex's Remote Access page does (§4), plus
   fixing the four code findings above. The full list is §6.

## 2. How each of Orpheum's ways in can fail

| Way in | Needs | Typical failure |
| --- | --- | --- |
| **Direct IPv6** (AAAA name → the core's global address) | the ISP gives IPv6; every router between the core and the ISP routes it (prefix delegation); every firewall on that path lets the port in; the client has IPv6 | the router's IPv6 firewall drops inbound by default; the second router has no delegated prefix; the address the name carries changed (prefix change, temporary address) |
| **Mapped IPv4** (UPnP/NAT-PMP/PCP on the router) | a public IPv4 on the router the core talks to; the protocol on; the router accepts the ports asked | carrier NAT/DS-Lite (no public IPv4); double NAT (the router's WAN is private); protocol off; port < 1024 refused; the core in a container asks the wrong gateway |
| **Manual IPv4 forward** | as above, by hand | as above minus the protocol; the owner gets a step wrong |
| **WebRTC data channel** (hole punching, signalled through the discovery) | UDP outbound on both sides; at least one side's NAT endpoint-independent, or one side reachable | both sides symmetric; UDP blocked at the client; the core in a container advertising container addresses |
| **Voice** (WebRTC between people; TURN on the core) | peers punch through to each other, or reach the core's TURN by UDP | TURN unreachable when the core has no inbound UDP (carrier NAT without IPv6 on the peer); the TURN's configured public address is stale |
| **LAN** (the client at home) | the name resolves to an address on the LAN, or hairpinning, or WebRTC host candidates | rebinding protection; no hairpin on the router; client isolation; Docker bridge hides the LAN address |
| **The owner's VPS** (tunnel + TURN, S2) | a VPS with a public IPv4 | cost and effort only; always works [V, direct-reachability TL;DR] |

## 3. The situations

### 3.1 Carrier NAT and its relatives: CGNAT, DS-Lite, MAP-E, MAP-T, 464XLAT

**What they are.**

| Mechanism | The household gets | Where |
| --- | --- | --- |
| **CGNAT (NAT444)** | a private IPv4 on the router's WAN, usually 100.64.0.0/10; often native IPv6 beside it | Deutsche Glasfaser, Starlink Residential, US fixed wireless, AU and UK altnets, all mobile networks [V, direct-reachability §1.1] |
| **DS-Lite** (RFC 6333) | native IPv6; IPv4 tunnelled in IPv6 to the ISP's AFTR, which NATs it; the router's tunnel end is 192.0.0.2, the AFTR 192.0.0.1 | Vodafone cable and DSL, 1&1, O2 cable in Germany; Ziggo NL; transix JP [V/unverified per ISP, direct-reachability §1.1] |
| **MAP-E / MAP-T** (RFC 7597/7599) | native IPv6; a shared public IPv4 with a fixed *port set* (in Japan's v6plus, 240 ports in 4096–65535, so never 80 or 443) | Japan (v6plus, OCN virtual connect) [V, direct-reachability §1.1] |
| **464XLAT** (RFC 6877) | IPv6-only access, IPv4 translated by CLAT on the device and NAT64 at the carrier | T-Mobile US (mobile and Home Internet), Telstra mobile, EE [V, direct-reachability §2.2] |

**How common.** No current per-subscriber census exists [V, direct-reachability §1.1]. In Germany,
of the four large fixed ISPs only Telekom gives a public IPv4 by default; Vodafone says new cable
customers get only DS-Lite, and Vodafone DSL on resold (bitstream) lines is reported DS-Lite with no
switch to dual stack, where cable customers sometimes get dual stack as goodwill [unverified,
Vodafone community]. **[I]** A third to a half of German home lines are DS-Lite or CGNAT. In the US,
the big cable and telco ISPs give public IPv4; T-Mobile Home Internet, Verizon 5G Home and Starlink
Residential do not [V/unverified, direct-reachability §1.1]. Every mobile network is behind CGN (over
92% of cellular ASes in 2016) [V].

**PCP at the AFTR, the reality check.** RFC 7291 lets a DS-Lite router ask the AFTR for a port by
PCP, and the FRITZ!Box speaks it (FRITZ!OS 6.50 onwards [unverified]). For years no German ISP ran
it [V, reachability-approaches §4]. Since about 2024, **1&1 reportedly assigns DS-Lite customers a
small block of fixed ports (about 10–20) by PCP**, shown in the FRITZ!Box online monitor as the PCP
server and an "IPv4 address on the internet", and used automatically by the FRITZ!Box's WireGuard
[unverified, two German forums]. Whether the FRITZ!Box forwards a LAN device's own UPnP/PCP request
onward to the AFTR (acting as a PCP proxy, RFC 7648) is not documented anywhere found [I: assume
not]; an owner can forward one of the assigned ports to the core by hand. No such report was found
for Vodafone.

**What breaks.**

- Mapped and manual IPv4: impossible (except MAP-E within its port set, and 1&1's PCP ports).
- Direct IPv6: works where the router's IPv6 firewall can be opened (FRITZ!Box: yes; Vodafone
  Station: no; Speedport: no; Starlink and T-Mobile stock routers: no [unverified, §3.9]).
- WebRTC: works when the carrier NAT maps endpoint-independently and the client side is not also
  symmetric. RFC 6888 requires CGNs to be endpoint-independent, but 11% of fixed-line and about 40%
  of cellular CGNs were symmetric in 2016 [V, direct-reachability §1.1, §2.2].
- Voice: TURN on the core is unreachable over IPv4; peers without IPv6 need hole punching between
  themselves or the owner's TURN.

**Detect.**

- The router's external IPv4, from NAT-PMP's public-address request, PCP's MAP answer or UPnP's
  `GetExternalIPAddress`: in 100.64.0.0/10 → carrier NAT; 192.0.0.0/29 → DS-Lite; RFC 1918 →
  double NAT (§3.2) [I, the address ranges are standard]. A refusal from a router with no public
  IPv4 is a signal too.
- **Compare it with the address the discovery sees** the heartbeat come from (Plex's documented
  test, which also flags CGNAT by the 100.64/10 range [V, Plex troubleshooting]). Different → a
  NAT the core cannot map lies upstream.
- Heartbeat over each family: if the discovery is reachable at an IPv4-only and an IPv6-only
  address, the core learns whether it has IPv6 egress at all and both observed addresses [I].
- The outside probe's per-family result (already in C2): IPv4 candidate unreachable after a
  successful mapping = upstream NAT.

**Do.** Say so in one line ("Your internet provider shares one IPv4 address among many customers
(carrier NAT / DS-Lite): visitors can reach this server over IPv6 and WebRTC, not over IPv4"), then
steer to IPv6 (§3.3), and offer the owner's VPS for IPv4-only visitors. Where the ISP sells or gives
a public IPv4 (Free and Orange FR, most AU RSPs, UK altnets for a fee, Vodafone cable sometimes as
goodwill [V, direct-reachability §1.3]), a guidance text can say "ask your provider for dual stack
or a public IPv4". For 1&1 PCP ports: "forward one of the ports your FRITZ!Box shows under PCP to
this server" [unverified that it works for a LAN device].

### 3.2 Double NAT: an ISP router plus the owner's own

**How common.** No prevalence data [V, direct-reachability §1.4]. **[I]** It is common in exactly
our audience: homelabbers put OpenWrt, GL.iNet, Unifi, pfSense or OPNsense behind the ISP's box,
and mesh systems often run in router mode behind it (§3.10). Plex has a support section for it [V].

**What port mapping can and cannot do.**

- The core asks its own router (the inner one). That router can map a port, but onto its WAN
  address, which is a private address on the outer router's LAN. Nothing from the internet reaches
  it unless the outer router forwards the same port to the inner router.
- **miniupnpd refuses outright:** when its WAN address is private it *"disable[s] port
  forwarding"* unless `ext_allow_private_ipv4` is set, or STUN (`ext_perform_stun`) shows a 1:1
  unfiltered NAT upstream [V, miniupnpd.conf]. OpenWrt's init script exposes both
  (`ext_allow_private_ipv4`, `use_stun`), off by default [V]. So on a GL.iNet or OpenWrt inner
  router, IPv4 mapping fails, which is the honest answer.
- The outer router cannot be asked: UPnP's SSDP and NAT-PMP/PCP go to the core's own gateway, and
  PCP's "third party" option is refused by sane routers.
- **IPv6 is unaffected by the NAT, but needs two things the owner must arrange:** the outer router
  delegates a prefix to the inner one (DHCPv6-PD), and the outer router's firewall lets traffic for
  that delegated prefix in. The FRITZ!Box has exactly this: in Internet > Freigaben (Permit Access),
  the inner router's entry has **"Firewall für delegierte IPv6-Präfixe dieses Gerätes öffnen"**
  ("open firewall for delegated IPv6 prefixes of this device"), from FRITZ!OS 6.83 [unverified,
  several German forums agree]. It opens the *whole* delegated prefix, leaving the inner router's
  firewall in charge [unverified].

**FRITZ!Box specifics** (FRITZ!OS 7.x; menu names vary slightly between versions [I]):

| Setting | Where | What it does |
| --- | --- | --- |
| Hand out prefixes to routers | Heimnetz > Netzwerk > Netzwerkeinstellungen > IPv6-Einstellungen: DHCPv6-Server: **"DNS-Server, Präfix (IA_PD) und IPv6-Adresse (IA_NA) zuweisen"** | lets a downstream router get its own prefix [unverified, forums and a UDM Pro guide] |
| Per-device automatic port sharing | Internet > Freigaben > Portfreigaben > the device > **"Selbstständige Portfreigaben für dieses Gerät erlauben"** (allow independent port sharing) | the device may open ports itself by UPnP IGD or PCP; needs a public IPv4 or IPv6 from the ISP [unverified, AVM KB via snippet]; off by default per device [unverified] |
| Per-device IPv6 sharing | the same device entry: "IPv6-Interface-ID" plus a rule, or "PING6 / Firewall komplett öffnen" | opens IPv6 for that device's interface ID, which follows prefix changes [unverified]; the interface ID is the stable part of the address, so a rotating temporary address breaks it (§3.3) |
| Delegated prefix firewall | the downstream router's entry > **"Firewall für delegierte IPv6-Präfixe dieses Gerätes öffnen"** | as above |
| Exposed host | the device entry > "Exposed Host" | forwards all *IPv4* ports to one device; useless under DS-Lite [I] |
| Rebinding exceptions | Heimnetz > Netzwerk > Netzwerkeinstellungen > DNS-Rebind-Schutz: host-name exceptions | §3.7 |
| Parental controls | Internet > Filter > Kindersicherung: the *default* profile | reported to apply to IPv6 traffic of devices behind a downstream router, which the FRITZ!Box cannot tell apart; set it to unrestricted if IPv6 behind the second router misbehaves [unverified, one forum thread] |

**GL.iNet / OpenWrt specifics.** Stock GL.iNet firmware is OpenWrt with GL.iNet's own UI; the
GL-MT6000's stable 4.8.x line is reported to be based on OpenWrt 21.02, with an OpenWrt 24-based
line beside it [unverified]. LuCI (Advanced Settings) is available on it.

| Setting | Stock value | Effect on Orpheum |
| --- | --- | --- |
| UPnP service `enabled` | 0 (off) [V, OpenWrt config] | nothing maps until the owner turns it on (GL.iNet: Applications > UPnP [I]) |
| `enable_natpmp`, `enable_upnp` | 1, 1 (once enabled) [V]; newer packages fold them into `enable_protocols` [V, PR #28765] | NAT-PMP/PCP and UPnP answer |
| `secure_mode` | 1 [V] | a client may map only to its own address: a containerised core's request names the container address and is refused [I] |
| `igdv1` (newer: `upnp_igd_compat=igdv1`) | 1 [V] | the router *advertises* IGDv1 (`force_igd_desc_v1`), which has no `WANIPv6FirewallControl`: our IPv6 pinhole call finds no service. Unticking it in LuCI ("IGDv1 compatibility") advertises IGDv2 [V, init script] |
| Default rules | allow 1024–65535 → 1024–65535; deny everything else [V] | **mapping internal or external port 443 is refused** |
| WAN address private (double NAT) | `ext_allow_private_ipv4` 0, `use_stun` 0 [V] | IPv4 mapping off entirely |
| IPv6 pinholes | need `--igd2` and `--ipv6` at build time (OpenWrt builds both) [V, Makefile]; the request must come **over IPv6 from the address being opened**, else SOAP error 606; internal port ≥ 1024, else 606 [V, upnpsoap.c `PinholeVerification`] | our call (over IPv4, port 443) is always refused |
| IPv6 mode (GL.iNet UI: Network > IPv6) | Native, Passthrough (relay), NAT6, Static [V, GL.iNet docs]; default not documented [unverified: reported off on some firmware] | Native requests a delegated prefix by DHCPv6-PD [I]; NAT6 gives LAN devices ULA addresses only (no inbound IPv6 at all); Passthrough relays the upstream /64 (then the FRITZ!Box sees the core as its own device) |
| GL.iNet's own firewall UI | "some features (e.g., firewall…) do not yet support IPv6" [V, GL.iNet docs] | IPv6 inbound rules for LAN hosts must be made in LuCI (Network > Firewall > Traffic Rules), not GL.iNet's UI [I] |

**What breaks.** Mapped IPv4: always, without owner action. Direct IPv6: until the owner arranges
delegation and both firewalls. WebRTC: usually still works, since two endpoint-independent NATs
stacked are still endpoint-independent [I]. Voice: as WebRTC; the core's TURN needs IPv6 or a
double forward.

**Detect.** The router's external IPv4 is private (RFC 1918) or the mapping protocol refuses with a
network-failure result → "double NAT: your router (X) sits behind another router (the gateway at
its WAN address Y)". The core's IPv6: global addresses absent while the discovery says the line has
IPv6 (another device on it, e.g. the client, used IPv6) → "the router in front of this server has
no IPv6 prefix" [I].

**Do (guidance text, in order of preference).**

1. Make one router stop routing: the ISP router in bridge/modem mode (not offered on DS-Lite
   FRITZ!Box or Vodafone Station configurations [unverified]), or the owner's router in access-point
   mode (loses its own features).
2. Forward on both: the outer router forwards the port to the inner router's WAN address, the inner
   router to the core (the FRITZ!Box can also make the inner router its exposed host, IPv4 only).
3. For IPv6: delegation on the outer router, the delegated-prefix firewall opened (FRITZ!Box) or
   the equivalent, and a rule on the inner router (§7 has the exact steps for the owner's pair).

### 3.3 IPv6 realities

| Situation | How common | What breaks | Detect | Do |
| --- | --- | --- | --- | --- |
| **Inbound blocked by the router's firewall** (RFC 6092's recommended default) | nearly every consumer router [V, direct-reachability §1.3] | direct IPv6 | the outside probe to the IPv6 candidate times out (dropped) | automatic: PCP or UPnP pinhole (see the fixes below); guidance: the router's per-device IPv6 sharing |
| **Firewall not configurable** | Vodafone Station (except an "IPv6 Host Exposure" rule [unverified]), Speedport ("no plan" for IPv6 port forwarding [unverified, Telekom community]), T-Mobile and Starlink stock routers [unverified] | direct IPv6 | as above, and the router answers no pinhole protocol | guidance: own router behind it with delegation, or bridge mode where offered (Vodafone Station bridge mode gives no IPv6 [unverified]) |
| **No delegated prefix behind a second router** | every double-NAT household that has not set up DHCPv6-PD [I] | direct IPv6 (the core has only ULA or link-local) | no global IPv6 on the core | §3.2 |
| **Prefix changes** | Telekom gives a new prefix on each new session; the FRITZ!Box's optional daily reconnect triggers it; Vodafone DSL says it has had no forced disconnect for years [unverified, community]; RIPE-690 calls non-persistent prefixes harmful [V] | the names published carry the old address until the next heartbeat (5 minutes, C2) | interface addresses change between ticks | automatic: re-heartbeat at once on an address change (netlink, or the C5 loop's 1-minute check); short DNS TTL |
| **Temporary (privacy) addresses** (RFC 8981) | default on desktop Linux, macOS, Windows; on Debian servers usually off [I] | a candidate built from a temporary address dies within a day; FRITZ!Box and OpenWrt suffix rules match the stable interface ID, not the temporary one | `/proc/net/if_inet6` flag `IFA_F_TEMPORARY` (0x01); deprecated (0x20) | report only stable, non-deprecated addresses; prefer the EUI-64 or RFC 7217 stable one |
| **ULA only** (fd00::/8, e.g. GL.iNet NAT6, Docker's default IPv6 pools [V], Tailscale's fd7a:115c:a1e0::/48) | homelabs with IPv6 NAT or no ISP IPv6 | no global path; ULA is LAN-only | address in fc00::/7 | treat as LAN-scope; never publish it as internet-reachable (Go's `IsPrivate` already does) |
| **NPTv6** (RFC 6296: the router translates a ULA prefix 1:1 to the global one) | rare; OPNsense/pfSense and some OpenWrt setups [I] | the core sees only its ULA; the global address is unknown to it | the discovery's observed IPv6 is global while the core has only ULA | publish the observed IPv6 (its interface ID matches) as a candidate [I] |
| **The client has no IPv6** | weekday Google traffic 46–47% IPv6; DE 75%, ES 13%; UK O2/Vodafone/Three mobile none; hotels and offices mostly none [V, direct-reachability §2] | direct IPv6 for that client | the client's race: AAAA path fails fast | WebRTC or mapped IPv4; the owner's VPS |

**Two code fixes for pinholes** (§1 finding a):

- **PCP for IPv6.** A PCP MAP request whose client address is the core's global IPv6 address, sent
  over IPv6 to the router's address (the IPv6 default route's gateway, usually link-local), asks the
  router's firewall to open that address and port (RFC 6887 §11). miniupnpd implements PCP for IPv6
  pinholes when built with IPv6 [I, from the 24.10 LuCI text "PCP … IPv6 port mapping"; V that the
  text exists]; the FRITZ!Box accepts PCP from devices allowed to share ports [unverified]. Today
  `mapPCP` sends only IPv4 MAP requests.
- **UPnP over IPv6.** `AddPinhole` must reach the router over IPv6 from the address being opened
  (search on `[ff02::c]:1900` or use the description's IPv6 URL), and the internal port must be at
  least 1024 [V, miniupnpd]. Also untick IGDv1 compatibility on OpenWrt (§3.2).

### 3.4 NAT types and WebRTC

**The two properties that matter** (RFC 4787; Tailscale's "easy" and "hard" NAT [V]):

- **Mapping:** endpoint-independent (one public port for a socket, whatever the destination) vs
  endpoint-dependent ("symmetric": a new port per destination). Hole punching works if at least one
  side is endpoint-independent; with two hard NATs only guessing ("birthday paradox", Tailscale's
  trick, 99.9% in 20 s for one hard side, impractical for two [V]) or a relay works. Browsers do not
  do the birthday trick [I].
- **Filtering:** whether unsolicited packets from other addresses are let in; punching handles it.

**Who is hard.** pfSense and OPNsense rewrite source ports on outbound NAT by default, which makes
them endpoint-dependent; the fixes are static-port outbound NAT or NAT-PMP [V, Tailscale docs].
pfSense Plus 25.11 adds experimental endpoint-independent NAT [unverified]. Unifi gateways punch fine
unless threat detection blocks P2P [V, Tailscale firewall page]. Barracuda, Cisco, Fortinet and Palo
Alto are hard by default (offices) [V]. Mobile carriers: about 40% symmetric (2016) [V]. Most
consumer routers (Linux conntrack masquerade keeps the source port when free) behave
endpoint-independently [I].

**Hairpinning** (NAT loopback: a client at home using the public address). Many otherwise good
NATs lack it [V, Tailscale]. With DS-Lite there is no public IPv4 to loop back to; on IPv6 nothing
loops (the address is the core's own). It matters for IPv4-only households using the mapped name at
home; the client's race prefers the LAN path anyway (D273 §7).

**UDP blocked** (hotel, corporate, school Wi-Fi). About 5% of connections had UDP impaired in
Google's QUIC data [V, direct-reachability §2.4]. Then WebRTC cannot start, and only TCP paths
remain: direct HTTPS (IPv6 or mapped IPv4), or TURN over TCP/TLS on 443 somewhere reachable.

**ICE-TCP.** pion supports passive ICE-TCP candidates through a TCP mux [V, pion docs], and
browsers send active TCP candidates [I]. It only helps a client whose UDP is blocked *and* when the
core already has a reachable TCP port, which is exactly when plain HTTPS already works, so it adds
nothing for the API and media. For voice to the core (TURN), TURN over TCP/TLS on a reachable port is
the equivalent [I].

**The browser hides LAN addresses.** Chrome (from M74), Firefox and Safari replace private host
candidates with random `.local` mDNS names unless the page has camera or microphone permission [V,
discuss-webrtc PSA]. The core must resolve them by multicast DNS (pion's mDNS query mode [I]) to pair
on the LAN, which needs the core on the LAN's multicast domain: not in a Docker bridge (§3.5), not
across VLANs or client-isolated Wi-Fi (§3.10). Without it, LAN WebRTC falls back to server-reflexive
candidates through the router (hairpin) [I].

**When a relay is unavoidable** (for the WebRTC transport and for voice): both sides symmetric; or
the client's UDP blocked and the core has no reachable TCP; or the core is behind carrier NAT and
the client has neither IPv6 nor a punchable NAT. Then the owner's own TURN on a VPS (S2) is the only
direct-ish answer under D273 §2.

**Detect from the core.** The discovery runs STUN for W4 anyway; if it answers on two addresses or
ports (RFC 5780's behaviour discovery), the core can tell its own mapping type (same public port to
both → endpoint-independent) and whether the router preserves ports [I]. Show "NAT: easy / hard"
on the System page with the router-family hint ("pfSense: enable static port or NAT-PMP").

### 3.5 Containers and virtualisation

| Setup | Share | What the core sees | Port mapping | WebRTC / voice | Do |
| --- | --- | --- | --- | --- | --- |
| **Docker, host networking** | — | the host's real addresses | works | works | **recommend as the default compose** for the core [I] |
| **Docker, default bridge** | Docker 89% of self-hosters [V] | 172.17/16 container address; the default gateway is the Docker host (172.17.0.1) | **fails**: `DefaultGateway` reads the container's routing table and finds the Docker host, so NAT-PMP/PCP go to it; SSDP multicast does not leave the bridge; any request that does reach the router names the container's address and is refused by `secure_mode` / PCP's address check [I, code + config] | host candidates are container addresses; srflx through two NATs usually works; TURN needs its ports published | detect (`/.dockerenv`, gateway = a bridge address); say "use host networking, or publish port N/tcp and N/udp and set the public addresses"; pion's address rewrite rules (`SetICEAddressRewriteRules`, v4.2+) or `SetNAT1To1IPs` with the host's address [V, pion] |
| **Docker IPv6** | — | IPv4 only unless the daemon or the network enables IPv6; `ip6tables` on by default [V] | — | — | without IPv6 on the network, an IPv6 published port is proxied to the container's IPv4 by `docker-proxy` and the core sees the proxy, not the client [V, Docker docs]; with IPv6, the default `nat` mode NATs IPv6 too, `gateway_mode_ipv6=routed` gives the container a routable address [V] — or just host networking |
| **Docker macvlan/ipvlan** (Unraid's "br0" custom network; Unraid suggests ipvlan for stability [unverified]) | Unraid 10% [V] | its own LAN address | works for the container's address; ipvlan shares the host's MAC, which some routers (reported: FRITZ!Box) handle badly for port sharing [unverified] | host candidates on the LAN; works | supported; mention the FRITZ!Box/ipvlan quirk |
| **Kubernetes / k3s** | small among homelabs [I] | pod address | fails | fails | `hostNetwork: true`, or a LoadBalancer/NodePort with the address configured [I] |
| **Proxmox LXC or VM on vmbr0** | Proxmox 45% [V] | its own LAN address (bridged) | works | works | none; Landlock note separately (self-hosting-ways §5) |
| **TrueNAS SCALE apps** | 19% [V] | Docker since 24.10 (Kubernetes before) [V] | as Docker | as Docker | custom app with host networking [I] |
| **Synology Container Manager / QNAP Container Station** | DSM 10% [V] | Docker; bridge by default [I] | as Docker; DSM's own firewall may also block | as Docker | host networking; DSM firewall rule [I] |

### 3.6 Overlays and VPNs on the host; multiple interfaces

- **Tailscale / WireGuard already on the box.** Tailscale's interface holds an address in
  100.64.0.0/10 and one in fd7a:115c:a1e0::/48. **The core's candidate code treats the 100.x
  address as public** (`netip.Addr.IsPrivate` covers only RFC 1918 and fc00::/7) and publishes it;
  it is reachable only by tailnet members [V, code; I, consequence]. Classify 100.64/10 as
  shared-space (and recognise `tailscale0`/`wg*` as overlays), publish it only as an overlay-scope
  candidate, or not at all. A client on the same tailnet can use it (Orpheum works fully over a
  mesh VPN, self-hosting-ways row J).
- **A commercial VPN routing all the host's traffic out** (common on download boxes). The heartbeat
  leaves through the VPN, so the discovery sees the VPN's exit; port mappings on the home router
  answer, but replies to inbound connections leave by the VPN and are dropped (asymmetric routing)
  [I]. Detect: the default route is a tunnel interface (`wg*`, `tun*`) or the observed IPv4 differs
  from the router's external one while the router reports a public address. Do: "split-tunnel: let
  the server's port answer outside the VPN, or run Orpheum outside it". Some VPN providers offer
  port forwarding (as a configured candidate) [I].
- **Multiple interfaces / VLANs.** `DefaultGateway` takes the first IPv4 default route; candidates
  list every interface. A core on a server VLAN reaches the router fine, but LAN clients on another
  VLAN need routing between them and mDNS does not cross (§3.4) [I]. Bonds and bridges are fine.
- **Docker's and libvirt's own bridges** on a host-networked core (docker0, virbr0) appear as
  LAN-scope candidates that no client can reach; skip known virtual interfaces [I].

### 3.7 DNS

**Rebinding protection** drops public DNS answers that point at private addresses, so a LAN name
like `192-168-1-5.<coreId>.<zone>` resolves to nothing.

| Resolver | Default | Exemption |
| --- | --- | --- |
| dnsmasq (OpenWrt, GL.iNet, DD-WRT) | `rebind_protection 1` on OpenWrt [V]; blocks RFC 1918, IPv4-mapped private, link-local and **ULA**, not global IPv6 [V, man page] | `rebind-domain-ok=/<zone>/` (OpenWrt: `list rebind_domain '<zone>'`, LuCI DHCP and DNS > Filter) [V] |
| FRITZ!Box | on [V, reachability-approaches §9.1] | Heimnetz > Netzwerk > Netzwerkeinstellungen > DNS-Rebind-Schutz host-name exceptions [V, it-halbwissen guide] |
| pfSense (Unbound) | on | `server: private-domain: "<zone>"` in custom options [V, Plex] |
| OPNsense (Unbound) | on [I] | Unbound > Advanced > private domains [I] |
| Unifi | content filter / CyberSecure blocks; allowlist the domain [unverified, Plex forum] | Settings > CyberSecure > Content Filter allowlist |
| Pi-hole | off by default, unless its upstream does it [unverified, Pi-hole forum] | dnsmasq line in `/etc/dnsmasq.d/` |
| Firefox's DoH (TRR) | answers with RFC 1918 addresses are treated as failures (`network.trr.allow-rfc1918` false), mode 2 falls back to the system resolver [V, Mozilla wiki] | — |
| Chrome's secure DNS | upgrades only when the system resolver is a known DoH provider [I] | — |

**Consequences.** (a) Prefer the **global IPv6 name** at home: it is not filtered and needs no
hairpin [V for dnsmasq; I for others]. (b) A client whose LAN name fails should say so to the core,
which turns it into the guidance line for the household's resolver. (c) With two dnsmasq routers in
a row (the owner's case), both filter; the exemption is needed on the inner one and, unless it
forwards to a public resolver, on the outer one too [I]. (d) Chrome's Local Network Access prompt
applies to a LAN address reached from app.orpheum-app.com [V, reachability-approaches §2].

**Split horizon** (a local DNS entry pointing the name at the LAN address, as Nextcloud AIO
recommends with Pi-hole or its dnsmasq container [V]) is the owner-side alternative; our names carry
the address already, so it is only needed where rebinding cannot be exempted.

### 3.8 Ports

- **443 and 80 taken** on the host by a reverse proxy (67% of self-hosters run one [V]): the core
  cannot also listen there; self-hosting-ways covers proxy mode. For the discovery's names, any port
  works if the name carries it (the candidate already has a port) [I].
- **Ports < 1024 refused by the router's mapping rules** (miniupnpd's stock rules, §3.2), and
  pinholes below 1024 refused always [V]. **Do:** map a high port. The core can listen on a second,
  high port (Plex uses 32400 and any external port in 20000–50000 [V, Plex]) for the mapped and
  pinholed path, while 443 stays for the LAN and the proxy [I].
- **Router grants another external port** than asked (UPnP and PCP may): the candidate carries the
  granted port (the code already does).
- **ISP blocks inbound ports.** Comcast does not block 80/443 [V]; Cox reportedly blocks 80
  [unverified]; MAP-E cannot have them. A high port avoids most of it.
- **Non-standard ports in URLs.** Browsers are fine; corporate proxies and some captive networks
  allow only 80/443 outbound [V, direct-reachability §2.3]: such a client cannot reach `:32400`
  either way, and WebRTC (UDP) is usually blocked there too; only the owner's VPS on 443 helps.
- **The service's outside probe** should use the same port the names publish, from addresses the
  owner can allowlist (Plex publishes its probe workers' IPs for pfSense users [V]).

### 3.9 Router families

| Family | UPnP IGD | NAT-PMP / PCP | IPv6 pinholes | Inbound IPv6 by hand | Quirks |
| --- | --- | --- | --- | --- | --- |
| **AVM FRITZ!Box** | yes, per device ("Selbstständige Portfreigaben") [unverified] | PCP yes (also to the ISP's AFTR) [unverified] | via UPnP/PCP for permitted devices [I] | per device, by interface ID; delegated-prefix checkbox [unverified] | rebind protection on; exposed host IPv4 only; 7490 gets only maintenance updates since 7.60 (now 7.62) [unverified, news] |
| **Telekom Speedport** | not supported (Smart 4) [unverified] | no [I] | no | **no** ("no plan") [unverified] | port forwards hidden behind expert mode |
| **Vodafone Station** (cable) | only on dual-stack lines, off on DS-Lite [V, Vodafone] | no [I] | no | "IPv6 Host Exposure" rule [unverified] | bridge mode has no IPv6 [unverified] |
| **OpenWrt / GL.iNet** (miniupnpd) | off; IGDv1 advertised by default [V] | NAT-PMP and PCP once enabled [V] | only over IPv6 from the address itself, port ≥ 1024, IGDv2 [V] | LuCI traffic rule; `::<suffix>/-64` matches a host whatever the prefix [unverified, OpenWrt forum] | rebind on; ports < 1024 refused; refuses mapping on a private WAN [V] |
| **pfSense** | off (service "UPnP & NAT-PMP") [I] | NAT-PMP checkbox [V, Tailscale] | miniupnpd built without IGDv2 (bug 15448, "miniupnpd lacks IGDv2 support") [unverified, title only] | firewall rule | hard NAT by default [V] |
| **OPNsense** | plugin `os-upnp`, off [I] | NAT-PMP in the plugin [V, Tailscale] | as miniupnpd [I] | firewall rule | hard NAT by default [V] |
| **Unifi** | off; "secure mode" and NAT-PMP options [unverified] | NAT-PMP [unverified] | unknown | firewall rule | P2P blocked when threat detection on [V]; content filter blocks rebinding [unverified] |
| **ASUS, TP-Link, Netgear consumer** | on by default on many models [I; 76% of routers had UPnP on in a 2019 scan, V] | ASUS (miniupnpd) yes [I] | varies | varies; TP-Link guides tell users to switch IPv6 on at all [V] | |
| **Comcast Xfinity gateway** | off by default; Advanced > Device Discovery or the app [unverified] | no [I] | no ("xFi does not … allow port forward for … IPv6" [unverified]) | no | xFi Advanced Security breaks Plex remote access with no exception possible [V, Plex] |
| **BT Smart Hub 2** | yes [unverified] | no [I] | no | "pinholes", reported to work only with the IPv4 firewall disabled [unverified, BT community] | |
| **Starlink router** | none useful (CGNAT) | — | — | no; bypass mode + own router gets a /56 [unverified] | Residential is CGNAT, Priority gets a public IPv4 [V] |
| **T-Mobile Home Internet gateway** | — (CGNAT, 464XLAT) | — | — | no [unverified] | |

### 3.10 Mesh Wi-Fi, guest networks, client isolation

- **Mesh systems in router mode** behind the ISP box (Eero, Deco, Google/Nest Wifi, Orbi) make a
  double NAT (§3.2); in bridge/AP mode they do not. Several limit or omit UPnP and IPv6 settings
  [I].
- **Guest networks** isolate guests from the home LAN (FRITZ!Box guest access, GL.iNet guest Wi-Fi):
  a client there reaches the core only by its public paths (IPv6, mapped IPv4), as a visitor would
  [I]. That is the right default for a guest, and the race handles it.
- **Client / AP isolation** on a main network (some ISP routers, "smart" mesh profiles, office Wi-Fi)
  blocks device-to-device traffic: LAN names, mDNS and WebRTC host candidates fail; the client falls
  back to public paths [I]. Voice between two people at home goes through hairpin or TURN.
- **Detect:** the client reports that the LAN path failed while the public one worked (§6, new
  item N7); the core cannot see it [I].

## 4. How others solved it

| Product | Path in | Detection and what it tells users | Relay? |
| --- | --- | --- | --- |
| **Plex** | `*.plex.direct` names with addresses in them, wildcard certificate per server; UPnP/NAT-PMP mapping; manual port with "manually specify public port" [V] | Settings > Remote Access shows "Fully accessible outside your network" or "Not available outside your network" after plex.tv's workers probe the server [V]; support pages walk through double NAT (compare the router's WAN address with what-is-my-IP; 100.64/10 = CGNAT), bridge mode, forwarding on both routers, rebinding exemptions per resolver, jumbo frames, xFi Advanced Security [V] | yes, a 2 Mbit/s (1 Mbit/s free) relay [V, self-hosting cache] |
| **Jellyfin / Emby** | the owner's own reverse proxy or port forward; Emby Connect only finds servers [V] | none built in; docs [V] | no |
| **Tailscale** | WireGuard over UDP, hole punching with STUN-like probes, birthday trick, UPnP/NAT-PMP/PCP on the router, IPv6 [V] | `tailscale netcheck` reports UDP, IPv4/IPv6, mapping varies by destination ("hard NAT"), port mapping protocols found, nearest relay; docs per firewall family [V] | yes, DERP over HTTPS when direct fails [V] |
| **Syncthing** | TCP/QUIC on 22000; UPnP and NAT-PMP mapping; local discovery by broadcast [V] | logs "Created UPnP port mapping…"; the GUI shows connection type (relay or direct) [V/I] | yes, community relays, "will perform poorly in comparison to a direct connection" [V] |
| **Home Assistant Cloud** | SniTun: the instance dials out; Nabu Casa's server forwards TLS by SNI over a multiplexed connection; TLS is end to end with the instance's own certificate [V, SniTun README] | remote access on/off and status [I] | yes, all traffic through the SNI proxy (our D273 §2 excludes this) |
| **Nextcloud AIO** | ports 443 (and 80/8443 for setup) forwarded; Talk's TURN needs 3478 TCP+UDP forwarded; or a reverse proxy, or Cloudflare Tunnel with caveats, or Tailscale [V] | a domain check container that refuses to proceed until the domain reaches the server (skippable); errors for a private address [V]; local access by a dnsmasq/Pi-hole local record [V] | no (Cloudflare optional) |
| **Cloudflare Tunnel** | outbound connector; HTTP only to browsers [V] | dashboard health | yes, and video is against the CDN terms [V, self-hosting-ways] |
| **ZeroTier** | P2P over UDP with root servers for rendezvous, relaying through roots when direct fails [I] | `zerotier-cli peers` shows DIRECT/RELAY [I] | yes |

**Lessons [I].** (1) Every product with a high success rate tries several paths and shows *which
one* each connection uses. (2) The useful diagnostics are specific: Plex's double-NAT test and
Tailscale's netcheck name the situation and the router family's fix. (3) The ones without relays
(Jellyfin, Nextcloud AIO) put the burden on documentation and a hard pre-flight check; the
pre-flight check is worth copying, the burden is not.

## 5. The matrix: situation × approach

Legend: **yes** works automatically; **user** works after a one-time owner step; **client** depends
on the visitor's network; **VPS** needs the owner's TURN/tunnel (S2); **no** impossible.
"Voice" is two people, one at home, one away; "LAN" is a client at home.

| Situation | Direct IPv6 | Mapped IPv4 (auto) | Manual IPv4 | WebRTC data channel | Voice | LAN client |
| --- | --- | --- | --- | --- | --- | --- |
| Public IPv4, single router, UPnP/PCP on | user (router IPv6 firewall) or yes with fixed pinholes; client needs IPv6 | **yes** (high port) | — | yes | yes | yes (IPv6 name; LAN name if no rebind filter) |
| Public IPv4, mapping off | user; client | user (turn it on) | user | yes; client | yes once mapped, else client | as above |
| Double NAT, public IPv4 outside | user (PD + both firewalls) | **no** (inner router refuses on private WAN) | user (both routers, or bridge/AP mode) | yes, usually; client | user / client | yes |
| DS-Lite, router opens IPv6 (FRITZ!Box) | user, then yes; client | no | no | client (~70%) | IPv6 peers yes; IPv4-only peers VPS | yes |
| DS-Lite with ISP PCP ports (1&1) | as above | no [I] | user (forward an assigned port) [unverified] | client | user | yes |
| DS-Lite, router cannot open IPv6 (Vodafone Station, Speedport) | no (own router behind it: user) | no | no | client | VPS for most | yes |
| CGNAT with IPv6 (Deutsche Glasfaser) | user; client | no | no | client | client / VPS | yes |
| CGNAT, stock router blocks IPv6 (Starlink Residential, T-Mobile Home Internet) | no (Starlink: bypass + own router: user) | no | no | client | VPS | yes |
| MAP-E (Japan) | user; client | user/yes within the port set, never 80/443 | user, port set only | client | client | yes |
| No IPv6 from the ISP (Virgin Media, TalkTalk, Odido) | no | yes / user | user | yes | yes | yes |
| IPv6 without delegation behind the 2nd router | user (PD) | as the IPv4 row | as the IPv4 row | as row | as row | yes |
| Core in Docker bridge | user (IPv6 network or host net) | **no** → user (host net) | user (publish + forward + set address) | user (publish UDP, set address) | user | partial (no mDNS pairing) |
| Host VPN routes all traffic | no until split-tunnel (user) | no until split-tunnel (user) | same | client (via the VPN's NAT) | client | yes |
| Router hard NAT (pfSense/OPNsense default) | unaffected | yes with NAT-PMP on | yes | client→ user (static port / NAT-PMP) | user | yes |
| Visitor on UDP-blocked Wi-Fi | client | yes | yes | **no** | VPS (TURN over TCP/TLS) | — |
| Visitor on symmetric mobile NAT, core behind CGNAT | client (most mobile has IPv6: yes) | no | no | **no** when the core side is hard too | VPS unless IPv6 | — |
| Visitor IPv4-only (hotel, UK mobile), core DS-Lite | **no** | no | no | client | VPS | — |
| LAN with rebinding protection | yes (global IPv6 not filtered) | hairpin, router-dependent | hairpin | yes (host candidates) | yes | user for LAN names (exemption) |
| Guest Wi-Fi / client isolation | as a visitor | as a visitor | as a visitor | as a visitor | as a visitor | no LAN path (by design) |

## 6. What to build and document, in priority order

### 6.1 Already planned or built

| Item | State |
| --- | --- |
| C5 port mapping: PCP, NAT-PMP, UPnP IGD for IPv4; UPnP IGDv2 pinholes for IPv6; on by default, reported on the System page | built in the working tree, uncommitted; the IPv6 pinhole and port findings below apply |
| The outside probe and `reachableFrom` per candidate (C2) | built |
| LAN, IPv6 and IPv4 names with a wildcard certificate (C4) | built |
| W2c: the client races every path and moves to a better one (D273 §7) | planned |
| W4: signalling and STUN through the discovery | planned |
| W6: one TURN library; TURN on the core for voice (D243 §8) | planned / built |
| S2: the owner's single-target discovery with tunnel and TURN on a VPS | planned (reachability-approaches §7) |
| The System page's Discovery section with the claim code and the probe's findings | built |

### 6.2 New, highest value first

1. **N1. Fix IPv6 pinholes.** Open them by PCP over IPv6 (MAP with the core's global address, sent
   to the IPv6 gateway), and by UPnP over IPv6 (the request from the address opened); never ask
   for an internal port below 1024. Without this, automatic IPv6 opening fails on every miniupnpd
   router (OpenWrt, GL.iNet, pfSense, OPNsense, ASUS) [V/I]. Test against a miniupnpd in the
   integration tier (it is packaged in Debian) [I].
2. **N2. A high mapping port.** The core listens on a second, high HTTPS port (default chosen once
   and kept) used for mapping, pinholes and the names' published port, so stock router rules accept
   it; 443 stays for LAN and proxies. Configurable; the System page says which.
3. **N3. Candidate hygiene.** Classify 100.64.0.0/10 as shared space (overlay or carrier, never
   published as internet); skip known virtual interfaces (docker0, br-*, veth*, virbr*, tailscale0,
   wg* unless configured); publish only stable, non-deprecated IPv6 addresses; heartbeat at once
   when addresses change.
4. **N4. The reachability diagnosis on the System page** (the Plex/netcheck lesson). One verdict
   line plus facts, each with the fix that applies:
   - router found / method / external IPv4 → *private*: double NAT; *100.64/10*: carrier NAT;
     *192.0.0.x*: DS-Lite; *differs from the discovery's view*: another NAT or a VPN upstream;
   - IPv6: global address present? stable? the probe's per-family result, distinguishing
     *timeout* (firewall) from *refused* (reached, wrong port) from *TLS failure*;
   - NAT behaviour from two STUN answers (easy/hard) and the router-family hint;
   - running in a container (and which network mode), a tunnel as default route, overlays present;
   - hairpin: can the core reach its own mapped address;
   - UDP: did the probe's UDP check reach the TURN/WebRTC port.
   The discovery's probe needs per-family and UDP checks for this (a spec change for the service).
5. **N5. Guidance texts per situation and router family**, from §3.2, §3.3, §3.7 and §3.9: FRITZ!Box,
   OpenWrt/GL.iNet, Unifi, pfSense/OPNsense, Speedport, Vodafone Station, Xfinity, Starlink, Docker
   (host networking), Synology/QNAP, Unraid, Tailscale. Short, linked from the diagnosis line,
   written as steps; the user docs keep the long form.
6. **N6. The container defaults.** The shipped compose file uses host networking for the core
   (`network_mode: host`); a bridge-mode recipe publishes the HTTPS port, one UDP port for WebRTC
   (pion's single-port UDP mux) and TURN's, and sets the public addresses; pion's address rewrite
   maps the container address to the host's. The core detects bridge mode and says that automatic
   mapping is off in it.
7. **N7. Client-side diagnostics back to the core.** The client reports which path won, which
   failed and how (LAN name did not resolve → rebinding; Local Network Access denied; UDP failed),
   so the System page can list "recent connections by path" and turn a rebinding failure into the
   resolver's exemption text.
8. **N8. UDP mapping for WebRTC and TURN.** Map one UDP port (the WebRTC mux) and TURN's listen
   port by the same protocols; the TURN's public address follows the mapped or observed address
   instead of a static `turn.public_address`. Then IPv4-public homes need no hole punching at all.
9. **N9. TURN over TCP/TLS** on the reachable high port, for visitors on UDP-blocked networks
   (voice only; media goes over HTTPS there anyway).
10. **N10. Document the owner's VPS (S2) as the answer** for carrier NAT with IPv4-only visitors,
    linked from the diagnosis when it applies.
11. **Later:** native apps' raw-UDP punching and LAN discovery (D3); 1&1-style PCP ports, if PCP
    proxying through a FRITZ!Box turns out to work (needs a 1&1 tester).

## 7. The owner's network: FRITZ!Box 7490 + GL-MT6000, Vodafone DSL

**Known.** On 2026-09-24 the owner reported DS-Lite (the IPv4 shown is the DS-Lite tunnel's), native
IPv6 with a delegated /56, Vodafone DSL, 111.9/43.7 Mbit/s. The GL-MT6000 is the FRITZ!Box's only
client; its LAN is 192.168.1.0/24, the FRITZ!Box's 192.168.178.0/24. **[I]** Consequences: no
IPv4 path at all (and Vodafone DSL reportedly cannot be switched to dual stack [unverified]); IPv6
is the direct path; IPv4-only visitors need WebRTC or the owner's VPS (as staging does today).

### 7.1 Check first

1. **DS-Lite confirmed?** FRITZ!Box > Übersicht / Internet > Online-Monitor: the connection line
   says DS-Lite or "IPv4 über DS-Lite" and shows an IPv6 prefix. While there, look for a **PCP
   server** or "IPv4-Adresse im Internet" entry: present means Vodafone grants PCP ports (unlikely,
   not reported for Vodafone).
2. **Does the GL-MT6000 get a delegated prefix?** GL.iNet UI > Network > IPv6: mode **Native**.
   Then on the core (or any LAN machine): `ip -6 addr show scope global` should list a 2a02:… (or
   other global) address with the GL.iNet's LAN prefix; `ip -6 route` a default via the GL.iNet's
   link-local address. In LuCI > Status > Overview, the IPv6 upstream should show an
   "IPv6-PD" prefix. If the LAN has only fd…/ULA addresses, delegation is not happening (or NAT6
   is selected).
3. **Is the core's address stable?** `ip -6 addr show` on the core: the address the names use must
   not carry `temporary`; on Debian servers it usually does not. Note its interface ID (the last
   64 bits); under Docker bridge the core will not see it at all (use host networking).
4. **Outbound IPv6 works?** `curl -6 https://ifconfig.co` (or any IPv6 echo) from the core prints
   the same global address.

### 7.2 Get direct IPv6

1. **FRITZ!Box: delegate a prefix.** Heimnetz > Netzwerk > Netzwerkeinstellungen > IPv6-Einstellungen
   (shown with the advanced view on): DHCPv6-Server im Heimnetz aktivieren, **"DNS-Server, Präfix
   (IA_PD) und IPv6-Adresse (IA_NA) zuweisen"**. Then reconnect the GL-MT6000's WAN (or reboot it)
   and repeat check 2.
2. **FRITZ!Box: open the firewall for the GL-MT6000's prefix.** Internet > Freigaben > Portfreigaben
   > the GL-MT6000's entry (add it if missing) > tick **"Firewall für delegierte IPv6-Präfixe dieses
   Gerätes öffnen"**. This makes the GL-MT6000's own firewall the only gate.
3. **GL-MT6000: allow the core's port in.** In LuCI (GL.iNet UI > System > Advanced Settings) >
   Network > Firewall > Traffic Rules > Add: name `orpheum-https-v6`; protocol TCP; source zone
   `wan`; destination zone `lan`; destination address the core's **interface ID with a negative
   mask**, `::<last 64 bits>/-64`, so the rule survives prefix changes [unverified, OpenWrt forum;
   else the full address]; destination port the core's HTTPS port; restrict to IPv6; action accept.
   Save & Apply. Add the same for the TURN UDP port if voice should reach the core over IPv6.
4. **Test from outside** (phone on mobile data with IPv6, not Wi-Fi): `https://[<core IPv6>]:<port>/`
   answers (certificate warning expected for the bare address), and the System page's probe shows
   the IPv6 candidate reachable.
5. **Optional, automatic later:** once N1 is built, steps 3–4 can be done by the core itself: in
   LuCI > Services > UPnP IGD & PCP, enable the service, keep NAT-PMP/PCP on, untick **IGDv1
   compatibility** (so IGDv2 and its IPv6 firewall service are advertised), and keep secure mode.
   The FRITZ!Box step 2 stays manual.

**Simpler alternative** (if the GL-MT6000's router features are not needed): GL.iNet > Network >
Network Mode > **Access Point**. The FRITZ!Box then sees the core directly: Internet > Freigaben >
add the core > allow independent port sharing, or an IPv6 sharing for its interface ID with the
port. One NAT, one firewall, and the FRITZ!Box's PCP/UPnP works for the core [unverified for PCP
IPv6].

### 7.3 IPv4

None direct: DS-Lite has no public IPv4 for any port to land on, and the GL-MT6000's miniupnpd
would refuse mapping anyway (private WAN address). IPv4-only visitors come in by **WebRTC** (W4)
when their NAT punches, otherwise by **the owner's VPS** (tunnel + TURN, as staging today). Asking
Vodafone for dual stack costs nothing to try (goodwill, reported to work on cable, reported
impossible on resold DSL [unverified]).

### 7.4 At home

Both routers run DNS rebinding protection: the GL.iNet's dnsmasq (rebind protection on by
default in OpenWrt) and the FRITZ!Box behind it. The **IPv6 name** works at home without changes
(global addresses are not filtered). For the 192-168-1-… LAN name: LuCI > Network > DHCP and DNS
> Filter > add the names' zone under "Domain whitelist" (`rebind_domain`), and on the FRITZ!Box add
the zone to the DNS-Rebind-Schutz exceptions, since the GL.iNet forwards to it [I]. If the
FRITZ!Box's default parental-controls profile is restrictive, IPv6 from behind the GL-MT6000 may be
filtered: set it to unrestricted [unverified].

## 8. Open questions for the owner

1. **May the core listen on a second, high port for the mapped path** (N2), with names carrying
   that port, or must every public URL be on 443?
2. **Should the shipped compose file use host networking** for the core (N6)? It makes every
   automatic step work and costs port isolation.
3. **The discovery's probe grows per-family, UDP and STUN behaviour checks** (N4): a change to the
   service's spec (draft 3, §3.4). Agreed?
4. **Should candidates on overlays (Tailscale 100.x) be published at all**, with an overlay scope
   the client tries only when it is on the same tailnet, or dropped?
5. **The owner's own network:** do you want to try the FRITZ!Box delegation + GL-MT6000 rule
   (§7.2), or switch the GL-MT6000 to access-point mode? Either gives a real-world test of the IPv6
   path before the beta.

## Sources

Checked 2026-10-10; raw copies and notes under `/yoloai/cache/http/homelab-networks/`.

- miniupnpd: [miniupnpd.conf](https://github.com/miniupnp/miniupnp/blob/master/miniupnpd/miniupnpd.conf)
  (`ext_allow_private_ipv4`, `ext_perform_stun`, `secure_mode`, `force_igd_desc_v1`);
  [upnpsoap.c `PinholeVerification`](https://github.com/miniupnp/miniupnp/blob/master/miniupnpd/upnpsoap.c);
  [issue 731, AddPinhole 606](https://github.com/miniupnp/miniupnp/issues/731).
- OpenWrt: [upnpd.config](https://github.com/openwrt/packages/blob/master/net/miniupnpd/files/upnpd.config),
  [miniupnpd.init](https://github.com/openwrt/packages/blob/master/net/miniupnpd/files/miniupnpd.init),
  [Makefile](https://github.com/openwrt/packages/blob/master/net/miniupnpd/Makefile),
  [packages PR 28765](https://github.com/openwrt/packages/pull/28765), [LuCI PR 7407](https://github.com/openwrt/luci/pull/7407),
  [dhcp.conf (rebind_protection)](https://github.com/openwrt/openwrt/blob/main/package/network/services/dnsmasq/files/dhcp.conf);
  [suffix rules with /-64](https://forum.openwrt.org/t/solved-firewall-dest-ipv6-xxx-xxx-xxx-64-not-possible/55644).
- dnsmasq: [man page](https://thekelleys.org.uk/dnsmasq/docs/dnsmasq-man.html) (`--stop-dns-rebind`, `--rebind-domain-ok`).
- GL.iNet: [IPv6 interface guide](https://docs.gl-inet.com/router/en/4/interface_guide/ipv6/);
  [MT6000 firmware lines](https://forum.gl-inet.com/t/confusing-firmware-versions-flint-2-gl-mt6000/43817).
- AVM / FRITZ!Box: [automatic port sharing KB](https://fritz.com/service/wissensdatenbank/dok/FRITZ-Box-7590/894_Automatische-Portfreigaben-in-FRITZ-Box-einrichten/) (snippet only; the page did not render);
  [delegated-prefix firewall](https://www.computerbase.de/forum/threads/ds-lite-myfritz.1925757/page-2);
  [UDM Pro behind a FRITZ!Box (IA_PD)](https://nerdig.es/udm-pro-ipv6/);
  [PD and parental controls](https://v64.tech/t/fritz-box-ipv6-prefix-delegation-administratively-prohibited/1958);
  [IPv6 interface ID and prefix changes](https://help.nextcloud.com/t/fritz-box-ipv6-port-freigabe-was-soll-man-bei-ipv6-interface-id-eintragen/184210);
  [7490 updates](https://winfuture.de/news,154418.html).
- PCP at DS-Lite ISPs: [heimnetz.de thread](https://forum.heimnetz.de/threads/fritz-box-pcp-portfreigaben-beim-provider-einrichten.203/);
  [glasfaserforum thread](https://www.glasfaserforum.de/forum/thread/2482-wireguard-via-ipv4-an-ds-lite-anschluessen-mittels-pcp-providerabhaengig/).
- Vodafone: [UPnP on the Vodafone Station](https://www.vodafone.de/featured/article/upnp-aktivieren-vodafone-179510);
  [dual stack on request](https://forum.vodafone.de/t5/Ger%C3%A4te/Umstellung-auf-Dual-Stack-IPv4-%C3%B6ffentliche-Adresse-f%C3%BCr-FritzBox/td-p/3307604);
  [no forced disconnect on DSL](https://forum.vodafone.de/t5/St%C3%B6rungen-im-DSL-Netz/DSL-Zwangstrennung-alle-24-Stunden/td-p/3307287).
- Telekom: [Speedport and IPv6 forwarding](https://telekomhilft.telekom.de/conversations/festnetz-internet/portfreigabe-ipv6/66bbc074389f9f597b98095c).
- BT: [Smart Hub pinholes](https://community.bt.com/t5/BT-Devices/IPv6-Pinholes-and-conflict-with-IPv4-Firewall-on-SmartHub/td-p/2004009).
- Xfinity: [UPnP on the gateway](https://forums.xfinity.com/conversations/your-home-network/how-do-i-keep-upnp-on-my-gateway-always-on/6820a83fcfe77238ea27b567).
- Starlink: [bypass mode and IPv6](https://www.packetville.net/docs/starlink/starlink-ipv6-self-hosting/) (via direct-reachability).
- Tailscale: [How NAT traversal works](https://tailscale.com/blog/how-nat-traversal-works);
  [firewalls](https://tailscale.com/kb/1181/firewalls); [pfSense](https://tailscale.com/docs/integrations/firewalls/pfsense).
- pfSense: [bug 15448](https://redmine.pfsense.org/issues/15448) (title only).
- Unifi: [Plex forum, rebinding allowlist](https://forums.plex.tv/t/unifi-gateway-plex-remote-access-dns-rebinding-protection-whitelist/936735).
- Plex: [troubleshooting remote access](https://support.plex.tv/articles/200931138-troubleshooting-remote-access/) (double NAT, CGNAT, probe workers, xFi);
  [secure connections and rebinding](https://support.plex.tv/articles/206225077-how-to-use-secure-server-connections/).
- Pi-hole: [rebinding default](https://discourse.pi-hole.net/t/add-plex-direct-to-dns-rebinding/74780).
- Firefox: [Trusted Recursive Resolver](https://wiki.mozilla.org/Trusted_Recursive_Resolver).
- WebRTC mDNS candidates: [discuss-webrtc PSA](https://groups.google.com/g/discuss-webrtc/c/6stQXi72BEU).
- pion: [ice package (TCP mux, NAT 1:1)](https://pkg.go.dev/github.com/pion/ice);
  [address rewrite rules, livekit 4487](https://github.com/livekit/livekit/issues/4487).
- Docker: [IPv6](https://docs.docker.com/engine/network/ipv6/) and [port publishing](https://docs.docker.com/engine/network/port-publishing/) (docs repo copies cached).
- TrueNAS: [24.10 release notes](https://www.truenas.com/docs/scale/24.10/gettingstarted/scalereleasenotes/).
  Unraid: [6.12.4 notes on macvlan/ipvlan](https://docs.unraid.net/unraid-os/release-notes/6.12.4/).
- Syncthing: [firewall setup](https://docs.syncthing.net/users/firewall.html).
  Home Assistant Cloud: [SniTun](https://github.com/NabuCasa/snitun).
  Nextcloud AIO: [readme](https://github.com/nextcloud/all-in-one) (ports, Talk TURN, Cloudflare notes, domain check, local DNS).
