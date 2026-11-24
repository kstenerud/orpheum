// SPDX-License-Identifier: MPL-2.0

// The cast remote's own layout (D548 §1, §5, §6, §8), drawn in a real browser at a phone upright, a
// phone on its side and a computer: one column with the pad at the bottom under the offer area, or
// two with the pad's column at the end; the pad's buttons at their sizes; nothing past the screen;
// and Mute pressed on the status ground.

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";

import { CastRemote } from "./cast-remote.tsx";
import { castingRemote, receiverSays } from "./test-cast-remote.tsx";
import type { Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.restoreAllMocks();
});

/** The remote on film 4's Play page in a window width × height, its receiver saying the film's level, muted. */
async function remoteAt(width: number, height: number): Promise<HTMLElement> {
  await page.viewport(width, height);
  const drawn = await castingRemote({}, [], { remote: CastRemote, onPlayHere: vi.fn() });
  mounted = drawn.mounted;
  await receiverSays(drawn.sockets.session, {
    delivery: "direct",
    sound: {
      isLevellingOn: false,
      audioTracks: [],
      channels: [{ channel: "film", level: 0.5, isMuted: true }],
    },
  });
  return drawn.mounted.container;
}

/** Where part (a selector under the remote) is drawn. */
function boxOf(container: HTMLElement, part: string): DOMRect {
  const element = container.querySelector(part);
  if (element === null) {
    throw new Error(`no ${part}`);
  }
  return element.getBoundingClientRect();
}

/** A pad button's drawn width, by its name. */
function padSize(container: HTMLElement, name: string): number {
  return boxOf(container, `.cast-remote__pad [aria-label="${name}"]`).width;
}

/** That every part sits inside the window, so none is scrolled to. */
function expectAllInside(container: HTMLElement, width: number, height: number): void {
  for (const part of [
    ".cast-remote__bar",
    ".cast-remote__offer",
    ".cast-remote__timeline",
    ".cast-remote__pad",
    ".cast-remote__buttons",
  ]) {
    const box = boxOf(container, part);
    expect(box.left, part).toBeGreaterThanOrEqual(0);
    expect(box.right, part).toBeLessThanOrEqual(width + 0.5);
    expect(box.bottom, part).toBeLessThanOrEqual(height + 0.5);
  }
}

/** That nothing scrolls: the menus' bottom is in the window, and the card is 9rem tall or more. */
function expectFits(container: HTMLElement, height: number): void {
  expect(boxOf(container, ".cast-remote__buttons").bottom).toBeLessThanOrEqual(height + 0.5);
  expect(boxOf(container, ".cast-remote__offer").height).toBeGreaterThanOrEqual(144 - 0.5);
}

/** That the offer area is drawn 16:9. */
function expectSixteenByNine(container: HTMLElement): void {
  const offer = boxOf(container, ".cast-remote__offer");
  expect(offer.width / offer.height).toBeCloseTo(16 / 9, 1);
}

/** A pad button's ground, by its name. */
function groundOf(container: HTMLElement, name: string): string {
  const button = container.querySelector(`.cast-remote__pad [aria-label="${name}"]`);
  return button === null ? "" : getComputedStyle(button).backgroundColor;
}

describe("the cast remote's layout (D548 §1, §8)", () => {
  it("lays a phone upright out as one column, the pad at the bottom under the offer area, Play 96 px, the others 72 and 64 px, Mute pressed on the status ground", async () => {
    const upright = await remoteAt(360, 780);
    expectAllInside(upright, 360, 780);
    expectFits(upright, 780);
    expectSixteenByNine(upright);
    const offer = boxOf(upright, ".cast-remote__offer");
    const timeline = boxOf(upright, ".cast-remote__timeline");
    const pad = boxOf(upright, ".cast-remote__pad");
    const buttons = boxOf(upright, ".cast-remote__buttons");
    expect(timeline.top).toBeGreaterThanOrEqual(offer.bottom);
    expect(pad.top).toBeGreaterThanOrEqual(timeline.bottom);
    expect(buttons.top).toBeGreaterThanOrEqual(pad.bottom);
    // The room left over goes above the pad: the menus end at the bottom of the screen.
    expect(780 - buttons.bottom).toBeLessThan(2);
    expect(offer.left).toBeCloseTo(16, 0);
    expect(padSize(upright, "Play")).toBeCloseTo(96, 0);
    expect(padSize(upright, "Back 10 s")).toBeCloseTo(72, 0);
    expect(padSize(upright, "Volume up")).toBeCloseTo(64, 0);
    expect(groundOf(upright, "Mute")).not.toBe(groundOf(upright, "Volume up"));
  });

  it("lays a phone on its side out as two columns, the pad's 15rem at the end, Play 72 px and the others 56 px, all within its height", async () => {
    const side = await remoteAt(780, 360);
    expectAllInside(side, 780, 360);
    expectFits(side, 360);
    expectSixteenByNine(side);
    const offer = boxOf(side, ".cast-remote__offer");
    const pad = boxOf(side, ".cast-remote__pad");
    expect(pad.left).toBeGreaterThanOrEqual(offer.right);
    expect(pad.width).toBeCloseTo(240, 0);
    expect(boxOf(side, ".cast-remote__bar").bottom).toBeLessThanOrEqual(
      Math.min(offer.top, pad.top) + 0.5,
    );
    expect(padSize(side, "Play")).toBeCloseTo(72, 0);
    expect(padSize(side, "Back 10 s")).toBeCloseTo(56, 0);
    expect(padSize(side, "Volume up")).toBeCloseTo(56, 0);
  });

  it("lays a computer out as two columns, the pad's 22rem at the end, Play 96 px", async () => {
    const wide = await remoteAt(1280, 800);
    expectAllInside(wide, 1280, 800);
    expectFits(wide, 800);
    expectSixteenByNine(wide);
    const offer = boxOf(wide, ".cast-remote__offer");
    const pad = boxOf(wide, ".cast-remote__pad");
    expect(pad.left).toBeGreaterThanOrEqual(offer.right);
    expect(pad.width).toBeCloseTo(352, 0);
    expect(padSize(wide, "Play")).toBeCloseTo(96, 0);
  });

  it("keeps the card a third or more of the width on a short phone on its side, as a browser's bars leave it", async () => {
    // The bar, the timeline and the pad at its smaller sizes keep their heights; the card has what
    // they leave, and nothing scrolls.
    const short = await remoteAt(740, 300);
    expectSixteenByNine(short);
    expectFits(short, 300);
    const offer = boxOf(short, ".cast-remote__offer");
    expect(offer.width).toBeGreaterThanOrEqual(740 * 0.3);
    expect(Math.abs(offer.width / 16 - offer.height / 9) * 9).toBeLessThan(1);
  });

  it("fits a phone upright with its browser's bars, nothing to scroll to", async () => {
    const bars = await remoteAt(390, 660);
    expectAllInside(bars, 390, 660);
    expectFits(bars, 660);
    expectSixteenByNine(bars);
    expect(padSize(bars, "Play")).toBeCloseTo(72, 0);
    expect(boxOf(bars, ".cast-remote__offer").width).toBeCloseTo(390 - 32, 0);
  });
});
