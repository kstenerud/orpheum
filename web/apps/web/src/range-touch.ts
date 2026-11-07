// SPDX-License-Identifier: MPL-2.0

import { useEffect, type RefObject } from "react";
import { useLatest } from "./latest.ts";

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
 * onTouched where along it, from the slider's own box and the finger's point (valueAt): Safari on an iPhone moves a slider
 * only by a touch that starts on its thumb, and the owner's volumes would not move unless hit just so
 * (2026-10-12). The page does not scroll under the finger. The latest onTouched is told each time.
 */
export function useTouchAnywhere(
  inputRef: RefObject<HTMLInputElement | null>,
  valueAt: (clientX: number, rect: DOMRect, point: TouchPoint) => number,
  onTouched: (value: number) => void,
): void {
  const touchedRef = useLatest({ valueAt, onTouched });
  useEffect(() => {
    const input = inputRef.current;
    if (input === null) {
      return undefined;
    }
    const follow = (event: TouchEvent): void => {
      const touch = event.touches[0];
      if (touch === undefined || input.disabled) {
        return;
      }
      event.preventDefault(); // the finger moves the slider, not the page
      const touched = touchedRef.current;
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
