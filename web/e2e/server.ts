// SPDX-License-Identifier: MPL-2.0

/**
 * The browser tier's servers: each worker starts one of its own (its fixture, browser-map.ts), on
 * loopback, in a directory of its own, over a library generated here with the pinned FFmpeg, so
 * workers run side by side and no test meets another worker's state. `make test-browser` builds the
 * binary, with the web client in it, and names it and FFmpeg in the environment. Each library is
 * new, one timing film per browser, so no browser plays a copy another already prepared (cut-a §8,
 * M3); its films are copies of those the film cache keeps, made once, by Playwright's global setup
 * here, before any worker starts (film-cache.ts).
 */

import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { request } from "@playwright/test";
import { mkdirSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import path from "node:path";

import { filmCache } from "./film-cache.ts";

/** The people and films the tests use; the spec reads them from here. */
export const owner = { name: "owner", password: "owner-browser-test" } as const;
export const warmUpFilm = "Warm Up Film";

/**
 * A film no browser plays from its ladder, which is prepared instead (D260 §1): an MPEG-2 picture no
 * browser decodes, smaller than the lowest quality its chunks offer.
 */
export const preparedFilm = "Prepared Film";

/**
 * Where the last run's server logs are kept, beside the dev loop's state and ignored by git: one per
 * server, named by who started it ("browser-server-chromium-0.log", a worker's).
 */
const serverLogDirectory = new URL("../../.dev/", import.meta.url);

/** The log a server started as name keeps. */
function serverLogPath(name: string): URL {
  return new URL(`browser-server-${name}.log`, serverLogDirectory);
}

/** The environment variable that carries the server's address from the setup to the spec. */
export const serverAddressEnvVariable = "ORPHEUM_BROWSER_SERVER";

/** The film one browser times, made for it alone. */
export function timingFilm(browser: string): string {
  return `Timing Film ${browser}`;
}

const browsers = ["chromium", "firefox"] as const;

/** A 5.1 source whose centre pulses, as a film's dialogue would. */
const fiveOneSourceFilter =
  "aevalsrc=0.1*sin(2*PI*220*t)|0.1*sin(2*PI*247*t)|0.35*sin(2*PI*1000*t)*mod(floor(t*2)\\,2)|" +
  "0.05*sin(2*PI*45*t)|0.04*sin(2*PI*330*t)|0.04*sin(2*PI*392*t):c=5.1:s=48000";

const timingAssText = `[Script Info]
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

function requiredEnv(name: string): string {
  const value = process.env[name];
  if (value === undefined || value === "") {
    throw new Error(`${name} is not set: run the browser tests with make test-browser`);
  }
  return value;
}

function nfoXml(title: string, year: number): string {
  return `<?xml version="1.0" encoding="UTF-8"?>\n<movie>\n  <title>${title}</title>\n  <year>${String(year)}</year>\n</movie>\n`;
}

/** A film a test needs: its name, and how FFmpeg makes it. */
export interface Film {
  readonly title: string;
  readonly year: number;
  readonly extension: string;
  readonly ffmpegInputArguments: readonly string[];
  /** Whether a poster.jpg sits beside it, the local artwork a library reads (no provider is reached). */
  readonly hasPoster?: boolean;
}

/** Makes one film, in its own folder with an NFO naming it, as a library holds one. */
async function makeFilm(ffmpeg: string, library: string, film: Film): Promise<void> {
  const folder = path.join(library, `${film.title} (${String(film.year)})`);
  mkdirSync(folder, { recursive: true });
  writeFileSync(
    path.join(folder, `${film.title} (${String(film.year)}).nfo`),
    nfoXml(film.title, film.year),
  );
  await filmCache.put(
    ffmpeg,
    film.ffmpegInputArguments,
    path.join(folder, `${film.title} (${String(film.year)}).${film.extension}`),
  );
  if (film.hasPoster === true) {
    execFileSync(ffmpeg, [
      ...["-v", "error", "-f", "lavfi", "-i", "color=c=teal:s=200x300"],
      ...["-frames:v", "1", path.join(folder, "poster.jpg")],
    ]);
  }
}

/**
 * The film the acceptance names: H.264 at 1080p, TrueHD 5.1, which no browser plays, and an
 * ASS track, a minute and a half long.
 */
function timingFilmArguments(assPath: string, hueDegrees: number): string[] {
  return [
    ...[
      "-f",
      "lavfi",
      "-i",
      `testsrc2=size=1920x1080:rate=24:duration=90,hue=h=${String(hueDegrees)}`,
    ],
    ...["-f", "lavfi", "-i", `${fiveOneSourceFilter}:d=90`],
    ...["-i", assPath],
    ...["-map", "0:v", "-map", "1:a", "-map", "2:s"],
    ...["-c:v", "libx264", "-preset", "veryfast", "-crf", "30", "-pix_fmt", "yuv420p", "-g", "48"],
    ...["-c:a", "truehd", "-strict", "-2", "-c:s", "ass"],
    ...["-metadata:s:a:0", "language=eng", "-metadata:s:s:0", "language=eng"],
    // A track named as a release is, wider than a phone's screen: the menu must not widen the page.
    ...["-metadata:s:s:0", `title=${longTrackName}`],
  ];
}

/** A subtitle track's name as downloaded subtitles and rips carry them: one unbroken release name. */
export const longTrackName =
  "Timing.Film.2026.Directors.Cut.1080p.BluRay.x264.DTS-HD.MA.5.1-ReleaseGroupName";

/** A short film every browser plays as it is: answering the sound question costs no prepare. */
const preparedFilmArguments = [
  ...["-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24:duration=10"],
  ...["-f", "lavfi", "-i", "sine=frequency=330:sample_rate=48000:duration=10"],
  ...["-c:v", "mpeg2video", "-q:v", "5", "-c:a", "aac", "-ac", "2"],
];

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

/** The core instance's full config (D242): where it listens, and its log. */
function coreConfigToml(port: number): string {
  return `[server]
listen = "127.0.0.1:${String(port)}"

[log]
level = "info"
format = "text"
`;
}

/** The node instance's full config: the core's socket it dials, its media and its FFmpeg. */
function nodeConfigToml(paths: {
  coreSocket: string;
  library: string;
  ffmpegDirectory: string;
}): string {
  const { coreSocket, library, ffmpegDirectory } = paths;
  return `[node]
name = "browser-test"
core_socket = "${coreSocket}"
root_paths = ["${library}"]
ffmpeg_dir = "${ffmpegDirectory}"
confinement_policy = "report"
prepared_budget_gib = 1

[log]
level = "info"
format = "text"
`;
}

function seedToml(library: string): string {
  return `node_name = "browser-test"

[[user]]
name = "${owner.name}"
password = "${owner.password}"
role = "admin"

[[library]]
name = "Films"
kind = "movies"
root_path = "${library}"
metadata_source = "nfo"
change_detection = "watch"
is_writable = false

[settings]
should_check_for_updates = false
`;
}

/** Waits until the core says it is ready, failing if it exits or is not within a minute. */
async function waitUntilReady(
  url: string,
  core: ChildProcess,
  readLog: () => string,
): Promise<void> {
  const deadline = Date.now() + 60_000;
  const client = await request.newContext();
  while (Date.now() < deadline) {
    if (core.exitCode !== null) {
      throw new Error(`the core exited (${String(core.exitCode)}):\n${readLog()}`);
    }
    try {
      // Health answers 200 while the core starts too, so a container manager leaves it be; its
      // status says when the rest answers (a worker's first sign-in was told "migrating", 2026-10-18).
      const answer = await client.get(`${url}/api/v1/health`);
      const health: unknown = answer.ok() ? await answer.json() : undefined;
      if (
        typeof health === "object" &&
        health !== null &&
        Reflect.get(health, "status") === "ready"
      ) {
        await client.dispose();
        return;
      }
    } catch {
      // not listening yet
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  await client.dispose();
  throw new Error(`the core was not ready within 60 s:\n${readLog()}`);
}

/** How a role is started: the binary, the instances, and where its output goes. */
interface Launch {
  readonly binary: string;
  readonly instances: { readonly core: string; readonly node: string };
  readonly appendToLog: (chunk: Buffer) => void;
}

/** Starts one of orpheum's roles on its own instance, its output appended to the log. */
function startRole(
  role: "core" | "node",
  { binary, instances, appendToLog }: Launch,
): ChildProcess {
  const started = spawn(binary, [role, "--instance", instances[role]], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  started.stdout.on("data", appendToLog);
  started.stderr.on("data", appendToLog);
  return started;
}

/**
 * Makes the core's and the node's instances, each of its own (D242), as an admin makes them, with
 * their full configs written for the test.
 */
function makeInstances(paths: {
  binary: string;
  root: string;
  library: string;
  ffmpegDirectory: string;
  port: number;
}): Launch["instances"] {
  const { binary, root, library, ffmpegDirectory, port } = paths;
  const instances = { core: path.join(root, "core"), node: path.join(root, "node") };
  execFileSync(binary, ["default-config", instances.core]);
  execFileSync(binary, ["default-config", "--node", instances.node]);
  writeFileSync(path.join(instances.core, "persistent", "config.toml"), coreConfigToml(port));
  writeFileSync(
    path.join(instances.node, "persistent", "config.toml"),
    nodeConfigToml({
      coreSocket: path.join(instances.core, "run", "core.sock"),
      library,
      ffmpegDirectory,
    }),
  );
  return instances;
}

/**
 * The browser tier's setup: the films every spec uses made once, into the film cache, before the
 * workers start a server each over them (the worker's fixture, browser-map.ts), so no two encode the
 * same film at once. The last run's server logs go, as each server writes its own.
 */
export default async function globalSetup(): Promise<void> {
  const ffmpeg = path.join(requiredEnv("ORPHEUM_TEST_FFMPEG_DIR"), "ffmpeg");
  const root = mkdtempSync(path.join(tmpdir(), "orpheum-browser-films-"));
  try {
    await makeFilms(ffmpeg, path.join(root, "films"), specFilms(root));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
  // The timed pass's setup keeps the first pass's logs, beside which it writes its own.
  if (process.env["ORPHEUM_E2E_TIMING"] === "1") {
    return;
  }
  for (const name of readdirSync(serverLogDirectory)) {
    if (/^browser-server(-.+)?\.log$/u.test(name)) {
      rmSync(new URL(name, serverLogDirectory));
    }
  }
}

/** The films the browser tier's specs use, their subtitles written under root. */
function specFilms(root: string): Film[] {
  const assPath = path.join(root, "timing.ass");
  writeFileSync(assPath, timingAssText);
  return [
    {
      title: warmUpFilm,
      year: 2020,
      extension: "mp4",
      ffmpegInputArguments: warmUpArguments,
      hasPoster: true,
    },
    {
      title: preparedFilm,
      year: 2021,
      extension: "mkv",
      ffmpegInputArguments: preparedFilmArguments,
    },
    ...browsers.map((browser, index) => ({
      title: timingFilm(browser),
      year: 2024,
      extension: "mkv",
      ffmpegInputArguments: timingFilmArguments(assPath, index * 90),
    })),
  ];
}

/** Makes films in library, all at once, each from the film cache where it has it. */
async function makeFilms(ffmpeg: string, library: string, films: readonly Film[]): Promise<void> {
  filmCache.prune();
  await Promise.all(
    films.map(async (film) => {
      await makeFilm(ffmpeg, library, film);
    }),
  );
}

/**
 * Makes the library, with extraFilms beside the films every spec uses, or alone when
 * shouldMakeSpecFilms is false, as for the acceptance, which plays none of them (the test audit, 2026-10-08); starts
 * the server — the core, then the node beside it, two services as in deployment (D235 §4) — and
 * returns what stops both, keeps its log as logName's, and removes everything.
 */
export async function startServer(
  extraFilms: readonly Film[],
  shouldMakeSpecFilms = true,
  logName = "acceptance",
): Promise<() => Promise<void>> {
  const binary = requiredEnv("ORPHEUM_TEST_BINARY");
  const ffmpegDirectory = requiredEnv("ORPHEUM_TEST_FFMPEG_DIR");
  const ffmpeg = path.join(ffmpegDirectory, "ffmpeg");
  const root = mkdtempSync(path.join(tmpdir(), "orpheum-browser-"));
  const library = path.join(root, "films");
  await makeFilms(ffmpeg, library, [
    ...(shouldMakeSpecFilms ? specFilms(root) : []),
    ...extraFilms,
  ]);

  const port = await freePort();
  const instances = makeInstances({ binary, root, library, ffmpegDirectory, port });
  const seedPath = path.join(root, "seed.toml");
  writeFileSync(seedPath, seedToml(library));
  execFileSync(binary, ["dev", "seed", "--instance", instances.core, "--seed", seedPath]);

  let log = "";
  const appendToLog = (chunk: Buffer): void => {
    log += chunk.toString();
  };
  const launch: Launch = { binary, instances, appendToLog };
  const core = startRole("core", launch);
  const url = `http://127.0.0.1:${String(port)}`;
  // The spec reads the address from here: in a worker, the server its fixture started.
  process.env[serverAddressEnvVariable] = url;
  await waitUntilReady(url, core, () => log);
  // The node dials the core's socket, which exists once the core answers: started now, its first
  // hello does not wait for a status tick.
  const node = startRole("node", launch);
  // A key the core does not read is a key this config names wrongly, as after a rename (D227):
  // the tests would run on its starting value, not the one written here.
  const unread = log.split("\n").filter((line) => line.includes("nothing reads it"));
  if (unread.length > 0) {
    node.kill("SIGTERM");
    core.kill("SIGTERM");
    throw new Error(`the test config names keys the core does not read:\n${unread.join("\n")}`);
  }

  return async () => {
    const exited = [node, core].map(
      (started) => new Promise((resolve) => started.once("exit", resolve)),
    );
    node.kill("SIGTERM");
    core.kill("SIGTERM");
    await Promise.all(exited);
    // The server's log outlives the run, beside its results: a failure is read from it.
    writeFileSync(serverLogPath(logName), log);
    rmSync(root, { recursive: true, force: true });
  };
}
