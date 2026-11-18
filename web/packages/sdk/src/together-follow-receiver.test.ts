// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { everyRight, noRight, type PlaybackRights } from "./together.ts";
import { browserPauseWindowMs, checkDriftEveryMs } from "./together-follow.ts";
import { receiverLeavesDriftUpToMs } from "./together-follow-options.ts";
import { OwnMoves, receiverAnswersWithinMs } from "./together-own-moves.ts";
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

/**
 * A guest's player following a group paused at 60 s, its film on an AirPlay receiver where
 * isOnReceiver, the group then playing from 1 s on: its follower's own play made then.
 */
function playingAlong(
  isOnReceiver: boolean,
  leads: PlaybackRights = noRight,
): ReturnType<typeof fakeConnection> & {
  media: FakeMedia;
  checkBehindBy: (behindMs: number) => void;
} {
  const media = new FakeMedia();
  media.currentTime = 60;
  const given = fakeConnection(paused);
  followGroup(media, given.connection, {
    fileId: 5,
    leads,
    monotonicMs,
    isOnWirelessTarget: () => isOnReceiver,
  });
  monotonicNowMs = 1000;
  given.setGroup({ ...paused, state: "playing" }, 1000);
  media.fire("play");
  media.fire("playing");
  /** Its player, playing, left behind its group by behindMs at its next drift check. */
  function checkBehindBy(behindMs: number): void {
    monotonicNowMs = 1000 + checkDriftEveryMs;
    media.currentTime = 60 + (checkDriftEveryMs - behindMs) / 1000;
    vi.advanceTimersByTime(checkDriftEveryMs);
  }
  return { ...given, media, checkBehindBy };
}

describe("a guest's sync while its film plays on an AirPlay receiver (D522 §2, D523 §5)", () => {
  it("leaves a receiver a second behind its group alone: no nudge and no seek", () => {
    const { media, notes, checkBehindBy } = playingAlong(true);
    checkBehindBy(1000);
    expect(media.playbackRate).toBe(1);
    expect(media.currentTime).toBe(60);
    expect(notes.filter((note) => note.kind === "nudged" || note.kind === "sought")).toEqual([]);
  });

  it("still nudges a local player a second behind, as before", () => {
    const { media, notes, checkBehindBy } = playingAlong(false);
    checkBehindBy(1000);
    expect(media.playbackRate).toBeGreaterThan(1);
    expect(notes.at(-1)).toMatchObject({ kind: "nudged", driftMs: 1000 });
  });

  it("corrects a receiver once it is further than about 2 s behind", () => {
    const { media, notes, checkBehindBy } = playingAlong(true);
    checkBehindBy(3000);
    expect(media.playbackRate).toBeGreaterThan(1);
    expect(notes.at(-1)).toMatchObject({ kind: "nudged", driftMs: 3000 });
    expect(receiverLeavesDriftUpToMs).toBeLessThan(3000);
  });

  it("holds a receiver 280 ms off its paused group where it is, and seeks a local player there", () => {
    for (const isOnReceiver of [true, false]) {
      const media = new FakeMedia();
      media.currentTime = 59.72; // the guest's seek to hold, 280 ms off (VQ364, 2026-11-18)
      const { connection, notes } = fakeConnection(paused);
      const stop = followGroup(media, connection, {
        fileId: 5,
        leads: noRight,
        monotonicMs,
        isOnWirelessTarget: () => isOnReceiver,
      });
      expect(notes.filter((note) => note.kind === "sought")).toHaveLength(isOnReceiver ? 0 : 1);
      expect(media.currentTime).toBe(isOnReceiver ? 59.72 : 60);
      stop();
    }
  });

  it("sends no pause for the receiver's answer to its own play, and the guest's own pause later", () => {
    const { media, commands } = playingAlong(true, everyRight);
    // The receiver's play state reaching the page late: Safari fires pause, then play again.
    monotonicNowMs = 1020;
    media.paused = true;
    media.fire("pause");
    vi.advanceTimersByTime(browserPauseWindowMs);
    expect(commands).toEqual([]);
    // The guest presses pause, well after the follower's own move.
    monotonicNowMs = 1020 + receiverAnswersWithinMs + 100;
    media.paused = true;
    media.fire("pause");
    vi.advanceTimersByTime(browserPauseWindowMs);
    expect(commands).toEqual([{ command: "pause" }]);
  });

  it("takes the receiver's scrub, a pause then a play with no seeking, for the receiver's: no pause sent", () => {
    for (const isOnReceiver of [true, false]) {
      const { media, commands } = playingAlong(isOnReceiver, everyRight);
      // The Mac's bar, well after the follower's own play: pause, then play 0.2 s later 19 s on.
      monotonicNowMs = 1000 + receiverAnswersWithinMs + 100;
      media.paused = true;
      media.fire("pause");
      vi.advanceTimersByTime(200);
      media.currentTime = 79;
      media.paused = false;
      media.fire("play");
      vi.advanceTimersByTime(browserPauseWindowMs);
      expect(commands).toEqual(isOnReceiver ? [] : [{ command: "pause" }]);
      stopEveryFollower();
    }
  });

  it("takes a pause or play after its own seek on a receiver for the receiver's, never on a local player", () => {
    const onReceiver = new OwnMoves(() => true);
    onReceiver.mark("seek", 0);
    expect(onReceiver.heard("seeking", 10)).toBe(true);
    expect(onReceiver.heard("seeked", 500)).toBe(true);
    expect(onReceiver.heard("pause", 520)).toBe(true);
    expect(onReceiver.heard("play", 700)).toBe(true);
    expect(onReceiver.heard("pause", 500 + receiverAnswersWithinMs)).toBe(true);
    expect(onReceiver.heard("play", 500 + receiverAnswersWithinMs + 1)).toBe(false);
    const local = new OwnMoves(() => false);
    local.mark("seek", 0);
    expect(local.heard("seeking", 10)).toBe(true);
    expect(local.heard("seeked", 500)).toBe(true);
    expect(local.heard("pause", 520)).toBe(false);
    expect(local.heard("play", 700)).toBe(false);
  });
});
