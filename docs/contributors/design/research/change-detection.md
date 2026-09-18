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

## What this settles

1. **Change detection is a strategy interface with an explicit capability report**, not a
   mechanism. Never watch silently-not-working.
2. **Periodic walking must be good enough alone**, because it is what most installs will use.
   That makes its efficiency a real requirement, not a fallback afterthought — and
   [`performance-targets.md`](../plans/performance-targets.md) should carry a number for it.
3. **The NAS index agent is the right answer for network shares** and should be built as a
   tier-3 capability service, later, with the index-file transport rather than a network
   protocol.
4. **An explicit rescan trigger — button and API — always exists**, because every automatic
   mechanism above fails for somebody and the manual path is what makes that recoverable.
