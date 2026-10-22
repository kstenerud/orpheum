> **ABOUTME:** The final audit of the data-timeliness scenarios before design, by an independent
> data-systems expert and an independent UX expert. It covers where the document disagreed with
> established practice or with itself, what was missing that would change the design, and how each
> finding was resolved in the plan. It proposes; the plan and the owner decide.

# Data timeliness: final audit, 2026-10-22

**What prompted it.** The owner, before design began: *"Do one last audit of this before we begin
design. Make sure we're in accordance with the experts in these fields."*

**How it was done.** Two agents audited
[data-timeliness.md](../plans/data-timeliness.md) as it stood at an earlier commit, independently of each
other:

- **A data-systems expert:** scheduling, admission, caching, reliability, video delivery.
- **A UX expert:** perceived performance, streaming products, TV, accessibility, households.

Each read the earlier [review](data-timeliness-review-2026-10-22.md) and the
[follow-ups](timeliness-followups-2026-10-22.md), and checked claims against sources. Their caches
are `/yoloai/cache/http/final-audit-systems/` and `final-audit-ux/`. Both verdicts were **ready for
design with fixes**. The direction matches expert consensus. The fixes concern contradictions
inside the document and guarantees it couldn't keep.

## 1. The cited research, checked

The UX auditor checked the UX sources that the first review had cited from memory.

**Correct as cited:**
- **Nielsen's 0.1 / 1 / 10 s limits** ([NN/g](https://www.nngroup.com/articles/response-times-3-important-limits/)).
- **Krishnan and Sitaraman:** abandonment starts at 2 s, then +5.8% per second.
- **YouSlow:** one rebuffer causes three times the abandonment of one bitrate change.
- **Rehman and Wang:** drops are punished more than rises are rewarded.
- **Maister's propositions** on uncertain and unexplained waits, which are verbatim.
- **Harrison et al.:** a pause near the end of a progress bar feels slowest.
- **Viget's skeleton test:** 136 people, skeleton worst. It is an agency study, not peer-reviewed.

**Needed their scope stated:**
- **YouSlow measured short YouTube videos.** It also found that *raising* the bitrate often raised
  abandonment up to fourfold, which supports a lowered stream rising "once, one step".
- **Krishnan and Sitaraman found long-form viewers more patient, and well-connected viewers less.**
  So a 2 s start is a conservative target for films. It is not a figure that forces trade-offs, and
  the stricter LAN target is justified.
- **Their "failure lowers return" finding concerns a failure to play.** It does not justify resuming
  a few seconds before a freeze. That remains good practice, but on its own merits.
- **"Usually 0.3 s, sometimes 8 s is worse than always 1.5 s" had no citation.** It is judgement,
  consistent with Shneiderman's guidance on response-time variability.

The systems auditor confirmed:
- RFC 9113's deprecation of HTTP/2 priorities;
- SRE's four criticalities, set at the trusted frontend and not by the client;
- SQLite WAL's single writer;
- the default NFS `acdirmax` of 60 s;
- that gRPC carries deadlines on the wire as a relative timeout.

It found one overreach: the review's claim, marked unchecked, that reading once for two needs is
"the cheapest idea". It is not cheap (§2, S6 below).

## 2. Systems findings

### Must-fix

1. **S1. Leases cannot carry work nobody is watching.**
   - Patient work ("does something else and comes back") and Due work have no live client. A
     lease-only want would be cancelled when the admin closes the app after "Scan now".
   - **Resolution:** two kinds of want. *Ephemeral* wants are held by a session. *Durable* wants are
     persisted, owned by the server, and ended only by completion or an explicit cancel (plan
     §3.4).
2. **S2. A client could claim a higher class.**
   - SRE sets criticality at the trusted frontend.
   - **Resolution:** a client declares facts (what it shows, plays and predicts). The core derives
     the class from them. Leases and predictions are capped per holder, and guests have their own
     caps (§3.4).
3. **S3. Quiet hours and the idle trigger contradicted the scenarios.**
   - A scan "taking what the playback leaves" contradicted "stops when a playback starts".
   - A backup due tonight could be vetoed past its night.
   - Batches "right after a playback" mostly fall after 22:00.
   - **Resolution:** a table gives each kind of work its tier and whether it obeys the veto, the
     idle trigger and the no-wake rule. Quiet hours govern media-disk I/O only. A latest start
     overrides the veto. The household's time zone governs quiet hours (A8).
4. **S4. The rules about waking a disk contradicted the deadlines.**
   - Returning to the app triggers B5's freshness check, which is a timer in disguise.
   - Warming Continue Watching could wake a disk.
   - P1's 2 s p95 can't hold if the disk sleeps.
   - **Resolution:**
     - predictions never wake a disk;
     - the freshness check has a minimum interval per root;
     - P1's targets are for a disk that is awake, and a wake has its own target (W7);
     - B5's row is marked not in time on a share until NFS revalidation is measured.
5. **S5. A "reservation" promises what Orpheum can't keep.** It owns neither the NAS nor the
   household uplink.
   - **Resolution:**
     - it is called *admission against an estimate, protected by feedback*;
     - the uplink is measured, not configured;
     - "peak" is a token-bucket rate and burst;
     - who yields when capacity itself shrinks goes to the owner (plan §11).
6. **S6. The second transcode was both "best effort" and "strict admission".** An overcommitted
   encoder stalls both streams (jellyfin-chromecast #40).
   - **Resolution:** best effort is defined mechanically. It runs at a lower CPU weight, is never
     charged against the first stream's lead, and is stepped down or stopped first when that lead
     shrinks (W11).
7. **S7. Absolute deadlines across machines with wrong clocks.**
   - **Resolution:** a want carries a remaining budget, converted at each hop to that machine's
     monotonic clock, as gRPC does. Quiet hours are in the household's IANA time zone, with rules
     for daylight-saving changes (§3.4, A8).
8. **S8. The SQLite writer is a scheduled resource.** Jellyfin #14366 showed a scan stalling
   playback through it.
   - **Resolution:** a §9 item. Write transactions are bounded and sliced. Play-state writes go
     before scan batches. Leases are never written per renewal.

### Should-fix

All are folded into the plan:

- **"Hard" misnamed a p95 target.** It is renamed **visible**, and whether a value must be right is
  kept apart from whether it is on time.
- **Percentiles over a household's handful of events need a window**, and a rule per event: never
  unexplained past 3 s.
- **Continuous work and interactive work are separate.** A reservation sits outside the priority
  order, as mClock separates reservation from weight.
- **"One decider per resource" needs coordination.**
  - The uplink spans nodes and the relay.
  - A playback needs disk, encoder and uplink together, so admission is all-or-nothing in a fixed
    order.
  - For fairness: Dominant Resource Fairness or deficit round robin.
- **Learning a limit from latency is weak at concurrencies of 1–3 over NFS.** Control on the
  playback's lead over the playhead, as io.latency protects a group. Throttle background work by
  rate, and treat latency as a hint.
- **Reading once for two needs is an opportunistic consumer**, with a map of what it has covered.
  - EBU R128's integrated loudness merges only through a histogram of block loudness.
  - Nothing may depend on it.
  - The risk is scope, which the owner accepted in deciding to build it now.
- **P4's always-subtitles row was missing "not in time cold".**
- **Remote direct play rarely fits a 20 Mbit/s uplink.** A 1080p remux peaks at 20–40 Mbit/s, so
  "direct play preferred" is mostly a LAN rule, and remote play is mostly a transcode.
- **A risk of the owner's multi-viewer decision:** whoever arrives first can hold the uplink all
  evening.
  - Mitigation: give the newcomer an estimate of how long the wait is.
- **Shadow mode fits a stateful scheduler poorly,** and a household sees little overload. Prefer
  trace replay, fault injection on staging (netem, dm-delay) and a canary. The simulation must model
  reads stuck in the kernel and the 48 s NFS recovery.
- **Overload inside a tier needs rules:**
  - per-holder caps;
  - a minimum share for background work;
  - an order of shedding when a provider's quota runs out.
- **Wants need an identity:** an idempotency key, and a key for the work they refer to.

## 3. UX findings

### Must-fix

1. **U1. "The admin already knows" can't hold with a badge and a next-day email alone.**
   - **Resolution:** an opt-in immediate channel for issues touching a playback now, silenced in
     quiet hours, goes to the owner (plan §11). Until the owner decides, A6 says plainly when the
     admin learns.
2. **U2. Bare, unidentified entries break parental limits.**
   - **Resolution:**
     - a restricted profile sees nothing until a rating exists;
     - guests and users see a cleaned title, never the raw filename (B5, §6).
3. **U3. B5's unreadable-file text was written for the admin and shown to everyone.**
   - **Resolution:** split by audience (B5).
4. **U4. With the administrator taken out, users had no recourse, and "it will pick up by itself"
   was false for lasting faults.**
   - **Resolution:**
     - messages are split into *transient* and *lasting*;
     - "will resume by itself" is said only when a probe can recover the cause;
     - lasting faults say "This has been reported", which names no one. That wording goes to the
       owner (§5.4, §11).
5. **U5. Captions are part of the film, not an extra.**
   - **Resolution:**
     - for a profile that always uses subtitles, the start waits for them and says so;
     - SDH tracks are preferred;
     - image subtitles are rendered on the client, so they never take the encoder (P4).
6. **U6. No accessibility model for loading, stale and error states.**
   - **Resolution:** a §5.5:
     - every "Meanwhile" state has a text form that is announced (WCAG 4.1.3);
     - poster arrivals are silent;
     - nothing relies on colour alone;
     - reduced motion is honoured;
     - countdowns can be extended or switched off (WCAG 2.2.1).
7. **U7. An HDR popup on every play becomes noise, and may lead to a failure.**
   - **Resolution:**
     - the device's ability to decode the file is checked first;
     - the warning is shown once per device and profile, with "Don't warn me again on this
       screen";
     - after that, a note in the player's info;
     - a device that can't decode the file is told so plainly (W11).
8. **U8. "Wait or play reduced" had no default.**
   - **Resolution:**
     - the default focus is "Play now", worded as picture quality;
     - a child's profile skips the choice;
     - the choice is remembered on the device for the evening;
     - an upgrade is offered when room appears;
     - an estimate is given without naming the other film (P5).

### Should-fix

All are folded into the plan:

- **Badges:**
  - a household banner for an outage affecting a whole place, and per-film badges only for one
    file's faults;
  - clearing rules;
  - the email groups a share that flaps.
- **Indicators:**
  - an indicator only for waits expected past about 1 s, held at least 500 ms;
  - a percent-done indicator beyond 10 s;
  - a bar that eases near its end and changes its wording at the expected maximum.
- **Start targets per path:** 1.5 s for any known cause.
- **B7's reorder** happens before focus lands.
- **"Availability can never be stale"** is reworded: never stale without a label.
- **The jump bar:**
  - uses the viewer's locale, on the localized title;
  - skips leading articles;
  - is hidden when the view isn't sorted by title;
  - dims empty letters on touch and web, and hides them on a TV.
- **Internationalisation:**
  - a message catalog with plural rules;
  - relative ages through `Intl`;
  - time zones for quiet hours, the email and reported times.
- **Privacy:** users are told what the admin can see.
- **A5's removal** gets an undo.
- **Age labels** only where the age matters.
- **The owner's recourse under "all equal":** stopping a stream by hand, as Plex and Jellyfin
  allow, goes to the owner (§11).

## 4. What both auditors endorse

- **Deadlines belong to wants**, and a prediction never inherits a waiting person's class.
- **Few absolute tiers**, with EDF only inside one.
- **Slicing long work**, shallow lower queues, and rate limits on background reads.
- **Admission for playback** that protects running streams. Direct play is preferred.
- **Failures:** breakers, gradual recovery, retry budgets.
- **Caching:** negative caching, availability as a lease, and three freshness states.
- **Presentation:**
  - an honest Play;
  - backdrop and title during a start;
  - an explanation by 3 s;
  - placeholders of the final size, and nothing reflowing under a TV's focus.
- **The jump bar from CLDR and ICU index characters.**
- **A digest email**, and quiet hours as a veto with idleness as the trigger.
- **Two audiences for every failure.**
