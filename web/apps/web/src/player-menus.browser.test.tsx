// SPDX-License-Identifier: MPL-2.0

// The overlay's pop-ups laid out by a real browser (D408 §2): what they hold decides their size.

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import type { PlayerSound } from "./player-controls.tsx";
import { PlayerMenus } from "./player-menus.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A film's sound at half, which the test never sets. */
const sound: PlayerSound = {
  volume: 0.5,
  isMuted: false,
  canSetVolume: true,
  setVolume: () => undefined,
  setMuted: () => undefined,
};

/** How far layout's rounding may put a box. */
const roundingPx = 1;

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("the overlay's pop-ups, laid out by a real browser", () => {
  // The owner, 2026-11-02: "The volume control popup is far too wide."
  it("draws 🔈's pop-up as wide as its slider and mute, with no Close", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(
      withServices(
        <div style={{ position: "fixed", right: "16px", bottom: "16px" }}>
          <PlayerMenus
            videoRef={createRef()}
            shown={["sound"]}
            sound={sound}
            onOpenChange={() => undefined}
          />
        </div>,
      ),
    );
    await userEvent.click(page.getByRole("button", { name: "Volume" }));
    const popUp = document.querySelector("dialog");
    const controls = popUp?.querySelector(".player-controls__sound") ?? null;
    const [slider, mute] = [
      controls?.querySelector(".volume-range") ?? null,
      controls?.querySelector("button") ?? null,
    ];
    expect(popUp?.querySelector('button[aria-label="Close"]')).toBeNull();
    // The pop-up no wider than what it holds, and that no wider than the slider standing upright or
    // mute under it.
    expect(boxOf(popUp).width).toBeLessThanOrEqual(
      boxOf(controls).width + sidesOf(popUp, "border") + sidesOf(popUp, "padding") + roundingPx,
    );
    expect(boxOf(controls).width).toBeLessThanOrEqual(
      Math.max(
        boxOf(slider).width + sidesOf(slider, "margin"),
        boxOf(mute).width + sidesOf(mute, "margin"),
      ) +
        sidesOf(controls, "padding") +
        roundingPx,
    );
    // The slider upright, and a fingertip wide, as mute is: no wider than its button.
    expect(boxOf(slider).width).toBeLessThan(boxOf(slider).height);
    expect(boxOf(slider).width).toBeLessThanOrEqual(boxOf(mute).width + roundingPx);
  });
});

/** An element's box, or a failure where it is not drawn. */
function boxOf(element: Element | null | undefined): DOMRect {
  if (element === null || element === undefined) {
    throw new Error("an element the test measures is not drawn");
  }
  return element.getBoundingClientRect();
}

/** An element's border, padding or margin on its left and right together, in pixels. */
function sidesOf(element: Element | null, edge: "border" | "padding" | "margin"): number {
  if (element === null) {
    throw new Error("an element the test measures is not drawn");
  }
  const its = getComputedStyle(element);
  const sides = {
    border: [its.borderLeftWidth, its.borderRightWidth],
    padding: [its.paddingLeft, its.paddingRight],
    margin: [its.marginLeft, its.marginRight],
  } as const;
  const [left, right] = sides[edge];
  return Number.parseFloat(left) + Number.parseFloat(right);
}
