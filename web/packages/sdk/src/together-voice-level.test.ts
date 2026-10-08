// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  levelOf,
  mixToMono,
  nextVoiceGain,
  voiceGainMost,
  voiceGainRise,
  voiceSpeechFloor,
  voiceTargetLevel,
} from "./together-voice-level.ts";

describe("raising a quiet microphone to a speaking level", () => {
  it("rises toward the gain the target needs, slowly, for a quiet voice", () => {
    // The owner's Android voice arrived at 0.005: it needs 20 times, reached a step at a time.
    expect(nextVoiceGain(1, 0.005)).toBeCloseTo(voiceGainRise);
    let gain = 1;
    for (let look = 0; look < 40; look++) {
      gain = nextVoiceGain(gain, 0.005);
    }
    expect(gain).toBeCloseTo(voiceTargetLevel / 0.005);
  });

  it("falls at once when a voice comes in loud, and never below 1", () => {
    expect(nextVoiceGain(20, 0.05)).toBeCloseTo(voiceTargetLevel / 0.05);
    expect(nextVoiceGain(20, 0.5)).toBe(1);
  });

  it("never rises past the most, and holds while only the room is heard", () => {
    let gain = 1;
    for (let look = 0; look < 100; look++) {
      gain = nextVoiceGain(gain, voiceSpeechFloor);
    }
    expect(gain).toBe(voiceGainMost);
    expect(nextVoiceGain(7, voiceSpeechFloor / 2)).toBe(7);
  });

  it("measures a level as the samples' root mean square", () => {
    expect(levelOf(new Float32Array([0.1, -0.1, 0.1, -0.1]))).toBeCloseTo(0.1);
    expect(levelOf(new Float32Array([]))).toBe(0);
  });
});

describe("a voice's channels", () => {
  it("are mixed to one, which plays in both ears", () => {
    const node: {
      channelCount: number;
      channelCountMode: ChannelCountMode;
      channelInterpretation: ChannelInterpretation;
    } = { channelCount: 2, channelCountMode: "max", channelInterpretation: "discrete" };
    mixToMono(node);
    expect(node).toEqual({
      channelCount: 1,
      channelCountMode: "explicit",
      channelInterpretation: "speakers",
    });
  });
});
