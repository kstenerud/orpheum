# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Done 2026-10-27:** the batch of D360–D362, D361's pages, the Play page's Back: accepted, on nas.example.
   **Now (owner 2026-10-27: "go ahead with the next batch"):** D363 recorded (overlap, copies, lists). Agents:
   D363 build; media (matcher runtime, MPEG-TS, readthrough,
   old setting rows). After D363 lands: sorting both ways + release date, and per-person home visibility.
   `ssh prod-nas` (192.168.1.250): runs an earlier commit (bundle, branch sandbox-main), root /tank/media.
   **Next batch, first (decided 2026-10-27; the decision after D362):** libraries may overlap. Model (owner
   confirmed): the file is the base unit (scanned once, sidecars, watch progress); a file is part of at most
   one film, one run of episodes, one other video; a view may span files (versions, parts); same kind shares
   one view, different kinds get their own. Access: any library reaching it. Lift D359's no-overlap rule;
   remove the "mixed" kind (none exist). Same kind, conflicting sources: a person's match wins, else the
   NFO; doctor lists conflicts. Deleting a library deletes no item: uncovered items' data becomes reapable,
   reaped only under storage pressure (existing budgets), oldest first; watch progress stays with the file.
   Views stay collection members (D160). Migration. Test with staging's empty "shows" library.
   **With it (owner 2026-10-27):** lists show items, each once however reached (nested folders collapse to
   the outer). Byte-identical copies are one file at several locations (full read confirms); a person sees
   and plays only locations their libraries cover, never learns of others; sidecars per location. Different
   encodes of one other video stay separate (owner: no handling; the admin fixes the files).
   **After the overlap batch:** an inventory of every decided-but-unbuilt feature (decision statuses, plans,
   code-debt, the owner's asks; collections' UI, D160) for the owner to choose from.
   **Next batch, also (owner 2026-10-27):** a library's setting for whether its items show on the home page.
   Each person chooses for themselves (owner 2026-10-27), beside D349 §8's per-person leave-out.
   **Next batch, also (owner 2026-10-27):** the matcher checks a guess against the file's own length: fetch the
   candidates' runtimes and pass over one the file's duration contradicts (a margin for credits and PAL
   speed-up), then on to the year-off tier. Evidence: staging's "Crowd" (88 min, file 2022) took TMDB
   1120937 (an obscure 2022 "Crowd"); the real film is 1020910, TMDB-dated 2023 (festival 2022).
   **Next batch, also (found 2026-10-27, container sweep):** an H.264 MPEG-TS whose only keyframe is its first
   (one chunk for the whole film) fails to make its first copied chunk ("no init or no fragments"); affects
   .ts generally, not new. And readthrough's hasIndexReader doesn't know qt, f4v, 3g2 (reads them through
   early; order only).
   **Next batch (owner 2026-10-27):** films and series sort by release date too (now: name, date added; other
   videos: recorded). Every sort runs either way: dates newest↔oldest, names A–Z↔Z–A; unknown dates last.
   **Player fixes done** (staging): VQ78, VQ79 for the owner's devices.
   **Batch after (owner 2026-10-27):** D364, casting as a session (top-bar Cast, tray icon, ask once).
   **Settled (owner 2026-10-27):** D338 stays as it is: a receiver taking control back stays on the video.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" above (with its evidence), not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
