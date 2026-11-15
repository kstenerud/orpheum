// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type RefObject } from "react";

/**
 * Whether a finger, a pen or a mouse is held down on the element in ref: from a pointerdown on it
 * to that pointer's pointerup or pointercancel, heard on the window, so a lift off the element ends
 * it too. Chrome on an Android phone, a slow drag on the player's scrub bar, sends pointerdown and
 * touchstart on it, the range capturing the pointer, then pointerup, lostpointercapture and touchend
 * at the lift, and no pointercancel, the page's own touch handling taking the gesture
 * (useTouchAnywhere; idiosyncrasies.md). A touchend that leaves no finger on the screen ends a
 * finger's hold too, so it never stays held after the finger is up whatever pointer event is lost.
 */
export function useHeldDown(ref: RefObject<HTMLElement | null>): boolean {
  const [isHeld, setIsHeld] = useState(false);
  useEffect(() => {
    const element = ref.current;
    if (element === null) {
      return undefined;
    }
    // Each pointer down on the element, by its id, with its type.
    const down = new Map<number, string>();
    const pressed = (event: PointerEvent): void => {
      down.set(event.pointerId, event.pointerType);
      setIsHeld(true);
    };
    const lifted = (event: PointerEvent): void => {
      if (down.delete(event.pointerId) && down.size === 0) {
        setIsHeld(false);
      }
    };
    const fingersLifted = (event: TouchEvent): void => {
      if (event.touches.length > 0) {
        return;
      }
      for (const [id, type] of down) {
        if (type === "touch") {
          down.delete(id);
        }
      }
      if (down.size === 0) {
        setIsHeld(false);
      }
    };
    element.addEventListener("pointerdown", pressed);
    window.addEventListener("pointerup", lifted, true);
    window.addEventListener("pointercancel", lifted, true);
    window.addEventListener("touchend", fingersLifted, true);
    window.addEventListener("touchcancel", fingersLifted, true);
    return () => {
      element.removeEventListener("pointerdown", pressed);
      window.removeEventListener("pointerup", lifted, true);
      window.removeEventListener("pointercancel", lifted, true);
      window.removeEventListener("touchend", fingersLifted, true);
      window.removeEventListener("touchcancel", fingersLifted, true);
    };
  }, [ref]);
  return isHeld;
}
