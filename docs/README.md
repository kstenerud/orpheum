# Docs

Docs are organised by **role** — the layer you are operating in. Pick your tier.

| Tier | Start here |
| --- | --- |
| **Users** — running the server | [`users/install.md`](users/install.md): installing it; [`users/setup-problems.md`](users/setup-problems.md): problems seen setting up a server, and their fixes; [`users/discovery-signing.md`](users/discovery-signing.md): a discovery's owner signing with YubiKeys; `GUIDE.md` is not written yet |
| **Contributors** — building the thing | [`contributors/`](contributors/) |

The project is in a **closed beta**: its first release shipped on 2026-10-21 and runs in its
owner's home. How it was built, by AI agents under one engineer's direction, is in
[`how-it-was-built.md`](how-it-was-built.md).

Start at [`contributors/design/overview.md`](contributors/design/overview.md) for the
product-altitude picture, and
[`contributors/design/research/`](contributors/design/research/) for the evidence behind it.

Four documents answer four different questions, and it is worth knowing which one you want:

| Question | Document |
| --- | --- |
| **whether, and how much** — every feature, ranked, including the declined ones | [`FEATURES.md`](FEATURES.md) |
| **when** | [`ROADMAP.md`](ROADMAP.md) |
| **how** | [`contributors/design/plans/`](contributors/design/plans/) |
| **why** | [`contributors/decisions/working-notes.md`](contributors/decisions/working-notes.md) |

[`FEATURES.md`](FEATURES.md) is the one to read before freezing a schema or closing an
enumeration: its **retrofit-cost** column, not its ranking, is what says which decisions
foreclose which futures ([principles §12](contributors/principles/12-defer-the-work-never-the-seam.md)).
