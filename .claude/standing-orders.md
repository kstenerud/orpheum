# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Done:** the 10-27, 10-30, 10-31 and 11-01 batches. The 11-01 batch (fixes from the owner's
   testing) is built, passed check-pr at "refactor(web): reportReloadForNewerClient is reportReloadNote" and is deployed (2026-11-01): nas.example runs an earlier commit
   ("refactor(web): reportReloadForNewerClient is reportReloadNote"'s tree), production runs connect-production-9cbe6f0-d35c68e1 at D390's names (orpheum.cc,
   account.orpheum.cc; the API on orpheum-app.com), realm `karls`. Owed by the owner: VQ85, 87, 88, 90, 91,
   93, 96, 97, 102(2), 115, 118, 120, 194, and the 11-01 batch's (VQ195–201, 204–207,
   215, 220, 221, 225, 226, 230, 231); VQ81's disk case; D368's loudness on stepping down; the open items in the plan
   file's "From the production deploy" section. Deploy: `ssh prod-nas`, `make deploy DIR=/srv/orpheum`.
   **The 11-02 batch (owner 2026-11-01, agreed to the lead's proposal):** networking and the beta made
   solid, as the plan file's "The 11-02 batch" lists: the iPhone's full screen (iOS's own player),
   the LAN origin (decision first), WebRTC for Chromium, recovery, a page that cannot reach its core
   says why, playback's start, the deploy's small items, two gate fixes (not check-pr's time). The
   owner checks devices alongside (`/yoloai/files/testing-plan-2026-11-01.md`). End: one check-pr, the
   owner's go, nas.example. **Not this batch:** inventory features; the decoder-limit codec drop; the
   average-bitrate rung; the cosmetic UI items.
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
