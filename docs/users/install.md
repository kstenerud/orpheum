# Installing Orpheum

Orpheum runs as two containers from one image: the **core** (the server, its web client and API)
and the **node** (which reads your media and runs FFmpeg). It needs a machine with Docker Engine
and its compose plugin, run as root as Docker's own packages install it (rootful), and your films
and series on folders that machine can read.

There are two ways to install it:

- **One compose file** ([below](#one-compose-file)): paste it into OpenMediaVault, Portainer or
  any compose tool, or put it in a folder, fill in three values, and start it. Its first start
  writes the server's settings from those values (D447); nothing runs on the machine itself.
- **The install script** ([the other route](#the-install-script)): one command at a terminal,
  which finds the machine's address, starts the server and asks for the first admin there.

Something went wrong? [setup-problems.md](setup-problems.md) lists what has, and the fixes.

## One compose file

The file is [`deploy/omv/compose.yaml`](../../deploy/omv/compose.yaml). It reads these variables;
the first three are needed, and compose refuses to start without them.

| Variable | What it is | Unless set |
| --- | --- | --- |
| `PATH_TO_APPDATA` | the folder that holds each app's data; Orpheum keeps its own in `orpheum/` there | needed |
| `ORPHEUM_MEDIA` | the folder that holds your films and series, as the machine names it | needed |
| `ORPHEUM_LAN_ADDRESS` | the machine's address on your home network, `192.168.1.20`; keep it fixed with a reservation in your router | needed |
| `ORPHEUM_TAG` | the version to run | the file's |
| `APPUSER_PUID`, `APPUSER_PGID` | the user and group the server runs as, which own its data and must be able to read the media | 1000, 100 |
| `TIME_ZONE_VALUE` | the server's time zone, `Europe/Berlin` | UTC |
| `ORPHEUM_HTTP_PORT` | the port a browser at home opens it on: `http://<address>:8440` | 8440 |
| `ORPHEUM_HTTPS_PORT` | the port devices at home reach it on over HTTPS, and over WebRTC (UDP) | 8444 |
| `ORPHEUM_ROUTER` | your router's address, `192.168.1.1`, which the server asks to forward the HTTPS port so devices away from home reach it | none |
| `ORPHEUM_NAME` | the name the media node goes by in the server's pages | orpheum |
| `ORPHEUM_IPV6_SUBNET` | the private IPv6 /64 of the server's network, if another network on the machine has its default | `fd77:5e:0:ffff::/64` |
| `RENDER_GID` | the render group's number, for a GPU ([hardware encoding](#hardware-encoding)) | — |

Inside the containers your media folder is **`/media`**: a library's folder is `/media/Films`, not
the machine's own path. More folders go under it, each a line of its own in the node's `volumes`:
`- /srv/disk2/Shows:/media/Shows:ro`.

On the first start the core writes its settings into `PATH_TO_APPDATA/orpheum` from these values.
After that the files there rule and the values are not read again, so changing
`ORPHEUM_LAN_ADDRESS` alone changes nothing. To have the server take the values again, delete
`PATH_TO_APPDATA/orpheum/values.toml` and restart it: its settings are written again from the
values, and its database, people, libraries and keys are kept. A port is the exception: the port
mappings are compose's own, so `ORPHEUM_HTTP_PORT` takes effect at the next start; for
`ORPHEUM_HTTPS_PORT`, delete `values.toml` too, as the server tells devices that port.

### On any machine with Docker

1. Make a folder for it, and put `compose.yaml` in it.
2. Beside it, a file named `.env`:

   ```sh
   PATH_TO_APPDATA=/srv/appdata
   ORPHEUM_MEDIA=/srv/media
   ORPHEUM_LAN_ADDRESS=192.168.1.20
   APPUSER_PUID=1000
   APPUSER_PGID=1000
   TIME_ZONE_VALUE=Europe/Berlin
   ```

   `id -u` and `id -g` print your own user's numbers.
3. `docker compose up -d` in that folder.
4. Make the [first admin](#the-first-admin).

### OpenMediaVault

This follows omv-extras' own pages for apps such as Jellyfin. It needs OpenMediaVault 7 or 8 with
**omv-extras** and the **openmediavault-compose** plugin installed, and Docker installed from the
plugin's settings (**Services > Compose > Settings**, Docker's status *Installed and running*).

1. **Shared folders.** In **Storage > Shared Folders** you have, or make, a folder for your media
   (`Media`) and one for app data (`appdata`), as omv-extras' guide sets up. Each lies on a disk at
   a path like `/srv/dev-disk-by-uuid-1234abcd-…/Media`: the **Absolute Path** column shows it
   (switch it on from the column menu if it is hidden).
2. **The global environment.** In **Services > Compose > Files**, **Edit global environment file**
   should already set `PATH_TO_APPDATA` (the appdata folder's absolute path), `APPUSER_PUID` and
   `APPUSER_PGID` (the user apps run as, `appuser` in the guide) and `TIME_ZONE_VALUE`. Orpheum
   uses these as every app does. The app user needs read access to the media folder: **Storage >
   Shared Folders > Media > Permissions**, *Read-only* or *Read/Write* for that user.
3. **The port.** OpenMediaVault's own web interface takes port 80, and 443 when its HTTPS is on;
   omv-extras' guide moves it to 8443 to free them. Orpheum takes none of these: its plain page is
   on 8440 and its HTTPS on 8444. If something else has either, set `ORPHEUM_HTTP_PORT` or
   `ORPHEUM_HTTPS_PORT` to a free one.
4. **The file.** In **Services > Compose > Files**, **+ Add > Add**: name it `orpheum`, paste the
   whole of `compose.yaml` into **File**, and into **Environment** (this file's own variables):

   ```sh
   ORPHEUM_MEDIA=/srv/dev-disk-by-uuid-1234abcd-…/Media
   ORPHEUM_LAN_ADDRESS=192.168.1.20
   ```

   The address is the one OpenMediaVault's **Dashboard** shows under its network, or **Network >
   Interfaces**. **Save**.
5. **Start it.** Select the `orpheum` file and press **Up**. The first time it pulls the image, so
   give it a minute. Three containers appear in **Services > Compose > Containers**:
   `orpheum-folders`, which prepares the data folder and stops (*exited* is right), then
   `orpheum-core` and `orpheum-node`, both *healthy* once up.
6. Make the [first admin](#the-first-admin), reading the log from **Services > Compose >
   Containers**: select `orpheum-core`, then **Logs**.

To update, change `ORPHEUM_TAG` in the file's **Environment** (or leave it and take the file's
newer version), then **Pull** and **Up**. To stop it, **Down**; its data stays in
`appdata/orpheum`.

### Hardware encoding

With an Intel or AMD GPU, the node encodes on it. In the node's part of the file, uncomment
`devices` and `group_add`, and set `RENDER_GID` to the number of the group that owns the GPU's
render node: `ls -n /dev/dri/renderD128` shows it (often 105 or 106 on Debian), and in
OpenMediaVault **Users > Groups** lists `render` with its GID. The node runs as `APPUSER_PUID`, so
it needs that group to open the GPU; adding the app user to `render` and `video` in **Users >
Users**, as omv-extras' Jellyfin page does, is not enough on its own, since a container takes only
the groups it is given. Without a GPU, films that need preparing are encoded in software.

## The first admin

On its first start, and on every start until someone is made admin, the core writes a one-time
**setup code** to its log, in a line that begins `no one can sign in yet` and ends
`setupCode=xxxx-xxxx-xxxx`:

- OpenMediaVault: **Services > Compose > Containers**, `orpheum-core`, **Logs**.
- Elsewhere: `docker logs orpheum-core 2>&1 | grep setupCode`.
- Or ask for it: `docker exec orpheum-core orpheum setup-code --instance /var/lib/orpheum/core`.

Then open `http://<ORPHEUM_LAN_ADDRESS>:8440` in a browser **on your home network**, and give the
code, a name and a password. That person is the admin. An admin can administer only from the home
network, the one `ORPHEUM_LAN_ADDRESS` is on.

Next, make a library: its folder is under `/media` ([above](#one-compose-file)).

## From away from home (optional)

At home, devices reach the server directly. To reach it from away, the server is paired with your
Orpheum account's hosted discovery, from the discovery section of the server's **System** page,
as an admin at home. Devices away from home then sign in at the discovery's page with your realm, and
reach the server over WebRTC, with no port to open. Setting `ORPHEUM_ROUTER` lets the server ask
the router to forward its HTTPS port, which helps where the router allows it. There is no relay:
a device that can meet the server only through one does not connect, and the System page counts
each time.

## The install script

At a terminal on the machine, as a user in the `docker` group or as root:

```sh
docker run --rm ghcr.io/kstenerud/orpheum:<tag> install-script | sh -s -- --media /path/to/films [--media /path/to/shows]
```

It writes the deployment into `~/orpheum` (`--dir` names another), finds the machine's address
and router itself, starts the server, reports what the machine has (Landlock, a GPU, IPv6), and
asks there for the first admin and, optionally, your account's key; with no terminal it prints the
setup code instead. Run again with a newer tag, it updates, keeping everything. Media folders are
seen inside at their own paths, so a library names the path the machine knows.
