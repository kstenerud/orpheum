# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Now: this batch is built (main an earlier commit + the lead's UI fixes, committing).** D360 RealMedia + constant
   lists, the container sweep, D361 library pages, one scan per root path, D362 frame stills, the Play page's
   Back, the browse bar fixes, three posters a row on phones. Then: full staging deploy (node protocol 50),
   verify, and wait for the owner to say they are happy. Waiting on the owner: VQ76, VQ77; stray "lead
   matcher check" devices.
   **Gate before the next batch (owner 2026-10-27):** when this batch is done AND the owner says they are
   happy with it, deploy nas.example (production-on-nas-lan runbook; `make deploy DIR=/srv/orpheum` from main,
   untagged) and run one full check-pr first. Do not start the next batch until nas.example is deployed. SSH works
   (new key 2026-10-27; `ssh prod-nas` = 192.168.1.250; the name nas.example resolves elsewhere). nas.example runs "feat(interest): a season's page raises that season's episodes".
   **Next batch, first (decided 2026-10-27; record as the decision after D362):** libraries may overlap. A path is scanned/probed once; a library holds every item whose files lie under
   its folders; items are shared, not copied (watch, match, edits the same everywhere); same kind → one
   shared item, different kind → its own item (option 1): a video has at most one film view, one episode
   view, one other view. Access: any library reaching it. Deleting a library drops only its view. Lift
   D359's no-overlap rule. Schema change (migration). Staging's empty "shows" library tests it. Same kind,
   different metadata sources: a person's chosen match wins, else the NFO; doctor lists libraries whose
   sources conflict over the same items. Remove the "mixed" kind outright (owner: none exist anywhere; no conversion).
   Deleting a library never deletes items (owner 2026-10-27): an item no library covers stays, its data
   (match, edits, art, stills, watch state) becoming reapable; a library rebuilt one folder up or down finds
   it again intact; reaped only when the space is needed (storage pressure, oldest-orphaned first), never on
   a timer alone, through the server's existing storage budgets. Person-owned records (watch progress) stay with
   the file even when the item's data is reaped (owner agreed both, 2026-10-27).
   Model (owner confirmed): the file is the base unit (scan, sidecars, watch progress); a file is part of at
   most one film, one run of episodes of one series, one other video; a view may span files (versions,
   parts). Views stay collection members (D160: films, other videos and whole series in one ordered
   collection, e.g. all of Metropolis) — that decision must keep the collection_member links working.
   **After the overlap batch (owner 2026-10-27):** an inventory of every decided-but-unbuilt feature (decisions'
   status lines, `make handoff`, plans, code-debt, the owner's filed asks; collections' UI among them, D160)
   for the owner to choose from. Collections are "later" until then.
   **Next batch (filed 2026-10-27):** a Fire TV is named "Chrome on Android" (Silk's agent says "Android …
   like Chrome"; staging device 33): name it "Silk on Fire TV" (AFT model codes, as tv-device.ts knows).
   **Next batch, also (found 2026-10-27, container sweep):** an H.264 MPEG-TS whose only keyframe is its first
   (one chunk for the whole film) fails to make its first copied chunk ("no init or no fragments"); affects
   .ts generally, not new. And readthrough's hasIndexReader doesn't know qt, f4v, 3g2 (reads them through
   early; order only).
   **Settled (owner 2026-10-27):** D338 stays as it is: a receiver taking control back stays on the video.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" above (with its evidence), not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
