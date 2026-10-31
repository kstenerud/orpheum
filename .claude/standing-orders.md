# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Done:** the 10-27, 10-30 and 10-31 batches (nas.example runs an earlier commit; prod connect
   connect-production-656e4c4-9a4d70b6). Owed by the owner: VQ85–88, 90, 91, 93, 96, 97, 102(2), 103, 115,
   118, 120, 142, 194; VQ81's disk case; D368's loudness on stepping down. Deploy: `ssh prod-nas`, bundle
   fetched as bundle/sandbox-main, `make deploy DIR=/srv/orpheum`.
   **The 11-01 batch (owner 2026-10-31, "Let's do that", to the lead's proposal): fixes from the owner's
   testing, so the beta feels solid.** From the plan file: the bypass note's `registration.update()`
   (VQ194 again); security updates Parts A and B; noexec step 1; seamless audio track switching
   (design to the owner first); the cast seek jump-back (file 5680) and the first-cast freeze (file
   5671); B-frame re-timing by picture order at scan (design points to the owner first); Firefox's slow
   recovery; the `live:` socket through the shown-again check; main-thread freezes logged on every
   page; sign out confirms and lands on the discovery's sign-in; the Fire TV checkbox; a refused link
   said before the join form; "Identified correctly"; invite links kept (7 days); D384 §3; the
   specials-folder misreading. **Not this batch:** networking (LAN origin, WebRTC tuning, IPv4
   reach), which is the batch after; inventory features; the decoder-limit codec drop; the
   average-bitrate rung. End: one check-pr, the owner's go, nas.example.
   **PAUSED (owner 2026-10-31, quota at 91%):** no new agents. The three running (D393 key rework, the
   cast fixes, damaged-chunk checks) finish and are merged; then stop. Not started: connect's side of
   D391/D393, the release-workflow and install-default gate edits, check-pr, deploy. Next: token use,
   looked at by the lead alone (plan file, "Token use").
   **Queued for the owner to order later:** the rest of the plan file and inventory-2026-10-27.md.
   **Superseded (owner 2026-10-30, D385):** a receiver taking control back ends the cast and goes home; the sender plays on.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" in the plan file (with its evidence),
not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start. Small, well-located changes the lead makes itself; agents get
briefs naming files and mechanism (memory agent-step-count).
