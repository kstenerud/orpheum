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

## 3. When a relay is genuinely the only option

Researched **2026-09-19**, because "about 10% need a relay" is a number to design around and the
residual deserves naming.

### The taxonomy that matters is two bits, not four cones

The old STUN "full cone / restricted / port-restricted / symmetric" vocabulary is obsolete.
RFC 4787's model separates **mapping** behaviour from **filtering** behaviour, and for traversal
purposes only the mapping bit decides anything:

| | Behaviour | Traversable? |
| --- | --- | --- |
| **Endpoint-Independent Mapping (EIM)** — "easy NAT" | one public `ip:port` per internal `ip:port`, whatever the destination | **Yes.** STUN tells you the mapping and the peer can aim at it |
| **Endpoint-Dependent Mapping (EDM)** — "hard NAT", *symmetric* | a different public port per destination | Not directly — the peer cannot know which port will be allocated *to them* |

Tailscale's framing is the useful one: *"the major distinction we care about is Symmetric versus
anything else."*

### One hard NAT is a solved problem; two is not

This is the crux, and it is quantified. Against a hard NAT you can **guess the port**, because
you only need any one of your many open source ports to collide with any one of your probes —
the birthday paradox working for you. Probing random destination ports with 256 open source
ports:

| Probes | Success |
| --- | --- |
| 174 | 50% |
| 256 | 64% |
| 1,024 | 98% |
| 2,048 | 99.9% |

At 100 packets per second, *"half the time we'll get through in under 2 seconds."* So **a single
hard NAT costs a couple of seconds and is otherwise invisible.**

**Two hard NATs collapse this.** Now a collision must be found in both directions at once, which
takes roughly **170,000 probes per side — about 28 minutes** for the same 99.9% confidence. That
is not a connection attempt, it is a weather system. This is the first of exactly two cases
where a relay is not a fallback but the answer.

### The second case: UDP does not leave the building

Hole punching of any kind needs UDP to traverse. Corporate networks, hotels, conference venues
and captive portals routinely permit only TCP/443 outbound, sometimes only through an HTTP
proxy. No amount of cleverness produces a direct path through that, and the only remedy is a
relay reachable on **TCP/443 with TLS**, because that port is the one nobody blocks.

### So the list is short

**Relay is the only option when:**

1. **both** endpoints are behind hard/symmetric NAT, or
2. UDP is blocked outright at either end.

Everything else — one hard NAT, restrictive filtering, CGNAT with endpoint-independent mapping,
double NAT — is traversable with enough effort.

### Why published relay rates vary by 8×, and what it tells us to build

| Source | Relayed |
| --- | --- |
| Tailscale | **<10%** |
| WebRTC, general production | **20–25%** |
| WebRTC, corporate networks with managed firewalls | **60–85%** |

The population differs — corporate networks are the bad case and Tailscale's users are mostly
home and mobile — but the larger part of the gap is **effort**, and the difference is
instructive:

- **Tailscale tries harder.** Port prediction, UPnP-IGD, NAT-PMP and PCP for explicit mappings,
  IPv6, and continuous background retrying.
- **And crucially, it has no deadline.** A WebRTC call must connect in a second or two, so ICE
  gives up and relays. Tailscale **starts relayed and upgrades to direct in the background**, so
  it can spend thirty seconds hunting a path nobody is waiting on.

That last point is the design, not a detail: **relay-first, upgrade-later removes the time
pressure that forces most WebRTC relaying.** The user gets an instant connection either way and
never learns which one they got.

### IPv6 is quietly winning, and it deletes the problem

On **2026-03-28 IPv6 crossed 50% of Google's traffic** for the first time; APNIC, which measures
differently and reads lower, puts IPv6-capable networks at **43.13%**. France, Germany and India
are majority-IPv6; the US, Brazil and Japan are near half; China is under 5%.

With IPv6 at both ends there is no NAT, no mapping to predict and no STUN — just a stateful
firewall, which simultaneous transmission opens. Every year this problem gets smaller on its
own, which argues for **not over-investing in traversal cleverness** and for making sure IPv6 is
tried first rather than as an afterthought.

### And the port-mapping protocols are free when they work

UPnP-IGD, NAT-PMP and PCP (RFC 6887) let a host simply *ask* the router for a public port. When
available it makes the whole problem vanish. You cannot rely on them — frequently disabled by
policy or absent — but trying costs a few packets, so they belong in the ladder.

One related trap worth knowing: **many otherwise well-behaved NATs do not support hairpinning**,
so two peers behind the *same* NAT cannot reach each other via the public address. That is a
LAN-adjacent case and the fix is to try local addresses first, which ICE already does.

### The finding that changes our architecture

All of the above is the general peer-to-peer problem. **Ours is not the general problem**, and
the difference is worth a great deal.

For Watch Together, the two endpoints are two *people*, but there is a third party both are
**already connected to by construction**: the media server. If she cannot reach the server there
is no film to watch together, so by the time voice matters, a mutually reachable rendezvous
already exists.

So **the media server relays the voice**, and:

- **It costs almost nothing.** Opus speech is roughly 24–64 kbps per stream, against a film
  already flowing over the same uplink.
- **It costs almost no latency**, which is the surprising part. A conventional TURN sits in a
  datacentre somewhere between the peers and adds a real detour. Our relay sits **at one of the
  endpoints** — the server is on the home participant's LAN — so `her → server → him` is
  essentially `her → him`.
- **It needs no new infrastructure or service.**

The conclusion is that **TURN is not an independent requirement for us**; voice relaying is
subsumed by whatever already makes the server reachable. See
[D68](../../decisions/working-notes.md), which corrects
[D64](../../decisions/working-notes.md) on exactly this point.

## 4. The collision is bigger than remote access — it starts with HTTPS

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

## 5. What a relay would actually cost

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

## 6. Certificates and identity are the same problem as sharing

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

## 7. The promise has to be restated, not quietly weakened

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

## 8. What is still open

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
