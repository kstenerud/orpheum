> **ABOUTME:** Index for the design directory — the shaping cluster of the product overview,
> the research that backs it, and the plans that come out of it. Routes; does not inventory.

# Design

Start with **[overview.md](overview.md)** for the product-altitude picture: what we are
building, who it is for, what makes it different, and the architecture that follows.

| Document | What it covers |
| --- | --- |
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
