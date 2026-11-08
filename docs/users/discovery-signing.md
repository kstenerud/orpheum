# Signing keys: the ceremony, lost keys, and releases

Two things in Orpheum are trusted because a key you hold signed them:

- **Which service a core trusts.** A core finds the paid service through its *address document*,
  and takes it only when one of your *discovery* keys signed it. Someone who takes over the website
  cannot point cores somewhere else.
- **Which software a core installs.** Each release has a *release record*, and a core installs a
  release only when one of your *publisher* keys signed its record.

Each of the two has its own keys, so a signature for one never counts for the other. The same
YubiKey holds one key of each. The rules behind this are in D391, D393 and D394, and the
[root key audit](../contributors/design/research/root-key-audit-2026-10-31.md).

Your keys come in two kinds:

- **Everyday keys** sign the routine things: the address document and each release. You carry
  one (`e1`) and may keep a second at home (`e2`). The one change to the keys an everyday key can
  sign is suspending another everyday key, so a thief holding one can never add a key of their own
  or remove yours (D395).
- **Recovery keys** can sign anything: adding and removing keys of either kind, and replacing the
  everyday keys. They are locked away, one at a bank (`r1`) and one at a second site (`r2`). A thief
  who steals an everyday key never holds them, so the recovery keys can always cancel a stolen key.

A stolen YubiKey is useless without its PIN: every signature needs both the key and its PIN.

## What you need

- A Mac with the Orpheum source and Go (macOS's own OpenSSH is new enough).
- Three YubiKeys or more with firmware 5.2.3 or later (`ykman info` shows it), and a sticker on each.
- A PIN of 8 or more characters on each YubiKey, different on each: `ykman fido access change-pin`.
- A **records folder**, kept in a private git repository (`orpheum-security`). Nothing in it is
  secret: it holds what was signed, the keys' public halves, and the files to publish. What matters
  is that it is not lost or changed.

Every command below is run from the Orpheum source as `go run ./cmd/discovery-sign …`. The tool asks
plain questions and runs `ssh-keygen` itself; you type no signing commands. It checks every signature
with the same code a core runs before it keeps anything, so a mistake is caught on the spot, not by a
core later.

**A Mac is not a key.** Its signatures cannot show a core that Touch ID was asked for, so it
protects less than a YubiKey and its PIN (D395 §3). The tool makes no Mac key and asks no question
about one.

## Day one: the ceremony

```sh
go run ./cmd/discovery-sign ceremony --folder ~/orpheum-security
```

It takes about an hour. You can stop at any point and run the same command again: it carries on
from where it stopped, and never makes a key twice. It walks you through:

1. **Which keys you have:** how many YubiKeys, what each is called, and whether each is everyday or
   recovery.
2. **Making the keys:** for each YubiKey, plug in that one alone, then touch it and type its PIN when
   it blinks. Each YubiKey makes two keys, one for discovery and one for publisher. Only the public
   halves leave the YubiKey.
3. **The address document:** the service's address and the addresses it lists, which you confirm or
   change.
4. **The first statement of each set of keys:** the list of keys allowed to sign. Every key signs it
   once, which proves that key and its PIN work before anything depends on them.
5. **Signing the address document** with an everyday key.
6. **Writing the files to publish,** including the install script.

At the end it prints what is left to do by hand:

- paste the two first statements into `internal/core/discovery/suggested.go` and commit them, with
  the identities and fingerprints it prints in the commit message;
- copy the files to publish to the server, so it serves them under `/.well-known/`;
- write each key's fingerprint on paper and keep the paper with that key;
- take the keys where they live: carried, at home, at the bank, at the second site;
- commit the records folder.

Then start the watcher on the production NAS, every 15 minutes:
`go run ./cmd/discovery-watch --expected ~/orpheum-security/ledger/expected --place https://<each place>`.
It checks every place serves exactly what you signed, and a non-zero exit is an alert.

**If a computer loses a YubiKey's key file** (the small "stub" file that points at the key inside
the YubiKey): plug the YubiKey in and run `ssh-keygen -K`, which gets the stub back. This is not yet
confirmed on a real YubiKey.

**Every year:** sign the address document again before it expires, and check each key still
signs (see "By hand" below).

## When a key is lost, stolen, or turns up again

Each of these changes a key in both sets at once, shows you exactly what changes, offers only the
keys allowed to sign it, checks the result as a core would, and writes the files to publish. Name the
key by the name the ceremony gave it.

| What happened | Command | Who signs |
| --- | --- | --- |
| A key is missing but may turn up | `suspend --folder ~/orpheum-security --key e1` | any other key (a recovery key, when the missing one is a recovery key) |
| The missing key turned up | `resume --folder ~/orpheum-security --key e1` | a recovery key |
| A key is gone for good, or stolen | `remove --folder ~/orpheum-security --key e1` | a recovery key |
| A new key replaces it | `add --folder ~/orpheum-security --key e3` | a recovery key |
| Every key is lost | a new ceremony, which is a new identity: every core enrols again, and installs a release by hand | nothing can |

- **Suspending** stops the key's signatures counting until a recovery key resumes it. A key cannot
  sign its own suspension, and only a recovery key can resume one, so a thief holding an everyday key
  cannot undo it.
- **Removing** is for good: a removed key can never be added back. If the key may turn up, suspend
  it instead. Removing and adding need a recovery key, so when an everyday key goes missing, suspend
  it at once with your other everyday key, and remove or replace it at the bank when convenient.
- **After a change,** if the address document was signed by the key that stopped counting, the
  tool signs it again with a key that still counts. If the newest release record was, it says so:
  make a new unsigned record (`release-record --root-version <n>`) and sign it with `sign-release`.
- **Then** copy the files to publish to the server, check the watcher shows every place the same,
  commit the records folder, and look for cores whose heartbeat names an older root version.

**A key stolen together with its PIN** is worse: the thief can sign. Suspend it at once with another
key, then remove it with a recovery key. If the
watcher shows the thief already signed a statement, the next statement must jump past the thief's
with a recovery key; that one is done by hand (below, `--skip`).

## Releases

The release workflow writes an unsigned release record. To sign it:

```sh
go run ./cmd/discovery-sign sign-release --folder ~/orpheum-security --record <the unsigned record>
```

It shows the version, the digest (the fingerprint of the exact software cores will install) and
whether the release fixes a security problem. Compare the digest with the one on the release
workflow's summary page, then type its first 12 characters to confirm. It signs with a publisher
everyday key, checks the result, and writes the files to publish; copy them to the server.

A version's digest never changes: a rebuilt release takes a new version.

## Staging and self-hosters

Staging (`orpheum.dev`) has its own keys, plain key files with no YubiKey (D394 §3), so a leaked
staging key reaches staging alone. They are made by the same ceremony:

```sh
go run ./cmd/discovery-sign ceremony --folder <records folder> --software <keys directory>
```

The keys directory must be outside the records folder and outside every repository; back it up.
Staging's was made on 2026-10-31: records in `orpheum-security/staging`, keys kept apart.

## By hand

Underneath, the tool has plain commands for what the guided ones do not cover, each writing a
statement and checking a signature, run in a ledger directory: `genesis`, `root` (with `--skip`,
`--threshold`, `--location` as well as the key changes), `address`, `drill` (each key signs a test
statement, proving it and its PIN still work), `bootstrap`, `check` and `publish`.
`go run ./cmd/discovery-sign` with no arguments lists them. You sign their output with
`ssh-keygen -Y sign -n <namespace> -f <the key's stub> <file>`, the namespace and file each command
names, and then run `check`.
