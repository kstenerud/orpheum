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

### A3 — Removal logic built without reading the decisions that make removal safe (2026-09-24)

- **What happened:** the scanner and the ingest (an earlier commit, 2563802) were built from D39
  and D40 — completeness, scopes, tombstones only after a complete scan — and presented as
  removal-safe. D41 and D42, the next two decisions, say the most dangerous case has **no error
  at all**: an unmounted disk leaves an empty directory that lists cleanly, so a complete scan of
  it tombstones the whole library. They require the node to report each root's device id and
  mount facts and to tell "listed and empty" from "not enumerated", and the core to withhold
  removals on implausible shrinkage and ask. None of it was built. The false belief came from
  reading the decision the code cited (D40) and not the ones around it.
- **Caught by:** the owner's request for user-story scenarios; walking "a USB disk is unplugged"
  through the design found D41's section on the silent case.
- **Gate:** the scenario tier (plans/scenarios.md): the unplugged-disk scenario becomes a test
  that fails if a clean scan of an emptied root removes anything. General lesson: before building
  a mechanism, grep the decision log for every decision that names it, not only the one cited.

### A2 — Two rules that contradicted each other, built side by side without a word (2026-09-24)

- **What happened:** D200 said a missing setting is an error, never a fallback, and in the same
  change the server seeded every missing setting at start, so a later release could add one. The
  agent built both, plus `orpheum seed-settings` to paper over the gap, and presented them as one
  coherent model. The owner saw that "balk at a missing value" and "seed any missing value"
  cannot both be the rule (D202). The false belief came from building each requirement as it
  arrived, and checking each against its own tests, never against the others.
- **Caught by:** the owner, reading the summary.
- **Gate:** none can catch a contradiction between requirements. The between-milestone audit is
  the check, and a decision entry should now list the earlier rules it touches and say how each
  still holds.

### A1 — "Modified" computed from the value, so a choice looked like no choice (2026-09-24)

- **What happened:** the settings model marked a setting *modified* when its value differed from
  its starting value (D200). A person who set a value equal to the default became
  indistinguishable from nobody, so a release that changed the default would have overwritten a
  deliberate choice. The owner's question — *"does that still get set if the user modified the
  value back to its default?"* — had the honest answer "no". The belief came from about:config,
  whose computed "modified" the agent copied without asking what upgrades need of it.
- **Caught by:** the owner's question.
- **Gate:** `TestSet_MarksModifiedEvenAtTheStartingValueAndResetClearsIt` and
  `TestSeed_MovesOnlyUnmodifiedSettingsToAChangedStartingValue` now pin the corrected behaviour
  (D201). Nothing general: the lesson is to test a state model against the transitions a later
  release makes, not only the ones today's code makes.
