// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { GroupView } from "./together.ts";
import type {
  GroupCommand,
  PlayerState,
  TogetherConnection,
  TogetherSnapshot,
} from "./together-connection.ts";
import {
  checkDriftEveryMs,
  driftCorrection,
  expectedPositionMs,
  fastestNudgeFactor,
  followGroup,
  leaveDriftUpToMs,
  seekDriftBeyondMs,
  type FollowedMedia,
} from "./together-follow.ts";

describe("where the group is (D66: no clock sync)", () => {
  const playing: GroupView = {
    state: "Playing",
    positionMs: 10_000,
    rate: 1.5,
    fileId: 1,
    event: undefined,
    seekedFromMs: 0,
    byMemberId: undefined,
  };

  it("moves on at its rate from when its message arrived while it plays", () => {
    expect(expectedPositionMs(playing, 500, 1500)).toBe(11_500);
  });

  it("stays where it was told while it does not play", () => {
    expect(expectedPositionMs({ ...playing, state: "Waiting" }, 500, 1500)).toBe(10_000);
  });
});

describe("correcting drift (watch-together.md, Sync)", () => {
  it("leaves a drift within the threshold alone", () => {
    expect(
      driftCorrection({ expectedMs: 1000, actualMs: 1000 - leaveDriftUpToMs, groupRate: 1 }),
    ).toEqual({ kind: "none" });
  });

  it("nudges the rate to close a drift just past the threshold within about a second", () => {
    const behind = driftCorrection({ expectedMs: 1100, actualMs: 1000, groupRate: 1 });
    expect(behind.kind).toBe("nudge");
    expect(behind.kind === "nudge" ? behind.rate : 0).toBeCloseTo(1.1);
    const ahead = driftCorrection({ expectedMs: 1000, actualMs: 1200, groupRate: 1 });
    expect(ahead.kind === "nudge" ? ahead.rate : 0).toBeCloseTo(0.8);
  });

  it("nudges no faster than the fastest factor of the group's rate", () => {
    expect(driftCorrection({ expectedMs: 3000, actualMs: 100, groupRate: 1.25 })).toEqual({
      kind: "nudge",
      rate: 1.25 * fastestNudgeFactor,
    });
  });

  it("seeks across a drift beyond the seek threshold", () => {
    expect(
      driftCorrection({ expectedMs: 9000, actualMs: 9000 - seekDriftBeyondMs - 1, groupRate: 1 }),
    ).toEqual({ kind: "seek", toMs: 9000 });
  });
});

/** A stand-in video element: its position, play state and events, as a test sets them. */
class FakeMedia {
  currentTime = 0;
  playbackRate = 1;
  /* eslint-disable @typescript-eslint/naming-convention -- HTMLMediaElement's own names, which this stands in for */
  paused = true;
  ended = false;
  seeking = false;
  /* eslint-enable @typescript-eslint/naming-convention -- the fake's own names follow */
  readyState = 4;
  readonly listeners = new Map<string, () => void>();

  play(): Promise<void> {
    this.paused = false;
    return Promise.resolve();
  }

  pause(): void {
    this.paused = true;
  }

  addEventListener(type: string, listener: () => void): void {
    this.listeners.set(type, listener);
  }

  removeEventListener(type: string): void {
    this.listeners.delete(type);
  }

  /** The browser firing an event at the element. */
  fire(type: string): void {
    this.listeners.get(type)?.();
  }
}

/** A stand-in connection whose group a test sets, keeping the commands and reports it is given. */
function fakeConnection(group: GroupView): {
  connection: TogetherConnection;
  setGroup: (next: GroupView, receivedAtMs: number) => void;
  commands: GroupCommand[];
  reports: PlayerState[];
} {
  let snapshot: TogetherSnapshot = {
    status: "connected",
    memberId: 1,
    isHost: false,
    group,
    groupReceivedAtMs: 0,
    roster: [],
    endReason: undefined,
    minimumRttMs: undefined,
    lastRejectionCode: undefined,
  };
  let listener = (): void => undefined;
  let readPlayer: (() => PlayerState) | undefined;
  const commands: GroupCommand[] = [];
  const reports: PlayerState[] = [];
  return {
    commands,
    reports,
    setGroup: (next, receivedAtMs) => {
      snapshot = { ...snapshot, group: next, groupReceivedAtMs: receivedAtMs };
      listener();
    },
    connection: {
      snapshot: () => snapshot,
      subscribe: (heard) => {
        listener = heard;
        return () => undefined;
      },
      setPlayer: (read) => {
        readPlayer = read;
      },
      reportState: () => {
        const state = readPlayer?.();
        if (state !== undefined) {
          reports.push(state);
        }
      },
      command: (command) => {
        commands.push(command);
      },
      open: () => undefined,
      close: () => undefined,
    },
  };
}

const paused: GroupView = {
  state: "Paused",
  positionMs: 60_000,
  rate: 1,
  fileId: 5,
  event: undefined,
  seekedFromMs: 0,
  byMemberId: undefined,
};

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 0;
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a player following its group", () => {
  it("waits paused at the group's position, and plays in step once the group plays", () => {
    const media = new FakeMedia();
    const { connection, setGroup } = fakeConnection(paused);
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    expect(media.paused).toBe(true);
    expect(media.currentTime).toBe(60);
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000);
    expect(media.paused).toBe(false);
    monotonicNowMs = 1000 + checkDriftEveryMs;
    // The element did not move on: a second behind, it is nudged faster.
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(media.playbackRate).toBeGreaterThan(1);
  });

  it("does not follow a group playing another film", () => {
    const media = new FakeMedia();
    const { connection } = fakeConnection({ ...paused, fileId: 6 });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    expect(media.currentTime).toBe(0);
  });

  it("counts as buffering until it has once been where the group is (D246 §6)", () => {
    const media = new FakeMedia();
    media.readyState = 1;
    const { connection, reports } = fakeConnection(paused);
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    expect(reports.at(-1)?.isBuffering).toBe(true);
    media.readyState = 4;
    media.fire("seeked");
    expect(reports.at(-1)).toEqual({ positionMs: 60_000, isPlaying: false, isBuffering: false });
  });

  it("turns the host's own play, pause and seek into commands, but not its own following", () => {
    const media = new FakeMedia();
    const { connection, setGroup, commands } = fakeConnection(paused);
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: true,
      monotonicMs,
    });
    media.fire("seeked"); // the follower's own seek to the group's position
    expect(commands).toEqual([]);
    media.paused = false;
    media.fire("play");
    expect(commands).toEqual([{ command: "play" }]);
    setGroup({ ...paused, state: "Playing" }, 0);
    media.paused = true;
    media.fire("pause");
    expect(commands.at(-1)).toEqual({ command: "pause" });
    setGroup({ ...paused, state: "Paused" }, 0);
    media.currentTime = 600;
    media.fire("seeked");
    expect(commands.at(-1)).toEqual({ command: "seek", positionMs: 600_000 });
  });

  it("pauses the host's video while the group waits for a viewer, and sends no command", () => {
    const media = new FakeMedia();
    const { connection, setGroup, commands } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: true,
      monotonicMs,
    });
    expect(media.paused).toBe(false);
    setGroup({ ...paused, state: "Waiting", event: "buffering", byMemberId: 2 }, 0);
    expect(media.paused).toBe(true);
    media.fire("pause");
    expect(commands).toEqual([]);
  });
});
