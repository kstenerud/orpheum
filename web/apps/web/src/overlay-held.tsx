// SPDX-License-Identifier: MPL-2.0

import type { JSX, ReactNode } from "react";

import { useHasControlsOverThePicture } from "./upright.ts";

/**
 * What the overlay's menus hold (player-menus.tsx), under the film: hidden wherever the overlay lies
 * over the picture, in full screen, on a device on its side and on a computer (D555 §8), its menus
 * holding it then (D407 §8); shown only on a device held upright out of full screen, whose controls
 * lie under the picture. Kept mounted so its state stays.
 */
export function OverlayHeld(props: { readonly children: ReactNode }): JSX.Element {
  return (
    <div className="play__overlay-held" hidden={useHasControlsOverThePicture()}>
      {props.children}
    </div>
  );
}
