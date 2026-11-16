// SPDX-License-Identifier: MPL-2.0

/**
 * The sign over a film waiting for data, as a real browser draws it: a turning ring, and, asked for
 * less motion, the still-reading hourglass in its place, white on the film's dark disc, pulsing once
 * every 3 seconds (the owner, 2026-11-16, "Use the 3s hourglass (white) for the video").
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { act, useRef, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { FilmWaitsSpinner, spinnerAfterMs } from "./film-waits.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

/** A video and its sign, as the player's page lays them out. */
function WaitingFilm(): JSX.Element {
  const videoRef = useRef<HTMLVideoElement>(null);
  return (
    <div style={{ position: "relative", width: "640px", height: "360px" }}>
      <video ref={videoRef}>
        <track kind="captions" />
      </video>
      <FilmWaitsSpinner videoRef={videoRef} isFailed={false} />
    </div>
  );
}

/** Mounts a film that plays with nothing loaded, a stall, and waits until its sign shows. */
async function stalledFilm(): Promise<Mounted> {
  const mounted = await mount(<WaitingFilm />);
  const video = mounted.container.querySelector("video");
  if (video === null) {
    throw new Error("the page has no video");
  }
  Object.defineProperty(video, "paused", { value: false });
  Object.defineProperty(video, "readyState", { value: 0 });
  await act(async () => {
    video.dispatchEvent(new Event("waiting"));
    await new Promise((resolve) => setTimeout(resolve, spinnerAfterMs + 100));
  });
  await expect
    .poll(() => mounted.container.querySelector('[aria-label="Getting ready to play"]'))
    .not.toBeNull();
  return mounted;
}

/** The rules every stylesheet keeps for Reduce Motion, added again for every motion, as asked for less. */
function askForLessMotion(): () => void {
  const added = document.createElement("style");
  const rules = [...document.styleSheets]
    .flatMap((sheet) => [...sheet.cssRules])
    .filter(
      (rule): rule is CSSMediaRule =>
        rule instanceof CSSMediaRule && rule.conditionText.includes("prefers-reduced-motion"),
    )
    .flatMap((rule) => [...rule.cssRules].map((inner) => inner.cssText));
  added.textContent = rules.join("\n");
  document.head.append(added);
  return () => {
    added.remove();
  };
}

/** The one element selector matches in mounted. */
function one(mounted: Mounted, selector: string): HTMLElement | SVGElement {
  const found = mounted.container.querySelector(selector);
  if (!(found instanceof HTMLElement || found instanceof SVGElement)) {
    throw new Error(`the page has no ${selector}`);
  }
  return found;
}

describe("the sign over a film waiting for data", () => {
  let mounted: Mounted | undefined;
  let restore: (() => void) | undefined;
  afterEach(() => {
    restore?.();
    restore = undefined;
    mounted?.unmount();
    mounted = undefined;
  });

  it("is the turning ring, with no hourglass, when motion is not asked to stop", async () => {
    mounted = await stalledFilm();
    const ring = getComputedStyle(one(mounted, ".film-waits__spinner"));
    expect(ring.display).not.toBe("none");
    expect(ring.animationName).toBe("film-waits-turn");
    expect(getComputedStyle(one(mounted, ".film-waits__hourglass")).display).toBe("none");
  });

  it("is the white hourglass, pulsing once every 3 seconds, in the ring's place under Reduce Motion", async () => {
    mounted = await stalledFilm();
    const ringColour = getComputedStyle(one(mounted, ".film-waits__spinner")).borderTopColor;
    restore = askForLessMotion();
    expect(getComputedStyle(one(mounted, ".film-waits__spinner")).display).toBe("none");
    const hourglass = getComputedStyle(one(mounted, ".film-waits__hourglass"));
    expect(hourglass.display).toBe("block");
    expect(hourglass.animationName).toBe("film-waits-pulse");
    // The global reset sets every animation to 0 s under Reduce Motion; this one keeps its length.
    expect(hourglass.animationDuration).toBe("3s");
    expect(hourglass.animationIterationCount).toBe("infinite");
    const icon = one(mounted, ".film-waits__hourglass .still-reading-icon");
    // White, as the ring's lit quarter is, and at the ring's size.
    expect(getComputedStyle(icon).color).toBe(ringColour);
    const box = icon.getBoundingClientRect();
    expect(box.width).toBeCloseTo(56, 0);
    expect(box.height).toBeCloseTo(56, 0);
  });
});
