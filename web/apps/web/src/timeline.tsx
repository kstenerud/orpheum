// SPDX-License-Identifier: MPL-2.0

import "./timeline.css";

import { useRef, useState, type JSX, type PointerEvent, type RefObject } from "react";

import { fractionAlong, useTouchAnywhere } from "./range-touch.ts";

/** How finely a drag along a timeline moves the film: a second. */
const dragStepMs = 1000;

/** A length of film as a player shows it: m:ss under an hour, h:mm:ss from one. */
export function clockText(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = String(seconds % 60).padStart(2, "0");
  return hours === 0
    ? `${String(minutes)}:${rest}`
    : `${String(hours)}:${String(minutes).padStart(2, "0")}:${rest}`;
}

/** Where the film stands and its length, "1:01 / 10:00"; where it alone while its length is not known. */
export function ClockTimes(props: {
  readonly atMs: number;
  readonly durationMs: number;
}): JSX.Element {
  const { atMs, durationMs } = props;
  return (
    <span className="timeline__times">
      {durationMs > 0 ? `${clockText(atMs)} / ${clockText(durationMs)}` : clockText(atMs)}
    </span>
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
 * over it shows the time under the pointer above it (TimeUnderPointer).
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
}): JSX.Element {
  const { label, atMs, durationMs, onSeek, isDisabled, hasArrowsAsCommands } = props;
  const ownRef = useRef<HTMLInputElement | null>(null);
  const inputRef = props.inputRef ?? ownRef;
  const mostMs = Math.max(durationMs, 1);
  useTouchAnywhere(
    inputRef,
    (clientX, rect) =>
      Math.round((fractionAlong(clientX, rect) * mostMs) / dragStepMs) * dragStepMs,
    onSeek,
  );
  const shownMs = durationMs > 0 ? Math.min(atMs, durationMs) : atMs;
  const [pointedFraction, setPointedFraction] = useState<number | undefined>(undefined);
  const notePointer = (event: PointerEvent<HTMLInputElement>): void => {
    setPointedFraction(
      event.pointerType === "mouse" && durationMs > 0
        ? fractionAlong(event.clientX, event.currentTarget.getBoundingClientRect())
        : undefined,
    );
  };
  return (
    <div className="timeline">
      <div className="timeline__track">
        <input
          ref={inputRef}
          type="range"
          className="timeline__range"
          aria-label={label}
          aria-valuetext={clockText(shownMs)}
          min={0}
          max={mostMs}
          step={dragStepMs}
          value={Math.min(shownMs, mostMs)}
          disabled={isDisabled}
          data-focus-arrows-are-commands={hasArrowsAsCommands === true ? "" : undefined}
          onChange={(event) => {
            onSeek(Number(event.currentTarget.value));
          }}
          onPointerMove={notePointer}
          onPointerLeave={() => {
            setPointedFraction(undefined);
          }}
        />
        {pointedFraction === undefined ? null : (
          <TimeUnderPointer fraction={pointedFraction} durationMs={durationMs} />
        )}
      </div>
      <ClockTimes atMs={shownMs} durationMs={durationMs} />
    </div>
  );
}

/**
 * The time under a mouse's pointer on a timeline, above it at the pointer (the UI review's "the
 * scrub bar's time", item 10 of the UI revamp's first wave, navigation-and-widgets-answers-2026-10-27.md): fraction along
 * the film of durationMs. A mouse's alone: a finger on it seeks at once (useTouchAnywhere), and a
 * remote's focus has the times beside it. A screen reader hears the slider's own value instead.
 */
function TimeUnderPointer(props: {
  readonly fraction: number;
  readonly durationMs: number;
}): JSX.Element {
  return (
    <span
      className="timeline__pointed"
      style={{ insetInlineStart: `${String(props.fraction * 100)}%` }}
      aria-hidden="true"
    >
      {clockText(props.fraction * props.durationMs)}
    </span>
  );
}
