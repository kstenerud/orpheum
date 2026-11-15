// SPDX-License-Identifier: MPL-2.0

/** A finger's taps and a mouse's clicks on the player's picture (D415, D407 §6), for the overlay. */

import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";

import { useLatest } from "./latest.ts";
import { tapSlopPx } from "./range-touch.ts";
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
  /** Whether it fell on a control (controlSelector): a button, link, field, menu or open pop-up. */
  readonly isOnControl: boolean;
  readonly x: number;
  /** When it lifted, the clock of an event's timeStamp. */
  readonly atMs: number;
}

/**
 * How long the overlay's controls take no finger's, pen's or mouse's touch after the overlay appears,
 * in ms: on a phone a second tap, meant for the picture, landed on the full-width scrub bar the first
 * tap had just shown and jumped the film. The owner chose a hold for the scrub bar (2026-11-15,
 * option a of three) and widened it to every control of the overlay, in full screen too (VQ347): a
 * tap, a click or a drag that comes down in it does nothing to its end, and one after works as
 * before. A key's or a remote's press is not held, nor the click a key makes (useOverlayHeld).
 */
export const overlayHeldForMs = 500;

/** The events of a finger, a pen or a mouse on the overlay that its hold stops (useOverlayHeld). */
const heldEvents = [
  "pointerdown",
  "pointermove",
  "pointerup",
  "pointercancel",
  "touchstart",
  "touchmove",
  "touchend",
  "touchcancel",
  "mousedown",
  "mousemove",
  "mouseup",
  "click",
  "dblclick",
  "contextmenu",
] as const;

/** Those of heldEvents a pointer makes by moving, which a held touch's end no longer stops. */
const moveEvents: ReadonlySet<string> = new Set(["pointermove", "touchmove", "mousemove"]);

/**
 * The last touch to come down on the overlay (its pointerdown): one that came down in the hold, still
 * down or lifted, or one that came down outside it.
 */
type OverlayTouch = "held" | "heldLifted" | "free";

/**
 * Holds every control of the overlay in overlayRef for overlayHeldForMs from the moment isShown turns
 * true; returns whether the hold is on, which the overlay marks data-held, its controls then taking
 * no hit (player-overlay.css). In browser full screen Chromium makes a touchstart not cancelable, so
 * the scrub bar's own slider took a held tap whatever the page's listener did (idiosyncrasies.md):
 * a held touch falls on the overlay's ground instead, and the overlay stops each of its events
 * (heldEvents) in its capture phase, so neither a control nor the page's tap readers take it: from
 * its pointerdown to its lift, a drag to its end even after the hold, and after the lift its click
 * and the mouse's events a touch makes, until the next touch comes down; a move then is no longer
 * stopped. A touch that came down before the hold keeps its click, as the tap on an offer's button
 * that shows the rest; a click a key makes (no count) is never held.
 */
export function useOverlayHeld(
  overlayRef: RefObject<HTMLElement | null>,
  isShown: boolean,
): boolean {
  const [isHeld, setIsHeld] = useState(false);
  const touchRef = useRef<OverlayTouch>("free");
  // A layout effect, so the frame that first shows the overlay holds it already.
  useLayoutEffect(() => {
    setIsHeld(isShown);
    if (!isShown) {
      return undefined;
    }
    const timer = setTimeout(() => {
      setIsHeld(false);
    }, overlayHeldForMs);
    return () => {
      clearTimeout(timer);
    };
  }, [isShown]);
  const isHeldRef = useLatest(isHeld);
  useEffect(() => {
    const overlay = overlayRef.current;
    if (overlay === null) {
      return undefined;
    }
    const stop = (event: Event): void => {
      const touch = heldTouchAfter(touchRef.current, event, isHeldRef.current);
      touchRef.current = touch;
      const isHeldNow = touch === "held" || (touch === "heldLifted" && !moveEvents.has(event.type));
      const isKeysClick =
        event.type === "click" && event instanceof MouseEvent && event.detail === 0;
      if (!isHeldNow || isKeysClick) {
        return;
      }
      event.stopPropagation();
      if (event.cancelable) {
        event.preventDefault();
      }
    };
    for (const type of heldEvents) {
      overlay.addEventListener(type, stop, { capture: true, passive: false });
    }
    return () => {
      for (const type of heldEvents) {
        overlay.removeEventListener(type, stop, { capture: true });
      }
    };
  }, [overlayRef]);
  return isHeld;
}

/** The overlay's last touch after event, the hold on or not (isHeld). */
function heldTouchAfter(touch: OverlayTouch, event: Event, isHeld: boolean): OverlayTouch {
  if (event.type === "pointerdown") {
    return isHeld ? "held" : "free";
  }
  const isLifted = event.type === "pointerup" || event.type === "pointercancel";
  return touch === "held" && isLifted ? "heldLifted" : touch;
}

/** The longest a touch may rest and be a tap, in ms: a longer one is a press, which taps nothing. */
const tapMaxMs = 500;

/**
 * How long after a touch lifts the click it makes is its tap's own, in ms: a browser sends a tap's
 * click at once, or after its own double-tap wait (300 ms) where it keeps one.
 */
const tapClickWithinMs = 700;

/** Whether a click came within a tap's click time (tapClickWithinMs) of a touch at touchAtMs. */
function isWithinATapsClick(click: MouseEvent, touchAtMs: number): boolean {
  return click.timeStamp - touchAtMs <= tapClickWithinMs;
}

/** The events a TouchClock hears. */
export const touchClockEvents = ["pointerdown", "pointerup", "touchstart", "touchend"] as const;

/**
 * When the screen was last touched, by a finger or a pen, from the page's pointer and touch events
 * (touchClockEvents, its listener heard): a click soon after is the touch's (isATouchsClick),
 * whatever pointerType it carries, as some browsers give a tap's click a mouse's or none
 * (idiosyncrasies.md).
 */
export class TouchClock {
  private lastAtMs = -Infinity;

  readonly heard = (event: Event): void => {
    const isMouse = "pointerType" in event && event.pointerType === "mouse";
    if (!isMouse) {
      this.lastAtMs = event.timeStamp;
    }
  };

  isATouchsClick(click: MouseEvent): boolean {
    return isWithinATapsClick(click, this.lastAtMs);
  }
}

/**
 * Whether a click is known to be a mouse's: a PointerEvent a mouse made (pointerType "mouse"), with
 * a count (a key's click has none), and no finger or pen on the screen just before (touches), as
 * some browsers give a tap's click a mouse's pointerType or none (idiosyncrasies.md).
 */
export function isAMousesClick(event: MouseEvent, touches: TouchClock): boolean {
  const isMouse = "pointerType" in event && event.pointerType === "mouse";
  return isMouse && event.detail > 0 && !touches.isATouchsClick(event);
}

/**
 * What a tap answers as a control's, in the player: a button, a button by role (button-by-role.tsx,
 * which opens the subtitle and settings pop-ups), a link, a field, a menu, or anything inside an open
 * pop-up, the overlay's or one the page draws over the film. The overlay's own ground, its scrim and
 * the room between its rows, is the picture's: in a window as short as a phone's on its side with the
 * browser's bar showing, it lies over three quarters of the film, and a tap there must hide it as one
 * above it does (the owner, 2026-11-14, VQ249). A tap on a button by role counted as the picture's
 * hid the overlay a moment after it opened its pop-up, and the pop-up, still modal but drawn nowhere,
 * held the whole page until Back closed it (the owner, 2026-11-14).
 */
const controlSelector =
  "button, [role='button'], a, input, select, textarea, [role='menu'], dialog";

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
  const openRef = useLatest(open);
  const touchRef = useLatest(touch);
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
    const isTapsClick = isWithinATapsClick(event, this.lastTapAtMs);
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
  const toggleRef = useLatest(toggle);
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
