// SPDX-License-Identifier: MPL-2.0

import {
  createContext,
  use,
  useEffect,
  useRef,
  useState,
  type Context,
  type MouseEvent,
  type RefObject,
} from "react";

import { doubleTapWaitMs } from "./player-overlay-state.ts";
import { TouchClock, touchClockEvents } from "./player-taps.ts";

/**
 * What the phone's overlay gives the buttons in its centre cluster that reach into the picture's
 * side thirds, previous and next (player-spec, on a touch phone): the side's double-tap seek, back
 * (-1) or forward (1). Undefined outside a phone's overlay, where previous and next act at once.
 */
export interface SideTaps {
  /** Seeks as a double tap's run on that side does, the run tapsInRun taps long (player-taps.ts). */
  readonly seekBySide: (direction: -1 | 1, tapsInRun: number) => void;
}

/** The phone overlay's side taps (SideTaps), provided by the overlay laid out as a phone's. */
export const SideTapsContext: Context<SideTaps | undefined> = createContext<SideTaps | undefined>(
  undefined,
);

/** A run of taps on one of previous and next: how many, and when the last came, in ms. */
interface TapRun {
  readonly count: number;
  readonly lastAtMs: number;
}

/**
 * The press of previous or next (onMove) in the phone's centre cluster, where each reaches into the
 * picture's side third its double tap seeks (player-spec): a finger's tap waits doubleTapWaitMs
 * before it moves; a second tap in that time makes the pair the side's double-tap seek, and the
 * move is not made; each further quick tap seeks on. A mouse's or a key's press, or one outside a
 * phone's overlay, moves at once. Returns the button's ref, which hears the touches, and its press.
 */
export function useSideTapsOf(
  isForward: boolean,
  onMove: () => void,
): {
  readonly buttonRef: RefObject<HTMLButtonElement | null>;
  readonly onClick: (event: MouseEvent<HTMLButtonElement>) => void;
} {
  const sideTaps = use(SideTapsContext);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const [touches] = useState(() => new TouchClock());
  const runRef = useRef<TapRun | undefined>(undefined);
  const waitRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => {
    const button = buttonRef.current;
    for (const type of touchClockEvents) {
      button?.addEventListener(type, touches.heard, true);
    }
    return () => {
      clearTimeout(waitRef.current);
      for (const type of touchClockEvents) {
        button?.removeEventListener(type, touches.heard, true);
      }
    };
  }, [touches]);
  const onClick = (event: MouseEvent<HTMLButtonElement>): void => {
    if (sideTaps === undefined || !touches.isATouchsClick(event.nativeEvent)) {
      onMove();
      return;
    }
    const atMs = event.timeStamp;
    const run = runRef.current;
    if (run !== undefined && atMs - run.lastAtMs <= doubleTapWaitMs) {
      clearTimeout(waitRef.current);
      waitRef.current = undefined;
      const count = run.count + 1;
      runRef.current = { count, lastAtMs: atMs };
      sideTaps.seekBySide(isForward ? 1 : -1, count);
      return;
    }
    runRef.current = { count: 1, lastAtMs: atMs };
    waitRef.current = setTimeout(() => {
      waitRef.current = undefined;
      onMove();
    }, doubleTapWaitMs);
  };
  return { buttonRef, onClick };
}
