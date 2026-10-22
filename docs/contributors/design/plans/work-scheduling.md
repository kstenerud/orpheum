> **ABOUTME:** The design that answers [data-timeliness.md](data-timeliness.md): how Orpheum decides
> what work runs, when and where, from what people want. Wants and work, the resources and who
> decides each, admission for playback, the tiers, long work cut into pieces, every media read
> through the node's own scheduler, quiet hours and sleeping disks, restart, observability, how
> it is checked, and the order to build it. It replaces numeric job priorities, the play-driven
> flag, the core's window and bump, and the quick pass.

# Work scheduling

- **Status:** PLANNED — decided as
  [D329](../../decisions/working-notes.md#d329--work-is-scheduled-from-wants-each-resource-is-decided-next-to-it-playback-is-admitted-and-long-work-is-cut-into-pieces)
  on 2026-10-22, and §8.1 (FFmpeg reading through the node's byte path) on its test the same day.
  The owner: *"1: Yes. 2: Yes, test this and then we decide based on the results. 3: I'm fine with
  whichever ordering is more efficient. I don't need the features in a hurry."* It answers the
  scenarios in [data-timeliness.md](data-timeliness.md), which are the requirements. Their research
  is in [the review](../research/data-timeliness-review-2026-10-22.md),
  [the follow-ups](../research/timeliness-followups-2026-10-22.md) and
  [the audit](../research/data-timeliness-audit-2026-10-22.md). The code it changes is mapped in
  [the queue inventory](../research/job-queues-inventory-2026-10-21.md).
- **Depends on:** [data-timeliness.md](data-timeliness.md). It supersedes
  [interest-priority.md](interest-priority.md)'s priority numbers. That plan's interest register is
  kept as one source of wants (§3).

## 1. The design in one page

**Wants, not priorities.**
- A **want** is someone's need for some data by a deadline (data-timeliness §3.4). It is either:
  - **ephemeral**: a lease held by a session, such as a page shown, a poster in view, a playback
    or a prediction;
  - **durable**: kept by the server until done, such as "Scan now", a file to show, or a backup.
- The core gives each want its **tier** from what it is. A client never states a tier.

**Work is keyed and derived.**
- **Work** is a unit of producing data, keyed by its kind, the identity of its content and its
  parameters. Wants for the same data meet on the same work.
- Work's tier is the highest of its live wants. Its deadline is the earliest of their deadlines.
- Work with no live want is cancelled at its next piece boundary.
- Background work has a standing durable want from the library's state: "this film is owed its
  loudness". So D231's derivation by sweeps stays as it is.

**Each resource has one decider, next to it.**

| Resource | Decider |
| --- | --- |
| A disk's reads | The node's **disk scheduler** for that device |
| A node's encoders | The node's **encoder scheduler** |
| The household's uplink | The core, where every playback byte already passes (`api/pacing.go`) |
| A provider's rate and quota | The core's provider service |
| The database writer | The store's writer gate |

**Playback is admitted, not prioritised.**
- A playback asks every resource it needs for a share: disk read rate, encoder, uplink.
- It is admitted all or nothing, in a fixed order. Once admitted, its share sits outside the
  priority order and is protected by feedback.
- Nobody's arrival lowers a running playback (P5).

**Everything else is ordered by tier, inside each decider.**

| Order | Tier | What | How it is ordered |
| --- | --- | --- | --- |
| — | **Continuous** | Admitted playbacks | Outside the order; they hold their shares |
| 1 | **Interactive** | Someone is looking at the screen for it: a page, a poster, a seek, the first frame | Earliest deadline first; a firm want that can be abandoned goes newest first |
| 2 | **Attended** | A person's durable request: Scan now, a fixed match, a new file to show | Earliest deadline first |
| 3 | **Predicted** | Warming what is likely to be wanted | Only from spare capacity, capped per session, never wakes a disk |
| 4 | **Background** | Due and Eventually | A Due want past its latest start rises to Attended. Eventually work is shared between kinds by deficit round robin with a minimum share, and within a kind ordered by value over cost |

**Long work is made of short pieces.** Every piece of work holds a disk or an encoder for a bounded
time, a few seconds at most. Its progress is saved between pieces, and the decider chooses again
after each. Nothing more urgent ever waits behind a whole film.

**Every media read goes through the node's disk scheduler.** FFmpeg reads media from the node's own
HTTP byte path on the loopback, not from the filesystem. So:
- every read is paced, ordered and cut into pieces by the scheduler;
- a dead share blocks a thread of the node, which it can count and route around, and no FFmpeg
  process is stuck in the kernel;
- one read of a film can feed two needs: the playback, and the read-through computed from the same
  bytes (data-timeliness §4.4).

**What waits says why.** Every work item knows its decider and what it waits behind. Every decider
measures the deadline misses and lateness of each tier. The status page shows both.

## 2. What it keeps, and what it replaces

**Kept:**
- **D231.** Work is derived from durable state by sweeps. The job queue is a regenerable cache. A
  destination that is down parks its work. A failure that is the item's own is a fact. Durable
  wants are these facts and requests.
- **D233, D234 and D238.** The link between core and node: commands, acknowledged results,
  reconciliation at hello, the watchdog, the node holding its work in memory.
- **D309.** A player says what it wants: one quality from a position. This becomes the playback's
  ephemeral want for chunks, and a run no player wants still stops.
- **D324's interest register.** Pages and playbacks said and lapsing. It becomes a source of wants.
  D324 §4 (work in the way is stopped) and §5 (playing is not starved) are met by tiers, pieces and
  admission, not by raising numbers.
- **D325–D328.** Pages pull; places, probes and the availability table stay as they are. The
  breaker of §8.7 is D328's place, marked unavailable.
- **The store's writer gate** (`store/writer_gate.go`): a person's write first. It gains tiers
  (§11).
- **The provider service's token buckets.** They gain tiers and a quota reserve (§11).

**Replaced:**
- **Numeric priorities.** All of them: `PriorityPlayNow`, `PriorityLookedAt`,
  `PriorityPersonsRequest` and the rest. `IsPlayDriven` goes too.
- **The core's window, bump and quick pass.** D240 §5 and §6, and D255 §2 and §3. The node's disk
  and encoder schedulers decide instead (§17 Q1).
- **Raising and demoting by interest.** Wants are added and end instead.
- **The node's work list's numeric order, and SIGSTOP pausing.** The node's schedulers order by tier
  and stop work at piece boundaries.
- **The hourly read-through sweep's order.** Read-throughs are ordered by value over cost (§7.4).

## 3. Wants

### 3.1 The kinds of want

| Want | Declared by | Kind | Tier | Lives until | Cap |
| --- | --- | --- | --- | --- | --- |
| A playback (its bytes or chunks ahead of the playhead) | The playback session (`playback/tokens.go`) and D309's interest | ephemeral | Continuous, once admitted | The session ends or lapses (D309 §3) | One per player |
| A playback's start, or a seek | The player | ephemeral | Interactive, with Start's budget | The first frame | One per player |
| A page shown (film, series) | `PUT /interest` (D324) | ephemeral | Interactive for what the page shows; Predicted for what it may play next | The page is left or lapses | One page per session |
| A poster or an image in view | The HTTP request for it | ephemeral, the request itself | Interactive, newest first | The request ends or is aborted | The browser's own connection limit |
| An availability ask (`waitSeconds`) | The request | ephemeral | Interactive | The request ends | One per film per session |
| Focus resting on a tile, or a phone's page open | A prediction declared by the client | ephemeral | Predicted | Focus leaves, or 60 s | **Two per session**, oldest dropped first |
| Continue Watching's first item, the next episode in its credits | The core, from the home page or the playback | ephemeral | Predicted | The page is left, or the next episode starts | Counted in the session's two |
| A library opened: is anything new? (B5) | Opening the library | durable while it runs | Attended | Its check is done | One per root per **minimum interval** |
| Scan now, fix a match, look again, a subtitle asked for | The person's action, kept as a fact (D231 §4) | durable | Attended | Done, or cancelled by its owner or by removal | Per person (§3.3) |
| A new file to know and show | The listing that found it | durable | Attended, at a limited rate | Shown as a bare entry | — |
| A library's examine, identification, artwork | The library's state | durable, standing | Background | Done | — |
| Read-through, trick-play, subtitle extraction | The library's state | durable, standing | Background, by value over cost | Done | — |
| Tonight's backup; a certificate before expiry | The schedule | durable | Due: Background until its latest start, then Attended | Done | — |

### 3.2 Where a tier comes from

**A client declares facts, and the core turns them into wants.** The facts are:
- "this page is shown";
- "this film is playing at 0:41:10 at this quality";
- "focus rests on this tile".

**A want's tier comes from a table in the core,** the one above, never from the request.
- Only a playback the core has admitted is Continuous.
- Only a start, a seek, or a page's or image's own data is Interactive.
- A guest's wants take the same tiers, but count against smaller caps.

### 3.3 Caps and fairness between holders

- **Each session has caps:** live predictions (two), live pages (one), live availability asks (one
  per film).
- **Each person's durable Attended wants are capped** (a setting, default 20). Past the cap a request
  is accepted and queued behind the person's own earlier ones, not behind everyone's.
- **Inside a tier, holders take turns.** One person's 300 scrolled posters don't delay another
  person's page. Interactive work is round-robin by session first, and earliest deadline first
  within the session. This is the same rule RFC 9218 §13 gives a server that merges many clients
  onto one connection.

### 3.4 Deadlines travel as budgets

- **A want's deadline is a remaining budget**, in milliseconds. Each hop (client, core, node)
  converts it to its own monotonic clock on arrival. A TV whose clock is an hour wrong can't make
  work urgent or stale.
- **Background and Due wants carry a wall-clock deadline instead**, kept by the core in the
  household's time zone (data-timeliness A8). The core alone turns it into a latest start and a
  remaining budget.

### 3.5 Identity

- **Each want carries a key.** For a client's want it is the session and what it is about. For a
  durable one, the fact's row. Five presses of Play are one want (W15).
- **Each piece of work has a key:** `kind / content / parameters`. Examples:
  - `read_through / payload-hash / -`;
  - `chunks / store-id / quality,from-position`;
  - `artwork / image-sha / size`;
  - `list / place / depth`.

  Two wants for the same key meet on one piece of work, and a request for a key already running
  joins it. This is D231 §8's merge, keyed on content rather than on a job row.

## 4. Work

### 4.1 Its record

The core keeps each live piece of work in memory, mirrored to cache.db as today for whatever goes to
a node:

| Field | Holds |
| --- | --- |
| key | `kind / content / parameters` (§3.5) |
| wants | The live wants that hold it, by key |
| tier | The highest tier of its wants |
| budget | The smallest remaining budget among its wants, or its latest start |
| value | For Background: predicted watching (§7.4) |
| cost | Estimated device time and encoder time, from size and measured speeds (`core/waits`) |
| place | The place it reads (D328) and the device that place is on |
| progress | The saved state of its pieces (§6) |
| waiting on | The decider and what it waits behind, with since when (§13) |

### 4.2 Its life

1. **Derived or requested.** A sweep derives Background work from state (D231 §3). A want creates
   or joins work by key.
2. **Routed.** Work the core does itself goes to the core's deciders: provider fetches, database
   writes, the uplink. Node work goes to its node: reads, encodes, artwork cuts.
3. **Sent to the node with its attributes**, not an order: its key, tier, budget, value, cost,
   place and saved progress. Each node is sent:
   - all its live Interactive and Attended work;
   - its Predicted work;
   - **for each device, the Background work worth the next hour of that device's time**, ordered
     by value over cost.

   That is enough for the node's schedulers to choose well, and small enough that the core can
   still change it.
4. **Updated** when its wants change: its tier rises or falls, or its budget shrinks. **Cancelled**
   when its last want ends. A cancel takes effect at the next piece boundary, and the progress made
   so far is kept (§6).
5. **Answered** piece by piece. Each piece's result updates the work's progress. The last piece's
   result is the work's result, acknowledged as today (D234 §5).

### 4.3 The node holds no queue of its own making

The node's schedulers hold what the core sent, and nothing else. A node restart loses it. The core
sends it again at hello (D234 §6), with the progress the core has recorded, so work resumes from its
last answered piece.

## 5. Resources and their deciders

### 5.1 Who decides what

| Resource | Decider, next to it | Measured by | Notes |
| --- | --- | --- | --- |
| A disk's reads: concurrency and throughput | The node's **disk scheduler**, one per device | Read latency, bytes per second, stuck reads (diskwatch) | Several places on one device share it (D240 §5) |
| A node's encoders | The node's **encoder scheduler** | Each run's speed against real time, and its lead (§9) | A hardware encoder is a bonus slot (W11) |
| The household's uplink | The core's **uplink admission** (`playback/upload.go`, `pacing.go`) | Bytes sent per playback, which is counted today; the players' reported link speeds; the uplink's measured capacity (§10.2) | Every playback byte passes through the core, local and remote |
| A provider's rate and daily quota | The core's **provider service** | Its token buckets, Retry-After, quota answers | Tiers added (§11.2) |
| The database writer, per database | The store's **writer gate** | Each write's wait and hold | Tiers added (§11.1) |
| The core's CPU | — | — | Not scheduled: the core does no media work (D193 §1) |

### 5.2 How the deciders coordinate

**Most work needs one resource.** A provider fetch needs a provider. A read-through needs a disk,
plus some CPU that is not scheduled. An artwork cut needs the node's artwork slot, and a disk only
for an image beside the media.

**A playback needs three, together:** a disk's read rate, an encoder if it is transcoded, and the
uplink if its viewer is remote.

- **The order is fixed: uplink, then node (disk and encoder in one step), then commit.** The core
  checks the uplink in memory. It then asks the node to admit the disk share and the encoder share
  together, which the node decides at once in one step. Only then is the uplink share committed.
- **A refusal anywhere releases what was held**, and the playback goes to its floor (§10.3). No
  hold is ever left half made.
- **The node and the core run on one machine** (the byte socket is a Unix socket), so the ask costs
  microseconds. A node elsewhere later would cost one round trip at a start, which Start's budget
  allows.

**Work that crosses deciders in sequence holds one at a time.** Artwork, for example, is a provider
fetch in the core, then a cut on the node. Each step is its own piece, with the same work key.

## 6. Pieces

Every kind of work is cut into pieces, each holding its decider's resource for at most about
**2 s of device time**. That is `piece_seconds`, a setting. Between pieces the work's progress is
saved, and its decider chooses again. A piece can't be interrupted, so the longest anything waits
behind other work on a resource is one piece.

| Kind | A piece is | Saved between pieces | Resumes |
| --- | --- | --- | --- |
| Listing a place (a scan) | One directory's entries, or a page of them | The directories still to list | From the next directory |
| Examining (FFprobe and hashes) | One file | The files done | From the next file |
| Keyframe index | The whole index read, a fraction of a second | — | — |
| Read-through (loudness, timeline) | A byte range, about `piece_seconds` × the disk's throughput (about 200 MB on a NAS) | **A coverage map** of ranges read; R128 block loudness as a **histogram**, which merges across pieces and gates correctly; the timeline's keyframes found so far | From any uncovered range, including one filled by a playback's read (§8.5) |
| Embedded subtitle extraction | A byte range; FFmpeg reads through the node (§8.1) and waits between pieces | The cues so far, and the byte offset reached | From the offset |
| Trick-play images | A time range of the film | The images made | From the next range |
| Chunks (D309) | One chunk | Chunks in the store | From the next chunk not made |
| Prepare (a whole copy) | A segment | The segments written, under a temporary name until the copy is whole (W13) | From the next segment |
| Artwork cut | One image | The sizes made | From the next image |
| Provider fetch | One request | — | — |
| A database write | One transaction, bounded (§11.1) | — | — |

**FFmpeg is kept running across pieces where it can wait.** A read-through or extraction FFmpeg
reads its input from the node's byte path (§8.1). Between pieces the node just doesn't answer its
next read, so FFmpeg waits on a socket, holding no disk and using no CPU. Where waiting would hold a
scarce slot for long, the run is stopped and restarted from its saved state instead:
- the waiting reader of a cancelled want;
- a run whose resume costs little, such as a chunk, segment or image.

## 7. The order inside a decider

### 7.1 Admission comes first

A decider first serves its admitted Continuous shares: playbacks' reads, encodes, uplink. These are
not in the order. They hold what they were admitted for, and are protected by feedback (§9, §10).
What is left is ordered as below.

### 7.2 The tiers

1. **Interactive.** Round-robin by session, then earliest budget first. Firm wants that can be
   abandoned (posters, a superseded view's fetch, a seek's old position) go newest first, and are
   dropped when their want ends.
2. **Attended.** Earliest budget first. A Due want past its latest start joins this tier.
3. **Predicted.** Spare capacity only: it runs when no Interactive or Attended work waits on that
   resource. Capped per session, and never wakes a disk.
4. **Background.**
   - **Between kinds:** deficit round robin across examine, identification, artwork, read-through,
     trick-play and subtitle extraction, with a **minimum share** so that Due work and each kind
     always progress. Fairness here keeps 3,412 read-throughs from starving the trick-play images,
     and the other way round.
   - **Within a kind:** by value over cost (§7.4).
   - It obeys quiet hours, idleness and the disk's sleep (§12).

### 7.3 Preemption is at piece boundaries only

A higher tier takes the resource at the next piece boundary of what holds it. The lower work's
progress is kept, and it resumes later from where it was. No work is thrown away to make room, so
D255 §2's "throws out its work" goes. Work blocked on a dead place is not holding anything (§8.4).

### 7.4 Value over cost, for Eventually work

- **Value:** how likely the film is to be watched soon, from D324's shown interest, Next Up,
  recently added, partly watched, and a small base for the rest.
- **Cost:** the device time the work needs, which is the film's size over the disk's measured
  throughput, less what is already covered.
- **The core ranks Background work by value ÷ cost** when it sends a node its next hour (§4.2).
  Meta orders its encodes the same way: benefit over cost, after a cheap pass covering everything.

### 7.5 Overload and shedding

- Inside a tier, the caps of §3.3 bound any one holder.
- **When a provider's daily quota runs low, a share of it is held back for Attended wants:**
  `provider_quota_attended_reserve_percent`, 20% by default. Background calls stop first.
- **When a disk or an encoder can't keep its admitted shares** (capacity shrank: a RAID scrub, the
  household's own upload), Background yields first, then Predicted, then Attended. Only then is an
  admitted playback lowered: **the most recently started first** (the owner: *"seniority wins if
  someone has to suffer"*).

## 8. The node's disk scheduler

### 8.1 Every media read goes through it

**Today,** FFmpeg opens media files directly. Its reads bypass every limit, block in the kernel on a
dead share, and outlive SIGKILL under virtiofs (the NFS research).

**In this design, FFmpeg reads media from the node's own HTTP byte path,** on a loopback address the
node serves for its own FFmpeg runs. It uses FFmpeg's `http` protocol with `Range`, `seekable` and
reconnect options. The node's own reads (listing, examining, hashing) call the scheduler directly.
So:
- **Every read is a request the scheduler orders, paces and attributes** to a want and a tier.
- **A stuck read on a dead share blocks a node thread in a read the diskwatch gate already watches**
  (D233 §10, D328). FFmpeg sees a slow HTTP answer and times out. It is never stuck in the kernel,
  and the node can kill it.
- **Pieces need no SIGSTOP:** the scheduler holds back the next answer.
- **A read can serve two consumers** (§8.5).

**As decided for the closed beta** (D329 §7, after three tests in
[ffmpeg-http-input-2026-10-22.md](../research/ffmpeg-http-input-2026-10-22.md)):
- **A run that reads straight through a container that streams** (a read-through, an extraction, a
  whole prepare, of Matroska, MPEG-TS, or an MP4 with its index first) reads **a pipe the node feeds**
  through this scheduler. Its reads are pieces, paced, and shareable, and a dead share stops the
  node's feeding, never FFmpeg in the kernel.
- **A run that seeks** (a chunk run, a probe, any run of an MP4 with its index at the end) reads the
  file itself, as today. The scheduler counts it as a reader it cannot see, holding one of the
  device's places while it runs, and orders everything else around it.
- **FFmpeg opens no socket** (N1 stands).
- A run is judged by whether its source was read to the end and by the demuxer's I/O error, never by
  FFmpeg's exit status alone.

**After the beta,** a media program of our own, in Rust, reads by offset through the node with no
socket, so the scheduler sees every read. The HTTP path was tested and works, but it needs a socket.

### 8.2 Shallow below, ordered above

- **Each device keeps only a few reads in flight:** one or two on a spinning disk, a few on an SSD
  or a NAS. That is the device's concurrency, a setting per device as today (D240 §5, D244 §2).
- **Everything else waits in the scheduler, where it can still be reordered.** Its order is §7.
- **Reads are of bounded size:** a read-through's piece is many reads of a few MB. A playback's read
  is never queued behind more than one other read.

### 8.3 Protecting playback

- **A playback's reads go first,** within its admitted rate.
- **The control signal is the playback's read latency on that device.** This is io.latency's model:
  a target for the protected group.
  - When its reads take longer than `playback_read_latency_target_ms` (a setting, from the device's
    measured normal), Background's rate on that device is halved.
  - When latency recovers, it rises again slowly. That is additive increase and multiplicative
    decrease, so the cut is quick and the recovery gradual.
- **For a transcoded playback, the encoder's lead over the playhead is the second signal** (§9).
- **Read latency is a signal for throttling, not for concurrency.** The node doesn't learn a
  concurrency limit from latency: at one to three readers over NFS such limiters oscillate (the
  audit).

### 8.4 A dead place is an open breaker

- **D328's unavailable place is the breaker.** Nothing new is admitted to it.
- **Work waiting on it holds nothing.** It sits in the scheduler under "waiting on the place" (§13).
- **A read stuck in the kernel counts as capacity lost** until it returns, not as a reader in use.
- **On recovery, admission ramps up:** playback first, then the tiers in order, with a token bucket
  that refills over `recovery_ramp_seconds` (30 by default). Retries have a budget, and wakeups are
  jittered. This prevents the herd the queue inventory found, where "an unpark wakes every runner of
  every kind".

### 8.5 One read, two consumers

- **Every range read for a playback is offered to that film's read-through**, if it is owed one.
  The read-through feeds the bytes to its consumers: R128 block loudness, keyframes for the
  timeline, and subtitle packets for any track owed an extraction. It records the range as covered.
- **The consumer is opportunistic.** It never slows the playback's read, and when it falls behind
  it drops the range, which stays uncovered. Nothing depends on it.
- **A film watched start to end has its read-through done** without a read of its own. A film never
  watched gets its own read, piece by piece, as Background work. A film half watched fills only its
  gaps.

### 8.6 A disk's sleep

- **The scheduler knows when it last read the device.** After `disk_presumed_asleep_after_minutes`
  (20 by default) without a read, it treats the device as **possibly asleep**. Over a share it can't
  know: the NAS decides (data-timeliness A8).
- **A possibly-asleep device is read only for:** Continuous, Interactive or Attended work; Due work
  past its latest start; or a batch of Background work that has grown past
  `wake_batch_device_minutes` (30 minutes of device time by default) or has waited past its deadline.
- **When it is read, everything owed on it runs together** while it spins, in tier order.
- **Every read of a possibly-asleep device is logged as a wake**, with the want that caused it, and
  shown to the admin (A8).

## 9. The node's encoder scheduler

- **Slots:**
  - One guaranteed software encode slot per node (W11).
  - A second one is best effort. It runs at a lower CPU weight (`cpu.weight` where cgroups allow,
    `nice` otherwise), is never counted against the first's lead, and is stepped down or stopped
    first when that lead shrinks.
  - A hardware encoder found at start adds its own slots, as many as it is measured to sustain.
- **An admitted transcode holds its slot** (Continuous). D309 still decides which runs a player
  wants. A run no player wants stops.
- **Lead is the control.** Each run's lead is how far its newest chunk is ahead of its player's
  position. When the guaranteed run's lead falls below `encode_lead_floor_seconds` (20 by default):
  1. best-effort encodes yield;
  2. then Background CPU work on the node (trick-play, artwork cuts) yields;
  3. then the run itself steps down a rung for its own player (D253, P5's own-client step down).
- **The artwork slot** cuts images, one image a piece, at Background or Interactive tier.

## 10. Playback admission and the uplink

### 10.1 What a playback asks for

| Path | Disk | Encoder | Uplink (remote only) |
| --- | --- | --- | --- |
| Direct play | The file's peak rate | — | The file's peak rate |
| Remux | The file's peak rate | A light slot (a remux needs little CPU) | The output's peak rate |
| Transcode | The source's read rate at real time | A software or hardware slot | The rung's peak rate |

**Peak** is a token-bucket rate and burst over the client's buffer, not the average bitrate. The
burst is the largest amount the stream sends in a buffer's length. Plex budgets the same way, after
peaks of 3–4× the average broke streams budgeted at their average. For a direct play it comes from
the file's own bitrate profile, measured by the read-through and estimated from the container until
then.

### 10.2 The uplink's capacity is measured, not configured

- `server_upload_mbps` stays, as a ceiling an admin may set. When it is unset, as it is today by
  default, the core estimates the capacity:
  - the most bytes per second it has sent to remote players when they reported no stall;
  - the remote players' reported link speeds;
  - an optional speed test at setup.

  It keeps 20% headroom, as Plex does.
- `KeepUnderShare` and crowding (`playback/upload.go`, `crowding.go`) stay. They become the uplink
  decider's feedback.

### 10.3 Admission

1. **The order of choices** (W11): direct play, then a remux, then an audio-only transcode, then a
   video transcode. A remote direct play rarely fits a household uplink (data-timeliness §4.3), so a
   remote start usually lands on a transcode.
2. **Admit at the best choice that fits** on every resource, in the order of §5.2.
3. **If nothing fits above the floor** (`playback_floor_kbps`, by screen class: phone, laptop, TV),
   the newcomer is offered **"Play now" at the best that fits** (focused by default) or **"Wait"**,
   with an estimate of when there will be room.
   - The estimate comes from when the admitted playbacks are expected to end, without naming them.
   - A child's profile skips the choice and plays now.
   - The choice is remembered on the device for the evening.
4. **No running playback is lowered to admit a newcomer** (the owner, 2026-10-22).
5. **A running direct play steps down for its own client**, when the client can't keep up and the
   encoder has room. It becomes a transcode at a rung that fits. With no room, its viewer gets the
   same choice as a newcomer below the floor.
6. **When a playback ends,** a lowered stream rises once, one step. A waiting newcomer is told, and
   starts with one press.
7. **A long pause** (P10) releases the playback's shares after `paused_share_kept_minutes` (10 by
   default). Its resume is admitted like a start, warmed by D309's minute kept ahead.

### 10.4 4K HDR on an SDR screen without hardware

Admission sees that the tone-mapping transcode can't run in real time. It then:
1. checks that the device can decode the file (HEVC Main10);
2. if it can, admits a direct play behind the one-time warning (W11);
3. if it can't, says so plainly.

## 11. The core's own deciders

### 11.1 The writer gate gains tiers

The gate already lets a person's write go first, and lets Background be passed over at most four
times. In this design:
- **Writes carry the tier of the work that makes them.** Play-state reports (B7, P8) are
  Interactive. An examine batch's ingest is Background, or Attended for a person's scan.
- **A write transaction is bounded:** `write_transaction_max_ms`, 200 by default. Long writes are
  cut into batches that re-check what they decided (D303's short writes). The identification
  pass's 25–30 s holds, found in the database audit, are what this rules out.
- **Leases and interest are never written to the database.** They live in memory (§3.1).
- **A checkpoint starved by a long reader is logged**, with the reader.

### 11.2 The provider service gains tiers

- Its buckets are shared by tier. Interactive and Attended calls come first, then Background, which
  is held to `tmdb_background_rate_share_percent` (kept).
- A daily quota keeps its Attended reserve (§7.5).
- Parking and probing stay as D231 has them.

## 12. Quiet hours, idleness and wakes

The rules are data-timeliness A8's. The core holds the policy and sends it to each node's disk
scheduler:
- the quiet hours, in the household's IANA time zone, with the start and end configurable;
- whether they are lifted ("sound-isolated");
- whether the household is **idle**, meaning no playback anywhere.

The node applies the policy to every piece of work:

| The work's tier | In quiet hours | When not idle | A possibly-asleep disk |
| --- | --- | --- | --- |
| Continuous, Interactive, Attended | Runs | Runs | Wakes it |
| Predicted | Doesn't run on media disks | Runs from spare | Never wakes it |
| Due past its latest start | Runs | Runs | Wakes it |
| Background | Doesn't run on media disks | Doesn't run | Only in a batch (§8.6) |

Quiet hours govern media disks only. The core's own disk work (its backup, its database) is not
noise. A "knowing" listing for B5 is Attended, so it runs, at a limited rate.

## 13. Observability: what waits, and why

**Every work item shows its waiting state** on the status page (A3), and on its film's page where a
person waits for it:
- where it is: its decider;
- what it waits behind: a tier ahead, an admitted playback, quiet hours, a sleeping disk, a dead
  place, a provider's limit;
- since when, and its estimate (D324 §6).

**Every decider keeps these, per tier, in memory:**
- the deadline-miss ratio and the lateness at p50 and p95, over a window of 24 h and 7 days;
- utilisation;
- the waiting count.

The status page shows them.

**Every wake of a disk is logged**, with the want that caused it (§8.6).

**Every rule per event that is broken is logged at warn** (data-timeliness §3.1: no wait
unexplained past 3 s), with the work and its waiting state. Each is a defect to fix, not a statistic.

## 14. Restart

| What | On a core restart | On a node restart | On a client reconnect |
| --- | --- | --- | --- |
| Ephemeral wants | Lost; clients declare them again **on reconnecting**, not at their next tick (C3) | Kept in the core; the node's work is sent again | Declared again at once |
| Durable wants | Kept as facts (D231 §4) | Kept | — |
| Work and its progress | Derived again by the startup sweep. Progress is kept where it is a durable fact: a read-through's coverage map and histogram, and a prepare's segments, kept beside its output | Sent again at hello with the core's recorded progress, and resumed from the last answered piece | — |
| Admitted playbacks | Lost with their sessions, which reopen (`reopen.ts`) and are admitted again | Their reads and encodes stop. Players see a stall, the node returns, and they are admitted again | Re-admitted on the reopened session |
| The node's scheduler state | — | Rebuilt from what the core sends | — |
| Measured speeds, the uplink estimate, quiet-hours policy | Speeds and estimate in memory, rebuilt within minutes; policy from settings | Policy sent at hello | — |

## 15. The client's side

- **Images are wants.**
  - Posters in view are fetched with `fetchpriority` set by tier: high for in view, low for
    prefetched screens.
  - Requests for posters scrolled past are **aborted** (`AbortController`). Plain lazy `<img>` can't
    be aborted, so the grid loads posters itself.
  - The grid prefetches one or two screens ahead (B1).
- **The grid is windowed**, and a jump goes straight to a letter.
  - The browse API gains a **letter index**: for the view as sorted and filtered, the letters present
    in the viewer's locale and the keyset cursor at each (data-timeliness B1).
  - Jumping is a fetch from that cursor. The keyset pagination (`library/browse.go`) needs no
    offsets.
- **Fetches are cancelled when their view is superseded:** a filter changed, or a page left.
- **Predictions are declared:** focus resting on a tile, a phone's page open. At most two are live
  per session.
- **Wants are declared again on reconnecting.** The interest hook and the player resend at once
  when the connection returns.
- **Metered connections** follow data-timeliness C6.
- **Accessibility and copy** follow data-timeliness §5.5.

## 16. Checking it, and the order to build it

### 16.1 Checking

- **A simulation before code.**
  - A discrete-event model of the deciders on `clocktest` replays the scenarios and the combinations
    X1–X3.
  - It includes the failures that started this: reads stuck on a dead share, the 48 s NFS recovery,
    a 23 s spin-up, and a RAID scrub.
  - It checks each tier's miss ratio against data-timeliness's targets, and its rules per event.
  - It stays as a test.
- **Each phase lands behind the gate as usual:** tests that fail when reverted (D190), arch tests for
  the load-bearing claims (rule 10), and VQ items for what only a device shows.
- **Faults injected on staging:**
  - a share dropped with netem or iptables;
  - a slow disk with dm-delay;
  - a full prepared directory;
  - a provider refusing.
- **Traces of staging's real days**, replayed through the simulation.
- **Then the owner's household as the canary.**

### 16.2 Phases

Each phase leaves the system working.

| Phase | What | Replaces |
| --- | --- | --- |
| 0 | The simulation, with today's scheduler modelled too, as the baseline it must beat. **Begun 2026-10-22:** `internal/schedtest`, its disk (seeks, sleep, death and NFS recovery), a direct-play client, and today's node work list, which reproduces the 2026-10-21 listing (`TestToday_AListingWaitsBehindAPlayRaisedReadThroughOnceTheShareAnswers`: 1 min 51 s after the share answered). The encoder, the uplink and the scenarios X1–X3 follow as the phases that need them land | — |
| 1 | **Begun 2026-10-22:** `internal/node/diskschedule`, the decider, run through the bench's 2026-10-21 scenario: the listing ends 1.0 s after the share answers, against 1 min 51 s of waiting today, the read-through finishing no later (`TestDecided_AListingReadsAsSoonAsTheShareAnswers`). Then **the node's read path:** FFmpeg reads media through the node's loopback byte path; the disk scheduler with tiers, pieces of reads, and the latency feedback; stuck reads counted apart | SIGSTOP pausing; FFmpeg's own file reads |
| 2 | **Wants and work keys in the core:** the tier table; ephemeral and durable wants; attributes sent to the node instead of priorities; cancel at the next piece | Numeric priorities, `IsPlayDriven`, interest raising, the window, bump and quick pass |
| 3 | **Pieces for long work:** read-through with its coverage map and histogram, subtitle extraction, trick-play, prepare by segments | Whole-film runs |
| 4 | **Admission:** encoder and disk shares on the node; uplink admission in the core, with the measured estimate; floor and choice; own-client step down | Today's ceilings as the only control |
| 5 | **One read, two consumers** | The read-through's second read of watched films |
| 6 | **Quiet hours, idleness and wakes;** value over cost; deficit round robin with minimum shares | The hourly sweep's order |
| 7 | **The core's deciders:** writer-gate tiers and bounded writes; provider tiers and the quota reserve | — |
| 8 | **Observability:** waiting states, miss ratios, the wake log, warnings for broken per-event rules | — |
| 9 | **The client:** abortable posters with priorities, the windowed grid and the letter index, predictions, re-declaring on reconnect | — |

The user-facing pieces of the scenarios ride on these phases rather than in them: issues and badges
with the daily email (A6), bare entries and unreadable files (B5), the HDR warning (W11), the
wait-or-play choice (P5), captions (P4), and accessibility (§5.5).

## 17. The owner's answers, 2026-10-22

1. **The node decides its disks and encoders** (§5): yes. D240 §5 and §6 are revised by D329.
2. **FFmpeg reads media only through the node's byte path** (§8.1): tested, then decided: *"Yes, go
   ahead. If we hit a blocker during our move to this setup, we have the rust binary option."*
3. **The scope.** The order is the agent's to choose, for efficiency. The user-facing pieces (§16)
   are built as a batch after phase 4, when admission and the waiting states exist for them to
   show, and not interleaved.

Data-timeliness §11's four questions were answered the same day: no immediate channel, no "This
has been reported", seniority wins when capacity shrinks, and stopping a stream by hand only if a
user asks.
