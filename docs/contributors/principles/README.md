> **ABOUTME:** The engineering disposition for this project — the small set of principles that
> decide arguments, each earned from a specific failure observed in the competitive research.

# Principles

Principles explain **why**; the per-technology standards under `../standards/` explain **what**
and **how**. A principle wins over any conflicting standard or design choice.

These are deliberately few. They will grow when a real argument needs settling, not before.

Each principle is a file of its own; open the one a citation names. They are listed by the
work they bear on, and one that bears on two kinds of work is listed under both.

## Testing and the gates

- [§17](17-a-rule-nothing-runs.md) — A rule that nothing runs is a hope
- [§18](18-a-check-never-seen-failing.md) — A check you have never seen fail proves nothing
- [§29](29-a-failed-test-questions-its-premise.md) — A failed test questions its premise before it names a defect
- [§20](20-a-gate-changes-on-purpose.md) — A gate changes on purpose, never to land a change
- [§21](21-an-owners-correction.md) — An owner's correction is a specimen, and it is written down
- [§23](23-proportionate-test-runs.md) — A test run is proportionate to the change
- [§24](24-cheap-checks-first.md) — Cheap checks first, never behind an expensive one; piecework
  fails fast, the final run finishes
- [§25](25-velocity-is-a-resource.md) — Velocity is a resource, and the dev cycle's time costs it
  quadratically
- [§26](26-poor-code-is-a-recurring-tax.md) — Poor code is a recurring tax; an improvement seen is
  recorded, and fixed in a batch

## Data and the user's files

- [§2](02-the-users-files.md) — The user's files are not ours to touch
- [§7](07-migrations-are-safety.md) — Migrations are a safety feature
- [§13](13-state-attaches-to-what-you-observed.md) — State attaches to what you observed, never to
  what you inferred
- [§14](14-a-cache-may-always-be-wrong.md) — A cache may always be wrong, so the path that ignores
  it is the ordinary one

## Security

- [§6](06-untrusted-code-is-sandboxed.md) — Untrusted code is sandboxed, or it does not run
- [§10](10-only-what-a-client-may-use.md) — Give a client only what it may use; a refusal is an
  alarm, not a branch
- [§22](22-untrusted-data-is-cleaned.md) — Untrusted data is cleaned where it enters; what must stay
  raw stays inert

## Design and code

- [§1](01-boring-technology.md) — Boring technology, and innovation tokens
- [§4](04-one-way.md) — One way to do it
- [§5](05-an-interface-not-a-branch.md) — Combinatorial complexity gets an interface, not a branch
- [§9](09-sensors-report.md) — Sensors report; the core decides
- [§11](11-hot-path-values-are-cached.md) — A value on a hot path is cached against a known event,
  not computed per request
- [§12](12-defer-the-work-never-the-seam.md) — Defer the work, never the seam
- [§15](15-safe-visible-defaults.md) — A default must be safe to be wrong and visible where it acts;
  otherwise, ask
- [§27](27-decisions-are-revisable.md) — A decision is the best answer on what was known; a better
  way found later is raised at once
- [§28](28-messages-get-lost.md) — Every message may be lost and every copy of state may drift;
  each exchange recovers
- [§30](30-one-request-before-the-person-acts.md) — What a person waits on comes in one request;
  what is only cosmetic follows

## The product and its documents

- [§3](03-explain-the-decision.md) — Explain the decision to the user
- [§8](08-factual-accuracy.md) — Factual accuracy in documentation
- [§16](16-people-are-responsible-for-what-they-play.md) — People are responsible for what they
  play; we never supply, locate, unlock or promote it

## Interface

How the client looks and behaves on a desk, a phone, a touch screen and a TV. The standard that
applies them, rule by rule, is [ui.md](../standards/ui.md).

- [§31](31-the-art-is-the-colour.md) — The art is the colour; the chrome is neutral
- [§32](32-near-black-not-black.md) — Near-black, not black
- [§33](33-each-colour-has-one-meaning.md) — Each colour has one meaning, and never carries it alone
- [§34](34-every-value-is-a-token.md) — Every value is a token
- [§35](35-focus-only-adds.md) — Focus only adds, and is always seen
- [§36](36-fingertip-sized.md) — Fingertip-sized, for every hand and every remote
- [§37](37-short-rare-optional-motion.md) — Motion is short, rare and optional
- [§38](38-plain-specific-words.md) — Plain, specific words
- [§39](39-one-home.md) — Each thing has one home, and other places link to it
- [§40](40-problems-first.md) — Problems come first
- [§41](41-one-way-for-each-interaction.md) — One way for each kind of interaction; [§4](04-one-way.md)
  for the interface

## Debugging

- [§19](19-a-defect-is-a-sample.md) — A defect is a sample: look for its siblings before fixing it
- [§29](29-a-failed-test-questions-its-premise.md) — A failed test questions its premise before it names a defect
- [§21](21-an-owners-correction.md) — An owner's correction is a specimen, and it is written down
