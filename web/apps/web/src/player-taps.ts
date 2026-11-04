// SPDX-License-Identifier: MPL-2.0

/** A finger's taps and a mouse's clicks on the player's picture (D415, D407 §6), for the overlay. */

import { useEffect, useRef, useState, type RefObject } from "react";

import {
  doubleTapSeekMs,
  doubleTapSeekTo,
  doubleTapWaitMs,
  tapAnswer,
  tapZoneOf,
  type TapRun,
} from "./player-overlay-state.ts";

/**
 * A tap that was not a drag: where it fell on the page, and whether it fell on a control.
 */
export interface PictureTap {
  /** Whether it fell on the overlay or on a button, link or field, which a tap answers as it did. */
  readonly isOnControl: boolean;
  readonly x: number;
  /** When it lifted, the clock of an event's timeStamp. */
  readonly atMs: number;
}

/**
 * How a touch tap is told from a drag, in px: a finger that moved further is not a tap (the width of
 * a fingertip's wobble; Android's own touch slop is 8 dp).
 */
const tapSlopPx = 10;

/** The longest a touch may rest and be a tap, in ms: a longer one is a press, which taps nothing. */
const tapMaxMs = 500;

/**
 * How long after a touch lifts the click it makes is its tap's own, in ms: a browser sends a tap's
 * click at once, or after its own double-tap wait (300 ms) where it keeps one.
 */
const tapClickWithinMs = 700;

/** What a tap is on, in the player: the overlay, or a control the page draws over the film. */
const controlSelector = "[data-player-controls], button, a, input, select, textarea, [role='menu']";

interface TouchDown {
  readonly id: number;
  readonly x: number;
  readonly y: number;
  readonly atMs: number;
  readonly isHeld: boolean;
}

/**
 * Tells a finger's tap from a drag, a press and a pinch, from the pointer events a touch makes:
 * pressed notes where it came down, and lifted returns that down where it was a tap, else undefined.
 */
class TapReader {
  private down: TouchDown | undefined;

  pressed(event: PointerEvent, isHeld: boolean): void {
    // A second finger is a pinch or a spread, no tap.
    this.down =
      this.down === undefined
        ? {
            id: event.pointerId,
            x: event.clientX,
            y: event.clientY,
            atMs: event.timeStamp,
            isHeld,
          }
        : undefined;
  }

  lifted(event: PointerEvent): TouchDown | undefined {
    const start = this.down;
    this.down = undefined;
    const isTap =
      start?.id === event.pointerId &&
      Math.hypot(event.clientX - start.x, event.clientY - start.y) <= tapSlopPx &&
      event.timeStamp - start.atMs <= tapMaxMs;
    return isTap ? start : undefined;
  }

  cancelled(): void {
    this.down = undefined;
  }
}

/** What the overlay gives the pointer's hook for a finger's taps. */
export interface TouchAnswers {
  /** Whether a menu is open, which a tap that comes down on the picture then only closes. */
  readonly isHeld: () => boolean;
  readonly onPictureTap: (tap: PictureTap) => void;
}

/**
 * Opens a player's controls (open) at a mouse's move, or a click or a tap anywhere on the page, as a
 * desktop player shows its controls, the overlay hiding them again after its time: a cast's receiver
 * on a computer showed no controls to its mouse (the owner, 2026-10-27). A move that did not change
 * the pointer's place is none (a browser may move it by itself as what lies under it changes), and a
 * click a key made (no count) is the key's, which the overlay answers as a press already. Focus
 * stays where it is. The latest open is called each time.
 *
 * A finger's tap is told from its click (D415): with touch given, a tap on a control opens as a
 * click does, one on the picture is touch's own (onPictureTap), none where touch.isHeld says a menu
 * was open as it came down, so the tap that closes a menu closes only it; the click a tap makes is
 * not heard twice. A mouse's click stays as it was.
 */
export function useOpensAtThePointer(open: () => void, touch?: TouchAnswers): void {
  const openRef = useRef(open);
  const touchRef = useRef(touch);
  useEffect(() => {
    openRef.current = open;
    touchRef.current = touch;
  });
  useEffect(() => {
    const reader = new PointerReader(
      () => {
        openRef.current();
      },
      () => touchRef.current,
    );
    document.addEventListener("pointermove", reader.moved);
    document.addEventListener("pointerdown", reader.pressed);
    document.addEventListener("pointerup", reader.lifted);
    document.addEventListener("pointercancel", reader.cancelled);
    document.addEventListener("click", reader.clicked);
    return () => {
      document.removeEventListener("pointermove", reader.moved);
      document.removeEventListener("pointerdown", reader.pressed);
      document.removeEventListener("pointerup", reader.lifted);
      document.removeEventListener("pointercancel", reader.cancelled);
      document.removeEventListener("click", reader.clicked);
    };
  }, []);
}

/** What the page's pointer events say to the overlay (useOpensAtThePointer): its listeners. */
class PointerReader {
  private lastPlace: string | undefined;
  private lastTapAtMs = -Infinity;
  private readonly taps = new TapReader();

  constructor(
    private readonly open: () => void,
    private readonly touch: () => TouchAnswers | undefined,
  ) {}

  readonly moved = (event: PointerEvent): void => {
    if (event.pointerType !== "mouse") {
      return;
    }
    const place = `${String(event.screenX)},${String(event.screenY)}`;
    const isMoved = this.lastPlace !== undefined && place !== this.lastPlace;
    this.lastPlace = place;
    if (isMoved) {
      this.open();
    }
  };

  readonly pressed = (event: PointerEvent): void => {
    if (event.pointerType === "touch") {
      this.taps.pressed(event, this.touch()?.isHeld() === true);
    }
  };

  readonly lifted = (event: PointerEvent): void => {
    const tap = event.pointerType === "touch" ? this.taps.lifted(event) : undefined;
    if (tap === undefined) {
      return;
    }
    this.lastTapAtMs = event.timeStamp;
    const target = event.target instanceof Element ? event.target : null;
    const isOnControl = target?.closest(controlSelector) instanceof Element;
    const answers = this.touch();
    if (answers === undefined || isOnControl) {
      this.open();
    } else if (!tap.isHeld) {
      answers.onPictureTap({ isOnControl, x: event.clientX, atMs: event.timeStamp });
    }
  };

  readonly cancelled = (): void => {
    this.taps.cancelled();
  };

  readonly clicked = (event: MouseEvent): void => {
    const isTapsClick = event.timeStamp - this.lastTapAtMs <= tapClickWithinMs;
    if (event.detail > 0 && !isTapsClick) {
      this.open();
    }
  };
}

/** How long a double-tap seek's indicator stays after the last tap, in ms, before it fades (D415 §5). */
export const seekFeedbackShownMs = 700;

/** What a double-tap seek's indicator says: the side tapped, the run's running total, and whether it shows. */
export interface SeekFeedback {
  readonly direction: -1 | 1;
  /** The seconds the run has seeked so far, always positive: 10, 20, 30. */
  readonly seconds: number;
  /** False in the fade, which keeps the last text so it can go out. */
  readonly isShown: boolean;
}

/** The indicator's state, and what a seek calls (direction, the run's total in s) to show it and start its fade. */
function useSeekFeedback(): [
  SeekFeedback | undefined,
  (direction: -1 | 1, seconds: number) => void,
] {
  const [feedback, setFeedback] = useState<SeekFeedback | undefined>(undefined);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(
    () => () => {
      clearTimeout(timerRef.current);
    },
    [],
  );
  return [
    feedback,
    (direction, seconds) => {
      setFeedback({ direction, seconds, isShown: true });
      clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => {
        setFeedback((last) => (last === undefined ? undefined : { ...last, isShown: false }));
      }, seekFeedbackShownMs);
    },
  ];
}

/** The seconds a run of taps has seeked: its first tap seeks nothing, each later one a step. */
function seekedBy(run: TapRun | undefined): number {
  return ((run?.count ?? 2) - 1) * (doubleTapSeekMs / 1000);
}

/** Seeks a film by a double tap's side, kept inside it (D415 §3). */
function seekByTap(video: HTMLVideoElement, direction: -1 | 1): void {
  Reflect.set(video, "currentTime", doubleTapSeekTo(video.currentTime, video.duration, direction));
}

/**
 * Answers a tap on the picture (D415): a tap shows or hides the overlay (afterPictureTap), and two
 * quick ones on the left or right third seek back or forward doubleTapSeekMs, each further quick tap
 * on that side stacking another. A tap on a side waits doubleTapWaitMs for a second before it shows
 * or hides, so a double's taps do neither; the middle third does not wait. Returns what answers a tap.
 */
export function useAnswersPictureTaps(
  videoRef: RefObject<HTMLVideoElement | null>,
  toggle: () => void,
): {
  readonly onPictureTap: (tap: PictureTap) => void;
  readonly feedback: SeekFeedback | undefined;
} {
  const [feedback, noteSeek] = useSeekFeedback();
  const toggleRef = useRef(toggle);
  useEffect(() => {
    toggleRef.current = toggle;
  });
  const runRef = useRef<TapRun | undefined>(undefined);
  const waitRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(
    () => () => {
      clearTimeout(waitRef.current);
    },
    [],
  );
  const onPictureTap = (tap: PictureTap): void => {
    const video = videoRef.current;
    const width = video?.getBoundingClientRect().width ?? 0;
    const zone = tapZoneOf(tap.x, width > 0 ? width : window.innerWidth);
    const answer = tapAnswer(runRef.current, { zone, atMs: tap.atMs });
    runRef.current = answer.run;
    const isWaiting = waitRef.current !== undefined;
    clearTimeout(waitRef.current);
    waitRef.current = undefined;
    if (isWaiting && answer.action !== "seek") {
      toggleRef.current(); // a tap that does not go on a run: the one waiting was a lone tap
    }
    if (answer.action === "seek") {
      if (video !== null && answer.direction !== undefined) {
        seekByTap(video, answer.direction);
        noteSeek(answer.direction, seekedBy(answer.run));
      }
    } else if (answer.action === "toggle") {
      toggleRef.current();
    } else {
      waitRef.current = setTimeout(() => {
        waitRef.current = undefined;
        toggleRef.current();
      }, doubleTapWaitMs);
    }
  };
  return { onPictureTap, feedback };
}
