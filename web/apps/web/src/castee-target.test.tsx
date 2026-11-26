// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { everyRight, startingTuning, type TogetherSnapshot } from "@orpheum/sdk";
import { act, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { forgetCastDeviceVolume, tellCastDeviceVolume } from "./cast-device-volume.ts";
import type { CastDeviceVolume } from "./cast-sdk.ts";
import {
  useCasteeTarget,
  type CasteeOf,
  type CasteeTarget,
  type WorkedSession,
} from "./castee-target.ts";
import { mount, type Mounted } from "./test-dom.tsx";

/** The castee's say of how it plays: its film's level, a sound track and a subtitle. */
const playback = {
  delivery: "converted",
  sound: {
    isLevellingOn: true,
    audioTracks: [],
    channels: [{ channel: "film", level: 0.5, isMuted: false }],
  },
  picture: { subtitles: [], canPlayOriginal: false },
} as const;

/** The cast's session, the castee leading it, its film paused at 1:01. */
const snapshot = {
  status: "connected",
  memberId: 1,
  deviceId: 7,
  isHost: true,
  rights: everyRight,
  group: {
    state: "paused",
    positionMs: 61_000,
    rate: 1,
    fileId: 4,
    event: undefined,
    seekedFromMs: 0,
  },
  groupReceivedAtMs: 0,
  groupPlacedByRttMs: undefined,
  roster: [
    {
      id: 2,
      name: "owner",
      isHost: true,
      devices: [
        {
          id: 6,
          role: "video",
          isBuffering: false,
          cannotKeepUp: undefined,
          status: undefined,
          isLeader: true,
          playback,
        },
      ],
    },
  ],
  endReason: undefined,
  minimumRttMs: undefined,
  lastRejectionCode: undefined,
  answersHeard: 0,
  tuning: startingTuning,
} as unknown as TogetherSnapshot;

/** A connection whose commands and choices are heard. */
function heardConnection(): NonNullable<WorkedSession["connection"]> & {
  readonly command: ReturnType<typeof vi.fn>;
  readonly chooseSound: ReturnType<typeof vi.fn>;
} {
  return {
    command: vi.fn(),
    chooseSound: vi.fn(),
    choosePicture: vi.fn(),
  } as unknown as NonNullable<WorkedSession["connection"]> & {
    readonly command: ReturnType<typeof vi.fn>;
    readonly chooseSound: ReturnType<typeof vi.fn>;
  };
}

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  forgetCastDeviceVolume(9);
});

/** A Google Cast device's volume, as its Cast session says it, keeping what was set on it. */
function deviceVolume(): CastDeviceVolume & {
  readonly set: string[];
  readonly changeTo: (level: number, isMuted: boolean) => void;
} {
  let level = 0.4;
  let isMuted = false;
  const set: string[] = [];
  const listeners = new Set<() => void>();
  return {
    set,
    read: () => ({ level, isMuted }),
    setLevel: (to) => set.push(`level ${String(to)}`),
    setMuted: (isMutedNow) => set.push(`muted ${String(isMutedNow)}`),
    onChange: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    changeTo: (toLevel, isMutedNow) => {
      level = toLevel;
      isMuted = isMutedNow;
      for (const listener of listeners) {
        listener();
      }
    },
  };
}

/** The castee's target as made of castee, read after each render. */
async function targetOf(castee: CasteeOf): Promise<() => CasteeTarget> {
  let latest: CasteeTarget | undefined;
  function Probe(): JSX.Element {
    latest = useCasteeTarget(castee);
    return <span />;
  }
  mounted = await mount(<Probe />);
  return () => {
    if (latest === undefined) {
      throw new Error("the probe did not render");
    }
    return latest;
  };
}

describe("the castee as the playback target (D555 §8)", () => {
  it("plays and pauses through the session, stands where the session says, and is ready once the castee is heard", async () => {
    const connection = heardConnection();
    const session = { snapshot, connection };
    const target = await targetOf({
      worked: session,
      cast: session,
      sent: { castId: 9, fileId: 4, durationMs: 600_000 },
      isHeard: true,
    });
    expect(target()).toMatchObject({
      kind: "castee",
      atMs: 61_000,
      reportedAtMs: 61_000,
      durationMs: 600_000,
      isPaused: true,
      isReady: true,
    });
    await act(async () => {
      target().playOrPause();
      await Promise.resolve();
    });
    expect(connection.command).toHaveBeenCalledWith({ command: "play" });
  });

  it("shows a seek chosen at once, its reported place left where the castee said until the castee says another (the rework plan, phase 3)", async () => {
    const connection = heardConnection();
    const session = { snapshot, connection };
    const target = await targetOf({
      worked: session,
      cast: session,
      sent: { castId: 9, fileId: 4, durationMs: 600_000 },
      isHeard: true,
    });
    await act(async () => {
      target().seekTo(300_000);
      await Promise.resolve();
    });
    expect(target().atMs).toBe(300_000);
    expect(target().reportedAtMs).toBe(61_000);
  });

  it("takes its film's sound and its choices from the cast's session, the castee's say, and sends a choice there", async () => {
    const worked = { snapshot, connection: heardConnection() };
    const castConnection = heardConnection();
    const target = await targetOf({
      worked,
      cast: { snapshot, connection: castConnection },
      sent: { castId: 9, fileId: 4, durationMs: 600_000 },
      isHeard: true,
    });
    expect(target().sound).toMatchObject({ volume: 0.5, isMuted: false, canSetVolume: true });
    expect(target().choices).toMatchObject({
      kind: "castee",
      playback,
      report: { fileId: 4, snapshot },
    });
    await act(async () => {
      target().sound?.setMuted(true);
      await Promise.resolve();
    });
    expect(castConnection.chooseSound).toHaveBeenCalledWith({
      kind: "channel",
      channel: "film",
      level: 0.5,
      isMuted: true,
    });
  });

  // The pad showed no volume for a Chromecast, which says no film channel of its own (the owner,
  // 2026-11-26: "Anything it can do, I want").
  it("takes a Google Cast device's own volume where the castee says no film channel, set through its Cast session", async () => {
    const silent: TogetherSnapshot = {
      ...snapshot,
      roster: snapshot.roster.map((member) => ({
        ...member,
        devices: member.devices.map((device) => ({ ...device, playback: undefined })),
      })),
    };
    const volume = deviceVolume();
    tellCastDeviceVolume(9, volume);
    const session = { snapshot: silent, connection: heardConnection() };
    const target = await targetOf({
      worked: session,
      cast: session,
      sent: { castId: 9, fileId: 4, durationMs: 600_000 },
      isHeard: true,
    });
    expect(target().sound).toMatchObject({ volume: 0.4, isMuted: false, canSetVolume: true });
    target().sound?.setVolume(0.7);
    target().sound?.setMuted(true);
    expect(volume.set).toEqual(["level 0.7", "muted true"]);
    // The TV's own remote turned it down: the pad follows.
    await act(async () => {
      volume.changeTo(0.2, true);
      await Promise.resolve();
    });
    expect(target().sound).toMatchObject({ volume: 0.2, isMuted: true });
    // Its Cast session over, the page has no say of the device's volume.
    await act(async () => {
      forgetCastDeviceVolume(9);
      await Promise.resolve();
    });
    expect(target().sound).toBeUndefined();
  });

  it("takes no press while the castee is unheard, and says no sound where the castee says none (a Google Cast device's today)", async () => {
    const silent: TogetherSnapshot = {
      ...snapshot,
      roster: snapshot.roster.map((member) => ({
        ...member,
        devices: member.devices.map((device) => ({ ...device, playback: undefined })),
      })),
    };
    const session = { snapshot: silent, connection: heardConnection() };
    const target = await targetOf({
      worked: session,
      cast: session,
      sent: { castId: 9, fileId: 4, durationMs: 600_000 },
      isHeard: false,
    });
    expect(target().isReady).toBe(false);
    expect(target().sound).toBeUndefined();
    expect(target().choices.playback).toBeUndefined();
  });
});
