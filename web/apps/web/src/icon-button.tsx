// SPDX-License-Identifier: MPL-2.0

import "./icon-button.css";

import type { JSX, MouseEvent, PointerEvent, ReactNode, Ref } from "react";

/**
 * A button that is an icon alone, the one way to draw one (D369 §1, icon-button.test.ts): one size,
 * but the cast remote's pad's (size, D548 §11), a fingertip's (--size-control, 2.75rem), the status tray's too (the owner, 2026-10-27: "Let's try
 * sizing them the same as the others and see how that feels"); its icon drawn by `Icon`, never a text
 * character, which a television's font may lack; its name, which a screen reader says and a test
 * finds it by, given as label. Over a film it is round, on the film's control ground.
 */
/** Each mark's class, written out whole, so a search for a stylesheet's class finds it. */
const markClass = {
  running: "icon-button--running",
  applied: "icon-button--applied",
} as const;

/** Each of the cast remote's sizes' class (D548 §11), written out whole as markClass's are. */
const sizeClass = {
  56: "icon-button--remote-56",
  64: "icon-button--remote-64",
  72: "icon-button--remote-72",
  96: "icon-button--remote-96",
} as const;

/** A size a pad of round buttons takes, in px (the cast remote's pad, D548 §5, §11). */
export type IconButtonSize = keyof typeof sizeClass;

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
  /**
   * Whether it is pressed, for a button that is a state kept on (a password field's Show password,
   * password-field.tsx): aria-pressed, so its name stays the same in either state.
   */
  readonly isPressed?: boolean | undefined;
  readonly isDisabled?: boolean | undefined;
  /**
   * Whether what it does cannot be done now (a stepper's − at its least, stepper.tsx): marked
   * aria-disabled and drawn faint, a press doing nothing, but kept in the focus order, where a
   * disabled button would drop a television's focus with nowhere to go (D538 §9).
   */
  readonly isUnavailable?: boolean | undefined;
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
  /**
   * Whether only a pointer uses it (a poster row's arrows, poster-row.tsx): out of Tab's order and a
   * remote's (focus-engine.ts takes no element with a negative tabindex), and a press leaves focus
   * where it was, as what it does a keyboard or a remote already does another way.
   */
  readonly isPointerOnly?: boolean | undefined;
  /**
   * A pad's size in px, where it is not a fingertip's (the cast remote's pad, D548 §5): round, its
   * icon 2rem, 2.5rem at 96. Unsized, --size-control.
   */
  readonly size?: IconButtonSize | undefined;
  /** Where a held press repeats it (the cast remote's volume, D548 §6): its pointer's going down. */
  readonly onPointerDown?: ((event: PointerEvent<HTMLButtonElement>) => void) | undefined;
}): JSX.Element {
  const { ref, label, icon, onClick, isOverVideo, isExpanded, isDisabled, isOnCard, mark } = props;
  const isPointerOnly = props.isPointerOnly === true;
  const classes = ["icon-button"];
  if (isOverVideo === true) {
    classes.push("icon-button--over-video");
  }
  if (isOnCard === true) {
    classes.push("icon-button--on-card");
  }
  if (mark !== undefined) {
    classes.push(markClass[mark]);
  }
  if (props.size !== undefined) {
    classes.push(sizeClass[props.size]);
  }
  const isUnavailable = props.isUnavailable === true;
  return (
    <button
      ref={ref}
      type="button"
      className={classes.join(" ")}
      aria-label={label}
      aria-expanded={isExpanded}
      aria-pressed={props.isPressed}
      disabled={isDisabled}
      aria-disabled={isUnavailable ? true : undefined}
      tabIndex={isPointerOnly ? -1 : undefined}
      onMouseDown={
        isPointerOnly
          ? (event) => {
              event.preventDefault();
            }
          : undefined
      }
      onPointerDown={isUnavailable ? undefined : props.onPointerDown}
      onClick={isUnavailable ? undefined : onClick}
    >
      {icon}
    </button>
  );
}
