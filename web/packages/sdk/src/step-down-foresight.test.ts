// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  foreseenStepDownOf,
  playerForesawStepDownEvent,
  stepDownForeseer,
} from "./step-down-foresight.ts";

describe("a step-down foreseen, told the video", () => {
  it("fires the event with the step, and null for none", () => {
    const target = new EventTarget();
    const details: unknown[] = [];
    target.addEventListener(playerForesawStepDownEvent, (event) => {
      details.push(foreseenStepDownOf(event));
    });
    const foresee = stepDownForeseer(target);
    foresee({ pictureStream: "q3000k720p", fromMs: 36_000 });
    foresee(undefined);
    expect(details).toEqual([{ pictureStream: "q3000k720p", fromMs: 36_000 }, undefined]);
  });

  it("is read from the event as none where its detail is not a step-down", () => {
    const of = (detail: unknown): unknown =>
      foreseenStepDownOf(new CustomEvent(playerForesawStepDownEvent, { detail }));
    expect(of({ pictureStream: "q3000k720p", fromMs: "36000" })).toBeUndefined();
    expect(of({ pictureStream: 7, fromMs: 36_000 })).toBeUndefined();
    expect(of("q3000k720p")).toBeUndefined();
    expect(foreseenStepDownOf(new Event(playerForesawStepDownEvent))).toBeUndefined();
  });
});
