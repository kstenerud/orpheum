> **ABOUTME:** Principle §38: The interface speaks plainly and specifically, in full sentences,
> saying what happened and what to do next.

# §38 — Plain, specific words

Every message says what happened and what the person can do next, never a code and never "Something
went wrong". The project's own phrasing gives the register: "Your own hardware. Your own videos.
Play from anywhere." and "No telemetry. No storefront. No paywall. No ads." Labels are verbs or
plain nouns: Play, Resume at 41:12, Films, Libraries. An empty state says why it is empty and offers
the next step. An error says what could not be done, and a request for something that cannot be
undone says so before the person commits. [§3](03-explain-the-decision.md) is the same rule for a
decision the server made.

Words are written for other languages from the start: every word in a locale file, logical
properties only, no width fixed to an English label, and `dir="auto"` on metadata.

**Bites when:** a message shows a code or a vague apology, an empty list offers no next step, or a
label is written into a component instead of a locale file.
