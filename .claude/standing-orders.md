# Standing orders (the owner's; kept current by the lead, read at every prompt)

1. **Done 2026-10-27:** the process review (all of sets 1–3, F12, F1; F11a dropped).
2. **Done 2026-10-27:** the batch of D360–D362, D361's pages, the Play page's Back: accepted, on nas.example.
   `ssh prod-nas` (192.168.1.250): runs an earlier commit (bundle, branch sandbox-main), root /tank/media.
   **This batch (owner 2026-10-27: "go ahead with the next batch"), on staging as it lands:**
   - Done: D363 overlap and copies (owner accepted its Test-Weakened; VQ80 passed, VQ81 mostly), D365
     runtime matching (Crowd fixed), the rescan's hour-long wait, the player fixes (VQ78 passed; VQ79
     owes the new device name), the MPEG-TS seek, readthrough's formats.
   - Icon component done. Agent now: D366 (sorting both ways, release date, each person's Home
     libraries). Brave on the Mac: all fine (owner, 2026-10-27).
   - Then: one full check-pr, the owner's go, nas.example (step 0060 there is VQ82); then the inventory of every
     decided-but-unbuilt feature (decision statuses, plans, code-debt, the owner's asks; collections' UI,
     D160) for the owner to choose from (owner: "once the overlap batch is done"), with EVERYTHING queued
     beside it, the next batch's items included: the owner decides what happens when (owner 2026-10-27).
     The next batch below is a proposal until then.
   **Next batch (filed 2026-10-27):**
   - D364, casting as a session (top-bar Cast, ask once, tray icon on the sender, the remote a page of its
     own, one place for controls beside Watch Together); `CastIcon` in status-tray.tsx is the owner's icon.
   - Our own player controls on computers too, replacing Chrome's, reachable by arrows (owner chose).
   - From VQ81: a play whose file is gone at one place (renamed, deleted) answers 404 instead of trying
     the person's next reached place; a vanished library folder is not marked unavailable when its scan's
     listing is refused ("no such file"), still "complete".
   - The ladder's top rung plays the film's own sound, copied (5.1 where the device plays it), lower rungs the
     chunk_sound_kbps stereo (owner 2026-10-27: nas.example's Chrome play heard "AAC stereo 96 kb/s, transcoded
     from AAC stereo" at the top rung); HLS audio groups per variant; test a switch's blip on devices. And
     the play line's `delivery=direct` on a chunked play says what was sent (startsOnLadderKbps was set).
   **After the icons, D364 and our own controls (owner 2026-10-27):** a DRY audit of the TS code toward a
   standard set of UI widgets, findings first, then the consolidation.
   **Settled (owner 2026-10-27):** D338 stays as it is: a receiver taking control back stays on the video.
3. **Only then:** new work (Diagnostics, D341, D345, the queued items), and code-fixing batches from
   docs/contributors/code-debt.md when the owner schedules one.

**Batching (owner 2026-10-27):** a bug found mid-batch that seriously hurts using the app is folded into the
current batch; any other bug or request is filed under "Next batch" above (with its evidence), not started.

Velocity is a resource (principles §25): 30 s cadence, 5 min only when the protection is worth it, one
comprehensive run per batch. Poor code is a recurring tax (§26): an improvement spotted is fixed if small
and in code being changed, else recorded in code-debt.md, never dropped. Finish what is asked before
proposing side work; propose, don't start.
