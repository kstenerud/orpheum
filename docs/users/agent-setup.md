# Setting Up Orpheum: Help for a Person's AI Agent

This page is for an AI agent helping a person install Orpheum, a server that plays their own films
and series to their browsers, phones and televisions. They may have said no more than "set up
Orpheum, which is at orpheum.cc". Ask them the questions below, then follow the section of the
[install guide](install.md) each answer leads to. The guide holds every step; this page only says
which of them are the person's.

## Your rules

- **Never ask for a password**: not the one they sign in to Orpheum with, not their orpheum.cc
  account's, not the NAS's or the machine's. Where a step needs one, the person types it
  themselves, on the page or at the terminal that asks for it.
- **An invitation link is a secret.** Whoever opens it gets what it was made for: an orpheum.cc
  invitation makes an account, and a link from a server's **Invite someone** lets a person in.
  Never paste one into a chat, a document or an issue that anyone else can read. If the person
  gives you one, use it only to tell them to open it themselves.

## What to ask

1. **The machine, and its system.** Which machine will run the server, and what runs on it:
   - **OpenMediaVault**: [OpenMediaVault](install.md#openmediavault).
   - **A NAS**: its make, and its model. A Synology follows [Synology](install.md#synology), whose
     model matters: some models need an unofficial script first, and the 32-bit ones cannot run it
     at all. A NAS of another make, or Portainer or CasaOS, follows
     [one compose file](install.md#one-compose-file) or [a catalogue](install.md#from-a-catalogue).
   - **Docker.** The server needs Docker Engine and its compose plugin, run as root as Docker's own
     packages install it ([the guide's opening](install.md)). With them, a machine follows
     [any machine with Docker](install.md#on-any-machine-with-docker), or, from a terminal on it,
     [the install script](install.md#the-install-script). Rootless Docker needs
     [a change first](setup-problems.md#the-installer-says-rootless-docker-its-userland-proxy-hides-each-devices-address).
   - **x86 or arm64**: a 64-bit PC (amd64, `x86_64`) or a 64-bit ARM board such as a Raspberry Pi
     4 or 5 (arm64, `aarch64`); `uname -m` on the machine says which. Nothing 32-bit runs it.
2. **Where the films and series are, and how they are arranged.**
   - The folder or folders that hold them, as the machine names them, capitals included
     (`/srv/media`, `/volume1/video`): they are `ORPHEUM_MEDIA` and up to three more, in
     [one compose file](install.md#one-compose-file). The user the server runs as must be able to
     read them.
   - Whether films and series are apart: a library holds one kind, so films are one library and
     series another, and a library may read a folder inside the media folder
     ([your first library](install.md#your-first-library)).
3. **A graphics card, for hardware encoding.** An Intel or AMD GPU on a 64-bit PC encodes on the
   card ([hardware encoding](install.md#hardware-encoding)); an NVIDIA card needs NVIDIA's toolkit
   ([an NVIDIA GPU](setup-problems.md#the-installer-says-an-nvidia-gpu-yes-but-docker-cannot-hand-it-to-a-container));
   an ARM board, or a machine with none, encodes on the processor. A slow NAS cannot re-encode a
   film as fast as it plays, as [Synology](install.md#synology) explains for its older models.
4. **Watching away from home.** At home no account is needed. Away from home needs an Orpheum
   account at orpheum.cc, which is made from an invitation: ask whether they have one
   ([from away from home](install.md#from-away-from-home-optional),
   [the account](install.md#your-account-at-orpheumcc),
   [connecting the server to it](install.md#connect-it-to-your-account)). Without one, set up for
   home alone; the guide marks the rest optional.

## Then

The guide's own order holds for everyone: install the server, make the
[first admin](install.md#the-first-admin) from a browser at home, add a
[first library](install.md#your-first-library), connect it to the account if they have one, and
[watch](install.md#watching). When a step fails, [setup-problems.md](setup-problems.md) lists what
has gone wrong before, and the fix.
