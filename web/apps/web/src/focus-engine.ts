// SPDX-License-Identifier: MPL-2.0

/**
 * The app's focus engine (controller-input-2026-10-25.md §6.1 item 2), proven by the device lab's
 * button test (plans/tv-device-tests.md): which element an arrow moves real DOM focus to, behind a
 * narrow interface, so another engine (@bbc/tv-lrud-spatial, should the owner take it) drops in
 * beside ours and is measured the same way.
 * Ours is a minimal geometric search: the nearest element whose box lies in the arrow's direction,
 * nearness along the arrow counting less than the step across it.
 */

import type { Direction } from "./input-buttons.ts";

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
 * across; undefined where none lies that way. Left and Right move along from's line alone, to a box
 * beside it: Right at the end of a grid's short last row goes nowhere rather than up a row (a walk
 * at Silk's 960 × 480, 2026-10-25; kept by the owner, ui-audit-2026-11-04.md Q3).
 */
export function nearestInDirection<T>(
  from: Box,
  candidates: readonly Placed<T>[],
  direction: Direction,
): T | undefined {
  const isAlongALine = direction === "left" || direction === "right";
  return nearestBy(from, candidates, direction, (along, across) =>
    isAlongALine && across > 0 ? undefined : along + acrossWeight * across,
  );
}

/**
 * How much a step across counts beside one along where nothing lies in line Left or Right and the
 * search falls back to what lies off it (nearestOffTheLine): a control on the next line a little to
 * the side is chosen over one far along it on another. The W3C CSS Spatial Navigation draft's
 * orthogonalWeight for Left and Right ("find the shortest distance", https://www.w3.org/TR/css-nav-1/),
 * whose 2 for Up and Down is acrossWeight's. A justified constant, the draft's.
 */
const offTheLineWeight = 30;

/**
 * The candidate an arrow in direction falls back to where nothing lies in line that way (R6 of
 * ui-audit-2026-11-04.md): the nearest whose box lies wholly past from's edge that way, at any step
 * across, which weighs offTheLineWeight steps along for Left and Right and acrossWeight for Up and
 * Down. The usual TV engines' way: Android's FocusFinder prefers a candidate in the arrow's beam and
 * else takes the nearest in the half-plane (`beamBeats`, then `getWeightedDistanceFor`); the W3C
 * draft searches the half-plane by a distance weighted off the axis. Without it a control with no
 * other on its line was left by no Left or Right (Your account's code field, the name field under it
 * reached by no arrow, the audit's walk).
 */
export function nearestOffTheLine<T>(
  from: Box,
  candidates: readonly Placed<T>[],
  direction: Direction,
): T | undefined {
  const weight = direction === "left" || direction === "right" ? offTheLineWeight : acrossWeight;
  return nearestBy(from, candidates, direction, (along, across) => along + weight * across);
}

/**
 * The candidate wholly past from's edge in direction (a box may touch it) that scoreOf scores least
 * by its gap along and its gap across, the first of equals; one scored undefined is none.
 */
function nearestBy<T>(
  from: Box,
  candidates: readonly Placed<T>[],
  direction: Direction,
  scoreOf: (along: number, across: number) => number | undefined,
): T | undefined {
  let best: { readonly item: T; readonly score: number } | undefined;
  for (const candidate of candidates) {
    const along = gapAlong(from, candidate.box, direction);
    const score = along < 0 ? undefined : scoreOf(along, gapAcross(from, candidate.box, direction));
    if (score !== undefined && (best === undefined || score < best.score)) {
      best = { item: candidate.item, score };
    }
  }
  return best?.item;
}

/**
 * A focus engine: given the focused element, an arrow and the region it moves within, the element
 * focus moves to, or undefined to leave it where it is.
 */
export interface FocusEngine {
  readonly name: string;
  readonly next: (from: Element, direction: Direction, region: Element) => HTMLElement | undefined;
}

/**
 * What may take focus: links, buttons, form fields, a details' summary, a video with its own
 * controls and anything with a tabindex, each as long as it is not switched off.
 */
export const focusableSelector = [
  "a[href]",
  "button:not([disabled])",
  'input:not([disabled]):not([type="hidden"])',
  "select:not([disabled])",
  "textarea:not([disabled])",
  "summary",
  "video[controls]",
  "[tabindex]",
].join(", ");

/** Whether element is inside a closed details, but for its summary, which stays shown. */
function isInClosedDetails(element: HTMLElement): boolean {
  const details = element.closest("details");
  return details !== null && !details.open && element.closest("summary")?.parentElement !== details;
}

/**
 * Whether element is drawn: laid out (one inside a closed menu or a closed details has no box, and
 * would otherwise be found at the screen's corner), and, where the browser can say, neither hidden
 * nor skipped as a closed details' content is in Chromium, which still gives it a box.
 */
function isDrawn(element: HTMLElement): boolean {
  if (element.getClientRects().length === 0 || isInClosedDetails(element)) {
    return false;
  }
  return (
    typeof element.checkVisibility !== "function" ||
    element.checkVisibility({ visibilityProperty: true })
  );
}

/** Whether element can take focus by an arrow now: in the tab order, drawn, and not inert. */
export function isFocusTarget(element: HTMLElement): boolean {
  return element.tabIndex >= 0 && element.closest("[inert]") === null && isDrawn(element);
}

/** The elements in region an arrow can move focus to, in the document's order. */
export function focusTargetsIn(region: ParentNode): HTMLElement[] {
  return [...region.querySelectorAll<HTMLElement>(focusableSelector)].filter((element) =>
    isFocusTarget(element),
  );
}

/**
 * The attribute that marks an element fixed over the page's top, as the shell's sticky banner: an
 * element focused beneath it is scrolled clear of it (focus-navigation.ts), and the search reads it
 * where it stands at the page's top, not where it sticks.
 */
export const coversTopAttribute = "data-focus-covers-top";

/**
 * Where element is for the search: its box on the screen, or, inside the banner over the page's
 * top, its box at the page's top, where it stands unscrolled. A page scrolled down holds what is
 * above the screen between its content and the banner, and Up from the screen's top row goes there
 * before the banner (a walk at Silk's 960 × 480, 2026-10-25: Up from the films skipped Continue
 * Watching, scrolled out above, for the banner over it).
 */
export function searchBoxOf(element: Element): Box {
  const box = element.getBoundingClientRect();
  if (element.closest(`[${coversTopAttribute}]`) === null) {
    return box;
  }
  const scrolled = window.scrollY;
  return {
    left: box.left,
    right: box.right,
    top: box.top - scrolled,
    bottom: box.bottom - scrolled,
  };
}

/**
 * Our geometric engine over the region's elements that can take focus, but from and those in it: the
 * nearest in line (nearestInDirection); where none is, and from is in no focus container in the
 * region, the nearest off the line (nearestOffTheLine). A row's or a grid's control keeps to its
 * line, so Right at the end of a grid's short last row stays (Q3) and at a row's end goes nowhere;
 * a page's loose controls, as a form's fields set in no grid, reach each other.
 */
export const geometricFocus: FocusEngine = {
  name: "geometric",
  next: (from, direction, region) => {
    const candidates: Placed<HTMLElement>[] = [];
    for (const element of focusTargetsIn(region)) {
      if (element !== from && !from.contains(element)) {
        candidates.push({ item: element, box: searchBoxOf(element) });
      }
    }
    const box = searchBoxOf(from);
    const inLine = nearestInDirection(box, candidates, direction);
    return inLine !== undefined || isInAContainer(from, region)
      ? inLine
      : nearestOffTheLine(box, candidates, direction);
  },
};

/** Whether element is inside a focus container in region, region itself counting. */
function isInAContainer(element: Element, region: Element): boolean {
  const container = element.parentElement?.closest(`[${focusContainerAttribute}]`);
  return container !== null && container !== undefined && region.contains(container);
}

/**
 * The attribute that marks a focus container in the markup (controller-input-2026-10-25.md §6.1
 * item 2): the shell's banner, a page's main content, a row or grid of tiles, a menu, the player.
 * An arrow looks first inside the container that holds focus, and only where nothing lies that way
 * there, in the one around it, out to the whole region; so Right at a row's end does not leap to
 * whatever is to the right on the page while the row has more. Each container remembers what last
 * had focus in it (focus-memory.ts).
 */
export const focusContainerAttribute = "data-focus-container";

/** The container nearest around element, inside root; root itself where none is. */
export function containerAround(element: Element, root: Element): Element {
  const container = element.parentElement?.closest(`[${focusContainerAttribute}]`);
  return container !== null && container !== undefined && root.contains(container)
    ? container
    : root;
}

/**
 * The element an arrow in direction moves focus to from from, by engine, within root: the nearest
 * that way inside from's container, else inside the container around that, out to root; undefined
 * where none lies that way in root.
 */
export function nextInContainers(
  from: HTMLElement,
  direction: Direction,
  { root, engine }: { readonly root: Element; readonly engine: FocusEngine },
): HTMLElement | undefined {
  let scope = containerAround(from, root);
  for (;;) {
    const next = engine.next(from, direction, scope);
    if (next !== undefined || scope === root) {
      return next;
    }
    scope = containerAround(scope, root);
  }
}

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

/**
 * The attribute that marks a range whose Left and Right are commands, not its own steps: a player's
 * scrub bar, whose arrows the player answers by seeking (player-overlay.tsx), as a TV's player does.
 */
export const arrowsAreCommandsAttribute = "data-focus-arrows-are-commands";

/**
 * Whether an arrow in direction is element's own rather than ours: a text field's, by its caret
 * (isArrowTheFields), and every arrow while it composes; a range's Left and Right, which move its
 * value, Up and Down leaving it, but a range marked arrowsAreCommandsAttribute.
 */
export function isArrowTheElements(
  element: Element,
  direction: Direction,
  isComposing: boolean,
): boolean {
  const caret = caretOf(element);
  if (caret !== undefined) {
    return isComposing || isArrowTheFields(direction, caret);
  }
  const isAcross = direction === "left" || direction === "right";
  return (
    element instanceof HTMLInputElement &&
    element.type === "range" &&
    isAcross &&
    !element.hasAttribute(arrowsAreCommandsAttribute)
  );
}
