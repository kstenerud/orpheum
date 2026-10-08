// SPDX-License-Identifier: MPL-2.0

/**
 * The browser tier's films, made once and kept: every argument that makes one is fixed, so a film
 * made with the same FFmpeg from the same arguments is the same film. Each 1080p timing film took
 * some 20 s of FFmpeg on every run, before any test ran (the test audit, 2026-10-08).
 */

import { execFile, execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  constants,
  copyFileSync,
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  renameSync,
  rmSync,
  statSync,
  utimesSync,
} from "node:fs";
import path from "node:path";
import { promisify } from "node:util";

/** Where the films are kept: in the checkout's ignored .dev/, which outlives the dev container. */
const cacheDirectory = new URL("../../.dev/browser-films/", import.meta.url).pathname;

/** How long a film no run has used is kept, in days: a changed film's old copy goes after this. */
const keptForDays = 14;

const run = promisify(execFile);

/** FFmpeg's version line, read once a run: another FFmpeg may make another film. */
let versionLine: string | undefined;

function ffmpegVersion(ffmpeg: string): string {
  versionLine ??= execFileSync(ffmpeg, ["-version"]).toString().split("\n")[0] ?? "";
  return versionLine;
}

/**
 * The key a film is kept under: FFmpeg's version and the arguments, a file among them by its
 * contents, since its path is a new temporary one each run.
 */
function filmKey(ffmpeg: string, ffmpegArguments: readonly string[]): string {
  const hash = createHash("sha256").update(ffmpegVersion(ffmpeg));
  for (const argument of ffmpegArguments) {
    const isFile = path.isAbsolute(argument) && existsSync(argument);
    hash.update(
      isFile ? createHash("sha256").update(readFileSync(argument)).digest("hex") : argument,
    );
    hash.update("\0");
  }
  return hash.digest("hex").slice(0, 32);
}

/** Removes the films no run has used for keptForDays. */
export function pruneFilmCache(): void {
  if (!existsSync(cacheDirectory)) {
    return;
  }
  const oldestMs = Date.now() - keptForDays * 24 * 60 * 60 * 1000;
  for (const name of readdirSync(cacheDirectory)) {
    const file = path.join(cacheDirectory, name);
    if (statSync(file).mtimeMs < oldestMs) {
      rmSync(file, { force: true });
    }
  }
}

/**
 * Puts at target the film FFmpeg makes from ffmpegArguments: the kept one when there is one, else
 * made, kept, and then put there.
 */
export async function makeFilmFile(
  ffmpeg: string,
  ffmpegArguments: readonly string[],
  target: string,
): Promise<void> {
  const extension = path.extname(target);
  const kept = path.join(cacheDirectory, `${filmKey(ffmpeg, ffmpegArguments)}${extension}`);
  if (existsSync(kept)) {
    const now = new Date();
    utimesSync(kept, now, now);
  } else {
    // Two films of one run made from the same arguments are one film, made once: made twice at
    // once under one name, the first renamed it away and the second's FFmpeg failed (2026-10-08).
    let making = makings.get(kept);
    if (making === undefined) {
      making = makeKept(ffmpeg, ffmpegArguments, kept).finally(() => {
        makings.delete(kept);
      });
      makings.set(kept, making);
    }
    await making;
  }
  copyFileSync(kept, target, constants.COPYFILE_FICLONE);
}

/** The films this process is making, each by where it is kept, until it is kept. */
const makings = new Map<string, Promise<void>>();

/** Makes at kept the film FFmpeg makes from ffmpegArguments. */
async function makeKept(
  ffmpeg: string,
  ffmpegArguments: readonly string[],
  kept: string,
): Promise<void> {
  mkdirSync(cacheDirectory, { recursive: true });
  // Made under another name, so a run stopped halfway keeps no half film.
  const making = `${kept}.${String(process.pid)}.making${path.extname(kept)}`;
  await run(ffmpeg, ["-hide_banner", "-loglevel", "error", "-y", ...ffmpegArguments, making]);
  renameSync(making, kept);
}
