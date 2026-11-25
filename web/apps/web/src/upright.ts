// SPDX-License-Identifier: MPL-2.0

import { useIsFullScreen } from "./full-screen-button.tsx";
import { useIsUpright } from "./input-mode.ts";

/**
 * Whether the player's controls lie over the picture, the overlay (D555 §8): in full screen, on a
 * device on its side and on a computer; a device held upright out of full screen has them under the
 * picture, never hidden (page-controls.tsx).
 */
export function useHasControlsOverThePicture(): boolean {
  const isFullScreen = useIsFullScreen();
  const isUprightNow = useIsUpright();
  return isFullScreen || !isUprightNow;
}
