> **ABOUTME:** Principle §4: One way to do it.

# §4 — One way to do it

One server language. One formatter. One linter config. One router. One way to write a query.
The value is not aesthetic: the fewer ways there are to express a thing, the more of the
codebase looks like the rest of it, and the better both reviewers and agents perform.

The same holds inside the codebase: a second home-grown mechanism for one job is the commoner
case, and [§19](19-a-defect-is-a-sample.md) is its rule.

**Bites when:** a new dependency introduces a second idiom for something we already do.
