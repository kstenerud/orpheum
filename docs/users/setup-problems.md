# Setting up a server: problems seen, and their fixes

What went wrong when a server was set up as a household sets one up, and what fixed it. Each entry
starts with what you see. They come from real installs (the first, the production NAS, on 2026-10-19), and
grow with each one.

Commands marked **as root** run on the machine the server is installed on, or, in a Proxmox
container, inside the container (`pct enter <id>` on the Proxmox host).

## The installer says "a public IPv6 address: no"

Devices away from home then reach the server only where IPv4 lets them. If your home has no public
IPv4 of its own, as with DS-Lite (common in Germany), or behind a provider's shared address
(carrier-grade NAT), IPv6 is the only way a phone away from home reaches the server without a
relay. The server has no relay, so a device that can meet it only through one does not connect;
the server's Remote access page counts each time.

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

## Pulling the image is refused: `denied`, `unauthorized`, `403 Forbidden` or a timeout from ghcr.io

The image is published on GitHub's image registry, `ghcr.io`, alone, and anyone may pull it without
signing in. A refused pull comes from between the machine and the registry:

- **A firewall, proxy or sandbox that blocks GitHub's hosts.** The pull reaches `ghcr.io`, and the
  image's layers come from `pkg-containers.githubusercontent.com`; both must be allowed. An AI
  agent's own sandbox often allows neither: run the pull at a terminal of your own instead.
- **An old sign-in to ghcr.io** that has expired or been revoked makes the registry answer `denied`,
  though the image needs no sign-in. Sign out, **as the user who runs Docker**:
  `docker logout ghcr.io`, then pull again.
- **A name typed wrong** answers `denied` or `manifest unknown`, as an image that does not exist
  does. The name is `ghcr.io/kstenerud/orpheum`, with a tag such as `latest`.

To check, `docker pull ghcr.io/kstenerud/orpheum:latest` on the machine should end with
`Status: Downloaded newer image` or `Status: Image is up to date`.

## A phone away from home says "Your server was found, but can't be reached from this network right now"

The sign-in worked; the phone and the server found no way to each other. Common causes:

- **A travel eSIM or a VPN on the phone.** These usually send the phone's traffic out through
  their operator's own gateway, often in another country, over IPv4 only and behind a shared
  address. The phone then has no IPv6 to meet the server on. Try again on the phone's own mobile
  data or on Wi-Fi, with any VPN off; `test-ipv6.com` on the phone shows whether it has IPv6.
- **The server has no IPv6 route out**: the two entries above.

Each such failure is counted on the server's **Remote access** page, in the menu under **Manage**
("Times in the last week a device found no way to reach this server").

## Signing in by household says "The household, name or password is wrong" just after the household was given or changed

Signing in by household, name and password, away from home, works only after each person has signed
in once **with their password on the server itself**: at home, or through their own link. That
sign-in makes the key the household sign-in is checked against, from the household, the name and
the password. The server keeps only a hash of each password, so it cannot make the key by itself.
The same happens each time the server is given a household or its household is renamed.
Once that sign-in is done, signing in by household works.

It says the same, for everyone, until the server is connected to your account and claimed
([Connect it to your account](install.md#connect-it-to-your-account)): your account's page says
whether a server was enrolled. At home you need none of this: open the server's own address,
`http://<the machine's address>:8440`.

## The page is blank, or says "This browser is too old for Orpheum"

The browser is older than the web client reads. It needs **Chrome or Edge 120, Firefox 140, or
Safari 17.1** (on macOS, iPhone and iPad) or newer. An older one that loads the page says
"This browser is too old for Orpheum" since 2026-11-19; before then it showed a blank page. Update
the browser, or use another one on the same device.

A TV or a TV box often keeps an old browser the app store no longer updates: an NVIDIA Shield whose
Chrome was 67 showed a blank page until a current Chrome was installed on it. Where no current
browser can be installed and the TV has Chromecast built in, cast to it from a phone instead.

## After the server was replaced, a device says "This link no longer finds its server"

A device that used the old server opened the page it had kept from then. **Reload the page once**;
it then takes you to the sign-in, and signing in finds the new server, after asking you to confirm
that the server has changed. Since 2026-10-19 the page is loaded fresh each time it opens, and the
kept copy is used only when that fails, so this happens only to a device that kept a page from
before then.

## The installer says "rootless Docker: its userland proxy hides each device's address"

Rootless Docker passes every connection to the server through a proxy of its own, so the server
sees each phone and computer at Docker's address instead of its own, never at home. Administering
works only from home, so no one can administer it, and the server says "administering works only
from the server's own network for now: open it at home".

RootlessKit 3, which Docker 29 ships, keeps each device's address once the proxy is off. In this
order (turning the proxy off first stops Docker from starting at all):

1. As root, load the kernel module it needs, now and at every start:
   `modprobe br_netfilter && echo br_netfilter > /etc/modules-load.d/br_netfilter.conf`
2. As the user Docker runs as, add `"userland-proxy": false` to `~/.config/docker/daemon.json`
   (keep anything else it has), then `systemctl --user restart docker`.
3. Run the installer again with the same command. It says the proxy is off.

RootlessKit 3.2.0 alone lost the address even so; 3.3.0 fixed it (`rootlesskit --version`).

## The installer says Landlock "cannot be seen from here"

In a container, as in Proxmox's, the installer cannot read the host's list of security modules,
so it cannot tell. The server checks for itself: its **System** page warns if FFmpeg runs
unconfined. To check the host itself, run `cat /sys/kernel/security/lsm` on it (on Proxmox, the
host's shell, not the container's): the list must include `landlock`; if it does not,
`cat /proc/cmdline` shows whether the kernel was started with `lsm=landlock,…`.

## The installer says "an NVIDIA GPU: yes, but Docker cannot hand it to a container"

An NVIDIA card encodes only with its driver's own libraries, which must match the driver the
machine runs, so the server's image cannot carry them. NVIDIA's Container Toolkit puts them into
the container as it starts. Until it is installed, films that need preparing are encoded on the
processor.

- **Install the toolkit** as NVIDIA's guide says
  (https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html),
  then run the installer again with the same command. It tries the card both ways the toolkit
  offers, CDI and Docker's `nvidia` driver, and uses whichever works.
- **Rootless Docker** takes the card by CDI only: as the user Docker runs as,
  `nvidia-ctk cdi generate --output ~/.config/cdi/nvidia.yaml`, then run the installer again.
- **It worked before, and stopped after an update.** A driver update replaces the driver's
  libraries at once but its kernel module only at a restart, and until then nothing can use the
  card: `nvidia-smi` says "Driver/library version mismatch". Restart the machine. A CDI spec made
  by hand also names the driver's files as they were, so make it again after any driver update
  (the toolkit 1.18 and later keeps its own up to date), then run the installer again.
- **To check that it worked:** the server's System page names the media node's encoder, "NVIDIA
  NVENC".

## System says the media node is locked

The node runs FFmpeg on every film it reads, and a crafted file can attack FFmpeg, so a release node
runs it inside Landlock and seccomp. On a kernel without one of them the node runs without it, and
the System page says, calmly, which kernel brings it (since 2026-11-09). It starts locked
only when the kernel has a mechanism but putting it in force failed, which a container runtime's
own policy usually causes: it connects to the server, and the System page says what failed, but it
reads no films and runs no FFmpeg. Its log (`docker compose logs orpheum-node`, in the server's
directory) says `confinement is not in force; the node is locked` with the node's policy.

- **Landlock not listed** (the note, not a lock): the host's kernel must list it. On the host (on Proxmox, the host's shell,
  not the container's), `cat /sys/kernel/security/lsm` must include `landlock`; if it does not, add
  `lsm=landlock,` before the list `cat /proc/cmdline` shows, in the kernel's command line
  (`GRUB_CMDLINE_LINUX` in `/etc/default/grub`, then `update-grub` and a restart), or use a newer
  kernel.
- **seccomp missing:** the kernel was built without `CONFIG_SECCOMP_FILTER`; use one with it.
- **Blocked by the runtime** (a lock): the container runtime refused it, as an AppArmor or seccomp
  profile of its own may; run the node with the compose file's own settings.
- **If you cannot fix the kernel** and accept the risk, an admin, at home, presses **Run without
  confinement** on the System page. The node then starts its work unconfined, the System page says
  who allowed it and when, and **Withdraw** locks it again. (Setting `confinement_policy = "report"`
  under `[node]` in the node's config does the same for good, without asking.)

## Films on a NAS stop playing when the NAS is off, or System warns "mounted to wait for its server for good"

When the machine your films are shared from goes away (switched off, asleep, rebooting), a share
mounted the way NFS mounts by default ("hard") makes every read of it wait until it is back. The
wait happens in the kernel, so nothing can end it: not the media node, and not `docker stop`. The
node gives up on such a read after 30 seconds and says the folder cannot be reached. Players then
say so too, rather than spin. But the read itself stays stuck until the share answers, and the node
cannot restart until then.

Mount the share so its reads give up instead. **As root**, on the machine that mounts it, add
`vers=3,soft,timeo=100,retrans=3,nolock` to its options in `/etc/fstab`, for example:

```
nas:/films  /media/films  nfs  ro,vers=3,soft,timeo=100,retrans=3,nolock  0  0
```

Then remount it (`umount /media/films && mount /media/films`) and restart the node. A read then
fails after about 40 seconds while the NAS is away; the server makes the folder unavailable at once,
and brings it back by itself within seconds of the share answering again. For read-only media,
`soft` loses nothing. Do not add `softreval`: it lets a check of the share pass while the NAS is
away.

Use NFS version 3 (`vers=3`). Versions 4.1 and later can stop answering for good after the NAS has
been unreachable, until the machine restarts, and their reads do not give up while the connection
looks alive.

**After a network outage, the share may come back up to a minute after the NAS does.** That is the
Linux network stack waiting longer between each retry; nothing in Orpheum can shorten it. On Linux
6.15 or later, setting `net.ipv4.tcp_rto_max_ms = 5000` (in `/etc/sysctl.d/`, then a remount) on
the machine that mounts the share should cut the wait to a few seconds (not yet tested here).

If the films reach the server through a virtual machine or container that passes on a share from
its host (virtiofs, 9p), the System page notes that it cannot see how the share behind is mounted:
set these options where the share is actually mounted, on the host. With virtiofs, also give its daemon a
pool of threads (`--thread-pool-size=16`, or your VM manager's equivalent): without one, a single
read waiting on the NAS holds up every other file request from the VM.


## The node's log says "a streamed film's pipe could not take its size"

The node feeds FFmpeg each film it reads through a pipe of 1 MiB. Linux limits the pipe memory one
user may hold (`fs.pipe-user-pages-soft`, 64 MiB by default), and past it every new pipe holds
8 KiB. Films still play and prepare, only slower. It takes dozens of runs at once to reach the
limit, so a node that logs this is running many, or shares its user with other programs that hold
pipes.

Raise the limit on the host (a container shares its host's): add
`fs.pipe-user-pages-soft = 65536` (256 MiB) to `/etc/sysctl.d/90-orpheum-pipes.conf`, then run
`sysctl --system`. The node logs this once per start.

## Installed from the one compose file, or on OpenMediaVault

These are for a server installed from OpenMediaVault's `orpheum-omv.yaml`
([`deploy/omv/compose.yaml`](../../deploy/omv/compose.yaml) in the repository), or a Synology's
`orpheum-synology.yaml` ([`deploy/synology/compose.yaml`](../../deploy/synology/compose.yaml))
([install.md](install.md)). Its containers are `orpheum-core`, `orpheum-node` and `orpheum-folders`;
in OpenMediaVault their logs are under **Services > Compose > Containers**, elsewhere
`docker logs <name>`.

### Up says "required variable PATH_TO_APPDATA is missing a value" (or `ORPHEUM_MEDIA`)

Compose stops before anything starts: the file needs that value. In OpenMediaVault, put it in the
file's **Environment** (tick **Show environment file** to see it): `ORPHEUM_MEDIA`, and
`PATH_TO_APPDATA` with the user and time zone unless the global environment file sets them
([Add the compose file](install.md#add-the-compose-file), step 6);
elsewhere, in the `.env` file beside `compose.yaml`. (Before 2026-11-09 it needed
`ORPHEUM_LAN_ADDRESS` too; the server now learns the address.)

### Save says "Please set shared folder for file storage"

The whole message reads `500 - Internal Server Error: Please set shared folder for file storage.`,
and comes when **Save** is pressed in **Services > Compose > Files**. Compose has nowhere to keep
the file: open **Services > Compose > Settings**, choose a **Shared folder** under **Compose
Files** (`appdata`, or any other), press **Save**, then apply
([Tell Compose where its files go](install.md#tell-compose-where-its-files-go)). Then **Add** the
file again, and **Save** works.

### Up says "container orpheum-core is unhealthy"

Or "dependency failed to start", or `orpheum-node` "not started". Compose waits for the core to say
it is well before it starts the node, and the core did not. While it starts, **Up** stops at
`orpheum-core Waiting` for up to a minute or two: that is normal. When it ends unhealthy instead, the
core could not start, and its log says why. The core stays up to report it.

Read the core's log: in OpenMediaVault, **Services > Compose > Containers**, select `orpheum-core`,
**Logs**; on a Synology, **Container Manager → Container**, `orpheum-core`, **Details**, **Log**;
elsewhere, `docker logs orpheum-core`. Its last `ERROR` line says `the server could not start`,
and its `err=` names the cause. Look for its words on this page; the most common is
the next entry. From a terminal, a compose file from a release after v0.8.7 also keeps the reason in the health
check: `docker inspect --format '{{json .State.Health}}' orpheum-core`.

### Asked for a problem report, and the core does not start, so System cannot make one

Make it from a terminal instead:
`docker exec orpheum-core orpheum bugreport safe --instance /var/lib/orpheum/core --words "what went wrong"`
prints a report to share, each title, path and name in it replaced by a tag. Read it before you post
it. `unsafe` in place of `safe` prints everything, with what each tag stands for: keep that one to
yourself. Either says what doctor finds, whether the database opens, and the core's recent log when
it writes one to a file; a core that logs to the container's log has none to give, so add the end of
`docker logs orpheum-core` yourself, read first. A report made this way asks no node for its log.

### The core's log says `disk I/O error (5386)`

The whole line reads like `database "/var/lib/orpheum/core/persistent/persistent.db": read PRAGMA
journal_mode: disk I/O error (5386)`, and **Up** said `orpheum-core is unhealthy`. A release after
v0.8.7 starts it with the cause in words: `the data folder's filesystem refuses the shared memory
map the database needs`.

The app data folder (`PATH_TO_APPDATA`) is on a filesystem that refuses a shared memory map of a
file, which the server's database needs (SQLite's write-ahead log maps its `-shm` file). A mergerfs
pool mounted with `cache.files=off` refuses it, and OpenMediaVault's mergerfs plugin mounts its pools
so. Media on such a pool is fine; only the app data folder must move.

Put `PATH_TO_APPDATA` on a single disk instead. On OpenMediaVault, a pool's folder lies on one of the
pool's disks, under `/srv/dev-disk-by-uuid-…`, and that path can be used directly:

1. Over SSH, find the disk that holds the folder: `ls -ld /srv/dev-disk-by-uuid-*/appdata`. It
   prints each disk's `appdata`, such as `/srv/dev-disk-by-uuid-1234abcd-…/appdata`. If it prints
   more than one, use the one that holds Orpheum's data, which
   `ls -ld /srv/dev-disk-by-uuid-*/appdata/orpheum` shows.
2. Select the `orpheum` file and press **Down**.
3. Edit the file, and in **Environment** set `PATH_TO_APPDATA` to that path. **Save**. (If the global
   environment file sets it, change it there.)
4. Press **Up**. The core starts, with the data it already had.

Without a shell, make a new shared folder, such as `orpheum-data`, in **Storage > Shared Folders**,
choosing a **File system** that starts with `/dev/` (each is a single disk; a pool is listed by its
own name), and set `PATH_TO_APPDATA` to its **Absolute Path**; the server starts there afresh.

A pool mounted with `cache.files=partial`, `auto-full` or `per-process` allows the map, and holds the
app data as a disk does. `systemctl cat srv-mergerfs-<pool>.mount` prints how a pool is mounted.

### Up says "bind source path does not exist" and names a folder

That folder is `ORPHEUM_MEDIA`, and it is not there as you typed it: a letter wrong, a capital
missed, or the wrong volume (`/volume1` for `/volume2`). Nothing was made. Read the folder's real
path (OpenMediaVault: **Storage > Shared Folders**, the **Absolute Path** column; Synology: **File
Station**, right-click the folder, **Properties**, **Location**), set `ORPHEUM_MEDIA` to it exactly
(on a Synology, the line after `&media ` under `# Yours`), and start it again.

### Up says a port "is already allocated", or the page opens OpenMediaVault instead

Another service has the port: OpenMediaVault's own web interface takes 80 (and 443), or 8443 once
moved as omv-extras' guide moves it; another app may have 8440 or 8444. If it is 8440, set `ORPHEUM_HTTP_PORT` to a free
port, then **Up** again: every start takes the new value. If `127.0.0.1:8441` is the one
taken, set `ORPHEUM_THIS_MACHINE_PORT`. The HTTPS and WebRTC ports are set on the **System** page,
under **The house's network**: one that will not open at the start falls back to 8444, and the page
says so.

### `orpheum-folders` exited with an error, or the core's log says "permission denied" under /var/lib/orpheum

`orpheum-folders` makes `appdata/orpheum` and gives it to `APPUSER_PUID`:`APPUSER_PGID` before the
server starts; the server never runs as root, so it cannot write a folder Docker made for root.
It fails where the app data lies on a filesystem that refuses a change of owner, such as a network
share (NFS, SMB). Put `PATH_TO_APPDATA` on a local disk, or make `appdata/orpheum` and its `node`
and `run` folders yourself, owned by that user.

### Where is the link that makes the first admin? The core's log has scrolled away

Until the first admin is made, the link, with the setup code in it, is in `SETUP-CODE.txt` in
`appdata/orpheum`, readable by the app user (`APPUSER_PUID`) alone: on OpenMediaVault, where
that user is not the one you browse the shared folders as, the log is the way. The core also writes it
to its log at every start: restart `orpheum-core`, or run
`docker exec orpheum-core orpheum setup-code --instance /var/lib/orpheum/core`, which prints the code
and the links to open. Open one in a browser on your home network.

### Admin is refused: "only from the home network"

The server counts a browser as at home by its address; through a VPN, or from another network of
yours, it does not. Open it on the machine itself at `http://127.0.0.1:8441`
(`ORPHEUM_THIS_MACHINE_PORT`), which only the machine can reach and the server counts as home;
from another computer, `ssh -L 8441:127.0.0.1:8441 <the machine>` first.

### I changed a variable and nothing changed

Each start writes the settings from the variables, but a compose tool applies a changed
variable only when it recreates the containers: **Up** again (`docker compose up -d`), not a restart.
An edit to `appdata/orpheum/values.toml` is lost at the next start; change the variable instead.

### A library's folder is "not found", or the node's log says "permission denied" reading a film

Inside the containers each media folder keeps the machine's own name: a library names
`/srv/dev-disk-by-uuid-…/Media/Films`. An older install had its folder at `/media`;
repoint such a library to the machine's name. If the path is right, the app user cannot read it: in
OpenMediaVault, **Storage > Shared Folders**, the media folder, **Permissions**: give the user
`APPUSER_PUID` names read access (the folder's ACL too, if it has one).

### The GPU is not used: films are encoded in software

The node needs `/dev/dri` and the render group's number ([install.md](install.md#hardware-encoding)):
uncomment `devices` and `group_add` in the node's part of the file and set `RENDER_GID`, then **Up**.
The System page names the node's encoder once it uses the GPU.
