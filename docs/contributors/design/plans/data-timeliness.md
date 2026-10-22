> **ABOUTME:** User scenarios read for timeliness. In each: what a person expects to see, by when,
> what that takes, what producing it costs and on which resource, what a late answer is worth, and
> what is shown meanwhile. The input to a redesign of how Orpheum fetches, computes, queues and
> caches its data. The scenarios are meant to decide that redesign before any mechanism is chosen.

# Data timeliness

- **Status:** SKETCH. Written 2026-10-22 at the owner's request, as the first step of the queue and
  priority redesign. The owner said: *"The question ultimately comes down to the timeliness of data:
  Is the data there when the user expects it to be? … before we go into the redesign (and I don't
  care even if we have to rip all the pieces out and greenfield them), let's start with making user
  scenarios."*
  - The same day it was reviewed by a data-systems and a UX reviewer and set against prior art, in
    [data-timeliness-review-2026-10-22.md](../research/data-timeliness-review-2026-10-22.md).
    The review's corrections are folded in here, on the owner's word: *"yes, fold them in"*.
  - The owner answered §11 on 2026-10-22:
    - The deadlines are right, and direct play is preferred.
    - Comments on C6, B1, B5 and A8.
    - The scale holds, except whether a GPU can be assumed.
    - All viewers are equal.
    - Reading once for two needs is taken on now.
    - The admin gets a badge and a daily email.

    Those answers are folded in. The four questions still open (the metered connection, the GPU,
    whether running playbacks drop a rung, and quiet hours) were researched in
    [timeliness-followups-2026-10-22.md](../research/timeliness-followups-2026-10-22.md). Its
    proposals are folded in as **Proposed** and wait on §11.
  - The design built from these scenarios is not written yet.
  - This replaces the five design questions of the
    [queue inventory](../research/job-queues-inventory-2026-10-21.md) §5, which the design answers.
- **Depends on:** — (it reads [scenarios.md](scenarios.md), which asks whether the model is right,
  and asks something else: whether the data is there in time.)

## 1. Why

Data has deadlines: someone is waiting for it, or soon will be. Several people can want different
things at once. Producing the data uses up resources:
- CPU, GPU and memory;
- the network to the viewer;
- a disk's seeks, throughput and concurrency;
- a provider's rate and quota.

One priority number per job squeezes all of that into a single dimension, and it failed in
practice. The [queue inventory](../research/job-queues-inventory-2026-10-21.md) found three places
comparing priority by three rules. A listing waited 163 s behind a read-through that nobody was
waiting for, because a playing film had raised that read-through to priority 1000.

The scenarios come first, so that the design answers to them and not to the mechanisms that exist
today. Each scenario also doubles as a load test (§10).

## 2. How to read a scenario

Each scenario is a short story. Then a **Load** line gives how often it happens, how many at once,
and what it costs. Then a table has one row per moment where a person expects something:

| Column | Holds |
| --- | --- |
| **Moment** | What the person does or looks at. |
| **Expects** | What they should see. |
| **By** | The deadline: a class from §3, or a time as p50 / p95, counted from the moment. |
| **Late** | What a late answer is worth (§3.2): **hard**, **firm** or **soft**. |
| **Data** | What has to exist to show it, and where it comes from (§4). |
| **Meanwhile** | What is shown until it is there (§5). |

Some rows cannot be met in time from a cold start. They say so (**not in time cold**), and name the
prediction or the partial result that meets them, or the degraded answer that stands in.

## 3. Deadlines

### 3.1 Classes

These are the classic response-time limits (Nielsen, *Usability Engineering*, 1993), with the start
of a video split out as its own class, and two classes for work no one sits and watches:

| Class | Within | What it means to the person |
| --- | --- | --- |
| **Instant** | 0.1 s | It reacts like a physical object: a button, a scroll, a TV's focus moving. |
| **Flow** | 1 s | The wait is noticed but the train of thought is kept: a page, a poster. |
| **Start** | 2 s at p95, explained at 3 s | The first frame. Viewers begin to give up after 2 s, and each further second loses about 5.8% more of them (Krishnan and Sitaraman, IMC 2012). |
| **Progress** | 10 s, with a determinate indicator | Still watching the screen, and needs to see how far along it is. |
| **Patient** | minutes | Does something else and comes back to check. |
| **Due** | a time, hours to days ahead | No one waits, but it must be done by then: tonight's backup, a certificate before it expires. |
| **Eventually** | no time | Improves things when done, and nothing is due: loudness for a film nobody has opened. |

**Continuous** marks a deadline that keeps renewing. During playback the next chunk is due before
the buffer runs out, again and again for two hours.

**Percentiles.** A deadline is a p50 and a p95, not a single point.
- Users calibrate to the typical case and punish the outliers. A start that is usually 0.3 s and
  sometimes 8 s is worse than one that is always 1.5 s.
- A stall during play costs more than a slower start: one rebuffer causes three times the
  abandonment of one bitrate change (YouSlow, INFOCOM 2016). So the design may buy a better p95 for
  a continuous deadline with a little more startup, for example a longer first buffer for a remote
  play.

### 3.2 What a late answer is worth

| Value | Means | Examples |
| --- | --- | --- |
| **Hard** | Missing it is a failure the person sees. | A chunk past the buffer: a stall. |
| **Firm** | Late is worth nothing. Drop the work; don't finish it. | A poster that scrolled past; chunks for a position someone seeked away from. |
| **Soft** | Worth a little less the later it comes. | Loudness; a poster for the grid as a whole. |

Due work has a **latest start**: its deadline minus a high-percentile estimate of its cost, with
margin. It is Eventually until that point, and becomes urgent as its slack runs out.

### 3.3 Classes are not the scheduler's tiers

The classes describe what people expect. They are not the levels the scheduler acts on. The
prior art is consistent that the scheduler's vocabulary must stay tiny and absolute:
- HTTP/2's priority tree was deprecated as complex and "not successful" (RFC 9113 §5.3.1) and
  replaced by eight urgencies and one flag (RFC 9218).
- Google SRE keeps four criticalities and refuses to add more.
- Ceph hides mClock's settings behind three profiles.

How classes map to tiers is the design's to say (§9).

## 4. Where the data comes from, and what it costs

### 4.1 Sources

From cheapest to dearest. A "reader" is one of a disk's concurrent reads. **Longest piece** is the
longest stretch that cannot be interrupted today. That stretch bounds how long anything more urgent
waits behind it, and §9 asks that it be short.

| Source | Examples | Cost, and on which resource | Longest piece |
| --- | --- | --- | --- |
| The client's memory or cache | The page shown a moment ago, posters already loaded | Nothing; may be stale. A weak device may be slower than the network (Notion, 2024) | — |
| The core's memory | Interest, live playbacks, a node's last status | Microseconds; core memory | — |
| The core's database | Catalog, metadata, play state, availability | Milliseconds to read (performance-targets.md: under 50 ms a page); **one writer** for all writes | A write transaction |
| A node, without touching media | Status, sessions, capacity | One round trip | — |
| A probe of a place (D327) | Can this disk be read now? | One `statfs`: milliseconds, or as long as a dead share leaves it waiting; outside every queue | — |
| A node, reading a little of a file | A keyframe index; a direct play's bytes | A seek and some throughput, per reader; **wakes a sleeping disk (6–30 s; 23 s typical for an 8 TB drive)** | A read |
| A node, listing a directory tree | A scan's listing | Many metadata operations: seconds, more over a share | The whole listing today |
| A node, FFprobe on each file | A scan's examine | One open and a short read per file; a batch takes seconds | One batch |
| A node, reading a whole file | The full read-through (loudness, timeline); an embedded subtitle's extraction | The film's size over the disk's throughput: about 200 s for 20 GB at 100 MB/s; holds a reader | **The whole film today** |
| A node, encoding | Prepare, chunks, trick-play images | CPU or GPU, as long as the film plays or longer; a reader | A chunk; a prepare is the whole film |
| A node's prepared directory | Prepared copies, chunks | Disk space, a budget (W9) | — |
| An outside provider | TMDB, artwork originals, OpenSubtitles | Network; a **rate limit** and a **daily quota**; may be down | A request |
| The household's uplink | Every byte to a remote viewer | Bits per second, shared by every remote viewer; often the tightest resource behind DS-Lite and a relay | — |

### 4.2 Data kinds the first sketch missed

- **Trick-play images**: thumbnails shown while scrubbing (P3). They are made by encoding, and are
  Eventually work until someone scrubs.
- **Negative results**: "TMDB has no match", "this file is corrupt". Cache them with a time to live,
  or the same failure is paid for again every night. A Plex task retried the same 20 bad files
  nightly and woke the disks each time (the review, §2.10).
- **The age of a fact**: when availability or a disk's state was last measured (§5.1).

### 4.3 Costs are elastic

Some demand can shrink to fit:
- A playback can take a lower rung of the ladder.
- A poster can come at a smaller size.
- A preview can be skipped.

Stepping quality down is the first way to shed load, before queueing, just as Unreal Engine drops
texture detail "one mip at a time" to fit its budget.

**A direct play is not elastic.** Netflix and YouTube convert every title into a ladder of rungs
before they show it as available, so starting on a low rung costs them nothing. Orpheum plays
originals in their own formats, with their own quirks. Some play directly, and some need a remux or
an encode first. Lowering a direct play's bitrate means starting an encode. When direct play is
possible it is usually the right choice, ahead of a low transcoded rung (the owner, 2026-10-22).
The "start low and climb" tactic applies only to a playback that is transcoded anyway.

### 4.4 One read can serve two needs

During direct play the node already reads every byte of the film. Loudness, the timeline and
embedded subtitles can be computed from that stream at a cost in CPU only, instead of a second,
competing read of the same file. This is the "cooperative scan" from databases. **It is part of the
rebuild** (the owner, 2026-10-22: *"Take this on now. We want to get this rebuild right rather than
doing things piecemeal."*). A read-through then becomes something a playback can do on the way, and
a read of its own only for a film nobody plays.

## 5. Freshness, and what is shown meanwhile

### 5.1 Three states, not two

A value shown to a person is in one of three states:
- **fresh**;
- **stale but labelled**: shown with its age, then corrected in place;
- **unknown**: said plainly.

Zero's sync engine types every result the same way (unknown, cached, complete) and shows "not found"
only for a complete one.
- **Corrected in place** means values change and boxes keep their size. What is under a person's
  focus never moves. On a TV, a row reordering under the D-pad reads as a broken remote.
- **Availability is a lease, not a fact.** A probe's answer is stale the moment it returns. It is
  shown with its age, and becomes unknown when the lease runs out.
- **A disk has three states:** ready, waking (up to about 30 s), and not answering. A sleeping disk is
  never shown as unavailable.
- **After a core restart**, anything about "now" is unknown until the source says it again. It is
  not stale.

### 5.2 Optimistic, or honest

- **Optimistic about a person's own actions:** Scan now, fix a match, mark watched, remove. Show
  the result at once, and reconcile quietly.
- **Honest about anything that decides whether to press Play:** whether it can play, at what
  quality, with which subtitles. Play stays pressable, and the press answers at once with the
  truth. Play is never greyed out on an answer that may be stale.

### 5.3 Filling the wait

- Show nothing for a wait under about 300 ms. Once a spinner is shown, keep it at least 500 ms, so it
  does not flash.
- During a start, show the film's backdrop and title, not black with a spinner.
- A long job shows a count and an estimate, never only a spinner.
- A known, finite wait ("Waking the drive…") feels shorter than an unexplained one.
- Cached real content is shown before placeholders. Placeholders have the final size.

### 5.4 Telling people what went wrong

Every playback problem answers one question: **your side, the household's server, or this file?**
There are two audiences:
- **A user's message** never says disk, share or node. For example: *"Karl's library is having
  trouble reaching this film. Your connection is fine. We've let Karl know."* It also says what will
  happen next: *"It will pick up by itself when it's back. [Keep waiting] [Watch something else]"*.
- **The admin** gets the detail (the place, when it last answered, what waits behind it), and is
  **notified**, not left to find out from a text (A6).
- **Both see a badge.** An issue shows as a badge in the app for every person it touches, and on
  each film it affects. A user's badge carries only the friendly explanation and a note to let the
  administrator know. The admin's carries the detail (A6).
- A failure costs trust: viewers who meet one are less likely to come back (Krishnan and
  Sitaraman). So after recovery, playback resumes a few seconds before the freeze.

## 6. The people and their devices

- **The owner**: the admin, at home on the LAN with the core and the node, and the NAS on a share.
  They also watch, and while they watch they see no admin noise.
- **The partner**: a user, remote through discovery, on a home connection, often on a laptop.
- **A beta household**: one admin and two to four users, one node, one or two disks, and two people
  playing at once on some evenings.
- **A relative who is not technical**, on a TV with a remote. Every message is written for them by
  default, with the detail behind "Details".
- **A child**: presses Play again and again, can't read an error, and has parental limits.
- **A guest**: opens the household once, through discovery. That one open is their whole
  impression.

The devices are a TV with a D-pad, a phone (sometimes on a metered connection) and a laptop. **No
TV or phone has hover.** The signal that predicts a play is focus resting on a tile (about 500 ms)
on a TV, opening the page on a phone, and Continue Watching's first item as soon as the home page
loads.

## 7. Scenarios

### Setup

#### T1. The first library, 3,000 films on a NAS

The owner installs Orpheum, adds a library on the NAS share and watches. The first 60 seconds
decide whether they trust it.

**Load:** 3,000 files on one disk; one examine batch takes seconds; TMDB at its rate limit;
read-throughs total about a week of one disk's time at 20 GB and 100 MB/s each.

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Picks the folder | Told it can be read, or why not | Flow | Hard | A probe of the place | A spinner, then "not answering" |
| The library is created | Films appear, posters first in **grid order**, not folder order | First ~30 within Progress | Soft | The listing streams in; examine, identification and artwork for the first screenful first | "Looking through the folder: N found" |
| Watches the grid | The rest fill in | Patient | Soft | Identification, artwork | Filename and a placeholder of the poster's size |
| Clicks a film still unscanned | Its page, then it plays | Flow, then Start: **not in time cold** | Hard | That film's examine, identification and keyframe index, ahead of the other 2,999 | The page from the filename, saying what is still being read |
| Comes back later | All identified and playable | Patient (under 1 h per 3,500 items) | Soft | All the above | A count, with an estimate |
| Days later | Loudness measured, subtitles found | Eventually, by value over cost (§8.9) | Soft | Read-throughs, subtitle searches | "Not measured yet" |

#### T2. A second library added while the first still scans

**Load:** two disks; one rate limit at TMDB for both.

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Adds a folder on a USB disk | Its first films appear without waiting for the NAS scan | Progress | Soft | A listing on another disk | As T1 |

#### T3. A second node added

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Enrols it | Shown connected, with its disks | Flow | Hard | Its hello and status | "Connecting" |
| Adds a library on it | As T1 | | | | |

#### T4. Inviting the partner, and a guest's only visit

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Makes the invite | A link or code | Flow | Hard | The database | — |
| The partner or a guest opens it for the first time | The home page | ≤3 s p50, ≤5 s p95 | Hard | Discovery, the paths, the session, the home page | "Connecting to <household>", with the household's name |

### Connecting

#### C1. Opening the app at home

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Opens the app | The home page | The shell from cache ≤0.3 s; fresh data ≤1 s p95 | Soft | The database; posters from the client's cache | The last page seen, corrected in place |
| Looks at Continue Watching | Right, including last night on another device | Flow | Hard if wrong | Play state | Stale but labelled, never reordered under focus |
| — | The first item warmed to play | Before any press | Soft | Its keyframe index and first chunks (§8.5) | — |

#### C2. The partner opens it remotely

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Opens the app | The home page | The cached shell at once; first useful screen ≤3 s p50, ≤5 s p95 | Soft | Discovery, the paths, the session | The last page seen, marked as connecting |
| Scrolls | Posters | Flow | Firm per poster | Sizes for the link's speed | Placeholders of the final size |

**What it says:** the network to the viewer is a resource, and on a slow one, sending less is how the
data arrives sooner.

#### C3. Opening it after the core restarted or was updated

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Opens the app | As C1 | Flow | Soft | The database answers; interest, playbacks and nodes' statuses are gone | Anything about "now" is **unknown**, not stale |
| — | Interest is known again | Within seconds | Soft | Each client says its interest again **on reconnecting**, not at its next 15 s tick | — |

#### C4. A TV or a cast device joins

As C1, with less memory: the first screen and nothing it does not need. Focus moves are Instant
from the first screen on.

#### C5. The app comes back from the background (missing before)

This is the most frequent "open" by far: a phone unlocked, a TV woken.

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Returns to the app | What was there | Instant from cache | Soft | — | The cached page, revalidated quietly |

#### C6. A phone on a metered connection (missing before)

A data cap makes the client's own prefetch cost the person money. Warming on the node costs the
phone nothing, so only what the client fetches counts. Researched in the [follow-ups](../research/timeliness-followups-2026-10-22.md) §1.

- **Competitors:**
  - Netflix and Plex cap quality on cellular by default and let the person raise it.
  - Spotify downloads on Wi-Fi only.
  - Jellyfin, Emby and Infuse do nothing for cellular.
  - None documents switching prefetch off.
- **Complaints:**
  - Too much data is a real complaint, but rare.
  - Quality stuck too low on cellular is the more persistent one.
- **What the platforms say:** native apps get a reliable signal. The web client gets almost nothing:
  only Chrome for Android, and nothing from Safari.

**Proposed** (§11 Q1):
- On a connection the platform says is metered, the client doesn't prefetch video ahead of playback,
  and caps quality visibly and liftably.
- When the signal is unknown, it behaves as usual.
- A per-device "Mobile data" setting: Automatic, Save data, Unrestricted.
- For the closed beta: no detection, a small cancellable prefetch, and the toggle only if cheap.

### Browsing

#### B1. Scrolling the grid of 3,000 films, and moving focus on a TV

**Load:** a fast scroll asks for hundreds of posters in a second; most of them leave view before
they arrive.

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Scrolls, or moves focus | The grid moves; focus moves | Instant | Hard | Already in the client | — |
| Posters come into view | Each poster | **Before it is visible**: prefetched 1–2 screens ahead | Firm | The artwork size for this screen | A placeholder of the poster's size |
| Posters scrolled past | Nothing | — | — | Their requests are cancelled in the client, and the newest asked is served first | — |
| Looks at the side of the grid | A jump bar of first letters, in the locale's own alphabet, showing only letters some film starts with | With the grid | Soft | The **letter index** of the current view (below) | The bar without letters |
| Jumps from "A" to "L" | The grid at "L", posters at once | Flow for the grid; posters as above | Hard for the grid, firm per poster | The window of the view at L's offset, and its posters, ahead of the ones scrolled past | Placeholders of the final size at the right place |

**Jumping by letter** (the owner, 2026-10-22), as Jellyfin and Plex do:
- **A jump is a fetch at an offset.** It is not a scroll through what lies between, so the posters
  between "A" and "L" are never asked for.
- **The letter index is data of its own:** each first letter, with how many entries come before it,
  for the view as it is now sorted and filtered.
- **The letters are the locale's.** The ordering and the index letters come from the locale's
  collation: CLDR's index characters and ICU's alphabetic index exist for exactly this. A German
  library files "Ä" under "A"; a Swedish one shows "Ä" as a letter of its own after "Z". Titles in
  other scripts fall into the scripts' own groups.
- **Only letters some film starts with are shown.** The index is built from the titles, not from
  the alphabet.
- **The index belongs to the view.** A filter or a search changes it (B8).

#### B8. Filtering by genre and year, and searching within a view (missing before)

**Load:** a filter changes the whole set; a person tries several in a few seconds.

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Picks a genre or a year | The grid of matching films, its count, and its jump bar | Flow | Firm: superseded by the next pick | The view's first window, count and letter index, from the database | The previous view, marked as updating, never emptied first |
| Changes the filter again quickly | Only the last one counts | Flow | Firm | The earlier fetches are cancelled | — |
| Types a search inside the view | As B3, within the filter | As B3 | Firm per keystroke | As B3 | As B3 |

**What it says:** a view is a query, and its window, count and letter index come together. A
superseded view's fetches are worthless at once, so they are cancelled.

#### B2. Opening a film's page

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Opens it | Title, art, synopsis, cast | Flow | Soft | The database | — |
| Reads it | Whether it can play now | ≤1 s, shown with its age | Hard if wrong | Availability's lease (§5.1) | Unknown; **Play stays pressable** |
| Reads it | Tracks, subtitles, length, loudness | Flow, or "not measured yet" | Soft | The examine; the read-through, maybe not | "Not measured yet" |
| Focus rests on Play, or the page opens on a phone | — | — | — | A **prediction**: warm the keyframe index and the first chunks, within prediction's own small budget (§8.2) | — |

#### B3. Searching

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Types | Results as they type | 200 ms after typing stops (a 300 ms debounce on a TV keyboard) | Firm per keystroke | The database | The last results |

#### B4. A series

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Opens the series | Seasons, episodes, Next Up | Flow | Soft | The database | — |
| Will play the next episode | — | — | — | A prediction: warm the next three episodes (interest-priority.md) | — |

#### B5. A film copied to the NAS, then looked for in the app

The owner, 2026-10-22:
- *"When I copy a movie to my nas and then go back to orpheum, I get confused if the movie isn't
  already shown — even if it's just the basic grid entry (with no poster) to show that orpheum is
  aware of it now. When I don't see it, I start looking for the 'scan library now' button because I
  don't know if orpheum is looking for it or not."*
- *"What's also frustrating is copying a movie in, not seeing it, scanning, STILL not seeing it,
  scanning again, no dice, going through a long debugging process, only to discover that it saw it
  but did nothing because I had the file/directory permissions or ownership wrong."*

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Copies it, then opens the app or the library | **It is there**, at least as a bare grid entry with its file name and no poster, marked as being read | Flow to Progress, counted from opening the library | Hard: a missing entry sends the person looking for "Scan now" | The new file **known**, before it is examined or identified | — |
| Watches it | Its title and poster arrive in place | Patient | Soft | Examine, identification, artwork | The bare entry |
| The file can't be read (permissions, ownership, a name the system can't open) | **The entry is there, and says why**: "Orpheum can see this file but can't read it: check its permissions" | As the first row | Hard: silence here cost the owner a long debugging session | The failure, kept with the file (§4.2) | — |
| Presses "Scan now" anyway | It says what it found: N new, M it can't read, and why | Instant acknowledgement, then Progress | Soft | The scan's result, by outcome | — |

- **Knowing comes before understanding.** Seeing a new file is cheap (a listing). Examining and
  identifying it is not. The cheap step must not wait on the dear one, and the grid shows what is
  known.
- **Opening the library is a want for its freshness.** Over NFS there is no watcher: the kernel's
  change notification doesn't see changes made by other machines, and the scenarios can't rely on a
  scan's schedule. So opening the library, or returning to the app (C5), asks for a cheap check of
  what changed. It is pulled when looked at (D325), as availability is.
  - A directory's modification time changes when a file is added to it, so the check can start
    with the directories, not the files.
  - Over NFS the attribute cache can hide a change for up to a minute. That bounds this row's p95
    on a share unless the check reads past the cache.
  - The check is a person's want, so it may wake a sleeping disk (A8). It runs when someone looks,
    never on a timer: a timer is what kept Jellyfin's disks from ever sleeping.
- **Every file the scan sees appears**, readable or not. A file it can't use is never dropped
  silently. Its reason is shown on the entry, and it is counted in the scan's result and in the
  admin's issues (A6).

**What it says:** the catalog's "is the file there" can't be minutes stale once someone looks. Its
"what is the film" can be. Availability can never be stale. A notification bounds the catalog's
staleness too (A9).

#### B6. Two people browse while a big scan runs

Both get B1 and B2 as usual. Neither the scan's examines nor its TMDB fetches delay a page or a
poster someone is looking at.

#### B7. Read your own writes (missing before)

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Stops a film, presses Back | Continue Watching shows the new position | Instant | Hard if wrong | The client's own last report, before the server confirms | — |

### Playing

#### P1. Play a film that plays directly

**Load:** at most two playbacks at once per household on the evenings that have them; a
1080p direct play needs about 10 Mbit/s; a 4K one about 40–80.

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Presses Play, LAN | The first frame | ≤1 s p50, ≤2 s p95 | Hard | Availability; the keyframe index; the first bytes | The backdrop and title; at 3 s, why ("waking the drive", "the drive is busy") |
| Presses Play, remote | The first frame | ≤2 s p50, ≤4 s p95; may trade startup for fewer stalls | Hard | The same, over the uplink | As above |
| Plays from Continue Watching | The first frame | ≤1 s | Hard | Warmed when the home page loaded (C1) | As above |
| Watches | No stalls | Continuous | Hard | Bytes ahead of the playhead, under a **reservation** of disk throughput and uplink (§8.4) | A stall, said when it happens |

#### P2. Play a film that needs a remux or an encode

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Presses Play | The first frame | ≤3 s p95; past 3 s, a stated reason | Hard | The first chunks, encoded on demand | Backdrop and title, then progress |
| Watches | No stalls | Continuous: the encoder stays ahead of the playhead, then sleeps | Hard | CPU or GPU and a reader, reserved; the read rate limited to what it needs | A stall; a lower rung offered (D253) |

#### P3. Seeking and scrubbing

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Drags the bar | Thumbnails along it | Instant | Firm | **Trick-play images** (§4.2) | The bar alone |
| Lets go at 1:10:00 | The picture there | ≤1 s LAN, ≤2 s remote | Hard | The chunk there; work for the old position is **dropped** | The last frame, then a spinner |
| Skips ±10 s | The picture there | Instant | Hard | The buffer kept behind the playhead | — |

#### P4. Turning on an embedded subtitle

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Picks a track | Subtitles from the current line | ≤2 s: **not in time cold**, since extraction reads the whole file | Hard | The track's text near the playhead | "Subtitles loading", then text from the playhead on |
| Their profile always uses subtitles | Subtitles from the first line | Instant | Hard: for a deaf viewer the subtitles are the film | Predicted: extracted on the look, or from the playback's own read (§4.4) | — |

#### P5. Two people play two films from the same NAS disk

Both have P1's or P2's continuous deadlines on one disk's throughput. Both hold, because each
playback is admitted against its reservation (§8.4). Maintenance on that disk takes only what is
left, at a limited read rate. All viewers are equal: admin or user, at home or remote (the owner,
2026-10-22).

When they don't all fit, the research ([follow-ups](../research/timeliness-followups-2026-10-22.md) §3) points one way:
- No study compares the two choices at the moment someone arrives.
- A stall costs more than a switch, and viewers punish a drop more than they reward a rise.
- No competitor lowers running streams to admit a newcomer.

**Proposed** (§11 Q3): "equal" means one rule for everyone: *once you are watching, nobody's arrival
stalls you or lowers your picture.*

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| A newcomer presses Play and what is left fits | Plays at what is left, paced to it | Start | Hard | The budget: the uplink less headroom, counting each stream at its peak; the encoder's free slots; the disk's reservation | — |
| What is left is below the floor | A choice: "wait, we'll say when there's room" or "play now at reduced quality" | Flow | Hard | The floor, which may depend on the screen (a phone needs less than a TV) | — |
| Someone stops | A lowered stream rises once, one step | — | Soft | — | — |
| The encoder is full | Offered a direct play or remux if the device can take one; otherwise wait | Flow | Hard | The encoder's strict admission | — |

- A direct play is a fixed reservation at its peak and never steps down. Stepping down would start
  a transcode.
- An option, off by default: running *transcodes* drop one rung, but only to lift a newcomer from
  below the floor to it.

#### P6. Watch Together, one person remote

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Both press Play | Both start together | Start | Hard | The same chunks, perhaps at two qualities; the room's state | Wait for the slower one, and say so |
| One pauses | The other pauses | Instant to Flow | Hard | The room's state, between the peers over WebRTC, outside the job machinery | Drift, corrected |

#### P7. Next episode

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Credits roll | The next episode offered, with a countdown | Instant | Soft | Next Up | — |
| It starts | The first frame | ≤1 s, since it was predicted | Hard | Warmed during the credits or before | As P1 |

#### P8. Resuming on another device

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Stops on the laptop, opens the TV | Resume where it stopped | Flow | Hard if wrong | Play state from the last report | At most 10 s back |

#### P9. Background work on the film being watched

The read-through and the subtitle extraction help later; only the bytes for the playhead are
needed now. Nobody is blocked on them, so they never take the playback's reader or bandwidth, and
they are never raised to the playback's class. Better still, they are computed from the playback's
own read (§4.4). This is the 2026-10-21 failure seen from the playback's side.

#### P10. A long pause (missing before)

Someone pauses for 30 minutes. The playback's reservation is released after a while, and its lease
on interest lapses. On resume it is admitted again like a new start, but warmed, so it takes Flow.

#### P11. Audio track and language at the start (missing before)

The person's usual language picks the audio track and any forced subtitles before the first frame.
It is the same kind of prediction as P4.

#### P12. Casting from a phone to a TV mid-film (missing before)

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Casts | The TV plays from the same second | Start | Hard | The position, the TV's own play (new admission), and the phone's playback ended | The phone keeps playing until the TV starts |

### Administering

#### A1. "Scan now"

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Presses it | Acknowledged | Instant (optimistic) | Hard | — | — |
| Watches | Progress, and an estimate | The estimate within 10 s | Soft | The scan's counts | The last count |
| Waits | Done | Patient | Soft | The listing, examines | — |

An admin's request goes before maintenance and never before a playback.

#### A2. Fixing a wrong match

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Picks the right film | The page shows it, with its poster | Progress; the title at once (optimistic) | Soft | One TMDB fetch, one artwork original and its size | The old poster, then the new one; if TMDB refuses, when it will be tried again |

#### A3. The status page: nodes, disks, work

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Opens it | What is happening now: each disk's state, what runs, what waits and **why** | The last known, with its age, at once; live within 2 s | Hard if wrong | Pulled when looked at (D325): probes, the node's work, the core's queue | The last known, labelled |
| Reads it | Whether the scheduler is keeping up | Flow | Soft | The deadline-miss ratio and lateness of each tier on each resource (§10) | — |

#### A4. A backup and a certificate

Nobody watches them; each is Due. Each has a latest start (§3.2), and it rises as its slack runs out.
A backup that slips misses its night, and a certificate that slips lapses.

#### A5. Removing a library or a node

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Removes it | Gone from every page | Instant (optimistic) | Soft | The database | — |
| — | Its work is gone too | Flow | Hard | The work's wants end, so it is cancelled at its next slice | — |

#### A6. The admin is told (missing before)

A share stops answering, a node goes offline, a certificate or backup is at risk, a user's play
failed, or a file can't be read (B5). The admin is notified, with what a user met: *"a user hit
this at 21:14 on Film X"*. In a household the partner texts "it's broken" first, and the admin
should already know.

The owner, 2026-10-22: *"Show the badge, and email at most once a day to inform of new issues. The
badge will need to show for regular users as well, since the issue will likely lead to badges on
particular films."*

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| An issue begins | A badge in the app, for the admin and for each user it touches, and on each film it affects | Flow, once the issue is known | Hard | The issue, its place, the films on that place | — |
| The admin opens the badge | What happened, where, since when, who met it | Flow | Soft | The issue's record | — |
| A user opens the badge | A plain explanation and a note to let the administrator know | Flow | Soft | The same, in the user's words | — |
| The next day's email | New issues since the last email, **at most one email a day**, none when nothing is new | Due | Soft | The issues and when each began and ended | — |

An issue that began and ended between two emails is still reported: the admin learns that a share
dropped for ten minutes last night.

#### A7. Re-deriving everything (missing before)

A new loudness algorithm or a provider change re-derives 3,000 films: about a week of one disk's
time. It needs a budget and a rate. It is Eventually by value over cost, and it runs in quiet hours
on a disk that is awake.

#### A8. Quiet hours and sleeping disks (missing before)

The owner, 2026-10-22: *"Quiet hours by default, but if the machine is in a sound-isolated
environment, allow the user to decide if quiet hours matter. We still avoid waking disks unless
there's real work to be done."*

- **Quiet hours are on by default.** Noisy background work, meaning anything that keeps a disk
  seeking, doesn't run in them. An admin whose machine is sound-isolated can switch them off.
- **A disk is never woken for nothing.** Whatever quiet hours say, a sleeping disk is woken only for
  real work. Background work is batched so that one waking serves it all, and waits for a disk that
  is awake anyway.
- **A person's want wakes a disk at any hour.**
- **Researched** ([follow-ups](../research/timeliness-followups-2026-10-22.md) §4):
  - Plex (03:00–06:00) and Jellyfin (02:00–03:00) run heavy work at night because nobody watches
    then, which is when a disk near a bedroom is heard.
  - Both have public bugs that keep disks awake: retrying the same failed files every night, and
    stat-ing files every 5 minutes.
  - Modern drives are rated for 600,000 load cycles, and no data shows spin-down wears them.
  - A sleeping drive saves about €9–19 a year at German prices. Mostly it buys silence.
  - Seeking, not spinning, is what people hear.
  - Windows moved from a fixed maintenance hour to idle-triggered work with deadlines. Android's
    Doze batches deferred work into one maintenance window.

**Proposed** (§11 Q4):
- **Quiet hours are a veto, and idleness is the trigger.**
  - Non-urgent work runs only when nothing plays, and stops when a playback starts.
  - It never runs in quiet hours (on by default, for example 22:00–07:00).
  - Each kind of work has a deadline. Overdue work runs at the next idle time outside quiet hours.
- **A sleeping disk isn't woken for non-urgent work.** Its work is batched, and runs either right
  after a playback on that disk ends, while it still spins, or all at once when the batch passes a
  size or its oldest job passes its deadline.
- **The known disk-wakers are avoided:**
  - each failure is remembered until its file changes;
  - no file is polled on a schedule;
  - metadata and artwork stay off the media disks.
- **Over a share, the disk's state can't be seen**, so the node only avoids being the one that
  wakes it.
- **"Sound-isolated" lifts only the veto**, making night the preferred time. It is asked as a
  question about where the storage stands.
- **The admin sees:**
  - the quiet hours;
  - each disk's waiting work and its next expected run;
  - every wake Orpheum caused, and why;
  - that the NAS's own sleep setting decides whether its disks ever sleep.

#### A9. A notification of a new episode (missing before)

"New episode of X" is a promise. When the person opens it, the episode is in the catalog and plays,
so the catalog's staleness is bounded by the notification and not only by the scan's schedule.

### Things going wrong

#### W1. The share drops while a film plays (the 2026-10-21 tests)

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| The picture freezes | Told it is the household's server, not their connection | ≤3 s | Hard | A probe of the place | The two-audience message (§5.4); the admin notified (A6) |
| The share returns | Playback resumes, a few seconds before the freeze | Start, counted from when the share answers | Hard | A probe every 2 s (D327) | Today about 48 s of the host's NFS recovery, which no Orpheum code can shorten |
| After it returns | Nothing else breaks | — | — | Work comes back **gradually**, playback first; retries have a budget (§8.7) | — |
| Meanwhile | Browsing the rest is unaffected | Flow | Hard | Nothing waits on the dead place: its breaker is open | — |

#### W2. Browsing while the share is down

The film's page shows its availability at once, from the lease (§5.1). The page never waits on the
dead disk, and nothing waits for a stall to time out to find out.

#### W3. The node is off

As W1 and W2, for a whole node. Every film on it says so at once. Its queued work neither runs,
holds anything, nor is lost.

#### W4. The core restarts during a playback

The player keeps playing from its buffer, reconnects within Progress, and says its interest again
on reconnecting (C3).

#### W5. The partner's link degrades

ABR drops the quality. If that rung isn't prepared yet, its chunks become P2's continuous deadline
at the lower rung. A degrading link turns a "would be nice" rendition into a "due now" one.

#### W6. TMDB is down or rate-limited

New films show by filename. A person's "fix the match" waits and says when it will be tried again.
The provider's rate and quota are resources, and a person's request is spent from them before the
sweep's.

#### W7. A disk asleep or slow

| Moment | Expects | By | Late | Data | Meanwhile |
| --- | --- | --- | --- | --- | --- |
| Presses Play on a sleeping disk | Told the drive is waking | ≤1.5 s | Hard | The disk's state: waking | "Waking the drive…" with a determinate bar to about 30 s |
| — | The play goes first once it wakes | Start, after spin-up | Hard | The play's read ahead of the scan that woke the disk | — |

#### W8. 200 films copied in during a playback

The playback holds (P1 or P2). The new films appear within Patient. The scan takes what the
playback leaves, at a limited read rate, and never more.

#### W9. A full disk

When the prepared directory is full, prepares fail, unless eviction by value frees room first
(§8.10). A playback that needs a prepare is offered direct play or a lower rung if the film has
one. When the core's disk is full, writes fail (scenarios.md S77).

#### W10. A corrupt file that hangs FFmpeg

The file fails within a window set by the want's deadline. The failure is remembered (§4.2), so it
is not tried again every night. The disk's other work goes on, and a hang never holds the disk's
reader for everyone.

#### W11. A node at its limit: two encodes and a read-through

The two playbacks' encodes come first, and the read-through waits or yields at its next slice. A
third player is admitted at whatever fits (P5), and told why it got a lower rung, without naming
the other person's film.

**Load:** with no hardware encoder (§11 Q2), a 4-core node guarantees **one** 1080p software
transcode, and a second as best effort. 4K HDR tone-mapped to an SDR screen can't run in real time
in software at all.

#### W12. Files moved or renamed, or a disk replaced (missing before)

What was derived (the keyframe index, the read-through, subtitles, prepared copies) follows the file
if its identity holds (§8.10). A disk remounted under a new device brings back what was known, and
re-derives nothing that did not change.

#### W13. The node crashes mid-job (missing before)

Half-written outputs are never mistaken for whole ones: they are written to a temporary name, then
renamed. The job is offered again, and doing it twice does no harm.

#### W14. Another program on the NAS (missing before)

A RAID scrub or another media server takes the disk's throughput, and Orpheum can't see it. The
disk's limit, learned from read latency (§8.6), shrinks by itself. Playback keeps its reservation,
and maintenance yields.

#### W15. A child presses Play five times (missing before)

The five presses are one want, not five. Nothing is queued twice. An error on a child's profile is
shown as a picture, not as words.

### Combinations (missing before)

The real failures live in combinations. Each is a load test of its own (§10).

- **X1. The evening peak.** At 20:00 two people start films. The scheduled scan lands at the same
  time, and a batch of new episodes arrives.
- **X2. T1 with P5 with W8.** The first library is still scanning, two people play from its disk,
  and a batch is copied in.
- **X3. W1 at 21:00.** Two playbacks, the partner remote, the share drops and returns.

## 8. What the scenarios say

These are the scenarios' conclusions, corrected by the review. They are input to the design, not
the design itself.

1. **A deadline belongs to a want, not to a kind of work.** The same examine, keyframe index or
   poster is Eventually for the library and Flow for the film someone opened. Priority is worked out
   from the wants alive now, not stored in a job. OpenLayers drops map tiles that left the view, and
   Unreal re-plans texture loading every frame.
2. **A prediction is a want of its own class, with its own small budget.** It is the first thing
   cancelled. It never inherits the class of a person who is waiting: Netflix gives prefetch only
   what requests a person made leave over. When a prediction comes true, a new want appears for
   exactly the data now needed, and the background job doesn't change class. Priority passes only
   along a real "X is blocked on Y" edge.
3. **A want is a lease with a holder, an absolute deadline and a class**, carried from the client to
   the core to the node. Work shared by several wants takes the earliest deadline and highest class
   among them. When the last lease ends, the work is cancelled at its next slice.
   - Firm wants that are abandonable (posters, search) serve the newest first.
   - Cancelling a poster begins in the client.
4. **Continuous deadlines get a reservation and admission control.** A playback is admitted only if
   its reservation fits: disk throughput, encode rate and uplink. Otherwise it is offered a lower
   rung that does fit. A running playback is protected over a new start, because a stall costs more
   than a slower start. All viewers are equal under that rule (P5).
5. **Prediction turns expensive data into timely data.** These predict a play:
   - focus resting, or the page opening;
   - the first item of Continue Watching;
   - the credits, for the next episode;
   - a usual language, for a subtitle or audio track;
   - a slow link, for a lower rung.
6. **Costs are a vector, and capacity is learned.** Two jobs compete only for the same resource. A
   disk's safe concurrency is learned from its read latency, as Netflix's concurrency-limits and
   Meta's IOCost do. A dead or busy disk then shrinks its own limit, and background reads have a
   rate limit, not only a pause.
7. **Fail fast on a dead place, and come back slowly.**
   - A dead place's breaker is open: it admits nothing new, and a read stuck in the kernel counts as
     gone until it returns.
   - On recovery, work is let back gradually, playback first.
   - Retries have a budget, duplicates are merged, and wakeups are jittered.
   - Otherwise the system's own retries keep it overloaded after the cause is gone (a metastable
     failure).
8. **Long work is made of short pieces.** A read-through reads a slice at a time, with a saved
   position and loudness state that merges. The scheduler decides again between slices. Below the
   one place that decides, queues stay shallow. In *The Tail at Scale*: "keep low-level queues short
   so higher-level policies take effect more quickly".
9. **Eventually work is ordered by value over cost, after a cheap pass over everything.** Meta
   orders encodes by benefit ÷ cost. For Orpheum:
   - the cheap examine covers every file first;
   - read-throughs are then ordered by predicted watching ÷ the film's size over the disk's
     throughput;
   - they run in quiet hours, on an awake disk.
10. **Derived data is a cache with rules:**
    - an identity that survives a move;
    - a size budget, with eviction weighing the cost to recompute;
    - negative results kept with a time to live;
    - a staleness bound for each kind.

    Availability is a lease.
11. **Partial results count.** The keyframe index comes before the full read. The first chunks come
    before the rest. Subtitles come from the playhead on.
12. **One read can serve two needs** (§4.4).
13. **Late needs a defined answer**: stale but labelled, unknown, a smaller rung, or a reason with a
    retry time. Never just a spinner.
14. **Due is not low priority.** It has a latest start, and it rises as its slack runs out.

## 9. What the design must settle

This sketch does not decide these. The design does, against the scenarios.

- **Where each resource is decided.** A disk's readers and throughput are known best on its node,
  next to the disk. A provider's rate is known in the core. The prior art says each resource has
  exactly one place that decides, with shallow queues below it. The queue inventory's "one scheduler
  or two in series" becomes "one decider per resource, next to it".
- **The tiers**: perhaps three. Continuous or interactive; attended (a person's request, Patient);
  and background (Due and Eventually, ordered by latest start and by value over cost). Each tier has
  a reservation, a weight and a limit on each resource. EDF runs only inside a tier.
- **The length of a slice** for each kind of long work, and how a read-through's loudness merges
  across slices.
- **How wants travel**: as leases from the client, which says what it shows, plays and predicts.
- **The cache's identities, budgets and eviction.**
- **What is kept from D324, D327 and D328.** Interest becomes the source of wants. The probe and
  the place table stay.

## 10. Checking the design

- **A simulation first.** A discrete-event simulation of the scheduler on the fake clock
  (`clocktest`) replays the scenarios and the combinations X1–X3. It checks each tier's
  deadline-miss ratio before anything is built.
- **In shadow.** The new scheduler runs beside the old one, deciding nothing, until its decisions
  are trusted. Figma did this before switching its load sets on.
- **Measured.** The scheduler is judged by the deadline-miss ratio and the lateness of each tier on
  each resource. The status page shows the same numbers (A3).

## 11. Questions for the owner

### Answered 2026-10-22

- **The deadlines** are right. Direct play is preferred over a low transcoded rung, because Orpheum
  plays originals and is not a service with every title pre-converted (§4.3).
- **The scenarios:**
  - C6 was researched.
  - B1 gained a jump bar by first letter, and B8 filters.
  - B5 now has a file shown as soon as it is known, and an unreadable one says why.
  - A8 has quiet hours by default and no disk woken for nothing.
- **The scale:** a household of four, two playbacks at peak with one of them remote, one 4-core
  node, a NAS on 1 Gbit/s, 3,000 films and 20,000 episodes, the slowest remote link at 5 Mbit/s,
  and an uplink of 20 Mbit/s. Whether a GPU can be assumed is Q2.
- **All viewers are equal:** admin or user, at home or remote.
- **Reading once for two needs** is part of the rebuild (§4.4).
- **Telling the admin:** a badge for the admin and for every user an issue touches, a badge on each
  film affected, and at most one email a day of new issues (A6).

### Open

Each comes with a proposal from the
[follow-up research](../research/timeliness-followups-2026-10-22.md).

1. **Metered connections (C6).**
   - *Found:*
     - Mobile data in Germany is still mostly capped: 8.4 GB a SIM a month, against 3 GB or more
       for an hour of full-quality video. Home lines are not capped.
     - Netflix and Plex cap cellular quality by default. Jellyfin does nothing.
     - Complaints of too much data are rare. Complaints of quality stuck too low are more common.
     - The web client gets almost no signal: Chrome for Android only, nothing from Safari.
   - *Proposed:*
     - no detection in the beta;
     - the client's own video prefetch kept small and cancellable;
     - warming on the node unaffected, since it costs the phone nothing;
     - a per-device "Mobile data" setting (Automatic, Save data, Unrestricted) only if it is cheap;
     - detection once native apps exist.

   Agree?
2. **No GPU (§7 W11).**
   - *Found:* nobody publishes the share of hosts without a hardware encoder, but large groups have
     none:
     - every 2025 Synology "plus" model;
     - the Raspberry Pi 5;
     - rented servers, a quarter of self-hosters in one survey;
     - every free Plex user, since hardware transcoding needs Plex Pass.

     Passing a GPU into a container often fails. On a 4-core CPU, Plex's rule gives about two
     1080p software transcodes. A 4K HDR source tone-mapped to SDR runs at about a third of real
     time.
   - *Proposed:*
     - assume no hardware encoder, and detect one as a bonus;
     - guarantee one 1080p transcode per node, with a second as best effort;
     - order the choices: direct play, then remux, then an audio-only transcode, then a video
       transcode.

   Agree? And a decision only you can make: **what does a 4K HDR film do for an SDR screen when
   the node has no hardware?**
   - (a) direct play only, leaving the client to tone-map or show washed-out colour;
   - (b) a 1080p SDR version prepared ahead, in quiet time, which costs disk space;
   - (c) "this film needs a screen that can show HDR", with the alternatives.
3. **Several viewers on too little (P5).**
   - *Found:*
     - no study compares "the newcomer takes what is left" with "everyone drops a rung" at the
       moment someone arrives;
     - a stall costs three times a switch;
     - viewers punish a drop more than they reward a rise;
     - Plex, Jellyfin and Emby never lower a running stream for a newcomer;
     - the newer research weighs each viewer by their screen, since a phone needs less than a TV.
   - *Proposed:* "equal" means one rule for everyone: once watching, nobody's arrival stalls you or
     lowers your picture.
     - The newcomer takes what is left, down to a floor.
     - Below the floor, the newcomer chooses: wait, or play reduced.
     - The floor depends on the screen.
     - A direct play never steps down.
     - The encoder admits strictly.
     - An option, off by default, lets running transcodes drop one rung to lift a newcomer to the
       floor.

   Agree, and should that option exist at all?
4. **Quiet hours (A8).**
   - *Found:*
     - Plex and Jellyfin run heavy work at night, which is when a disk is heard, and both have
       bugs that keep disks awake;
     - modern drives take 600,000 load cycles, and no data shows spin-down wears them;
     - a sleeping drive saves €9–19 a year and buys silence;
     - seeking is what people hear;
     - Windows and Android moved to idle-triggered work with deadlines, batched into windows.
   - *Proposed:*
     - quiet hours as a veto (on by default, 22:00–07:00), with idleness as the trigger;
     - a deadline per kind of work;
     - a sleeping disk woken only for a person's want, or for a batch that has grown large or
       waited too long;
     - "sound-isolated" lifts only the veto.

   Agree? And are 22:00–07:00 the right default hours?
5. **A user's badge.** The admin already gets a badge and the daily email, so a user's note could
   say either *"The administrator has been told"* or *"Let the administrator know"*. Which did you
   mean? The first stops a user texting about something already known. The second covers the time
   before the next email.
