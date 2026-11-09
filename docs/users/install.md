# Installing Orpheum

Orpheum runs as two containers from one image: the **core** (the server, its web client and API)
and the **node** (which reads your media and runs FFmpeg). It needs a machine with Docker Engine
and its compose plugin, run as root as Docker's own packages install it (rootful), and your films
and series on folders that machine can read.

The image runs on a 64-bit PC (amd64) and on a 64-bit ARM board such as a Raspberry Pi 4 or 5
(arm64) from `v0.3.2`; `v0.3.0` is amd64 only. Hardware encoding through Intel Quick Sync and AMD's
VAAPI is amd64 only; an ARM board encodes in software.

There are two ways to install it:

- **One compose file** ([below](#one-compose-file)): paste it into OpenMediaVault, Portainer or
  any compose tool, or put it in a folder, fill in three values, and start it. Every start
  writes the server's settings from those values (D447, D449); nothing runs on the machine itself.
- **The install script** ([the other route](#the-install-script)): one command at a terminal,
  which finds the machine's address, starts the server and asks for the first admin there.

Something went wrong? [setup-problems.md](setup-problems.md) lists what has, and the fixes.

## One compose file

The file is [`deploy/omv/compose.yaml`](../../deploy/omv/compose.yaml). It reads these variables;
the first three are needed, and compose refuses to start without them.

| Variable | What it is | Unless set |
| --- | --- | --- |
| `PATH_TO_APPDATA` | the folder that holds each app's data; Orpheum keeps its own in `orpheum/` there | needed |
| `ORPHEUM_MEDIA` | the folder that holds your films and series, as the machine names it: one that does not exist stops the start, naming it | needed |
| `ORPHEUM_LAN_ADDRESS` | the machine's address on your home network, `192.168.1.20`: not needed, as the server learns it from the first browser at home, and the setup page asks if it cannot tell; keep it fixed with a reservation in your router | optional |
| `ORPHEUM_MEDIA_2`, `ORPHEUM_MEDIA_3`, `ORPHEUM_MEDIA_4` | more folders of films and series, as the machine names them | none |
| `ORPHEUM_TAG` | the version to run | the file's |
| `APPUSER_PUID`, `APPUSER_PGID` | the user and group the server runs as, which own its data and must be able to read the media | 1000, 100 |
| `TIME_ZONE_VALUE` | the server's time zone, `Europe/Berlin` | UTC |
| `ORPHEUM_HTTP_PORT` | the port a browser at home opens it on: `http://<address>:8440` | 8440 |
| `ORPHEUM_HTTPS_PORT` | the port devices at home reach it on over HTTPS, and over WebRTC (UDP) | 8444 |
| `ORPHEUM_THIS_MACHINE_PORT` | the port a browser on the machine itself opens it on, `http://127.0.0.1:8441`, as from home ([admin refused](#the-first-admin)) | 8441 |
| `ORPHEUM_ROUTER` | your router's address, `192.168.1.1`, which the server asks to forward the HTTPS port so devices away from home reach it | none |
| `ORPHEUM_NAME` | the name the media node goes by in the server's pages | orpheum |
| `RENDER_GID` | the render group's number, for a GPU ([hardware encoding](#hardware-encoding)) | — |

Each media folder keeps the machine's own name inside the containers: with
`ORPHEUM_MEDIA=/srv/media`, a library's folder is `/srv/media/Films`. A folder not set is none, and
the server does not show it.

At every start the core writes its settings into `PATH_TO_APPDATA/orpheum` from these values, so to
change one, change the variable and start it again (D449): its database, people, libraries and keys
are kept. `values.toml` there is written from the variables each time, so an edit to it is lost.

### On any machine with Docker

1. Make a folder for it, and put `compose.yaml` in it.
2. Beside it, a file named `.env`:

   ```sh
   PATH_TO_APPDATA=/srv/appdata
   ORPHEUM_MEDIA=/srv/media
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

### Synology

On DSM 7.2 or newer with Container Manager. The paths below are examples: a Synology names each
folder after the volume it is on and the name you gave it, so read your own before you type them.

**A DS218, DS218play, DS118, DS418 or RS819** (the models with a Realtek RTD1296 processor) can run
it, but Synology does not offer Container Manager for them. A community script,
[ContainerManager_for_all_armv8](https://github.com/007revad/ContainerManager_for_all_armv8),
installs Synology's own Container Manager package on them. It is unofficial: neither Synology nor
this project supports it, and this project has not tested it. It needs **DSM 7.2 or newer**: if
**Control Panel → Info Center** shows an older DSM, update it first (**Control Panel → Update &
Restore**). Follow the script's own page to run it, then open **Package Center**, where Container
Manager is now installed, and carry on with step 1 below. The 32-bit models, such as the DS218j and
DS216j, cannot run it at all.

Know before you start that such a model is slow for video. Its processor can send a film as it is
to a device that plays it as it is, at home, which is most films on most devices. What it cannot do
is re-encode a film as it plays: a device that cannot play the film's own format, or a link too slow
for the film's own size (often the case away from home), needs a lighter copy made on the fly, and
this processor cannot make one as fast as the film plays. Such a playback stops to wait, again and
again. Choosing a lower quality in the player does not help, since a lower quality is a re-encode
too.

Its system is older too (Linux 4.4), which Orpheum runs on with one protection fewer. The **System**
page says so for the media machine: "FFmpeg runs without part of its sandbox, which this machine's
kernel does not offer", and that Landlock, which limits the files FFmpeg may open, needs Linux 5.13
or newer. DSM cannot give it that, so leave it: films play as they would with it, and the rest of
the sandbox still holds.

1. **Your media folder's path.** In **File Station**, right-click the shared folder that holds your
   films and series and choose **Properties**: **Location** is its path, such as `/volume1/video`
   or `/volume2/Media`. Write it down exactly, capitals included.
2. **A folder for the server's data.** In **File Station**, make a shared folder for apps if you have
   none (`docker` is usual), and read its **Location** the same way, such as `/volume1/docker`.
3. **Your user's numbers.** The server runs as your own Synology user, so it needs that user's two
   numbers, its uid and its gid:
   1. Turn on SSH: **Control Panel → Terminal & SNMP**, tick **Enable SSH service**, **Apply**.
   2. On your computer, open a terminal (Windows: **PowerShell**; Mac: **Terminal**) and type
      `ssh <your user>@<the Synology's address>`, such as `ssh anna@192.168.1.30`, then the password
      you sign in to DSM with.
   3. Type `id` and press Enter. It shows something like:

      ```
      uid=1026(<your user>) gid=100(users) groups=100(users),101(administrators)
      ```

      The number after `uid=` is the uid, here `1026`; the number after `gid=` is the gid, here `100`.
      On most Synologys they are exactly these, 1026 and 100, for the first user made, but use the
      ones yours shows.
   4. Type `exit`. You may turn SSH off again.

   This user must be able to read the media folder: **Control Panel → Shared Folder**, select it,
   **Edit → Permissions**, and *Read only* or *Read/Write* for that user.
4. **The project.** **Container Manager → Project → Create**: name it `orpheum`, give it a new folder
   inside the data folder (`/volume1/docker/orpheum-project`), and paste
   [`deploy/omv/compose.yaml`](../../deploy/omv/compose.yaml) as the file.
5. **Its settings.** Beside that file, in the project's folder, put a file named `.env` holding your
   own values from steps 1–3 (make it on your computer and upload it with **File Station**):

   ```
   PATH_TO_APPDATA=<the data folder from step 2>
   ORPHEUM_MEDIA=<the media folder from step 1>
   APPUSER_PUID=<the uid from step 3>
   APPUSER_PGID=<the gid from step 3>
   ```

6. Build and start it, then follow [the first admin](#the-first-admin).

Other Synology models whose DSM runs an older Linux than 5.13 lack Landlock too, and the System page
says so the same way.

### Hardware encoding

With an Intel or AMD GPU, the node encodes on it. In the node's part of the file, uncomment
`devices` and `group_add`, and set `RENDER_GID` to the number of the group that owns the GPU's
render node: `ls -n /dev/dri/renderD128` shows it (often 105 or 106 on Debian), and in
OpenMediaVault **Users > Groups** lists `render` with its GID. The node runs as `APPUSER_PUID`, so
it needs that group to open the GPU; adding the app user to `render` and `video` in **Users >
Users**, as omv-extras' Jellyfin page does, is not enough on its own, since a container takes only
the groups it is given. Without a GPU, films that need preparing are encoded in software.

## The first admin

On its first start, and on every start until someone is made admin, the core makes a one-time
**setup code**. It is in the file **`SETUP-CODE.txt`** in `PATH_TO_APPDATA/orpheum` (on
OpenMediaVault, the `appdata` shared folder's `orpheum` folder), readable by the app user alone and
removed once the first admin is made. It is also in the core's log, in a line that begins
`no one can sign in yet` and ends `setupCode=xxxx-xxxx-xxxx`:

- OpenMediaVault: **Services > Compose > Containers**, `orpheum-core`, **Logs**.
- Elsewhere: `docker logs orpheum-core 2>&1 | grep setupCode`.
- Or ask for it: `docker exec orpheum-core orpheum setup-code --instance /var/lib/orpheum/core`.

Then open `http://<the machine's address>:8440` (or the port `ORPHEUM_HTTP_PORT` names, which the
file and the log line give) in a browser **on your home network**, and give the code, a name and a
password. That person is the admin. An admin can administer only from the home
network; the server learns the machine's address there from this first browser.

If the server refuses admin because it does not count your browser as at home (a VPN, a second
network), open it on the machine itself instead: `http://127.0.0.1:8441` (`ORPHEUM_THIS_MACHINE_PORT`)
is published on the machine's own loopback alone, and the server counts it as home (D334 §4). From
another computer, `ssh -L 8441:127.0.0.1:8441 <the machine>` and open the same address there.

Next, make a library: its folder is one of your media folders, by the machine's own name
([above](#one-compose-file)).

## From a catalogue

[`deploy/catalogue/`](../../deploy/catalogue/README.md) holds the same compose file packaged for
Portainer's app templates and CasaOS's app store. They ask the same values; the address at home is
learned, as above.

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
