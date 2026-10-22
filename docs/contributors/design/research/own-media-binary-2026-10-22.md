> **ABOUTME:** How much work it would be to have FFmpeg's reads served by the node, with no socket
> and no file access, by four routes: a Rust program on FFmpeg's libraries, a patched FFmpeg with a
> protocol of our own, an LD_PRELOAD interposer under the stock binary, and FFmpeg's built-in
> protocols. It covers sizes, build and CI, maintenance, risks and time, and a prototype that already
> works. It proposes and does not decide.

# Our own media binary: what it would take, 2026-10-22

**What prompted it.** The owner, weighing D329 §7's hybrid pipe against the failures it leaves
([ffmpeg-media-access-scenarios-2026-10-22](ffmpeg-media-access-scenarios-2026-10-22.md)): *"First,
investigate how much work it actually would be to write our own binary."*

**The requirement.** The node serves every read FFmpeg makes, seeks included, by byte offset, and
FFmpeg opens no socket and no media file. Today FFmpeg is confined by Landlock and by a seccomp filter
that forbids `socket(2)` (N1). The node's runs:
- the probe;
- the read-through: an `ebur128`/`pan` filter graph and a `framecrc` timeline;
- subtitle extraction to ASS and WebVTT;
- chunk runs: seek, copy or encode (software, QSV, NVENC), the segment muxer, forced keyframes, AAC;
- whole prepares to fMP4 HLS;
- WebP artwork;
- encoder detection.

**How it was done.**
- A research agent read the sources: FFmpeg n8.0 and jellyfin-ffmpeg at 8.1.3-2 (cloned to
  `/yoloai/cache/git/`), rsmpeg, rust-ffmpeg, rust-ffmpeg-sys and ez-ffmpeg, plus crates.io and
  GitHub's API.
- It ran the pinned 8.1.2-5 binaries against test files and built a prototype interposer.
- Pages are cached in `/yoloai/cache/http/own-binary/`.
- `jf:` below means a path in the jellyfin-ffmpeg tree.

## The fact that decides option C: the pinned binary is dynamically linked

The pinned `ffmpeg` and `ffprobe` are dynamically linked, position-independent executables:
- Every dependency is linked statically, except glibc, libm, libpthread, libdl and libmvec
  (`jf:builder/images/base-linux64/Dockerfile:15-35`; arm64 alike, `base-linuxarm64/Dockerfile:15-29`).
- Its toolchain is glibc 2.28 (`base-linux64/ct-ng-config:520`). So its imports include `open`,
  `open64`, `openat`, `read`, `pread64`, `lseek`, `lseek64`, `__fxstat64` and `__xstat64` (from
  `nm -D`).
- A preloaded library can stand in for them.

## The four routes

### D. FFmpeg's built-in protocols

No built-in protocol meets the whole requirement:

| Protocol | Seekable? | Verdict |
| --- | --- | --- |
| `fd:` (FFmpeg 6.0 and later, `-fd N -i fd:`) | Yes, for a regular file's fd (`jf:libavformat/file.c:506-508`) | No path or socket is needed, and the node can open the file and pass the fd: Landlock checks only opens. But the kernel serves the reads, not the node, so a dead share still leaves FFmpeg waiting in the kernel |
| `pipe:` | Never | Tested: an MP4 with its index at the end fails, and a seek on a pipe fails |
| `cache:` | Forward only by reading through, and needs write to `/tmp` (`jf:libavformat/cache.c:261-283`; `jf:libavutil/file_open.c:141`) | Unsuitable |
| `subfile:`, `concat:`, `async:` | Only as seekable as the URL inside them | They add nothing |
| `unix:` | Not seekable, and it opens a socket | Forbidden |

Protocols are fixed when FFmpeg is configured (`jf:configure:4526,8823`). A new one means rebuilding.

### C. An LD_PRELOAD interposer under the stock binary

**A prototype works.** It is 32 lines of C. It:
- intercepts `open`/`open64` of `/orpheum/input*`;
- serves `read`, `lseek` and `__fxstat64` from an inherited descriptor, standing in for the node;
- lets everything else through untouched.

Against the pinned 8.1.2-5 binaries:
- **an MP4 with its index at the end** probed correctly, with 3 reads, 3 seeks and 7 fstats served;
- **a seek to 0:15 in a Matroska file** gave the correct frames, with 34 reads, 5 seeks and 5
  fstats served;
- **without the interposer**, the same path does not exist.

**Why it works.** FFmpeg's file protocol opens through `open()` (`jf:libavutil/file_open.c:79-85`),
reads with `read()` (`file.c:146`), seeks with `lseek()` (`file.c:242`) and sizes with `fstat()`
(`file.c:236-239`), all dynamic imports. **Precedent:** libnfs's `ld_nfs.so` serves `nfs://` paths
the same way.

**Pitfalls, each testable:**
- The stat functions' names follow the build's glibc: `__fxstat64` on 2.28, `fstat64` or `statx`
  on 2.33 and later. Cover both, and check the binary's import list whenever the pin changes.
- arm64's imports were not checked.
- A fully static build would end the route. jellyfin's portable build links glibc dynamically,
  but nothing promises it stays so.
- FFmpeg 8 demuxes on its own thread, so the interposer keeps each descriptor's offset under a
  lock.
- It must also cover `openat`, `pread64` and `dup`/`fcntl` on the fake descriptor. `mmap` isn't
  used for demuxer input.
- Its placeholder descriptor must be one Landlock allows (an inherited one, or `memfd_create`), and
  every path not its own must pass through untouched: the GPU drivers open `/dev/dri` the same way.
- It works below FFmpeg's own layers, so an FFmpeg change (`file.c` moving to `pread`) would pass
  unseen without a test per pinned version.
- The prototype was run unconfined. Running it under Landlock and seccomp is untested.

**In numbers:**
- **Code:** 300–600 lines of C, or a Rust cdylib, plus the node's side (requests and answers over
  two pipes).
- **Build:** a few-kilobyte library per architecture. The pinned FFmpeg is unchanged.
- **Maintenance:** an import-list check and the integration tier at each pin bump.
- **First robust version:** 2–4 days.
- **Risk:** medium, tied to how jellyfin links.

### B. A patched FFmpeg with an `orpheumfd:` protocol

**Adding a protocol is small:**
- one line in `protocols.c`;
- one line in `libavformat/Makefile`;
- a `URLProtocol` with open, read, seek (with `AVSEEK_SIZE`) and close (`jf:libavformat/url.h:52-96`).

`subfile.c`, a whole protocol, is 157 lines, so ours would be about 300 lines of C. The CLI is
unchanged: `-i path` becomes `-i orpheumfd:`.

**The cost is the build:**
- jellyfin's portable build makes a crosstool-ng glibc-2.28 toolchain and about 50 dependency
  images (VAAPI, libvpl, ffnvcodec, x264, x265 and the rest).
- It applies `debian/patches/series` with quilt (`jf:builder/build.sh:73-77`). Our patch is one
  file and one line there, beside 101 others.
- **Cold, its release took 113 min (amd64) and 130 min (arm64) on GitHub's runners.** Warm rebuilds
  should be faster; not measured.
- The sandbox has no CI, so it needs the owner's CI or a VPS, plus an artifact host and a cache.
- `scripts/fetch_ffmpeg.py`, a gate, would pin our artifacts instead of jellyfin's.
- jellyfin releases every two to three weeks, and each pin bump means rebasing onto their tag.

**In numbers:**
- **Code:** ~300 lines of C.
- **First working version:** 3–5 days, most of it getting the builder running.
- **Risk:** low in the code, moderate in running a build of our own.

### A. A Rust program on FFmpeg's libraries

**Bindings:**
- **ffmpeg-next / ffmpeg-sys-next:**
  - maintained, with FFmpeg 8.x detected;
  - an AVIO over Rust's `Read + Seek`;
  - filter graphs;
  - **no hardware contexts** (raw FFI for those);
  - "maintenance-only".
- **rsmpeg:** has custom AVIO and hardware contexts, but targets 8.0, and was last pushed 14 months
  ago.
- **ez-ffmpeg:** custom input, FFmpeg 7.1–8.x. It ports the CLI's pipeline to Rust, and **its core
  is about 94,000 lines** while still missing features. That is the measure of rewriting the CLI.

**What a program would replace.** The CLI's logic is in fftools, not in a library: `ffmpeg_filter.c`
3,458 lines, `ffmpeg_mux_init.c` 3,523, `ffmpeg_sched.c` 2,776, `ffmpeg_demux.c` 2,224, `ffprobe.c`
3,411. jellyfin also patches fftools for hardware formats (patches 0067, 0068), and a program of ours
would reproduce those too.

**Estimated in Rust, for Orpheum's runs:**

| Run | Lines |
| --- | --- |
| Probe | 400–800 |
| Read-through | 600–1,000 |
| Subtitles | 400–700 |
| Chunk runs | 2,000–3,000 |
| Hardware encoding | 2,000–4,000 |
| HLS prepares | 1,500–3,000 |
| Artwork | 200–400 |
| Encoder detection | 200–400 |
| **Total** | **7,000–13,000**, with a long tail of timestamp and seeking cases the CLI handles today |

**It still needs FFmpeg's libraries built.** jellyfin publishes no shared build with headers, so the
program carries B's whole build pipeline, on top of the rewrite.

**In numbers:**
- **First working version:** 2–4 months to parity.
- **Maintenance:** high: each FFmpeg release changes behaviour it must track.
- **Risk:** high, highest in hardware encoding.

## Summary

| Route | Code | Build and CI | Maintenance | Risk | First working |
| --- | --- | --- | --- | --- | --- |
| A. Rust on the libraries | 7k–13k lines | B's pipeline, plus Rust | High | High | 2–4 months |
| B. `orpheumfd:` protocol | ~300 lines of C | A jellyfin fork, ~2 h per architecture cold, our own artifacts | A rebase per pin bump | Low code, moderate operations | 3–5 days |
| C. LD_PRELOAD | 300–600 lines | A tiny library | An import check per pin bump | Medium | 2–4 days; a prototype works |
| D. Built-ins | 0 | None | None | — | Fails the requirement: the kernel, not the node, serves reads |

**What it means.** The problem is FFmpeg's input, not its processing. A program of our own would
rewrite the processing to change the input, at roughly a hundred times the code of the routes that
change only the input:
- **C** changes it under the stock binary, and works today in a prototype.
- **B** changes it inside FFmpeg, at the cost of building FFmpeg ourselves.

**Either gives one read path for every run:**
- chunk runs and probes included, and MP4s with their index at the end;
- no socket, and FFmpeg opens no media file;
- every read is the node's to schedule, pace, share and give up on.

That closes the hybrid's gap. C could serve the beta, and B could replace C if jellyfin ever builds
fully static, or if the interposer proves fragile.

## Not verified

- The arm64 binary's imports.
- jellyfin's warm rebuild time and the disk it needs.
- The interposer under Landlock and seccomp.
- That FFmpeg 9.0 is released.
- That `fd:` first shipped in 6.0, inferred from its commit date.
