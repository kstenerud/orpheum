// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Hls, { ErrorDetails, ErrorTypes, Events, Fragment, PlaylistLevelType } from "hls.js";

import { asksAgainAfterAtMostMs, asksAgainAfterMs } from "./abr-choice.ts";
import { loadAgainFrom, orpheumAbr } from "./abr-controller.ts";
import type { PlaybackFailure } from "./failure.ts";
import { failOnFatal } from "./hls.ts";
import { loadOnAfterFailures } from "./load-again.ts";
import { levels, scene, useScene, videoAt, waitMs } from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

beforeEach(() => {
  // hls.js reads the page's clock through self, which a page has and node has not.
  vi.stubGlobal("self", globalThis);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

/** What carries each load, as the SDK's mediaCarrierOf says it. */
const carrierOf = (url: string): string => `WebRTC connection 3 for ${url}`;

/**
 * A chunked playback's hls.js with its own bitrate controller, its video playing at 42 s or as given, a film's
 * four qualities as hls.js holds them, loading again after a load that failed (loadOnAfterFailures),
 * and failing as failOnFatal tells; page is the window whose online event it hears.
 */
function playbackLoadingOn(
  page: EventTarget,
  video = videoAt(42, { isPaused: false, readyState: 4 }),
): {
  readonly hls: Hls;
  readonly loadsOnFromS: number[];
  readonly failures: [PlaybackFailure, string | undefined][];
} {
  const abr = orpheumAbr({
    startAtBps: 0,
    monotonicMs: () => scene.nowMs,
    isVoiceLive: () => false,
    fromCache: () => undefined,
    noteLink: (note) => scene.linkNotes.push(note),
    carrierOf,
  });
  const hls = new Hls({ abrController: abr.controller });
  Object.defineProperty(hls, "media", { value: video });
  // hls.js's own qualities, each with the counts its error handling keeps.
  const held = levels.map((level) => ({
    ...level,
    maxBitrate: level.bitrate,
    fragmentError: 0,
    loadError: 0,
    audioCodec: "mp4a.40.2",
    codecSet: "avc1",
    details: {},
    attrs: {},
    hasAudioGroup: () => false,
    hasSubtitleGroup: () => false,
  }));
  Object.defineProperty(hls, "levels", { value: held });
  Object.defineProperty(hls, "loadLevel", { value: 3, writable: true });
  Object.defineProperty(hls, "minAutoLevel", { value: 0 });
  Object.defineProperty(hls, "maxAutoLevel", { value: 3 });
  Object.defineProperty(hls, "autoLevelEnabled", { value: true });
  const loadsOnFromS: number[] = [];
  Object.defineProperty(hls, "startLoad", { value: (at: number) => loadsOnFromS.push(at) });
  const failures: [PlaybackFailure, string | undefined][] = [];
  loadOnAfterFailures(hls, {
    askingAgain: () => abr.choosing()?.askingAgain(),
    note: (note) => scene.linkNotes.push(note),
    carrierOf,
    page,
  });
  failOnFatal(hls, (failure, whatFailed) => failures.push([failure, whatFailed]), carrierOf);
  return { hls, loadsOnFromS, failures };
}

/** A picture chunk of level, its number sn, as hls.js's loader asks for it. */
function pictureChunk(level: number, sn: number): Fragment {
  const frag = new Fragment(PlaylistLevelType.MAIN, "");
  frag.level = level;
  frag.sn = sn;
  frag.duration = 4;
  frag.relurl = `/c/${String(level)}/${String(sn)}.m4s`;
  Reflect.set(frag, "_url", `https://core/c/${String(level)}/${String(sn)}.m4s`);
  return frag;
}

/**
 * The loader fails every chunk, as an offline page's does, its status 0, until hls.js gives the
 * playback's loading up as fatal, each quality given up in turn: how many failed.
 */
function failEveryChunkUntilFatal(hls: Hls): number {
  for (let failed = 1; failed <= 20; failed++) {
    const data = {
      type: ErrorTypes.NETWORK_ERROR,
      details: ErrorDetails.FRAG_LOAD_ERROR,
      fatal: false,
      frag: pictureChunk(hls.loadLevel, 7),
      response: { url: "", code: 0, text: "" },
      error: new Error("HTTP Error 0 ()"),
    };
    hls.trigger(Events.ERROR, data);
    if (data.fatal) {
      return failed;
    }
    // hls.js loads the next at the quality it switched to.
    const switchedTo: unknown = Reflect.get(
      Reflect.get(data, "errorAction") ?? {},
      "nextAutoLevel",
    );
    if (typeof switchedTo === "number") {
      Reflect.set(hls, "loadLevel", switchedTo);
    }
  }
  throw new Error("hls.js never gave the loading up");
}

/** The loader loads a picture chunk at last, as hls.js says. */
function loadAChunk(hls: Hls): void {
  const frag = pictureChunk(0, 8);
  frag.stats.loaded = 100_000;
  frag.stats.loading = { start: scene.nowMs, first: scene.nowMs + 20, end: scene.nowMs + 120 };
  hls.trigger(Events.FRAG_LOADED, {
    frag,
    part: null,
    payload: new ArrayBuffer(0),
    networkDetails: null,
  });
}

// The owner's Chrome over WebRTC, set offline: a load failed, hls.js gave its loading up as fatal,
// the player took that for the end of its playback, played its minute out and stood still though
// the link came back, saying only "the browser gave no reason" (staging, 2026-10-29; VQ125).
describe("a load that failed for good", () => {
  it("never ends the playback: hls.js is asked to load on from where the video stands, after the waits of asking again, until a chunk loads", () => {
    const { hls, loadsOnFromS, failures } = playbackLoadingOn(new EventTarget());
    expect(failEveryChunkUntilFatal(hls)).toBeGreaterThan(1); // each quality given up first
    expect(failures).toEqual([]);
    expect(scene.linkNotes.at(-1)).toBe(
      "loading stopped: networkError fragLoadError (fatal): HTTP Error 0 (); status 0; carried" +
        " WebRTC connection 3 for https://core/c/0/7.m4s; asking again in 8000 ms, time 1 in a row",
    );
    waitMs(asksAgainAfterMs - 1);
    expect(loadsOnFromS).toEqual([]);
    waitMs(1);
    expect(loadsOnFromS).toEqual([42]);
    expect(scene.linkNotes.at(-1)).toBe("asked again to load from 42.0 s, after 8000 ms");
    // Still offline: it fails at once again, and waits twice as long, never more than the most.
    failEveryChunkUntilFatal(hls);
    expect(scene.linkNotes.at(-1)).toMatch(/asking again in 16000 ms, time 2 in a row$/u);
    waitMs(asksAgainAfterAtMostMs);
    failEveryChunkUntilFatal(hls);
    expect(scene.linkNotes.at(-1)).toMatch(/asking again in 16000 ms, time 3 in a row$/u);
    waitMs(asksAgainAfterAtMostMs);
    expect(loadsOnFromS).toEqual([42, 42, 42]);
    // The link is back: a chunk loads, and a load that fails after waits the first wait again.
    loadAChunk(hls);
    failEveryChunkUntilFatal(hls);
    expect(scene.linkNotes.at(-1)).toMatch(/asking again in 8000 ms, time 1 in a row$/u);
    expect(failures).toEqual([]);
    hls.destroy();
  });

  it("asks hls.js to load on at once when the browser is back online, not waiting out its wait", () => {
    const page = new EventTarget();
    const { hls, loadsOnFromS } = playbackLoadingOn(page);
    failEveryChunkUntilFatal(hls);
    waitMs(1000);
    page.dispatchEvent(new Event("online"));
    expect(loadsOnFromS).toEqual([42]);
    expect(scene.linkNotes.at(-1)).toBe("asked again to load from 42.0 s, the browser back online");
    // Its wait was cleared: it does not ask a second time when the wait would have ended.
    waitMs(asksAgainAfterMs);
    expect(loadsOnFromS).toEqual([42]);
    // Online while nothing waits asks nothing.
    page.dispatchEvent(new Event("online"));
    expect(loadsOnFromS).toEqual([42]);
    hls.destroy();
  });

  it("is asked again after an exception hls.js caught in its own handling too, which said only that the browser gave no reason", () => {
    const { hls, loadsOnFromS, failures } = playbackLoadingOn(new EventTarget());
    hls.trigger(Events.ERROR, {
      type: ErrorTypes.OTHER_ERROR,
      details: ErrorDetails.INTERNAL_EXCEPTION,
      fatal: true,
      error: new TypeError("x is undefined"),
    });
    expect(failures).toEqual([]);
    waitMs(asksAgainAfterMs);
    expect(loadsOnFromS).toEqual([42]);
    hls.destroy();
  });
});

// A minute's outage, reproduced in headless Chrome with real hls.js (2026-10-29): the top quality
// was given up at 67.9 s and its decoding rebuilt, whose new media source put the video at 0, and
// when the link came back hls.js was asked to load from 0.0 s: the film started again from the start.
describe("loading on after its decoding was rebuilt", () => {
  it("asks hls.js to load on from where the video stood, never from the 0 a new media source puts it at", () => {
    const page = new EventTarget();
    const video = videoAt(67.9, { isPaused: false, readyState: 4 });
    const { hls, loadsOnFromS } = playbackLoadingOn(page, video);
    const attached: (() => void)[] = [];
    Object.defineProperty(hls, "once", {
      value: (_event: Events, then: () => void) => attached.push(then),
    });
    Object.defineProperty(hls, "recoverMediaError", {
      value: () => {
        video.currentTime = 0; // a new media source
      },
    });
    loadAgainFrom(hls, 67.9); // the quality given up as unplayable, its decoding built anew
    failEveryChunkUntilFatal(hls);
    page.dispatchEvent(new Event("online"));
    expect(loadsOnFromS).toEqual([67.9]);
    expect(scene.linkNotes.at(-1)).toBe("asked again to load from 67.9 s, the browser back online");
    // Attached, it loads from there; hls.js holds that start until it has loaded enough to start.
    for (const then of attached) {
      then();
    }
    Object.defineProperty(hls, "startPosition", { value: 67.9 });
    failEveryChunkUntilFatal(hls);
    waitMs(asksAgainAfterAtMostMs);
    expect(loadsOnFromS).toEqual([67.9, 67.9, 67.9]);
    hls.destroy();
  });
});

describe("a playback that cannot go on", () => {
  it("ends at the media's own fatal error alone, telling with its code what failed, for the core's log", () => {
    const { hls, loadsOnFromS, failures } = playbackLoadingOn(new EventTarget());
    hls.trigger(Events.ERROR, {
      type: ErrorTypes.MEDIA_ERROR,
      details: ErrorDetails.BUFFER_APPEND_ERROR,
      fatal: true,
      frag: pictureChunk(2, 9),
      error: new Error("append failed"),
    });
    expect(failures).toEqual([
      [
        "decode",
        "mediaError bufferAppendError (fatal): append failed; status none; carried WebRTC" +
          " connection 3 for https://core/c/2/9.m4s",
      ],
    ]);
    waitMs(asksAgainAfterAtMostMs);
    expect(loadsOnFromS).toEqual([]);
    hls.destroy();
  });

  it("ends at the core's word that its film cannot be read now, which asking again does not change, saying so", () => {
    const { hls, loadsOnFromS, failures } = playbackLoadingOn(new EventTarget());
    hls.trigger(Events.ERROR, {
      type: ErrorTypes.NETWORK_ERROR,
      details: ErrorDetails.FRAG_LOAD_ERROR,
      fatal: true,
      frag: pictureChunk(1, 4),
      response: { url: "", code: 503, text: "" },
      error: new Error("HTTP Error 503"),
    });
    expect(failures.map(([failure, whatFailed]) => [failure, whatFailed?.split(";")[1]])).toEqual([
      ["network", " status 503"],
    ]);
    waitMs(asksAgainAfterAtMostMs);
    expect(loadsOnFromS).toEqual([]);
    hls.destroy();
  });

  it("ends at the core's word that it no longer has the playback, never asking again (staging, 2026-11-12)", () => {
    const { hls, loadsOnFromS, failures } = playbackLoadingOn(new EventTarget());
    hls.trigger(Events.ERROR, {
      type: ErrorTypes.NETWORK_ERROR,
      details: ErrorDetails.FRAG_LOAD_ERROR,
      fatal: true,
      frag: pictureChunk(1, 457),
      response: { url: "", code: 404, text: "" },
      error: new Error("HTTP Error 404"),
    });
    expect(failures.map(([failure, whatFailed]) => [failure, whatFailed?.split(";")[1]])).toEqual([
      ["network", " status 404"],
    ]);
    waitMs(asksAgainAfterAtMostMs);
    expect(loadsOnFromS).toEqual([]);
    hls.destroy();
  });

  it("ends at the core's word that it failed on the film at this chunk each time, saying so, where it asked again every 2 s (toolkit.md item 12)", () => {
    const { hls, loadsOnFromS, failures } = playbackLoadingOn(new EventTarget());
    hls.trigger(Events.ERROR, {
      type: ErrorTypes.NETWORK_ERROR,
      details: ErrorDetails.FRAG_LOAD_ERROR,
      fatal: true,
      frag: pictureChunk(1, 349),
      response: { url: "", code: 422, text: "" },
      error: new Error("HTTP Error 422"),
    });
    expect(failures.map(([failure, whatFailed]) => [failure, whatFailed?.split(";")[1]])).toEqual([
      ["cannotMake", " status 422"],
    ]);
    waitMs(asksAgainAfterAtMostMs);
    expect(loadsOnFromS).toEqual([]);
    hls.destroy();
  });
});
