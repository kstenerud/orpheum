# Release: Orpheum on nas.example, from a tag

Release is the deployment staging is (`deploy/staging/README.md`): the same runtime image and the
same three services, written by the image's own `orpheum write-deployment`. What differs is where
the image comes from, a version tag of this repository built on the machine itself, and what the
core does with a database it cannot migrate: release refuses it and says why; it never starts over
(D249, D272).

It runs on nas.example (D272 §1): a Proxmox VE host on Debian trixie, with Orpheum in an LXC container,
the media bind-mounted into it and the Intel N95's render node passed through. Commands below say
where they run: **host** is the Proxmox host, **container** the LXC container.

## 1. The host, once

**Landlock.** The node confines every FFmpeg run with Landlock and seccomp; without them it runs
and the System page warns (D273 §5). Proxmox's kernel leaves Landlock out of its default LSM list,
so to have it, add it to the kernel's command line (docs/contributors/idiosyncrasies.md, "Whether Landlock is available"):

- Booted by systemd-boot (a ZFS root on UEFI): append to the one line of `/etc/kernel/cmdline`
  ` lsm=landlock,lockdown,yama,integrity,apparmor`, then `proxmox-boot-tool refresh`.
- Booted by GRUB: add the same to `GRUB_CMDLINE_LINUX_DEFAULT` in `/etc/default/grub`, then
  `update-grub`.

Reboot, then `cat /sys/kernel/security/lsm` must list `landlock`.

**Kernel modules** a container cannot load for itself: WireGuard, for the tunnel, and BBR, which
keeps a film's rate on a lossy link (D263 §3):

```sh
printf 'wireguard\ntcp_bbr\n' > /etc/modules-load.d/orpheum.conf
modprobe wireguard && modprobe tcp_bbr
```

## 2. The container, once

Create an **unprivileged** Debian trixie container (the web UI or `pct create`), then, with it
stopped, add to `/etc/pve/lxc/<id>.conf` on the host:

```
features: nesting=1,keyctl=1
mp0: /tank/media/video,mp=/tank/media/video,ro=1
dev0: /dev/dri/renderD128,gid=<the container's render group>
```

- `nesting` and `keyctl` let Docker run inside it.
- `mp0` gives it the media, read-only, at the same path. In an unprivileged container the host's
  owners show as `nobody`, so the media must be readable by others (`chmod -R o+rX` on the host if
  it is not); Orpheum only reads it.
- `dev0` passes the GPU's render node through. Its `gid` is the `render` group's inside the
  container: start it once and run `getent group render` there.

Give the container room: at least 32 GiB of disk, or a dataset of the pool mounted at
`/srv/orpheum` (`mp1: /tank/orpheum,mp=/srv/orpheum`). Proxmox's default 8 GiB is too little: the
node keeps prepared copies there, up to `prepared_budget_gib`, and a build pulls the toolchains.

In the container: install Docker from Docker's own packages (`docker-ce docker-ce-cli containerd.io
docker-buildx-plugin docker-compose-plugin`: Debian's `docker.io` has no BuildKit, which the build
needs, and staging runs on Docker's), make a user for Orpheum, add it to the `docker` and `render`
groups, and, for building from a tag (§3), as that user clone this repository:

```sh
git clone git@github.com:kstenerud/orpheum.git ~/orpheum
```

## 3. Install

As the Orpheum user in the container, with `/srv/orpheum` owned by it:

1. **Share staging's tunnel keys** (D272 §8): copy `.staging/tunnel/private` and `public` from the
   sandbox into `/srv/orpheum/tunnel/` (mode 600 for `private`), so the VPS needs no change. The
   agent puts them in the sandbox's shared files when asked; delete them there afterwards.
2. `~/orpheum/deploy/release/build.sh v0.1.0 /srv/orpheum` (the newest tag; `git -C ~/orpheum
   fetch --tags` brings new ones). It builds the image `orpheum:v0.1.0` (the first build takes
   several minutes), writes `/srv/orpheum/values.toml`, and stops.
3. Fill in `values.toml`: the values marked "fill in" are staging's (`.staging/values.toml` in the
   sandbox), and `[media] roots`, one for each folder a library will be: a library takes whole
   roots, so `["/tank/media/video"]` would make one library of everything under it. For example
   `["/tank/media/video/movies", "/tank/media/video/shows"]`, and a root for each series folder that
   sits beside `shows/` rather than in it. Leave out `[node] name` and the node is named after the
   container.
4. Run the same `build.sh` again. It writes the compose file and the configs.
5. **Swap the public name to nas.example** (§6), then `docker compose up -d` in `/srv/orpheum`.
6. `docker compose exec orpheum-core orpheum setup-code --instance /var/lib/orpheum/core` prints
   the one-time code. Open `http://<container>:8440`, make the first admin, and add the libraries.
7. In Settings, under Keys and passwords, enter the Cloudflare token as `dns_provider_token`; the
   certificate follows. Enter `tmdb_api_key` too: without it nothing is identified.
8. In Settings, answer `server_upload_mbps` (the house's upload the films may use, D286 §5) and the
   time zone.
9. Tell the agent which schema version nas.example runs (the core's log says it at start), so it
   commits `internal/store/migrations/CHECKPOINT` (D272 §6).

## 3a. Install without a tag (the weekend fallback)

Until there is a version tag, nas.example can run the image staging runs, copied over rather than built:

1. Ask the agent for the fallback. It saves staging's image (`docker save`, about 140 MB),
   staging's `values.toml` and the tunnel's keys into the sandbox's shared files. Take them with
   `yoloai files get`, copy them to the container, and delete the keys from the shared files.
2. On the container, as the Orpheum user: put `values.toml` in `/srv/orpheum/`, and `private` and
   `public` in `/srv/orpheum/tunnel/` (mode 600 for `private`).
3. Edit `/srv/orpheum/values.toml`: `deployment = "release"`, `[media] roots` as §3 step 3 says, and
   `[node] name` removed or set to the container's name (staging's says `"sandbox"`). Leave the
   VPS's values as they are.
4. `build.sh --load orpheum-fallback.tar.gz /srv/orpheum` loads the image and writes the compose
   file and the configs, building nothing (`deploy/release/build.sh`, which the agent puts in the
   shared files too, or a clone's).
5. Go on from §3 step 5. Nothing is built, so Docker alone is needed: no git, no BuildKit.

## 4. Update

```sh
git -C ~/orpheum fetch --tags
~/orpheum/deploy/release/build.sh v0.2.0 /srv/orpheum
cd /srv/orpheum && docker compose up -d
```

The build runs the new image's `write-deployment`, so the compose file and the configs are the new
version's (D272 §7); `values.toml` and the keys are never touched. The core migrates the database
at start, through every step since the version nas.example last ran (D272 §5), and keeps a snapshot
taken before migrating. When it has, tell the agent the version, so the checkpoint moves.

## 5. Going back

The same two commands with the older tag. An older binary refuses a database a newer one migrated,
unless the newer one declared it compatible, and its refusal names the snapshot taken before the
migration (D181): stop the deployment, put the snapshot in place of `core/persistent/persistent.db`,
and start it again. What happened since the snapshot is lost.

## 6. Swapping the public name between staging and nas.example

Both use the one tunnel key, and the VPS forwards to whichever end dials it, so only one may run
its tunnel at a time (D272 §2, §8):

- To nas.example: the agent stops staging with `./staging down`, which removes its containers. Only
  stopping the sandbox is not enough: staging's containers start again whenever the sandbox's
  Docker does, and would take the tunnel back. Then `docker compose up -d` on nas.example.
- To staging: `docker compose down` on nas.example; then the agent runs `./staging`.

`wg show` on the VPS shows a handshake, but both ends come from the house's one address, so it
cannot tell them apart: ask each end with `docker compose exec orpheum-tunnel wg show wg0
latest-handshakes`. Each keeps its own database,
libraries and certificate.

## When something is wrong

- **The System page warns that FFmpeg runs unconfined**: Landlock is not on (§1); the warning names
  the cause. The node runs anyway.
- **The core is unhealthy and its log says the database is newer, or below the baseline**: the
  image is older than the database (§5), or the database is older than the oldest version this one
  migrates from, which the checkpoint prevents while it is kept up to date.
- **No GPU encoding**: `ls -l /dev/dri` in the container must show `renderD128` (§2's `dev0`), and
  `compose.override.yaml` must give the node `/dev/dri` and its group; `build.sh` writes both when
  the render node is there, so run it again after adding it.
- **The libraries page says a folder is not watched, at the watch limit**: the media has more
  folders than the kernel lets one user watch. On the host, raise it
  (`sysctl fs.inotify.max_user_watches=1048576`, and the same line in
  `/etc/sysctl.d/orpheum.conf`), then restart the node. Meanwhile the library's periodic scan
  (`watched_library_scan_every_hours`) finds what changes.
- Everything else is as staging's "When something is wrong" says, with `docker compose` in
  `/srv/orpheum` for `./staging`.
