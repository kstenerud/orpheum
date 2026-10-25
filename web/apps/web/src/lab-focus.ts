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

/**
 * Our geometric engine over the region's focusable elements that are laid out: one inside a closed
 * menu or a closed details has no box, and would otherwise be found at the screen's corner.
 */
export const geometricFocus: FocusEngine = {
  name: "geometric",
  next: (from, direction, region) => {
    const candidates: Placed<HTMLElement>[] = [];
    for (const element of region.querySelectorAll<HTMLElement>(focusableSelector)) {
      if (element !== from && element.tabIndex >= 0 && element.getClientRects().length > 0) {
        candidates.push({ item: element, box: element.getBoundingClientRect() });
      }
    }
    return nearestInDirection(from.getBoundingClientRect(), candidates, direction);
  },
};

/** Where a text field's caret stands, as an arrow reads it. */
export interface Caret {
  /** Whether the field is one line (an input) rather than several (a textarea). */
  readonly isSingleLine: boolean;
  /** The selection's start and end, or undefined where the field has none (a number input). */
  readonly selectionStart: number | undefined;
  readonly selectionEnd: number | undefined;
  readonly valueLength: number;
}

/**
 * Whether an arrow in direction is the text field's own, to move its caret, rather than ours, to
 * move focus out (the usual TV practice, the owner, 2026-10-25): Left and Up are ours at the value's
 * start, Right and Down at its end, with nothing selected; Up and Down always leave a one-line field.
 */
export function isArrowTheFields(direction: Direction, caret: Caret): boolean {
  if (caret.isSingleLine && (direction === "up" || direction === "down")) {
    return false;
  }
  const { selectionStart, selectionEnd } = caret;
  if (selectionStart === undefined || selectionEnd === undefined) {
    return true;
  }
  if (selectionStart !== selectionEnd) {
    return true;
  }
  const isBackward = direction === "left" || direction === "up";
  return isBackward ? selectionStart > 0 : selectionEnd < caret.valueLength;
}

/** The input types whose value is text with a caret. */
const textInputTypes: readonly string[] = ["text", "search", "url", "tel", "email", "password"];

/** The caret of element, where it is a text field; undefined for anything else. */
export function caretOf(element: Element): Caret | undefined {
  if (element instanceof HTMLTextAreaElement) {
    return {
      isSingleLine: false,
      selectionStart: element.selectionStart,
      selectionEnd: element.selectionEnd,
      valueLength: element.value.length,
    };
  }
  if (element instanceof HTMLInputElement && textInputTypes.includes(element.type)) {
    return {
      isSingleLine: true,
      selectionStart: element.selectionStart ?? undefined,
      selectionEnd: element.selectionEnd ?? undefined,
      valueLength: element.value.length,
    };
  }
  return undefined;
}

/** What our navigation moves focus within, by which engine, and where its first arrow starts. */
export interface FocusDrive {
  readonly region: HTMLElement;
  readonly engine: FocusEngine;
  readonly start: HTMLElement;
}

/**
 * Moves real DOM focus from what has it in the drive's region by its engine, one step in direction;
 * while nothing in region has focus, focuses start's first element. Whether it took the arrow:
 * false where focus is in a field outside region, which keeps it.
 */
export function moveFocus({ region, engine, start }: FocusDrive, direction: Direction): boolean {
  const from = document.activeElement;
  if (from instanceof HTMLElement && from !== region && region.contains(from)) {
    engine.next(from, direction, region)?.focus(visibly);
    return true;
  }
  if (from instanceof HTMLInputElement || from instanceof HTMLSelectElement) {
    return false;
  }
  start.querySelector<HTMLElement>(focusableSelector)?.focus(visibly);
  return true;
}

/**
 * Focus moved by a controller, shown as a key's is: a browser draws :focus-visible only after a key,
 * and a gamepad's press is none (the owner, Firefox on a Mac, 2026-10-25: the D-pad moved focus,
 * the log said, and the page showed nothing move).
 */
const visibly: FocusOptions = { focusVisible: true };

/**
 * Moves real DOM focus by engine on each arrow key the page hears, the browser's own handling of the
 * key prevented, so the engine alone moves it; the input probe measures each move by the focus it
 * sees arrive. An arrow while nothing in region has focus (the page just opened, focus on its body)
 * focuses start's first element (region's, unless given), as a TV app's first press does (owner,
 * Firefox, 2026-10-25: the arrows reached the page and moved nothing). An arrow in a field of a
 * form outside region is the field's; in a text field inside it, the field's unless the caret is at
 * the value's end the arrow points to (isArrowTheFields), or the field is composing. Returns the
 * stopping.
 */
export function driveFocus(
  region: HTMLElement,
  engine: FocusEngine,
  start: HTMLElement = region,
): () => void {
  const handle = (event: KeyboardEvent): void => {
    const direction = directionOfKey(event.key, keyCodeOf(event));
    if (direction === undefined) {
      return;
    }
    const from = document.activeElement;
    const caret = from === null ? undefined : caretOf(from);
    if (caret !== undefined && (event.isComposing || isArrowTheFields(direction, caret))) {
      return;
    }
    if (moveFocus({ region, engine, start }, direction)) {
      event.preventDefault();
    }
  };
  document.addEventListener("keydown", handle);
  return () => {
    document.removeEventListener("keydown", handle);
  };
}
