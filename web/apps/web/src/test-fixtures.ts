// SPDX-License-Identifier: MPL-2.0

import type { Device } from "./accounts.ts";
import type { PlaybackDecision } from "./watching.ts";

/** A sound not measured yet: played as it is (D226). */
export const unlevelled = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };

/** A decision to play the original as it is, with what a test changes of it. */
export function directDecision(changes: Partial<PlaybackDecision> = {}): PlaybackDecision {
  return {
    delivery: "direct",
    url: "/api/v1/bytes/media/direct/abc",
    notDirectReasons: [],
    isPreparing: false,
    subtitles: [],
    audioTracks: [],
    preferences: { subtitleOffsetMs: 0, isDialogueBoostOn: false, isLevellingOn: true },
    resumePositionMs: 0,
    mediaToken: "t",
    levelling: unlevelled.levelling,
    fileBitRateKbps: 2400,
    ...changes,
  };
}

/** Another of the person's devices, the living room's television, with what a test changes of it. */
export function television(changes: Partial<Device> = {}): Device {
  return {
    id: 7,
    name: "Living room TV",
    platform: "web",
    addedAt: 1,
    lastUsedAt: 1,
    isSignedIn: true,
    isThisDevice: false,
    ...changes,
  };
}
