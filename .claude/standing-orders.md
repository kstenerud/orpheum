# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Now: the next batch** (the local-path batch is on staging, main an earlier commit; VQ76 the cast prompt works on
   Fire TV: ask, full screen, sound, double OK). In order: subagent cleanup (building), then the rest below.
   **This batch (owner 2026-10-27):** build D357, the rescan + re-match (per item, per library, all film and
   series libraries). Staging's "Crowd" (2022) is left wrongly matched on purpose: verify D357 on it.
   **This batch, also:** a cast receiver on a desktop opens no controls on a mouse move or click
   (tv-controls.tsx opens only on keys): D338 §5 unmet on a mouse-driven receiver (owner 2026-10-27).
   **This batch, first (owner 2026-10-27, gate work):** deterministic subagent cleanup (tag + kill at stop,
   sweep at each prompt, merged worktrees removed at a turn's end). Building.
   **This batch, also (owner 2026-10-27):** after the receiver takes control back, the sender's "Play here from
   ‹position›" plays the film from 0:00, not where the receiver was (like bug 1's playback asked for without
   its position).
   **This batch, also (owner 2026-10-27):** on Fire TV the receiver's first Back only leaves full screen (Silk
   takes the key); a second takes control back. Leaving full screen we did not ask for, while receiving,
   counts as Back. Then VQ76 can be ticked.
   **Settled (owner 2026-10-27):** D338 stays as it is: a receiver taking control back stays on the video.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" above (with its evidence), not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
