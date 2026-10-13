// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  browserVoiceDevices,
  microphoneConstraints,
  playbackOf,
  playWaitingAtTaps,
  resumeAtTaps,
  routeOf,
  TapWaiting,
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

describe("one tap for the film's sound and the voices (the owner, 2026-10-13)", () => {
  it("at every click readies voice to be heard and plays the voices refused, until stopped", () => {
    let isWaiting = true;
    let readied = 0;
    let played = 0;
    const clicks = new Set<() => void>();
    const stop = playWaitingAtTaps(
      {
        prepareToHear: () => {
          readied += 1;
        },
        isWaiting: () => isWaiting,
        playNow: () => {
          played += 1;
        },
      },
      {
        addEventListener: (_type, listener, isCapture) => {
          expect(isCapture).toBe(true);
          clicks.add(listener);
        },
        removeEventListener: (_type, listener) => {
          clicks.delete(listener);
        },
      },
    );
    const clickAll = (): void => {
      for (const click of clicks) {
        click();
      }
    };
    clickAll(); // the film's "Tap for sound", before anyone has spoken
    expect([readied, played]).toEqual([1, 1]);
    isWaiting = false;
    clickAll();
    expect([readied, played]).toEqual([2, 1]);
    stop();
    clickAll();
    expect(readied).toBe(2);
  });
});

describe("an iPhone's audio session as voice is first heard (D281)", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("is told it plays, so the silent switch mutes neither voice nor the film", () => {
    const session = { type: "auto" };
    class FakeContext {
      readonly state = "running";
      addEventListener(): void {
        // a state change is not looked at here
      }
      resume(): Promise<void> {
        return Promise.resolve();
      }
    }
    vi.stubGlobal("navigator", { audioSession: session });
    // eslint-disable-next-line @typescript-eslint/naming-convention -- the browser's own name
    vi.stubGlobal("window", { AudioContext: FakeContext });
    vi.stubGlobal("AudioContext", FakeContext);
    vi.stubGlobal("document", {
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    });
    browserVoiceDevices().prepareToHear();
    expect(session.type).toBe("playback");
  });
});

describe("TapWaiting", () => {
  const context = (state: AudioContextState) => ({
    state,
    resume: () => Promise.resolve(),
  });

  it("waits for a tap once Web Audio that ran stops, as a phone call stops it, and not after it closes", () => {
    let audio = context("suspended");
    const waiting = new TapWaiting(() => audio);
    waiting.changed();
    expect(waiting.isWaiting()).toBe(false);
    audio = context("running");
    waiting.changed();
    expect(waiting.isWaiting()).toBe(false);
    audio = context("interrupted");
    waiting.changed();
    expect(waiting.isWaiting()).toBe(true);
    audio = context("running");
    waiting.changed();
    expect(waiting.isWaiting()).toBe(false);
    audio = context("closed");
    waiting.changed();
    expect(waiting.isWaiting()).toBe(false);
  });
});
