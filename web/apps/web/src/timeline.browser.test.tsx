// SPDX-License-Identifier: MPL-2.0

/**
 * The scrub bar's time under a mouse's pointer as a real browser lays it out (timeline.css): above
 * the slider, so the pointer does not hide it, and centred on the pointer (the UI revamp's first
 * wave, item 10).
 */

import "./tokens.css";
import "./base.css";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { mount, type Mounted } from "./test-dom.tsx";
import { Timeline } from "./timeline.tsx";

/** How far layout's rounding may put a box, in px. */
const roundingPx = 2;

describe("the time under a mouse's pointer on the scrub bar", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("stands above the slider, centred where the pointer is", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(
      <div style={{ padding: "4rem 2rem", width: "40rem" }}>
        <Timeline label="Where" atMs={0} durationMs={600_000} onSeek={() => undefined} />
      </div>,
    );
    const range = mounted.container.querySelector<HTMLInputElement>(".timeline__range");
    if (range === null) {
      throw new Error("no slider drawn");
    }
    const slider = range.getBoundingClientRect();
    const pointerX = slider.width / 4;
    await userEvent.hover(range, { position: { x: pointerX, y: slider.height / 2 } });
    const pointed = (): HTMLElement | null =>
      mounted?.container.querySelector<HTMLElement>(".timeline__pointed") ?? null;
    await expect.poll(() => pointed() !== null).toBe(true);
    const label = pointed()?.getBoundingClientRect() ?? new DOMRect();
    expect(label.bottom).toBeLessThanOrEqual(slider.top + roundingPx);
    expect(Math.abs(label.left + label.width / 2 - (slider.left + pointerX))).toBeLessThan(
      label.width / 4,
    );
  });
});
