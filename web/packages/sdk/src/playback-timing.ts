// SPDX-License-Identifier: MPL-2.0

import type { FileSoundOffsetNow, SoundChoice } from "./playback-sound.ts";

/** A choice's or a roster's ms, the server omitting a zero. */
export function msIn(value: unknown): number {
  return typeof value === "number" ? value : 0;
}

/** A remote's choice of the file's own sound timing (D537), its person's own or every person's. */
export function fileSoundOffsetChoiceIn(fields: Readonly<Record<string, unknown>>): SoundChoice {
  return {
    kind: "fileSoundOffset",
    ms: msIn(fields["ms"]),
    ...(fields["isEveryones"] === true ? { isEveryones: true } : {}),
  };
}

/** A roster sound's file offset (D537), as a field to spread into it; none where it says none. */
export function fileSoundOffsetIn(fields: Readonly<Record<string, unknown>> | undefined): {
  readonly fileSoundOffset?: FileSoundOffsetNow;
} {
  const everyones = fields?.["everyonesMs"];
  if (fields === undefined || typeof everyones !== "number") {
    return {};
  }
  const own = fields["ownMs"];
  return {
    fileSoundOffset: { everyonesMs: everyones, ...(typeof own === "number" ? { ownMs: own } : {}) },
  };
}
