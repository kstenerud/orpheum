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

import { buttonOf, chipsShown, type ButtonName } from "./lab-buttons.ts";
import { LabFrame, LabSendButton, yesNo } from "./lab-frame.tsx";
import { driveFocus, geometricFocus } from "./lab-focus.ts";
import { heardKeyOf, keyEntryOf, useFirstPress, withKey, type KeyEntry } from "./lab-keys.ts";
import { useLabSending } from "./lab-send.ts";
import { eventTimeMs, latencySummary, type LatencySummary } from "./lab-timing.ts";
import { toneUrl } from "./lab-tone.ts";
import { useServices } from "./services.tsx";

/** Who moves focus on an arrow: the browser by itself, or our engine. */
type FocusMode = "browser" | "geometric";

/** Both modes, in the order the page offers them. */
const focusModes: readonly FocusMode[] = ["geometric", "browser"];

/** The Media Session actions the probe listens for. */
const mediaActions: readonly MediaSessionAction[] = [
  "play",
  "pause",
  "seekbackward",
  "seekforward",
  "previoustrack",
  "nexttrack",
];

/**
 * The tiles' size: two rows of six, enough to try every arrow while the whole page fits a Fire TV's
 * 960 × 540 without scrolling (the owner, 2026-10-25). A justified constant, the page's layout.
 */
const tileColumns = 6;
const tileRows = 2;

/** A key that came longer ago than this before focus moved did not move it, in milliseconds. */
const keyMovesFocusWithinMs = 1000;

/** How many moves' latencies a mode keeps: a long walk across the tiles. */
const latenciesKept = 200;

/** One press the page heard, with what the last-heard line shows of it. */
type Press =
  | { readonly kind: "key"; readonly key: string; readonly code: string; readonly keyCode: number }
  | { readonly kind: "historyStep" }
  | { readonly kind: "pad"; readonly button: number; readonly pad: string }
  | { readonly kind: "media"; readonly action: string };

/** The buttons heard, in the order first heard, and the last press. */
interface HeardButtons {
  readonly buttons: readonly ButtonName[];
  readonly last: { readonly press: Press; readonly button: ButtonName } | undefined;
  /** Every press heard, of whichever button. */
  readonly pressCount: number;
}

/** The buttons the page has heard, and the noting of each press. */
function useHeardButtons(): {
  readonly heard: HeardButtons;
  readonly note: (press: Press) => void;
} {
  const [heard, setHeard] = useState<HeardButtons>({ buttons: [], last: undefined, pressCount: 0 });
  const note = useCallback((press: Press) => {
    const button = buttonOf(press);
    setHeard((before) => ({
      buttons: before.buttons.includes(button) ? before.buttons : [...before.buttons, button],
      last: { press, button },
      pressCount: before.pressCount + 1,
    }));
  }, []);
  return { heard, note };
}

/** One gamepad as the probe last read it. */
interface PadState {
  readonly index: number;
  readonly id: string;
  readonly mapping: string;
  readonly pressed: readonly number[];
  readonly axes: readonly number[];
}

/** The pads navigator.getGamepads gives, as the probe shows them. */
function readPads(): PadState[] {
  const pads: PadState[] = [];
  for (const pad of navigator.getGamepads()) {
    if (pad !== null) {
      pads.push({
        index: pad.index,
        id: pad.id,
        mapping: pad.mapping,
        pressed: pad.buttons.flatMap((button, index) => (button.pressed ? [index] : [])),
        axes: pad.axes.map((axis) => Math.round(axis * 100) / 100),
      });
    }
  }
  return pads;
}

/**
 * The pads connected, polled each frame only while one is (controller-input §6.1 item 5), and every
 * pad seen since the probe opened; tells onPress each button as it goes down.
 */
function usePads(onPress: (press: Press) => void): {
  readonly pads: readonly PadState[];
  readonly seen: readonly string[];
} {
  const [pads, setPads] = useState<readonly PadState[]>([]);
  const [seen, setSeen] = useState<readonly string[]>([]);
  useEffect(() => {
    if (!("getGamepads" in navigator)) {
      return undefined;
    }
    let frame: number | undefined;
    const pressedBefore = new Map<number, readonly number[]>();
    const poll = (): void => {
      const read = readPads();
      for (const pad of read) {
        const before = pressedBefore.get(pad.index) ?? [];
        for (const button of pad.pressed.filter((each) => !before.includes(each))) {
          onPress({ kind: "pad", button, pad: pad.id });
        }
        pressedBefore.set(pad.index, pad.pressed);
      }
      setPads(read);
      setSeen((before) => {
        const names = read.map((pad) => `${pad.id} (${pad.mapping})`);
        return names.every((name) => before.includes(name))
          ? before
          : [...new Set([...before, ...names])];
      });
      frame = read.length > 0 ? requestAnimationFrame(poll) : undefined;
    };
    const connected = (): void => {
      frame ??= requestAnimationFrame(poll);
    };
    globalThis.addEventListener("gamepadconnected", connected);
    globalThis.addEventListener("gamepaddisconnected", connected);
    return () => {
      globalThis.removeEventListener("gamepadconnected", connected);
      globalThis.removeEventListener("gamepaddisconnected", connected);
      if (frame !== undefined) {
        cancelAnimationFrame(frame);
      }
    };
  }, [onPress]);
  return { pads, seen };
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
function useMediaActions(title: string, onPress: (press: Press) => void): readonly string[] {
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
function useBackSteps(onPress: (press: Press) => void): number {
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

/** The quiet tone, on from the page's first key or press and switched by its button after. */
function useTone(): { readonly isToneOn: boolean; readonly toggleTone: () => void } {
  const [isToneOn, setIsToneOn] = useState(false);
  useFirstPress(() => {
    setIsToneOn(true);
  });
  useQuietTone(isToneOn);
  const toggleTone = useCallback(() => {
    setIsToneOn((wasOn) => !wasOn);
  }, []);
  return { isToneOn, toggleTone };
}

/**
 * Each move's latency, by mode, from the key's time to the frame after focus arrived in the region:
 * the key's own time where the browser gives it on the page's clock, else when it was heard. Ours
 * moves across the whole region, the page's every control, so a remote reaches Send too (owner,
 * Silk, 2026-10-25: changing Silk's mode reloads the page, so Send must be reached in ours), and
 * the first arrow starts it at start, the tiles.
 */
function useFocusLatency(
  regionRef: RefObject<HTMLDivElement | null>,
  startRef: RefObject<HTMLDivElement | null>,
  mode: FocusMode,
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
    const region = regionRef.current;
    if (region === null) {
      return undefined;
    }
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
  }, [mode, regionRef, startRef]);
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

/** Two rows of tiles, all buttons, for the arrows to move across. */
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

/** Who moves between the tiles, and the Send button after them. */
function InputControls(props: {
  readonly mode: FocusMode;
  readonly onMode: (mode: FocusMode) => void;
  readonly children: ReactNode;
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
      {props.children}
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

/** What a report needs beyond the main view, closed until asked for; it may scroll when open. */
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

/** The page's hearing: every key, Back's history steps, Media Session's actions and the pads. */
function useHearing(
  noteKey: (atMs: number) => void,
  t: ReturnType<typeof useTranslation>["t"],
): {
  readonly heard: HeardButtons;
  readonly keys: readonly KeyEntry[];
  readonly backSteps: number;
  readonly mediaFired: readonly string[];
  readonly pads: readonly PadState[];
  readonly seen: readonly string[];
} {
  const { heard, note } = useHeardButtons();
  const onKeyDown = useCallback(
    (atMs: number, entry: KeyEntry) => {
      noteKey(atMs);
      note({ kind: "key", key: entry.key, code: entry.code, keyCode: entry.keyCode });
    },
    [noteKey, note],
  );
  const keys = useKeyLog(onKeyDown);
  const backSteps = useBackSteps(note);
  const mediaFired = useMediaActions(t("lab.input.mediaTitle"), note);
  const { pads, seen } = usePads(note);
  return { heard, keys, backSteps, mediaFired, pads, seen };
}

/**
 * The button test (plans/tv-device-tests.md H2; controller-input-2026-10-25.md §6.2 step 0): which
 * of a remote's, a keyboard's or a controller's buttons the page hears, each lighting its chip, the
 * last named large; tiles to move across with the arrows, by the browser's own navigation or ours,
 * each move's latency from the key to the frame after focus moved measured against D190's 250 ms;
 * the key log, Back, Media Session and the pads in the details and the report. It fits a Fire TV's
 * 960 × 540 without scrolling, the details closed (the owner, 2026-10-25).
 */
export function LabInput(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [mode, setMode] = useState<FocusMode>("geometric");
  const { isToneOn, toggleTone } = useTone();
  const pageRef = useRef<HTMLDivElement>(null);
  const tilesRef = useRef<HTMLDivElement>(null);
  const { latencies, noteKey } = useFocusLatency(pageRef, tilesRef, mode);
  const { heard, keys, backSteps, mediaFired, pads, seen } = useHearing(noteKey, t);
  const { sending, send } = useLabSending(api, "input");
  const summaries = {
    browser: latencySummary(latencies.browser),
    geometric: latencySummary(latencies.geometric),
  };
  const backKeys = keys.filter((key) => key.isBack && key.type === "keydown").length;
  return (
    <div ref={pageRef}>
      <LabFrame id="lab-input" heading={t("lab.input.heading")} isCompact={true}>
        <p className="lab__lead">{t("lab.input.explain")}</p>
        <ButtonChips heard={heard} />
        <LastHeard heard={heard} />
        <TileRows ref={tilesRef} />
        <InputControls mode={mode} onMode={setMode}>
          <LabSendButton
            sending={sending}
            onSend={() => {
              send({
                mode,
                latency: summaries,
                latenciesMs: latencies,
                backSteps,
                backKeysInLog: backKeys,
                mediaSessionFired: mediaFired,
                hasMediaSession: "mediaSession" in navigator,
                gamepadsSeen: seen,
                keys,
                buttonsHeard: heard.buttons,
                isToneOn,
                userAgent: navigator.userAgent,
              });
            }}
          />
        </InputControls>
        <p className="lab__small">
          {timingText(t, summaries[mode])}
          {isToneOn ? ` ${t("lab.input.toneOn")}` : ""}
        </p>
        <InputDetails isToneOn={isToneOn} onTone={toggleTone} keys={keys}>
          <InputFacts
            summaries={summaries}
            backSteps={backSteps}
            backKeys={backKeys}
            mediaFired={mediaFired}
            pads={pads}
          />
        </InputDetails>
      </LabFrame>
    </div>
  );
}
