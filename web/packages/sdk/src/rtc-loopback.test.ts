// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { failureText, receivedLevelOf } from "./rtc-loopback.ts";

describe("a loopback's received level", () => {
  it("is the received audio's, never the sent or the video's, the highest where several say", () => {
    expect(
      receivedLevelOf([
        { type: "outbound-rtp", kind: "audio", audioLevel: 0.9 },
        { type: "inbound-rtp", kind: "video", audioLevel: 0.8 },
        { type: "inbound-rtp", kind: "audio", audioLevel: 0.2 },
        { type: "inbound-rtp", mediaType: "audio", audioLevel: 0.31 },
      ]),
    ).toBe(0.31);
  });

  it("reads a browser's older track report, and says nothing where none says a level", () => {
    expect(
      receivedLevelOf([{ type: "track", kind: "audio", remoteSource: true, audioLevel: 0.05 }]),
    ).toBe(0.05);
    expect(
      receivedLevelOf([{ type: "track", kind: "audio", remoteSource: false, audioLevel: 0.5 }]),
    ).toBeUndefined();
    expect(receivedLevelOf([{ type: "inbound-rtp", kind: "audio" }])).toBeUndefined();
  });

  it("tells a failure by its name and message", () => {
    expect(failureText(new DOMException("no", "NotAllowedError"))).toBe("NotAllowedError: no");
    expect(failureText("odd")).toBe("failed");
  });
});
