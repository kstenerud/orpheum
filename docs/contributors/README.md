> **ABOUTME:** Router for docs/contributors — picks the right doc by task and states the
> conventions shared across the tree, including how content in it retires over time.

# Contributors

Docs for building and changing this project.

## Where to go

| You want to | Read |
| --- | --- |
| Understand what we are building and why | [`design/overview.md`](design/overview.md) |
| See what exists already and how it fails | [`design/research/`](design/research/README.md) |
| Know *why* a technology was picked | [`decisions/working-notes.md`](decisions/working-notes.md) |
| Find unbuilt work | [`design/plans/`](design/plans/README.md) |
| Know the coding rules | [`standards/`](standards/README.md) |
| Know the engineering disposition | [`principles/`](principles/README.md) |

## The directories

- **`design/`** — the shaping cluster: the overview, `research/` (evidence), `plans/`
  (designed-but-unbuilt work).
- **`decisions/`** — append-only, D-numbered. Non-trivial decisions land here first; other
  docs cite them by number.
- **`principles/`** — **why**. A principle wins over any conflicting standard.
- **`standards/`** — **what** and **how**, per technology.
- **`architecture/`** — how the code is put together. Empty until there is code.

## Doc conventions

Adopted wholesale from the [yoloAI docs tree](https://github.com/kstenerud/yoloai), which this
project uses as its template.

- Every source document under `docs/contributors/` opens with an **ABOUTME** blockquote saying
  what the file is for. Say what it is *for*; don't inventory what it contains, and **never
  state a count** — a count carries no information the artifact doesn't, and the next addition
  makes it false.
- Every directory `README.md` **routes**; it does not inventory. `ls` plus `head -3` is the
  inventory, and it cannot be incomplete.
- Filenames are lowercase kebab-case and name the subject, not its status.
- A plan declares where it stands in a bolded metadata list under its title
  (`- **Status:**`, `- **Depends on:**`), not in its ABOUTME.
- **Factual accuracy matters.** Star counts, feature claims, and performance numbers must be
  verified and dated. Marketing language and unverifiable numbers do not go in. Where a claim
  could not be verified, it is marked **UNVERIFIED** in place rather than dropped or laundered.

## How we work

- **Research before design changes.** A design question gets researched in `design/research/`
  with verified facts first; the design is then updated from the findings.
- **Commit granularity.** One commit per logical change. Research, design updates, and
  decisions are separate commits.
