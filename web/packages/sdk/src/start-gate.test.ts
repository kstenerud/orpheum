// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  readyAheadOfNextChunkPercent,
  setReadyAheadPercent,
  tellNextChunkLoadMs,
} from "./ready-ahead.ts";
import { startCheckEveryMs, startGateOf } from "./start-gate.ts";
import { everyRight, type RosterDevice, type RosterMember } from "./together.ts";
import type { TogetherConnection } from "./together-connection.ts";
import { refillAheadMs } from "./together-media.ts";
import { playerLoadedChunkEvent, playerMovedItselfEvent } from "./together-player-events.ts";
import {
  FakeMedia,
  fakeConnection,
  paused,
  followGroup,
  stopEveryFollower,
} from "./test-follow.ts";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  stopEveryFollower();
  vi.useRealTimers();
  setReadyAheadPercent(readyAheadOfNextChunkPercent);
});

/**
 * The owner's Marrowgate Family on a Fire TV (staging, 2026-10-28): resumed at 1798.9 s, its first
 * chunk loaded from 1794.96 s to 1800.47 s, canplay said; its next chunk, 10.4 s of film, about
 * 10 MB at some 9 Mb/s, expected to take 2.3 s.
 */
function marrowgateResumed(): FakeMedia {
  const media = new FakeMedia();
  media.currentTime = 1798.9;
  media.loaded = [[1794.96, 1800.47]];
  media.readyState = 4;
  tellNextChunkLoadMs(media, () => ({ loadMs: 2300, from: "confident" }));
  return media;
}

/** A play the gate makes, counted. */
function countedPlay(media: FakeMedia): { readonly how: () => void; readonly plays: () => number } {
  let plays = 0;
  return {
    how: () => {
      plays += 1;
      void media.play();
    },
    plays: () => plays,
  };
}

describe("a player alone starts only once what it loaded outlasts its next chunk's load (start-gate.ts)", () => {
  it("does not play the Addams shape, 1.6 s loaded with its next chunk 2.3 s away, until more arrives", () => {
    const media = marrowgateResumed();
    const gate = startGateOf(media);
    const play = countedPlay(media);
    gate.play(play.how);
    media.fire("canplay");
    vi.advanceTimersByTime(2000);
    expect(play.plays()).toBe(0);
    expect(media.paused).toBe(true);
    expect(gate.isWaiting()).toBe(true);
    // The next chunk arrives: 12 s ahead.
    media.loaded = [[1794.96, 1810.87]];
    media.fire(playerLoadedChunkEvent);
    expect(play.plays()).toBe(1);
    expect(media.paused).toBe(false);
    expect(gate.lastWait()).toEqual({
      waitedForAheadMs: (2300 * readyAheadOfNextChunkPercent) / 100,
      nextChunkLoadMs: 2300,
      nextChunkLoadFrom: "confident",
    });
    vi.advanceTimersByTime(2000);
    expect(play.plays()).toBe(1);
  });

  it("starts at once on a fast link with small chunks: 1 s loaded, its next 2 s chunk loading at 10 times", () => {
    const media = new FakeMedia();
    media.currentTime = 30;
    media.loaded = [[28, 31]];
    tellNextChunkLoadMs(media, () => ({ loadMs: 200, from: "confident" }));
    const play = countedPlay(media);
    const gate = startGateOf(media);
    gate.play(play.how);
    expect(play.plays()).toBe(1);
    expect(gate.lastWait()).toBeUndefined();
  });

  it("starts a film from its beginning, its first picture loaded from 0.08 s, not from 0", () => {
    // Season 2's first episode on the Mac and a film on the Fire TV never started by themselves
    // (staging, 2026-10-28: buffered 0.08–12.08 s, the position 0): the start read nothing loaded
    // where it stood, and a paused player steps over no gap, so the wait never ended.
    const media = new FakeMedia();
    media.currentTime = 0;
    media.loaded = [[0.08, 12.08]];
    media.readyState = 4;
    tellNextChunkLoadMs(media, () => ({ loadMs: 500, from: "confident" }));
    const play = countedPlay(media);
    startGateOf(media).play(play.how);
    expect(play.plays()).toBe(1);
  });

  it("starts a film of 10 s loaded whole, its last sample ending a hair before its duration", () => {
    // The browser tier's warm-up film never started (2026-10-28): 9.999999 s loaded ahead of its
    // first picture, short of the 10 s wait, and loaded to 10.083333 s of 10.083334.
    const media = new FakeMedia();
    media.currentTime = 0;
    media.duration = 10.083_334;
    media.loaded = [[0.083_334, 10.083_333]];
    const play = countedPlay(media);
    startGateOf(media).play(play.how);
    expect(play.plays()).toBe(1);
  });

  it("waits longer on a slow link: its next 10 s chunk loading at 0.8 times", () => {
    const media = new FakeMedia();
    media.currentTime = 30;
    media.loaded = [[30, 40]];
    tellNextChunkLoadMs(media, () => ({ loadMs: 12_500, from: "confident" }));
    const play = countedPlay(media);
    startGateOf(media).play(play.how);
    const readyOnMs = (12_500 * readyAheadOfNextChunkPercent) / 100;
    media.loaded = [[30, 30 + (readyOnMs - 100) / 1000]];
    vi.advanceTimersByTime(startCheckEveryMs);
    expect(play.plays()).toBe(0);
    media.loaded = [[30, 30 + (readyOnMs + 1) / 1000]];
    vi.advanceTimersByTime(startCheckEveryMs);
    expect(play.plays()).toBe(1);
  });

  it("waits by the server's margin once the page sets it", () => {
    setReadyAheadPercent(300);
    const media = new FakeMedia();
    media.currentTime = 30;
    media.loaded = [[30, 30 + 4.5]]; // enough at 150%, of a 2 s load
    tellNextChunkLoadMs(media, () => ({ loadMs: 2000, from: "confident" }));
    const play = countedPlay(media);
    startGateOf(media).play(play.how);
    expect(play.plays()).toBe(0);
    media.loaded = [[30, 36.001]];
    vi.advanceTimersByTime(startCheckEveryMs);
    expect(play.plays()).toBe(1);
  });

  it("waits for refillAheadMs where its player cannot say how long its next chunk takes", () => {
    const media = new FakeMedia();
    media.currentTime = 30;
    media.loaded = [[30, 30 + (refillAheadMs - 100) / 1000]];
    const play = countedPlay(media);
    const gate = startGateOf(media);
    gate.play(play.how);
    expect(play.plays()).toBe(0);
    media.loaded = [[30, 30 + refillAheadMs / 1000]];
    media.fire("progress");
    expect(play.plays()).toBe(1);
    expect(gate.lastWait()).toEqual({ waitedForAheadMs: refillAheadMs, nextChunkLoadFrom: "none" });
  });

  it("plays at once where the browser loads no more while it is paused", () => {
    const media = marrowgateResumed();
    media.networkState = 1; // NETWORK_IDLE: preload stopped at the metadata
    const play = countedPlay(media);
    startGateOf(media).play(play.how);
    expect(play.plays()).toBe(1);
  });

  it("gives its wait up when the person plays it themselves, playing nothing more", () => {
    const media = marrowgateResumed();
    const play = countedPlay(media);
    const gate = startGateOf(media);
    gate.play(play.how);
    void media.play(); // the person's press
    media.fire("play");
    media.loaded = [[1794.96, 1810.87]];
    vi.advanceTimersByTime(startCheckEveryMs);
    expect(play.plays()).toBe(0);
    expect(gate.isWaiting()).toBe(false);
  });
});

describe("a player alone plays on after a seek only once what it loaded outlasts its next chunk's load", () => {
  /** A playing film at 60 s, all of 0-120 s loaded, its next chunk 2 s away, which the gate started. */
  function playingStarted(): { media: FakeMedia; plays: () => number } {
    const media = new FakeMedia();
    media.currentTime = 60;
    media.loaded = [[0, 120]];
    tellNextChunkLoadMs(media, () => ({ loadMs: 2000, from: "confident" }));
    const play = countedPlay(media);
    startGateOf(media).play(play.how);
    media.fire("play");
    return { media, plays: play.plays };
  }

  it("is paused as it seeks where little is loaded, by its own pause, and played on once enough is", () => {
    const { media, plays } = playingStarted();
    const gate = startGateOf(media);
    media.currentTime = 1000;
    media.seeking = true;
    media.fire("seeking");
    expect(media.paused).toBe(true);
    expect(gate.isOwnPause()).toBe(true);
    media.fire("pause");
    media.seeking = false;
    media.loaded = [
      [0, 120],
      [998, 1001],
    ];
    media.fire("seeked");
    expect(plays()).toBe(1); // only the start's
    media.loaded = [
      [0, 120],
      [998, 1003.001],
    ];
    vi.advanceTimersByTime(startCheckEveryMs);
    expect(plays()).toBe(2);
    expect(media.paused).toBe(false);
    expect(gate.lastWait()).toEqual({
      waitedForAheadMs: 3000,
      nextChunkLoadMs: 2000,
      nextChunkLoadFrom: "confident",
    });
  });

  it("plays on through a seek into what it loaded, never paused", () => {
    const { media, plays } = playingStarted();
    media.currentTime = 90;
    media.seeking = true;
    media.fire("seeking");
    expect(media.paused).toBe(false);
    expect(plays()).toBe(1);
  });

  it("plays on through the player's own step over a gap", () => {
    const { media } = playingStarted();
    media.fire(playerMovedItselfEvent);
    media.currentTime = 120.05;
    media.seeking = true;
    media.fire("seeking");
    expect(media.paused).toBe(false);
  });
});

/** This device, 8, in the roster. */
const thisDevice = 8;

/** A device of the roster, leading its session or not. */
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

/** A cast's receiver at the Marrowgate shape, following its session as led by isLeading. */
function receiverAtMarrowgate(
  isLeading: boolean,
): { media: FakeMedia } & ReturnType<typeof fakeConnection> {
  const media = marrowgateResumed();
  const fake = fakeConnection({ ...paused, state: "Idle", fileId: 0 });
  const roster = [
    device(thisDevice, "video", isLeading),
    device(33, isLeading ? "panel" : "video", !isLeading),
  ];
  followGroup(media, withRoster(fake.connection, roster), {
    fileId: 5,
    leads: everyRight,
    monotonicMs: () => 0,
  });
  return { media, ...fake };
}

describe("a session's leader starts only with enough loaded ahead, and says so truthfully", () => {
  it("waits before its first play, saying it buffers and does not play until it plays", () => {
    const { media, setGroup, connection, reports, commands } = receiverAtMarrowgate(true);
    setGroup({ ...paused, positionMs: 1_798_900, state: "Waiting", event: "changedItem" }, 0);
    expect(media.paused).toBe(true);
    connection.reportState();
    expect(reports.at(-1)).toMatchObject({ isPlaying: false, isBuffering: true });
    media.loaded = [[1794.96, 1810.87]];
    media.fire(playerLoadedChunkEvent);
    expect(media.paused).toBe(false);
    media.fire("play");
    media.fire("playing");
    expect(reports.at(-1)).toMatchObject({ isPlaying: true, isBuffering: false });
    expect(commands).toEqual([]);
  });

  it("never plays a start its group paused while it waited", () => {
    const { media, setGroup } = receiverAtMarrowgate(true);
    setGroup({ ...paused, positionMs: 1_798_900, state: "Waiting", event: "changedItem" }, 0);
    setGroup({ ...paused, positionMs: 1_798_900, state: "Paused", event: "paused" }, 500);
    media.loaded = [[1794.96, 1810.87]];
    vi.advanceTimersByTime(startCheckEveryMs);
    expect(media.paused).toBe(true);
  });

  it("sends no pause for everyone as it holds its own seek until enough is loaded", () => {
    const { media, setGroup, commands } = receiverAtMarrowgate(true);
    media.loaded = [[0, 7200]];
    setGroup({ ...paused, positionMs: 1_798_900, state: "Playing", event: "played" }, 0);
    media.fire("play");
    expect(media.paused).toBe(false);
    media.loaded = [[1790, 1800.47]];
    media.currentTime = 600;
    media.seeking = true;
    media.fire("seeking");
    media.fire("pause");
    expect(media.paused).toBe(true);
    expect(commands.filter((command) => command.command === "pause")).toEqual([]);
  });

  it("stands aside while it follows another device's lead: its follower plays it", () => {
    const { media } = receiverAtMarrowgate(false);
    const play = countedPlay(media);
    startGateOf(media).play(play.how);
    expect(play.plays()).toBe(1);
  });
});

describe("a cast's receiver asked to play before its session's welcome says it leads", () => {
  /**
   * The owner's cast to a Fire TV (staging, 2026-10-28, fileId 5664): the receiver's player joined
   * its session as its page asked its start gate to play, its roster empty until the welcome; the
   * gate took it for a follower and played at once, and once it led it played with nothing loaded
   * ahead (loadedAheadMs=0), ran dry at once and played again 0.1 s later.
   */
  it("waits for enough loaded ahead, as a leader does, never playing at once as a follower would", () => {
    const media = new FakeMedia();
    media.currentTime = 2727.77;
    media.readyState = 0;
    const fake = fakeConnection({ ...paused, state: "Idle", fileId: 0 });
    let isWelcomed = false;
    const member: RosterMember = {
      id: 1,
      name: "owner",
      isHost: true,
      devices: [device(thisDevice, "video", true), device(33, "panel")],
    };
    followGroup(
      media,
      {
        ...fake.connection,
        snapshot: () => {
          const snapshot = fake.connection.snapshot();
          return isWelcomed
            ? { ...snapshot, deviceId: thisDevice, roster: [member] }
            : { ...snapshot, deviceId: undefined, group: undefined, roster: [] };
        },
      },
      { fileId: 5, leads: everyRight, monotonicMs: () => 0 },
    );
    const play = countedPlay(media);
    startGateOf(media).play(play.how);
    expect(play.plays()).toBe(0);
    isWelcomed = true;
    fake.setGroup({ ...paused, positionMs: 2_727_770, state: "Waiting", event: "changedItem" }, 0);
    media.readyState = 3;
    media.loaded = [[2725.92, 2728.5]]; // a moment loaded: canplay, where the TV played
    media.fire("canplay");
    vi.advanceTimersByTime(startCheckEveryMs);
    expect(media.paused).toBe(true);
    media.loaded = [[2725.92, 2727.77 + refillAheadMs / 1000]];
    vi.advanceTimersByTime(startCheckEveryMs);
    expect(media.paused).toBe(false);
  });
});
