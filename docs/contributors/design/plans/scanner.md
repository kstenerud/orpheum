> **ABOUTME:** Library scanning — phased, resumable, legible — and the filename parser it
> depends on.

# Scanner

- **Status:** PLANNED — designed 2026-09-17.
- **Depends on:** library-model.md

## Shape

Named phases in the Navidrome style rather than one recursive walk: enumerate folders → detect
missing/moved files → identify and enrich → resolve playlists/collections. Each phase is
separately testable, separately resumable, and separately logged.

A filesystem watcher for incremental updates, plus a full scan that is safe to interrupt.

## The two hard parts

1. **The filename parser is its own component** — a pure function from path to candidate
   structure, with a large table-driven corpus of real-world names as its test suite. `guessit`
   is the best prior art and Kyoo uses a *fork* of it, which tells you it needed patching.
   Anime needs explicit handling: absolute numbering, version suffixes, fansub brackets.
   Position, from Kyoo: "media not being scanned correctly (even with weird names) is a bug" —
   no mandated folder layout.
2. **Legibility.** "Logging around library scanning being pretty difficult to interpret... which
   for me tends to be the trouble spot in these systems" is a direct user complaint about
   Jellyfin. The scanner gets its own structured log channel and a per-run report the user can
   read: what was found, what was skipped, what was ambiguous, and why.

## Also

Manual identification override that survives rescans, provider changes and file moves (D7 —
written back to sidecars). Duplicate and re-encode detection within a library via perceptual
hashing (Stash's `goimagehash` is the prior art).

## Done when

A pathological real-world library scans correctly, an interrupted scan resumes, and the run
report answers "why is this file not in my library?" without reading a debug log.
