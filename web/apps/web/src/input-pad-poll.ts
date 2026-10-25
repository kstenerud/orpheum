// SPDX-License-Identifier: MPL-2.0

/**
 * The controllers connected, polled each frame only while one is (controller-input-2026-10-25.md
 * §6.1 item 5), once for every listener: the app's input layer, which turns them into commands, and
 * the device lab's button test, which logs them. Each pad is read by input-pads.ts for its layout and
 * by the OK the person chose (controller-choice.ts); each button told as it goes down, a standard
 * pad's left stick as a direction as it leans past half way, its stick or D-pad as a repeating arrow
 * while held, and a pad with no standard mapping's axes as they cross half way either way. Proven by
 * the button test (plans/tv-device-tests.md H8).
 */

import { chosenOkButton } from "./controller-choice.ts";
import type { Direction, OkButton } from "./input-buttons.ts";
import {
  axisLean,
  dpadDirectionOf,
  okButtonOf,
  padLayoutOf,
  stickAtRest,
  stickDirectionOf,
  stickStep,
  type PadLayout,
  type StickHold,
} from "./input-pads.ts";

/** One pad as the poll last read it. */
export interface PadState {
  readonly index: number;
  readonly id: string;
  readonly mapping: string;
  readonly isStandard: boolean;
  readonly layout: PadLayout;
  readonly okButton: OkButton;
  readonly pressed: readonly number[];
  readonly axes: readonly number[];
}

/** What a pad's press says of the pad. */
interface PadOf {
  readonly index: number;
  readonly pad: string;
  readonly layout: PadLayout;
}

/** A pad's button going down. */
export interface PadButtonPress extends PadOf {
  readonly kind: "pad";
  readonly button: number;
  readonly isStandard: boolean;
  readonly okButton: OkButton;
}

/** A standard pad's left stick leaning past half way along one axis. */
export interface StickPress extends PadOf {
  readonly kind: "stick";
  readonly direction: Direction;
  readonly axis: number;
  readonly value: number;
}

/** A pad with no standard mapping's axis crossing half way, either way. */
export interface AxisCrossing {
  readonly index: number;
  readonly pad: string;
  readonly axis: number;
  readonly value: number;
}

/** What a listener is told of the pads. */
export interface PadHandlers {
  /** A pad seen for the first time, or again with another name in its place. */
  readonly onSeen: (pad: PadState) => void;
  readonly onPress: (press: PadButtonPress | StickPress) => void;
  readonly onAxis: (crossing: AxisCrossing) => void;
  /**
   * A standard pad's arrow, from its left stick or its D-pad: as it is pushed a new way, then
   * repeating while held (input-pads.ts).
   */
  readonly onArrow: (direction: Direction) => void;
  /** The pads as read this frame. */
  readonly onRead: (pads: readonly PadState[]) => void;
}

/** The pads navigator.getGamepads gives, each read with the OK the person chose, where they did. */
function readPads(chosen: OkButton | undefined): PadState[] {
  const pads: PadState[] = [];
  for (const pad of navigator.getGamepads()) {
    if (pad !== null) {
      const layout = padLayoutOf(pad.id);
      pads.push({
        index: pad.index,
        id: pad.id,
        mapping: pad.mapping,
        isStandard: pad.mapping === "standard",
        layout,
        okButton: okButtonOf(layout, chosen),
        pressed: pad.buttons.flatMap((button, index) => (button.pressed ? [index] : [])),
        axes: pad.axes.map((axis) => Math.round(axis * 100) / 100),
      });
    }
  }
  return pads;
}

/** The stick's axes, x and y, that lean as arrows on a standard pad. */
const stickAxes: readonly number[] = [0, 1];

/** The direction an axis of the left stick leans, by its axis and lean. */
function stickDirectionOfAxis(axis: number, lean: -1 | 1): Direction {
  if (axis === 0) {
    return lean > 0 ? "right" : "left";
  }
  return lean > 0 ? "down" : "up";
}

/** What the poll keeps of a pad between frames. */
interface PadMemory {
  readonly id: string;
  readonly pressed: readonly number[];
  readonly leans: readonly number[];
  readonly hold: StickHold;
}

/** A pad read this frame, beside what the poll kept of it from the last. */
interface PadFrame {
  readonly pad: PadState;
  readonly before: PadMemory | undefined;
  readonly nowMs: number;
}

/** Tells onPress each of the pad's buttons that went down since the last frame. */
function tellButtons({ pad, before }: PadFrame, onPress: PadHandlers["onPress"]): void {
  const pressedBefore = before?.pressed ?? [];
  for (const button of pad.pressed.filter((each) => !pressedBefore.includes(each))) {
    onPress({
      kind: "pad",
      index: pad.index,
      pad: pad.id,
      layout: pad.layout,
      button,
      isStandard: pad.isStandard,
      okButton: pad.okButton,
    });
  }
}

/**
 * Tells handlers each axis whose lean changed since the last frame: a pad with no standard mapping's
 * every axis, either way; a standard pad's left stick as it leans out. The leans, to keep.
 */
function tellAxes({ pad, before }: PadFrame, handlers: PadHandlers): number[] {
  const leans = pad.axes.map((value) => axisLean(value));
  for (const [axis, lean] of leans.entries()) {
    const value = pad.axes[axis] ?? 0;
    if (lean === (before?.leans[axis] ?? 0)) {
      continue;
    }
    if (!pad.isStandard) {
      handlers.onAxis({ index: pad.index, pad: pad.id, axis, value });
    } else if (stickAxes.includes(axis) && lean !== 0) {
      const direction = stickDirectionOfAxis(axis, lean);
      const of = { index: pad.index, pad: pad.id, layout: pad.layout };
      handlers.onPress({ kind: "stick", ...of, direction, axis, value });
    }
  }
  return leans;
}

/**
 * A standard pad's arrow's hold after this frame, its left stick's or else its D-pad's, its arrow
 * told to onArrow as it moves.
 */
function nextHold({ pad, before, nowMs }: PadFrame, onArrow: PadHandlers["onArrow"]): StickHold {
  const hold = before?.hold ?? stickAtRest;
  if (!pad.isStandard) {
    return hold;
  }
  const direction =
    stickDirectionOf(pad.axes[0] ?? 0, pad.axes[1] ?? 0) ?? dpadDirectionOf(pad.pressed);
  const step = stickStep(hold, direction, nowMs);
  if (step.shouldMove && step.hold.direction !== undefined) {
    onArrow(step.hold.direction);
  }
  return step.hold;
}

/** Tells handlers what changed on the frame's pad since the last; what to keep for the next. */
function tellChanges(frame: PadFrame, handlers: PadHandlers): PadMemory {
  if (frame.before?.id !== frame.pad.id) {
    handlers.onSeen(frame.pad);
  }
  tellButtons(frame, handlers.onPress);
  const leans = tellAxes(frame, handlers);
  const hold = nextHold(frame, handlers.onArrow);
  return { id: frame.pad.id, pressed: frame.pad.pressed, leans, hold };
}

/** Each listener, the newest first: a page hears a pad before the app's navigation acts on it. */
const listeners: Partial<PadHandlers>[] = [];

/** The listeners not yet told of the pads seen before they came. */
const toGreet = new Set<Partial<PadHandlers>>();

/** What the poll keeps of each pad between frames, by its index. */
const memories = new Map<number, PadMemory>();

/** The frame the next poll is asked for, while one is. */
let frame: number | undefined;

/** The handlers telling every listener, the newest first. */
const everyListener: PadHandlers = {
  onSeen: (pad) => {
    for (const listener of [...listeners]) {
      listener.onSeen?.(pad);
    }
  },
  onPress: (press) => {
    for (const listener of [...listeners]) {
      listener.onPress?.(press);
    }
  },
  onAxis: (crossing) => {
    for (const listener of [...listeners]) {
      listener.onAxis?.(crossing);
    }
  },
  onArrow: (direction) => {
    for (const listener of [...listeners]) {
      listener.onArrow?.(direction);
    }
  },
  onRead: (pads) => {
    for (const listener of [...listeners]) {
      listener.onRead?.(pads);
    }
  },
};

/** Tells each listener new since the last frame of the pads seen before it came. */
function greet(pads: readonly PadState[]): void {
  for (const listener of toGreet) {
    for (const pad of pads.filter((each) => memories.get(each.index)?.id === each.id)) {
      listener.onSeen?.(pad);
    }
  }
  toGreet.clear();
}

/** One frame's poll: what changed told, and the next asked for while a pad is connected. */
function poll(nowMs: number): void {
  frame = undefined;
  const read = readPads(chosenOkButton());
  greet(read);
  for (const pad of read) {
    const padFrame = { pad, before: memories.get(pad.index), nowMs };
    memories.set(pad.index, tellChanges(padFrame, everyListener));
  }
  everyListener.onRead(read);
  if (read.length > 0 && listeners.length > 0) {
    frame = requestAnimationFrame(poll);
  }
}

/** Polls from the next frame, unless it already does. */
function pollSoon(): void {
  frame ??= requestAnimationFrame(poll);
}

/**
 * Tells listener what the pads do, polling them each frame while one is connected; returns the
 * stopping. A pad the page already knows is announced no more: a browser fires gamepadconnected once
 * a document, so a page opened again within the app reads the pads there are (the owner, Firefox on
 * a Mac, 2026-10-25: back and in again, the controller was heard no more). Nothing where the browser
 * has no Gamepad API.
 */
export function listenToPads(listener: Partial<PadHandlers>): () => void {
  if (!("getGamepads" in navigator)) {
    return () => undefined;
  }
  if (listeners.length === 0) {
    globalThis.addEventListener("gamepadconnected", pollSoon);
    globalThis.addEventListener("gamepaddisconnected", pollSoon);
  }
  listeners.unshift(listener);
  toGreet.add(listener);
  if (readPads(chosenOkButton()).length > 0) {
    pollSoon();
  }
  return () => {
    const at = listeners.indexOf(listener);
    if (at >= 0) {
      listeners.splice(at, 1);
    }
    toGreet.delete(listener);
    if (listeners.length > 0) {
      return;
    }
    globalThis.removeEventListener("gamepadconnected", pollSoon);
    globalThis.removeEventListener("gamepaddisconnected", pollSoon);
    if (frame !== undefined) {
      cancelAnimationFrame(frame);
      frame = undefined;
    }
    memories.clear();
  };
}
