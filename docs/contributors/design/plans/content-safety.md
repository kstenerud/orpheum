> **ABOUTME:** Proactive detection of hostile media — deliberately unscheduled, with the
> research done and the seams held open. Containment is the shipped answer; detection is a
> later, optional, plugin-shaped addition.

# Content safety

- **Status:** DEFERRED — researched 2026-09-19 ([threat-model.md](../research/threat-model.md) §5, [D61](../../decisions/working-notes.md)). Not for the first release. Four affordances are due **now**.
- **Depends on:** [threat-model.md](threat-model.md), [scanner.md](scanner.md)

## The position in one paragraph

Media, subtitles, embedded fonts and posters are all attacker-supplied parser input, and 39
entries in CISA's exploited-in-the-wild catalogue touch media, image or font parsing. Every
platform that has faced this at scale answered with **containment and structural validation, not
detection** — Apple's BlastDoor, Netflix's conformance validators, Google's fuzzing plus
sandboxing. We have already taken that answer ([D26](../../decisions/working-notes.md),
[D33](../../decisions/working-notes.md), [D43](../../decisions/working-notes.md)). Detection is
worth adding later, in a specific shape, and the cheapest thing to do today is not close the
door on it.

## Due now, because they are awkward later

| # | Affordance | Cost today |
| --- | --- | --- |
| 1 | **Unknown report record kinds are skipped, counted, and not fatal** | A rule in the wire format, before the first node ships |
| 2 | **The seccomp filter uses an action that can be observed** — `RET_LOG`, `RET_TRAP` or `RET_USER_NOTIF` rather than a silent kill | A flag choice |
| 3 | **Per-file peak CPU, memory and output size recorded while probing** | We already have cgroups |
| 4 | **Quarantine is a library state, never a file operation** | A rule, and it is already [D7](../../decisions/working-notes.md)'s |

Item 1 is the load-bearing one: it is what lets an `anomaly` record appear later without a
protocol break or a node/core upgrade ordering problem.

Item 2 is the highest-value one: **the sandbox is already a detector and we are not reading
it.** A file that makes FFmpeg attempt an unexpected syscall is stronger evidence than any
signature.

## Later, ranked by value over cost

1. **Sandbox-violation reporting** — free once item 2 is in place.
2. **Resource-bomb and outlier detection** — free once item 3 is. This is the *realistic* threat
   to a home server: denial of service, not remote code execution. ASS render bombs (Jellyfin
   ships `libassMemoryLimit` and `libassGlyphLimit` precisely for this), decompression bombs in
   WOFF2/brotli fonts and PNG posters, files that pin a CPU indefinitely.
3. **Declared-versus-actual type mismatch** — nearly free, since we already sniff rather than
   trust the container's MIME type ([D56](../../decisions/working-notes.md)).
4. **Structural conformance** — the Netflix Photon family: oversized boxes, negative lengths,
   impossible dimensions, declared-versus-actual stream mismatches.
5. **Cross-parser disagreement** — two independent parsers, compare. Powerful and expensive.
6. **Signature scanning** (YARA, ClamAV) — last, and only ever as a tier-3 plugin inside the
   same jail.

## Two things not to build

**Shellcode detection**, as such. Modern exploits often contain no shellcode at all — ROP and JOP
chains use gadgets already in the process, and data-only attacks corrupt state without injecting
code. And compressed video is high-entropy and statistically indistinguishable from packed or
encrypted payloads, so entropy heuristics are a 100% false-positive rate on our corpus. The
intent is right; items 1–4 above are the mechanism.

**A scanner with privilege.** ClamAV has 158 CVEs, 29 since 2023, seven on one day in
August 2026 — all file-format parsers, all "an unauthenticated remote attacker could…". A
scanner is another hostile-input parser. It runs in the same jail with no more privilege, and it
is never a trusted security component.

## The cheaper investment that competes with all of this

FFmpeg, FreeType, HarfBuzz and libass are **all in OSS-Fuzz**. Our entire parsing stack is
already under continuous adversarial testing by a better-resourced programme than we could run,
so **shipping current builds promptly beats scanning**, at a fraction of the cost.

The concrete deliverable: the server knows which FFmpeg, libass and FreeType it is running, and
says so when they are stale. That is the same capability
[D53](../../decisions/working-notes.md) wants for client FreeType versions, and it should be
built once.

## The constraint that shapes any eventual UI

These are the **user's own files**, acquired over fifteen years, many encoded by broken tools. A
server that warns about a 2004 home-video rip has not made anyone safer — it has taught them to
dismiss warnings, which is a security failure and not a cosmetic one.

So: default silent; surface only observations that can be stated concretely; always say what to
do. *"This file used 40× the memory of anything else in your library while probing"* is
actionable. *"Possible shellcode detected"* is not.

## Done when

Not scheduled. The four affordances are done when the node protocol skips unknown record kinds
under test, the seccomp filter's action is observable and its violations reach a report, probe
resource peaks are recorded, and a quarantine test asserts that no user file was moved, modified
or deleted.
