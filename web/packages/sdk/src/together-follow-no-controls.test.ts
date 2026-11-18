// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { noRight } from "./together.ts";
import { browserPauseWindowMs, scrubPauseWindowMs } from "./together-follow.ts";
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

/** A guest's player whose link gives no controls, following group as the guest's page does. */
function followingWithNoControls(group: typeof paused): {
  media: FakeMedia;
  commands: readonly unknown[];
} {
  const media = new FakeMedia();
  const following = fakeConnection(group);
  followGroup(media, following.connection, { fileId: 5, leads: noRight, monotonicMs });
  media.fire("seeked"); // the follower's own seek to the group's position
  return { media, commands: following.commands };
}

// AirPlay is offered whatever the link's controls (D520 §2): a press on the AirPlay device's remote,
// the phone's lock screen or Safari's own controls reaches the guest's page as its own player's
// pause, play or seek, which the page cannot tell from any other. Without the control, it sends no
// command and puts the player back where the group is.
describe("a guest's player whose link gives no controls (D520 §2)", () => {
  it("sends no pause for a pause from outside the page while the group plays, and plays on", () => {
    const { media, commands } = followingWithNoControls({ ...paused, state: "playing" });
    expect(media.paused).toBe(false);
    media.paused = true;
    media.fire("pause");
    vi.advanceTimersByTime(browserPauseWindowMs);
    expect(commands).toEqual([]);
    expect(media.paused).toBe(false);
  });

  it("sends no play for a play from outside the page while the group is paused, and pauses again", () => {
    const { media, commands } = followingWithNoControls(paused);
    media.paused = false;
    media.fire("play");
    vi.advanceTimersByTime(browserPauseWindowMs);
    expect(commands).toEqual([]);
    expect(media.paused).toBe(true);
  });

  it("sends no seek for a seek from outside the page, and goes back to where the group is", () => {
    const { media, commands } = followingWithNoControls(paused);
    media.currentTime = 600;
    media.fire("seeking");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    media.fire("seeked");
    vi.advanceTimersByTime(scrubPauseWindowMs);
    expect(commands).toEqual([]);
    expect(media.currentTime).toBe(paused.positionMs / 1000);
  });
});
