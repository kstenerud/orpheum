// SPDX-License-Identifier: MPL-2.0

import { useLayoutEffect, useRef, type RefObject } from "react";

/**
 * A ref that holds value as of the page as drawn, for listeners and timers set up once. It is kept in
 * a layout effect, which runs as the page is drawn: a passive effect may run a while after the page
 * shows, and an event that soon after (a Fire TV's Back just after Up showed the player's controls)
 * was answered with the render before's value (the overlay's browser test, 2026-11-05).
 */
export function useLatest<T>(value: T): RefObject<T> {
  const ref = useRef(value);
  useLayoutEffect(() => {
    ref.current = value;
  });
  return ref;
}
