# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Now: this batch is on staging (quick deploy), waiting for the owner to say they are happy.**
   D360 RealMedia + constant lists, the container sweep, D361 library pages, one scan per root path, D362
   frame stills, the Play page's Back (up to its film, series season or library when nothing is behind it),
   the browse bar fixes, three posters a row on phones. VQ76 and VQ77 passed (owner 2026-10-27).
   **Gate before the next batch (owner 2026-10-27):** when this batch is done AND the owner says they are
   happy with it, deploy nas.example (production-on-nas-lan runbook; `make deploy DIR=/srv/orpheum` from main,
   untagged) and run one full check-pr first. Do not start the next batch until nas.example is deployed. SSH works
   (new key 2026-10-27; `ssh prod-nas` = 192.168.1.250; the name nas.example resolves elsewhere). nas.example runs "feat(interest): a season's page raises that season's episodes".
   **Next batch, first (decided 2026-10-27; the decision after D362):** libraries may overlap. Model (owner
   confirmed): the file is the base unit (scanned once, sidecars, watch progress); a file is part of at most
   one film, one run of episodes, one other video; a view may span files (versions, parts); same kind shares
   one view, different kinds get their own. Access: any library reaching it. Lift D359's no-overlap rule;
   remove the "mixed" kind (none exist). Same kind, conflicting sources: a person's match wins, else the
   NFO; doctor lists conflicts. Deleting a library deletes no item: uncovered items' data becomes reapable,
   reaped only under storage pressure (existing budgets), oldest first; watch progress stays with the file.
   Views stay collection members (D160). Migration. Test with staging's empty "shows" library.
   **After the overlap batch (owner 2026-10-27):** an inventory of every decided-but-unbuilt feature (decisions'
   status lines, `make handoff`, plans, code-debt, the owner's filed asks; collections' UI among them, D160)
   for the owner to choose from. Collections are "later" until then.
   **Next batch, also (owner 2026-10-27):** a library's setting for whether its items show on the home page.
   Each person chooses for themselves (owner 2026-10-27), beside D349 §8's per-person leave-out.
   **Next batch, also (owner 2026-10-27):** the matcher checks a guess against the file's own length: fetch the
   candidates' runtimes and pass over one the file's duration contradicts (a margin for credits and PAL
   speed-up), then on to the year-off tier. Evidence: staging's "Crowd" (88 min, file 2022) took TMDB
   1120937 (an obscure 2022 "Crowd"); the real film is 1020910, TMDB-dated 2023 (festival 2022).
   **Next batch (filed 2026-10-27):** a Fire TV is named "Chrome on Android" (Silk's agent says "Android …
   like Chrome"; staging device 33): name it "Silk on Fire TV" (AFT model codes, as tv-device.ts knows).
   **Next batch, also (found 2026-10-27, container sweep):** an H.264 MPEG-TS whose only keyframe is its first
   (one chunk for the whole film) fails to make its first copied chunk ("no init or no fragments"); affects
   .ts generally, not new. And readthrough's hasIndexReader doesn't know qt, f4v, 3g2 (reads them through
   early; order only).
   **Next batch, also (owner, VQ77 2026-10-27):** on Chrome's cast receiver (the Mac) neither a click on the
   video nor Space pauses it; only the on-screen pause button does. Likely: Space is "select", which pauses
   only with focus on the video (player-commands.ts), and the receiver's click shows controls. Check the
   ordinary Play page too; if it has it, it is serious and joins the current batch.
   **Settled (owner 2026-10-27):** D338 stays as it is: a receiver taking control back stays on the video.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" above (with its evidence), not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
