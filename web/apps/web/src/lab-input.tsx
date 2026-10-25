// SPDX-License-Identifier: MPL-2.0

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type JSX,
  type ReactNode,
  type Ref,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { buttonOf, type ButtonName, type Direction, type OkButton } from "./input-buttons.ts";
import { chipsShown } from "./lab-buttons.ts";
import {
  useInputEvents,
  type EventSending,
  type FocusMode,
  type LabInputEvent,
  type NotedEvent,
} from "./lab-events.ts";
import { FieldTest, useViewportChanges } from "./lab-field.tsx";
import { LabFrame, yesNo } from "./lab-frame.tsx";
import { caretOf, driveFocus, geometricFocus } from "./focus-engine.ts";
import { FullscreenTest, fullscreenElementOf, leaveFullscreenAt } from "./lab-fullscreen.tsx";
import { heardKeyOf, keyEntryOf, withKey, type KeyEntry } from "./lab-keys.ts";
import {
  PadLine,
  padIdKept,
  usePadHandlers,
  usePadNavigation,
  usePads,
  type PadButtonPress,
  type PadState,
  type StickPress,
} from "./lab-pad-input.tsx";
import { padButtonNameOf } from "./input-pads.ts";
import { eventTimeMs, latencySummary, type LatencySummary } from "./lab-timing.ts";
import { toneUrl } from "./lab-tone.ts";
import { useServices } from "./services.tsx";

/** Both modes, in the order the page offers them. */
const focusModes: readonly FocusMode[] = ["geometric", "browser"];

/** The Media Session actions the probe listens for. */
const mediaActions: readonly NonNullable<LabInputEvent["action"]>[] = [
  "play",
  "pause",
  "seekbackward",
  "seekforward",
  "previoustrack",
  "nexttrack",
];

/**
 * The tiles' size: one row of six, with the text field and the full-screen test below them, enough
 * to try every arrow while the whole page fits a Fire TV's 960 × 540 without scrolling (the owner,
 * 2026-10-25). A justified constant, the page's layout.
 */
const tileColumns = 6;
const tileRows = 1;

/** A key that came longer ago than this before focus moved did not move it, in milliseconds. */
const keyMovesFocusWithinMs = 1000;

/** How many moves' latencies a mode keeps: a long walk across the tiles. */
const latenciesKept = 200;

/** One press the page heard, with what the last-heard line shows of it and the log keeps. */
type Press =
  | {
      readonly kind: "key";
      readonly key: string;
      readonly code: string;
      readonly keyCode: number;
      readonly isRepeat: boolean;
      readonly isInField: boolean;
    }
  | { readonly kind: "historyStep" }
  | PadButtonPress
  | StickPress
  | { readonly kind: "media"; readonly action: NonNullable<LabInputEvent["action"]> };

/** A press told on, with when it came on the page's clock, in milliseconds. */
type OnPress = (press: Press, atMs?: number) => void;

/** An event's fields a press gives, before when it came. */
type PressFields = Omit<NotedEvent, "atMs">;

/**
 * A key's fields as the log keeps them: in the text field, a key that is one character is a
 * character typed, sent as Character with no code or keyCode, which would spell it.
 */
function keyFieldsOf(press: Extract<Press, { kind: "key" }>): PressFields {
  const isTyped = press.isInField && /^.$/u.test(press.key);
  return {
    kind: "key",
    key: isTyped ? "Character" : press.key.slice(0, keyTextKept),
    ...(isTyped ? {} : { code: press.code.slice(0, keyTextKept), keyCode: press.keyCode }),
    isRepeat: press.isRepeat,
    ...(press.isInField ? { inField: true } : {}),
  };
}

/** The fields of a press's event its kind gives. */
function fieldsOfPress(press: Press): PressFields {
  switch (press.kind) {
    case "key":
      return keyFieldsOf(press);
    case "historyStep":
      return { kind: "back" };
    case "pad": {
      const name = press.isStandard ? padButtonNameOf(press.button) : undefined;
      return {
        kind: "gamepad",
        padIndex: press.index,
        padButton: press.button,
        ...(name === undefined ? {} : { padButtonName: name }),
        padId: press.pad.slice(0, padIdKept),
        padLayout: press.layout,
      };
    }
    case "stick":
      return {
        kind: "stick",
        padIndex: press.index,
        axisIndex: press.axis,
        axisValue: press.value,
        padLayout: press.layout,
      };
    case "media":
      return { kind: "media", action: press.action };
  }
}

/** The event the server logs of a press the page heard as button, at atMs. */
function eventOfPress(press: Press, button: ButtonName, atMs: number): NotedEvent {
  return {
    ...fieldsOfPress(press),
    button,
    atMs: Math.round(atMs),
    ...(fullscreenElementOf() === null ? {} : { inFullscreen: true }),
  };
}

/** How much of a key's key and code the log keeps: the server's bound (lab.go). A justified constant. */
const keyTextKept = 40;

/** The buttons heard, in the order first heard, and the last press. */
interface HeardButtons {
  readonly buttons: readonly ButtonName[];
  readonly last: { readonly press: Press; readonly button: ButtonName } | undefined;
  /** Every press heard, of whichever button. */
  readonly pressCount: number;
}

/** The buttons the page has heard, and the noting of each press, each told to onButton too. */
function useHeardButtons(onButton: (press: Press, button: ButtonName, atMs: number) => void): {
  readonly heard: HeardButtons;
  readonly note: OnPress;
} {
  const [heard, setHeard] = useState<HeardButtons>({ buttons: [], last: undefined, pressCount: 0 });
  const note = useCallback(
    (press: Press, atMs = performance.now()) => {
      const button = buttonOf(press);
      onButton(press, button, atMs);
      setHeard((before) => ({
        buttons: before.buttons.includes(button) ? before.buttons : [...before.buttons, button],
        last: { press, button },
        pressCount: before.pressCount + 1,
      }));
    },
    [onButton],
  );
  return { heard, note };
}

/** Sets session's handler for action, where the browser knows the action; a browser that does not throws. */
function handleAction(
  session: MediaSession,
  action: MediaSessionAction,
  handler: (() => void) | null,
): void {
  try {
    session.setActionHandler(action, handler);
  } catch {
    // The other actions are still heard.
  }
}

/** The Media Session actions fired, newest first, while the probe is open; each told to onPress. */
function useMediaActions(title: string, onPress: OnPress): readonly string[] {
  const [fired, setFired] = useState<readonly string[]>([]);
  useEffect(() => {
    if (!("mediaSession" in navigator)) {
      return undefined;
    }
    const session = navigator.mediaSession;
    if ("MediaMetadata" in globalThis) {
      session.metadata = new MediaMetadata({ title });
    }
    const startedAt = performance.now();
    for (const action of mediaActions) {
      handleAction(session, action, () => {
        const at = Math.round(performance.now() - startedAt);
        setFired((before) => [`${action} @${String(at)} ms`, ...before].slice(0, 30));
        onPress({ kind: "media", action });
      });
    }
    return () => {
      for (const action of mediaActions) {
        handleAction(session, action, null);
      }
    };
  }, [title, onPress]);
  return fired;
}

/**
 * How many times Back reached the page as a history step, each answered by a step forward again,
 * and told to onPress: Silk's Back is never a key (tv-device-tests.md §5).
 */
function useBackSteps(onPress: OnPress): number {
  const [steps, setSteps] = useState(0);
  useEffect(() => {
    globalThis.history.pushState({ lab: "back" }, "");
    const heard = (): void => {
      setSteps((before) => before + 1);
      onPress({ kind: "historyStep" });
      globalThis.history.pushState({ lab: "back" }, "");
    };
    globalThis.addEventListener("popstate", heard);
    return () => {
      globalThis.removeEventListener("popstate", heard);
    };
  }, [onPress]);
  return steps;
}

/** The keys the page hears, newest first, kept to keysKept; tells onKeyDown each key and its time. */
function useKeyLog(onKeyDown: (atMs: number, entry: KeyEntry) => void): readonly KeyEntry[] {
  const [keys, setKeys] = useState<readonly KeyEntry[]>([]);
  const previousAtRef = useRef<number | undefined>(undefined);
  useEffect(() => {
    const heard = (event: KeyboardEvent): void => {
      const atMs = eventTimeMs(event.timeStamp, performance.now());
      const entry = keyEntryOf(heardKeyOf(event, atMs), previousAtRef.current);
      setKeys((before) => withKey(before, entry));
      previousAtRef.current = atMs;
      if (event.type === "keydown") {
        onKeyDown(atMs, entry);
      }
    };
    globalThis.addEventListener("keydown", heard, true);
    globalThis.addEventListener("keyup", heard, true);
    return () => {
      globalThis.removeEventListener("keydown", heard, true);
      globalThis.removeEventListener("keyup", heard, true);
    };
  }, [onKeyDown]);
  return keys;
}

/** A quiet tone playing while isOn, so the page counts as playing media for Media Session. */
function useQuietTone(isOn: boolean): void {
  useEffect(() => {
    if (!isOn) {
      return undefined;
    }
    const url = toneUrl(220, 0.02);
    const tone = new Audio(url);
    tone.loop = true;
    void tone.play().catch(() => undefined);
    return () => {
      tone.pause();
      URL.revokeObjectURL(url);
    };
  }, [isOn]);
}

/**
 * The quiet tone, off until switched on in the details (the owner, 2026-10-25: "Do we need the tone
 * to keep playing?"): a TV's remote sends play/pause as a key without it, and only a browser that
 * gives a page a keyboard's media keys while it plays (desktop Chrome and Firefox) needs it.
 */
function useTone(): { readonly isToneOn: boolean; readonly toggleTone: () => void } {
  const [isToneOn, setIsToneOn] = useState(false);
  useQuietTone(isToneOn);
  const toggleTone = useCallback(() => {
    setIsToneOn((wasOn) => !wasOn);
  }, []);
  return { isToneOn, toggleTone };
}

/**
 * Each move's latency, by mode, from the key's time to the frame after focus arrived: the key's own
 * time where the browser gives it on the page's clock, else when it was heard; each told to onMove.
 * Ours moves across the whole app, the shell's title bar and the page's every control, so a remote
 * reaches them all (owner, Silk, 2026-10-25: changing Silk's mode reloads the page, so every control
 * must be reached in ours; and Up from the page's top reached nothing, the region being the page
 * alone), and the first arrow starts it at start, the tiles.
 */
function useFocusLatency(
  startRef: RefObject<HTMLDivElement | null>,
  mode: FocusMode,
  onMove: (mode: FocusMode, latencyMs: number) => void,
): {
  readonly latencies: Readonly<Record<FocusMode, readonly number[]>>;
  readonly noteKey: (atMs: number) => void;
} {
  const [latencies, setLatencies] = useState<Readonly<Record<FocusMode, readonly number[]>>>({
    browser: [],
    geometric: [],
  });
  const keyAtRef = useRef<number | undefined>(undefined);
  const noteKey = useCallback((atMs: number) => {
    keyAtRef.current = atMs;
  }, []);
  useEffect(() => {
    const region = document.body;
    const stopDriving =
      mode === "geometric"
        ? driveFocus(region, geometricFocus, startRef.current ?? region)
        : undefined;
    const moved = (): void => {
      const at = keyAtRef.current;
      keyAtRef.current = undefined;
      if (at === undefined || performance.now() - at > keyMovesFocusWithinMs) {
        return;
      }
      requestAnimationFrame(() => {
        const latency = Math.round(performance.now() - at);
        onMove(mode, latency);
        setLatencies((before) => ({
          ...before,
          [mode]: [...before[mode], latency].slice(-latenciesKept),
        }));
      });
    };
    region.addEventListener("focusin", moved);
    return () => {
      region.removeEventListener("focusin", moved);
      stopDriving?.();
    };
  }, [mode, startRef, onMove]);
  return { latencies, noteKey };
}

/** The chips of the buttons, each lit once heard, the last one marked. */
function ButtonChips(props: { readonly heard: HeardButtons }): JSX.Element {
  const { t } = useTranslation();
  const lastButton = props.heard.last?.button;
  return (
    <ul className="lab__chips" aria-label={t("lab.input.chipsLabel")}>
      {chipsShown(props.heard.buttons).map((button) => {
        const isHeard = props.heard.buttons.includes(button);
        const isLast = button === lastButton;
        return (
          <li
            key={button}
            className={`lab__chip${isHeard ? " lab__chip--heard" : ""}${isLast ? " lab__chip--last" : ""}`}
            data-button={button}
          >
            {t(`lab.input.button.${button}`)}
          </li>
        );
      })}
    </ul>
  );
}

/** What the last-heard line says of a press, in the small type, for a report. */
function pressDetail(t: ReturnType<typeof useTranslation>["t"], press: Press): string {
  switch (press.kind) {
    case "key":
      return t("lab.input.detail.key", {
        key: press.key,
        code: press.code,
        keyCode: String(press.keyCode),
      });
    case "historyStep":
      return t("lab.input.detail.historyStep");
    case "pad":
      return t("lab.input.detail.pad", { button: String(press.button), pad: press.pad });
    case "stick":
      return t("lab.input.detail.stick", {
        axis: String(press.axis),
        value: String(press.value),
        pad: press.pad,
      });
    case "media":
      return t("lab.input.detail.media", { action: press.action });
  }
}

/** The last button heard, large, with what it was in small type beside it. */
function LastHeard(props: { readonly heard: HeardButtons }): JSX.Element {
  const { t } = useTranslation();
  const { last, pressCount } = props.heard;
  return (
    <output className="lab__heard" aria-live="polite">
      {last === undefined ? (
        <span className="lab__heard-name">{t("lab.input.nothingYet")}</span>
      ) : (
        <>
          <span className="lab__heard-name">{t(`lab.input.button.${last.button}`)}</span>{" "}
          <span className="lab__small">
            {t("lab.input.detail.count", {
              count: String(pressCount),
              detail: pressDetail(t, last.press),
            })}
          </span>
        </>
      )}
    </output>
  );
}

/** A row of tiles, all buttons, for the arrows to move across. */
function TileRows({ ref }: { readonly ref: Ref<HTMLDivElement> }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div ref={ref} className="lab__tiles">
      {Array.from({ length: tileRows * tileColumns }, (_, index) => (
        <button key={index} type="button" className="lab__tile">
          {t("lab.input.tile", { number: String(index + 1) })}
        </button>
      ))}
    </div>
  );
}

/** Who moves between the tiles. */
function InputControls(props: {
  readonly mode: FocusMode;
  readonly onMode: (mode: FocusMode) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="lab__controls">
      <fieldset className="lab__modes">
        <legend>{t("lab.input.modeLabel")}</legend>
        {focusModes.map((each) => (
          <button
            key={each}
            type="button"
            className="lab__button"
            aria-pressed={props.mode === each}
            onClick={() => {
              props.onMode(each);
            }}
          >
            {t(`lab.input.mode.${each}`)}
          </button>
        ))}
      </fieldset>
    </div>
  );
}

/** How quickly the tiles' focus follows a key in mode, in one short line. */
function timingText(t: ReturnType<typeof useTranslation>["t"], summary: LatencySummary): string {
  return summary.count === 0
    ? t("lab.input.noMoves")
    : t("lab.input.timing", {
        count: String(summary.count),
        median: String(summary.medianMs),
        worst: String(summary.worstMs),
      });
}

/** The findings for a report, in the details. */
function InputFacts(props: {
  readonly summaries: Readonly<Record<FocusMode, LatencySummary>>;
  readonly backSteps: number;
  readonly backKeys: number;
  readonly mediaFired: readonly string[];
  readonly pads: readonly PadState[];
}): JSX.Element {
  const { t } = useTranslation();
  const latencyText = (summary: LatencySummary): string =>
    summary.count === 0
      ? t("lab.input.noMoves")
      : t("lab.input.latencyIs", {
          count: String(summary.count),
          median: String(summary.medianMs),
          worst: String(summary.worstMs),
          within: yesNo(t, summary.isWithinTarget),
        });
  const padsText =
    props.pads.length === 0
      ? t("lab.input.noPads")
      : props.pads
          .map((pad) =>
            t("lab.input.pad", {
              id: pad.id,
              pressed: pad.pressed.join(" "),
              axes: pad.axes.join(" "),
            }),
          )
          .join("; ");
  return (
    <dl className="lab__facts">
      {focusModes.map((mode) => (
        <div key={mode} className="lab__fact">
          <dt>{t(`lab.input.latency.${mode}`)}</dt>
          <dd>{latencyText(props.summaries[mode])}</dd>
        </div>
      ))}
      <div className="lab__fact">
        <dt>{t("lab.input.back")}</dt>
        <dd>
          {t("lab.input.backIs", { steps: String(props.backSteps), keys: String(props.backKeys) })}
        </dd>
      </div>
      <div className="lab__fact">
        <dt>{t("lab.input.media")}</dt>
        <dd>
          {props.mediaFired.length === 0
            ? t("lab.input.noMedia")
            : props.mediaFired.slice(0, 5).join(", ")}
        </dd>
      </div>
      <div className="lab__fact">
        <dt>{t("lab.input.pads")}</dt>
        <dd>{padsText}</dd>
      </div>
    </dl>
  );
}

/** The keys heard, newest first. */
function KeyTable(props: { readonly keys: readonly KeyEntry[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="lab__scroll">
      <table className="lab__table">
        <thead>
          <tr>
            <th scope="col">{t("lab.input.keyType")}</th>
            <th scope="col">{t("lab.input.key")}</th>
            <th scope="col">{t("lab.input.code")}</th>
            <th scope="col">{t("lab.input.keyCode")}</th>
            <th scope="col">{t("lab.input.repeat")}</th>
            <th scope="col">{t("lab.input.since")}</th>
          </tr>
        </thead>
        <tbody>
          {props.keys.map((key, index) => (
            // eslint-disable-next-line @eslint-react/no-array-index-key -- the log's entries have no identity but their place, newest first
            <tr key={index} className={key.isBack ? "lab__back-key" : undefined}>
              <td>{key.type}</td>
              <td>{key.key}</td>
              <td>{key.code}</td>
              <td>{key.keyCode}</td>
              <td>{yesNo(t, key.isRepeat)}</td>
              <td>{key.sincePreviousMs ?? ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/**
 * What a report needs beyond the main view, closed until asked for; it may scroll when open. The
 * tone's switch is here, off: only a keyboard's media keys need it.
 */
function InputDetails(props: {
  readonly isToneOn: boolean;
  readonly onTone: () => void;
  readonly keys: readonly KeyEntry[];
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <details className="lab__details">
      <summary>{t("lab.input.details")}</summary>
      <p className="lab__lead">{t("lab.input.explain")}</p>
      {props.children}
      <button
        type="button"
        className="lab__button"
        aria-pressed={props.isToneOn}
        onClick={props.onTone}
      >
        {t("lab.input.playTone")}
      </button>
      <h2>{t("lab.input.keys")}</h2>
      <KeyTable keys={props.keys} />
    </details>
  );
}

/**
 * The page's hearing: every key, Back's history steps, Media Session's actions and the pads; each
 * press, a key's going down alone, told to onButton; a pad seen and a pad's axis sent to noteEvent;
 * a stick's arrows to onStickArrow.
 */
function useHearing(
  noteKey: (atMs: number) => void,
  onButton: (press: Press, button: ButtonName, atMs: number) => void,
  pads: {
    readonly noteEvent: (event: NotedEvent) => void;
    readonly onStickArrow: (direction: Direction) => void;
    readonly okChosen: OkButton | undefined;
  },
): {
  readonly heard: HeardButtons;
  readonly keys: readonly KeyEntry[];
  readonly backSteps: number;
  readonly mediaFired: readonly string[];
  readonly pads: readonly PadState[];
} {
  const { t } = useTranslation();
  const { heard, note } = useHeardButtons(onButton);
  const onKeyDown = useCallback(
    (atMs: number, entry: KeyEntry) => {
      noteKey(atMs);
      const focused = document.activeElement;
      note(
        {
          kind: "key",
          key: entry.key,
          code: entry.code,
          keyCode: entry.keyCode,
          isRepeat: entry.isRepeat,
          isInField: focused !== null && caretOf(focused) !== undefined,
        },
        atMs,
      );
    },
    [noteKey, note],
  );
  const keys = useKeyLog(onKeyDown);
  const backSteps = useBackSteps(note);
  const mediaFired = useMediaActions(t("lab.input.mediaTitle"), note);
  const padHandlers = usePadHandlers(note, pads.noteEvent, pads.onStickArrow);
  const padsRead = usePads(padHandlers, pads.okChosen);
  return { heard, keys, backSteps, mediaFired, pads: padsRead };
}

/** When something came, on the page's clock, in whole milliseconds. */
function nowMs(): number {
  return Math.round(performance.now());
}

/** Where the sending stands, in one small line: the run's name to note down, and how many sent. */
function SendingLine(props: { readonly sending: EventSending }): JSX.Element {
  const { t } = useTranslation();
  const said = {
    run: props.sending.run,
    count: String(props.sending.sentCount),
  };
  return (
    <output className="lab__small" aria-live="polite">
      {props.sending.hasFailed ? t("lab.input.sendFailed", said) : t("lab.input.sending", said)}
    </output>
  );
}

/**
 * The test's hearing and measuring in mode, its events sent by noteEvent: every press, each move's
 * latency, the visual viewport's changes, Back or Escape leaving full screen, a pad's buttons and
 * stick driving our navigation, and the OK the person chose for a pad of unknown layout.
 */
function useButtonTest(
  mode: FocusMode,
  tilesRef: RefObject<HTMLDivElement | null>,
  noteEvent: (event: NotedEvent) => void,
): ReturnType<typeof useHearing> & {
  readonly latencies: Readonly<Record<FocusMode, readonly number[]>>;
  readonly chooseOk: (chosen: OkButton) => void;
} {
  const [okChosen, setOkChosen] = useState<OkButton | undefined>(undefined);
  useViewportChanges(noteEvent);
  const onMove = useCallback(
    (movedBy: FocusMode, latencyMs: number) => {
      noteEvent({ kind: "move", mode: movedBy, latencyMs, atMs: nowMs() });
    },
    [noteEvent],
  );
  const { latencies, noteKey } = useFocusLatency(tilesRef, mode, onMove);
  const { onPadButton, onStickArrow } = usePadNavigation(mode, tilesRef, noteKey);
  const onButton = useCallback(
    (press: Press, button: ButtonName, atMs: number) => {
      noteEvent(eventOfPress(press, button, atMs));
      if (button === "back" || (press.kind === "key" && press.key === "Escape")) {
        leaveFullscreenAt(noteEvent, button);
      }
      if (press.kind === "pad") {
        onPadButton(button);
      }
    },
    [noteEvent, onPadButton],
  );
  const hearing = useHearing(noteKey, onButton, { noteEvent, onStickArrow, okChosen });
  const chooseOk = useCallback(
    (chosen: OkButton) => {
      setOkChosen(chosen);
      noteEvent({ kind: "okChoice", okButton: chosen, atMs: nowMs() });
    },
    [noteEvent],
  );
  return { ...hearing, latencies, chooseOk };
}

/**
 * The button test (plans/tv-device-tests.md H2, H6–H8; controller-input-2026-10-25.md §6.2 step 0):
 * which of a remote's, a keyboard's or a controller's buttons the page hears, each lighting its chip,
 * the last named large; tiles to move across with the arrows, by the browser's own navigation or
 * ours, each move's latency from the key to the frame after focus moved measured against D190's
 * 250 ms; a text field, a full-screen test and the controller connected with its layout; the key
 * log, Back, Media Session and the pads in the details. Each press, move, the field's and full
 * screen's changes go to the server's log as they happen, under the run's name the page shows. It
 * fits a Fire TV's 960 × 540 without scrolling, the details closed (the owner, 2026-10-25).
 */
export function LabInput(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [mode, setMode] = useState<FocusMode>("geometric");
  const { isToneOn, toggleTone } = useTone();
  const tilesRef = useRef<HTMLDivElement>(null);
  const { sending, noteEvent } = useInputEvents(api, mode);
  const { heard, keys, backSteps, mediaFired, pads, latencies, chooseOk } = useButtonTest(
    mode,
    tilesRef,
    noteEvent,
  );
  const summaries = {
    browser: latencySummary(latencies.browser),
    geometric: latencySummary(latencies.geometric),
  };
  const backKeys = keys.filter((key) => key.isBack && key.type === "keydown").length;
  const changeMode = (chosen: FocusMode): void => {
    setMode(chosen);
    noteEvent({ kind: "mode", mode: chosen, atMs: nowMs() });
  };
  return (
    <LabFrame
      id="lab-input"
      heading={t("lab.input.heading")}
      isCompact={true}
      atTop={<SendingLine sending={sending} />}
    >
      <ButtonChips heard={heard} />
      <LastHeard heard={heard} />
      <TileRows ref={tilesRef} />
      <div className="lab__controls">
        <FieldTest noteEvent={noteEvent} />
        <FullscreenTest noteEvent={noteEvent} />
        <InputControls mode={mode} onMode={changeMode} />
      </div>
      <PadLine pads={pads} onChoose={chooseOk} />
      <InputDetails isToneOn={isToneOn} onTone={toggleTone} keys={keys}>
        <p className="lab__small">{timingText(t, summaries[mode])}</p>
        <InputFacts
          summaries={summaries}
          backSteps={backSteps}
          backKeys={backKeys}
          mediaFired={mediaFired}
          pads={pads}
        />
      </InputDetails>
    </LabFrame>
  );
}
