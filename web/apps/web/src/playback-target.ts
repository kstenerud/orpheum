// SPDX-License-Identifier: MPL-2.0

import type { CarriesPictureChoices, CarriesSoundChoices, PlaybackReport } from "@orpheum/sdk";
import type { RefObject } from "react";

import type { RemoteReport } from "./cast-remote-report.tsx";
import type { PlayerSource } from "./player-controls.tsx";

/**
 * What a person's controls play and command (D555 §8, §10): the playback target, one interface for
 * the video on this device (useVideoTarget, player-controls.tsx) and for the castee through the
 * session (useCasteeTarget, castee-target.ts). The bar, the timeline (TargetTimeline), the menus
 * (TargetMenus) and the cast's pad read it alone, so the one Play page can swap its target as a cast
 * starts and ends (the rework plan, phase 4). Play or pause, seeks and the film's sound are the
 * bar's part of it (PlayerSource); the menus' choices are the target's own (TargetChoices).
 */
export interface PlaybackTarget extends PlayerSource {
  /** Which device plays: this one, or its castee. */
  readonly kind: TargetKind;
  /**
   * Where the target last said its film stands, in ms, no seek chosen here counted: the video's own
   * place here, the castee's report through the session (D555 §5: nothing is anticipated). What a
   * castee's key frames are asked for at (the rework plan, phase 3).
   */
  readonly reportedAtMs: number;
  /** What the menus choose over (TargetMenus, target-menus.tsx). */
  readonly choices: TargetChoices;
}

/** The device a target plays on: this one ("here"), or the castee it passes the playing to. */
export type TargetKind = "here" | "castee";

/**
 * The choices the menus make (subtitles, sound track, levelling, timing, picture), as each target
 * holds them. Here they are the film's tracks around the player's stage (PlayerTracksContext) over
 * this video; a castee's are what it says of its sound and its picture, each choice sent to it to
 * carry out as its own press (D434). A choice the castee does not say, as a Google Cast device says
 * none today, is not drawn (D555 §10).
 */
export type TargetChoices = HereChoices | CasteeChoices;

/** The choices over the video on this device: its tracks, read from PlayerTracksContext. */
export interface HereChoices {
  readonly kind: "here";
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}

/** What a castee says of how it plays, and the way to send it the person's choices. */
export interface CasteeChoices {
  readonly kind: "castee";
  /** How the castee plays, its sound and its picture, as it last said; undefined until it says. */
  readonly playback: PlaybackReport | undefined;
  /** The session's connection its choices are sent over; undefined while there is none. */
  readonly connection: (CarriesSoundChoices & CarriesPictureChoices) | undefined;
  /** The film and session ⁝'s Report a problem takes the castee's playback from (D524 §2). */
  readonly report: RemoteReport | undefined;
}
