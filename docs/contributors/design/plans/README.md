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

`playback-survey.md` comes first. It is days of work and it decides how urgent `transcoding.md`
is — which re-orders everything else in either direction. Nothing about transcoding should be
guessed before it lands.

Then `library-model.md` and `scanner.md` in parallel with `stream-service.md` (direct play
only, per [D17](../../decisions/working-notes.md)). `web-client.md` needs `stream-service.md`;
its real deliverable is the shared SDK. `tv-client.md` needs `web-client.md` only for that SDK.

`transcoding.md` is deliberately late and is two components in one file — compatibility first
(offline, once per file), bandwidth second (live, and coupled to `remote-access.md` rather than
to the library). `playback-survey.md` sets its priority.

`jellyfin-api-shim.md` needs a stable `stream-service.md` and is otherwise independent. It may
reduce the urgency of `tv-client.md` — re-examine that once the adapter exists and its real
fidelity is known, not before.

`remote-access.md` and `sharing-model.md` are both `UNSPECIFIED` and both need a research round
before they can be designed. They are the two hardest product questions in the project.
