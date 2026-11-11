// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { PlayView } from "./play.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import {
  followPressesOfPlay,
  forgetVideoPressedForPlay,
  isThisIos,
} from "./video-pressed-for-play.ts";

/** Presses a link to href on the page, as a finger's tap does; returns the steps of each video's. */
function pressedLink(href: string): string[] {
  const steps: string[] = [];
  vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(() => {
    steps.push("play");
    return Promise.reject(new DOMException("paused", "AbortError"));
  });
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {
    steps.push("pause");
  });
  const link = document.createElement("a");
  link.setAttribute("href", href);
  link.textContent = "Play";
  document.body.append(link);
  link.addEventListener("click", (event) => {
    event.preventDefault(); // the app's Link follows it; here nothing does
    steps.push("followed");
  });
  link.click();
  link.remove();
  return steps;
}

/** The Play page drawn, for a film that plays its own file. */
let mounted: Mounted | undefined = undefined;

/** Whether the Play page, drawn, plays in its own video or in one kept for it. */
async function playPageVideo(): Promise<"own" | "kept" | "none"> {
  mounted?.unmount();
  mounted = await mount(
    withServices(
      <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
    ),
  );
  const stage = mounted.container.querySelector(".film-player__stage");
  if (stage?.querySelector(".film-player__kept video") instanceof HTMLVideoElement) {
    return "kept";
  }
  return stage?.querySelector("video") instanceof HTMLVideoElement ? "own" : "none";
}

describe("an iPhone's Play page, playing in the video its press readied", () => {
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    forgetVideoPressedForPlay();
    vi.restoreAllMocks();
  });

  // Chrome on an iPhone refused a start asked 0.58 s after the press (the owner, staging,
  // 2026-11-11): a video played in the press is let play later.
  it("plays and pauses a video in the press of a link to a Play page, before the link is followed", () => {
    followPressesOfPlay(document, true);
    expect(pressedLink("/play/2")).toEqual(["play", "pause", "followed"]);
  });

  it("plays its film in that video, not one of its own", async () => {
    followPressesOfPlay(document, true);
    pressedLink("/play/2");
    expect(await playPageVideo()).toBe("kept");
  });

  it("readies none for a press of a link elsewhere, nor where the browser needs none", async () => {
    followPressesOfPlay(document, true);
    expect(pressedLink("/titles/7")).toEqual(["followed"]);
    expect(await playPageVideo()).toBe("own");
    forgetVideoPressedForPlay();
    followPressesOfPlay(document, false);
    expect(pressedLink("/play/2")).toEqual(["followed"]);
    expect(await playPageVideo()).toBe("own");
  });

  it("needs it on an iPhone's or an iPad's browser, Chrome's too, and not on a Mac's or an Android's", () => {
    const chromeOnIphone =
      "Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/158.0.7462.0 Mobile/15E148 Safari/604.1";
    expect(isThisIos(chromeOnIphone)).toBe(true);
    expect(isThisIos("Mozilla/5.0 (iPad; CPU OS 18_5 like Mac OS X) AppleWebKit/605.1.15")).toBe(
      true,
    );
    expect(
      isThisIos(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15",
      ),
    ).toBe(false);
    expect(isThisIos("Mozilla/5.0 (Linux; Android 14; Pixel 3a) Chrome/138.0 Mobile")).toBe(false);
  });
});
