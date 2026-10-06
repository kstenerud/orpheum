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
  aimAheadAtMostMs,
  checkDriftEveryMs,
  scrubPauseWindowMs,
  driftCorrection,
  expectedPositionMs,
  fastestNudgeFactor,
  followGroup,
  fellBehindEvent,
  leaveDriftUpToMs,
  playerMovedItselfEvent,
  refillAheadMs,
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
  networkState = 2; // NETWORK_LOADING
  duration = 7200;
  /** What is loaded, as [start, end] in seconds: all of it unless a test says otherwise. */
  loaded: [number, number][] = [[0, 7200]];
  readonly listeners = new Map<string, () => void>();

  get buffered(): Pick<TimeRanges, "length" | "start" | "end"> {
    const { loaded } = this;
    return {
      length: loaded.length,
      start: (index) => loaded[index]?.[0] ?? 0,
      end: (index) => loaded[index]?.[1] ?? 0,
    };
  }

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

  /** The events fired at the element by its page, by type. */
  readonly dispatched: string[] = [];

  dispatchEvent(event: Event): boolean {
    this.dispatched.push(event.type);
    this.fire(event.type);
    return true;
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
    media.fire("seeking"); // the follower's own seek to the group's position
    media.fire("seeked");
    expect(commands).toEqual([]);
    media.paused = false;
    media.fire("play");
    expect(commands).toEqual([{ command: "play" }]);
    setGroup({ ...paused, state: "Playing" }, 0);
    media.paused = true;
    media.fire("pause");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands.at(-1)).toEqual({ command: "pause" });
    setGroup({ ...paused, state: "Paused" }, 0);
    media.currentTime = 600;
    media.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands.at(-1)).toEqual({ command: "seek", positionMs: 600_000 });
  });

  it("takes the host's player stepping a fraction of a second over a gap for no seek", () => {
    const media = new FakeMedia();
    const { connection, commands } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: true,
      monotonicMs,
    });
    media.fire("seeked"); // the follower's own seek to the group's position
    media.currentTime = 60.5; // hls.js steps over a gap at the start of what it loaded
    media.seeking = true;
    media.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([]);
  });

  it("takes a seek its player says it made itself, a step over a gap however long, for no seek", () => {
    const media = new FakeMedia();
    const { connection, commands } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: true,
      monotonicMs,
    });
    media.fire("seeked"); // the follower's own seek to the group's position
    media.currentTime = 66; // hls.js steps over a six-second gap in what it loaded
    media.fire(playerMovedItselfEvent);
    media.seeking = true;
    media.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([]);
  });

  it("sends the host's seek as its person makes it, before the place has loaded, once per drag", () => {
    const media = new FakeMedia();
    const { connection, commands, reports } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: true,
      monotonicMs,
    });
    media.fire("seeked"); // the follower's own seek to the group's position
    const reportsBefore = reports.length;
    // A drag: a seek at each step, and the last place never loads while the test runs.
    for (const positionS of [300, 900, 1500]) {
      media.currentTime = positionS;
      media.seeking = true;
      media.fire("seeking");
    }
    // Its loading is not told before the seek, or the group would take it for a stall.
    expect(reports).toHaveLength(reportsBefore);
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([{ command: "seek", positionMs: 1_500_000 }]);
    expect(reports.at(-1)).toMatchObject({ positionMs: 1_500_000, isBuffering: true });
    expect(media.currentTime).toBe(1500); // left where its person put it, not pulled back
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

  it("takes a pause the browser makes while its person drags the timeline for the seek it is", () => {
    const media = new FakeMedia();
    const { connection, commands } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: true,
      monotonicMs,
    });
    media.fire("seeked"); // the follower's own seek to the group's position
    media.paused = true;
    media.fire("pause");
    media.currentTime = 5400;
    media.fire("seeking");
    media.fire("seeked");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([{ command: "seek", positionMs: 5_400_000 }]);
  });

  it("after a stall holds the group up until it has loaded enough ahead to play on", () => {
    const media = new FakeMedia();
    const { connection, reports } = fakeConnection(paused);
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    media.fire("seeked"); // caught up where the group is
    expect(reports.at(-1)?.isBuffering).toBe(false);
    media.loaded = [[60, 60.5]];
    media.readyState = 2;
    media.fire("waiting");
    expect(reports.at(-1)?.isBuffering).toBe(true);
    media.loaded = [[60, 61]];
    media.readyState = 3;
    media.fire("canplay");
    expect(reports.at(-1)?.isBuffering).toBe(true);
    const reportsBefore = reports.length;
    media.fire("progress");
    expect(reports).toHaveLength(reportsBefore); // loading that changes nothing tells no one
    media.loaded = [[60, 60 + refillAheadMs / 1000]];
    media.fire("progress");
    expect(reports.at(-1)?.isBuffering).toBe(false);
  });

  it("after a stall plays on with what it has once the browser stops loading, or the film ends", () => {
    const media = new FakeMedia();
    const { connection, reports } = fakeConnection(paused);
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    media.fire("seeked");
    media.loaded = [[60, 61]];
    media.fire("waiting");
    expect(reports.at(-1)?.isBuffering).toBe(true);
    media.networkState = 1; // NETWORK_IDLE: the browser loads no more while paused
    media.fire("progress");
    expect(reports.at(-1)?.isBuffering).toBe(false);
    media.networkState = 2;
    media.duration = 61;
    media.fire("waiting");
    media.fire("canplay");
    expect(reports.at(-1)?.isBuffering).toBe(false);
  });

  it("tells a waiting group its state each check, so the server can end the wait on time", () => {
    const media = new FakeMedia();
    const { connection, reports, setGroup } = fakeConnection({ ...paused, state: "Waiting" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    const reportsBefore = reports.length;
    vi.advanceTimersByTime(checkDriftEveryMs * 3);
    expect(reports.length - reportsBefore).toBe(3);
    setGroup({ ...paused, state: "Paused" }, 0);
    const reportsPaused = reports.length;
    vi.advanceTimersByTime(checkDriftEveryMs * 3);
    expect(reports).toHaveLength(reportsPaused);
  });

  it("corrects nothing while it is still loading, which a correction would only start again", () => {
    const media = new FakeMedia();
    media.readyState = 2;
    const { connection } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    monotonicNowMs = 30_000;
    vi.advanceTimersByTime(checkDriftEveryMs * 3);
    expect(media.currentTime).toBe(0);
    expect(media.paused).toBe(false);
  });

  it("waiting, seeks once to the group's position, not again as the browser moves it while it loads", () => {
    const media = new FakeMedia();
    media.readyState = 1;
    const { connection } = fakeConnection({ ...paused, state: "Waiting" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    expect(media.currentTime).toBe(60);
    // The browser's player steps over a gap at the start of what it loads: a little later.
    media.currentTime = 60.4;
    media.readyState = 2;
    media.fire("seeked");
    vi.advanceTimersByTime(checkDriftEveryMs * 3);
    expect(media.currentTime).toBe(60.4);
  });

  it("says it fell behind as it seeks to catch up with the group, once it has been in step", () => {
    const media = new FakeMedia();
    media.readyState = 4;
    const { connection, setGroup } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    // Joining a group already playing elsewhere is no falling behind.
    expect(media.dispatched).toEqual([]);
    media.fire("seeked");
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(media.dispatched).toEqual([]);
    setGroup({ ...paused, state: "Playing", positionMs: 120_000 }, 0);
    expect(media.dispatched).toEqual([fellBehindEvent]);
  });

  it("held paused, waits at the start of what it loaded when the group's place falls just before it", () => {
    const media = new FakeMedia();
    media.currentTime = 60;
    media.loaded = [[60.4, 120]]; // a copied picture whose first loaded frame comes a moment late
    media.readyState = 1;
    const { connection, reports } = fakeConnection({ ...paused, state: "Waiting" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    expect(media.currentTime).toBe(60.4);
    media.readyState = 4;
    media.fire("seeked");
    vi.advanceTimersByTime(checkDriftEveryMs * 3);
    expect(media.currentTime).toBe(60.4); // held there, not sought back into the gap
    expect(reports.at(-1)?.isBuffering).toBe(false);
  });

  it("aims a catch-up seek ahead by how long its last seek took to load, up to a limit", () => {
    const media = new FakeMedia();
    const { connection } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media as unknown as FollowedMedia, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    expect(media.currentTime).toBe(60); // its first seek, with no load timed yet
    media.readyState = 2;
    media.fire("seeking");
    monotonicNowMs = 2000;
    media.currentTime = 0; // it fell behind while it loaded
    media.readyState = 4;
    media.fire("canplay");
    expect(media.currentTime).toBe(62 + 2);
    media.readyState = 2;
    media.fire("seeking");
    monotonicNowMs = 2000 + 3 * aimAheadAtMostMs;
    media.currentTime = 0;
    media.readyState = 4;
    media.fire("canplay");
    expect(media.currentTime).toBe((monotonicNowMs + 60_000 + aimAheadAtMostMs) / 1000);
  });
});
