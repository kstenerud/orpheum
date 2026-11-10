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

/**
 * A shell's banner across the top, its place straight above a page's list of two posters, with the
 * list's tools at the right of the line above the list, as Films, Series and Libraries lay them over
 * a list shorter than a row.
 */
const shortListPage = (
  <div style={{ position: "relative", height: "460px" }}>
    <header data-focus-container="">
      <a href="#home" style={placedAt(0, 0)}>
        home
      </a>
    </header>
    <main>
      <div data-focus-container="">
        <button type="button" style={placedAt(600, 80)}>
          search
        </button>
      </div>
      <ul data-focus-container="">
        {["one", "two"].map((name, index) => (
          <li key={name} style={placedAt(index * 140, 160)}>
            <a href={`#${name}`}>{name}</a>
          </li>
        ))}
      </ul>
    </main>
  </div>
);

describe("Up from a page's content with nothing in it straight above", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("goes to what is above in the page before the banner straight above, then the banner", async () => {
    await page.viewport(960, 480);
    mounted = await mount(withServices(shortListPage));
    document.querySelector<HTMLElement>('a[href="#one"]')?.focus();
    await userEvent.keyboard("{ArrowUp}");
    expect(document.activeElement?.textContent).toBe("search");
    await userEvent.keyboard("{ArrowUp}");
    expect(document.activeElement?.textContent).toBe("home");
  });
});

/**
 * A grid of three hundred tiles, ten to a row, one hidden tile in the row below the first, and a
 * button at the right of the grid's first row, outside it.
 */
const largeGridPage = (
  <main style={{ position: "relative", height: "3200px" }}>
    <ul data-focus-container="">
      {Array.from({ length: 300 }, (_, index) => (
        <li key={index} style={placedAt((index % 10) * 70, Math.floor(index / 10) * 60, 60)}>
          <a href={`#tile-${String(index)}`} style={index === 10 ? { visibility: "hidden" } : {}}>
            {`tile ${String(index)}`}
          </a>
        </li>
      ))}
    </ul>
    <button type="button" style={placedAt(800, 0)}>
      beside
    </button>
  </main>
);

/** How many times each of an element's costly readings ran while act did. */
async function readingsDuring(act: () => Promise<void>): Promise<{ boxes: number; drawn: number }> {
  const counted = { boxes: 0, drawn: 0 };
  const box = Element.prototype.getBoundingClientRect;
  const rects = Element.prototype.getClientRects;
  Element.prototype.getBoundingClientRect = function countedBox(this: Element) {
    counted.boxes += 1;
    return box.call(this);
  };
  Element.prototype.getClientRects = function countedRects(this: Element) {
    counted.drawn += 1;
    return rects.call(this);
  };
  try {
    await act();
  } finally {
    Element.prototype.getBoundingClientRect = box;
    Element.prototype.getClientRects = rects;
  }
  return counted;
}

describe("an arrow over a large grid", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  // A press on a TV reads every tile's box, a thousand once a library is scrolled: whether a tile can
  // take focus is asked of the winner alone, and the grid is not read again when the search widens.
  it("asks whether a tile can take focus only of the nearest, and reads the grid once as it widens", async () => {
    await page.viewport(960, 480);
    mounted = await mount(withServices(largeGridPage));
    document.querySelector<HTMLElement>('a[href="#tile-0"]')?.focus();
    const down = await readingsDuring(() => userEvent.keyboard("{ArrowDown}"));
    // The hidden tile straight below is passed over for the next drawn one.
    expect(document.activeElement?.textContent).toBe("tile 20");
    expect(down.drawn).toBeLessThan(20);
    document.querySelector<HTMLElement>('a[href="#tile-9"]')?.focus();
    const right = await readingsDuring(() => userEvent.keyboard("{ArrowRight}"));
    expect(document.activeElement?.textContent).toBe("beside");
    expect(right.boxes).toBeLessThan(400);
  });
});
