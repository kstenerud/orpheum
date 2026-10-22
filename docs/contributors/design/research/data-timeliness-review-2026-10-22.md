> **ABOUTME:** Four independent readings of the data-timeliness sketch: a data-systems review, a UX
> review, and prior art from the streaming industry and from other fields (collaborative editing,
> web performance, OS I/O scheduling, distributed systems, game engines). It says where the sketch
> agrees with what experts have published, where it is wrong or missing something, and what a
> redesign should borrow. It proposes and does not decide.

# Data timeliness: review and prior art, 2026-10-22

**What prompted it.** The owner asked for the sketch
[data-timeliness.md](../plans/data-timeliness.md) to be reviewed by an independent agent, *"from the
pov of a data expert and a ux expert. Also research what others have to say about this kind of thing
and compare. This ground has been covered many times before … We don't even need to constrain
ourselves to this industry. Other things like distributed document viewing and collaborative
editing would encounter similar issues."*

**How it was done.** Four agents worked in parallel, each reading the sketch and the
[queue inventory](job-queues-inventory-2026-10-21.md):

1. A data-systems reviewer (scheduling, queueing, caching, resource management).
2. A UX reviewer (perceived performance, streaming products, wait psychology, households).
3. A researcher of the streaming industry (Netflix, Google, Meta, Mux, Twitch, Plex, Jellyfin, Emby).
4. A researcher of other fields (collaborative editing, web performance, OS I/O, distributed
   systems, game engines).

The researchers fetched and cached their sources under `/yoloai/cache/http/timeliness-industry/`
and `/yoloai/cache/http/timeliness-crossfield/`.

**How far to trust the sources.** The two researchers say which sources they read in full and which
only from search excerpts or summaries. Their notes are carried below where they matter. The two
reviewers cited established literature from memory and fetched nothing; their citations are
well-known works but **were not checked here**. Where a claim rests only on a reviewer's citation it
is marked *(unchecked)*.

## Summary

- **The sketch's core conclusions hold up against everything found.** Every source that schedules
  well agrees that a deadline belongs to a want, not to a kind of job (§7.1), and that a want has a
  holder and a lifetime (§7.2). It also agrees that priority should be computed from what people
  want now rather than stored in jobs, that costs are a vector (§7.4), that continuous deadlines
  need a reserved share (§7.3), that prediction pays (§7.5), and that freshness is a separate axis
  (§7.7). Examples:
  - OpenLayers drops map tiles that left the view.
  - Unreal Engine recomputes texture streaming every frame from the camera.
  - Netflix separates a request a person made from a prefetch the app made.
  - Google SRE keeps criticality separate from latency.
  - Zero types each query result as unknown, cached or complete.
- **The single biggest correction is that work must be made preemptible, and the queues below the
  scheduler shallow.** All four reports reach this from different directions:
  - *The Tail at Scale*: "keep low-level queues short so higher-level policies take effect more
    quickly", and break long work into slices.
  - Linux BFQ: a budget timeout on each turn.
  - Google's RAIL: idle work in 50 ms chunks.
  - Cloudflare: "excessive buffering is pretty much the nemesis" of priority.

  No priority scheme fixes head-of-line blocking behind a read-through that takes minutes and can't
  be interrupted. That, plus a second deep queue on the node, is the direct cause of the 163 s
  listing.
- **The biggest warning is against an elaborate priority scheme.** HTTP/2's priority tree was
  deprecated as "complex … not uniformly implemented … not successful" (RFC 9113 §5.3.1), and was
  replaced by eight urgency levels and one flag (RFC 9218). Google SRE kept four criticality classes
  and refused more, and Ceph hid mClock's knobs behind three profiles. The scheduler needs a few
  absolute classes plus a reservation per resource, not a richer number.
- **A prediction must never inherit the class of a person waiting.** Netflix's playback service
  guarantees requests a person made 100% and gives prefetch what is left. When prediction requests
  spiked 12×, prefetch availability fell to 20% while requests from people stayed above 99.4%. The
  2026-10-21 failure was the reverse: a background read-through was raised to "play".
- **The sketch misses the human side of waiting.** It doesn't cover:
  - variance (p95, not just a target);
  - what fills the wait;
  - who gets told, and in what words;
  - the devices that matter, since TVs and phones have no hover;
  - stale-but-labelled as a third state between fresh and wrong.

  It also explains a slow start far too late: at 10 s, when abandonment begins at 2 s.
- **The sketch has no load model and no cache design.** A deadline without an arrival rate, a cost
  distribution and a capacity can't be checked. Derived data also has no budget, eviction policy,
  identity rule or staleness bound.

## 1. Where the sketch agrees with prior art

| Sketch | Prior art that agrees |
| --- | --- |
| §7.1 A deadline belongs to a want | OpenLayers' tile queue: priority comes from the current frame, and a tile no longer wanted gets `DROP` ([TileQueue.js](https://github.com/openlayers/openlayers/blob/main/src/ol/TileQueue.js)). Unreal's per-cycle texture plan ([Epic](https://dev.epicgames.com/documentation/en-us/unreal-engine/texture-streaming-overview-for-unreal-engine)). Zero's query-defined sync ([Zero](https://zero.rocicorp.dev/docs/introduction)). Netflix's split between requests a person made and prefetch ([2024](https://netflixtechblog.com/enhancing-netflix-reliability-with-service-level-prioritized-load-shedding-e735e6ce8f7d)). |
| §7.2 A want has a holder and lifetime | Leases (Gray and Cheriton 1989, via [Colyer](https://blog.acolyer.org/2014/10/31/leases-an-efficient-fault-tolerant-mechanism-for-distributed-file-cache-consistency/)). Zero's query TTL, 5 min by default and 10 at most ([Zero](https://zero.rocicorp.dev/docs/reading-data)). TanStack Query's `gcTime` ([TanStack](https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults)). Notion's subscriptions to what is rendered ([Notion](https://www.notion.com/blog/data-model-behind-notion)). |
| §7.3 Continuous deadlines need a reserved share | mClock reservations ([Ceph](https://docs.ceph.com/en/latest/rados/configuration/mclock-config-ref/)). cgroup io.latency ([kernel](https://docs.kernel.org/admin-guide/cgroup-v2.html)). The partitions of Netflix's concurrency-limits library ([GitHub](https://github.com/Netflix/concurrency-limits)). Buttazzo on EDF's domino effect and resource reservation ([InTech](https://cdn.intechopen.com/pdfs/35320/InTech-Handling_overload_conditions_in_real_time_systems.pdf)). Plex, Jellyfin and Emby's "stay N s ahead, then sleep" transcoder throttles. |
| §7.4 Costs are a vector | Google SRE: "queries per second" fails as a capacity model, so measure the resources ([Handling Overload](https://sre.google/sre-book/handling-overload/)). Meta's IOCost models device occupancy per I/O ([ASPLOS 2022](https://www.cs.cmu.edu/~dskarlat/publications/iocost_asplos22.pdf)). |
| §7.5 Prediction makes slow data timely | Figma preloads frames within N transitions ([Figma](https://www.figma.com/blog/speeding-up-file-load-times-one-page-at-a-time/)). instant.page: after 65 ms of hover, the odds of a click are one in two ([instant.page](https://instant.page/)). Netflix Open Connect fills caches in off-peak windows from predicted viewing (from search excerpts). |
| §7.6 Partial results count | PDF.js range-fetches the visible pages ([FAQ](https://github.com/mozilla/pdf.js/wiki/Frequently-Asked-Questions)). Mux encodes only the segments that are watched ([Mux](https://www.mux.com/blog/how-to-transcode-video-100x-faster-or-a-gordian-knot-cut)). RFC 9218's `incremental` flag. Meta: a cheap H.264 encode first, the costly encodes later ([Meta](https://engineering.fb.com/2021/04/05/video-engineering/how-facebook-encodes-your-videos/)). |
| §7.7 Freshness is separate from deadline | Each TanStack query has its own `staleTime`. RFC 5861 stale-while-revalidate. Google SRE: criticality "is orthogonal to its latency requirements". |
| §7.8 Late needs a defined fallback | Zero's `unknown`/`cached`/`complete`: show "not found" only when complete. Zuul's refusals carry `retryAfterSeconds` ([2020](https://netflixtechblog.com/keeping-netflix-reliable-using-prioritized-load-shedding-6cc827b02f94)). Tail at Scale's "good-enough" results. |
| §7.9 Dead resources must not hold live ones | Hystrix bulkheads, one pool per dependency ([wiki](https://github.com/Netflix/Hystrix/wiki/How-it-Works)). Google SRE on bimodal latency: if 5% of requests hang to a 100 s deadline, the frontend fails 80% ([Cascading Failures](https://sre.google/sre-book/addressing-cascading-failures/)). |
| §7.10 Due is not low priority | Netflix Timestone dequeues by earliest deadline first (from search excerpts). Google SRE: deadline and sheddability are separate axes. |
| §8 Q4 Who yields | Krishnan and Sitaraman (IMC 2012): abandonment starts at 2 s of startup, +5.8% a second, and a failure cuts the chance of return ([PDF](https://people.cs.umass.edu/~ramesh/Site/HOME_files/imc208-krishnan.pdf)). Conviva (SIGCOMM 2011): buffering ratio has "the largest impact" on engagement ([PDF](https://conferences.sigcomm.org/sigcomm/2011/papers/sigcomm/p362.pdf)). YouSlow (INFOCOM 2016): one rebuffer causes 3× the abandonment of one bitrate change ([PDF](http://www.cs.columbia.edu/~hn2203/papers/11_infocom2016.pdf)). **So a running playback is protected over a new start, and a lower quality is preferred to a stall.** Twitch guarantees transcodes to partners and gives others the spare capacity ([Twitch](https://blog.twitch.tv/en/2015/07/08/boom-more-transcode-servers-427bcbfb519/)). |

## 2. What the sketch gets wrong or misses, ranked

### 2.1 Long work must be preemptible, and the queues below the scheduler shallow

- **The problem:** a full read-through takes minutes and starts again from the beginning if
  stopped. Once a job is on the node's work list, the core's policy can't reach it. That combination
  is the 163 s listing.
- **The prior art:**
  - *The Tail at Scale* ([PDF](https://www.barroso.org/publications/TheTailAtScale.pdf), checked):
    Google's file servers "keep few operations outstanding in the operating system's disk queue,
    instead maintaining their own priority queues", and slice long requests so short ones can
    interleave.
  - BFQ: a budget timeout on each turn ([kernel](https://docs.kernel.org/block/bfq-iosched.html)).
  - Ceph: a deep "operation sequencer" below mClock defeats it.
  - Cloudflare: buffered bytes go out in buffer order whatever their priority
    ([blog](https://blog.cloudflare.com/http-2-prioritization-with-nginx/)).
- **What it means for Orpheum:**
  - Every job longer than about a second becomes bounded, idempotent slices with a saved cursor. A
    read-through becomes reads of tens or hundreds of MB at an offset, with loudness state that
    merges across slices. The scheduler decides again at each slice boundary.
  - The node holds only a few operations per disk in flight.
  - One place decides. The sketch's §4 gains a column: the longest uninterruptible unit of each kind.

### 2.2 A prediction is its own class and never inherits a waiting person's

- **The prior art:**
  - Netflix's PlayAPI: a prefetch failure "does not result in a playback failure", so prefetch gets
    only the spare capacity.
  - Chrome caps hover-triggered speculations at two, first in first out
    ([Chrome](https://developer.chrome.com/docs/web-platform/prerender-pages)).
  - Mars Pathfinder: priority may pass only along a real "X is blocked on Y" edge
    ([Jones/Wilner](https://www.cs.cornell.edu/courses/cs614/1999sp/papers/pathfinder.html)).
- **What it means for Orpheum:** D324's interest raise sends a playing film's read-through to 1000.
  That gives priority to work nobody is blocked on, and it is the inversion that caused 2026-10-21.
  A prediction that comes true creates a new want for exactly the data needed; it doesn't promote
  the background job. Predicted work gets a small budget of its own, evicts its oldest entry first,
  and is cancelled first.

### 2.3 A few absolute classes, plus reservations per resource, instead of a richer number

- **The prior art:**
  - HTTP/2's tree failed: RFC 9113 §5.3.1 calls it complex, unevenly implemented and "not
    successful" ([RFC 9113](https://www.rfc-editor.org/rfc/rfc9113#section-5.3.1)). RFC 9218 gives
    eight urgencies and an `incremental` flag, which are "only a suggestion" the server combines
    with its own knowledge ([RFC 9218](https://www.rfc-editor.org/rfc/rfc9218)).
  - Google SRE: four criticalities, each new one refused because it would "require more resources
    to operate".
  - Ceph: three mClock profiles, because raw reservations, weights and limits were too hard for
    users.
  - fetchpriority: three values.
  - TUF research uses step or linear functions "because they are easier to specify and schedule"
    ([Wikipedia](https://en.wikipedia.org/wiki/Time-utility_function)).
- **What it means for Orpheum:** the sketch's six deadline classes stay as *descriptions* of
  scenarios. The scheduler acts on perhaps three tiers: continuous or interactive, attended, and
  background. On each resource, each tier has a reservation, a weight and a limit. EDF (earliest
  deadline first) runs only *within* a reservation, never as one queue across everything. Under
  overload EDF's misses cascade (Buttazzo), and the standard cure is service adaptation, meaning a
  lower quality (D253).

### 2.4 Disk cost is disk time, learned online, not a reader count

- **The problem:**
  - Interleaving two sequential streams on a spinning disk costs seeks, so two readers don't each
    get half.
  - A file already in the NFS page cache costs almost nothing.
  - The NAS has load Orpheum can't see, such as a RAID scrub or other clients.
- **The prior art:**
  - IOCost shares measured device occupancy by weight.
  - Netflix's concurrency-limits library learns the safe concurrency from latency, the way TCP
    Vegas does ("we can use latency measurements to determine when queuing happens").
  - io.latency throttles the peers of a protected group when it misses its target.
- **What it means for Orpheum:**
  - Each disk gets a limit learned from read latency.
  - Playback is protected by feedback: when its read latency or buffer slips, background
    concurrency on that disk is cut, with no model of the NAS needed.
  - A dead share's latency drives its limit to the floor on its own, a second signal beside D327's
    probe.
  - The Linux controllers act at the block layer and, as far as the research found, don't govern
    NFS client I/O. So the node applies the same ideas itself, per disk.

### 2.5 Background reads need a rate limit, not just a pause

Jellyfin issue [#16608](https://github.com/jellyfin/jellyfin/issues/16608) shows a remux that finished
the whole film in 2–7 s, at 9–57× real time, before the throttle's 5-second check ever ran. Pausing
FFmpeg when it is far enough ahead doesn't protect the disk. A remux, read-through or extraction gets
a read rate per disk (FFmpeg's `-readrate`, or the node's own reads), so that the disk keeps
headroom for the stream someone is watching (P5, P9).

### 2.6 Playback needs admission control, not priority

The sketch says a playback needs a reserved share (§7.3), but nothing decides what happens when the
reservations don't fit (W11, §8 Q4).
- A playback is admitted only if its reservation fits: encode rate, disk MB/s and uplink bitrate.
  Otherwise it is offered a lower rung that does fit.
- Demand is *elastic* through the bitrate ladder, and so is poster size, so stepping quality down is
  the first way to shed load, ahead of queueing (Unreal drops "one mip at a time" to fit its pool).
  Elasticity is a dimension the sketch lacks.
- The QoE research decides the policy:
  - never stall or degrade someone already watching to admit a newcomer;
  - the newcomer gets the lower quality, with a reason that doesn't name the other person's film.

### 2.7 Deadlines, cancellation and the want's identity travel the whole path

- **The prior art:**
  - Google SRE: deadlines propagate as one absolute time, and each stage checks it before starting.
    "You don't get credit for late assignments." Cancellation propagates too.
  - Criticality is set "as close as possible to the browsers".
- **What it means for Orpheum:**
  - Each want carries an absolute deadline, a class and a lease, from the client to the core to the
    node.
  - Work shared by several wants holds a reference count of leases. Its effective deadline is the
    earliest, its class the highest. When the last lease ends, the work is cancelled at its next
    slice boundary.
  - D324's 45 s interest lapse is far too coarse for a poster that scrolled past within a second.
    That cancellation starts in the client, by aborting the request.
  - For abandonable interactive wants, serve the newest first. That is Facebook's adaptive LIFO, via
    [Colyer](https://blog.acolyer.org/2015/11/19/fail-at-scale-controlling-queue-delay/): the poster
    just scrolled to beats the one scrolled past. Never apply it to Due or Patient work, which nobody
    abandons.

### 2.8 Coming back from a failure is the next failure

- **The problem:** when a share returns, every parked job unparks at once. The queue inventory, §2.1.3:
  an unpark "wakes every runner of every kind and asks for every sweep". Retries and probes pile on
  top. A disk that has just recovered then takes its heaviest load. This is a metastable failure:
  the bad state persists after its trigger is gone, sustained by the system's own retries (Bronson
  et al., [HotOS 2021](https://sigops.org/s/conferences/hotos/2021/papers/hotos21-s11-bronson.pdf)).
- **What it means for Orpheum:**
  - Treat a dead place as an open circuit breaker: it admits nothing new.
  - On recovery, ramp admission back up with a token bucket, playback first.
  - Give retries a budget, coalesce duplicate wants, and jitter wakeups.
  - Memcache's leases cut peak database queries from 17K/s to 1.3K/s under a thundering herd
    (Colyer, above).
  - Tie stall windows to the want's deadline: a job nobody needs within 3 min shouldn't hold a
    reader for 3 min on a dead disk.
  - Hystrix also warns: "there's no way to force the latent thread to stop work". A reader blocked
    in the kernel is counted as gone until it returns. It isn't assumed freed by cancelling.

### 2.9 Order Eventually work by value over cost, and give it a time to run

- **The prior art:** Meta orders encodes by "Benefit/Cost", where benefit is compression gain times
  predicted watch time, after a cheap encode covers everything. Under load, Meta's SVE pulls only
  from its high-priority queue (via
  [Colyer](https://blog.acolyer.org/2017/11/27/sve-distributed-video-processing-at-facebook-scale/)).
  Open Connect fills in windows of low load.
- **What it means for Orpheum:**
  - After the cheap examine of every file, read-throughs are ordered by predicted watching (D324
    interest, Next Up, recently added) divided by cost (the film's size over the disk's measured
    throughput).
  - They run preferably when no one is watching, and on a disk that is already awake (§2.11).
  - For fairness inside the background tier, deficit round robin keeps every class making progress,
    so 3,412 read-throughs aren't starved *(unchecked: Shreedhar and Varghese 1995)*.

### 2.10 The sketch has no cache design

The derived data (keyframe indexes, read-throughs, subtitles, prepared copies, poster sizes) is a
cache. It needs rules the sketch doesn't give:

- **Identity:** what each artifact is keyed on (content hash or inode, size and mtime). This decides
  whether a move on the NAS throws away a week of read-throughs.
- **Budgets and eviction:** the prepared directory (W9) needs a size budget and eviction that weighs
  value. GreedyDual-Size weighs the cost to recompute against size and recency *(unchecked: Cao and
  Irani 1997)*.
- **Negative results:** cache them, such as a TMDB "no match", with a TTL. Plex's preview task
  retried the same ~20 failing files every night and spun the disks up each time
  ([forum](https://forums.plex.tv/t/plex-not-respecting-scheduled-task-settings-keeps-spinning-up-disks-help/871848/1)).
- **Staleness:** a staleness bound for each kind of data. The catalog suits stale-while-revalidate.
- **Availability:** this is a lease, not a fact. A probe is stale when it returns, so "true when
  shown" (§7.7) is impossible as written. Availability is shown with its age, becomes "unknown" when
  the lease expires, and Play stays live and fails fast with the truth.

### 2.11 Hidden resources are missing from §4

- **The household's uplink**, behind DS-Lite and relayed through discovery, probably the tightest
  resource for a remote viewer.
- **SQLite's single writer.** Jellyfin
  [#14366](https://github.com/jellyfin/jellyfin/issues/14366): a full scan made playback stutter on
  every device, with "database is locked".
- **The core's memory**, and a weak client's: Notion found its local SQLite cache made p95 *slower*
  on slow devices, and raced it against the network
  ([Notion](https://www.notion.com/blog/how-we-sped-up-notion-in-the-browser-with-wasm-sqlite)).
- **A provider's daily quota**, as well as its rate.
- **A disk's spin-up**, as a switching cost: batching maintenance onto a disk that is already awake
  saves latency and noise.

### 2.12 The playback's own bytes can do the read-through's work

During direct play the node already streams every byte of the film. Loudness, timeline and embedded
subtitles can be computed from that stream at a cost of CPU only, instead of a second read of the
same file competing with it. This is the "cooperative scan" of databases *(unchecked: Zukowski et
al., VLDB 2007)*. It removes most of P9's conflict, and it is the cheapest idea in this review.

### 2.13 Deadlines need percentiles, and the explanation comes far earlier

- **The problem:** abandonment starts at 2 s (Krishnan and Sitaraman, checked). P1's "past about
  10 s, say why" therefore explains things only to the viewers who stayed.
- **The research:**
  - Users calibrate to the typical case and punish outliers.
  - A stall during play hurts more than a longer start (YouSlow, checked; *unchecked: Hoßfeld et
    al., QoMEX 2012*).
  - Google's INP judges interactions near the worst case, at p75
    ([web.dev](https://web.dev/articles/inp)).
- **What it means for Orpheum:**
  - Each "By" gets a p50 and a p95.
  - The sketch's "Attention" splits into **Start** (≤2 s, explain at 3 s) and **Progress** (2–10 s,
    with a determinate indicator).
  - The design may buy p95 with startup: a longer initial buffer for a remote play, or a play from a
    busy disk, to protect the continuous deadline.

### 2.14 "Wrong if stale" needs a third state

- **The problem:** taken literally, "Wrong" blocks or blanks the screen.
- **The UX reviewer's third state:** **stale but labelled, then reconciled in place.** Cached
  content draws at once; boxes have fixed sizes; updates change values, never the order of what is
  under focus. On a TV, a row reordering under the D-pad's focus reads as a broken remote.
- **Prior art:** Google's CLS metric (layout shift, ≤0.1) exists for the same reason.
- **A clear rule for when to be optimistic:**
  - Be **optimistic about a person's own actions:** Scan now, fix match, mark watched, remove.
    Acknowledge at once, reconcile quietly.
  - Be **honest about anything that decides whether to press Play:** availability, the quality on
    offer, whether the subtitle will be there.
  - Play stays live. Its press answers at once with the truth.
- **Disks need three states, not two:** ready, waking (about 10 s), not answering. A sleeping disk
  must never be shown as unavailable (W7).

### 2.15 Telling people: who, in what words, and the admin first

- **Two audiences:** every playback problem answers *your side, the household's server, or this
  file?*
  - A user's copy never says "disk" or "share". The UX reviewer's example: *"Karl's library is
    having trouble reaching this film. Your connection is fine. We've let Karl know."*
  - The admin gets the detail: the place, when it last answered, what waits behind it.
  - Plex's "server not powerful enough to convert video" is the example to avoid: true, and useless
    to a person.
- **The admin is told, not left to discover it.** In a household the partner texts "it's broken"
  first. A share that stops answering, a node offline, or a certificate or backup at risk sends the
  admin a notification, with "a user hit this at 21:14 on *Film X*".
- **The wait is filled:**
  - The film's backdrop and title during startup, not black with a spinner.
  - No spinner under about 300 ms, and once one is shown, keep it at least about 500 ms.
  - A determinate count with an ETA for long jobs.
  - A known, finite wait ("Waking the drive…") feels shorter than an unexplained one *(unchecked:
    Maister 1985; Myers, CHI 1985; Harrison et al., UIST 2007)*.
- **After recovery:** resume a few seconds before the freeze, and offer "Keep waiting / Watch
  something else" while waiting.

### 2.16 Each scenario needs a load line, and the hard combinations need scenarios

A deadline can only be checked against a load. Each scenario gains:

- an arrival rate, a burst size and a concurrency;
- a cost distribution at p50 and p99;
- a **value when late**:
  - **hard:** a stall is seen;
  - **firm:** a poster scrolled past, or a seek superseded, is worth nothing late: drop it;
  - **soft:** loudness is worth a little less each day.

Some rows can't be met as written, and should say so. T1's "click an unscanned film, Flow" needs an
examine, a TMDB fetch and a keyframe index, perhaps on a disk that is spinning up. It is met only by
a degraded page, not in time.

The failures live in combinations, which need scenarios of their own:
- T1 with P5 with W8.
- W1 at 21:00 with two playbacks and the partner remote.
- The evening peak, when the scheduled scan lands just as everyone starts watching.

## 3. Proposed corrections to the deadlines

These come from the UX review (§2.13), with the research's corrections. LAN and remote are separated
where they differ. **They are proposals for the owner.**

| Moment | Sketch | Proposed |
| --- | --- | --- |
| C1 open at home | Flow (1 s) | Cached shell ≤0.3 s; fresh data ≤1 s p95; updates in place |
| C2/T4 remote, first open | Attention (10 s) | Cached shell at once; first useful screen ≤3 s p50, ≤5 s p95 |
| App returns from background (missing) | n/a | At once from cache; reconnects silently |
| B1 poster into view | Flow | Ready *before* it is visible: prefetch 1–2 screens ahead. TV focus moves within Instant |
| B2 availability | Flow; blocks Play if stale | Shown ≤1 s with its age; Play stays live; the press answers at once |
| P1 direct play, LAN | 2 s | ≤1 s p50, ≤2 s p95; explain at 3 s |
| P1 direct play, remote | 2 s | ≤2 s p50, ≤4 s p95; may trade startup for fewer stalls |
| P1 from Continue Watching | as P1 | ≤1 s: predicted when the home page loads |
| P2 encode start | 2–5 s | ≤3 s p95; past 3 s, a determinate reason |
| P3 scrub | 1–2 s | Thumbnails Instant while dragging (trick-play images: **a kind of data the sketch lacks**); landing ≤1 s LAN, ≤2 s remote; ±10 s skips Instant from the buffer behind the playhead |
| P4 subtitles | Flow–Attention | Instant for a profile that always uses subtitles (for a deaf viewer they are the film); ≤2 s otherwise |
| P7 next episode | Instant–Flow | ≤1 s, behind an up-next countdown |
| W1 told why | Attention | ≤3 s, and the admin notified |
| W7 disk waking | unstated | "Waking the drive…" by 1.5 s, with a determinate bar to ~10 s |
| A1 Scan now | Instant ack | Instant ack, and an ETA within 10 s |
| A3 status page | Flow, true when shown | Last known, with its age, at once; live within 2 s |

## 4. Scenarios to add

**From the UX review:**
- The first 60 seconds after install. Identify and show posters for the first ~30 films *in grid
  order*, not folder order, so the screen looks alive.
- A TV with a remote: focus speed, prefetching rows, scrubbing with a D-pad, no shift under focus.
- A phone on cellular: a data cap makes prediction cost money. Respect metered connections and start
  at a lower rung.
- Read-your-own-writes: stop a film, press Back, and Continue Watching shows the new position at
  once.
- Kids:
  - errors in pictures, not words;
  - repeated presses must not queue duplicate work;
  - parental limits.
- A guest's first and only open, through discovery.
- A relative who isn't technical, on a TV. All copy is written for them by default.
- The admin as a viewer: no admin noise during their own playback.
- The evening peak.
- Audio track and language preference at start.
- A "new episode" notification, which makes the catalog's staleness visible.
- Casting from phone to TV mid-film.
- The admin alerted (§2.15).

**From the data review:**
- Bulk invalidation: a new loudness algorithm re-derives 3,000 films, about a week of disk time.
- Files moved or renamed on the NAS, and a disk replaced or remounted under a new device.
- A long pause: does the playback's reservation, or its lease, hold for 30 minutes?
- Quiet hours and spin-down policy.
- A node shared with other software: a RAID scrub, or Plex beside it.
- The node crashing mid-job: outputs written to a temporary file and renamed, idempotent
  redelivery.
- The core restarting with all interest lost: clients say their interest again at reconnect, not
  on the next 15 s tick.

## 5. Lessons, ranked

1. **Slice long work and keep the lower queues shallow; one place decides.** (Tail at Scale, BFQ,
   Ceph, Cloudflare, RAIL.) This is the direct cause and fix of the 163 s listing.
2. **A prediction is its own class, with its own budget, and never inherits a waiting person's.**
   (Netflix PlayAPI, Chrome speculation, Pathfinder.)
3. **A few absolute tiers, each with a reservation, weight and limit on each resource; EDF only
   inside a reservation.** (RFC 9113/9218, Google SRE, Ceph, Buttazzo.)
4. **One bulkhead per disk and per provider, with a limit learned from latency, and background
   reads rate-limited.** (Hystrix, concurrency-limits, IOCost, io.latency, Jellyfin #16608.)
5. **Admit playback against its reservation, and use the bitrate ladder as the first way to shed
   load; protect a running playback over a new start.** (QoE research, Twitch, Unreal's mip
   budget.)
6. **Wants are leases carrying an absolute deadline and a class from the client to the node;
   cancel when the last lease ends.** (Google SRE, Zero, leases.)
7. **Fail fast on a dead place, and come back slowly.** (Metastable failures, bimodal latency,
   memcache leases.)
8. **Eventually work is ordered by value over cost, after a cheap pass, and runs when it is quiet
   on an awake disk.** (Meta's encode priority, SVE, Open Connect.)
9. **Derived data is a cache with identity, budgets, eviction and negative results;
   availability is a lease.** (RFC 5861, Zero's result types, the Plex spin-up thread.)
10. **The human side is part of the design:**
    - percentiles;
    - an explanation at 3 s;
    - stale-but-labelled, reconciled in place;
    - optimistic about a person's own actions, honest about Play;
    - two audiences for every failure;
    - the admin notified.

**Verify the design before building it.** Run a discrete-event simulation of the scheduler on the
fake clock (`clocktest`), replaying the scenario combinations and checking the deadline-miss ratio
of each tier. Run the new scheduler in shadow beside the old one before it decides anything, as
Figma did with its load sets. Judge it by the deadline-miss ratio and lateness of each tier on each
resource, which is also what the status page (A3) should show.
