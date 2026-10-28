// SPDX-License-Identifier: MPL-2.0

/**
 * Moving real DOM focus by an arrow, and pressing what has it (controller-input-2026-10-25.md §6.1
 * item 2): within the region a person can reach now (a full-screen element, else an open modal
 * dialog, else the page), by the focus engine bounded by containers, back into a container at what
 * last had focus there, and the element moved to scrolled into view no further than it must be.
 */

import {
  coversTopAttribute,
  focusTargetsIn,
  geometricFocus,
  isFocusTarget,
  nextInContainers,
  searchBoxOf,
  type FocusEngine,
} from "./focus-engine.ts";
import { rememberingAround, type ContainerMemory } from "./focus-memory.ts";
import type { Direction } from "./input-buttons.ts";

/**
 * The region an arrow moves within: the element shown full screen, else the open modal dialog,
 * whose page behind is inert, else the document's body.
 */
export function focusRegion(): HTMLElement {
  const fullscreen = document.fullscreenElement;
  if (fullscreen instanceof HTMLElement) {
    return fullscreen;
  }
  const dialogs = [...document.querySelectorAll<HTMLDialogElement>("dialog[open]")];
  return dialogs.at(-1) ?? document.body;
}

/**
 * The attribute that marks where a page's first arrow starts, where that is not its main content's
 * first control: the device lab's tiles.
 */
export const focusStartAttribute = "data-focus-start";

/**
 * Where the first arrow goes while nothing in region has focus: in the page's main content (not the
 * shell's banner, the owner, 2026-10-25), its marked start, else its first control; in a dialog, its
 * first control.
 */
export function startIn(region: HTMLElement): HTMLElement | undefined {
  const main = region.querySelector("main") ?? region;
  const marked = main.querySelector<HTMLElement>(`[${focusStartAttribute}]`);
  if (marked !== null) {
    const first = isFocusTarget(marked) ? marked : focusTargetsIn(marked)[0];
    if (first !== undefined) {
      return first;
    }
  }
  return focusTargetsIn(main)[0] ?? focusTargetsIn(region)[0];
}

/**
 * The room kept between a focused element and the banner over it, in CSS pixels: the focus ring's
 * two widths and a little air (tokens.css). A justified constant, the ring's geometry.
 */
const roomAboveFocusPx = 12;

/**
 * Scrolls element into view no further than it must be, across as in a row of tiles and down as on a
 * page, and clear of a banner over the page's top where it is on the page: an open dialog's control,
 * over the banner, moves no page under it (a drop-down's Close, the owner on a Fire TV, 2026-10-28).
 */
export function scrollIntoViewGently(element: HTMLElement): void {
  // A test's DOM may have no scrolling at all.
  if (typeof element.scrollIntoView !== "function") {
    return;
  }
  element.scrollIntoView({ block: "nearest", inline: "nearest" });
  const cover = document.querySelector(`[${coversTopAttribute}]`);
  if (cover === null || cover.contains(element) || element.closest("dialog[open]") !== null) {
    return;
  }
  const hiddenBy = cover.getBoundingClientRect().bottom + roomAboveFocusPx;
  const top = element.getBoundingClientRect().top;
  if (top < hiddenBy) {
    window.scrollBy({ top: top - hiddenBy });
  }
}

/**
 * Focuses element and scrolls it into view gently. Where a controller moved it, the focus is shown as
 * a key's is: a browser draws :focus-visible only after a key, and a gamepad's press is none (the
 * owner, Firefox on a Mac, 2026-10-25: the D-pad moved focus, the log said, and the page showed
 * nothing move); a pointer's never is.
 */
export function focusAndShow(element: HTMLElement, isShownAsAKeys: boolean): void {
  element.focus(
    isShownAsAKeys ? { preventScroll: true, focusVisible: true } : { preventScroll: true },
  );
  scrollIntoViewGently(element);
}

/** How an arrow moves focus: by which engine, and with which containers' memory. */
export interface FocusMoving {
  readonly engine?: FocusEngine;
  readonly memory: ContainerMemory;
}

/**
 * Where an arrow in direction goes from from within region: the engine's choice, unless that enters
 * a container from outside it which remembers what last had focus in it, which is then where it goes.
 */
function destinationOf(
  from: HTMLElement,
  direction: Direction,
  { region, moving }: { readonly region: HTMLElement; readonly moving: FocusMoving },
): HTMLElement | undefined {
  const next = nextInContainers(from, direction, {
    root: region,
    engine: moving.engine ?? geometricFocus,
  });
  if (next === undefined) {
    return undefined;
  }
  // The outermost of what remembers around next that from is not in: what the arrow enters.
  const entered = rememberingAround(next, region)
    .filter((around) => !around.contains(from))
    .at(-1);
  return (entered === undefined ? undefined : moving.memory.rememberedIn(entered)) ?? next;
}

/**
 * Moves real DOM focus one step in direction within the region a person can reach now; while nothing
 * there has focus, to where the first arrow starts. Whether it moved: false where nothing lies that
 * way, so the browser does with the arrow what it would, as scrolling a page past its last control.
 */
export function moveFocus(
  direction: Direction,
  moving: FocusMoving,
  isFromAController: boolean,
): boolean {
  const region = focusRegion();
  const from = document.activeElement;
  const next =
    from instanceof HTMLElement && from !== region && region.contains(from)
      ? destinationOf(from, direction, { region, moving })
      : startIn(region);
  if (next === undefined) {
    return false;
  }
  if (from instanceof HTMLElement && isReadOnTheWay(from, next, direction)) {
    return true;
  }
  focusAndShow(next, isFromAController);
  return true;
}

/**
 * How far apart two controls must be, in screens, for an arrow from one to the other to scroll the
 * page first: half a screen and more of text between them, which a person moving by a remote would
 * otherwise jump past unread (a film's overview, About, the System page). A justified constant.
 */
const readingGapScreens = 0.5;

/** How far such an arrow scrolls the page, in screens: as Page Down does. A justified constant. */
const readingStepScreens = 0.8;

/**
 * Scrolls the page a step toward next instead of moving focus to it, where Up or Down passes more
 * than readingGapScreens of what takes no focus and next is not on the screen yet; whether it did.
 * Once next comes on the screen, the arrow moves focus to it.
 */
function isReadOnTheWay(from: HTMLElement, next: HTMLElement, direction: Direction): boolean {
  if (direction !== "up" && direction !== "down") {
    return false;
  }
  const [fromBox, nextBox] = [searchBoxOf(from), searchBoxOf(next)];
  const isDown = direction === "down";
  const gap = isDown ? nextBox.top - fromBox.bottom : fromBox.top - nextBox.bottom;
  const height = window.innerHeight;
  const isNextOnScreen = nextBox.bottom > 0 && nextBox.top < height;
  if (gap <= height * readingGapScreens || isNextOnScreen) {
    return false;
  }
  const scrolledBefore = window.scrollY;
  window.scrollBy({ top: (isDown ? 1 : -1) * height * readingStepScreens });
  return window.scrollY !== scrolledBefore;
}

/** The input types whose value is typed. */
const typedInputTypes: readonly string[] = [
  "text",
  "search",
  "url",
  "tel",
  "email",
  "password",
  "number",
];

/** The input types whose Enter is the browser's own: a typed one's submits its form, a button's presses. */
const enterOwnInputTypes: readonly string[] = [
  ...typedInputTypes,
  "submit",
  "button",
  "reset",
  "image",
];

/** Whether a key's Enter on element is the browser's own to act on: a link's, a button's, a form's. */
function isEntersOwn(element: HTMLElement): boolean {
  if (element instanceof HTMLInputElement) {
    return enterOwnInputTypes.includes(element.type);
  }
  return (
    element instanceof HTMLAnchorElement ||
    element instanceof HTMLButtonElement ||
    element instanceof HTMLTextAreaElement ||
    element.tagName === "SUMMARY"
  );
}

/**
 * Presses what has focus, as OK on a remote or a controller does: a list's choices opened by a key,
 * its next chosen by a controller, anything else clicked. A key's Enter is left to the browser where it acts on its own (a link, a button, a
 * text field submitting its form), and a controller's OK does nothing to a text field, whose typing
 * is the keyboard's; a checkbox, which a browser's Enter does not tick, is clicked. Whether it
 * pressed anything.
 */
export function pressFocused(isFromAController: boolean): boolean {
  const focused = document.activeElement;
  if (!(focused instanceof HTMLElement) || focused === document.body) {
    return false;
  }
  if (focused instanceof HTMLSelectElement) {
    return isFromAController ? chooseNext(focused) : openChoices(focused);
  }
  if (!isFromAController && isEntersOwn(focused)) {
    return false;
  }
  if (focused instanceof HTMLTextAreaElement || isTextInput(focused)) {
    return false;
  }
  focused.click();
  return true;
}

/** Whether element is an input whose value is typed. */
function isTextInput(element: HTMLElement): boolean {
  return element instanceof HTMLInputElement && typedInputTypes.includes(element.type);
}

/**
 * Opens a list's choices, where the browser lets a page (showPicker, which wants a key's press): a
 * remote's OK is the only way a TV opens one; else chooses the next.
 */
function openChoices(select: HTMLSelectElement): boolean {
  try {
    select.showPicker();
    return true;
  } catch {
    return chooseNext(select);
  }
}

/**
 * Chooses select's next choice, after the last the first, telling the page as a choice made does;
 * whether it could. A controller's OK does this in place of opening the choices: its press is no
 * activation a browser opens them by (the HTML standard's are keys and pointers), and a list a key's
 * recent press let open would stay open, its own popup out of the controller's reach.
 */
function chooseNext(select: HTMLSelectElement): boolean {
  const count = select.options.length;
  if (count < 2) {
    return false;
  }
  // eslint-disable-next-line no-param-reassign -- choosing is what the press asks of the list
  select.selectedIndex = (select.selectedIndex + 1) % count;
  select.dispatchEvent(new Event("input", { bubbles: true }));
  select.dispatchEvent(new Event("change", { bubbles: true }));
  return true;
}
