// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import { useEffect, type ComponentType, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { timeText } from "./date-time-text.ts";
import type { FilmParts } from "./film-player.tsx";

/** What the overlay tells the offer it draws in its convenience area (D407 §3, §5). */
export interface OfferPlace {
  /** Whether all of the overlay shows, where its first Back hides it and leaves the offer be. */
  readonly isOverlayFull?: boolean | undefined;
  /** Told whether the offer shows, so the overlay shows it alone while hidden. */
  readonly onOffered?: ((isOffered: boolean) => void) | undefined;
}

/**
 * An offer the player draws: Skip intro and its kind (segments.tsx), Up next or Still watching
 * (up-next.tsx). Given to the overlay rather than imported, so the overlay knows no offer's module.
 */
export type OfferPiece = ComponentType<FilmParts & OfferPlace>;

/**
 * The overlay's convenience area, at the end of its top row (D407 §1, §3;
 * plans/player-overlay.md §2.1): the offer while one is on, which shows alone while the rest of the
 * overlay is hidden, drawn in the film's top-right corner whether the rest shows or not (the owner,
 * 2026-11-09; player-overlay.css); with nothing on offer and all of the overlay shown, when the film ends ("Ends
 * at 9:41"), leftMs from its end.
 */
export function ConvenienceArea(props: {
  readonly film: FilmParts;
  readonly offer: OfferPiece | undefined;
  readonly hasOffer: boolean;
  readonly isOverlayFull: boolean;
  readonly onOffered: (isOffered: boolean) => void;
  readonly leftMs: number | undefined;
  readonly areaRef: RefObject<HTMLDivElement | null>;
}): JSX.Element {
  const { film, offer: Offer, hasOffer, isOverlayFull, onOffered, leftMs, areaRef } = props;
  return (
    <div ref={areaRef} className="convenience-area">
      {Offer === undefined ? null : (
        <Offer {...film} isOverlayFull={isOverlayFull} onOffered={onOffered} />
      )}
      {isOverlayFull && !hasOffer ? <EndsAt leftMs={leftMs} /> : null}
    </div>
  );
}

/** When the film ends if it plays on from now, leftMs from its end; nothing while its length is not known. */
function EndsAt(props: { readonly leftMs: number | undefined }): JSX.Element {
  const { t, i18n } = useTranslation();
  const { leftMs } = props;
  return (
    <span className="convenience-area__ends">
      {leftMs === undefined
        ? ""
        : t("playerControls.endsAt", { time: endsAt(leftMs, i18n.language) })}
    </span>
  );
}

/**
 * When the film ends if it plays on from now, as the viewer's clock says it, in their chosen
 * language as every time on a page is (timeText), not the browser's.
 */
function endsAt(leftMs: number, locale: string): string {
  return timeText(Date.now() + leftMs, locale);
}

/** Tells onOffered, where given, whether an offer shows, and that it does not once it goes. */
export function useTellsOffered(
  isShown: boolean,
  onOffered: ((isOffered: boolean) => void) | undefined,
): void {
  useEffect(() => {
    if (!isShown || onOffered === undefined) {
      return undefined;
    }
    onOffered(true);
    return () => {
      onOffered(false);
    };
  }, [isShown, onOffered]);
}

/**
 * Focuses an offer's button in buttonRef as it shows where nothing but the video or the page has
 * focus, so a remote's OK presses it, and wherever focus is where isTaken (Up next as the film ends);
 * never from another control, so a press already on its way goes where it was aimed.
 */
export function useFocusAsItShows(
  isShown: boolean,
  isTaken: boolean,
  buttonRef: RefObject<HTMLButtonElement | null>,
): void {
  useEffect(() => {
    const focused = document.activeElement;
    const isFree =
      focused === null || focused === document.body || focused instanceof HTMLVideoElement;
    if (isShown && (isTaken || isFree)) {
      buttonRef.current?.focus();
    }
  }, [isShown, isTaken, buttonRef]);
}
