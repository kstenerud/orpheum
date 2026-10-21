> **ABOUTME:** Why a Linux NFS client took about 48 s to resume reads after the NAS's cable went
> back in, why FFmpeg outlived SIGKILL in the VM, what can be tuned, how to probe whether the share is
> back, and whether NFSv3 soft is the right mount for a read-only media share. Read from kernel source
> and the linux-nfs list on 2026-10-21.

# NFS client recovery after a dead share, 2026-10-21

**What prompted it.** In the dead-share test on 2026-10-21, the NAS's network cable was pulled for
about 50 s and then plugged back in. The VM host mounts the NAS's share as NFSv3
`soft,timeo=100,retrans=3` over TCP. It was NFSv4.2 until that morning, when the client deadlocked
in `nfs4_begin_drain_session`. The host passes the mount into the VM through virtiofs. Reads that
were blocked in the kernel returned only about 48 s after the cable went back in. FFmpeg children
blocked on the share did not die on SIGKILL, and exited only once the share answered.

**How it was done.** The sunrpc, NFS client, FUSE/virtio-fs and page-cache code was read at
`torvalds/linux` master (7.3, fetched 2026-10-21). The constants that matter were checked unchanged
at v6.12, the kernel Debian 13 ships. The linux-nfs and LKML threads, nfs(5), the kernel's
`ip-sysctl` documentation, virtiofsd's README and the media servers' own advice were read too.
Nothing was measured: no packet capture was taken during the test. Each claim gives its confidence.
**High** means it was read in the source or in primary documentation. **Medium** means it was
inferred from the source and fits the observation, but no trace confirms it. **Low** means a
plausible guess.

## Recommendations

1. **Keep NFSv3 `soft` over TCP for the read-only media share.** Do not go back to NFSv4.x (§6).
   Add `ro` and `nolock`. `timeo=100,retrans=3` gives a READ 40 s before it fails, which covers a
   NAS disk spinning up. Do not add `softreval` (§4). *High.*
2. **On the NFS client host, set `net.ipv4.tcp_rto_max_ms=5000`, then remount** (§1, §2). The
   wait after the share comes back is TCP's exponential retransmission back-off on the NFS
   connection, which no NFS or sunrpc option controls. The sysctl needs kernel **6.15 or later**,
   and Debian 13's stock 6.12 does not have it. It applies to sockets created after it is set, so
   remount afterwards; per the owner's notes, a remount behind virtiofs needs a VM restart. Setting
   it on the NAS too (if it runs Linux 6.15 or later) shortens the server side's own back-off.
   *Mechanism: high. That it brings recovery under about 5 s: medium; test it with the same cable
   pull.*
3. **Give virtiofsd a thread pool** (`--thread-pool-size=16` or so) if the VM's virtiofsd is the
   Rust one with its default of 0 (§5). With no pool, virtiofsd answers requests one at a time on
   the queue's thread. One read blocked on the dead share then holds up every guest request behind
   it, including a probe's `statfs` and the `FLUSH` a killed FFmpeg sends on exit. *Default: high
   (README). That it serialises the guest: medium.* Check how the VM launches virtiofsd first.
4. **Expect a killed FFmpeg to stay in D state until the share answers, and plan around it** (§3).
   In the VM, the wait is FUSE's uninterruptible wait for a request already handed to virtiofsd.
   virtio-fs implements no FUSE interrupt. Count such children apart from the device's limits, so
   that they do not hold slots, and do not wait on them to free the disk. *High.*
5. **Probe with `statfs`, one probe in flight at a time, each with a deadline the caller
   abandons** (§4). Do not probe with `stat`: the attribute caches in the guest and on the host can
   answer "fine" for up to 60 s after the server dies. A probe through the mount cannot see the
   share back before the stalled reads do, because it travels on the same TCP connection. To see the
   NAS reachable again earlier, add a separate TCP connect to `nas:2049`. *High.*
6. **Next cable pull: capture it.** On the host, run `ss -tni dst <nas>` once a second (its `rto:`
   and `backoff:` fields), `tcpdump -w` on port 2049, and the `sunrpc:rpc_timeout_status` and
   `tcp:tcp_retransmit_skb` tracepoints. In the VM, save `/proc/<ffmpeg pid>/stack`. That settles
   every "medium" below in one run.

## 1. Why the client took about 48 s after the link came back

**The cause is TCP's retransmission back-off on an established connection, not sunrpc's reconnect
back-off and not the RPC retransmit timer.** *High for the mechanism; medium that it accounts for
this test's 48 s, since no capture was taken.*

- **The connection was never closed during a 50 s outage.** For NFSv3, the transport's TCP socket
  options come from `xs_tcp_default_timeout` (60 s initial, 60 s max, 2 retries,
  `net/sunrpc/xprtsock.c`), not from the mount's `timeo`/`retrans`. The mount's values go only to
  the RPC client (`cl_timeout`, `rpc_new_client` in `net/sunrpc/clnt.c`). `xs_tcp_set_socket_timeouts`
  therefore sets `TCP_USER_TIMEOUT` to 60 s × 3 = **180 s**, and keepalive to idle 60 s, interval
  60 s, count 3. Only NFSv4 lowers these, to its lease period (`nfs4_set_lease_period`). In May 2024
  a v3 user on LKML observed exactly this: *"the NFS v3 client always tries to reconnect after some
  request hangs for 3 minutes no matter what timeo and retrans is"*, and `tcp_retries2` had no
  effect. `TCP_USER_TIMEOUT` overrides it.
- **So sunrpc's reconnect back-off never ran.** That back-off is `XS_TCP_INIT_REEST_TO` = 3 s,
  doubled per attempt (`xprt_reconnect_backoff`) up to `max_reconnect_timeout`: the transport's
  `to_maxval`, 60 s for v3 and half the lease for v4. Any data received resets it to 0
  (`xs_data_ready`). It matters only for outages longer than 180 s. Each reconnect attempt then
  also has TCP's SYN retries, `tcp_syn_retries` = 6, about 67 s to the last one.
- **The RPC timer does not decide when bytes reach the wire.** With `timeo=100,retrans=3` over TCP,
  `nfs_init_timeout_values` gives an initial 10 s, an increment of 10 s and a maximum of 40 s, with
  no exponential back-off. A request's major timeout is 10 + 3 × 10 = **40 s**
  (`xprt_calc_majortimeo`), after which a soft request fails with EIO (`rpc_check_timeout`). At
  each minor timeout, v3 re-queues the request on the same socket (`rpc_task_need_encode`).
  That puts more bytes behind the unacknowledged ones but sends nothing sooner.
- **TCP decides.** Linux's retransmission timeout starts near 200 ms on a LAN (`TCP_RTO_MIN`, plus
  the round-trip time) and doubles on every timeout, up to `TCP_RTO_MAX` = 120 s
  (`include/net/tcp.h`). The retransmissions of the first unacknowledged segment fall roughly at
  0.2, 0.6, 1.4, 3.0, 6.2, 12.6, 25.4, 51.0 and **102.2 s** after the loss. Nothing is sent between
  two of them: the link coming up does not trigger a retransmission. An outage that ends at about
  52–54 s therefore waits until about 102 s: **48–50 s after the cable is back.** The LKML 2024
  report's tcpdump shows the same ladder, ending with a retransmission 52 s after the server was
  ready again.
- **The NAS backs off too.** Its retransmissions of READ replies follow the same ladder.
  Whichever side's timer fires first resumes the flow in that direction, but the client's queued
  requests still wait for the client's own timer.
- **Why the reads did not fail at 40 s.** The host's soft READs should have failed with EIO about
  40 s into the outage. Two things stretch what a reader sees. (a) After a failed readahead, the
  page-cache read path issues a fresh synchronous read of the same folio (`filemap_update_page`
  calls `filemap_read_folio` in `mm/filemap.c`), and that read gets a new 40 s. (b) virtiofsd
  serves the guest's requests one at a time when it has no thread pool (§5), so the guest's queued
  reads take their 40 s timeouts one after another, until TCP resumes and everything completes at
  once. *Medium: read in the source, consistent with what was seen, not traced.*

## 2. What can be tuned

| Knob | Effect on the wait after the share returns | Effect during the outage | For a read-only media share |
| --- | --- | --- | --- |
| `net.ipv4.tcp_rto_max_ms` (6.15+, per network namespace, read at socket creation) | **Caps the TCP back-off: 5000 means at most about 5 s.** The fix. | More retransmissions on every stalled socket on the host, which is negligible. The doc asks you to review `tcp_retries2`, which does not apply to sunrpc's sockets because `TCP_USER_TIMEOUT` overrides it. | Safe. Host-wide. *High (kernel docs, `tcp_init_sock`).* |
| `timeo`, `retrans` | None. | They set when a soft READ fails: `timeo × (retrans + 1)` over TCP (linear back-off), 40 s now. | Keep it at 30 s or more, or a disk spinning up on the NAS fails reads. *High.* |
| `soft` / `softerr` | None. | `soft` returns EIO, `softerr` returns ETIMEDOUT. `softerr` also turns on `softreval` (`fs/nfs/fs_context.c`); write `softerr,nosoftreval` to avoid that. ETIMEDOUT would let the node tell "share timed out" from a disk error, if virtiofsd passes the errno through. | `soft` is fine. `softerr,nosoftreval` is worth a try. *High for the client; low that the errno survives virtiofs.* |
| `softreval` | None. | A timed-out revalidation answers from cache, so `stat` succeeds while the server is down. | **Avoid**: it makes `stat` lie to a probe. *High.* |
| `actimeo`, `lookupcache`, `nocto` | None. | They only bound how long cached metadata is believed. | `nocto` is fine for media that changes rarely. Long `actimeo` delays a scan seeing changes. *High.* |
| `nconnect=N` | Little. Each connection backs off on its own, and requests go round-robin over all of them. | None. | Not a fix. *Low.* |
| `proto=udp` | No TCP back-off, but the RPC's own back-off is exponential to 60 s. | — | **No.** Distributions disable it (`CONFIG_NFS_DISABLE_UDP_SUPPORT`, default y), and nfs(5) warns of silent corruption at gigabit speeds. *High.* |
| `sunrpc.tcp_max_slot_table_entries`, other sunrpc/nfs module parameters | None. No module parameter or sysctl sets the transport's TCP user timeout or reconnect cap for v3. | — | — *High (source).* |
| Write the server's own address to `/sys/kernel/sunrpc/xprt-switches/*/xprt-*/dstaddr` | Calls `xprt_force_disconnect`, so the client reconnects at once on a fresh socket. | — | An emergency lever for kernels older than 6.15, for something that sees the NAS answer again. Untested; the old connection's source-port reuse may add a few seconds. *Low.* |

**What the media servers advise.** Nothing on recovery time. Jellyfin's storage page says only to
mount the share in the operating system, and to use `nolock`, NFSv4 or an environment variable when
.NET's locking makes NFSv3 slow to start playing; it also warns that its scheduled tasks remove
items when storage is unavailable. Forum advice for Plex and Kodi (Unraid's pinned
`hard,timeo=50,retrans=5`, Kodi's `hard,intr`, the Arch wiki's `timeo=900,retrans=5`) is folklore:
`intr` has been ignored since 2.6.25 (nfs(5)), and nobody discusses the TCP back-off. Orpheum's
read-only use is the case that nfs(5)'s warning about `soft` (silent corruption when writing)
does not reach.

## 3. Is a process blocked on an NFS read killable?

**On the host, almost always: yes, for both soft and hard mounts.** *High.* Since 2.6.25 the NFS
client sleeps in `TASK_KILLABLE` (LWN, 2008). A synchronous RPC waits in `rpc_wait_bit_killable`.
A read waiting for readahead waits in `folio_put_wait_locked(folio, TASK_KILLABLE)`. `read_folio`
waits in `folio_wait_locked_killable`. `i_rwsem` is taken with `down_read_killable`
(`fs/nfs/io.c`). A page fault on a mapped file uses `__folio_lock_killable` when the fault is
`FAULT_FLAG_KILLABLE`, which every user-space fault is. Killing the reader does not cancel its
asynchronous readahead RPCs; they run to their own soft timeout, harmlessly.

What stays unkillable on the host: `filemap_invalidate_lock_shared`, which is contended only by
truncation or hole punching (not on a read-only share); folio locks taken by kernel-internal access
without `FAULT_FLAG_KILLABLE` (`get_user_pages` from `O_DIRECT` or `splice` with an NFS-mapped
buffer, core dumps); and anything in the exit path. Soft and hard differ only in that soft fails
on its own after the major timeout; both are killable.

**In the VM, through virtiofs: no, once the request has reached virtiofsd.** *High for the
mechanism; medium that it is the path seen.* FUSE's `request_wait_answer` (`fs/fuse/dev.c`) waits
interruptibly, then killably while the request is still pending. Once the daemon has the request,
it waits uninterruptibly: *"Either request is already in userspace, or it was forced. Wait it
out."* virtio-fs hands a request to the host as soon as it is queued (`virtio_fs_send_req` clears
`FR_PENDING`), and `virtio_fs_send_interrupt` is a stub (*"TODO interrupts"*). A SIGKILL also makes
the process close its files on exit, and `fuse_flush` sends `FUSE_FLUSH` with `force = true`, which
always waits uninterruptibly. So a guest FFmpeg blocked on the share, or killed while virtiofsd is
busy with the share, stays in D state until the host's read returns. To confirm, its
`/proc/<pid>/stack` will show `request_wait_answer`, under `fuse_flush` or a read.

## 4. Do `stat` and `statfs` reach the server?

- **`statfs` always does.** NFS's `nfs_statfs` sends an FSSTAT RPC on every call; there is no
  cache. FUSE's `fuse_statfs` likewise sends `FUSE_STATFS` every time, and virtiofsd answers it with
  `fstatvfs` on the host, which reaches NFS. It is the honest probe. It blocks for up to the major
  timeout (40 s) while the server is down, so a probe needs a deadline and an abandoned waiter. With
  one probe in flight at a time, abandoned probes cannot pile up. *High.*
- **`stat` often does not.** On the host, `nfs_getattr` revalidates only when the attribute cache
  has expired: 3–60 s for files and 30–60 s for directories by default (nfs(5)). In the guest,
  FUSE's own attribute timeout comes first (virtiofsd `--cache auto`). Within those windows, `stat`
  succeeds from cache with the server dead. Once the cache has expired, a GETATTR blocks up to 40 s
  and then fails. With `softreval` (or `softerr`, which implies it), the failed GETATTR returns
  **success** from cache (`nfs_revalidate_inode` turns `-ETIMEDOUT` into 0). `AT_STATX_FORCE_SYNC`
  forces a GETATTR in the guest, but virtiofsd's host-side `stat` need not pass the flag on. *High
  for NFS; medium for the virtiofs hop.*
- **Implication.** Any probe through the mount shares the stalled TCP connection, and in the guest
  it may share virtiofsd's single queue. It reports "back" when the reads come back, not when the NAS
  comes back. That is the right answer for "can I play from it". For "is the NAS up", a TCP connect
  to port 2049 on its own socket answers within a second. The difference between the two answers is
  the TCP back-off window of §1. This fits D327: a probe outside every queue still waits for the
  shared connection.

## 5. Does virtiofs change the picture?

Yes, in three ways. *High from source and README, except where marked.*

1. **Kills in the guest are outlived** (§3).
2. **virtiofsd may serialise everything.** The Rust virtiofsd's `--thread-pool-size` defaults to
   0, which means no pool: requests are handled on the queue's thread (README). One host `pread`
   blocked on the dead share then holds every later guest request on that mount, including
   `statfs` probes and the `FLUSH` of a killed process. A pool (the old C virtiofsd defaulted to 64)
   lets the probe and other files proceed. The NFS connection's stall still gates anything that
   needs the server. *Default: high. That the observed run had no pool: unknown; check the
   command line.*
3. **The guest has its own caches.** `--cache auto` (the default) gives the guest a short attribute
   cache and a page cache in front of the host's, so §4's caveats apply twice.

What virtiofs does not change: the host's NFS timeouts and TCP back-off. A guest read lasts as long
as the host read serving it, plus its place in virtiofsd's queue.

## 6. NFSv4.x again?

**No. NFSv3 soft is the right recommendation for a read-only media share today.** *High.*

- **Soft does not work as soft on v4 during a pull.** Every v4 client (4.0 too) sets
  `NFS_CS_NO_RETRANS_TIMEOUT` (`fs/nfs/nfs4client.c`), and `rpc_check_timeout` then keeps a sent
  soft request waiting while the transport counts as connected: *"Once a 'no retrans timeout'
  soft tasks (a.k.a NFSv4) has been sent, it should time out only if the transport connection gets
  terminally broken."* During a 50 s pull the connection stays up, so v4 reads hang, where v3's fail
  at 40 s.
- **Soft is a known hazard for v4.1+ sessions.** Rick Macklem wrote on linux-nfs (2026-05-16):
  *"soft mounts are often going to be troublesome for NFSv4.1. Whenever an RPC times out and
  doesn't wait for a reply from the server, it will leave a session slot messed up."* There are
  three open reports. 2025-12: slot-table draining plus memory reclaim deadlocks the creation of the
  state manager, after a server outage; Trond Myklebust replied, and the thread has no fix. 2026-03:
  OPEN reclaim stuck in `rpc_wait_bit_killable` after `NFS4ERR_EXPIRED`, on 5.15. 2026-05: v4.1
  soft clients stuck in `nfs4_drain_slot_tbl`, no RPCs sent, on 6.1. Hard v4.1 avoids the slot
  damage, but then a dead share never fails a read, which the node's disk gate relies on.
- **v4's advantages do not apply here.** It brings one port without rpcbind, mountd or NLM,
  delegations, and integrated locking. A read-only share on a LAN needs none of them, and `nolock`
  removes v3's lock daemons.
- **The only v4 workaround** would be `hard` with v4.1+ (no soft-slot damage), together with the
  node detecting the dead share itself and never waiting on a read. That is the opposite of what
  the node's design leans on. Not recommended.

## Sources

Kernel source, `git.kernel.org` torvalds/linux master (VERSION 7.3) as of 2026-10-21. The marked
constants were checked unchanged in stable v6.12.

- `net/sunrpc/xprtsock.c`: `XS_TCP_INIT_REEST_TO` (3 s), `xs_tcp_default_timeout` (60 s/60 s/2,
  checked in v6.12), `xs_tcp_set_socket_timeouts` (`TCP_USER_TIMEOUT`, keepalive, SYN count),
  `xs_connect`, `xs_data_ready`, `xs_tcp_ops` (no `.timer`).
- `net/sunrpc/xprt.c`: `xprt_reconnect_delay`, `xprt_reconnect_backoff` (checked in v6.12),
  `xprt_calc_majortimeo`, `xprt_adjust_timeout`, `xprt_request_retransmit_after_disconnect`.
- `net/sunrpc/clnt.c`: `rpc_new_client` (the mount's timeout goes to `cl_timeout` only),
  `call_status`, `rpc_check_timeout`, `rpc_task_need_encode`. `net/sunrpc/sysfs.c`:
  `rpc_sysfs_xprt_dstaddr_store`.
- `fs/nfs/client.c`: `nfs_init_timeout_values` (checked in v6.12). `fs/nfs/nfs4client.c`,
  `fs/nfs/nfs4renewd.c` (`rpc_set_connect_timeout` at half the lease). `fs/nfs/fs_context.c`
  (soft, softerr, softreval). `fs/nfs/inode.c` (`nfs_getattr`, `nfs_revalidate_inode`).
  `fs/nfs/nfs3proc.c`, `fs/nfs/super.c` (`nfs_statfs`), `fs/nfs/io.c`, `fs/nfs/read.c`,
  `fs/nfs/Kconfig` (`NFS_DISABLE_UDP_SUPPORT`).
- `mm/filemap.c`: `filemap_update_page`, `filemap_read_folio`, `lock_folio_maybe_drop_mmap`.
- `fs/fuse/dev.c` (`request_wait_answer`), `fs/fuse/virtio_fs.c` (`virtio_fs_send_req`,
  `virtio_fs_send_interrupt`), `fs/fuse/file.c` (`fuse_flush`, `force = true`), `fs/fuse/inode.c`
  (`fuse_statfs`).
- `include/net/tcp.h` (`TCP_RTO_MIN`, `TCP_RTO_MAX`), `net/ipv4/tcp.c` (`tcp_init_sock` reads
  `tcp_rto_max_ms`), `net/ipv4/sysctl_net_ipv4.c` (present in v6.15, absent in v6.14).

Documents and threads:

- nfs(5), man7.org: `soft`/`softerr`/`hard`, `softreval`, `intr`, `timeo`, `retrans`, attribute
  caching, `lookupcache`, `nconnect`, `nocto`, `nolock`, UDP corruption.
  <https://man7.org/linux/man-pages/man5/nfs.5.html>
- Kernel IP sysctl documentation: `tcp_rto_max_ms`, `tcp_rto_min_us`, `tcp_retries2`,
  `tcp_syn_retries`. <https://docs.kernel.org/networking/ip-sysctl.html>
- Eric Dumazet, "tcp: allow to reduce max RTO", netdev, 2025-02-07 (`TCP_RTO_MAX_MS`,
  `tcp_rto_max_ms`). <https://lwn.net/Articles/1008854/>
- Zhitao Li, "How to customize retransmission timeout of unacknowledged NFS v3 TCP packet?", LKML,
  2024-05-29 (52 s retransmission after failover; 3 min reconnect regardless of timeo/retrans).
  <https://lkml.iu.edu/hypermail/linux/kernel/2405.3/05635.html>,
  <https://lkml.rescloud.iu.edu/2405.3/05656.html>
- Wang Zhaolong, "[RFC] NFSv4.1: slot table draining + memory reclaim can deadlock state manager
  creation", linux-nfs, 2025-12-30, with Trond Myklebust's reply.
  <https://ratatoskr.run/linux-nfs/2025/12/3348485/t>
- "[BUG] NFSv4.1 client hang in OPEN reclaim path (rpc_wait_bit_killable) on 5.15", linux-nfs,
  2026-03-25. <https://ratatoskr.run/linux-nfs/2026/03/16643228/t>
- "[BUG] NFSv4.1 client hang in nfs4_drain_slot_tbl under concurrent workload", linux-nfs,
  2026-05-06, with Rick Macklem's reply on soft mounts.
  <https://ratatoskr.run/linux-nfs/2026/05/3546788/t>
- Jonathan Corbet, "TASK_KILLABLE", LWN, 2008-07-01. <https://lwn.net/Articles/288056/>
- virtiofsd README (`--thread-pool-size` default 0, `--cache` default auto).
  <https://gitlab.com/virtio-fs/virtiofsd/-/blob/main/README.md>
- Jellyfin, "Storage". <https://jellyfin.org/docs/general/administration/storage/>
- Unraid forum, "Plex folders randomly becoming unavailable with Unraid NFS mount", 2026-02.
  <https://forums.unraid.net/topic/197029-plex-folders-randomly-becoming-unavailable-with-unraid-nfs-mount/>
- Arch wiki, "NFS" and "Kodi". <https://wiki.archlinux.org/title/NFS>,
  <https://wiki.archlinux.org/title/Kodi>

The fetched pages and source files are cached in the sandbox at `/yoloai/cache/nfs-research/`.
