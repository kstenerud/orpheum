> **ABOUTME:** W4b: `orpheum discovery`, the single-target discovery a self-hoster runs on a VPS
> for their one core (D273 §3, D274 §5), built as public packages the closed multi-core service
> imports (D275 §1). The steps, the shape, and what waits on the owner.

# W4b: the single-target discovery

- **Status:** IN-PROGRESS — begun 2026-10-11 on the owner's word (*"let's move on with the
  milestone"*), the next phase of [webrtc-transport.md](webrtc-transport.md) after W4a.
- **Depends on:** D273 §3 (the owner's own discovery, one core), D274 §5 (it has the paid tier's
  shape: it serves the web app, taking the files from the core; it claims its core by
  `orpheum discovery claim <code>`; it relays by TURN; the tunnel becomes optional), D275 §1 (public
  building blocks, the closed service assembling them for many cores, nothing in Orpheum it does
  not use), [discovery-core.md](discovery-core.md) (the core's side, C1–C5, built), and
  [discovery-protocol.md](../discovery-protocol.md), the protocol both discoveries speak, whose §2
  and §3 it answers for one core (moved 2026-10-13 from the paid service's spec in the shared files).

## The shape

- **Public packages under `discovery/`** at the module's root, not `internal/` (D275 §1), so the
  closed service imports them. Each has a `doc.go` with the same headings as every package's, and
  the gates that walk `cmd/` and `internal/` walk `discovery/` too.
  - `discovery/discoveryproto`: the wire types, moved from `internal/discoveryproto`, which the core
    and the discovery both speak; its name kept, so it reads at every use as it did.
  - `discovery/proof`: what a signature is over, and its check: Ed25519 over
    `orpheum-discovery-v1:<purpose>:<nonce>`, a nonce working once.
  - `discovery/single`: the core-facing protocol answered for one core (challenge, enrol, session,
    status, addresses, signal, acme-challenge, leave) and the client-facing one (§8's paths and
    offer), over a small store of its own: one file, since a single-target discovery holds one core.
- **`orpheum discovery`** is a mode of the one binary (`cmd/orpheum`), as core and node are: it
  serves `discovery/single` over HTTPS, runs STUN through the embedded TURN relay pion already gives
  the binary (no new dependency), and `orpheum discovery claim <code>` binds the core whose code it
  is.
- **Nothing for many cores in Orpheum** (D275 §1): accounts, billing, names under a shared zone and
  rate limits across customers are the closed service's.

## Steps

| Step | What | State |
| --- | --- | --- |
| W4b.1 | The public tree: `discovery/discoveryproto` (moved from `internal/discoveryproto`) and `discovery/proof`, the core signing with it; the architecture tests that load `cmd/...` and `internal/...` load `discovery/...` too, and `TestArch_PublicPackagesImportNothingInternal` holds the tree free of `internal/` | BUILT 2026-10-11 |
| W4b.2 | `discovery/single`, the core-facing protocol for one core against the core's real client (`internal/core/discovery`), whose fake in its tests it replaces as the reference; the state in one file; `orpheum discovery` and `orpheum discovery claim <code>` | the package BUILT 2026-10-11 (`discovery/single`, with `discovery/safe32code` moved out of `internal/core/auth` for its claim code): enrolment, claiming, sessions, status, heartbeat, the certificate's record through a DNS provider when given one, leaving, and the core's signalling, the core's own client tested against it end to end. The command waits on Q5 |
| W4b.3 | STUN: the discovery's `iceServers` in its status answer point at its own STUN, the embedded relay's | |
| W4b.4 | The client-facing protocol (§8): paths and offer for the one core, passing the client's offer on the signalling connection; locate once the core registers its link hashes (Q4 below) | |
| W4b.5 | Serving the web app at the owner's domain, its files taken from the core (D274 §5) | waits on Q3 |
| W4b.6 | An end-to-end test: a core, its discovery and a browser, the browser reaching the core through the discovery's offer; then the owner's real run (W5) | |

## Questions for the owner

1. **Where the public packages live:** `discovery/` at the module's root (as above), or another
   name. Their import path is what the closed service writes, so it is worth fixing once.
2. **Names and the certificate on the owner's domain:** the core asks its discovery to set its
   DNS-01 record (`PUT /acme-challenge`, C4). A single-target discovery would set it in the owner's
   own DNS through a provider token it holds (the core already speaks Cloudflare's API for its own
   name). Is Cloudflare enough to begin with, and is the core's name `<coreId>.<owner's zone>` as on
   the paid tier, or one name the owner chooses?
3. **"Taking the files from the core"** (D274 §5): the discovery could fetch the built client from
   its core over the signalling connection as the core starts, or over the core's HTTPS, or the
   operator could copy them. Which?
4. **Locating on a single-target discovery:** the spec's client API finds a core by the hash of a
   link's secret, which the core registers (`PUT /locate-hashes`, not built). A discovery with one
   core could answer `paths` to anyone who asks, since it serves that core alone and its owner chose
   to publish it. Keep the hashes, which hide the core's addresses from strangers, or answer
   anyone?
5. **How `orpheum discovery` runs on the VPS:** the core speaks only to an `https://` discovery. Does
   the discovery get its own certificate for its name (by ACME, through the same DNS provider as Q2),
   or sit behind a reverse proxy or the tunnel the VPS already runs, listening on plain HTTP on the
   loopback? And are its settings (its zone, its listen address, where its record is kept) a config
   file in the core's form, made by `orpheum default-config --discovery`, or flags?
