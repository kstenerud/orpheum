> **ABOUTME:** Routes to the live plans without listing them — how to find one, and the
> ordering between them that no single plan states.

# Unfinished Work

Every plan for work that is designed, part-built, or merely wanted is a **file in this
directory**. An idea nobody has fleshed out yet is a plan with `**Status:** UNSPECIFIED`, not a
bullet in this README — a backlog kept as README prose is invisible to `ls`, to `grep` and to
every gate.

A plan states where it stands in a metadata list under its title:

| `**Status:**` | Means | Lives in |
| --- | --- | --- |
| `UNSPECIFIED` | An idea. No design yet. | here |
| `PLANNED` | Designed, not built. | here |
| `IN-PROGRESS` | Partly built. The Status prose says what's left. | here |
| `IMPLEMENTED` / `ABANDONED` | Done or dropped. | `archive/plans/` (does not exist yet) |

`- **Depends on:**` names other live plans, or `—`. It is declared one way: "A depends on B"
lives in A. The reverse view is a grep (`grep -l 'Depends on:.*B.md' *.md`).

## Finding a plan

`ls` is the complete list. `head -3 <file>` gives a plan's purpose. `grep '^- \*\*Status:\*\*' *.md`
shows where each stands.

## Sequence

`transcode-core-spike.md` gates everything. It is the one assumption in the design that would
invalidate the plan if false, and nothing else should be built until it is settled.

After it: `library-model.md` and `scanner.md` can proceed in parallel with `stream-service.md`.
`web-client.md` needs `stream-service.md`. `tv-client.md` needs `web-client.md` only for the
shared SDK. `jellyfin-api-shim.md` is independent and may pre-empt `tv-client.md` entirely —
decide that before starting the TV client, not during.

`remote-access.md` and `sharing-model.md` are both `UNSPECIFIED` and both need a research round
before they can be designed. They are the two hardest product questions in the project.
