// SPDX-License-Identifier: MPL-2.0

/**
 * Back on a list takes focus up to the banner's place for it (the owner, 2026-11-21: "Pressing back
 * while on a film list should bring the cursor to the top bar, over whichever item the view belongs
 * to"): with focus anywhere but the banner, Back focuses the banner's place marked current, shown as
 * an arrow's focus is, and does nothing else; with focus in the banner, Back goes back as it always
 * did. A remote's or a controller's Back is a command (input-layer.tsx); a Fire TV's remote Back in
 * Silk is a step of the history, never a key (idiosyncrasies.md), so on a television an entry is held
 * over the page's (navigation.ts's holdBack) while focus is outside the banner, and let go as focus
 * enters it, so the browser's own Back from there leaves the page as it did. On a computer nothing is
 * held: its browser's Back button only moving focus would be lost on a mouse.
 */

import { useEffect, type RefObject } from "react";

import { firstFocusTargetIn } from "./focus-engine.ts";
import { focusAndShow } from "./focus-navigation.ts";
import { useCommands } from "./input-layer.tsx";
import { routeHref } from "./router.ts";
import { useRoute, useServices } from "./services.tsx";
import { isOnTelevision } from "./input-mode.ts";

/** The banner's place for the page shown: its places for watching, the one marked current. */
const currentPlaceSelector = '.app-shell__watching > li[aria-current="page"]';

/** The control of the banner's place for the page shown, where focus can go to it now. */
function currentPlaceIn(banner: HTMLElement): HTMLElement | undefined {
  const place = banner.querySelector(currentPlaceSelector);
  return place === null ? undefined : firstFocusTargetIn(place);
}

/** Whether focus is in banner. */
function isFocusIn(banner: HTMLElement): boolean {
  return banner.contains(document.activeElement);
}

/**
 * While isOn (the page is a list), Back with focus outside the banner in bannerRef focuses the
 * banner's place for the page instead of going back. On a television, the entry held for Silk's Back
 * is laid again for each page the address names, since a go or a replace lets every hold go
 * (navigation.ts), as a list's sort, a view of it, does.
 */
export function useBackToBannerPlace(
  bannerRef: RefObject<HTMLElement | null>,
  isOn: boolean,
): void {
  const { navigation } = useServices();
  const href = routeHref(useRoute());
  useCommands("document", (detail, event) => {
    const banner = bannerRef.current;
    if (!isOn || detail.command !== "back" || banner === null || isFocusIn(banner)) {
      return;
    }
    const place = currentPlaceIn(banner);
    if (place !== undefined) {
      event.preventDefault();
      focusAndShow(place, true);
    }
  });
  useEffect(() => {
    const banner = bannerRef.current;
    if (!isOn || banner === null || !isOnTelevision()) {
      return undefined;
    }
    let release: (() => void) | undefined;
    const holdWhileOutside = (): void => {
      if (isFocusIn(banner)) {
        release?.();
        release = undefined;
      } else if (release === undefined && currentPlaceIn(banner) !== undefined) {
        release = navigation.holdBack(() => {
          release = undefined;
          const place = currentPlaceIn(banner);
          if (place !== undefined) {
            focusAndShow(place, true);
          }
        });
      }
    };
    holdWhileOutside();
    document.addEventListener("focusin", holdWhileOutside);
    return () => {
      document.removeEventListener("focusin", holdWhileOutside);
      release?.();
    };
  }, [bannerRef, isOn, navigation, href]);
}
