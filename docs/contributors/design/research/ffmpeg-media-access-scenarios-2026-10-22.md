> **ABOUTME:** Every way FFmpeg's access to a film can go wrong, from the disk to the file to the
> process to the scheduler to security and operations, each judged under the four ways FFmpeg could
> get its bytes: the file itself (today), a pipe the node feeds (the hybrid D329 §7 chose), HTTP
> confined by Landlock, and a media program of our own in Rust. It ends with what the comparison
> says. It proposes and does not decide.

# FFmpeg's access to media: what can go wrong, 2026-10-22

**What prompted it.** The owner, after D329 §7 chose the hybrid pipe: *"Actually I'm starting to
worry about the decided approach for ffmpeg. Let's build scenarios to describe all the things that
can go wrong, and then gauge our approach."*

**The four approaches.**
- **File:** FFmpeg opens the file itself, as today. Its reads go straight to the kernel.
- **Pipe (hybrid):** the node reads the file through its disk scheduler and feeds FFmpeg a pipe,
  for runs that read straight through a container that streams. Other runs read the file itself.
  FFmpeg opens no socket. This is what D329 §7 decided.
- **HTTP + Landlock:** FFmpeg reads from the node over loopback HTTP, asking for byte ranges, on
  kernels whose Landlock can restrict it to the node's one port. On older kernels it reads the file.
- **Own program (Rust):** a program of ours, built on FFmpeg's libraries. It asks the node for
  bytes by offset over an inherited pipe pair, with no socket and no file access.

The tests behind the facts here are in
[ffmpeg-http-input-2026-10-22](ffmpeg-http-input-2026-10-22.md).

**How each scenario is judged:**
- **✓** handled well;
- **~** partly handled, or handled with a cost;
- **✗** the failure happens.

## A. The storage under the file

| # | What goes wrong | File | Pipe (hybrid) | HTTP + Landlock | Own program |
| --- | --- | --- | --- | --- | --- |
| A1 | **A share drops mid-read, soft NFS mount:** reads fail with EIO after the mount's timeouts | ~ FFmpeg logs it and exits 0 with a partial result; caught since f65cac8d by its log line | ✓ for piped runs: the node's read fails and the node knows. Seeking runs: as File | ✓ the node's read fails, FFmpeg errors, and the node knows the file was not served | ✓ as HTTP |
| A2 | **A share drops, hard mount or virtiofs:** the read never returns, and the process waits in the kernel and outlives SIGKILL (D233 §10, the NFS research) | ✗ FFmpeg is stuck and unkillable, holding its slot and memory until the share returns | ✓ for piped runs: the node's thread waits, and FFmpeg waits on a pipe and can be killed. **✗ for seeking runs: chunk runs and probes are stuck as today** | ✓ only the node's thread waits | ✓ as HTTP |
| A3 | **The share comes back ~48 s later** | ~ stuck runs resume or fail at random, and work the scheduler gave up on carries on | ✓ the node decides what reads first; seeking runs as File | ✓ | ✓ |
| A4 | **A sleeping disk takes 6–30 s to spin up** | ~ FFmpeg waits; the scheduler can't tell a wait from a stall | ✓ the node knows it woke the disk (the wake log, "Waking the drive…") | ✓ | ✓ |
| A5 | **A slow or loaded disk** (USB 2, a RAID scrub) | ✗ FFmpeg's reads compete unseen with playback | ✓ paced, playback first; seeking runs unseen | ✓ every read paced | ✓ |
| A6 | **The file is replaced or renamed mid-read** (a better copy dropped in; ESTALE over NFS) | ~ FFmpeg keeps reading the old inode, or fails | ~ the same, through the node, which can check identity between pieces | ~ as Pipe; the byte path checks the payload hash per request (byteserver) | ~ as HTTP |
| A7 | **The file is still being written** (a download) | ✗ FFmpeg reads a short file and answers as if complete | ~ the node can check size and mtime before and after | ~ as Pipe | ~ as Pipe |
| A8 | **A bad sector:** EIO on one range of a local disk | ~ partial result; the log is caught | ✓ the node knows which range failed; it can't skip it, since FFmpeg reads in order | ✓ known by range; the run fails | ✓ could fail just that range |
| A9 | **Permissions change mid-read** | ✗ fails at open or mid-read, as FFmpeg's error | ✓ the node's own error, before FFmpeg | ✓ | ✓ |

## B. The file's content

| # | What goes wrong | File | Pipe (hybrid) | HTTP + Landlock | Own program |
| --- | --- | --- | --- | --- | --- |
| B1 | **Damaged frames** FFmpeg conceals | ✓ plays and encodes with a glitch | ✓ the same | ✓ | ✓ |
| B2 | **A damaged container** ("Invalid data found", the demuxer resyncs) | ✓ | ~ the same, unless resyncing needs a seek: the pipe can't, so the result may differ from File's | ✓ as File | ✓ |
| B3 | **A truncated file** (EOF early) | ~ exits 0, shorter; read-through and duration checks may catch it | ~ the same; the node knows it served the whole file, so a short result means a short file | ~ as Pipe | ~ as Pipe |
| B4 | **An MP4 with its index at the end** | ✓ | ~ read as File, so its reads are unseen and A2 applies | ✓ | ✓ |
| B5 | **A Matroska file whose cues or seek head sit at the end** | ✓ | ✓ for read-throughs, which read in order (tested); a probe wanting the cues reads as File | ✓ | ✓ |
| B6 | **A container that isn't Matroska, TS or MP4** (AVI, old formats) | ✓ | ~ read as File: unseen, and A2 applies | ✓ | ✓ for formats our program handles |
| B7 | **A huge file** (80 GB 4K remux) | ✓ | ✓ paced; a pipe copy costs a little CPU | ~ ~1.6 CPU-s per GB more | ✓ |
| B8 | **A file needing a long probe** (big `probesize` or `analyzeduration`) | ✓ | ✓ FFmpeg buffers what it probed | ✓ | ✓ |

## C. The FFmpeg process

| # | What goes wrong | File | Pipe (hybrid) | HTTP + Landlock | Own program |
| --- | --- | --- | --- | --- | --- |
| C1 | **It crashes** (segfault on a hostile or odd file) | ✓ the run fails, the node goes on | ✓; the node stops feeding | ✓ | ~ our program crashes instead, still a separate process |
| C2 | **It hangs in a decoder loop**, CPU busy | ✓ the watchdog's no-progress window kills it | ✓ the same | ✓ | ✓ |
| C3 | **It waits for input forever** | ✗ in the kernel, unkillable (A2) | ✓ on a pipe: killable, and the node knows why it waits | ✓ on a socket: killable | ✓ |
| C4 | **It exits 0 with a partial result** | ~ caught by its log line since an earlier commit | ✓ the node knows whether it fed the whole file | ✓ the node knows whether it served the whole file | ✓ the program reports what it read |
| C5 | **The node restarts mid-run** | ~ FFmpeg is orphaned until killed, maybe stuck | ✓ the pipe closes, and FFmpeg errors and exits | ✓ the socket closes | ✓ |
| C6 | **Too little memory** (OOM kill) | ✓ the run fails | ~ a large pipe buffer adds a little, bounded by the pipe | ✓ | ✓ |
| C7 | **A run killed mid-write leaves a partial output** | ✓ temporary name, then rename (W13) | ✓ the same | ✓ | ✓ |

## D. Working with the scheduler

| # | What goes wrong | File | Pipe (hybrid) | HTTP + Landlock | Own program |
| --- | --- | --- | --- | --- | --- |
| D1 | **A long read holds the disk for minutes** (the 2026-10-21 cause) | ✗ nothing can interrupt it but SIGSTOP | ✓ for straight-through runs: the node stops feeding between pieces | ✓ the node holds back answers | ✓ |
| D2 | **A chunk run reads unseen while another playback reads the same disk** | ✗ | **✗ chunk runs seek, so they read as File** | ✓ | ✓ |
| D3 | **FFmpeg decodes slower than the node feeds** | — | ~ the pipe fills and the node's feeding waits; it must not hold the disk's place while it waits (release between pieces) | ~ the same with the socket buffer | ✓ the program asks only when ready |
| D4 | **A run paused for hours** (interest gone, waiting its turn) | ✗ SIGSTOP; memory held | ~ FFmpeg alive and idle on a pipe, memory held; or stopped and restarted from saved state | ~ as Pipe | ✓ the program can save and stop |
| D5 | **One read for two needs** (playback and read-through) | ✗ | ~ only for a direct play of a straight-through container: the node tees its reads | ✓ any run | ✓ |
| D6 | **The playback seeks away while a read-through shares its read** | — | ~ the read-through keeps a coverage map and fills the gap later | ~ as Pipe | ~ as Pipe |
| D7 | **A probe on a dead share** (examining a new file) | ✗ stuck in the kernel | ✗ probes read as File | ✓ | ✓ |

## E. Security (a crafted file takes over the process)

| # | What it can reach | File | Pipe (hybrid) | HTTP + Landlock | Own program |
| --- | --- | --- | --- | --- | --- |
| E1 | **Files:** Landlock grants read of the one file today | the one file | the piped run reads **no media file at all**, only the bytes fed; seeking runs, the one file | **no media file** | no media file |
| E2 | **The network and local sockets** (N1) | none | none | **one TCP port, on any address** (Landlock can't restrict addresses); the node's byte path checks a per-run token. On kernels before 6.7, falls back to File | none |
| E3 | **Code we wrote that parses hostile bytes** | FFmpeg only | FFmpeg; the node only copies bytes | FFmpeg; the node serves bytes | **our program also parses hostile media through the libraries.** Rust keeps our own part memory-safe; the libraries are FFmpeg's C either way |

## F. Correctness and operations

| # | Concern | File | Pipe (hybrid) | HTTP + Landlock | Own program |
| --- | --- | --- | --- | --- | --- |
| F1 | **Output identical to today** | ✓ | ✓ for the runs it pipes (tested), and those it doesn't read as File | ✓ every run (tested) | needs its own tests against the CLI |
| F2 | **Two ways for every kind of run, to test and keep working** | one | **two**: piped and file, by container | two: HTTP, and File on old kernels | one, after the change |
| F3 | **Running a failed run again by hand to debug it** | ✓ the command line | ~ the command needs its input fed; the node can log a command that reads the file instead | ~ needs the node running | ~ our program's own flags |
| F4 | **Building and shipping** | ✓ the pinned jellyfin-ffmpeg binary | ✓ the same | ✓ the same | ✗ we build FFmpeg's libraries for x86 and arm64 with every hardware encoder (VAAPI, QSV, NVENC), or link against a distribution's; a new build pipeline, and new licence review |
| F5 | **Other operating systems** (a node on macOS or Windows, later) | ✓ | ✓ pipes work everywhere | ~ no Landlock, so File there | ✓ |
| F6 | **Hardware encoding** | ✓ | ✓ unaffected; only input changes | ✓ | ~ we drive the hardware encoders ourselves through the libraries |
| F7 | **Cost to build for the closed beta** | none | small: a feeder, the container check, two paths | medium: the byte path, the Landlock network rule, the seccomp change, the token | large |

## What the comparison says

1. **The hybrid fixes what broke on 2026-10-21 and leaves one class of failure in place.** The long
   background reads (read-through, extraction, whole prepares) become paced, interruptible and
   killable: D1, A2 for those runs, C3. But **chunk runs and probes still read the file itself**, so:
   - a dead share can still leave them stuck in the kernel (A2, D7);
   - the scheduler can't see or pace them (D2).

   Chunk runs are the playback path. When a share dies mid-chunk, an FFmpeg stuck in the kernel
   holds an encoder slot until the share returns. That is exactly the case the redesign set out to
   handle, now confined to playback and probes.
2. **The two paths double the testing** (F2) and leave a second, poorer class of run, chosen by the
   file's container: an MP4 with its index at the end, an AVI (B4, B6).
3. **HTTP + Landlock handles every scenario in A to D**, with one path on new kernels. It costs:
   - N1 narrowed to one TCP port, whatever the address (E2);
   - File again on kernels before 6.7;
   - ~1.6 CPU-s per GB more.
4. **Our own program handles everything, with the strongest isolation.** It is the biggest build
   (F4, F6), and its risk is in building FFmpeg's libraries with hardware encoders, not in the Rust.
5. **Unchanged under every approach:** damaged content (B1–B3), a file changing under a read (A6,
   A7), and crashes (C1). They need the node's own checks (identity and size before and after), not
   a different read path.

**Two ways to narrow the hybrid's gap, each to test before trusting:**
- **Chunk runs from a pipe.** A chunk run seeks once, to a keyframe the node already knows from the
  file's index (D259), then reads straight through its span. For Matroska, the node could feed
  the header and then the clusters from that keyframe's offset. If FFmpeg demuxes that correctly,
  chunk runs join the piped class. Not possible for MP4.
  - *A first try, the same day, was inconclusive.* The header was fed, with the segment's size made
    "unknown" as a live stream's is, then the bytes from the last Cluster marker found by a byte
    search before the 2:30 keyframe.
  - FFmpeg read about 450 KB and then met elements it took for invalid data, and all three chunk
    shapes (copy, encode, sound) differed from reading the file.
  - A byte search can match a marker inside the media data, so the real test feeds from the cluster
    position the file's cues give (`internal/node/container` reads cues). Until then this is
    neither confirmed nor ruled out.
- **Probes from the head of the file.** A probe of a straight-through container needs only its
  first megabytes (3.5 MB in the test). The node could feed those through the scheduler and close
  the pipe. That is untested for every file kind, and a probe that wants the cues would see less.

## Questions for the owner

1. **Is the hybrid's remaining gap acceptable for the closed beta?** Chunk runs and probes stuck on
   a dead share, and chunk runs the scheduler can't pace.
2. **Or is narrowing N1 to one port, on kernels 6.7 and later, the lesser risk** than that gap?
3. **Should the two ideas for narrowing the gap be tested first** (chunk runs and probes from a
   pipe)? Each is a short test, like the earlier ones.
