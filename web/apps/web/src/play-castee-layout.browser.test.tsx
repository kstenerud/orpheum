// SPDX-License-Identifier: MPL-2.0

// The castee's Play page as a real browser lays it out: the remote's layout (D548 §1, §5, §6, §8;
// the owner, 2026-11-26: "the remote-control looking layout (3x3 pad). That should be the layout for
// portrait and landscape mode when casting"), drawn at a phone upright, a phone on its side and a
// computer: one column with the pad at the bottom under the offer area, or two with the pad's column
// at the end; the pad's buttons at their sizes; nothing past the screen; Mute pressed on the status
// ground; no overlay and no ⤢ (the owner, 2026-11-26: "Never offer the fullscreen mode button when
// casting").

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";

import { CasteePlay } from "./play-castee.tsx";
import { castingRemote, receiverSays } from "./test-castee.tsx";
import type { Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  window.sessionStorage.clear();
});

/** The parts of the castee's page, by their selectors. */
const bar = ".castee-play__top";
const offer = ".castee-play__stage";
const timeline = ".castee-play__page > .remote-pad__timeline";
const pad = ".castee-play__page > .remote-pad";
const buttons = ".castee-play__page > .remote-pad__menus";

/** The castee's Play page on film 4 in a window width × height, its receiver saying the film's level, muted. */
async function casteeAt(width: number, height: number): Promise<HTMLElement> {
  await page.viewport(width, height);
  const drawn = await castingRemote({}, [], { remote: CasteePlay, onPlayHere: vi.fn() });
  mounted = drawn.mounted;
  await receiverSays(drawn.sockets.session, {
    delivery: "direct",
    sound: {
      isLevellingOn: false,
      audioTracks: [],
      channels: [{ channel: "film", level: 0.5, isMuted: true }],
    },
  });
  await expect
    .poll(() => drawn.mounted.container.querySelector(`${pad} [aria-label="Volume up"]`))
    .not.toBeNull();
  return drawn.mounted.container;
}

/** Where part (a selector under the page) is drawn; it fails the test when there is none. */
function boxOf(container: HTMLElement, part: string): DOMRect {
  const element = container.querySelector(part);
  if (element === null) {
    throw new Error(`no ${part}`);
  }
  return element.getBoundingClientRect();
}

/** A pad button's drawn width, by its name. */
function padSize(container: HTMLElement, name: string): number {
  return boxOf(container, `${pad} [aria-label="${name}"]`).width;
}

/** That every part sits inside the window, so none is scrolled to, and there is no overlay nor ⤢. */
function expectAllInside(container: HTMLElement, width: number, height: number): void {
  for (const part of [bar, offer, timeline, pad, buttons]) {
    const box = boxOf(container, part);
    expect(box.left, part).toBeGreaterThanOrEqual(0);
    expect(box.right, part).toBeLessThanOrEqual(width + 0.5);
    expect(box.bottom, part).toBeLessThanOrEqual(height + 0.5);
  }
  expect(container.querySelector(".player-overlay")).toBeNull();
  expect(container.querySelector('[aria-label="Full screen"]')).toBeNull();
}

/** That nothing scrolls: the menus' bottom is in the window, and the card is 9rem tall or more. */
function expectFits(container: HTMLElement, height: number): void {
  expect(boxOf(container, buttons).bottom).toBeLessThanOrEqual(height + 0.5);
  expect(boxOf(container, offer).height).toBeGreaterThanOrEqual(144 - 0.5);
}

/** That the offer area is drawn 16:9. */
function expectSixteenByNine(container: HTMLElement): void {
  const box = boxOf(container, offer);
  expect(box.width / box.height).toBeCloseTo(16 / 9, 1);
}

/** A pad button's ground, by its name. */
function groundOf(container: HTMLElement, name: string): string {
  const button = container.querySelector(`${pad} [aria-label="${name}"]`);
  return button === null ? "" : getComputedStyle(button).backgroundColor;
}

/** That the menus' row's last button ends at the row's end, inside its edge alone: the row is end-aligned. */
function expectMenusAtTheEnd(container: HTMLElement): void {
  const row = container.querySelector<HTMLElement>(buttons)!;
  const last = [...row.querySelectorAll<HTMLElement>("[role=button], button")].at(-1)!;
  const end =
    row.getBoundingClientRect().right - Number.parseFloat(getComputedStyle(row).paddingRight);
  expect(Math.abs(last.getBoundingClientRect().right - end)).toBeLessThan(1.5);
}

/**
 * That "Ends: ‹time›" is drawn directly under the way up, at its start (the owner, 2026-11-26: "The
 * "Ends" line is placed wrong. It should be under the backlink at the top"), sharing the way up's
 * touch box as the sign under it did, "On: ‹TV›" after it on its line.
 */
function expectEndsUnderTheWayUp(container: HTMLElement): void {
  const up = boxOf(container, ".castee-play__heading > :first-child");
  const ends = boxOf(container, ".castee-play__where .convenience-area__ends");
  const sign = boxOf(container, ".castee-play__sign");
  expect(
    container.querySelector(".castee-play__where .convenience-area__ends")?.textContent,
  ).toMatch(/^Ends: /u);
  expect(Math.abs(ends.left - up.left)).toBeLessThan(1.5);
  expect(ends.top).toBeGreaterThanOrEqual(up.bottom - 10);
  expect(ends.top).toBeLessThanOrEqual(up.bottom + 4);
  expect(sign.left).toBeGreaterThanOrEqual(ends.right);
}

/** That the timeline's track's middle is 16 px under frame, the slider's thumb clear of it. */
function expectTrackAStepUnder(container: HTMLElement, frame: string): void {
  const range = boxOf(container, `${timeline} input[type="range"]`);
  const middle = range.top + range.height / 2;
  expect(Math.abs(middle - boxOf(container, frame).bottom - 16)).toBeLessThan(1.5);
}

/**
 * That the time sits close under the timeline's track at its end (the owner, 2026-11-26: "directly
 * under the scrub bar on the right-hand side, right up close so that there's just enough clearance
 * for the position gadget to not overlap"): its end a step in from the slider's, its top no more than
 * 12 px under the track's middle, clear of the thumb's half of 8 px.
 */
function expectTimeCloseUnderAtItsEnd(container: HTMLElement): void {
  const range = boxOf(container, `${timeline} input[type="range"]`);
  const time = boxOf(container, `${timeline} .remote-pad__time`);
  const middle = range.top + range.height / 2;
  expect(Math.abs(time.right - (range.right - 8))).toBeLessThan(1.5);
  expect(time.left).toBeGreaterThan(range.left + range.width / 2);
  expect(time.top - middle).toBeGreaterThanOrEqual(7.5);
  expect(time.top - middle).toBeLessThanOrEqual(12);
}

describe("the castee's Play page laid out as the remote (the owner, 2026-11-26)", () => {
  it("draws when the film ends directly under the way up, upright, on its side and on a computer", async () => {
    expectEndsUnderTheWayUp(await casteeAt(390, 844));
    mounted?.unmount();
    expectEndsUnderTheWayUp(await casteeAt(780, 360));
    mounted?.unmount();
    expectEndsUnderTheWayUp(await casteeAt(1280, 800));
  });

  it("draws the time close under the timeline's track at its end, upright and on its side", async () => {
    expectTimeCloseUnderAtItsEnd(await casteeAt(390, 844));
    mounted?.unmount();
    expectTimeCloseUnderAtItsEnd(await casteeAt(780, 360));
  });

  it("lays a phone upright out as one column, the pad at the bottom under the offer area, Play 96 px, the others 72 and 64 px, Mute pressed on the status ground", async () => {
    const upright = await casteeAt(360, 780);
    expectAllInside(upright, 360, 780);
    expectFits(upright, 780);
    expectSixteenByNine(upright);
    const offerBox = boxOf(upright, offer);
    const timelineBox = boxOf(upright, timeline);
    const padBox = boxOf(upright, pad);
    const buttonsBox = boxOf(upright, buttons);
    // The slider's room for a finger reaches over the card's foot, its track under it (the owner, 2026-11-26).
    expect(timelineBox.top + timelineBox.height / 2).toBeGreaterThanOrEqual(offerBox.bottom);
    expect(padBox.top).toBeGreaterThanOrEqual(timelineBox.bottom);
    expect(buttonsBox.top).toBeGreaterThanOrEqual(padBox.bottom);
    // The room left over goes above the pad: the menus end at the bottom of the screen.
    expect(780 - buttonsBox.bottom).toBeLessThan(2);
    expect(offerBox.left).toBeCloseTo(16, 0);
    expect(padSize(upright, "Play")).toBeCloseTo(96, 0);
    expect(padSize(upright, "Back 10 s")).toBeCloseTo(72, 0);
    expect(padSize(upright, "Volume up")).toBeCloseTo(64, 0);
    expect(groundOf(upright, "Mute")).not.toBe(groundOf(upright, "Volume up"));
    expectMenusAtTheEnd(upright);
  });

  it("lays a phone on its side out as two columns, the pad's 15rem at the end, Play 72 px and the others 56 px, all within its height", async () => {
    const side = await casteeAt(780, 360);
    expectAllInside(side, 780, 360);
    expectFits(side, 360);
    expectSixteenByNine(side);
    const offerBox = boxOf(side, offer);
    const padBox = boxOf(side, pad);
    expect(padBox.left).toBeGreaterThanOrEqual(offerBox.right);
    expect(padBox.width).toBeCloseTo(240, 0);
    expect(boxOf(side, bar).bottom).toBeLessThanOrEqual(Math.min(offerBox.top, padBox.top) + 0.5);
    expect(padSize(side, "Play")).toBeCloseTo(72, 0);
    expect(padSize(side, "Back 10 s")).toBeCloseTo(56, 0);
    expect(padSize(side, "Volume up")).toBeCloseTo(56, 0);
  });

  it("lays a computer out as two columns, the pad's 22rem at the end, Play 96 px", async () => {
    const wide = await casteeAt(1280, 800);
    expectAllInside(wide, 1280, 800);
    expectFits(wide, 800);
    expectSixteenByNine(wide);
    const offerBox = boxOf(wide, offer);
    const padBox = boxOf(wide, pad);
    expect(padBox.left).toBeGreaterThanOrEqual(offerBox.right);
    expect(padBox.width).toBeCloseTo(352, 0);
    expect(padSize(wide, "Play")).toBeCloseTo(96, 0);
    expectMenusAtTheEnd(wide);
  });

  it("keeps the card a third or more of the width on a short phone on its side, as a browser's bars leave it", async () => {
    const short = await casteeAt(740, 300);
    expectSixteenByNine(short);
    expectFits(short, 300);
    const offerBox = boxOf(short, offer);
    expect(offerBox.width).toBeGreaterThanOrEqual(740 * 0.3);
  });

  // The page felt cramped once every control was on (the owner, 2026-11-25): the sign shares the way
  // up's touch box, the timeline sits close under the card, the buttons row has a step more above it.
  // The owner, 2026-11-26: "In portrait mode, the scrub bar can be much closer to the video frame
  // since the frame itself is non-interactive": the track's middle 16 px under the key frame.
  it("keeps the sign close under the way up, the timeline's track a step under the card and a step above the buttons row, on a phone upright", async () => {
    const upright = await casteeAt(390, 844);
    const up = boxOf(upright, ".castee-play__heading > :first-child");
    expect(boxOf(upright, ".castee-play__where").top).toBeLessThanOrEqual(up.bottom - 7.5);
    expectTrackAStepUnder(upright, offer);
    const firstButton = boxOf(upright, `${buttons} [role=button]`);
    expect(firstButton.top - boxOf(upright, pad).bottom).toBeGreaterThanOrEqual(11.5);
  });

  it("draws the timeline's track a step under the card on a phone upright with its browser's bars too", async () => {
    expectTrackAStepUnder(await casteeAt(390, 660), offer);
  });

  it("fits a phone upright with its browser's bars, nothing to scroll to", async () => {
    const bars = await casteeAt(390, 660);
    expectAllInside(bars, 390, 660);
    expectFits(bars, 660);
    expectSixteenByNine(bars);
    expect(padSize(bars, "Play")).toBeCloseTo(72, 0);
    expect(boxOf(bars, offer).width).toBeCloseTo(390 - 32, 0);
  });
});
