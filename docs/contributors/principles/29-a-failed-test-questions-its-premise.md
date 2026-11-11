> **ABOUTME:** Principle §29: A failed test questions its premise before it names a defect.

# §29 — A failed test questions its premise before it names a defect

**When a test fails, ask three questions before fixing the code:** does the test make sense; are
we solving the right problem; and does this failure invalidate an assumption we made? A failure can
mean much more than "this is a code defect" (the owner, 2026-11-11: "A failed test can sometimes
mean so much more than 'this is a code defect'").

VQ294's run found /play/1504's heavy stretch cut into chunks of 13 to 23 MB, against its clause of
5 MiB at most. The lead reported the clause as unmeetable and asked for a ruling on it, as though
the question were whether the item could pass. The failure said more: the clause measured the
wrong thing. A copied picture cuts only at its keyframes, and what a small device must hold is a
chunk and its neighbour together, so the assumption beneath the sized grid, that a small enough
chunk always exists, was false. The answer was a different design, not a reworded clause (the
owner: "I'm not interested in just making VQs pass for the sake of passing them. I want a good user
experience."; A69).

**The same holds for a test that passes.** A test written to a wrong premise is green for the wrong
reason; §18 asks whether a check can fail, this asks whether it checks what matters.

**Bites when:** a fix you are about to make changes the test's wording or bound rather than the
behaviour, or a failure is reported to the owner as a ruling owed on the test rather than as what
it showed.
