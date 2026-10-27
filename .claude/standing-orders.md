# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Now (owner 2026-10-27):** the public-IP playback is explained (local-path-2026-10-27.md: Firefox never
   tries the LAN name; staging's UDP 8443 is not published from the VM; the IPv4-only core can't prove the
   client is home); waiting on the owner's choice of fixes. Owner questions open:
   D352 Q2 (where the steadiness level is remembered: core beside the speed, or browser keyed by a network
   name the core hands out); D353's per-stream positions (now, or code debt). Done 2026-10-27: audio switch
   at 0, seek wait and backup snapshot, the first measurement (13009698),
   D353 calculated seek threshold, D352 adaptive bitrate controller.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
