> **ABOUTME:** Principle §26: Poor code is a recurring tax; an improvement seen is recorded, and fixed in a batch.

# §26 — Poor code is a recurring tax; an improvement seen is recorded, and fixed in a batch

**A ticket's work is paid once; poor code is paid again by every ticket after it.** The owner,
2026-10-27, taking §25 further: *"Codebase hygiene translates to better velocity via two
mechanisms"*:

- **Better architected code, the app's and the tests', breaks less often**, so fewer dev cycles go to
  fixes: velocity improves by the fix cycles that never happen.
- **More efficient testing reduces the per-cycle and per-batch cost**, so velocity holds up over time.

*"We therefore keep an eye out for opportunities to improve the code whenever we spot them. We
don't necessarily have to fix it right away if it would take a lot of work, but we always record it
for a code fixing batch later. Per-ticket work items are one-time costs. Poor code is a recurring
cost that every per-ticket work item has to pay in tax."*

So an improvement spotted while doing something else is never dropped and never smuggled in:

- **Small and in the code you are already changing:** fix it there, in its own commit.
- **Anything larger, or outside the task:** add it to [code-debt.md](../code-debt.md), saying what it
  costs (which work pays the tax, and how often) and what would fix it. The task goes on; the
  owner schedules a code-fixing batch from the list.
- **An audit, a review or an agent's report that names one** puts it on that list too, so a finding
  outlives the conversation it was found in.

**Bites when:** you work around a clumsy harness for the third time, notice a duplicate and move on,
or report a weakness in a reply that nothing records.
