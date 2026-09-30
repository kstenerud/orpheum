> **ABOUTME:** Pieces of code that look alike where it is not yet clear whether they are the same
> thing (D229 §3). Each waits for the question that would settle it; audits revisit them.

# The look-alike watch list

[D229](decisions/working-notes.md#d229--code-that-looks-alike-is-merged-only-when-it-is-the-same-thing-settled-look-alikes-are-listed-here-and-doubtful-ones-are-watched)
says code is merged only when it is the same concept. What an audit found alike and could not
judge goes here, with the question that would settle it. A settled entry leaves this list: merged
if it is one thing, or added to D229 §2's table, with why, if it is two. Audit checklist item 10
reads this list and D229 §2 before reporting duplicates.

| Look-alikes | Found | The question that settles it |
| --- | --- | --- |
| CSS `.poster-grid__card` (browse) and `.continue-watching__play` (home) | audit-2026-09-29 | Is a Continue Watching item a poster card with progress on it, or its own kind of tile? If the first, they are one component. |
| The wait-then-attach hook in `play.tsx` and `subtitles.tsx` | audit-2026-09-29 | The two are the same pattern; a generic hook would defeat the `exhaustive-deps` lint. Is there a shape that keeps the lint's check, or does a third copy appear? |
