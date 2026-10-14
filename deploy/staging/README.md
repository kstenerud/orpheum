# Staging: the deployment, from this checkout, beside the dev loop

Staging is Orpheum as it is deployed (D249, D272): the runtime image and its compose file's
three services, written by the image's own `orpheum write-deployment` as release is, from an image
built from this checkout instead of a tag. It runs beside `./dev`
on the same machine, with its own data in `.staging/` and its own port, and it is what the VPS
(`deploy/vps/README.md`) forwards the public name to.

```
the house's network ── http://devnas.example:8440 ──┐
                                                ▼
browser ─► media.orpheum-app.com ─► VPS ═ WireGuard ═► orpheum-tunnel ── orpheum-core ── orpheum-node
                                         (dialled from   10.77.0.2,        HTTPS :443,     media read-only,
                                          the house)     fd77::2           TURN, HTTP      FFmpeg confined
```

- **orpheum-tunnel** holds the core's end of the tunnel to the VPS. The core shares its network, so
  the tunnel is there and nowhere else: nothing about WireGuard is installed or configured on the
  machine. It needs `CAP_NET_ADMIN` for that network only.
- **orpheum-core** is the server: HTTPS for the public name, the voice relay, and plain HTTP on
  port 8440 for the house's network.
- **orpheum-node** reads the media, read-only, and runs FFmpeg, confined. It has no network: it
  reaches the core by the sockets in `.staging/run/`.

The templates are in the binary (`internal/deployment/templates/`): each `./staging` writes the
compose file, the configs and the tunnel's config into `.staging/` from your values,
`.staging/values.toml`. `tunnel.sh`, here, is the tunnel service's.

## What the machine needs

- **Docker** with the compose plugin: `docker compose version` should answer. Your user in the
  `docker` group, so `./staging` runs without `sudo`.
- **WireGuard in the kernel**, which Debian's kernel has. `sudo modprobe wireguard` loads it and
  says nothing when it is there.
- **BBR in the kernel**, which sends the films so they keep their rate on a lossy link (D263 §3).
  `sudo modprobe tcp_bbr` loads it now; `echo tcp_bbr | sudo tee /etc/modules-load.d/bbr.conf`
  loads it at every boot. Without it the core says so once in its log and sends with the kernel's
  default.
- **The VPS set up** as `deploy/vps/README.md` §1 and §2 say, with its IPv6 address.
- **Port 8440** free on the machine (or another, see `[http]` in the values file). Staging never
  uses the dev loop's 8432 or 5173, so both run at once.

## The first run

1. `./staging`. It makes `.staging/`, builds the image (the first build fetches FFmpeg, Go and
   Node, and takes several minutes; later ones take about a minute), makes the tunnel's key pair,
   and stops, printing the tunnel's **public key**.
2. On the VPS, put that key in `/etc/wireguard/wg0.conf`, `[Peer]`'s `PublicKey`, and restart the
   tunnel there: `systemctl restart wg-quick@wg0`.
3. Fill in `.staging/values.toml`: the contact address; the VPS's endpoint, its address and
   WireGuard port, IPv4 (`"203.0.113.7:51820"`) where the machine has only IPv4 or reaches the VPS
   through DS-Lite's shared IPv4, or IPv6 in brackets (`"[2a0f:4000:…::1]:51820"`); its public IPv4
   address, for the voice relay; its WireGuard public key (`cat /etc/wireguard/public` on the
   VPS); and the media directories, for example `roots = ["/mnt/tank/media/video/movies"]`. The
   tunnel's MTU follows the endpoint: 1380 over IPv4, which leaves room for DS-Lite, and 1420 over
   IPv6; `mtu` in the file overrides it. Beside another deployment on the same VPS, such as
   nas.example's, give staging its own `tunnel_ipv4` and `tunnel_ipv6` (`deploy/release/README.md` §6).
4. `./staging` again. It renders the configs, starts the three services and waits until they are
   healthy, then prints the address on the house's network.
5. `./staging setup-code` prints the one-time code. `http://devnas.example:8440` opens on **Set up this
   server**, which takes the code and the first admin's name and password. Then add the libraries.
6. In **Settings**, under **Keys and passwords**, enter the Cloudflare token as
   `dns_provider_token` (`deploy/vps/README.md` §2 says how to make it). The certificate follows,
   and `https://media.orpheum-app.com` works from outside.

On the VPS, `wg show` shows a handshake with the house once staging is up.

## Every day

| Command | What it does |
| --- | --- |
| `./staging` | build the image from the checkout and start or restart staging |
| `./staging logs` | follow the three services' logs; each `./staging` keeps the run it replaces in `.staging/logs/` |
| `./staging status` | the services and whether they are healthy |
| `./staging setup-code` | the code that makes the first admin |
| `./staging down` | stop staging; its data stays |
| `./staging wipe` | stop staging and delete its data, after you type `wipe`; your values and the tunnel's keys stay |

**After `yoloai apply`**, run `./staging` again: it builds only what changed and restarts the
services whose image changed. The dev loop goes on as it was.

**When the schema changed.** Until migrations are live (D228), a schema change can leave staging's
database one the new build cannot bring forward. Staging then sets it aside in
`.staging/core/backups/` as `persistent-set-aside-<time>.db`, starts with an empty one, and says so
in its log (D249 §4). You make the first admin and the libraries again; the media is untouched.
Release will refuse instead.

## When something is wrong

- **`./staging` stops with what `.staging/values.toml` needs**: fill in what it names.
- **No handshake on the VPS**: the VPS's `wg0.conf` must hold staging's public key
  (`.staging/tunnel/public`), and Hetzner's firewall must let in UDP 51820. `./staging logs`
  shows the tunnel's line naming the VPS's end and the MTU.
- **Large pages or videos stall from outside, small ones load**: the tunnel's MTU is too large for
  the path. Set `mtu = 1280` in `[vps]` and run `./staging` again.
- **`./staging status` shows the core unhealthy after the tunnel's container restarted on its
  own**: the core shares the tunnel's network, and a tunnel container restarted alone has a new one
  the core is not in. `./staging` recreates both, which mends it. (A reboot, or Docker restarting,
  brings all three back together and needs nothing.)
- **The System page warns that FFmpeg runs unconfined**: the kernel lacks Landlock or seccomp,
  which Docker's default profile allows on a kernel that has them. The node runs anyway and says
  which is missing (D273 §5); `confinement_policy = "require"` in the node's config would refuse.
- **Port 8440 is taken**: set `[http] port` to another and run `./staging` again.
