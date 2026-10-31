# Security updates reaching households (draft for the owner, 2026-10-31)

**Status:** agreed by the owner, 2026-10-31 ("1, 2, 3: yes, go ahead", to the lead's recommendations
on §5's questions): the banner as §3 Part B says; Part C's timer built only once signing exists,
updates manual with the banner until then; the release record written by the release workflow, not
by hand; Parts A and B in the next batch. Parts A and B built for the 11-01 batch (§6); the release
workflow's step and the paid service's call are owed.

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
- **The 24-hour hold (D393 §8; the root key audit's F4), a rule Part C must keep:** an automatic install
  installs a release only once the core has known the record naming it as the latest for 24 hours,
  counted on the core's own monotonic clock from first sight, and only once the core has caught up
  with the publisher's chain since it started; never a version at or below its own. The core already
  computes it: `discovery.Release.MayInstallAutomatically(now)` (`AutomaticInstallHold`), which Part
  C's updater asks of the core instead of deciding for itself. A recovery statement during the hold
  that removes the record's signer drops the record, and the install with it. The admin's banner and
  an install by hand are not held.

**Standalone cores** (no discovery) read the same record from the record's published locations or a
file their operator places (D391 §6, which replaces a fixed URL on the owner's site: nothing reaches
the owner's domains for it). The record is the same file.

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

## 6. As built (Parts A and B)

**The record (D391 §6, corrected by D393 §2, §5).** A text payload in the strict line format of the
root key audit's §3.1, `discovery/release`'s `Record`, and its detached SSH signature, by an everyday
or recovery key of **Orpheum's publisher root**, a chain of its own compiled into every binary (never a
discovery's), in the namespace `orpheum-release-v1`:

```
orpheum-signed release 1
id op1-…
root-version 3
sequence 57
issued 2026-11-02T09:00:00Z
expires 2026-12-02T09:00:00Z
latest v0.4.0
image v0.4.0 sha256:<the multi-architecture index>
image v0.3.2 sha256:…
pending v0.3.2 v0.4.0 <one sentence for the admin>
pull ghcr.io/kstenerud/orpheum
location https://<where the record is served>
```

`(root-version, sequence)` only rises; `expires` is a month after `issued`, and past it a core keeps
the record and says it cannot confirm it is current; a version's `image` digest never changes once
signed (a rebuilt release takes a new version). `pull` lines are mirrors: the digest is the release.

**Its home.** The record and its `.sig` are served at `/.well-known/orpheum/release` beside the
publisher's chain (`/.well-known/orpheum/publisher-root/<n>` and `latest`) at the publisher's places
(compiled in) and by every discovery (`single.Parts.Published`, the paid service's own). Nothing
reaches the owner's domains but through those places, and an own discovery depends on nothing of the
owner's: its cores read the publisher's record wherever it is served, the signature being what counts.

**Writing and signing it.** `go run ./cmd/release-record --record <file> --version <tag> --digest
<sha256:…> [--security "<summary>"] [--pull <repository>] [--id op1-… --root-version <n>]` writes one
release into the unsigned record, dated, its sequence one more: with `--security`, every older release
with no fix pending is marked as needing this one; a release older than the latest, and a version given
another digest, are refused. It prints the command that signs it: `ssh-keygen -Y sign -n
orpheum-release-v1 -f <the key's stub> <file>`, then `go run ./cmd/discovery-sign check --kind release
--publish <dir>`, which checks it with the code a core uses, keeps it in the owner's ledger and writes
the files to publish. The release workflow (a gate file) is to run `release-record` after the image is
pushed; the owner signs on the Mac after checking the digest (docs/users/discovery-signing.md).

**The core reads it.** The core follows the publisher's chain from the genesis it was built with, then
fetches the record from every place, takes the newest above its floor that the chain's current set
signed (each key's user-verified flag checked), refuses one that changes a digest it has seen or
another of its floor's own sequence, and keeps what it says of its own version alone (`Record.For`).
It reads at its start and daily, an hour after a reading that found none; a development build reads
nothing; a build with no publisher root reads none and says so; a failure is never the core's
standing problem. Its trust state is `trust-publisher` in the persistent directory, checked again at
each start.

**The install checks it before running anything (the audit's §3.9, F10).** A household installs with
the bootstrap the owner publishes (`discovery-sign bootstrap`): it holds the publisher chain's current
keys as `ssh-keygen`'s allowed signers for `orpheum-release-v1` alone, fetches the record and its
signature, verifies it with `ssh-keygen -Y verify`, reads the latest's image digest with `awk`, pulls
the image by that digest, and only then runs that image's own install script with `--image
<repository>@<digest>`. `ssh-keygen` cannot check the user-verified flag, so this first install has a
touch's strength; every later update goes through the core's check, which has the PIN's.

**The admin.** `GET /api/v1/system/update` (admin) says the version, the latest when newer, its
digest, the fix pending, when the record expired and what was wrong reading it, and the command: `docker run --rm <location>@<digest> install-script | sh`
from the first location the signed record names, by the digest it signed, or, with no record's
location, the image the build was published as and the latest's tag; the latest's own install script
run again over the deployment (D314), which keeps everything. The client's banner (`security-update-note.tsx`) stands above every page for an
admin while a fix is pending, with the command, Update now (the How to update page, `/update`) and
Snooze for a day (this browser, that fix). The settings page's Updates row shows the version and a
newer one, the only place a plain release shows.

**Not yet:** the publisher root's genesis is made at the owner's day-one ceremony and compiled in;
until then a release build reads no record, and says so. The paid service (orpheum-connect) is to
serve the record and the publisher's chain at their paths. The banner's
command assumes the deployment's default folder; the How to update page says what to add for another.
