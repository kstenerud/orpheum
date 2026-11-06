// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { toneWav } from "./lab-tone.ts";

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
});
