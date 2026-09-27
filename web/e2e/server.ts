// SPDX-License-Identifier: MPL-2.0

/**
 * Playwright's global setup: a server of its own, on loopback, in a directory of its own, over a
 * library generated here with the pinned FFmpeg. `make test-browser` builds the binary, with the
 * web client in it, and names it and FFmpeg in the environment. The films are made for this run,
 * one per browser, so no browser plays a copy another already prepared (cut-a §8, M3).
 */

import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { request } from "@playwright/test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import path from "node:path";

/** The people and films the tests use; the spec reads them from here. */
export const owner = { name: "owner", password: "owner-browser-test" } as const;
export const warmUpFilm = "Warm Up Film";

/** The environment variable that carries the server's address from the setup to the spec. */
export const serverAddress = "ORPHEUM_BROWSER_SERVER";

/** The film one browser times, made for it alone. */
export function timingFilm(browser: string): string {
  return `Timing Film ${browser}`;
}

const browsers = ["chromium", "firefox"] as const;

/** A 5.1 source whose centre pulses, as a film's dialogue would. */
const fiveOne =
  "aevalsrc=0.1*sin(2*PI*220*t)|0.1*sin(2*PI*247*t)|0.35*sin(2*PI*1000*t)*mod(floor(t*2)\\,2)|" +
  "0.05*sin(2*PI*45*t)|0.04*sin(2*PI*330*t)|0.04*sin(2*PI*392*t):c=5.1:s=48000";

const ass = `[Script Info]
ScriptType: v4.00+
PlayResX: 1920
PlayResY: 1080

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, OutlineColour, BorderStyle, Outline, Alignment, MarginV
Style: Default,sans-serif,64,&H00FFFFFF,&H00000000,1,3,2,60

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:04.00,Default,,0,0,0,,{\\i1}The timing film{\\i0} carries an ASS track.
`;

function required(name: string): string {
  const value = process.env[name];
  if (value === undefined || value === "") {
    throw new Error(`${name} is not set: run the browser tests with make test-browser`);
  }
  return value;
}

function nfo(title: string, year: number): string {
  return `<?xml version="1.0" encoding="UTF-8"?>\n<movie>\n  <title>${title}</title>\n  <year>${String(year)}</year>\n</movie>\n`;
}

/** Makes one film, in its own folder with an NFO naming it, as a library holds one. */
function makeFilm(
  ffmpeg: string,
  library: string,
  film: { title: string; year: number; file: string; arguments: readonly string[] },
): void {
  const folder = path.join(library, `${film.title} (${String(film.year)})`);
  mkdirSync(folder, { recursive: true });
  writeFileSync(
    path.join(folder, `${film.title} (${String(film.year)}).nfo`),
    nfo(film.title, film.year),
  );
  execFileSync(ffmpeg, [
    "-hide_banner",
    "-loglevel",
    "error",
    "-y",
    ...film.arguments,
    path.join(folder, `${film.title} (${String(film.year)}).${film.file}`),
  ]);
}

/**
 * The film the acceptance names: H.264 at 1080p, TrueHD 5.1, which no browser plays, and an
 * ASS track, a minute and a half long.
 */
function timingFilmArguments(assPath: string, hue: number): string[] {
  return [
    ...["-f", "lavfi", "-i", `testsrc2=size=1920x1080:rate=24:duration=90,hue=h=${String(hue)}`],
    ...["-f", "lavfi", "-i", `${fiveOne}:d=90`],
    ...["-i", assPath],
    ...["-map", "0:v", "-map", "1:a", "-map", "2:s"],
    ...["-c:v", "libx264", "-preset", "veryfast", "-crf", "30", "-pix_fmt", "yuv420p", "-g", "48"],
    ...["-c:a", "truehd", "-strict", "-2", "-c:s", "ass"],
    ...["-metadata:s:a:0", "language=eng", "-metadata:s:s:0", "language=eng"],
  ];
}

/** A short film every browser plays as it is: answering the sound question costs no prepare. */
const warmUpArguments = [
  ...["-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24:duration=10"],
  ...["-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=10"],
  ...["-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "2"],
  ...["-movflags", "+faststart"],
];

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const probe = createServer();
    probe.once("error", reject);
    probe.listen(0, "127.0.0.1", () => {
      const address = probe.address();
      probe.close(() => {
        if (address === null || typeof address === "string") {
          reject(new Error("no port"));
        } else {
          resolve(address.port);
        }
      });
    });
  });
}

function config(place: {
  root: string;
  library: string;
  ffmpegDirectory: string;
  port: number;
}): string {
  const { root, library, ffmpegDirectory, port } = place;
  return `[server]
listen = "127.0.0.1:${String(port)}"
data_dir = "${root}/state"
cache_dir = "${root}/cache"
socket_dir = "${root}/run"

[node]
name = "browser-test"
roots = ["${library}"]
ffmpeg_dir = "${ffmpegDirectory}"
confinement = "report"
prepared_dir = "${root}/cache/prepared"
prepared_budget_gib = 1

[log]
level = "info"
format = "text"
`;
}

function seed(library: string): string {
  return `[[user]]
name = "${owner.name}"
password = "${owner.password}"
role = "admin"

[[library]]
name = "Films"
kind = "movies"
root = "${library}"
metadata_source = "nfo"
change_detection = "watch"
writable = false

[settings]
check_for_updates = false
`;
}

async function waitForHealth(url: string, server: ChildProcess, log: () => string): Promise<void> {
  const deadline = Date.now() + 60_000;
  const client = await request.newContext();
  while (Date.now() < deadline) {
    if (server.exitCode !== null) {
      throw new Error(`the server exited (${String(server.exitCode)}):\n${log()}`);
    }
    try {
      const answer = await client.get(`${url}/api/v1/health`);
      if (answer.ok()) {
        await client.dispose();
        return;
      }
    } catch {
      // not listening yet
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  await client.dispose();
  throw new Error(`the server did not answer /health within 60 s:\n${log()}`);
}

/** Makes the library, starts the server, and returns what stops it and removes everything. */
export default async function globalSetup(): Promise<() => Promise<void>> {
  const binary = required("ORPHEUM_TEST_BINARY");
  const ffmpegDirectory = required("ORPHEUM_TEST_FFMPEG_DIR");
  const ffmpeg = path.join(ffmpegDirectory, "ffmpeg");
  const root = mkdtempSync(path.join(tmpdir(), "orpheum-browser-"));
  const library = path.join(root, "films");
  const assPath = path.join(root, "timing.ass");
  writeFileSync(assPath, ass);
  makeFilm(ffmpeg, library, {
    title: warmUpFilm,
    year: 2020,
    file: "mp4",
    arguments: warmUpArguments,
  });
  browsers.forEach((browser, index) => {
    makeFilm(ffmpeg, library, {
      title: timingFilm(browser),
      year: 2024,
      file: "mkv",
      arguments: timingFilmArguments(assPath, index * 90),
    });
  });

  const port = await freePort();
  const configPath = path.join(root, "orpheum.toml");
  const seedPath = path.join(root, "seed.toml");
  writeFileSync(configPath, config({ root, library, ffmpegDirectory, port }));
  writeFileSync(seedPath, seed(library));
  execFileSync(binary, ["dev", "seed", "--config", configPath, "--seed", seedPath]);

  let log = "";
  const server = spawn(binary, ["run", "--config", configPath], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  const keep = (chunk: Buffer): void => {
    log += chunk.toString();
  };
  server.stdout.on("data", keep);
  server.stderr.on("data", keep);
  const url = `http://127.0.0.1:${String(port)}`;
  // The spec reads the address from here: Playwright hands a global setup's environment to its
  // workers.
  process.env[serverAddress] = url;
  await waitForHealth(url, server, () => log);

  return async () => {
    server.kill("SIGTERM");
    await new Promise((resolve) => server.once("exit", resolve));
    rmSync(root, { recursive: true, force: true });
  };
}
