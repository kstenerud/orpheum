// SPDX-License-Identifier: MPL-2.0

/**
 * Where focus was (controller-input-2026-10-25.md §6.1 item 2): the last element focused in each
 * focus container and in a page's main content, which an arrow coming into it from outside goes
 * back to, as a TV app's rows do; and a note of what had focus on a page, kept with its history entry, by which the
 * page's element is found again when Back returns to it, once the page has drawn it.
 */

import { focusableSelector, focusContainerAttribute, isFocusTarget } from "./focus-engine.ts";

/**
 * The attribute that marks what remembers its last focused element without bounding an arrow's
 * search: a page's main content, from which Up to the banner must find the banner's link straight
 * above, not a control at the far side of the page that is nearer up.
 */
export const focusMemoryAttribute = "data-focus-memory";

/** What remembers its last focused element: every focus container, and what is marked to. */
const rememberingSelector = `[${focusContainerAttribute}], [${focusMemoryAttribute}]`;

/** What remembers around element, inside root, the nearest first. */
export function rememberingAround(element: Element, root: Element): Element[] {
  const around: Element[] = [];
  let container = element.parentElement?.closest(rememberingSelector);
  while (container !== null && container !== undefined && root.contains(container)) {
    around.push(container);
    container = container.parentElement?.closest(rememberingSelector);
  }
  return around;
}

/** The last element focused inside each container. */
export interface ContainerMemory {
  /** Notes that element took focus, in every container around it. */
  readonly noteFocused: (element: Element) => void;
  /** What last had focus in container, where it is still there and can take focus again. */
  readonly rememberedIn: (container: Element) => HTMLElement | undefined;
}

/** A memory of the containers' last focused elements, kept no longer than the elements are. */
export function createContainerMemory(
  canTakeFocus: (element: HTMLElement) => boolean,
): ContainerMemory {
  const lastFocused = new WeakMap<Element, HTMLElement>();
  return {
    noteFocused: (element) => {
      if (!(element instanceof HTMLElement)) {
        return;
      }
      for (const container of rememberingAround(element, document.documentElement)) {
        lastFocused.set(container, element);
      }
    },
    rememberedIn: (container) => {
      const remembered = lastFocused.get(container);
      return remembered !== undefined &&
        remembered.isConnected &&
        container.contains(remembered) &&
        canTakeFocus(remembered)
        ? remembered
        : undefined;
    },
  };
}

/**
 * What had focus on a page, as its history entry keeps it: a link by its address, anything else by
 * its name; and which of the page's elements so named it was, in the document's order.
 */
export interface FocusNote {
  readonly href?: string;
  readonly name?: string;
  readonly ordinal: number;
}

/**
 * How much of an element's name a note keeps: enough to tell a page's controls apart, short enough
 * for a history entry. A justified constant.
 */
const nameKept = 80;

/** What a note knows an element by: its address where it is a link, else its name. */
function keyOf(element: HTMLElement): Pick<FocusNote, "href" | "name"> {
  const href = element.getAttribute("href");
  if (element instanceof HTMLAnchorElement && href !== null) {
    return { href };
  }
  const name = element.getAttribute("aria-label") ?? element.textContent;
  return { name: name.trim().slice(0, nameKept) };
}

/** Whether element is the one key names. */
function isKeyed(element: HTMLElement, key: Pick<FocusNote, "href" | "name">): boolean {
  const own = keyOf(element);
  return key.href === undefined ? own.name === key.name : own.href === key.href;
}

/** Every element of root that may take focus, shown now or not, in the document's order. */
function focusablesIn(root: ParentNode): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>(focusableSelector)];
}

/** The note of element, which had focus among root's elements. */
export function noteOf(element: HTMLElement, root: ParentNode): FocusNote {
  const key = keyOf(element);
  const ordinal = focusablesIn(root)
    .filter((each) => isKeyed(each, key))
    .indexOf(element);
  return { ...key, ordinal: Math.max(ordinal, 0) };
}

/**
 * What takes focus in element's place: element, where it can; else the summary of the closed menu or
 * details it is in, which Back then returns to (a place chosen from the banner's menu, which closed
 * as the page was left); else nothing yet.
 */
function standInFor(element: HTMLElement): HTMLElement | undefined {
  if (isFocusTarget(element)) {
    return element;
  }
  const summary = element.closest("details:not([open])")?.querySelector("summary");
  return summary instanceof HTMLElement && isFocusTarget(summary) ? summary : undefined;
}

/** The element of root note names, or what stands in for it, where root draws it now. */
export function elementOfNote(note: FocusNote, root: ParentNode): HTMLElement | undefined {
  const element = focusablesIn(root).filter((each) => isKeyed(each, note))[note.ordinal];
  return element === undefined ? undefined : standInFor(element);
}

/** A note kept in a history entry, read back: undefined where it is none. */
export function asFocusNote(kept: unknown): FocusNote | undefined {
  if (typeof kept !== "object" || kept === null) {
    return undefined;
  }
  const href: unknown = Reflect.get(kept, "href");
  const name: unknown = Reflect.get(kept, "name");
  const ordinal: unknown = Reflect.get(kept, "ordinal");
  if (typeof ordinal !== "number" || !Number.isInteger(ordinal) || ordinal < 0) {
    return undefined;
  }
  if (typeof href === "string") {
    return { href, ordinal };
  }
  return typeof name === "string" ? { name, ordinal } : undefined;
}

/**
 * How long a page is watched for the element focus is to be given to as it draws, returned to by
 * Back or opened: as long as a slow link's first answer takes, after which the person has moved on.
 * A justified constant.
 */
export const restoreWaitsMs = 10_000;

/**
 * Gives focus, by focus, to the element of root note names once root draws it, at once where it
 * does: a page returned to reads its data again, so its tiles come later. Watching ends with the
 * element found, after restoreWaitsMs, or when focus goes anywhere first, the person having moved
 * on. Returns the stopping.
 */
export function restoreWhenDrawn(
  note: FocusNote,
  root: HTMLElement,
  focus: (element: HTMLElement) => void,
): () => void {
  return watchWhileDrawn(root, (give) => {
    const element = elementOfNote(note, root);
    if (element === undefined) {
      return false;
    }
    give(element, focus);
    return true;
  });
}

/**
 * Gives focus by give to what look finds as root draws: look is called now and in the frame after
 * changes to root's tree, once however many came, given give, until it answers that it is done,
 * restoreWaitsMs pass, or focus goes anywhere not by give, the person having moved on. The one watch both a page returned to
 * (restoreWhenDrawn) and a page opened (focus-navigation.ts arriveWhenDrawn) use. Returns the
 * stopping.
 */
export function watchWhileDrawn(
  root: HTMLElement,
  look: (give: (element: HTMLElement, focus: (element: HTMLElement) => void) => void) => boolean,
): () => void {
  let isGiving = false;
  const give = (element: HTMLElement, focus: (element: HTMLElement) => void): void => {
    isGiving = true;
    try {
      focus(element);
    } finally {
      isGiving = false;
    }
  };
  if (look(give)) {
    return () => undefined;
  }
  // A page drawing changes its tree many times a frame, as its tiles come, and look reads every
  // control's box: it looks once a frame, before the frame is drawn, not at each change.
  let frame: number | undefined;
  const observer = new MutationObserver(() => {
    frame ??= requestAnimationFrame(() => {
      frame = undefined;
      if (look(give)) {
        stop();
      }
    });
  });
  const timer = setTimeout(() => {
    stop();
  }, restoreWaitsMs);
  const movedOn = (): void => {
    if (!isGiving) {
      stop();
    }
  };
  function stop(): void {
    observer.disconnect();
    if (frame !== undefined) {
      cancelAnimationFrame(frame);
      frame = undefined;
    }
    clearTimeout(timer);
    document.removeEventListener("focusin", movedOn);
  }
  observer.observe(root, { childList: true, subtree: true });
  document.addEventListener("focusin", movedOn);
  return stop;
}
