// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Hls, { ErrorDetails, ErrorTypes, Events, Fragment, PlaylistLevelType } from "hls.js";

import {
  Choosing,
  isBeingMade,
  isRecovered,
  lookAtChunkEveryMs,
  lookForStepUpEveryMs,
  orpheumAbr,
  type ChoosingHls,
  type FollowedChunk,
  loadAgainFrom,
} from "./abr.ts";
import { commonStartForMs } from "./abr-choice.ts";
import { failOnFatal } from "./hls.ts";
import { holdBeforeStepUpMs, stepUpBufferMs } from "./step-up.ts";

/** Bram's ladder on staging: four qualities, then its own picture, its peak 36 Mb/s. */
const levels = [
  { bitrate: 696_000, averageBitrate: 496_000 },
  { bitrate: 1_896_000, averageBitrate: 1_296_000 },
  { bitrate: 4_596_000, averageBitrate: 3_096_000 },
  { bitrate: 36_208_176, averageBitrate: 5_117_192 },
];

/** A video at positionS, paused or not, at readyState, its picture and sound loaded there unless not. */
function videoAt(
  positionS: number,
  state: { readonly isPaused: boolean; readonly readyState: number; readonly isLoaded?: boolean },
): Pick<HTMLMediaElement, "buffered" | "currentTime" | "paused" | "readyState"> {
  const ranges = (state.isLoaded ?? true) ? [[positionS - 5, positionS + 15]] : [];
  return {
    currentTime: positionS,
    paused: state.isPaused,
    readyState: state.readyState,
    buffered: {
      length: ranges.length,
      start: (index) => ranges[index]?.[0] ?? 0,
      end: (index) => ranges[index]?.[1] ?? 0,
    },
  };
}

/** A stand-in for hls.js as choosing reads it. */
class FakeHls implements ChoosingHls {
  levels: ChoosingHls["levels"] = levels;
  currentLevel = -1;
  loadLevel = -1;
  readonly minAutoLevel = 0;
  readonly maxAutoLevel = 3;
  mainForwardBufferInfo = { len: 0 };
  media: Pick<HTMLMediaElement, "buffered" | "currentTime" | "paused" | "readyState"> | null = null;
}

/** A chunk of picture at level, ten seconds long, asked for at 0, with nothing arrived yet. */
function chunkAt(level: number): FollowedChunk & {
  stats: { loaded: number; total: number; loading: { first: number; end: number } };
} {
  return {
    url: `/c/${String(level)}.m4s`,
    level,
    duration: 10,
    stats: { loaded: 0, total: 0, aborted: false, loading: { start: 0, first: 0, end: 0 } },
  };
}

let nowMs = 0;
let isVoiceLive = false;
let isInAGroup = false;
/** The chunks the browser says it took from its cache, by URL; any other it says nothing of. */
let cachedUrls = new Set<string>();
/** The steps the choosing noted, by each test's choosing. */
let steps: string[] = [];

/** A playback's choosing, offered startAtBps, the link estimated at what the test sets. */
function choosingFrom(startAtBps: number): {
  hls: FakeHls;
  choosing: Choosing<FollowedChunk>;
  link: { estimateBps: number };
  givenUp: FollowedChunk[];
  loadsAgain: number[];
  /** Where each load again was from, in seconds of the film. */
  loadsAgainFromS: number[];
  unplayable: { readonly qualityKbps: number; readonly positionMs: number }[];
} {
  const hls = new FakeHls();
  const link = { estimateBps: 0 };
  const givenUp: FollowedChunk[] = [];
  const loadsAgain: number[] = [];
  const loadsAgainFromS: number[] = [];
  const unplayable: { readonly qualityKbps: number; readonly positionMs: number }[] = [];
  const choosing = new Choosing<FollowedChunk>(
    hls,
    {
      startAtBps,
      monotonicMs: () => nowMs,
      isVoiceLive: () => isVoiceLive,
      isInAGroup: () => isInAGroup,
      fromCache: (url) => (cachedUrls.has(url) ? true : undefined),
      onUnplayable: (found) => unplayable.push(found),
      noteStep: (step) => steps.push(step),
    },
    {
      estimateBps: () => link.estimateBps,
      giveUp: (chunk) => givenUp.push(chunk),
      // As hls.js's estimator takes a sample: here it becomes the estimate outright.
      measureGivenUp: (durationMs, bytes) => {
        link.estimateBps = (bytes * 8 * 1000) / durationMs;
      },
      loadAgain: (positionS) => {
        loadsAgain.push(nowMs);
        loadsAgainFromS.push(positionS);
      },
    },
  );
  return { hls, choosing, link, givenUp, loadsAgain, loadsAgainFromS, unplayable };
}

/** Loads a chunk at level over the link: bytes in loadMs, its first after 50 ms. */
function load(
  choosing: Choosing<FollowedChunk>,
  level: number,
  arrival: { readonly bytes: number; readonly loadMs: number },
): boolean {
  const chunk = chunkAt(level);
  choosing.loading(chunk);
  chunk.stats.loaded = arrival.bytes;
  chunk.stats.loading = { start: 0, first: Math.min(50, arrival.loadMs), end: arrival.loadMs };
  choosing.loaded(chunk);
  return choosing.measures(chunk);
}

beforeEach(() => {
  vi.useFakeTimers();
  nowMs = 0;
  isVoiceLive = false;
  isInAGroup = false;
  cachedUrls = new Set<string>();
  steps = [];
});

afterEach(() => {
  vi.useRealTimers();
});

describe("choosing a chunked playback's quality (D265)", () => {
  it("starts at the highest quality within the speed it was offered, and stays until it measures", () => {
    const { choosing } = choosingFrom(2_000_000);
    expect(choosing.firstLevel()).toBe(1);
    expect(choosing.nextLevel()).toBe(1);
    expect(choosing.linkKbps()).toBeUndefined();
  });

  it("steps down to what the measured link carries, and stays down until the step-up", () => {
    const { choosing, link } = choosingFrom(40_000_000);
    link.estimateBps = 5_000_000;
    expect(load(choosing, 3, { bytes: 6_400_000, loadMs: 10_000 })).toBe(true);
    expect(choosing.linkKbps()).toBe(5000);
    expect(choosing.nextLevel()).toBe(2); // 70% of 5 Mb/s carries 720p, not the picture
    link.estimateBps = 50_000_000;
    expect(choosing.nextLevel()).toBe(2); // the step-up's to raise, not the next look's
  });

  it("steps up one quality once the link has held well above it, and holds again after", () => {
    const { hls, choosing, link } = choosingFrom(1_000_000);
    hls.mainForwardBufferInfo = { len: stepUpBufferMs / 1000 };
    hls.currentLevel = 0;
    load(choosing, 0, { bytes: 1_000_000, loadMs: 1000 });
    link.estimateBps = 10_000_000;
    nowMs = holdBeforeStepUpMs - lookForStepUpEveryMs;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(0); // not held long enough yet
    nowMs = holdBeforeStepUpMs;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(1);
    hls.currentLevel = 1;
    choosing.switched();
    nowMs += lookForStepUpEveryMs;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(1); // held again at the new quality
  });

  it("gives up a slow chunk on its way, playing or not, for the quality its speed carries", () => {
    const { choosing, givenUp } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    chunk.stats.loaded = 750_000; // 1 Mb/s over six seconds
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(givenUp).toEqual([chunk]);
    expect(choosing.nextLevel()).toBe(0);
    expect(choosing.firstLevel()).toBe(0); // a first chunk given up starts again from there
  });

  it("judges a chunk the core is making from when it is made, and measures nothing by its wait", () => {
    const { choosing, givenUp } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    // Being made for six seconds, the core answering 425 each time it is asked again.
    for (nowMs = 1000; nowMs <= 6000; nowMs += 1000) {
      choosing.beingMade(chunk);
      vi.advanceTimersByTime(lookAtChunkEveryMs);
    }
    expect(givenUp).toEqual([]);
    expect(choosing.linkKbps()).toBeUndefined();
    expect(choosing.nextLevel()).toBe(3); // the film's own picture, not the lowest
    // Made, it arrives slowly, and is judged as any chunk is from there.
    chunk.stats.loaded = 750_000;
    chunk.stats.loading = { start: 6000, first: 6100, end: 0 }; // hls.js times each ask afresh
    nowMs = 12_000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(givenUp).toEqual([chunk]);
  });

  it("knows the core's 425 for a chunk from hls.js's other load errors", () => {
    expect(
      isBeingMade({
        details: ErrorDetails.FRAG_LOAD_ERROR,
        response: { url: "/c/3.m4s", code: 425 },
      }),
    ).toBe(true);
    expect(
      isBeingMade({
        details: ErrorDetails.FRAG_LOAD_ERROR,
        response: { url: "/c/3.m4s", code: 404 },
      }),
    ).toBe(false);
    expect(
      isBeingMade({
        details: ErrorDetails.FRAG_LOAD_TIMEOUT,
        response: { url: "/c/3.m4s", code: 425 },
      }),
    ).toBe(false);
  });

  it("notes each change of its ceiling with why: the link, a chunk given up, a step up", () => {
    const { hls, choosing, link } = choosingFrom(40_000_000);
    link.estimateBps = 5_000_000;
    load(choosing, 3, { bytes: 6_400_000, loadMs: 10_000 });
    choosing.nextLevel();
    expect(steps).toEqual(["ceiling 3 to 2: link 5000 kb/s, 0.0 s ahead"]);
    const chunk = chunkAt(2);
    choosing.loading(chunk);
    chunk.stats.loaded = 100_000;
    link.estimateBps = 0;
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(steps[1]).toMatch(/^ceiling 2 to 0: gave up a chunk of 2: 100000 B in 6000 ms$/u);
    hls.mainForwardBufferInfo = { len: stepUpBufferMs / 1000 };
    hls.currentLevel = 0;
    link.estimateBps = 50_000_000;
    nowMs += holdBeforeStepUpMs;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(steps[2]).toBe("ceiling 0 to 1: stepped up");
  });

  it("waits for a chunk that loaded before it was looked at", () => {
    const { choosing, givenUp } = choosingFrom(40_000_000);
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    choosing.loaded(chunk);
    nowMs = 8000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(givenUp).toEqual([]);
  });

  it("does not count a chunk from the browser's cache as a measure of the link", () => {
    const { choosing } = choosingFrom(40_000_000);
    expect(load(choosing, 3, { bytes: 6_400_000, loadMs: 4 })).toBe(false);
    expect(choosing.linkKbps()).toBeUndefined();
  });

  it("does not count a chunk the browser says came from its cache, however long it took", () => {
    const { choosing } = choosingFrom(40_000_000);
    cachedUrls.add("/c/3.m4s");
    expect(load(choosing, 3, { bytes: 6_400_000, loadMs: 2000 })).toBe(false);
    expect(load(choosing, 2, { bytes: 3_200_000, loadMs: 2000 })).toBe(true);
  });

  it("obeys a quality hls.js forces until a chunk of it loads, and its echo of a choice changes nothing", () => {
    const { choosing, link } = choosingFrom(40_000_000);
    link.estimateBps = 20_000_000;
    choosing.setLevel(choosing.firstLevel()); // the echo
    expect(choosing.forcedLevel()).toBe(-1);
    choosing.setLevel(1); // a switch down after a load error
    expect(choosing.forcedLevel()).toBe(1);
    expect(choosing.nextLevel()).toBe(1);
    load(choosing, 1, { bytes: 1_600_000, loadMs: 1000 });
    expect(choosing.forcedLevel()).toBe(-1);
    expect(choosing.nextLevel()).toBe(1); // and the step down holds
  });
});

describe("a quality this device cannot play (D266)", () => {
  /** A video asked to play at positionS, its picture loading at level 3 on a fast link. */
  function standingAt(positionS: number): ReturnType<typeof choosingFrom> {
    const chosen = choosingFrom(40_000_000);
    chosen.link.estimateBps = 30_000_000;
    chosen.hls.loadLevel = 3;
    chosen.hls.media = videoAt(positionS, { isPaused: false, readyState: 1 });
    chosen.choosing.firstLevel();
    vi.advanceTimersByTime(lookAtChunkEveryMs); // it stands still from here
    return chosen;
  }

  it("is given up when the video stands still while its chunks arrive, and loaded again lower", () => {
    const { choosing, loadsAgain, loadsAgainFromS, unplayable } = standingAt(420.5);
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([
      { qualityKbps: 36_208, positionMs: 420_500, isFilmsOwnPicture: false },
    ]);
    expect(loadsAgain).toEqual([6000]);
    expect(loadsAgainFromS).toEqual([420.5]); // from where it stood
    expect(choosing.nextLevel()).toBe(2);
  });

  it("is never chosen again, by the step-up or by hls.js's forcing", () => {
    const { hls, choosing } = standingAt(420.5);
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    hls.media = videoAt(430, { isPaused: false, readyState: 4 });
    hls.currentLevel = 2;
    hls.mainForwardBufferInfo = { len: 60 };
    nowMs = 1_000_000;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(2);
    choosing.setLevel(3);
    expect(choosing.forcedLevel()).toBe(-1);
  });

  it("is given up when the video, playing, stands still with its buffer full and no chunk coming", () => {
    const { hls, unplayable } = standingAt(0.05);
    hls.mainForwardBufferInfo = { len: 30 }; // an older phone's Chrome, its decoder unable to keep up
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([{ qualityKbps: 36_208, positionMs: 50, isFilmsOwnPicture: false }]);
  });

  it("is given up at a hole hls.js cannot step over, and loaded again below it; never the lowest", () => {
    const { hls, choosing, loadsAgain, unplayable } = standingAt(5.6);
    expect(choosing.canGiveUpAtAHole()).toBe(true);
    choosing.giveUpAtAHole(5.6);
    expect(unplayable).toEqual([
      { qualityKbps: 36_208, positionMs: 5600, isFilmsOwnPicture: false },
    ]);
    expect(loadsAgain).toHaveLength(1);
    hls.loadLevel = hls.minAutoLevel;
    expect(choosing.canGiveUpAtAHole()).toBe(false);
  });

  it("is given up at once when the browser holds nothing of two of its chunks", () => {
    const { choosing, unplayable } = standingAt(416.9);
    choosing.notHeld(3);
    choosing.notHeld(3);
    nowMs = 1000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([
      { qualityKbps: 36_208, positionMs: 416_900, isFilmsOwnPicture: false },
    ]);
  });

  it("is given up when the browser holds nothing of two of its chunks, though nothing is loaded where the video stands", () => {
    const { hls, choosing, unplayable } = standingAt(445.3);
    hls.media = videoAt(445.3, { isPaused: true, readyState: 1, isLoaded: false }); // its picture never held
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    choosing.notHeld(3);
    choosing.notHeld(3);
    nowMs = 1000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([
      { qualityKbps: 36_208, positionMs: 445_300, isFilmsOwnPicture: false },
    ]);
  });

  it("counts its time standing still only from when what it plays is loaded where it stands", () => {
    const { hls, choosing, unplayable } = standingAt(420.5);
    hls.media = videoAt(420.5, { isPaused: false, readyState: 1, isLoaded: false });
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    hls.media = videoAt(420.5, { isPaused: false, readyState: 1 }); // the sound came
    nowMs = 7000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([]);
    nowMs = 13_000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([
      { qualityKbps: 36_208, positionMs: 420_500, isFilmsOwnPicture: false },
    ]);
  });

  it("says when it is the film's own picture, which the server then offers the device no more", () => {
    const { hls, choosing, unplayable } = standingAt(416.9);
    hls.levels = levels.map((level, index) =>
      index === 3 ? { ...level, uri: "picture/stream.m3u8" } : level,
    );
    choosing.notHeld(3);
    choosing.notHeld(3);
    nowMs = 1000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([
      { qualityKbps: 36_208, positionMs: 416_900, isFilmsOwnPicture: true },
    ]);
  });

  it("ends hls.js's forcing of it, which would otherwise keep loading it", () => {
    const { hls, choosing, unplayable } = standingAt(416.9);
    choosing.setLevel(2); // hls.js forces a switch down after a load error
    hls.loadLevel = 2;
    choosing.notHeld(2);
    choosing.notHeld(2);
    nowMs = 1000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toHaveLength(1);
    expect(choosing.forcedLevel()).toBe(-1);
    expect(choosing.nextLevel()).toBeLessThan(2);
  });

  it("is given up on an autoplaying video, paused until it can play, that never can", () => {
    const { hls, choosing, unplayable } = standingAt(303.98);
    hls.media = videoAt(303.98, { isPaused: true, readyState: 1 });
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([
      { qualityKbps: 36_208, positionMs: 303_980, isFilmsOwnPicture: false },
    ]);
  });

  it("is not given up while chunks are slow to come, while paused, while its sound is not there, or while the video plays", () => {
    const slow = standingAt(420.5);
    load(slow.choosing, 3, { bytes: 6_000_000, loadMs: 1000 }); // one chunk: the link may be why
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(slow.unplayable).toEqual([]);

    nowMs = 0;
    const paused = standingAt(420.5);
    paused.hls.media = videoAt(420.5, { isPaused: true, readyState: 4 }); // its data there
    load(paused.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(paused.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(paused.unplayable).toEqual([]);

    nowMs = 0;
    const waitingForSound = standingAt(420.5);
    waitingForSound.hls.media = videoAt(420.5, { isPaused: false, readyState: 1, isLoaded: false }); // a seek's sound not yet come
    load(waitingForSound.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(waitingForSound.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(waitingForSound.unplayable).toEqual([]);

    nowMs = 0;
    const playing = standingAt(420.5);
    load(playing.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(playing.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    playing.hls.media = videoAt(423.5, { isPaused: false, readyState: 4 });
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(playing.unplayable).toEqual([]);
  });
});

describe("the server's ceiling (D286 §5)", () => {
  it("keeps every choice at or under it, the step-up too, until it is lifted", () => {
    const { hls, choosing, link } = choosingFrom(40_000_000);
    link.estimateBps = 30_000_000;
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    expect(choosing.nextLevel()).toBe(3);
    choosing.obeyServer(1400); // 480p's 1296 with its sound
    expect(choosing.nextLevel()).toBe(1);
    hls.currentLevel = 1;
    hls.mainForwardBufferInfo = { len: 60 };
    nowMs = 1_000_000;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(1);
    choosing.obeyServer(100); // under every quality: the lowest
    expect(choosing.nextLevel()).toBe(0);
    choosing.obeyServer(undefined);
    expect(choosing.nextLevel()).toBe(3);
  });
});

describe("choosing while voice is live (D262 §2, D265 §6, §7)", () => {
  it("paces each download a quarter above its quality, and only while voice is live", () => {
    const { hls, choosing } = choosingFrom(40_000_000);
    hls.loadLevel = 2;
    expect(choosing.paceKbps()).toBeUndefined();
    isVoiceLive = true;
    expect(choosing.paceKbps()).toBe(5745);
  });

  it("caps the film at half the link measured before voice, which its paced chunks do not lower", () => {
    const { hls, choosing, link } = choosingFrom(40_000_000);
    link.estimateBps = 3_000_000;
    load(choosing, 1, { bytes: 1_600_000, loadMs: 1000 });
    isVoiceLive = true;
    expect(choosing.nextLevel()).toBe(1); // 1.5 Mb/s carries 480p
    hls.loadLevel = 1;
    // Paced at 2.4 Mb/s, a chunk arrives at its pace: it measures the pace, not the link.
    expect(load(choosing, 1, { bytes: 1_185_000, loadMs: 4000 })).toBe(false);
    expect(choosing.nextLevel()).toBe(1);
  });
});

describe("a player in a Watch Together group keeping the film's own picture (D286 §2)", () => {
  /** choosingFrom, its top quality named as the film's own picture, as the master lists it. */
  function withOwnPicture(): ReturnType<typeof choosingFrom> {
    const chosen = choosingFrom(40_000_000);
    const named = levels.map((level, index) =>
      index === levels.length - 1 ? { ...level, uri: "https://s/c/t/picture/stream.m3u8" } : level,
    );
    Object.defineProperty(chosen.hls, "levels", { value: named });
    return chosen;
  }

  it("keeps it while its link carries the picture's average, though voice would halve its share", () => {
    const { choosing, link } = withOwnPicture();
    link.estimateBps = 8_800_000;
    load(choosing, 3, { bytes: 4_400_000, loadMs: 4000 });
    isVoiceLive = true;
    expect(choosing.nextLevel()).toBe(2); // alone, voice leaves it half: 720p
    isInAGroup = true;
    expect(choosing.nextLevel()).toBe(3); // in a group: the film's own picture
    link.estimateBps = 4_000_000;
    load(choosing, 3, { bytes: 2_000_000, loadMs: 4000 });
    expect(choosing.nextLevel()).toBeLessThan(3); // a link too slow for it steps down
  });
  it("steps down from it once a chunk of it is given up, though no chunk measured the slower link", () => {
    const { choosing, link, givenUp } = withOwnPicture();
    link.estimateBps = 8_800_000;
    load(choosing, 3, { bytes: 4_400_000, loadMs: 4000 });
    isInAGroup = true;
    expect(choosing.nextLevel()).toBe(3);
    // The link slows: the picture's next chunk crawls, and is given up before it ever loads, so
    // nothing loaded measures the link. Kept on the old measure, the group's choice took the picture
    // again, which was given up again, every few seconds (the owner's group test, 2026-10-14).
    const crawling = chunkAt(3);
    choosing.loading(crawling);
    crawling.stats.loaded = 750_000; // 1 Mb/s over six seconds
    crawling.stats.loading = { start: 0, first: 50, end: 0 };
    nowMs += 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(givenUp).toEqual([crawling]);
    expect(choosing.nextLevel()).toBeLessThan(3);
  });
});

describe("a group's jump, which a device on an encoded quality starts on the common one (D286 §3)", () => {
  it("loads the common quality for its while after the jump, then chooses as it would", () => {
    const { hls, choosing, link } = choosingFrom(40_000_000);
    const tall = levels.map((level, index) => ({ ...level, height: [432, 540, 720, 1080][index] }));
    Object.defineProperty(hls, "levels", { value: tall });
    link.estimateBps = 1_000_000;
    load(choosing, 0, { bytes: 500_000, loadMs: 4000 });
    expect(choosing.nextLevel()).toBe(0); // a slow link: the lowest
    choosing.groupJumped();
    expect(choosing.nextLevel()).toBe(2); // the common 720p, which the others share
    nowMs += commonStartForMs;
    expect(choosing.nextLevel()).toBe(0); // then it settles as it would
  });
});

describe("the bitrate controller inside hls.js (D265 §1)", () => {
  /** A picture chunk hls.js would load: a second of 1 MB at level 0, its first byte at 50 ms. */
  function pictureChunk(loadMs: number): Fragment {
    const frag = new Fragment(PlaylistLevelType.MAIN, "");
    frag.sn = 1;
    frag.level = 0;
    frag.duration = 10;
    frag.stats.loaded = 1_000_000;
    frag.stats.loading = { start: 0, first: Math.min(50, loadMs), end: loadMs };
    frag.stats.parsing = { start: loadMs, end: loadMs };
    return frag;
  }

  it("is what hls.js asks for its levels, and counts a chunk in its estimate only when it measures the link", () => {
    const abr = orpheumAbr({
      startAtBps: 0,
      monotonicMs: () => nowMs,
      isVoiceLive: () => false,
      fromCache: () => undefined,
    });
    const hls = new Hls({ abrController: abr.controller, abrEwmaDefaultEstimate: 500_000 });
    const choosing = abr.choosing();
    expect(choosing).toBeDefined();
    expect(hls.nextAutoLevel).toBe(choosing?.nextLevel());
    hls.nextAutoLevel = 2; // a force, as hls.js's error handling makes
    expect(choosing?.forcedLevel()).toBe(2);
    const cached = pictureChunk(4);
    hls.trigger(Events.FRAG_BUFFERED, {
      stats: cached.stats,
      frag: cached,
      part: null,
      id: PlaylistLevelType.MAIN,
    });
    expect(hls.bandwidthEstimate).toBe(500_000);
    const overTheLink = pictureChunk(1000);
    hls.trigger(Events.FRAG_BUFFERED, {
      stats: overTheLink.stats,
      frag: overTheLink,
      part: null,
      id: PlaylistLevelType.MAIN,
    });
    expect(hls.bandwidthEstimate).toBeGreaterThan(7_000_000);
    expect(choosing?.linkKbps()).toBe(hls.bandwidthEstimate / 1000);
    hls.destroy();
  });
});

describe("a chunk on its way while the video plays (D265 §3, D323)", () => {
  it("is given up by choosing alone, never by hls.js's own check, which took a chunk being made for a stalled one", () => {
    const abr = orpheumAbr({
      startAtBps: 0,
      monotonicMs: () => nowMs,
      isVoiceLive: () => false,
      fromCache: () => undefined,
    });
    const hls = new Hls({ abrController: abr.controller });
    const video = { ...videoAt(10, { isPaused: false, readyState: 4 }), playbackRate: 1 };
    Object.defineProperty(hls, "media", { value: video });
    // hls.js reckons a quality's chunk by its peak, maxBitrate, and checks only qualities it may play.
    const reckoned = levels.map((level) => ({ ...level, maxBitrate: level.bitrate, details: {} }));
    Object.defineProperty(hls, "levels", { value: reckoned });
    Object.defineProperty(hls, "minAutoLevel", { value: 0 });
    Object.defineProperty(hls, "maxAutoLevel", { value: 3 });
    Object.defineProperty(hls, "autoLevelEnabled", { value: true });
    Object.defineProperty(hls, "mainForwardBufferInfo", { value: { len: 4 } });
    const chunk = new Fragment(PlaylistLevelType.MAIN, "");
    chunk.level = 3;
    chunk.duration = 10;
    chunk.stats.loading = { start: performance.now() - 30_000, first: 0, end: 0 };
    const controller: unknown = Reflect.get(hls, "abrController");
    Reflect.set(controller as object, "fragCurrent", chunk);
    const check: unknown = Reflect.get(controller as object, "_abandonRulesCheck");
    if (typeof check !== "function") {
      throw new Error("hls.js has no abandon check to call");
    }
    check.call(controller, undefined);
    expect(abr.choosing()?.forcedLevel()).toBe(-1);
    hls.destroy();
  });
});

describe("a hole hls.js cannot step over (D266; the owner's group test, 2026-10-13)", () => {
  it("is recovered from by giving the quality up, so the playback is not failed", () => {
    const abr = orpheumAbr({
      startAtBps: 0,
      monotonicMs: () => nowMs,
      isVoiceLive: () => false,
      fromCache: () => undefined,
    });
    const hls = new Hls({ abrController: abr.controller });
    const video = videoAt(5.6, { isPaused: false, readyState: 1 });
    Object.defineProperty(hls, "media", { value: video });
    Object.defineProperty(hls, "loadLevel", { value: 3, writable: true });
    Object.defineProperty(hls, "levels", { value: levels });
    const startedAt: number[] = [];
    Object.defineProperty(hls, "startLoad", { value: (at: number) => startedAt.push(at) });
    Object.defineProperty(hls, "recoverMediaError", { value: () => undefined });
    const failures: string[] = [];
    failOnFatal(hls, (failure) => failures.push(failure));
    const hole = {
      type: ErrorTypes.MEDIA_ERROR,
      details: ErrorDetails.BUFFER_SEEK_OVER_HOLE,
      fatal: true,
      error: new Error("Playhead still not moving after seeking over hole"),
    };
    hls.trigger(Events.ERROR, hole);
    expect(isRecovered(hole)).toBe(true);
    expect(failures).toEqual([]);
    vi.advanceTimersByTime(0);
    expect(startedAt).toEqual([5.6]);
    const other = { ...hole, details: ErrorDetails.BUFFER_STALLED_ERROR };
    hls.trigger(Events.ERROR, other);
    expect(failures).toHaveLength(1);
    hls.destroy();
  });
});

describe("loading again after the video's decoding is built anew (D266)", () => {
  it("starts loading from where the video stood once the new media source is attached (the owner's test, 2026-10-18)", () => {
    const calls: string[] = [];
    const onAttached: (() => void)[] = [];
    const hls = {
      once: (event: Events, listener: () => void) => {
        if (event === Events.MEDIA_ATTACHED) {
          onAttached.push(listener);
        }
      },
      recoverMediaError: () => calls.push("recoverMediaError"),
      startLoad: (positionS?: number) => calls.push(`startLoad ${String(positionS)}`),
    } as unknown as Parameters<typeof loadAgainFrom>[0];
    loadAgainFrom(hls, 0.04);
    // Not before: hls.js's loader is detached, and a chunk the detach aborted is still to mark it idle.
    expect(calls).toEqual(["recoverMediaError"]);
    for (const attached of onAttached) {
      attached();
    }
    expect(calls).toEqual(["recoverMediaError", "startLoad 0.04"]);
  });
});
