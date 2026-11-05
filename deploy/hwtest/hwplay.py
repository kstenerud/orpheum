#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Plays test films of several kinds through an installed deployment as an H.264-only browser,
# ABOUTME: checking the segments and the encoder its node runs (deploy/hwtest/README.md).
#
# Hardware playback check, run on the machine a deployment is installed on. It makes test films of
# several kinds in a fresh folder under one of the deployment's media folders, with the image's own
# FFmpeg, and plays each as a browser that plays only H.264 and AAC stereo would: from the quality
# ladder, the chunks a player starts on, fetching the first segments of its lowest quality, which
# is always encoded, and probing them. Throughout, it watches this machine's processes for the
# node's FFmpeg and the encoders it names, which must be only the encoder the node chose (its log)
# and, with --expect, that one. (A whole prepare, which only a film that cannot be cut gets, uses
# the arguments the node's encoder trial runs at its start, so the choice proves them.)
#
#   python3 hwplay.py --dir ~/orpheum --media /path/to/films [--expect h264_nvenc]
#
# The core must take this machine's requests as from home: run it where the deployment's HTTP is
# reached from an address in its home networks.
import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

ENCODERS = ("h264_qsv", "h264_nvenc", "h264_vaapi", "libx264")
H264_ONLY = {"videoCodecs": ["h264-sdr"], "audioCodecs": ["aac-stereo"], "subtitleFormats": ["webvtt"],
             "containers": ["mp4"]}

# The films: a name, its picture size, FFmpeg's video arguments, and its container.
FILMS = [
    ("MPEG-4 Part 2 720p", "1280x720", ["-c:v", "mpeg4", "-q:v", "4"], "mkv"),
    ("H.264 1080p high rate", "1920x1080", ["-c:v", "libx264", "-preset", "ultrafast", "-b:v", "20M",
                                             "-pix_fmt", "yuv420p"], "mkv"),
    ("HEVC 10-bit 1080p", "1920x1080", ["-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt",
                                         "yuv420p10le", "-x265-params", "log-level=error"], "mkv"),
    ("H.264 2160p", "3840x2160", ["-c:v", "libx264", "-preset", "ultrafast", "-b:v", "30M",
                                   "-pix_fmt", "yuv420p"], "mp4"),
]
# Films whose files have no index of their keyframes, cut on time (D269): AVI and MPEG-TS, as old
# rips and recordings are.
UNINDEXED_FILMS = [
    ("MPEG-4 Part 2 480p AVI", "854x480", ["-c:v", "mpeg4", "-q:v", "4"], "avi"),
    ("MPEG-2 576p TS", "720x576", ["-c:v", "mpeg2video", "-q:v", "4"], "ts"),
    ("HEVC 10-bit 1080p TS", "1920x1080", ["-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt",
                                            "yuv420p10le", "-x265-params", "log-level=error"], "ts"),
]
SECONDS = 40


def log(*a):
    print("hwplay:", *a, file=sys.stderr, flush=True)


class Core:
    def __init__(self, base):
        self.base = base.rstrip("/")
        self.token = None

    def call(self, method, path, body=None, raw=False):
        url = path if path.startswith("http") else self.base + path
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(url, data=data, method=method)
        if data is not None:
            req.add_header("Content-Type", "application/json")
        if self.token:
            req.add_header("Authorization", "Bearer " + self.token)
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                payload = resp.read()
                return resp.status, payload if raw else (json.loads(payload) if payload else None)
        except urllib.error.HTTPError as e:
            payload = e.read()
            if raw:
                return e.code, payload
            return e.code, json.loads(payload) if payload.strip().startswith(b"{") else payload


def compose(directory, *args):
    return subprocess.run(["docker", "compose", *args], cwd=directory, capture_output=True, text=True)


def watch_encoders(seen, stop):
    """Collects the encoders named by every ffmpeg running on this machine until stop is set."""
    while not stop.is_set():
        for pid in os.listdir("/proc"):
            if not pid.isdigit():
                continue
            try:
                with open(f"/proc/{pid}/cmdline", "rb") as f:
                    args = f.read().split(b"\0")
            except OSError:
                continue
            if not args or not args[0].endswith(b"ffmpeg"):
                continue
            words = [a.decode(errors="replace") for a in args]
            if "lavfi" in words:
                continue  # this script's own film-making, or the node's encoder trial
            for enc in ENCODERS:
                if enc in words:
                    seen.setdefault(enc, " ".join(words))
        time.sleep(0.1)


def is_rootless():
    return subprocess.run(["sh", "-c", "docker info 2>/dev/null | grep -qE '^[[:space:]]+rootless$'"]).returncode == 0


def in_image(image, mounts, entry, args):
    # As this user, who can write the folder: under rootless Docker, the container's root is.
    cmd = ["docker", "run", "--rm", "--user", "0:0" if is_rootless() else f"{os.getuid()}:{os.getgid()}"]
    for m in mounts:
        cmd += ["-v", f"{m}:{m}"]
    return subprocess.run(cmd + ["--entrypoint", f"/opt/orpheum/ffmpeg/{entry}", image] + args,
                          capture_output=True, text=True)


def probe(path, image):
    out = in_image(image, [os.path.dirname(path)], "ffprobe",
                   ["-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name,width,height",
                    "-of", "json", path])
    return (json.loads(out.stdout or "{}").get("streams") or [{}])[0]


def make_films(folder, image, films):
    os.makedirs(folder)
    made = []
    for name, size, codec, ext in films:
        title = f"{name} (2020)"
        os.makedirs(os.path.join(folder, title))
        path = os.path.join(folder, title, f"{title}.{ext}")
        out = in_image(image, [folder], "ffmpeg", [
            "-v", "error", "-f", "lavfi", "-i", f"testsrc2=size={size}:rate=24:duration={SECONDS}",
            "-f", "lavfi", "-i", f"sine=frequency=440:duration={SECONDS}", *codec, "-c:a", "aac", "-shortest",
            path])
        if out.returncode != 0:
            sys.exit(f"making {name} failed: {out.stderr}")
        made.append(path)
    return made


def fetch_segments(core, url, count, work, lowest=False):
    """The first count video segments of the playlist at url, or of its first variant (its last, the
    lowest quality, when lowest), each joined to its init segment, waiting for each as the node makes
    it."""
    os.makedirs(work, exist_ok=True)
    deadline = time.time() + 300
    got = []
    playlist_url = url
    while time.time() < deadline and len(got) < count:
        status, text = core.call("GET", playlist_url, raw=True)
        if status != 200:
            time.sleep(1)
            continue
        lines = [l.strip() for l in text.decode().splitlines() if l.strip()]
        if any(l.startswith("#EXT-X-STREAM-INF") for l in lines):
            variants = [l for l in lines if not l.startswith("#")]
            variant = variants[-1] if lowest else variants[0]
            playlist_url = urllib.parse.urljoin(playlist_url, variant)
            continue
        init = next((re.search(r'URI="([^"]+)"', l).group(1) for l in lines if l.startswith("#EXT-X-MAP")), None)
        segs = [l for l in lines if not l.startswith("#")]
        for seg in segs[len(got):count]:
            status, data = core.call("GET", urllib.parse.urljoin(playlist_url, seg), raw=True)
            if status != 200:
                break
            if init:
                _, head = core.call("GET", urllib.parse.urljoin(playlist_url, init), raw=True)
                data = head + data
            path = os.path.join(work, f"seg{len(got)}.mp4")
            with open(path, "wb") as f:
                f.write(data)
            got.append(path)
        if len(got) < count:
            time.sleep(1)
    return got


def sign_in(core, directory, password):
    status, setup = core.call("GET", "/setup")
    if setup.get("isRequired"):
        code = compose(directory, "exec", "-T", "orpheum-core", "orpheum", "setup-code", "--instance",
                       "/var/lib/orpheum/core").stdout.strip().splitlines()[-1]
        status, body = core.call("POST", "/setup", {"setupCode": code, "name": "hwplay", "password": password})
        log("made the first admin:", status)
    status, session = core.call("POST", "/session", {"name": "hwplay", "password": password,
                                                     "device": {"name": "hwplay", "platform": "web"}})
    if status != 201:
        sys.exit(f"sign-in failed: {status} {session}")
    core.token = session["token"]


def library_of(core, directory, media):
    status, libs = core.call("GET", "/libraries")
    library = next((l for l in libs.get("libraries") or []
                    if any(r.get("rootPath") == media for r in l.get("roots") or [])), None)
    if library:
        return library["id"]
    node = compose(directory, "exec", "-T", "orpheum-node", "sh", "-c",
                   "grep '^name' /var/lib/orpheum/node/persistent/config.toml").stdout
    node_name = re.search(r'"([^"]+)"', node).group(1)
    deadline = time.time() + 120
    while True:
        status, made = core.call("POST", "/libraries", {
            "name": "hwplay", "kind": "movie", "metadataSource": "nfo", "changeDetection": "manual",
            "isWritable": False, "nodeName": node_name, "rootPaths": [media]})
        # A node not yet back since the core started is waited for.
        if status != 503 or time.time() > deadline:
            break
        time.sleep(2)
    if status != 201:
        sys.exit(f"making the library failed: {status} {made}")
    log("made library", made["id"], "on node", node_name)
    return made["id"]


def files_of(core, library, paths):
    core.call("POST", f"/libraries/{library}/scan")
    deadline = time.time() + 300
    while time.time() < deadline:
        status, out = core.call("GET", f"/files?libraryId={library}&limit=500")
        files = {f["path"]: f for f in (out or {}).get("files") or [] if f.get("path") in paths}
        if len(files) == len(paths):
            return [files[p] for p in paths]
        time.sleep(2)
    sys.exit(f"the scan did not find all of {paths}")


def chosen_encoder(directory):
    logs = compose(directory, "logs", "orpheum-node").stdout + compose(directory, "logs", "orpheum-node").stderr
    found = re.findall(r'chosenEncoder=(\S+)', logs)
    return found[-1] if found else None


def play(core, file, image, segments, work):
    """Whether the film plays from its ladder, its lowest quality's first segments H.264."""
    name = os.path.basename(file["path"])
    status, started = core.call("POST", "/playback", {"fileId": file["id"], "playableFormats": H264_ONLY,
                                                      "cause": "playPage", "pageStarts": 1})
    if status != 200:
        log(f"{name}: FAIL: playback refused: {status} {started}")
        return False
    url = (started.get("onLadder") or {}).get("url")
    if not url:
        log(f"{name}: FAIL: not on its ladder (delivery {started.get('delivery')})")
        return False
    url = re.sub(r"/api/v1$", "", core.base) + url if url.startswith("/") else url
    got = fetch_segments(core, url, segments, os.path.join(work, str(file["id"])), lowest=True)
    probed = [probe(p, image) for p in got]
    ok = len(got) == segments and all(s.get("codec_name") == "h264" for s in probed)
    picture = f'{probed[0].get("width")}x{probed[0].get("height")}' if probed else "-"
    log(f"{name}: {'PASS' if ok else 'FAIL'}: {len(got)} segments of its lowest quality, "
        f"{sorted({s.get('codec_name') for s in probed})} {picture}")
    return ok


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dir", required=True, help="the deployment's directory")
    ap.add_argument("--media", required=True, help="one of the deployment's media folders, writable here")
    ap.add_argument("--core", default="http://127.0.0.1:8440/api/v1")
    ap.add_argument("--expect", help="the encoder the node must use, e.g. h264_nvenc")
    ap.add_argument("--segments", type=int, default=3)
    ap.add_argument("--password", default="hwplay-test-password")
    args = ap.parse_args()
    directory = os.path.expanduser(args.dir)
    media = os.path.abspath(os.path.expanduser(args.media))
    env = dict(line.split("=", 1) for line in open(os.path.join(directory, ".env")).read().splitlines()
               if "=" in line and not line.startswith("#"))
    image = env["ORPHEUM_IMAGE"]
    chosen = chosen_encoder(directory)
    log("image", image, "; the node chose", chosen)

    core = Core(args.core)
    sign_in(core, directory, args.password)
    library = library_of(core, directory, media)
    for old in os.listdir(media):
        if old.startswith("hwplay-"):
            shutil.rmtree(os.path.join(media, old))
    work = os.path.join(media, ".hwplay-work")
    shutil.rmtree(work, ignore_errors=True)

    seen, stop = {}, threading.Event()
    watcher = threading.Thread(target=watch_encoders, args=(seen, stop), daemon=True)
    watcher.start()
    stamp = time.strftime("%Y%m%d%H%M%S")
    films = make_films(os.path.join(media, f"hwplay-{stamp}"), image, FILMS + UNINDEXED_FILMS)
    results = [play(core, file, image, args.segments, work) for file in files_of(core, library, films)]
    time.sleep(1)
    stop.set()
    watcher.join()
    want = args.expect or chosen
    encoders = sorted(seen)
    isEncoder = encoders == [want]
    log(f"encoders run: {encoders}; want only {want}: {'PASS' if isEncoder else 'FAIL'}")
    if not isEncoder:
        for cmd in seen.values():
            log("   ", cmd[:600])
    results.append(isEncoder)
    log(f"{sum(results)} of {len(results)} passed")
    sys.exit(0 if all(results) else 1)


if __name__ == "__main__":
    main()
