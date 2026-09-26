> **ABOUTME:** Checks that only a person with the hardware can do — a film on a television, a
> voice call across two houses. A fix that no automated test can turn red names its item here
> in a `Verified-By: VQ<n>` trailer, and the revert-red gate accepts it only once it is ticked.

# Verification queue

[D190](decisions/working-notes.md#d190--the-testing-strategy-devices-report-on-themselves-against-numbers-a-missing-capability-fails-every-check-first-shows-it-can-fail-and-the-lab-is-built-from-what-is-on-the-shelf)
requires every behaviour change to be able to turn a test red when reverted. A few cannot: the
symptom shows only on a device, a network or a room. For those, the change adds an item here,
someone does it, ticks it, and the commit carries `Verified-By: VQ<n>`.
`scripts/check_revert_red.py` refuses the trailer if the item is missing or unticked. yoloAI's
free-text trailer was never used, because nothing checked it.

**Format**, one line per item, IDs never reused:

```
- [ ] **VQ<n>** — <what to do, on which device, and what "passes" means> (<commit or branch>)
- [x] **VQ<n>** — … — done <date> by <who>: <what was seen>
```

An item is ticked only by the person who did it, and only with what they saw.

## Items

- [ ] **VQ1** — On devnas (J3455), run
  `go test -run '^$' -bench HashPassword -benchtime 10x ./internal/core/auth` and read
  `BenchmarkHashPassword_DefaultParameters`. Passes at 500 ms/op or less
  ([D194](decisions/working-notes.md#d194--secrets-our-users-passwords-are-hashed-third-party-secrets-are-sealed-with-a-key-that-never-enters-the-database-and-nothing-reads-a-secret-back-out)
  §2); above it, the memory cost comes down to OWASP's 46 MiB, t = 1. For reference, an
  i3-N305 VM measured 50 ms. (M1)
- [x] **VQ2** — In Firefox on the dev loop, play a prepared film or episode with sound in its
  centre (any dev-library MKV but the stereo ones), and tick and untick **Dialogue boost**
  while it plays. Passes when the change is heard within about a second of the click, with at
  most a moment's pause. Before this change it took two to four seconds whatever the segment
  length. (the seek after an audio switch in `web/packages/player/src/hls.ts`) — done
  2026-09-26 by the owner: *"Yup, works within about a second now."*
- [x] **VQ3** — On an Android phone in Chrome, and in desktop Firefox, on the dev loop, play
  **Remux Film (2021)** and choose **English** under **Subtitles**. Passes when the text
  appears over the picture, styled as its ASS says, inside the picture's area and not under the
  controls, within a few seconds of choosing it. Then press **Later** three times: the offset
  reads +0.3 s and the text moves at once, paused or playing. Leave and play the film again:
  it starts at +0.3 s. (JASSUB in `web/packages/player/src/subtitles.ts`) — done 2026-09-26 by the owner, on an Android phone in Chrome: *"Subtitles render, and I've
  observed the subtitles arriving later and earlier when I adjust the offset"*, and the offset
  was still set after the page reloaded. Desktop Firefox not yet seen.
