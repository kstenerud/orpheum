# Hardware playback tests

Every kind of GPU the owner has a machine for is tested for hardware playback, installed as a
household installs, in the situations households meet. One command per machine:

```sh
sh deploy/hwtest/hwtest.sh <image or `docker save` archive> --expect <encoder>
```

`hwtest.sh` installs the image with its own install script into a scratch deployment (`~/hwtest`,
compose project `hwtest`, HTTP on `127.0.0.1:18440` alone, so it runs beside a real deployment, as
on the production NAS). `hwplay.py` then makes seven test films with the image's own FFmpeg and plays each as a
browser that plays only H.264 and AAC stereo does: from its quality ladder, fetching the first
segments of the lowest quality, which is always encoded, and probing them. Throughout, it watches
the machine's processes for the node's FFmpeg: the only encoder run must be the one the node chose,
and `--expect`'s. The deployment is removed afterwards (`--keep` leaves it).

The films: MPEG-4 Part 2 720p, H.264 1080p at 20 Mb/s and H.264 2160p (whose top quality is their
own picture, copied), and 10-bit HEVC 1080p, in MKV and MP4, which have an index of their keyframes;
and MPEG-4 Part 2 in AVI, MPEG-2 in MPEG-TS and 10-bit HEVC in MPEG-TS, which do not and are cut on
time (D269). A whole prepare, which only a film that cannot be cut gets, uses the arguments the
node's encoder trial runs at its start, so a node choosing the encoder proves them.

A request from the machine itself reaches the core from Docker's gateway, not the house's network,
so the script names that gateway a home network in the scratch deployment's core config; only the
machine itself can come from it, as nothing else reaches 127.0.0.1.

## Machines and situations

| Machine | GPU | Encoder | Docker | Situation it covers | Last run |
| --- | --- | --- | --- | --- | --- |
| The GPU test machine | RTX 3090 (driver 580) | `h264_nvenc` | 29, rootless, NVIDIA by CDI | NVIDIA; rootless Docker; the toolkit's CDI spec | 8/8, 2026-10-24, through fdproxy; 8/8, 2026-10-23 |
| The GPU test machine | as above | `h264_nvenc` | 29.9.2, rootful, toolkit 1.20 from apt | NVIDIA as most households install it: the install chose CDI, the toolkit's own spec | 8/8, 2026-10-24, through fdproxy; 8/8, 2026-10-23 |
| The GPU test machine | as above | `h264_nvenc` | as above, the override switched to the `nvidia` driver's reservation (by hand on 10-23; on 10-24 by `write-deployment --nvidia driver`) | Docker's `nvidia` driver (VQ53) | 8/8, 2026-10-24, through fdproxy; 8/8, 2026-10-23 |
| amd | Radeon RX 6600 XT | `h264_vaapi` | 29, rootful, compose as a user plugin | AMD through Mesa (D332) | 8/8, 2026-10-24, through fdproxy; 8/8, 2026-10-23 |
| The staging host | Iris Plus 640, Gen 9.5 | `h264_vaapi` | as amd | Intel before Gen 12: Quick Sync fails, VAAPI by iHD | 8/8, 2026-10-24, through fdproxy; 8/8, 2026-10-23 |
| T450 | HD Graphics 5500, Gen 8 | `h264_vaapi` | as amd | the oldest Intel: VAAPI by iHD | 8/8, 2026-10-24, through fdproxy; 8/8, 2026-10-23 |
| The production NAS | i3-N305, Gen 12 | `h264_qsv` | 29.9.0, Proxmox LXC | Intel Quick Sync; a container in a Proxmox container; beside production | 8/8, 2026-10-24, through fdproxy, beside production; 8/8, 2026-10-23, reading through ffstreamer |
| MacBook Air M4 | none in a container | `libx264` (the node's `x264`) | OrbStack 2.2.1, Docker 29.4.0, arm64 | Apple silicon; the arm64 image, built on the Mac (57 s on 10-24, 87 s on 10-23); software encoding | 7/7 films, 2026-10-24, through fdproxy; 7/7 films, 2026-10-23, reading through ffstreamer; both with the encoder check by hand (below) |

The 2026-10-24 runs used one image built from main at 999d0938, whose FFmpeg is the pinned
ffmpeg-fdproxy 8.1.2-5-2 release (the Mac's built there, for arm64, from the same commit): on every
machine the node logged "FFmpeg reads each film through the node (fdproxy)" and chose the encoder in
the table, and only that encoder ran. The production NAS's ran beside production with no film playing there.

The other 2026-10-23 runs used an image built from main at d1968b1b with jellyfin-ffmpeg 8.1.2-5 as
published, before the ffstreamer build was pinned: films were read by their path. The production NAS's ran on
v0.3.0-beta.7's image, with ffmpeg-ffstreamer 8.1.2-5-1, every film served to FFmpeg by the node; it
could run beside production once each deployment's network had a subnet of its own.

**On a Mac** the kit does not run as it stands, and the run above went around three things:
`/bin/sh` there (bash 3.2) cannot parse `hwtest.sh` (a `case` inside `$( )`; run it with
`zsh --emulate sh`); `hwplay.py` finds the node's FFmpeg in `/proc`, which macOS lacks and which
could not see OrbStack's VM anyway, so its encoder check reports nothing (a watcher inside the VM
saw 38 encodes, every one `libx264` reading `ffstreamer:`); and the install script finds the
machine's address at home with `ip`, which macOS lacks, so a first install stops asking for
`lan.address` (the run filled it in from `ipconfig getifaddr en0`). The install script now asks
`ipconfig` where `ip` is missing, and on 2026-10-24 found the Mac's address itself; the first two
remain, so that run, too, ran `hwtest.sh` with `zsh --emulate sh` and watched the VM from a container
with its process namespace (`--pid=host`): 7 encodes, every one `libx264`, every film read as
`fdproxy:`, none by its path; `hwplay.py` itself reports 7 of 8, the eighth its empty encoder check.

Situations each machine met on the way, now handled or documented: rootless Docker's user mapping
(the install script runs as the container's root there), a CDI spec gone stale after a driver
update, and Docker without the compose plugin (the install script says to install it), in
`docs/users/setup-problems.md` and `docs/contributors/idiosyncrasies.md`.

**Rootless Docker** hides each device's address behind its userland proxy, so no one administers
from home; the install script now says so and gives the fix (`docs/users/setup-problems.md`),
which waits on VQ54 to be seen working.
