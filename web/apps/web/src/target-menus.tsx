// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";

import { noOverlay, RemoteMenus } from "./cast-remote-choices.tsx";
import type { PlaybackTarget, PlayerSound } from "./playback-target.ts";
import { PlayerMenus, VolumeMenu } from "./player-menus.tsx";
import type { OverlayControl } from "./player-overlay-state.ts";
import { peopleOf, useSessionVoice } from "./together-voice.tsx";

/** No control of the bar's: a castee's menus, or a page that names none. */
const noControls: readonly OverlayControl[] = [];

/**
 * The menus over the playback target (D555 §8, §10), the one entry point the Play page's bar and
 * overlay and the cast's remote draw, so no page chooses its menus by what plays. They are drawn from
 * the target's choices: over the video here, its tracks (PlayerMenus), 🔈 its sound where the bar
 * shows it; over a castee, what it says of its sound and its picture, each choice sent to it
 * (RemoteMenus, D434). A choice the target has no use for is not drawn: a castee's 🔈 shows only
 * while this page is in a Watch Together session with others' voices to mix (CasteesVoices), its
 * film's level otherwise the remote's pad's alone (D548 §6), and a menu with nothing the castee says
 * is not drawn. The two bodies stay two until the video here says its sound and picture as a castee
 * does, which the receiver's command core moving into a package gives (the rework plan, phase 3).
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
  const onOpenChange = props.onOpenChange ?? noOverlay;
  if (choices.kind === "castee") {
    return (
      <>
        <CasteesVoices sound={target.sound} onOpenChange={onOpenChange} />
        <RemoteMenus
          playback={choices.playback}
          connection={choices.connection}
          report={choices.report}
        />
      </>
    );
  }
  return (
    <PlayerMenus
      videoRef={choices.videoRef}
      shown={props.shown ?? noControls}
      sound={target.sound}
      onOpenChange={onOpenChange}
    />
  );
}

/**
 * 🔈 over a castee while this page is in a Watch Together session with others' voices to mix: the
 * castee's film level, then each person's voice, the row the player's 🔈 opens over the video here
 * (VolumeMenu, D440 §2), so the people's volumes are in the bar's volume button, not under the stage
 * (the owner, 2026-11-26: "merged into the volume button in the bottom bar"). Nothing outside a
 * session, with no one else's voice, or while the castee says no film level (useReceiverFilmSound).
 */
function CasteesVoices(props: {
  readonly sound: PlayerSound | undefined;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element | null {
  const voice = useSessionVoice();
  if (props.sound === undefined || voice === undefined || peopleOf(voice.snapshot).length === 0) {
    return null;
  }
  return <VolumeMenu sound={props.sound} onOpenChange={props.onOpenChange} />;
}
