// SPDX-License-Identifier: MPL-2.0

import "./offer-card.css";

import type { JSX, ReactNode, RefObject } from "react";

import { IconButton } from "./icon-button.tsx";
import { CloseIcon } from "./player-icons.tsx";

/** The way to put an offer away, where it can be: its ✕'s name ("Dismiss Up next") and its press. */
export interface OfferDismissal {
  readonly label: string;
  readonly onDismiss: () => void;
}

/**
 * A card over a film offering one thing to take or leave (standards/ui.md, over the film, rule 15;
 * D538 §1): Skip, Up next, Still watching?. One at a time, in the film's top-right corner, which
 * its user lays it in, on the film's layer for offers. Its words are read out once as it shows
 * (an `<output>`), with a sentence said alone where given ("Plays in 10 seconds", as a countdown
 * starts); a line under them (the countdown itself) is not, as it changes every second. Where the
 * offer is taken by pressing the card itself (Skip, Up next), the still and the words are one
 * button; otherwise its buttons follow the words (Still watching's Continue and Stop). A ✕ puts it
 * away only where it can be put away.
 */
export function OfferCard(props: {
  /** What it offers: "Up next: Episode 3", "Still watching?". */
  readonly words: ReactNode;
  /** A line under the words that is not read out at each change: "Plays in 10 s". */
  readonly detail?: ReactNode;
  /** A sentence read out with the words but not shown, once, as it comes: "Plays in 10 seconds". */
  readonly said?: string | undefined;
  /** A still of what it offers, an image the card sizes to 8rem, 16:9. */
  readonly still?: ReactNode;
  /** Taking the offer by pressing the card itself. */
  readonly onTake?: (() => void) | undefined;
  /** The card's own button, where it is pressed itself, for its user to focus. */
  readonly takeRef?: RefObject<HTMLButtonElement | null> | undefined;
  /** The card's buttons, where it is not pressed itself, the main one first. */
  readonly buttons?: ReactNode;
  readonly dismissal?: OfferDismissal | undefined;
}): JSX.Element {
  const { onTake, dismissal, takeRef, buttons } = props;
  const content = (
    <>
      {props.still === undefined ? null : <span className="offer-card__still">{props.still}</span>}
      <span className="offer-card__words">
        <output className="offer-card__offer">
          {props.words}
          {props.said === undefined ? null : <span className="offer-card__said">{props.said}</span>}
        </output>
        {props.detail === undefined ? null : (
          <span className="offer-card__detail">{props.detail}</span>
        )}
      </span>
    </>
  );
  return (
    <div className="offer-card" data-focus-container="">
      <div className="offer-card__body">
        {onTake === undefined ? (
          <div className="offer-card__content">{content}</div>
        ) : (
          <button
            ref={takeRef}
            type="button"
            className="offer-card__content offer-card__take"
            onClick={onTake}
          >
            {content}
          </button>
        )}
        {buttons === undefined ? null : <div className="offer-card__buttons">{buttons}</div>}
      </div>
      {dismissal === undefined ? null : (
        <IconButton
          label={dismissal.label}
          icon={<CloseIcon />}
          isOverVideo
          isOnCard
          onClick={dismissal.onDismiss}
        />
      )}
    </div>
  );
}
