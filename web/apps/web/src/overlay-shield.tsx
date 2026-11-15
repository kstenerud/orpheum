// SPDX-License-Identifier: MPL-2.0

/** The shield over the player's overlay just shown, which takes every touch for a moment (VQ347). */

import { useCallback, useLayoutEffect, useRef, useState, type JSX } from "react";

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

/** What the shield keeps between renders: whether its time is up, the touches down on it, a click awaited. */
class ShieldHold {
  isTimeUp = false;
  readonly down = new Set<number>();
  private clickWait: ReturnType<typeof setTimeout> | undefined = undefined;

  get isClickAwaited(): boolean {
    return this.clickWait !== undefined;
  }

  /** Waits shieldWaitsForClickMs for the lifted touch's click, then calls then. */
  awaitClick(then: () => void): void {
    clearTimeout(this.clickWait);
    this.clickWait = setTimeout(() => {
      this.clickWait = undefined;
      then();
    }, shieldWaitsForClickMs);
  }

  endClickWait(): void {
    clearTimeout(this.clickWait);
    this.clickWait = undefined;
  }
}

/**
 * Hears the touches on the shield, keeping it up while one is down and until its click comes; returns
 * the unbinding. A touch's lift is heard on the window, so a mouse released off the shield lifts it too.
 */
function bindShieldTouches(
  shield: HTMLElement,
  hold: ShieldHold,
  lowerIfDone: () => void,
): () => void {
  const pressed = (event: PointerEvent): void => {
    hold.down.add(event.pointerId);
  };
  const lifted = (event: PointerEvent): void => {
    if (!hold.down.delete(event.pointerId) || hold.down.size > 0) {
      return;
    }
    hold.awaitClick(lowerIfDone);
  };
  const clicked = (): void => {
    if (hold.down.size === 0 && hold.isClickAwaited) {
      hold.endClickWait();
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
}

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
  const [isUp, setIsUp] = useState(isShown);
  // Raised or lowered in the render isShown changes in, so the frame that first shows the overlay
  // has the shield over it already.
  const [wasShown, setWasShown] = useState(isShown);
  if (wasShown !== isShown) {
    setWasShown(isShown);
    setIsUp(isShown);
  }
  const holdRef = useRef(new ShieldHold());
  const lowerIfDone = useCallback((): void => {
    const hold = holdRef.current;
    if (hold.isTimeUp && hold.down.size === 0 && !hold.isClickAwaited) {
      setIsUp(false);
    }
  }, []);
  useLayoutEffect(() => {
    const hold = holdRef.current;
    hold.isTimeUp = false;
    hold.down.clear();
    if (!isShown) {
      return undefined;
    }
    const timer = setTimeout(() => {
      hold.isTimeUp = true;
      lowerIfDone();
    }, overlayHeldForMs);
    return () => {
      clearTimeout(timer);
      hold.endClickWait();
    };
  }, [isShown, lowerIfDone]);
  const shieldRef = useRef<HTMLDivElement>(null);
  // Bound as the shield is drawn, before a touch can land on it.
  useLayoutEffect(() => {
    const shield = shieldRef.current;
    if (!isUp || shield === null) {
      return undefined;
    }
    return bindShieldTouches(shield, holdRef.current, lowerIfDone);
  }, [isUp, lowerIfDone]);
  return isUp ? <div ref={shieldRef} className={overlayShieldClass} aria-hidden="true" /> : null;
}
