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

**The record (signed since D391 §6).** One JSON file, `discovery/release`'s `Record`, the exact bytes
a root key of the discovery's set signs (`trust.KindReleaseRecord`, the SSH signature namespace
`orpheum-release-record-v1`):

```json
{"issuedAt": 1800000000000, "latest": "v0.3.2", "locations": ["registry.example/orpheum"],
 "releases": {"v0.3.2": {"digest": "sha256:…"},
              "v0.3.1": {"digest": "sha256:…", "security": true, "summary": "…", "fixedIn": "v0.3.2"},
              "v0.3.0": {"digest": "sha256:…"}}}
```

`latest` is the newest release; `releases` names every release the record has seen, each with its
image's digest, and for an older one whether a security fix is pending for it, what the fix fixes (one
sentence for an admin) and the first release that has it; `locations` are the repositories the image
may be pulled from, each a mirror, since the signed digest, not the place, says it is the release.
It is served as `{"payload": "<the record's exact bytes>", "signature": "-----BEGIN SSH SIGNATURE-----…"}`.

**Its home (D391 §6).** The signed record is published with each release, and served by every
discovery at `/.well-known/orpheum-release-record`, at its issuer and at each location its address
document names: `single.Parts.ReleaseRecord` for Orpheum's own discovery (`orpheum dev discovery
--release-record <file>` in development), and the paid service's own setting. Nothing reaches the
owner's domains for it, and an own discovery depends on nothing of the owner's: its operator signs
the record with their own root key, or serves the owner's to cores that trust the owner's keys. An
absent file tells every core nothing, which is never an error to the core.

**Writing and signing it.** `go run ./cmd/release-record --record <file> --version <tag> --digest
<sha256:…> [--security "<summary>"] [--location <repository>]` writes one release into the unsigned
record, dated: with `--security`, every older release with no fix pending is marked as needing this
one (one already pending keeps its first fix, which the latest has too); a release older than the
latest is refused, and so is a digest that is no image's. It then prints how to sign it: `go run
./cmd/discovery-sign record --record <file> --signer <key> --out <signed>`, with any one root key, a
YubiKey's resident key included (docs/users/discovery-signing.md), or `ssh-keygen -Y sign -n
orpheum-release-record-v1 -f <key> <file>` by hand and `discovery-sign seal`. It is a program of its
own, not an `orpheum` subcommand, because no household runs it, and Go, not a script under
`scripts/`, so the record is written by the very type a core reads it with. The release workflow (a
gate file) is to run it after the image is pushed, with the pushed digest; the owner signs, since a
root key is a YubiKey in a drawer, not a CI secret.

**The core reads it.** No call under the session: the core fetches the signed record from its
discovery's issuer and each location its document names, takes the newest one a root key of the set
it pinned signed, and keeps what it says of its own version alone (`Record.For`): `latest`, its digest
and a location only when the latest is newer, the fix only when its version has one pending. A record
no root key signed is never believed. The core reads at its start and daily, an hour after a
failure; a development build (`dev`) reads nothing; a discovery serving none knows nothing, and a
failure is never the core's standing problem.

**The install checks it.** `sh install.sh … --release-record <signed record> [--root-keys <file>]`
checks the pulled image's digest against the signed record before it installs anything, in Go:
`orpheum verify-release --digest <sha256:…> [--root-key <line>]… < <signed record>`, run from the
image the deployment ran before on an update (so a new image does not vouch for itself), with the
suggested discovery's root keys unless the operator names their own. Not yet the default: an install
without the flag checks nothing, until the owner's keys are filled in and the record is published
(the install script's tests are a gate file; see the plan's record of what is owed).

**The admin.** `GET /api/v1/system/update` (admin) says the version, the latest when newer, its
digest, the fix pending and the command: `docker run --rm <location>@<digest> install-script | sh`
from the first location the signed record names, by the digest it signed, or, with no record's
location, the image the build was published as and the latest's tag; the latest's own install script
run again over the deployment (D314), which keeps everything. The client's banner (`security-update-note.tsx`) stands above every page for an
admin while a fix is pending, with the command, Update now (the How to update page, `/update`) and
Snooze for a day (this browser, that fix). The settings page's Updates row shows the version and a
newer one, the only place a plain release shows.

**Not yet:** the paid service (orpheum-connect) serves the signed record at its well-known path; until
then a core in the beta reads none, and knows nothing. The banner's
command assumes the deployment's default folder; the How to update page says what to add for another.
