// SPDX-License-Identifier: MPL-2.0

import type { JSX, ReactNode } from "react";

import { useIsFullScreen } from "./full-screen-button.tsx";

/**
 * What the full-screen overlay's menus hold (player-menus.tsx), under the film: hidden while full
 * screen draws the overlay, which holds them then (D407 §8), kept mounted so their state stays.
 */
export function OverlayHeld(props: { readonly children: ReactNode }): JSX.Element {
  return (
    <div className="play__overlay-held" hidden={useIsFullScreen()}>
      {props.children}
    </div>
  );
}
