// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { startingTuning, type GroupView } from "./together.ts";
import type { GroupCommand, PlayerNote } from "./together-connection.ts";
import {
  aimAheadAtMostMs,
  checkDriftEveryMs,
  scrubPauseWindowMs,
  followGroup,
  playerLoadsAheadMs,
  waitAheadAtMostMs,
} from "./together-follow.ts";
import {
  fellBehindEvent,
  playerLoadedChunkEvent,
  playerMovedItselfEvent,
  playerSwitchedQualityEvent,
  type ChunkLoaded,
  type QualitySwitched,
} from "./together-player-events.ts";
import { nudgeForMs } from "./together-drift.ts";
import { refillAheadMs } from "./together-media.ts";
import { ownMarkLastsMs } from "./together-own-moves.ts";
import { FakeMedia, fakeConnection, paused } from "./test-follow.ts";

/**
 * A player in step with a group playing from 100 s that has measured its link loading half a
 * second of film each second, and then falls 54 s behind: its catch-up jump, made at 4 s. A
 * viewer's, or the host's when isLeading.
 */
function fallenBehindOnAHalfSpeedLink(isLeading = false): {
  media: FakeMedia;
  setGroup: (next: GroupView, receivedAtMs: number, placedByRttMs?: number) => void;
  commands: GroupCommand[];
} {
  const media = new FakeMedia();
  media.currentTime = 100;
  media.loaded = [[0, 105]];
  const { connection, setGroup, commands } = fakeConnection({
    ...paused,
    state: "Playing",
    positionMs: 100_000,
  });
  followGroup(media, connection, {
    fileId: 5,
    isLeading,
    monotonicMs,
  });
  for (let atMs = 1000; atMs <= 4000; atMs += 1000) {
    monotonicNowMs = atMs;
    media.currentTime = 100 + atMs / 1000;
    media.loaded = [[0, 105 + (0.5 * atMs) / 1000]];
    media.fire("progress");
  }
  media.currentTime = 50; // a stall the group played on through
  media.fire("waiting");
  return { media, setGroup, commands };
}

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
    followGroup(media, connection, {
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

  it("holds its player where the group starts until the moment the server named, then plays (D268 §3)", () => {
    const media = new FakeMedia();
    const { connection, setGroup } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, isLeading: false, monotonicMs });
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing", startsInMs: 500 }, 1000);
    expect(media.paused).toBe(true);
    expect(media.currentTime).toBe(60);
    monotonicNowMs = 1499;
    vi.advanceTimersByTime(499);
    expect(media.paused).toBe(true);
    monotonicNowMs = 1500;
    vi.advanceTimersByTime(1);
    expect(media.paused).toBe(false);
  });

  it("tells the server, for its log, each seek and nudge it makes and why, and what its player loads", () => {
    const media = new FakeMedia();
    const { connection, setGroup, notes } = fakeConnection(paused);
    followGroup(media, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    expect(notes).toEqual([{ kind: "sought", positionMs: 0, toMs: 60_000, why: "hold" }]);
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000);
    monotonicNowMs = 1000 + checkDriftEveryMs;
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(notes.at(-1)).toEqual({
      kind: "nudged",
      positionMs: 60_000,
      driftMs: 1000,
      rate: media.playbackRate,
      returnCostMs: 0, // a player that has not gone back to the group's rate has learned nothing
    });
    const loaded: ChunkLoaded = { track: "video", loadMs: 2000, kbps: 2400, qualityKbps: 1200 };
    media.dispatchEvent(new CustomEvent(playerLoadedChunkEvent, { detail: loaded }));
    const switched: QualitySwitched = { qualityKbps: 800, estimateKbps: 2500 };
    media.dispatchEvent(new CustomEvent(playerSwitchedQualityEvent, { detail: switched }));
    expect(notes.slice(-2)).toEqual([
      { kind: "chunkLoaded", ...loaded },
      { kind: "qualitySwitched", ...switched },
    ]);
  });

  it("nudges no player stalled and refilling, whose position does not move, until it has refilled", () => {
    const media = new FakeMedia();
    const { connection, setGroup, notes } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, isLeading: false, monotonicMs });
    media.fire("seeked");
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000);
    media.loaded = [[60, 60.5]]; // it stalls with half a second loaded
    media.fire("waiting");
    monotonicNowMs = 1000 + checkDriftEveryMs;
    vi.advanceTimersByTime(checkDriftEveryMs); // a second behind, stalled
    expect(media.playbackRate).toBe(1);
    expect(notes.filter((note) => note.kind === "nudged")).toEqual([]);
    media.loaded = [[60, 75]]; // refilled
    media.fire("progress");
    monotonicNowMs = 1000 + 2 * checkDriftEveryMs;
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(media.playbackRate).toBeCloseTo(1.1);
  });

  it("reads no drift for the tuning's settling time after it starts playing or changes its rate, as Safari's position stands still then (D250 §7)", () => {
    const media = new FakeMedia();
    const { connection, setGroup, notes } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, isLeading: false, monotonicMs });
    const settleMs = startingTuning.settleAfterChangeMs;
    const nudges = (): PlayerNote[] => notes.filter((note) => note.kind === "nudged");
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000);
    monotonicNowMs = 2000; // it starts playing a second late: its position has not moved yet
    media.fire("playing");
    monotonicNowMs = 2000 + settleMs - 1;
    media.fire("canplay");
    expect(nudges()).toEqual([]);
    monotonicNowMs = 2000 + settleMs;
    media.fire("canplay");
    expect(media.playbackRate).toBeCloseTo(1.1);
    // The nudge ends in step, and the rate goes back to the group's: a stale reading just after is
    // not taken for drift.
    const backAtMs = 2000 + settleMs + nudgeForMs;
    monotonicNowMs = backAtMs;
    media.currentTime = (60_000 + backAtMs - 1000) / 1000;
    media.fire("canplay");
    expect(media.playbackRate).toBe(1);
    monotonicNowMs = backAtMs + settleMs - 1;
    media.currentTime -= 1;
    media.fire("canplay");
    expect(nudges()).toHaveLength(1);
    monotonicNowMs = backAtMs + settleMs;
    media.fire("canplay");
    expect(nudges()).toHaveLength(2);
  });

  it("reads no drift for the settling time after a seek it made lands", () => {
    const media = new FakeMedia();
    const { connection, setGroup, notes } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, isLeading: false, monotonicMs });
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000);
    monotonicNowMs = 3000;
    media.fire("seeked"); // two seconds behind just as a seek lands
    expect(notes.filter((note) => note.kind === "nudged")).toEqual([]);
    monotonicNowMs = 3000 + startingTuning.settleAfterChangeMs;
    media.fire("canplay");
    expect(notes.filter((note) => note.kind === "nudged")).toHaveLength(1);
  });

  it("keeps a nudged player's pitch, or not, as the tuning says (D250 §7)", () => {
    for (const shouldNudgeKeepPitch of [true, false]) {
      const media = new FakeMedia();
      media.preservesPitch = !shouldNudgeKeepPitch;
      const { connection, setGroup } = fakeConnection(paused, {
        ...startingTuning,
        shouldNudgeKeepPitch,
      });
      followGroup(media, connection, { fileId: 5, isLeading: false, monotonicMs });
      monotonicNowMs = 1000;
      setGroup({ ...paused, state: "Playing" }, 1000);
      monotonicNowMs = 2000;
      media.fire("canplay");
      expect(media.playbackRate).toBeCloseTo(1.1);
      expect(media.preservesPitch).toBe(shouldNudgeKeepPitch);
    }
  });

  it("takes the group's message as half the shortest round trip old, so it plays where the group is now (D66)", () => {
    const media = new FakeMedia();
    // A round trip of 2 s: the group message took 1 s to come, and the group has played on.
    const { connection, setGroup, notes } = fakeConnection(paused, startingTuning, 2000);
    followGroup(media, connection, { fileId: 5, isLeading: false, monotonicMs });
    media.currentTime = 61; // where the group is, a second after it was sent
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000);
    expect(notes.filter((note) => note.kind === "nudged" || note.kind === "sought")).toEqual([
      { kind: "sought", positionMs: 0, toMs: 60_000, why: "hold" },
    ]);
    expect(media.playbackRate).toBe(1);
  });

  it("takes a group placed by a pong as half that pong's round trip old, not the shortest's (D264)", () => {
    const media = new FakeMedia();
    // The shortest round trip was 200 ms, but the pong that placed the group took 2 s.
    const { connection, setGroup, notes } = fakeConnection(paused, startingTuning, 200);
    followGroup(media, connection, { fileId: 5, isLeading: false, monotonicMs });
    media.currentTime = 61; // where the group is, a second after the pong said 60 s
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000, 2000);
    expect(notes.filter((note) => note.kind === "nudged" || note.kind === "sought")).toEqual([
      { kind: "sought", positionMs: 0, toMs: 60_000, why: "hold" },
    ]);
    expect(media.playbackRate).toBe(1);
  });

  it("does not follow a group playing another film", () => {
    const media = new FakeMedia();
    const { connection } = fakeConnection({ ...paused, fileId: 6 });
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
      fileId: 5,
      isLeading: true,
      monotonicMs,
    });
    expect(media.paused).toBe(false);
    setGroup({ ...paused, state: "Waiting", event: "played", byMemberId: 2 }, 0);
    expect(media.paused).toBe(true);
    media.fire("pause");
    expect(commands).toEqual([]);
  });

  it("takes a pause the browser makes while its person drags the timeline for the seek it is", () => {
    const media = new FakeMedia();
    const { connection, commands } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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
    followGroup(media, connection, {
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

  it("held paused at the film's start, steps onto its first loaded frame however small the gap", () => {
    const media = new FakeMedia();
    media.currentTime = 0;
    media.loaded = [[0.04, 60.03]]; // a prepared copy's first frame, 40 ms in (staging, 2026-10-06)
    media.readyState = 4;
    const { connection, reports } = fakeConnection({ ...paused, state: "Waiting", positionMs: 0 });
    followGroup(media, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    expect(media.currentTime).toBe(0.04);
    media.fire("seeked");
    vi.advanceTimersByTime(checkDriftEveryMs * 3);
    expect(media.currentTime).toBe(0.04); // held there, not sought back into the gap
    expect(reports.at(-1)?.isBuffering).toBe(false);
  });

  it("catching up on a slow link, jumps ahead by its time to fill 80% of its buffer and waits there", () => {
    const { media } = fallenBehindOnAHalfSpeedLink();
    // 80% of the 30 s it loads ahead is 24 s, which at half a second of film a second takes 48 s:
    // the group, at 104 s, is met at 152 s (D250 §4).
    expect((playerLoadsAheadMs * startingTuning.catchUpBufferPercent) / 100).toBe(24_000);
    expect(media.currentTime).toBe(152);
    media.fire("seeked");
    expect(media.paused).toBe(true);
    monotonicNowMs = 30_000;
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(media.paused).toBe(true); // the group is 22 s away: it waits, loading
    expect(media.currentTime).toBe(152);
    monotonicNowMs = 51_950;
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(media.paused).toBe(false); // the group arrived: it plays on from where it waited
    expect(media.currentTime).toBe(152);
  });

  it("catching up as the host, waits ahead as a viewer does, and its pause to wait is no command", () => {
    const { media, commands } = fallenBehindOnAHalfSpeedLink(true);
    expect(media.currentTime).toBe(152);
    media.fire("seeked");
    expect(media.paused).toBe(true);
    media.fire("pause");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([]); // the group plays on, not paused by the host's wait
    monotonicNowMs = 51_950;
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(media.paused).toBe(false); // the group arrived: it plays on from where it waited
    expect(media.currentTime).toBe(152);
  });

  it("jumps no further ahead than its player loads, however slow its link", () => {
    const media = new FakeMedia();
    media.currentTime = 100;
    media.loaded = [[0, 105]];
    const { connection } = fakeConnection({ ...paused, state: "Playing", positionMs: 100_000 });
    followGroup(media, connection, {
      fileId: 5,
      isLeading: false,
      monotonicMs,
    });
    for (let atMs = 1000; atMs <= 4000; atMs += 1000) {
      monotonicNowMs = atMs;
      media.currentTime = 100 + atMs / 1000;
      media.loaded = [[0, 105 + (0.1 * atMs) / 1000]];
      media.fire("progress");
    }
    media.currentTime = 50;
    media.fire("waiting");
    expect(media.currentTime).toBe(104 + waitAheadAtMostMs / 1000);
  });

  it("waiting ahead, follows a group that pauses to where it paused, and plays on from there", () => {
    const { media, setGroup } = fallenBehindOnAHalfSpeedLink();
    media.fire("seeked");
    monotonicNowMs = 10_000;
    setGroup({ ...paused, positionMs: 110_000 }, 10_000);
    expect(media.currentTime).toBe(110);
    media.fire("seeked");
    monotonicNowMs = 12_000;
    setGroup({ ...paused, state: "Playing", positionMs: 110_000 }, 12_000);
    expect(media.paused).toBe(false);
    expect(media.currentTime).toBe(110); // not back to where it had waited
  });

  it("aims a catch-up seek ahead by how long its last seek took to load, up to a limit", () => {
    const media = new FakeMedia();
    const { connection } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media, connection, {
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

describe("a leading player held at a sync point", () => {
  it("does not take the hold's own pause for its person's, once the group plays on", () => {
    const media = new FakeMedia();
    const { connection, setGroup, commands } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media, connection, { fileId: 5, isLeading: true, monotonicMs });
    media.paused = false;
    // The host pressed play: the group waits at its sync point, then plays.
    setGroup({ ...paused, state: "Waiting" }, 0);
    expect(media.paused).toBe(true);
    setGroup({ ...paused, state: "Playing" }, 0);
    media.fire("pause"); // the hold's pause, its event arriving once the group plays on
    vi.advanceTimersByTime(scrubPauseWindowMs * 2);
    expect(commands).toEqual([]);
  });
});

describe("a leading player's own moves", () => {
  it("gives up a mark whose event never came, noting it, so its person's later pause is a command (D268 §4)", () => {
    const media = new FakeMedia();
    const { connection, setGroup, commands, notes } = fakeConnection({
      ...paused,
      state: "Playing",
    });
    followGroup(media, connection, { fileId: 5, isLeading: true, monotonicMs });
    media.paused = false;
    // The host pressed play: the group waits, and the hold's pause fires no event.
    setGroup({ ...paused, state: "Waiting" }, 0);
    expect(media.paused).toBe(true);
    monotonicNowMs = ownMarkLastsMs + 1;
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(notes).toContainEqual({ kind: "ownMarkExpired", why: "pause" });
    setGroup({ ...paused, state: "Playing" }, monotonicNowMs);
    expect(media.paused).toBe(false);
    media.paused = true;
    media.fire("pause"); // its person's pause, a minute later
    vi.advanceTimersByTime(scrubPauseWindowMs * 2);
    expect(commands).toEqual([{ command: "pause" }]);
  });
});

describe("a leading player awaiting the answer to its command", () => {
  it("is not held back to the group as it was by a pong or a roster before the answer", () => {
    const media = new FakeMedia();
    const { connection, setGroup, heardNoAnswer, commands } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, isLeading: true, monotonicMs });
    media.fire("seeked"); // the follower's own seek to the group's position
    // The host presses play: its command is sent, and its video plays.
    media.paused = false;
    media.fire("play");
    expect(commands).toEqual([{ command: "play" }]);
    heardNoAnswer(); // a pong arrives before the server's answer
    expect(media.paused).toBe(false);
    setGroup({ ...paused, state: "Waiting" }, 0); // the answer: it waits at its sync point
    expect(media.paused).toBe(true);
  });
});

describe("a leading player's command", () => {
  it("is noted with the player's last events, its own marked, so the log says what made it", () => {
    const media = new FakeMedia();
    const { connection, notes } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media, connection, { fileId: 5, isLeading: true, monotonicMs });
    media.paused = false;
    monotonicNowMs = 500;
    media.fire("pause"); // its person's pause
    vi.advanceTimersByTime(scrubPauseWindowMs * 2);
    expect(notes.filter((note) => note.kind === "led")).toEqual([
      { kind: "led", why: "pause", events: "pause@500" },
    ]);
  });
});
