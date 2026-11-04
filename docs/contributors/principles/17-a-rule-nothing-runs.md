> **ABOUTME:** Principle §17: A rule that nothing runs is a hope.

# §17 — A rule that nothing runs is a hope

**Put each rule at the leftmost place that can hold it: the type system, then a generator, then
a lint rule, then a test, then a hook — and prose last, naming the mechanism that enforces it.**
Prose is read; it is not applied. A rule only holds when breaking it makes something fail.

The evidence is unusually direct. yoloAI measured its own guidance: its factual-accuracy
principle *"fired zero times in six"*, and its log of agent failures concludes *"Execution
catches my errors. Reading does not."* Three of its specimens broke a rule that was written in
the very file being edited (A9, A28, A30). This tree has its own: §13 exists because
[D59](../decisions/working-notes.md)'s *"store the observation"* was broken twice in three days
by the author who had cited it ([D155](../decisions/working-notes.md),
[D156](../decisions/working-notes.md)).

**Documentation is where this bites hardest, because nothing executes it.** A yoloAI design doc
said, in bold, "no exceptions". It was false, and the owner and an agent both built on it. So a
load-bearing claim about what the code does — one where, if the sentence were false, a reader
would build the wrong thing — is backed by a test named `TestArch_<Claim>` and cites it by name,
and a gate checks that every cited test exists and every such test is cited
([D190](../decisions/working-notes.md)). *"This is a judgement, not a claim"* is the right answer
for most sentences; only checkable facts about the code need a test.

**A step that must happen is a rule too.** Asking an agent to remember to do something guarantees
that one day it will not, because of how agents work (the owner, 2026-11-04). So a step the code
needs to work — a setup a test tier depends on, a value a deploy must carry — is either done
automatically or checked for, and the check is enough: it fails deterministically and its message
names the fix, and whoever it alerts fixes it. The browser tier's setup, for example, refuses a
machine whose Docker rewrites the packets it sends itself, and prints the `iptables` command that
fixes it (`web/e2e/local-paths.ts`), instead of a note saying to remember the rule.

Where a rule cannot be mechanical, say so: every rule in `../standards/` names its mechanism or
says it is review-only, so nobody mistakes a hope for a gate.

**Bites when:** you write "must", "never" or "always" in a doc, a comment or `AGENTS.md`, and
nothing would fail if the code did the opposite — or you answer a repeated mistake by rewording
the rule, or you write "remember to" for a step something could do or check.
