> **ABOUTME:** Orpheum and orpheum-connect read against the classes of bug Plex2Shell chained to
> run code on Plex servers in October 2026: what holds, what does not, and the fixes the owner chose.

# The Plex2Shell classes, read against Orpheum

**What prompted it.** Plex Media Server was exploited in the wild in October 2026 by a chain of four
CVEs (CVE-2026-96652, -96654, -96655, -96656; write-up "Plex2Shell", zmain.info, 2026-10-16; the
owner sent the Hacker News thread). The chain's steps:
1. A token gate checked that a token header was there, not that it was valid.
2. A client's field, spliced raw into a callback URL, made the server request itself on loopback.
3. Requests from loopback were trusted without a token.
4. An identifier was spliced unencoded into an internal URL.
5. Outbound requests whose URL *began* with `http://127.0.0.1` were given the owner's token, so
   `http://127.0.0.1.<attacker>.sslip.io` took it away.
6. An admin-writable preference was appended verbatim to x264's options (`dump-yuv=<path>`): an
   arbitrary file write.
7. The transcoder fetched media from any URL.
8. The transcoder `dlopen()`ed every `.so` in a writable directory.

**How the review was done.** On 2026-10-20 an agent that had not built the code read Orpheum at
An earlier commit and orpheum-connect at an earlier commit for each class, and changed nothing. The building agent
then checked T1 and N1 against the code; both hold.

## What holds

- **Locality never stands in for a credential.** Coming "from home" only decides whether an admin
  already signed in with a valid session administers on that request (`administersHere`). There
  is no unauthenticated loopback API. No `X-Forwarded-*` header is read, and WebRTC requests drop
  `Origin`, `Host`, `Forwarded` and `X-Forwarded-*`.
- **Sessions are checked by the token's hash.** The check covers the grant's kind, its user, its
  idle expiry and that the person is still there. `requireSignedIn` is the one way in.
- **No outbound request picks its destination by string prefix:**
  - TMDB, its images and Cloudflare use fixed bases, with escaped or pattern-checked parts.
  - OpenSubtitles' links are checked by parsed host.
  - The discovery is https only, but for loopback.
- **FFmpeg's inputs are files or pipes alone** (`execx.FileInput`, `PipeInput`). Every argument
  is a number or a constant, and the environment is empty. No admin setting reaches an argument,
  an environment variable or a path.
- **A confined FFmpeg** may write only its run's directory, without execute or symlink rights.
  Nothing it can write is later loaded as code, and the node container has no network.
- **Library roots are the node's own configuration** (D107), opened through `os.Root`, so an admin
  cannot make `/` a library.
- **connect's outbound requests are bounded:**
  - its probe dials public IP literals only, with no redirects;
  - its DNS-01 proxy takes only `_acme-challenge.<coreId>.<zone>`;
  - its operator panel needs a passkey.

## Findings

| ID | Severity | What | Fix |
| --- | --- | --- | --- |
| N1 | medium-high | FFmpeg's seccomp filter refuses only inet and packet sockets, and Landlock does not govern `connect()` on a named Unix socket. The core takes any process on its node socket as the node, by name alone (`nodes/enrol.go`). A crafted file that exploits a decoder bug could say hello as `local`, take over the node's queue and post forged results. | Refuse every socket family to FFmpeg; the node proves itself with a secret FFmpeg cannot read |
| T1 | medium | Activating a device for someone else checks `IsAdmin()`, not `administersHere` (`api/pairing.go`). A stolen admin session used from away can pair the attacker's device as anyone, an admin who administers from away included. | `administersHere`; never activate for another admin |
| N2 | medium | The release node's `confinement_policy` is `"report"`: on a kernel without Landlock, FFmpeg runs unconfined with a warning | Default to `"require"` |
| L1 | medium (against admin from home) | The default home networks are loopback, 10/8, 172.16/12, 192.168/16 and `fc00::/7`, and the deployment falls back to 192.168/16 and 172.16/12 without a LAN address. A reverse proxy, docker-proxy or a rootless port driver then makes the internet look like home. | No loopback, Docker, 10/8 or ULA by default; require the LAN address |
| L2 | medium-low | The core's WireGuard peer allows every source, and the VPS keeps the client's source address, so whoever holds the edge VPS can inject packets from a home address | Drop home and private sources arriving on wg0 |
| L3 | medium-low | The house's IPv6 /64 is learned in part from STUN servers the discovery names, so a discovery could make its own /64 home | The house's /64s from host candidates only |
| O1 | low-medium | Outbound clients follow redirects; Go keeps custom headers, OpenSubtitles' `Api-Key` among them, on a redirect to anywhere | No redirects on clients that carry credentials |
| O2 | low | UPnP's description and control URLs come from the router's answer, not tied to the gateway, so a LAN device can make the core send blind requests to itself | Require the gateway's address |
| — | low | Watch Together's relay credential is shared by everyone who looks a core up by a link | Per lookup, short-lived |
| C1 | low | connectd trusts `CF-Connecting-IP`, and its listeners and operator panel listen on every address of a network that also holds wg0 | Bind to the addresses meant |
| T2 | low | Libraries' file paths are shown to admins away from home (`IsAdmin()` in `browse.go`, `libraries.go`) | `administersHere` |

The owner chose all of them on 2026-10-20. L3 was built otherwise than the table says, by the owner's
choice that day: a deployed core sits on Docker's translated IPv6 network, so it has no public host
candidate, and host candidates alone would have ended admin over IPv6 at home (VQ43). The core instead
gathers through STUN servers of its own config (`webrtc.stun_servers`; a deployment's
`stun.cloudflare.com`), never its discovery's, and takes only TURN servers from the discovery.
