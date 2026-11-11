// SPDX-License-Identifier: MPL-2.0

/**
 * A poster row's arrows, for a mouse with no sideways wheel (the owner, 2026-11-04), as a real
 * browser lays the row out and scrolls it: an arrow only where there is more its way, shown while
 * the pointer is over the row or focus is in it, and a press moving the row about its width less one
 * poster, onto a poster's edge.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";

import { EntryPoster, type Entry } from "./browse.tsx";
import { PosterRow } from "./poster-row.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A film as a list sends it. */
function film(id: number): Entry {
  return {
    id,
    name: `Film ${String(id)}`,
    kind: "film",
    identificationState: "identified",
    addedAt: id,
  };
}

/** A row of count films, 600 pixels wide, with somewhere below it for the pointer to rest. */
async function rowOf(count: number): Promise<Mounted> {
  await page.viewport(1000, 800);
  const films = Array.from({ length: count }, (_, index) => film(index + 1));
  const mounted = await mount(
    withServices(
      <div style={{ width: "600px" }}>
        <PosterRow>
          {films.map((entry) => (
            <EntryPoster key={entry.id} kind="film" entry={entry} />
          ))}
        </PosterRow>
        <div className="elsewhere" style={{ height: "100px" }} />
      </div>,
    ),
  );
  // The row's own observers update it as the browser scrolls and lays it out, outside any act(), as
  // test-walk.tsx's whole client is.
  Reflect.set(globalThis, "IS_REACT_ACT_ENVIRONMENT", false);
  return mounted;
}

/** The element selector names, which the test needs drawn. */
function drawnAt(selector: string): HTMLElement {
  const element = document.querySelector<HTMLElement>(selector);
  if (element === null) {
    throw new Error(`nothing drawn at ${selector}`);
  }
  return element;
}

/** The arrow named name, where one is drawn and shown. */
function shownArrow(name: string): HTMLButtonElement | undefined {
  const button = document.querySelector<HTMLButtonElement>(`button[aria-label="${name}"]`);
  return button?.checkVisibility() === true ? button : undefined;
}

/** Which arrows show: the left's and the right's. */
function arrowsShown(): string[] {
  return ["Scroll left", "Scroll right"].filter((name) => shownArrow(name) !== undefined);
}

/** How far the poster nearest the row's start stands from where the row's snap puts one. */
function offsetFromAPosterEdge(list: HTMLElement): number {
  const start =
    list.getBoundingClientRect().left + parseFloat(getComputedStyle(list).scrollPaddingLeft);
  return Math.min(
    ...[...list.children].map((poster) => Math.abs(poster.getBoundingClientRect().left - start)),
  );
}

describe("a poster row's arrows, for a mouse", () => {
  let mounted: Mounted | undefined;
  afterEach(async () => {
    // The pointer left off the row, not hovering what the next test draws there.
    const elsewhere = document.querySelector(".elsewhere");
    if (elsewhere !== null) {
      await userEvent.hover(elsewhere);
    }
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows none on a row that fits, the pointer over it", async () => {
    mounted = await rowOf(2);
    await userEvent.hover(drawnAt(".poster-row"));
    // Given the row's observers their time to measure, as the next test waits for them to.
    await new Promise((resolve) => requestAnimationFrame(resolve));
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(arrowsShown()).toEqual([]);
  });

  it("shows the way there is more only while the pointer is over the row or focus is in it", async () => {
    mounted = await rowOf(20);
    await userEvent.hover(drawnAt(".elsewhere"));
    // Drawn, as the row has more to the right, but not shown with the pointer elsewhere.
    await expect
      .poll(() => document.querySelector('button[aria-label="Scroll right"]'))
      .not.toBeNull();
    expect(arrowsShown()).toEqual([]);
    await userEvent.hover(drawnAt(".poster-row"));
    await expect.poll(arrowsShown).toEqual(["Scroll right"]);
    await userEvent.hover(drawnAt(".elsewhere"));
    await expect.poll(arrowsShown).toEqual([]);
    drawnAt(".poster-row a").focus();
    await expect.poll(arrowsShown).toEqual(["Scroll right"]);
    // Out of Tab's order and a remote's.
    expect(shownArrow("Scroll right")?.tabIndex).toBe(-1);
  });

  it("moves the row about its width less one poster onto a poster's edge, and back", async () => {
    mounted = await rowOf(20);
    const list = drawnAt(".poster-row");
    await userEvent.hover(list);
    await expect.poll(arrowsShown).toEqual(["Scroll right"]);
    const posterWidth = drawnAt(".poster-row > li").getBoundingClientRect().width;
    await userEvent.click(shownArrow("Scroll right") ?? list);
    await expect.poll(arrowsShown).toEqual(["Scroll left", "Scroll right"]);
    // Under way first: at its start the row is on a poster's edge too, the first's.
    await expect.poll(() => list.scrollLeft).toBeGreaterThan(list.clientWidth / 2);
    await expect.poll(() => offsetFromAPosterEdge(list)).toBeLessThan(2);
    // About a row's width less one poster: past half the row, short of the whole of it.
    expect(list.scrollLeft).toBeGreaterThan(list.clientWidth / 2);
    expect(list.scrollLeft).toBeLessThan(list.clientWidth - posterWidth / 2);
    // Focus where it was: a press is a pointer's alone.
    expect(document.activeElement?.closest(".poster-row__arrow")).toBeNull();
    await userEvent.click(shownArrow("Scroll left") ?? list);
    await expect.poll(() => list.scrollLeft).toBe(0);
    await expect.poll(arrowsShown).toEqual(["Scroll right"]);
  });

  it("shows only the left arrow at the row's end", async () => {
    mounted = await rowOf(20);
    const list = drawnAt(".poster-row");
    await userEvent.hover(list);
    list.scrollLeft = list.scrollWidth;
    await expect.poll(arrowsShown).toEqual(["Scroll left"]);
  });
});

describe("a poster row on a device with no mouse (web architecture audit, 2026-11-10)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.unstubAllGlobals();
  });

  it("measures its ends for no arrow, and does once a mouse is its pointer", async () => {
    const mouse = handMouseQuery();
    mounted = await rowOf(20);
    // Given the row its time to measure, as a mouse's row is.
    await new Promise((resolve) => requestAnimationFrame(resolve));
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(document.querySelector('button[aria-label="Scroll right"]')).toBeNull();
    mouse.attach();
    await expect
      .poll(() => document.querySelector('button[aria-label="Scroll right"]'))
      .not.toBeNull();
  });
});

/**
 * The page's answer to a mouse's media query, (hover: hover) and (pointer: fine), no until the test
 * attaches one; every other query the browser's own.
 */
function handMouseQuery(): { readonly attach: () => void } {
  const ownMatchMedia = window.matchMedia.bind(window);
  const listeners = new Set<() => void>();
  let isAttached = false;
  const query = {
    addEventListener: (_type: string, listener: () => void) => {
      listeners.add(listener);
    },
    removeEventListener: (_type: string, listener: () => void) => {
      listeners.delete(listener);
    },
  };
  // MediaQueryList's own name, which the code under test reads.
  Object.defineProperty(query, "matches", { get: () => isAttached });
  vi.stubGlobal("matchMedia", (text: string) =>
    text === "(hover: hover) and (pointer: fine)" ? query : ownMatchMedia(text),
  );
  return {
    attach: () => {
      isAttached = true;
      for (const listener of listeners) {
        listener();
      }
    },
  };
}

describe("a poster row's Tab stop (the owner, 2026-11-11: one Tab stop per poster grid and row)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("is one: Tab leaves the row, and comes back to the poster last focused", async () => {
    mounted = await rowOf(8);
    const list = drawnAt(".poster-row");
    const links = [...list.querySelectorAll<HTMLElement>(".poster-card__link")];
    links[1]?.focus();
    await userEvent.keyboard("{Tab}");
    expect(list.contains(document.activeElement)).toBe(false);
    await userEvent.keyboard("{Shift>}{Tab}{/Shift}");
    expect(document.activeElement).toBe(links[1]);
    expect(links.filter((link) => link.tabIndex >= 0)).toHaveLength(1);
  });
});
