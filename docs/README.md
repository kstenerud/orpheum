# Docs

Docs are organised by **role** — the layer you are operating in. Pick your tier.

| Tier | Start here |
| --- | --- |
| **Users** — running the server | [`users/setup-problems.md`](users/setup-problems.md): problems seen setting up a server, and their fixes; [`users/discovery-signing.md`](users/discovery-signing.md): a discovery's owner signing with YubiKeys; `GUIDE.md` is not written yet |
| **Contributors** — building the thing | [`contributors/`](contributors/) |

The project is **pre-implementation**. Nothing has been built. Everything currently in this
tree is research and design: what exists in this space, why it fails its users, and what we
intend to do differently.

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
