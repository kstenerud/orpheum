> **ABOUTME:** Release mode (D249 §5, D272): production on nas.example from an image built there, set
> up by the image's own command, migrated through a sliding window, and watching its media. The
> phases, what each changes, and where each stands.

# Release mode

- **Status:** BUILT — begun 2026-10-09 on the owner's word (*"Yes start building release
  mode"*). R1 to R7 are built, and R8 is done: nas.example has run release mode since 2026-10-13
  (`v0.1.0-nas.1`), and `v0.2.0-nas.x` since 2026-10-14; it runs `v0.2.0-nas.11` now.
- **Depends on:** D249, D272 (and its §6–§9, the owner's choices for the build), D27, D240,
  D228, [scanner.md](scanner.md).

## What D272 asks

- Production runs on nas.example, a Proxmox VE host on Debian trixie, in an LXC container, with the
  core and its node on one machine; Landlock on in the host's kernel; the Intel N95's render node
  passed through; `/tank/media/video` (ZFS) bind-mounted and watched (§1).
- Installing is the image's setup command, then `docker compose up -d`; updating is a new image,
  then `docker compose up -d`; the tunnel is optional. Staging is set up the same way (§3).
- The image is built on nas.example from a tagged checkout by one command, which then runs that
  image's setup over the deployment (§4, §7).
- Migrations keep every step since `internal/store/migrations/CHECKPOINT` (§5, §6).
- Staging and nas.example share one tunnel key pair; one runs its tunnel at a time (§8).
- A change the watcher sees becomes a scoped scan after a quiet spell; an overflow scans the root;
  the library page says whether each root is watched; the periodic scan stays beneath (§9).

## Found while planning

- **There is no periodic scan.** `library.change_detection` and `check_every_hours` are stored and
  shown, but nothing reads them: a root is scanned when its node says hello, when the library is
  made or moved, and when the owner presses rescan. D272 §1 keeps a periodic scan beneath the
  watcher, so R5 builds it.
- **A listing is always of the whole root.** `nodeproto.List` can be scoped and the node honours
  it (`TestList_GoesOnlyAsDeepAsAskedFromWhereAsked`), but the core sends only whole-root listings,
  and its comparison removes every file under the root that the listing did not see. A scoped scan
  must remove only within its scope (R6).
- **The node process is not confined**; only its FFmpeg runs are. inotify needs no change to the
  confinement.

## Phases

| Phase | What | State |
| --- | --- | --- |
| R1 | The migration window: `CHECKPOINT`, `TestSteps_KeepTheMigrationWindow`, the baseline frozen while it exists | built, an earlier commit |
| R2 | `orpheum setup <dir>`: the deployment's directory written by the binary from templates embedded in it (compose file, values file, configs, tunnel keys, media and GPU override, `.env` naming the image); a values file says staging or release and whether there is a tunnel. `./staging` becomes: build the image, run its setup over `.staging/`, `docker compose up`. The tunnel stays required for now: a deployment without one (D272 §3) waits for a machine that needs it | built |
| R3 | The build command for nas.example: from a tag, build `orpheum:<tag>` with its commit in it, then run that image's setup over the deployment directory: `deploy/release/build.sh`, version tags only, so the binary records its version | built |
| R4 | `deploy/release/README.md`: the LXC container (unprivileged, nesting, the render node, the media bind mount), Docker in it, the Landlock check, install, update, going back, and the swap between staging and nas.example | written; untried, since only nas.example can try it (R8) |
| R5 | The periodic scan: a sweep, every 15 minutes, asks for the scan of each root whose library checks every N hours, or is watched, once its interval has passed since its last scan; a watched library's interval is the setting `watched_library_scan_every_hours`, 24 | built |
| R6 | Scoped scans in the core (`Scans.RequestUnder`): a scan's scope kept with it in cache.db; removals, moves and mount checks only within it, the withhold share judged against the whole root; requests made while one runs widen to the folder holding them all; one that may not judge becomes the whole root's; it never counts as the root's last scan, so the periodic scan stays due | built |
| R7 | The watcher, `internal/node/watch`: inotify on every folder of every root not on a network filesystem; per root, the folder holding every change is noticed once the root has gone `node.watch_quiet_seconds` (30) quiet, or after 20 of them while changes keep coming; an overflow notices every root whole; a root that cannot be watched whole is not watched and says why. Notices go to `/node/v1/changed` (protocol 31), and the core scans the folder only for a watched library; each root's watch rides in the status, and the library page shows it | built |
| R8 | On nas.example with the owner: install, first run, `CHECKPOINT` committed at the version it runs | done: installed 2026-10-13 (`v0.1.0-nas.1`); `CHECKPOINT` committed at 36 on 2026-10-14, and moved to 48 by D303's fresh baseline |

## Values to classify

Each value a phase adds is classified as a setting, a config key or a justified constant
(hardcoded-values watch):

- **The quiet spell** (R7): a node config key, `node.watch_quiet_seconds` (30), since roots and
  their watching are the node's (D107).
- **The longest hold** (R7): a justified constant, 20 quiet spells, so a copy that never pauses is
  still noticed.
- **The periodic scan's interval**: the library's own `check_every_hours`, already a column; for a
  watched library, which has none, the setting `watched_library_scan_every_hours` (24).
- **How often the schedule is checked**: a justified constant, 15 minutes, a bound on lateness
  against intervals of hours, as the scheduled backup's.
