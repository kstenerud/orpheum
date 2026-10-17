> **ABOUTME:** Principle §22: Untrusted data is cleaned where it enters; what must stay raw stays
> inert.

# §22 — Untrusted data is cleaned where it enters; what must stay raw stays inert

**Every value an attacker can influence is cleaned at the boundary where it enters the system**,
so nothing inside ever has to ask whether it is safe: filenames and paths, NFO and provider text,
probe output and track tags, a node's error text, subtitle files, and whatever a person types.
Past the boundary a string is a clean string, and every consumer — the pass, the store, the API,
a log line — may use it as one. Cleaning later, at each place text is shown, means one missed
place is a hole, and the unclean value travels through everything in between.

**When the original must be kept — a filename's bytes to open the file, an NFO's bytes to parse
again — it is kept in a form that cannot be mistaken for text**: `[]byte` in Go, a `BLOB` in the
database, never a `string` that a formatter, a template or a log could print as it stands. The
raw form's only readers are the ones that need the original: opening the file, parsing it
again.

The one sanitiser is `internal/core/textindex` ([D211](../decisions/working-notes.md)); its
cousins are D49's NFC and the node's parse-don't-validate of FFmpeg output.

**Bites when:** a value is "only shown in the admin view", "only logged" or "cleaned where it is
displayed"; or when keeping the original as a `string` is one line shorter.
