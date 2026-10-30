# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Done 2026-10-30:** the 10-27 batch (D364, D367–D377, the Fire TV, casting and Watch Together fixes):
   check-pr green but for unticked VQs; the owner's "deploy anyway"; nas.example runs an earlier commit (`ssh
   prod-nas`, bundle fetched as bundle/sandbox-main, `make deploy DIR=/srv/orpheum`); prod connect
   0a3fcd5-216be59e. Owed by the owner: VQ85–88, 90, 91, 93, 96, 97, 102(2), 103, 115, 118, 120, 142;
   VQ81's disk case; D368's loudness on stepping down.
   **This batch (owner 2026-10-30: cosmetic issues and what makes testing easier), from
   design/plans/navigation-and-widgets-answers-2026-10-27.md:**
   - Cosmetic: the session-ended notice as a modal; a title page's one "Identified incorrectly" entry
     opening a modal (Rescan, Enter manually, Report this), Play first; season posters in place of the
     season choice box, rare items under a "…" menu; a modal opened from a choice box swaps with it,
     never stacks; the watched check over an episode's still; the Libraries icon as a menu; Watch
     Together's invite pop-up closes on an outside click; the Fire TV's black blip at a start (the player
     chooses its first rung); the local-network note only where the browser asks (not Silk).
   - Testing: item 7 whole (D372 §9–§10: a signed-in guest's page has the banner, the guest stays a
     member while elsewhere and catches up on return; a link guest returns by the same link); a debug
     link throttle and connection drop (Show debug tools); the page says when DevTools bypass its
     worker; the cast remote's sound controls and the receiver's quality/link on the sender; core and
     discovery find each other whatever the restart order; pagehide gives up the lead.
   - Process (owner 2026-10-30): D378 built (unticked VQs owed, not failures; web reds keyed by their
     tests' modules); CI frees the runner's disk; files near 800 lines split; a security audit of every
     pin (CVEs), bumps proposed to the owner.
   - **Not this batch (owner):** the remembered link kept across deploys (a clean slate on deploy).
   - End: one check-pr, the owner's go, nas.example.
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
