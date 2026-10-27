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
  readonly ref?: Ref<HTMLButtonElement>;
  /** Whether it is drawn over a film: round, on --color-video-control, in --color-on-video. */
  readonly isOverVideo?: boolean;
  /** Whether what it opens is open, for a button that opens a panel. */
  readonly isExpanded?: boolean;
  readonly isDisabled?: boolean;
  /** The class of where it is drawn, which may colour it; never its size. */
  readonly className?: string;
}): JSX.Element {
  const { ref, label, icon, onClick, isOverVideo, isExpanded, isDisabled, className } = props;
  const classes = ["icon-button"];
  if (isOverVideo === true) {
    classes.push("icon-button--over-video");
  }
  if (className !== undefined) {
    classes.push(className);
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
