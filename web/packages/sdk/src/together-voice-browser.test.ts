// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { microphoneConstraints } from "./together-voice-browser.ts";

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
