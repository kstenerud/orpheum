// SPDX-License-Identifier: MPL-2.0

/**
 * The latency of this device's sound through Web Audio, as the player last measured it, kept in
 * this browser's storage: a later playback here moves its sound by it from its first chunk, so its
 * start throws no sound away. A Fire TV (Silk) measures some 330 ms there, and moving its sound by
 * it as each cast began threw the sound loaded away, a wait of seconds (the owner's casts, staging,
 * 2026-10-28; idiosyncrasies.md, the TCL TV's entry).
 */

import type { SoundLatencyMemory } from "@orpheum/player";

import { browserStorage, type StringStorage } from "./browser-storage.ts";

/** The storage key the latency is kept under (client-keys.ts), in whole ms. */
export const soundLatencyKey = "orpheum.sound.latencyMs";

/** The longest latency kept, in ms: a value beyond it, or below 0, is no latency's, and is not read. */
const longestKeptLatencyMs = 2000;

/** The latency a kept value says, or undefined for none, or for one that is no latency. */
function latencyOf(kept: string | null): number | undefined {
  if (kept === null || !/^\d+$/u.test(kept)) {
    return undefined;
  }
  const ms = Number(kept);
  return ms <= longestKeptLatencyMs ? ms : undefined;
}

/** This device's memory of its sound's latency, in storage, this browser's when not given. */
export function soundLatencyMemory(
  storage: () => StringStorage = browserStorage,
): SoundLatencyMemory {
  return {
    recall: () => latencyOf(storage().getItem(soundLatencyKey)),
    keep: (ms) => {
      if (Number.isInteger(ms) && ms >= 0 && ms <= longestKeptLatencyMs) {
        storage().setItem(soundLatencyKey, String(ms));
      }
    },
  };
}
