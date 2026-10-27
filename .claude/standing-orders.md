# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Now: the local-path batch (owner 2026-10-27).** Wave 1 on main and staging (D354 hairpin + path logging,
   warning kept as built; D355; D352 Q2 + its setting). Building: D356 (owner agreed all five answers as
   recommended; agent running, touches orpheum-connect too, re-pin connect after main moves). Then ONE make
   check, commit, deploy staging (check for live playbacks first). Verified on staging 2026-10-27: hairpins
   named `hairpinned_ipv4`. yoloAI UDP bug report: /yoloai/files/yoloai-bug-udp-ports.md.
   **Next batch (owner 2026-10-27):** build D357, the rescan + re-match (per item, per library, all film and
   series libraries). Staging's "Crowd" (2022) is left wrongly matched on purpose: verify D357 on it.
   **Next batch, also:** cast receiver on a desktop (owner 2026-10-27, Firefox Mac → Chrome Linux): (a) no
   controls, as tv-controls.tsx opens only on keys (OK/Up/Down/arrows), never on a mouse move or click: D338 §5
   unmet on a mouse-driven receiver; (b) Chrome started the cast muted: its autoplay policy refuses sound to a
   play no gesture on that tab started; check the "turn the sound back on" prompt shows, and build (owner
   agreed) a tab waiting to receive casts asking once, while it has had no click, "Click to receive casts with
   sound", which grants its activation; add the Chrome rule to idiosyncrasies.md.
   **Next batch, also (owner 2026-10-27):** a receiver that takes control back (Back or a double OK) returns to
   the page it was on before the cast (the owner's Chrome was on the films listing), as Stop casting does
   (D338 §1): this replaces D338 §6's "its own playback, which plays on there"; record as an amendment to D338.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" above (with its evidence), not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
