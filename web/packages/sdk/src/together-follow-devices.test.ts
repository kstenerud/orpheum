// SPDX-License-Identifier: MPL-2.0

/**
 * How a following player copes with what particular browsers do: refusing a film's sound without a
 * recent tap, and losing time whenever its rate goes back to its group's (Safari, WebKit 163433).
 */

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FakeMedia, fakeConnection, paused } from "./test-follow.ts";
import { startingTuning } from "./together.ts";
import { checkDriftEveryMs, followGroup } from "./together-follow.ts";

let monotonicNowMs = 0;
const monotonicMs = (): number => monotonicNowMs;

beforeEach(() => {
  vi.useFakeTimers();
  monotonicNowMs = 0;
});

afterEach(() => {
  vi.useRealTimers();
});

describe("a following player the browser will not start with sound", () => {
  it("plays muted when its group plays, as playOrPlayMuted does", async () => {
    const media = new FakeMedia();
    media.play = function refusingSound(this: FakeMedia): Promise<void> {
      if (!this.muted) {
        return Promise.reject(new DOMException("no recent tap", "NotAllowedError"));
      }
      this.paused = false;
      return Promise.resolve();
    };
    const { connection } = fakeConnection({ ...paused, state: "Playing" });
    followGroup(media, connection, { fileId: 5, isLeading: false, monotonicMs });
    await vi.waitFor(() => {
      expect(media.paused).toBe(false);
    });
    expect(media.muted).toBe(true);
  });
});

describe("a following player that loses time whenever its rate goes back to its group's", () => {
  it("learns the loss, and is nudged a few times in a minute, not every other second", () => {
    const media = new FakeMedia();
    // As an iPhone's does: 220 ms lost each time a nudge ends (the owner's tests, 2026-10-08).
    let rate = 1;
    Object.defineProperty(media, "playbackRate", {
      get: () => rate,
      set: (next: number) => {
        if (rate !== 1 && next === 1) {
          media.currentTime -= 0.22;
        }
        rate = next;
      },
    });
    const { connection, setGroup, notes } = fakeConnection(paused);
    followGroup(media, connection, { fileId: 5, isLeading: false, monotonicMs });
    monotonicNowMs = 1000;
    setGroup({ ...paused, state: "Playing" }, 1000);
    media.currentTime = 59.6; // 400 ms behind
    for (let second = 0; second < 60; second++) {
      monotonicNowMs += checkDriftEveryMs;
      media.currentTime += (rate * checkDriftEveryMs) / 1000;
      vi.advanceTimersByTime(checkDriftEveryMs);
    }
    const nudgedAtMs = notes.flatMap((note) => (note.kind === "nudged" ? [note.positionMs] : []));
    expect(nudgedAtMs.length).toBeLessThanOrEqual(8);
    // In step after the first returns taught it the loss: no nudge in the last 40 s.
    expect(Math.max(...nudgedAtMs)).toBeLessThan(80_000);
    const expectedS = 60 + (monotonicNowMs - 1000) / 1000;
    expect(Math.abs(expectedS - media.currentTime)).toBeLessThanOrEqual(
      startingTuning.leaveDriftUpToMs / 1000,
    );
  });
});
