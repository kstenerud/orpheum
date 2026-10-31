# Signing for a discovery with YubiKeys

A discovery is known by a set of root keys, not by its names (D391 §2): a core trusts its address
document and its release record only when a key of the set signed them. Any one key of the set signs
anything, the set's own change included; the others are spares against a loss. This guide is for a
discovery's owner: the paid service's, whose three keys live on three YubiKeys, or an own discovery's,
whose set may be one key.

Everything here runs on the owner's own computer, with OpenSSH 8.2 or newer (`ssh -V`) and the
Orpheum source (`go run ./cmd/discovery-sign`). No private key leaves a YubiKey.

## Once per YubiKey: make its key

Plug in one YubiKey and run:

```sh
ssh-keygen -t ed25519-sk -O resident -O application=ssh:orpheum -O verify-required \
  -C "orpheum root 1" -f ~/.ssh/orpheum-root-1
```

- `ed25519-sk` makes the key on the YubiKey (YubiKey firmware 5.2.3 or later). An older one
  takes `-t ecdsa-sk` instead; both are accepted.
- `-O resident` keeps the key's handle on the YubiKey, so any computer can load it later with
  `ssh-keygen -K` or `ssh-add -K`: nothing on this computer is needed to sign again.
- `-O application=ssh:orpheum` names it, so it is told apart from other SSH keys on the same YubiKey.
- `-O verify-required` asks for the YubiKey's PIN as well as a touch for every signature. Set a PIN
  first if it has none (`ykman fido access change-pin`).

Do the same for the second and third YubiKey, with `-C "orpheum root 2"` and `-f
~/.ssh/orpheum-root-2`, and so on. Each `.pub` file is the public key: it is what goes into the root
key set, and, for the paid service, into Orpheum itself.

Keep one YubiKey with you, one at home and one in a bank: any one signs.

## The first root key set

```sh
eval "$(ssh-agent)"     # if no agent runs
ssh-add -K              # touch the YubiKey: loads its resident keys into the agent
go run ./cmd/discovery-sign root-set --sets root-sets.json \
  --add ~/.ssh/orpheum-root-1.pub --add ~/.ssh/orpheum-root-2.pub --add ~/.ssh/orpheum-root-3.pub
```

It writes `root-sets.json`, the chain of the set's statements, signed by whichever of the set's keys
the agent holds (touch it when it blinks), and prints the keys' fingerprints. Keep the file with the
discovery's deployment, and in the repository of whoever runs it: every later step reads it.

**For the paid service only:** put the three `.pub` lines into `RootKeys` in
`internal/core/discovery/suggested.go`. Until they are there, a core refuses to enrol with the
suggested discovery, saying it does not know its keys.

## Losing, replacing or adding a key

Plug in any YubiKey of the set that remains, then:

```sh
ssh-add -K
go run ./cmd/discovery-sign root-set --sets root-sets.json \
  --remove ~/.ssh/orpheum-root-1.pub --add ~/.ssh/orpheum-root-4.pub
```

It writes the next version of the set, signed by the key at hand, and adds it to the chain. Then
sign the address document again (below), so every discovery serves the new chain: a core catches up
from any version it holds, one set after another. The rules for a removed key and for two sets of
one version are being settled by an outside audit (D391 §2); until then a removed key signs no later
set, but is not otherwise remembered.

## The address document

```sh
go run ./cmd/discovery-sign document --sets root-sets.json --out orpheum-discovery.json \
  --issuer discovery.example \
  --core-api https://api.discovery.example/core/v1 --client-api https://api.discovery.example/client/v1 \
  --loader https://discovery.example --client-origins 'https://{core}.client.example' \
  --homepage https://about.example --location https://mirror.example/discovery
```

Serve the file at `/.well-known/orpheum-discovery` at the issuer and at each `--location`
(`discovery/single` serves it from its `AddressDocument` file). Signing again with a later time moves
every core to the new addresses within the hour. A running discovery may sign its own documents with
an online key a root key authorised for a time:

```sh
go run ./cmd/discovery-sign delegation --sets root-sets.json --issuer discovery.example \
  --key online.pub --for 2160h --out delegation.json
```

## The release record

The release workflow writes the unsigned record (`go run ./cmd/release-record`, which prints the
next step). Sign it:

```sh
ssh-add -K
go run ./cmd/discovery-sign record --sets root-sets.json --record release.json --out release.signed.json
```

Publish `release.signed.json` with the release, and put it where each discovery serves it, at
`/.well-known/orpheum-release-record`.

## Without the agent

`ssh-keygen` signs by hand, and `discovery-sign seal` puts the signature with what it signed,
checking it first:

```sh
ssh-keygen -Y sign -n orpheum-release-record-v1 -f ~/.ssh/orpheum-root-2 release.json
go run ./cmd/discovery-sign seal --sets root-sets.json --kind orpheum-release-record-v1 \
  --payload release.json --signature release.json.sig --out release.signed.json
```

On a new computer, `ssh-keygen -K` writes the YubiKey's resident key's handle and `.pub` into the
current directory, for `-f`. An operator with no hardware key uses a plain `ssh-keygen -t an earlier commit
key: `--key-file` names it, if it has no passphrase, or `ssh-add` loads it.
