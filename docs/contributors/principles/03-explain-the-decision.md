> **ABOUTME:** Principle §3: Explain the decision to the user.

# §3 — Explain the decision to the user

The server constantly decides things on the user's behalf — what to transcode, what a file is,
which provider won a field. Every one of those decisions must be inspectable, and the important
ones must be overridable. "It constantly tries to re-encode video that could be played perfectly
fine" is a complaint about an *unexplained* decision as much as a wrong one.

**Bites when:** you implement an automatic behaviour without a way to see why it fired or to
turn it off for one item.
