// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { playerMovedItselfEvent } from "@orpheum/sdk";
import Hls from "hls.js";

import {
  hlsMethod,
  playNative,
  preparedHlsConfig,
  startWithAudio,
  tellOwnMoves,
  type AudioStart,
  type NativeTarget,
} from "./hls.ts";

describe("playing a prepared rendition", () => {
  it("uses hls.js wherever there is MSE, the element's own HLS where there is only that", () => {
    expect(hlsMethod({ hasMse: true, canPlayNatively: true })).toBe("hls.js");
    expect(hlsMethod({ hasMse: false, canPlayNatively: true })).toBe("native");
    expect(hlsMethod({ hasMse: false, canPlayNatively: false })).toBe("none");
  });

  it("starts a rendition still being prepared at its start, not at its newest segment", () => {
    // hls.js's own default, -1, starts an open (EVENT) playlist near its live edge.
    expect(new Hls().config.startPosition).toBe(-1);
    expect(new Hls(preparedHlsConfig).config.startPosition).toBe(0);
  });

  it("hands the playlist to the element itself, and lets go of it", () => {
    const removed: string[] = [];
    let source = "";
    let loads = 0;
    const target: NativeTarget = {
      setAttribute: (name, value) => {
        source = name === "src" ? value : source;
      },
      removeAttribute: (name) => removed.push(name),
      load: () => {
        loads++;
      },
    };
    const playback = playNative(target, "/api/v1/bytes/media/hls/t/master.m3u8");
    expect(source).toBe("/api/v1/bytes/media/hls/t/master.m3u8");
    playback.destroy();
    expect(removed).toEqual(["src"]);
    expect(loads).toBe(1);
  });

  it("tells the video when hls.js moves it itself, and not for any other error", () => {
    let listener: ((event: unknown, data: { details: string }) => void) | undefined;
    tellOwnMoves(
      {
        on: (event, heard) => {
          expect(event).toBe(Hls.Events.ERROR);
          listener = heard as typeof listener;
        },
      },
      {
        dispatchEvent: (event) => {
          told.push(event.type);
          return true;
        },
      },
    );
    const told: string[] = [];
    listener?.(undefined, { details: Hls.ErrorDetails.BUFFER_SEEK_OVER_HOLE });
    listener?.(undefined, { details: Hls.ErrorDetails.FRAG_LOAD_ERROR });
    listener?.(undefined, { details: Hls.ErrorDetails.BUFFER_NUDGE_ON_STALL });
    expect(told).toEqual([playerMovedItselfEvent, playerMovedItselfEvent]);
  });

  it("starts with the chosen mix once the master playlist is read, and leaves the default alone", () => {
    const listeners: (() => void)[] = [];
    const hls: AudioStart = {
      on: (event, listener) => {
        expect(event).toBe(Hls.Events.MANIFEST_PARSED);
        listeners.push(listener);
      },
      audioTrack: -1,
    };
    startWithAudio(hls, 1);
    expect(hls.audioTrack).toBe(-1);
    for (const listener of listeners) {
      listener();
    }
    expect(hls.audioTrack).toBe(1);
    const untouched: AudioStart = { on: () => expect.fail("nothing to wait for"), audioTrack: -1 };
    startWithAudio(untouched, 0);
    expect(untouched.audioTrack).toBe(-1);
  });
});
