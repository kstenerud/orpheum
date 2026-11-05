> **ABOUTME:** Principle §24: Cheap checks first, never behind an expensive one; piecework fails
> fast, the final run finishes.

# §24 — Cheap checks first, never behind an expensive one; piecework fails fast, the final run finishes

**A run is ordered by cost, and stops or finishes by what it is for.** The owner, after a
`make check-pr` spent an hour, most of it in browser runs inside its quick tier, one of which hung
until its timeout and took every other probe with it (test-weight.md §14):

- *"Cheap tests run first, expensive tests run after."*
- *"Don't let an expensive test block a cheap test."*
- *"Set up the test granularity such that cheap tests can be separated from expensive ones."*
- *"Parallelization only so far as it actually brings performance gains, and doesn't starve other
  tests of resources (e.g. CPU, memory, etc)."*
- *"Fail fast on piecework check tests. Run to completion on the final test run for the full work
  block (which could be multi-commit), where we already expect it to pass (because we've run the
  relevant tests in isolation already and are just verifying that nothing unrelated broke)."*

So:

- **Order by cost.** A run's cheap checks come before its expensive ones, and a cheap check never
  waits in a queue behind an expensive one, for a lock or a slot. The expensive work goes in a
  later tier of its own, not at the end of each cheap item.
- **Make the split possible.** Where a behaviour can only be checked expensively (a browser, a real
  FFmpeg), look first for a cheap test of the same logic (`sw.test.ts` for the service worker); what
  is left expensive is tested at the grain that lets it be scheduled apart.
- **Parallelize only what gains.** More at once is right only while it shortens the run and leaves
  the others their CPU and memory; a timing test failing under a neighbour's load is a cost, not a
  saving.
- **Piecework fails fast.** A check of one piece (a commit's revert-red, a re-run of what failed, a
  probe) stops at its first failure: one red settles it, and the rest is time spent on nothing.
- **The final run finishes.** The run over a whole block of work (`make check-pr`) is expected to
  pass, since its pieces were checked already. It runs every tier to its end, a failed one too, so
  one run names every failure; only a failed quick tier stops it before the long ones, as a
  minute's finding is not held behind half an hour (D424).

The gate holds it: check-pr's tiers from quickest to slowest, revert-red's code part with cheap runs
alone and its browser runs last, each probe's browser run stopped at its first failure, and every
tier run to its end (standards/makefile.md, "The fix-and-re-run loop").

**Bites when:** a quick tier holds an expensive run; a check of one piece runs past its first red; a
final run stops at its first failure and leaves the rest unknown; or more jobs are added because the
machine has the cores, not because the run got shorter.
