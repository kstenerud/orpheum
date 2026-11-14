// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { lookAtChunkEveryMs } from "./abr.ts";
import { asksAgainAfterMs } from "./abr-choice.ts";
import {
  chunkAt,
  choosingFrom,
  fireTvPlaying,
  scene,
  topChunkOnItsWay,
  useScene,
  waitMs,
} from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

// The page's outages, by which a film loads deeper ahead after its link was out, start at each sign
// the choosing tells (D487): the owner's phone, out 6.4 minutes mid-film, held 30 s (staging,
// 2026-11-14).
describe("a choosing tells each sign that its link was lost (D487)", () => {
  it("tells a chunk of picture that failed and a load hls.js stopped, as the video plays on", () => {
    const { choosing } = choosingFrom(40_000_000);
    choosing.firstLevel();
    choosing.failed(chunkAt(0), "fragLoadError");
    choosing.askingAgain();
    expect(scene.linkLost).toEqual(["a chunk failed (fragLoadError)", "its loading stopped"]);
  });

  it("tells the browser offline at each look while it says so, and nothing once it is back", () => {
    choosingFrom(40_000_000).choosing.firstLevel();
    scene.isOffline = true;
    waitMs(lookAtChunkEveryMs);
    waitMs(lookAtChunkEveryMs);
    expect(scene.linkLost).toEqual(["the browser offline", "the browser offline"]);
    scene.isOffline = false;
    waitMs(lookAtChunkEveryMs);
    expect(scene.linkLost).toHaveLength(2);
  });

  it("tells a chunk stuck on its way", () => {
    const { choosing } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(0); // the lowest, which no slowness gives up
    choosing.loading(chunk);
    chunk.stats.loaded = 19;
    chunk.stats.loading = { start: scene.nowMs, first: scene.nowMs + 20, end: 0 };
    for (let waitedMs = 0; waitedMs <= asksAgainAfterMs; waitedMs += lookAtChunkEveryMs) {
      waitMs(lookAtChunkEveryMs);
    }
    expect(scene.linkLost).toContain("a chunk stuck on its way");
  });

  it("tells nothing of a chunk given up for its slowness, which is the link's speed, not its loss", () => {
    const chosen = fireTvPlaying();
    topChunkOnItsWay(chosen, { pieceAtMs: 500, afterBytes: 375_000 });
    expect(chosen.givenUp).toHaveLength(1);
    expect(scene.linkLost).toEqual([]);
  });
});
