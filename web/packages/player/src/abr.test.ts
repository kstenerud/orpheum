// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Hls, { ErrorDetails, ErrorTypes, Events, Fragment, PlaylistLevelType } from "hls.js";

import { lookAtChunkEveryMs, lookForStepUpEveryMs } from "./abr.ts";
import { commonStartForMs } from "./abr-choice.ts";
import { isBeingMade, isRecovered, loadAgainFrom, orpheumAbr } from "./abr-controller.ts";
import type { QualitySwitched } from "@orpheum/sdk";

import { tellChunkedLoading } from "./chunked.ts";
import { failOnFatal, type LoadingHls } from "./hls.ts";
import { linkSamplesKept } from "./link-estimate.ts";
import { holdBeforeStepUpMsAt } from "./steadiness.ts";
import { stepUpBufferMs } from "./step-up.ts";
import {
  chunkAt,
  choosingFrom,
  levels,
  load,
  loadAt,
  pastStarting,
  scene,
  useScene,
  videoAt,
  waitMs,
} from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

describe("choosing a chunked playback's quality (D265)", () => {
  it("starts at the highest quality within the speed it was offered, and stays until it measures", () => {
    const { choosing } = choosingFrom(2_000_000);
    expect(choosing.firstLevel()).toBe(1);
    expect(choosing.nextLevel()).toBe(1);
    expect(choosing.linkKbps()).toBeUndefined();
  });

  it("steps down to what the measured link carries, and stays down until the step-up", () => {
    const chosen = choosingFrom(40_000_000);
    const { choosing } = chosen;
    pastStarting(chosen, 3, 5);
    expect(choosing.linkKbps()).toBeCloseTo(5000);
    expect(choosing.nextLevel()).toBe(2); // 70% of 5 Mb/s carries 720p, not the picture
    loadAt(choosing, 2, { mbps: 50 });
    expect(choosing.nextLevel()).toBe(2); // the step-up's to raise, not the next look's
  });

  it("steps down within the share of the link its offer says (the constants sweep, 2026-10-20)", () => {
    const { choosing } = choosingFrom(40_000_000, 0.5);
    expect(loadAt(choosing, 3, { mbps: 5 })).toBe(true);
    expect(choosing.nextLevel()).toBe(1); // half of 5 Mb/s carries 480p, where 70% carries 720p
  });

  it("gives up a slow chunk on its way, playing or not, for the quality its speed carries", () => {
    const { choosing, givenUp } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    chunk.stats.loaded = 750_000; // 1 Mb/s over six seconds
    chunk.stats.loading = { start: 0, first: 50, end: 0 };
    scene.nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(givenUp).toEqual([chunk]);
    expect(choosing.nextLevel()).toBe(0);
    expect(choosing.firstLevel()).toBe(0); // a first chunk given up starts again from there
  });

  it("judges a chunk the core is making from when it is made, and measures nothing by its wait", () => {
    const { choosing, givenUp } = choosingFrom(40_000_000);
    choosing.firstLevel();
    // The twin's length: half of it shorter than the answers' two seconds apart.
    const chunk = { ...chunkAt(3), duration: 2.3 };
    choosing.loading(chunk);
    // Being made for six seconds, the core answering 425 two seconds apart, each answer's 81-byte
    // body counted loaded with its first byte, as hls.js counts it (the owner's Firefox, 2026-10-20).
    chunk.stats.loaded = 81;
    chunk.stats.loading = { start: 0, first: 20, end: 0 };
    for (scene.nowMs = 0; scene.nowMs <= 6000; scene.nowMs += 500) {
      if (scene.nowMs % 2000 === 0) {
        choosing.beingMade(chunk);
      }
      vi.advanceTimersByTime(lookAtChunkEveryMs);
    }
    expect(givenUp).toEqual([]);
    expect(choosing.linkKbps()).toBeUndefined();
    expect(choosing.nextLevel()).toBe(3); // the film's own picture, not the lowest
    // Made, it arrives slowly, and is judged as any chunk is from there.
    chunk.stats.loaded = 750_081;
    chunk.stats.loading = { start: 6000, first: 6100, end: 0 }; // hls.js times each ask afresh
    scene.nowMs = 12_000;
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

  it("notes each change of its ceiling with why and what it rests on: starting's end, the link, a chunk given up, a step up (D352 §5)", () => {
    const chosen = choosingFrom(40_000_000);
    const { hls, choosing } = chosen;
    pastStarting(chosen, 3, 5);
    const sample = "3 625000B 1000ms fb50ms";
    expect(scene.steps).toEqual([
      "started, ceiling 3 to 2; confident 5000 kb/s (estimate 5000, error 0%, 4 samples), 0.0 s ahead," +
        ` steadiness 0, hold 30 s; samples ${[sample, sample, sample, sample].join(", ")}`,
    ]);
    // A link that falls: three slow chunks of five move the estimate, where one would not.
    for (let index = 0; index < 3; index += 1) {
      loadAt(choosing, 2, { mbps: 2, seconds: 2 });
    }
    choosing.nextLevel();
    expect(scene.steps[1]).toMatch(
      /^ceiling 2 to 1: link; confident \d+ kb\/s \(estimate 2\d\d\d, error \d+%, 5 samples\), 0\.0 s ahead, steadiness 0, hold 30 s$/u,
    );
    const chunk = chunkAt(1);
    choosing.loading(chunk);
    chunk.stats.loaded = 100_000;
    chunk.stats.loading = { start: scene.nowMs, first: scene.nowMs + 50, end: 0 };
    waitMs(6000);
    expect(scene.steps[2]).toMatch(
      /^ceiling 1 to 0: gave up a chunk of 1, 100000 B in 6000 ms, too few to measure; confident \d+ kb\/s \(estimate \d+, error \d+%, 5 samples\), 0\.0 s ahead, steadiness 0, hold 30 s$/u,
    );
    hls.mainForwardBufferInfo = { len: stepUpBufferMs / 1000 };
    hls.currentLevel = 0;
    for (let index = 0; index < linkSamplesKept; index += 1) {
      loadAt(choosing, 0, { mbps: 20 });
    }
    waitMs(holdBeforeStepUpMsAt[0] ?? 0);
    expect(scene.steps[3]).toMatch(
      /^ceiling 0 to 1: stepped up; confident \d+ kb\/s .*, 20\.0 s ahead, steadiness 0, hold 30 s$/u,
    );
  });

  it("tells each switch of quality why it was chosen: first, then a chunk given up, its bytes, its wait and the estimate after (staging, Nosferatu, 2026-10-28)", () => {
    const { hls, choosing } = choosingFrom(40_000_000);
    const listeners = new Map<string, (event: unknown, data: unknown) => void>();
    const told: QualitySwitched[] = [];
    tellChunkedLoading(
      {
        levels: hls.levels,
        bandwidthEstimate: 0,
        on: ((event: string, listener: (event: unknown, data: unknown) => void) => {
          listeners.set(event, listener);
        }) as LoadingHls["on"],
      },
      {
        dispatchEvent: (event) => {
          told.push((event as CustomEvent<QualitySwitched>).detail);
          return true;
        },
      },
      () => choosing,
    );
    const switchedTo = (level: number): void => {
      listeners.get(Hls.Events.LEVEL_SWITCHED)?.(undefined, { level });
    };
    switchedTo(choosing.firstLevel());
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    chunk.stats.loaded = 750_000; // arriving at 1 Mb/s from its first byte, 50 ms in
    chunk.stats.loading = { start: 0, first: 50, end: 0 };
    scene.nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    switchedTo(choosing.nextLevel());
    expect(told[0]).toEqual({ qualityKbps: 36_208.176, linkEstimateKbps: undefined, why: "first" });
    expect(told[1]?.fromQualityKbps).toBe(36_208.176);
    expect(told[1]?.qualityKbps).toBe(696);
    expect(told[1]?.why).toMatch(
      /^gave up a chunk of 3, 750000 B in 6000 ms; confident 1008 kb\/s \(estimate 1008, error none, 1 sample\), /u,
    );
    // The sample it took: its bytes over its first byte to the give-up.
    expect(told[1]?.linkSamples).toBe("samples 3 750000B 5950ms fb50ms");
  });

  it("tells the page its last samples, each its quality, bytes, time to arrive and wait for its first byte (D352 §5)", () => {
    const { choosing } = choosingFrom(40_000_000);
    for (let index = 1; index <= linkSamplesKept + 1; index += 1) {
      loadAt(choosing, 3, { mbps: index * 2 });
    }
    expect(scene.samplesNoted).toHaveLength(linkSamplesKept + 1);
    expect(scene.samplesNoted.at(-1)).toBe(
      "samples 3 500000B 1000ms fb50ms, 3 750000B 1000ms fb50ms, 3 1000000B 1000ms fb50ms," +
        " 3 1250000B 1000ms fb50ms, 3 1500000B 1000ms fb50ms",
    );
  });

  it("waits for a chunk that loaded before it was looked at", () => {
    const { choosing, givenUp } = choosingFrom(40_000_000);
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    choosing.loaded(chunk);
    scene.nowMs = 8000;
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
    scene.cachedUrls.add("/c/3.m4s");
    expect(load(choosing, 3, { bytes: 6_400_000, loadMs: 2000 })).toBe(false);
    expect(load(choosing, 2, { bytes: 3_200_000, loadMs: 2000 })).toBe(true);
  });

  it("obeys a quality hls.js forces until a chunk of it loads, and its echo of a choice changes nothing", () => {
    const { choosing } = choosingFrom(40_000_000);
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
    scene.nowMs = 6000;
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
    scene.nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    hls.media = videoAt(430, { isPaused: false, readyState: 4 });
    hls.currentLevel = 2;
    hls.mainForwardBufferInfo = { len: 60 };
    scene.nowMs = 1_000_000;
    vi.advanceTimersByTime(lookForStepUpEveryMs);
    expect(choosing.nextLevel()).toBe(2);
    choosing.setLevel(3);
    expect(choosing.forcedLevel()).toBe(-1);
  });

  it("is given up when the video, playing, stands still with its buffer full and no chunk coming", () => {
    const { hls, unplayable } = standingAt(0.05);
    hls.mainForwardBufferInfo = { len: 30 }; // an older phone's Chrome, its decoder unable to keep up
    scene.nowMs = 6000;
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
    scene.nowMs = 1000;
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
    scene.nowMs = 1000;
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
    scene.nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    hls.media = videoAt(420.5, { isPaused: false, readyState: 1 }); // the sound came
    scene.nowMs = 7000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([]);
    scene.nowMs = 13_000;
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
    scene.nowMs = 1000;
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
    scene.nowMs = 1000;
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
    scene.nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(unplayable).toEqual([
      { qualityKbps: 36_208, positionMs: 303_980, isFilmsOwnPicture: false },
    ]);
  });

  it("is not given up while chunks are slow to come, while paused, while its sound is not there, or while the video plays", () => {
    const slow = standingAt(420.5);
    load(slow.choosing, 3, { bytes: 6_000_000, loadMs: 1000 }); // one chunk: the link may be why
    scene.nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(slow.unplayable).toEqual([]);

    scene.nowMs = 0;
    const paused = standingAt(420.5);
    paused.hls.media = videoAt(420.5, { isPaused: true, readyState: 4 }); // its data there
    load(paused.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(paused.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    scene.nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(paused.unplayable).toEqual([]);

    scene.nowMs = 0;
    const waitingForSound = standingAt(420.5);
    waitingForSound.hls.media = videoAt(420.5, { isPaused: false, readyState: 1, isLoaded: false }); // a seek's sound not yet come
    load(waitingForSound.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(waitingForSound.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    scene.nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(waitingForSound.unplayable).toEqual([]);

    scene.nowMs = 0;
    const playing = standingAt(420.5);
    load(playing.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    load(playing.choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    playing.hls.media = videoAt(423.5, { isPaused: false, readyState: 4 });
    scene.nowMs = 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(playing.unplayable).toEqual([]);
  });
});

describe("the server's ceiling (D286 §5)", () => {
  it("keeps every choice at or under it, the step-up too, until it is lifted", () => {
    const { hls, choosing } = choosingFrom(40_000_000);
    load(choosing, 3, { bytes: 6_000_000, loadMs: 1000 });
    expect(choosing.nextLevel()).toBe(3);
    choosing.obeyServer(1400); // 480p's 1296 with its sound
    expect(choosing.nextLevel()).toBe(1);
    hls.currentLevel = 1;
    hls.mainForwardBufferInfo = { len: 60 };
    scene.nowMs = 1_000_000;
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
    scene.isVoiceLive = true;
    expect(choosing.paceKbps()).toBe(5745);
  });

  it("caps the film at half the link measured before voice, which its paced chunks do not lower", () => {
    const { hls, choosing } = choosingFrom(40_000_000);
    loadAt(choosing, 1, { mbps: 3 });
    scene.isVoiceLive = true;
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
    const { choosing } = withOwnPicture();
    loadAt(choosing, 3, { mbps: 8.8, seconds: 4 });
    scene.isVoiceLive = true;
    expect(choosing.nextLevel()).toBe(2); // alone, voice leaves it half: 720p
    scene.isInAGroup = true;
    expect(choosing.nextLevel()).toBe(3); // in a group: the film's own picture
    loadAt(choosing, 3, { mbps: 2.5, seconds: 4 }); // the estimate of both, 4.7 Mb/s, short of the picture's 5.1
    expect(choosing.nextLevel()).toBeLessThan(3); // a link too slow for it steps down
  });
  it("steps down from it once a chunk of it is given up, though no chunk measured the slower link", () => {
    const { choosing, givenUp } = withOwnPicture();
    loadAt(choosing, 3, { mbps: 8.8, seconds: 4 });
    scene.isInAGroup = true;
    expect(choosing.nextLevel()).toBe(3);
    // The link slows: the picture's next chunk crawls, and is given up before it ever loads, so
    // nothing loaded measures the link. Kept on the old measure, the group's choice took the picture
    // again, which was given up again, every few seconds (the owner's group test, 2026-10-14).
    const crawling = chunkAt(3);
    choosing.loading(crawling);
    crawling.stats.loaded = 750_000; // 1 Mb/s over six seconds
    crawling.stats.loading = { start: 0, first: 50, end: 0 };
    scene.nowMs += 6000;
    vi.advanceTimersByTime(lookAtChunkEveryMs);
    expect(givenUp).toEqual([crawling]);
    expect(choosing.nextLevel()).toBeLessThan(3);
  });
});

describe("a group's jump, which a device on an encoded quality starts on the common one (D286 §3)", () => {
  it("loads the common quality for its while after the jump, then chooses as it would", () => {
    const { hls, choosing } = choosingFrom(40_000_000);
    const tall = levels.map((level, index) => ({ ...level, height: [432, 540, 720, 1080][index] }));
    Object.defineProperty(hls, "levels", { value: tall });
    loadAt(choosing, 0, { mbps: 1, seconds: 4 });
    expect(choosing.nextLevel()).toBe(0); // a slow link: the lowest
    choosing.groupJumped();
    expect(choosing.nextLevel()).toBe(2); // the common 720p, which the others share
    scene.nowMs += commonStartForMs;
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
      monotonicMs: () => scene.nowMs,
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
    // hls.js's estimator is fed for its own readers; the controller reports its own, never from a
    // lone first sample (D352 §1, §2).
    expect(choosing?.linkKbps()).toBeUndefined();
    hls.destroy();
  });
});

describe("a chunk on its way while the video plays (D265 §3, D323)", () => {
  it("is given up by choosing alone, never by hls.js's own check, which took a chunk being made for a stalled one", () => {
    const abr = orpheumAbr({
      startAtBps: 0,
      monotonicMs: () => scene.nowMs,
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
      monotonicMs: () => scene.nowMs,
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

describe("a chunk whose film cannot be read now (the owner's test, 2026-10-21)", () => {
  it("fails the playback at once and stops its loading, though hls.js holds a sound chunk's 503 no failure", () => {
    let onError: ((event: unknown, data: unknown) => void) | undefined;
    let stops = 0;
    const hls = {
      on: (event: Events, listener: (event: unknown, data: unknown) => void) => {
        if (event === Events.ERROR) {
          onError = listener;
        }
      },
      stopLoad: () => (stops += 1),
    } as unknown as Parameters<typeof failOnFatal>[0];
    const failures: string[] = [];
    failOnFatal(hls, (failure) => failures.push(failure));
    const loadFailed = (code: number) => ({
      type: ErrorTypes.NETWORK_ERROR,
      details: ErrorDetails.FRAG_LOAD_ERROR,
      fatal: false,
      response: { code, text: "" },
    });
    onError?.(Events.ERROR, loadFailed(404));
    expect([failures, stops]).toEqual([[], 0]);
    onError?.(Events.ERROR, loadFailed(503));
    expect([failures, stops]).toEqual([["network"], 1]);
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
