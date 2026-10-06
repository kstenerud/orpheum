#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: The development loop behind `make dev`: renders the dev config, seeds an empty
# ABOUTME: database, runs the core, the node and the Vite dev server, and restarts both on change.
"""Run Orpheum for development, with opinionated settings that are printed, never hidden.

`make dev` runs `loop`:

1. **Prepare.** Fetch the pinned FFmpeg (`make ffmpeg`) and generate the dev library
   (`make dev-media`).
2. **Build.** `go build -tags dev -race` into `.dev/bin/orpheum`. The dev tag adds the
   `orpheum dev` subcommand, and the race detector is on because two people and a watcher are
   exactly where data races hide. `DEV_RACE=0` turns it off.
3. **Instances.** The core and the node are each an instance of their own (D242): `.dev/core` and
   `.dev/node`, made by `orpheum default-config` the first time. Their full configs are rendered
   from `deploy/dev/core.dev.toml` and `deploy/dev/node.dev.toml` on every start. A `.dev` from
   before D242, with `state/` and `cache/`, is moved into the instances once.
4. **Seed.** If the core has no database, run `orpheum dev seed` with `deploy/dev/seed.toml`,
   rendered into `.dev/seed.toml` like the configs.
5. **Run.** Start `orpheum core` and `orpheum node`, each on its instance, two processes as in
   deployment, where each is a service of its own (D233 §2, D235 §4), and the Vite dev server with
   hot reload, which proxies the API and media to the core. Vite starts only once the first
   build's core accepts connections, so a tab left open waits instead of hitting a proxy with
   nothing behind it. Output goes to the terminal, prefixed
   `[core]`, `[node]` and `[web]`; the core's and the node's are also appended to `.dev/orpheum.log`.
6. **Watch.** Poll the Go sources, `go.mod` and `deploy/dev/*.toml` every second. On a change, rebuild
   and restart the core and the node. The web dev server reloads itself. A process that exits is
   reported with its status, and the loop keeps watching, so a failed build is fixed and retried
   without restarting anything.

Ctrl-C stops everything. Stdlib only, so the loop adds no dependency.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import signal
import socket
import subprocess
import sys
import threading
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEV = ROOT / ".dev"
CORE_TEMPLATE = ROOT / "deploy" / "dev" / "core.dev.toml"
NODE_TEMPLATE = ROOT / "deploy" / "dev" / "node.dev.toml"
CORE = DEV / "core"
NODE = DEV / "node"
INSTANCE_FILE = "orpheum.toml"
FULL_CONFIG = Path("persistent") / "config.toml"
SEED_TEMPLATE = ROOT / "deploy" / "dev" / "seed.toml"
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


# A media directory of your own: ./dev mounts it read-only at its own path, and the dev node is
# given it as one more root, so a new library can choose it (D107: roots are the node's to name).
MEDIA_ENV = "ORPHEUM_DEV_MEDIA"


def with_media_root(config: str, media: str | None) -> str:
    """The config with media first among the node's root_paths; unchanged when media is unset."""
    if not media:
        return config
    return config.replace("root_paths = [", f"root_paths = [{json.dumps(media)}, ", 1)


def listen_address(config: str) -> str:
    for line in config.splitlines():
        if line.strip().startswith("listen"):
            return line.split("=", 1)[1].split("#", 1)[0].strip().strip('"')
    return "127.0.0.1:8432"


def dial_address(address: str) -> tuple[str, int]:
    """Where to reach a core listening on address: a wildcard host is reached on loopback."""
    host, port = address.rsplit(":", 1)
    host = host.strip("[]")
    host = {"0.0.0.0": "127.0.0.1", "::": "::1", "": "127.0.0.1"}.get(host, host)
    return host, int(port)


def wait_until_listening(address: str, timeout: float = 60.0,
                         connect=socket.create_connection, sleep=time.sleep) -> bool:
    """Whether the core accepted a connection on address within timeout seconds.

    Vite starts only then: started first, it proxied every request an open tab made during the Go
    build to nothing and printed an ECONNREFUSED for each, 15-20 s of them after an apply
    (the owner, 2026-10-06)."""
    target = dial_address(address)
    waited = 0.0
    while True:
        try:
            connect(target, timeout=1).close()
            return True
        except OSError:
            if waited >= timeout:
                return False
            sleep(0.25)
            waited += 0.25


def web_host(listen: str | None) -> str:
    """The address Vite binds: the host part of `listen`, so DEV_LISTEN opens both to the LAN."""
    if not listen:
        return "127.0.0.1"
    return listen.rsplit(":", 1)[0].strip("[]")


def lan_ip() -> str:
    """This machine's address on the LAN: the source address of its default route. Connecting a
    UDP socket sends nothing; it only picks the route."""
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("10.255.255.255", 1))
            return s.getsockname()[0]
    except OSError:
        return "127.0.0.1"


def web_url(listen: str | None, lan: str) -> str:
    """Where to open the web client; a wildcard bind is reached at `lan`, the LAN address."""
    host = web_host(listen)
    if host in ("0.0.0.0", "::"):
        host = lan
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
        watched = watched or (path.parent == root / "deploy" / "dev" and path.suffix == ".toml")
        if watched and path.is_file():
            times[rel] = path.stat().st_mtime
    return times


def prepare() -> Path:
    ffmpeg_dir = fetch_ffmpeg.install(fetch_ffmpeg.this_platform(), ROOT)
    dev_media.build(ffmpeg_dir, DEV / "media", seconds=30, force=False)
    DEV.mkdir(exist_ok=True)
    SEED.write_text(render_config(SEED_TEMPLATE.read_text(), ROOT, ffmpeg_dir, None))
    return ffmpeg_dir


# What a .dev from before D242 kept where, and where each goes in the instances: the core's
# database, key and setup code, its backups and cache, and the node's prepared renditions.
OLD_LAYOUT_MOVES = (
    ("state/persistent.db", "core/persistent/persistent.db"),
    ("state/persistent.db-wal", "core/persistent/persistent.db-wal"),
    ("state/persistent.db-shm", "core/persistent/persistent.db-shm"),
    ("state/secret.key", "core/persistent/secret.key"),
    ("state/setup-code", "core/run/setup-code"),
    ("state/backups", "core/backups"),
    ("cache/cache.db", "core/cache/cache.db"),
    ("cache/art", "core/cache/art"),
    ("cache/prepared", "node/cache/prepared"),
)
OLD_LAYOUT_LEFTOVERS = ("state", "cache", "run", "orpheum.toml")


def move_old_layout(dev: Path) -> list[str]:
    """Move a pre-D242 .dev's state into the core and node instances, already made, and remove
    what is left of the old layout. Returns what it moved, for the log."""
    moved = []
    for old, new in OLD_LAYOUT_MOVES:
        source, target = dev / old, dev / new
        if not source.exists():
            continue
        if target.is_dir() and source.is_dir():
            shutil.rmtree(target)  # default-config made it empty
        target.parent.mkdir(parents=True, exist_ok=True)
        source.rename(target)
        moved.append(old)
    for leftover in OLD_LAYOUT_LEFTOVERS:
        path = dev / leftover
        if path.is_dir():
            shutil.rmtree(path)
        elif path.exists():
            path.unlink()
    return moved


def ensure_instances(ffmpeg_dir: Path, listen: str | None) -> None:
    """Make the core and node instances when they are missing, moving a pre-D242 .dev into them,
    and render their full configs from the dev templates."""
    is_old_layout = (DEV / "state").exists() and not (CORE / INSTANCE_FILE).exists()
    for instance, flags in ((CORE, []), (NODE, ["--node"])):
        if not (instance / INSTANCE_FILE).exists():
            subprocess.run([str(BINARY), "default-config", *flags, str(instance)], cwd=ROOT, check=True,
                           stdout=subprocess.DEVNULL)
    if is_old_layout:
        moved = move_old_layout(DEV)
        print(f"[dev] moved the .dev from before D242 into .dev/core and .dev/node: {', '.join(moved)}",
              flush=True)
    core = render_config(CORE_TEMPLATE.read_text(), ROOT, ffmpeg_dir, listen)
    (CORE / FULL_CONFIG).write_text(core)
    node = render_config(NODE_TEMPLATE.read_text(), ROOT, ffmpeg_dir, None)
    (NODE / FULL_CONFIG).write_text(with_media_root(node, os.environ.get(MEDIA_ENV)))


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


# The roles the loop runs, each a process of its own as in deployment (D233 §2, D235 §4).
ROLES = ("core", "node")


def role_commands(binary: Path, dev: Path) -> dict[str, list[str]]:
    """The command that runs each role on its own instance: `orpheum core` on .dev/core and
    `orpheum node` on .dev/node (D242)."""
    return {role: [str(binary), role, "--instance", str(dev / role)] for role in ROLES}


def seed_command(binary: Path) -> list[str]:
    return [str(binary), "dev", "seed", "--instance", str(CORE), "--seed", str(SEED)]


def seed_if_empty() -> None:
    if (CORE / "persistent" / "persistent.db").exists():
        return
    print("[dev] empty database: seeding from deploy/dev/seed.toml", flush=True)
    subprocess.run(seed_command(BINARY), cwd=ROOT, check=False)


def banner(ffmpeg_dir: Path, with_web: bool, race: bool, listen: str | None) -> None:
    address = listen_address(render_config(CORE_TEMPLATE.read_text(), ROOT, ffmpeg_dir, listen))
    lines = [
        "Orpheum dev loop — every setting below is a debugging choice, not a default",
        f"  core         http://{address}   (pprof at /debug/pprof, metrics at /metrics)",
        "  node         its own process beside the core, as in deployment (D235 §4)",
        f"  web (Vite)   {web_url(listen, lan_ip())}   hot reload; proxies /api and /media to the server"
        if with_web else "  web          served by the server (built into the binary)",
        "  users        owner / owner (admin)      partner / partner (outsider)",
        f"  core         {CORE}   (its config from deploy/dev/core.dev.toml)",
        f"  node         {NODE}   (its config from deploy/dev/node.dev.toml)",
        f"  seed         {SEED_TEMPLATE}   (applied to an empty database)",
        f"  media        {DEV / 'media'}   (read-only, generated)",
        *([f"  your media   {os.environ[MEDIA_ENV]}   (read-only; a root a new library can choose)"]
          if os.environ.get(MEDIA_ENV) else []),
        f"  ffmpeg       {ffmpeg_dir}",
        f"  logs         {LOG}",
        f"  race         {'on' if race else 'off (DEV_RACE=0)'}; confinement reported, not required",
        "  rebuilds     on any .go, go.mod or deploy/dev/*.toml change; Ctrl-C stops everything",
    ]
    print("\n".join(lines), flush=True)


def loop(with_web: bool, race: bool, listen: str | None) -> int:
    ffmpeg_dir = prepare()
    banner(ffmpeg_dir, with_web, race, listen)
    # Vite starts once the first build's core listens, so a tab left open never meets a proxy
    # with nothing behind it.
    web = None
    address = listen_address(render_config(CORE_TEMPLATE.read_text(), ROOT, ffmpeg_dir, listen))
    # The core and the node: each run of each, and whether its exit was reported.
    running: dict[str, subprocess.Popen[str]] = {}
    reported: set[str] = set()
    seen = snapshot(ROOT)
    rebuild = True
    try:
        while True:
            if rebuild:
                for proc in running.values():
                    stop(proc)
                running.clear()
                reported.clear()
                if build(race):
                    ensure_instances(ffmpeg_dir, listen)
                    seed_if_empty()
                    for role, command in role_commands(BINARY, DEV).items():
                        running[role] = start(command, f"[{role}]", LOG)
                    if with_web and web is None:
                        if not wait_until_listening(address):
                            print(f"[dev] the core is not listening on {address}; starting the web "
                                  "dev server anyway", flush=True)
                        env = {**os.environ, "ORPHEUM_DEV_SERVER": f"http://{address}"}
                        web = start(["npm", "--prefix", "web", "--workspace", "@orpheum/web", "run",
                                     "dev", "--", "--host", web_host(listen), "--port", "5173",
                                     "--strictPort"], "[web]", env=env)
                rebuild = False
            for role, proc in running.items():
                if proc.poll() is not None and role not in reported:
                    print(f"[dev] the {role} exited with status {proc.returncode}; "
                          "still watching for changes", flush=True)
                    reported.add(role)
            time.sleep(1)
            now = snapshot(ROOT)
            if now != seen:
                seen = now
                rebuild = True
                print("[dev] change detected: rebuilding", flush=True)
    except KeyboardInterrupt:
        print("\n[dev] stopping", flush=True)
    finally:
        for proc in running.values():
            stop(proc)
        stop(web)
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    sub = ap.add_subparsers(dest="command", required=True)
    run = sub.add_parser("loop", help="build, seed, run and watch")
    run.add_argument("--no-web", action="store_true", help="skip the Vite dev server")
    run.add_argument("--listen", help="override the listen address, e.g. 0.0.0.0:8432")
    sub.add_parser("config", help="build, make the dev instances and print their full configs")
    seed = sub.add_parser("seed", help="build, then seed the database (empty or not)")
    seed.add_argument("--listen")
    args = ap.parse_args()
    race = os.environ.get("DEV_RACE", "1") != "0"
    if args.command == "config":
        ffmpeg_dir = prepare()
        if not build(race):
            return 1
        ensure_instances(ffmpeg_dir, None)
        print((CORE / FULL_CONFIG).read_text())
        print((NODE / FULL_CONFIG).read_text())
        return 0
    if args.command == "seed":
        ffmpeg_dir = prepare()
        if not build(race):
            return 1
        ensure_instances(ffmpeg_dir, args.listen)
        return subprocess.run(seed_command(BINARY), cwd=ROOT, check=False).returncode
    return loop(with_web=not args.no_web, race=race, listen=args.listen)


if __name__ == "__main__":
    sys.exit(main())
