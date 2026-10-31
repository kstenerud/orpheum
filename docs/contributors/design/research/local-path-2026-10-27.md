> **ABOUTME:** Why the owner's playback on staging on 2026-10-26, from a laptop in the house, went
> over WebRTC through the carrier's shared IPv4 address (11.22.33.144) instead of over the house's
> network; which code chose it and why; what the logs could not say; and the fix options. Read before
> changing how a page at home finds its core, or before judging staging's playback as "local".

# The local path: why a page at home reached staging through the carrier

- **Date:** 2026-10-27. Asked by the owner after a film's playback reports of 2026-10-26
  (`/yoloai/files/bug-report-*.log`), where every playback logged
  `over=webrtc clientAddress=11.22.33.144`.
- **Method:** read-only. The client's path code, the core's WebRTC and discovery configuration,
  staging's rendered config (`.staging/core/persistent/config.toml`, `.staging/values.toml`), the
  core's logs (`docker logs` and `.staging/logs/`, 2026-10-24 to 2026-10-26), connectd's log, and
  staging's `device` table (read-only). Nothing was changed, restarted or redeployed. nas.example could
  not be compared: its SSH key is not in this sandbox.
- **Labels:** **[L]** seen in a log or the database; **[C]** read in the code or config;
  **[M]** from the agent's memory notes, not re-verified here; **[W]** from the web; **[I]**
  inference.

---

## 1. Summary

The playback was never offered a LAN path it could use, for three causes that stack. Any one of
them is enough to rule out a path in the house:

1. **The page never tries the LAN name in Firefox, by design.** The device was *Firefox on Linux*
   (device 29) **[L]**. `usesLanName` (`web/apps/web/src/local-network.ts`) returns false for any
   Firefox, so the page reports the local-network permission as `denied` and leaves the LAN name out
   of every race. This was the owner's choice on 2026-10-20, because Firefox's Local Network Access
   fails a controlled page's chunk requests to a LAN name even after the person allows it
   (idiosyncrasies.md, "Firefox fails a public page's requests to a LAN name…"). In Firefox the plan
   is for **WebRTC to find the LAN pair** instead (D300).
2. **The WebRTC LAN pair cannot form on staging.** The core offers `192.168.1.199:8443/udp` as a
   host candidate (D300 §1; `ListenLAN` in `internal/core/rtcapi/rtcapi.go`; staging's config has
   `udp_port = 8443`, `lan_addresses = ["192.168.1.199:8443"]`) **[C]**. The core's container
   publishes `0.0.0.0:8443->8443/udp` inside the sandbox **[L]**, but the yoloAI sandbox publishes
   only TCP from the VM into the sandbox (`--port 8443:8443` and the other three are TCP; a Docker
   `-p` without `/udp` is TCP only **[W]**). "No UDP published" is a known staging gap, noted
   2026-10-23 **[M]**. The laptop's checks to 192.168.1.199:8443/udp never reach the core, so ICE
   settles on the next pair that works: server-reflexive to server-reflexive over IPv4, both ends
   behind the same DS-Lite shared address. **Every one of 1,478 connections** in staging's logs
   since 2026-10-24 from 11.22.33.144 took `path=punched_ipv4`, and **none** took `direct_ipv4`
   **[L]**.
3. **The discovery would not offer the LAN name anyway, even to Chrome.** connectd logged, for this
   core, `client=2a0f:2000:4753:8afc::/64 coreFrom=11.22.33.144/32 isAtHome=false` **[L]**. The
   laptop asks over native IPv6, but the core's heartbeat comes over IPv4: the sandbox container has
   no IPv6 (`eth0` holds only 172.17.0.2) **[L]**. The discovery's `isAtHome`
   (`orpheum-connect/internal/clients/athome.go`) matches a household by a shared public IPv6 /64,
   or by a public IPv4 that is the household's own. It rightly refuses a carrier's shared IPv4,
   because DS-Lite's address belongs to many households **[C]**. With no IPv6 on the core's side,
   nothing can match.

**The bytes did not pass through app-staging or the edge VPS.** app-staging served the page, and
api-staging (through Cloudflare Tunnel) carried the signalling. The media went peer to peer over a
punched IPv4 pair, not a relay (`relayed` would say so). The core's public IPv4, which its heartbeat
comes from, is the same 11.22.33.144 that the laptop's packets arrive from **[L]**. So the pair
leaves the house over the DSL line and is **hairpinned back by the carrier's NAT** (the DS-Lite
AFTR, which RFC 6333 allows to hairpin between B4s **[W]**). That means up the 43.7 Mbit/s uplink
and back down **[M: owner-home-network]**. The core then judges the request as not from home,
correctly by D300 §3: the selected pair is not direct, and its client end is a public address.

**What this means for the 2026-10-26 bug reports [I]:** the link the bitrate controller measured on
staging (1,008 kb/s at the start, 2.8–5.6 Mb/s later) is the carrier hairpin, not the house's LAN.
Staging cannot reproduce a LAN playback in any browser until cause 2 is gone, and in Chrome not
until cause 3 is gone either.

---

## 2. How the web client chooses its path (question 1)

The page is served by the discovery's client origin (D308), and `reachCore` in
`web/apps/web/src/discovery-start.ts` chooses the path, following webrtc-transport.md W2c and the
discovery protocol's §3.2–§3.3:

1. **What the discovery offers.** `locate` / `pathsOf` / `realmSignIn` return `directUrls` (the
   core's published names, in order) and `lanUrls` (the subset that are LAN names). The discovery
   puts the LAN name first, and includes it at all, only when `isAtHome(client, heartbeat)` is true
   (connectd `clients.go`, which logs `a core's paths were asked for … isAtHome=`). Staging has no
   domain, no `[vps]` and `send_interface_addresses = false`, so its only published name is the LAN
   one. A device judged away from home gets **no direct names at all**.
2. **The permission.** `browserLocalNetwork()` reads the `local-network` permission. In Firefox it
   is always `denied` (`usesLanName`), so the LAN name is never fetched and never prompts.
3. **The first race.** `raced()` gives the names the page may try. With `denied` that is
   `directUrls` minus `lanUrls`, which on staging is empty. `firstDirectPath` then returns nothing,
   and `overWebrtc()` connects through the discovery's signalling.
4. **WebRTC's own choice.** ICE (in the browser, and pion in the core) picks the candidate pair.
   The core's answer carries its container host candidates, the house-address host candidate
   `192.168.1.199:8443` (D300 §1, UDP4 only), and a server-reflexive one from STUN
   (`stun.cloudflare.com`). Firefox's host candidates go out as mDNS `.local` names
   (draft-ietf-mmusic-mdns-ice-candidates **[W]**). That does not block a host pair: the core
   learns the laptop's address peer-reflexively, from the laptop's check to 192.168.1.199. That
   check has to arrive, though.
5. **Later upgrades.** `better()` looks every 30 s and up (kept-path.ts). It asks only the LAN
   names once the page has proven itself at home, and otherwise the non-LAN names. On staging in
   Firefox that is nothing, so the page stays on WebRTC.
6. **Home, as the core sees it.** A request over WebRTC is from home only when the selected pair is
   not relayed and its client end is in `home_networks` (D300 §3; staging has
   `home_networks = ["192.168.1.0/24", …]`). A punched pair from 11.22.33.144 is not.

## 3. The 2026-10-26 session, step by step (question 2)

| Step | What happened | Evidence |
| --- | --- | --- |
| Browser | Firefox on Linux, device 29 (user 1) | `device` table: `29, 'Firefox on Linux', 'web'` **[L]**; Firefox/158 user agents on staging on 2026-10-25 **[L]** |
| Paths offered | The discovery judged the laptop away from home, so no LAN name was offered | connectd 13:16Z: `client=2a0f:2000:4753:8afc::/64 coreFrom=11.22.33.144/32 isAtHome=false` **[L]**; the evening's requests are past connectd's retained log, and nothing changed in between **[I]** |
| LAN name tried? | No. Firefox never tries it, and here none was offered | `usesLanName`, `raced()` **[C]** |
| Direct probe failed? | No direct probe was made. No LNA prompt, no mixed content, no certificate or timeout was involved | as above **[C]** |
| Core knows a LAN address? | Yes: `192.168.1.199:8443`, offered both as a discovery LAN name and as a WebRTC host candidate | `config.toml` **[C]** |
| WebRTC pair | `punched_ipv4 remote=11.22.33.144:<port>`, connectMs about 540–680 | the 2026-10-26 logs, lines 37, 47, 98 and others **[L]** |
| Why not the host pair | UDP 8443 is not published from the VM into the sandbox | yoloAI's TCP-only `--port` flags; memory "no UDP published" **[M]**; 0 `direct_*` paths in all logs since 2026-10-24 **[L]** |
| Why not IPv6 | The core has no global IPv6 in the sandbox: `eth0` is IPv4 only, and the staging bridge has only the ULA `fd77:5e::/64` | `ip -br addr` **[L]** |
| Seen as local? | No. `clientAddress=11.22.33.144`, and the pair is not a direct one into `home_networks` | D300 §3 **[C]**; staging uses "administer from away" so that the admin still works (orpheum-ops staging README §4) |

**Not verified directly:** that the VM drops UDP 8443. The proof would be on the VM:
`docker port <sandbox>` or `ss -ulnp | grep 8443` shows no UDP mapping, or a `tcpdump udp port
8443` in the sandbox while the laptop connects shows nothing arriving. The 0-of-1,478 count makes
any other explanation unlikely **[I]**.

## 4. What the logs could not say (question 3)

Settling this took a database read, two services' logs and the sandbox's interfaces. Following
"digging means missing logs", these are the lines that would have said it at once:

1. **The page's path decision, reported to the core once per reach**, beside `a player's page
   moved to another path`. It should carry:
   - the browser family;
   - `usesLanName`;
   - the permission state;
   - how many `directUrls` and `lanUrls` the discovery offered;
   - which names were raced, and each one's outcome (answered, failed and how, or skipped and why);
   - the result (`direct <origin>` or `webrtc`).

   Today nothing from the client says why it is on WebRTC.
2. **The selected pair as the browser sees it**, from `RTCPeerConnection.getStats()`: the local
   and remote candidate types (host, srflx, prflx, relay), their protocol and family. It should
   also say **whether the pair to the core's LAN host candidate was tried and failed** (the
   `candidate-pair` stats with state `failed` or `in-progress` for the 192.168.1.199 candidate).
   That one line would have read "LAN host pair failed: the UDP port is unreachable".
3. **The core's end of the pair, by type and address**, in `webrtc: a client's connection`. Today
   it logs only the path class and the remote address. Adding `localType` and `localAddress` would
   have shown srflx with 11.22.33.144 on both ends, which is a hairpin.
4. **A named hairpin.** When the core's server-reflexive address equals the client's public
   address, the pair goes out to the carrier and back. It deserves its own path name (say
   `hairpinned_ipv4`) in the counts the System page lists (D274 §3), not a quiet `punched_ipv4`.
5. **The core's own reachability on its LAN UDP port.** Nothing at startup says whether
   `[lan] address`'s UDP port is published. The core cannot test that from inside, but a page at
   home can (see 2). The first connection from `home_networks`' prefix whose LAN host pair fails
   could be logged as a WARN that names the port.
6. **Why the discovery judged "away".** connectd logs `isAtHome` with the client's and the
   heartbeat's networks, which is good. It should also say *why* a household cannot be matched at
   all: a heartbeat over a shared IPv4 with no public IPv6 candidate can never match. That should
   be logged once per core as a standing condition, not inferred from each request.
7. **The playback log's `userAgent=""`.** Over WebRTC the user agent is not carried, so
   `a playback started` cannot name the browser. The device id or the device's name should be
   logged there instead.

## 5. Fix options (question 4)

### 5.1 The gap, put conceptually

There are two independent questions, and today they share one answer:

- **Can this device reach the core inside the house?** This is a question of reachability. The
  answers are a LAN HTTPS name, a WebRTC host pair, or nothing.
- **Is this device in the house?** This is a question of trust: admin (D297 §6), and which names
  the discovery offers. Today it is inferred from addresses: the discovery's /64 match, and the
  core's `home_networks` on a direct pair.

On staging the first fails for an environmental reason (no UDP), and the second fails because the
core's side has no IPv6 identity. In production on a normal Docker host the WebRTC host pair should
work, but the second failure can recur. Any household core whose heartbeat leaves over IPv4 only (a
Docker without IPv6 egress, which is Docker's default) and sits behind CGNAT or DS-Lite is never
judged at home by the discovery **[I]**. Chrome users there never get the LAN name. They still get
the WebRTC host pair, if UDP is published.

### 5.2 How others solved it **[W]**

- **Plex** publishes per-server, per-address names under `*.plex.direct`. For example,
  `192-168-10-220.<server hash>.plex.direct` resolves to the LAN address with a valid wildcard
  certificate. app.plex.tv simply tries them, and the local one wins on the LAN. It fails where a
  router's DNS-rebind protection drops private answers, and users whitelist `plex.direct`
  ([Plex: Why can't the Plex app find or connect](https://support.plex.tv/articles/204604227-why-can-t-the-plex-app-find-or-connect-to-my-plex-media-server/);
  [Plex forum: indirect connection on local network](https://forums.plex.tv/t/solved-plex-server-has-indirect-connection-on-local-network/793370);
  [How does Plex decide if the server is on a local network?](https://forums.plex.tv/t/how-does-plex-decide-if-the-server-is-on-a-local-network/406500)).
  Plex decides "local" by the connection the client actually made (its `LAN Networks` setting plus
  the source address), not by a pre-judgement of where the client is. Orpheum's LAN name
  (`*.orpheum-client-test.com` on staging) is the same idea. Plex predates browsers' Local Network
  Access, though, and now meets the same prompt.
- **Jellyfin** has no discovery and no relay. "Local" is the source address against its `LAN
  Networks` setting, and the setting fails exactly when a proxy hides the source, as the tunnel
  does here
  ([Jellyfin: reverse proxy, Known Proxies](https://jellyfin.org/docs/general/post-install/networking/reverse-proxy/);
  [How to identify a connection from the internet properly?](https://github.com/jellyfin/jellyfin/discussions/13239)).
  Jellyfin's lesson is that trust by source address needs the true source, which Orpheum's WebRTC
  pair gives only when it is direct.
- **WebRTC ICE.** Browsers hide host candidates behind random mDNS `.local` names, which the far
  side resolves only with multicast on the same link
  ([draft-ietf-mmusic-mdns-ice-candidates](https://datatracker.ietf.org/doc/html/draft-ietf-rtcweb-mdns-ice-candidates-04)).
  A server in a container cannot resolve them, but it does not need to: the browser checks the
  server's host candidate, and the server learns the browser peer-reflexively. That is why D300's
  design is sound, given a reachable UDP port.
- **Chrome's Local Network Access** asks before a public page fetches a private address, since
  Chrome 142. A fetch can declare `targetAddressSpace: "local"`, which also exempts it from mixed
  content checks. **WebRTC was exempt in the first milestone** and is "to be folded in", while
  WebSockets became gated in Chrome 147
  ([Chrome for Developers: Local Network Access](https://developer.chrome.com/blog/local-network-access);
  [WICG spec](https://wicg.github.io/local-network-access/)). Firefox's LNA and its worker problem
  are in idiosyncrasies.md. The risk for the plan **[I]**: if browsers gate WebRTC to private
  addresses later, the WebRTC host pair will need the same permission that the LAN name needs
  today.
- **DS-Lite.** An AFTR may hairpin between two B4s that share it, and that is what carried this
  pair ([RFC 6333](https://www.rfc-editor.org/info/rfc6333/)).

### 5.3 Options

**A. Make staging able to show a LAN path at all** (environment, not product).
- *A1.* Publish UDP 8443 from the VM into the sandbox. This needs yoloAI to support `/udp` ports,
  which is the owner's tool, or a one-off `iptables`/`socat` forward on the VM.
- *A2.* Accept the gap and say so wherever staging is judged. Never read staging's link, quality
  or "local" behaviour as the house's. Test the LAN path on the amd box (the owner's 2026-10-23
  choice, **[M]**).
- *A3.* Give the sandbox IPv6, so that the core's heartbeat and an IPv6 host pair can come from the
  house's /64.

**B. Report the path and its reasons** (section 4 items 1–5). These change no behaviour and are owed
under "digging means missing logs" in any case. Item 2, the browser's `getStats` pair report, is the
single most useful one.

**C. Recognise a hairpin, and stop treating it as an ordinary punch.** When the core's srflx
address equals the client's address, the core knows the two share a public IPv4. It also knows the
house's LAN host pair failed, which means the UDP port is not reachable. The core should name the
path (`hairpinned_ipv4`) and show an admin-facing warning: *"devices at home reach this server
through your provider, not your network: UDP port 8443 at 192.168.1.199 is not reachable."* This
must not grant home (D297 §6): a shared CGN address is many households.

**D. Let the discovery judge "at home" for an IPv4-only core.** The core could tell its discovery
the house's IPv6 /64 when it cannot see it itself. `[lan]` could carry it, or the deployment could
read it from the host. It could also report the house's public IPv6 /64 learnt from a STUN binding
over IPv6, where the machine has IPv6 but the container does not. This needs a design decision,
because it widens what the discovery is told about a household (the owner's call; it touches D290
§2).

**E. Make the in-house path not depend on address guesses.** Once connected over any path, the page
could ask the core for its LAN candidates and run a fresh ICE restart (W2d's restart, built
An earlier commit) aimed at the host pair only. If the host pair succeeds, the page is at home, by the
strongest evidence there is: a direct pair into `home_networks`. This is what D300 already trusts.
It only gives the house a second chance after a cold start in which the LAN pair lost its race. It
does not help where the UDP port is unreachable (staging), so it follows A, not replaces it.

### 5.4 Recommendation **[I]**

1. **B now**, as the fix for this report: the page's path decision with its reasons, the browser's
   selected-pair stats with the LAN host pair's state, and the core's local candidate type and
   address. All three are cheap, and each would have answered this report in one line.
2. **A2 now, and A1 if yoloAI can do it cheaply.** Record in the 2026-10-26 bug's notes and in
   staging's README that staging's playback is always the carrier hairpin, so the bitrate
   controller's link numbers there are not the house's.
3. **C next.** It turns a silent misconfiguration, which any household with an unpublished or
   firewalled UDP port will hit, into a named path and a warning. It needs no new trust.
4. **D** goes to the owner as a design question (§5.1's second question). It matters for Chrome
   users behind DS-Lite or CGNAT whose core is IPv4-only, which is the owner's own ISP pattern.
   **E** is worth it only after B shows that a house's LAN pair loses races it could have won.

**Not recommended:** relaxing the discovery's `isAtHome` to accept a shared IPv4 match, or counting
a hairpin as home. A DS-Lite or CGNAT address is shared across households, so either would offer
LAN prompts to strangers and admin to the wrong network (D297 §6, D300 §2).
