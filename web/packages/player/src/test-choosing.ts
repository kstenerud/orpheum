// SPDX-License-Identifier: MPL-2.0

/**
 * A chunked playback's bitrate controller (abr.ts) under test: a stand-in for hls.js, its chunks,
 * a clock the test moves with vitest's fake timers, and what the controller tells. Shared by the
 * controller's test files, each of which calls useScene with vitest's own, which production code may
 * not import (web/.dependency-cruiser.cjs).
 */

import type { afterEach, beforeEach, vi } from "vitest";

import {
  Choosing,
  lookAtChunkEveryMs,
  startingPlayedMs,
  startingSamples,
  type ChoosingHls,
  type FollowedChunk,
} from "./abr.ts";
import type { RememberedLink } from "./link-estimate.ts";

/** Bram's ladder on staging: four qualities, then its own picture, its peak 36 Mb/s. */
export const levels = [
  { bitrate: 696_000, averageBitrate: 496_000 },
  { bitrate: 1_896_000, averageBitrate: 1_296_000 },
  { bitrate: 4_596_000, averageBitrate: 3_096_000 },
  { bitrate: 36_208_176, averageBitrate: 5_117_192 },
];

/** A video as choosing reads it. */
export type ChosenVideo = Pick<
  HTMLMediaElement,
  "buffered" | "currentTime" | "paused" | "readyState"
>;

/** A video at positionS, paused or not, at readyState, its picture and sound loaded there unless not. */
export function videoAt(
  positionS: number,
  state: { readonly isPaused: boolean; readonly readyState: number; readonly isLoaded?: boolean },
): ChosenVideo {
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
export class FakeHls implements ChoosingHls {
  levels: ChoosingHls["levels"] = levels;
  currentLevel = -1;
  loadLevel = -1;
  readonly minAutoLevel = 0;
  readonly maxAutoLevel = 3;
  mainForwardBufferInfo = { len: 0 };
  hasEnoughToStart = true;
  startPosition = -1;
  media: ChosenVideo | null = null;

  /** Its video playing at positionS, everything loaded. */
  playingAt(positionS: number): void {
    this.media = videoAt(positionS, { isPaused: false, readyState: 4 });
  }

  /** Its video set aside, so its standing still decides nothing. */
  setVideoAside(): void {
    this.media = null;
  }
}

/** A chunk of picture at level, ten seconds long, asked for at 0, with nothing arrived yet. */
export function chunkAt(level: number): FollowedChunk & {
  stats: { loaded: number; total: number; loading: { start: number; first: number; end: number } };
} {
  return {
    url: `/c/${String(level)}.m4s`,
    level,
    duration: 10,
    stats: { loaded: 0, total: 0, aborted: false, loading: { start: 0, first: 0, end: 0 } },
  };
}

/** What every choosing of a test reads and tells, reset before each test (useScene). */
export interface Scene {
  nowMs: number;
  isVoiceLive: boolean;
  isInAGroup: boolean;
  /** Whether the browser says it is offline. */
  isOffline: boolean;
  /** The chunks the browser says it took from its cache, by URL; any other it says nothing of. */
  cachedUrls: Set<string>;
  /** The steps the choosing noted. */
  steps: string[];
  /** The link's samples in a line, each time the choosing told them. */
  samplesNoted: string[];
  /** The steadiness level, each time the choosing told it. */
  steadinessNoted: number[];
  /** The link's spread, each time the choosing told it. */
  spreadsNoted: number[];
  /** The notes on the link for the play report, in order. */
  linkNotes: string[];
}

export const scene: Scene = {
  nowMs: 0,
  isVoiceLive: false,
  isInAGroup: false,
  isOffline: false,
  cachedUrls: new Set<string>(),
  steps: [],
  samplesNoted: [],
  steadinessNoted: [],
  spreadsNoted: [],
  linkNotes: [],
};

/** The fake timers useScene was given, which waitMs moves. */
let timers: typeof vi | undefined;

/** Resets the scene and fakes the timers before each test, and gives the real ones back after. */
export function useScene(vitest: {
  readonly beforeEach: typeof beforeEach;
  readonly afterEach: typeof afterEach;
  readonly vi: typeof vi;
}): void {
  timers = vitest.vi;
  vitest.beforeEach(() => {
    vitest.vi.useFakeTimers();
    scene.nowMs = 0;
    scene.isVoiceLive = false;
    scene.isInAGroup = false;
    scene.isOffline = false;
    scene.cachedUrls = new Set<string>();
    scene.steps = [];
    scene.samplesNoted = [];
    scene.steadinessNoted = [];
    scene.spreadsNoted = [];
    scene.linkNotes = [];
  });
  vitest.afterEach(() => {
    vitest.vi.useRealTimers();
  });
}

/** A test's choosing, and what it was told. */
export interface Chosen {
  readonly hls: FakeHls;
  readonly choosing: Choosing<FollowedChunk>;
  /** Each chunk given up's measure, as hls.js's estimator is fed it: ms and bytes. */
  readonly fedToHls: [number, number][];
  readonly givenUp: FollowedChunk[];
  readonly loadsAgain: number[];
  /** Where each load again was from, in seconds of the film. */
  readonly loadsAgainFromS: number[];
  readonly unplayable: { readonly qualityKbps: number; readonly positionMs: number }[];
}

/**
 * A playback's choosing, offered startAtBps and, where the test names them, the share of the link a
 * quality may take and the steadiness level it starts at, or that and the link its device remembers;
 * the link estimated by the chunks it measures.
 */
export function choosingFrom(
  startAtBps: number,
  shareForQuality?: number,
  started?:
    | number
    | {
        readonly steadiness?: number;
        readonly rememberedLink?: RememberedLink;
        /** Whether its first quality is chosen from the remembered link (D381). */
        readonly shouldStartByRememberedLink?: boolean;
      },
): Chosen {
  const {
    steadiness: startSteadiness,
    rememberedLink,
    shouldStartByRememberedLink,
  } = typeof started === "number"
    ? { steadiness: started, rememberedLink: undefined, shouldStartByRememberedLink: undefined }
    : (started ?? {});
  const hls = new FakeHls();
  const fedToHls: [number, number][] = [];
  const givenUp: FollowedChunk[] = [];
  const loadsAgain: number[] = [];
  const loadsAgainFromS: number[] = [];
  const unplayable: { readonly qualityKbps: number; readonly positionMs: number }[] = [];
  const choosing = new Choosing<FollowedChunk>(
    hls,
    {
      startAtBps,
      shouldStartByRememberedLink,
      shareForQuality,
      startSteadiness,
      rememberedLink,
      monotonicMs: () => scene.nowMs,
      isVoiceLive: () => scene.isVoiceLive,
      isInAGroup: () => scene.isInAGroup,
      isOffline: () => scene.isOffline,
      fromCache: (url) => (scene.cachedUrls.has(url) ? true : undefined),
      onUnplayable: (found) => unplayable.push(found),
      noteStep: (step) => scene.steps.push(step),
      noteSamples: (samples) => scene.samplesNoted.push(samples),
      noteSteadiness: (level) => scene.steadinessNoted.push(level),
      noteLinkSpread: (spread) => scene.spreadsNoted.push(spread),
      noteLink: (note) => scene.linkNotes.push(note),
      carrierOf: (url) => `WebRTC connection 2 for ${url}`,
    },
    {
      giveUp: (chunk) => givenUp.push(chunk),
      measureGivenUp: (durationMs, bytes) => fedToHls.push([durationMs, bytes]),
      loadAgain: (positionS) => {
        loadsAgain.push(scene.nowMs);
        loadsAgainFromS.push(positionS);
      },
    },
  );
  return { hls, choosing, fedToHls, givenUp, loadsAgain, loadsAgainFromS, unplayable };
}

/** Loads a chunk at level over the link: bytes in loadMs, its first after 50 ms. */
export function load(
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

/** Loads a chunk at level whose bytes arrive at mbps for seconds (one unless said), its first after 50 ms. */
export function loadAt(
  choosing: Choosing<FollowedChunk>,
  level: number,
  arrival: { readonly mbps: number; readonly seconds?: number },
): boolean {
  const seconds = arrival.seconds ?? 1;
  return load(choosing, level, {
    bytes: arrival.mbps * 125_000 * seconds,
    loadMs: 50 + seconds * 1000,
  });
}

/** Waits ms on the clock, the controller's looks with it. */
export function waitMs(ms: number): void {
  if (timers === undefined) {
    throw new Error("waitMs needs useScene first");
  }
  scene.nowMs += ms;
  timers.advanceTimersByTime(ms);
}

/** Plays the video on for ms from where it stands, the controller looking every half second. */
export function playFor(hls: FakeHls, ms: number): void {
  for (let playedMs = 0; playedMs < ms; playedMs += lookAtChunkEveryMs) {
    hls.playingAt((hls.media?.currentTime ?? 0) + lookAtChunkEveryMs / 1000);
    waitMs(lookAtChunkEveryMs);
  }
}

/**
 * Takes a playback past its starting (D352 §1): startingSamples chunks of level at mbps each, or a
 * chunk at each of several speeds, then startingPlayedMs played; its video then set aside, so its
 * standing still decides nothing after.
 */
export function pastStarting(chosen: Chosen, level: number, ...mbps: number[]): void {
  const speeds =
    mbps.length === 1 ? Array.from({ length: startingSamples }, () => mbps[0] ?? 0) : mbps;
  for (const speed of speeds) {
    loadAt(chosen.choosing, level, { mbps: speed });
  }
  playFor(chosen.hls, startingPlayedMs + lookAtChunkEveryMs);
  if (chosen.choosing.isStarting()) {
    throw new Error("the playback is still starting");
  }
  chosen.hls.setVideoAside();
}
