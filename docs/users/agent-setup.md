# Setting Up Orpheum: Help for a Person's AI Agent

The facts first:

- Orpheum is a self-hosted media server. It runs in Docker (Docker Engine and its compose plugin)
  on a 64-bit machine, amd64 or arm64.
- Install guide: https://github.com/kstenerud/orpheum/blob/main/docs/users/install.md
- Its exact text: https://raw.githubusercontent.com/kstenerud/orpheum/main/docs/users/install.md
- Compose file for OpenMediaVault:
  https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-omv.yaml
- Compose file for a Synology:
  https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-synology.yaml
- Compose file for any other machine with Docker:
  https://github.com/kstenerud/orpheum/releases/latest/download/orpheum-docker.yaml
- Never ask for a password: the person types it themselves.
- An invitation link is a secret: never paste one into a chat, a document or an issue that anyone
  else can read.

This page is for an AI agent helping a person install Orpheum, a server that plays their own films
and series to their browsers, phones and televisions. They may have said no more than "set up
Orpheum, which is at orpheum.cc". Ask them the questions below, then follow the section of the
[install guide](install.md) each answer leads to. The guide holds every step; this page only says
which of them are the person's. Its [Before you start](install.md#before-you-start) checks are worth
running with the person first.

Anyone may install Orpheum and use it at home, with no account and no invitation. Only watching
away from home needs an account at orpheum.cc, and during the closed beta the project's owner
invites the people who get one, about ten; the beta opens to the public later. Mac and Windows are
not supported yet.

## Your rules

- **Never ask for a password**: not the one they sign in to Orpheum with, not their orpheum.cc
  account's, not the NAS's or the machine's. Where a step needs one, the person types it
  themselves, on the page or at the terminal that asks for it.
- **An invitation link is a secret.** Whoever opens it gets what it was made for: an orpheum.cc
  invitation makes an account, and a link from a server's **Invite someone** lets a person in.
  Never paste one into a chat, a document or an issue that anyone else can read. If the person
  gives you one, use it only to tell them to open it themselves.
- **Follow the guide's steps in order, with every condition in them.** A summary that drops a
  condition has cost installs before: the pool rule for `appdata`, the Compose shared folder made to
  look optional, a TMDB step that no longer exists (the OpenMediaVault mock install, 2026-11-20).
  Where a step branches (a pool or none, Compose installed or not, media here or on another NAS),
  ask which applies and give that branch whole.
- **On OpenMediaVault, before the person presses Up, confirm with them** that:
  - **Services > Compose > Settings** has a **Shared folder** under **Compose Files**, saved and
    applied;
  - `appdata`'s shared folder is on a **File system** that starts with `/dev/`, not a pool;
  - for media on another NAS, the mount's **Options** are set as
    [films on another NAS](install.md#films-on-another-nas) gives them, and
    `sudo -u <the user> ls <the media folder>` lists the folders.

## What to ask

1. **The machine, and its system.** Which machine will run the server, and what runs on it:
   - **OpenMediaVault**: [OpenMediaVault](install.md#openmediavault), whose opening installs Docker
     and the compose plugin when the machine has neither. The compose file's link redirects to
     release-assets.githubusercontent.com: if your tool follows redirects, as `curl -L` does, fetch
     the file yourself and show it to the person; if it does not, ask them to open the link in their
     own browser. Either way the person pastes the whole file into OpenMediaVault themselves, as
     [Add the compose file](install.md#add-the-compose-file) says.
   - **A NAS**: its make, and its model. A Synology follows [Synology](install.md#synology), whose
     model matters: some models need an unofficial script first, and the 32-bit ones cannot run it
     at all. Before any step, ask a Synology's owner for what
     [check your Synology first](install.md#check-your-synology-first) reads:
     - the **Model name** and **DSM version**, from **Control Panel → Info Center**, the
       **General** tab (DSM older than 7.2 is updated first);
     - whether **Package Center** finds **Container Manager**, and whether it is installed;
     - whether they can sign in to DSM as an administrator and turn on SSH (**Control Panel →
       Terminal & SNMP**), which step 3 needs, and the unofficial script too;
     - the **Location** of the shared folder that holds their films, from File Station's
       **Properties**.

     A NAS of another make, or Portainer or CasaOS, follows
     [one compose file](install.md#one-compose-file) or [a catalogue](install.md#from-a-catalogue).
   - **Docker.** The server needs Docker Engine and its compose plugin, run as root as Docker's own
     packages install it ([the guide's opening](install.md)). With them, a machine follows
     [any machine with Docker](install.md#on-any-machine-with-docker), or, from a terminal on it,
     [the install script](install.md#the-install-script). Rootless Docker needs
     [a change first](setup-problems.md#the-installer-says-rootless-docker-its-userland-proxy-hides-each-devices-address).
   - **x86 or arm64**: a 64-bit PC (amd64, `x86_64`) or a 64-bit ARM board such as a Raspberry Pi
     4 or 5 (arm64, `aarch64`); `uname -m` on the machine says which. Nothing 32-bit runs it.
   - **A Mac or a Windows PC** is not supported yet: say so, and offer a Linux machine or a NAS
     instead.
2. **Where the films and series are, and how they are arranged.**
   - The folder or folders that hold them, as the machine names them, capitals included
     (`/srv/media`, `/volume1/video`): they are `ORPHEUM_MEDIA` and up to three more, in
     [one compose file](install.md#one-compose-file). The user the server runs as must be able to
     read them.
   - Where the server's own data goes (`PATH_TO_APPDATA`): a local disk. Not a network share, and
     not a mergerfs pool mounted `cache.files=off`, as OpenMediaVault's plugin mounts its pools:
     the database cannot open there ([the variables](install.md#the-variables)). On
     OpenMediaVault, that is a shared folder whose **File system** starts with `/dev/`
     ([the shared folders](install.md#make-the-shared-folders)). The media may be on either.
   - Whether the films are on another NAS: on OpenMediaVault, follow
     [films on another NAS](install.md#films-on-another-nas) too.
   - Whether films and series are apart: a library holds one kind, so films are one library and
     series another, and a library may read a folder inside the media folder
     ([your first library](install.md#your-first-library)).
3. **A graphics card, for hardware encoding.** An Intel or AMD GPU on a 64-bit PC encodes on the
   card ([hardware encoding](install.md#hardware-encoding)); an NVIDIA card needs NVIDIA's toolkit
   ([an NVIDIA GPU](setup-problems.md#the-installer-says-an-nvidia-gpu-yes-but-docker-cannot-hand-it-to-a-container));
   an ARM board, or a machine with none, encodes on the processor. A slow NAS cannot re-encode a
   film as fast as it plays, as [Synology](install.md#synology) explains for its older models.
4. **Watching away from home.** At home no account is needed. Away from home needs an Orpheum
   account at orpheum.cc, which is made from an invitation; during the closed beta the project's
   owner sends them. Ask whether they have one, and when it came: its link lasts 14 days and makes
   one account, so one older than that needs a new link from whoever sent it, before anything else. Making the
   account first, while the link is good, is safest
   ([from away from home](install.md#from-away-from-home-optional),
   [the account](install.md#your-account-at-orpheumcc),
   [connecting the server to it](install.md#connect-it-to-your-account)). Without one, set up for
   home alone; the guide marks the rest optional. A server installed again on an account that
   held one before takes its place
   ([installing again](install.md#installing-again-on-the-same-account)).

## Then

The guide's own order holds for everyone: install the server, make the
[first admin](install.md#the-first-admin) from a browser at home, add a
[first library](install.md#your-first-library), connect it to the account if they have one, and
[watch](install.md#watching). When a step fails, [setup-problems.md](setup-problems.md) lists what
has gone wrong before, and the fix.
