// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { startingTuning } from "./together.ts";
import { aimAheadAtMostMs, LoadSpeed, waitAheadAtMostMs } from "./together-load-speed.ts";

/** A LoadSpeed told three chunks of picture, each of filmMs loaded in loadMs. */
function measuredBy(chunk: { readonly filmMs: number; readonly loadMs: number }): LoadSpeed {
  const speed = new LoadSpeed();
  for (let count = 0; count < 3; count += 1) {
    speed.chunkLoaded({ track: "video", loadedKbps: 0, qualityKbps: 0, ...chunk });
  }
  return speed;
}

describe("how far ahead of its group a catch-up aims (D250 §4)", () => {
  it("counts its last seek's load no more than aimAheadAtMostMs, a seek that waited out an outage too (VQ133)", () => {
    const speed = measuredBy({ filmMs: 4000, loadMs: 1600 });
    // Its last seek's load waited out a minute without Wi-Fi: no time to load at its link's speed.
    expect(speed.catchUpLeadMs(70_000, 1, startingTuning)).toEqual({
      leadMs: aimAheadAtMostMs + 9600,
      by: "fill",
    });
  });

  it("measures its link by the chunks of picture it loaded, none from the cache nor of sound", () => {
    const speed = measuredBy({ filmMs: 4000, loadMs: 1600 });
    speed.chunkLoaded({
      track: "audio",
      loadMs: 16_000,
      loadedKbps: 0,
      qualityKbps: 0,
      filmMs: 4000,
    });
    speed.chunkLoaded({ track: "video", loadMs: 2, loadedKbps: 0, qualityKbps: 0, filmMs: 4000 });
    expect(speed.ratio()).toBe(2.5);
  });

  it("waits no further ahead than a person waits, on a link slower than that, and by lastLoad alone with no measure", () => {
    expect(
      measuredBy({ filmMs: 4000, loadMs: 16_000 }).catchUpLeadMs(2000, 1, startingTuning),
    ).toEqual({ leadMs: waitAheadAtMostMs, by: "mostAhead" });
    expect(new LoadSpeed().catchUpLeadMs(2000, 1, startingTuning)).toEqual({
      leadMs: 2000,
      by: "lastLoad",
    });
  });
});
