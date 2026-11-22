// SPDX-License-Identifier: MPL-2.0

/**
 * What the browser tests of the form's parts share (D538; stepper.tsx, switch.tsx, checkbox.tsx,
 * field.tsx, submit-button.tsx): the widths a part is looked at in, a finger's tap, and whether the
 * focus ring shows on what has focus.
 */

import { act } from "react";

/** The widths a part is looked at in: a phone's, a computer's and a television's page (D538). */
export const partWidths = [360, 1280, 1920] as const;

/** A finger's tap on element: a touch's pointer down and up, then the click the browser makes of it. */
export function tapOn(element: Element | null | undefined): void {
  if (!(element instanceof HTMLElement)) {
    throw new Error("nothing to tap");
  }
  const box = element.getBoundingClientRect();
  const init = {
    bubbles: true,
    pointerType: "touch",
    pointerId: 7,
    isPrimary: true,
    clientX: box.left + box.width / 2,
    clientY: box.top + box.height / 2,
  };
  act(() => {
    element.dispatchEvent(new PointerEvent("pointerdown", init));
    element.dispatchEvent(new PointerEvent("pointerup", init));
    element.click();
  });
}

/**
 * Whether what has focus is something, not the page, and shows the focus ring (tokens.css): drawn
 * by the key that moved it there, an outline of the ring's width.
 */
export function isRingShown(): boolean {
  const focused = document.activeElement;
  if (!(focused instanceof HTMLElement) || focused === document.body) {
    return false;
  }
  const style = getComputedStyle(focused);
  return (
    focused.matches(":focus-visible") &&
    style.outlineStyle !== "none" &&
    Number.parseFloat(style.outlineWidth) > 0
  );
}

/** The accessible name of what has focus, its label or its text. */
export function focusedName(): string | null | undefined {
  const focused = document.activeElement;
  return focused?.getAttribute("aria-label") ?? focused?.textContent;
}
