// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { canFollowAGroup, type GlobalOf } from "./follow-support.ts";

/** A page whose globals are those given, by name, and nothing else. */
function pageWith(...globals: (readonly [string, unknown])[]): GlobalOf {
  const byName = new Map(globals);
  return (name) => byName.get(name);
}

/** A MediaSource that plays the types given. */
function mediaSourcePlaying(...types: string[]): unknown {
  return { isTypeSupported: (type: string) => types.includes(type) };
}

const wholeSourceBuffer = { prototype: { appendBuffer: () => undefined, remove: () => undefined } };

describe("whether a page can follow a Watch Together group (D250 §7)", () => {
  it("can where a MediaSource plays H.264 and AAC in MP4, or only one of hls.js's other types", () => {
    const h264 = mediaSourcePlaying("video/mp4;codecs=avc1.42E01E,mp4a.40.2");
    expect(
      canFollowAGroup(pageWith(["MediaSource", h264], ["SourceBuffer", wholeSourceBuffer])),
    ).toBe(true);
    const flacOnly = mediaSourcePlaying("audio/mp4;codecs=fLaC");
    expect(canFollowAGroup(pageWith(["MediaSource", flacOnly]))).toBe(true);
  });

  it("can on an iPhone's ManagedMediaSource alone, iOS 17.1 and later", () => {
    const managed = mediaSourcePlaying("video/mp4;codecs=avc1.42E01E,mp4a.40.2");
    expect(canFollowAGroup(pageWith(["ManagedMediaSource", managed]))).toBe(true);
  });

  it("cannot with no MediaSource, one that plays none of the types, or a SourceBuffer short of its API", () => {
    expect(canFollowAGroup(pageWith())).toBe(false);
    expect(canFollowAGroup(pageWith(["MediaSource", mediaSourcePlaying()]))).toBe(false);
    const h264 = mediaSourcePlaying("video/mp4;codecs=avc1.42E01E,mp4a.40.2");
    const halfBuffer = { prototype: { appendBuffer: () => undefined } };
    expect(canFollowAGroup(pageWith(["MediaSource", h264], ["SourceBuffer", halfBuffer]))).toBe(
      false,
    );
    expect(canFollowAGroup(pageWith(["MediaSource", {}]))).toBe(false);
  });
});
