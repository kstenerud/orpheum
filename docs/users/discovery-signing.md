# Signing for a discovery and for releases, with YubiKeys

A discovery is known by its root chain, not by its names (D391, corrected by D393 on the
[root key audit](../contributors/design/research/root-key-audit-2026-10-31.md)): a core trusts its
address document only when a key of the chain signed it, and Orpheum's releases are signed under a
second chain, the publisher's, compiled into every binary. Each chain has two tiers of keys, each
signature one key:

- **Everyday keys** (`e1` carried, `e2` at home) sign address documents, release records, and root
  statements that change the everyday tier alone.
- **Recovery keys** (`r1` in a bank, `r2` at a second site) sign anything, alone change the recovery
  tier, thresholds and roles, and may skip versions past a thief's statements.

Keys of both tiers are added and removed by a signed root statement. A key removed stays named
removed for good, and a key that leaves the set has the new set sign too. The owner's same
YubiKeys hold both chains under separate keys. An own discovery's operator may have one key, listed
once per tier.

Everything here runs on the owner's Mac with OpenSSH 8.2 or newer and the Orpheum source. The tool,
`go run ./cmd/discovery-sign`, writes each payload and says the `ssh-keygen` command that signs it;
you run that command (one touch, one PIN), then `check`, which checks the signature with the code a
core uses, keeps both in your **ledger** (a private git repository, on nas.example), and writes the files
to publish. Root keys are never loaded into `ssh-agent`.

## Day one (the audit's §4)

1. **Check the tokens.** `ykman info` on each: firmware 5.2.3 or later (5.7 or later preferred);
   note the serial numbers.
2. **Set a distinct FIDO2 PIN on each** (`ykman fido access change-pin`), 8 characters or more.
3. **Make each credential**, one per key and per chain (`e1`, `e2`, `r1`; then `pe1`, `pe2`, `pr1`
   for the publisher), for example:

   ```sh
   ssh-keygen -t ed25519-sk -O resident -O verify-required -O application=ssh:orpheum-root \
     -O user=e1 -O write-attestation=e1.att -C e1 -f orpheum-root-e1
   ```

   `-O verify-required` makes every signature carry the PIN's user-verified flag, which a core
   checks; `-O resident` lets any computer get the key's stub back with `ssh-keygen -K`. Keep the
   attestation files.
4. **Make the second recovery key** `r2`: a fourth YubiKey the same way, or on an offline machine a
   plain `ssh-keygen -t an earlier commit -a 200 -C r2 -f orpheum-root-r2` with a long passphrase, printed and the
   file destroyed.
5. **Record the fingerprints** (`ssh-keygen -lf orpheum-root-*.pub`) on paper, with each key.
6. **Write each genesis:**

   ```sh
   go run ./cmd/discovery-sign genesis --ledger ~/ledger --chain discovery \
     --everyday orpheum-root-e1.pub --everyday orpheum-root-e2.pub \
     --recovery orpheum-root-r1.pub --recovery orpheum-root-r2.pub --location https://<issuer>
   ```

   and the same with `--chain publisher` and the publisher's keys. Read every line it shows.
7. **Sign each genesis with every key** (one is enough; every one proves each key and PIN work),
   appending each signature to the same `.sig`:

   ```sh
   ssh-keygen -Y sign -n orpheum-discovery-root-v1 -f orpheum-root-e1 ~/ledger/pending/discovery-root-1
   ```

   then `go run ./cmd/discovery-sign check --ledger ~/ledger --kind root --payload
   ~/ledger/pending/discovery-root-1`, and for the publisher `-n orpheum-publisher-root-v1` and
   `--chain publisher`. Verify each on a second machine with `ssh-keygen -Y verify`.
8. **Drill each key:** `discovery-sign drill --chain discovery`, sign with each key in namespace
   `orpheum-drill-v1`, then `check --kind drill`: it names each key that signed.
9. **Put the genesis statements in Orpheum:** each `root/1` payload and its `.sig`, verbatim, into
   `Suggested().Genesis` and `Publisher().Genesis` in `internal/core/discovery/suggested.go`; commit
   with the identities (`od1-…`, `op1-…`) and fingerprints in the message.
10. **Publish:** `discovery-sign address …` then sign it with an everyday key (namespace
    `orpheum-discovery-address-v1`) and `check --kind address --publish <dir>`; copy `<dir>` to every
    place. Write the install's bootstrap with `discovery-sign bootstrap --out install --location
    https://<publisher place>` and publish it with the SHA-256 it prints.
11. **Start the watcher** on nas.example every 15 minutes: `go run ./cmd/discovery-watch --expected
    ~/ledger/expected --place https://<each place>`; a non-zero exit is an alert.
12. **Distribute the keys:** `e1` carried, `e2` at home, `r1` to the bank, `r2` to its own site.

Every year: sign a drill with each key, and sign the address document again before it expires.

## Rotations (the audit's §3.6)

Each is a `root` command; it shows the statement and who must sign it.

| Event | Command | Signs |
| --- | --- | --- |
| `e1` lost or broken | `root --chain discovery --remove e1.pub --add-everyday e3.pub` | `e2`, and `e3` too |
| `e1` stolen, PIN safe | the same, at once | `e2`, and `e3` |
| `e1` stolen with its PIN | `root --chain discovery --skip 1000 --remove e1.pub --remove <each key the thief added> --add-everyday e3.pub` | `r1` (or `r2`), and `e3` |
| both everyday keys lost | `root --chain discovery --remove e1.pub --remove e2.pub --add-everyday e3.pub --add-everyday e4.pub` | `r1` |
| `r1` lost | `root --chain discovery --remove r1.pub --add-recovery r3.pub` | `r2`, and `r3` |
| `r1` stolen with its PIN | the same, at once, with `--skip 1000` if the thief signed a statement | `r2`, and `r3` |
| all keys lost | a new `genesis`: a new identity; every admin enrols again, and installs a release by hand | nothing can |

The watcher names each statement a thief made and the keys it added. After any rotation: sign the
address document and the release record again under the new version (`address`, `release-record`
with `--root-version`), publish at every place, check the watcher shows every place consistent, and
look for cores whose heartbeat names an older root version.

## Releases

The release workflow writes the unsigned record (`go run ./cmd/release-record`); check its digest,
then sign with an everyday key of the publisher chain, namespace `orpheum-release-v1`, and `check
--kind release --publish <dir>`. A version's digest never changes: a rebuilt release takes a new
version, and `check` refuses a record that changes one.
