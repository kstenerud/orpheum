// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { pictureStreamOf, playerForesawStepDownEvent, stepDownForeseer } from "./step-foresight.ts";

describe("the stream a quality's playlist address names (D532 §1)", () => {
  it("is the folder its stream.m3u8 is in, past any query, and none for another address", () => {
    expect(pictureStreamOf("q3000k720p/stream.m3u8")).toBe("q3000k720p");
    expect(pictureStreamOf("/api/v1/x/picture/stream.m3u8?t=1#a")).toBe("picture");
    expect(pictureStreamOf("stream.m3u8")).toBeUndefined();
    expect(pictureStreamOf("/stream.m3u8")).toBeUndefined();
    expect(pictureStreamOf("q3000k720p/other.m3u8")).toBeUndefined();
    expect(pictureStreamOf(undefined)).toBeUndefined();
  });
});

describe("a step-down foreseen, told the video", () => {
  it("fires the event with the step, and null for none", () => {
    const target = new EventTarget();
    const details: unknown[] = [];
    target.addEventListener(playerForesawStepDownEvent, (event) => {
      details.push((event as CustomEvent).detail);
    });
    const foresee = stepDownForeseer(target);
    foresee({ pictureStream: "q3000k720p", fromMs: 36_000 });
    foresee(undefined);
    expect(details).toEqual([{ pictureStream: "q3000k720p", fromMs: 36_000 }, null]);
  });
});
