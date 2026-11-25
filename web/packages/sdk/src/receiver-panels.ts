// SPDX-License-Identifier: MPL-2.0

/**
 * What a receiver of a cast does with its remote's choices of sound and picture, apart from any
 * framework (D555 §3): each choice carried out by the panel it is of, each change of the panel told
 * at once, and how the film plays told to the cast's session where the receiver's video leads
 * another (D364 §11). The web receiver's hooks call these (cast-receiver-sound.ts,
 * cast-receiver-picture.ts), and any receiver may.
 */

import type { PlaybackPicture } from "./playback-picture.ts";
import type { PlaybackSound } from "./playback-sound.ts";
import type { TellsPlaybackAlone, TogetherConnection } from "./together-connection.ts";
import { PlaybackTold } from "./together-leader.ts";
import { playerSwitchedQualityEvent } from "./together-player-events.ts";
import type { PlaybackReport } from "./together.ts";

/**
 * A receiver's panel of what its remote may choose, its sound or its picture: told of each change of
 * how its controls stand, and carrying a choice out as that control's own press.
 */
export interface RemotesPanel<Choice> {
  /** Calls listener at each change of how the panel's controls stand; returns the unsubscribing. */
  readonly subscribe: (listener: () => void) => () => void;
  /** Carries choice out by the control it is of, as its own press. */
  readonly choose: (choice: Choice) => void;
}

/** The sessions a receiver tells of its panel's changes. */
export interface PanelSessions {
  /** The cast's own session, which its remote works, told how the film plays alone. */
  readonly cast: TellsPlaybackAlone;
  /** The session the video leads, told the player's state with it; the cast's, or another. */
  readonly led: TogetherConnection | undefined;
}

/**
 * Has a cast's remote work panel: each choice hearChoices hears carried out by it, and each change of
 * how its controls stand told at once, for the remote to show: to the session the video leads, with
 * its state, and to the cast's alone where that is another (a Fire TV's own controls are hidden while
 * it is controlled, the owner, VQ116's run). Returns the stopping.
 */
export function workedByTheRemote<Choice>(
  hearChoices: (listener: (choice: Choice) => void) => () => void,
  panel: RemotesPanel<Choice>,
  sessions: PanelSessions,
): () => void {
  const unsubscribe = hearChoices(panel.choose);
  const unwatch = panel.subscribe(() => {
    sessions.led?.reportState();
    sessions.cast.reportPlayback();
  });
  return () => {
    unsubscribe();
    unwatch();
  };
}

/** How a receiver's film plays and what of it may be chosen, as its cast's session is told it. */
export interface HowItPlays {
  readonly delivery: PlaybackReport["delivery"] | undefined;
  readonly readSound?: (() => PlaybackSound | undefined) | undefined;
  readonly readPicture?: (() => PlaybackPicture | undefined) | undefined;
}

/**
 * Tells cast's session how the film on media plays, sounds and shows while the receiver's video leads
 * another, its person's Watch Together session (D364 §11), where the receiver says no state in the
 * cast's: its delivery, the quality its player says it began at each switch, and its sound and
 * picture as read now (the owner, 2026-11-06: "It shouldn't be watch together's business at all
 * which TV has what offset or playback characteristics"). Returns the stopping.
 */
export function toldTheCastHowItPlays(
  cast: TellsPlaybackAlone,
  media: Pick<EventTarget, "addEventListener" | "removeEventListener">,
  how: HowItPlays,
): () => void {
  const playback = new PlaybackTold(how.delivery, how.readSound, how.readPicture);
  const heardQuality = (event: Event): void => {
    if (playback.heard(event)) {
      cast.reportPlayback();
    }
  };
  media.addEventListener(playerSwitchedQualityEvent, heardQuality);
  cast.setPlayback(() => playback.report());
  return () => {
    media.removeEventListener(playerSwitchedQualityEvent, heardQuality);
    cast.setPlayback(undefined);
  };
}
