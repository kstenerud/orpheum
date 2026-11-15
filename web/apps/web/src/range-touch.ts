// SPDX-License-Identifier: MPL-2.0

import { useEffect, type RefObject } from "react";
import { useLatest } from "./latest.ts";
import { tapSlopPx } from "./player-taps.ts";

/** How far along a slider spanning rect a touch at clientX is: 0 at its start, 1 at its end. */
export function fractionAlong(
  clientX: number,
  rect: { readonly left: number; readonly width: number },
): number {
  if (rect.width <= 0) {
    return 0;
  }
  return Math.min(1, Math.max(0, (clientX - rect.left) / rect.width));
}

/** Where a finger touches the screen, as a touch's clientX and clientY say. */
export interface TouchPoint {
  readonly clientX: number;
  readonly clientY: number;
}

/**
 * Has a touch anywhere on the slider in inputRef set it where the finger is and follow it, telling
 * touching.onTouched where along it, from the slider's own box and the finger's point (valueAt):
 * Safari on an iPhone moves a slider only by a touch that starts on its thumb, and the owner's
 * volumes would not move unless hit just so (2026-10-12). The page does not scroll under the
 * finger. The latest onTouched is told each time.
 *
 * Where touching.isTapHeld says a touch coming down is held, it sets nothing until the finger has
 * moved further than a tap's wobble (tapSlopPx): a tap then does nothing, and a drag follows the
 * finger from there (the overlay's scrub bar just shown, scrubTapHeldForMs).
 */
export function useTouchAnywhere(
  inputRef: RefObject<HTMLInputElement | null>,
  valueAt: (clientX: number, rect: DOMRect, point: TouchPoint) => number,
  touching: {
    readonly onTouched: (value: number) => void;
    readonly isTapHeld?: (() => boolean) | undefined;
  },
): void {
  const touchedRef = useLatest({ valueAt, ...touching });
  useEffect(() => {
    const input = inputRef.current;
    if (input === null) {
      return undefined;
    }
    // Where a held touch came down, until it moves further than a tap would.
    let heldAt: TouchPoint | undefined;
    const follow = (event: TouchEvent): void => {
      const touch = event.touches[0];
      if (touch === undefined || input.disabled) {
        return;
      }
      event.preventDefault(); // the finger moves the slider, not the page
      const touched = touchedRef.current;
      if (event.type === "touchstart") {
        const isHeld = touched.isTapHeld?.() === true;
        heldAt = isHeld ? { clientX: touch.clientX, clientY: touch.clientY } : undefined;
      }
      if (heldAt !== undefined) {
        const moved = Math.hypot(touch.clientX - heldAt.clientX, touch.clientY - heldAt.clientY);
        if (moved <= tapSlopPx) {
          return;
        }
        heldAt = undefined; // a drag, which follows the finger from here
      }
      touched.onTouched(touched.valueAt(touch.clientX, input.getBoundingClientRect(), touch));
    };
    input.addEventListener("touchstart", follow, { passive: false });
    input.addEventListener("touchmove", follow, { passive: false });
    return () => {
      input.removeEventListener("touchstart", follow);
      input.removeEventListener("touchmove", follow);
    };
  }, [inputRef]);
}
