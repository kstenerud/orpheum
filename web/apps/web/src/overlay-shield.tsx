// SPDX-License-Identifier: MPL-2.0

/** The shield over the player's overlay just shown, which takes every touch for a moment (VQ347). */

import { useLayoutEffect, useRef, useState, type JSX } from "react";

/**
 * How long the overlay's controls take no finger's, pen's or mouse's touch after the overlay appears,
 * in ms: on a phone a second tap, meant for the picture, landed on the full-width scrub bar the first
 * tap had just shown and jumped the film. The owner chose a hold for the scrub bar (2026-11-15,
 * option a of three) and widened it to every control of the overlay, in full screen too (VQ347): a
 * tap, a click or a drag that comes down in it does nothing to its end, and one after works as
 * before. A key's or a remote's press is not held (OverlayShield). Half a second felt a little long, so
 * it is 0.3 s (the owner, 2026-11-15: "Let's try 0.3s").
 */
export const overlayHeldForMs = 300;

/**
 * How long the shield stays after the last touch on it lifts, past its time, in ms, unless the
 * touch's click comes first: a browser sends a tap's click a moment after the lift, at what is under
 * the finger then, and a shield gone by then would let the control under it take the click.
 */
const shieldWaitsForClickMs = 300;

/** The shield's class, by which the page's tap readers know a touch on it is no tap of theirs. */
export const overlayShieldClass = "player-overlay__shield";

/**
 * A transparent element over the whole overlay, in front of every control, while the overlay has
 * just shown (the owner, 2026-11-15: "put a second overlay on front that swallows all taps, then
 * remove it"): from the frame isShown turns true for overlayHeldForMs, a touch comes down on it and
 * on no control, so no control's listener, nor a native range's own slider, sees it, cancelable or
 * not (idiosyncrasies.md). A touch that came down on it keeps it until the touch lifts and its click
 * comes, so a drag begun in the hold moves nothing to its end. It takes no focus and is hidden from
 * a screen reader: a key's or a remote's press goes to the focused control, never held.
 */
export function OverlayShield(props: { readonly isShown: boolean }): JSX.Element | null {
  const { isShown } = props;
  const [isUp, setIsUp] = useState(false);
  const isTimeUpRef = useRef(false);
  const downRef = useRef(new Set<number>());
  const clickWaitRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const lowerIfDone = useRef((): void => {
    if (isTimeUpRef.current && downRef.current.size === 0 && clickWaitRef.current === undefined) {
      setIsUp(false);
    }
  }).current;
  // A layout effect, so the frame that first shows the overlay has the shield over it already.
  useLayoutEffect(() => {
    isTimeUpRef.current = false;
    downRef.current.clear();
    setIsUp(isShown);
    if (!isShown) {
      return undefined;
    }
    const timer = setTimeout(() => {
      isTimeUpRef.current = true;
      lowerIfDone();
    }, overlayHeldForMs);
    return () => {
      clearTimeout(timer);
      clearTimeout(clickWaitRef.current);
      clickWaitRef.current = undefined;
    };
  }, [isShown, lowerIfDone]);
  const shieldRef = useRef<HTMLDivElement>(null);
  // Bound as the shield is drawn, before a touch can land on it. A touch's lift is heard on the
  // window, so a mouse released off the shield lifts it too.
  useLayoutEffect(() => {
    const shield = shieldRef.current;
    if (!isUp || shield === null) {
      return undefined;
    }
    const pressed = (event: PointerEvent): void => {
      downRef.current.add(event.pointerId);
    };
    const lifted = (event: PointerEvent): void => {
      if (!downRef.current.delete(event.pointerId) || downRef.current.size > 0) {
        return;
      }
      clearTimeout(clickWaitRef.current);
      clickWaitRef.current = setTimeout(() => {
        clickWaitRef.current = undefined;
        lowerIfDone();
      }, shieldWaitsForClickMs);
    };
    const clicked = (): void => {
      if (downRef.current.size === 0 && clickWaitRef.current !== undefined) {
        clearTimeout(clickWaitRef.current);
        clickWaitRef.current = undefined;
        lowerIfDone();
      }
    };
    shield.addEventListener("pointerdown", pressed);
    shield.addEventListener("click", clicked);
    window.addEventListener("pointerup", lifted, true);
    window.addEventListener("pointercancel", lifted, true);
    return () => {
      shield.removeEventListener("pointerdown", pressed);
      shield.removeEventListener("click", clicked);
      window.removeEventListener("pointerup", lifted, true);
      window.removeEventListener("pointercancel", lifted, true);
    };
  }, [isUp, lowerIfDone]);
  return isUp ? <div ref={shieldRef} className={overlayShieldClass} aria-hidden="true" /> : null;
}
