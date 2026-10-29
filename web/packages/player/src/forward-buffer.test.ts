// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import Hls from "hls.js";

import {
  aheadTargetOf,
  deviceFactsOf,
  keepForwardBuffer,
  loadsAheadAtLeastS,
  loadsAheadAtTopS,
  loadsAheadBelowTopS,
  videoLimitBytes,
  videoLimitKindOf,
  type AheadHls,
  type DeviceFacts,
} from "./forward-buffer.ts";

/** A quality at averageMbps, its peak half again. */
function levelAt(averageMbps: number): { bitrate: number; averageBitrate: number } {
  return { bitrate: averageMbps * 1.5e6, averageBitrate: averageMbps * 1e6 };
}

const desktopChrome =
  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36";
const fireTvSilk =
  "Mozilla/5.0 (Linux; Android 9; AFTSSS Build/PS7633) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.3.1 like Chrome/138.0.7204.0 Safari/537.36";
const chromecast =
  "Mozilla/5.0 (X11; Linux armv7l) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36 CrKey/1.56.500000";
const macFirefox =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:144.0) Gecko/20100101 Firefox/144.0";
const macSafari =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.6 Safari/605.1.15";
const iphoneChrome =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/141.0 Mobile/15E148 Safari/604.1";
const androidChrome =
  "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36";

describe("how far ahead a quality loads (forward-buffer-2026-10-29.md P1)", () => {
  const desktop = videoLimitBytes.ordinary;

  it("loads 30 s ahead below the top, the step-up's 20 s and a chunk, and 60 s at the top", () => {
    expect(aheadTargetOf({ level: levelAt(0.5), isTop: false, limitBytes: desktop })).toEqual({
      aheadS: loadsAheadBelowTopS,
      by: "below top",
    });
    expect(aheadTargetOf({ level: levelAt(3), isTop: true, limitBytes: desktop })).toEqual({
      aheadS: loadsAheadAtTopS,
      by: "top",
    });
    expect([loadsAheadBelowTopS, loadsAheadAtTopS]).toEqual([30, 60]);
  });

  it("never loads a low quality further for its bytes, as hls.js's 60 MB did to ten minutes of 360p", () => {
    expect(aheadTargetOf({ level: levelAt(0.4), isTop: false, limitBytes: desktop }).aheadS).toBe(
      30,
    );
  });

  it("is lowered by its group's runway, hls.js's own cut and the page's own, whichever is least", () => {
    const top = { level: levelAt(3), isTop: true, limitBytes: desktop };
    expect(aheadTargetOf(top, { runwayS: 10 })).toEqual({ aheadS: 10, by: "runway" });
    expect(aheadTargetOf(top, { cutS: 35, runwayS: 40 })).toEqual({ aheadS: 35, by: "cut" });
    expect(aheadTargetOf(top, { atMostS: 10, cutS: 35 })).toEqual({ aheadS: 10, by: "page" });
    // Below the floor too: a runway is the group's, not a guess at the device's memory.
    expect(aheadTargetOf(top, { runwayS: 5 }).aheadS).toBe(5);
  });
});

describe("the bytes cap: a third of the browser's memory for a page's picture (forward-buffer-2026-10-29.md P2)", () => {
  it("holds the report's table: 46 s and 21 s at 9 and 20 Mb/s on a desktop, Firefox's 31 s, never below 20 s", () => {
    const at = (mbps: number, limitBytes: number): { aheadS: number; by: string } => {
      const target = aheadTargetOf({ level: levelAt(mbps), isTop: true, limitBytes });
      return { aheadS: Math.round(target.aheadS), by: target.by };
    };
    expect(at(9, videoLimitBytes.ordinary)).toEqual({ aheadS: 47, by: "bytes" });
    expect(at(20, videoLimitBytes.ordinary)).toEqual({ aheadS: 21, by: "bytes" });
    expect(at(9, videoLimitBytes.firefox)).toEqual({ aheadS: 31, by: "bytes" });
    expect(at(20, videoLimitBytes.firefox)).toEqual({ aheadS: loadsAheadAtLeastS, by: "floor" });
    expect(at(9, videoLimitBytes.lowMemory)).toEqual({ aheadS: loadsAheadAtLeastS, by: "floor" });
    expect(loadsAheadAtLeastS).toBe(20);
  });

  it("caps below the top too, never above its own 30 s", () => {
    const target = aheadTargetOf({
      level: levelAt(9),
      isTop: false,
      limitBytes: videoLimitBytes.cast,
    });
    expect(target).toEqual({ aheadS: loadsAheadAtLeastS, by: "floor" });
  });

  it("assumes each device's limit from its browser and its memory", () => {
    const kindOf = (userAgent: string, deviceMemoryGb?: number): string =>
      videoLimitKindOf({ userAgent, deviceMemoryGb });
    expect(kindOf(desktopChrome, 8)).toBe("ordinary");
    expect(kindOf(androidChrome, 4)).toBe("ordinary");
    expect(kindOf(fireTvSilk, 1)).toBe("lowMemory");
    expect(kindOf(androidChrome, 0.5)).toBe("lowMemory");
    expect(kindOf(chromecast)).toBe("cast");
    expect(kindOf(macFirefox)).toBe("firefox");
    expect(kindOf(macSafari)).toBe("webkit");
    expect(kindOf(iphoneChrome)).toBe("webkit");
    expect(videoLimitBytes).toEqual({
      ordinary: 157_286_400,
      lowMemory: 31_457_280,
      cast: 31_457_280,
      firefox: 104_857_600,
      webkit: 100_000_000,
    });
  });

  it("reads deviceMemory where the browser says it, and nothing where it does not", () => {
    const silk = { userAgent: fireTvSilk, deviceMemory: 1 };
    expect(deviceFactsOf(silk)).toEqual({
      userAgent: fireTvSilk,
      deviceMemoryGb: 1,
    });
    expect(deviceFactsOf({ userAgent: macFirefox })).toEqual({
      userAgent: macFirefox,
      deviceMemoryGb: undefined,
    });
  });
});

/** A stand-in for hls.js as the forward buffer reads it, its listeners called as hls.js would. */
class FakeHls implements AheadHls {
  readonly config = { maxBufferLength: 30, maxMaxBufferLength: 30 };
  readonly levels = [levelAt(0.5), levelAt(1.3), levelAt(9)];
  media: AheadHls["media"] = null;
  readonly #listeners = new Map<string, ((event: unknown, data: unknown) => void)[]>();

  on(event: string, listener: (event: unknown, data: unknown) => void): void {
    this.#listeners.set(event, [...(this.#listeners.get(event) ?? []), listener]);
  }

  off(event: string, listener: (event: unknown, data: unknown) => void): void {
    this.#listeners.set(
      event,
      (this.#listeners.get(event) ?? []).filter((each) => each !== listener),
    );
  }

  trigger(event: string, data: unknown): void {
    for (const listener of this.#listeners.get(event) ?? []) {
      listener(event, data);
    }
  }
}

/** A playback's forward buffer on a FakeHls whose top is level 2, on device, and its notes. */
function keptOn(device: DeviceFacts): {
  readonly hls: FakeHls;
  readonly notes: string[];
  readonly stop: () => void;
} {
  const hls = new FakeHls();
  const notes: string[] = [];
  const { stop } = keepForwardBuffer(hls as unknown as Parameters<typeof keepForwardBuffer>[0], {
    device,
    isTop: (level) => level >= 2,
    note: (note) => notes.push(note),
  });
  return { hls, notes, stop };
}

describe("keeping hls.js to the target as its quality changes (P1, P2, P6)", () => {
  it("sets the target as each quality begins loading, before hls.js reads it, and notes each change of it once, none for a quality whose target is the same", () => {
    const { hls, notes, stop } = keptOn({ userAgent: desktopChrome, deviceMemoryGb: 8 });
    expect(notes).toEqual([
      `device memory 8 GB, video buffer 157.3 MB assumed (ordinary); ${desktopChrome}`,
    ]);
    hls.trigger(Hls.Events.LEVEL_SWITCHING, { level: 0 });
    expect(hls.config).toEqual({ maxBufferLength: 30, maxMaxBufferLength: 30 });
    hls.trigger(Hls.Events.LEVEL_SWITCHING, { level: 1 });
    hls.trigger(Hls.Events.LEVEL_SWITCHING, { level: 2 });
    const atTopS = (videoLimitBytes.ordinary * 8) / 3 / 9e6;
    expect(hls.config).toEqual({ maxBufferLength: atTopS, maxMaxBufferLength: atTopS });
    expect(notes.slice(1)).toEqual([
      "loads ahead 30.0 s (1.9 MB), by below top: level 0 of 0-2, 750/500 kb/s; cap 52.4 MB",
      "loads ahead 46.6 s (52.4 MB), by bytes: level 2 of 0-2, 13500/9000 kb/s; cap 52.4 MB",
    ]);
    stop();
    hls.trigger(Hls.Events.LEVEL_SWITCHING, { level: 0 });
    expect(hls.config.maxBufferLength).toBe(atTopS);
  });

  it("holds a 1 GB Fire TV's top to 20 s ahead, as Chromium's low-end limit would need", () => {
    const { hls, notes } = keptOn({ userAgent: fireTvSilk, deviceMemoryGb: 1 });
    hls.trigger(Hls.Events.LEVEL_SWITCHING, { level: 2 });
    expect(hls.config.maxMaxBufferLength).toBe(20);
    expect(notes.at(-1)).toMatch(/^loads ahead 20\.0 s .*, by floor: level 2 /u);
  });

  it("notes a buffer run full with what was loaded, and keeps hls.js's cut at every quality after", () => {
    const { hls, notes } = keptOn({ userAgent: desktopChrome, deviceMemoryGb: 8 });
    hls.trigger(Hls.Events.LEVEL_SWITCHING, { level: 2 });
    hls.media = {
      currentTime: 100,
      buffered: { length: 1, start: () => 70, end: () => 140 } as unknown as TimeRanges,
    };
    hls.config.maxMaxBufferLength = 25; // hls.js's reduceMaxBufferLength, before its error is told
    hls.trigger(Hls.Events.ERROR, {
      details: Hls.ErrorDetails.BUFFER_FULL_ERROR,
      sourceBufferName: "video",
    });
    expect(notes.at(-2)).toBe(
      "buffer full on video: 30.0 s behind, 40.0 s ahead (about 78.8 MB at 9000 kb/s); cut to 25.0 s",
    );
    expect(notes.at(-1)).toMatch(/^loads ahead 25\.0 s .*, by cut: level 2 /u);
    hls.trigger(Hls.Events.LEVEL_SWITCHING, { level: 1 });
    expect(hls.config).toEqual({ maxBufferLength: 25, maxMaxBufferLength: 25 });
  });
});
