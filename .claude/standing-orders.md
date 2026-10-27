# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Now: the next batch** (the local-path batch is on staging, main an earlier commit; VQ76 the cast prompt works on
   Fire TV: ask, full screen, sound, double OK). In order: subagent cleanup (building), then the rest below.
   **This batch (owner 2026-10-27):** build D357, the rescan + re-match (per item, per library, all film and
   series libraries). Staging's "Crowd" (2022) is left wrongly matched on purpose: verify D357 on it.
   **Done:** subagent cleanup (the ten old worktrees removed). D358 note: desktops go full screen too.
   **Done:** the three cast fixes (device check VQ77). **Done:** D357, D359 the folder
   picker (owner 2026-10-27: any subfolder of a node's root, new and existing
   libraries, hidden folders left out). At deploy: staging's [media] roots become /tank/media and
   /home/yoloai/synctest (full ./staging, not quick). **Owner away (2026-10-27): deploy as soon as the batch
   is ready, even if a device shows as viewing** (this batch only).
   **Settled (owner 2026-10-27):** D338 stays as it is: a receiver taking control back stays on the video.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" above (with its evidence), not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
