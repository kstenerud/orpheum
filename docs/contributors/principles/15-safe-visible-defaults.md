> **ABOUTME:** Principle §15: A default must be safe to be wrong and visible where it acts;
> otherwise, ask.

# §15 — A default must be safe to be wrong and visible where it acts; otherwise, ask

A default is a decision made on someone's behalf without asking them. That is fine when being
wrong costs nothing and the person can see it happening. It is a trap when it does something
consequential they never chose and only find out about afterwards.

So two tests, and a default must pass both:

- **Safe to be wrong.** If the default turns out not to be what this person wanted, does anything
  get lost, exposed, deleted, overwritten, fetched, or spent? If yes, it is not a default; it is a
  question.
- **Visible where it acts.** The effect is labelled at the point the person meets it — "from NFO",
  "default", "not fetched" — not only on a settings page they have no reason to open.

Where a choice is consequential, **ask at the moment it first matters** (creating a library,
adding a device, the first remote sign-in), state the effect in the question, and show the answer
wherever it acts afterwards. Asking once is cheaper than a support thread, and far cheaper than a
loss.

**The field's examples.** Home Assistant's backup-before-update is **off by default** since
2025.2 — the release notes' own mitigation is to show *"the last time you made a backup"* beside
the switch, which is the visibility half of this rule arriving after the fact. And Plex's
*"Empty trash automatically after every scan"*: harmless while a network share never drops; when
it drops, a scan flags every file on it as deleted and the trash is emptied, taking the entries'
metadata with it — a consequential behaviour a person turns on without the question stating that
effect. *(Whether it was ever on by default is not verified here.)* [D34](../decisions/working-notes.md)
and [D167](../decisions/working-notes.md) — *absence is never evidence* — are this principle's
first application; [D183](../decisions/working-notes.md) states it.

**How this project keeps a default visible.** Since [D200](../decisions/working-notes.md) nothing
has a default in code: every opinion is a setting row seeded with its **starting value**, or a
config key with one, and both are shown beside their starting value, marked when a person chose
them, and listed by `doctor` when they run on a starting value
([plans/settings.md](../design/plans/settings.md)). A starting value must still pass both tests
above; a consequential one is a question, which starts unanswered.

**Bites when:** a setting's default is chosen because it is "what most people want", without
asking what it does to the people who wanted something else.
