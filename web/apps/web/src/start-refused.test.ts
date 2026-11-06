// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { followPressesForRefusals, pageAtRefusal, startRefusedNote } from "./start-refused.ts";

describe("a film the browser would not start", () => {
  it("says the refusal and the page's press and full screen as it came (the owner's TCL TV, 2026-11-06)", () => {
    const refusal = new DOMException(
      "play() can only be initiated by a user gesture.",
      "NotAllowedError",
    );
    expect(
      startRefusedNote(refusal, {
        isPressLive: false,
        hasBeenPressed: true,
        isFullScreen: true,
        sincePressMs: 1240,
      }),
    ).toBe(
      "the browser refused to start a film (NotAllowedError: play() can only be initiated by a user" +
        " gesture.); pressLive=false pressed=true lastPress=1240 ms ago fullScreen=true",
    );
  });

  it("says when the page had no press, and a browser that does not say whether a press is live", () => {
    expect(
      startRefusedNote("refused", {
        isPressLive: undefined,
        hasBeenPressed: undefined,
        isFullScreen: false,
        sincePressMs: undefined,
      }),
    ).toBe(
      "the browser refused to start a film (not an error: refused); pressLive=undefined" +
        " pressed=undefined lastPress=none fullScreen=false",
    );
  });

  it("counts from the page's last press, a remote's key among them", () => {
    followPressesForRefusals();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    const since = pageAtRefusal().sincePressMs;
    expect(since).toBeDefined();
    expect(since).toBeLessThan(1000);
  });
});
