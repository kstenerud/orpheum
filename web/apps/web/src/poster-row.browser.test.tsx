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

import { afterEach, describe, expect, it } from "vitest";
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
    kind: "movie",
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
            <EntryPoster key={entry.id} kind="movie" entry={entry} />
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
