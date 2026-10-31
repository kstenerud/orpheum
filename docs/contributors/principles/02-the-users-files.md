> **ABOUTME:** Principle §2: The user's files are not ours to touch.

# §2 — The user's files are not ours to touch

Media is mounted read-only by default. The database is a rebuildable index, never the authority
(D7). Destructive actions say exactly what they will destroy.

**What the user authored survives us two ways, and they are not interchangeable**
([D98](../decisions/working-notes.md)). The automatic database backup
([D77](../decisions/working-notes.md)) is **durability** — it is on by default and needs no
permission. Sidecars beside the media are **portability**, the part that means "survive *us*",
and writing one needs a writable path this principle does not grant by default. And the split
is by subject, not by convenience: **a sidecar carries what is true about the work; the
database carries what is true about a person**, because the media tree is shared with the
household and often with a grantee.

**Deleting media is deferred, and that is now the stated policy**
([D144](../decisions/working-notes.md)). It appears **0 times in 120 top-reacted Jellyfin
issues and 0 times in Plex's yearly top 50**; [D89](../decisions/working-notes.md) came from an
audit noticing the tree said nothing, not from anyone asking. Its real purpose — *"unstated,
this ends with `os.Remove` in a handler and the read-only promise becomes a comment"* — is
kept as **an enforced absence**: no code path removes a file beneath a media root, asserted by
a test. A retention rule may still only ever *mark*
([D75](../decisions/working-notes.md)). **Running entirely read-only is a supported
configuration**, not a cautious user's workaround.

**And the user's *arrangement* of their files is no more ours to correct than the bytes are**
([D158](../decisions/working-notes.md)). Someone who re-rips a film because the first attempt
broke at 61 minutes, or who hand-deletes the opening scenes of a film because they give the
story away, has made a decision. Our job is to index what is there, not to work out what they
meant and offer to reconcile it. **The concrete rule: we group files for browsing and never
relate their timelines** — if a feature needs to know that a moment in one file corresponds to
a moment in another, it is out of scope. A user-created Edition is their data and a rescan
never overwrites it, exactly as [D141](../decisions/working-notes.md)'s lock markers already
require for any other correction.

**But we do write to the media tree, so this principle still has a real consumer.**
[D141](../decisions/working-notes.md)'s sidecar export needs an **opt-in writable path per
library**, off by default; it happens in the node, never the core
([D33](../decisions/working-notes.md)); writes are **atomic**, and **the file it replaces is
retained**. "Destructive actions say exactly what they will destroy" applies to an overwrite
as much as to a delete.

**And where we do destroy, it is reversible** ([D143](../decisions/working-notes.md)). A
delete goes to a **trash at the root of each library root** — one per root, because
`rename(2)` does not cross filesystems and a central one would make every delete a
copy-and-delete that can fail half way. It never empties itself: automatic expiry would be
the single place this product destroys something nobody asked it to. So the two phases become
**confirm → trash → empty**, and the confirmation can be calmer because the act is undoable.

**Running with the media entirely read-only is a supported configuration, not a cautious
user's workaround.** Someone who never grants a writable path has a complete product, minus
deleting and minus sidecar export. Say so, because the alternative is people assuming they
must loosen permissions to get full function.

**Bites when:** a feature would be easier if the server could just write to the media tree, or
if some state existed only in the database — or when "we don't support deleting" quietly
becomes an `os.Remove` in a handler.
