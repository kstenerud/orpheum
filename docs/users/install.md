# Installing Orpheum

Orpheum is a media server you run at home. It plays your own films and series to the browsers,
phones and TVs in your household.

**Anyone can install Orpheum and use it at home; no account or invitation is needed.** Only
watching away from home needs one: it goes through the Orpheum service at orpheum.cc, and during
the closed beta the project's owner invites the people who use it, about ten. The beta opens to the
public later.

Something went wrong? [setup-problems.md](setup-problems.md) lists the problems other installs have
met, and their fixes.

## What you need

- **A Linux machine with Docker Engine and its compose plugin**, with Docker run as root, the way
  Docker's own packages install it ("rootful" Docker). Rootless Docker works after
  [one change](setup-problems.md#the-installer-says-rootless-docker-its-userland-proxy-hides-each-devices-address).
  OpenMediaVault and a Synology NAS both work, and this guide has a section for each.
- **A 64-bit processor**: a 64-bit PC (amd64, which `uname -m` shows as `x86_64`) or a 64-bit ARM
  board such as a Raspberry Pi 4 or 5 (arm64, shown as `aarch64`). Every release from `v0.3.2` runs
  on both; `v0.3.0` ran on amd64 only. Nothing 32-bit can run it.
- **Your films and series in folders that machine can read.**

Mac and Windows are not supported yet.

Orpheum runs as two containers from one image:

- the **core**: the server, with its web pages and its API;
- the **node**: the part that reads your media and runs FFmpeg.

A third container, `orpheum-folders`, prepares the data folder at each start and then stops.

Hardware encoding, through Intel Quick Sync or AMD's VAAPI, works on amd64 only. An ARM board
encodes in software.

## Before you start

Run these at a terminal on the machine. Each one says what you should see.

1. `uname -m` prints `x86_64` or `aarch64`. Anything else, such as `armv7l` or `i686`, cannot run
   Orpheum.
2. `docker version` prints a **Client** section and a **Server** section. If it says
   `Cannot connect to the Docker daemon`, Docker is not running: start it with
   `sudo systemctl start docker`. If it says `permission denied`, your own user is not in the
   `docker` group: run Docker's commands as root, with `sudo` in front (`sudo docker version`).
3. `docker compose version` prints `Docker Compose version v2.…` or newer. If it says
   `'compose' is not a docker command`, install the compose plugin (the package
   `docker-compose-plugin`).
4. `docker info --format '{{.SecurityOptions}}'` does not include `rootless`. If it does, make
   [the rootless change](setup-problems.md#the-installer-says-rootless-docker-its-userland-proxy-hides-each-devices-address)
   first.

The project names no minimum version of Docker or Compose.

On OpenMediaVault and on a Synology you may have no terminal. There:

- **OpenMediaVault**: if **Services** has **Compose**, Docker and its compose plugin are installed.
  If it does not, the [OpenMediaVault section](#openmediavault) installs them.
- **Synology**: **Package Center** shows whether Container Manager is installed. The
  [Synology section](#synology) lists the models that can run it.

## Quick start

These are the shortest routes. Each links to its full steps below, which also say what to expect
and what to do when a step fails.

### Quick start: any Linux machine with Docker

At a terminal, as the user who will own the server's data (not root). Change the two folders and the
time zone first, to your own:

```sh
mkdir -p ~/orpheum-compose && cd ~/orpheum-compose
curl -fL -o compose.yaml https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-docker.yaml
cat > .env <<EOF
PATH_TO_APPDATA=/srv/appdata
ORPHEUM_MEDIA=/srv/media
APPUSER_PUID=$(id -u)
APPUSER_PGID=$(id -g)
TIME_ZONE_VALUE=Europe/Berlin
EOF
docker compose up -d
docker logs orpheum-core 2>&1 | grep -E "to finish setting up|no one can sign in"
```

Run the `docker` lines with `sudo` in front if your user is not in the `docker` group. The last
line prints the link that makes you the server's first admin: open it in a browser at home
([the first admin](#the-first-admin)). The full steps are in
[On any machine with Docker](#on-any-machine-with-docker).

### Quick start: OpenMediaVault

1. If **Services** has no **Compose**,
   [install it](#install-compose-only-if-services-has-no-compose).
2. Make an `appdata` shared folder **on a single disk, not on a mergerfs pool**, and note its
   **Absolute Path** ([shared folders](#make-the-shared-folders)).
3. Note the media folder's **Absolute Path**, and the UID of the user the server will run as
   ([the user](#choose-the-user-the-server-runs-as)).
4. **Services > Compose > Files**, **Add**, name it `orpheum`, and paste the whole of
   [`orpheum-omv.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-omv.yaml)
   into **File**.
5. Tick **Show environment file**, and paste your values into **Environment**:

   ```sh
   PATH_TO_APPDATA=/srv/dev-disk-by-uuid-1234abcd-…/appdata
   ORPHEUM_MEDIA=/srv/dev-disk-by-uuid-1234abcd-…/Media
   APPUSER_PUID=1000
   APPUSER_PGID=100
   TIME_ZONE_VALUE=Europe/Berlin
   ```

6. **Save**, select `orpheum`, and press **Up**.
7. Open the link in `orpheum-core`'s log in a browser at home ([the first admin](#the-first-admin)).

The full steps are in [OpenMediaVault](#openmediavault).

### Quick start: Synology

1. Read your media folder's path and make a folder for the server
   ([steps 1 and 2](#synology)).
2. Read your user's uid and gid with `id` over SSH ([step 3](#synology)).
3. **Container Manager → Project → Create**, and paste
   [`orpheum-synology.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-synology.yaml).
   Change these three lines under `# Yours` to your own:

   ```yaml
   media: &media /volume1/video
   user: &user "1026:100"
   time-zone: &timezone Europe/Berlin
   ```

4. Finish the wizard with **Start the project once it is created** ticked.
5. Open the link in `orpheum-core`'s log in a browser at home ([the first admin](#the-first-admin)).

The full steps are in [Synology](#synology).

### Quick start: the install script

At a terminal on the machine, as a user in the `docker` group or as root:

```sh
docker pull ghcr.io/kstenerud/orpheum:latest
docker run --rm ghcr.io/kstenerud/orpheum:latest install-script | sh -s -- --media /path/to/films
```

It asks for the first admin at the terminal. The full steps, and how to read the script before you
run it, are in [The install script](#the-install-script).

### After any of them

If you were invited to the Orpheum service, the whole way is:

1. Open the invitation link and make your account, choosing your household's name
   ([your account](#your-account-at-orpheumcc)). You can do this at any time before step 5.
2. Install the server, by one of the routes above.
3. Make yourself its [first admin](#the-first-admin), by opening the link the server prints.
4. Add a [library](#your-first-library) of your films or series.
5. [Connect the server to your account](#connect-it-to-your-account), with a key from your account's
   page.
6. [Watch](#watching), at home or away.

Without an invitation, do steps 2 to 4 and watch at home.

## One compose file

There are two ways to install Orpheum:

- **A compose file**, described here: you paste it into OpenMediaVault, a Synology's Container
  Manager, Portainer or another compose tool, or save it in a folder, then fill in a few values and
  start it. Each start writes the server's settings from those values. Nothing else is installed on
  the machine.
- **[The install script](#the-install-script)**: one command at a terminal. It finds the machine's
  address, starts the server and asks for the first admin at the terminal.

Each release comes with three compose files, one for each kind of machine:

- on OpenMediaVault, [`orpheum-omv.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-omv.yaml);
- on a Synology, [`orpheum-synology.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-synology.yaml);
- on any other machine with Docker, [`orpheum-docker.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-docker.yaml).

The links always give the latest release's files, which were tested with that release. Each link
downloads the file; open it in a text editor to copy it.

The three files differ in one line, which tells the server how it was installed. The server's
**Update** page uses it to show only the update steps for your kind of machine. The Synology file
also has its values written in it, under `# Yours`
([the Synology compose file](#the-synology-compose-file)); the other two read them from variables.

### The variables

The first two are required: compose refuses to start without them.

| Variable | What it is | Default |
| --- | --- | --- |
| `PATH_TO_APPDATA` | the folder that holds each app's data; Orpheum keeps its own in `orpheum/` inside it. It holds the server's database, so put it on a local disk: not on a network share (NFS, SMB), and not on a mergerfs pool mounted with `cache.files=off`, as OpenMediaVault's mergerfs plugin mounts its pools ([why](setup-problems.md#the-cores-log-says-disk-io-error-5386)) | required |
| `ORPHEUM_MEDIA` | the full path, on the machine, of the folder that holds your films and series. A path that does not exist stops the start, and the error names it | required |
| `ORPHEUM_MEDIA_2`, `ORPHEUM_MEDIA_3`, `ORPHEUM_MEDIA_4` | the full paths of more folders of films and series | none |
| `ORPHEUM_TAG` | the version to run, such as `vX.Y.Z` from the [release list](https://github.com/kstenerud/orpheum/releases) | `latest`: each new release, when you pull it |
| `APPUSER_PUID`, `APPUSER_PGID` | the user and group the server runs as. They own its data, and must be able to read the media. `id -u` and `id -g` print them for your own user | 1000, 100. 100 is the `users` group on OpenMediaVault and on a Synology; elsewhere, set your own |
| `TIME_ZONE_VALUE` | the server's time zone, such as `Europe/Berlin` | UTC |
| `ORPHEUM_HTTP_PORT` | the port a browser at home opens it on: `http://<address>:8440` | 8440 |
| `ORPHEUM_THIS_MACHINE_PORT` | the port a browser on the machine itself opens it on, `http://127.0.0.1:8441`, which counts as at home ([when admin is refused](#the-first-admin)) | 8441 |
| `ORPHEUM_NAME` | the name the media node has in the server's pages | orpheum |
| `RENDER_GID` | the number of the render group, for a GPU ([hardware encoding](#hardware-encoding)) | none |

The media folders, unlike the other variables, may be on a mergerfs pool or a network share.

Inside the containers, each media folder has the same path as on the machine. With
`ORPHEUM_MEDIA=/srv/media`, a library's folder is `/srv/media/Films`. A media variable left unset is
no folder, and the server does not show it.

### What happens at each start

At every start, the core writes its deployment into `PATH_TO_APPDATA/orpheum` from these variables.
To change a value, change the variable and start the server again; its database, people, libraries
and keys are kept. The file `values.toml` there is rewritten from the variables at every start, so
an edit to it is lost.

Every other setting belongs to the server: it is kept in its database and changed on its pages. The
house's network is on the **System** page, under **The house's network**:

- the HTTPS port that devices at home reach it on (8444);
- the WebRTC port (UDP, 8444 too);
- your router's address, which the server uses to ask the router to forward the HTTPS port, so
  that devices away from home can reach it.

A change there takes effect when the server starts again, which the page offers to do. If a port
does not open at the start, the server falls back to 8444 and the page says so: no setting there
can lock you out. The HTTP port is set in the compose file for the same reason.

The machine's address at home is not set anywhere. The server learns it from the first browser that
opens it at home, and the setup page asks for it if the server cannot tell. Give the machine a fixed
address with a reservation in your router.

### On any machine with Docker

1. Make a folder for the compose file, and change to it:
   `mkdir -p ~/orpheum-compose && cd ~/orpheum-compose`.
2. Download [`orpheum-docker.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-docker.yaml)
   into it as `compose.yaml`:
   `curl -fL -o compose.yaml https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-docker.yaml`.
3. Find your user's numbers. `id -u` prints the user's number, often `1000`. `id -g` prints its
   group's number: often `1000` on Debian, Ubuntu and Fedora. The default, `100`, suits only
   OpenMediaVault and Synology, so use the number `id -g` prints.
4. Make a file named `.env` beside `compose.yaml`, with your own folders, numbers and time zone:

   ```sh
   PATH_TO_APPDATA=/srv/appdata
   ORPHEUM_MEDIA=/srv/media
   APPUSER_PUID=1000
   APPUSER_PGID=1000
   TIME_ZONE_VALUE=Europe/Berlin
   ```

5. Start it: `docker compose up -d`. The first time, it downloads the image. Then it waits at
   `orpheum-core Waiting` for up to a minute or two, while the core starts; that is normal. You
   should then see `orpheum-core` and `orpheum-node` as **Healthy** or **Started**.
6. Check that it is running: `docker compose ps` lists `orpheum-core` and `orpheum-node` as
   `healthy` (`health: starting` for their first minute). If `up` ended with
   `orpheum-core is unhealthy`, read the core's log, `docker logs orpheum-core`, and look up its
   last `ERROR` line in
   [setup-problems.md](setup-problems.md#up-says-container-orpheum-core-is-unhealthy).
7. Make the [first admin](#the-first-admin).

The user that runs `docker` here is you, at the terminal. It must be root (or use `sudo`) or be in
the `docker` group. The server itself runs as `APPUSER_PUID`, which needs no Docker access.

To update, follow the server's **Update** page: in the same folder, run
`docker compose pull && docker compose up -d`.

### OpenMediaVault

These steps are for OpenMediaVault 7 or 8, and follow omv-extras' own guides for apps such as
Jellyfin. Everything is done in OpenMediaVault's web interface, except installing omv-extras, which
needs a terminal once.

**First, check:** if **Services** has **Compose**, skip to
[Make the shared folders](#make-the-shared-folders). No terminal is needed at all.

#### Install Compose (only if Services has no Compose)

Compose comes from omv-extras, and omv-extras is installed from a terminal. The web interface's
`admin` user cannot sign in over SSH, so first make a user that can.

1. Open **Services > SSH**. Tick **Enabled** and **Password authentication**, and press **Save**.

   After each change, OpenMediaVault shows a yellow bar asking you to apply it. Press its tick
   (**Apply**) every time, or the change does not take effect.
2. Open **Users > Users** and press **+ (Create)**. Give the user a name and a password (type the
   password yourself; never give it to an AI agent). Under **Groups**, add `_ssh` and `sudo`.
   Press **Save**, then apply.

   This user can also be the one the server runs as, later.
3. On your computer, open a terminal (Windows: **PowerShell**; Mac: **Terminal**) and type
   `ssh <the user>@<the address>`, such as `ssh anna@192.168.1.30`. The address is the one in your
   browser's address bar when you open OpenMediaVault. The first time, it asks whether to trust
   the machine: type `yes`. Then type the password; it does not show as you type.

   You should see a prompt. A warning `Could not chdir to home directory` is harmless:
   OpenMediaVault makes users without a home folder.
4. Type `sudo -v` and the password again. The prompt comes back with no message.

   Do this first because the next command's download progress hides sudo's password prompt, so the
   terminal looks finished while it is waiting for the password.
5. Install omv-extras:

   ```sh
   wget -O - https://github.com/OpenMediaVault-Plugin-Developers/packages/raw/master/install | sudo bash
   ```

   It takes a few minutes and ends at the prompt. (At the machine's own keyboard, signed in as
   root, leave out `sudo`.) Type `exit`.
6. Load OpenMediaVault's page again. **System** now has **omv-extras**.
7. Open **System > omv-extras**, switch on **Docker repo**, and press **Save**.
8. Open **System > Plugins**, select **openmediavault-compose**, and press **Install**. Docker comes
   with it. It also installs openmediavault-sharerootfs: leave that installed.

   **Services** now has **Compose**.

You may turn SSH off again in **Services > SSH** once you no longer need a terminal.

#### Make the shared folders

The server needs two shared folders: `appdata` for its data, and one for your media (`Media` here).
If you already have them, check where `appdata` is (step 1) and go on to step 4.

**Put `appdata` on a single disk, not on a mergerfs pool.** It holds the server's database, which
needs a feature (a shared memory map of a file) that a mergerfs pool refuses when it is mounted with
`cache.files=off`, as OpenMediaVault's mergerfs plugin mounts its pools. The core then stops with
`disk I/O error (5386)`
([setup-problems.md](setup-problems.md#the-cores-log-says-disk-io-error-5386)). A pool mounted with
`cache.files=partial`, `auto-full` or `per-process` works. The media folder may be on a disk or a
pool; for media on another NAS, see [Films on another NAS](#films-on-another-nas).

1. Open **Storage > Shared Folders**. If the **Absolute Path** column is hidden, turn it on from the
   column menu at the top right of the table.

   A folder on a disk has a path like `/srv/dev-disk-by-uuid-1234abcd-…/appdata`; one on a mergerfs
   pool has a path like `/srv/mergerfs/<pool>/appdata`.
2. To make `appdata`, press **+ (Create)**. Name it `appdata`, choose a disk (not a pool) as its
   file system, give its **Relative path** on that disk (`appdata/`), and press **Save**, then
   apply.
3. To make the media folder, if you have none, press **+ (Create)** again. Name it `Media`, choose a
   disk or a pool, give its **Relative path** (`Media/`), and press **Save**, then apply.
4. Write down both **Absolute Path**s exactly, capitals included.

If your films are in a folder inside a shared folder (`Storage/Video`, say), OpenMediaVault shows
only the shared folder's path. Find the inner folder's path in one of two ways, capitals included:

- make a shared folder for the inner one too, with that folder's **Relative path**
  (`Storage/Video/`): its **Absolute Path** is then the exact path; or
- over SSH, list what the shared folder holds: `ls /srv/mergerfs/<pool>/Storage` shows `Video` as it
  is spelled, so the path is `/srv/mergerfs/<pool>/Storage/Video`.

#### Tell Compose where its files go

1. Open **Services > Compose > Settings**.
2. Under **Compose Files**, choose `appdata` as the **Shared folder**, and press **Save**, then
   apply.

#### Choose the user the server runs as

Any existing user except `root` and `admin` works, your own included, even one in the `sudo` group.
The server sees only its own data folder and your media, so what that user may do elsewhere on the
machine gives the server nothing.

If you would rather keep the server's data apart from everything else that user signs in to or
runs, make a user for apps alone, as omv-extras' guide does: **Users > Users**, **+ (Create)**, name
`appuser`, a password (OpenMediaVault asks for one for every user; type it yourself), **Save**, then
apply.

**Never add the user the server runs as to the `docker` group.**

1. Find its UID. Open **Users > Users**. The **UID** column is hidden at first: turn it on from the
   column menu at the top right of the table. (Over SSH, `id <user>` prints it too, as
   `uid=1000(anna) gid=100(users) …` for `id anna`.)
2. Its GID is 100. That is the group `users`, which OpenMediaVault puts every user it makes in.
3. Let it read the media folder. Open **Storage > Shared Folders**, select `Media`, and press
   **Permissions**. Give the user *Read-only* or *Read/Write*, and press **Save**, then apply.
   (This does not apply to media on another NAS: see [Films on another NAS](#films-on-another-nas).)

#### Check the ports

OpenMediaVault's own web interface uses port 80, and 443 when its HTTPS is on; omv-extras' guide
moves it to 8443 to free them. Orpheum uses none of these: its plain page is on 8440 and its HTTPS
on 8444. If something else uses 8440, add `ORPHEUM_HTTP_PORT=` with a free port to your values
below. The HTTPS port is changed later, on the server's **System** page.

#### Add the compose file

1. Gather your values, your own in place of the examples:

   ```sh
   PATH_TO_APPDATA=/srv/dev-disk-by-uuid-1234abcd-…/appdata
   ORPHEUM_MEDIA=/srv/dev-disk-by-uuid-1234abcd-…/Media
   APPUSER_PUID=1000
   APPUSER_PGID=100
   TIME_ZONE_VALUE=Europe/Berlin
   ```

   `PATH_TO_APPDATA` is the `appdata` folder's **Absolute Path**, and `ORPHEUM_MEDIA` the media
   folder's, capitals included. `APPUSER_PUID` is the user's UID.
2. Open **Services > Compose > Files** and press **Add** (not *Add from example*). Some versions
   open a small menu first: choose **Add** in it. A form opens.
3. In **Name**, type `orpheum`.
4. Open [`orpheum-omv.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-omv.yaml)
   in a text editor, copy all of it, and paste it into **File**. Paste it; do not type it: the
   editor indents as you type, which breaks the file.
5. Tick **Show environment file**. An **Environment** field appears below **File**.

   Do not skip this. Without it, **Up** fails with
   `required variable PATH_TO_APPDATA is missing a value`.
6. Paste your values from step 1 into **Environment**. They belong to this file alone and change no
   other app.
7. Press **Save**.

Instead of the **Environment** field, the first four values may come from OpenMediaVault's global
environment file (**Services > Compose > Files**, **Edit global environment file**), which every app
reads. If omv-extras' guide set them up there, leave them out of **Environment**. Put each value in
one place or the other, not both. If the global file is empty, leave it empty and use
**Environment**.

#### Start it

1. Select the `orpheum` file and press **Up**.

   The first time, it downloads the image, which takes a minute or so. Then the output stops at
   `Container orpheum-core Waiting` for up to a minute or two, while the core starts. That is
   normal; wait for it to finish.
2. Open **Services > Compose > Containers**. You should see three containers:
   - `orpheum-folders`, which prepared the data folder and stopped: *exited* is right;
   - `orpheum-core` and `orpheum-node`, both *healthy*.

   If **Up** ended with `orpheum-core is unhealthy` or `dependency failed to start`, select
   `orpheum-core` and press **Logs**. Its last `ERROR` line says why; look it up in
   [setup-problems.md](setup-problems.md#up-says-container-orpheum-core-is-unhealthy).
3. Make the [first admin](#the-first-admin). The link is in the core's log: select `orpheum-core`
   and press **Logs**. (The link is also in a file in `appdata`, but only the server's user can read
   it, so the log is easier.)

#### Films on another NAS

If your films are on another NAS, shared over NFS or SMB and mounted on OpenMediaVault by
omv-extras' **remotemount** plugin, the steps above hold, with these differences:

- **Keep `appdata` on a local disk.** It cannot be on the network share: `orpheum-folders` cannot
  change the owner of a folder there, and stops with an error.
- **Mount the share so that its reads give up when the NAS is away.** For NFS, use the options
  `vers=3,soft,timeo=100,retrans=3,nolock`, and `ro` if the server only reads it. A share mounted
  the default way (`hard`, or NFS 4.1 and later) makes the server wait for good whenever the other
  NAS is off or rebooting ([why](setup-problems.md#films-on-a-nas-stop-playing-when-the-nas-is-off-or-system-warns-mounted-to-wait-for-its-server-for-good)).
- **`ORPHEUM_MEDIA` is the mount's path**, under `/srv/remotemount/`, such as
  `/srv/remotemount/<name>/Films`.
- **Shared Folders' Permissions do not decide who can read it.** The mount and the other NAS's own
  permissions do. Check that the server's user can read it, over SSH:
  `sudo -u <the user> ls /srv/remotemount/<name>`. It should list your folders, not
  `Permission denied`.

#### Updating on OpenMediaVault

Follow the server's **Update** page, in the admin's menu:

1. Select the `orpheum` file and press **Pull**, then **Up**.
2. If the update brings a newer compose file, the Update page gives it to you with your values in
   it. Keep a copy of your old file, then edit the `orpheum` file, paste the new one over it, press
   **Save**, and press **Up**.

To stop the server, press **Down**. Its data stays in `appdata/orpheum`.

### Synology

These steps are for DSM 7.2 or newer, with Container Manager. The paths below are examples: a
Synology names each folder after the volume it is on and the name you gave it, so read your own
before you type them.

**A DS218, DS218play, DS118, DS418 or RS819** (the models with a Realtek RTD1296 processor) can run
Orpheum, but Synology does not offer Container Manager for them. A community script,
[ContainerManager_for_all_armv8](https://github.com/007revad/ContainerManager_for_all_armv8),
installs Synology's own Container Manager package on them. It is unofficial: neither Synology nor
this project supports it, and this project has not tested it. It needs **DSM 7.2 or newer**: if
**Control Panel → Info Center** shows an older DSM, update it first (**Control Panel → Update &
Restore**). Follow the script's own page to run it, then open **Package Center**, where Container
Manager is now installed, and go on with step 1 below. The 32-bit models, such as the DS218j and
DS216j, cannot run Orpheum at all.

Such a model is slow for video, so know this before you start. Its processor can send a film
unchanged to a device that plays it unchanged, at home, which covers most films on most devices.
What it cannot do is re-encode a film while it plays. A device that cannot play the film's own
format, or a link too slow for the film's own size (often the case away from home), needs a lighter
copy made on the fly, and this processor cannot make one as fast as the film plays. Such a playback
stops to wait, again and again. Choosing a lower quality in the player does not help, because a
lower quality is a re-encode too.

Its system is older too (Linux 4.4), and Orpheum runs on it with one protection fewer. The
**System** page says so for the media machine: "FFmpeg runs without part of its sandbox, which this
machine's kernel does not offer", and that Landlock, which limits the files FFmpeg may open, needs
Linux 5.13 or newer. DSM cannot provide it, so leave it: films play as they would with it, and the
rest of the sandbox still holds.

1. **Your media folder's path.** In **File Station**, right-click the shared folder that holds your
   films and series and choose **Properties**. **Location** is its path, such as `/volume1/video`
   or `/volume2/Media`. Write it down exactly, capitals included.
2. **A folder for the server.** In **File Station**, make a shared folder for apps if you have none
   (`docker` is usual). In it, make a folder for Orpheum named `orpheum` (**Create → Create
   folder**). Read its **Location** as in step 1, such as `/volume1/docker/orpheum`. The server
   keeps its data in a folder inside it, `data`, which it makes itself.
3. **Your user's numbers.** The server runs as your own Synology user, so it needs that user's two
   numbers, its uid and its gid:
   1. Turn on SSH: **Control Panel → Terminal & SNMP**, tick **Enable SSH service**, and press
      **Apply**.
   2. On your computer, open a terminal (Windows: **PowerShell**; Mac: **Terminal**) and type
      `ssh <your user>@<the Synology's address>`, such as `ssh anna@192.168.1.30`. The address is
      the one in your browser's address bar when you use DSM, without the `:5000` after it. The user
      must be an administrator, as the one you set the Synology up with is. The first time, it asks
      whether to trust the machine: type `yes`. Then type the password you sign in to DSM with; it
      does not show as you type.
   3. Type `id` and press Enter. It shows something like:

      ```text
      uid=1026(<your user>) gid=100(users) groups=100(users),101(administrators)
      ```

      The number after `uid=` is the uid, here `1026`; the number after `gid=` is the gid, here
      `100`. On most Synologys the first user made has exactly these, 1026 and 100, but use the ones
      yours shows.
   4. Type `exit`. You may turn SSH off again.

   This user must be able to read the media folder: **Control Panel → Shared Folder**, select it,
   **Edit → Permissions**, and give the user *Read only* or *Read/Write*.
4. **The project.**
   1. Open **Container Manager → Project → Create**.
   2. Name it `orpheum`. For **Path**, choose the `orpheum` folder from step 2. For **Source**,
      choose **Create docker-compose.yml**.
   3. Paste [the Synology compose file](#the-synology-compose-file) into the editor.
   4. Near its top, under `# Yours`, change three lines to your own, in the editor:

      ```yaml
      media: &media /volume1/video
      user: &user "1026:100"
      time-zone: &timezone Europe/Berlin
      ```

      - after `&media `, the media folder from step 1;
      - between the quotes after `&user `, the uid from step 3, a colon, then the gid: `"1026:100"`;
      - after `&timezone `, your time zone, such as `Europe/London` or `America/New_York`.

      Change only what follows the `&` word; leave the rest of each line as it is.
   5. Press **Next** past the web portal page. On the last page, leave **Start the project once it
      is created** ticked, and press **Done**.

   The first time, it downloads the image, which takes a few minutes. Then, in **Container Manager
   → Container**, `orpheum-folders` has stopped (that is right: it only prepares the data folder),
   and `orpheum-core` and `orpheum-node` are running.
5. **Keep a copy of the whole file** somewhere of your own. When an update brings a newer compose
   file, the server's **Update** page gives it to you, and it keeps only the lines under `# Yours`.
6. Make the [first admin](#the-first-admin). The link to open is in the log: **Container Manager →
   Container**, select `orpheum-core`, **Details**, then the **Log** tab. It is also in
   `SETUP-CODE.txt` in the `data` folder (`/volume1/docker/orpheum/data`), which File Station can
   download for you to open.

To change a value later: **Project**, select `orpheum`, **Action → Stop**, then **YAML
Configurations**. Change the line, press **Save**, then **Action → Start**.

#### Updating on a Synology

Follow the server's **Update** page, in the admin's menu:

1. **Container Manager → Project**, select `orpheum`, press **Stop**, then **Build**.
2. If the Update page still says the server runs the old version, Container Manager kept the image
   it had. Under **Image**, select the `orpheum` image, press **Update**, then **Build** the project
   again.
3. If the update brings a newer compose file, the Update page gives it to you with your values from
   `# Yours` filled in. Keep a copy of your old file. Then **Project**, stop `orpheum`, open its
   **YAML Configurations**, paste the new file over it, press **Save**, and press **Build**.

#### The Synology compose file

The file is [`orpheum-synology.yaml`](https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-synology.yaml).
It is the [compose file](#one-compose-file) with the values a Synology needs written under
`# Yours`, so it needs no `.env` file, and it keeps its data in the project's own folder. Open the
downloaded file in a text editor to copy it. The ports are under `# Yours` too; every other setting
in the [variables table](#the-variables) keeps its default.

If DSM's firewall is on (**Control Panel → Security → Firewall**), allow port 8440 (TCP) and the
HTTPS port, 8444 unless you changed it on the **System** page (TCP and UDP). Otherwise the server's
page does not open from your other devices.

Other Synology models whose DSM runs a Linux older than 5.13 lack Landlock too, and the System page
says so in the same way.

### If an update broke it

Go back to the version before, which your data still works with:

1. **Pin the version before.** The Update page, or the
   [release list](https://github.com/kstenerud/orpheum/releases) on GitHub, names it, such as
   `vX.Y.Z`.
   - On OpenMediaVault, add `ORPHEUM_TAG=vX.Y.Z` to the file's **Environment**.
   - On any other machine with Docker, add it to the `.env` file.
   - On a Synology, in the project's **YAML Configurations**, write `vX.Y.Z` in place of each
     `${ORPHEUM_TAG:-latest}`: the `image:` lines and the line after `--image-tag`.
2. **Start it again**: **Pull** and **Up** on OpenMediaVault, **Stop** then **Build** on a
   Synology, `docker compose pull && docker compose up -d` elsewhere.
3. **If it does not start** and the core's log says its database needs a newer release, the update
   had already moved the database on. Before it did, the server kept a copy of it,
   `pre-migrate-v<n>.sqlite`, in `orpheum/core/backups/` in the app data folder
   (`data/core/backups/` in the project's folder on a Synology). Stop the server (**Down**, or
   **Stop**). Then restore the copy from a terminal on the machine, as an administrator, with the
   version before, your app data folder and the user the server runs as:

   ```sh
   docker run --rm --user 1000:100 -v /srv/appdata/orpheum:/var/lib/orpheum \
     -v /srv/appdata/orpheum/run:/run/orpheum ghcr.io/kstenerud/orpheum:vX.Y.Z \
     restore --instance /var/lib/orpheum/core --from /var/lib/orpheum/core/backups/pre-migrate-v<n>.sqlite
   ```

   On a Synology the folder is the project's `data` folder, `/volume1/docker/orpheum/data` (and
   `/volume1/docker/orpheum/data/run` for the second), and `sudo` goes before `docker`. The command
   says what it would do; run it again with `--yes` to restore. Then start the server as in step 2.

The Update page then says your compose file pins a version. Set it back to `latest` once a newer
release fixes what broke.

### Hardware encoding

With an Intel or AMD GPU, the node encodes on it:

1. In the node's part of the compose file, uncomment `devices` and `group_add`.
2. Set `RENDER_GID` to the number of the group that owns the GPU's render node.
   `ls -n /dev/dri/renderD128` shows it (often 105 or 106 on Debian). On OpenMediaVault,
   **Users > Groups** lists `render` with its GID.

The node runs as `APPUSER_PUID`, so it needs that group to open the GPU. Adding the app user to
`render` and `video` in **Users > Users**, as omv-extras' Jellyfin guide does, is not enough on its
own, because a container gets only the groups it is given. Without a GPU, films that need preparing
are encoded in software.

## The first admin

On its first start, and on every start until someone is made admin, the core prints a **link** that
makes the first admin. It prints one for each of the machine's addresses on your home network, such
as `http://192.168.1.30:8440/#code=7kqm-2xhp-9rtw`.

1. Find the link. It is in:
   - the core's log, on a line of its own that starts
     `to finish setting up, open this link in a browser at home:`, just after a warning that says
     `no one can sign in yet`.
     - OpenMediaVault: **Services > Compose > Containers**, `orpheum-core`, **Logs**.
     - Synology: **Container Manager → Container**, `orpheum-core`, **Details**, **Log**.
     - Elsewhere:
       `docker logs orpheum-core 2>&1 | grep -E "to finish setting up|no one can sign in"`.
   - the file **`SETUP-CODE.txt`** in `PATH_TO_APPDATA/orpheum` (on OpenMediaVault, the `orpheum`
     folder in the `appdata` shared folder). Only the server's user can read it, and it is removed
     once the first admin is made.
   - the output of `docker exec orpheum-core orpheum setup-code --instance /var/lib/orpheum/core`,
     which prints the code and, beneath it, the links.
2. Open the link in a browser **on your home network**. A page titled **Set up this server** opens,
   with the one-time setup code already filled in.
3. Type a name and a password. These are what you sign in to Orpheum with: your own, not the NAS's.
   If the page also asks for this server's address in the house, give the address you opened it at.
4. Press **Create the admin**. You are now the admin.

An admin can administer the server only from the home network. The server learns the machine's
address there from this first browser.

The link works until someone is made admin. If there is no such line in the log and no file, and no
one is admin yet, restart `orpheum-core`: each start writes them again until an admin exists. If
they are gone because the admin was made, sign in as that admin.

The link carries the code after a `#`, which a browser never sends to the server, so no log of the
server or of a proxy holds it. The code is also printed on its own: three groups of four letters and
digits, such as `7kqm-2xhp-9rtw`. Use it when the link does not work: on a machine that shows no
address on the home network, or one you reach by a different address. Open
`http://<the machine's address>:8440` (or the port `ORPHEUM_HTTP_PORT` names) in a browser on your
home network, and type the code on the page.

The machine's address is the one in your browser's address bar when you open the NAS's own page,
without the port after it. If you open that page at `192.168.1.30:5000`, Orpheum is at
`http://192.168.1.30:8440`.

**If the server refuses admin** because it does not count your browser as at home (a VPN, a second
network), open it on the machine itself instead: `http://127.0.0.1:8441`
(`ORPHEUM_THIS_MACHINE_PORT`). That port is open only on the machine's own loopback, and the server
counts it as home. From another computer, run `ssh -L 8441:127.0.0.1:8441 <the machine>` and open
the same address there.

### Your first library

Once the first admin is made, Orpheum takes you through three pages. You can skip each one.

1. **Add your videos.** It lists the folders inside your media folder and offers to make a library
   of each. Tick the ones you want and check the kind it guessed: **Films**, **Series** or
   **Other**. A folder named Movies, Films or Film is guessed as Films, and one named TV, TV Shows,
   Shows or Series as Series. One press makes each ticked library. A folder the server refuses is
   named with the reason, and the others are made. A library holds one kind, so films and series
   are a library each. Below the offer you can instead make one library of a whole folder.
2. **Posters, descriptions and cast** come from TMDB, which needs a free key. Make an account at
   themoviedb.org, open its **Settings**, then **API**, request a Developer key, and paste the **API
   Read Access Token** on the page. If you skip it, nothing is identified until you add the key
   under **Manage**, **Settings**. The page does not appear when a key is set already.
3. **Watching away from home** says what connecting a discovery gives (watching away from home,
   signing in on a TV by a short code, and the household's other people joining by a link), and
   that the server works fully at home without one. **Connect later from System** leads to the page
   that connects it.

Later, **Manage**, **Libraries**, **Create a library** makes more, with the same offer above its
form. The scan finds the films as it goes, and they appear in the libraries as it finds them.

## From a catalogue

[`deploy/catalogue/`](../../deploy/catalogue/README.md) holds the same compose file packaged for
Portainer's app templates and CasaOS's app store. They ask for the same values; the server learns
its address at home, as above.

## From away from home (optional)

At home, devices reach the server directly. To reach it from away, you pair the server with your
Orpheum account at orpheum.cc, from the **Remote access** section of the server's **System** page,
as an admin at home. Devices away from home then sign in at orpheum.cc with your household's name,
and reach the server over WebRTC, with no port to open.

An account needs an invitation. During the closed beta, the project's owner sends them, to about
ten people; the beta opens to the public later.

Your router's address, on the **System** page under **The house's network**, lets the server ask the
router to forward its HTTPS port, which helps where the router allows it. There is no relay: a
device that can reach the server only through one does not connect, and the System page counts
each time this happens.

### Your account at orpheum.cc

1. Open the invitation link you were sent. It opens `account.orpheum.cc`, at **Make your Orpheum
   account**.
2. Type your e-mail address and a password of at least 15 characters. This password is the
   account's own, not the server's.
3. Under **Choose your realm**, type your household's name: the name your household signs in to
   your server by. It is 5 to 32 letters and digits, with hyphens between them, such as `annanas`.
   It is yours alone and cannot be changed, so choose it with care.
4. Press **Make my account**.
5. Open the link in the e-mail the service sends you, within 24 hours. (**Send a new link** on your
   account's page sends another.) An account not verified within 7 days is deleted.

An invitation's link lasts 14 days and makes one account. A link that has lapsed or was used says
"this invitation has expired or was used; ask for a new one". Ask whoever invited you: they can send
another link for the same invitation.

### Connect it to your account

With the server installed and you its admin, on a computer at home:

1. **Make a key.** Sign in to your account at `https://account.orpheum.cc`, where the invitation
   took you. Under **Your server**, type your orpheum.cc password again, then press **Make a key**.
   The key shows on the page, with the time it works until. Leave the page open.
2. **Connect.** In Orpheum, at `http://<the machine's address>:8440`, open the menu, then under
   **Manage**, **System**, and find **Remote access**. **The discovery's name** already says
   `orpheum.cc`. Type **Your password**, the one you sign in to Orpheum with, and press
   **Connect**. It says *Connected to the Orpheum service* and *Waiting to be claimed.* (Your
   account's page calls this place **System → Discovery**.)
3. **Claim it.** Under **Your account's key**, type or paste the key from step 1, type your Orpheum
   password again, and press **Claim**. It says *Claimed: clients can find this server.* and names
   your household. Your account's page says your server was enrolled. If the key is refused, make a
   new one and try again.

### Watching

Any device with a current browser can watch: **Chrome or Edge 120, Firefox 140, or Safari 17.1**
(on macOS, iPhone and iPad) or newer. An older one says the browser is too old
([the page is blank](setup-problems.md#the-page-is-blank-or-says-this-browser-is-too-old-for-orpheum)).

- **At home**, open `http://<the machine's address>:8440`. The page notes that plain http turns
  some features off; **Dismiss** hides the note, and the browser remembers it.
- **Anywhere**, open `https://orpheum.cc` and sign in with your **Household**, your **Username** and
  your **Password** on the server. This works once the claim is done.
- **On a TV**, where a password is hard to type, open the same address and choose **Get a code**.
  Scan the code it shows with a phone that is signed in, and tap **Activate**. Or enter the code on
  a signed-in phone or computer under **Your account**, **Activate a device**.

To add someone, open the menu, then under **Manage**, **Users**, press **Invite someone**: it makes
a link for them. Each person signs in once at home, or through their own link, before signing in by
the household works for them
([why](setup-problems.md#signing-in-by-household-says-the-household-name-or-password-is-wrong-just-after-the-household-was-given-or-changed)).

## The install script

The install script is the other way to install, from a terminal on the machine. Run it as a user in
the `docker` group, or as root: that is the user you are signed in as at the terminal.

1. Download the latest image:

   ```sh
   docker pull ghcr.io/kstenerud/orpheum:latest
   ```

2. If you want to read the script before running it, save it to a file and open it:

   ```sh
   docker run --rm ghcr.io/kstenerud/orpheum:latest install-script > orpheum-install.sh
   less orpheum-install.sh
   ```

   Then run it with your media folders, one `--media` for each:

   ```sh
   sh orpheum-install.sh --media /path/to/films --media /path/to/shows
   ```

3. Or run it straight from the image:

   ```sh
   docker run --rm ghcr.io/kstenerud/orpheum:latest install-script \
     | sh -s -- --media /path/to/films --media /path/to/shows
   ```

To install one version instead of the latest, use its tag, such as `vX.Y.Z` from the
[release list](https://github.com/kstenerud/orpheum/releases), in place of `latest` in both
commands.

The script:

- writes the deployment into `~/orpheum` (`--dir` names another folder);
- finds the machine's address and its router itself;
- starts the server;
- reports what the machine has (Landlock, a GPU, IPv6);
- asks at the terminal for the first admin and, optionally, your account's key. With no terminal,
  it prints the setup code instead.

Run again, with a newer image or tag, it updates, keeping everything. Media folders are seen inside
the containers at their own paths, so a library names the same path the machine does.
