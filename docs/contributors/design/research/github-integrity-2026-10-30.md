> **ABOUTME:** Where Orpheum trusts GitHub today (source, the runtime image on GHCR, the FFmpeg
> release, Actions), what an attacker who holds the owner's account or GitHub itself could do with
> each, which defences still hold when GitHub is hostile, and a ranked list of what to do now,
> before the public release, and later.

# Guarding the Code and the Releases if GitHub Is Compromised

- **Date:** 2026-10-30. Asked by the owner: *"We use github to hold our source code and deploy our
  software. How do we verify and guard the integrity of all that if github ever gets hacked?"*
- **Method:** the repositories read at `main` an earlier commit (orpheum), and the checkouts of
  orpheum-connect, orpheum-ops, orpheum-site and ffmpeg-fdproxy beside it; GitHub's account
  settings cannot be seen from the sandbox, so what they hold is marked *owner to check*. A few web
  sources, linked at the end.
- **Proposes, decides nothing.** Every change below touches a workflow, a gate, a key or a
  dependency, which are the owner's.

## 1. Summary

**The source is safe from GitHub; the households are not.** The owner's own checkouts, which the
sandbox's commits are applied to and which are pushed from, are the real copy of the code: GitHub
holds a copy of it. Connect (the loader every browser runs) and the site are built and shipped from
the owner's machines and never touch GitHub. But **the runtime image a household installs is built,
stored and served by GitHub alone**, pulled by a tag GitHub can move, and run with Docker's access
(root on the host) by piping its own install script to `sh`. Nothing in that path is anchored to
anything only the owner holds. Whoever controls the owner's GitHub account, or GitHub itself, or a
tag of an Action the release workflow uses, can put any code on every household that installs or
updates.

**The fix that matters is a signature made with a key GitHub never sees**, checked by the household
before it runs anything, with the key's public half reaching the household by a path other than
GitHub. Everything else (hardware 2FA, SHA-pinned Actions, signed tags, a mirror, reproducible
builds) narrows the window or makes an attack visible; that signature is what stops it.

## 2. Inventory: every place GitHub is trusted

| # | What | How GitHub is trusted | If GitHub (or the account) is hostile |
| --- | --- | --- | --- |
| 1 | **Runtime image** `ghcr.io/kstenerud/orpheum:<tag>` and `:latest` (D314; `.github/workflows/release-image.yml`) | Built on a GitHub runner from the tag's checkout, pushed to GHCR with `GITHUB_TOKEN`. Households pull by tag; `installImage` falls back to `:latest` for an untagged build (`cmd/orpheum/deployment.go`). No signature, no digest pin. | Swap the image or move the tag: every new install and every update runs it. **Largest household-facing risk.** |
| 2 | **Install script** (`internal/deployment/templates/install.sh`) | Printed by the image itself: `docker run --rm IMAGE install-script \| sh` (also `make deploy-published`). | The script is whatever the image says; it runs as a user in the `docker` group, which is root on the host. |
| 3 | **Client the core serves** (D308 §6) | The core signs its client's file hashes and the loader checks them, but the signing key is the core's own. | A swapped image signs its own swapped client, and the check passes. The loader's version floor does not help. So item 1 also reaches the household's browsers and sessions. |
| 4 | **FFmpeg** (`scripts/fetch_ffmpeg.py`) | ffmpeg-fdproxy's release, built on GitHub runners (`release.yml`, `build.yml`), downloaded from GitHub releases. **Pinned by SHA-256** per platform, taken from the release's asset digests at pin time. | A *later* swap of the asset is refused (good). A build that was already tampered with on the runner when it was pinned is accepted. The macOS binary is jellyfin's own release, the same trust in another project's GitHub. |
| 5 | **jellyfin-ffmpeg source** (`ffmpeg-fdproxy/checkout.sh`) | Cloned from GitHub at the tag `JELLYFIN_TAG` names, not at a commit. Jellyfin's builder fetches its own toolchain. | An upstream tag moved, or a hostile GitHub serving other contents for it, changes what is built, unnoticed. |
| 6 | **Actions in Orpheum's workflows** | `actions/checkout@v4`, `actions/cache@v4` (and `restore`/`save`): **by tag**. ffmpeg-fdproxy pins its actions by full SHA already. | The tj-actions incident (March 2025) moved an Action's tags to a malicious commit across 23,000 repositories. `release-image` runs `checkout@v4` in a job holding `packages: write`. |
| 7 | **nas.example's build from a tag** (`deploy/release/build.sh`, `make deploy`) | `git clone`/`git fetch --tags`/`git pull` from GitHub, then builds and runs what it got. Tags and commits are unsigned. | Rewritten history or a moved tag is built and deployed on production. |
| 8 | **Commits** | Unsigned (`git log --show-signature`: no signatures; no `user.signingkey`, no `commit.gpgsign`). The author is a string anyone can write. | Nothing distinguishes the owner's commits from an attacker's on GitHub. |
| 9 | **The owner's GitHub account** | Owns all five repositories and the GHCR package; pushing a `v*` tag publishes an image. Collaborators, SSH keys, PATs, OAuth apps, 2FA method, rulesets: *owner to check*. | An account takeover reaches items 1–7 without GitHub itself being hostile. |
| 10 | **Go modules** | Fetched through `proxy.golang.org`, checked against `go.sum` and the checksum database `sum.golang.org`, a transparency log not run by GitHub. | Existing versions hold. A new version first fetched at a bump is trusted on first sight; D220's audit covers it. |
| 11 | **npm packages** | `web/package-lock.json` integrity hashes (sha512); the Containerfile checks Playwright's tarball against it. | Existing versions hold; new ones at a bump are first-seen. |
| 12 | **Toolchains in the image** (`deploy/Containerfile`) | Debian base by digest; Go and Node by SHA-256; FFmpeg as item 4. Debian packages from apt at build time, not from a snapshot. | Not GitHub-dependent. The image is not reproducible (apt at build time, timestamps), which matters for §4's independent rebuild. |
| 13 | **Connect and the loader** (`orpheum-connect/deploy/production/package.sh`) | **Not trusted:** built from the two *local* HEADs by `git archive`, on the owner's machine, shipped as a `.run` file to prod-api. connect pins Orpheum's `discovery/` by commit (`ORPHEUM_PIN`). No CI. | Safe from GitHub as long as the owner builds from checkouts that came from the sandbox, not freshly cloned from GitHub. The `.run`'s SHA-256 is a check of its own bytes, not of who made it. |
| 14 | **orpheum-site** | Not trusted: uploaded to Cloudflare Pages directly (its README), no Git integration. | Unaffected. Being off GitHub makes it a useful second channel (§4.2). |
| 15 | **Secrets in workflows** | Only `GITHUB_TOKEN`; no PATs, no cloud keys, no `id-token: write`. | Little to steal from a runner beyond the GHCR push right, which is item 1. Good as it stands. |

**Riskiest, in order:** 1–3 (image → install script → client, one chain), 9 (the account is the
cheapest way into that chain), 6 (a third party's tag inside the publishing job), 7 (production
builds whatever GitHub hands it), 4–5 (FFmpeg built where it cannot be checked).

## 3. The threats, and what each needs

- **The owner's account taken over** (phished password, stolen session cookie, stolen PAT or SSH
  key). The attacker pushes a tag; the workflow does the rest. GitHub's own controls still work and
  can help: hardware-key 2FA, rulesets, immutable releases, the audit log.
- **GitHub itself hostile** (an insider, a breach, a legal order). Repository contents, tags,
  release assets, GHCR images and workflow runs can all be rewritten, and *every check GitHub
  performs or attests to can be forged*: rulesets, "Verified" badges, artifact attestations, OIDC
  tokens. Only checks made outside GitHub, against keys or hashes GitHub never held, survive.
- **A malicious Action** (its tag moved, its maintainer compromised). Runs with the job's token;
  in `release-image`, that is the GHCR push.
- **The install and update path.** First install trusts whatever the household was told to run; an
  update trusts whatever the tag points to now. The second can be protected by what the first
  installed (a key, trusted on first use); the first can only be protected by giving it a second,
  independent channel.

**Which defences survive a hostile GitHub:**

| Defence | Account takeover | GitHub hostile |
| --- | --- | --- |
| Hardware-key 2FA, rulesets, tag protection, immutable releases | helps a lot | nothing |
| Actions pinned by full SHA | stops a moved tag of someone else's Action | nothing (GitHub serves the commit) |
| Sigstore *keyless* signing / GitHub artifact attestations | nothing if the attacker runs the workflow | **nothing**: the certificate is issued for a GitHub OIDC token, which GitHub mints. Rekor does log the signing, so a monitor watching for unexpected entries *detects* it |
| Signature with an **owner-held key** (SSH or GPG for tags; cosign `--key` or `ssh-keygen -Y` for images), public key distributed off GitHub, checked by the consumer | **holds** | **holds** |
| Pull by **digest** published off GitHub | holds | holds (a digest is content-addressed) |
| Independent mirror + ref comparison | detects | detects |
| Reproducible build checked by an independent builder | detects | detects; with signing on match, prevents |
| Hash pins in the repository (FFmpeg, Go, Node, Debian) | hold for what was pinned | hold, if the repository copy checked is the owner's own |

## 4. How others do it

- **Git's own signatures.** Since git 2.34 commits and tags can be signed with an SSH key
  (`gpg.format=ssh`), checked against an `allowed_signers` file with `git verify-tag` /
  `git verify-commit`. A FIDO key (`ed25519-sk`) keeps the private half on a hardware token.
  GitHub's "Verified" badge is GitHub's word; `git verify-tag` against a file the owner controls is
  not. The Linux kernel and Git itself rely on signed tags, not on the host.
- **Go.** Modules are checked against `sum.golang.org`, a transparency log; the toolchain is
  perfectly reproducible since 1.21, and `gorebuild` lets anyone rebuild the published archives and
  compare, which go.dev/rebuild does nightly and an outsider has done for thousands of builds.
- **Debian, Arch.** Packages are signed by maintainers' keys; the archive's `Release` file is signed
  by keys kept apart from the mirrors, so any mirror can serve the bytes. Debian's
  reproducible-builds effort lets independent rebuilders confirm a binary came from its source.
- **Tor Browser.** Reproducible builds; several builders build each release independently and sign
  the hashes; a release ships only when they match. **Signal** for Android publishes how to rebuild
  its APK and compare.
- **Homebrew.** Signs its bottles with GitHub's artifact attestations (Sigstore via GitHub OIDC): a
  real defence against a swapped asset or a stolen token, *not* against GitHub itself.
- **Sigstore.** Keyless signing binds a signature to an OIDC identity; with GitHub as issuer that
  identity is GitHub's to grant. cosign also signs with a plain key, including one on a YubiKey
  (`--sk`, PIV), and verifies offline. Rekor is a public log of every signature, useful as a
  tripwire when someone monitors it for the project's identity or key.
- **TUF / Uptane.** A framework for update metadata: an offline root key, separate keys for what is
  released and for freshness, thresholds, and expiry, so a compromised server cannot serve old
  (rollback) or stale (freeze) releases, or anything unsigned. Used by Docker Notary, PyPI's design,
  automotive updates (Uptane). Heavy for one maintainer; the ideas carry over cheaply (a version
  counter the updater never goes back on).
- **SLSA.** A ladder of build-provenance levels. GitHub-hosted runners with GitHub's attestations
  reach Build L3, but the provenance is GitHub's statement: SLSA itself says the build platform is
  trusted. It is evidence against everyone but GitHub.
- **GitHub's immutable releases** (GA October 2025): a release's assets and tag cannot be changed
  after publishing, and GitHub signs a release attestation. Stops an account takeover from swapping
  a published asset; GitHub itself is still trusted.

## 5. Recommendations, ranked by protection per effort

### Now (closed beta)

1. **Harden the account** (protects: takeover, the cheapest attack). Owner, about an hour:
   passkeys or two hardware security keys as the only 2FA (no SMS, no TOTP backup on the phone);
   review and prune SSH keys, PATs, OAuth and GitHub Apps, deploy keys, collaborators; a ruleset on
   every repository refusing force-pushes and deletions on `main` and limiting `v*` tag creation to
   the owner; turn on **immutable releases** for ffmpeg-fdproxy and orpheum; read the security log
   once. Cost: two FIDO keys (~€50 each) if not already held.
2. **Pin Orpheum's Actions by full SHA** (protects: a third party's moved tag, inside the job that
   publishes). A workflow edit, so the owner's: `actions/checkout@<sha> # v4.x`, the same for the
   three cache actions, as ffmpeg-fdproxy already does. Minutes. Dependabot or a periodic agent
   task can bump them.
3. **Sign tags with an SSH key on a hardware token, and verify before building anything that came
   from GitHub** (protects: production building rewritten source; makes the owner's releases
   recognisable). Owner: `ssh-keygen -t ed25519-sk` on a FIDO key (one per key, both listed), git
   config `gpg.format=ssh`, `user.signingkey`, `tag.gpgSign=true`; tags are made on the owner's
   machine, which already pushes them. The `allowed_signers` file is committed *and* kept on nas.example
   and prod-api outside the clone, so a rewritten repository cannot change it. Then (a gate and
   script change, the owner's) `deploy/release/build.sh` runs `git verify-tag` against that file
   before checking a tag out. Signing every commit is optional: the sandbox's commits are applied
   by the owner, so signing at apply or at the tag gives the same assurance. No new tool:
   OpenSSH ≥ 8.2 and git ≥ 2.34.
4. **Sign each release's image digest with the owner's key, and have the install and update check
   it** (protects: GHCR image swap, a moved tag, a hostile GitHub, a compromised runner whose
   output the owner then signs only after item 7's second build matches). The smallest version needs no new tool:
   - At a release, the owner pulls the GHCR image, notes its digest, and signs a one-line manifest
     (`orpheum <version> <counter> ghcr.io/kstenerud/orpheum@sha256:<digest>`) with
     `ssh-keygen -Y sign` and the hardware key.
   - The manifest and signature are published **off GitHub**: on orpheum-app.com (Cloudflare Pages,
     uploaded directly), and also as release assets, so either channel can serve them and both
     must fall to forge one.
   - Households start from a short bootstrap script on orpheum-app.com that holds the public key,
     fetches the manifest, verifies it with `ssh-keygen -Y verify`, and pulls **by digest**, never
     by tag; `:latest` is retired. The deployment keeps the public key and the last counter, so
     each later update is checked against the key installed first and never goes back to an older
     version (TUF's rollback rule, in one integer).
   - The design question for the owner: whether the household's install command changes from
     `docker run … install-script | sh` to `curl https://orpheum-app.com/install | sh` (the
     bootstrap), or keeps the docker form and prints a digest to compare. The first is the one
     that protects the first install. Cosign with `--key` / `--sk` is the standard tool for the same
     job and stores the signature in the registry; it is a new dependency on every household's host,
     the owner's decision; `ssh-keygen` is already there.
   Cost: a design decision, perhaps a day of building, and one signing step per release that the
   owner does by touching the key.
5. **Push to a second remote and compare** (protects: detects rewritten history or deleted repos;
   keeps a copy GitHub cannot erase). A bare repository on nas.example or the owner's NAS (and later
   Codeberg), pushed together with GitHub; a small script compares `git ls-remote` of both against
   the local refs and says when they differ. nas.example then clones from the owner's own mirror.
   Minutes to set up; the owner's machines.

### Before the public release

6. **Sign the connect package and verify it on prod-api** (protects: the loader, which every
   household's browser runs; today safe only by discipline). `package.sh` already builds locally;
   add an `ssh-keygen -Y sign` of the `.run` and a verify in `unpack.sh` against a key installed on
   prod-api once. Keep building connect from the owner's checkouts, never from a fresh GitHub clone.
7. **Make the runtime image reproducible, and build it twice** (protects: a compromised GitHub
   runner, or a compromised owner machine, either alone). Debian packages from
   `snapshot.debian.org` at a pinned date, `SOURCE_DATE_EPOCH` and BuildKit's timestamp rewriting,
   no build-time network outside pinned hashes. Then the GitHub-built image and one built on
   gpuhost must have the same digest before the owner signs it (step 4): the Tor model with two
   builders. If it cannot be made reproducible soon, build the published image on the owner's
   machine and push it, so GitHub only stores bytes the signature covers.
8. **The same for FFmpeg**: check jellyfin out by commit, not tag (record the commit beside
   `JELLYFIN_TAG`); rebuild a release once on gpuhost (`make remote-build` exists) and compare the
   tarball's hash with the release's before pinning it. If jellyfin's builder is not reproducible,
   the fallback is to build the release off GitHub and upload it, and pin that.
9. **Fold this into D220's supply-chain audit**: the same release checklist pins dependencies, signs
   the manifest and verifies the second build.

### Later (more maintainers, more households)

10. **Proper TUF-style roles**: an offline root key that only certifies the release key, so a lost or
    stolen release key is replaced without every household reinstalling; a threshold of maintainer
    keys; expiring freshness metadata against freeze attacks. A library (go-tuf) would be a new
    dependency, the owner's decision.
11. **A public log of releases** (Rekor with the owner's key, or a self-run append-only list
    mirrored by others), with a monitor that alerts on any signing the owner did not do.
12. **SLSA provenance** from GitHub's attestations, as useful evidence for users who trust GitHub,
    on top of, never instead of, the owner's signature.

## 6. What the owner would have to do personally

- Buy or find **two FIDO2 security keys** (one kept apart as the spare): GitHub 2FA, git tag signing,
  release signing.
- Do the **account hardening** (item 1) in GitHub's settings; no one else can.
- **Touch the key at each release**: sign the tag, sign the manifest (items 3, 4).
- **Decide**: the household install command (item 4), cosign or `ssh-keygen` (item 4), a mirror's
  host (item 5), and the workflow and gate edits items 2, 3, 4 and 7 need (`Gate-Changed:`).

## Sources

Checked this session:

- GitHub, [Immutable releases are now generally available](https://github.blog/changelog/2025-10-28-immutable-releases-are-now-generally-available/) (2025-10-28)
- CISA, [Supply chain compromise of tj-actions/changed-files (CVE-2025-30066) and reviewdog/action-setup](https://www.cisa.gov/news-events/alerts/2025/03/18/supply-chain-compromise-third-party-tj-actionschanged-files-cve-2025-30066-and-reviewdogaction); Wiz, [tj-actions/changed-files supply chain attack](https://www.wiz.io/blog/github-action-tj-actions-changed-files-supply-chain-attack-cve-2025-30066)
- Go, [Perfectly Reproducible, Verified Go Toolchains](https://go.dev/blog/rebuild) and the [rebuild report](https://go.dev/rebuild)
- Sigstore, [cosign README](https://github.com/sigstore/cosign/blob/main/README.md) (keys, `--sk`, PIV) and [Verifying signatures](https://docs.sigstore.dev/cosign/verifying/verify/) (offline verification)

General references, not re-fetched:

- Git, [`git config` gpg.format / gpg.ssh.allowedSignersFile](https://git-scm.com/docs/git-config#Documentation/git-config.txt-gpgformat); OpenSSH `ssh-keygen -Y sign/verify` (man page)
- [The Update Framework](https://theupdateframework.io/) and [Uptane](https://uptane.org/)
- [SLSA build levels](https://slsa.dev/spec/v1.0/levels)
- [Reproducible Builds](https://reproducible-builds.org/) (Debian, Arch, Tor)
- Sigstore, [Fulcio and OIDC identities](https://docs.sigstore.dev/certificate_authority/overview/)
