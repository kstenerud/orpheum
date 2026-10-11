# Contributing to Orpheum

Orpheum is developed in a private repository while its first users test it, and will open for
public betas later ([D192](docs/contributors/decisions/working-notes.md)). These rules hold from
the first commit, so that nothing has to change when it opens.

## Licensing is by directory

| Directory | Licence | Header every source file carries |
| --- | --- | --- |
| everything outside `web/` (server, media node, relay, scripts) | AGPL-3.0-or-later ([`LICENSE`](LICENSE)) | `SPDX-License-Identifier: AGPL-3.0-or-later` |
| `web/` (the SDK, the player, the web client, the cast receiver) | MPL-2.0 ([`web/LICENSE`](web/LICENSE)) | `SPDX-License-Identifier: MPL-2.0` |

The split is deliberate: a GPL-family application cannot ship through Apple's App Store, and MPL's
per-file copyleft keeps the clients open while letting them ship
([D80](docs/contributors/decisions/working-notes.md)). `make check` fails on a file whose header
does not match its directory. **Code must not move from `web/` into the server tree, or the
other way, without the owner's decision**, because that changes its licence.

## The contributor licence agreement

Before your first contribution is merged, you accept the
[Individual Contributor License Agreement](CLA.md) (D275). You keep the copyright in your work; you
give the project's owner a licence to use it under the directory's licence and under other terms,
as Google's and the Apache Software Foundation's agreements do. The owner runs a hosted service
built on Orpheum, and the agreement is what lets your contribution be part of it. If your employer
owns what you write, it signs the corporate agreement first: ask the owner for it.

## Setting up

The simplest way needs only git and a container engine
([D235](docs/contributors/decisions/working-notes.md#d235--development-and-deployment-run-in-containers-one-image-family-one-command-each-built-by-goreleaser)).
Install [Docker](https://docs.docker.com/engine/install/) (Podman should work too), then:

```
git clone <the repository> orpheum && cd orpheum
./dev                  # a shell in the development container; the first run builds its image
./dev make check       # the gate, exactly as it runs everywhere
./dev make dev         # the app on http://127.0.0.1:5173
```

The image (`deploy/Containerfile`) holds every pinned tool; the caches live in named volumes,
so nothing but the clone lands on the machine. `./dev` explains its knobs at the top of the file:
`ORPHEUM_DEV_MEDIA` mounts a media directory read-only, `ORPHEUM_DEV_PUBLISH=0.0.0.0` opens the
dev ports to the LAN, and `./dev --stop` removes the container. The git pre-commit hook runs the
gate through `./dev` too when the machine has no Go or Node of its own.

Without a container, you need Go (go.mod's `toolchain` line names the exact version), Node 22,
Python 3, shellcheck and a C compiler (for the race detector). The gate fetches the pinned
golangci-lint and sqlc into `.tools/` itself.

```
make setup     # git hooks, web dependencies
make check     # the gate; must pass before every commit
```

`make help` lists every target.

## Developing

```
make dev          # the app on http://127.0.0.1:5173, rebuilt on every change
```

The first run fetches the pinned FFmpeg and generates a small synthetic library, then prints
every setting it chose: users `owner`/`owner` and `partner`/`partner`, debug logs, pprof, the
race detector. The choices live in `deploy/dev/core.dev.toml`, `deploy/dev/node.dev.toml` and
`deploy/dev/seed.toml`; all state lives in `.dev/`, in the core's and the node's instances
`.dev/core` and `.dev/node` (D242), which `make dev-reset` and `make dev-clean` throw away. The debugging targets —
`dev-db`, `dev-logs`, `dev-debug`, `pprof`, `probe`, `test-one`, `cover` and more — are in
`make help` and [D197](docs/contributors/decisions/working-notes.md).

## Making a change

- Read [`AGENTS.md`](AGENTS.md). Its rules apply to humans too, and the commit-msg and
  pre-commit hooks enforce the mechanical ones.
- Standards: [`docs/contributors/standards/`](docs/contributors/standards/README.md).
- The build contract for the first release:
  [`docs/contributors/design/plans/cut-a.md`](docs/contributors/design/plans/cut-a.md).
- Before opening a pull request, run `make check-pr` against your base branch.
