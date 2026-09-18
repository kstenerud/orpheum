> **ABOUTME:** How the server learns that the library changed, across the filesystems people
> actually use — and an evaluation of the NAS-side index agent idea, which is the only approach
> that genuinely solves the network-share case.

# Library Change Detection

Researched **2026-09-18**. Feeds [`scanner.md`](../plans/scanner.md).

## The problem

Two complaints in this field share one root: *"the scanner did not notice my new file"* and
*"the scanner rescanned everything at 3 a.m. and pegged the CPU"*. Both are change detection
failing, in opposite directions.

The constraint that causes it: **`inotify` does not work over SMB or NFS**, which is how a large
share of self-hosters mount their media. It also has per-user watch limits that a large library
exhausts, and it reports nothing about changes made by the remote server. So the mechanism that
works perfectly on a developer's laptop is unavailable to a meaningful fraction of users, and
fails *silently* — which is the worst property a detection mechanism can have.

## The strategies, and what each actually costs

| Strategy | Works on | Latency | Cost | Fails how |
| --- | --- | --- | --- | --- |
| `inotify` / `kqueue` / `FSEvents` | Local filesystems | Instant | Cheap | Watch-limit exhaustion; **silently absent on SMB/NFS** |
| Periodic full walk | Everything | Scan interval | Expensive — stats every file | Noticeably, in CPU and disk |
| mtime-delta walk | Most things | Scan interval | Cheaper than full | Wrong where shares do not update directory mtimes |
| **NAS-side index agent** | Network shares | Near-instant | Cheap on both ends | The agent is not installed, or is stale |
| Explicit "rescan" button / API | Everything | User-driven | Free | Depends on a human, but always works |

**Position: this is a strategy interface with a capability report, not a single mechanism.** The
server picks the best available per library root, *tells the user which one it is using*, and
degrades to periodic walking. The "tells the user" half is the important half — the current
failure mode in this field is that watching silently is not happening and nobody knows.

## The NAS-side index agent — evaluation

The proposal: a small program installed on the NAS watches the filesystem locally (where
`inotify` works fine) and maintains an index file on the share. The media server then watches
**one file** over SMB/NFS rather than a tree it cannot watch at all.

**It is a good idea, and it is architecturally consistent.** It is precisely a **tier-3
capability service** under [D19](../../decisions/working-notes.md) — an out-of-process component
that registers with the core, may run on a different machine, and is reached over a defined
interface. It needs no new concepts, and it is the second independent case for that tier after
the transcoder and intro-detection, which is good evidence the tier is real rather than fitted.

**What is genuinely elegant about the index-file transport**: no network protocol, no listening
port, no authentication, no firewall rule, no service discovery. Both sides already have the
share mounted. That removes almost all of the operational surface a conventional agent would
carry, and it is the reason to prefer it over having the agent speak HTTP to us.

**The costs, stated honestly:**

- **Installation on a hostile platform.** Consumer NAS vendors' app ecosystems are closed,
  inconsistent, and split across x86 and several ARM variants. Some units will not run an
  arbitrary binary at all. "Install our agent on your NAS" is a large ask and adoption will be a
  minority of a minority.
- **A second thing to version.** An agent that is one release behind, or silently dead, produces
  a library that quietly stops updating — the exact failure we are trying to remove. It must
  heartbeat into the index, and the server must surface staleness as a visible warning.
- **Atomicity over network filesystems.** The index has to be written by one machine and read by
  another without tearing. Write-temp-then-atomic-rename in the same directory is the standard
  idiom and NFS honours it; SMB is mostly fine but client-side caching can delay visibility.
  Polling one small file is cheap enough that this is a latency question rather than a
  correctness one, provided the reader never trusts a partial file — so the index needs a length
  or checksum it can validate.
- **It does not remove the fallback.** Because most users will not install it, **periodic
  walking has to be good enough on its own**, and the agent has to be an optimisation rather
  than a requirement. If the product is unusable without it, the agent is a trap.

**Worth noting for the specific case at hand:** the inventory in
[`test-hardware.md`](test-hardware.md) has both NAS units running Proxmox and Debian, so the
server would run *on* the NAS and `inotify` works natively. The agent solves other people's
problem, not the author's — which is an argument for building it later and with real demand,
and an argument for making sure it is testable without a second machine.

## The no-agent path, in detail

[D27](../../decisions/working-notes.md) said periodic walking must be good enough alone, because
most installs will not run an agent. That is right, but it under-weighted the best mechanism —
which is not watching at all.

### Correction: the best mechanism is being *told*

Nothing that polls or watches beats the thing that wrote the file telling us it did.

**And for this audience that is the expected case, not an edge case.** The companion-app survey
found roughly 80,000 stars of `*arr` ecosystem
([plugins-and-workarounds.md](plugins-and-workarounds.md)), and **Sonarr and Radarr already
emit webhooks on "On File Import", "On File Upgrade" and "On Import Complete", with paths in
the payload.** A user running that stack has a component that knows precisely when a file
landed and where.

So the first thing to build is not a watcher. It is **an inbound endpoint** — a `POST` that says
"this path changed, look at it" — plus a CLI command and a UI button that hit the same path.
It is a few dozen lines, it is instant, it costs nothing to run, and for a large share of real
libraries it is the whole feature. Polling is then the fallback for manually-managed content
rather than the primary mechanism.

### The ladder, most to least preferred

| Tier | Mechanism | Works on | Latency |
| --- | --- | --- | --- |
| **A** | **Be told** — inbound webhook / API / CLI | Everything | Instant |
| B | Native watching (`inotify`, `FSEvents`, `kqueue`) | Local filesystems | Instant |
| C | **SMB `CHANGE_NOTIFY`** — protocol-level events | SMB shares | Instant — *but see below, this is not available today* |
| D | **Directory-mtime pruned walk** | Everything | Poll interval |
| E | Full `stat` walk | Everything | Rare — the safety net |
| F | Content hashing | Everything | On demand only |

Each tier catches what the tier above misses. D is the workhorse for network shares.

### Why tier D works unusually well here: media libraries are append-mostly

A directory's mtime changes when an entry is added or removed from it. It does **not** change
when a file is modified in place.

For most workloads that makes directory-mtime pruning unreliable. For a **media library** it is
close to ideal: files are added, removed or replaced wholesale, and almost never edited in
place. So an unchanged directory mtime is strong evidence that nothing happened in it, and the
walk collapses from *O(all files)* to *O(changed directories)*.

**Honest caveats, all of which need the tier-E net behind them:**

- `rsync -t`, `mv` and some copy tools **preserve** mtimes and can defeat the check.
- In-place modification of an existing file changes nothing at the directory level.
- Some SMB and NFS server configurations do not update directory mtimes dependably, and client
  caching can delay visibility.

### The cost model: round trips, not bytes

The reason a network walk is slow is **one network round trip per operation**, not data volume.
At 1 ms RTT, a naive walk that stats every file in a 100,000-file library is minutes of
latency; over Wi-Fi it is far worse.

Two things follow:

- **Never `stat` per file.** SMB2 and NFS return attributes in the directory listing, so a
  correctly written walk costs **one round trip per directory**, not per file. In Go that means
  `filepath.WalkDir` and being careful not to call `Info()` where the entry already carries what
  is needed — an easy and expensive mistake.
- **Slice the work.** Walk a portion per tick and resume, so the scanner never blocks for
  minutes and never presents as a stall.

### Adaptive cadence, because libraries are extremely skewed

A "recent downloads" directory changes daily; a 2004 films directory has not changed in years,
and never will. Tracking per-directory change frequency and checking hot directories often
while cold ones fall to a slow cycle is a large win for almost no code — and it beats any
global interval, because a global interval is either too slow for the hot corner or too
expensive for the cold mass.

The user should also be able to **declare a library archival or hot**, because an explicit
statement beats any heuristic and costs one config field.

### Probe at setup, then report honestly

Make D27's capability report concrete and measured, at the moment a library is added:

1. Write a temporary file into a test directory and check whether the parent's mtime moved —
   that is a **direct test of whether tier D is trustworthy on this share**.
2. Time a listing of a few hundred entries to get a per-directory round-trip cost, and from it
   an **estimated full-scan duration**.
3. Report the protocol, the chosen tier, the estimated deep-scan cost and the planned cadence.

And extend the reporting requirement: with polling, the user's real question is not "which
strategy" but **"when did you last actually look?"** So surface the **last successful deep scan
and the current staleness estimate**, per library.

### SMB `CHANGE_NOTIFY`: real, valuable, and not available off the shelf

SMB2 has a genuine change-notification primitive — `SMB2_CHANGE_NOTIFY`, with completion filters
and an optional watch-tree flag. It would give instant, event-driven detection on SMB shares
with no agent installed. NFS has no equivalent worth relying on: v3 has none, and v4.1
directory delegations are rarely implemented or enabled.

Two findings, checked 2026-09-18:

- **No Go SMB client implements it.** `CloudSoda/go-smb2` carries the `SMB2_CHANGE_NOTIFY`
  command constant and handles its `STATUS_NOTIFY_ENUM_DIR` error case, but there is **no
  request struct, no completion-filter constants and no public API** — a placeholder, not a
  feature. `hirochachacha/go-smb2` is the older and less active of the two.
- **Using an OS mount forecloses it anyway.** A `cifs` mount does not surface SMB change
  notifications to userspace, so the kernel path cannot expose this even in principle.

**The contained design, if it is ever worth doing:** keep the OS mount for reading media — it is
faster, gets the page cache and `sendfile` — and open a **separate, minimal SMB session used
only for `CHANGE_NOTIFY`** on the library roots. That needs connect, authenticate, tree-connect,
open-directory and change-notify, rather than a complete filesystem client. It is a real build
and a real differentiator (no incumbent does this), but it is not v1 work.

**Trigger to revisit:** if the tiered polling above proves inadequate in practice for SMB users,
*and* agent adoption is as low as expected.

## What this settles

1. **Change detection is a strategy interface with an explicit capability report**, not a
   mechanism. Never watch silently-not-working.
2. **Build the inbound endpoint first.** Being told beats every form of watching, and for this
   audience — `*arr` users with import webhooks already available — it is the expected path, not
   a fallback. A few dozen lines.
3. **Periodic walking must be good enough alone**, because it is what most installs will use.
   Its efficiency is a real requirement with a number in
   [`performance-targets.md`](../plans/performance-targets.md) — and the achievable technique is
   **directory-mtime pruning**, which suits media libraries specifically because they are
   append-mostly.
4. **Optimise round trips, not bytes**: one directory listing per directory, never a `stat` per
   file; slice the walk so it never blocks; and use adaptive per-directory cadence because
   libraries are extremely skewed between hot and cold corners.
5. **Probe the share at setup and report measured facts** — does this share update directory
   mtimes, and how long will a deep scan take? Then surface **last successful deep scan and
   staleness**, because that is the question a polling user actually has.
6. **The NAS index agent is the right answer for network shares** and should be built as a
   tier-3 capability service, later, with the index-file transport rather than a network
   protocol.
7. **SMB `CHANGE_NOTIFY` is a genuine differentiator and is deferred** — no Go client implements
   it and an OS mount cannot expose it, so it needs a purpose-built minimal SMB session
   alongside the mount.
8. **An explicit rescan trigger — button, API and CLI — always exists**, because every automatic
   mechanism above fails for somebody and the manual path is what makes that recoverable.
