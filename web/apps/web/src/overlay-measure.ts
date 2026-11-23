// SPDX-License-Identifier: MPL-2.0

import { useLayoutEffect, useState, type RefObject } from "react";

import { isTouchPointer } from "./input-mode.ts";

/**
 * The widest an overlay's stage lays it out as a phone's, in CSS px, where the pointer is a finger
 * (player-spec, on a touch phone): under it, play and the steps stand in the picture's middle. A
 * justified constant, a layout's breakpoint.
 */
export const phoneOverlayWidthPx = 600;

/** Whether an overlay whose stage is widthPx wide is laid out as a phone's (phoneOverlayWidthPx). */
export function isPhoneLayout(widthPx: number, isTouch: boolean = isTouchPointer()): boolean {
  return isTouch && widthPx > 0 && widthPx < phoneOverlayWidthPx;
}

/** The custom property the overlay's measured height sets on its stage, which lifts what lies above it. */
export const overlayLiftProperty = "--player-overlay-lift";

/**
 * Measures the overlay in overlayRef as it is drawn and as its stage or rows change size: the
 * height of its rows from the top row to its bottom, set on its stage as overlayLiftProperty, which
 * lifts the subtitles and the bottom-left notes above it (standards/ui.md, over the film; the fixed
 * guess it replaces left a phone's subtitles under the buttons); and whether its stage lays it out
 * as a phone's (isPhoneLayout), which it returns. Hidden, the overlay keeps the last height.
 */
export function useOverlayMeasured(
  overlayRef: RefObject<HTMLElement | null>,
  shown: string,
): boolean {
  const [isPhone, setIsPhone] = useState(false);
  useLayoutEffect(() => {
    const overlay = overlayRef.current;
    const stage = overlay?.parentElement;
    if (overlay === null || stage === null || stage === undefined) {
      return undefined;
    }
    const measure = (): void => {
      const top = overlay.querySelector(".player-overlay__top");
      const liftPx =
        top === null ? 0 : overlay.getBoundingClientRect().bottom - top.getBoundingClientRect().top;
      if (liftPx > 0) {
        stage.style.setProperty(overlayLiftProperty, `${String(Math.ceil(liftPx))}px`);
      }
      setIsPhone(isPhoneLayout(stage.getBoundingClientRect().width));
    };
    measure();
    if (typeof ResizeObserver !== "function") {
      return undefined;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(stage);
    for (const row of overlay.querySelectorAll(".player-overlay__row")) {
      observer.observe(row);
    }
    return () => {
      observer.disconnect();
    };
  }, [overlayRef, shown]);
  return isPhone;
}
