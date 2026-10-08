// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  microphoneConstraints,
  playbackOf,
  resumeAtTaps,
  routeOf,
} from "./together-voice-browser.ts";

describe("the microphone voice asks the browser for", () => {
  it("levels it without cancelling echo, which on Android would make a call of it", () => {
    expect(microphoneConstraints({ shouldCancelEcho: false, shouldLevel: true })).toEqual({
      audio: { echoCancellation: false, noiseSuppression: true, autoGainControl: true },
      video: false,
    });
  });

  it("cancels echo apart from levelling, as the tuning says", () => {
    expect(microphoneConstraints({ shouldCancelEcho: true, shouldLevel: false })).toEqual({
      audio: { echoCancellation: true, noiseSuppression: false, autoGainControl: false },
      video: false,
    });
  });
});

describe("a partner's voice as it plays", () => {
  it("plays through Web Audio at its gain while that runs, and through its element otherwise", () => {
    expect(routeOf("webAudio", 2)).toEqual({
      isElementMuted: true,
      elementVolume: 1,
      webAudioGain: 2,
    });
    // iOS, as the microphone opened: Web Audio stopped, the element plays, never both.
    expect(routeOf("elementInterrupted", 2)).toEqual({
      isElementMuted: false,
      elementVolume: 1,
      webAudioGain: 0,
    });
    expect(routeOf("elementSuspended", 0.5).elementVolume).toBe(0.5);
  });

  it("says how it plays from Web Audio's state", () => {
    expect(playbackOf({ state: "running" })).toBe("webAudio");
    expect(playbackOf({ state: "suspended" })).toBe("elementSuspended");
    expect(playbackOf(undefined)).toBe("element");
  });
});

describe("Web Audio stopped by the browser", () => {
  it("is resumed at the person's next tap anywhere, until it runs", () => {
    const changes: (() => void)[] = [];
    const context = {
      state: "interrupted",
      resumed: 0,
      resume: async (): Promise<void> => {
        context.resumed += 1;
        await Promise.resolve();
      },
      addEventListener: (_type: "statechange", listener: () => void) => {
        changes.push(listener);
      },
    };
    const taps = new Set<() => void>();
    resumeAtTaps(context, {
      addEventListener: (_type, listener) => {
        taps.add(listener);
      },
      removeEventListener: (_type, listener) => {
        taps.delete(listener);
      },
    });
    for (const tap of taps) {
      tap();
    }
    expect(context.resumed).toBe(1);
    context.state = "running";
    for (const change of changes) {
      change();
    }
    expect(taps.size).toBe(0);
  });
});
