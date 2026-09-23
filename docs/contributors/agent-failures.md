> **ABOUTME:** The log of mistakes the code-writing agents made that the owner had to correct:
> what happened, what caught it, and whether a gate now catches it. The raw material from
> which the next lint rule, test or hook gets built.

# Agent failures

This project is written by agents, so **the agent is a component with failure patterns**, like
any dependency whose behaviour we cannot change and can only characterise and guard against.
This file records the specimens. Its model is yoloAI's log of the same name, whose conclusion
was *"Execution catches my errors. Reading does not"*, and whose gates against the agent's own
habits were each built from an entry written here first
([principles §21](principles/README.md#21--an-owners-correction-is-a-specimen-and-it-is-written-down)).

## When to write an entry

**The trigger is an owner correction.** The owner contradicts a claim, or asks a question
("what did you run?", "why does that need X?") whose honest answer is "I was wrong". Record it
**before moving on**, while the reason the wrong claim felt like knowledge is still
recoverable. Once the answer is known the error looks obviously avoidable, and all that is left
to write is "be more careful", which is the one lesson known not to work.

**The bar is that the false belief reached something durable**, or would have: a commit, a
decision, a plan, a filed defect, a recommendation someone acted on. A wrong guess that the next
command corrected is the process working, and does not belong here. A log of every fumble is
unreadable.

A gate that catches a mistake before the owner does is the system working; it needs no entry
unless the mistake is new in kind.

## The entry format

Newest first. IDs are `A<n>`, allocated as one more than the highest in this file, and never
reused.

```markdown
### A<n> — <what went wrong, in a phrase> (<YYYY-MM-DD>)

- **What happened:** what was claimed or built, and what was true. Name where the false
  belief came from: a search result, a subagent's summary, a comment, an error string, a
  document, a test that could not fail.
- **Caught by:** who or what noticed, and how. This is the field that says what to build: if
  it keeps reading "the owner asked", the mechanisms are absent, not weak.
- **Gate:** what would catch it now — a lint rule, a test, a hook, a check — named precisely,
  or "none" with the reason. "None" is an honest answer and a to-do.
```

**Cite entries by ID, never by count.** A pattern drawn from these entries names the IDs it
rests on, so the claim can be checked against them; a total goes stale with the next entry.

## Log

*No entries yet.*
