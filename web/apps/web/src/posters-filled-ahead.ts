// SPDX-License-Identifier: MPL-2.0

import { useEffect, useRef, type RefObject } from "react";

import { tileArtAhead } from "./poster.tsx";

/**
 * How many posters either side of the one at hand are filled in ahead (the owner, 2026-11-11: "the
 * 100 items before and 100 items after are filled (giving a good buffer of scroll room)"). Those
 * further off are let go, so a long list holds at most twice this many images.
 */
export const postersFilledEachWay = 100;

/**
 * How many posters are asked for ahead at once: a few, at a low priority, so the posters in view
 * and a press's own work come first, and the rest of the browser's connections stay free for them.
 */
const fillsAtOnce = 2;

/** How long the page stays put after a scroll before where it stopped moves the window. */
const scrollSettleMs = 150;

/** Which way, up or down the grid, the person last moved; undefined while they have not. */
export type Moved = "down" | "up" | undefined;

/**
 * The places to fill around anchor in a grid of count posters, columns a row, nearest first (the
 * owner, 2026-11-11): "Fill the next 100 posters in each direction, filling first in the vertical
 * direction they last moved, or if they haven't moved, alternate 1 row down, 1 row up, until we fill
 * both windows." The anchor itself is in view, and shows itself.
 */
export function fillOrder(around: Around, moved: Moved): readonly number[] {
  const { anchor, columns, count } = around;
  const after: number[] = [];
  for (
    let place = anchor + 1;
    place < Math.min(count, anchor + 1 + postersFilledEachWay);
    place++
  ) {
    after.push(place);
  }
  const before: number[] = [];
  for (let place = anchor - 1; place >= Math.max(0, anchor - postersFilledEachWay); place--) {
    before.push(place);
  }
  if (moved === "down") {
    return [...after, ...before];
  }
  if (moved === "up") {
    return [...before, ...after];
  }
  // Not moved: row by row, the anchor's own row first, then one down, one up, two down, two up.
  const across = Math.max(1, columns);
  const anchorRow = Math.floor(anchor / across);
  const rowsAway = (place: number): number => Math.floor(place / across) - anchorRow;
  return [...after, ...before].sort((one, other) => {
    const oneAway = rowsAway(one);
    const otherAway = rowsAway(other);
    if (Math.abs(oneAway) !== Math.abs(otherAway)) {
      return Math.abs(oneAway) - Math.abs(otherAway);
    }
    if (oneAway !== otherAway) {
      return otherAway - oneAway; // down before up
    }
    return one - other;
  });
}

/** Where in a grid the fill is centred: the anchor's place, the grid's columns and its count. */
export interface Around {
  readonly anchor: number;
  readonly columns: number;
  readonly count: number;
}

/** What a poster's card shows as its art, to fill in ahead; undefined for a card with none. */
export interface ArtAhead {
  readonly sourceSha256: string;
  readonly kind: "poster" | "still";
}

/**
 * Fills in the posters of the grid at gridRef ahead of their being scrolled to, so moving through a
 * list does not wait on them (the owner, 2026-11-11: "continue filling out posters for offscreen
 * items so that it's not fetching on navigation"): the window of postersFilledEachWay either side of
 * the poster focused, or of the one in the middle of the view after a scroll, in fillOrder's order,
 * a few at a time at a low priority. The images are kept while their posters stay in the window, so
 * the browser keeps them at hand, and let go once the window moves past them. arts names the art of
 * the poster at each place, and changes as the list does: the images still in the window are kept.
 */
export function usePostersFilledAhead(
  gridRef: RefObject<HTMLElement | null>,
  arts: readonly (ArtAhead | undefined)[],
): void {
  const anchorRef = useRef<{ place: number; moved: Moved }>({ place: 0, moved: undefined });
  // Kept across the list's changes, as a poll's, and let go when the grid goes.
  const heldRef = useRef(new Map<string, Held>());
  // The window's latest filling, which an image asked for under an earlier list calls as it ends.
  const fillRef = useRef(doNothing);
  useEffect(() => {
    const held = heldRef.current;
    return () => {
      for (const kept of held.values()) {
        letGo(kept);
      }
      held.clear();
    };
  }, []);
  useEffect(() => {
    const grid = gridRef.current;
    if (grid === null) {
      return undefined;
    }
    const fill = (): void => {
      const { place, moved } = anchorRef.current;
      const around = { anchor: place, columns: columnsOf(grid), count: arts.length };
      fillWindow(heldRef.current, artsInOrder(arts, fillOrder(around, moved)), () => {
        fillRef.current();
      });
    };
    const stop = watchAnchor(grid, (place) => {
      const before = anchorRef.current;
      const columns = columnsOf(grid);
      const rows = { now: Math.floor(place / columns), before: Math.floor(before.place / columns) };
      anchorRef.current = { place, moved: movedBetween(rows, before.moved) };
      fill();
    });
    fillRef.current = fill;
    fill();
    return () => {
      fillRef.current = doNothing;
      stop();
    };
  }, [gridRef, arts]);
}

/** The art to fill, once each, in order's places of arts. */
function artsInOrder(
  arts: readonly (ArtAhead | undefined)[],
  order: readonly number[],
): Map<string, ArtAhead> {
  const wanted = new Map<string, ArtAhead>();
  for (const each of order) {
    const art = arts[each];
    if (art !== undefined && !wanted.has(keyOf(art))) {
      wanted.set(keyOf(art), art);
    }
  }
  return wanted;
}

/** Which art an image ahead holds. */
function keyOf(art: ArtAhead): string {
  return `${art.kind}:${art.sourceSha256}`;
}

/**
 * Brings held to wanted: lets go of the images no longer wanted, and asks for the next wanted, in
 * its order, while fewer than fillsAtOnce are loading; done is called as each asked for ends.
 */
function fillWindow(
  held: Map<string, Held>,
  wanted: Map<string, ArtAhead>,
  done: () => void,
): void {
  for (const [key, kept] of held) {
    if (!wanted.has(key)) {
      held.delete(key);
      letGo(kept);
    }
  }
  let loading = [...held.values()].filter((kept) => !kept.isDone).length;
  for (const [key, art] of wanted) {
    if (loading >= fillsAtOnce) {
      break;
    }
    if (held.has(key)) {
      continue;
    }
    const kept: Held = {
      image: tileArtAhead(art.sourceSha256, art.kind, () => {
        kept.isDone = true;
        done();
      }),
      isDone: false,
    };
    held.set(key, kept);
    loading += 1;
  }
}

/** Which way the anchor moved from the row before to the row now; as before within one row. */
function movedBetween(rows: { now: number; before: number }, before: Moved): Moved {
  if (rows.now === rows.before) {
    return before;
  }
  return rows.now > rows.before ? "down" : "up";
}

/**
 * Calls moveTo with the place of the poster the window centres on: the one that takes focus, or,
 * once a scroll settles, the one in the middle of the view unless the focused poster is still in it.
 * The returned stop ends the watch.
 */
function watchAnchor(grid: HTMLElement, moveTo: (place: number) => void): () => void {
  const moveWithin = (place: number): void => {
    if (place >= 0) {
      moveTo(place);
    }
  };
  const focused = (event: FocusEvent): void => {
    moveWithin(placeOf(grid, event.target));
  };
  let settling: ReturnType<typeof setTimeout> | undefined;
  const scrolled = (): void => {
    clearTimeout(settling);
    settling = setTimeout(() => {
      // The focused poster is the anchor while it is in view; a mouse's scroll can leave it.
      const active = grid.ownerDocument.activeElement;
      if (active !== null && grid.contains(active) && isInView(active)) {
        return;
      }
      moveWithin(placeOf(grid, middleOfView(grid)));
    }, scrollSettleMs);
  };
  grid.addEventListener("focusin", focused);
  window.addEventListener("scroll", scrolled, { passive: true });
  return () => {
    clearTimeout(settling);
    grid.removeEventListener("focusin", focused);
    window.removeEventListener("scroll", scrolled);
  };
}

function doNothing(): void {
  // No grid to fill: it has gone, or not yet drawn.
}

/** An image asked for ahead, and whether it has loaded or failed. */
interface Held {
  readonly image: HTMLImageElement;
  isDone: boolean;
}

/** Lets an image asked for ahead go, stopping its load if still under way. */
function letGo(kept: Held): void {
  const { image, isDone } = kept;
  image.onload = null;
  image.onerror = null;
  if (!isDone) {
    image.src = "";
  }
}

/** How many posters a row of the grid holds, by its laid-out columns; one where it has none. */
function columnsOf(grid: HTMLElement): number {
  const columns = getComputedStyle(grid).gridTemplateColumns.split(" ").filter(Boolean).length;
  return Math.max(1, columns);
}

/** The place in grid of the poster holding target, -1 for none. */
function placeOf(grid: HTMLElement, target: EventTarget | Element | null): number {
  const item = target instanceof Element ? target.closest("li") : null;
  return item?.parentElement === grid ? [...grid.children].indexOf(item) : -1;
}

/** Whether any of element is in the window's view. */
function isInView(element: Element): boolean {
  const box = element.getBoundingClientRect();
  return box.bottom > 0 && box.top < window.innerHeight;
}

/** What stands in the middle of the view, across the middle of grid. */
function middleOfView(grid: HTMLElement): Element | null {
  const box = grid.getBoundingClientRect();
  return grid.ownerDocument.elementFromPoint(box.left + box.width / 2, window.innerHeight / 2);
}
