# Security updates reaching households (draft for the owner, 2026-10-31)

**Status:** a design draft. Nothing here is decided or built. The owner agreed the aim, "design
first" (research/security-week-2026-10-30.md §7, 2026-10-31); the questions in §5 are the owner's.

## 1. Why

The week's attacks on self-hosted servers (Plex2Shell, a NAS maker's ransomware, PHP2Shell) all hit
machines that had missed fixes released weeks or months before. Exploits now arrive within days of a
fix (the report's §3). A household today learns of a new version only if someone tells it; nothing
in Orpheum says "you are running a version with a known hole".

## 2. What exists

- A household installs and updates by running the image's own install script for a tag
  (D314): `docker run --rm ghcr.io/kstenerud/orpheum:<tag> install-script | sh -s -- --dir …`.
- Since an earlier commit the install pins the image by its digest, so nothing moves under a household
  unasked. An update is always an act.
- Every core in the beta has a discovery it keeps a signalling connection to (D274; the beta assumes
  a discovery, the owner 2026-10-30). Standalone cores (no discovery) are coming.
- Signed releases are planned before the public release (github-integrity recommendations 3 and 4:
  signed tags, a signed digest published off GitHub). Nothing is signed yet.

## 3. Proposal, in three parts

**Part A: the discovery says what is current (closed beta).**
- The discovery publishes one small release record: the latest version, its image digest, and for
  each older version whether it has a **security** fix pending: `{latest, digest, versions:
  {"v0.3.1": {"security": true, "summary": "…", "fixedIn": "v0.3.2"}}}`. The owner writes it at each
  release (a file in the discovery's deployment, served at a fixed path).
- A core asks for it over the connection it already keeps, at start and daily. The answer is about
  the core's own version only, so the discovery learns nothing new: it already knows which core is
  calling.

**Part B: the admin is told (closed beta).**
- A core whose version has a security fix pending shows its admins a banner on every page: "A
  security update is available (v0.3.2): <summary>. Update now", with the exact command to run on
  the server and a "How to update" page. Other people see nothing.
- A plain new version without a security fix shows only on the server's settings page, never a
  banner.
- The banner cannot be dismissed while the fix is pending; it can be snoozed for a day.

**Part C: an update the admin opted into installs itself (before the public beta).**
- The core's container has no Docker access, and must not get it (the node's confinement, D193 §1;
  a Docker socket is root on the host). So the updater lives on the host: `install.sh` asks once,
  "Install security updates automatically?", and on yes installs a systemd timer, or a cron job
  where there is no systemd. The timer runs a small script daily. The script asks the discovery's
  record, and if this version has a security fix pending, it runs the install script of the record's
  digest. It refuses unless the digest's signature verifies against the owner's key.
- Without signing (Part C's prerequisite), auto-install would trust whoever controls the discovery
  or the registry. So Part C waits for the signed digest.
- An auto-install restarts the core. The script waits until no playback has been active for 30 minutes,
  or until 04:00 local, whichever comes first.

**Standalone cores** (no discovery) read the same record from a fixed URL on orpheum-app.com, the
static site, so they need no service of ours to stay safe. The record is the same file.

## 4. What it costs and what it gives up

- Part A: a static file and one request a day per core; no new dependency.
- Part B: a banner component and a settings row; the core's own version is already known.
- Part C: a host script and timer the install writes and removes; it depends on signing, which is
  already planned before the public release.
- Privacy: a core's daily request reveals its version to the discovery, which it already knows
  from the connection. A standalone core's request reveals its IP address and version to
  orpheum-app.com's host (Cloudflare Pages).

## 5. Questions for the owner

1. **Part B's banner:** shown to every admin on every page, snoozable for a day, never dismissable
   while the fix is pending. Is that the right level of nagging?
2. **Part C:** a host timer that the install script offers, opt-in. Or would you rather updates stay
   manual through the beta, with the banner only?
3. **The record's home:** the discovery's deployment (beta) and orpheum-app.com (standalone), written
   by you at each release. Is that right, or should a release's workflow write it?
4. **When to build:** A and B now for the closed beta, C after signing exists. Agreed?
