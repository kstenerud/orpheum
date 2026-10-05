# Reaching the server from outside: a VPS and a WireGuard tunnel

The house's internet is DS-Lite: there is no public IPv4 address to forward a port from, and the
IPv6 prefix changes. So the server's public name points at a small VPS, and the core's machine
dials out to the VPS over WireGuard, which works on DS-Lite and carries UDP. The VPS forwards
HTTPS, TURN and the relay's ports into the tunnel without rewriting the client's address. Replies
go back the way they came, so the core sees who each client is (D243 §7).

```
browser ──► media.orpheum-app.com (the VPS) ══ WireGuard ══► the core's machine (10.77.0.2)
             TCP 443, TCP+UDP 3478,                          the core: HTTPS on :443,
             UDP 49160–49199                                 TURN on 10.77.0.2:3478
```

The files here:

| File | Goes to | What it does |
| --- | --- | --- |
| `vps-wg0.conf` | the VPS's `/etc/wireguard/wg0.conf` | the tunnel's far end |
| `orpheum.nft` | the VPS's `/etc/wireguard/orpheum.nft` | the port forwards into the tunnel |
| `core-wg0.conf` | the core machine's `/etc/wireguard/wg0.conf` | dials the VPS; answers go back through it |
| `core-config.toml` | the core's full config | `[https]` and `[turn]` for the deployment |

## 1. The VPS

Hetzner Cloud's smallest plan, in Nuremberg or Falkenstein, with Debian or Ubuntu. Then:

1. In Hetzner's firewall for it, let in UDP 51820 (WireGuard), TCP 443, TCP and UDP 3478, and
   UDP 49160–49199. Leave SSH as it is.
2. Install WireGuard and nftables: `apt install wireguard nftables`.
3. Make its key pair: `wg genkey | tee /etc/wireguard/private | wg pubkey > /etc/wireguard/public`.
4. Copy `vps-wg0.conf` to `/etc/wireguard/wg0.conf` and `orpheum.nft` to
   `/etc/wireguard/orpheum.nft`. In them, fill in the keys, the VPS's public IPv4 address and its
   public interface's name (`ip -br addr` shows it; Hetzner's is `eth0`).
5. `systemctl enable --now wg-quick@wg0`.

If the VPS runs `ufw`, it filters forwarded packets too: allow forwarding between `eth0` and `wg0`
for those ports, or the forwards will be dropped silently.

## 2. The name

At Cloudflare, give `media.orpheum-app.com` an A record with the VPS's IPv4 address and an AAAA
record with its IPv6 address, both **DNS only** (grey cloud): Cloudflare's proxy would end TLS
itself and carries no TURN. The certificate comes by DNS-01 through the DNS provider's token,
which the admin enters as the `dns_provider_token` secret (D243 §6). Make it from Cloudflare's
**Edit zone DNS** template, limited to `orpheum-app.com`.

## 3. The core's machine

1. Install WireGuard, make its key pair as on the VPS, and copy `core-wg0.conf` to
   `/etc/wireguard/wg0.conf` with both keys and the VPS's IPv6 address filled in.
2. `systemctl enable --now wg-quick@wg0`. `wg show` should show a handshake within a few seconds.
3. Run the core on the host's network: as a plain process, or in a container with
   `--network host`. Its answers have to leave from the tunnel's address, and a container's
   bridge network would send them out the house's own connection instead, where the browser
   would drop them. Port 443 needs root or `CAP_NET_BIND_SERVICE`.
4. Put `core-config.toml`'s two tables in the core's full config, with your contact address and
   the VPS's IPv4 address, and restart the core. Its log says `relaying voice` with the relay's
   address.
5. If the machine has a firewall, let in TCP 443, TCP and UDP 3478, and UDP 49160–49199 on `wg0`.

`Table = off` keeps the tunnel from becoming the machine's default route: the house still
reaches the internet directly. Only what leaves from the tunnel's addresses, the answers to what
came in through it, is routed back through it. That is also why TURN listens on `10.77.0.2`
rather than on every address. A UDP answer leaves from the address the socket is bound to, and a
socket bound to every address would answer from the house's own.

## 4. Checking it

From a phone on mobile data, not the house's Wi-Fi, open `https://media.orpheum-app.com`. It
should load with a valid certificate, and `wg show` on the core's machine should show the traffic
growing. Voice through the relay is checked by a call with the relay forced (VQ19).
