// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { noRight, startingTuning } from "./together.ts";
import { checkDriftEveryMs } from "./together-follow.ts";
import { playedMutedEvent, soundOnAtTap } from "./together-sound.ts";
import {
  FakeMedia,
  FakePage,
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

/** Advances the timers and the monotonic clock together by ms. */
function advance(ms: number): void {
  monotonicNowMs += ms;
  vi.advanceTimersByTime(ms);
}

/** Lets the promises a play() returned settle. */
async function settled(): Promise<void> {
  for (let turn = 0; turn < 5; turn += 1) {
    await Promise.resolve();
  }
}

/** How long the phone is locked, as in the owner's VQ105. */
const lockedMs = 35_000;

/**
 * A handheld's follower of a group playing from 60 s, in step, whose page is hidden at once, the lock
 * pausing its film where it stands, and shown again lockedMs on.
 */
function lockedAndShownAgain(): ReturnType<typeof fakeConnection> & { readonly media: FakeMedia } {
  const media = new FakeMedia();
  const page = new FakePage();
  media.currentTime = 60;
  const following = fakeConnection({ ...paused, state: "Playing" });
  followGroup(media, following.connection, { fileId: 5, leads: noRight, monotonicMs, page });
  expect(media.paused).toBe(false);
  page.hide();
  media.pause(); // the browser's, as the screen locks
  advance(lockedMs);
  expect(media.currentTime).toBe(60); // held while hidden
  page.show();
  return { ...following, media };
}

describe("a follower whose page is shown again (the owner's VQ105, 2026-10-29)", () => {
  it("plays and catches up with its playing group at once, not paused where it was hidden", () => {
    const { media, notes } = lockedAndShownAgain();
    const groupMs = 60_000 + lockedMs;
    // As it is shown, before any check: played, and sought across to where the group is.
    expect(media.paused).toBe(false);
    expect(media.currentTime * 1000).toBe(groupMs);
    expect(notes).toContainEqual({
      kind: "sought",
      positionMs: 60_000,
      toMs: groupMs,
      why: "catchUp",
    });
    media.currentTime += checkDriftEveryMs / 1000; // it plays on, as the group does
    advance(checkDriftEveryMs);
    expect(media.paused).toBe(false);
    const driftMs = Math.abs(media.currentTime * 1000 - (groupMs + checkDriftEveryMs));
    expect(driftMs).toBeLessThanOrEqual(startingTuning.leaveDriftUpToMs);
  });

  it("whose play the browser refuses, of any kind, says so and is offered at its page's tap, which plays it in step", async () => {
    const media = new FakeMedia();
    let isRefusing = false;
    media.play = (): Promise<void> => {
      if (isRefusing) {
        return Promise.reject(new DOMException("refused", "AbortError"));
      }
      media.paused = false;
      return Promise.resolve();
    };
    const page = new FakePage();
    media.currentTime = 60;
    const { connection, notes } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media, connection, { fileId: 5, leads: noRight, monotonicMs, page });
    page.hide();
    media.pause();
    advance(lockedMs);
    isRefusing = true; // the phone, unlocked, will not play its film on
    page.show();
    await settled();
    const soughtToMs = 60_000 + lockedMs;
    expect(media.currentTime * 1000).toBe(soughtToMs); // where the group was as it was shown
    advance(checkDriftEveryMs); // its next check plays it again, refused again
    await settled();
    expect(media.paused).toBe(true);
    expect(notes.filter((note) => note.kind === "playRefused")).toEqual([
      { kind: "playRefused", why: "AbortError", positionMs: soughtToMs },
    ]);
    expect(media.muted).toBe(true);
    expect(media.dispatched).toContain(playedMutedEvent);
    // The page's tap, which its page offers at playedMutedEvent: the browser plays it now, and the
    // follower, hearing it play, catches it up to its group, a second ahead of it now, by its rate.
    isRefusing = false;
    soundOnAtTap(media, connection);
    media.fire("play");
    expect(media.paused).toBe(false);
    expect(media.muted).toBe(false);
    expect(media.playbackRate).toBeGreaterThan(1);
    advance(3 * checkDriftEveryMs);
    await settled();
    expect(notes.filter((note) => note.kind === "playRefused")).toHaveLength(1); // told once
  });
});
