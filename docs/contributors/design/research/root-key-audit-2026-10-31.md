> **ABOUTME:** An outside audit of D391's trust design before it is built: the discovery's root key
> set on the owner's YubiKeys, any one key signing anything, root-set statements, the signed address
> document and release records, and how a core pins them. Findings ranked by severity; a corrected
> design stated precisely (formats, the verification algorithm, rotation and recovery); the owner's
> day-one key ceremony. It proposes; it decides nothing.

# The root key and signing design: an outside audit

- **Date:** 2026-10-31, on an earlier commit. **Asked by** the lead, for the owner: *"This is the one part
  that absolutely MUST be correct from the start."* Research only.
- **The design audited:** D391 §2–§8 in the working notes (with D390 above it), as amended by
  Earlier commits, plus one element the brief states that D391's text does not: a root key
  may delegate to an online signing key with a validity window, for the running discovery.
- **Read:** D390, D391, D273, D292, D293, D308, D310, D314 in the working notes;
  [domain-assumptions-audit-2026-10-31.md](domain-assumptions-audit-2026-10-31.md) §4–§6;
  [domains-audit-2026-10-31.md](domains-audit-2026-10-31.md) §6;
  [security-updates.md](../plans/security-updates.md);
  [github-integrity-2026-10-30.md](github-integrity-2026-10-30.md) recommendations 3 and 4; the code
  in `internal/core/discovery/` (`discovery.go`, `discovery_call.go`, `doc.go`),
  `internal/core/identity/identity.go`, `discovery/proof/proof.go`, `discovery/release/release.go`,
  `discovery/discoveryproto/`, `cmd/release-record/`, `internal/core/backup/backup.go`,
  `internal/core/instancedir/`, `internal/deployment/templates/install.sh`; golang.org/x/crypto
  v0.58.0's `ssh/keys.go` (the version `go.mod` pins), read locally; OpenSSH's `PROTOCOL.sshsig`,
  `sshsig.c`, `ssh-keygen.c` and the `ssh-keygen(1)` manual. The worktree building D391
  (`agent-ab46de1ca217b285f`) was read: it differs from `main` only in the decision's text, which
  lacks the root-set paragraph an earlier commit added; no code of the design exists there yet.
- **Markers:** **[V]** verified against the code or a source listed at the end; **[I]** inferred,
  reasoned rather than shown.

## 0. The plain answer

The shape of D391 is right: a discovery is a key set, its names are locations, every signed object is
checked against a pin the core holds, nothing of the owner's is needed by an own discovery. Four things
in it are wrong in ways that would be painful to change once the first core has pinned anything:

1. **Equal keys with sticky removal have no way back.** A thief holding one key and its PIN removes
   the other two, and every core that sees his statement first is his for good; worse, the rule "two
   sets of one version are both refused" deadlocks the *owner* too, since an honest replacement then
   has nowhere to go. The cheapest fix that keeps the owner's "one key signs, no quorum": **two tiers,
   one signature each**. The carried and home keys sign everything routine, and replace each other;
   only a **recovery key** (the bank's, plus a second one) may change the recovery tier, and a
   recovery-signed root statement may jump over anything an everyday key signed. That gives the owner
   a guaranteed way back without ever needing two keys at once (§2, F1).
2. **The release record is not the discovery's to sign.** An own discovery's operator must not be able
   to push images to his cores, and a standalone core has no discovery. Releases need their own trust
   root, Orpheum's, compiled into every binary; the owner's same YubiKeys may hold it, under a
   separate chain and namespace (F2).
3. **"Useless without its PIN" is true only if the key demands the PIN and the verifier checks it.**
   golang.org/x/crypto checks the touch flag, not the PIN flag; `ssh-keygen -Y verify` checks neither
   **[V]**. Keys must be made with `-O verify-required` and Orpheum must check the user-verified bit
   itself (F3).
4. **The formats must be versioned and extensible before the first pin**: a format header, a
   criticality rule for unknown lines, roles and per-role thresholds (fixed at 1, but present, so a
   quorum can be adopted later without a format change), and a discovery identifier that is a hash of
   the first root statement, not a name (F5, F7).

Everything else is ordinary update-system hygiene from TUF and Uptane: monotonic sequences per object
type, expiry that warns rather than bricks, signatures over the exact bytes served, one namespace per
object type, a hold on automatic installs and a watcher that tells the owner of every signature it did
not expect. The online delegated key is **overkill for the beta**; reserve its place in the format and
build it only if freshness ever matters more than a monthly re-sign.

## 1. Findings, ranked

Severity: **critical** (would let one stolen key own households, or lock the owner out, with no way
back), **high** (a format or protocol choice that cannot be changed once pinned, or a hole of wide
reach), **medium** (a real weakness, fixable later at some cost), **low**.

| # | Severity | Finding | Verdict |
| --- | --- | --- | --- |
| F1 | critical | Any-one-key + sticky removal + "both refused" = permanent capture by a key-and-PIN thief, and deadlock for the honest owner | fix needed: recovery tier (§2) |
| F2 | critical | Release records signed by "the discovery's root": wrong principal for own discoveries and standalone cores | fix needed: a separate release root |
| F3 | critical | PIN protection depends on `verify-required` and on a UV check no library does for you | fix needed |
| F4 | high | A root key is code execution on every auto-updating core; nothing bounds or detects a stolen key's window | fix needed: hold + watcher |
| F5 | high | First use: a name is pinned where an identity should be; own discovery is pure TOFU; RFC 8414 exact match conflicts with fallback locations | fix needed |
| F6 | high | A stolen key can move a core's locations so it never hears the recovery (eclipse) | fix needed: sticky locations |
| F7 | high | No format version, criticality, roles or thresholds in the signed formats: the first change locks old cores out | fix needed |
| F8 | high | What is verified must be the bytes served: today's `POST /release` answer is derived and cannot carry a signature; SSHSIG and JSON verification pitfalls | fix needed |
| F9 | medium | Rollback and freeze: "higher version" for root only; nothing for documents and records; fast-forward by a stolen key; clocks | fix needed |
| F10 | medium | The install runs the image (`install-script`) before anything verifies it | fix needed |
| F11 | medium | `release.Released` gives a re-released version a new digest: a signed version's digest must never change | fix needed |
| F12 | medium | Backups restore the trust state too: a restore rolls the pin back | fix needed |
| F13 | medium | All keys lost has no recovery; no tested key ever proves it still works | fix needed: second recovery key, drills |
| F14 | medium | Core signatures bound to the issuer name (D391 §4) should bind to the discovery's identity | fix needed |
| F15 | low | The online delegation is in the brief, not in D391; unscoped it would undo the offline root | overkill now; reserve |
| F16 | low | Algorithm and token details: `ed25519-sk` over `ecdsa-sk` (EUCLEAK), distinct PINs, a dedicated credential never in an agent | fix needed (ceremony) |
| F17 | low | Equivocation is detected only by a core that sees both statements | cheap fix: heartbeat reports the pin |
| F18 | low | The build worktree works from D391's text before the root-set paragraph | tell the build |

### F1 (critical). Any one key, sticky removal, and a version race have no way back

D391 §2: any key of the pinned set signs a root-set statement at the next version; a removed key is
never accepted again; two different statements of one version are both refused. Play it out:

- **Thief first.** The carried key is stolen with its PIN (watched over a shoulder at a café while
  signing a release). The thief signs v6 = {his own key, the carried key}, removing the home and bank
  keys. Every core that fetches v6 before the owner acts now refuses the owner's two remaining keys
  forever (sticky removal), follows the thief's address documents and installs the thief's release
  records. The owner's only recourse is every household's admin re-enrolling by hand, and the owner
  cannot even tell them so through the product, since the product now speaks for the thief.
- **Owner first, then thief.** The owner notices, signs v6 = {home, bank} removing the carried key.
  The thief signs his own v6 at the same moment. A core that sees both refuses both (D391's rule) and
  stays at v5, *which still contains the stolen key*. The owner cannot publish v7 because a core
  catches up "through each version in order" and v6 is in dispute. **The honest owner is deadlocked**,
  and the thief keeps signing address documents and releases under v5.
- **Symmetry is the root cause.** With equal keys the core has no way to tell owner from thief. The
  only asymmetries available are: more keys (a quorum, refused by the owner for routine use), time
  (who acts first, or a delay), and authority (a key that may do what others may not). Of the options
  the brief lists:
  - *A delay with veto* is symmetric: the thief vetoes the owner's removal as easily as the reverse,
    and it relies on cores being online and on the clocks the design must not trust. **Not
    recommended** on its own.
  - *The signing key must remain in the new set* stops nothing a thief wants (he keeps his own key and
    removes the others). It is worth having for a different reason, against lock-out by mistake (§3.4
    step 6).
  - *Out-of-band notification* detects; it does not repair. Needed beside a fix (F4), not instead.
  - *A last-resort key of higher authority* is the one that gives the owner a guaranteed way back with
    one key at a time. **Recommended**, precisely as §3.4 states it.
- **The fix:** two tiers in the root set, each signing alone. *Everyday* keys (carried, home) sign
  address documents, release records and root statements that change only the everyday tier.
  *Recovery* keys (the bank's YubiKey, plus a second recovery key at another site, F13) may sign
  anything, are the only keys that may change the recovery tier, the thresholds or the roles, and a
  recovery-signed root statement may skip versions. A thief with an everyday key and its PIN can then
  do damage for as long as the owner takes to reach the bank, and not a moment longer on any core: the
  owner's recovery statement at a version far beyond the thief's removes his keys everywhere, without
  any core having to rewind. This is not a quorum: every signature is one key, one touch, one PIN.
- **What it costs the owner:** recovery requires a trip to the bank. Losing the carried key, or
  having it stolen without its PIN, is still repaired with the home key in minutes.

### F2 (critical). The release record has the wrong signer

D391 §6 has the release record "signed by a root key" and served by every discovery, and §8 lets an own
discovery read it from the record's published locations. But whose root? A core enrolled with an own
discovery pins *that operator's* root set; if the release record is checked against the pinned
discovery root, then either the operator signs Orpheum releases for his cores (he can then push any
image to any household that trusts his discovery, including friends' households that never agreed to
run his code), or the core cannot verify the owner's record at all. A standalone core has no discovery
and no root. **[I]**

Two different principals exist and must be two trust roots:

- the **publisher** of Orpheum (today the owner), whose root is compiled into every binary and governs
  release records, for every core whatever its discovery; a fork changes it in one file;
- the **discovery**, whose root the core pins at enrolment and which governs address documents.

The same three YubiKeys may hold both (same credentials, separate chains and namespaces), so the
ceremony does not double. Mixing them is a cross-principal confusion of exactly the kind TUF's
separate roles exist to prevent.

### F3 (critical). The PIN protects only if the key demands it and the verifier checks it

D391: "a stolen key is useless without its PIN". Checked against the code:

- golang.org/x/crypto v0.58.0's `skEd25519PublicKey.Verify` and `skECDSAPublicKey.Verify` reject a
  signature without the user-presence bit (0x01) unless the key is marked no-touch-required, and
  check nothing else: not the user-verified bit (0x04), not the counter, not the application beyond
  hashing it into the signed data **[V]** (`ssh/keys.go`, `flagUserPresence`, `errSKMissingUserPresence`).
- OpenSSH's `sshsig.c` passes the signature's details back but does not check them, and `ssh-keygen -Y
  verify` only logs `sk_flags` at debug level; the allowed-signers file has no `verify-required`
  option **[V]**. A shell verifier therefore accepts a touch-only signature.
- `-O verify-required` makes OpenSSH *ask* the token for verification at signing; the token itself
  does not refuse an assertion without it unless configured to (CTAP 2.1 `alwaysUv`). A resident
  credential made by OpenSSH is protected from *discovery* without the PIN (credProtect), but anyone
  holding the key and a key-handle stub (an `id_ed25519_sk` file on the owner's laptop, written by
  `ssh-keygen -K` once) can ask it for a touch-only signature with a few lines of libfido2. **[I]**,
  from CTAP 2.1's credProtect levels and OpenSSH's `sk-usbhid.c` behaviour.

So the claim holds only if: (a) every root key is made with `-O verify-required`; (b) Orpheum's
verifier requires the UV bit for every key the root set marks `uv`; (c) where the firmware allows
(YubiKey 5.7+), `alwaysUv` is switched on, making the token itself refuse. Without (b), a thief who
takes the laptop bag with the key in it needs no PIN at all.

### F4 (high). A root key is code execution on every household that auto-updates

Once Part C of security-updates.md lands, a valid release record makes every opted-in core pull and run
the image it names. A stolen everyday key (with PIN) therefore runs the thief's code on every such
household within a day, and F1's recovery cannot undo an install that already happened. D391 bounds
nothing here. Two cheap measures, both consistent with one-key signing:

- **A hold on automatic installs**: a core installs a release automatically only once it has known the
  record naming it for 24 hours (counted on its own monotonic clock from first sight). The admin's
  banner and manual update are not held. A recovery statement during the hold removes the signer and
  cancels the install.
- **A watcher that tells the owner of every signature**: every signed object is public by design; a
  small job on the owner's machine (nas.example) fetches the root chain, address document and release
  record from every location every 15 minutes, compares each against the owner's own ledger of what the
  signing tool produced (§3.7), and alerts the owner's phone on anything it did not produce. This is
  Certificate Transparency's and Sigstore's monitor idea at the scale of one owner. With it, the
  window F1 leaves is "the time to notice an alert plus the trip to the bank", which the hold covers.

The hold delays an urgent fix by a day for households who opted in to automatic installs; exploit
timelines are days (security-updates.md §1), so a day is affordable. The owner may prefer a shorter
hold; 24 hours is a recommendation, not a derivation.

### F5 (high). First use pins a name where it should pin an identity

- **Shipped fingerprints must be an identity, and the core must walk from it.** D391 §7 ships "the
  root set's fingerprints" beside the suggested issuer. If the core treats the issuer as the identity
  and the fingerprints as a check, a core shipped with root v1 that enrols after the set moved to v3
  must verify v2 and v3 from v1: the shipped value is a *starting point of a chain*, not the current
  set, and every location must serve the whole chain (TUF's `N.root.json`). The identity of a
  discovery should be **the SHA-256 of its first (genesis) root statement**, a value that never
  changes however the keys rotate and whatever names it holds; call it the discovery id. Orpheum ships
  the paid service's genesis statement itself, verbatim.
- **The suggested issuer must not decide anything.** If `orpheum.cc` is lost (the very hedge D391 is
  for), a new core typing it must fail against the shipped identity, not pin whatever the new holder
  serves. With the identity shipped, it does.
- **An own discovery is pure TOFU as written.** The admin types an issuer; the core fetches a root set
  from it and pins it. Anyone who controls that name's DNS or TLS at that moment (a hijacked domain, a
  mis-issued certificate, a lookalike the admin was told to type) is pinned. The fix costs nothing in
  the flow D310 §7 already has: the discovery's command line that prints the core enrolment key prints
  the discovery id in the same string (`https://media.example.org#od1-…`), and the core refuses a root
  whose genesis hash differs. Typed without an id, the core shows the id and the words "compare this
  with what your discovery's operator shows you" before enrolling; that is honest TOFU, chosen.
- **What an attacker can do at enrolment** under the corrected design: with the admin's password, make
  the core trust his discovery (out of scope: he is the admin); without it, nothing (enrolment is the
  core's admin API only, never a page, the domain-assumptions audit §4.4); with the domain or its TLS
  at enrolment, nothing for the paid service (identity shipped) and nothing for an own discovery given
  its id; with a phishing page that tells the admin to type another issuer, a core enrolled with an
  unknown discovery, which the System page then names as "not the Orpheum service" rather than by its
  name alone.
- **RFC 8414's exact match conflicts with fallbacks.** D391 §3 refuses a document unless the issuer
  matches exactly, and also serves "the same signed document" at fallback locations, whose names are
  not the issuer. Under key identity, the check that matters is the discovery id and the signature;
  the location check becomes: at enrolment, the location fetched from must be the document's issuer or
  one of its listed locations (a document cannot be lifted to a name it does not claim); afterwards,
  any listed or sticky location (F6) serves.

### F6 (high). An address move can hide the recovery (eclipse)

A thief with an everyday key signs an address document whose locations are all his. Cores follow it and
from then on fetch root statements only from him, so the owner's recovery statement, published at the
honest locations, never reaches them. Fix: **root statements are looked for at sticky locations**, the
union of the compiled-in locations (paid service), the enrolment location, and every location any
accepted address document named in the last 365 days (capped at 16, oldest dropped). The thief can add
locations; he cannot make a core stop asking the old ones within a year.

### F7 (high). The formats have no version, criticality, roles or thresholds

Whatever the first core pins must be readable by it forever, and the first statement that adds a field
it does not know must neither lock it out nor be silently half-understood. Needed in the very first
format (§3.1): a header naming the object type and format version; a rule for unknown lines (ignorable
unless marked critical, X.509's extension criticality in one character); roles and per-role thresholds
written out even though every threshold is 1 (the owner can adopt a quorum later by a statement, not a
format change); keys written as whole OpenSSH public keys (type first) so a new algorithm is a new
line, not a new format; and the reserved delegation line (F15).

### F8 (high). Verify the bytes served, with the namespace and key you expect

- **The release answer today cannot be signed.** `POST /release` returns `release.Record.For(version)`,
  a value derived per core **[V]** (`discovery/release/release.go`). A signature over the record does
  not cover that answer. The discovery must hand the core the signed record's exact bytes and
  signatures, and the core computes `For` itself after verifying.
- **SSHSIG pitfalls**, each a known way to get signature checking wrong: taking the namespace from the
  signature instead of from the object type expected; trusting the public key embedded in the SSHSIG
  blob instead of comparing it, byte for byte, with a pinned key; accepting any `hash_algorithm`
  (accept `sha512` and `sha256` only); accepting a `SIG_VERSION` other than 1 (`PROTOCOL.sshsig`
  requires verifiers to reject newer); and verifying one representation while acting on another.
- **JSON pitfalls**, if JSON were kept: Go's `encoding/json` matches field names case-insensitively and
  takes the last of duplicate keys, other parsers the first, so one signed file can mean two things to
  two readers (Bishop Fox, JSON interoperability). Canonicalisation (RFC 8785) is unnecessary when the
  exact bytes are signed and served, and harmful when it tempts anyone to re-serialise before
  checking. The corrected design uses a strict line format instead (§3.1), which also lets the install
  script read a verified record in plain `sh` (F10) and lets the owner read, at the ceremony, exactly
  what the token signs.

### F9 (medium). Rollback, freeze, mix-and-match, and clocks

TUF's list, against D391:

- **Rollback** (an old valid object served): D391 orders root statements by version but says nothing
  for address documents and records. Each object needs a sequence that only rises, and the core keeps
  the highest it has accepted per (identity, type).
- **Fast-forward**: a stolen everyday key signs a record at sequence 2^63, and after recovery the
  owner can never publish a higher one (TUF spec §5.3.11 exists for exactly this). Fix: every
  document and record names the root version it was signed under, and the core orders by (root
  version, sequence); the recovery statement's higher root version resets every floor. An object
  claiming a root version above the core's is refused until the chain catches up.
- **Freeze** (the current objects withheld, so a security fix stays hidden): the core asks every
  location and takes the highest; records carry `expires` (30 days after `issued`) and address documents
  `expires` (one year). An expired object stays in force (nothing newer is known) but the System page
  says "this server cannot confirm it is up to date since <date>". The owner re-signs the record at
  each release, or monthly when there is none (one touch). An online freshness key (TUF's timestamp
  role) would shorten that to days; overkill now (F15).
- **Mix-and-match** (a record of one discovery or chain shown with another's): every object names its
  identity (discovery id or publisher id) and type, and the namespace differs per type, so no
  signature crosses objects.
- **Clocks**: the cores' clocks may be wrong, so no acceptance decision depends on the core's
  wall-clock time. Time decides only warnings (expiry) and the install hold, the latter on the
  monotonic clock. A clock far ahead shows a stale warning; a clock far behind hides one. Neither
  accepts anything forged.

### F10 (medium). The install runs the image before verifying it

The household command is `docker run --rm ghcr.io/kstenerud/orpheum:<tag> install-script | sh` **[V]**
(security-updates.md §6, D314): the image is pulled by a tag and run before anything checks it, so the
check D391 §6 promises ("installs check the digest against the signed record") would be run by the very
code it should judge. Fix: a short bootstrap shell script that holds the publisher's root keys,
fetches the signed record, verifies it with `ssh-keygen -Y verify` (OpenSSH ≥ 8.2, on every current
distribution), reads the latest's digest, pulls by digest, and only then runs that image's own install
script. The bootstrap's own integrity is the usual one (HTTPS from the owner's site, its hash printed in
the documentation and on the release page). Note F3: `ssh-keygen` cannot check UV, so the shell path is
touch-only strength; `alwaysUv` on the tokens closes that, and every later update goes through the
Go verifier, which checks UV.

### F11 (medium). A released version's digest must never change

`release.Record.Released` gives the latest release a new digest when the workflow re-runs **[V]**
("The latest released again takes its digest anew"). Once signed, `v0.4.0 → sha256:A` must hold forever;
a record naming `v0.4.0 → sha256:B` is equivocation, and a core that knew A should refuse B and tell its
admins. A rebuilt release takes a new version. The signing tool refuses to change a published digest.

### F12 (medium). A restore rolls the pin back

Backups are a tarball of the persistent directory **[V]** (`internal/core/backup/backup.go`), which
holds `discovery.json` and will hold the trust state. A restore from a backup older than a recovery
statement brings back a set that still contains the removed key. Fix: the trust state carries a
"caught up" mark that a restore clears; until the core has fetched the chain from a location again, it
accepts no address move and installs nothing.

### F13 (medium). All keys lost, and keys never tested

Three YubiKeys in three places protect against loss well, but a fire at home while carrying the third
on a trip, or a firmware or middleware change in ten years, are real. And no procedure in D391 proves
each key still works and its PIN is still known until the day it is needed. Fix: a second recovery key
at a second site (§3.6), and a yearly drill in which every key signs a test statement (§4 step 12).

### F14 (medium). Bind core signatures to the identity, not the name

D391 §4 binds a core's signatures to the issuer so the API can move. The issuer itself can move too (a
newer document names a new issuer when a name is lost), and then every signature's binding changes with
it. `proof.Message` should bind the **discovery id** (stable for life) in place of the base URL **[V]**
(`discovery/proof/proof.go` binds `baseURL` today). A discovery cannot then relay a core's challenge
answer to another discovery, which is the property the binding exists for.

### F15 (low). The online delegated key

The brief has a root key delegating to an online key with a validity window; D391's text has none.
Scope it now in the format or it will be scoped by whoever builds it:

- **What it may sign:** only a *freshness* statement, `{identity, root version, the hashes and
  sequences of the current address document and release record, issued, expires ≤ 7 days}`. Never root
  statements, address documents or release records: the online key lives on the discovery's server, and
  a key on that server must not be able to do more than serving from that server already can.
- **Lifetime:** the delegation line in the root statement carries `expires` (at most a year); revoked
  by a root statement without it, or by its expiry.
- **What its compromise gives:** an attacker can vouch that stale-but-valid objects are fresh, for
  cores that cannot reach any honest location. No rollback (floors), no forgery, no move.
- **Verdict:** overkill for the beta and probably for the public launch: the freeze it closes is
  already narrowed to "every location withholds", and a monthly re-sign covers the rest. Reserve the
  `delegate` line and the namespace; build nothing.

### F16 (low). Algorithms and the tokens

- **`ed25519-sk` over `ecdsa-sk`.** `ed25519-sk` needs YubiKey firmware 5.2.3 or later; older keys do
  only `ecdsa-sk`. YubiKey 5 firmware before 5.7.0 is subject to EUCLEAK (YSA-2024-03), an
  electromagnetic side channel in Infineon's ECDSA that recovers an ECDSA private key given physical
  possession, hours and a lab. Ed25519 signing does not use the leaking modular inversion. For a bank
  key that could be borrowed and returned, use `ed25519-sk`, and `ecdsa-sk` only on a key that cannot
  do Ed25519; check with `ykman info`. The verifier accepts both, plus plain `ssh-ed25519` for the paper
  key (§3.6), and nothing else (no RSA, no DSA).
- **Application string:** OpenSSH requires it to start with `ssh:`; use `ssh:orpheum-root` for these
  credentials so they are distinct from any login or git-signing credential on the same token. It is
  part of the public key blob, so pinning the key pins it.
- **Counter:** x/crypto does not check it; a YubiKey's counter is not per credential and gives no
  useful clone signal here. Record it in the watcher's ledger; enforce nothing.
- **PINs:** distinct per key, and the recovery keys' PINs never typed outside home. One shoulder-surfed
  PIN must not unlock all three.
- **Never in an agent:** the root credentials are never added to `ssh-agent` or forwarded; the signing
  tool calls `ssh-keygen -Y sign -f <stub>` directly.
- **Touch-only acceptable?** Not for root keys: D391's own argument rests on the PIN (F3).

### F17 (low). Equivocation is seen only by a core that sees both

Two different statements of one version, each validly signed, reach different cores; each core is
consistent and nobody knows. Fix: the heartbeat carries the core's pinned root version and the hash of
its statement, and the discovery's operator panel shows any core whose pin differs from the published
chain. A transparency log proper (Rekor, a Merkle log) is overkill.

### F18 (low). The build works from the older text

The worktree building D391 has the decision as of an earlier commit, without the paragraph on root-set
statements that an earlier commit added, and no code of the design yet **[V]**. The build should be paused or
rebased onto whatever the owner decides from this audit before any format is written.

### What is sound

- **Names as locations, identity as keys**, and an own discovery depending on nothing of the owner's
  (D391 §2, §8): sound, and the field's practice (TUF, Debian's archive keys, Tor's onion identities).
- **Hardware-resident roots, offline** (D391 §2): sound; Sigstore's root and TUF's guidance keep root
  keys on hardware tokens, offline.
- **SSHSIG with `ssh-keygen`** (D391 §2): sound and cheap. No new dependency: the verifier is
  golang.org/x/crypto/ssh (allowed in `deps/go.allow`) plus about a hundred lines that build the
  `SSHSIG` signed data; the shell side needs only OpenSSH.
- **The Mac's passkey anchors nothing** (D391 §2): sound. An account compromise yields enrolment keys
  for cores (D310), not root authority.
- **Digests, not registries, are trusted** (D391 §6): sound; a digest pull is content-addressed, so a
  registry is only a mirror. Sign the multi-architecture index digest, not a per-platform manifest, so
  arm64 stays open.
- **Refusing a root statement signed by a key outside the pinned set, and catching up in order**:
  sound, and TUF's §5.3 exactly, once the recovery skip (§3.4) is added.

### What the design does not defend, by choice

- A thief with a recovery key and its PIN (the bank box and a PIN never typed outside home).
- A malicious build: the owner signs what the release workflow built (github-integrity rec. 7's second
  build is the mitigation, outside this design).
- The browser path: the loader at the typed name is served by whoever holds the name and TLS; the
  root keys protect cores and installs, not web pages (the core's signed client manifest, D308 §6,
  protects the client's code).

## 2. Answers to the nine questions

1. **Trust on first use. Fix needed.** The paid service: ship the genesis root statement (identity =
   its SHA-256) and walk the chain from it; never pin by name. An own discovery: the enrolment key
   string carries the discovery id; without it, explicit TOFU with the id shown for comparison. At
   enrolment an attacker without the admin's password can do nothing; with the name or its TLS,
   nothing against a known identity (F5).
2. **Any-one-key and sticky revocation. Fix needed.** Recommended: everyday and recovery tiers, each
   signing alone; recovery statements may skip versions and alone change the recovery tier; plus the
   24-hour install hold and the owner's watcher. Rejected: delay-and-veto (symmetric, clock-bound);
   "signer stays in the set" as a theft defence (it is kept, as a lock-out-by-mistake defence). The
   owner's recovery path is the bank, and it always works (F1, §3.4).
3. **Rollback, freeze, mix-and-match. Fix needed.** Order by (root version, sequence) per identity
   and type; expiry warns, never refuses; every location asked, the highest taken; no acceptance
   decision on the core's clock; type and identity inside every payload and namespace (F9).
4. **Online delegation. Overkill now.** If ever built: freshness statements only, never root,
   address or release; at most a year's delegation and seven days' statements; its compromise gives a
   bounded freeze and nothing else (F15).
5. **Domain separation. Fix needed.** One SSHSIG namespace per object type and chain; the payload
   repeats type and identity; exact bytes signed and served; a strict line format instead of JSON
   (F8, §3.1).
6. **Algorithms and FIDO flags. Fix needed.** `ed25519-sk` (firmware ≥ 5.2.3; ≥ 5.7 preferred),
   `ecdsa-sk` only where Ed25519 is impossible; `-O resident -O verify-required
   -O application=ssh:orpheum-root`; `alwaysUv` where supported. x/crypto checks UP only: Orpheum
   checks UV itself from the signature's flags. The application string is pinned with the key; the
   counter is recorded, not enforced. Touch-only is not acceptable for root keys (F3, F16).
7. **Release records. Fix needed.** A separate publisher root; version-to-digest immutable;
   multi-architecture index digest; pull locations are mirrors (integrity is the digest); freeze
   narrowed by expiry and many locations; the install verified by a bootstrap with `ssh-keygen -Y
   verify` before any image runs (F2, F10, F11).
8. **Backups and loss. Fix needed.** A second recovery key at a second site: a fourth YubiKey is
   best; a passphrase-protected paper `ssh-ed25519` key is acceptable and has the merit of needing no
   FIDO middleware in ten years. Total loss means a new identity and every admin re-enrolling, so it
   must be made unlikely, and drilled yearly (F13, §3.6, §4).
9. **What must be right from the start.** The format header and versioning, criticality, roles and
   thresholds, key identifiers (OpenSSH SHA-256 fingerprints), the discovery and publisher ids as
   genesis hashes, the namespaces, the ordering key (root version, sequence), sticky locations, the
   separate release root, the binding of core signatures to the id, and the trust state stored
   verbatim and re-verified at load (F5, F7, F12, F14, §3).

## 3. The corrected design

### 3.1 Signed objects: format

Every signed object is a UTF-8 text payload and a detached signature file:

- `<path>`: the payload, exactly as signed. Lines end in LF; the last line ends in LF; no CR, no tab,
  no trailing space, no empty line, at most 64 KiB. Each line is `<key> <value>`, the key from the
  type's fixed list, in the order given below, repeated only where marked `*`.
- `<path>.sig`: one or more armored SSHSIG blocks (`-----BEGIN SSH SIGNATURE-----`), concatenated.
- **Line 1** is `orpheum-signed <type> <format>`, today `orpheum-signed root 1`, `… address 1`,
  `… release 1`. A core refuses a format number it does not know, and tells its admin to update.
- **Unknown keys:** a line whose key the core does not know is ignored, unless the key begins with `!`
  (critical), in which case the core refuses the object and tells its admin to update Orpheum. New
  lines that only grant (a new key type, a delegation) are added plain; new lines that restrict are
  added critical.
- **Times** are RFC 3339 UTC with seconds and `Z`. **Ids** are `od1-` (a discovery) or `op1-` (a
  publisher) followed by the lowercase RFC 4648 base32, unpadded, of the SHA-256 of the genesis root
  payload. **Key ids** are OpenSSH fingerprints, `SHA256:<base64>`, of the public key blob, as
  `ssh-keygen -l` prints them, so the owner can compare them by eye.

**Namespaces** (SSHSIG): `orpheum-discovery-root-v1`, `orpheum-discovery-address-v1`,
`orpheum-publisher-root-v1`, `orpheum-release-v1`, reserved `orpheum-discovery-freshness-v1` and
`orpheum-drill-v1` (drills, accepted by nothing). The namespace must match the payload's type and
chain; a verifier takes it from the type expected, never from the signature.

**Root statement** (one format for the discovery's and the publisher's chains):

```
orpheum-signed root 1
chain discovery                      # or: publisher
id od1-…                             # absent in the genesis statement (its hash is the id)
version 7
issued 2026-11-01T12:00:00Z
key recovery uv sk-ssh-ed25519@openssh.com AAAA… r1      *
key recovery - ssh-ed25519 AAAA… r2                       *
key everyday uv sk-ssh-ed25519@openssh.com AAAA… e1       *
key everyday uv sk-ssh-ed25519@openssh.com AAAA… e2       *
threshold recovery 1
threshold everyday 1
removed SHA256:…                                          *  (cumulative: every key ever removed)
location https://orpheum.cc                               *  (where this chain is served; discovery chain only)
```

`uv` means every signature by that key must carry the user-verified flag; `-` is for a plain key. The
comment (`r1`, `e1`) is a neutral label: the file is public, so it says which tier, not which drawer. A
key line whose type the core does not know is kept and unusable by that core. An own discovery with one
key lists it twice, once per tier.

**Address document** (discovery chain only; D391 §3's fields):

```
orpheum-signed address 1
id od1-…
root-version 7
sequence 12
issued 2026-11-01T12:00:00Z
expires 2027-11-01T12:00:00Z
name Orpheum
issuer https://orpheum.cc
location https://orpheum.cc                               *
location https://<fallback>                               *
core-api https://api.orpheum-app.com/core/v1
client-api https://api.orpheum-app.com/client/v1
loader-origin https://orpheum.cc
client-origins https://{core}.orpheum-client.com
account-origin https://account.orpheum.cc
homepage https://orpheum.mov
support support@orpheum.cc
```

**Release record** (publisher chain; one line per fact, readable by `awk` after `ssh-keygen` verified
it):

```
orpheum-signed release 1
id op1-…
root-version 3
sequence 57
issued 2026-11-02T09:00:00Z
expires 2026-12-02T09:00:00Z
latest v0.4.0
image v0.4.0 sha256:<64 hex, the multi-architecture index>   *
image v0.3.2 sha256:…                                       *
pending v0.3.2 v0.4.0 <one sentence for the admin>          *  (version, fixed in, summary to end of line)
pull ghcr.io/kstenerud/orpheum                              *
pull <mirror>                                               *
location https://<where this record is served>             *
```

Paths at every location: `/.well-known/orpheum-discovery` (the address document) and `…/root/<n>`,
`…/root/latest` for the discovery chain; `…/release`, `…/publisher-root/<n>`, `…/publisher-root/latest`
for the publisher's; each with its `.sig`. Every location serves every version of each chain, forever.

### 3.2 Verifying one signature

Given payload bytes `P`, signature file `S`, the expected namespace `N`, and a set of acceptable keys
`K` (each with its `uv` mark):

1. Parse each armored block in `S`; for each: magic `SSHSIG`, `SIG_VERSION` exactly 1, then
   `publickey`, `namespace`, `reserved` (ignored), `hash_algorithm`, `signature`.
2. Refuse the block unless `namespace == N` (from the expected type, not from the block).
3. Refuse unless `hash_algorithm` is `sha512` or `sha256`.
4. Refuse unless `publickey` is byte-for-byte equal to a key in `K`, and that key's type is one of
   `sk-ssh-ed25519@openssh.com`, `sk-ecdsa-sha2-nistp256@openssh.com`, `ssh-ed25519`.
5. Build `D = "SSHSIG" ‖ string(N) ‖ string("") ‖ string(hash_algorithm) ‖ string(H(P))` and call the
   key's `Verify(D, sig)` (x/crypto; for sk keys it also requires the user-present bit).
6. If the key is marked `uv`, parse the signature's trailing fields (`byte flags ‖ uint32 counter`) and
   refuse unless `flags & 0x04`.
7. The block's signer is that key. An object is signed by a role when some block's signer holds that
   role, up to the role's threshold of distinct keys.

### 3.3 The core's trust state

One file per chain the core follows, `trust-discovery` and `trust-publisher`, in the persistent
directory, written atomically (`instancedir`'s write-then-rename, which it does today **[V]**):

- every accepted root statement, verbatim with its signatures; the current address document and
  release record, verbatim; the floors `(root version, sequence)` per type; the sticky locations with
  the date each was last named; the digests of every version the core has seen named; and a
  `caught-up` mark.
- On load the core re-verifies the whole chain from the genesis it holds (cheap) and refuses to use a
  state that does not verify.
- A restore clears `caught-up` (F12). A Leave removes the discovery's file; the publisher's stays.
- Only the admin's enrolment (with the admin's password) creates the discovery file; nothing a
  discovery says can.

### 3.4 Following a root chain

Pinned set `S_n` at version `n`, its removed set `R_n`. At start and every 6 hours, and before using a
new address document or record that names a root version above `n`:

1. Ask every sticky location for `root/latest`, then for `root/n+1`.
2. For each candidate statement `T` at version `v > n`: it must parse, name the same chain and id, and
   verify (§3.2) under keys of `S_n` not in `R_n`.
3. **Who may sign what.** If `v == n+1`: an everyday or recovery signer of `S_n` may sign it, unless it
   changes recovery keys, thresholds or roles, which only a recovery signer may. If `v > n+1`: only a
   recovery signer of `S_n` (the skip that makes recovery possible from any point).
4. **Removed stays removed:** `T`'s removed set must contain `R_n` and every key of `S_n` absent from
   `T`; no key of `T` may be in its removed set.
5. **Thresholds:** each role's threshold of distinct signers, today 1.
6. **No lock-out by mistake:** if the signer is not in `T`'s set, `T` must also carry a signature by a
   key of `T` of the same role (TUF's "the new root signs itself"), proving the new keys are held.
7. Accept `T` (persist, then act), set `n = v`, and repeat from step 1.
8. **Conflict:** two different valid statements at the same version: a recovery-signed one wins over an
   everyday-signed one; otherwise refuse both, stay at `n`, tell every admin on the System page and in
   the heartbeat, and keep asking (a recovery statement at a higher version resolves it).
9. After accepting any statement, drop the current address document or record if its signer is no
   longer in the set, and lift each type's floor to the new root version.

### 3.5 Address documents and release records

1. Fetch from every location (address: the document's and the sticky ones; record: the record's and
   the compiled-in publisher locations); verify (§3.2) under the chain's current everyday or recovery
   keys.
2. The payload's id and type must be the chain's; its `root-version` must be ≤ `n` (else catch up
   first, §3.4).
3. Accept only if `(root-version, sequence)` is above the floor; equal bytes are a no-op; the same
   pair with different bytes is equivocation: refuse, tell the admins.
4. A record that names a different digest for a version the core has seen named is refused and told
   (F11).
5. `expires` past (core's clock): keep using it, and say on the System page that the server cannot
   confirm it is current since then.
6. A core never installs a version lower than or equal to its own, and installs automatically only
   the `latest` it has known for 24 hours on its monotonic clock (F4).
7. Enrolment (§3.8) is the only time a location not already known is fetched.

### 3.6 Rotation and recovery procedures

The root set for the paid discovery and for the publisher: everyday `e1` (carried), `e2` (home);
recovery `r1` (the bank's YubiKey), `r2` (a second recovery key at a second site: a fourth YubiKey, or
a paper `ssh-ed25519` key protected by a passphrase kept apart from it).

| Event | Procedure | Signs |
| --- | --- | --- |
| `e1` lost or broken | statement `n+1`: remove `e1`, add new `e3` (the new key also signs, §3.4 step 6) | `e2` |
| `e1` stolen, PIN safe | the same, at once | `e2` |
| `e1` stolen with PIN | statement at `n + 1000`: remove `e1` and anything added since the theft, re-add `e2` or a new key | `r1` (or `r2`) |
| both everyday keys lost | statement at `n+1` (or a skip): two new everyday keys | `r1` |
| `r1` lost | statement `n+1`: remove `r1`, add a new recovery key | `r2` |
| `r1` stolen with its PIN | the same, at once, from `r2`; any conflict resolved by the version skip | `r2` |
| all keys lost | new genesis, new id; a release ships the new genesis; every admin re-enrols; the publisher's loss means every admin installs a release by hand | nothing can |

After any recovery: publish the statement at every location, re-sign the address document and record
under the new root version, check the watcher shows every location consistent, and check the operator
panel for cores whose pinned version lags.

### 3.7 The signing tool

A Go program beside `cmd/release-record` (one more under `cmd/`, run by the owner only): it builds a
payload from a typed edit (add key, remove key, new release, re-sign), prints the payload and its
difference from the current one, asks for confirmation, runs `ssh-keygen -Y sign -n <namespace> -f
<stub>` (one touch, one PIN), verifies the result with the same code the core uses, appends payload and
signature to the owner's ledger (a private git repository on nas.example), and writes the files to publish.
The release workflow produces the unsigned record; the owner signs it on the Mac after checking the
digest (github-integrity rec. 7's second build, when it exists). The owner should check, on a second
machine, that the published bytes read as intended before they go out: a token has no screen.

### 3.8 Enrolment

1. The admin enters an issuer (default the suggested one) and optionally an id (`…#od1-…`), with the
   admin's password, at the core's own admin API.
2. The core fetches the address document and the chain from that location.
3. Paid service: the chain must start at the compiled-in genesis. Own discovery with an id: the
   genesis' hash must equal it. Without one: the core shows the id and asks the admin to compare it
   with what the operator shows.
4. The chain is followed (§3.4) from genesis to latest; the document verified (§3.5); the location
   fetched from must be its issuer or one of its locations.
5. The System page names the discovery by its document's name *and* whether it is the known Orpheum
   service; the trust file is written; enrolment proceeds as today, signing with the id bound (F14).

### 3.9 The install

A bootstrap `sh` script from the owner's site holds the publisher's genesis keys and the current set's
keys (as an `allowed_signers` file with `namespaces="orpheum-release-v1"`), fetches the record and its
signature, runs `ssh-keygen -Y verify -f allowed_signers -I orpheum -n orpheum-release-v1 -s
release.sig < release`, reads the `latest` and its `image` line with `awk`, pulls by digest, and only
then runs that image's `install-script`. Later updates go through the core's verifier (UV checked).

## 4. The owner's day-one ceremony

On the Mac, offline for steps 5–8 if practical; an hour.

1. **Check the tokens.** `ykman info` on each: firmware 5.2.3+ (5.7+ preferred, EUCLEAK); note serial
   numbers. A key below 5.2.3 is a recovery spare only with `ecdsa-sk`, or retired.
2. **Set distinct FIDO2 PINs** (`ykman fido access change-pin`), at least 8 characters; the recovery
   key's never typed outside home. On 5.7+ firmware, enable always-UV (CTAP 2.1 `alwaysUv`; `ykman
   fido config toggle-always-uv` in current ykman, to be checked against `ykman fido config --help`)
   **[I]**.
3. **Make each credential** (`e1`, `e2`, `r1`):
   `ssh-keygen -t ed25519-sk -O resident -O verify-required -O application=ssh:orpheum-root -O user=<label>
   -O write-attestation=<label>.att -C <label> -f orpheum-root-<label>`. Keep the attestation files:
   they show the keys were made inside genuine YubiKeys.
4. **Make the second recovery key** `r2`: preferably a fourth YubiKey the same way; else on an offline
   machine `ssh-keygen -t an earlier commit -a 200 -C r2 -f orpheum-root-r2` with a long passphrase, printed (the
   private key file as text and a QR code) and the file then destroyed; the passphrase written down and
   kept apart from the paper.
5. **Record the fingerprints** (`ssh-keygen -lf orpheum-root-*.pub`) on paper, one copy kept with each
   key, and in the repository.
6. **Write the two genesis statements** (discovery and publisher) with the signing tool; read every
   line before touching.
7. **Sign each genesis with `e1`**, and add a signature from every other key (not required by the
   rules, but it proves each key and PIN work before they leave the desk). Verify each with the tool
   and with `ssh-keygen -Y verify` on a second machine.
8. **Drill each key**: sign and verify one `orpheum-drill-v1` statement each, so a wrong PIN or a dead
   key is found now.
9. **Compute the ids** (`od1-…`, `op1-…`) and put the genesis statements and ids in Orpheum's one Go
   file (D391 §7); commit with the fingerprints in the message.
10. **Publish** the chains, the first address document and the first record at every location; start
    the watcher on nas.example with the ledger.
11. **Distribute**: `e1` carried, `e2` at home, `r1` to the bank, `r2` to its own site (a relative's, or
    a second deposit box), each with its fingerprint card and a one-page instruction ("what this key is
    for, how to use it, whom to call"), for the person who might one day need to act for the owner.
12. **Every year**: fetch each key, sign a drill statement, check the envelopes, re-sign the address
    document before it expires.

## Sources

- The Update Framework specification (client workflow §5.3, root rotation, fast-forward recovery
  §5.3.11, expiry and versions): https://theupdateframework.github.io/specification/latest/ ; Samuel,
  Mathewson, Cappos, Dingledine, *Survivable Key Compromise in Software Update Systems* (CCS 2010):
  https://theupdateframework.io/papers/survivable-key-compromise-ccs2010.pdf ; Cappos et al., *A Look In
  The Mirror: Attacks on Package Managers* (CCS 2008): https://theupdateframework.io/papers/attacks-on-package-managers-ccs2008.pdf
- Uptane Standard (full and partial verification, the director/image repository split):
  https://uptane.github.io/docs/standard/uptane-standard
- Sigstore root signing (offline hardware-token root, ceremony, TUF trust root):
  https://github.com/sigstore/root-signing
- OpenSSH `PROTOCOL.sshsig`: https://github.com/openssh/openssh-portable/blob/master/PROTOCOL.sshsig ;
  `PROTOCOL.u2f`: https://github.com/openssh/openssh-portable/blob/master/PROTOCOL.u2f ; `sshsig.c` and
  `ssh-keygen.c` (flags not checked at verify): https://github.com/openssh/openssh-portable ;
  `ssh-keygen(1)` (FIDO `-O` options, allowed signers, default `sha512`): https://man.openbsd.org/ssh-keygen.1
- golang.org/x/crypto v0.58.0 `ssh/keys.go` (`skEd25519PublicKey.Verify`, `flagUserPresence`), read
  locally from the module cache: https://pkg.go.dev/golang.org/x/crypto/ssh
- Yubico, *Securing SSH with FIDO2* (ed25519-sk needs firmware 5.2.3+; resident, verify-required):
  https://developers.yubico.com/SSH/Securing_SSH_with_FIDO2.html ; *Git commit signing with SSH and
  FIDO2*: https://developers.yubico.com/SSH/Securing_git_with_SSH_and_FIDO2.html
- Yubico security advisory YSA-2024-03 (EUCLEAK): https://www.yubico.com/support/security-advisories/ysa-2024-03/ ;
  NinjaLab, *EUCLEAK*: https://eprint.iacr.org/2024/1380
- FIDO CTAP 2.1 (UP and UV flags, credProtect, alwaysUv):
  https://fidoalliance.org/specs/fido-v2.1-ps-20210615/fido-client-to-authenticator-protocol-v2.1-ps-20210615.html
- RFC 8414 §3.3 (issuer match): https://www.rfc-editor.org/rfc/rfc8414 ; RFC 8785 (JSON
  canonicalisation): https://www.rfc-editor.org/rfc/rfc8785 ; RFC 8032 (Ed25519):
  https://www.rfc-editor.org/rfc/rfc8032 ; RFC 4648 (base32): https://www.rfc-editor.org/rfc/rfc4648 ;
  RFC 9162 (Certificate Transparency 2.0, monitors): https://www.rfc-editor.org/rfc/rfc9162
- Go `encoding/json` (case-insensitive field matching): https://pkg.go.dev/encoding/json#Unmarshal ;
  Bishop Fox, *An Exploration of JSON Interoperability Vulnerabilities*:
  https://bishopfox.com/blog/json-interoperability-vulnerabilities
