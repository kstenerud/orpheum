// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { LoaderContext } from "hls.js";

import {
  catchUpSeekEvent,
  playerMovedItselfEvent,
  waitAheadAtMostMs,
  type CannotKeepUp,
} from "@orpheum/sdk";

import {
  beyondTheLink,
  chunkLoadPolicy,
  ceilingHeader,
  chunkedHlsConfig,
  paceHeader,
  playerFoundUnplayableEvent,
  sendUnplayable,
  settledAtBps,
  unplayableOf,
} from "./chunked.ts";
import { markSeeks, soughtHeader } from "./hls.ts";

/** Three qualities, as hls.js reads them from the playlist. */
const levels = [
  { bitrate: 1_242_000, averageBitrate: 892_000, height: 432 },
  { bitrate: 1_992_000, averageBitrate: 1_392_000, height: 480 },
  { bitrate: 4_692_000, averageBitrate: 3_192_000, height: 720 },
];

describe("whether a stall is beyond the link (D247 §8)", () => {
  /** Playing level, on a link measured at linkKbps (undefined: not yet measured). */
  function playingAt(level: number, linkKbps: number | undefined): CannotKeepUp | undefined {
    return beyondTheLink({ levels, currentLevel: level }, linkKbps);
  }

  it("is, at the lowest quality on a link measured too slow for it, with both numbers", () => {
    expect(playingAt(0, 1000)).toEqual({ linkKbps: 1000, lowestKbps: 892 });
  });

  it("is, at the lowest quality on a link not yet measured", () => {
    expect(playingAt(0, undefined)).toEqual({ linkKbps: undefined, lowestKbps: 892 });
  });

  it("is not, at the lowest quality on a link measured well above it: something else stalled it", () => {
    // staging, 2026-10-06: a 98 Mbit/s link was told it could not keep up with 892 kbit/s
    expect(playingAt(0, 98_348)).toBeUndefined();
    expect(playingAt(0, 1338)).toBeUndefined(); // 1.5 times the lowest
  });

  it("is not, above the lowest quality, where a lower one answers it", () => {
    expect(playingAt(1, 500)).toBeUndefined();
  });
});

describe("loading a chunk the core is making (D250 §5)", () => {
  it("asks again after a 425 for longer than a player catching up waits ahead", () => {
    const retry = chunkLoadPolicy.default.errorRetry;
    expect(retry?.shouldRetry?.(retry, 0, false, { url: "00001.m4s", code: 425 }, false)).toBe(
      true,
    );
    expect((retry?.maxNumRetry ?? 0) * (retry?.retryDelayMs ?? 0)).toBeGreaterThan(
      waitAheadAtMostMs,
    );
    // A 429, the core having no room to make it, is not asked for again (D250 §6).
    expect(retry?.shouldRetry?.(retry, 0, false, { url: "00001.m4s", code: 429 }, true)).toBe(
      false,
    );
  });
});

describe("where a playback's stepping has settled (D260 §3)", () => {
  it("is the quality it last loaded a chunk of, and nothing before it loads one", () => {
    expect(settledAtBps({ levels, loadLevel: -1 })).toBeUndefined();
    expect(settledAtBps({ levels, loadLevel: 1 })).toBe(1_992_000);
  });
});

describe("how far a chunked playback loads ahead (D260 §3)", () => {
  it("is hls.js's own unless told, and at most what it is told", () => {
    expect(chunkedHlsConfig({ startPositionS: 40 })).not.toHaveProperty("maxBufferLength");
    expect(chunkedHlsConfig({ startPositionS: 40, loadsAheadAtMostS: 10 })).toMatchObject({
      startPosition: 40,
      maxBufferLength: 10,
      maxMaxBufferLength: 10,
    });
  });
});

describe("asking the server to pace a download (D262 §2)", () => {
  it("asks the server to pace each download at the pace given, and nothing without one", () => {
    const headers: [string, string][] = [];
    const xhr = {
      setRequestHeader: (name: string, value: string) => {
        headers.push([name, value]);
      },
      addEventListener: () => undefined,
    } as unknown as XMLHttpRequest;
    const loaderContext = {} as unknown as LoaderContext;
    let pace: number | undefined = 1863;
    const config = chunkedHlsConfig({ startPositionS: 0, paceKbps: () => pace });
    void config.xhrSetup?.(xhr, "/c/q1200k480p/00001.m4s", loaderContext);
    pace = undefined;
    void config.xhrSetup?.(xhr, "/c/q1200k480p/00002.m4s", loaderContext);
    expect(headers).toEqual([[paceHeader, "1863"]]);
  });
});

describe("saying a seek took the player where it loads (D284 §2)", () => {
  it("says so on each request while the video loads where its person's or group's seek took it", () => {
    const headers: [string, string][] = [];
    const xhr = {
      setRequestHeader: (name: string, value: string) => {
        headers.push([name, value]);
      },
      addEventListener: () => undefined,
    } as unknown as XMLHttpRequest;
    const loaderContext = {} as unknown as LoaderContext;
    const video = new EventTarget();
    const seeks = markSeeks(video);
    const config = chunkedHlsConfig({ startPositionS: 0, isSought: seeks.isSought });
    const load = (): void => {
      void config.xhrSetup?.(xhr, "/c/q1200k480p/00001.m4s", loaderContext);
    };
    load(); // playing on: nothing said
    video.dispatchEvent(new Event("seeking"));
    load();
    video.dispatchEvent(new Event("canplay"));
    load(); // it can play there
    video.dispatchEvent(new Event(catchUpSeekEvent));
    video.dispatchEvent(new Event("seeking"));
    load(); // a catch-up, no jump
    video.dispatchEvent(new Event("playing"));
    video.dispatchEvent(new Event(playerMovedItselfEvent));
    video.dispatchEvent(new Event("seeking"));
    load(); // hls.js's own step over a gap, no jump
    seeks.stop();
    expect(headers).toEqual([[soughtHeader, "true"]]);
  });
});

describe("a quality given up as unplayable, as its event tells it (D266)", () => {
  it("carries where, and the video as it was before its decoding was built anew", () => {
    const detail = {
      qualityKbps: 25_991,
      positionMs: 382_111,
      readyState: 1,
      isPaused: true,
      videoBuffered: "378.59-501.83",
      audioBuffered: "387.95-508.10",
      lastError: "",
    };
    expect(unplayableOf(new CustomEvent(playerFoundUnplayableEvent, { detail }))).toEqual(detail);
    expect(
      unplayableOf(new CustomEvent(playerFoundUnplayableEvent, { detail: { qualityKbps: 1 } })),
    ).toBeUndefined();
  });

  it("is sent with the video's readyState and whether it stood paused (D266 §1)", () => {
    const sent: Event[] = [];
    const video = {
      readyState: 1,
      paused: true,
      error: null,
      dispatchEvent: (event: Event) => sent.push(event) > 0,
    };
    sendUnplayable(video)({ qualityKbps: 25_991, positionMs: 382_111, isFilmsOwnPicture: true });
    expect(sent.map(unplayableOf)).toEqual([
      {
        qualityKbps: 25_991,
        positionMs: 382_111,
        isFilmsOwnPicture: true,
        readyState: 1,
        isPaused: true,
      },
    ]);
  });
});

describe("the server's ceiling on what the player loads (D286 §5)", () => {
  it("is read from each chunk's answer, and none where the answer names none", () => {
    const told: (number | undefined)[] = [];
    const config = chunkedHlsConfig({
      startPositionS: 0,
      onCeiling: (kbps) => {
        told.push(kbps);
      },
    });
    const answer = (status: number, header: string | null): void => {
      let onLoad = (): void => undefined;
      const xhr = {
        status,
        setRequestHeader: () => undefined,
        addEventListener: (_type: string, listener: () => void) => {
          onLoad = listener;
        },
        getResponseHeader: (name: string) => (name === ceilingHeader ? header : null),
      } as unknown as XMLHttpRequest;
      void config.xhrSetup?.(xhr, "/c/q1200k480p/00001.m4s", {} as unknown as LoaderContext);
      onLoad();
    };
    answer(200, "1296");
    answer(200, null);
    answer(425, "700"); // not made yet: says nothing of the ceiling
    answer(200, "lots");
    expect(told).toEqual([1296, undefined, undefined]);
  });
});
