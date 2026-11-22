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
  ownSoundGivenUpOf,
  playerOwnSoundGivenUpEvent,
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

/** A film's chunk sn, 2.3 s long, in the quality level, as hls.js holds it. */
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
  /** How many times its decoding was built anew (recoverMediaError). */
  rebuilds = 0;
  /**
   * Whether a rebuild's new media source is attached at once, or only when attach says, as a
   * browser's is once the loads the stop aborted have ended.
   */
  isAttachedAtOnce = true;
  readonly #video: SkippingVideo;
  readonly #onAttached: (() => void)[] = [];

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

  once(event: Events, listener: () => void): void {
    if (event === Events.MEDIA_ATTACHED) {
      this.#onAttached.push(listener);
    }
  }

  recoverMediaError(): void {
    this.rebuilds++;
    if (this.isAttachedAtOnce) {
      this.attach();
    }
  }

  /** Attaches a rebuild's new media source. */
  attach(): void {
    for (const attached of this.#onAttached.splice(0)) {
      attached();
    }
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
  skipChunksNotMade(asHls, video, { note: () => undefined });
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

  it("loads on past it only once its decoding is built anew, the loads its stop aborted ended", () => {
    // Started again at once, an init the stop aborted, failing after, aborted the chunk then loading,
    // and the init was never loaded (file 17137 on staging, 2026-11-16; idiosyncrasies.md).
    const video = new FilmVideo(0);
    const { hls, loadsFromS, skipped } = skipping(video);
    hls.isAttachedAtOnce = false;
    cannotMake(hls, 0); // 0 s to 2.3 s, where the film starts
    expect([hls.stops, hls.rebuilds, loadsFromS]).toEqual([1, 1, []]);
    hls.attach();
    expect(loadsFromS).toHaveLength(1);
    expect(loadsFromS[0]).toBeCloseTo(2.8); // into the next chunk
    expect(skipped).toEqual([{ fromMs: 0, toMs: 2300 }]);
  });

  it("leaves a chunk's init it cannot make to hls.js, which says it with the chunk", () => {
    // Stepped over as chunk 0, the init of file 17137's picture was never asked for again, and its
    // player stood with nothing loaded (staging, 2026-11-16; idiosyncrasies.md).
    const video = new FilmVideo(0);
    const { hls, loadsFromS, failures, skipped } = skipping(video);
    const first = chunk(0);
    const init = new Fragment(
      PlaylistLevelType.MAIN,
      "https://core.test/chunks/picture/playlist.m3u8",
    );
    init.sn = "initSegment";
    init.relurl = "init.mp4";
    first.initSegment = init;
    hls.trigger(Events.ERROR, {
      type: ErrorTypes.NETWORK_ERROR,
      details: ErrorDetails.FRAG_LOAD_ERROR,
      fatal: false,
      frag: first, // as hls.js 1.7.3 says an init's failure: with the chunk it loaded it for
      response: { url: init.url, code: 422, text: "" },
      error: new Error("HTTP Error 422"),
    });
    expect([hls.stops, loadsFromS, skipped, failures]).toEqual([0, [], [], []]);
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

describe("a chunk of the film's own picture the core cannot make (D500 §2)", () => {
  /**
   * hls.js loading quality loadLevel of a film whose own picture, copied, is level 1 above its twin,
   * level 0, the video at positionS, its qualities' give-ups counted, at the positions given.
   */
  function givingUp(
    positionS: number,
    loadLevel: number,
  ): {
    readonly hls: LoadingHls;
    readonly givenUpAtS: number[];
    readonly failures: PlaybackFailure[];
    readonly skipped: SkippedStretch[];
  } {
    const video = new FilmVideo(positionS);
    const hls = Object.assign(new LoadingHls(video), {
      levels: [{ uri: "q4000k1080p/stream.m3u8" }, { uri: "picture/stream.m3u8" }],
      loadLevel,
      hasEnoughToStart: true,
      startPosition: -1,
      media: video,
    });
    const asHls = hls as unknown as Hls;
    const givenUpAtS: number[] = [];
    const quality = {
      canGiveUpLoading: () => hls.loadLevel > 0,
      giveUpLoading: (atS: number) => {
        givenUpAtS.push(atS);
      },
    };
    const failures: PlaybackFailure[] = [];
    skipChunksNotMade(asHls, video, { note: () => undefined, quality: () => quality });
    failOnFatal(asHls, (failure) => failures.push(failure));
    const skipped: SkippedStretch[] = [];
    video.addEventListener(playerSkippedEvent, (event) => {
      const stretch = skippedStretchOf(event);
      if (stretch !== undefined) {
        skipped.push(stretch);
      }
    });
    return { hls, givenUpAtS, failures, skipped };
  }

  it("gives the copy up for the twin rather than stepping over it, as file 17099's was", () => {
    const { hls, givenUpAtS, failures, skipped } = givingUp(2520, 1);
    cannotMake(hls, 1100, { level: 1, isFatal: true });
    expect([givenUpAtS, hls.stops, skipped, failures]).toEqual([[2520], 0, [], []]);
  });

  it("steps over a chunk of the twin: an encode the core cannot make is the file's fault", () => {
    const { hls, givenUpAtS, skipped } = givingUp(2529.8, 0);
    cannotMake(hls, 1100, { level: 0 });
    expect([givenUpAtS, skipped]).toEqual([[], [{ fromMs: 2_530_000, toMs: 2_532_300 }]]);
  });
});

describe("hls.js at a chunk the core cannot make", () => {
  /**
   * hls.js with the player's controller, told that a chunk failed with code, a picture's of quality 2
   * or the one given; what it made of it.
   */
  function told(code: number, frag: Fragment = chunk(1100, 2)): ErrorData {
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
      frag,
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

  it("keeps the sound at a chunk of its own sound, copied, which the playback started again replaces (D533 §2)", () => {
    const frag = new Fragment(
      PlaylistLevelType.AUDIO,
      "/api/v1/chunks/token/sound1-own/stream.m3u8",
    );
    frag.level = 1;
    frag.sn = 18;
    const notMade = told(422, frag);
    expect(notMade.errorAction?.action).toBe(0); // NetworkErrorAction.DoNothing
    expect(notMade.fatal).toBe(false);
  });

  it("leaves a chunk the link failed to hls.js's own handling", () => {
    expect(told(503).errorAction?.action).not.toBe(0);
  });
});

/** hls.js's word that the core refused sn's chunk of the sound whose playlist is at playlist, 422. */
function soundCannotMake(
  hls: LoadingHls,
  sn: number,
  asked: { readonly playlist: string; readonly isFatal?: boolean },
): void {
  const frag = new Fragment(PlaylistLevelType.AUDIO, asked.playlist);
  frag.level = 0;
  frag.sn = sn;
  frag.start = sn * 2.002;
  frag.duration = 2.002;
  hls.trigger(Events.ERROR, {
    type: ErrorTypes.NETWORK_ERROR,
    details: ErrorDetails.FRAG_LOAD_ERROR,
    fatal: asked.isFatal ?? false,
    frag,
    response: { url: "", code: 422, text: "" },
    error: new Error("HTTP Error 422"),
  });
}

describe("a chunk of the film's own sound, copied, that the core cannot make (D533 §2)", () => {
  const ownSound = "https://orpheum.example/api/v1/chunks/token/sound0-own/stream.m3u8";

  it("asks the page once to start the playback again where the video stands, and does not end it", () => {
    const video = new FilmVideo(35.2);
    const { hls, failures, skipped } = skipping(video);
    const asked: unknown[] = [];
    video.addEventListener(playerOwnSoundGivenUpEvent, (event) => {
      asked.push(ownSoundGivenUpOf(event));
    });
    soundCannotMake(hls, 18, { playlist: ownSound });
    soundCannotMake(hls, 18, { playlist: ownSound, isFatal: true });
    expect(asked).toEqual([{ positionMs: 35_200 }]);
    expect(failures).toEqual([]);
    expect(hls.stops).toBeGreaterThan(0); // where hls.js would ask for it again
    expect(skipped).toEqual([]);
  });

  it("leaves a sound the server makes to fail as it did, the playback started again offering it the same", () => {
    const video = new FilmVideo(35.2);
    const { hls, failures } = skipping(video);
    const asked: unknown[] = [];
    video.addEventListener(playerOwnSoundGivenUpEvent, (event) => {
      asked.push(ownSoundGivenUpOf(event));
    });
    soundCannotMake(hls, 18, {
      playlist: "https://orpheum.example/api/v1/chunks/token/sound0-96k-ownlevel/stream.m3u8",
      isFatal: true,
    });
    expect(asked).toEqual([]);
    expect(failures).toEqual(["cannotMake"]);
  });
});
