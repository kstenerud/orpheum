# The public release (the owner, 2026-11-08)

The owner is making the repository public as evidence of a product built with AI across frontend,
backend and operations. It is not claimed ready for general use: it stays a closed beta. The owner
also wants installation as painless as possible, and Orpheum to install on OpenMediaVault.

## The owner's answers (2026-11-08)

- **History:** a separate public repository from a filtered clone of `main`: home addresses,
  hostnames and private paths replaced, the once-committed 14 MB `discovery-sign` binary and the
  `Claude-Session:` trailers dropped. The private repository is not rewritten. The result is
  scanned again before the owner pushes it.
- **The OpenSubtitles application key** stays in the source: it ships in every binary (D224 §7).
- **OpenMediaVault's settings:** given on first start, so that one pasted compose file installs
  it. D204 stands (a release reads no environment but TZ): compose substitutes its variables into
  the command line, which the binary reads.
- **Test account names** in the docs are replaced with generic ones.

## The work

1. **The tree, scrubbed:** the home network's addresses and hostnames, the owner's access notes
   and private paths leave the current documents; a note names the private companion
   repositories once.
2. **The front page:** a README for a visitor (what it is, its status, screenshots, features,
   architecture, stack, install, how it was built) and `docs/how-it-was-built.md`; the stale
   "nothing built yet" status lines corrected.
3. **Install:** a first-start command that writes the deployment from its arguments when there is
   none and then serves; `deploy/omv/compose.yaml`; `docs/users/install.md` with an
   OpenMediaVault section.
4. **A multi-architecture image** (amd64 and arm64): a change to `.github/`, a gate, so it waits
   for the owner's `Gate-Changed:`.
5. **The filtered public clone,** scanned, handed to the owner to push.
