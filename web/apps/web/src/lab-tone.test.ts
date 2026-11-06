// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { beepFadeMs, beepsWav, toneWav } from "./lab-tone.ts";

describe("a lab tone", () => {
  it("is a mono 16-bit WAV of whole cycles, so it loops without a click", () => {
    const bytes = toneWav(440, { seconds: 1, sampleRate: 22_050 });
    const view = new DataView(bytes.buffer);
    const text = (at: number): string => String.fromCodePoint(...bytes.slice(at, at + 4));
    expect([text(0), text(8), text(12), text(36)]).toEqual(["RIFF", "WAVE", "fmt ", "data"]);
    expect(view.getUint16(22, true)).toBe(1);
    expect(view.getUint32(24, true)).toBe(22_050);
    expect(view.getUint16(34, true)).toBe(16);
    // 440 cycles of 22050/440 samples each.
    const samples = view.getUint32(40, true) / 2;
    expect(samples).toBe(22_050);
    expect(bytes.length).toBe(44 + samples * 2);
    expect(view.getInt16(44, true)).toBe(0);
  });

  it("is silent but for its beeps, each faded in and out, in a WAV as long as asked", () => {
    const sampleRate = 8000;
    const bytes = beepsWav([{ frequencyHz: 1000, atMs: 500, lastsMs: 100 }], {
      lastingMs: 1000,
      sampleRate,
      amplitude: 0.5,
    });
    const view = new DataView(bytes.buffer);
    expect(view.getUint32(24, true)).toBe(sampleRate);
    const samples = view.getUint32(40, true) / 2;
    expect(samples).toBe(8000);
    const sample = (index: number): number => view.getInt16(44 + index * 2, true);
    const loudestIn = (from: number, to: number): number =>
      Math.max(...Array.from({ length: to - from }, (_, at) => Math.abs(sample(from + at))));
    // Silent before the beep's 500 ms and after its 600 ms.
    expect(loudestIn(0, 4000)).toBe(0);
    expect(loudestIn(4800, 8000)).toBe(0);
    // Faded in: its first sample is silent, and past the fade it is at its amplitude.
    expect(sample(4000)).toBe(0);
    expect(loudestIn(4000 + (beepFadeMs * sampleRate) / 1000, 4790)).toBeGreaterThan(16_000);
  });
});
