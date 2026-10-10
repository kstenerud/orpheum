> **ABOUTME:** The core's side of the discovery protocol (D273): its identity key, enrolling with a
> discovery (the paid service at app.orpheum-app.com or the owner's own), keeping its addresses
> current, and answering WebRTC offers the discovery passes on. The phases, the wire as the core
> speaks it, and where each stands.

# Discovery: the core's side

- **Status:** IN-PROGRESS — begun 2026-10-09 on the owner's word (*"Do the core side of the
  discovery protocol"*). The service is a separate, closed-source project, specified in the
  sandbox's shared files (`discovery-service-spec.md`, draft 3); this plan builds the core's end of
  its §3 against a fake service, and the spec is kept in step with what is built here.
- **Depends on:** D273, D214 §4, [webrtc-transport.md](webrtc-transport.md) (W4 is this plan's
  signalling), the spec.

## The wire, as the core speaks it

Paths are under the discovery's base URL (`https://api.orpheum-app.com/core/v1` for the service,
the owner's own otherwise). JSON bodies; every call carries `Orpheum-Discovery-Protocol: 1`, and an
answer carrying `Orpheum-Discovery-Deprecated: <date>` makes the core warn its admin (D273 §6, §7).
The types are `internal/discoveryproto`'s, kept apart so they can move to the shared protocol
module once its licence is chosen (the spec's Q6).

| Call | What |
| --- | --- |
| `POST /challenge` `{publicKey}` → `{nonce}` | a nonce to sign, for enrolling or a session |
| `POST /enrol` `{publicKey, protocol, nonce, signature}` → `{coreId, claimCode, claimExpiresAt}` | a pending core and its claim code |
| `POST /session` `{coreId, nonce, signature}` → `{token, expiresAt}` | a bearer token for the calls below |
| `GET /status` → `{state, zone, claimCode?, claimExpiresAt?}` | pending, claimed, grace, frozen, released or disabled |
| `PUT /addresses` `{protocol, orpheumVersion, apiMajor, candidates, turn}` → `{published, probe, reachableFrom}` | the heartbeat |
| `GET /signal` (WebSocket) | offers `{type:"offer", offerId, sdp}` in; `{type:"answer", offerId, sdp}` or `{type:"refusal", offerId, reason}` out |
| `DELETE /core` | the core leaves |

A signature is Ed25519 over `orpheum-discovery-v1:<purpose>:<nonce>`, the purpose `enrol` or
`session`, so one made for one cannot be used for the other.

## Phases

| Phase | What | State |
| --- | --- | --- |
| C1 | The identity key: Ed25519, made on first start, `identity.key` in the persistent directory beside `secret.key`, mode 0600; kept in backups until the spec's Q12 is answered | BUILT 2026-10-09 |
| C2 | `internal/discoveryproto` and `internal/core/discovery`: connect (challenge, enrol, claim code), poll the status until claimed, sessions, the heartbeat every 5 minutes with the core's candidates; the discovery's state kept in `discovery.json` in the persistent directory; `GET/POST/DELETE /discovery` for an admin; the System page's Discovery section with the claim code and what the probe found | BUILT 2026-10-09, but its candidates are the machine's interface addresses alone (C5 adds mapped ones) and TURN is not sent (W6) |
| C3 | The signalling connection: offers the discovery passes on answered by rtcapi, without a signed-in check (the discovery checked the locator); reconnects with backoff | BUILT 2026-10-09; reconnects every 30 s rather than with growing backoff |
| C4 | Certificates through the discovery: a wildcard `*.<coreId>.<zone>` by DNS-01, the discovery setting the TXT record; HTTPS answering every name under it | BUILT 2026-10-09: `*.<coreId>.<zone>` through `PUT/DELETE /acme-challenge`, kept as `discovery-tls-*.pem`; a browser is served the certificate for the name it asks; a core with no `https.domain` opens HTTPS on `https.listen` once the wildcard is obtained, and its heartbeat's candidates carry that port. The System page's certificate line still shows only the own name's |
| C5 | Port mapping: UPnP IGD, NAT-PMP and PCP, on by default, reported on the System page with the probe's result (D273 §7) | BUILT 2026-10-10, hand-written on the owner's word (*"Make your own implementations"*): `internal/core/portmap`, PCP → NAT-PMP → UPnP IGD for IPv4, UPnP IGD 2 pinholes for IPv6, `should_map_ports` (on), only while the discovery publishes the core. Asks only the default route's router: behind double NAT the outer router is the owner's to set; PCP for IPv6 not yet |

## Found while planning

- **A core in a container sees the container's addresses**, not the host's: the bridge network's
  private ones. The heartbeat's `interface` candidates are only real under host networking. Until a
  deployment without the tunnel (D272 §3) chooses its network, a containerised core reports its
  configured addresses and the discovery's observed one; which is the spec's to say (§3.4).

## After C5: what the network research found (D274)

[homelab-networks-2026-10-10.md](../research/homelab-networks-2026-10-10.md) §6 read against
miniupnpd's source, none yet tried on a real router. Since D274 port mapping is an upgrade, not the
way in, so these wait behind W4:

- **IPv6 pinholes are refused by miniupnpd** unless asked over IPv6 from the address opened, and
  for a port of 1024 or more; OpenWrt advertises IGD 1, which has no IPv6 firewall control. Ask
  over IPv6 (UPnP or PCP), on a high port.
- **Map a high port** of the core's choosing (D274 §2): stock OpenWrt maps only 1024–65535.
- **miniupnpd maps nothing for IPv4 when its own WAN address is private**: every inner router of a
  double NAT. Say so on the System page rather than retry.
- **The candidates carry addresses no one outside can use:** 100.64.0.0/10 (carrier NAT, and
  Tailscale), IPv6 temporary addresses, a bridge network's. Leave them out, or scope them.
