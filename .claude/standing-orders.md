# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Done 2026-10-27:** the overlap batch (D363, D365, D366, icons, player fixes): check-pr passed, on nas.example.
   `ssh prod-nas` (192.168.1.250): runs an earlier commit (bundle, branch sandbox-main), root /tank/media.
   VQ82 passed. Owed by the owner: VQ81's disk case.
   **This batch (owner 2026-10-27: "go with your proposed batch", from inventory-2026-10-27.md):**
   - Phase 1, agents: (a) VQ81's two bugs (a gone file falls back to the person's next reached place; a
     vanished library folder is marked unavailable); (b) D368, the top rung's own sound + the play line's
     delivery field; (c) D369, the standard player widgets, then our own controls on every device.
   - Phase 2, after (c): D364 casting as a session, revised first by the owner's answers (no top-bar items;
     the two tray icons white, blue when on; the on/off table; Watch Together's page holds the controls;
     leaving the film no longer stops it; no mini player); D367 revised (Up never adds history: back to the
     nearest parent behind, else replace; one series page with a season picker preselecting the season
     left; a trail on computers and phones, "‹ Parent" on TVs); the library Configure dialog.
   - Folded in (owner 2026-10-27, serious): (d) the Fire TV still named "Chrome on Android" (VQ84 failed;
     its sign-in was through the discovery's page); no way to sign out on the Fire TV; a device removed
     elsewhere keeps showing its page until touched (should clear at once); and then lands on the core's
     sign-in, not the discovery's. (e) Watched marks: D370 (owner agreed the
     defaults; a white check, no fading). (f) RealMedia cook sound never cut right (file 15102 on nas.example;
     never record that library's names). D369, D368, the Fire TV fixes, the RealMedia fix (protocol 54), D370 (step 0062), D364, D367 + Configure
     merged, on staging; VQ81 fixed. check-pr: all green but revert-red's device-only files, which
     wait on the owner's VQ85, VQ86, VQ87, VQ88, VQ90, VQ91 (+VQ89); then check-pr-failed, then the owner's go
     for nas.example. Connect packaged for prod: /yoloai/files/connect-production-0a3fcd5-2336a94f.run (VQ84, VQ87).
     Owner to choose (D368's status): loudness when stepping down from the copied sound. Runs overnight.
   - Folded in 2026-10-28 (owner, serious), all on staging unless noted: Fire TV cast fixes, phone banner,
     chooser/menu as one modal drop-down, refused key → sign-in (needs connect re-pin + repackage for prod),
     D250 §2 amended; **D371** (a session's leader plays as a device alone, everyone catches up, a cast takes
     its sender's part; VQ101–103); **D372 §1–8** (lead passes to the longest-present player; one session per
     person; VQ104–106; §9–10 next batch); the start rule (film ahead outlasts next chunk's fetch × 1.5,
     on the remembered link, VQ110–114); dropped frames logged; the WebRTC relay cap (skew-safe, VQ115).
     No ABR drop with ≥20 s loaded + robust spread; a sound switch keeps loaded sound (VQ116).
     Owner, after the network reviews: the efficiency audit's cheap fixes this batch (after the TV's
     Throughput run on WebRTC); LAN-origin tab at home + WebRTC tuning next batch (plan file, last section).
     Owner VQs owed: 85–100 open (84 passed, its package on prod-api).
   - Answers to build from and **next batch**: design/plans/navigation-and-widgets-answers-2026-10-27.md
     (the guest's words, the warning sign's page, the uncertain match, season posters, "…" menu, the
     player's controls, the nameless-series parser, the Libraries icon as a menu, invitation links shown
     again with a 7-day expiry).
   - End: one check-pr, the owner's go, nas.example.
   **Queued for the owner to order later:** the rest of inventory-2026-10-27.md (132 items).
   **Settled (owner 2026-10-27):** D338 stays as it is: a receiver taking control back stays on the video.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" above (with its evidence), not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
