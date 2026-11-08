> **ABOUTME:** The core's half of the discovery infrastructure plan of 2026-10-13: what a
> self-hoster's core needs to use the paid service or their own discovery (network, names, values),
> and the Orpheum changes the plan asks for so staging and the production NAS run at once behind one VPS.

# Discovery infrastructure: what the core needs, and what Orpheum must change (2026-10-13)

**Scope.** The paid service's infrastructure plan, written the same day in its own repository
(`docs/research/infrastructure-2026-10-13.md` in the service's repository, "the plan"), covers the
service's repositories, zones, machines and bring-up. This file holds the part that concerns only
Orpheum: what a core needs from its network and its deployment to use a discovery, and what Orpheum
must change for the plan. Read: [the protocol](../discovery-protocol.md),
[discovery-core.md](../plans/discovery-core.md), [discovery-mode.md](../plans/discovery-mode.md),
D272, D274, D275, D290, D291, `deploy/vps/`, `deploy/staging/`, `deploy/release/README.md`,
`./staging`, `internal/core/providers/cloudflare/`, `internal/core/discovery/`,
`cmd/orpheum/https.go`.

**Markers.** **[V]** checked at the source named (the code at an earlier commit, or a provider's page cached
under `/yoloai/cache/discovery-infra/`). **[I]** my inference or recommendation. "Audit2" is the
service's second security audit of 2026-10-13 (its findings I1–I15), kept in the service's
repository.

## Summary

- **A core needs nothing inbound to use the paid service** (D274 §2): outbound HTTPS and a
  WebSocket to `api.orpheum-app.com`, outbound UDP to the service's STUN servers, outbound HTTPS to
  Let's Encrypt, and UDP for WebRTC. No domain, no DNS provider, no Cloudflare token, no VPS.
- **The plan asks four things of Orpheum:** the tunnel's address chosen per deployment, so staging and
  the production NAS run at once with their own keys (revises D272 §8); `deploy/vps/` updated for forwards matched
  by destination address, a filter chain and several peers (audit2 I14, I1); `configured` candidates
  (K4) and the owner's TURN entry, which the staging core's VPS path needs; and, only as a fallback
  the plan does not recommend, DNS-01 through a CNAME.
- **The staging core changes configuration, not code:** no `https.domain`, the staging discovery's
  address entered under *use my own*, the Cloudflare token deleted.

## 1. What a core needs to use the paid service

| Need | What | Why |
| --- | --- | --- |
| Outbound TCP 443 | `https://api.orpheum-app.com/core/v1`, the core's default (`ServiceURL`, `internal/core/discovery/discovery.go`) **[V]**; Cloudflare's anycast addresses, behind which the service sits (the paid spec §7.3) | enrolment, sessions, heartbeats, the ACME proxy (protocol §2) |
| A long-lived WebSocket | `/core/v1/signal` on the same name, kept open, pinged every 25–30 s (protocol §2.5) | WebRTC offers; Cloudflare's releases drop sockets, so the core reconnects with jitter (C11) |
| Outbound UDP 3478 | `stun1.` and `stun2.` under the service's zone (the plan: `orpheum-core.app`, two addresses on one probe machine) | the core's server-reflexive candidates and its NAT measurement (protocol §2.6, §2.9) |
| Outbound TCP 443 to Let's Encrypt | `acme-v02.api.letsencrypt.org`, or the directory in `https.acme_directory` **[V, `cmd/orpheum/config.go`]**, which the discovery's wildcard uses too **[V, `cmd/orpheum/https.go`]** | the core runs ACME itself (protocol §2.7) |
| UDP, any port, both directions once punched | ICE between client and core | WebRTC data channels (protocol §3.2) |
| Inbound | **nothing required**; TCP 443 mapped by the router (TCP only, D290 §9; 443 first, D291 §7) or opened in the IPv6 firewall is an upgrade | direct paths (D274 §2) |
| DNS | none of the owner's: the core's names are the service's zone's, computed or written by the service | protocol §2.7 |
| Values | `domain` and `[vps]` not needed; today `values.toml` still asks for them (the paid spec §9.1 item 21; D272 §3, decided, not built) | a deployment without the tunnel |

**A core in a container** sees the container's addresses, and its port mapping asks the bridge's
gateway, not the router, so a containerised core reports `configured` addresses or runs on the host's
network ([discovery-core.md](../plans/discovery-core.md), *Found while planning*; `portmap`'s doc).
The release compose file puts the core on a bridge network (`.staging/compose.yaml` **[V]**), so the
tunnel-less deployment must choose one of the two [I].

**A strict egress firewall** at a household must allow the three destinations above and UDP for ICE;
Cloudflare's addresses change, so a name-based rule is needed [I].

## 2. What an owner's own discovery needs (W4b)

From the protocol and [discovery-mode.md](../plans/discovery-mode.md), for the VPS guide of W4b.8 [I]:

- **A domain of the owner's that holds nothing else they value.** The names are the owner's domain's
  (protocol §2.7), set through their DNS provider; a Cloudflare token reaches a whole zone, never a
  name, and subdomain zones are Enterprise-only (audit2 I1), so a domain shared with mail or a
  website hands the VPS the power to rewrite them.
- **Ports:** TCP 443 (the discovery's API and the web app); UDP 3478 (STUN); TURN on TCP and UDP 3478
  and its relay range if the owner relays; UDP 51820 only while the WireGuard tunnel is used.
- **The NAT measurement** needs a second STUN server on another address (protocol §2.6): a second
  address on the VPS, or `stun.cloudflare.com`, free and outside the owner's control (discovery-mode.md
  question 7).
- **The VPS's own hygiene** (audit2 I14): a forward chain admitting only the forwarded ports, SSH
  limited to the owner or over the tunnel, STUN rate-limited per source.

## 3. What the plan asks Orpheum to change

### Change 1 — the tunnel's address per deployment (revises D272 §8)

**Today:** staging and the production NAS share one key pair, so only one runs its tunnel, and
`media.orpheum-app.com` is swapped between them (D272 §2, §8). The tunnel's addresses are fixed:
`10.77.0.2` and `fd77::2` in `deploy/staging/tunnel.sh` **[V]**, and the VPS's peer names them.

**Wanted** (the owner: a second WireGuard peer and key and separate names, so both run at once): the
values file's `[vps]` gains the deployment's tunnel addresses, defaulting to today's, which the
tunnel script, its compose environment and the rendered `wg0.conf` take. The plan gives the staging
core `10.77.0.3`/`fd77::3` on its own key, the production NAS keeping `10.77.0.2` and its key, untouched.

**Touches** [I]: `internal/deployment/templates/` (values, compose, `wg0.conf`), `deploy/staging/tunnel.sh`,
`write-deployment`'s values checks, `deploy/staging/README.md`, `deploy/release/README.md` §3 step 1
(keys are no longer copied from staging) and §6 (no more swapping). A decision recording the
revision of D272 §2 and §8 comes first (AGENTS rule 1). **Tests:** a values file naming `10.77.0.3`
renders a tunnel and a compose environment with it; one naming none renders today's.

### Change 2 — `deploy/vps/` for several peers and addresses

The plan's §3.2 has the sketch. For the generic, public guide [I]:

- every forward matches its destination address (`ip daddr`), so a second address on the VPS (a
  second deployment, or the owner's own discovery beside the core's forward) is not sent to the first
  core;
- a forward chain with policy drop, admitting `ct status dnat` and the cores' own outbound relay
  traffic; an input chain admitting WireGuard and SSH from the tunnel only (audit2 I14);
- `[Peer]` blocks per deployment, each its own key and addresses;
- §2's token wording corrected: an *Edit zone DNS* token edits every record of the zone, so the guide
  advises a domain used for nothing else (audit2 I1; §2 above);
- §4's check extended: `nft list ruleset` shows each address forwarded to its own peer.

### Change 3 — the staging core's configuration (no code)

When staging moves to the staging discovery [I]: `domain` emptied in `.staging/values.toml`, so the
core serves no name of its own; `[vps] public_ipv4` set to the plan's Floating IPv4; the discovery
connected under *use my own* with `https://api.orpheum-app-staging.com/core/v1` (the address check in
`internal/core/discovery` accepts any `https://` URL **[V]**); `dns_provider_token` deleted from
Settings. U5's plan to hide the address behind *use my own* must keep that path working, since
staging depends on it.

### Change 4 — `configured` candidates and the owner's TURN (K4, K13)

The staging core reaches the internet directly only through the VPS, so its direct IPv4 path is a
`configured` candidate (protocol §2.4), and its voice relay the owner's TURN entry with time-limited
credentials (C14). Both are planned (K4, K13) and neither is built [V: the core sends no `turn` and no
`configured` candidate, protocol §2.4]. Until they are, the staging core is reached by WebRTC alone,
which is itself the baseline worth testing (D274 §2).

### Not recommended now — DNS-01 through a CNAME

If the legacy name `media.orpheum-app.com` had to outlive the revocation of its zone-wide token (the
plan §2.8), the core could follow a CNAME at `_acme-challenge.<name>` and write the TXT at its target,
in a throwaway zone its token reaches, as many ACME clients do. Today `internal/core/providers/cloudflare`
writes the TXT at `_acme-challenge.<name>` in the zone that holds the name and follows no CNAME
(`zoneOf`) **[V]**. The plan prefers retiring the legacy name by moving the production NAS onto the paid
service, and a separate domain as the fallback, so this stays unbuilt.

## 4. Dates that bind Orpheum's side

- **`media.orpheum-app.com`'s certificate** on the production NAS was issued 2026-10-13 and expires 2027-01-11
  **[V, Cert Spotter]**; the core, renewing 30 days before expiry as built (protocol §2.7), first tries
  about 2026-12-12. Once the owner revokes the token (the plan's step 29), renewals fail and the name
  works until 2027-01-11.
- **The production NAS joining the paid service** (the paid spec's M-S7 order) is therefore due by about
  2027-01-04, or the fallback domain is set up before then.

## 5. For the owner

1. **Record the revision of D272 §2 and §8** (separate keys and names for staging and the production NAS, no more
   swapping) before change 1 is built. *Recommended:* yes, as the owner asked.
2. **Change 2's public guide advising a dedicated domain** for an own discovery or core. *Recommended:*
   yes (audit2 I1).

## Sources

- Orpheum at an earlier commit: `deploy/vps/{README.md,orpheum.nft,vps-wg0.conf}`, `deploy/staging/{README.md,tunnel.sh}`,
  `deploy/release/README.md`, `./staging`, `.staging/{values.toml,compose.yaml}`,
  `internal/deployment/templates/values.toml`, `internal/core/providers/cloudflare/client.go`,
  `internal/core/discovery/discovery.go`, `cmd/orpheum/{https.go,config.go}`, `internal/core/portmap/doc.go`.
- Certificate Transparency: Cert Spotter's issuances for `orpheum-app.com`, fetched 2026-10-13
  (`/yoloai/cache/discovery-infra/certspotter-orpheum-app.json`).
- The service's repository: `docs/research/infrastructure-2026-10-13.md` (the plan),
  `docs/research/audit2-security-infra-2026-10-13.md` (audit2), `docs/spec.md` (the paid spec).
