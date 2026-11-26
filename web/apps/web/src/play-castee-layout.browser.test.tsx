// SPDX-License-Identifier: MPL-2.0

// The castee's Play page as a real browser lays it out (D555 §8): on a phone on its side and on a
// computer, the video's overlay over the castee's key frame, the larger play in it, no bar under it.

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";

import { CasteePlay } from "./play-castee.tsx";
import { castingRemote } from "./test-castee.tsx";
import { settleUntil, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  window.sessionStorage.clear();
});

/** The castee's Play page on film 4 in a window width × height. */
async function casteeAt(width: number, height: number): Promise<HTMLElement> {
  await page.viewport(width, height);
  const drawn = await castingRemote({}, [], { remote: CasteePlay, onPlayHere: vi.fn() });
  mounted = drawn.mounted;
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

/** Shows the overlay as a mouse moved over the page does (player-overlay-layout's shown). */
async function shown(container: HTMLElement): Promise<void> {
  for (const screenX of [5, 50]) {
    act(() => {
      document.body.dispatchEvent(
        new PointerEvent("pointermove", {
          bubbles: true,
          pointerType: "mouse",
          screenX,
          screenY: 5,
        }),
      );
    });
  }
  await expect
    .poll(() => container.querySelector(".player-overlay")?.getAttribute("data-shown"))
    .toBe("full");
}

/** That the overlay lies over the whole key frame, within the stage, its play 64 px, no bar. */
async function expectOverlayOverTheKeyFrame(container: HTMLElement, width: number): Promise<void> {
  await settleUntil(() => container.querySelector(".castee-play__stage .player-overlay") !== null);
  expect(container.querySelector(".castee-play__bar")).toBeNull();
  await shown(container);
  const stage = boxOf(container, ".castee-play__stage");
  const picture = boxOf(container, ".castee-play__picture");
  const overlay = boxOf(container, ".castee-play__stage .player-overlay");
  expect(stage.width).toBeGreaterThan(0);
  expect(stage.right).toBeLessThanOrEqual(width);
  for (const box of [picture, overlay]) {
    expect(box.left).toBeCloseTo(stage.left, 0);
    expect(box.top).toBeCloseTo(stage.top, 0);
    expect(box.width).toBeCloseTo(stage.width, 0);
    expect(box.height).toBeCloseTo(stage.height, 0);
  }
  const play = boxOf(container, '.castee-play__stage .player-overlay [aria-label="Play"]');
  expect(play.width).toBe(64);
  expect(play.top).toBeGreaterThanOrEqual(stage.top);
  expect(play.bottom).toBeLessThanOrEqual(stage.bottom);
}

describe("the castee's Play page laid out (D555 §8)", () => {
  it("lays the overlay over the castee's key frame on a phone on its side, no bar under it", async () => {
    await expectOverlayOverTheKeyFrame(await casteeAt(844, 390), 844);
  });

  it("lays the overlay over the castee's key frame on a computer, no bar under it", async () => {
    await expectOverlayOverTheKeyFrame(await casteeAt(1280, 800), 1280);
  });
});
