// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Hls, { ErrorDetails, ErrorTypes, Events, Fragment, PlaylistLevelType } from "hls.js";

import type { PlaybackFailure } from "./failure.ts";
import { failOnFatal } from "./hls.ts";
import {
  playerSkippedEvent,
  skipChunksNotMade,
  skippedStretchOf,
  type SkippedStretch,
  type SkippingVideo,
} from "./skip-unmade.ts";

beforeEach(() => {
  // hls.js reads the page's clock through self, which a page has and node has not.
  vi.stubGlobal("self", globalThis);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

/** A video at positionS in a film durationS long, a seek firing seeking as a browser's does. */
class FilmVideo extends EventTarget implements SkippingVideo {
  #at: number;
  readonly duration: number;

  constructor(positionS: number, durationS = 6000) {
    super();
    this.#at = positionS;
    this.duration = durationS;
  }

  get currentTime(): number {
    return this.#at;
  }

  set currentTime(positionS: number) {
    this.#at = positionS;
    this.dispatchEvent(new Event("seeking"));
  }

  /** Plays on to positionS, as the film does, telling the time as it goes. */
  playTo(positionS: number): void {
    this.#at = positionS;
    this.dispatchEvent(new Event("timeupdate"));
  }
}

/** The Gold Rush's chunk sn, 2.3 s long, as hls.js holds it. */
function chunk(sn: number): Fragment {
  const frag = new Fragment(PlaylistLevelType.MAIN, "");
  frag.level = 0;
  frag.sn = sn;
  frag.start = sn * 2.3;
  frag.duration = 2.3;
  return frag;
}

/** A chunked playback's hls.js stepping over chunks not made, and failing as failOnFatal tells. */
function skipping(video: FilmVideo): {
  readonly hls: Hls;
  readonly loadsFromS: number[];
  readonly failures: PlaybackFailure[];
  readonly skipped: SkippedStretch[];
} {
  const hls = new Hls();
  const loadsFromS: number[] = [];
  Object.defineProperty(hls, "startLoad", { value: (at: number) => loadsFromS.push(at) });
  const failures: PlaybackFailure[] = [];
  skipChunksNotMade(hls, video, () => undefined);
  failOnFatal(hls, (failure) => failures.push(failure));
  const skipped: SkippedStretch[] = [];
  video.addEventListener(playerSkippedEvent, (event) => {
    const stretch = skippedStretchOf(event);
    if (stretch !== undefined) {
      skipped.push(stretch);
    }
  });
  return { hls, loadsFromS, failures, skipped };
}

/** hls.js's word that the core cannot make sn's chunk (422), as it gives it after its first try. */
function cannotMake(hls: Hls, sn: number): void {
  hls.trigger(Events.ERROR, {
    type: ErrorTypes.NETWORK_ERROR,
    details: ErrorDetails.FRAG_LOAD_ERROR,
    fatal: true,
    frag: chunk(sn),
    response: { url: "", code: 422, text: "" },
    error: new Error("HTTP Error 422"),
  });
}

describe("a chunk the core cannot make (toolkit.md item 12)", () => {
  it("is stepped over once the film reaches it, the page told, where the playback ended", () => {
    const video = new FilmVideo(2520);
    const { hls, loadsFromS, failures, skipped } = skipping(video);
    cannotMake(hls, 1100); // 2530 s to 2532.3 s, loaded ten seconds ahead
    expect(video.currentTime).toBe(2520);
    video.playTo(2529.8);
    expect(failures).toEqual([]);
    expect(video.currentTime).toBeCloseTo(2532.3);
    expect(loadsFromS).toEqual([video.currentTime]);
    expect(skipped).toEqual([{ fromMs: 2_530_000, toMs: 2_532_300 }]);
    hls.destroy();
  });

  it("counts chunks in a row not made as one stretch, which grows", () => {
    const video = new FilmVideo(2529.9);
    const { hls, skipped } = skipping(video);
    cannotMake(hls, 1100);
    cannotMake(hls, 1101);
    expect(skipped).toEqual([
      { fromMs: 2_530_000, toMs: 2_532_300 },
      { fromMs: 2_530_000, toMs: 2_534_600 },
    ]);
    hls.destroy();
  });

  it("ends the playback where the stretch would pass two minutes", () => {
    const video = new FilmVideo(2529.9);
    const { hls, failures, skipped } = skipping(video);
    for (let sn = 1100; sn < 1100 + 52; sn++) {
      cannotMake(hls, sn); // 52 chunks of 2.3 s, 119.6 s
    }
    expect(failures).toEqual([]);
    cannotMake(hls, 1152);
    expect(failures).toEqual(["cannot_make"]);
    expect(skipped.slice(-2)).toEqual([
      { fromMs: 2_530_000, toMs: 2_649_600 },
      { fromMs: 2_530_000, toMs: 2_651_900, cannotGoOn: true },
    ]);
    hls.destroy();
  });

  it("ends the playback at the film's last chunk, there being nothing after it to play", () => {
    const video = new FilmVideo(2529.9, 2532.3);
    const { hls, failures, skipped } = skipping(video);
    cannotMake(hls, 1100);
    expect(failures).toEqual(["cannot_make"]);
    expect(skipped).toEqual([{ fromMs: 2_530_000, toMs: 2_532_300, cannotGoOn: true }]);
    hls.destroy();
  });

  it("goes on loading from where the person went, where they seek before it is reached", () => {
    const video = new FilmVideo(2520);
    const { hls, loadsFromS, skipped } = skipping(video);
    cannotMake(hls, 1100);
    video.currentTime = 100;
    expect(loadsFromS).toEqual([100]);
    video.playTo(2531);
    expect(skipped).toEqual([]);
    hls.destroy();
  });
});
