// SPDX-License-Identifier: MPL-2.0

import { useEffect, type RefObject } from "react";

import { focusableSelector, rovingAttribute } from "./focus-engine.ts";

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
    let stop: HTMLElement | undefined;
    // The controls root holds, and root itself where it is one, out of Tab's order.
    const takeIn = (root: HTMLElement): void => {
      for (const control of [root, ...root.querySelectorAll<HTMLElement>(focusableSelector)]) {
        if (control !== stop && control.tabIndex >= 0 && control.matches(focusableSelector)) {
          control.tabIndex = -1;
          control.setAttribute(rovingAttribute, "");
        }
      }
    };
    const moveStopTo = (control: HTMLElement | undefined): void => {
      if (stop !== undefined && stop !== control) {
        stop.tabIndex = -1;
      }
      stop = control;
      if (stop !== undefined) {
        stop.tabIndex = 0;
      }
    };
    const keepAStop = (): void => {
      if (stop === undefined || !container.contains(stop)) {
        moveStopTo(container.querySelector<HTMLElement>(`[${rovingAttribute}]`) ?? undefined);
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
        moveStopTo(target);
      }
    };
    container.addEventListener("focusin", focused);
    return () => {
      changes.disconnect();
      container.removeEventListener("focusin", focused);
    };
  }, [containerRef]);
}
