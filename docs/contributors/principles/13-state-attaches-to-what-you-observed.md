> **ABOUTME:** Principle §13: State attaches to what you observed, never to what you inferred.

# §13 — State attaches to what you observed, never to what you inferred

**A conclusion may be wrong. Data the user cannot afford to lose must not hang off one.**
Store a value against the thing you directly observed; derive the grouping at read time, where
revising it costs nothing.

The distinction is not about confidence, it is about **what happens when the inference
changes**:

| | Attach state to it? |
| --- | --- |
| *this file*, *this device*, *this user reported this timestamp* | **yes** — it is what happened |
| *this is the director's cut*, *these two files are the same cut*, *this item is a horror film* | **no** — it is what we concluded, and we may re-conclude tomorrow |

**This is [§9](09-sensors-report.md) pointed at rows instead of components**, and
it is [D59](../decisions/working-notes.md)'s *"store the observation, not the
interpretation"* promoted out of one decision, because it was violated twice in three days by
the same author who had cited it: [D155](../decisions/working-notes.md) put a playback
position on a Title, [D156](../decisions/working-notes.md) moved it onto an inferred Edition,
and [D157](../decisions/working-notes.md) put it where the player actually was — the File.
Both errors were found by describing an ordinary library, not by review.

**The tell is a migration.** If changing your mind about a classification means *moving* user
data rather than *regrouping* it, the state is attached one level too high. A relabel should
write one column.

**Bites when:** you attach a position, a preference, a rating or a progress counter to an
entity whose identity came from a parser, a provider or a heuristic.
