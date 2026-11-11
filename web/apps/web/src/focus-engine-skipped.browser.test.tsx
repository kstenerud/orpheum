// SPDX-License-Identifier: MPL-2.0

/**
 * An arrow's search over a grid whose items off screen the browser skips laying out
 * (content-visibility: auto), as a real browser skips them: it reads no box inside a skipped item,
 * each such read laying the page out again (the Fire TV's trace, 2026-11-11), and still moves to
 * the tile below.
 */

import { afterEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";

import { focusContainerAttribute, geometricFocus, nextInContainers } from "./focus-engine.ts";

/** A grid of rows of four links, each in an item the browser skips while it is off screen. */
function gridOf(rows: number): HTMLElement {
  const grid = document.createElement("ul");
  grid.setAttribute(focusContainerAttribute, "");
  grid.style.cssText =
    "display: grid; grid-template-columns: repeat(4, 100px); gap: 12px; margin: 0; padding: 0; list-style: none";
  for (let index = 0; index < rows * 4; index += 1) {
    const item = document.createElement("li");
    item.style.cssText = "content-visibility: auto; contain-intrinsic-size: auto 200px";
    const link = document.createElement("a");
    link.href = `#tile-${String(index)}`;
    link.textContent = `Tile ${String(index)}`;
    link.style.cssText = "display: block; height: 180px";
    item.append(link);
    grid.append(item);
  }
  document.body.append(grid);
  return grid;
}

/** Two animation frames: the browser has decided which items it skips. */
async function framesPassed(): Promise<void> {
  for (let frame = 0; frame < 2; frame += 1) {
    await new Promise((resolve) => requestAnimationFrame(resolve));
  }
}

afterEach(() => {
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

describe("an arrow's search over items the browser skips (content-visibility: auto)", () => {
  it("reads no box inside a skipped item, and moves to the tile below", async () => {
    await page.viewport(960, 480);
    const grid = gridOf(30);
    await framesPassed();
    const links = [...grid.querySelectorAll("a")];
    const skipped = links.filter((link) => !link.checkVisibility({ contentVisibilityAuto: true }));
    expect(skipped.length).toBeGreaterThan(40);
    const from = links[1];
    if (from === undefined) {
      throw new Error("no tile");
    }
    from.focus();
    const read: Element[] = [];
    const original = Element.prototype.getBoundingClientRect;
    vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (
      this: Element,
    ) {
      read.push(this);
      return original.call(this);
    });
    const next = nextInContainers(from, "down", { root: document.body, engine: geometricFocus });
    expect(next?.textContent).toBe("Tile 5");
    expect(read.filter((element) => skipped.some((link) => link === element))).toEqual([]);
  });
});
