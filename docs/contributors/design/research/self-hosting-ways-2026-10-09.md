> **ABOUTME:** The ways self-hosters expose a media server (port-forward, IPv6, reverse proxies,
> tunnels, mesh VPNs), how common each is, what each can carry (HTTPS, WebSocket, TURN over UDP),
> who holds the certificate, which host platforms run the node, and what Orpheum lacks for each.
> Answers the owner's closed-beta question (2026-10-09); recommends a support tiering.

# Self-hosting: the ways hosters expose a server, and what Orpheum must allow

Researched **2026-10-09**. The owner: *"As for self hosting, what are the different ways to do it? We
should allow for the common ways a hoster would want to do things."* Paid discovery
(app.orpheum-app.com, D214) is a separate product and out of scope here.

Builds on, and does not repeat: [beta-inventory-2026-10-09.md](beta-inventory-2026-10-09.md) §(c)
(what the owner's setup hardcodes), [direct-reachability.md](direct-reachability.md) §5–§6 (which
server/client topologies need a third party; VPS prices; Funnel and Cloudflare terms),
[remote-access.md](remote-access.md) §4 (HTTPS on the LAN needs a third party; DNS rebinding),
`deploy/vps/README.md`, `deploy/release/README.md`, `internal/deployment/templates/`, and D193
§4–§5, D243 §6–§8, D249, D272.

**Legend.** **[1P]** the vendor's or project's own words, docs, source or config.
**[J]** a third party (survey, forum, blog). **[I]** my inference. **[unverified]** seen only in a
search snippet or secondary source.

**Sources** (cached under `/yoloai/cache/http/self-hosting/`; summaries in `*.summary.md`):

- selfh.st 2025 Self-Host User Survey, 4,081 responses, raw JSON
  ([results](https://selfh.st/survey/2025-results/),
  [data](https://raw.githubusercontent.com/selfhst/cdn/refs/heads/main/assets/surveys/annual/2025-results.json)).
  Counts in `selfhst-2025-selected.txt`.
- deployn.de [Self-Hosted Survey 2025](https://selfhosted-survey-2025.deployn.de/), 850+ responses.
- Jellyfin docs (repo `jellyfin/jellyfin.org`, cloned to `/yoloai/cache/jellyfin.org`): networking,
  reverse proxy, Tailscale, community standards.
- Plex support: [remote access](https://support.plex.tv/articles/200289506-remote-access/),
  [troubleshooting](https://support.plex.tv/articles/200931138-troubleshooting-remote-access/),
  [relay](https://support.plex.tv/articles/216766168-accessing-a-server-through-relay/).
- Cloudflare [service-specific terms](https://www.cloudflare.com/service-specific-terms-application-services/)
  and [Tunnel protocols](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/routing-to-tunnel/protocols/);
  Tailscale [Funnel](https://tailscale.com/kb/1223/funnel); Synapse
  [TURN how-to](https://element-hq.github.io/synapse/latest/turn-howto.html); Let's Encrypt
  [challenge types](https://letsencrypt.org/docs/challenge-types/); lego README (DNS providers);
  Pangolin source (`fosrl/pangolin`); the [Public Suffix List](https://publicsuffix.org/list/public_suffix_list.dat).
- Kernel configs: Raspberry Pi `rpi-6.12.y` `bcm2711/2712_defconfig`; Debian's
  `debian/config/config`; TrueNAS `scripts/package/truenas/debian_amd64.config` (linux-6.18);
  Synology kernels per platform ([007revad wiki](https://github.com/007revad/Synology_Information_Wiki/blob/main/pages/Linux-Kernel-in-each-platform-arch.md)).
- GitHub stars and last push, 2026-10-09 (`github-stars-2026-10-09.txt`).

Search budget used: 9 web searches, plus direct fetches. Both surveys draw on r/selfhosted-style
enthusiasts, who are more technical than the average Plex owner [I]. Treat their shares as "what
our beta testers will already run", not as the whole market.

---

## Summary

1. **The hoster's own reverse proxy is the norm.** In selfh.st 2025, 67% of respondents reach their
   services remotely through a reverse proxy and 71% through a VPN. Only 18% forward ports straight
   to an app. The proxies are Nginx Proxy Manager, Traefik, Caddy, plain nginx and Cloudflare Tunnel,
   at 13–21% each [J]. **Orpheum cannot run behind one today.** The deployment requires the tunnel,
   the core trusts no forwarded headers, and "has a public name" is the same switch as "obtain a
   certificate by Cloudflare DNS-01". This is the largest gap for a beta.
2. **TURN is what separates the methods.** HTTPS, WebSocket and HLS pass through anything that carries
   HTTP. Voice's relay needs UDP 3478 and a port range, and it needs the packets to arrive **with
   their real source address**. The relay-to-relay hop is checked against a permission that names the
   public address: `deploy/vps/orpheum.nft` SNATs it to the VPS address for exactly this reason. So
   only a layer-3 forward (a router port-forward, our WireGuard + DNAT, or IPv6) carries Orpheum's
   relay. Userspace forwarders do not: frp, rathole, Pangolin, and the UDP streams of Traefik, nginx
   or Nginx Proxy Manager all rewrite the source [I, from RFC 8656's permission check and our own nft
   rule]. Cloudflare Tunnel and Tailscale Funnel carry no UDP at all [1P].
3. **Video through Cloudflare Tunnel is off the table**, by Cloudflare's CDN terms [1P]. Jellyfin's
   community rules forbid even suggesting it [1P]. Yet 14% of selfh.st respondents list Cloudflare
   Tunnel as their proxy [J], so testers will try it, and the docs must say no and why.
4. **Certificates.** 88% of respondents have their own domain [J], and Cloudflare is the registrar
   of about a third of them. Namecheap, Porkbun, OVH and DuckDNS follow. A hoster behind a proxy
   wants Orpheum to **not** obtain a certificate. A hoster without one wants HTTP-01/TLS-ALPN-01 (no
   DNS API), a DNS provider other than Cloudflare, or a certificate file someone else renews.
5. **Platforms.** Docker (89%) and Compose (88%) are universal [J]. The host underneath is often
   **Proxmox (45%)**, Raspberry Pi OS (24%), TrueNAS (19%), Unraid (10%) or Synology DSM (10%) [J].
   Of these, only TrueNAS SCALE and plain Debian/Ubuntu ship Landlock enabled. Proxmox needs a kernel
   command-line edit. Raspberry Pi OS's kernel is built without Landlock. Synology and QNAP kernels
   predate it [1P configs]. With `confinement_policy = "require"` hardcoded in the deployment, the
   node refuses to start on most of the platforms the surveys list.

**Recommended for the closed beta** (§5): **must**: tunnel-optional deployment (port-forward, IPv6),
TLS-terminating reverse proxy with trusted-proxy handling, the certificate decoupled from the name
(none / DNS-01 / HTTP-01 / file), keep our VPS + WireGuard path but scripted, and a Landlock answer.
**Should**: more DNS-01 providers through CNAME delegation, SNI passthrough, a documented LAN HTTPS
recipe. **Later**: external TURN, TURN over TLS on 443, UPnP/PCP, Kubernetes, NAS app-store listings.

---

## 1. How hosters actually expose services

selfh.st 2025, n = 4,081, multi-select; % of respondents [J]:

| Question | Answers |
| --- | --- |
| Remote access | VPN 71%, reverse proxy 67%, forwarded ports 18%, LAN only 12.5% |
| Reverse proxy (4,023 answers; % of answers) | Nginx Proxy Manager 21%, Traefik 19%, Caddy 15%, **Cloudflare Tunnels 14%**, nginx 13%, Pangolin 7%, HAProxy 3%, Apache 2%, SWAG 2%. The nginx family together: 36% |
| Own domain | 88% |
| Registrar | Cloudflare 35%, Namecheap 16%, Porkbun 13%, OVH 6%, GoDaddy 3%, DuckDNS 3%, Netcup/IONOS/Hetzner 2% each. The survey asked for the registrar, not the DNS host, and notes that Cloudflare DNS is "also popular" [1P] |
| Hardware | low-powered device 50%, consumer 58%, **cloud VPS 34%** |
| Users served (of personal setups) | 1–2 users: 49% |
| Media server | Jellyfin 61%, Plex 30%, Emby 4% |

deployn 2025, n ≈ 850 [J]: exposure by reverse proxy 68.5%, Tailscale 33.6%, Cloudflare Tunnel
22.3%, direct port forward 10.6%. Hetzner leads VPS providers at 26%. Pi-hole (34%) and AdGuard Home
(32%) are common, and both do local DNS rewrites, which is split-horizon DNS by another name [I].

What the media servers tell their users:

- **Jellyfin** serves plain HTTP on 8096 and recommends **Caddy** in front of it. Its docs say: "the
  IP address(es) of your reverse proxy must be configured under 'Known Proxies'". Forwarded headers
  from anything else are discarded "so that malicious devices will not be able to hide their IP
  address". WebSockets must be allowed through the proxy. Its docs present Tailscale as the answer to
  CGNAT, and warn that "Devices like Google Chromecast or Google TV Streamer may ignore the DNS server
  provided by your local network" [1P]. UPnP mapping left core for a plugin (PR #4543) [1P].
- **Plex** tries UPnP/NAT-PMP first, then a manual forward to 32400. It detects double NAT and CGNAT
  (100.64/10) and tells the user to bridge. Otherwise it falls back to its Relay, "limited to 2 Mbps
  maximum for streams" even for paying users [1P].

GitHub stars on 2026-10-09 [1P]: frp 110k, Caddy 78k, Traefik 65k, Headscale 44k, Nginx Proxy
Manager 34k, NetBird 30k, Pangolin 23k (and rising fast), cloudflared 16k, rathole 14k, SWAG 3.7k.
boringproxy has had no push since 2024-07: dormant.

## 2. The methods compared

"TURN?" asks whether voice's relay can work through the method as Orpheum builds it today (UDP 3478
plus a relay range, source address preserved).

| Method | How common [J] | Skill | Needs from Orpheum | TLS: who, how | TURN? | Behind CGNAT / DS-Lite? | Cost |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **A. IPv4 port-forward + DDNS**, Orpheum on 443 itself | 18% forward ports | low–medium | tunnel-less deployment; publish 443, 3478 TCP+UDP and the relay range; HTTP-01/TLS-ALPN-01 or DNS-01 | Orpheum (ACME) | **yes**. Relay-to-relay needs the router's hairpin to keep the public source [I] | **no** | €0 + domain (or free DDNS name) |
| **B. Inbound IPv6** (AAAA + router rule) | low; often a fallback on DS-Lite | medium | as A, plus an IPv6 TURN address and an AAAA kept current (prefix changes) | Orpheum | v6 clients only; TURN's public address is IPv4-only today | **yes, for IPv6 clients** (direct-reachability §6 R2) | €0 |
| **C. UPnP / NAT-PMP / PCP** | Plex's default; Jellyfin moved it out of core | none | a port-mapping client in the core (new dependency or own code) | Orpheum | yes, if the router maps UDP | **no** (PCP on DS-Lite's AFTR is rare) | €0 |
| **D. Hoster's reverse proxy terminates TLS** (Caddy, Traefik labels, NPM, SWAG, nginx, HAProxy), same host or another | **67% use a proxy**; the most common setup | medium | plain-HTTP upstream (exists: :8440); a public name without our ACME; **trusted proxies** for X-Forwarded-For/Proto; WebSocket through it (the proxy's job); TURN ports forwarded *beside* the proxy | the proxy (its own ACME, often a wildcard by DNS-01) | only if UDP 3478 and the range are forwarded to Orpheum at layer 3 beside the proxy. A UDP "stream" through the proxy rewrites the source [I] | no, unless the proxy sits on a VPS (then see F/G) | €0 |
| **E. Reverse proxy, SNI passthrough** (Traefik TCP `HostSNI` passthrough, nginx `ssl_preread`, HAProxy `mode tcp`, caddy-l4) | uncommon, for hosters who want end-to-end TLS [I] | high | Orpheum's own cert; PROXY protocol to learn the client address | Orpheum | as D | as D | €0 |
| **F. Own VPS + WireGuard, layer-3 DNAT** (ours, D243 §7) | VPS: 34% have one; this exact shape is rare [I] | high today (≈15 root steps) | exists; needs a script, configurable addresses, any DNS provider | Orpheum on the home side (end-to-end) | **yes**, the only tunnel that is | **yes** | €0–6/month (direct-reachability §5) |
| **G. Own VPS + userspace tunnel**: frp, rathole, Pangolin (Newt + Gerbil + Traefik), Tailscale/Headscale node + proxy on the VPS | Pangolin 7% and rising; frp has the most stars | medium (Pangolin: one installer) | as D (TLS at the VPS proxy) or E (TCP passthrough) | VPS proxy or Orpheum | **no**, as built: they carry UDP ports but rewrite the source [I]. Would work with TURN on the VPS (§4.6) | **yes** | €0–6/month |
| **H. Cloudflare Tunnel** | 14% (selfh.st), 22% (deployn) | low | as D | Cloudflare (it decrypts) | **no**: HTTP only to browsers; TCP needs `cloudflared` on the client; no UDP [1P] | yes | €0, but **video is against the terms** [1P] |
| **I. Tailscale Funnel** | small [I] | low | as D | tailscaled on the host; `*.ts.net` names only [1P] | **no**: TLS/TCP on 443, 8443, 10000 only [1P] | yes | €0, "non-configurable bandwidth limits" [1P] |
| **J. Mesh VPN for viewers** (Tailscale, Headscale, NetBird, ZeroTier) | 71% use some VPN | low for the hoster, an app on every viewer device | nothing beyond LAN mode; LAN secure-context problem (§4.7) | none, or `tailscale cert` | not needed inside the mesh [I] | yes | €0 |
| **K. ngrok / bore / localtonet** | rare for media [I] | low | as D | the service | no (HTTP/TCP) | yes | free tiers are capped [unverified] |

**What the table says** [I]:

- **D (proxy) is what most testers will try first**, and it costs Orpheum the least: trusted proxies
  and a certificate-less public name. Voice then needs the TURN ports forwarded beside the proxy,
  which works for every hoster whose proxy is at home on a public IPv4. It fails for one who is
  behind CGNAT and reaches their proxy through a tunnel (G, H).
- **F is the only path that gives a CGNAT/DS-Lite host voice through the relay without changes to
  the TURN design.** That is why D243 §7 chose it. Keep it, and make it scripted.
- **G (Pangolin, frp, rathole) is where the CGNAT crowd is going**, and it carries everything except
  the relay. A browser pair that does not need the relay still connects. The relay is needed only for
  two hard NATs or blocked UDP (remote-access.md §3).
- **H and I are documented "no"s.**

## 3. Certificates and names

- **Let's Encrypt's three challenges** [1P]: HTTP-01 "can only be done on port 80" and cannot issue
  wildcards. TLS-ALPN-01 works "purely at the TLS layer" on 443 and is "best suited to authors of
  TLS-terminating reverse proxies", which Orpheum is. DNS-01 issues wildcards and works with no
  inbound port, but "putting your full DNS API credentials on your web server significantly increases
  the impact if that web server is hacked". It follows CNAMEs, so `_acme-challenge.media.example.com`
  can be delegated to another zone, as acme-dns does.
- **Fit to the methods** [I]. A and F can use TLS-ALPN-01 on the 443 they already forward, with no
  DNS provider at all. `golang.org/x/crypto/acme`, which is already allowed, speaks it. HTTP-01 needs
  one more forwarded port. DNS-01 is the only option for B behind a firewall the CA cannot reach,
  for a name not yet pointed at the host, and for wildcards. D, G, H and I need no certificate from
  Orpheum.
- **DNS providers.** lego (MIT, ~9.9k stars) supports ~194 DNS providers, including every registrar
  above plus deSEC, DuckDNS, Route 53, Hetzner, RFC 2136, acme-dns and an `exec` hook [1P]. It is a
  large dependency tree, though, and only `golang.org/x/crypto` is allowed today (`deps/go.allow`;
  rule 3 makes adding it the owner's call). The cheaper route [I]: keep the provider code on the
  standard library, as Cloudflare's is (D243 §6), add **CNAME delegation** (any registrar, one
  supported zone), and add two or three providers with trivial HTTP APIs (deSEC, DuckDNS, Porkbun or
  Hetzner DNS).
- **Free names.** `duckdns.org`, `dedyn.io` (deSEC), `dynv6.net`, `ddns.net`, `synology.me`,
  `myqnapcloud.com` and `ts.net` are on the Public Suffix List [1P]. Let's Encrypt's per-domain limits
  therefore count each subdomain separately. DuckDNS and deSEC set TXT records by API, so DNS-01
  works without buying a domain.
- **Bring your own certificate**, as a fullchain and key file pair reloaded when it changes, serves
  every hoster whose certificate tool already runs: SWAG, certbot, acme.sh, Traefik's exported certs,
  a NAS's built-in Let's Encrypt.

## 4. What Orpheum must support, and what it lacks

Per method, beyond the gaps beta-inventory §(c) already lists.

### 4.1 Tunnel-less deployment (A, B, D, E): decided by D272 §3, not built

- `write-deployment` needs a mode without `[vps]`. In it, the core publishes 443, 3478 TCP+UDP and the
  relay range on the host, or uses host networking. It also needs a TURN listen address that is not
  `10.77.0.2`, and a TURN public address that is the line's own IPv4 (and IPv6, §4.5) rather than the
  VPS's (`templates/core.toml:28`, `values.go:116-135`).
- **[I] Risk.** On a plain port-forward, a relayed packet to another allocation on the same server
  goes to the router's public address and must hairpin back with that address as its source, which
  the other allocation's permission names. Many routers rewrite hairpinned packets to their LAN
  address. That cell (both browsers forced to relay) needs a test on a real router, or the relay
  short-circuiting its own allocations.

### 4.2 Behind a reverse proxy (D, G, H-like): new

- **A public name independent of certificate issuance.** Today `https.domain != ""` both names the
  server and turns on ACME and the 443 listener (`cmd/orpheum/config.go:65`). A proxied hoster needs
  the name (for the System page's reachability check, and for links and QR codes) without either.
- **Trusted proxies**: a list of addresses or CIDRs from which `X-Forwarded-For` / `X-Forwarded-Proto`
  (and RFC 7239 `Forwarded`) are believed, and ignored from anyone else, as Jellyfin's Known Proxies
  and Traefik's `forwardedHeaders.trustedIPs` work [1P]. Today the core takes `RemoteAddr`
  (`internal/core/api/session.go:84-88`). Behind a proxy every client then has the proxy's address,
  with these effects [I]:
  - the sign-in rate limit keyed by address (`internal/core/auth/auth.go:212-221`) locks out
    everyone together;
  - D257's link speed is remembered per client network, so it is shared by all clients;
  - now-playing shows the proxy;
  - `Secure` cookies follow `ctx.TLS() != nil` (`media_cookie.go:29-45`) and are never set, though
    the browser is on HTTPS.
- **PROXY protocol** on the TLS listener for E (SNI passthrough), where no header can carry the client
  address.
- **Docs per proxy**: Caddy, Traefik labels, Nginx Proxy Manager and SWAG at least (57% of proxy
  answers; 70% with plain nginx). Each needs WebSocket upgrade for Watch Together, no response
  buffering for HLS, a long read timeout for the session socket, and the TURN ports forwarded beside
  it.

### 4.3 Certificates (A, B, E, F): extend

Certificate mode as a value: `none` (proxied), `acme-dns01` (provider plus token, or CNAME
delegation), `acme-tls-alpn01` / `acme-http01`, or `files` (paths, reloaded). The deployment's default
name `media.orpheum-app.com` must become a required blank (beta-inventory §(c)).

### 4.4 Own VPS (F): exists, hand-built

Script it: a cloud-init or one-command install that writes `vps-wg0.conf`, `orpheum.nft` and the
units, as direct-reachability option B suggests. Make the tunnel addresses values. Document providers
other than Hetzner (firewall UI steps differ). Pangolin shows the bar hosters now expect: one
installer, a web UI, automatic certificates [1P README] [I].

### 4.5 IPv6 (B)

TURN's public address is IPv4-only (`values.toml:22-23`). An IPv6-reachable host also needs its AAAA
record kept current as the prefix changes [I]. The core already holds a DNS API token, so a DDNS
updater is a small step, but it is new behaviour.

### 4.6 TURN for hosters who cannot forward UDP at layer 3 (G, D on CGNAT): later

Two known shapes [I]:

- **External TURN.** A coturn on the VPS or the router host, with the TURN REST shared secret, as
  Synapse (`turn_shared_secret`) and Nextcloud Talk do. Synapse's own guide warns that TURN behind NAT
  "is known to cause issues and to often not work" [1P]. Running it at the edge avoids that. It
  changes D243 §8's per-membership credentials into time-limited ones.
- **TURN over TLS on 443**, multiplexed by SNI/ALPN, so it rides any TCP path. The relay ports would
  still need the public source, or, when every client relays through the same server, an internal
  relay address. That last option collides with `internal/core/turn/relay.go:116`, which relays only
  to public addresses on purpose. Either shape is a design change (D193 §8 freeze): owner question 4.

### 4.7 LAN access and secure context (all methods)

`getUserMedia` (voice) and `crypto.randomUUID` need a secure context. The LAN path is plain HTTP on
:8440 (idiosyncrasies; the `insecure-context` notes). The workable recipes [I]:

- **Split-horizon DNS**: the public name resolves to the host's LAN address, through a Pi-hole or
  AdGuard rewrite (a third of hosters run one [J]) or the router's DNS. Orpheum must then serve its
  certificate on the LAN too, which proxied hosters already do.
- **Router hairpin**: works on routers that support it, but not over DS-Lite's IPv4.
- **Through the VPS (F)**: needs no setup, but a film watched at home then crosses the home uplink
  twice [I].

Cast devices that ignore the LAN's DNS [1P Jellyfin] resolve the public name to the VPS and take that
path anyway. A public A record pointing at a private address is blocked by many routers' rebind
protection (remote-access.md §4). The docs should give the split-horizon recipe and say what each
fallback costs.

### 4.8 Platforms

| Platform | Survey share [J] | Runs our compose? | Landlock | Notes |
| --- | --- | --- | --- | --- |
| Debian / Ubuntu + Docker | Linux 81% | yes | **yes**: Debian `CONFIG_LSM="landlock,…"` [1P] | the reference |
| Proxmox VE (LXC or VM) | 45% | yes (LXC: nesting, keyctl) | **off by default**: `CONFIG_LSM` lacks it; needs `lsm=` (idiosyncrasies; `deploy/release/README.md` §1) | the owner's own |
| TrueNAS SCALE | 19% | yes: apps are Docker Compose since 24.10 [unverified]; custom YAML app | **yes**: its kernel config carries Debian's LSM list with landlock [1P] | |
| Raspberry Pi OS (arm64) | 24% | needs the arm64 image (D205: MKVToolNix amd64 only) | **no**: `bcm2711/2712_defconfig` has no `CONFIG_SECURITY_LANDLOCK`, `CONFIG_LSM=""` [1P]. Ubuntu for Pi differs [I] | |
| Unraid | 10% | via Community Apps template or compose plugin | **unknown**, reportedly absent [unverified] | |
| Synology DSM 7.2 | 10% | Container Manager projects (compose) | **no**: kernel 4.4.302 or 5.10.55; Landlock needs 5.13 [J] | no kernel WireGuard, so F's tunnel container fails [unverified] |
| QNAP QTS | small | Container Station | **no**: 5.10-era kernels [unverified] | |
| Kubernetes / k3s | 23% orchestrate; 9.6% k8s (deployn) | needs manifests; the UDP range needs hostNetwork or a LoadBalancer [I] | host's | later |
| Bare binary + systemd | 4% bare metal only | not packaged | host's | later |

`confinement_policy` already accepts `"report"` (`cmd/orpheum/config.go:538`). The deployment
hardcodes `"require"` (`templates/node.toml:11`) and the values file does not expose it.

## 5. Recommended tiering for the closed beta

**Must** (testers cannot install, or the first common setup fails):

1. **Tunnel-less deployment** (A/B): D272 §3, already decided. It is the precondition for every
   method but F.
2. **Behind a TLS-terminating reverse proxy** (D): certificate mode `none`, a public name apart from
   ACME, trusted proxies feeding the client address and the `Secure` decision, and recipes for Caddy,
   Traefik, NPM and SWAG with the TURN ports beside the proxy. Two thirds of testers already run one.
   Without trusted proxies the rate limit, D257's speed memory and now-playing are wrong in a way
   nobody will diagnose.
3. **A certificate path without Cloudflare**: TLS-ALPN-01 on the 443 the hoster already forwards (no
   new dependency), and certificate files. Cloudflare DNS-01 stays.
4. **F scripted, not hand-built**: the only path that gives CGNAT/DS-Lite hosts the relay. Its
   addresses become values. It stays the documented CGNAT answer.
5. **A Landlock decision**, since the node refuses to start on Proxmox (by default), Raspberry Pi OS,
   Synology and QNAP. Either the deployment exposes `report` with a plain warning, or the docs list
   these platforms as unsupported and give Proxmox's fix. This one is the owner's (D112).
6. **Docs that say no**: Cloudflare Tunnel (the terms) and Funnel (bandwidth, no UDP), with what to
   use instead.

**Should** (common, cheap once the musts exist):

- DNS-01 by **CNAME delegation** plus two or three providers on the standard library (deSEC, DuckDNS,
  Porkbun or Hetzner): free names and the non-Cloudflare registrars.
- **SNI passthrough with PROXY protocol** (E), for hosters who keep TLS end to end, and for Pangolin
  or frp with TCP passthrough.
- **A LAN HTTPS recipe** (split-horizon), and a System-page hint when the page is opened over plain
  HTTP.
- **IPv6 TURN address and AAAA upkeep** (B), for DS-Lite hosts whose viewers have IPv6.
- **Recipes for G** (Pangolin, frp, rathole) that say plainly that voice works without the relay but
  not through it.

**Later** (design changes, or few hosters):

- External TURN (coturn, shared secret) and TURN over TLS on 443 (§4.6): they open voice's relay to G
  and to CGNAT proxy setups, but change D243 §8.
- UPnP/NAT-PMP/PCP: useless on CGNAT, a security smell to the hosters surveyed, and Jellyfin moved it
  out of core.
- Kubernetes manifests, NAS app-store listings (Unraid CA, TrueNAS catalogue), a bare-binary/systemd
  package, and a published arm64 image. These wait on the registry (D272 §4).
- lego-scale DNS coverage, if the CNAME route proves insufficient.

## 6. Questions for the owner

1. **Who is the beta for?** Hosters who already run a proxy and a domain (most of the surveyed),
   households behind CGNAT who need our VPS path, or both? This sets whether the proxy path or the
   VPS script comes first.
2. **Does Orpheum own TLS, or may it hand TLS to the hoster's proxy?** Handing it over means
   trusting forwarded headers from configured addresses. D243 §6 assumed Orpheum owns the name's
   certificate.
3. **Confinement on hosts without Landlock.** Is a node that runs FFmpeg with seccomp and read-only
   mounts but no Landlock acceptable on a hoster's own box, with a visible warning? Or is such a host
   unsupported? D193 §4 ranks Landlock "where the kernel has it". The deployment says "require".
4. **Is voice's relay part of the promise for every method?** If yes, methods G and CGNAT-behind-proxy
   need a different TURN shape (external TURN, or TURN over 443), which is a design change. If no, the
   beta says "the relay works with port-forward, IPv6 and our VPS path; elsewhere, voice works when the
   two browsers can reach each other".
5. **Dependencies for names.** Is lego (MIT, about 194 DNS providers, a large tree) acceptable, or do
   we stay on the standard library with CNAME delegation and a handful of providers?
6. **LAN.** Should the beta require HTTPS at home (split-horizon) for voice, or is "use the public
   name; at home it goes through your VPS" acceptable for now, at the cost of the uplink?

## Not verified

- Unraid's and QNAP's kernel Landlock state; Synology's lack of kernel WireGuard. Each needs a box or
  a published config.
- That consumer routers' UDP hairpin rewrites the source (§4.1). This is inference, and testable with
  any FRITZ!Box or OpenWrt router.
- That userspace UDP forwarders break the relay (§2). This is inferred from RFC 8656's permission
  check and our own nft SNAT rule. A test with frp and a forced relay would settle it.
- ngrok's and similar services' current free-tier caps.
- How many of Plex's and Jellyfin's non-enthusiast owners run a proxy. The surveys reach enthusiasts
  only.
