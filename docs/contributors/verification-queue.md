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

*(none yet)*
