// SPDX-License-Identifier: MPL-2.0

import "./icon-button.css";

import type { JSX, MouseEvent, ReactNode, Ref } from "react";

/**
 * A button that is an icon alone, the one way to draw one (D369 §1, icon-button.test.ts): one size,
 * a fingertip's (--size-control, 2.75rem), the status tray's too (the owner, 2026-10-27: "Let's try
 * sizing them the same as the others and see how that feels"); its icon drawn by `Icon`, never a text
 * character, which a television's font may lack; its name, which a screen reader says and a test
 * finds it by, given as label. Over a film it is round, on the film's control ground.
 */
export function IconButton(props: {
  /** What the button does, in the viewer's words: its accessible name. */
  readonly label: string;
  /** The icon, an `Icon` drawing. */
  readonly icon: ReactNode;
  readonly onClick: (event: MouseEvent<HTMLButtonElement>) => void;
  readonly ref?: Ref<HTMLButtonElement> | undefined;
  /** Whether it is drawn over a film: round, on --color-video-control, in --color-on-video. */
  readonly isOverVideo?: boolean | undefined;
  /** Whether what it opens is open, for a button that opens a panel. */
  readonly isExpanded?: boolean | undefined;
  readonly isDisabled?: boolean | undefined;
  /**
   * Whether it is drawn on a card over a film (the up-next offer's), not on the film's control
   * ground: --color-video-card, hover included. Only with isOverVideo.
   */
  readonly isOnCard?: boolean | undefined;
  /**
   * What its colour says, where it says anything: "running", what it stands for is on, the status
   * blue, its icon breathing slowly (not while a film plays, nor for a person who asked for less
   * motion); "applied", a filter it opens is applied, the accent. Unmarked, the region's colour.
   */
  readonly mark?: "running" | "applied" | undefined;
}): JSX.Element {
  const { ref, label, icon, onClick, isOverVideo, isExpanded, isDisabled, isOnCard, mark } = props;
  const classes = ["icon-button"];
  if (isOverVideo === true) {
    classes.push("icon-button--over-video");
  }
  if (isOnCard === true) {
    classes.push("icon-button--on-card");
  }
  if (mark !== undefined) {
    classes.push(`icon-button--${mark}`);
  }
  return (
    <button
      ref={ref}
      type="button"
      className={classes.join(" ")}
      aria-label={label}
      aria-expanded={isExpanded}
      disabled={isDisabled}
      onClick={onClick}
    >
      {icon}
    </button>
  );
}
