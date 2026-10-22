> **ABOUTME:** Two independent adversarial audits of option C, an LD_PRELOAD interposer that has the
> node serve every FFmpeg read, wargamed by experiment. It covers every way found to wedge, desync or
> leak the channel between FFmpeg and the node, and what happens then; whether the interception is
> complete under the real confinement; and how C compares with the other approaches on each class of
> failure. It ends with the rules an implementation must follow. It proposes and does not decide.

# Option C wargamed, 2026-10-22

**What prompted it.** The owner, on
[own-media-binary-2026-10-22](own-media-binary-2026-10-22.md)'s recommendation of option C: *"Do an
independent audit of the options, with wargaming. I want to know if this really handles all failure
modes. For example is there a way to wedge the pipe that we haven't thought of? And what happens
then?"*

**How it was done.** Two auditors, each told to break C by experiment, worked independently:

1. **The channel and the node's side.** They built:
   - a shim with switches for its variants;
   - a stand-in node that could be slow, dead, dying, partial or erroring;
   - a Landlock-and-seccomp runner.

   Their rig is in `.git/spikes/shimwg/`, outside the tree.
2. **The interception itself.** They built:
   - a fuller shim;
   - a validating stand-in node;
   - a hostile client.

   They ran every run shape through **the node's real confinement** (`orpheum __confined-exec`,
   built from HEAD). Their rig is in the session scratchpad. They installed strace, ltrace and gdb in
   the sandbox for it.

Both used the pinned jellyfin-ffmpeg 8.1.2-5 and synthetic media.

## The verdict

**Option C holds, under rules; the 32-line prototype does not.** Neither auditor found a wedge that
leaves FFmpeg unkillable or blocks the node forever, provided the node and the shim follow the rules
in the last section. Under the real confinement:
- the shim saw every read of the input in every run shape the node uses;
- the outputs were byte-identical to direct reads;
- Landlock granted no media file at all.

**The systemic danger is not a hang. It is that every failure of the channel becomes a quiet partial
success:** FFmpeg logs `Error during demuxing: Input/output error` and exits 0, and ffprobe exits 0
with a plausible answer. The node, which sees every answer it gives, must judge each run's success
itself.

## Ways to wedge or break the channel, and what happens

| # | How | What happened, or the argued result | Consequence | Rule that closes it |
| --- | --- | --- | --- | --- |
| W1 | **The channel breaks**: the node answers an error, dies mid-request, or closes between answers | Tested: FFmpeg exited 0 after 7.5 s of a 300 s film; ffprobe exited 0 with the full stream list | Truncated chunks, read-throughs and subtitles stored as complete | The node fails the run on any error, break or refusal it served, whatever FFmpeg's exit status. Closes fully |
| W2 | **Two inputs read at once** (two `-i`, or a sidecar and the video): FFmpeg 8 runs a demuxer thread per input | Tested, without a channel lock: one file's bytes delivered as the other's, then exit 0 | Corrupt output that can pass silently | One channel per opened file, or one lock held for the whole exchange; answers echo sequence, file and offset, and any mismatch shuts the channel for good. Closes fully |
| W3 | **The shim gives up waiting on an answer** and the late answer is taken as the next one's | Tested: 559 of 35,413 frames, exit 0 | Desync, silently wrong | Never abandon a request. Keep it pending and return `EAGAIN` (FFmpeg retries the same read), or shut the channel. Closes fully |
| W4 | **The node holds a read** (pacing, or a dead share under it) **while the node's watchdog stops the run** | Tested: FFmpeg, waiting on a pipe, ignores SIGINT and SIGTERM (its handlers use `SA_RESTART`, and file reads never check its interrupt callback); SIGKILL ends it at once | The INT→TERM→KILL stop collapses to KILL, and an HLS or segment trailer is lost | The node answers the held read with EIO before any signal, and FFmpeg then ends cleanly (tested). The watchdog's stall clock pauses while the node holds a request. Closes |
| W5 | **The node's read is stuck on a dead NFS share** | Argued (an uninterruptible NFS read can't be faked here): the node thread is pinned; if the node times it out and frees its place, the next read sticks too, and threads grow until Go's 10,000-thread limit crashes the node; a reused buffer gets written by the late read | A node crash, or corrupted answers | A read past its time keeps its device place and its own buffer until it returns. Stuck threads are bounded by the device's limit, and listed as stuck. Closes, at one thread per stuck place |
| W6 | **FFmpeg is paused (SIGSTOP) mid-answer** | Tested: the node's write blocks; SIGCONT carries on correctly; SIGKILL gives the node a broken pipe | If the node holds its disk place while writing, a paused background run holds the disk and a playback waits (priority inversion) | The node releases its place as soon as its `pread` returns, before writing. Answers are no larger than the pipe's capacity, and no write deadline applies while the run is paused. Closes fully |
| W7 | **One scheduled read per FFmpeg read**: FFmpeg reads 32 KiB at a time, so a 6 s chunk made 480 requests | Simulated: with 40 ms per answer, the chunk took 19.5 s; with 2 MiB of read-ahead, 0.9 s, the output identical | Playback stalls under background load | Read ahead per file, 1–4 MiB, dropped on a seek outside it; consider keeping one disk place for playback. Largely closes; tune on the bench |
| W8 | **A hostile file asks for a huge read**: one `read()` per packet, up to ~2 GiB from the file's own index | Tested: FFmpeg asked for 24.8 MB in one read; capping requests at 1 MiB, and answering only 64 KiB, left the output identical | The node allocates gigabytes | The shim caps each request; the node caps each answer and never allocates from the requested length. Closes fully |
| W9 | **A compromised FFmpeg pipelines requests**: 1,000 × 4 MiB without taking answers | Tested: the stand-in node blocked until its own timeout | The node's goroutine is stuck | One request in flight; a second before the first is drained is a violation, and the run is killed. Closes |
| W10 | **A compromised FFmpeg sends garbage**: a length over the cap, a negative offset, an unknown op or handle | Tested under the real confinement: each one killed the run | — | The node validates every frame. Closes |
| W11 | **Hostile input makes FFmpeg reopen the magic path** (an ffconcat whose entries name it) | Tested: the concat demuxer recursed; a cap of 4 opens ended it | Unbounded opens | A table of handles in the shim, a cap on opens in the node. Closes |
| W12 | **FFmpeg's threads race the shim's table**: a descriptor closed and its number reused, or a second open overwriting the first | Tested with the prototype: input 0 read `/dev/null` after the second open; 8 of 7,200 frames, exit 0 | Silent truncation | Handles at reserved, never-opened descriptor numbers (900+), so every call the shim misses fails with `EBADF` and the kernel never reuses the number (tested: a missed `fstat` fell back to `lseek` correctly) |
| W13 | **ffprobe meets a closed channel**: ffprobe doesn't ignore SIGPIPE, as ffmpeg does | Tested: ffprobe killed by SIGPIPE | A probe dies unexplained | The shim ignores SIGPIPE. Closes |
| W14 | **A forked child inherits the pipes** (a GPU driver can run a helper); the node then never sees end-of-file | Argued: neither binary imports fork or exec | A leaked goroutine per run | Close-on-exec on the channel in the shim's constructor; the node detects FFmpeg's end by its exit, not by end-of-file. Closes |
| W15 | **The node deadlocks on itself**: one goroutine answering requests and draining stdout | Argued | A run hangs | Separate goroutines per stream. Closes |
| W16 | **One input stuck on a dead share blocks the other** on a shared channel | Argued | A sidecar stalls the video | One channel per file. Closes |
| W17 | **A descriptor clash**: the prototype served from fd 3, which is execx's `-progress` pipe | Found by reading | Progress and data cross | Fixed, higher descriptor numbers. Closes |

## Is the interception complete?

**For the pinned amd64 binary, yes.**
- A logging preload over every run shape (probe, read-through, subtitles, chunk copy, x264, sound
  and HLS) saw the input opened **exactly once**, by `open64`.
- Under the shim, strace saw only `fcntl` and `close` reach the kernel on the placeholder.
- `sendfile`, `syscall()`, `mmap` of the file, `pread64`, `access`, `realpath` and `glob` are
  imported (by statically linked libraries) but were never called on the input.

**Not yet tested:**
- QSV and NVENC (no GPU here);
- arm64, beyond its imports, which match amd64's names;
- `fopen`, including libass's in a subtitle burn-in. It goes through glibc's internal `open`, which a
  preload can't reach. It fails loudly with reserved descriptors.

**Confinement is a blocker, but a cheap one.** `confine.Shim` execs the tool with an empty
environment, so LD_PRELOAD is dropped.
- A `Preload` field in `confine.Spec` closes it, with the empty-environment test narrowed to allow
  that one variable.
- The `.so` must ship in the image beside the tools, granted read by Landlock, and be built against
  glibc 2.28, as jellyfin's toolchain is. Built on this machine's glibc 2.41, it wouldn't load on
  glibc 2.28–2.33 hosts.
- Inherited descriptors survive the exec, and seccomp blocks nothing the shim calls.

**Over time:**
- A newer glibc renames the stat calls (`fstat64`, `stat64`, `statx`), so the shim covers both
  sets.
- **A fully static or musl jellyfin build ends option C, loudly**: every run fails with ENOENT on the
  magic path. A gate at each pin bump catches it.
- macOS and Windows nodes have no preload, and would read files directly.

**Cost:** about 0.9 CPU-seconds per GB, against HTTP's 1.6.

## Compared on each class of failure

✓ handled · ~ partly, or at a cost · ✗ fails

| Failure class | Direct (today) | Hybrid pipe | HTTP + Landlock | B: `orpheumfd:` | C: LD_PRELOAD |
| --- | --- | --- | --- | --- | --- |
| Dead share: read-through, extraction, prepare | ✗ stuck in the kernel | ✓ | ✓ | ✓ | ✓ (tested: killable) |
| Dead share: chunk runs, probes | ✗ | ✗ | ✓ | ✓ | ✓ |
| Every read paced or shared | ✗ | ~ straight-through runs only | ✓ | ✓ | ✓ |
| MP4 with its index at the end, AVI | ✓ unseen | ~ read as a file | ✓ | ✓ | ✓ (tested) |
| Output identical to today | — | ✓ | ✓ | needs tests | ✓ (tested, every shape) |
| Graceful stop while the node holds a read | ✓ | ✗ unless EIO first | ✓ | ✗ unless it polls the interrupt | ✗ unless EIO first |
| A compromised FFmpeg: files it can reach | its one film | none (piped runs) | none | none | none |
| A compromised FFmpeg: sockets | none | none | one TCP port | none | none |
| A static or musl jellyfin build | ✓ | ✓ | ✓ | ✓ (our own build) | ✗ (loud) |
| macOS and Windows nodes | ✓ | ✓ | ~ falls back to a file | ✓ if built | ✗ falls back to a file |
| Cost to build and run | none | small | medium | high: a jellyfin fork, ~2 h per architecture | small |

**C is worse than today in two places:**
- the graceful stop, closed by answering EIO first;
- its dependence on how jellyfin links, caught at the pin bump, with B as the way out.

**It is better than every other route, B aside, on everything else, and safer than today:** no media
file in Landlock's grant, reads the node can pace, and no run stuck in the kernel.

## The rules an implementation must follow

**The channel:**
- One channel (pipe pair) per opened file, at fixed descriptors above the ones execx uses, set
  close-on-exec in the shim's constructor.
- A fixed-size request: magic, version, sequence, op, handle, offset, length.
- A fixed-size answer echoing sequence, handle and offset, a status, and exactly the data it says.
- Full reads and writes, looping on short ones and EINTR.
- **One request in flight. A request is never abandoned:** wait, keep it pending (`EAGAIN`), or shut
  the channel for good. Any mismatch, early end, or broken frame shuts it, and every later call
  fails with EIO. Never resynchronise.
- Requests capped at 1 MiB in the shim, answers capped and no larger than the pipe's capacity in the
  node, short reads served. The node never allocates from a requested length.

**The shim:**
- An exact magic path that keeps its extension.
- Handles at reserved, never-opened descriptor numbers.
- Coverage of every open, read, `pread`/`preadv`, seek, `fstat`/`stat`/`statx` and access variant, in
  both glibc's old and new names.
- `fcntl` handled; `dup`, `fdopen` and `mmap` of a handle refused.
- SIGPIPE ignored.
- A strict mode for tests that aborts on anything unhandled.
- Built against glibc 2.28 for each architecture, shipped in the image, and loaded through a preload
  field in `confine.Spec`.

**The node:**
- **It owns every timeout and judges every run**: a run fails on any error, break or refusal it
  served, or when no open ever arrived, whatever FFmpeg's exit status. `sawDemuxingFail` stays as a
  second check.
- It validates every frame: op, handle, offset ≥ 0, length cap, opens cap (about 4), one in flight.
  A violation kills the run.
- It reads ahead 1–4 MiB per file through the disk scheduler, and releases its place as soon as its
  read returns.
- A read past its time keeps its place and its buffer. Stuck reads are listed, as stuck processes
  are today.
- Before any signal it answers a held read with EIO, and its watchdog's stall clock pauses while it
  holds a request.
- It reports the run's movement and its "waiting on the disk" state to execx, since FFmpeg's own
  read counters now stay at zero.
- It ends runs with SIGKILL of the process group, and detects their end by their exit, never by
  end-of-file.
- It drains each of FFmpeg's streams on its own goroutine.

**The gates, at every pin bump:**
- the imports of both binaries on both architectures, checked against a committed list the shim
  covers, and that they still link glibc dynamically;
- every run shape through the shim in strict mode, compared byte for byte with direct reads
  (encodes at one thread, or without VBV, which varies between direct runs too);
- a test that a shim which fails to load fails the run;
- and at the node's start, a self-check through the shim on a small generated file, reported as
  confinement's mechanisms are (D112).
