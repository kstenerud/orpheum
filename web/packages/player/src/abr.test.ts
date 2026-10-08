// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Hls, { Events, Fragment, PlaylistLevelType } from "hls.js";

import {
  Choosing,
  lookAtChunkEveryMs,
  lookForStepUpEveryMs,
  orpheumAbr,
  type ChoosingHls,
  type FollowedChunk,
} from "./abr.ts";
import { holdBeforeStepUpMs, stepUpBufferMs } from "./step-up.ts";

/** Bram's ladder on staging: four qualities, then its own picture, its peak 36 Mb/s. */
const levels = [
  { bitrate: 696_000, averageBitrate: 496_000 },
  { bitrate: 1_896_000, averageBitrate: 1_296_000 },
  { bitrate: 4_596_000, averageBitrate: 3_096_000 },
  { bitrate: 36_208_176, averageBitrate: 5_117_192 },
];

/** A stand-in for hls.js as choosing reads it. */
class FakeHls implements ChoosingHls {
  readonly levels = levels;
  currentLevel = -1;
  loadLevel = -1;
  readonly minAutoLevel = 0;
  readonly maxAutoLevel = 3;
  mainForwardBufferInfo = { len: 0 };
  media: Pick<HTMLMediaElement, "currentTime" | "paused"> | null = null;
}

/** A chunk of picture at level, ten seconds long, asked for at 0, with nothing arrived yet. */
function chunkAt(level: number): FollowedChunk & {
  stats: { loaded: number; total: number; loading: { first: number; end: number } };
} {
  return {
    level,
    duration: 10,
    stats: { loaded: 0, total: 0, aborted: false, loading: { start: 0, first: 0, end: 0 } },
  };
}

let nowMs = 0;
let isVoiceLive = false;

/** A playback's choosing, offered startAtBps, the link estimated at what the test sets. */
function choosingFrom(startAtBps: number): {
  hls: FakeHls;
  choosing: Choosing<FollowedChunk>;
  link: { estimateBps: number };
  givenUp: FollowedChunk[];
  loadsAgain: number[];
  unplayable: { readonly qualityKbps: number; readonly positionMs: number }[];
} {
  const hls = new FakeHls();
  const link = { estimateBps: 0 };
  const givenUp: FollowedChunk[] = [];
  const loadsAgain: number[] = [];
  const unplayable: { readonly qualityKbps: number; readonly positionMs: number }[] = [];
  const choosing = new Choosing<FollowedChunk>(
    hls,
    {
      startAtBps,
      monotonicMs: () => nowMs,
      isVoiceLive: () => isVoiceLive,
      onUnplayable: (found) => unplayable.push(found),
    },
    {
      estimateBps: () => link.estimateBps,
      giveUp: (chunk) => givenUp.push(chunk),
      loadAgain: () => loadsAgain.push(nowMs),
    },
  );
  return { hls, choosing, link, givenUp, loadsAgain, unplayable };
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
    chosen.hls.media = { currentTime: positionS, paused: false };
    chosen.choosing.firstLevel();
    vi.advanceTimersByTime(lookAtChunkEveryMs); // it stands still from here
    return chosen;
  }

  it("is given up when the video stands still while its chunks arrive, and loaded again lower", () => {
    const { choosing, loadsAgain, unplayable } = standingAt(420.5);
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([{ qualityKbps: 36_208, positionMs: 420_500 }]);
    expect(loadsAgain).toEqual([6000]);
    expect(choosing.nextLevel()).toBe(2);
  });

  it("is never chosen again, by the step-up or by hls.js's forcing", () => {
    const { hls, choosing } = standingAt(420.5);
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    hls.media = { currentTime: 430, paused: false };
    hls.currentLevel = 2;
    hls.mainForwardBufferInfo = { len: 60 };
    nowMs = 1_000_000;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(2);
    choosing.setLevel(3);
    expect(choosing.forcedLevel()).toBe(-1);
  });

  it("is not given up while chunks are slow to come, while paused, or while the video plays", () => {
    const slow = standingAt(420.5);
    load(slow.choosing, 3, { bytes: 6_000_000, loadMs: 1000 }); // one chunk: the link may be why
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(slow.unplayable).toEqual([]);

    nowMs = 0;
    const paused = standingAt(420.5);
    paused.hls.media = { currentTime: 420.5, paused: true };
    load(paused.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(paused.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(paused.unplayable).toEqual([]);

    nowMs = 0;
    const playing = standingAt(420.5);
    load(playing.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(playing.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    playing.hls.media = { currentTime: 423.5, paused: false };
    nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(playing.unplayable).toEqual([]);
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
    const abr = orpheumAbr({ startAtBps: 0, monotonicMs: () => nowMs, isVoiceLive: () => false });
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
