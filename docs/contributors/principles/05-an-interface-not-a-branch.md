> **ABOUTME:** Principle §5: Combinatorial complexity gets an interface, not a branch.

# §5 — Combinatorial complexity gets an interface, not a branch

Hardware accelerators, containers, codecs, subtitle formats and metadata providers all multiply.
Each family goes behind one interface with one implementation per member and a capability probe.
`EncodingHelper.cs` is 8,042 lines because the alternative was chosen.

**Bites when:** you add an `if` for a specific accelerator, codec or provider outside that
family's own implementation file.

The same reasoning at the scale of one function: **push ifs up, fors down**
([Debasish Ghosh's account](https://debasishg.github.io/blog/push-ifs-up-fors-down/) of the idiom
from TigerBeetle's style guide, made known by matklad; adopted by the owner, 2026-10-08).

- **Ifs up.** A function takes what it needs, not what might be there: the caller decides once,
  and passes the decided value, so the function has no branch for a case it should never see.
  A nil part checked at every use is a decision put off; make it where the thing is built (the
  node exchanges take their unsent-attempt hearing from their sender once, in `NewExchanges`,
  rather than checking for it at each assignment).
- **Fors down, where a boundary is crossed.** A function that crosses one, the database, the
  network, the node protocol, takes the whole batch and loops inside: one query for a hundred rows,
  not a hundred queries. A loop that crosses nothing may stay where it reads best; the article's
  reason, vectorized tight loops, rarely applies to code that waits on disks and links.
- **Not at §4's cost.** Pushing a check up into every caller makes it many checks; when the callers
  multiply, the decision belongs in one constructor or one type, not in each of them.

**Bites when:** a function's first lines branch on whether an argument is usable, or a loop
calls something that crosses a boundary once per item.
