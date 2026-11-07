// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { findsMicrophone, type MicrophoneFinder } from "./has-microphone.ts";

function listing(kinds: readonly MediaDeviceKind[]): MicrophoneFinder {
  return {
    enumerateDevices: () =>
      Promise.resolve(kinds.map((kind) => ({ kind }) as unknown as MediaDeviceInfo)),
  };
}

describe("whether the device has a microphone (D440 §3)", () => {
  it("finds one where the browser lists an audio input", async () => {
    expect(await findsMicrophone(listing(["videoinput", "audioinput"]))).toBe(true);
  });

  it("finds none where it lists no audio input, as a television's browser does", async () => {
    expect(await findsMicrophone(listing(["audiooutput"]))).toBe(false);
  });

  it("finds none without media devices, or where listing them fails", async () => {
    expect(await findsMicrophone(undefined)).toBe(false);
    expect(
      await findsMicrophone({ enumerateDevices: () => Promise.reject(new Error("refused")) }),
    ).toBe(false);
  });

  it("asks whether a microphone can open at all where the browser cannot list devices", async () => {
    const getUserMedia = (): Promise<MediaStream> => Promise.reject(new Error("unused"));
    expect(await findsMicrophone({ getUserMedia })).toBe(true);
    expect(await findsMicrophone({})).toBe(false);
  });
});
