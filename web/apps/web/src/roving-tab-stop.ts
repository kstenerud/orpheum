// SPDX-License-Identifier: MPL-2.0

import { useEffect, type RefObject } from "react";

import { entryAttribute, focusableSelector, rovingAttribute } from "./focus-engine.ts";

/** The roving controls in container: those its tab stop took out of Tab's order, and its stop. */
function rovingIn(container: Element): HTMLElement[] {
  return [...container.querySelectorAll<HTMLElement>(`[${rovingAttribute}]`)];
}

/** container's tab stop: its roving control still in Tab's order, if it has one. */
function stopIn(container: Element): HTMLElement | undefined {
  return rovingIn(container).find((control) => control.tabIndex >= 0);
}

/** Makes control container's one tab stop, every other roving control in it out of Tab's order. */
function moveStopTo(container: Element, control: HTMLElement): void {
  for (const each of rovingIn(container)) {
    each.tabIndex = each === control ? 0 : -1;
    each.removeAttribute(entryAttribute);
  }
}

/**
 * Makes control, one of a roving tab stop's container's, the stop, and where an arrow coming into
 * the container from outside goes (entryAttribute), until focus moves in the container: the
 * alphabet rail's letter of the poster focused beside it (the owner, 2026-11-11).
 */
export function standTabStopAt(container: Element, control: HTMLElement): void {
  if (!control.hasAttribute(rovingAttribute) || control.contains(document.activeElement)) {
    return;
  }
  moveStopTo(container, control);
  control.setAttribute(entryAttribute, "");
}

/**
 * Makes the controls in the list containerRef holds one Tab stop (a roving tab stop): Tab enters the
 * list at the one last focused in it, its first until one is, and leaves it with the next Tab, as
 * the arrows still move from one to the next in it (the owner, 2026-11-11: "One Tab stop per poster
 * grid and row … yes, do it now"). A keyboard on a desktop then passes a list of 332 films with one
 * press, not 332; a remote's arrows are as they were (isInArrowsOrder).
 *
 * Its controls are taken out of Tab's order as they come, the list growing page by page; one already
 * out of it, a pointer's alone (IconButton), is left as it is. Where the tab stop leaves the list,
 * its first control takes it.
 */
export function useRovingTabStop(containerRef: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    const container = containerRef.current;
    if (container === null) {
      return undefined;
    }
    // The controls root holds, and root itself where it is one, out of Tab's order.
    const takeIn = (root: HTMLElement): void => {
      const stop = stopIn(container);
      for (const control of [root, ...root.querySelectorAll<HTMLElement>(focusableSelector)]) {
        if (control !== stop && control.tabIndex >= 0 && control.matches(focusableSelector)) {
          control.tabIndex = -1;
          control.setAttribute(rovingAttribute, "");
        }
      }
    };
    const keepAStop = (): void => {
      const first = rovingIn(container)[0];
      if (stopIn(container) === undefined && first !== undefined) {
        moveStopTo(container, first);
      }
    };
    takeIn(container);
    keepAStop();
    const changes = new MutationObserver((records) => {
      for (const record of records) {
        for (const added of record.addedNodes) {
          if (added instanceof HTMLElement) {
            takeIn(added);
          }
        }
      }
      keepAStop();
    });
    changes.observe(container, { childList: true, subtree: true });
    const focused = (event: FocusEvent): void => {
      const target = event.target;
      if (target instanceof HTMLElement && target.hasAttribute(rovingAttribute)) {
        moveStopTo(container, target);
      }
    };
    container.addEventListener("focusin", focused);
    return () => {
      changes.disconnect();
      container.removeEventListener("focusin", focused);
    };
  }, [containerRef]);
}
