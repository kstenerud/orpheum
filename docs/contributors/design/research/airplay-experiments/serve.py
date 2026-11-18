# ABOUTME: Serves the AirPlay experiment pages and their test stream, and keeps each page's log,
# which the pages send every 3 s, under logs/ (airplay-2026-11-18-research.md). Run: python3 serve.py
"""Serves this folder on port 8000 and writes each page run's log to logs/<page>-<time>.txt."""

import http.server
import pathlib
import re
import socket
import urllib.parse

FOLDER = pathlib.Path(__file__).resolve().parent
LOGS = FOLDER / "logs"


class Handler(http.server.SimpleHTTPRequestHandler):
    """Serves the folder; a POST to /log?run=<name> replaces that run's log file."""

    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(FOLDER), **kwargs)

    def do_POST(self):  # noqa: N802 (the standard library's name)
        query = urllib.parse.parse_qs(urllib.parse.urlparse(self.path).query)
        run = re.sub(r"[^A-Za-z0-9_.-]", "_", query.get("run", ["unnamed"])[0])
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        LOGS.mkdir(exist_ok=True)
        (LOGS / f"{run}.txt").write_bytes(body)
        self.send_response(204)
        self.end_headers()

    def log_message(self, format, *args):  # noqa: A002 (the standard library's name)
        if "POST /log" not in str(args[0] if args else ""):
            super().log_message(format, *args)


if __name__ == "__main__":
    probe = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    probe.connect(("192.0.2.1", 9))  # no packet is sent: it only picks the LAN address
    print(f"On the iPhone, open http://{probe.getsockname()[0]}:8000/")
    probe.close()
    print(f"Logs are kept in {LOGS}")
    http.server.ThreadingHTTPServer(("", 8000), Handler).serve_forever()
