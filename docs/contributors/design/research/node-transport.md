> **ABOUTME:** How the core and a media node should actually talk — the options, what the field
> and the large operators converged on, and the measurement that reframes the question: the
> reports are not the payload.

# Node ↔ core transport

Researched **2026-09-21**. Sources: Netflix's Open Connect overview (primary, the PDF itself
rather than a summary of it), Tdarr's documentation, Immich's and Kyoo's source read locally
from the cache, GitHub's and AWS's agent documentation, and `nfs(5)`. Measurements are on the
usual rig (Intel i3-1315U, 6c/8t, 62 GB, Linux 6.17).

Backs [D125](../../decisions/working-notes.md)–[D129](../../decisions/working-notes.md).

## 0. The finding that reframes the question

**The node's reports are not the payload.** A 50,000-entry snapshot is **50.8 MB** and takes
**551 ms** end to end. That is the number the whole transport discussion has been implicitly
about, and it is noise.

What the node actually has to get to the core is **its derived artefacts**, and nobody has
written down how they cross:

| Node-produced, core-served | Reference library (3,500 items, 3,167 h) | Source |
| --- | --- | --- |
| Subtitle overlay bitmaps | **42 GB** | 12 MB/film × 3,500 ([D78](../../decisions/working-notes.md)) |
| Trickplay sprites | **5.7 GB** | measured 30,040 B per minute of video |
| Posters, three sizes | ~1.0 GB | [artwork.md](../plans/artwork.md) |
| Thumbnails, subtitle extracts, font subsets | < 1 GB | [D55](../../decisions/working-notes.md) |
| **One snapshot report** | **0.051 GB** | measured |

**Three orders of magnitude.** The tree says derived images are *"produced at scan time, in the
node"* ([artwork.md](../plans/artwork.md)) and that *"all of it lives in `cache/`"*, which the
architecture diagram draws **beside the core**. Both are stated; the arrow between them is not.
In the default install they are the same machine and the gap is invisible, which is exactly how
it survived four rounds of review.

## 1. What the large operators do

**Netflix Open Connect is the closest large-scale analogue there is**: an appliance sitting
inside a network Netflix does not control, managed by a control plane in AWS. Verbatim from
their own overview:

> OCAs do not store client data (for example - viewing history, DRM info, or member data).
> Essentially, OCA servers only do the following two things:
>
> ● Report their status to the Open Connect control plane services in Amazon Web Services
> (AWS). For example, they report health metrics, BGP routes they have learned from the BGP
> peer … and **what files they have stored on disk**.
>
> ● **Serve content via HTTP/HTTPS when it is requested by a client device.**
>
> The control plane services in AWS take the data that the OCAs report and use it to **steer
> clients via URL** to the most optimal OCAs given their file availability, health, and network
> proximity to the client.

Four things transfer, and it is worth noticing how exactly:

1. **The appliance does two things: report, and serve.** That is our node
   ([D33](../../decisions/working-notes.md)) with no adjustment at all.
2. **"What files they have stored on disk" is the report.** The largest video operator on earth
   has an appliance whose primary upward message is an inventory of its own disk. That is
   [D59](../../decisions/working-notes.md)'s snapshot, independently arrived at.
3. **The control plane decides; the appliance does not.** Steering, fill, storage layout and
   telemetry interpretation are all central. That is
   [principles §9](../../principles/README.md) and [D42](../../decisions/working-notes.md),
   at planetary scale.
4. **Bytes never transit the control plane.** Clients are steered *via URL* to the appliance.
   The control plane is a control plane.

Point 4 is the one that answers §0: Netflix does not move the payload to the centre, it moves
a URL to the client. **UNVERIFIED:** what an OCA does when it loses the control plane — a
partner-documentation summary says it stops serving, the primary overview does not say, and the
two readings disagree. Not relied on below; our answer differs regardless, because
[D37](../../decisions/working-notes.md) and [D44](../../decisions/working-notes.md) already say
a component reports rather than exits.

**Everyone else converged on one thing about direction.** An agent inside a network the control
plane does not own **dials out, and nothing dials in**:

| System | Mechanism | Inbound port on the agent |
| --- | --- | --- |
| GitHub Actions self-hosted runner | HTTPS **long poll**, 50 s timeout, reopened | none |
| AWS Systems Manager Agent | outbound 443 to `ssmmessages` message-gateway endpoints | none |
| **Tdarr node** | persistent outbound **Socket.IO** to server port 8266 | none |

Tdarr is the one in our own domain — a media server with a distributed transcoding node fleet —
and it is worth reading twice: *"no inbound Node port is required"*. (The Socket.IO and port
detail is from secondary summaries of `docs.tdarr.io`; the filesystem split below is from their
node documentation directly.)

Tdarr also already has our other split: **mapped** nodes share a filesystem with the server, and
**unmapped** nodes do not, with the system transferring files instead. That is exactly the choice
[transcoding.md](../plans/transcoding.md) leaves open.

## 2. What the open field does

Read locally from the cache:

- **Immich** (`server/src/repositories/machine-learning.repository.ts`). The core holds a
  **list** of worker URLs, pings each for health on an interval, and orders requests
  healthy-first-then-unhealthy, failing over down the list. It **reads the file and uploads the
  bytes** — `readFile(imagePath)` into a multipart `Blob` — so the worker needs no shared mount
  and can genuinely be on another machine. This is the model
  [transcoding.md](../plans/transcoding.md) calls *"the core streaming bytes to it"*, shipped.
  Note also what it means for us: Immich's server holds hostile image bytes in memory without
  decoding them, which is [D99](../../decisions/working-notes.md) in production.
- **Kyoo**. The scanner dials **in** to the API over HTTP with a static `X-API-KEY` from an
  environment variable; the transcoder validates JWTs against a JWKS. Media is a shared
  read-only mount (`${LIBRARY_ROOT}:/video:ro`) on both. The static key is a standing privileged
  credential of the kind [D105](../../decisions/working-notes.md) rejects for adapters — less
  bad for a service than for a deputy, but it is the same shape.
- **`rffmpeg`** (1,073★, written by a Jellyfin founder). **SSH.** Worth naming because it is the
  honest zero-infrastructure answer and it is what people actually reach for.
- **Jellyfin, Plex, Emby.** No such split exists. There is no prior art to borrow because the
  monolith never needed any.

## 3. The options

| # | Transport | Who dials | Shared FS | Streams? | Auth available |
| --- | --- | --- | --- | --- | --- |
| 1 | **Unix domain socket** | either, same machine | n/a | yes | socket file permissions + peer creds |
| 2 | **HTTP, node → core** | node | no | **yes** | TLS, mTLS, bearer token |
| 3 | **HTTP, core → node** | core | no | yes | same, but needs an inbound port on the node |
| 4 | **Long poll / WebSocket, node → core** | node | no | yes | as (2), plus commands with no poll delay |
| 5 | **File exchange directory** | neither | **required** | **no** | filesystem permissions only |
| 6 | **SSH** | core | no | yes | keys |
| 7 | **Message broker** (NATS/MQTT) | both, to the broker | no | yes | broker's |

(6) and (7) are listed to be dismissed. SSH means shipping or requiring an SSH server and
managing keys, for a channel we already need to authenticate anyway. A broker is a second
service, which [principles §1](../../principles/README.md) spends an innovation token on for
nothing at household scale.

(3) is in [media-nodes.md](../media-nodes.md) today, justified as *"for nodes behind a firewall
that cannot dial out"* — which is **backwards**: a node that cannot dial out to the core is not
more reachable inbound, it is less. The real case it serves is exotic enough to question whether
the mode earns its maintenance.

## 4. Measured

Realistic 50,000-entry snapshot — varied titles, release groups, containers, codecs, track
counts and language tags — **50.76 MB, 1,015 B/entry**.

```
== compression ==
  gzip -1    8,829,427 B    5.7x smaller   262 ms  (193 MB/s in)
  gzip -6    6,513,053 B    7.8x smaller   551 ms  ( 92 MB/s in)
  100 Mbit LAN: raw 4.1 s    gzip -1 0.7 s + 0.26 s compress
    1 Gbit LAN: raw 0.41 s   gzip -1 0.07 s + 0.26 s compress

== file transport (write .partial, fsync, atomic rename, core reads) ==
  node write+fsync+rename      27 ms
  core read+parse             524 ms
  END-TO-END                  551 ms   -- core cannot start until the rename

== HTTP transport (node POSTs, core parses the stream as it arrives) ==
  tcp loopback, plain    first record parsed at  0.1 ms   50,000 in 536 ms
  unix socket, plain     first record parsed at  0.1 ms   50,000 in 544 ms
  tcp loopback, gzip     first record parsed at  0.7 ms   50,000 in 686 ms

== command latency ==
  HTTP request/response, keep-alive, loopback: 0.048 ms
  file channel: latency IS the node's poll interval, by construction
```

Three readings:

- **Throughput does not discriminate.** Everything is within 30% of everything else and all of
  it is sub-second. Anyone choosing this transport on speed is choosing on noise.
- **Latency to the first usable record differs by ~5,000×** — 0.1 ms against 551 ms — because
  HTTP parses the stream as it arrives and the file channel cannot start until the rename.
  **This is [D118](../../decisions/working-notes.md)'s incremental ingest, for free, without
  tailing a `.partial` file.** D118 invented a mechanism that one transport gives away.
- **Compress, and let the link choose the codec.** In HTTP that is `Accept-Encoding`,
  negotiated per request and free; on the file channel it is a thing you invent. See §4a for
  which codec — the answer is not the same on both sides of the core.

### 4a. zstd against gzip, and why the answer differs per channel

[D129](../../decisions/working-notes.md). Same corpus, best of three, 8 cores, via
`github.com/klauspost/compress` — **pure Go, no cgo**, BSD-3-Clause, with optional amd64/arm64
assembly and generic fallbacks.

| codec | bytes | ratio | compress | decompress |
| --- | --- | --- | --- | --- |
| gzip -1 (stdlib) | 8,829,427 | 5.7× | 193 ms | 129 ms |
| gzip -6 (stdlib) | 6,513,053 | 7.8× | 456 ms | 97 ms |
| **zstd fastest** | **7,904,912** | **6.4×** | **122 ms** | **68 ms** |
| zstd default | 7,513,344 | 6.8× | 145 ms | 91 ms |
| zstd better | 6,602,112 | 7.7× | 313 ms | 80 ms |

**zstd fastest beats gzip -1 on all three axes simultaneously** — 11% smaller, 1.6× faster in,
1.9× faster out. No trade is being made. Decompression is the axis that matters most because
the **core** pays it on every report.

Total wire time (compress + transfer + decompress) for one snapshot:

| link | none | gzip -1 | zstd fastest |
| --- | --- | --- | --- |
| 100 Mbit | 4.06 s | 1.02 s | **0.83 s** |
| 1 Gbit | 0.41 s | 0.39 s | **0.26 s** |
| loopback / Unix socket | **0.04 s** | 0.33 s | 0.21 s |

**On a socket, compressing is 5× slower than not.** The default install's "link" is a memcpy.
That is an argument for negotiation rather than a special case to code.

**But `Content-Encoding: zstd` does not transfer to the client channel.** `caniuse`, read
2026-09-21: 85.17% global, Chrome/Edge 123+, Firefox 126+ — **Safari desktop from 26.0 only,
iOS Safari fully at 26.3, and Samsung Internet unsupported through v30**. Samsung Internet is
the Tizen lineage, and [D85](../../decisions/working-notes.md)'s whole premise is that
televisions never update, so a global percentage computed over phones is the wrong
denominator. zstd on the node channel, where both ends are our binary; gzip stays the floor
facing clients.

**The 96× trap.** The first run of this benchmark reported gzip at **96×**, using the synthetic
snapshot from the 2026-09-20 round — whose 50,000 lines are near-identical, differing only in an
episode number. Any compression figure from a generated corpus is a figure about the generator.
The realistic corpus gives **5.7×**, and that is the number.

## 5. The file channel's two real costs

It is not a bad idea — it is the only option that crosses a network filesystem with no port, no
certificate and no configuration, and on a NAS where a share is already mounted that is genuinely
zero friction. But two costs are load-bearing and neither is currently written down.

### 5a. The client attribute cache, which polling does not fix

`nfs(5)` defaults on Linux: **`acregmin` 3 s, `acregmax` 60 s, `acdirmin` 30 s, `acdirmax` 60 s**.
Attribute timeouts start at the minimum and **double towards the maximum** on each revalidation
that shows no change — so a quiet exchange directory converges on the *slowest* setting exactly
when nothing is happening.

Consequences the design assumes away:

- **A rename is atomic on the server and is not immediately visible to another client.** The
  core can keep seeing `snap.jsonl.partial` for up to `acdirmax` after the node renamed it.
- **[D35](../../decisions/working-notes.md) already noticed half of this** — *"the node must poll
  the config file as well as watch it, because the exchange directory may be on the share"* — but
  polling does not fix it. The poller reads the cache. It fixes *silence*, not *staleness*.
- **D118's tailing is a local-filesystem feature.** Over NFS the core's view of a growing file's
  length is cached, so *"items appear while the scan is still running"* arrives in 60-second
  lumps. The promise is not wrong, it is local.
- `actimeo=0`/`noac` removes all of it and costs a round trip per operation — and it is a **mount
  option the user controls**, not something we can set.

SMB is worse in a different way: some server/client combinations **reject a rename onto an
existing target** or do not make it atomic. We rename onto a *new* name, which mostly avoids it,
and it belongs in [testing-strategy.md](../plans/testing-strategy.md)'s `assumptions/` rather
than in a paragraph anyone will remember.

### 5b. The permissions it assumes are not the ones a home share has

[D35](../../decisions/working-notes.md) says *"the permissions split **is** the security model"*,
and that a careful user can enforce it with filesystem permissions alone. Correct, and the
operative word is **careful**.

[D107](../../decisions/working-notes.md) then established that the dangerous direction is
**core→node**, because `config.json` carries commands and, since
[D89](../../decisions/working-notes.md), deletion. Put those together on a typical home NAS —
where a media share is readable and writable by everything on the LAN, because that is how people
set them up — and **anyone on the LAN can write the command file**. D107 answered this for HTTP
with mutual authentication. **On the file channel there is no transport to authenticate**, so
the answer has to move into the payload: the command file is **signed**, and the node verifies
before obeying. Otherwise the file channel is the one transport where D107's finding has no
mitigation at all.

### 5c. And it makes you rebuild what HTTP has

D35 already hand-built acknowledgement — the `generation` counter echoed as `observed_config` —
because *"a shared directory offers no acknowledgement of its own"*. That is one of a set:

| Needed | HTTP | File channel |
| --- | --- | --- |
| Acknowledgement | status code | **D35's `generation`/`observed_config` pair** |
| Incremental delivery | chunked encoding | **not possible** (or D118's `.partial` tailing) |
| **Bulk core→node transfer** | a request body | **nothing** — added as `payload/` by [D142](../../decisions/working-notes.md) when sidecar export needed it |
| Compression negotiation | `Accept-Encoding` | invent it |
| Authentication | TLS / mTLS / bearer | filesystem permissions, if any |
| Resume a partial upload | `Range` | invent it |
| Error reporting | status + body | a file nobody is watching |
| Backpressure | TCP | disk fills |

Every row is buildable. The question the effort budget asks is why.

## 6. What this suggests

1. **Unix socket for the default install.** One machine, a supervised child, no network, no TLS,
   no enrolment, and the socket's permissions are the security model in exactly the sense D35
   meant. [media-nodes.md](../media-nodes.md) already says local roles should speak the same
   protocol as remote ones over a Unix socket; this makes that the primary path rather than an
   aside.
2. **Node dials out for the remote case.** The whole industry converged here and the reason is
   not only firewalls: **if the node always initiates, the dangerous direction stops existing as
   a network path.** D107's spoofed core cannot write to a node that has no inbound surface; it
   can only try to answer a connection the node opened and authenticated. The security argument
   and the NAT argument point the same way, which is unusual and worth taking.
3. **Keep the file channel as the fallback it was always described as** — with 5a and 5b written
   down, and the command file signed.
4. **Do not move the artefacts to the core.** Netflix's answer to §0 is a URL, and
   [media-nodes.md](../media-nodes.md) already contains ours under *"what this permits later, for
   free"*. The open question is not whether but **which**: 42 GB of overlay bitmaps should stay
   where they were produced, while fifty posters on a home page want to be one hop away, not
   fifty redirects.
