// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  everyRight,
  noRight,
  type GroupView,
  type RosterDevice,
  type RosterMember,
} from "./together.ts";
import type { TogetherConnection } from "./together-connection.ts";
import { isFollowingAGroup } from "./following-a-group.ts";
import { checkDriftEveryMs } from "./together-follow.ts";
import {
  groupJumpedEvent,
  playerSwitchedQualityEvent,
  type QualitySwitched,
} from "./together-player-events.ts";
import {
  FakeMedia,
  fakeConnection,
  paused,
  followGroup,
  stopEveryFollower,
} from "./test-follow.ts";

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 0;
});

afterEach(() => {
  stopEveryFollower();
  vi.useRealTimers();
});

/** This device, 8, in the roster. */
const thisDevice = 8;

/** A device of the roster: a player or a panel, leading its session or not. */
function device(id: number, role: string, isLeader = false): RosterDevice {
  return { id, role, isBuffering: false, cannotKeepUp: undefined, status: "watching", isLeader };
}

/** connection, its snapshot's roster the devices given, this device among them. */
function withRoster(
  connection: TogetherConnection,
  devices: readonly RosterDevice[],
): TogetherConnection {
  const member: RosterMember = { id: 1, name: "owner", isHost: true, devices };
  return {
    ...connection,
    snapshot: () => ({ ...connection.snapshot(), deviceId: thisDevice, roster: [member] }),
  };
}

/** A cast's receiver leading its session of one: this device, the leader, and its remote, a panel. */
const castAlone = [device(thisDevice, "video", true), device(33, "panel")];

/** A player leading its session from group, with its media at mediaAtS, playing or not. */
function leading(
  group: GroupView,
  options: {
    readonly mediaAtS: number;
    readonly isPlaying: boolean;
    readonly delivery?: "direct" | "converted";
  },
): { media: FakeMedia } & ReturnType<typeof fakeConnection> {
  const media = new FakeMedia();
  media.currentTime = options.mediaAtS;
  media.paused = !options.isPlaying;
  const fake = fakeConnection(group);
  followGroup(media, withRoster(fake.connection, castAlone), {
    fileId: 5,
    leads: everyRight,
    monotonicMs,
    ...(options.delivery === undefined ? {} : { delivery: options.delivery }),
  });
  return { media, ...fake };
}

describe("a player that leads its session (D371 §1)", () => {
  it("chooses its quality as a device alone does, not keeping the film's own picture for a group", () => {
    // The Fire TV, leading a cast, jumped to its film's own picture on its second chunk with a link
    // of 1.5 Mb/s (2026-10-28): the follower's rule of D286 §2, judged on one sample.
    const media = new FakeMedia();
    const fake = fakeConnection(paused);
    const stop = followGroup(media, withRoster(fake.connection, castAlone), {
      fileId: 5,
      leads: everyRight,
      monotonicMs,
    });
    expect(isFollowingAGroup()).toBe(false);
    stop();
    const follower = followGroup(
      new FakeMedia(),
      withRoster(fake.connection, [device(thisDevice, "video"), device(9, "video", true)]),
      {
        fileId: 5,
        leads: everyRight,
        monotonicMs,
      },
    );
    expect(isFollowingAGroup(), "a follower keeps it").toBe(true);
    follower();
    expect(isFollowingAGroup()).toBe(false);
  });

  it("is never told its group jumped, so never drops to the common quality at a jump of its own", () => {
    // A cast's receiver on the Fire TV, leading, fell from the film's own 9 Mb/s to 1.9 Mb/s on a
    // link of 18 Mb/s at a seek of its own: "the common quality after its group jumped" (D286 §3,
    // a follower's rule), 2026-10-29.
    const { media, setGroup } = leading(paused, { mediaAtS: 60, isPlaying: true });
    setGroup(
      { ...paused, state: "Waiting", event: "seeked", seekedFromMs: 60_000, positionMs: 900_000 },
      0,
    );
    vi.advanceTimersByTime(3000);
    expect(media.dispatched).not.toContain(groupJumpedEvent);
  });

  it("is never nudged, skipped or sought to where the group reckons it should be: the group follows it", () => {
    const { media, setGroup, notes, commands } = leading(paused, { mediaAtS: 60, isPlaying: true });
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000);
    for (const behindS of [1, 2, 5, 30]) {
      monotonicNowMs += checkDriftEveryMs;
      vi.advanceTimersByTime(checkDriftEveryMs); // the element does not move on: it falls behind
      expect(media.playbackRate, `${String(behindS)} s on`).toBe(1);
      expect(media.paused).toBe(false);
    }
    expect(media.currentTime).toBe(60);
    expect(notes.filter((note) => note.kind === "nudged" || note.kind === "sought")).toEqual([]);
    expect(commands).toEqual([]);
  });

  it("plays the film it put on as soon as the group stands for it, and is held by nothing after", () => {
    const { media, setGroup, notes } = leading(
      { ...paused, state: "Idle", fileId: 0 },
      { mediaAtS: 60, isPlaying: false },
    );
    setGroup({ ...paused, state: "Waiting", event: "changedItem" }, 0);
    expect(media.paused).toBe(false);
    // The group stands for it as it stalls: nothing pauses it, or seeks it back.
    media.currentTime = 64;
    setGroup({ ...paused, state: "Waiting", event: "waitingFor", positionMs: 63_000 }, 4000);
    monotonicNowMs = 5000;
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(media.paused).toBe(false);
    expect(media.currentTime).toBe(64);
    expect(notes.filter((note) => note.kind === "playedWhileHeld")).toEqual([]);
  });

  it("carries out another device's pause, play and seek once each, as its own press, sending no command", () => {
    const { media, setGroup, notes, commands } = leading(
      { ...paused, state: "Playing" },
      { mediaAtS: 60, isPlaying: true },
    );
    setGroup({ ...paused, state: "Paused", event: "paused", positionMs: 61_000 }, 1000);
    media.fire("pause");
    expect(media.paused).toBe(true);
    expect(media.currentTime).toBe(60); // paused where it is, not where the group reckoned
    setGroup({ ...paused, state: "Waiting", event: "played" }, 2000);
    media.fire("play");
    expect(media.paused).toBe(false);
    setGroup(
      { ...paused, state: "Waiting", event: "seeked", seekedFromMs: 60_000, positionMs: 600_000 },
      3000,
    );
    media.fire("seeking");
    expect(media.currentTime).toBe(600);
    expect(notes.filter((note) => note.kind === "sought")).toEqual([
      { kind: "sought", positionMs: 60_000, toMs: 600_000, why: "command" },
    ]);
    media.currentTime = 603; // it plays on from there, and is sought there no more
    monotonicNowMs = 6000;
    vi.advanceTimersByTime(3 * checkDriftEveryMs);
    expect(media.currentTime).toBe(603);
    expect(commands).toEqual([]);
  });

  it("does not seek back to its own seek as its group's change for it comes, having played on", () => {
    // A Fire TV leading a cast sought back about 200 ms after each of its own 10 s skips (staging,
    // 2026-10-28, 18:32:39 and 18:32:44).
    const { media, setGroup, notes, commands } = leading(
      { ...paused, state: "Playing" },
      { mediaAtS: 60, isPlaying: true },
    );
    media.currentTime = 70;
    media.fire("seeking");
    media.fire("seeked");
    vi.advanceTimersByTime(1000);
    expect(commands).toEqual([{ command: "seek", positionMs: 70_000 }]);
    media.currentTime = 70.3; // it plays on as the group hears of its seek
    setGroup(
      { ...paused, state: "Waiting", event: "seeked", seekedFromMs: 60_000, positionMs: 70_000 },
      2000,
    );
    expect(media.currentTime).toBe(70.3);
    expect(notes.filter((note) => note.kind === "sought")).toEqual([]);
    // Another device's seek to the same place later is carried out.
    media.currentTime = 90;
    setGroup(
      { ...paused, state: "Waiting", event: "seeked", seekedFromMs: 90_000, positionMs: 70_000 },
      3000,
    );
    expect(media.currentTime).toBe(70);
  });

  it("sends its own person's pause as a command, the group following it", () => {
    const { media, commands } = leading(
      { ...paused, state: "Playing" },
      { mediaAtS: 60, isPlaying: true },
    );
    media.paused = true;
    media.fire("pause");
    vi.advanceTimersByTime(1000);
    expect(commands).toEqual([{ command: "pause" }]);
  });

  it("takes the place of a group under way far from it as it begins to lead, and not of one it put on", () => {
    const joined = leading(
      { ...paused, state: "Playing", positionMs: 900_000 },
      { mediaAtS: 60, isPlaying: true },
    );
    expect(joined.media.currentTime).toBe(900);
    expect(joined.notes.filter((note) => note.kind === "sought")).toEqual([
      { kind: "sought", positionMs: 60_000, toMs: 900_000, why: "join" },
    ]);
    const putOn = leading(
      { ...paused, state: "Waiting", event: "changedItem", positionMs: 900_000 },
      { mediaAtS: 60, isPlaying: false },
    );
    expect(putOn.media.currentTime).toBe(60);
  });

  it("says it buffers only while it cannot play on, never for want of being where the group is", () => {
    const { media, reports, connection } = leading(
      { ...paused, state: "Playing", positionMs: 900_000 },
      { mediaAtS: 60, isPlaying: true },
    );
    media.currentTime = 30; // as if it never reached the group
    connection.reportState();
    expect(reports.at(-1)?.isBuffering).toBe(false);
    media.readyState = 2;
    connection.reportState();
    expect(reports.at(-1)?.isBuffering).toBe(true);
  });

  it("tells its state each second while it stalls and its group plays on, by which the server hands its lead on (D373 §2)", () => {
    const { media, reports, connection } = leading(
      { ...paused, state: "Playing", positionMs: 60_000 },
      { mediaAtS: 60, isPlaying: true },
    );
    const playing = reports.length;
    vi.advanceTimersByTime(3 * checkDriftEveryMs);
    expect(reports.length, "playing on, it tells no state each second").toBe(playing);
    media.readyState = 2; // stalled
    connection.reportState();
    const stalled = reports.length;
    vi.advanceTimersByTime(3 * checkDriftEveryMs);
    expect(reports.length - stalled).toBe(3);
    expect(reports.slice(stalled).every((report) => report.isBuffering)).toBe(true);
    media.readyState = 4; // plays on
    connection.reportState();
    const again = reports.length;
    vi.advanceTimersByTime(3 * checkDriftEveryMs);
    expect(reports.length).toBe(again);
  });

  it("catches up to its group after a stall the group played on through, nudged forward, then leads from there (D374)", () => {
    const media = new FakeMedia();
    media.currentTime = 60;
    media.paused = false;
    const fake = fakeConnection({ ...paused, state: "Playing", positionMs: 60_000 });
    const withAFollower = [device(thisDevice, "video", true), device(9, "video")];
    followGroup(media, withRoster(fake.connection, withAFollower), {
      fileId: 5,
      leads: everyRight,
      monotonicMs,
    });
    const { connection, reports, notes, commands } = fake;
    media.readyState = 2; // stalled, the group playing on
    connection.reportState();
    monotonicNowMs = 2000; // the group 2 s on
    media.readyState = 4; // plays again where it stalled, 2 s behind
    connection.reportState();
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(media.playbackRate, "nudged forward to its group, as a follower is").toBeGreaterThan(1);
    expect(notes.some((note) => note.kind === "nudged")).toBe(true);
    expect(commands, "the group is moved back for no stall").toEqual([]);
    const catchingUp = reports.length;
    monotonicNowMs = 3000;
    media.currentTime = 63; // caught up
    vi.advanceTimersByTime(checkDriftEveryMs);
    expect(media.playbackRate, "leading again, at its group's rate").toBe(1);
    expect(reports.length).toBeGreaterThan(catchingUp);
    expect(reports.at(-1)).toMatchObject({ positionMs: 63_000, isBuffering: false });
    const caughtUp = reports.length;
    monotonicNowMs = 5000; // drifting from its group as a leader may: it is never steered again
    vi.advanceTimersByTime(3 * checkDriftEveryMs);
    expect(media.playbackRate).toBe(1);
    expect(reports.length).toBe(caughtUp);
  });

  it("tells its session how it plays the film: its delivery, the quality it began and the link it chose it on (D371 §4)", () => {
    const { media, reports } = leading(
      { ...paused, state: "Playing" },
      { mediaAtS: 60, isPlaying: true, delivery: "converted" },
    );
    const switched: QualitySwitched = { qualityKbps: 4200, linkEstimateKbps: 18_000 };
    media.fire(
      playerSwitchedQualityEvent,
      new CustomEvent(playerSwitchedQualityEvent, { detail: switched }),
    );
    expect(reports.at(-1)?.playback).toEqual({
      delivery: "converted",
      qualityKbps: 4200,
      linkKbps: 18_000,
    });
  });
});

describe("a following player whose session another device leads", () => {
  /** A player a second behind its group once it plays, its session's devices those given. */
  function aSecondBehind(devices: readonly RosterDevice[]): {
    media: FakeMedia;
    notes: ReturnType<typeof fakeConnection>["notes"];
  } {
    const media = new FakeMedia();
    const { connection, setGroup, notes } = fakeConnection(paused);
    followGroup(media, withRoster(connection, devices), { fileId: 5, leads: noRight, monotonicMs });
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000);
    monotonicNowMs = 1000 + checkDriftEveryMs;
    vi.advanceTimersByTime(checkDriftEveryMs); // the element did not move on: a second behind
    return { media, notes };
  }

  /** A follower of a session device 40 leads, its media at 60 s, paused, and its connection. */
  function following(group: GroupView): { media: FakeMedia } & ReturnType<typeof fakeConnection> {
    const media = new FakeMedia();
    media.currentTime = 60;
    const fake = fakeConnection(group);
    const devices = [device(thisDevice, "video"), device(40, "video", true)];
    followGroup(media, withRoster(fake.connection, devices), {
      fileId: 5,
      leads: noRight,
      monotonicMs,
    });
    return { media, ...fake };
  }

  it("is held where the group stands for its leader, plays at once as the leader plays, and is nudged into step (D371 §2, §6)", () => {
    const { media, setGroup, commands } = following({
      ...paused,
      state: "Waiting",
      event: "changedItem",
    });
    expect(media.paused).toBe(true);
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing", event: "leaderPlays" }, 1000);
    expect(media.paused).toBe(false); // at once: no moment named, no run-up
    expect(media.currentTime).toBe(60);
    monotonicNowMs = 2000;
    vi.advanceTimersByTime(checkDriftEveryMs); // it did not move on: a second behind the leader
    expect(media.playbackRate).toBeCloseTo(1.1);
    expect(commands).toEqual([]);
  });

  it("after a seek, holds where the leader went until it plays, then plays from there", () => {
    const { media, setGroup, notes } = following({ ...paused, state: "Playing" });
    setGroup(
      { ...paused, state: "Waiting", event: "seeked", seekedFromMs: 60_000, positionMs: 600_000 },
      0,
    );
    expect(media.paused).toBe(true);
    expect(media.currentTime).toBe(600);
    expect(notes.filter((note) => note.kind === "sought")).toEqual([
      { kind: "sought", positionMs: 60_000, toMs: 600_000, why: "hold" },
    ]);
    setGroup({ ...paused, state: "Playing", event: "leaderPlays", positionMs: 600_000 }, 0);
    expect(media.paused).toBe(false);
  });

  it("after a stall, jumps ahead to catch up to the leader, the group playing on and told nothing", () => {
    const { media, setGroup, notes, commands } = following(paused);
    setGroup({ ...paused, state: "Playing", event: "leaderPlays" }, 0);
    media.readyState = 2; // it stalls at 60 s while the leader plays on
    media.fire("waiting");
    monotonicNowMs = 5000;
    media.readyState = 4; // loaded again, 5 s behind the leader
    vi.advanceTimersByTime(5 * checkDriftEveryMs);
    expect(notes.filter((note) => note.kind === "sought" && note.why === "catchUp")).toHaveLength(
      1,
    );
    expect(media.currentTime).toBeGreaterThanOrEqual(65);
    expect(commands).toEqual([]);
  });

  it("is nudged to the leader's place (D371 §2)", () => {
    const { media, notes } = aSecondBehind([device(thisDevice, "both"), device(40, "video", true)]);
    expect(media.playbackRate).toBeCloseTo(1.1);
    expect(notes.filter((note) => note.kind === "nudged")).toHaveLength(1);
  });

  it("says, as it begins to hold up, what holds it, its readyState and rate, what it has loaded ahead and how long since its rate changed", () => {
    const { media, notes } = aSecondBehind([device(thisDevice, "both"), device(40, "video", true)]);
    expect(media.playbackRate).toBeCloseTo(1.1); // nudged at 2 s
    monotonicNowMs = 2350;
    media.currentTime = 61;
    media.loaded = [[0, 72]];
    media.readyState = 2;
    media.fire("waiting");
    expect(notes.filter((note) => note.kind === "buffering")).toEqual([
      {
        kind: "buffering",
        why: "waiting",
        positionMs: 61_000,
        readyState: 2,
        rate: media.playbackRate,
        loadedAheadMs: 11_000,
        sinceRateChangeMs: 350,
      },
    ]);
    media.fire("waiting");
    expect(notes.filter((note) => note.kind === "buffering")).toHaveLength(1); // said once
  });
});
