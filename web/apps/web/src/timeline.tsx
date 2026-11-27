// SPDX-License-Identifier: MPL-2.0

import "./timeline.css";

import {
  useEffect,
  useRef,
  useState,
  type JSX,
  type KeyboardEvent,
  type PointerEvent,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { clockText } from "./date-time-text.ts"; // the one way a film's clock is written
import { fractionAlong, useTouchAnywhere } from "./range-touch.ts";
import { ScrubPicture, scrubCellAt, type ScrubPictures } from "./scrub-pictures.tsx";

/** How finely a drag along a timeline moves the film: a second. */
const dragStepMs = 1000;

/** How long the picture of a place stepped or dragged to stays after the last move. */
export const steppedPictureMs = 1500;

/** Where the film stands and its length, "1:01 / 10:00"; where it alone while its length is not known. */
export function ClockTimes(props: {
  readonly atMs: number;
  readonly durationMs: number;
}): JSX.Element {
  const { atMs, durationMs } = props;
  const { language } = useTranslation().i18n;
  return (
    <span className="timeline__times">
      {durationMs > 0
        ? `${clockText(atMs, language)} / ${clockText(durationMs, language)}`
        : clockText(atMs, language)}
    </span>
  );
}

/** A touch anywhere along the slider seeks to there, in the drag's steps (useTouchAnywhere). */
function useTouchSeeks(
  inputRef: RefObject<HTMLInputElement | null>,
  mostMs: number,
  onSeek: (atMs: number) => void,
): void {
  useTouchAnywhere(
    inputRef,
    (clientX, rect) =>
      Math.round((fractionAlong(clientX, rect) * mostMs) / dragStepMs) * dragStepMs,
    onSeek,
  );
}

/**
 * A film's timeline (D369 §1), the one way to draw one: a slider in milliseconds from the film's
 * start to its end, and its times beside it. Each move of a drag or a press is a seek, which its
 * player makes at once (a video on this page, which shows each place) or once the moves rest (a
 * remote's, where each seek is a command to another device, as a leading player's drag is one
 * seek); atMs is where the player shows the film, the place of a seek on its way, so the marker
 * moves at once and stays (seek-ahead.ts). A touch anywhere on it sets it where
 * the finger is, as a volume's does (range-touch.ts), Safari on an iPhone moving a slider only from
 * its thumb. On a television its Left and Right are commands, which its player answers by seeking a
 * step (arrowsAreCommandsAttribute in focus-engine.ts), not the slider's own small steps. A mouse
 * over it shows the time under the pointer above it (TimeUnderPointer); where the film has
 * scrub-bar pictures (pictures, scrub-pictures.tsx), the picture of that place above its time, and
 * the picture of the marker's place while a finger drags it or arrows step it, for a while after
 * each move (steppedPictureMs). A stretch whose pictures are not made shows the time alone.
 */
export function Timeline(props: {
  /** What the slider is, in the viewer's words: its accessible name. */
  readonly label: string;
  readonly atMs: number;
  /** The film's length; 0 while not known. */
  readonly durationMs: number;
  readonly onSeek: (atMs: number) => void;
  readonly isDisabled?: boolean | undefined;
  /** Whether Left and Right are the player's commands rather than the slider's own steps. */
  readonly hasArrowsAsCommands?: boolean | undefined;
  /** The slider, for a player that moves focus to it or asks whether it has it. */
  readonly inputRef?: RefObject<HTMLInputElement | null> | undefined;
  /** Whether the times stand beside the slider; false where the page draws them itself (the overlay's). */
  readonly hasTimes?: boolean | undefined;
  /** The film's scrub-bar pictures made so far; none, the time alone. */
  readonly pictures?: ScrubPictures | undefined;
}): JSX.Element {
  const { label, atMs, durationMs, onSeek, isDisabled, hasArrowsAsCommands } = props;
  const { language } = useTranslation().i18n;
  const ownRef = useRef<HTMLInputElement | null>(null);
  const inputRef = props.inputRef ?? ownRef;
  const mostMs = Math.max(durationMs, 1);
  const shownMs = shownAtMsOf(atMs, durationMs);
  const { shownPlace, stepped, notePointer, setPointed } = useShownPlace(inputRef, shownMs, props);
  useTouchSeeks(inputRef, mostMs, (seekMs) => {
    stepped.note();
    onSeek(seekMs);
  });
  return (
    <div className={props.hasTimes === false ? "timeline timeline--alone" : "timeline"}>
      <div className="timeline__track">
        <input
          ref={inputRef}
          type="range"
          className="timeline__range"
          aria-label={label}
          aria-valuetext={clockText(shownMs, language)}
          min={0}
          max={mostMs}
          step={dragStepMs}
          value={Math.min(shownMs, mostMs)}
          disabled={isDisabled}
          data-focus-arrows-are-commands={hasArrowsAsCommands === true ? "" : undefined}
          onChange={(event) => {
            stepped.note();
            onSeek(Number(event.currentTarget.value));
          }}
          onKeyDown={(event: KeyboardEvent<HTMLInputElement>) => {
            // A television's arrows are the player's seeks (player-commands.ts), heard here too.
            if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
              stepped.note();
            }
          }}
          onPointerMove={notePointer}
          onPointerLeave={() => {
            setPointed(undefined);
          }}
        />
        {shownPlace === undefined ? null : (
          <TimeUnderPointer place={shownPlace} durationMs={durationMs} pictures={props.pictures} />
        )}
      </div>
      {props.hasTimes === false ? null : <ClockTimes atMs={shownMs} durationMs={durationMs} />}
    </div>
  );
}

/**
 * The place a timeline shows above itself, if any: a mouse's pointer's, else the marker's while a
 * finger drags or arrows step it where the film has pictures (useStepped).
 */
function useShownPlace(
  inputRef: RefObject<HTMLInputElement | null>,
  shownMs: number,
  film: { readonly durationMs: number; readonly pictures?: ScrubPictures | undefined },
): {
  readonly shownPlace: Place | undefined;
  readonly stepped: { readonly note: () => void };
  readonly notePointer: (event: PointerEvent<HTMLInputElement>) => void;
  readonly setPointed: (place: Place | undefined) => void;
} {
  const { durationMs } = film;
  const stepped = useStepped(inputRef, film.pictures !== undefined && durationMs > 0);
  const [pointed, setPointed] = useState<Place | undefined>(undefined);
  const notePointer = (event: PointerEvent<HTMLInputElement>): void => {
    const rect = event.currentTarget.getBoundingClientRect();
    setPointed(
      event.pointerType === "mouse" && durationMs > 0
        ? { fraction: fractionAlong(event.clientX, rect), widthPx: rect.width }
        : undefined,
    );
  };
  const shownPlace: Place | undefined =
    pointed ??
    (stepped.widthPx === undefined
      ? undefined
      : { fraction: Math.min(shownMs / Math.max(durationMs, 1), 1), widthPx: stepped.widthPx });
  return { shownPlace, stepped, notePointer, setPointed };
}

/** A place along a timeline shown above it: its fraction of the film, and the slider's width. */
export interface Place {
  readonly fraction: number;
  readonly widthPx: number;
}

/**
 * Whether a finger's drag or an arrow's step moved the marker lately (steppedPictureMs), and the
 * slider's width as it moved: the picture of the marker's place is shown then. Only where
 * isPictured: with no pictures a drag and a step show nothing above the slider, as before.
 */
function useStepped(
  inputRef: RefObject<HTMLInputElement | null>,
  isPictured: boolean,
): { readonly widthPx: number | undefined; readonly note: () => void } {
  const [widthPx, setWidthPx] = useState<number | undefined>(undefined);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(
    () => () => {
      clearTimeout(timerRef.current);
    },
    [],
  );
  const note = (): void => {
    const input = inputRef.current;
    if (!isPictured || input === null) {
      return;
    }
    setWidthPx(input.getBoundingClientRect().width);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      setWidthPx(undefined);
    }, steppedPictureMs);
  };
  return { widthPx: isPictured ? widthPx : undefined, note };
}

/**
 * The time at a place on a timeline, above it there (the UI review's "the scrub bar's time", item 10
 * of the UI revamp's first wave, navigation-and-widgets-answers-2026-10-27.md): a mouse's pointer's,
 * or the marker's as a finger drags or arrows step it. Where the film's picture of the place is made
 * (pictures), the picture above the time, the two kept inside the slider's ends (pictureCentrePx);
 * else the time alone, centred on the place. A screen reader hears the slider's own value instead.
 */
function TimeUnderPointer(props: {
  readonly place: Place;
  readonly durationMs: number;
  readonly pictures: ScrubPictures | undefined;
}): JSX.Element {
  const { language } = useTranslation().i18n;
  const { place, durationMs, pictures } = props;
  const placeMs = place.fraction * durationMs;
  const cell = pictures === undefined ? undefined : scrubCellAt(pictures, placeMs);
  const time = clockText(placeMs, language);
  if (pictures === undefined || cell === undefined) {
    return (
      <span
        className="timeline__pointed"
        style={{ insetInlineStart: `${String(place.fraction * 100)}%` }}
        aria-hidden="true"
      >
        {time}
      </span>
    );
  }
  const centrePx = pictureCentrePx(place, pictures.pictureWidth);
  return (
    <span
      className="timeline__pointed timeline__pointed--pictured"
      style={{ insetInlineStart: `${String(centrePx)}px` }}
      aria-hidden="true"
    >
      <ScrubPicture pictures={pictures} cell={cell} />
      <span className="timeline__picture-time">{time}</span>
    </span>
  );
}

/**
 * Where a picture pictureWidthPx wide is centred over place: at it, but never so near an end of the
 * slider that the picture passes it, so it stays on the stage at the film's start and end; at the
 * slider's middle where the slider is narrower than the picture.
 */
export function pictureCentrePx(place: Place, pictureWidthPx: number): number {
  const half = pictureWidthPx / 2;
  if (place.widthPx <= pictureWidthPx) {
    return place.widthPx / 2;
  }
  return Math.min(Math.max(place.fraction * place.widthPx, half), place.widthPx - half);
}

/** Where a film stands as its times show it: never past its end, once its length is known. */
export function shownAtMsOf(atMs: number, durationMs: number): number {
  return durationMs > 0 ? Math.min(atMs, durationMs) : atMs;
}
