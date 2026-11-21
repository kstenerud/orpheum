// SPDX-License-Identifier: MPL-2.0

import { playerForesawStepDownEvent } from "@orpheum/player";
import { startGateOf } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { leavePlayback, sayInterestOf, sendInterest, type SaidInterest } from "./interest.ts";
import { keepThePressForTheStart } from "./start-refused.ts";
import { failOnUnexpectedRequests, servedBy, type Heard } from "./test-dom.tsx";

// This file never mounts, so it owns the check unmount() would otherwise do for it (finding 2,
// test-architecture-2026-10-26.md; test-dom.tsx's servedBy doc).
afterEach(failOnUnexpectedRequests);

/**
 * A video as sayInterestOf and its start gate read one: where it is, whether it is paused or ended,
 * nothing loaded, its events. Its pause and play fire their events, as the element's do.
 */
function fakeVideo(): HTMLVideoElement & { fire: (event: string) => void } {
  const target = new EventTarget();
  const fire = (event: string): void => {
    target.dispatchEvent(new Event(event));
  };
  const video = Object.assign(target, {
    currentTime: 0,
    paused: false,
    ended: false,
    seeking: false,
    error: null,
    readyState: 1,
    networkState: 2, // NETWORK_LOADING: the gate waits for what it loads
    duration: 7200,
    buffered: { length: 0, start: () => 0, end: () => 0 },
    pause: () => {
      Object.assign(video, { paused: true });
      fire("pause");
    },
    play: () => {
      Object.assign(video, { paused: false });
      fire("play");
      return Promise.resolve();
    },
    fire,
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

  it("says the step-down its player foresees as it does and with each word after, until it is gone (D532 §1)", () => {
    const video = fakeVideo();
    const said: SaidInterest[] = [];
    const stop = sayInterestOf(video, (interest) => said.push(interest));
    const foresee = (detail: unknown): void => {
      video.dispatchEvent(new CustomEvent(playerForesawStepDownEvent, { detail }));
    };
    video.currentTime = 30;
    foresee({ pictureStream: "q3000k720p", fromMs: 36_000 });
    video.fire("pause");
    Object.assign(video, { paused: true });
    foresee(null);
    stop();
    foresee({ pictureStream: "q400k360p", fromMs: 42_000 });
    const step = { stepDownPictureStream: "q3000k720p", stepDownFromMs: 36_000 };
    expect(said).toEqual([
      { positionMs: 30_000, isPaused: false, ...step },
      { positionMs: 30_000, isPaused: true, ...step },
      { positionMs: 30_000, isPaused: true },
    ]);
  });

  it("says nothing of the pause that keeps the press for the start, but the next is the person's", () => {
    const video = fakeVideo();
    Object.assign(video, {
      paused: true,
      play: () => Promise.reject(new DOMException("paused", "AbortError")),
      pause: () => {
        video.fire("pause");
      },
    });
    const said: SaidInterest[] = [];
    const stop = sayInterestOf(video, (interest) => said.push(interest));
    keepThePressForTheStart(video);
    video.currentTime = 1668;
    video.fire("pause");
    stop();
    // A film on the owner's TCL TV said it paused at 0, and its resume's sound was never made.
    expect(said).toEqual([{ positionMs: 1_668_000, isPaused: true }]);
  });

  it("a seek while the film waits to resume says it plays", () => {
    const video = fakeVideo();
    const gate = startGateOf(video); // the page's, made as its stage is (useStartsWithEnoughAhead)
    const said: SaidInterest[] = [];
    const stop = sayInterestOf(video, (interest) => said.push(interest));
    video.currentTime = 6563;
    video.fire("seeking"); // to where nothing is loaded: the gate pauses it until enough is
    expect(video.paused).toBe(true);
    video.currentTime = 19_782;
    video.fire("seeking"); // again, while it waits
    stop();
    gate.cancel(); // its wait's looking again stops
    // Said paused, the core made each seek only the half minute ahead (file 15497, 2026-11-17).
    expect(said).toEqual([
      { positionMs: 6_563_000, isPaused: false },
      { positionMs: 19_782_000, isPaused: false },
    ]);
  });

  it("a seek while its person paused the film says it is paused, and so does Pause pressed while a seek waits", () => {
    const video = fakeVideo();
    const gate = startGateOf(video);
    const said: SaidInterest[] = [];
    const stop = sayInterestOf(video, (interest) => said.push(interest));
    video.currentTime = 600;
    video.pause(); // its person's
    video.currentTime = 6563;
    video.fire("seeking");
    void video.play();
    video.fire("playing");
    video.currentTime = 19_782;
    video.fire("seeking"); // held by the gate, until enough is loaded
    gate.cancel(); // Pause pressed while it waits (player-commands.ts's playOrPause)
    stop();
    expect(said).toEqual([
      { positionMs: 600_000, isPaused: true },
      { positionMs: 6_563_000, isPaused: true },
      { positionMs: 6_563_000, isPaused: false },
      { positionMs: 19_782_000, isPaused: false },
      { positionMs: 19_782_000, isPaused: true },
    ]);
  });

  it("is sent for its playback, and a leave says why", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy(
      {
        "PUT /playback/a-token/wants": [204, undefined],
        "DELETE /playback/a-token": [503, { error: { code: "updating", message: "back soon" } }],
      },
      heard,
    );
    sendInterest(api, { mediaToken: "a-token" }, { positionMs: 10_000, isPaused: true });
    leavePlayback(api, "a-token", "replaced");
    await vi.waitFor(() => {
      expect(heard).toHaveLength(2);
    });
    expect(heard).toContainEqual({
      methodAndPath: "PUT /playback/a-token/wants",
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
