// SPDX-License-Identifier: MPL-2.0

/**
 * The button test's controllers (plans/tv-device-tests.md H8): the pads connected, polled each frame
 * only while one is (controller-input §6.1 item 5), each read by input-pads.ts for its layout; each
 * button told as it goes down, a standard pad's left stick as a direction as it leans past half way
 * and as a repeating arrow while held, and a pad with no standard mapping's axes as they cross half
 * way either way.
 */

import { useCallback, useEffect, useMemo, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import type { ButtonName, Direction, OkButton } from "./input-buttons.ts";
import type { FocusMode, NotedEvent } from "./lab-events.ts";
import { geometricFocus, moveFocus } from "./focus-engine.ts";
import {
  axisLean,
  okButtonOf,
  padLayoutOf,
  padMappingOf,
  stickAtRest,
  stickDirectionOf,
  stickStep,
  type PadLayout,
  type StickHold,
} from "./input-pads.ts";

/** One pad as the test last read it. */
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

/** What the test is told of its pads. */
export interface PadHandlers {
  /** A pad seen for the first time, or again with another name in its place. */
  readonly onSeen: (pad: PadState) => void;
  readonly onPress: (press: PadButtonPress | StickPress) => void;
  readonly onAxis: (crossing: AxisCrossing) => void;
  /** A stick's arrow: as it leans a new way, then repeating while held (input-pads.ts). */
  readonly onStickArrow: (direction: Direction) => void;
}

/** The pads navigator.getGamepads gives, each read with the OK the person chose for an unknown one. */
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

/** A standard pad's stick's hold after this frame, its arrow told to onStickArrow when it moves. */
function nextHold(
  { pad, before, nowMs }: PadFrame,
  onStickArrow: PadHandlers["onStickArrow"],
): StickHold {
  const hold = before?.hold ?? stickAtRest;
  if (!pad.isStandard) {
    return hold;
  }
  const step = stickStep(hold, stickDirectionOf(pad.axes[0] ?? 0, pad.axes[1] ?? 0), nowMs);
  if (step.shouldMove && step.hold.direction !== undefined) {
    onStickArrow(step.hold.direction);
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
  const hold = nextHold(frame, handlers.onStickArrow);
  return { id: frame.pad.id, pressed: frame.pad.pressed, leans, hold };
}

/**
 * The pads connected, polled each frame only while one is, each change told to handlers; chosen is
 * the OK the person chose for a pad of unknown layout.
 */
export function usePads(handlers: PadHandlers, chosen: OkButton | undefined): readonly PadState[] {
  const [pads, setPads] = useState<readonly PadState[]>([]);
  const chosenRef = useRef(chosen);
  useEffect(() => {
    chosenRef.current = chosen;
  }, [chosen]);
  useEffect(() => {
    if (!("getGamepads" in navigator)) {
      return undefined;
    }
    let frame: number | undefined;
    const memories = new Map<number, PadMemory>();
    // The pads as last shown, so the page draws again only when one changed, not each frame.
    let shown = "";
    const poll = (nowMs: number): void => {
      const read = readPads(chosenRef.current);
      for (const pad of read) {
        const padFrame = { pad, before: memories.get(pad.index), nowMs };
        memories.set(pad.index, tellChanges(padFrame, handlers));
      }
      const reading = JSON.stringify(read);
      if (reading !== shown) {
        shown = reading;
        setPads(read);
      }
      frame = read.length > 0 ? requestAnimationFrame(poll) : undefined;
    };
    const connected = (): void => {
      frame ??= requestAnimationFrame(poll);
    };
    globalThis.addEventListener("gamepadconnected", connected);
    globalThis.addEventListener("gamepaddisconnected", connected);
    // A pad the page already knows is announced no more: a browser fires gamepadconnected once a
    // document, so a test opened again within the app reads the pads there are (the owner, Firefox
    // on a Mac, 2026-10-25: back and in again, the controller was heard no more).
    if (readPads(chosenRef.current).length > 0) {
      connected();
    }
    return () => {
      globalThis.removeEventListener("gamepadconnected", connected);
      globalThis.removeEventListener("gamepaddisconnected", connected);
      if (frame !== undefined) {
        cancelAnimationFrame(frame);
      }
    };
  }, [handlers]);
  return pads;
}

/** How much of a controller's name the log keeps: the server's bound (lab.go). A justified constant. */
export const padIdKept = 100;

/** When something came, on the page's clock, in whole milliseconds. */
function nowMs(): number {
  return Math.round(performance.now());
}

/** What the test does with its pads: a press is noted, a pad seen and an axis sent, a stick's arrow moves. */
export function usePadHandlers(
  note: (press: PadButtonPress | StickPress) => void,
  noteEvent: (event: NotedEvent) => void,
  onStickArrow: (direction: Direction) => void,
): PadHandlers {
  return useMemo(
    () => ({
      onSeen: (pad) => {
        noteEvent({
          kind: "pad",
          padIndex: pad.index,
          padId: pad.id.slice(0, padIdKept),
          padMapping: padMappingOf(pad.mapping),
          padLayout: pad.layout,
          okButton: pad.okButton,
          atMs: nowMs(),
        });
      },
      onPress: (press) => {
        note(press);
      },
      onAxis: (crossing) => {
        noteEvent({
          kind: "axis",
          padIndex: crossing.index,
          axisIndex: crossing.axis,
          axisValue: crossing.value,
          atMs: nowMs(),
        });
      },
      onStickArrow,
    }),
    [note, noteEvent, onStickArrow],
  );
}

/**
 * A pad's buttons and stick driving our navigation as the arrows do, while it is the mode: the
 * D-pad and the stick move focus, the layout's OK presses what has it; each move timed as a key's.
 */
export function usePadNavigation(
  mode: FocusMode,
  startRef: RefObject<HTMLDivElement | null>,
  noteKey: (atMs: number) => void,
): {
  readonly onPadButton: (button: ButtonName) => void;
  readonly onStickArrow: (direction: Direction) => void;
} {
  const modeRef = useRef(mode);
  useEffect(() => {
    modeRef.current = mode;
  }, [mode]);
  const onStickArrow = useCallback(
    (direction: Direction) => {
      const start = startRef.current;
      if (modeRef.current !== "geometric" || start === null) {
        return;
      }
      noteKey(performance.now());
      moveFocus({ region: document.body, engine: geometricFocus, start }, direction);
    },
    [startRef, noteKey],
  );
  const onPadButton = useCallback(
    (button: ButtonName) => {
      if (button === "up" || button === "down" || button === "left" || button === "right") {
        onStickArrow(button);
        return;
      }
      const focused = document.activeElement;
      if (
        button === "select" &&
        modeRef.current === "geometric" &&
        focused instanceof HTMLElement &&
        focused !== document.body
      ) {
        focused.click();
      }
    },
    [onStickArrow],
  );
  return { onPadButton, onStickArrow };
}

/** How much of a controller's name the page shows on its one line: enough to know it by. */
const padIdShown = 32;

/** The first controller connected, on one line: its name, its mapping, its layout, and its OK. */
export function PadLine(props: {
  readonly pads: readonly PadState[];
  readonly onChoose: (okButton: OkButton) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const pad = props.pads[0];
  if (pad === undefined) {
    return <p className="lab__small">{t("lab.input.controller.none")}</p>;
  }
  const id = pad.id.length > padIdShown ? `${pad.id.slice(0, padIdShown - 1)}…` : pad.id;
  // Every standard pad offers the choice: a pad's labels need not match the layout it reports.
  const isChosen = pad.isStandard;
  const layout = t(`lab.input.controller.layout.${pad.layout}`);
  const ok = t(`lab.input.controller.ok.${pad.okButton}`);
  let said = t("lab.input.controller.noStandard");
  if (pad.isStandard) {
    said =
      pad.layout === "unknown"
        ? t("lab.input.controller.unknown")
        : t("lab.input.controller.standard", { layout, ok });
  }
  return (
    <div className="lab__controls lab__small">
      <span>
        {t("lab.input.controller.line", { id, mapping: padMappingOf(pad.mapping), said })}
      </span>
      {isChosen ? (
        <fieldset className="lab__modes">
          <legend>{t("lab.input.controller.choose")}</legend>
          {(["bottom", "right"] as const).map((each) => (
            <button
              key={each}
              type="button"
              className="lab__button"
              aria-pressed={pad.okButton === each}
              onClick={() => {
                props.onChoose(each);
              }}
            >
              {t(`lab.input.controller.ok.${each}`)}
            </button>
          ))}
        </fieldset>
      ) : null}
    </div>
  );
}
