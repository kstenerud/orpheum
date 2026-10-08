// SPDX-License-Identifier: MPL-2.0

import { chmodSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";

import { FilmCache } from "./film-cache.ts";

/** An FFmpeg that writes "film" to its last argument and notes each run in runs beside it. */
const fakeFfmpeg = `#!/bin/sh
if [ "$1" = "-version" ]; then echo "ffmpeg version fake"; exit 0; fi
echo run >> "$(dirname "$0")/runs"
for last; do :; done
echo film > "$last"
`;

const scratches: string[] = [];

afterEach(() => {
  for (const scratch of scratches.splice(0)) {
    rmSync(scratch, { recursive: true, force: true });
  }
});

/** A scratch folder holding the fake FFmpeg, and a cache kept in it. */
function scratchWithFakeFfmpeg(): { scratch: string; ffmpeg: string; cache: FilmCache } {
  const scratch = mkdtempSync(path.join(tmpdir(), "film-cache-"));
  scratches.push(scratch);
  const ffmpeg = path.join(scratch, "ffmpeg");
  writeFileSync(ffmpeg, fakeFfmpeg);
  chmodSync(ffmpeg, 0o755);
  return { scratch, ffmpeg, cache: new FilmCache(path.join(scratch, "kept")) };
}

describe("FilmCache", () => {
  it("makes a film asked for twice at once only once, and puts it at both targets", async () => {
    const { scratch, ffmpeg, cache } = scratchWithFakeFfmpeg();
    const targets = [path.join(scratch, "a.mp4"), path.join(scratch, "b.mp4")];
    await Promise.all(targets.map((target) => cache.put(ffmpeg, ["-i", "same"], target)));
    expect(readFileSync(path.join(scratch, "runs"), "utf8")).toBe("run\n");
    for (const target of targets) {
      expect(readFileSync(target, "utf8")).toBe("film\n");
    }
  });

  it("puts a film it keeps without making it again", async () => {
    const { scratch, ffmpeg, cache } = scratchWithFakeFfmpeg();
    await cache.put(ffmpeg, ["-i", "same"], path.join(scratch, "a.mp4"));
    await cache.put(ffmpeg, ["-i", "same"], path.join(scratch, "b.mp4"));
    expect(readFileSync(path.join(scratch, "runs"), "utf8")).toBe("run\n");
    expect(readFileSync(path.join(scratch, "b.mp4"), "utf8")).toBe("film\n");
  });
});
