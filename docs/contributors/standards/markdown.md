> **ABOUTME:** How documentation in this tree is written — the ABOUTME header, metadata lists,
> headings and anchors, decision citations, claims backed by tests, and the prose rules that keep
> a large design corpus usable by people and agents who arrive mid-file. Each rule names its
> enforcement.

# Markdown Standard

This tree is mostly prose, and most of it is read by an agent that arrived through a search and
landed half-way down a file. Its rules exist to make that reader right: every document says what
it is for, every citation resolves, and no sentence claims more than something checks. The
decision log keeps extra rules of its own in
[decisions/conventions.md](../decisions/conventions.md); this standard does not repeat them.

Scope: every `.md` file under `docs/`, plus `README.md`, `AGENTS.md`, `FEATURES.md` and
`ROADMAP.md` at the root. Not Markdown emitted by the program.

## 1. The ABOUTME header

Every document under `docs/contributors/` opens with an ABOUTME **blockquote**, before its
title:

```markdown
> **ABOUTME:** What this document is for, and when to read it. A second line if needed.

# Title
```

- **Say what the document is for; do not inventory what it contains.** A list of sections or a
  count is a second copy of the headings, and the next addition makes it false.
- **Never state a count** anywhere a gate does not enforce it
  ([principles §8](../principles/README.md#8--factual-accuracy-in-documentation)). yoloAI's
  principles doc opened "Thirteen principles" above sixteen headings.
- It stays visible — a blockquote, not an HTML comment — because it is written for people as
  much as for agents.

**Enforced by:** **review-only**; a presence check is cheap to add to `check_docs.py` if a
document ever ships without one.

## 2. Metadata lists, not front matter

A document with facts about its own state carries them as a bold list under its title. Plans do:

```markdown
- **Status:** PLANNED — 2026-09-23 ([D193](../../decisions/working-notes.md) §8).
- **Depends on:** [schema.md](schema.md)
```

The Status vocabulary and the `Depends on` / `Blocks` rules live in
[design/plans/README.md](../design/plans/README.md). Status never goes in the ABOUTME, which
says what the document is for and would then hold a second, staler copy. No YAML front matter:
it renders as noise in a plain view.

**Enforced by:** **review-only**.

## 3. Headings and anchors

- One `#` title per file. ATX headings (`##`), never underlined ones. Title Case for the `#`
  title; sentence case below it.
- Numbered sections where a document is cited by section: plans use `## 3. Repository layout`,
  the principles use `## §17 — …`, decisions `## D190 — …`. A citation then reads
  "cut-a.md §3".
- **Anchors are GitHub's slugs. Do not derive one by hand**; write the link, run the checker,
  and copy the anchor it names. Hand-slugged anchors have been wrong in this tree more than
  once.
- Renaming a heading breaks every link to it. Search for the old anchor before renaming.

**Enforced by:** `scripts/check_docs.py` fails on an anchor that names no heading.

## 4. Links and citations

- **Relative links** from the current file: `../decisions/working-notes.md`, never an absolute
  path or a GitHub URL to our own tree.
- **Cite a decision by its number, linked**: `[D190](../decisions/working-notes.md)` from outside
  the log, `[D190](#d190--…)` inside it. The link must land on the file that defines that
  number, and on its heading if it carries an anchor.
- **A live document never cites an archived decision.** Plans, principles, standards, the
  overview, `FEATURES.md` and `ROADMAP.md` describe the design now; only the decision logs and
  `design/research/` may cite what no longer holds.
- **Cite what you opened.** A claim that rests on a research document or a plan section names
  it, and the author has read that section, not a summary of it.
- **External sources** are dated, and a claim that rests on one quotes it, because pages
  change. Web pages fetched for research are cached under `/yoloai/cache/http/`.
- Code identifiers, paths, commands, flags and environment variables are in backticks.

**Enforced by:** `scripts/check_docs.py` fails on a broken link, a decision link that lands on
the wrong entry, a gap or duplicate in the decision numbers, and a live document citing an
archived decision; CI runs it on every push (`.github/workflows/docs.yml`). "Cite what you
opened" is **review-only**.

## 5. Claims about the code are backed by tests

When a sentence in `architecture/`, `principles/`, a plan or the decision log states a
checkable fact about what the code does — and a reader who believed it falsely would build the
wrong thing — it cites the test that makes it true, by bare name in backticks:
`` `TestArch_<Claim>` `` ([testing.md](testing.md#9-testarch_-is-reserved-for-claims-the-docs-make)).
Most sentences are judgements, trade-offs or reasons, and need no test; do not dress a judgement
up as a claim.

**Enforced by:** `scripts/check_claims.py` (`make check-claims`): every cited `TestArch_` test
exists, and every one is cited.

## 6. Accuracy

- **Verify and date** a number, a feature claim or a performance figure; mark what could not be
  verified **UNVERIFIED**, in place, rather than dropping it or smoothing it into confident prose
  ([principles §8](../principles/README.md#8--factual-accuracy-in-documentation)).
- **Research says what it did not try.** A research document ends with a "Not tried" section:
  the hosts, versions, inputs and questions outside its evidence. yoloAI's parallel research
  passes differed on exactly this: the pass that wrote such sections made no over-broad
  conclusions, and the other made five.
- **Do not paste a list that lives elsewhere** — a Makefile prerequisite line, a lint rule set,
  a directory listing. Point at it. A pasted copy is a second source that drifts.
- **Correct in place, visibly.** A superseded clause is struck through with its successor
  beside it, as [decisions/conventions.md](../decisions/conventions.md) describes, so a reader
  who lands on it by search sees the correction.

**Enforced by:** **review-only**, except that no count or pasted list can be checked, which is
why they are banned.

## 7. Form

- **Wrap prose at 100 columns.** A hard break never falls inside a code span or a link.
- Lists use `-`. Numbered lists use real numbers. Nested lists indent two spaces.
- Tables use `| --- |` separators without alignment padding; a table whose cells need
  paragraphs should be a list.
- Every fenced code block names its language.
- Bold states the rule; the sentences after it give the reason. A document's lead sentence in a
  section should survive being read alone.
- Write for the reader's task: direct, specific, no marketing words, no emoji. The owner is "the
  owner", referred to as they/them.
- **Do not run a Markdown formatter.** Prose intent does not survive one.

**Enforced by:** **review-only**. `.editorconfig` keeps trailing whitespace in Markdown, where it
can be a line break.

## 8. Filenames and placement

- Lowercase kebab-case, naming the subject, not its status: `cut-a.md`, not `cut-a-draft.md`.
  `README.md` is the one uppercase name inside `docs/`.
- **A directory's `README.md` routes; it does not inventory.** `ls` is the inventory and cannot
  be incomplete.
- An idea worth building is a plan file in `design/plans/`, never a bullet in a README.
- Evidence goes in `design/research/`; a choice in the decision log; the reason for a choice
  never only in a commit message.

**Enforced by:** **review-only**.

## 9. Before committing

Run `python3 scripts/check_docs.py` and fix what it names. Its tests run with
`python3 -m unittest discover -s scripts/tests`. When a decision is added,
`python3 scripts/check_docs.py --write-index` regenerates `decisions/index.md`.

**Enforced by:** `make check-docs`, which `make check`, the pre-commit hook and CI run; the Stop
hook runs it at the end of any agent turn that changed a document.
