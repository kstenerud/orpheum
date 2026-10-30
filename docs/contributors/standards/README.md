> **ABOUTME:** Router for the per-technology standards — what and how, per language — and the
> one rule they all follow: every rule names the mechanism that enforces it.

# Standards

Standards say **what** and **how**, per technology. [Principles](../principles/README.md) say
**why**, and win where they conflict.

## The rule behind every standard here

**A rule names what enforces it**: a compiler flag, a lint rule, a test, a gate script or a hook.
If nothing can, it says *review-only*, and review-only rules are kept few. yoloAI measured its own
guidance and found that rules written as prose were followed unreliably, and mechanical gates
were not. That finding is recorded in
[yoloai-lessons.md](../design/research/yoloai-lessons.md) and
[audit-2026-09-23.md](../design/research/audit-2026-09-23.md) §7.

Four aims, in the owner's words, shape every rule:

- **Human readability.**
- **Low complexity, enforced by tooling**: complexity and size limits that fail the build.
- **Understandable without chasing definitions**: names that say what a thing is.
- **Linting that catches LLM errors early**: strict compilers, strict linters, and gates that
  agents cannot loosen.

## Where the rules are enforced

`make check` runs them all ([`Makefile`](../../../Makefile)). The Claude Code Stop hook runs it at
the end of every agent turn, the pre-commit hook before every commit, and CI on every push. The
branch gates (`make check-pr`) run on pull requests. Gate files are the owner's to change
([`scripts/gate_paths.py`](../../../scripts/gate_paths.py)).

## By technology

| File | Covers |
| --- | --- |
| [go.md](go.md) | the server, the node, the tools |
| [sql.md](sql.md) | SQLite, sqlc, migrations |
| [typescript.md](typescript.md) | the web client, cast receiver, SDK and player; React; CSS |
| [testing.md](testing.md) | tests in every language |
| [python.md](python.md) | the gate scripts and hooks |
| [shell.md](shell.md) | shell scripts and git hooks |
| [makefile.md](makefile.md) | the Makefile |
| [markdown.md](markdown.md) | documentation |
| [design-cycle.md](design-cycle.md) | iterating on a look with the owner: the gate deferred, then taken up once |
