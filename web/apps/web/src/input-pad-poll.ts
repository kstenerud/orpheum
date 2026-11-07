// SPDX-License-Identifier: MPL-2.0

/**
 * The controllers connected, polled padPollEveryMs apart only while one is and someone listens
 * (controller-input-2026-10-25.md §6.1 item 5), once for every listener: the app's input layer, which turns them into commands, and
 * the device lab's button test, which logs them. Each pad is read by input-pads.ts for its layout and
 * by the OK the person chose (controller-choice.ts); each button told as it goes down and as it goes
 * up, a standard pad's left stick as a direction as it leans past half way, its stick or D-pad as an
 * arrow that repeats while held and is let go, the controllers' driver of the input layer's holds
 * (CommandHold in input-commands.ts), and a pad with no standard mapping's axes as they cross half
 * way either way. Proven by
 * the button test (plans/tv-device-tests.md H8).
 */

import { chosenOkButton } from "./controller-choice.ts";
import { isSeekPadButton, type Direction, type OkButton } from "./input-buttons.ts";
import {
  axisLean,
  buttonRepeatStep,
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
  /**
   * A seek button held on, told as its press was at each repeat (input-pads.ts): the buttons that
   * seek repeat as a stick does, so a held one seeks by the time held, and no other does.
   */
  readonly onButtonRepeat: (press: PadButtonPress) => void;
  /** A button going up, told as its press was; also a held seek button's pad gone. */
  readonly onButtonUp: (press: PadButtonPress) => void;
  readonly onAxis: (crossing: AxisCrossing) => void;
  /**
   * A standard pad's arrow, from its left stick or its D-pad: as it is pushed a new way (isRepeat
   * false), then repeating while held (input-pads.ts).
   */
  readonly onArrow: (direction: Direction, isRepeat: boolean) => void;
  /**
   * A standard pad's arrow let go: its stick back at its centre or turned another way, its D-pad's
   * button up, or its pad gone.
   */
  readonly onArrowUp: (direction: Direction) => void;
  /** The pads as read this poll. */
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

/** What the poll keeps of a pad between polls. */
interface PadMemory {
  readonly id: string;
  readonly pressed: readonly number[];
  readonly leans: readonly number[];
  readonly hold: StickHold;
  /** The seek buttons held, each with when it next repeats. */
  readonly repeats: ReadonlyMap<number, number>;
  /** The pad as last read, to tell a held button's release with if it is gone. */
  readonly pad: PadState;
}

/** A pad read this poll, beside what the poll kept of it from the last. */
interface PadPolled {
  readonly pad: PadState;
  readonly before: PadMemory | undefined;
  readonly nowMs: number;
}

/** Pad's button as a press tells it. */
function buttonPressOf(pad: PadState, button: number): PadButtonPress {
  return {
    kind: "pad",
    index: pad.index,
    pad: pad.id,
    layout: pad.layout,
    button,
    isStandard: pad.isStandard,
    okButton: pad.okButton,
  };
}

/** Tells handlers each of the pad's buttons that went down, then each that went up, since the last poll. */
function tellButtons({ pad, before }: PadPolled, handlers: PadHandlers): void {
  const pressedBefore = before?.pressed ?? [];
  for (const button of pad.pressed.filter((each) => !pressedBefore.includes(each))) {
    handlers.onPress(buttonPressOf(pad, button));
  }
  for (const button of pressedBefore.filter((each) => !pad.pressed.includes(each))) {
    handlers.onButtonUp(buttonPressOf(pad, button));
  }
}

/**
 * Tells handlers each axis whose lean changed since the last poll: a pad with no standard mapping's
 * every axis, either way; a standard pad's left stick as it leans out. The leans, to keep.
 */
function tellAxes({ pad, before }: PadPolled, handlers: PadHandlers): number[] {
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
 * A standard pad's seek buttons held after this poll, each with when it next repeats: told to
 * handlers at each repeat. A button not held, or a pad not standard, is kept no more.
 */
function nextRepeats(
  { pad, before, nowMs }: PadPolled,
  handlers: PadHandlers,
): ReadonlyMap<number, number> {
  const repeats = new Map<number, number>();
  if (!pad.isStandard) {
    return repeats;
  }
  for (const button of pad.pressed.filter((each) => isSeekPadButton(each))) {
    const step = buttonRepeatStep(before?.repeats.get(button), nowMs);
    repeats.set(button, step.nextAtMs);
    if (step.shouldRepeat) {
      handlers.onButtonRepeat(buttonPressOf(pad, button));
    }
  }
  return repeats;
}

/**
 * A standard pad's arrow's hold after this poll, its left stick's or else its D-pad's: told to
 * handlers as it is let go, and as it moves, a repeat where it was held that way already.
 */
function nextHold({ pad, before, nowMs }: PadPolled, handlers: PadHandlers): StickHold {
  const hold = before?.hold ?? stickAtRest;
  if (!pad.isStandard) {
    return hold;
  }
  const direction =
    stickDirectionOf(pad.axes[0] ?? 0, pad.axes[1] ?? 0) ?? dpadDirectionOf(pad.pressed);
  const step = stickStep(hold, direction, nowMs);
  tellArrow(hold, step, handlers);
  return step.hold;
}

/**
 * Tells handlers how a pad's arrow went from its hold to step's: let go where it was held another way
 * or none, then moved where step says, a repeat where it was held that way already.
 */
function tellArrow(
  hold: StickHold,
  step: ReturnType<typeof stickStep>,
  handlers: PadHandlers,
): void {
  if (hold.direction !== undefined && step.hold.direction !== hold.direction) {
    handlers.onArrowUp(hold.direction);
  }
  if (step.shouldMove && step.hold.direction !== undefined) {
    handlers.onArrow(step.hold.direction, step.hold.direction === hold.direction);
  }
}

/** Tells handlers what changed on the polled pad since the last; what to keep for the next. */
function tellChanges(polled: PadPolled, handlers: PadHandlers): PadMemory {
  if (polled.before?.id !== polled.pad.id) {
    handlers.onSeen(polled.pad);
  }
  tellButtons(polled, handlers);
  const leans = tellAxes(polled, handlers);
  const hold = nextHold(polled, handlers);
  const repeats = nextRepeats(polled, handlers);
  return { id: polled.pad.id, pressed: polled.pad.pressed, leans, hold, repeats, pad: polled.pad };
}

/**
 * Lets go the arrow and the seek buttons of each pad gone since the last poll where they were held,
 * telling their release: a controller unplugged or asleep with its stick or trigger held leaves no
 * hold on.
 */
function letGoneGo(read: readonly PadState[], handlers: PadHandlers): void {
  for (const [index, memory] of memories) {
    if (read.some((pad) => pad.index === index)) {
      continue;
    }
    memories.set(index, { ...memory, hold: stickAtRest, repeats: new Map() });
    if (memory.hold.direction !== undefined) {
      handlers.onArrowUp(memory.hold.direction);
    }
    for (const button of memory.repeats.keys()) {
      handlers.onButtonUp(buttonPressOf(memory.pad, button));
    }
  }
}

/** Each listener, the newest first: a page hears a pad before the app's navigation acts on it. */
const listeners: Partial<PadHandlers>[] = [];

/** The listeners not yet told of the pads seen before they came. */
const toGreet = new Set<Partial<PadHandlers>>();

/** What the poll keeps of each pad between polls, by its index. */
const memories = new Map<number, PadMemory>();

/**
 * How long the poll waits between reads of the pads, in milliseconds: about 15 a second, not each
 * animation frame, which cost a main-thread frame each vsync on every page while a controller was
 * connected (web-client-efficiency-audit-2026-10-28.md). "Poll 15x per second and we'll see if
 * people complain. Also measure" (the owner, 2026-11-07): padPollCost measures it. A press shorter
 * than this may go unread; a hold's repeats keep their times, which are measured, not counted in
 * polls (input-pads.ts).
 */
export const padPollEveryMs = Math.floor(1000 / 15);

/** The timer polling the pads, while one is. */
let polling: ReturnType<typeof setInterval> | undefined;

/** What the polls have cost on this page: how many, and the main thread's time in them. */
export interface PadPollCost {
  readonly polls: number;
  readonly spentMs: number;
}

/** The polls' cost so far, added to by each poll. */
let cost: PadPollCost = { polls: 0, spentMs: 0 };

/**
 * The polls' cost on this page so far, for a page to show (the button test, lab-pad-input.tsx): the
 * owner asked it measured (2026-11-07). Read twice, the difference is a span's.
 */
export function padPollCost(): PadPollCost {
  return cost;
}

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
  onButtonRepeat: (press) => {
    for (const listener of [...listeners]) {
      listener.onButtonRepeat?.(press);
    }
  },
  onButtonUp: (press) => {
    for (const listener of [...listeners]) {
      listener.onButtonUp?.(press);
    }
  },
  onArrow: (direction, isRepeat) => {
    for (const listener of [...listeners]) {
      listener.onArrow?.(direction, isRepeat);
    }
  },
  onArrowUp: (direction) => {
    for (const listener of [...listeners]) {
      listener.onArrowUp?.(direction);
    }
  },
  onRead: (pads) => {
    for (const listener of [...listeners]) {
      listener.onRead?.(pads);
    }
  },
};

/** Tells each listener new since the last poll of the pads seen before it came. */
function greet(pads: readonly PadState[]): void {
  for (const listener of toGreet) {
    for (const pad of pads.filter((each) => memories.get(each.index)?.id === each.id)) {
      listener.onSeen?.(pad);
    }
  }
  toGreet.clear();
}

/** Stops polling the pads, where it does. */
function stopPolling(): void {
  if (polling !== undefined) {
    clearInterval(polling);
    polling = undefined;
  }
}

/**
 * One poll: what changed told and its time added to the cost; polling stopped once no pad is
 * connected or no one listens.
 */
function poll(): void {
  const nowMs = performance.now();
  const read = readPads(chosenOkButton());
  greet(read);
  letGoneGo(read, everyListener);
  for (const pad of read) {
    const polled = { pad, before: memories.get(pad.index), nowMs };
    memories.set(pad.index, tellChanges(polled, everyListener));
  }
  everyListener.onRead(read);
  cost = { polls: cost.polls + 1, spentMs: cost.spentMs + (performance.now() - nowMs) };
  if (read.length === 0 || listeners.length === 0) {
    stopPolling();
  }
}

/** Polls padPollEveryMs from now and each padPollEveryMs on, unless it already does. */
function pollSoon(): void {
  polling ??= setInterval(poll, padPollEveryMs);
}

/**
 * Tells listener what the pads do, polling them padPollEveryMs apart while one is connected; returns the
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
    stopPolling();
    memories.clear();
  };
}
