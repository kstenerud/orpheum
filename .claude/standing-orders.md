# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Now: deploy the local-path batch to staging** (main an earlier commit: D352–D356, D358 the cast prompt, the
   matcher fix; connect pinned an earlier commit): check for live playbacks first, then `./staging quick` and redeploy
   connectd. Device check owed: VQ76 (the cast prompt on Fire TV and Chrome). yoloAI UDP bug report:
   /yoloai/files/yoloai-bug-udp-ports.md.
   **Next batch (owner 2026-10-27):** build D357, the rescan + re-match (per item, per library, all film and
   series libraries). Staging's "Crowd" (2022) is left wrongly matched on purpose: verify D357 on it.
   **Next batch, also:** a cast receiver on a desktop opens no controls on a mouse move or click
   (tv-controls.tsx opens only on keys): D338 §5 unmet on a mouse-driven receiver (owner 2026-10-27).
   **Next batch, also (owner 2026-10-27, gate work, owner approved):** deterministic subagent cleanup. (1)
   pre_bash rewrites each subagent command: own process group, an agent-id tag, </dev/null, a time limit on
   background ones (~30 min); on_subagent_stop kills every process with that tag. Confirm first that a
   PreToolUse hook may rewrite the command (updatedInput); if not, come back to the owner. (3) a sweeper at
   each prompt kills or reports processes older than a few hours whose agent finished. (4) a script, run when
   a turn ends, removes a worktree and its branch once all its commits are in main by patch-id and its tree
   is clean; one holding anything else is listed, never removed. Evidence: a review agent's `rg` with no path
   waited on stdin for 12 h (killed 2026-10-27); nine merged worktrees left behind.
   **Settled (owner 2026-10-27):** D338 stays as it is: a receiver taking control back stays on the video.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" above (with its evidence), not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
