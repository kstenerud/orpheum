// SPDX-License-Identifier: MPL-2.0

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type JSX,
  type Ref,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

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

/** The grid's size, a TV home screen's: six tiles a row, four rows. */
const gridColumns = 6;
const gridRows = 4;

/** A key that came longer ago than this before focus moved did not move it, in milliseconds. */
const keyMovesFocusWithinMs = 1000;

/** How many moves' latencies a mode keeps: a long walk across the grid. */
const latenciesKept = 200;

/** One gamepad as the probe last read it. */
interface PadState {
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
 * pad seen since the probe opened.
 */
function usePads(): { readonly pads: readonly PadState[]; readonly seen: readonly string[] } {
  const [pads, setPads] = useState<readonly PadState[]>([]);
  const [seen, setSeen] = useState<readonly string[]>([]);
  useEffect(() => {
    if (!("getGamepads" in navigator)) {
      return undefined;
    }
    let frame: number | undefined;
    const poll = (): void => {
      const read = readPads();
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
  }, []);
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

/** The Media Session actions fired, newest first, while the probe is open. */
function useMediaActions(title: string): readonly string[] {
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
      });
    }
    return () => {
      for (const action of mediaActions) {
        handleAction(session, action, null);
      }
    };
  }, [title]);
  return fired;
}

/** How many times Back reached the page as a history step, each answered by a step forward again. */
function useBackSteps(): number {
  const [steps, setSteps] = useState(0);
  useEffect(() => {
    globalThis.history.pushState({ lab: "back" }, "");
    const heard = (): void => {
      setSteps((before) => before + 1);
      globalThis.history.pushState({ lab: "back" }, "");
    };
    globalThis.addEventListener("popstate", heard);
    return () => {
      globalThis.removeEventListener("popstate", heard);
    };
  }, []);
  return steps;
}

/** The keys the page hears, newest first, kept to keysKept; tells onKeyDown each key's time. */
function useKeyLog(onKeyDown: (atMs: number) => void): readonly KeyEntry[] {
  const [keys, setKeys] = useState<readonly KeyEntry[]>([]);
  const previousAtRef = useRef<number | undefined>(undefined);
  useEffect(() => {
    const heard = (event: KeyboardEvent): void => {
      const atMs = eventTimeMs(event.timeStamp, performance.now());
      const entry = keyEntryOf(heardKeyOf(event, atMs), previousAtRef.current);
      setKeys((before) => withKey(before, entry));
      previousAtRef.current = atMs;
      if (event.type === "keydown") {
        onKeyDown(atMs);
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
 * the first arrow starts it at start, the grid.
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

/** The probe's findings above the grid, large. */
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
    <dl className="lab__facts lab__big">
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

/** A header row and a grid of tiles, all buttons, for the arrows to move across. */
function TileGrid({ ref }: { readonly ref: Ref<HTMLDivElement> }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div ref={ref} className="lab__region">
      <div className="lab__header-row">
        {[1, 2, 3, 4].map((place) => (
          <button key={place} type="button" className="lab__tile lab__tile--header">
            {t("lab.input.headerTile", { place: String(place) })}
          </button>
        ))}
      </div>
      <div className="lab__grid">
        {Array.from({ length: gridRows * gridColumns }, (_, index) => (
          <button key={index} type="button" className="lab__tile">
            {t("lab.input.tile", {
              row: String(Math.floor(index / gridColumns) + 1),
              column: String((index % gridColumns) + 1),
            })}
          </button>
        ))}
      </div>
    </div>
  );
}

/** The keys heard, newest first. */
function KeyTable(props: { readonly keys: readonly KeyEntry[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="lab__scroll">
      <table className="lab__table lab__big">
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

/** The mode switch, the tone and the send button. */
function InputActions(props: {
  readonly mode: FocusMode;
  readonly onMode: (mode: FocusMode) => void;
  readonly isToneOn: boolean;
  readonly onTone: () => void;
  readonly children: JSX.Element;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <fieldset className="lab__actions">
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
      <button
        type="button"
        className="lab__button"
        aria-pressed={props.isToneOn}
        onClick={props.onTone}
      >
        {t("lab.input.playTone")}
      </button>
      {props.children}
    </fieldset>
  );
}

/**
 * The controller and keyboard probe (plans/tv-device-tests.md; controller-input-2026-10-25.md §6.2
 * step 0): every key the page hears, whether Back and the media keys reach it, any gamepad, and a
 * grid of tiles to move across with the browser's own navigation or ours, each move's latency from
 * the key to the frame after focus moved measured against D190's 250 ms.
 */
export function LabInput(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [mode, setMode] = useState<FocusMode>("geometric");
  const { isToneOn, toggleTone } = useTone();
  const pageRef = useRef<HTMLDivElement>(null);
  const gridRef = useRef<HTMLDivElement>(null);
  const { latencies, noteKey } = useFocusLatency(pageRef, gridRef, mode);
  const keys = useKeyLog(noteKey);
  const backSteps = useBackSteps();
  const mediaFired = useMediaActions(t("lab.input.mediaTitle"));
  const { pads, seen } = usePads();
  const { sending, send } = useLabSending(api, "input");
  const summaries = {
    browser: latencySummary(latencies.browser),
    geometric: latencySummary(latencies.geometric),
  };
  const backKeys = keys.filter((key) => key.isBack && key.type === "keydown").length;
  return (
    <LabFrame id="lab-input" heading={t("lab.input.heading")}>
      <div ref={pageRef}>
        <p className="lab__lead">{t("lab.input.explain")}</p>
        <InputActions mode={mode} onMode={setMode} isToneOn={isToneOn} onTone={toggleTone}>
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
                userAgent: navigator.userAgent,
              });
            }}
          />
        </InputActions>
        <InputFacts
          summaries={summaries}
          backSteps={backSteps}
          backKeys={backKeys}
          mediaFired={mediaFired}
          pads={pads}
        />
        <TileGrid ref={gridRef} />
        <h2>{t("lab.input.keys")}</h2>
        <KeyTable keys={keys} />
      </div>
    </LabFrame>
  );
}
