> **ABOUTME:** The build of D558's first step: one hosted relay for the beta's households, its
> credentials made by the paid discovery, and the wire contract the relay, the discovery and the
> deployment share. Read before changing `internal/relay` or Connect's relay credentials.

# The hosted relay's first step

- **Status:** IN PROGRESS — begun 2026-11-26 on the owner's go ("Yes do the relay first"). §1 (the
  credential, `discovery/relaycred`), §2 (the relay, `internal/relay` and `orpheum relay
  --config <file>`) and §3 (orpheum-connect's `internal/hostedrelay`) have code, and §4 runs on the
  edge VPS as `relay.orpheum.cc`.
  §5's items 1 to 3 have code, and so has D559, which changed who asks the relay and when.
  Production's discovery names it since 2026-11-26, for the owner's realm.
  What the staging walk found is at the end.
- **Depends on:** D558; the design is
  [the research's §4](../research/official-relay-2026-11-26.md#4-the-proposed-design), and this
  file is what is being built of it.

## What already carries it

Neither the discovery's protocol nor the core's connection code changes for this step:

- A core takes the TURN servers its discovery's status names, with their credentials, into every
  WebRTC connection it answers (`iceServersFor`, `cmd/orpheum/discovery.go`).
- A device takes the TURN servers the discovery's `found` answer names into the connection it
  offers (`ICEServers`, Connect's `internal/clients`).

So the step is three parts: a relay program, the discovery making its credentials, and the server.

## 1. The wire contract

One TURN server per relay, RFC 8656, long-term credentials, realm `orpheum-relay`.

**A credential** is a username and a password:

- the username is `<expiry>:<role>:<household>:<kbps>`:
  - `expiry`: Unix seconds, decimal, after which no new allocation is admitted;
  - `role`: `core` or `client`;
  - `household`: the discovery's opaque name for a household, `[A-Za-z0-9_-]{8,32}`, which names
    no account and no core;
  - `kbps`: the household's rate in kbit/s, decimal, 1 to 1,000,000;
- the password is the standard base64, padded, of HMAC-SHA1 under the shared secret over the
  username's bytes (the TURN REST form).

A relay holds one secret or several, each at least 32 bytes, and admits a credential made under any
of them: staging's discovery and production's each have their own, and a rotation has two for a
while. A secret never prints (rule 12).

**Lifetimes.** A core's lasts an hour and is made again at every heartbeat's answer; a client's
lasts 15 minutes, made in each `found` answer. An allocation made while its credential was good is
served after, as the embedded relay's is.

**What the discovery names.** To a core: `turn:<name>:3478?transport=udp`. To a client:
`turn:<name>:3478?transport=udp` and `turns:<name>:443?transport=tcp`, in one entry.

## 2. The relay (`orpheum relay`, this repository)

1. **Listeners:** TURN on UDP 3478 and TCP 3478; TURN over TLS on TCP 443, its certificate and key
   read from two files and read again when they change. No demultiplexer on 443 in this step: the
   server's 443 is the relay's alone.
2. **Closed to all but a household's own two ends** (D558 §4):
   - an allocation is admitted on a credential that is whole, unexpired and under a known secret;
   - a permission is granted for any address and opens nothing (D560: Firefox drops its relayed
     candidates when one is refused);
   - an allocation whose owner sends nothing through it for a minute is ended (D561 §1);
   - a datagram leaves a relayed socket only for the relayed address of a live allocation of the
     same household and the other role; anything else is dropped and counted.
3. **The household's rate** (D558 §1): one token bucket per household at the credential's `kbps`,
   over every byte relayed toward its clients; what it cannot pass is dropped.
4. **Limits**, each a refusal: 16 allocations a client credential, 64 a household, 128 a source
   address (D559 §3, the relayed ports being UDP 49160–51159). The first was 2 and starved the web
   client: Chrome allocates once for each interface and each of the relay's URLs on every
   connection, and a page holds up to four connections (staging, 2026-11-26). The third is the
   lead's, above a household's because a core's allocations all come from its one address; the
   owner's to confirm.
5. **What it records** (D558 §11): for each allocation that ends, a line with its household, its
   role, its transport (udp, tcp, tls), its source address shortened (an IPv4 to its /24, an IPv6
   to its /48), its seconds, its bytes each way and the highest rate it reached toward its client
   over any ten seconds; and a daily total a household. Nothing of what is relayed.
6. **Configuration:** a file naming the public address, the name, the listeners, the relayed
   ports' range, the certificate's files and the secrets' file.

Not in this step: the pairing of a household's own relay, the control connection to the
discovery, the reports to it, the 443 demultiplexer.

## 3. The discovery (orpheum-connect)

1. Its configuration names the relay (its name, the secret's file, and the rate, which the early
   beta sets far above any film's so the figures show what people use, D558 §14) and the
   households admitted: a hand-kept list of accounts (D558 §7).
2. A core of a listed household gets the core's credential in its status; a device finding that
   core gets the client's in `found`, after the core's own relay if it has one.
3. A household's name is derived from its account under a key of the discovery's, so the relay
   learns nothing it could look up.

## 4. The server (orpheum-ops)

The edge VPS (D558 §12): the binary under systemd as its own user, the filter and Hetzner's
firewall opened for UDP 3478, TCP 3478, TCP 443 and the relayed ports, a certificate for the
relay's name from Let's Encrypt, renewed on the server. The forwards to the owner's NAS and the old
staging core go; staging's and production's probe forwards stay.

## 5. Counting why connections fail (the research's phase 0)

What is recorded of each connection's path, and of each that never formed, for D558 §13, so that
the log says whether a device took the relay needlessly (the owner, 2026-11-26):

1. **The core**, on each connection's `clientConnected` line: `offered`, the client's candidates
   counted by type, and for a relayed one `relayedWhy` and `directPairs`, how each pair without a
   relay stood as it connected. `noDirectPair` and `directFailed` say the relay was needed;
   `directUnfinished` that the relay answered first; `directSucceeded` that it was not needed.
2. **The relay**, a `relayAllocationRefused` line for each allocation past a limit, naming it.
3. **The web client**, in its problem report: the kinds of candidates it gathered and offered and
   the errors of its gathering; and an offer with nothing past the device is gathered once more
   before it is sent.
4. Not yet: a connection that moves from the relay to a direct pair after it connected, or back,
   says nothing; and the System page counts `relayed` as one figure, not by reason.

## Proof

- The relay's rules each have a test that fails on revert, on real sockets on the loopback.
- Staging: a browser allowed to reach the relay's TCP 443 alone plays a film from the staging core.
  Shown 2026-11-26: a Chromium held to `turns:relay.orpheum.cc:443` and to relayed candidates
  signed in to the staging realm and played file 1286 for 30 s at 1280 wide, its selected pair its
  TLS allocation to the core's relayed address; the relay's lines for the walk name the client's
  allocations as `tls` and the core's as `udp`, the largest 15.6 MB toward the client at a
  ten-second peak of 6003 kbit/s, nothing dropped.
- A device queue item for the two friends' networks: VQ408.

## What the staging walk found (2026-11-26)

1. **Two allocations a client credential starved the web client** (§2.4, now eight): the second
   connection's allocations were all refused and its offer carried no candidate.
2. **One browser takes many allocations.** That walk's one browser took 33 client allocations and
   the core 11 over a sign-in and half a minute of a film: four a connection for each of its three
   or four connections at once, though one pair carries each. The relayed ports are 40 (Hetzner's
   firewall), and a household's limit is 16, which that one browser reached. Decided as D559: a
   device asks a relay only once a connection has not formed without one, the core only for an
   offer that carries a relayed candidate, and the ports and limits grow once the owner has
   widened the firewall. Production's discovery names the relay after that.
3. **A core has no relay credential for up to five minutes after the discovery restarts**: it
   takes one with its heartbeat alone. Open.
4. **The relay says nothing of an allocation it refuses.** A line for each refusal, with which
   limit, would have named finding 1 at once. It now does (§5 item 2).
5. **A relay that does not answer holds the core's answer** to an offer with a relayed candidate
   until the core has a reflexive candidate, or for five seconds and then fails it where it has no
   STUN server (seen in D559 §2's test, 2026-11-26). Before D559 §2 that was every offer. Open.
6. **Firefox never connected through the relay**, on the owner's phone on production and on
   staging: the relay's refusal of a permission for the core's other candidates carries no error
   code, and Firefox then drops every relayed candidate. Decided as D560: every permission is
   granted, the relayed sockets alone keep the relay closed. The staging proof had been walked in
   Chromium alone.
7. **One phone took 24 allocations and left eight behind for ten minutes** (production, the owner's
   retest): six a connection (three local addresses, two URLs), four connections, the fourth
   refused four past the credential's 16; the film went through UDP and the TLS allocations
   carried nothing; the UDP ones outlived the page by the library's ten minutes. Decided as D561:
   the relay ends an allocation idle a minute, and a connection asks the UDP address first.
8. **Each connection makes one UDP and one TLS allocation that end within their first second**,
   carrying nothing (Firefox, the phone and the desktop). Not explained. Open.
9. **D561 walked on staging** (2026-11-26). A Firefox held to relayed candidates kept its
   connections 13 minutes after the relay ended its spare allocations at one minute, then played;
   a Chromium the same over three minutes. With the ladder, both played asking the relay's UDP
   address alone and made no TLS allocation; a Chromium allowed the TLS address alone played
   through the last rung, 20 TLS allocations over the walk, five of them carrying anything. A
   browser's allocations left behind ended as idle 60 to 70 s after its page went.
