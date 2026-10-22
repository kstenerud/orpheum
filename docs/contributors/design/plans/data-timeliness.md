> **ABOUTME:** User scenarios read for timeliness: in each, what a person expects to see, by when,
> what data that takes, what producing it costs and on which resource, and what is shown if it is
> late. The input to a redesign of how Orpheum fetches, computes, queues and caches its data, which
> the scenarios are meant to decide before any mechanism is chosen.

# Data timeliness

- **Status:** SKETCH. Written 2026-10-22 at the owner's request, as the first step of the queue and
  priority redesign. The owner said: *"The question ultimately comes down to the timeliness of data:
  Is the data there when the user expects it to be? … before we go into the redesign (and I don't
  care even if we have to rip all the pieces out and greenfield them), let's start with making user
  scenarios."* Every deadline here is the agent's proposal, for the owner to confirm or change. The
  design that comes from them is not written yet.
- **Depends on:** — (it reads [scenarios.md](scenarios.md), which asks whether the model is right,
  and asks something else: whether the data is there in time.)

## 1. Why

The data has deadlines: a person is waiting for it, or soon will be. Several people can want
different things at once. Producing the data costs CPU, GPU, memory, network, and a disk's seeks,
throughput and concurrency, and it can take a provider's quota too. One number per job collapses all
of that into a single dimension, and it failed in practice. The
[queue inventory](../research/job-queues-inventory-2026-10-21.md) found three places comparing
priority by three rules. A listing waited 163 s behind a read-through that nobody was waiting for,
because a playing film had raised that read-through to priority 1000.

The scenarios come first, so that the design answers to them, not to the mechanisms that exist.

## 2. How to read a scenario

Each scenario is a short story, then a table with one row per moment where the person expects
something:

| Column | Holds |
| --- | --- |
| **Moment** | What the person does or looks at. |
| **Expects** | What they should see. |
| **By** | The deadline, as a class from §3, counted from the moment. |
| **Data** | What has to exist to show it, and where it comes from (§4). |
| **If late** | What is shown in the meantime. "Wrong" means showing stale data would mislead. |

## 3. Deadline classes

These are the classic response-time limits (Nielsen, *Usability Engineering*, 1993), plus two
classes for work no one sits and watches:

| Class | Within | What it means to the person |
| --- | --- | --- |
| **Instant** | 0.1 s | It reacts like a physical object: a button press, a scrub, a scroll. |
| **Flow** | 1 s | The person notices the wait but does not lose their train of thought: a page, a poster. |
| **Attention** | 10 s | The person keeps watching the screen and needs to see progress. Starting a video sits at the short end: viewers begin to give up after 2 s, and each further second loses about 6% more of them (Krishnan and Sitaraman, IMC 2012). |
| **Patient** | minutes | The person does something else and comes back to check. |
| **Due** | a time, hours to days ahead | No one waits now, but it must be done by then: a certificate before it expires, tonight's backup. |
| **Eventually** | no time | It improves things when done, and nothing is due: loudness for a film nobody has opened. |

**Continuous** marks a deadline that keeps renewing. During playback, the next chunk is due before
the buffer runs out, again and again for two hours. Missing it once is a stall the person sees.

## 4. Where the data comes from

What it costs to produce each kind of data, from cheapest to dearest. A "reader" is one of a disk's
concurrent readers.

| Source | Examples | Cost, and on which resource |
| --- | --- | --- |
| The client's memory or cache | The page shown a moment ago, posters already loaded | Nothing; may be stale |
| The core's memory | Interest, live playbacks, a node's last status | Microseconds |
| The core's database | Catalog, metadata, play state, availability | Milliseconds (performance-targets.md: under 50 ms a page) |
| A node, without touching media | Status, sessions, capacity | One round trip |
| A probe of a place on a node (D327) | Can this disk be read now? | One `statfs`: milliseconds, or as long as a dead share leaves it waiting, outside every queue |
| A node, reading a little of a file | A keyframe index; the bytes for a direct play | A seek and a little throughput, per reader |
| A node, listing a directory tree | A scan's listing | Many metadata operations: seconds, more over a share |
| A node, running FFprobe on each file | A scan's examine | One open and one short read per file; a batch takes seconds |
| A node, reading a whole file | The full read-through (loudness, timeline); an embedded subtitle's extraction | The film's size over the disk's throughput: minutes, holding a reader |
| A node, encoding | Prepare, chunks | CPU or GPU for as long as the film plays or longer; a reader too |
| An outside provider | TMDB metadata, artwork originals, OpenSubtitles | Network, a rate limit, a daily quota; the provider can be down |

## 5. The people

- **The owner**: the admin, at home on the LAN with the core and the node, and the NAS on a share.
- **The partner**: a user, remote, reached through discovery, on a home connection.
- **A beta household**: one admin, two to four users, one node, one or two disks, and sometimes two
  people playing at once.

## 6. Scenarios

### Setup

#### T1. The first library, 3,000 films on a NAS

The owner installs Orpheum, adds a library on the NAS share and watches what happens.

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Picks the folder | Told it is readable, or why it is not | Flow | A probe of the place | A spinner, then "not answering" |
| The library is created | The first films appear | Attention | The listing streams in; the examine of the first batch | "Looking through the folder: N found" |
| Watches the grid | Titles and posters fill in | Patient | Identification (TMDB, rate-limited), artwork originals, then sizes | Filename and a placeholder poster |
| Clicks a film still unscanned | Its page, then it plays | Flow, then Attention | That one film's examine, identification and keyframe index, ahead of the 2,999 others | The page from the filename, saying what is still being read |
| Comes back later | All films identified and playable | Patient (performance-targets.md: under 1 h per 3,500 items) | All of the above, for everything | A progress count |
| Days later | Loudness measured, subtitles found | Eventually | Read-throughs, subtitle searches | Shown as not measured |

**What it says:** the same work (examine, identify, artwork) is "Patient" in bulk, but "Flow" for the
film the person clicked. The deadline belongs to whoever is asking, not to the kind of job.

#### T2. A second library added while the first still scans

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Adds a folder on a USB disk | Its first films appear, without waiting for the NAS scan to finish | Attention | A listing on another disk | As T1 |

**What it says:** different disks are separate resources. Work on one should never wait for a reader
on the other, and the TMDB rate limit is shared by both.

#### T3. A second node added

The owner adds a second machine that holds its own disks.

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Enrols it | It shows as connected, with its disks | Flow | The hello and the status | "Connecting" |
| Adds a library on it | As T1 | | | |

#### T4. Inviting the partner

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Makes the invite | A link or code | Flow | The core's database | n/a |
| Partner claims it and opens the app | The home page | Attention (first time, through discovery) | Discovery, the session, the home page | "Connecting to <household>" |

### Connecting

#### C1. Opening the app at home

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Opens the app | The home page: Continue Watching, recently added | Flow | The database; posters from the client's cache or the core | The last page seen, from the client's cache |
| Looks at Continue Watching | It is right, including what they watched on another device last night | Flow | Play state | Wrong: stale progress misleads |

#### C2. The partner opens it remotely

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Opens the app | The home page | Attention: the connection is raced over several paths first | Discovery, the paths, the session | The last page seen, marked as connecting |
| Scrolls | Posters | Flow, on a slower link | Smaller sizes of the artwork | Placeholders |

**What it says:** the network between the client and the core is a resource too, and a slow one has
to be planned for. On a slow link, sending less is how you get data there sooner.

#### C3. Opening it after the core restarted or was updated

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Opens the app | As C1 | Flow | Interest, live playbacks and nodes' statuses are gone until said again | The database answers; anything about "now" is unknown, not stale |

**What it says:** the core's memory starts empty. Every answer about "now" must either be refilled
from the source within seconds, or say plainly that it is not known yet.

#### C4. A TV or a cast device joins

It is the same as C1, on a device with less memory. It gets the first screen and nothing it does not
need.

### Browsing

#### B1. Scrolling the grid of 3,000 films

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Scrolls | The grid moves | Instant | Already in the client | n/a |
| Posters come into view | Each poster shows | Flow | The artwork size for this screen: cached, or made on the spot | A placeholder, then the poster |
| Posters far below | Nothing yet | n/a | No work should start for posters nobody has scrolled to | |

**What it says:** a want can start and end within a second. A poster that scrolled past should stop
being made, or at least stop being urgent.

#### B2. Opening a film's page

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Opens it | Title, art, synopsis, cast | Flow | The database | n/a |
| Reads it | Whether it can play now | Flow | Availability of its place | Wrong if stale: "Play" on a dead disk misleads; check without blocking the page |
| Reads it | Tracks, subtitles, length, loudness | Flow, but may be unknown | The examine, done; the read-through, maybe not | "Not measured yet" |
| Hovers over Play | Nothing visible | n/a | A strong sign of what will be played: warm its keyframe index and the first chunks | |

**What it says:** looking at a film predicts playing it. Work that costs too much to do on demand
can be done on time if it starts on the look, which D324 began.

#### B3. Searching

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Types | Results as they type | Flow (performance-targets.md: under 200 ms) | The database | The last results |

#### B4. A series

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Opens the series | Seasons and episodes, Next Up | Flow | The database | n/a |
| Will play the next episode | | | Warm the next three episodes (interest-priority.md) | |

#### B5. A film copied to the NAS while the app is open

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Copies it, then looks for it | It appears | Patient: a minute is fine | The watcher, or the scheduled scan; one examine; identification | Not shown yet |

**What it says:** the catalog can be minutes stale. Availability can not.

#### B6. Two people browse while a big scan runs

Both expect B1 and B2 as normal. The scan's examines and the TMDB fetches must not delay a page or a
poster that someone is looking at.

### Playing

#### P1. Play a film that plays directly

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Presses Play | The first frame | Attention, short end: 2 s | Availability; the keyframe index; the first bytes | A spinner; past about 10 s, say why ("the disk is busy", "waking the disk") |
| Watches | No stalls | Continuous | Bytes ahead of the playhead, from the disk's reader, the node and the network | A stall, said when it happens |

#### P2. Play a film that needs a remux or an encode

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Presses Play | The first frame | Attention: 2–5 s | The first chunks, encoded on demand | A spinner with progress |
| Watches | No stalls | Continuous: the encoder must keep ahead of the playhead | CPU or GPU, and a reader | A stall; a lower quality offered (D253) |

#### P3. Seeking

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Drags to 1:10:00 | The picture there | Flow to Attention: 1–2 s | The chunk at the new place; work for the old place stops | The last frame, with a spinner |

**What it says:** a continuous deadline can jump. Work queued for the old position is now worth
nothing and holds a reader for no reason.

#### P4. Turning on an embedded subtitle

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Picks a track | Subtitles from the current line | Flow to Attention | The track's text near the playhead; extracting it reads the whole file, which takes minutes | "Subtitles loading"; text appears from where it is extracted |

**What it says:** this data costs minutes but is wanted in seconds. It must be predicted (on the look,
or when the track is the person's usual language) or produced from the playhead onwards.

#### P5. Two people play two films from the same NAS disk

Both have P1's or P2's continuous deadlines on one disk's throughput and readers. Both must hold.
Maintenance on that disk waits. Read-throughs and scans give way to them; they do not compete with
them as equals.

#### P6. Watch Together, one person remote

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Both press Play | Both start together | Attention | The same chunks, at two qualities perhaps; the room's state | Wait for the slower one, and say so |
| One pauses | The other pauses | Instant to Flow | The room's state, pushed between peers | Drift, corrected |

**What it says:** the room's state is the umi no tobira of data that two people need fresh to within a
second. It travels over WebRTC between the players, not through the job machinery.

#### P7. Next episode

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Credits roll | The next episode offered | Instant | Next Up, from the database | n/a |
| It starts | The first frame | Instant to Flow, because this one was predictable | Warmed during the credits, or earlier | As P1 |

#### P8. Resuming on another device

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Stops on the laptop, opens the TV | Resume from where it stopped | Flow | Play state, written by the last report | Wrong if stale: resumes 10 s back at most |

#### P9. Background work on the film being watched

The film's read-through and subtitle extraction help later, but only the bytes for the playhead are
needed now. Nobody blocks on them, so they must never take the playback's reader or bandwidth. This
is the 2026-10-21 failure seen from the playback's side.

### Administering

#### A1. "Scan now"

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Presses it | It is acknowledged | Instant | n/a | n/a |
| Watches | Progress | Flow, for each update | The scan's counts | The last count |
| Waits | Done | Patient | Listing, examines | |

**What it says:** an admin's request is "Patient", but it should still go before scheduled
maintenance. It should never go before a playback.

#### A2. Fixing a wrong match

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Picks the right film | The page shows it, with its poster | Attention | One TMDB fetch, one artwork original and its size | The old poster, then the new one |

#### A3. The status page: nodes, disks, work

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Opens it | What is happening now: each disk answering or not, what runs, what waits and why | Flow, and true when shown | Pulled when looked at (D325): probes, the node's work, the core's queue | Wrong if stale: it is where the admin goes when something is off |

**What it says:** a waiting job must be able to say what it is waiting for. Neither queue can say
that today (the queue inventory, §5).

#### A4. A backup and a certificate

Nobody watches these. A backup is due tonight, and a certificate is due before it expires. If they
slip from Due to Eventually, the backup goes missing or the certificate lapses. They need a deadline,
not a low priority.

#### A5. Removing a library or a node

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| Removes it | It is gone from every page | Flow | The database | n/a |
| | Its queued work is gone too | Flow | The queues | Work for a gone thing holds a reader |

### Things going wrong

#### W1. The share drops while a film plays (the 2026-10-21 tests)

| Moment | Expects | By | Data | If late |
| --- | --- | --- | --- | --- |
| The picture freezes | To be told it is the disk, not their network | Attention | A probe of the place | A spinner with no explanation, which is what we had |
| The share returns | Playback resumes | Attention, counted from when the share answers again | A probe every 2 s (D327) | Today about 48 s of the host's NFS recovery, which no Orpheum code can shorten |
| Meanwhile | Browsing the rest of the library is unaffected | Flow | Nothing waits on the dead disk | |

#### W2. Browsing while the share is down

The film's page says at once that its disk is not answering (B2's availability). The page itself
never waits on that disk, and nothing on the page waits for a stall to time out to find out.

#### W3. The node is off

This is W1 and W2 at the scope of a node. Every film on it says so at once, and its queued work
neither runs, nor holds anything, nor is lost.

#### W4. The core restarts during a playback

The player keeps playing from its buffer. It reconnects and resumes within Attention. Interest and
playbacks are said again (C3).

#### W5. The partner's link degrades

ABR drops the quality. If that quality is not prepared yet, it becomes P2's deadline at a lower
quality. A degrading link turns a "would be nice" rendition into a "due now" one.

#### W6. TMDB is down or rate-limited

New films show by filename. A person's "fix the match" waits and says why. The provider's limit is a
resource, and a person's request should be spent from it before the sweep's.

#### W7. A disk asleep or slow (USB 2, spun down)

The first access after idle takes 5–10 s to spin up. Whatever touches the disk first wakes it and
waits. A press of Play should not queue behind a scan that woke it. The person should be told the
disk is waking up, not shown a stall.

#### W8. 200 films copied in during a playback

The playback (P1 or P2) holds. The new films appear within Patient. The scan takes what the playback
leaves and never more.

#### W9. A full disk

When the node's prepared directory is full, prepares fail. A playback that needs one should be told,
and should be offered direct play or a lower quality if the film has them. When the core's disk is
full, writes fail (scenarios.md S77).

#### W10. A corrupt file that hangs FFmpeg

The one file fails within its window. Its disk's other work goes on. A hang must not hold the disk's
reader for every other job.

#### W11. A node at its limit: two encodes and a read-through

The two playbacks' encodes come first. The read-through waits or pauses. A third player is told that
this film needs more than the node has free, and given whatever it can have.

## 7. What the scenarios say so far

These are notes for the design, not the design.

1. **A deadline belongs to a want, not to a kind of work.** The same examine, keyframe index,
   artwork size or subtitle extraction is "Eventually" for the library and "Flow" or "Attention" for
   the film someone has opened. Its class also changes from moment to moment: when a person looks,
   when they leave, when they seek.
2. **A want has a holder and a lifetime.** It is a person on a page, a player at a playhead, the
   admin who pressed a button, or a date in the future. It ends when they leave. Work whose wants
   have all ended is worth less at once, and should give back what it holds.
3. **Some deadlines are continuous.** A playback is a stream of deadlines, each a few seconds out,
   for hours. It needs a share of a disk's throughput and of the CPU reserved, not a single turn in a
   queue.
4. **The costs are a vector, not a number.** The resources are a disk's readers and throughput (per
   disk), a node's CPU and GPU, its memory, the network to the client, and a provider's rate limit
   and quota. Two jobs only compete when they want the same resource. A scan on one disk and a
   playback on another do not compete at all.
5. **Prediction turns expensive data into timely data.** Looking predicts playing, the credits
   predict the next episode, a usual language predicts a subtitle track, and a slow link predicts a
   lower quality. What costs minutes can be ready in seconds if it starts on the prediction.
6. **Partial results count.** A keyframe index before the full read-through, the first chunks before
   the rest, and subtitles from the playhead onwards each meet the deadline with less work than the
   whole job.
7. **Freshness is a separate dimension from deadline.** The catalog can be minutes stale (B5).
   Availability, play state and a room's state must be true when shown (B2, P8, P6). For "now"
   data, pull when looked at (D325).
8. **Being late needs a defined fallback.** Every row above has an "If late". Some fallbacks are
   harmless (a placeholder poster, "not measured yet"). Some are "Wrong", meaning stale data
   misleads, and those must be fetched fresh or shown as unknown.
9. **Work on a dead resource must not hold a live one.** The 2026-10-21 failures came from this: W1,
   W7, W10, and the 163 s listing.
10. **Due is not low priority.** A backup and a certificate have deadlines that nobody watches (A4).

## 8. Open for the owner

1. **The deadlines.** Each "By" above is a proposal. Which are wrong?
2. **Missing scenarios.** What happens that is not here? Music, photos and offline download are left
   out on purpose, as later work.
3. **The scale to design for in the beta.** How many people play at once per household, per node,
   and per disk? How large a library? How slow can a remote viewer's link be?
4. **The worst case for a person.** When two people's wants collide on one resource (P5 or W11 with
   one more viewer), who yields? The person who started later, the one remote, or both at lower
   quality?
