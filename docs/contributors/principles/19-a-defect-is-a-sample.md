> **ABOUTME:** Principle §19: A defect is a sample: look for its siblings before fixing it.

# §19 — A defect is a sample: look for its siblings before fixing it

**Before fixing a defect, search for its shape** — the same kind of fault on a sibling path,
another implementation, the other half of a pair. If it recurs, the design is generating it and
the instance is a symptom. If a sibling path already solves it, converge on that one and delete
the divergence, rather than adding a second mechanism for one job.

yoloAI found four defects in one workstream with one shape — a capability implemented on some
backends and not their siblings — and one backend was already immune because it did the job
differently. The durable fix was one conformance case every backend must pass, not four patches
(DF150–DF158). Its rule that a failed subprocess's error carries the tool's stderr was *"re-broken
by every new backend path"* that did not copy an existing one, until it became a single helper.
And a proposed fix for one finding would have rebuilt another by hand (A4).

**Naming the pattern is mandatory; acting on it is a decision.** A finding that says "this is the
fourth of its kind" is a complete response when rewriting the design mid-fix is not justified.
§5 is this principle applied in advance to the families we already know will multiply.

**Bites when:** you find yourself writing "a mechanism on one path and not its sibling", or a fix
introduces a helper whose name is close to one that already exists.
