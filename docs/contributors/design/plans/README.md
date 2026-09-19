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

`- **Blocks:**` is the one exception, and it means something narrower: *B cannot be finished
correctly until A has been decided*, usually because A contributes schema. It lives in A
because A is where the reader needs the warning. Several of the
[2026-09-19 audit](../research/audit-2026-09-19.md) plans block `library-model.md` for exactly
that reason.

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

## What the 2026-09-19 audit added

An independent audit against industry practice, the large operators and the complaint record
([audit-2026-09-19.md](../research/audit-2026-09-19.md),
[D80](../../decisions/working-notes.md)–[D95](../../decisions/working-notes.md)) produced ten
new plans. The ordering that matters:

- **The schema set is due before `library-model.md`** — which is Phase 1, so this is the urgent
  group: [play-state.md](play-state.md), [media-segments.md](media-segments.md),
  [audio-presentation.md](audio-presentation.md), [parental-controls.md](parental-controls.md),
  plus four things that landed directly in `library-model.md` — People
  ([D95](../../decisions/working-notes.md)), certifications
  ([D84](../../decisions/working-notes.md)), **episode orderings**
  ([D96](../../decisions/working-notes.md)) and **extras**
  ([D97](../../decisions/working-notes.md)). The last two are
  [D79](../../decisions/working-notes.md)'s failure repeating: one position where there should
  be one per ordering, and no level that fits a trailer.
- **[auth.md](auth.md) is a Phase 1 subsystem that had no plan.** Identity, sessions, tokens
  and device pairing were spread across three other documents, none of which owned them.
- **[api-compatibility.md](api-compatibility.md) must land before the first client ships**, not
  before the first release. Its CI gate is the cheap half and the deadline is the expensive
  half: our clients go through app stores and televisions never update.
- **[accessibility.md](accessibility.md)** is mostly already built by
  [D50](../../decisions/working-notes.md) and [D55](../../decisions/working-notes.md) without
  anyone noticing; what it needs is the mapping made explicit and the overlay rendition's
  losses declared.
- **[offline-download.md](offline-download.md)** is late in client terms and early in *manifest*
  terms: an offline manifest that cannot enumerate its own dependencies is a migration, and the
  font pipeline currently assumes connectivity.
- **[observability.md](observability.md)** and **[artwork.md](artwork.md)** are independent and
  can land whenever; both are prerequisites for
  [performance-targets.md](performance-targets.md) meaning anything outside CI.

**Six more came from reading the decisions against each other** rather than against the
outside world, and no new plan came out of them — they are corrections to existing ones:
[D98](../../decisions/working-notes.md) (D7, D24 and D77 disagree about what a sidecar carries,
and the mechanism needs a writable path nobody granted it),
[D99](../../decisions/working-notes.md) (a TMDB poster is attacker-supplied and the core
fetches it; D26's rule was never extended past media files),
[D100](../../decisions/working-notes.md) (a delete on D38's *replay* channel can destroy a
different file that has since appeared at the path — a contradiction this audit itself
created), [D101](../../decisions/working-notes.md) (the intro detector needs library knowledge
the node is forbidden to have, so the core supplies the set and the node supplies the
measurement), [D102](../../decisions/working-notes.md) (**nine passes** now run over every
file and nobody had added them up) and [D103](../../decisions/working-notes.md) (a device
*preference* is stored, a device *capability* never is — the distinction that keeps a device
scope from becoming a device-profile database), [D104](../../decisions/working-notes.md)
(nobody said who may *join* a Watch Together group, and a group link routes around every share
predicate), [D105](../../decisions/working-notes.md) (the Jellyfin adapter has no stated
identity, and the convenient answer is a confused deputy holding the owner's token) and
[D106](../../decisions/working-notes.md) (play state through an adapter has no sequence number,
so D81's guard silently does not apply there).

Plus [D107](../../decisions/working-notes.md): [D35](../../decisions/working-notes.md) states
the file transport's security model precisely and then says "where HTTP exists, prefer it"
without giving the preferred one any — and the dangerous direction turns out to be core→node,
because `config.json` carries commands and, since D89, deletion.

**Three of those fail open** — D104, D105 and D107 — which is the category a keyword sweep is
worst at finding, because the missing thing is a check rather than a feature. All three came
from asking one question of every component in turn: *what is the access path here, and who is
the principal on it?* Worth re-running whenever a surface is added.

And one that changed an existing schedule: [D86](../../decisions/working-notes.md) puts
`transcoding.md` **§(b)** on [remote-access.md](remote-access.md)'s critical path rather than
at Phase 4, because remote playback over a degraded link has nothing lower to fall back to.

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
- **`backup-restore.md` is done** (2026-09-19, [D77](../../decisions/working-notes.md)). The
  headline is a measurement: copying a live WAL database's `.db` file produces a valid SQLite
  file containing **no tables**, silently, discovered only at restore. `VACUUM INTO` instead.
- **`testing-strategy.md` and `performance-targets.md`** remain. `performance-targets.md`
  cannot be finished without hardware — it now carries the list of thresholds other decisions
  are guessing at, which is what it can usefully hold until Phase 0 runs.
- **`threat-model.md`** is PLANNED; what is left there is the per-platform confinement matrix
  and the signed-URL design.
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
