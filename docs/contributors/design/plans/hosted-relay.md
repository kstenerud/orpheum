> **ABOUTME:** The build of D558's first step: one hosted relay for the beta's households, its
> credentials made by the paid discovery, and the wire contract the relay, the discovery and the
> deployment share. Read before changing `internal/relay` or Connect's relay credentials.

# The hosted relay's first step

- **Status:** IN PROGRESS — begun 2026-11-26 on the owner's go ("Yes do the relay first"). Nothing
  below has code yet; each part's line says when it does.
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
   - an allocation's permissions are for the relay's own public address alone;
   - a datagram leaves a relayed socket only for the relayed address of a live allocation of the
     same household and the other role; anything else is dropped and counted.
3. **The household's rate** (D558 §1): one token bucket per household at the credential's `kbps`,
   over every byte relayed toward its clients; what it cannot pass is dropped.
4. **Limits**, each a refusal: 2 allocations a client credential, 16 a household, 50 a source
   address. The figures are the research's estimates.
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

What a core records of each connection's path, and of each that never formed, for D558 §13. After
parts 1 to 4 work on staging.

## Proof

- The relay's rules each have a test that fails on revert, on real sockets on the loopback.
- Staging: a browser allowed to reach the relay's TCP 443 alone plays a film from the staging core.
- A device queue item for the two friends' networks.
