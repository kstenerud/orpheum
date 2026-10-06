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

The core's end of the tunnel is not here: it is a service of the deployment itself, which
`./staging` runs (`deploy/staging/README.md`, D249). The files here are the VPS's:

| File | Goes to | What it does |
| --- | --- | --- |
| `vps-wg0.conf` | the VPS's `/etc/wireguard/wg0.conf` | the tunnel's far end |
| `orpheum.nft` | the VPS's `/etc/wireguard/orpheum.nft` | the port forwards into the tunnel |
| `orpheum-forwards.service` | the VPS's `/etc/systemd/system/` | loads the port forwards at boot |
| `99-orpheum-forwarding.conf` | the VPS's `/etc/sysctl.d/` | lets the VPS forward packets |

## 1. The VPS

Hetzner Cloud's smallest plan, in Nuremberg or Falkenstein, with Debian or Ubuntu. Then:

1. In Hetzner's firewall for it, add an inbound rule for each, from any IPv4 and IPv6 address:

   | Protocol | Port | For |
   | --- | --- | --- |
   | UDP | 51820 | the WireGuard tunnel |
   | **TCP** | 443 | HTTPS |
   | TCP | 3478 | TURN over TCP |
   | UDP | 3478 | TURN over UDP |
   | UDP | 49160-49199 | the voice relay |

   Leave SSH as it is, and check that the firewall is attached to the server. A port opened for the
   wrong protocol fails silently: with 443 opened for UDP, browsers time out, and with 51820 left
   out, `wg show` on the core's end shows packets sent and none received (the owner's VPS,
   2026-10-06). `tcpdump -ni eth0 port 443` on the VPS shows whether a request gets that far.
2. Install WireGuard and nftables: `apt install wireguard nftables`.
3. Make its key pair: `wg genkey | tee /etc/wireguard/private | wg pubkey > /etc/wireguard/public`.
4. Copy `vps-wg0.conf` to `/etc/wireguard/wg0.conf` and `orpheum.nft` to
   `/etc/wireguard/orpheum.nft`. In them, fill in the keys, the VPS's public IPv4 address and its
   public interface's name (`ip -br addr` shows it; Hetzner's is `eth0`).
5. Copy `99-orpheum-forwarding.conf` to `/etc/sysctl.d/` and apply it: `sysctl --system`.
6. Copy `orpheum-forwards.service` to `/etc/systemd/system/` and start it:
   `systemctl daemon-reload && systemctl enable --now orpheum-forwards`.
   `nft list table inet orpheum` then shows the forwards.
7. `systemctl enable --now wg-quick@wg0`.

The forwarding and the port forwards are set up apart from the tunnel, not in `wg0.conf`'s
`PostUp`: Ubuntu confines `wg-quick` with AppArmor, which refuses it, even as root, both the sysctl
and reading the nftables file (`permission denied`).

**If the VPS runs `ufw`** (`ufw status` says `Status: active`; on Hetzner's images it is installed
but inactive, and then there is nothing to do): it drops forwarded packets unless told otherwise, so
the forwards would be dropped silently. Let them through, and the tunnel in:

```sh
ufw allow 51820/udp
ufw route allow in on eth0 out on wg0 to 10.77.0.2 port 443,3478 proto tcp
ufw route allow in on eth0 out on wg0 to 10.77.0.2 port 3478,49160:49199 proto udp
ufw route allow in on eth0 out on wg0 to fd77::2 port 443 proto tcp
ufw route allow in on wg0 out on eth0
ufw route allow in on wg0 out on wg0 to 10.77.0.2 port 49160:49199 proto udp
```

The last line is the relay's packet sent from one browser's allocation to another's, which goes
back into the tunnel. The answers to what came in need no rule: `ufw` lets an established
connection's packets through.

## 2. The name

At Cloudflare, give `media.orpheum-app.com` an A record with the VPS's IPv4 address and an AAAA
record with its IPv6 address, both **DNS only** (grey cloud): Cloudflare's proxy would end TLS
itself and carries no TURN.

**The certificate.** Browsers trust HTTPS only with a certificate, which the server gets from
Let's Encrypt by itself (D243 §6). Let's Encrypt first checks that whoever asks controls the name,
and the way it checks here is called DNS-01: the server briefly adds a TXT record to the name's DNS
at Cloudflare, Let's Encrypt looks it up, and the server removes it. It does so when it first starts
with a name, and again before each renewal, about every 60 days.

To change DNS at Cloudflare, the server needs an **API token**: a password for programs, limited
to what it allows. Make one that may edit only this domain's DNS records:

1. At Cloudflare, top right: your profile icon, **My Profile**, then **API Tokens** on the left.
2. **Create Token**, then **Use template** beside **Edit zone DNS**.
3. Under **Zone Resources**, keep *Include · Specific zone* and choose **orpheum-app.com**. Leave the
   rest as it is.
4. **Continue to summary**, then **Create Token**.
5. **Copy the token now**: Cloudflare shows it only once. (Lost, it is deleted and another made.)

Once the server runs (§3), sign in as its admin, open **Settings**, and under **Keys and
passwords** paste it as `dns_provider_token`. The server keeps it encrypted and shows only its last
few characters; the certificate follows within a minute or two.

## 3. The core's machine

Run staging there: `deploy/staging/README.md` says how, from a clean machine. Its tunnel service
dials the VPS and holds the core's end of the tunnel, `10.77.0.2` and `fd77::2`, inside the
deployment's own network: nothing about WireGuard is installed or configured on the machine itself.
Its first run prints the public key that goes in the VPS's `wg0.conf` as the core machine's.

The tunnel is not that network's default route: the core's own requests, and its answers to the
house's network, leave the usual way. Only what leaves from the tunnel's addresses, the answers to
what came in through it, is routed back through it. That is also why TURN listens on `10.77.0.2`
rather than on every address. A UDP answer leaves from the address the socket is bound to, and a
socket bound to every address would answer from the house's own.

## 4. Checking it

From a phone on mobile data, not the house's Wi-Fi, open `https://media.orpheum-app.com`. It
should load with a valid certificate, and `wg show` on the VPS should show the traffic growing.
Voice through the relay is checked by a call with the relay forced (VQ19).
