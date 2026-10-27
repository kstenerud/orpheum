# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Now:** the process review's last two items: F12 (handoffs generated from the decisions' status),
   then F1 (the gate container). Narrow tests while working; ONE make check and ONE commit per batch.
2. **Then:** the three open playback bugs recorded 2026-10-26 (audio-track switch restarts at 0; a seek loops
   on an init.mp4 no run covers; the ABR ceiling never rises), and the backup "file already exists" bug.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
