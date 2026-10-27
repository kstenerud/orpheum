# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Batch done and on staging (2026-10-27, main an earlier commit):** subagent cleanup, the cast fixes, D357 (Crowd
   verified), D359 the folder picker (roots /tank/media, /home/yoloai/synctest), and the watcher fix: the node
   watches only folders of libraries set to "watch" (staging: "shows", 894 watches). Waiting on the owner:
   device checks VQ76 and VQ77; D357's rescan state in cache.db (lead recommends keeping); stray "lead
   matcher check" devices on staging to delete. Next: whatever the owner files.
   **Settled (owner 2026-10-27):** D338 stays as it is: a receiver taking control back stays on the video.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" above (with its evidence), not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
