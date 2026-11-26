> **ABOUTME:** Research, a proposed design and a cost breakdown for an official relay: one run
> by us for paid orpheum.cc households, capped per household, closed to everything but a
> household's core and its own clients. It also covers a household's own relay, used standalone or
> registered with orpheum.cc and then uncapped. Read it before reopening D273 §2, D314 §4, D438 or
> D448 §10. It proposes; it decides nothing.

# An Official Relay: Research, a Proposed Design and Its Costs

- **Date:** 2026-11-26. The owner asked: *"I've now got 2 friends who are hitting the issue where
  they can't get through to their home networks from work or school networks, so I don't think we
  can get away with not having an official relay server. Therefore I want a research round on how
  we can implement this."*
- **The owner's points:**
  - a paid orpheum.cc account includes relay up to X Mb/s per household, not per client ("maybe 2?");
  - a household may run its own relay, standalone or registered as its relay at orpheum.cc, and
    then relay through it with no limit;
  - easy: no long list of steps, no keys carried between systems;
  - secure: usable only between a household's core and its authorised clients;
  - a last resort, used only when nothing else reaches the core;
  - blind to what it carries, and keeping no record of it.
- **Scope:** reading, the repository and arithmetic. No code changed. Fetched pages are noted with
  their URLs and figures in `/yoloai/cache/http/official-relay-2026-11-26/notes.md`.
- **Builds on:** [relay-costs-2026-11-06.md](relay-costs-2026-11-06.md) (the traffic model),
  [household-relay-2026-11-13.md](household-relay-2026-11-13.md) (`orpheum relay`, its pairing,
  ports, credentials and cap) and [legal.md](legal.md) §4 (a hosted service's exposure). This
  document does not repeat their evidence; it cites their sections.
- **Marks:** **[source]** a page fetched today, in the notes; **[repo]** read in this repository
  or orpheum-connect today; **[unverified]** recalled or inferred, not confirmed; **[est]** my
  estimate, with its arithmetic.

## Summary

- **Everyone who relays for home media caps it, and caps it low.**
  - Plex relays at "2 Mbps maximum for streams", for its paid passes too, per connection, and
    transcodes to fit **[source]**.
  - Nabu Casa's remote access throttles each instance through a figure in its session token
    **[source]**.
  - iroh's paid relays limit at 5 MB/s **[source]**. Syncthing's relay has a per-session and a
    global rate **[source]**.
  - Every one of them is end-to-end encrypted and used only when direct fails, or as the
    bootstrap. Every one of them knows who connected and how many bytes passed, and nothing else.
- **What makes a relay closed is that both ends authenticate to it for the same tenant.**
  - Plex, Nabu Casa, Tailscale and iroh relay only between parties their service has
    authenticated. Syncthing's public pool is the counter-example: any device may use it, and it
    caps rates to keep that tolerable.
  - TURN's own rule is weaker: a TURN client may send to any address it has a permission for.
    Our hosted relay should therefore relay **between two allocations of the same household
    only**. A byte leaves the relay only toward a TURN client holding a live credential for that
    household. That one rule makes it no use as a generic relay, even to someone holding a stolen
    credential.
- **Work and school networks: TURN over TLS on 443, under a well-categorised name, survives the
  most.**
  - Blocked UDP and port filtering are common. TURNS on 443 looks like HTTPS and passes both
    **[source]**.
  - What defeats it is TLS interception, on devices that trust the inspector's root, and
    category filtering that blocks new or "proxy" domains **[source; medium confidence]**.
  - A friend's own phone or laptop does not trust a school's root, so interception mostly hits
    managed machines **[unverified]**.
  - A WebSocket tunnel would survive interception, but browsers cannot carry WebRTC over it. It
    would be a second transport with its own encryption. Proposed as a later phase, only if the
    failures the core already records show it is needed.
- **The design, in short (§4):**
  - the official relay is `orpheum relay` from the household-relay design, run by us, with a
    multi-household mode;
  - orpheum.cc mints short-lived credentials per session for paid households;
  - one home relay server is assigned to each household, so the relay meters the household's cap
    exactly;
  - the core picks the rung that fits the cap and the player says so;
  - ICE keeps the relay last, and the client retries direct.
  - A household's own relay pairs with its core by code. Registering it with orpheum.cc is one
    switch on the core's System page, because the core already hands its relay's address out
    through the heartbeat's `turn` **[repo]**.
- **Recommended X: 3 Mb/s per household** (§4.2):
  - one viewer gets 540p with room for sound and peaks; two viewers get 432p each;
  - the worst possible household month costs under €1 at Hetzner's overage price;
  - 2 Mb/s, the owner's figure, matches Plex's per-connection cap but leaves two viewers at 360p.
- **Costs (§5):**
  - On flat-traffic hosts, the relay is cheap at every size: about $13 a month for a two-server
    floor in one region. That is $1.32 per household at 10 households, $0.13 at 100, and under
    $0.02 from 1,000 up. At 10,000 households it reaches $40 to $96 a month.
  - Cloudflare TURN would cost $0.37–$1.41 per household at 10,000 households, depending on the
    share that relays and the cap. That is up to half of the $3 price.
  - Cloudflare also cannot hold the closed-relay rule. It is a comparison, not a candidate.
- **It amends** D273 §2, D314 §4, D438's preface and §1, §2 and §4, and D448 §10, and it
  realises D274 §2 (§4.9). It keeps D71, D72, D214 §1 and D438 §3.

## 1. What is settled and what is open

- **Settled:**
  - **No traffic is relayed by us.** The paid tier is "discovery … with no relay"; the owner said
    *"I don't want to be relaying traffic"* (D273 §2), and D438's preface keeps it.
  - **No public relay for WebRTC in the closed beta, and each failure is recorded** (D314 §4).
  - **A household's own relay is deferred until the beta's data asks for it.** When it is built,
    one core pairs with one relay, enrolled by the core (D438 §1–§3). Relayed playback is capped
    (D438 §4).
  - **A relay run by us is recorded as an idea only** (D448 §10).
  - **A relay is the last resort.** For the paid tier it would carry only the connections that
    fail, through TURN, encrypted end to end, with credentials the discovery issues per session.
    That is "open in principle but not built" (D274 §2).
  - **One relay artefact, and the core dials out** (D71).
  - **No free tier; the relay holds no data and no accounts; payment is the abuse control;**
    "state the allowance; never throttle silently" (D72).
  - **A relay brings legal.md §4's duties** (D214 §1).
  - **The price is $3 a month for one core**, in one tier for now (D273 §3, D298 §7).
- **What the owner's request changes:**
  - We relay, for paid households, under a cap. That amends D273 §2, D314 §4, D438's preface and
    D448 §10.
  - The data D438 §1 waited for has arrived: two of the beta's friends cannot reach home from
    work or school.
- **What it keeps:**
  - D438 §3's pairing, and the rule that the relay holds no discovery account;
  - D72's "never throttle silently" and its payment-as-abuse-control;
  - D274 §2's shape.
- **The repository today:**
  - The core's own relay hands out 15-minute TURN credentials, serves TCP beside UDP, and relays
    only to the public internet (`internal/core/turn/doc.go`) **[repo]**. That is not this relay:
    it runs at home, behind the same blocked network the friends cannot reach.
  - Both discoveries append the heartbeat's `turn` to a found client's ICE servers:
    - orpheum.cc in `internal/clients/clients.go`, the line
      `paths.ICEServers = append(paths.ICEServers, heartbeat.TURN...)`;
    - the single-household discovery in `discovery/single/client.go` **[repo]**.
  - orpheum.cc has no billing yet: its account plan holds "No real name, no payment detail"
    **[repo: orpheum-connect `docs/`]**.
  - Its service plan's P8 lists STUN and TURN servers as not built **[repo]**.

## 2. What others do

Setup steps are counted from what the person does, not what the vendor's software does
unattended.

### 2.1 Plex Relay

- **How it works:** the server and the app each make a secure connection to Plex's relay. Content
  "is encrypted end-to-end and tunneled … not terminated on our servers" **[source]**.
- **Steps: 0.** It is on by default. It needs "Secure connections" set to Preferred or Required,
  which is also the default **[source]**. It can be turned off.
- **Closed by:** a Plex account on both ends. The relay joins the server's own outbound tunnel
  to an app signed in to an account the server shares with **[source; the join's mechanics are
  not published]**.
- **Limits:** "Connections are limited to 2 Mbps maximum for streams (this applies to both Plex
  Pass and Remote Watch Pass subscribers)". The server transcodes to fit, and downloads are not
  allowed through the relay **[source]**.
  - This is **per connection**, not per household. Two viewers relayed are two 2 Mb/s streams.
- **Logs:** not stated on the page **[unverified]**.
- **Last resort:** yes. The app tries the direct and published addresses first, and the relay is
  used "when a direct connection is not possible" **[source]**.
- **Price:** remote streaming of personal media needs Plex Pass or the Remote Watch Pass. The
  Remote Watch Pass is $2.99 a month, or $29.99 a year, after an introductory $1.99 that ended
  on 2026-06-01 **[source]**. The relay comes with it.

### 2.2 Jellyfin and Emby

- **No relay.**
  - Jellyfin's networking documentation offers port forwarding, a reverse proxy and a Tailscale
    page **[source]**.
  - Emby offers UPnP automatic port mapping and a Port Mapper plugin. Emby Connect is sign-in,
    not transport **[source]**.
- **What their users do on blocked networks:** Tailscale, a VPS reverse proxy, or Cloudflare
  Tunnel against its terms (§2.7) **[unverified: forum lore, not fetched]**.
- **Steps:** a reverse proxy on a VPS is roughly 6–10 steps: a VPS, DNS, a certificate, a tunnel
  home, a firewall and the proxy's configuration **[est]**. This is the burden the owner wants us
  to avoid.

### 2.3 Tailscale: DERP and peer relays

- **DERP**
  - **How it works:** Tailscale's own fleet relays WireGuard packets that are already encrypted
    end to end, over HTTPS. "Any device that can open an HTTPS connection to an arbitrary host can
    build a tunnel using DERP relays" **[source]**.
  - **Steps: 0.**
  - **Closed by:** a node key the coordination server knows.
  - **Limits:** "limited QoS … lower maximum throughput" **[source]**; the figure is not
    published.
  - **Last resort:** yes, behind direct and peer relays. DERP is also how two peers first reach
    each other.
  - **A custom DERP** is "advanced": build `cmd/derper`, open ports 80, 443 and 3478, and run
    tailscaled beside it for `--verify-clients`. It is roughly 5–7 steps **[source; count est]**.
- **Peer relays**
  - **How it works:** a device in the tailnet relays for the others over UDP.
  - **Steps: 2.**
    1. `tailscale set --relay-server-port=40000`.
    2. A grant naming `tailscale.com/cap/relay` with source and destination tags **[source]**.
  - **Closed by:** the tailnet's policy. "A device without that grant can't allocate relay
    bindings" **[source]**.
  - **Limits:** none published. It is UDP only, and iOS, tvOS and Android cannot act as relays
    **[source]**.
  - **Logs:** status shows the peer relay's address and the bytes sent and received **[source]**.
  - **Order:** direct, then a peer relay, then DERP **[source]**.
  - Tailscale recommends peer relays over custom DERP.
- **Lesson:** the household's own relay should be as easy as a peer relay: a code, not a
  config file. And it should come ahead of the official one when it is up (§4.4).

### 2.4 Syncthing: the relay pool and strelaysrv

- **How it works:** a relay joins two devices' TLS connections, which stay end to end.
- **Steps: 0** with the public pool. A private relay is 2–3: run `strelaysrv`, then add
  `relay://host:443/?id=<relay's device ID>` to each device **[source]**.
- **Closed by:** nothing in the public pool. Any Syncthing device may use any pool relay, and the
  pool's relays "are not managed or vetted by the Syncthing project" **[source]**.
  - A private relay uses `-token` (which "Disables joining any pools") **[source]**.
- **Limits:** `-global-rate` and `-per-session-rate`, in bytes a second **[source]**.
- **Logs:** "The relay operator can see the amount of traffic" and the IP addresses and device
  IDs **[source]**.
- **Last resort:** yes. It is used only when direct fails, and Syncthing "will periodically retry
  a direct connection" and drops the relay when one works **[source]**.
- **Lesson:** retrying direct while relayed is the norm, and §4.5 copies it.

### 2.5 Nabu Casa (Home Assistant Link)

- **How it works:**
  - Home Assistant dials out to Nabu Casa's SniTun server, a TCP multiplexer behind an SNI proxy.
  - A browser's TLS connection to the instance's name is routed by SNI down that tunnel.
  - TLS ends on the instance, which holds a Let's Encrypt certificate whose private key never
    leaves it **[source: snitun README; Nabu Casa's security page via search, the page refused
    a direct fetch]**.
  - The tunnel is authorised by a Fernet token from Nabu Casa's session master. The token holds
    an expiry, the hostname, the multiplexer's key and, in the client library, a `throttling`
    figure that the multiplexer applies **[source: hass-nabucasa `remote.py`, snitun's
    multiplexer]**.
- **Steps: 2–3:** sign in to the subscription in Home Assistant, then turn on remote access
  **[est]**.
- **Closed by:** the hostname. A connection reaches only the one instance whose name it asked
  for.
  - A critic points out that the name is not access control, so the instance's own login is the
    gate **[source, opinion piece]**.
- **Limits:** a per-instance throttle carried in the token. Its value is not published
  **[source; value unverified]**.
- **Logs:** Nabu Casa cannot read the content, and certificate transparency lets anyone check
  that no other certificate exists for the name **[source]**.
- **Last resort:** not as such. It is the remote address. The local address is used at home.
- **Price:** €7.50 a month or €75 a year, as part of a bundle (backups, voice). It is being
  renamed Home Assistant Link from 2 December **[source]**.
- **Lesson:** a cap carried inside the token the service mints is the cleanest way to cap per
  tenant across many relay servers. §4.2 copies it.

### 2.6 iroh

- **How it works:** relays carry QUIC that is already end-to-end encrypted, keyed by node ID.
- **Steps:**
  - **0** for the public n0 relays, which are free and rate-limited **[source]**;
  - **2** for the paid ones: an API key in the app's configuration.
- **Closed by:** an API key, checked by the relay, with live revocation. Authentication is on by
  default for shared and dedicated relays **[source]**.
- **Limits and price:**
  - **Pro:** $19 a month for 4 relays, 10,000 connections and 100 GB of egress, then $0.09 a GB,
    with a 5 MB/s rate limit **[source]**.
  - **Dedicated:** $199 more a month per region **[source]**.
- **Last resort:** yes, after hole punching; the relay is also the bootstrap.

### 2.7 Cloudflare: Realtime TURN and Tunnel

- **Realtime TURN**
  - **How it works:** anycast TURN on 3478/udp, 443/udp, 3478/tcp, 5349/tcp and 443/tcp
    **[source]**.
  - **Steps:** for us, a key and an API call per session. Credentials carry a `ttl` and can be
    revoked by username **[source]**.
  - **Price:** $0.05 a GB of egress, with the first 1,000 GB a month free (shared with the SFU).
    "Only egress to the TURN client" is billed, not the leg to the peer **[source]**.
  - **Limits:** per allocation, above 5–10 kpps or 50–100 Mb/s it drops packets. There is nothing
    per tenant **[source]**.
  - **Closed by:** our credentials alone. Cloudflare cannot read the WebRTC it carries
    **[source]**.
  - **What it cannot do:**
    - pin a client's allocation to one household's core allocation;
    - meter a household across several clients;
    - refuse a TURN client that relays to an arbitrary public address.

    The closed rule (§4.3) and the household cap (§4.2) would both be ours to approximate from
    outside, through credential TTLs and usage analytics **[inference]**.
  - It does not serve China **[source]**.
- **Tunnel:** the CDN's terms say *"Unless you are an Enterprise customer, Cloudflare offers
  specific Paid Services (e.g., the Developer Platform, Images, and Stream) that you must use in
  order to serve video and other large files via the CDN"*, and Cloudflare may "disable or limit"
  such use **[source]**.
  - Whether a Tunnel's traffic counts as the CDN is not stated on that page
    **[unverified]**. It is commonly read as covered **[unverified]**.
  - Either way, a Tunnel ends TLS at Cloudflare, so it is not content-blind. It fails the owner's
    last point, whatever the terms say.

### 2.8 ZeroTier

- **How it works:** ZeroTier's roots relay when peers cannot reach each other directly.
- **Steps:** 0 for the default relays.
- **Limits:** "Premium Relays" is listed as "Coming Soon!" on every paid plan **[source]**. The
  free plan has 10 devices; Essential is $18 a month **[source]**.
- **Lesson:** relaying is offered as a paid differentiator, as the owner proposes.

### 2.9 Managed TURN prices

| Provider | Price **[source]** | Counts |
| --- | --- | --- |
| Cloudflare Realtime TURN | $0.05/GB after 1,000 GB free | egress to the TURN client only |
| Twilio | $0.40/GB (US West, Germany), $0.60 (Singapore, Mumbai, Tokyo), $0.80 (Sydney, São Paulo) | |
| Metered | $99 for 150 GB then $0.40; $199 for 500 GB then $0.20; $499 for 2 TB | ingress plus egress |
| Xirsys | $39 for 50 GB (+$0.50); $99 for 150 GB (+$0.40); $199 for 500 GB (+$0.20) | |
| iroh Pro (not TURN) | $19 for 100 GB then $0.09, 5 MB/s | egress |

Everything but Cloudflare costs more than the $3 price for a household that relays 10 GB.

### 2.10 What the survey shows

| Who | Steps | Closed by | Limit | Last resort | Content |
| --- | --- | --- | --- | --- | --- |
| Plex | 0 | account on both ends | 2 Mb/s per connection | yes | E2E TLS |
| Tailscale DERP | 0 | node keys | "limited QoS" | yes | WireGuard |
| Tailscale peer relay | 2 | tailnet grant | none published | yes, before DERP | WireGuard |
| Syncthing pool | 0 | nothing (rate caps) | per session and global | yes, retries direct | E2E TLS |
| Nabu Casa | 2–3 | hostname + token | throttle in token | it is the remote path | E2E TLS |
| iroh | 0–2 | API key | 5 MB/s (Pro) | yes | QUIC |
| Cloudflare TURN | ours | our credentials | 50–100 Mb/s per allocation | ours to arrange | DTLS |

## 3. Work and school networks

### 3.1 What they block, and what survives

| Network | TURN/UDP 3478 | TURN/TCP 443 | TURNS 443 | HTTPS/WebSocket tunnel | Confidence |
| --- | --- | --- | --- | --- | --- |
| UDP blocked but DNS, any TCP port open | fails | works | works | works | high **[source]** |
| Egress only on TCP 80 and 443 | fails | works | works | works | high **[source]** |
| Protocol checks on 443 (NGFW application ID, no decryption) | fails | likely fails | works | works | medium **[source]** |
| Category filter by SNI (new or "proxy" domains blocked) | fails | by IP only, likely works | works if the name is categorised | same as TURNS | medium **[unverified]** |
| Explicit HTTP proxy only, CONNECT to 443 | fails | fails | works if the browser uses the proxy for TURN | works | medium **[source]**, browser gaps in household-relay §2.2 |
| TLS interception, device trusts the inspector's root | fails | fails | fails | works if the proxy allows WebSocket | medium **[source]** |
| Allow-list of named services only | fails | fails | fails | fails | high |

- **Why TURNS on 443 survives the most.**
  - A proxy that allows only 443 "can not observe whether the HTTP CONNECT method is being used to
    reach a web server, … a WebSocket server or even something else" **[source:
    rtcquickstart]**.
  - Vendors who must work behind corporate firewalls list TURN over TLS on 443 as the path to
    allow (Daily, Microsoft Teams) **[source]**.
- **Interception is the failure case.**
  - Adobe advises that with a proxy "you may need to turn off SSL inspection of TLS/TURNS traffic
    on 443" **[source, via search]**.
  - How common interception is: about half of enterprises inspect some TLS, and about 3% inspect
    all of it. That figure is vendor commentary, undated, and no authoritative 2025 survey was
    found **[source, weak]**.
- **Schools.** Nothing published ties Lightspeed, Securly or GoGuardian to blocking WebRTC.
  - School Wi-Fi commonly blocks UDP, which is why voice chat fails while text loads
    **[source, a VPN vendor's blog]**.
  - Category filtering of new domains is standard in school filters **[unverified]**.
- **Personal devices.** A friend's own phone or laptop on work or school Wi-Fi does not trust the
  inspector's root. Interception would break every HTTPS site for it, so networks that admit
  personal devices tend to filter by SNI rather than decrypt **[unverified, reasoned]**.
  - So for the two friends' case, TURNS on 443 under a categorised name should work. It is the
    one thing to measure first (§4.8, phase 1).

### 3.2 What each client can use

From household-relay §2.2, re-checked against the repository today:

- **Chrome, Edge, Firefox and Safari** all gather `turn:…?transport=tcp` and `turns:` relay
  candidates.
  - Chrome needs a name with a valid certificate for TURNS, not an address.
  - Firefox and Safari have open bugs on TURN through an explicit proxy (Firefox 1922559 and
    2011209; WebKit 219274 and 320931) **[repo: household-relay §2.2]**.
- **Fire TV (Silk, Chromium 138):** as Chrome **[repo: household-relay §2.2]**. A Fire TV is rarely
  on a work or school network.
- **The Cast receiver** connects by WebRTC (`web/apps/cast-receiver/src/reach.ts`) **[repo]**, so
  it takes the same ICE servers. A Chromecast at work is unlikely. The case that matters is a
  phone at a friend's house, which is not a blocked network.
- **The SDK** waits `gatherForRelayMs = 1_000` for relay candidates and checks `hasRelayServer`
  (`web/packages/sdk/src/rtc-connect.ts`) **[repo]**, so a relay server in the list is already
  handled.
- **A WebSocket tunnel** cannot carry a browser's WebRTC. It would mean a second transport:
  - the SDK fetching over a WebSocket to the relay, which splices it to the core's own WebSocket;
  - with the end-to-end encryption done by the page (WebCrypto, a Noise-like handshake pinned to
    the core's signed key), because TLS to the relay would end at the relay.

  It is several weeks of work and a second security surface **[est]**. Proposed only if the
  failure count shows interception (§4.8, phase 7).

## 4. The proposed design

Clauses a decision could adopt. "The relay" is `orpheum relay` from household-relay §4, extended.

### 4.1 Shape

1. **One relay artefact, two modes.** `orpheum relay` (D71) serves either:
   - **household mode:** one core, paired by code, as household-relay §4 designs it (D438 §2–§3);
   - **hosted mode:** many households, each identified by an opaque household id minted by
     orpheum.cc. It holds no account and no key of any household, only the fleet's credential
     secret.

   Both modes keep the 443 demultiplexer, the TURN listeners and the token buckets.
2. **The hosted relay is only for paid orpheum.cc households.** Credentials are minted by
   orpheum.cc only while the account is paid and active. Payment is the abuse control (D72).
3. **The official relay terminates nothing.** DTLS runs end to end between the client and the
   core, whose fingerprint the core signs (discovery-protocol.md §3.7, as D438 §3).
4. **The official relay dials nothing.** The core and the client each allocate on it; neither
   needs a port forward (D71).

### 4.2 The cap, and the recommended X

1. **The cap is per household, carried in the credential.**
   - orpheum.cc writes the household's rate into each credential it mints: 3 Mb/s for a paid
     household, none for a household's own relay.
   - The relay keeps one token bucket per household id, filled at that rate. Every byte it sends,
     toward the client or toward the core, draws from it.
   - The bucket holds two seconds of the rate, so a chunk's burst is not dropped [est].
   - This is Nabu Casa's throttle-in-the-token, applied per household rather than per instance.
2. **One home relay server per household.**
   - orpheum.cc assigns each household one server in the core's region, by a stable hash over the
     live servers, and every credential for that household names it.
   - So one bucket on one box sees all of the household's relayed traffic, from any number of
     clients, and the cap is exact without counters shared between boxes.
   - If that server dies, orpheum.cc assigns the next one. The household's bucket starts again
     full, which loses at most two seconds of allowance.
3. **Metering across servers.**
   - Each relay reports, each minute, the bytes per household id and the number of live
     allocations to orpheum.cc over its control connection.
   - orpheum.cc keeps the month's total per household for the account page and the abuse
     threshold (§4.3.6).
   - No per-session record goes there (§4.6).
4. **The core shapes to the cap, so the bucket never bites.**
   - The core learns the household's relay rate from the heartbeat's answer.
   - A connection whose selected pair is relayed (`PathRelayed`) shares that rate with the
     household's other relayed connections. The core already lowers the playback needing most
     onto a quality made [repo: household-relay §4.8].
   - The pacer follows the relay's address at the rate (household-relay §4.8: rtcapi does not pace
     a relayed send today).
   - Browsing, artwork and control are not shaped, but they draw from the bucket. They are small.
5. **The player states it** (D72): "Relayed · up to 3 Mb/s" in the quality menu and in the
   stream's info, and the rung shown is the one the cap allows.

**The arithmetic.** The rungs are `3000k@720p, 2000k@540p, 1200k@480p, 700k@432p, 400k@360p`.
Hardware adds `6000k@1080p`. Sound defaults to 96 kb/s
(`internal/core/settings/registry.go`) **[repo]**, and peaks are capped at 1.5 times the rung
**[repo: relay-costs]**.

Assumptions:

- The data channel's overhead over TURN is about 7%: SCTP, DTLS, UDP or TCP, and TURN's channel
  header **[est]**.
- A rung plays smoothly when its average fits in 85% of its share. The rest is headroom for the
  start, seeking and control **[est]**.
- It plays without the buffer ever draining when its 1.5 times peak also fits.

| Rung | Average need, Mb/s (rung + 0.096) × 1.07 | Peak need, Mb/s (1.5 × rung + 0.096) × 1.07 |
| --- | --- | --- |
| 400k@360p | 0.53 | 0.75 |
| 700k@432p | 0.85 | 1.23 |
| 1200k@480p | 1.39 | 2.03 |
| 2000k@540p | 2.24 | 3.31 |
| 3000k@720p | 3.31 | 4.92 |

| Cap X | One viewer: rung (avg ≤ 0.85 X / peak ≤ X) | Two viewers at once, X/2 each | Worst month (X × 720 h × 0.45 GB) | At Hetzner overage €1/TB |
| --- | --- | --- | --- | --- |
| 1 Mb/s | 432p marginal (0.85 ≤ 0.85) / 360p | 360p marginal (0.53 ≤ 0.43 fails; plays with stalls) | 324 GB | €0.32 |
| 2 Mb/s | 480p (1.39 ≤ 1.70) / 432p | 432p marginal (0.85 ≤ 0.85) / 360p | 648 GB | €0.65 |
| 3 Mb/s | 540p (2.24 ≤ 2.55) / 480p | 432p (0.85 ≤ 1.28) / 432p (1.23 ≤ 1.5) | 972 GB | €0.97 |
| 4 Mb/s | 720p marginal (3.31 ≤ 3.40) / 540p | 480p (1.39 ≤ 1.70) / 432p | 1,296 GB | €1.30 |

- **Recommended X = 3 Mb/s.**
  - One viewer plays 540p with 12% to spare on average, and two viewers each play 432p with peaks
    covered.
  - The worst month a household could ever use costs under €1 even at overage, a third of the
    price. So the rate alone bounds the cost, and no monthly byte budget is needed on a
    flat-traffic host (§5.4).
- **The owner's 2 Mb/s** equals Plex's figure, but Plex's is per connection: two Plex viewers get
  2 Mb/s each. Per household, 2 Mb/s leaves two viewers on 360p or a marginal 432p.
- **4 Mb/s** buys 720p for one viewer only marginally and costs a third more at the margin. It is
  worth it only if the owner wants two viewers at 480p.

### 4.3 Security: closed to all but a household's core and its clients

1. **Short-lived credentials per session, two roles, the household inside the HMAC.**
   - TURN REST credentials have the username `<expiry>:<role>:<household id>:<rate>:<server>` and
     an HMAC-SHA1 password under the fleet's secret, which orpheum.cc and the relays share. These
     are the household-relay §4.5 roles, with the tenant added.
   - **Client role:** minted by orpheum.cc in a `found` answer, for a client that the core has
     authorised. It lasts 15 minutes, like the core's TURN credentials today [repo].
   - **Core role:** minted by orpheum.cc in the heartbeat's answer, refreshed each heartbeat, and
     held by the core alone.
   - An allocation made while a credential was good outlives it, as today [repo].
   - Neither role can be turned into the other, nor moved to another household, without the
     secret.
2. **Relay between two allocations of the same household only.** This is the rule that makes the
   hosted relay closed.
   - A **client** allocation may hold a permission only for the relayed address of a live **core**
     allocation tagged with the same household id on the same server.
   - A **core** allocation may hold a permission only for the relayed address of a live **client**
     allocation of the same household.
   - TURN permissions are by IP alone (RFC 8656), and every allocation on one server shares its
     IP. So the relayed socket's wrapper also drops any datagram whose destination address and
     port is not a live allocation of the same household (household-relay §4.5's wrapper, keyed
     by household).
   - **So:** a byte leaves the relay only toward a TURN client holding a live credential for that
     household. A stolen client credential reaches that one core's allocation and nothing else.
     A stolen core credential reaches only that household's clients.
   - **Its price:** both ends must allocate (relay to relay, by loopback on one box). The pair of
     a client's server-reflexive candidate and the core's relayed one, which household-relay §4.5
     allows, is refused here.
     - That costs nothing in traffic: the box's billed egress is still one copy toward the client,
       and the loopback hop is not billed [source: Hetzner bills outgoing traffic only;
       loopback assumed not counted, unverified].
     - It adds one allocation's set-up to the start, about one round trip [est].
     - A household-mode relay may keep household-relay §4.5's rule, or take this one for a single
       rule. That is question 6.
3. **The core allocates only for a client it has admitted.**
   - The core gathers its official-relay candidate only for a WebRTC offer it has already
     authenticated: the pairing and session checks of today. The allocation dies with the
     connection.
   - A core never holds an idle allocation.
4. **Limits.** Each is a refusal, not a slowdown, except the bucket:
   - at most 2 allocations per client credential;
   - at most twice the core's `webrtc.max_connections` allocations per household;
   - at most 50 allocations per source IP;
   - one bucket per household at its rate.

   `QuotaHandler` refuses the rest. The figures are [est].
5. **Attacker cases.**

   | Attacker | Gets | Stopped by |
   | --- | --- | --- |
   | Anyone without a credential | nothing; STUN binding only | `AuthHandler` refuses allocations |
   | A paying customer wanting a free proxy | their own allocations, which reach only their own core's | §4.3.2; payment and the account on record |
   | A stolen client credential (15 min) | the same household's core allocation, already DTLS-pinned to the core | expiry; the DTLS fingerprint the core signs; the bucket |
   | A stolen core credential | relays to that household's clients only | §4.3.2; revoked by rotating the household's tag at orpheum.cc |
   | A household hosting strangers | up to 3 Mb/s, inside its own core's admission rules | the cap makes "share with 500 strangers" impractical (legal.md §4); the abuse threshold |
   | Flooding the relay | refused allocations; UDP flood | quotas per IP; the host's DDoS filtering **[unverified per host]** |
   | Traffic analysis by us or the host | timing and sizes | stated, not mitigated; the cap blurs bursts (household-relay §4.12) |
   | The fleet's secret leaked | mint any credential for any household id | rotate (two secrets live during rotation); the secret lives in orpheum-ops' secret store, never in a repo |

6. **Abuse handling.**
   - orpheum.cc flags a household whose relayed month passes a threshold, for example 500 GB, or
     about 17 hours a day at 3 Mb/s [est]. A flag is looked at, not acted on automatically.
   - A repeat-infringer policy and a copyright contact are published (legal.md §4, D214 §1).
   - A valid notice about a household leads to that household's relay access being suspended.
     The relay cannot judge content, and does not try.
7. **Legal exposure.**
   - As a content-blind conduit, end-to-end encrypted, selecting neither recipients nor content,
     the relay fits US §512(a) and EU DSA Article 4. §512(i)'s repeat-infringer policy is a
     precondition (legal.md §4).
   - legal.md §4 also warns against a price "tied to bandwidth or volume". The relay is included
     in the one $3 tier with a fixed ceiling, which fits. An upsell of more megabits for more
     money would not, which is question 4.

### 4.4 The household's own relay, standalone or registered

1. **Standalone:** as household-relay §4. Four steps with compose:
   1. open the ports;
   2. paste the compose file;
   3. `up`, and read the code it prints;
   4. enter the code on the core's System page.

   It works with the household's own discovery (D448 §10) or none.
2. **Registered with orpheum.cc: no further step.** The core already sends its relay's addresses
   in the heartbeat's `turn`, and orpheum.cc already appends them to a found client's ICE servers
   [repo: `internal/clients/clients.go`]. So a paired relay is registered as soon as it is
   paired.
   - What changes: orpheum.cc marks the household as having its own relay while the heartbeat
     reports it connected. It then hands out that relay first and the official relay second
     (question 5).
   - The household's own relay is uncapped by us. Its rate is the household's own setting
     (household-relay §8, question 5), and the official cap never applies to it.
3. **How to pair the relay, compared:**

   | Method | Steps | Keys carried | Who the relay trusts | Fits D438 §3 |
   | --- | --- | --- | --- | --- |
   | Pairing code: the relay prints a code, entered on the core | 1 after `up` | none; a 12-character code read off a screen | the core's key | yes |
   | Device-flow sign-in: the relay prints a URL and code, the admin signs in at orpheum.cc | 2 (open the URL, sign in and approve) | none | an orpheum.cc account token | no: the relay would hold a discovery credential, and orpheum.cc must still learn which core it serves |
   | Join token: copy a token from orpheum.cc into the relay's environment | 2–3 (create, copy, paste, restart) | a long secret, carried by hand | orpheum.cc | no; and it is the "carrying keys across systems" the owner rules out |

   **Recommended: the pairing code**, as D438 §3 decided.
   - It is the only one that works standalone.
   - Registration then follows from the core with no step.
   - The relay never holds an orpheum.cc credential, so a lost relay costs only its address.
4. **A household with its own relay that also pays** keeps the official relay as its fallback,
   at the cap. This covers its own relay's outage, and needs no setting.

### 4.5 Last resort

1. **ICE keeps it last.**
   - Relayed candidates carry ICE's lowest type preference, so every host, peer-reflexive and
     server-reflexive pair is checked and preferred first [RFC 8445; repo: the core's ICE today].
   - The official relay is listed after the core's own TURN and after a household's own relay, so
     among relayed pairs it ranks last (local preference by list order) **[unverified per
     browser]**.
2. **Credentials are always handed out; allocation is the gate.**
   - Withholding credentials until direct fails would add a round trip to orpheum.cc on exactly
     the networks that are already slow to start.
   - Instead, the client and the core allocate only after the 1-second relay gather
     (`gatherForRelayMs`) has gone by with no direct pair succeeding. The discarded allocation
     costs nothing [repo: rtc-connect; the timing change is new work].
3. **The starved pair:** household-relay §4.7's relay-only retry. If a punched pair connects but
   moves less than a fraction of the rung after 10 s, the client restarts ICE with
   `iceTransportPolicy: "relay"` and remembers it for that network (`clientPrefix`).
4. **Back to direct, as Syncthing does.**
   - While relayed, the client tries an ICE restart with all candidates every 5 minutes and on
     every network change. It moves off the relay if a non-relayed pair succeeds and carries the
     current rung for 10 seconds.
   - The restart happens between chunks, so play does not stop **[est; ICE restart keeps the data
     channel, per WebRTC 1.0, unverified on Silk]**.
5. **Telling the viewer** (D72):
   - "Relayed · up to 3 Mb/s" while relayed, in the quality menu and the info sheet;
   - one short note the first time in a session that a stream is lowered by the cap;
   - nothing when it moves back to direct, except that the label goes.
6. **Counting:** the core records each session's path (direct, punched, relayed by whose relay)
   in its own path counts, which D314 §4 asked for and D438 §1 calls the data. orpheum.cc sees
   only the relay's byte totals (§4.6).

### 4.6 Content-blind: what is logged, and for how long

1. **The relay never sees:** content, a user, a session's purpose, a film's id or a household's
   name. The household id is an opaque random token. Only orpheum.cc maps it to an account.
2. **In memory, for an allocation's life only:**
   - the client's and the core's addresses;
   - the five-tuple;
   - the username;
   - the bucket's state.
3. **In the relay's log, kept 7 days** (legal.md §4: "days not months"). One line per allocation
   when it ends:
   - the time it started and its duration;
   - the role;
   - the household id;
   - the bytes each way;
   - the reason it ended;
   - the client's address truncated to /24 for IPv4 or /48 for IPv6, for abuse and legal process
     only.

   No per-packet or per-request record is kept. The log is the relay's own, not shipped.
4. **At orpheum.cc:**
   - per household, per day: bytes relayed and peak allocations, kept 13 months for the account
     page and billing disputes;
   - per server, per minute: bytes and allocations, for capacity.

   No addresses and no times of individual sessions.
5. **Published:** what is kept and how long, on orpheum.cc's privacy page, with the
   repeat-infringer policy.
6. **Traffic analysis is possible in principle** and is stated, not mitigated (household-relay
   §4.12).

### 4.7 Where the work lands

| Repository | Work |
| --- | --- |
| orpheum (this repo) | `orpheum relay` (household-relay steps 0–9) plus hosted mode: tenant in the credential, the same-household permission rule, per-household buckets, the minute report, the control connection to orpheum.cc. The core: the official relay's credentials from the heartbeat, allocating only for an admitted offer, the shaping to the household rate, the pacer. The SDK and player: the deferred allocation, the starved retry, the periodic return to direct, the label. `discoveryproto` gains the fields, additively (D273 §6) |
| orpheum-connect | minting both roles; assigning the home server; the fleet's registry and secret rotation; the minute reports, the month's totals and the abuse flag; the account page's "relayed this month"; the own-relay mark; and billing, which does not exist yet and gates the paid relay |
| orpheum-ops | the servers (Hetzner, one region first); DNS names under orpheum.cc for TURNS and their certificates; the secret in its store; monitoring and alerts on bytes, allocations and refusals; the DMCA agent registration and the published policy |

### 4.8 Build order, smallest first

| Phase | What | Where | Size **[est]** | Shows |
| --- | --- | --- | --- | --- |
| 0 | The core reports its path counts and dead ends per session (D314 §4, D438 §1) | orpheum | 2–3 days | how many fail, and on what |
| 1 | One hosted relay server for the beta's households, credentials minted by orpheum.cc for a hand-kept list of household ids, the same-household rule, a fixed 3 Mb/s bucket, TURNS on 443 under a name | all three | 2–3 weeks (the relay core is household-relay steps 0–4) | whether TURNS 443 fixes the two friends |
| 2 | The core shapes to the household rate; the pacer; the player's label | orpheum | 1 week | no bucket drops; D72 honoured |
| 3 | Last resort: deferred allocation, the starved retry, the periodic return to direct | orpheum (SDK, core) | 1 week | the relay's share falls to what must relay |
| 4 | Billing, and the relay credentials gated on a paid account | orpheum-connect | 2–4 weeks, outside this design | the paid tier |
| 5 | The household's own relay: pairing, household mode, the own-relay mark at orpheum.cc | orpheum, orpheum-connect | 2–3 weeks (household-relay steps 5–9) | the owner's second point |
| 6 | A second server and region, home-server assignment and failover, minute reports, abuse flag, the 7-day log | all three | 1–2 weeks | scale and the legal duties |
| 7 | The WebSocket tunnel with page-level encryption, only if phase 0's counts show interception | orpheum | 3–5 weeks | the last networks |

- Phase 1 alone fixes the friends.
- Phases 1–3 make it right.
- Phase 4 makes it a product.

### 4.9 What a decision would amend

| Decision | Clause | Change |
| --- | --- | --- |
| D273 | §2 ("paid tier is discovery … with no relay"; "I don't want to be relaying traffic") | the paid tier includes a capped, content-blind relay of the connections that fail |
| D314 | §4 ("there is no public relay for WebRTC, and that is accepted for the closed beta") | the beta's paid households get the official relay from phase 1. The recording of each failure stays |
| D438 | preface ("It keeps D273: no traffic is relayed by us") | no longer true |
| D438 | §1 (deferred until the beta's data) | the data has come; build |
| D438 | §2 ("One core, one relay … no accounts, no other households") | stays for household mode. Hosted mode serves many households, under orpheum.cc's credentials |
| D438 | §4 (relayed playback capped "near 1.3 TB") | official: 3 Mb/s per household. Own relay: the household's own setting, uncapped by us |
| D448 | §10 ("A relay run by us is recorded as an idea only") | it is to be built |
| D274 | §2 ("open in principle but not built") | realised as designed there |

**Kept:**

- D71: one artefact, the core dials out;
- D72: no free tier, payment as the abuse control, no data held, "never throttle silently";
- D214 §1: the legal duties;
- D438 §3: the code pairing, and no discovery account on the relay.

## 5. Costs

### 5.1 The model

relay-costs' traffic model, with the cap applied:

- **Rate:** 0.45 GB an hour per Mb/s.
- **Hours:** the mixed population (50% low, 40% typical, 10% heavy) streams 0.5 × 10 + 0.4 × 40 +
  0.1 × 210 = **42 hours a month** per household **[repo: relay-costs]**.
- **A relaying household** is assumed to relay all 42 hours at the full cap. This is an upper
  bound twice over: it relays only away from home on a blocked network, and the rung under the
  cap averages below it.
  - So a relaying household moves 42 × 0.45 × X = 18.9 X GB a month: **56.7 GB at 3 Mb/s**, 37.8
    at 2 Mb/s (relay-costs' addendum's figure) and 75.6 at 4 Mb/s.
- **Peak:** mean concurrency is 42 / 720 = 0.058 streams. relay-costs' peak is 4 times the mean,
  0.233. The household's cap holds even with two viewers, so the peak is 0.233 × X Mb/s per
  relaying household **[est]**.
- **Shares of households relaying:**
  - **low 5%;**
  - **central 20%:** relay-costs' published WebRTC relay shares are 15–25%;
  - **high 50%:** the beta, where two of a handful of households hit it.
- **Hosts:**
  - **Hetzner Cloud CX23:** €5.99, 20 TB included in the EU, then €1 a TB, outbound only
    **[source]**.
    - Assumed to relay 300 Mb/s on 2 vCPU with pion/turn **[est, not measured]**.
    - The number of servers is the cheapest that carries the peak, against overage. The floor is
      2 servers in one region.
  - **OVH VPS-2:** $8.50 a month, 1 Gb/s, unlimited traffic **[source]**. Assumed to relay
    500 Mb/s **[est]**. The floor is 2.
  - **Cloudflare Realtime TURN:** $0.05 a GB after 1,000 GB free **[source]**.
    - The client is the TURN client on a blocked network, so the video is billed.
    - It has no floor, and no closed rule (§2.7).
- **Currency:** €1 = $1.10 **[est]**.

### 5.2 At the recommended 3 Mb/s

| Households | Share relaying | Relaying | GB a month | Peak Mb/s | Hetzner $ (servers) | per household | OVH $ (servers) | per household | Cloudflare $ | per household |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 10 | low 5% | 0.5 | 28 | 0.1 | 13.18 (2) | 1.318 | 17.00 (2) | 1.700 | 0 | 0 |
| 10 | central 20% | 2 | 113 | 1 | 13.18 (2) | 1.318 | 17.00 (2) | 1.700 | 0 | 0 |
| 10 | high 50% | 5 | 284 | 4 | 13.18 (2) | 1.318 | 17.00 (2) | 1.700 | 0 | 0 |
| 100 | low 5% | 5 | 284 | 4 | 13.18 (2) | 0.132 | 17.00 (2) | 0.170 | 0 | 0 |
| 100 | central 20% | 20 | 1,134 | 14 | 13.18 (2) | 0.132 | 17.00 (2) | 0.170 | 7 | 0.067 |
| 100 | high 50% | 50 | 2,835 | 35 | 13.18 (2) | 0.132 | 17.00 (2) | 0.170 | 92 | 0.917 |
| 1,000 | low 5% | 50 | 2,835 | 35 | 13.18 (2) | 0.013 | 17.00 (2) | 0.017 | 92 | 0.092 |
| 1,000 | central 20% | 200 | 11,340 | 140 | 13.18 (2) | 0.013 | 17.00 (2) | 0.017 | 517 | 0.517 |
| 1,000 | high 50% | 500 | 28,350 | 350 | 13.18 (2) | 0.013 | 17.00 (2) | 0.017 | 1,368 | 1.367 |
| 10,000 | low 5% | 500 | 28,350 | 350 | 13.18 (2) | 0.001 | 17.00 (2) | 0.002 | 1,368 | 0.137 |
| 10,000 | central 20% | 2,000 | 113,400 | 1,400 | 39.53 (6) | 0.004 | 25.50 (3) | 0.003 | 5,620 | 0.562 |
| 10,000 | high 50% | 5,000 | 283,500 | 3,500 | 96.10 (14) | 0.010 | 59.50 (7) | 0.006 | 14,125 | 1.413 |

How a row is worked out, taking 10,000 households at the central share on Hetzner:

- 2,000 relaying households × 56.7 GB = 113.4 TB a month.
- The peak is 2,000 × 0.233 × 3 = 1,400 Mb/s, which needs 5 servers at 300 Mb/s.
- 6 servers include 120 TB, so there is no overage: 6 × €5.99 = €35.94, or $39.53. Five servers
  with 13.4 TB of overage would cost €43.35.
- That is $39.53 / 10,000 = $0.004 a household.

### 5.3 At 2 and 4 Mb/s

| Households, central 20% | 2 Mb/s: GB, Hetzner, OVH, Cloudflare | 4 Mb/s: GB, Hetzner, OVH, Cloudflare |
| --- | --- | --- |
| 10 | 76 GB; $13.18; $17.00; $0 | 151 GB; $13.18; $17.00; $0 |
| 100 | 756 GB; $13.18; $17.00; $0 | 1,512 GB; $13.18; $17.00; $26 |
| 1,000 | 7,560 GB; $13.18; $17.00; $328 | 15,120 GB; $13.18; $17.00; $706 |
| 10,000 | 75,600 GB; $26.36 (4); $17.00 (2); $3,730 | 151,200 GB; $52.71 (8); $34.00 (4); $7,510 |

At the high share and 10,000 households:

| Cap | Hetzner | OVH | Cloudflare |
| --- | --- | --- | --- |
| 2 Mb/s | $65.89 (10 servers) | $42.50 | $9,400 ($0.94 a household) |
| 4 Mb/s | $125.19 (19 servers) | $85.00 | $18,850 ($1.89 a household; relay-costs' addendum's figure) |

### 5.4 Against the price, the floor, regions and latency

- **Against $3 a month (D273 §3, D298 §7):**
  - On Hetzner or OVH the relay costs under 1% of the price from 1,000 households up, and 4–6%
    at 100.
  - At 10 households the two-server floor is 44% of the price ($1.32 of $3). Phase 1's single
    server halves that to $0.66.
  - On Cloudflare it is 2–47% at 1,000 households and above, depending on the share, and up to
    63% at 4 Mb/s.
  - A payment processor's fee on $3 is likely larger than the relay on a flat host: about $0.39
    at 2.9% + $0.30 **[unverified; no processor chosen]**.
- **The worst single household:** X × 720 hours × 0.45 GB.
  - At 3 Mb/s that is 972 GB: €0.97 at Hetzner's overage, $48.60 on Cloudflare.
  - So on a flat host the cap alone bounds the loss, and on Cloudflare a monthly byte budget
    would be needed too. This is another reason against Cloudflare.
- **The fixed floor:**
  - 2 servers in one region, about €12 ($13.18);
  - DNS names under the existing orpheum.cc domain, $0;
  - certificates by ACME, $0;
  - the DMCA agent registration, about $6 for three years **[unverified]**;
  - monitoring on the existing ops stack, $0 [est].

  The floor is about $13 a month in one region, or $26 in two.
- **Regions and latency:**
  - The relay sits near the **core**, which never moves, and the client's leg crosses the
    distance.
  - Within Europe, a Hetzner box adds roughly 10–30 ms of round trip **[est]**. A US core served
    from Germany adds 80–100 ms **[est]**, which slows the start (one round trip per request) and
    SCTP's throughput over loss.
  - So a second region opens when the first US core relays.
  - Hetzner's US servers include only 1–2 TB, not 20 **[source]**. At 3 Mb/s that is 18–35
    relaying households per server before overage. OVH's US VPS is unlimited **[source]**, which
    makes OVH the better US host.
  - OVH in Asia-Pacific has a quota, then 10 Mb/s **[source]**, so Asia waits.

## 6. Questions for the owner

1. **X.** 3 Mb/s per household (my recommendation: one viewer 540p, two at 432p, under €1 worst
   case), or the 2 Mb/s you suggested (one viewer 480p, two at 360p)?
2. **Phase 1 now, before billing exists?** Recommendation: yes. One server, the beta's
   households on a hand-kept list, free during the closed beta as D314's successor. Billing gates
   it from phase 4.
   - D72's "no free tier" is about the product. A beta list is a trial, which a decision should
     say.
3. **Same-household-only relaying (§4.3.2)**, which refuses the server-reflexive-to-relayed pair
   and makes both ends allocate. Recommendation: yes. It is the one rule that makes "only between
   their core and their authorised clients" true even with a stolen credential.
4. **No upsell of megabits.** Recommendation: none. legal.md §4 warns against a price tied to
   bandwidth, and the household's own relay is the route to "no limits".
5. **A household with its own relay and a paid account.** Hand out its own first and ours second,
   or its own only? Recommendation: own first, ours as a capped fallback.
6. **One permission rule for both modes?** Recommendation: household mode takes §4.3.2's rule
   too, so there is one rule, one test and no open path at home either. This narrows
   household-relay §4.5.
7. **The abuse threshold** (500 GB a month, flagged for a look, not acted on). Recommendation:
   adopt it, and publish the policy beside the repeat-infringer one.
8. **Logs:** 7 days of per-allocation lines with truncated addresses, and 13 months of
   per-household daily totals. Recommendation: adopt; lawyer to confirm (legal.md §6).
9. **Host:** Hetzner in the EU first, OVH when a US core relays? Recommendation: yes.
   Cloudflare TURN is not used (no closed rule, no household cap, and costs that grow with
   bytes).
10. **The WebSocket tunnel (phase 7):** build only if phase 0's counts show TLS interception?
    Recommendation: yes, measure first (D274 §3).

## 7. What I could not verify

- Plex's relay logs, and how its relay joins the two ends.
- Nabu Casa's throttle value, and its security page's full text, which refused a direct fetch
  (the figures come from search snippets and the open-source code).
- Whether Cloudflare's video clause covers Tunnel (§2.7).
- How common TLS interception is in 2025–2026 workplaces and schools. No primary survey was found.
- Whether school filters (Lightspeed, Securly, GoGuardian) block TURNS or new domains by default.
- pion/turn's relaying throughput on a 2-vCPU Hetzner box. The 300 Mb/s figure is an estimate to
  measure in phase 1.
- Whether Hetzner counts loopback relay-to-relay traffic. It should not cross the NIC.
- Hetzner US's overage price, assumed to be €1 a TB as in the EU.
- Whether a browser ranks relayed candidates by the ICE server list's order, and whether an ICE
  restart keeps playback seamless on Silk.
- The data channel's overhead through TURN (7% assumed), and the 1.5 times peak at the chunk level.
- How many beta households hit the problem, beyond the owner's two friends.

## Sources

Fetched 2026-11-26. Each page is cached with its figures in
`/yoloai/cache/http/official-relay-2026-11-26/notes.md`.

- **Plex:**
  - [Accessing a server through Relay](https://support.plex.tv/articles/216766168-accessing-a-server-through-relay/)
  - [Remote Watch Pass](https://support.plex.tv/articles/remote-watch-pass-overview/)
- **Jellyfin and Emby:**
  - [Jellyfin networking](https://jellyfin.org/docs/general/post-install/networking/)
  - [Emby connectivity](https://emby.media/support/articles/Connectivity.html)
- **Tailscale:**
  - [peer relays](https://tailscale.com/kb/1591/peer-relays)
  - [DERP servers](https://tailscale.com/kb/1232/derp-servers)
  - [connection types](https://tailscale.com/kb/1257/connection-types)
- **Syncthing:**
  - [relaying](https://docs.syncthing.net/users/relaying.html)
  - [strelaysrv](https://docs.syncthing.net/users/strelaysrv.html)
  - [the relay pool](https://relays.syncthing.net/)
- **Nabu Casa:**
  - [pricing](https://www.nabucasa.com/pricing/)
  - [SniTun](https://github.com/NabuCasa/snitun)
  - [hass-nabucasa](https://github.com/NabuCasa/hass-nabucasa)
- **iroh:**
  - [relays](https://www.iroh.computer/docs/concepts/relays)
  - [pricing](https://www.iroh.computer/pricing)
  - [shared relays](https://docs.iroh.computer/iroh-services/relays/shared)
- **Cloudflare:**
  - [Realtime TURN](https://developers.cloudflare.com/realtime/turn/)
  - [pricing](https://developers.cloudflare.com/realtime/pricing/)
  - [the TURN FAQ](https://developers.cloudflare.com/realtime/turn/faq/)
  - [credentials](https://developers.cloudflare.com/realtime/turn/generate-credentials/)
  - [service-specific terms](https://www.cloudflare.com/service-specific-terms-application-services/)
- **ZeroTier:** [pricing](https://www.zerotier.com/pricing/)
- **Managed TURN:**
  - [Twilio](https://www.twilio.com/en-us/stun-turn/pricing)
  - [Metered](https://www.metered.ca/stun-turn)
  - [Xirsys](https://xirsys.com/pricing/)
- **Hosts:**
  - [Hetzner Cloud](https://www.hetzner.com/cloud/)
  - [Hetzner billing FAQ](https://docs.hetzner.com/cloud/billing/faq/)
  - [OVH VPS](https://www.ovhcloud.com/en/vps/)
- **Firewalls:**
  - [RTC Quick Start: getting through firewalls](https://rtcquickstart.org/guide/multi/optimal-connectivity-firewalls.html)
  - [Daily networking guide](https://docs.daily.co/docs/guides/privacy-and-security/corporate-firewalls-nats-allowed-ip-list)
  - [Adobe Connect firewall and proxy](https://helpx.adobe.com/adobe-connect/firewall-proxy-server-configuration-adobe-connect.html)
