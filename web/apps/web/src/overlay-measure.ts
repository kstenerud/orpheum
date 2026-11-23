// SPDX-License-Identifier: MPL-2.0

import { useLayoutEffect, useState, type RefObject } from "react";

import { isTouchPointer } from "./input-mode.ts";

/**
 * The shorter side under which an overlay's stage is laid out as a phone's, in CSS px, where the
 * pointer is a finger (player-spec, on a touch phone): under it, previous, the steps, play and next
 * stand in the picture's middle. The shorter side, so a phone on its side, whose overlay row would
 * not fit at its width either, keeps the phone's layout and a tablet does not. A justified
 * constant, a layout's breakpoint.
 */
export const phoneOverlaySidePx = 600;

/**
 * Whether an overlay whose stage is widthPx by heightPx is laid out as a phone's
 * (phoneOverlaySidePx); a stage of no height yet is read by its width alone.
 */
export function isPhoneLayout(
  widthPx: number,
  heightPx: number,
  isTouch: boolean = isTouchPointer(),
): boolean {
  const shortPx = heightPx > 0 ? Math.min(widthPx, heightPx) : widthPx;
  return isTouch && shortPx > 0 && shortPx < phoneOverlaySidePx;
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
      const box = stage.getBoundingClientRect();
      setIsPhone(isPhoneLayout(box.width, box.height));
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
