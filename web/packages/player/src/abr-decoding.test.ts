// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Hls, { ErrorDetails, ErrorTypes, Events } from "hls.js";

import { orpheumAbr } from "./abr-controller.ts";
import { failOrLoadOn } from "./chunked.ts";
import { levels, scene, useScene, videoAt } from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

// A TCL TV's browser (TV Bro) said it played HEVC and failed to decode the film's own HEVC picture at
// every rebuild, each playback failing though H.264 qualities were below it (staging, 2026-11-06).
describe("a decoding error a rebuild did not mend (D266; the owner's TCL TV, 2026-11-06)", () => {
  function playingTheTop(): {
    readonly hls: Hls;
    readonly trigger: () => void;
    readonly failures: string[];
    readonly unplayable: unknown[];
    readonly rebuilds: string[];
  } {
    const unplayable: unknown[] = [];
    const abr = orpheumAbr({
      startAtBps: 0,
      monotonicMs: () => scene.nowMs,
      isVoiceLive: () => false,
      fromCache: () => undefined,
      onUnplayable: (quality) => unplayable.push(quality),
    });
    const hls = new Hls({ abrController: abr.controller });
    const video = Object.assign(
      new EventTarget(),
      videoAt(7.2, { isPaused: false, readyState: 1 }),
    );
    Object.defineProperty(hls, "media", { value: video });
    Object.defineProperty(hls, "loadLevel", { value: 3, writable: true });
    Object.defineProperty(hls, "levels", { value: levels });
    Object.defineProperty(hls, "minAutoLevel", { value: 0 });
    Object.defineProperty(hls, "startLoad", { value: () => undefined });
    const rebuilds: string[] = [];
    Object.defineProperty(hls, "recoverMediaError", {
      value: () => rebuilds.push(`at ${String(hls.loadLevel)}`),
    });
    const failures: string[] = [];
    failOrLoadOn(hls, video as unknown as HTMLVideoElement, {
      abr,
      onFailed: (failure) => failures.push(failure),
    });
    const trigger = (): void => {
      hls.trigger(Events.ERROR, {
        type: ErrorTypes.MEDIA_ERROR,
        details: ErrorDetails.MEDIA_SOURCE_REQUIRES_RESET,
        fatal: true,
        error: new Error(
          "Failed to execute 'appendBuffer' on 'SourceBuffer': The HTMLMediaElement.error attribute is not null.",
        ),
      });
    };
    return { hls, trigger, failures, unplayable, rebuilds };
  }

  it("gives its quality up as one the device cannot play, the playback going on below it", () => {
    const { hls, trigger, failures, unplayable, rebuilds } = playingTheTop();
    trigger(); // recovered once, at the same quality
    expect([failures, unplayable, rebuilds]).toEqual([[], [], ["at 3"]]);
    trigger(); // the rebuild did not mend it
    expect(failures).toEqual([]);
    expect(unplayable).toEqual([
      { qualityKbps: 36_208, positionMs: 7200, isFilmsOwnPicture: false },
    ]);
    expect(rebuilds).toHaveLength(2);
    hls.destroy();
  });

  it("fails the playback at the lowest quality, nothing below it to play", () => {
    const { hls, trigger, failures, unplayable } = playingTheTop();
    hls.loadLevel = 0;
    trigger();
    trigger();
    expect([failures, unplayable]).toEqual([["decode"], []]);
    hls.destroy();
  });
});
