// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ErrorDetails } from "hls.js";

import { lookAtChunkEveryMs } from "./abr.ts";
import { asksAgainAfterAtMostMs, asksAgainAfterMs } from "./abr-choice.ts";
import { isLoadFailure } from "./abr-controller.ts";
import { chunkAt, choosingFrom, scene, useScene, waitMs } from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

// The owner's Chrome over WebRTC, throttled until its film stalled and unthrottled: a chunk stuck
// after 19 bytes was noted, not given up, and the player never asked for anything again (staging,
// 2026-10-29; VQ125).
describe("a chunk stuck on its way", () => {
  it("asks again for a chunk stuck after some of it came, at any quality, each time in a row after twice the wait, until the link comes back", () => {
    const { choosing, givenUp } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(0); // the lowest, which no slowness gives up
    let loading = chunk;
    /** Waits ms a look at a time, as the clock runs. */
    const lookFor = (ms: number): void => {
      for (let lookedMs = 0; lookedMs < ms; lookedMs += lookAtChunkEveryMs) {
        waitMs(lookAtChunkEveryMs);
      }
    };
    /** hls.js asks for loading, and 19 bytes of it come by the next look, then nothing. */
    const askedFor = (): void => {
      choosing.loading(loading);
      loading.stats.loaded = 19;
      loading.stats.loading = { start: scene.nowMs, first: scene.nowMs + 20, end: 0 };
      lookFor(lookAtChunkEveryMs);
    };
    askedFor();
    lookFor(asksAgainAfterMs - lookAtChunkEveryMs);
    expect(givenUp).toEqual([]);
    lookFor(lookAtChunkEveryMs);
    expect(givenUp).toEqual([chunk]);
    expect(scene.linkNotes.at(-1)).toBe(
      "asked again for a chunk of 0, time 1 in a row, after 8000 ms with nothing: stuck: 19 B in 8500 ms, none for 8000 ms; carried WebRTC connection 2 for /c/0.m4s",
    );
    // Asked again, it sticks again: asked again after twice the wait, and never more than the most.
    askedFor();
    lookFor(2 * asksAgainAfterMs - lookAtChunkEveryMs);
    expect(givenUp).toHaveLength(1);
    lookFor(lookAtChunkEveryMs);
    expect(givenUp).toHaveLength(2);
    expect(scene.linkNotes.at(-1)).toMatch(
      /^asked again for a chunk of 0, time 2 in a row, after 16000 ms/u,
    );
    askedFor();
    lookFor(asksAgainAfterAtMostMs);
    expect(givenUp).toHaveLength(3);
    // The link is back: its bytes arrive, it loads, and the next chunk stuck waits the first wait again.
    askedFor();
    for (let atMs = 0; atMs < 3 * asksAgainAfterAtMostMs; atMs += lookAtChunkEveryMs) {
      chunk.stats.loaded += 50_000;
      lookFor(lookAtChunkEveryMs);
    }
    expect(givenUp).toHaveLength(3);
    choosing.loaded(chunk);
    loading = chunkAt(0);
    askedFor();
    lookFor(asksAgainAfterMs);
    expect(givenUp.at(-1)).toBe(loading);
    expect(scene.linkNotes.at(-1)).toMatch(/^asked again for a chunk of 0, time 1 in a row/u);
  });

  it("leaves a chunk not yet answered to hls.js's own wait, the core perhaps holding it as it makes it", () => {
    const { choosing, givenUp } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(0);
    choosing.loading(chunk);
    waitMs(asksAgainAfterAtMostMs * 2);
    expect(givenUp).toEqual([]);
  });

  it("notes a chunk whose load failed or timed out with what carried it, the core's 425 aside, so a loader waiting on a dead connection shows in the log", () => {
    const timedOut = { details: ErrorDetails.FRAG_LOAD_TIMEOUT };
    const failed = { details: ErrorDetails.FRAG_LOAD_ERROR, response: { url: "", code: 0 } };
    const made = { details: ErrorDetails.FRAG_LOAD_ERROR, response: { url: "", code: 425 } };
    expect([timedOut, failed, made].map((data) => isLoadFailure(data))).toEqual([
      true,
      true,
      false,
    ]);
    const { choosing } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(2);
    choosing.loading(chunk);
    choosing.failed(chunk, ErrorDetails.FRAG_LOAD_TIMEOUT);
    expect(scene.linkNotes.at(-1)).toBe(
      "a chunk of 2 failed to load (fragLoadTimeOut); carried WebRTC connection 2 for /c/2.m4s",
    );
  });
});
