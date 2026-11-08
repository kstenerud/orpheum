> **ABOUTME:** The lead's independent audit of how a server is installed and how its parts find
> each other (2026-11-08), which D448 adopted. Its comparisons with Syncthing, Tailscale, Immich,
> Unraid and TrueNAS, and its Synology Landlock claim, are from general knowledge, not checked.

# Orpheum deployment and discovery audit (2026-11-08)

An independent review of how Orpheum is installed and how its parts find each other, written against the goal: *as little technical skill as possible, without giving up security*, with OpenMediaVault (OMV) and ordinary home Docker hosts as the targets. Sources: `deploy/omv/compose.yaml`, `cmd/orpheum/{start,main,pair}.go`, `internal/deployment/` (values.go, templates), `internal/core/api/home_network.go`, `internal/core/{nodes,discovery,certificate,auth,portmap}`, `internal/core/rtcapi/own_network.go`, `discovery/single`, `docs/users/{install,setup-problems}.md`, decisions D127, D297, D298, D300, D314, D334, D447 and others, the closed-beta and public-release plans, orpheum-connect `docs/spec.md`, and orpheum-ops READMEs.

## 1. Run modes as they ship today

| Mode | What it is for | Who runs it | How it is deployed |
|---|---|---|---|
| `core` (via `start`) | API, web client, HTTPS at home, WebRTC, the discovery client | every household | the `orpheum-core` service in one compose file, or the install script |
| `node` | every file read and FFmpeg run, under Landlock and seccomp | every household, on the same box | the `orpheum-node` service: `network_mode: none`, read-only root, connected to the core only through a Unix socket in `run/` |
| discovery (paid) | sign-in by realm, names under `orpheum-core.com`, DNS-01 for the core's wildcard certificate, signalling, STUN | the owner (orpheum-connect on `prod-api`) | a `.run` package pushed over SSH (orpheum-ops README) |
| discovery (self-hosted) | one core, the owner's own VPS and domain | technical self-hosters | **not shipped.** `discovery/single` exists, but the command is only in dev builds (`dev_discovery.go`, `//go:build dev`). Deferred to the open beta (D334 §1, spec §1.4) |
| remote node | a node on a second machine | nobody yet | **not built.** The design is in D127 (the node dials out over HTTP and is mutually authenticated, per D107), and schema.md says a node credential comes "with remote nodes" |

**Compose route on OMV, step by step, and the skill each step needs:**
1. Install omv-extras, the compose plugin and Docker. *OMV GUI skill.*
2. Create the shared folders; read the **absolute path** (`/srv/dev-disk-by-uuid-…`) from a column that may be hidden. *Moderate.*
3. Check that the global env sets `PATH_TO_APPDATA`, the PUID/PGID and the time zone. *Moderate.*
4. Give the app user read permission on the media. *Moderate. A common failure.*
5. Paste the YAML and set `ORPHEUM_MEDIA` and **`ORPHEUM_LAN_ADDRESS`**. The docs also say to keep the address fixed "with a reservation in your router". *Knowing your LAN IP, plus router configuration.*
6. Up. Then find `setupCode=` in the container logs. *Reading logs.*
7. Open `http://<IP>:8440` and create the admin.
8. Create a library under `/media/...`. This is not the host path, which confuses people.
9. Optional extras: a GPU (uncomment YAML lines and look up `RENDER_GID` with `ls -n`); more media folders (edit `volumes` in the YAML); `ORPHEUM_ROUTER` (the router's IP).
10. Remote access: make an account at account.orpheum-app.com (beta, invitation only), choose a realm, create a key, paste it into System → Discovery while at home. Each existing person must then sign in once on the core directly before realm sign-in works (setup-problems.md).

**Install-script route.** You run `docker run … install-script | sh -s -- --media …` at a terminal. It detects the LAN IP (`ip route get 1.1.1.1`) and the router, asks for the admin on `/dev/tty`, then optionally for the key. It needs terminal skill but no knowledge of IPs. It also publishes `127.0.0.1:8441` ("this machine") and pins the image by digest. **The compose route publishes neither 8441 nor a digest pin.**

## 2. How components find each other

**Node ↔ core, same box.** This needs no configuration. On its first start the core writes the node's instance (D447 §3). The node dials `/run/orpheum/core/core.sock` and is **enrolled by name on its first hello**, with no credential (`internal/core/nodes/enrol.go`). Trust comes from the socket file's permissions. That is sound on one box.

**Node ↔ core, separate boxes.** This is impossible today.

**Core ↔ discovery.** The core enrols with the suggested issuer (`suggested.go`), verifies the discovery's signed root chain and address document, and waits *pending*. The admin then types a key that the account site generated (`Claim`, D310). After that the core keeps a session and signalling connection, sends candidates as heartbeats, and receives a wildcard `*.<coreId>.orpheum-core.com`. The core runs ACME itself; the discovery writes only the TXT record and the CAA record. The core's address is discovered automatically. The key is copied by hand.

**Browser at home.** Before pairing there is only `http://<LAN-IP>:8440`: plain HTTP, so no service worker, clipboard or microphone (D334 §1). After pairing there is `https://<label>.<coreId>.orpheum-core.com:8444`, a name that encodes the private IP. Routers with DNS-rebind protection (dnsmasq `stop-dns-rebind`, FRITZ!Box by default) drop that answer, and the page then falls back to WebRTC on the LAN port (D300 §4). TVs pair by QR or device code through the discovery (D321). "Casting" means sending to another signed-in Orpheum tab (D337). There is no Google Cast/Chromecast receiver.

**Browser away from home.** Sign-in by realm, name and password at app.orpheum-app.com, then WebRTC punched through STUN, or a mapped HTTPS port when `--router` is given and the router speaks PCP, NAT-PMP or UPnP. **There is no relay** (D314 §4). Failures are counted.

**What breaks:**
- **The DHCP lease changes.** `values.toml` is frozen after the first start (D447 §2). `home_networks` (the LAN address's /24), the WebRTC host candidate (D441), the LAN names the discovery publishes and the port-mapping target all go stale. Admin is then refused from every device, because the OMV compose has no 8441 escape hatch. The only fix is to delete `values.toml` and edit the env. **This is the worst latent failure in the design.** The documented mitigation needs router skills.
- **Subnets that are not /24.** `HomeNetworks()` hard-codes /24. Eero's /22 and 10.0.0.0/16 LANs leave some household devices "not home".
- **IPv6 at home.** Direct requests from a LAN IPv6 address (for example, when `nas.local` resolves to AAAA) are not in `home_networks`. Only WebRTC learns the house /64 (`ownIPv6Networks`).
- **CGNAT/DS-Lite.** Inbound IPv4 is impossible, so away access depends on IPv6 at both ends or on punchable NATs. The orpheum-connect spec itself estimates that phones on mobile data may fail well over 30% of the time without a relay.
- **IPv6-only or IPv6 trouble.** ghcr.io has no IPv6. `accept_ra=2` and Proxmox pitfalls need root sysctl edits.
- **Docker bridge networking.** With rootful Docker and iptables, IPv4 client addresses are preserved. Rootless Docker, the userland proxy, IPv6 without ip6tables, and Docker Desktop all show clients as the Docker gateway. The design **correctly refuses to treat the gateway as home** (the Plex-chain review's L1). The result is that admin is impossible in those setups, and it is impossible from the NAS's own browser on OMV. Multicast (SSDP, mDNS) does not cross the bridge, which is why the router's IP and the LAN IP must be supplied by hand.
- **Old or vendor kernels.** A release node *requires* Landlock and seccomp and will not start otherwise. NAS kernels older than 5.13 (Synology DSM, older QNAP) never have Landlock, and the fix means editing a TOML inside appdata. MPTCP sysctls fail on kernels without MPTCP, which means editing the YAML.

## 3. Security posture of each step

- **First admin.** A 60-bit one-time setup code, read from logs or `setup-code`. Not tied to location, so anyone who can read the logs can create the admin. Strong, and better than Jellyfin's "first visitor wins" wizard.
- **Admin location rule (D297 §6, D300, D439).** Admin rights apply only to direct requests from the LAN /24 or loopback, or to a direct (non-relayed, non-punched) WebRTC pair from home. Its real value: a password stolen from outside, or JavaScript served by the discovery (D292 §4, spec §1.5), cannot administer. Its costs: it depends on the hand-typed IP; it also trusts guest Wi-Fi or IoT devices in the same /24; and on ISP routers that SNAT forwarded traffic to their own LAN address, internet clients would look "home" under the current rule too.
- **Node trust.** Socket permissions plus a network-less, read-only, confined node. Excellent for one box.
- **Discovery pairing.** A single-use account key, typed at home, signed calls, the root chain verified, and the discovery identity confirmed for non-suggested issuers. The discovery cannot answer in the core's place for realm sign-ins (the PBKDF2 tag), but it serves the JavaScript and so is trusted. That is the plex.tv model, stated honestly in the spec.
- **TLS.** Real certificates via DNS-01 with the core's own ACME key, and CAA pinned to that account. No HTTPS at all before pairing.
- **Secrets.** Never in flags or env (D204, D334 §2). Passwords and keys go over stdin.
- **Supply chain.** The install script pins by digest and release records are verified. The compose route runs a tag, which is weaker.
- **Where ease and security pull apart:** the IP-based home rule against zero configuration; the setup code against a code-free wizard; no relay (privacy and cost) against reachability; Landlock required against Synology support; no env or flag secrets against Plex-style `PLEX_CLAIM`.

## 4. Friction ranked by how many non-technical users it stops

| Rank | Friction | Who it stops | Why |
|---|---|---|---|
| 1 | Away access fails behind CGNAT, symmetric NAT or IPv4-only mobile, with no relay | very many: a large share of phone sessions | Nothing the user can do; the error offers only workarounds |
| 2 | `ORPHEUM_LAN_ADDRESS` required, plus a DHCP reservation; silent admin lockout when the IP changes | most first-timers, and later everyone whose lease moves | Requires knowing what an IP is and using the router UI; breakage appears weeks later |
| 3 | Remote setup chain: an invite-only account, realm, key copy-paste, "each person signs in once at home" | most who want remote access | Four concepts and two websites |
| 4 | Media paths and permissions: absolute OMV paths, `/media` inside versus the host path, ACLs, extra folders need YAML edits | many | Errors show up as "not found" in the library UI |
| 5 | Setup code buried in container logs | many | Logs are unfamiliar; the code scrolls away |
| 6 | Node refuses to start without Landlock (Synology, older kernels); MPTCP and IPv6 network errors | a whole platform class | Needs kernel or YAML edits |
| 7 | Plain HTTP at home until paired; DNS-rebind protection on the LAN name | moderate | Features missing for no visible reason; slower WebRTC fallback |
| 8 | Port clashes on OMV; changing the HTTPS port requires deleting `values.toml` | moderate | A hidden rule |
| 9 | GPU: uncomment lines, look up `RENDER_GID` | those with GPUs | Software encode still works |
| 10 | Updates: edit the tag, or re-paste the file; configs never re-templated (D447's open point) | everyone, over time | Drift and a silent miss of new defaults |

## 5. Proposals

**P1. No required LAN address: learn it, with a defined trust rule.** *Change:* `--lan-address` becomes optional. On the setup request (which already carries the 60-bit code), the core records the `Host` header's private IP and the request's source network as `lan.address` and `home_networks`, and shows them for confirmation in the wizard. If a direct request later arrives on a different private Host IP, the System page shows "Your server's address changed from A to B. Confirm with your password and the code in the log." A fresh code from the log, as in `reset-admin`, guards the re-learn. Learn the home prefix from the client's source rather than assuming /24. Optionally the OMV docs can say "open http://<omv-hostname>.local:8440"; OMV already runs avahi. *User effect:* step 5's IP and the router reservation disappear, and DHCP changes become self-healing. *Security:* the household is defined by whoever holds the setup code, which is the same trust root as admin creation. Strictly better than a typo-prone hand-typed value. *Effort:* M. *Revises:* D447 §1–2 (the address is "needed"), D297 §6 (where home networks come from), values.go's /24 rule.

**P2. Optional host-network mode for the core.** *Change:* a second compose variant with `network_mode: host` for the core. The core then sees real interfaces, the gateway, real client v4 and v6 addresses, and mDNS/SSDP, and it serves 8444 directly with no 443 bind. *Effect:* no LAN IP and no router IP needed, and IPv6 home clients are recognised. This is what Jellyfin and Home Assistant recommend. *Security:* the core loses its private netns and the per-network MPTCP sysctl, so the CVE mitigation must move to the host or be documented. The core remains non-root with all capabilities dropped, and the node stays `network_mode: none`. *Effort:* M. *Revises:* D300/D441's container assumptions and the compose header's MPTCP rationale.

**P3. Claim through the account by approval, not a pasted key.** *Change:* after the first admin exists, the wizard's "Connect to your Orpheum account" opens account.orpheum-app.com with the core's pending claim, the way a TV device code works (RFC 8628). The user signs in and presses Approve, then sees the same short code the core displays. Realm keys are derived during the same sign-in. Make wizard order "admin → connect → invite people" so the "sign in once at home" problem rarely arises. *Effect:* one click instead of copying a key; remote access in the first five minutes. *Security:* equivalent to the key. The admin at home starts it and the account owner approves it. A matching displayed code defeats a LAN neighbour approving their own claim. No admin goes through the discovery (D292 §4 holds). *Effort:* M (both repositories). *Revises:* D310 §5, D298 §5, D334 §3.

**P4. A relay for the failures nothing else carries.** *Change:* TURN on the paid service, used only after direct paths fail. It carries DTLS whose fingerprint the core signs, under per-session credentials, as the spec's A7 already sketches. Bandwidth-capped and part of the subscription. Also document a "bring your own Tailscale/VPS" path. *Effect:* friction #1 largely disappears. *Security:* the relay sees only ciphertext and metadata (traffic volume, IPs). *Effort:* L, plus running costs. *Revises:* D314 §4, D292 §5, D438, D72/D214 ("no traffic relayed by us"). The counts already collected exist to make exactly this decision. Make it.

**P5. Settings reconciled on every start, not frozen.** *Change:* `start` re-applies flags and image templates at every start (as `write-deployment` does for the script route). Drift between the flags and `values.toml` is reported on the System page rather than silently ignored. *Effect:* changing a port or a path in OMV's env "just works"; updates bring new config defaults. *Security:* neutral. *Effort:* S–M. *Revises:* D447 §2, and resolves D447's open question.

**P6. Media without YAML editing.** *Change:* add optional `ORPHEUM_MEDIA_2..4` variables, each defaulting to an empty bind, and have the library picker show host-side names next to `/media/...`. *Effect:* most users never edit the YAML. *Security:* neutral (still read-only). *Effort:* S.

**P7. Kernel gaps handled in the UI, not by editing files.** *Change:* when Landlock or seccomp is missing, the node starts in a "locked" state: it refuses media work. The System page explains the risk and lets the admin choose "run without confinement" with an explicit, logged acknowledgement. Detect a missing MPTCP and say which lines to delete, or ship a variant without MPTCP. *Effect:* Synology and older NASes become installable. *Security:* the risk is still accepted explicitly, but in a UI rather than a TOML. Slightly weaker by default-friction, the same in principle. *Effort:* S. *Revises:* the 2026-10-20 "require" change (D273 §5's revision).

**P8. Setup code where people look.** Keep the code. Also print it in the container's healthcheck/status line and write it to `appdata/orpheum/SETUP-CODE.txt`, which is visible in OMV's file browser or an SMB share. Have the first-run page say exactly where to find it on OMV, Portainer and the CLI. *Effort:* S. *Revises:* none (extends D447 §5).

**P9. Catalogue entries.** Contribute the compose file to the OMV compose plugin's built-in examples ("Add from example"), plus a Portainer template, a TrueNAS community-train app (its question form can ask for the media path), and CasaOS/Umbrel. Unraid's Community Apps model is one container per template, which fits the core-plus-node pair badly; a compose-manager template or a combined image is needed there. *Effect:* no copy-paste from GitHub. *Effort:* M each. *Revises:* none. Depends on P1, since catalogues cannot ask "what is your IP" well.

**P10. Remote nodes joined by a short code.** When remote nodes are built (D127), the core's UI should get "Add a node", which shows a compose snippet carrying a **join token**: core address, core certificate fingerprint and a single-use secret (Tailscale auth keys and `kubeadm join` work this way). Optionally find the core on the LAN by mDNS and then confirm a 6-digit code shown on both. The node generates its own key, dials out (D127) and is then pinned. *Security:* mutual authentication with no trust on first use, because the fingerprint is in the token. A spoofed core can never push commands (D107). *Effort:* L. *Revises:* schema.md's node grant, D107/D127's open questions.

**P11. Updates.** Use a channel tag in the OMV file (`:0.3` or `:stable`). Have the core, which already verifies release records, show "Version X is available: in OMV press Pull, then Up" with the release notes. Keep digest pinning for the script route. *Security:* a moving tag is weaker than a digest, but the release-record check on the core can warn when a running image's digest is not a published release. *Effort:* S. *Revises:* none (refines D447 §1's tag default).

**P12. The "this machine" port in the OMV compose,** or drop the need for it via P1 or P2. *Effort:* S.

**P13. Self-hosted discovery: keep it a technical path.** It inherently needs a VPS, a domain and DNS. Ship it as `orpheum discovery` in the release image with one compose file and HTTP-01/ALPN ACME on the owner's domain, but spend the "non-technical" effort on the paid path. *Effort:* M. *Revises:* D334 §1's deferral.

Not recommended: merging core and node into one container. The node's no-network, read-only and confined container is the best security property in the design. A root entrypoint that chowns and drops privileges (the linuxserver style) could remove `orpheum-folders`, but it adds root at start for a small gain.

## 6. What to keep

- **One image, subcommands as roles,** and the node split into a network-less, read-only, Landlock/seccomp container.
- **Unix-socket node trust on one box,** and D127's dial-out design for remote nodes.
- **The 60-bit setup code** and secrets read only from stdin or the TTY, never from flags or env.
- **Refusing to treat the Docker gateway as home.** This lesson was paid for; P1 must not undo it.
- **No admin through the discovery** (D292 §4). It caps the damage from the hosted-JavaScript trust problem.
- **The core's own ACME key, with CAA and DNS-01 through the discovery.** The discovery never holds the private key, and plex.direct-style names avoid bare-IP certificates.
- **Realm sign-in with PBKDF2 proofs in both directions.** There is no realm directory to harvest.
- **Measuring before building:** relay-needed failures counted per network kind.
- **`setup-problems.md` grown from real installs,** values written last so an interrupted start retries, healthchecks with `depends_on`, and digest pinning on the script route.

**Overall view:** the security architecture is careful and mostly right. The ease gap comes from one bad assumption, that *the user types the LAN address*, and from one deferred decision, *no relay*. Fix the first with P1 (or P2) and P5, and reduce the second with P3 and P4, and the OMV install becomes: paste the file, set the media folder, press Up, open the page, enter the code, click Connect.
