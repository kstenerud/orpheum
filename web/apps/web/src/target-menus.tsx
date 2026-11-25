// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";

import { noOverlay, RemoteMenus } from "./cast-remote-choices.tsx";
import type { PlaybackTarget } from "./playback-target.ts";
import { PlayerMenus } from "./player-menus.tsx";
import type { OverlayControl } from "./player-overlay-state.ts";

/** No control of the bar's: a castee's menus, or a page that names none. */
const noControls: readonly OverlayControl[] = [];

/**
 * The menus over the playback target (D555 §8, §10), the one entry point the Play page's bar and
 * overlay and the cast's remote draw, so no page chooses its menus by what plays. They are drawn from
 * the target's choices: over the video here, its tracks (PlayerMenus), 🔈 its sound where the bar
 * shows it; over a castee, what it says of its sound and its picture, each choice sent to it
 * (RemoteMenus, D434). A choice the target has no use for is not drawn: a castee's has no 🔈, its
 * film's level the remote's pad's (D548 §6), and a menu with nothing the castee says is not drawn.
 * The two bodies stay two until the video here says its sound and picture as a castee does, which
 * the receiver's command core moving into a package gives (the rework plan, phase 3).
 */
export function TargetMenus(props: {
  readonly target: PlaybackTarget;
  /**
   * The controls this device's bar or overlay shows (controlsShown): which menus over the video here
   * are drawn, none of the bar's by default. A castee's are what it says.
   */
  readonly shown?: readonly OverlayControl[] | undefined;
  /** Told whether a menu is open, for an overlay to hold its time (D407 §6). */
  readonly onOpenChange?: ((isOpen: boolean) => void) | undefined;
}): JSX.Element {
  const { target } = props;
  const { choices } = target;
  if (choices.kind === "castee") {
    return (
      <RemoteMenus
        playback={choices.playback}
        connection={choices.connection}
        report={choices.report}
      />
    );
  }
  return (
    <PlayerMenus
      videoRef={choices.videoRef}
      shown={props.shown ?? noControls}
      sound={target.sound}
      onOpenChange={props.onOpenChange ?? noOverlay}
    />
  );
}
