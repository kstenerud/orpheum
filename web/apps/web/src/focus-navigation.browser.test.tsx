// SPDX-License-Identifier: MPL-2.0

/**
 * Moving focus over a page of text by a remote's keys in a real browser (focus-navigation.ts): the
 * text between two controls is scrolled through before the next control takes focus, so it is read
 * rather than jumped past.
 */

import "./tokens.css";
import "./base.css";

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
