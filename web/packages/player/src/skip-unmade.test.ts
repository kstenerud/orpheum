// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import Hls, {
  ErrorDetails,
  ErrorTypes,
  Events,
  Fragment,
  PlaylistLevelType,
  type ErrorData,
} from "hls.js";

import { orpheumAbr } from "./abr-controller.ts";

import type { PlaybackFailure } from "./failure.ts";
import { failOnFatal } from "./hls.ts";
import {
  playerSkippedEvent,
  skipChunksNotMade,
  skippedStretchOf,
  type SkippedStretch,
  type SkippingVideo,
} from "./skip-unmade.ts";
import { HlsEvents } from "./test-hls.ts";

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

/** The Gold Rush's chunk sn, 2.3 s long, in the quality level, as hls.js holds it. */
function chunk(sn: number, level = 0): Fragment {
  const frag = new Fragment(PlaylistLevelType.MAIN, "");
  frag.level = level;
  frag.sn = sn;
  frag.start = sn * 2.3;
  frag.duration = 2.3;
  return frag;
}

/** A stand-in for hls.js as a playback's loading is started and stopped. */
class LoadingHls extends HlsEvents {
  /**
   * Where the video stood each time loading was started, in seconds of the film: where hls.js
   * loads from, whatever startLoad is given (staging, 2026-11-03).
   */
  readonly loadsFromS: number[] = [];
  /** How many times loading was stopped. */
  stops = 0;
  readonly #video: SkippingVideo;

  constructor(video: SkippingVideo) {
    super();
    this.#video = video;
  }

  startLoad(): void {
    this.loadsFromS.push(this.#video.currentTime);
  }

  stopLoad(): void {
    this.stops++;
  }
}

/** A chunked playback's hls.js stepping over chunks not made, and failing as failOnFatal tells. */
function skipping(video: FilmVideo): {
  readonly hls: LoadingHls;
  readonly loadsFromS: number[];
  readonly failures: PlaybackFailure[];
  readonly skipped: SkippedStretch[];
} {
  const hls = new LoadingHls(video);
  const asHls = hls as unknown as Hls;
  const failures: PlaybackFailure[] = [];
  skipChunksNotMade(asHls, video, () => undefined);
  failOnFatal(asHls, (failure) => failures.push(failure));
  const skipped: SkippedStretch[] = [];
  video.addEventListener(playerSkippedEvent, (event) => {
    const stretch = skippedStretchOf(event);
    if (stretch !== undefined) {
      skipped.push(stretch);
    }
  });
  return { hls, loadsFromS: hls.loadsFromS, failures, skipped };
}

/**
 * hls.js's word that the core cannot make sn's chunk (422) in the quality level, as it gives it after
 * its first try, kept from trying another quality (abr-controller.ts's keepTheQualityAtNotMade): fatal where
 * isFatal says, as a film with one quality's was before that.
 */
function cannotMake(
  hls: LoadingHls,
  sn: number,
  asked: { readonly level?: number; readonly isFatal?: boolean } = {},
): void {
  hls.trigger(Events.ERROR, {
    type: ErrorTypes.NETWORK_ERROR,
    details: ErrorDetails.FRAG_LOAD_ERROR,
    fatal: asked.isFatal ?? false,
    frag: chunk(sn, asked.level),
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
    expect(video.currentTime).toBeCloseTo(2532.8); // into the next chunk
    expect(loadsFromS).toEqual([2532.8]);
    expect(skipped).toEqual([{ fromMs: 2_530_000, toMs: 2_532_300 }]);
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
  });

  it("ends the playback where the stretch would pass two minutes", () => {
    const video = new FilmVideo(2529.9);
    const { hls, failures, skipped } = skipping(video);
    for (let sn = 1100; sn < 1100 + 52; sn++) {
      cannotMake(hls, sn); // 52 chunks of 2.3 s, 119.6 s
    }
    expect(failures).toEqual([]);
    cannotMake(hls, 1152);
    expect(failures).toEqual(["cannotMake"]);
    expect(skipped.slice(-2)).toEqual([
      { fromMs: 2_530_000, toMs: 2_649_600 },
      { fromMs: 2_530_000, toMs: 2_651_900, isWhereItStops: true },
    ]);
  });

  it("ends the playback at the film's last chunk, there being nothing after it to play", () => {
    const video = new FilmVideo(2529.9, 2532.3);
    const { hls, failures, skipped } = skipping(video);
    cannotMake(hls, 1100);
    expect(failures).toEqual(["cannotMake"]);
    expect(skipped).toEqual([{ fromMs: 2_530_000, toMs: 2_532_300, isWhereItStops: true }]);
  });

  it("steps over a chunk of a film with several qualities at its first refusal, in the quality asked", () => {
    const video = new FilmVideo(2520);
    const { hls, loadsFromS, failures, skipped } = skipping(video);
    cannotMake(hls, 1100, { level: 2 }); // a fault of the file: no other quality is tried
    expect(hls.stops).toBe(1);
    video.playTo(2529.8);
    expect(failures).toEqual([]);
    expect(loadsFromS).toEqual([2532.8]);
    expect(skipped).toEqual([{ fromMs: 2_530_000, toMs: 2_532_300 }]);
  });

  it("steps over a chunk whose error hls.js said was fatal, as it did for a film of one quality", () => {
    const video = new FilmVideo(2529.9);
    const { hls, failures, skipped } = skipping(video);
    cannotMake(hls, 1100, { isFatal: true });
    expect(failures).toEqual([]);
    expect(skipped).toEqual([{ fromMs: 2_530_000, toMs: 2_532_300 }]);
  });

  it("ends the playback of a film with several qualities where it cannot go on", () => {
    const video = new FilmVideo(2529.9, 2532.3);
    const { hls, failures, skipped } = skipping(video);
    cannotMake(hls, 1100, { level: 1 });
    expect(failures).toEqual(["cannotMake"]);
    expect(skipped).toEqual([{ fromMs: 2_530_000, toMs: 2_532_300, isWhereItStops: true }]);
  });

  it("goes on loading from where the person went, where they seek before it is reached", () => {
    const video = new FilmVideo(2520);
    const { hls, loadsFromS, skipped } = skipping(video);
    cannotMake(hls, 1100);
    video.currentTime = 100;
    expect(loadsFromS).toEqual([100]);
    video.playTo(2531);
    expect(skipped).toEqual([]);
  });
});

describe("hls.js at a chunk the core cannot make", () => {
  /** hls.js with the player's controller, told that sn's chunk failed with code; what it made of it. */
  function told(code: number): ErrorData {
    const abr = orpheumAbr({
      startAtBps: 0,
      monotonicMs: () => 0,
      isVoiceLive: () => false,
      fromCache: () => undefined,
    });
    const hls = new Hls({ abrController: abr.controller });
    const data: ErrorData = {
      type: ErrorTypes.NETWORK_ERROR,
      details: ErrorDetails.FRAG_LOAD_ERROR,
      fatal: false,
      frag: chunk(1100, 2),
      response: { url: "", code, text: "" },
      error: new Error(`HTTP Error ${String(code)}`),
    };
    hls.trigger(Events.ERROR, data);
    hls.destroy();
    return data;
  }

  it("keeps the quality: a fault of the file is no reason to step down (the owner, 2026-11-03)", () => {
    const notMade = told(422);
    expect(notMade.errorAction?.action).toBe(0); // NetworkErrorAction.DoNothing
    expect(notMade.errorAction?.nextAutoLevel).toBeUndefined();
    expect(notMade.fatal).toBe(false);
  });

  it("leaves a chunk the link failed to hls.js's own handling", () => {
    expect(told(503).errorAction?.action).not.toBe(0);
  });
});
