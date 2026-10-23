# Hardware playback tests

Every kind of GPU the owner has a machine for is tested for hardware playback, installed as a
household installs, in the situations households meet. One command per machine:

```sh
sh deploy/hwtest/hwtest.sh <image or `docker save` archive> --expect <encoder>
```

`hwtest.sh` installs the image with its own install script into a scratch deployment (`~/hwtest`,
compose project `hwtest`, HTTP on `127.0.0.1:18440` alone, so it runs beside a real deployment, as
on nas.example). `hwplay.py` then makes seven test films with the image's own FFmpeg and plays each as a
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
| gpuhost | RTX 3090 (driver 580) | `h264_nvenc` | 29, rootless, NVIDIA by CDI | NVIDIA; rootless Docker; the toolkit's CDI spec | 8/8, 2026-10-23 |
| gpuhost | as above | `h264_nvenc` | rootful, NVIDIA by the toolkit's `nvidia` driver | NVIDIA as most households install it (VQ53) | not run: needs `orpheum` in the `docker` group |
| amd | Radeon RX 6600 XT | `h264_vaapi` | 29, rootful, compose as a user plugin | AMD through Mesa (D332) | 8/8, 2026-10-23 |
| staging host | Iris Plus 640, Gen 9.5 | `h264_vaapi` | as amd | Intel before Gen 12: Quick Sync fails, VAAPI by iHD | 8/8, 2026-10-23 |
| T450 | HD Graphics 5500, Gen 8 | `h264_vaapi` | as amd | the oldest Intel: VAAPI by iHD | 8/8, 2026-10-23 |
| nas.example | i3-N305, Gen 12 | `h264_qsv` | Proxmox LXC | Intel Quick Sync; a container in a Proxmox container; beside production | not run yet |
| MacBook Air M4 | none in a container | `libx264` | OrbStack, arm64 | Apple silicon; the arm64 image; software encoding | not run yet: needs an arm64 image |

The 2026-10-23 runs used an image built from main at d1968b1b with jellyfin-ffmpeg 8.1.2-5 as
published, before the ffstreamer build was pinned: films were read by their path.

Situations each machine met on the way, now handled or documented: rootless Docker's user mapping
(the install script runs as the container's root there), a CDI spec gone stale after a driver
update, and Docker without the compose plugin (the install script says to install it), in
`docs/users/setup-problems.md` and `docs/contributors/idiosyncrasies.md`.

**Open:** under rootless Docker the core sees every device at Docker's gateway, never at its own
address, so no one administers it from home ("administering works only from the server's own
network"). Docker's userland proxy re-originates each connection; turning it off needs the
`br_netfilter` module, which only root loads.
