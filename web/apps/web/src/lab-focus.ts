// SPDX-License-Identifier: MPL-2.0

/**
 * The input probe's focus engine (plans/tv-device-tests.md): which element an arrow key moves real
 * DOM focus to, behind a narrow interface, so another engine (@bbc/tv-lrud-spatial, should the owner
 * take it, controller-input-2026-10-25.md §6.1) drops in beside ours and is measured the same way.
 * Ours is a minimal geometric search: the nearest element whose box lies in the arrow's direction,
 * nearness along the arrow counting less than the step across it.
 */

import { keyCodeOf } from "./lab-keys.ts";

/** Where an arrow moves focus. */
export type Direction = "up" | "down" | "left" | "right";

/** An element's box on the screen, as getBoundingClientRect gives it. */
export interface Box {
  readonly left: number;
  readonly top: number;
  readonly right: number;
  readonly bottom: number;
}

/** A candidate for focus: the thing itself and its box. */
export interface Placed<T> {
  readonly item: T;
  readonly box: Box;
}

/**
 * How much a step across the arrow's direction counts against a candidate, beside one along it: a
 * tile straight below is chosen over a nearer one below and to the side. A justified constant, the
 * usual weighting of geometric searches (Jellyfin's, the W3C spatial-navigation draft's); the probe
 * measures it, nothing depends on it.
 */
const acrossWeight = 2;

/** The gap between from and to along direction, negative where to is not past from that way. */
function gapAlong(from: Box, to: Box, direction: Direction): number {
  switch (direction) {
    case "up":
      return from.top - to.bottom;
    case "down":
      return to.top - from.bottom;
    case "left":
      return from.left - to.right;
    case "right":
      return to.left - from.right;
  }
}

/** How far to lies across direction from from: 0 where they overlap, else the gap between them. */
function gapAcross(from: Box, to: Box, direction: Direction): number {
  const isVertical = direction === "up" || direction === "down";
  const [fromStart, fromEnd, toStart, toEnd] = isVertical
    ? [from.left, from.right, to.left, to.right]
    : [from.top, from.bottom, to.top, to.bottom];
  if (toEnd < fromStart) {
    return fromStart - toEnd;
  }
  return toStart > fromEnd ? toStart - fromEnd : 0;
}

/**
 * The candidate an arrow in direction moves to from the box from: the nearest whose box lies wholly
 * past from's edge that way (a box may touch it), by the gap along plus acrossWeight times the gap
 * across; undefined where none lies that way.
 */
export function nearestInDirection<T>(
  from: Box,
  candidates: readonly Placed<T>[],
  direction: Direction,
): T | undefined {
  let best: { readonly item: T; readonly score: number } | undefined;
  for (const candidate of candidates) {
    const along = gapAlong(from, candidate.box, direction);
    if (along < 0) {
      continue;
    }
    const score = along + acrossWeight * gapAcross(from, candidate.box, direction);
    if (best === undefined || score < best.score) {
      best = { item: candidate.item, score };
    }
  }
  return best?.item;
}

/** The directions the arrow keys name, by key: the standard names, then old Edge's and IE's. */
const directionsByKey = new Map<string, Direction>([
  ["ArrowUp", "up"],
  ["ArrowDown", "down"],
  ["ArrowLeft", "left"],
  ["ArrowRight", "right"],
  ["Up", "up"],
  ["Down", "down"],
  ["Left", "left"],
  ["Right", "right"],
]);

/** The directions the arrow keys name by keyCode, which an old TV browser alone gives. */
const directionsByKeyCode = new Map<number, Direction>([
  [37, "left"],
  [38, "up"],
  [39, "right"],
  [40, "down"],
]);

/** The direction an arrow key names, by its key or, on an old TV browser, its keyCode. */
export function directionOfKey(key: string, keyCode: number): Direction | undefined {
  return directionsByKey.get(key) ?? directionsByKeyCode.get(keyCode);
}

/**
 * A focus engine: given the focused element, an arrow and the region it moves within, the element
 * focus moves to, or undefined to leave it where it is.
 */
export interface FocusEngine {
  readonly name: string;
  readonly next: (from: Element, direction: Direction, region: Element) => HTMLElement | undefined;
}

/** What may take focus inside a region: its links, buttons and anything with a tabindex. */
const focusableSelector = "a[href], button:not([disabled]), input, select, [tabindex]";

/** Our geometric engine over the region's focusable elements. */
export const geometricFocus: FocusEngine = {
  name: "geometric",
  next: (from, direction, region) => {
    const candidates: Placed<HTMLElement>[] = [];
    for (const element of region.querySelectorAll<HTMLElement>(focusableSelector)) {
      if (element !== from && element.tabIndex >= 0) {
        candidates.push({ item: element, box: element.getBoundingClientRect() });
      }
    }
    return nearestInDirection(from.getBoundingClientRect(), candidates, direction);
  },
};

/**
 * Moves real DOM focus by engine on each arrow key pressed within region, the browser's own handling
 * of the key prevented, so the engine alone moves it; the input probe measures each move by the
 * focus it sees arrive. Returns the stopping.
 */
export function driveFocus(region: HTMLElement, engine: FocusEngine): () => void {
  const handle = (event: KeyboardEvent): void => {
    const direction = directionOfKey(event.key, keyCodeOf(event));
    const from = document.activeElement;
    if (direction === undefined || from === null || !region.contains(from)) {
      return;
    }
    event.preventDefault();
    engine.next(from, direction, region)?.focus();
  };
  region.addEventListener("keydown", handle);
  return () => {
    region.removeEventListener("keydown", handle);
  };
}
