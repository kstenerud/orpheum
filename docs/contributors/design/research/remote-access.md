> **ABOUTME:** How a server in someone's house becomes reachable by their family elsewhere —
> the topology, the mechanisms, what each costs, and why the "no accounts" promise collides with
> this harder and earlier than expected. Covers the identity half of sharing, since it is the
> same question.

# Remote Access and Identity

Researched **2026-09-18**. This is the hardest open product question in the project
([D14](../../decisions/working-notes.md)) and the one most likely to change what we promise.

## 1. The topology fact, and it is not negotiable

**Two machines behind different NATs cannot find each other without a third party.** Not "usually
cannot" — cannot, by construction. Neither knows its own public address:port as seen from
outside, and neither can be contacted first. Something mutually reachable has to introduce them.

Tailscale's own description of its data path makes this concrete: **every connection begins via
a DERP relay**, exchanging observed addresses and keys, and only *then* attempts a direct UDP
path in parallel. The direct path is an optimisation over a rendezvous, never a replacement for
one.

> This confirms the instinct that prompted this research. There is no discoverability without a
> signpost. The only question left is **whose** signpost, and what it costs.

Tailscale reports direct connections succeeding **north of 90%** in typical conditions. The
residual is the hard cases: symmetric NAT (routine on mobile carriers), corporate firewalls
blocking outbound UDP, and double-NAT home setups where the ISP router and the user's router
compound.

## 2. The mechanisms, and what each actually delivers

| Mechanism | Needs a third party? | Works behind CGNAT? | Non-technical user can do it? |
| --- | --- | --- | --- |
| Manual port-forward + dynamic DNS | DNS provider | **No** | No — router config, and it is the advice that gets people owned |
| UPnP-IGD / NAT-PMP / PCP | No | **No** | Yes when the router cooperates — often disabled by default, and with a poor security history |
| IPv6 direct | No (DNS still) | N/A — no NAT | Partly; firewall rules still needed |
| STUN + hole punching | **Yes** (STUN + signalling) | Sometimes | Yes, invisibly |
| TURN / relay | **Yes**, and it carries the bytes | **Yes** | Yes, invisibly |
| WireGuard mesh (Tailscale/Headscale/NetBird) | **Yes** (coordination server) | Yes, via relay | **No** — the *remote* person must install and enrol a client |
| Tunnel service (Cloudflare Tunnel etc.) | **Yes**, entirely | Yes | Yes, but a company sits in the path |

Two conclusions fall out:

- **Everything in the "non-technical user" column that says yes requires a third party.** The
  only mechanisms that avoid one are the two that demand router competence.
- **The self-hosted field's standard answer — "use a VPN" — is in the one row that is explicitly
  unusable here**, because it requires the *recipient* to install and enrol software. This is
  exactly the complaint recorded in [sentiment.md](sentiment.md): "Tailscale is great for me,
  but not feasible for family."

**CGNAT** makes the port-forward rows unavailable for a growing share of users, and there is no
reliable public figure for its prevalence — directionally increasing, universal on mobile
carriers, and common on newer fixed-line ISPs. IPv6 would dissolve the problem and carries
roughly 45–50% of global traffic, which is precisely the wrong number: too much to ignore, far
too little to depend on.

## 3. The collision is bigger than remote access — it starts with HTTPS

The finding that reframes this. **A home server cannot obtain a publicly-trusted TLS certificate
for itself**, because it has no domain name and ACME will not issue for a bare IP in any way a
normal user can drive. So it is self-signed certificates — which browsers block, and which many
TV clients reject outright with no override — or a third party.

**Plex's solution is instructive and is worth copying wholesale if we build anything here.** Plex
operates the domain `plex.direct`, resolves `1-2-3-4.<hash>.plex.direct` to `1.2.3.4`, and issues
each server a wildcard certificate for `*.<hash>.plex.direct`. A client then connects to a name
that matches a real certificate held only by that server — over TLS, with no warnings, **on the
LAN**. The hash keeps one user's name from resolving to another's server.

The consequence for us is sharp: **a user who never leaves their own LAN and never shares with
anyone still needs a third party to get working HTTPS.** The "no accounts" promise in
[D13](../../decisions/working-notes.md) was written thinking about remote access. It collides
with the certificate problem first.

(Note also Plex's own caveat, which becomes ours: names under such a scheme resolve to private
IPs, which is what DNS-rebinding protection exists to block, so some users have to disable it.)

## 4. What a relay would actually cost

The premium-service idea is only worth discussing if the economics work. They do, by a wide
margin, and the reason is Tailscale's 90%.

- **Signalling is nearly free.** A rendezvous server exchanges a few kilobytes per connection
  attempt. One small VPS serves a very large number of households.
- **Relay is the expensive part**, because it carries video. A two-hour film at 20 Mbps is about
  18 GB.
- **At commodity European prices that is still cheap.** Hetzner bills roughly €1 per TB beyond a
  large included allowance — call it **~2 euro-cents per relayed film**. Hyperscaler egress is
  30–75× that, and indie-PaaS egress ($150–550/TB) is disqualifying. The provider choice matters
  more than the architecture.
- **The 90% figure is what makes it viable.** If nine in ten sessions go direct after
  introduction, relay is an exception path, not the service.

A rough shape: one commodity box with a 20 TB allowance covers on the order of a thousand
relayed film-hours a month. Households needing relay at all are the minority, and those that do
will not each stream continuously. **A modest subscription covers this comfortably** — the
binding constraint is abuse, not cost.

## 5. Certificates and identity are the same problem as sharing

[`sharing-model.md`](../plans/sharing-model.md) asks how to give one person access to part of a
library. Its hard part is **naming the other person** — and that is the same third party again.

This is why Plex has an account system. Not because Plex wanted a login; because *sharing with
your mother requires a stable identifier for your mother that both servers agree on*, and that
identifier has to live somewhere. Every open competitor that skipped the account also skipped
sharing: Immich's sharing issue has 1,020 reactions under a feature freeze, PhotoPrism's has 844
and is seven years old, and Jellyfin users symlink subsets into fake libraries.

Options for identity without our own account system, in increasing order of what they give up:

1. **Server-local accounts only** (Jellyfin's model). The owner creates an account for each
   guest and communicates the credentials. Works; is a poor experience; and gives no identity
   across two servers, so "my friend shares back with me" cannot work.
2. **Capability URLs** — an unguessable link that grants scoped access, no identity at all.
   Excellent for "here is this one film"; bad for ongoing access because it cannot be attributed,
   audited or revoked per person, only per link.
3. **Bring your own IdP (OIDC)** — the owner points the server at Google, GitHub, an Authelia
   instance. Identity exists and we operate nothing. Cost: configuration the target user cannot
   do, and it is *still* a third party, just one the user chose.
4. **Our optional directory** — a first-party account that exists only to be an identifier and a
   rendezvous. Maximum convenience, and precisely the thing D13 promised not to require.

**These are not exclusive, and that is the design.** 1 and 2 are always available and need
nothing from us. 3 is for the technical. 4 is opt-in and is the premium tier.

## 6. The promise has to be restated, not quietly weakened

D13 currently says "no third-party account is ever required". The research says that is
keepable **only if we accept that the non-technical-family case is out of reach without one**.

The honest restatement — and the one the project's owner proposed independently — is a promise
about *why* we would ever charge, rather than a promise that we never will:

> **We charge only for things that genuinely require another machine.** Everything that can run
> on your own hardware does, for free, forever. No feature that works locally will ever be moved
> behind a payment. No telemetry, no ads, no storefront, no upsell in the UI.

That is defensible in a way "no accounts" is not, because it is falsifiable and it names the
exact category — rendezvous, relay, certificates, cross-server identity — where a third party is
a law of physics rather than a business decision. It is also, precisely, the inversion of the
grievance against Plex: Plex took a feature that worked and paywalled it. This says we never
will, and states what was never free to begin with.

**The tiers this implies**, which is the "multiple qualities of service" framing:

| Tier | What it is | Who runs it |
| --- | --- | --- |
| **Local** | LAN access, self-signed or user-supplied certificates, server-local accounts, capability links | The user. Free, forever, no account. |
| **Self-hosted assist** | The user runs their own rendezvous/relay/IdP — our software, their machine, documented | The user. Free. **This must be real, not theatre.** |
| **Hosted assist** | We run rendezvous, relay, `*.<hash>.ourdomain` certificates and the identity directory | Us. Subscription. |

The middle tier is the load-bearing one ethically: if a user *can* self-host the signpost, then
the paid tier is a convenience rather than a gate, and the promise is true. It also costs us
little, because the hosted tier is the same software.

## 7. What is still open

- **Is the hosted tier something this project wants to operate at all?** It implies a company,
  liability, uptime obligations, abuse handling, and an entity that can be pressured. A project
  can decline this and simply ship tiers 1 and 2 — the honest cost being that the
  non-technical-family case stays unsolved, which is the single largest reason people stay on
  Plex.
- **Abuse.** A relay that forwards arbitrary bytes between arbitrary parties is attractive to
  people who are not streaming home videos. Rate limits, per-account caps and content-blind
  abuse handling need designing before anything is operated.
- **What happens when we go away.** A hosted rendezvous is a dependency, and the whole audience
  for this project was burned by exactly that. Anything hosted needs a documented, tested exit
  to the self-hosted tier, and the software must degrade to tier 1 rather than break.
- **Which NAT-traversal stack.** `pion/ice` is the mature Go ICE implementation; libp2p offers a
  larger toolkit including relay and hole punching; WireGuard plus our own coordination is the
  Tailscale/Headscale/NetBird shape. Not chosen here.
- **Prevalence numbers.** No reliable public CGNAT figure was found. If the tiering decision
  turns on it, it needs primary measurement rather than another search.

**On measurability, which is better than expected:** the hard topology is already available
without simulating anything. **A handset on mobile data is a real CGNAT client** — carriers
deploy carrier-grade NAT universally and typically symmetric NAT with it, which is precisely the
case where hole-punching fails and a relay is required. Paired with VMs behind simulated
double-NAT and IPv6-only on the Proxmox hosts, that is a complete test rig for both ends of
every row in §2 ([test-hardware.md](test-hardware.md)).
