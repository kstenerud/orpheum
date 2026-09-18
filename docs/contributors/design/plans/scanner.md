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

## Track language and kind normalisation

Part of identification, and easy to underestimate ([D24](../../decisions/working-notes.md)):

- **Normalise every track's language tag to BCP 47**, keeping the original. ISO 639-2 carries two
  competing 3-letter codes per language — `ger`/`deu`, `fre`/`fra`, `chi`/`zho`, `dut`/`nld` —
  and real containers use both inconsistently. FFmpeg's 695-entry `avlanguage.c` table exists for
  precisely this and is the reference.
- **Derive track *kind*, not just language.** `FORCED`, `HEARING_IMPAIRED`, `COMMENT`,
  `DESCRIPTIONS`, `DUB`, `ORIGINAL` come from container dispositions where present — and real
  files set them wrongly or not at all, so fall back to parsing the track title ("English (SDH)",
  "Signs & Songs", "Commentary by the director"). Record which source was used, so a user
  correction can beat a bad heuristic without being overwritten on rescan.
- This belongs in the same table-driven, real-corpus test suite as the filename parser. It is
  only ever right against real files.

## Watching: a strategy interface, not a mechanism

Researched — see [change-detection.md](../research/change-detection.md) and
[D27](../../decisions/working-notes.md). `inotify` does not work over SMB or NFS, has watch
limits a large library exhausts, and **fails silently**, which is why "the scanner did not
notice my new file" and "the scanner rescanned everything at 3 a.m." are the same bug.

So: a per-root strategy with an **explicit capability report** — the server says which strategy
is active and warns when it is degraded or stale. Native watching where it works; a NAS-side
index agent (a tier-3 capability service, built later) for network shares; mtime-delta or full
walking otherwise; and an explicit rescan trigger always.

**The load-bearing corollary for this plan: periodic walking must be good enough on its own**,
because most installs will use it. Its throughput is a number in
[performance-targets.md](performance-targets.md), not a fallback afterthought.

## Also

Manual identification override that survives rescans, provider changes and file moves (D7 —
written back to sidecars). Duplicate and re-encode detection within a library via perceptual
hashing (Stash's `goimagehash` is the prior art).

## Done when

A pathological real-world library scans correctly, an interrupted scan resumes, and the run
report answers "why is this file not in my library?" without reading a debug log.
