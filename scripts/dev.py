#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: The development loop behind `make dev`: renders the dev config, seeds an empty
# ABOUTME: database, runs the server and the Vite dev server, and restarts the server on change.
"""Run Orpheum for development, with opinionated settings that are printed, never hidden.

`make dev` runs `loop`:

1. **Prepare.** Fetch the pinned FFmpeg (`make ffmpeg`), generate the dev library
   (`make dev-media`) and render `dev/orpheum.dev.toml` into `.dev/orpheum.toml`.
2. **Build.** `go build -tags dev -race` into `.dev/bin/orpheum`. The dev tag adds the
   `orpheum dev` subcommand, and the race detector is on because two people and a watcher are
   exactly where data races hide. `DEV_RACE=0` turns it off.
3. **Seed.** If `.dev/state` is empty, run `orpheum dev seed` with `dev/seed.toml`, rendered
   into `.dev/seed.toml` like the config.
4. **Run.** Start `orpheum run` against the dev config, and the Vite dev server with hot reload,
   which proxies the API and media to the server. Output goes to the terminal, prefixed
   `[server]` and `[web]`; the server's is also appended to `.dev/orpheum.log`.
5. **Watch.** Poll the Go sources, `go.mod` and `dev/*.toml` every second. On a change, rebuild
   and restart the server. The web dev server reloads itself. A server that exits is reported
   with its status, and the loop keeps watching, so a failed build is fixed and retried without
   restarting anything.

Ctrl-C stops everything. Stdlib only, so the loop adds no dependency.
"""

from __future__ import annotations

import argparse
import os
import signal
import subprocess
import sys
import threading
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEV = ROOT / ".dev"
CONFIG_TEMPLATE = ROOT / "dev" / "orpheum.dev.toml"
CONFIG = DEV / "orpheum.toml"
SEED_TEMPLATE = ROOT / "dev" / "seed.toml"
SEED = DEV / "seed.toml"
BINARY = DEV / "bin" / "orpheum"
LOG = DEV / "orpheum.log"
WATCHED_SUFFIXES = (".go",)
WATCHED_NAMES = ("go.mod", "go.sum")
SKIPPED_PARTS = ("/web/", "/.dev/", "/.tools/", "/.git/", "/node_modules/")

sys.path.insert(0, str(ROOT / "scripts"))
import dev_media  # noqa: E402
import fetch_ffmpeg  # noqa: E402


def render_config(template: str, root: Path, ffmpeg_dir: Path, listen: str | None) -> str:
    """The dev config with its placeholders filled; `listen` overrides the address."""
    text = template.replace("{root}", str(root)).replace("{ffmpeg_dir}", str(ffmpeg_dir))
    if listen:
        text = text.replace('listen = "127.0.0.1:8432"', f'listen = "{listen}"')
    return text


def listen_address(config: str) -> str:
    for line in config.splitlines():
        if line.strip().startswith("listen"):
            return line.split("=", 1)[1].split("#", 1)[0].strip().strip('"')
    return "127.0.0.1:8432"


def web_host(listen: str | None) -> str:
    """The address Vite binds: the host part of `listen`, so DEV_LISTEN opens both to the LAN."""
    if not listen:
        return "127.0.0.1"
    return listen.rsplit(":", 1)[0].strip("[]")


def web_url(listen: str | None) -> str:
    """Where to open the web client: a wildcard bind is reached from the LAN by the name Vite
    accepts (web/apps/web/vite.config.ts allowedHosts)."""
    host = web_host(listen)
    if host in ("0.0.0.0", "::"):
        host = "orpheum-dev.lan"
    elif ":" in host:
        host = f"[{host}]"
    return f"http://{host}:5173"


def snapshot(root: Path) -> dict[str, float]:
    """Modification times of everything whose change means rebuilding the server."""
    times: dict[str, float] = {}
    for path in root.rglob("*"):
        rel = "/" + path.relative_to(root).as_posix()
        if any(part in rel + "/" for part in SKIPPED_PARTS):
            continue
        watched = path.suffix in WATCHED_SUFFIXES or path.name in WATCHED_NAMES
        watched = watched or (path.parent == root / "dev" and path.suffix == ".toml")
        if watched and path.is_file():
            times[rel] = path.stat().st_mtime
    return times


def prepare(listen: str | None) -> Path:
    ffmpeg_dir = fetch_ffmpeg.install(fetch_ffmpeg.this_platform(), ROOT)
    dev_media.build(ffmpeg_dir, DEV / "media", seconds=30, force=False)
    DEV.mkdir(exist_ok=True)
    CONFIG.write_text(render_config(CONFIG_TEMPLATE.read_text(), ROOT, ffmpeg_dir, listen))
    SEED.write_text(render_config(SEED_TEMPLATE.read_text(), ROOT, ffmpeg_dir, None))
    return ffmpeg_dir


def build(race: bool) -> bool:
    args = ["go", "build", "-tags", "dev", "-o", str(BINARY), "./cmd/orpheum"]
    if race:
        args.insert(2, "-race")
    env = {**os.environ, "CGO_ENABLED": "1" if race else os.environ.get("CGO_ENABLED", "0")}
    done = subprocess.run(args, cwd=ROOT, env=env, check=False)
    if done.returncode != 0:
        print("[dev] build failed; fix it and save — the loop is still watching", flush=True)
    return done.returncode == 0


def pump(stream, prefix: str, log: Path | None) -> None:
    sink = log.open("a") if log else None
    for line in iter(stream.readline, ""):
        sys.stdout.write(f"{prefix} {line}")
        sys.stdout.flush()
        if sink:
            sink.write(line)
            sink.flush()
    if sink:
        sink.close()


def start(args: list[str], prefix: str, log: Path | None = None,
          env: dict[str, str] | None = None) -> subprocess.Popen[str]:
    proc = subprocess.Popen(args, cwd=ROOT, env=env, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT, text=True, start_new_session=True)
    threading.Thread(target=pump, args=(proc.stdout, prefix, log), daemon=True).start()
    return proc


def stop(proc: subprocess.Popen[str] | None) -> None:
    if proc is None or proc.poll() is not None:
        return
    os.killpg(proc.pid, signal.SIGTERM)
    try:
        proc.wait(timeout=10)
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGKILL)


def seed_if_empty() -> None:
    state = DEV / "state"
    if state.exists() and any(state.iterdir()):
        return
    print("[dev] empty database: seeding from dev/seed.toml", flush=True)
    subprocess.run([str(BINARY), "dev", "seed", "--config", str(CONFIG), "--seed", str(SEED)],
                   cwd=ROOT, check=False)


def banner(ffmpeg_dir: Path, with_web: bool, race: bool, listen: str | None) -> None:
    address = listen_address(CONFIG.read_text())
    lines = [
        "Orpheum dev loop — every setting below is a debugging choice, not a default",
        f"  server       http://{address}   (pprof at /debug/pprof, metrics at /metrics)",
        f"  web (Vite)   {web_url(listen)}   hot reload; proxies /api and /media to the server"
        if with_web else "  web          served by the server (built into the binary)",
        "  users        owner / owner (admin)      partner / partner (outsider)",
        f"  config       {CONFIG}   (from dev/orpheum.dev.toml)",
        f"  seed         {ROOT / 'dev' / 'seed.toml'}   (applied to an empty database)",
        f"  media        {DEV / 'media'}   (read-only, generated)",
        f"  ffmpeg       {ffmpeg_dir}",
        f"  logs         {LOG}",
        f"  race         {'on' if race else 'off (DEV_RACE=0)'}; confinement reported, not required",
        "  rebuilds     on any .go, go.mod or dev/*.toml change; Ctrl-C stops everything",
    ]
    print("\n".join(lines), flush=True)


def loop(with_web: bool, race: bool, listen: str | None) -> int:
    ffmpeg_dir = prepare(listen)
    banner(ffmpeg_dir, with_web, race, listen)
    web = None
    if with_web:
        env = {**os.environ, "ORPHEUM_DEV_SERVER": f"http://{listen_address(CONFIG.read_text())}"}
        web = start(["npm", "--prefix", "web", "--workspace", "@orpheum/web", "run", "dev", "--",
                     "--host", web_host(listen), "--port", "5173", "--strictPort"], "[web]", env=env)
    server = None
    reported = False
    seen = snapshot(ROOT)
    rebuild = True
    try:
        while True:
            if rebuild:
                stop(server)
                server = None
                reported = False
                if build(race):
                    seed_if_empty()
                    server = start([str(BINARY), "run", "--config", str(CONFIG)], "[server]", LOG)
                rebuild = False
            if server is not None and server.poll() is not None and not reported:
                print(f"[dev] the server exited with status {server.returncode}; "
                      "still watching for changes", flush=True)
                reported = True
            time.sleep(1)
            now = snapshot(ROOT)
            if now != seen:
                seen = now
                rebuild = True
                print("[dev] change detected: rebuilding", flush=True)
    except KeyboardInterrupt:
        print("\n[dev] stopping", flush=True)
    finally:
        stop(server)
        stop(web)
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    sub = ap.add_subparsers(dest="command", required=True)
    run = sub.add_parser("loop", help="build, seed, run and watch")
    run.add_argument("--no-web", action="store_true", help="skip the Vite dev server")
    run.add_argument("--listen", help="override the listen address, e.g. 0.0.0.0:8432")
    sub.add_parser("config", help="render .dev/orpheum.toml and print it")
    seed = sub.add_parser("seed", help="build, then seed the database (empty or not)")
    seed.add_argument("--listen")
    args = ap.parse_args()
    race = os.environ.get("DEV_RACE", "1") != "0"
    if args.command == "config":
        prepare(None)
        print(CONFIG.read_text())
        return 0
    if args.command == "seed":
        prepare(args.listen)
        if not build(race):
            return 1
        return subprocess.run([str(BINARY), "dev", "seed", "--config", str(CONFIG), "--seed",
                               str(SEED)], cwd=ROOT, check=False).returncode
    return loop(with_web=not args.no_web, race=race, listen=args.listen)


if __name__ == "__main__":
    sys.exit(main())
