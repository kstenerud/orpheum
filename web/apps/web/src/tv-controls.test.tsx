// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { clockText, overlayAnswer, TvControls } from "./tv-controls.tsx";

describe("a television's player overlay, as Jellyfin's (the owner, Fire TV, 2026-10-25)", () => {
  it("seeks and shows the scrub bar alone on Left and Right while hidden, taking the press", () => {
    expect(overlayAnswer("none", "left", false)).toEqual({
      shown: "scrub",
      isTaken: true,
      seekByMs: -10_000,
    });
    expect(overlayAnswer("scrub", "right", false)).toEqual({
      shown: "scrub",
      isTaken: true,
      seekByMs: 10_000,
    });
  });

  it("opens all of it on OK, Up or Down while hidden, focus to play, and does nothing else", () => {
    for (const command of ["select", "up", "down"] as const) {
      expect(overlayAnswer("none", command, false)).toEqual({
        shown: "all",
        isTaken: true,
        isFocusToPlay: true,
      });
    }
  });

  it("hides on Back while it shows, and leaves Back to the page while hidden", () => {
    expect(overlayAnswer("all", "back", false)).toEqual({ shown: "none", isTaken: true });
    expect(overlayAnswer("none", "back", false)).toEqual({ shown: "none", isTaken: false });
  });

  it("while all of it shows, seeks by Left and Right on the scrub bar, and leaves other moves to focus", () => {
    expect(overlayAnswer("all", "left", true)).toEqual({
      shown: "all",
      isTaken: true,
      seekByMs: -10_000,
    });
    expect(overlayAnswer("all", "left", false)).toEqual({ shown: "all", isTaken: false });
    expect(overlayAnswer("all", "down", false)).toEqual({ shown: "all", isTaken: false });
  });

  it("leaves play, pause and the seeking keys to the player, showing the scrub bar", () => {
    expect(overlayAnswer("none", "playPause", false)).toEqual({ shown: "scrub", isTaken: false });
    expect(overlayAnswer("all", "seekForward", false)).toEqual({ shown: "all", isTaken: false });
  });

  it("says a film's times as a player does", () => {
    expect(clockText(0)).toBe("0:00");
    expect(clockText(1_466_000)).toBe("24:26");
    expect(clockText(3_725_000)).toBe("1:02:05");
  });
});

describe("the overlay as drawn", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("is play and a scrub bar a remote reaches, with no rewind or fast-forward", async () => {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", document.createElement("video"));
    mounted = await mount(
      withServices(
        <TvControls videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />,
      ),
    );
    expect(mounted.container.querySelector('button[aria-label="Play"]')).not.toBeNull();
    expect(mounted.container.querySelector('input[type="range"]')).not.toBeNull();
    expect(mounted.container.querySelectorAll("button")).toHaveLength(1);
  });
});
