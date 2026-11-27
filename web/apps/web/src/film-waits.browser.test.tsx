// SPDX-License-Identifier: MPL-2.0

/**
 * The sign over a film waiting for data, as a real browser draws it: a turning ring, and, asked for
 * less motion, the still-reading hourglass in its place, white on the film's dark disc, pulsing once
 * every 3 seconds (the owner, 2026-11-16, "Use the 3s hourglass (white) for the video").
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import type { LinkWait } from "@orpheum/player";
import { act, useRef, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { FilmWaitsSpinner, spinnerAfterMs } from "./film-waits.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

/** A video and its sign, as the player's page lays them out, its wait's chunks coming as wait says. */
function WaitingFilm(props: {
  readonly wait?: LinkWait | undefined;
  readonly stage: StageSize;
}): JSX.Element {
  const videoRef = useRef<HTMLVideoElement>(null);
  const { wait, stage } = props;
  return (
    <div
      className="test-stage"
      style={{
        position: "relative",
        width: `${String(stage.width)}px`,
        height: `${String(stage.height)}px`,
      }}
    >
      <video ref={videoRef}>
        <track kind="captions" />
      </video>
      <FilmWaitsSpinner
        videoRef={videoRef}
        isFailed={false}
        watchLinkWait={() => ({ read: () => wait, stop: () => undefined })}
      />
    </div>
  );
}

/** A stage's size in CSS pixels. */
interface StageSize {
  readonly width: number;
  readonly height: number;
}

/** The stage of a film at a desktop's width. */
const tallStage: StageSize = { width: 640, height: 360 };

/** Mounts a film that plays with nothing loaded, a stall, and waits until its sign shows. */
async function stalledFilm(wait?: LinkWait, stage: StageSize = tallStage): Promise<Mounted> {
  const mounted = await mount(<WaitingFilm wait={wait} stage={stage} />);
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
  // The sign shows with its words, "Getting ready to play", a Pill under it (D538 §1).
  await expect
    .poll(() => mounted.container.querySelector(".film-waits:not([hidden])")?.textContent)
    .toContain("Getting ready to play");
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

  it("lights a quarter of the ring and fades a tail half as long behind it, only on the ring itself (the owner, 2026-11-17)", async () => {
    mounted = await stalledFilm();
    const lit = getComputedStyle(one(mounted, ".film-waits__spinner"), "::before");
    expect(lit.backgroundImage).toMatch(/^conic-gradient\(from 270deg, /u);
    expect(lit.backgroundImage).toContain("rgb(255, 255, 255) 45deg");
    expect(lit.backgroundImage).toContain("rgb(255, 255, 255) 135deg");
    // Cut to the ring's width, so the see-through disc inside shows none of it.
    expect(lit.maskImage).toMatch(/^radial-gradient\(/u);
  });

  it("is the white hourglass, pulsing once every 3 seconds, in the ring's place under Reduce Motion", async () => {
    mounted = await stalledFilm();
    restore = askForLessMotion();
    expect(getComputedStyle(one(mounted, ".film-waits__spinner")).display).toBe("none");
    const hourglass = getComputedStyle(one(mounted, ".film-waits__hourglass"));
    expect(hourglass.display).toBe("block");
    expect(hourglass.animationName).toBe("film-waits-pulse");
    // The global reset sets every animation to 0 s under Reduce Motion; this one keeps its length.
    expect(hourglass.animationDuration).toBe("3s");
    expect(hourglass.animationIterationCount).toBe("infinite");
    const icon = one(mounted, ".film-waits__hourglass .waiting-icon");
    // White, as the ring's lit quarter is, and at the ring's size.
    expect(getComputedStyle(icon).color).toBe("rgb(255, 255, 255)");
    const box = icon.getBoundingClientRect();
    expect(box.width).toBeCloseTo(56, 0);
    expect(box.height).toBeCloseTo(56, 0);
  });
});

describe("the bar under the sign, a chunk held up on the link (the owner, 2026-11-17)", () => {
  /** A stalled film whose wait's chunks have half come for 3 seconds, once its bar shows. */
  async function barredFilm(): Promise<Mounted> {
    const shown = await stalledFilm({
      arrivedBytes: 200_000,
      neededBytes: 400_000,
      linksForMs: 3000,
    });
    await expect.poll(() => shown.container.querySelector(".film-waits__bar")).not.toBeNull();
    return shown;
  }
  let mounted: Mounted | undefined;
  let restore: (() => void) | undefined;
  afterEach(() => {
    restore?.();
    restore = undefined;
    mounted?.unmount();
    mounted = undefined;
  });

  it("is a third of the picture's width, under the ring, which stays in the middle, its filler white and as full as the bytes come", async () => {
    mounted = await barredFilm();
    const bar = one(mounted, ".film-waits__bar").getBoundingClientRect();
    const ring = one(mounted, ".film-waits__spinner").getBoundingClientRect();
    const fill = one(mounted, ".film-waits__fill");
    expect(bar.width).toBeCloseTo(640 / 3, 0);
    expect(bar.left).toBeCloseTo(ring.left + ring.width / 2 - bar.width / 2, 0);
    expect(bar.top).toBeGreaterThan(ring.bottom);
    expect(ring.top + ring.height / 2).toBeCloseTo(180, 0);
    expect(fill.getBoundingClientRect().width).toBeCloseTo((bar.width - 2 * 3) / 2, 0);
    expect(getComputedStyle(fill).backgroundColor).toBe("rgb(255, 255, 255)");
    expect(getComputedStyle(one(mounted, ".film-waits__bar")).backgroundColor).not.toBe(
      "rgba(0, 0, 0, 0)",
    );
  });

  it("fades its filler out and back in every 2 seconds, and every 3 under Reduce Motion", async () => {
    mounted = await barredFilm();
    const fill = getComputedStyle(one(mounted, ".film-waits__fill"));
    expect(fill.animationName).toBe("film-waits-fade");
    expect(fill.animationDuration).toBe("2s");
    expect(fill.animationIterationCount).toBe("infinite");
    restore = askForLessMotion();
    expect(getComputedStyle(one(mounted, ".film-waits__fill")).animationDuration).toBe("3s");
  });

  it("is not there while the wait says nothing, a chunk the server is still making", async () => {
    mounted = await stalledFilm();
    await new Promise((resolve) => setTimeout(resolve, 500)); // two of its reads
    expect(mounted.container.querySelector(".film-waits__bar")).toBeNull();
  });
});

describe("the sign, its bar and its words on a stage of any height (the owner, 2026-11-27)", () => {
  /** A wait whose chunks have half come for 3 seconds, so the bar shows. */
  const halfCome: LinkWait = { arrivedBytes: 200_000, neededBytes: 400_000, linksForMs: 3000 };
  /** A phone's stage in portrait: as the owner's showed it while a film started, and a film at
   * 2.76:1, the widest made, the shortest stage at a phone's width. */
  const phoneStages: readonly StageSize[] = [
    { width: 328, height: 150 },
    { width: 328, height: 119 },
  ];
  let mounted: Mounted | undefined;
  let restore: (() => void) | undefined;
  afterEach(() => {
    restore?.();
    restore = undefined;
    mounted?.unmount();
    mounted = undefined;
  });

  /** A stalled film on stage, its bar showing once it has if wait says chunks are coming. */
  async function shownOn(stage: StageSize, wait: LinkWait | undefined): Promise<Mounted> {
    const shown = await stalledFilm(wait, stage);
    if (wait !== undefined) {
      await expect.poll(() => shown.container.querySelector(".film-waits__bar")).not.toBeNull();
    }
    return shown;
  }

  for (const [named, wait] of [
    ["with its bar", halfCome],
    ["without its bar", undefined],
  ] as const) {
    it.each(phoneStages)(
      `keeps them all inside a phone's short stage ($width × $height), none over another, ${named}, ring or hourglass`,
      async (phoneStage) => {
        mounted = await shownOn(phoneStage, wait);
        expectAllInside(mounted, wait);
        restore = askForLessMotion();
        expectAllInside(mounted, wait);
      },
    );

    it(`keeps the designed places on a tall stage, the sign in its middle, ${named}`, async () => {
      mounted = await shownOn(tallStage, wait);
      const { stage, sign, bar, words } = boxes(mounted);
      const middle = stage.top + stage.height / 2;
      expect(sign.top + sign.height / 2).toBeCloseTo(middle, 0);
      // The bar 1rem under the sign, the words 1rem under the bar's place, which is 0.75rem tall.
      expect(words.top).toBeCloseTo(middle + 28 + 16 + 12 + 16, 0);
      if (bar !== undefined) {
        expect(bar.top).toBeCloseTo(middle + 28 + 16, 0);
      }
    });
  }
});

/** The boxes of the stage, the sign (the ring or the hourglass), the bar if shown and the words. */
function boxes(shown: Mounted): {
  stage: DOMRect;
  sign: DOMRect;
  bar: DOMRect | undefined;
  words: DOMRect;
} {
  return {
    stage: one(shown, ".test-stage").getBoundingClientRect(),
    // The box the ring or the hourglass stands in: the ring's own grows as it turns.
    sign: one(shown, ".film-waits__sign").getBoundingClientRect(),
    bar: shown.container.querySelector(".film-waits__bar")?.getBoundingClientRect(),
    words: one(shown, ".film-waits__words .pill").getBoundingClientRect(),
  };
}

/** Whether a lies wholly inside b. */
function isInside(a: DOMRect, b: DOMRect): boolean {
  return a.top >= b.top && a.bottom <= b.bottom && a.left >= b.left && a.right <= b.right;
}

/** Whether a and b share any of their area. */
function overlaps(a: DOMRect, b: DOMRect): boolean {
  return a.top < b.bottom && b.top < a.bottom && a.left < b.right && b.left < a.right;
}

/** Every part of the waiting sign mounted shows lies inside its stage, none over another. */
function expectAllInside(mounted: Mounted, wait: LinkWait | undefined): void {
  const { stage, sign, bar, words } = boxes(mounted);
  expect(sign.height).toBeCloseTo(56, 0);
  expect(words.height).toBeGreaterThan(0);
  expect(isInside(words, stage)).toBe(true);
  expect(isInside(sign, stage)).toBe(true);
  expect(overlaps(words, sign)).toBe(false);
  expect(bar === undefined).toBe(wait === undefined);
  if (bar !== undefined) {
    expect(isInside(bar, stage)).toBe(true);
    expect(overlaps(words, bar)).toBe(false);
    expect(overlaps(sign, bar)).toBe(false);
  }
}
