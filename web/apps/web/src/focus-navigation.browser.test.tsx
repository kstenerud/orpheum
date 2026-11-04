// SPDX-License-Identifier: MPL-2.0

/**
 * Moving focus over a page of text by a remote's keys in a real browser (focus-navigation.ts): the
 * text between two controls is scrolled through before the next control takes focus, so it is read
 * rather than jumped past.
 */

import "./tokens.css";
import "./base.css";

import type { CSSProperties } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A page of a control, three screens of text, and another control. */
const longPage = (
  <main>
    <button type="button">above</button>
    <p style={{ height: "1440px" }}>Three screens of text.</p>
    <button type="button">below</button>
  </main>
);

describe("an arrow over a page of text", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.scrollTo(0, 0);
  });

  it("scrolls through the text a step at a time before the control after it takes focus, both ways", async () => {
    await page.viewport(960, 480);
    mounted = await mount(withServices(longPage));
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement?.textContent).toBe("above");
    const scrolls: number[] = [];
    for (let press = 0; press < 8 && document.activeElement?.textContent === "above"; press += 1) {
      await userEvent.keyboard("{ArrowDown}");
      scrolls.push(window.scrollY);
    }
    expect(document.activeElement?.textContent).toBe("below");
    // Each press but the last moved the page on by less than a screen, as Page Down does.
    expect(scrolls.length).toBeGreaterThan(2);
    const steps = scrolls.slice(1, -1).map((at, index) => at - (scrolls[index] ?? 0));
    expect(steps.every((step) => step > 0 && step < window.innerHeight)).toBe(true);
    for (let press = 0; press < 8 && document.activeElement?.textContent === "below"; press += 1) {
      await userEvent.keyboard("{ArrowUp}");
    }
    expect(document.activeElement?.textContent).toBe("above");
    expect(window.scrollY).toBe(0);
  });
});

/** Where a control sits on the test's page, in pixels. */
function placedAt(left: number, top: number, width = 120): CSSProperties {
  return { position: "absolute", left, top, width, height: 40 };
}

/**
 * A field alone at the right of its line, a field and a button under it at the left, as Your
 * account's Activate a device lays them; and a grid of three over a short last row of one, with a
 * control up and to the right of that row, outside the grid.
 */
const offTheLinePage = (
  <main style={{ position: "relative", height: "460px" }}>
    <input aria-label="code" style={placedAt(600, 0, 200)} />
    <input aria-label="name" style={placedAt(0, 60, 200)} />
    <button type="button" style={placedAt(220, 60)}>
      activate
    </button>
    <ul data-focus-container="">
      {["one", "two", "three", "four"].map((name, index) => (
        <li key={name} style={placedAt((index % 3) * 140, 200 + Math.floor(index / 3) * 60)}>
          <a href={`#${name}`}>{name}</a>
        </li>
      ))}
    </ul>
    <button type="button" style={placedAt(600, 180)}>
      beside
    </button>
  </main>
);

describe("an arrow with nothing in line that way", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("goes to the nearest control off the line from a loose control, and stays at a grid's short last row", async () => {
    await page.viewport(960, 480);
    mounted = await mount(withServices(offTheLinePage));
    document.querySelector<HTMLElement>('[aria-label="code"]')?.focus();
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement?.textContent).toBe("activate");
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("name");
    document.querySelector<HTMLElement>('a[href="#four"]')?.focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(document.activeElement?.textContent).toBe("four");
  });
});
