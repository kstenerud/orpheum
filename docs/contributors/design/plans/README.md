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
| `DEFERRED` | Researched, and deliberately not being built yet. May carry affordances due now. | here |
| `PLANNED` | Designed, not built. | here |
| `IN-PROGRESS` | Partly built. The Status prose says what's left. | here |
| `IMPLEMENTED` / `ABANDONED` | Done or dropped. | `archive/plans/` (does not exist yet) |

`- **Depends on:**` names other live plans, or `—`. It is declared one way: "A depends on B"
lives in A. The reverse view is a grep (`grep -l 'Depends on:.*B.md' *.md`).

## Finding a plan

`ls` is the complete list. `head -3 <file>` gives a plan's purpose. `grep '^- \*\*Status:\*\*' *.md`
shows where each stands.

## Sequence

`playback-survey.md` comes first, using the web harness from `device-lab.md` on a handful of
representative devices — **not** the full device sweep, which is a separate and later effort.
Conflating the two would turn a days-long gate into a months-long one.

It is days of work and it decides how urgent `transcoding.md` is, which re-orders everything
else in either direction. Nothing about transcoding should be guessed before it lands.

`device-lab.md` runs **after** Phase 0 and in parallel with Phase 1, turning that one-off
measurement into a repeatable sweep and setting the support floor from evidence.

Then `library-model.md` and `scanner.md` in parallel with `stream-service.md` (direct play
only, per [D17](../../decisions/working-notes.md)). `identification.md` was split out of
`scanner.md` by [D59](../../decisions/working-notes.md) and follows it: the scanner is what a
node **observes**, identification is what the core **concludes**, and the split exists so a
parser improvement can be replayed over the database with every node switched off. `web-client.md` needs `stream-service.md`;
its real deliverable is the shared SDK. `tv-client.md` needs `web-client.md` only for that SDK.

`transcoding.md` is deliberately late and is two components in one file — compatibility first
(offline, once per file), bandwidth second (live, and coupled to `remote-access.md` rather than
to the library). `playback-survey.md` sets its priority.

`watch-together.md` needs `stream-service.md` and the SDK from `web-client.md`, and its
off-LAN half needs `remote-access.md`. Its LAN half does not, and is worth building first
because it exercises the same sync machinery with none of the NAT traversal
([D62](../../decisions/working-notes.md)).

`jellyfin-api-shim.md` needs a stable `stream-service.md` and is otherwise independent. It may
reduce the urgency of `tv-client.md` — re-examine that once the adapter exists and its real
fidelity is known, not before.

## The unspecified set

Everything at `UNSPECIFIED` needs a research round before it can be designed. `grep` for the
status rather than trusting this paragraph, but the ordering between them is:

- **`sharing-model.md` is done** (2026-09-19, [D76](../../decisions/working-notes.md)) and the
  answer was cheaper than expected: a share is a grantee plus a predicate, so it is a fourth
  binding of `rules-engine.md`. Its *identity* half remains the open product question below.
- **`remote-access.md` and `sharing-model.md` were one research round, not two.** Sharing needs
  an identity for the other person, and remote access is how that person reaches the server;
  Plex's resented account system exists because those two questions have one answer. They are
  the hardest product questions in the project, and `threat-model.md` depends on the outcome.
- **`rules-engine.md` is done** (2026-09-19, [D75](../../decisions/working-notes.md)) and was
  the urgent one, because it is schema-shaping and `library-model.md` is Phase 1. It hands the
  schema two things: a typed field registry with an `alias` column, and a forced decision on
  whether tags live in a join table or as JSON.
- **`internationalisation.md` is done** (2026-09-19, [D45](../../decisions/working-notes.md)–[D51](../../decisions/working-notes.md)),
  and it turned out to be schema-shaping too: the metadata translation split, the sort-key
  column and the FTS5 tokeniser choice all have to land before `library-model.md`. Two of its
  findings are due *now* rather than with the schema — NFC path normalisation is a field in the
  node report contract, and the Unicode-safe regex corpus belongs with the scanner.
- **`threat-model.md`, `testing-strategy.md`, `performance-targets.md` and
  `backup-restore.md`** are gaps in the foundations rather than features. Each is cheap now and
  expensive later; none blocks Phase 0.
- **`bug-reports.md`** is small, cheap and unusually load-bearing for a project whose logs are
  full of people's film collections. Its structure is adapted from
  [yoloai](https://github.com/kstenerud/yoloai); the design work is the pseudonymisation and
  the cross-component fan-in ([D74](../../decisions/working-notes.md)). It shares its version
  inventory with [D61](../../decisions/working-notes.md)'s staleness check, so build that once.
- **`content-safety.md` is `DEFERRED`, which is a different thing from unspecified.** The
  research is done and the conclusion is that containment beats detection
  ([D61](../../decisions/working-notes.md)); what remains is not a research gap but a decision
  to build it later. It carries **four affordances due now**, of which one — the node protocol
  skipping unknown record kinds — is a wire-format rule that must exist before the first node
  ships.
