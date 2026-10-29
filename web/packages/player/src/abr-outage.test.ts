// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { lookAtChunkEveryMs } from "./abr.ts";
import {
  chunkAt,
  choosingFrom,
  load,
  scene,
  useScene,
  videoAt,
  type ChosenVideo,
} from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

/** A video asked to play at positionS, holding what it plays, picture and sound, from fromS to toS. */
function videoHolding(positionS: number, fromS: number, toS: number): ChosenVideo {
  return {
    currentTime: positionS,
    paused: false,
    readyState: 2,
    buffered: { length: 1, start: () => fromS, end: () => toS },
  };
}

/** A playback at the top quality on a fast link, its video as given, standing still from 0. */
function standingAtTheTop(video: ChosenVideo): ReturnType<typeof choosingFrom> {
  const chosen = choosingFrom(40_000_000);
  chosen.hls.loadLevel = 3;
  chosen.hls.media = video;
  chosen.choosing.firstLevel();
  vi.advanceTimersByTime(lookAtChunkEveryMs);
  return chosen;
}

/** The controller looks at its video at nowMs. */
function lookAt(nowMs: number): void {
  scene.nowMs = nowMs;
  vi.advanceTimersByTime(lookAtChunkEveryMs);
}

// A minute's outage, reproduced in headless Chrome with real hls.js (2026-10-29): the top quality's
// video ran out what it held and stood 0.07 s short of its end, hls.js, its loading stopped, saying
// its picture was loaded 33.9 s ahead; 6 s later the quality was judged one the device cannot play,
// given up for good, and its decoding rebuilt, which put the video at 0 (D266).
describe("a quality standing still in an outage", () => {
  it("is not judged unplayable while the video holds next to nothing ahead, whatever hls.js says, and says why", () => {
    const { hls, unplayable } = standingAtTheTop(videoHolding(67.94, 34.08, 68.01));
    hls.mainForwardBufferInfo = { len: 33.9 };
    for (let nowMs = 1000; nowMs <= 60_000; nowMs += 1000) {
      lookAt(nowMs);
    }
    expect(unplayable).toEqual([]);
    expect(scene.linkNotes).toEqual([
      "quality 3 not judged unplayable, too little loaded ahead, the link's to bring: stood still" +
        " at 67.9 s for 6000 ms, 0 ms of it with 1 s loaded ahead; 0.1 s ahead, 33.9 s of picture," +
        " 0 chunks arrived, 0 not held",
    ]);
  });

  it("counts its time afresh at each load that fails: chunks arriving as the link comes back condemn nothing, standing still after them does", () => {
    const { choosing, unplayable } = standingAtTheTop(
      videoAt(420.5, { isPaused: false, readyState: 1 }),
    );
    for (let nowMs = 1000; nowMs <= 5000; nowMs += 1000) {
      choosing.failed(chunkAt(3), "fragLoadError");
      lookAt(nowMs);
    }
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    lookAt(6500);
    expect(unplayable).toEqual([]);
    expect(scene.linkNotes.at(-1)).toMatch(
      /^quality 3 not judged unplayable, its loads not arriving: a chunk failed \(fragLoadError\): stood still at 420\.5 s for 6500 ms, 1500 ms of it/u,
    );
    // Its loads arrive, and it stands still on: the decoder's, as D266 says.
    lookAt(11_000);
    expect(unplayable).toEqual([
      { qualityKbps: 36_208, positionMs: 420_500, isFilmsOwnPicture: false },
    ]);
    expect(scene.linkNotes.at(-1)).toMatch(
      /^quality 3 judged unplayable: stood still at 420\.5 s for 11000 ms, 6000 ms of it with 1 s loaded ahead; 15\.0 s ahead, 0\.0 s of picture, 2 chunks arrived, 0 not held$/u,
    );
  });

  it("counts its time afresh when its stopped loading is asked again, and while the browser is offline", () => {
    const stopped = standingAtTheTop(videoAt(420.5, { isPaused: false, readyState: 1 }));
    stopped.hls.mainForwardBufferInfo = { len: 30 };
    lookAt(5000);
    stopped.choosing.askingAgain(); // hls.js stopped its loading for a load that failed
    lookAt(10_000);
    expect(stopped.unplayable).toEqual([]);

    scene.nowMs = 0;
    const offline = standingAtTheTop(videoAt(420.5, { isPaused: false, readyState: 1 }));
    offline.hls.mainForwardBufferInfo = { len: 30 };
    scene.isOffline = true;
    for (let nowMs = 1000; nowMs <= 30_000; nowMs += 1000) {
      lookAt(nowMs);
    }
    expect(offline.unplayable).toEqual([]);
    expect(scene.linkNotes.at(-1)).toMatch(
      /^quality 3 not judged unplayable, its loads not arriving: the browser offline: /u,
    );
    scene.isOffline = false;
    lookAt(31_000);
    lookAt(37_000);
    expect(offline.unplayable).toHaveLength(1);
  });
});
