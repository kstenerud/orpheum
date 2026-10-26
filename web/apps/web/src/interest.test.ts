// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { leavePlayback, sayInterestOf, sendInterest, type SaidInterest } from "./interest.ts";
import { failOnUnexpectedRequests, servedBy, type Heard } from "./test-dom.tsx";

// This file never mounts, so it owns the check unmount() would otherwise do for it (finding 2,
// test-architecture-2026-10-26.md; test-dom.tsx's servedBy doc).
afterEach(failOnUnexpectedRequests);

/** A video as sayInterestOf reads one: where it is, whether it is paused or ended, its events. */
function fakeVideo(): HTMLVideoElement & { fire: (event: string) => void } {
  const target = new EventTarget();
  const video = Object.assign(target, {
    currentTime: 0,
    paused: false,
    ended: false,
    fire: (event: string) => {
      target.dispatchEvent(new Event(event));
    },
  });
  return video as unknown as HTMLVideoElement & { fire: (event: string) => void };
}

describe("what a player says it wants (D309)", () => {
  it("says where it paused, played and sought to, and nothing at the end or once stopped", () => {
    const video = fakeVideo();
    const said: SaidInterest[] = [];
    const stop = sayInterestOf(video, (interest) => said.push(interest));
    video.currentTime = 61.2345;
    video.fire("pause");
    Object.assign(video, { paused: true });
    video.currentTime = 600;
    video.fire("seeking");
    Object.assign(video, { paused: false });
    video.fire("playing");
    Object.assign(video, { ended: true });
    video.fire("pause");
    stop();
    video.fire("playing");
    expect(said).toEqual([
      { positionMs: 61_235, isPaused: true },
      { positionMs: 600_000, isPaused: true },
      { positionMs: 600_000, isPaused: false },
    ]);
  });

  it("is sent for its playback, and a leave says why", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy(
      {
        "PUT /playback/a-token/interest": [204, undefined],
        "DELETE /playback/a-token": [503, { error: { code: "updating", message: "back soon" } }],
      },
      heard,
    );
    sendInterest(api, "a-token", { positionMs: 10_000, isPaused: true });
    leavePlayback(api, "a-token", "replaced");
    await vi.waitFor(() => {
      expect(heard).toHaveLength(2);
    });
    expect(heard).toContainEqual({
      methodAndPath: "PUT /playback/a-token/interest",
      body: { positionMs: 10_000, isPaused: true },
    });
    // Refused, it is let go of: nothing waits on it.
    expect(heard).toContainEqual({
      methodAndPath: "DELETE /playback/a-token",
      body: undefined,
      query: "?reason=replaced",
    });
  });
});
