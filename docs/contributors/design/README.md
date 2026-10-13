> **ABOUTME:** Index for the design directory — the shaping cluster of the product overview,
> the research that backs it, and the plans that come out of it. Routes; does not inventory.

# Design

Start with **[overview.md](overview.md)** for the product-altitude picture: what we are
building, who it is for, what makes it different, and the architecture that follows.

| Document | What it covers |
| --- | --- |
| [architecture-map.md](architecture-map.md) | Which package does which job, where each kind of data lives and whether it is backed up, the seams between core, node, store and client, and the findings of the M3 closing audit's map. Re-derived at every milestone. |
| [discovery-protocol.md](discovery-protocol.md) | How a core, its clients and a discovery speak: enrolment, sessions, addresses, signalling, names, the paths a client races and how it chooses, the same for the paid service and an owner's own `orpheum discovery`. |
| [media-nodes.md](media-nodes.md) | The hub-and-spoke topology: a core that never touches a media filesystem, and N media nodes that live next to the files and do everything requiring access to them. Consolidates the scanner, the media worker and the transcoder. |

| Directory | What's there |
| --- | --- |
| [research/](research/README.md) | The evidence. Competitor surveys, user sentiment, and the technology evaluations. Every non-obvious decision cites one of these. |
| [plans/](plans/README.md) | Unbuilt work — one file per idea, from `UNSPECIFIED` through `PLANNED` to `IN-PROGRESS`. `ls` it for the complete list. |

Decisions live in [`../decisions/working-notes.md`](../decisions/working-notes.md) and are cited
by `D`-number throughout this tree.

## Review queues

Not yet created. They get added on the yoloAI four-file model (`<topic>-unresolved.md` draining
to `-resolved` / `-deferred` with a `Trigger:` / `-abandoned` with a `Why:`) the first time
there is something to put in one.
