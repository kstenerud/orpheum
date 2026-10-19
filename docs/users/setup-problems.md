# Setting up a server: problems seen, and their fixes

What went wrong when a server was set up as a household sets one up, and what fixed it. Each entry
starts with what you see. They come from real installs (the first, nas.example, on 2026-10-19), and
grow with each one.

Commands marked **as root** run on the machine the server is installed on, or, in a Proxmox
container, inside the container (`pct enter <id>` on the Proxmox host).

## The installer says "a public IPv6 address: no"

Devices away from home then reach the server only where IPv4 lets them. If your home has no public
IPv4 of its own, as with DS-Lite (common in Germany), or behind a provider's shared address
(carrier-grade NAT), IPv6 is the only way a phone away from home reaches the server without a
relay. The server has no relay, so a device that can meet it only through one does not connect;
the System page counts each time.

- **A Proxmox container has no IPv6 until its network is given it.** In the Proxmox web UI: the
  container → **Network** → `net0` → **Edit** → **IPv6: SLAAC** → **OK**, then restart the
  container (`pct reboot <id>` on the host).
- **Elsewhere:** check that the router gives the home network IPv6 at all, and that the machine
  takes it: `ip -6 addr show scope global` should list an address starting with 2 or 3.

## The installer says "a public IPv6 address: yes, but no route out by it"

Or: the machine has an IPv6 address starting with 2 or 3, but `ip -6 route show default` prints
nothing, and Docker fails with `network is unreachable` when the DNS server is an IPv6 address.

Docker turns on IPv6 forwarding for the server's network, and a Linux machine that forwards IPv6
ignores the router's announcements, which carry its default route, unless it is told to keep
them. **As root**, with the interface the installer names (often `eth0`):

```sh
echo 'net.ipv6.conf.eth0.accept_ra = 2' > /etc/sysctl.d/90-orpheum-ipv6.conf
```

Then restart the machine, or the container (`pct reboot <id>`), and run the installer again.

**Do not take the network interface down and up instead** in a Proxmox container: that brings the
IPv6 route back, but loses the IPv4 one, which Proxmox sets only as the container starts.

## Pulling the image fails with `network is unreachable` though IPv6 works

GitHub's image registry, `ghcr.io`, has no IPv6 address, so the machine needs working IPv4 to
install or update the server, whatever IPv6 does. Check that `ip route show default` prints a
`default via …` line; in a Proxmox container, restarting it restores the IPv4 route.

## A phone away from home says "Your server was found, but can't be reached from this network right now"

The sign-in worked; the phone and the server found no way to each other. Common causes:

- **A travel eSIM or a VPN on the phone.** These usually send the phone's traffic out through
  their operator's own gateway, often in another country, over IPv4 only and behind a shared
  address. The phone then has no IPv6 to meet the server on. Try again on the phone's own mobile
  data or on Wi-Fi, with any VPN off; `test-ipv6.com` on the phone shows whether it has IPv6.
- **The server has no IPv6 route out**: the two entries above.

Each such failure is counted on the server's **System → Discovery** page ("Times in the last week
a client found no way to reach this server").

## After the server was replaced, a device says "This link no longer finds its server"

A device that used the old server opened the page it had kept from then. **Reload the page once**;
it then takes you to the sign-in, and signing in finds the new server, after asking you to confirm
that the server has changed. Since 2026-10-19 the page is loaded fresh each time it opens, and the
kept copy is used only when that fails, so this happens only to a device that kept a page from
before then.

## The installer says Landlock "cannot be seen from here"

In a container, as in Proxmox's, the installer cannot read the host's list of security modules,
so it cannot tell. The server checks for itself: its **System** page warns if FFmpeg runs
unconfined. To check the host itself, run `cat /sys/kernel/security/lsm` on it (on Proxmox, the
host's shell, not the container's): the list must include `landlock`; if it does not,
`cat /proc/cmdline` shows whether the kernel was started with `lsm=landlock,…`.
