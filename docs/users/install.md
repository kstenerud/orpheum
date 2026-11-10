# Installing Orpheum

Orpheum runs as two containers from one image: the **core** (the server, its web client and API)
and the **node** (which reads your media and runs FFmpeg). It needs a machine with Docker Engine
and its compose plugin, run as root as Docker's own packages install it (rootful), and your films
and series on folders that machine can read.

The image runs on a 64-bit PC (amd64) and on a 64-bit ARM board such as a Raspberry Pi 4 or 5
(arm64) from `v0.3.2`; `v0.3.0` is amd64 only. Hardware encoding through Intel Quick Sync and AMD's
VAAPI is amd64 only; an ARM board encodes in software.

There are two ways to install it:

- **One compose file** ([below](#one-compose-file)): paste it into OpenMediaVault, a Synology's
  Container Manager, Portainer or any compose tool, or put it in a folder, fill in a few values, and
  start it. Every start
  writes the server's settings from those values (D447, D449); nothing runs on the machine itself.
- **The install script** ([the other route](#the-install-script)): one command at a terminal,
  which finds the machine's address, starts the server and asks for the first admin there.

Something went wrong? [setup-problems.md](setup-problems.md) lists what has, and the fixes.

If you were invited to the Orpheum service (orpheum.cc), which lets your household watch from
anywhere, the whole way is:

1. Open the invitation link and make your account there, choosing your realm
   ([your account](#your-account-at-orpheumcc)). You can do this at any point before step 5.
2. Install the server, on [OpenMediaVault](#openmediavault), a [Synology](#synology) or
   [any machine with Docker](#on-any-machine-with-docker).
3. Make yourself its [first admin](#the-first-admin), with the setup code.
4. Add a [library](#your-first-library) of your films or series.
5. [Connect the server to your account](#connect-it-to-your-account), with a key from your account's
   page.
6. [Watch](#watching), at home or away.

## One compose file

Each way has its own copy of one compose file, attached to the latest release, so the file you
install is the one the running version was tested with (D462 §5):

- on OpenMediaVault, [`orpheum-omv.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-omv.yaml);
- on a Synology, [`orpheum-synology.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-synology.yaml);
- on any other machine with Docker, [`orpheum-docker.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-docker.yaml).

Each link downloads the file; open it in a text editor to copy it. They differ only in the one line
that tells the server which way it was installed, so its **Update** page shows that way's steps
alone, and on a Synology in the values written in it. The file reads these variables; the first two
are needed, and compose refuses to start without them.

| Variable | What it is | Unless set |
| --- | --- | --- |
| `PATH_TO_APPDATA` | the folder that holds each app's data; Orpheum keeps its own in `orpheum/` there | needed |
| `ORPHEUM_MEDIA` | the folder that holds your films and series, as the machine names it: one that does not exist stops the start, naming it | needed |
| `ORPHEUM_MEDIA_2`, `ORPHEUM_MEDIA_3`, `ORPHEUM_MEDIA_4` | more folders of films and series, as the machine names them | none |
| `ORPHEUM_TAG` | the version to run, such as `v0.3.3` | `latest`: each new release, when you pull it |
| `APPUSER_PUID`, `APPUSER_PGID` | the user and group the server runs as, which own its data and must be able to read the media | 1000, 100 |
| `TIME_ZONE_VALUE` | the server's time zone, `Europe/Berlin` | UTC |
| `ORPHEUM_HTTP_PORT` | the port a browser at home opens it on: `http://<address>:8440` | 8440 |
| `ORPHEUM_THIS_MACHINE_PORT` | the port a browser on the machine itself opens it on, `http://127.0.0.1:8441`, as from home ([admin refused](#the-first-admin)) | 8441 |
| `ORPHEUM_NAME` | the name the media node goes by in the server's pages | orpheum |
| `RENDER_GID` | the render group's number, for a GPU ([hardware encoding](#hardware-encoding)) | — |

Each media folder keeps the machine's own name inside the containers: with
`ORPHEUM_MEDIA=/srv/media`, a library's folder is `/srv/media/Films`. A folder not set is none, and
the server does not show it.

At every start the core writes its deployment into `PATH_TO_APPDATA/orpheum` from these values, so to
change one, change the variable and start it again (D449): its database, people, libraries and keys
are kept. `values.toml` there is written from the variables each time, so an edit to it is lost.

Every other setting is the server's own, kept in its database and changed on its pages (D462 §2).
The house's network is on the **System** page, under **The house's network**: the HTTPS port
devices at home reach it on (8444), the WebRTC port (UDP, 8444 too) and your router's address,
which the server asks to forward the HTTPS port so devices away from home reach it. A change there
takes effect when the server starts again, which the page offers. A port that will not open at the
start falls back to 8444, and the page says so, so no setting there can lock you out; the HTTP port
stays in the file for the same reason. The machine's address at home is not set anywhere: the
server learns it from the first browser at home, and the setup page asks if it cannot tell. Keep it
fixed with a reservation in your router.

### On any machine with Docker

1. Make a folder for it, and put [`orpheum-docker.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-docker.yaml)
   in it as `compose.yaml`.
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

To update, follow the server's **Update** page: in that folder, `docker compose pull && docker
compose up -d`.

### OpenMediaVault

This follows omv-extras' own pages for apps such as Jellyfin. It needs OpenMediaVault 7 or 8 with
**omv-extras** and the **openmediavault-compose** plugin installed, and Docker installed from the
plugin's settings (**Services > Compose > Settings**, Docker's status *Installed and running*).

1. **Shared folders.** In **Storage > Shared Folders** you have, or make, a folder for your media
   (`Media`) and one for app data (`appdata`), as omv-extras' guide sets up. Each lies on a disk at
   a path like `/srv/dev-disk-by-uuid-1234abcd-…/Media`: the **Absolute Path** column shows it
   (switch it on from the column menu if it is hidden). To make one, **+ (Create)**, give it its
   name and the disk, and **Save**. After each change OpenMediaVault shows a yellow bar asking to
   apply it: press its tick (**Apply**), or the change does not take.
2. **The global environment.** In **Services > Compose > Files**, **Edit global environment file**
   should already set `PATH_TO_APPDATA` (the appdata folder's absolute path), `APPUSER_PUID` and
   `APPUSER_PGID` (the user apps run as, `appuser` in the guide) and `TIME_ZONE_VALUE`. Orpheum
   uses these as every app does. If yours is empty, add them, with your own path and numbers:

   ```sh
   PATH_TO_APPDATA=/srv/dev-disk-by-uuid-1234abcd-…/appdata
   APPUSER_PUID=1000
   APPUSER_PGID=100
   TIME_ZONE_VALUE=Europe/Berlin
   ```

   The user's number is its **UID** in **Users > Users** (switch the column on if it is hidden),
   and 100 is the group `users`, which OpenMediaVault puts every user in. The app user needs read
   access to the media folder: **Storage > Shared Folders**, select `Media`, **Permissions**,
   *Read-only* or *Read/Write* for that user, **Save**.
3. **The port.** OpenMediaVault's own web interface takes port 80, and 443 when its HTTPS is on;
   omv-extras' guide moves it to 8443 to free them. Orpheum takes none of these: its plain page is
   on 8440 and its HTTPS on 8444. If something else has 8440, set `ORPHEUM_HTTP_PORT` to a free
   one in step 4's **Environment**; the HTTPS port is changed later on the server's **System**
   page.
4. **The file.** In **Services > Compose > Files**, **+ Add > Add**: name it `orpheum`, paste the
   whole of [`orpheum-omv.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-omv.yaml)
   into **File**, and into **Environment** (this file's own variables):

   ```sh
   ORPHEUM_MEDIA=/srv/dev-disk-by-uuid-1234abcd-…/Media
   ```

   with the media folder's own **Absolute Path** from step 1, capitals included. **Save**.
5. **Start it.** Select the `orpheum` file and press **Up**. The first time it pulls the image, so
   give it a minute. Three containers appear in **Services > Compose > Containers**:
   `orpheum-folders`, which prepares the data folder and stops (*exited* is right), then
   `orpheum-core` and `orpheum-node`, both *healthy* once up.
6. Make the [first admin](#the-first-admin), reading the setup code from the log: **Services >
   Compose > Containers**, select `orpheum-core`, then **Logs**. (The code's file in `appdata` is
   readable by the app user alone, so the log is the easier way here.)

To update, follow the server's **Update** page, in the admin's menu: select the `orpheum` file,
press **Pull**, then **Up**. When the update brings a newer compose file, the same page hands it
over with your values in it: keep a copy of your old file, then edit the `orpheum` file, paste the
new one over it, **Save**, and **Up**. To stop it, **Down**; its data stays in `appdata/orpheum`.

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
2. **A folder for the server.** In **File Station**, make a shared folder for apps if you have none
   (`docker` is usual), and in it a folder for Orpheum (**Create → Create folder**), `orpheum`.
   Read its **Location** as in step 1, such as `/volume1/docker/orpheum`. The server keeps its data
   in a folder of its own inside it, `data`, which it makes itself.
3. **Your user's numbers.** The server runs as your own Synology user, so it needs that user's two
   numbers, its uid and its gid:
   1. Turn on SSH: **Control Panel → Terminal & SNMP**, tick **Enable SSH service**, **Apply**.
   2. On your computer, open a terminal (Windows: **PowerShell**; Mac: **Terminal**) and type
      `ssh <your user>@<the Synology's address>`, such as `ssh anna@192.168.1.30`, then the password
      you sign in to DSM with. The address is the one in your browser's address bar when you use
      DSM, without the `:5000` after it. The user must be an administrator, as the one you set the
      Synology up with is. The first time, it asks whether to trust the machine: type `yes`. The
      password does not show as you type it.
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
4. **The project.** **Container Manager → Project → Create**: name it `orpheum`; for **Path**, choose
   the `orpheum` folder from step 2; for **Source**, **Create docker-compose.yml**, and paste
   [the Synology compose file](#the-synology-compose-file) into the box. Near its top, under
   `# Yours`, are three lines to change to your own, in the box itself:

   ```yaml
   media: &media /volume1/video
   user: &user "1026:100"
   time-zone: &timezone Europe/Berlin
   ```

   - after `&media `, the media folder from step 1;
   - between the quotes after `&user `, the uid from step 3, a colon, then the gid: `"1026:100"`;
   - after `&timezone `, your time zone, such as `Europe/London` or `America/New_York`.

   Change only what follows the `&` word; the rest of each line stays as it is. **Next** past the web
   portal page, and on the last page leave **Start the project once it is created** ticked, then
   **Done**. The first time it pulls the image, so give it a few minutes. In **Container Manager →
   Container**, `orpheum-folders` has stopped (that is right: it only prepares the data folder), and
   `orpheum-core` and `orpheum-node` are running. To change a value later, **Project**, select
   `orpheum`, **Action → Stop**, then **YAML Configurations**, change the line, **Save**, and
   **Action → Start**. Keep a copy of the whole file somewhere of your own: the server's **Update**
   page hands you a newer one when an update brings it, and keeps only the lines under `# Yours`.
5. Make the [first admin](#the-first-admin). The setup code is in the log: **Container Manager →
   Container**, select `orpheum-core`, **Details**, then the **Log** tab. It is also in
   `SETUP-CODE.txt` in the `data` folder (`/volume1/docker/orpheum/data`), which File Station can
   download for you to open.

#### Updating on a Synology

Follow the server's **Update** page, in the admin's menu:

1. **Container Manager → Project**, select `orpheum`, press **Stop**, then **Build**.
2. If the Update page still says the server runs the old version, Container Manager kept the image
   it had: under **Image**, select the `orpheum` image, press **Update**, then **Build** the project
   again.
3. When the update brings a newer compose file, the Update page hands it over with your values from
   `# Yours` filled in. Keep a copy of your old file, then **Project**, stop `orpheum`, open its
   **YAML Configurations**, paste the new file over it, **Save**, and **Build**.

#### The Synology compose file

The file is [`orpheum-synology.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-synology.yaml):
the [one compose file](#one-compose-file) with the values a Synology needs written under `# Yours`,
so it needs no `.env` file, and its data in the project's own folder. Open the downloaded file in a
text editor to copy it. The ports are under `# Yours` too; every other setting in the table above
keeps its default.

If DSM's firewall is on (**Control Panel → Security → Firewall**), allow port 8440 (TCP) and the
HTTPS port, 8444 unless you changed it on the **System** page (TCP and UDP), or the server's page
does not open from your other devices.

Other Synology models whose DSM runs an older Linux than 5.13 lack Landlock too, and the System page
says so the same way.

### If an update broke it

Go back to the version before, which your data still works with:

1. **Pin the version before.** The Update page, or the release list on GitHub, names it, such as
   `v0.3.3`. On OpenMediaVault, add `ORPHEUM_TAG=v0.3.3` to the file's **Environment**; on any
   other machine with Docker, to the `.env` file. On a Synology, in the project's **YAML
   Configurations**, write `v0.3.3` in place of
   each `${ORPHEUM_TAG:-latest}`: the `image:` lines and the line after `--image-tag`.
2. **Start it again**: **Pull** and **Up** on OpenMediaVault, **Stop** then **Build** on a
   Synology, `docker compose pull && docker compose up -d` elsewhere.
3. **If it will not start** and the core's log says its database needs a newer release, the update
   had already moved the database on. Before it did, the server kept a copy of it,
   `pre-migrate-v<n>.sqlite`, in `orpheum/core/backups/` in the app data folder (`data/core/backups/`
   in the project's folder on a Synology). With the server stopped (**Down**, or **Stop**), restore
   it from a terminal on the machine, as an administrator, with the version before, your app data
   folder and the user the server runs as:

   ```sh
   docker run --rm --user 1000:100 -v /srv/appdata/orpheum:/var/lib/orpheum ghcr.io/kstenerud/orpheum:v0.3.3 \
     restore --instance /var/lib/orpheum/core --from /var/lib/orpheum/core/backups/pre-migrate-v<n>.sqlite
   ```

   On a Synology the folder is the project's `data` folder, `/volume1/docker/orpheum/data`, and
   `sudo` goes before `docker`.
   It says what it would do; run it again with `--yes` to restore. Then start the server as in
   step 2.

The Update page then says your compose file pins a version; set it back to `latest` once a newer
release fixes what broke.

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
removed once the first admin is made. It is also in the core's log, in a line that says
`no one can sign in yet` and ends `setupCode=` and the code, three groups of four letters and
digits such as `7kqm-2xhp-9rtw`:

- OpenMediaVault: **Services > Compose > Containers**, `orpheum-core`, **Logs**.
- Synology: **Container Manager → Container**, `orpheum-core`, **Details**, **Log**.
- Elsewhere: `docker logs orpheum-core 2>&1 | grep setupCode`.
- Or ask for it: `docker exec orpheum-core orpheum setup-code --instance /var/lib/orpheum/core`.

No file and no such line, and no one made admin yet? Restart `orpheum-core`: each start writes both
again until an admin exists. If they are gone because the admin was made, sign in as that admin.

Then open `http://<the machine's address>:8440` (or the port `ORPHEUM_HTTP_PORT` names, which the
file and the log line give) in a browser **on your home network**. The machine's address is the one
in your browser's address bar when you open the NAS's own page, without the port after it: open
that page at `192.168.1.30:5000`, and Orpheum is at `http://192.168.1.30:8440`. The page that
opens, **Set up this server**, asks the **Setup code**, a name and a password: these are the name
and password you sign in to Orpheum with, your own, not the NAS's. If it also asks for this
server's address in the house, give the same address. **Create the admin**: you are the admin. An
admin can administer only from the home network; the server learns the machine's address there
from this first browser.

If the server refuses admin because it does not count your browser as at home (a VPN, a second
network), open it on the machine itself instead: `http://127.0.0.1:8441` (`ORPHEUM_THIS_MACHINE_PORT`)
is published on the machine's own loopback alone, and the server counts it as home (D334 §4). From
another computer, `ssh -L 8441:127.0.0.1:8441 <the machine>` and open the same address there.

### Your first library

In the menu, under **Manage**, open **Libraries**, then **Create a library**. Give it a **Name**,
choose **What it holds** (**Films** or **Series**: a library holds one kind, so films and series
are a library each), and under **Folders this library reads** tick your media folder, which shows
by the machine's own name ([above](#one-compose-file)); if your films and series are in folders
inside it, press **Choose a folder inside one…** and pick that folder. **Create and scan**: the
films appear as the scan finds them.

## From a catalogue

[`deploy/catalogue/`](../../deploy/catalogue/README.md) holds the same compose file packaged for
Portainer's app templates and CasaOS's app store. They ask the same values; the address at home is
learned, as above.

## From away from home (optional)

At home, devices reach the server directly. To reach it from away, the server is paired with your
Orpheum account at orpheum.cc, from the **Remote access** section of the server's **System** page,
as an admin at home. Devices away from home then sign in at orpheum.cc with your realm, and
reach the server over WebRTC, with no port to open. Your router's address, on the **System** page under
**The house's network**, lets the server ask the router to forward its HTTPS port, which helps where the router allows it. There is no relay:
a device that can meet the server only through one does not connect, and the System page counts
each time.

### Your account at orpheum.cc

1. Open the invitation link you were sent, which opens `account.orpheum.cc`. **Make your Orpheum
   account** asks your e-mail address, a password of at least 15 characters (this account's own,
   not the server's), and **Choose your realm**: the name your household signs in to your server by, 5 to 32 letters and digits, with
   hyphens between them, such as `annanas`. It is yours alone and cannot be changed, so choose it
   with care. **Make my account**.
2. Open the link in the e-mail it sends you, within 24 hours (**Send a new link** on your account's
   page sends another). An account not verified within 7 days is deleted.

### Connect it to your account

With the server installed and you its admin, on a computer at home:

1. **A key.** Sign in to your account at `https://account.orpheum.cc`, where the invitation took
   you. Under **Your server**, it asks your orpheum.cc password again; then press **Make a key**. The key shows on the page, with the time it works
   until. Leave the page open.
2. **Connect.** In Orpheum, at `http://<the machine's address>:8440`, open the menu, under **Manage**,
   **System**, and find **Remote access**. **The discovery's name** already says `orpheum.cc`: type
   **Your password**, the one you sign in to Orpheum with, and press **Connect**. It says *Connected
   to the Orpheum service* and *Waiting to be claimed.* (Your account's page calls this place
   **System → Discovery**.)
3. **Claim it.** Under **Your account's key**, type or paste the key from step 1, and your Orpheum
   password again, and press **Claim**. It says *Claimed: clients can find this server.* and
   names your realm; your account's page says your server was enrolled. If the key is refused, make
   a new one and try again.

### Watching

- **At home**, open `http://<the machine's address>:8440`.
- **Anywhere**, open `https://orpheum.cc` and sign in with your **Realm**, your **Name** and your
  **Password** on the server. This works for you once the claim is done.

To add someone, in the menu under **Manage**, **Users**, **Invite someone** makes a link for them.
Each person signs in once at home, or through their own link, before signing in by the realm
works for them ([why](setup-problems.md#signing-in-by-realm-says-the-realm-name-or-password-is-wrong-just-after-the-realm-was-given-or-changed)).

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
