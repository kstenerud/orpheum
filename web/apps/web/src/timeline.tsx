// SPDX-License-Identifier: MPL-2.0

import "./timeline.css";

import { useCallback, useEffect, useRef, useState, type JSX, type RefObject } from "react";

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
 * start to its end, and its times beside it. A drag seeks, at once (settleMs 0, a local video, which
 * shows each place) or once the drag has rested settleMs (a remote's, where each seek is a command
 * to another device, as a leading player's drag is one seek). A touch anywhere on it sets it where
 * the finger is, as a volume's does (range-touch.ts), Safari on an iPhone moving a slider only from
 * its thumb. On a television its Left and Right are commands, which its player answers by seeking a
 * step (arrowsAreCommandsAttribute in focus-engine.ts), not the slider's own small steps.
 */
export function Timeline(props: {
  /** What the slider is, in the viewer's words: its accessible name. */
  readonly label: string;
  readonly atMs: number;
  /** The film's length; 0 while not known. */
  readonly durationMs: number;
  readonly onSeek: (atMs: number) => void;
  readonly isDisabled?: boolean;
  /** How long a drag rests before it seeks, in ms; 0, or none, seeks at each move. */
  readonly settleMs?: number;
  /** Whether Left and Right are the player's commands rather than the slider's own steps. */
  readonly hasArrowsAsCommands?: boolean;
  /** The slider, for a player that moves focus to it or asks whether it has it. */
  readonly inputRef?: RefObject<HTMLInputElement | null>;
}): JSX.Element {
  const { label, atMs, durationMs, onSeek, isDisabled, settleMs, hasArrowsAsCommands } = props;
  const ownRef = useRef<HTMLInputElement | null>(null);
  const inputRef = props.inputRef ?? ownRef;
  const [draftMs, setDraftMs] = useState<number | undefined>(undefined);
  const move = useSettledSeek(onSeek, settleMs ?? 0, setDraftMs);
  const mostMs = Math.max(durationMs, 1);
  useTouchAnywhere(
    inputRef,
    (clientX, rect) =>
      Math.round((fractionAlong(clientX, rect) * mostMs) / dragStepMs) * dragStepMs,
    move,
  );
  const atNowMs = draftMs ?? atMs;
  const shownMs = durationMs > 0 ? Math.min(atNowMs, durationMs) : atNowMs;
  return (
    <div className="timeline">
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
          move(Number(event.currentTarget.value));
        }}
      />
      <ClockTimes atMs={shownMs} durationMs={durationMs} />
    </div>
  );
}

/**
 * What a move along the timeline does: seeks at once where settleMs is 0, else shows the place
 * (onDraft) and seeks once no move has come for settleMs. A drag left unsettled as the timeline goes
 * seeks nothing.
 */
function useSettledSeek(
  onSeek: (atMs: number) => void,
  settleMs: number,
  onDraft: (atMs: number | undefined) => void,
): (atMs: number) => void {
  const seekRef = useRef(onSeek);
  useEffect(() => {
    seekRef.current = onSeek;
  });
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(
    () => () => {
      clearTimeout(timerRef.current);
    },
    [],
  );
  return useCallback(
    (atMs: number) => {
      if (settleMs <= 0) {
        seekRef.current(atMs);
        return;
      }
      onDraft(atMs);
      clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => {
        onDraft(undefined);
        seekRef.current(atMs);
      }, settleMs);
    },
    [settleMs, onDraft],
  );
}
