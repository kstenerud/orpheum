// SPDX-License-Identifier: MPL-2.0

import "./player-icons.css";

import type { JSX } from "react";

import { Icon } from "./icon.tsx";

/*
 * The player's icons (D369 §1), each drawn by Icon in the colour of where it is, for an IconButton:
 * in place of the text characters ▶, ❚❚, ⤢, ⤡ and ×, which a television's font may lack.
 */

/** Play: a triangle pointing on. */
export function PlayIcon(): JSX.Element {
  return (
    <Icon className="player-icons__icon">
      <path d="M8 4.5v15l12-7.5z" />
    </Icon>
  );
}

/** Pause: two bars. */
export function PauseIcon(): JSX.Element {
  return (
    <Icon className="player-icons__icon">
      <path d="M6 4.5h4v15H6zm8 0h4v15h-4z" />
    </Icon>
  );
}

/** Full screen: four corners pointing out. */
export function FullScreenIcon(): JSX.Element {
  return (
    <Icon className="player-icons__icon">
      <path d="M3 3h7v2.5H5.5V10H3zm11 0h7v7h-2.5V5.5H14zM3 14h2.5v4.5H10V21H3zm15.5 0H21v7h-7v-2.5h4.5z" />
    </Icon>
  );
}

/** Leave full screen: four corners pointing in. */
export function LeaveFullScreenIcon(): JSX.Element {
  return (
    <Icon className="player-icons__icon">
      <path d="M7.5 3H10v7H3V7.5h4.5zM14 3h2.5v4.5H21V10h-7zM3 14h7v7H7.5v-4.5H3zm11 0h7v2.5h-4.5V21H14z" />
    </Icon>
  );
}

/** Close: a cross. */
export function CloseIcon(): JSX.Element {
  return (
    <Icon className="player-icons__icon">
      <path d="M5.6 4.2 12 10.6l6.4-6.4 1.4 1.4-6.4 6.4 6.4 6.4-1.4 1.4-6.4-6.4-6.4 6.4-1.4-1.4 6.4-6.4-6.4-6.4z" />
    </Icon>
  );
}

/** Back a step: an arrow turning back round a circle. */
export function SeekBackIcon(): JSX.Element {
  return (
    <Icon className="player-icons__icon">
      <path d="M11 2v7L6 5.5z" />
      <path
        d="M11 5.5a7.5 7.5 0 1 1-7.5 7.5"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.25"
        strokeLinecap="round"
      />
    </Icon>
  );
}

/** Forward a step: an arrow turning on round a circle. */
export function SeekForwardIcon(): JSX.Element {
  return (
    <Icon className="player-icons__icon">
      <path d="M13 2v7l5-3.5z" />
      <path
        d="M13 5.5a7.5 7.5 0 1 0 7.5 7.5"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.25"
        strokeLinecap="round"
      />
    </Icon>
  );
}

/** Sound on: a speaker and its waves. */
export function SoundIcon(): JSX.Element {
  return (
    <Icon className="player-icons__icon">
      <path d="M3 9h4l5-4.5v15L7 15H3z" />
      <path
        d="M15.5 8.5a5 5 0 0 1 0 7M18 5.5a9 9 0 0 1 0 13"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      />
    </Icon>
  );
}

/** Sound off: a speaker and a cross. */
export function MutedIcon(): JSX.Element {
  return (
    <Icon className="player-icons__icon">
      <path d="M3 9h4l5-4.5v15L7 15H3z" />
      <path
        d="m15.5 9 5.5 6m0-6-5.5 6"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      />
    </Icon>
  );
}
