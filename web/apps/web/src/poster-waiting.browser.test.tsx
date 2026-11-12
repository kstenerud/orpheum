// SPDX-License-Identifier: MPL-2.0

/**
 * A poster still to come, as a real browser draws it (the owner, 2026-11-11): a faded clock that
 * does not move, centred on a poster-shaped card of the card's own ground, and the image laid over
 * it with no ground of its own, so the clock shows until the image is drawn.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Poster } from "./poster.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

describe("a poster still to come, laid out by a real browser", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("is a faded clock 30% of the poster wide, centred on a poster-shaped card, under an image with no ground", async () => {
    mounted = await mount(
      <div style={{ width: "240px" }}>
        <Poster sourceSha256="ab12" isTile />
      </div>,
    );
    const waiting = mounted.container.querySelector<HTMLElement>(".poster-waiting");
    const clock = waiting?.querySelector<SVGElement>(".poster__clock");
    const image = mounted.container.querySelector<HTMLImageElement>(".poster-waiting + img.poster");
    if (!waiting || !clock || !image) {
      throw new Error("no waiting poster drawn");
    }

    const card = waiting.getBoundingClientRect();
    expect(card.width).toBe(240);
    expect(card.height).toBeCloseTo(360, 0);
    const style = getComputedStyle(waiting);
    expect(style.backgroundColor).not.toBe("rgba(0, 0, 0, 0)");
    expect(style.boxShadow).not.toBe("none");

    const mark = clock.getBoundingClientRect();
    expect(mark.width).toBeCloseTo(72, 0);
    expect(getComputedStyle(clock).opacity).toBe("0.3");
    // Centred both ways in the card.
    expect(mark.left - card.left).toBeCloseTo(card.right - mark.right, 0);
    expect(mark.top - card.top).toBeCloseTo(card.bottom - mark.bottom, 0);

    const ground = getComputedStyle(image);
    expect(ground.backgroundColor).toBe("rgba(0, 0, 0, 0)");
    expect(ground.backgroundImage).toBe("none");
  });
});
