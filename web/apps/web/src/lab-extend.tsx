// SPDX-License-Identifier: MPL-2.0

import { failureText } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { LabFrame, LabSendButton, yesNo } from "./lab-frame.tsx";
import {
  extendChannelName,
  extendMessageOf,
  screensOf,
  windowFeaturesOn,
  type ExtendCommand,
  type ExtendMessage,
  type ScreenPlace,
} from "./lab-extend-channel.ts";
import { useLabSending } from "./lab-send.ts";
import { toneUrl } from "./lab-tone.ts";
import { routeHref } from "./router.ts";
import { useServices } from "./services.tsx";

/** The window the player opens in, one at a time. */
const playerWindowName = "orpheum-lab-player";

/** The two tones of the output test, Hz: A lower, B higher, told apart by ear. */
const toneHz = { a: 440, b: 660 } as const;

/** The player's last state, as its window tells it. */
type PlayerState = Extract<ExtendMessage, { readonly kind: "state" }>;

/** The remote's channel to the player: the commands it sends, the state it hears. */
function usePlayerChannel(): {
  readonly state: PlayerState | undefined;
  readonly command: (command: ExtendCommand) => void;
} {
  const [state, setState] = useState<PlayerState | undefined>();
  const channelRef = useRef<BroadcastChannel | undefined>(undefined);
  useEffect(() => {
    if (!("BroadcastChannel" in globalThis)) {
      return undefined;
    }
    const opened = new BroadcastChannel(extendChannelName);
    channelRef.current = opened;
    const heard = (event: MessageEvent): void => {
      const message = extendMessageOf(event.data);
      if (message?.kind === "state") {
        setState(message);
      }
    };
    opened.addEventListener("message", heard);
    return () => {
      opened.removeEventListener("message", heard);
      opened.close();
      channelRef.current = undefined;
    };
  }, []);
  return {
    state,
    command: (command) => {
      channelRef.current?.postMessage({ kind: "command", command });
    },
  };
}

/** What the output test found for one tone: where it played, or why it could not. */
interface ToneOutcome {
  readonly tone: "A" | "B";
  readonly output: string;
  readonly failure: string | undefined;
}

/** An output the page can send sound to, by its id and its label. */
interface AudioOutput {
  readonly id: string;
  readonly label: string;
}

/** The audio outputs, named once a microphone was allowed, which is what lets labels show. */
async function listOutputs(): Promise<AudioOutput[]> {
  const asked = await navigator.mediaDevices.getUserMedia({ audio: true });
  for (const track of asked.getTracks()) {
    track.stop();
  }
  const devices = await navigator.mediaDevices.enumerateDevices();
  return devices
    .filter((device) => device.kind === "audiooutput")
    .map((device, index) => ({
      id: device.deviceId,
      label: device.label === "" ? `#${String(index + 1)}` : device.label,
    }));
}

/** Plays element on output; answers what happened. */
async function playOn(
  element: HTMLAudioElement,
  tone: ToneOutcome["tone"],
  output: AudioOutput,
): Promise<ToneOutcome> {
  try {
    await element.setSinkId(output.id);
    await element.play();
    return { tone, output: output.label, failure: undefined };
  } catch (cause) {
    return { tone, output: output.label, failure: failureText(cause) };
  }
}

/** The two looping tones' elements, made for the page's life. */
function useTones(): { readonly a: HTMLAudioElement; readonly b: HTMLAudioElement } {
  const [tones] = useState(() => {
    const made = (hz: number): HTMLAudioElement => {
      const element = new Audio(toneUrl(hz));
      element.loop = true;
      return element;
    };
    return { a: made(toneHz.a), b: made(toneHz.b) };
  });
  useEffect(
    () => () => {
      for (const element of [tones.a, tones.b]) {
        element.pause();
        URL.revokeObjectURL(element.src);
      }
    },
    [tones],
  );
  return tones;
}

/** The manual recipe where the browser cannot place a window (casting-2026-10-25.md §5.1). */
function ExtendRecipe(): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <output className="lab__big">{t("lab.extend.noPlacing")}</output>
      <ol className="lab__recipe">
        <li>{t("lab.extend.recipeConnect")}</li>
        <li>{t("lab.extend.recipeDrag")}</li>
        <li>{t("lab.extend.recipeSound")}</li>
        <li>{t("lab.extend.recipeControls")}</li>
      </ol>
    </>
  );
}

/** The screens part: placing the player on another screen, or the recipe where it cannot. */
function ScreensPart(props: {
  readonly screens: readonly ScreenPlace[] | string | undefined;
  readonly onFind: () => void;
  readonly onOpen: (screen: ScreenPlace | undefined) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const found = typeof props.screens === "object" ? props.screens : [];
  return (
    <>
      <h2>{t("lab.extend.screens")}</h2>
      {"getScreenDetails" in globalThis ? (
        <div className="lab__actions">
          <button type="button" className="lab__button" onClick={props.onFind}>
            {t("lab.extend.findScreens")}
          </button>
        </div>
      ) : (
        <ExtendRecipe />
      )}
      {typeof props.screens === "string" ? <p role="alert">{props.screens}</p> : null}
      <div className="lab__actions">
        {found.map((screen) => (
          <button
            key={`${screen.label}-${String(screen.left)}`}
            type="button"
            className="lab__button"
            onClick={() => {
              props.onOpen(screen);
            }}
          >
            {t("lab.extend.openOn", {
              screen: screen.label,
              width: String(screen.width),
              height: String(screen.height),
            })}
          </button>
        ))}
        <button
          type="button"
          className="lab__button"
          onClick={() => {
            props.onOpen(undefined);
          }}
        >
          {t("lab.extend.openHere")}
        </button>
      </div>
    </>
  );
}

/** A choice of output for one tone. */
function ToneChoice(props: {
  readonly tone: "a" | "b";
  readonly outputs: readonly AudioOutput[];
  readonly chosen: string;
  readonly onChoose: (id: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <label className="lab__big">
      {t(`lab.extend.tone.${props.tone}`)}
      <select
        value={props.chosen}
        onChange={(event) => {
          props.onChoose(event.target.value);
        }}
      >
        {props.outputs.map((output) => (
          <option key={output.id} value={output.id}>
            {output.label}
          </option>
        ))}
      </select>
    </label>
  );
}

/** The choice of output for each tone. */
function ToneChoices(props: {
  readonly outputs: readonly AudioOutput[];
  readonly chosen: { readonly a: string; readonly b: string };
  readonly onChoose: (chosen: { readonly a: string; readonly b: string }) => void;
}): JSX.Element {
  return (
    <>
      {(["a", "b"] as const).map((tone) => (
        <ToneChoice
          key={tone}
          tone={tone}
          outputs={props.outputs}
          chosen={props.chosen[tone]}
          onChoose={(id) => {
            props.onChoose({ ...props.chosen, [tone]: id });
          }}
        />
      ))}
    </>
  );
}

/** Playing and stopping both tones. */
function ToneButtons(props: {
  readonly canPlay: boolean;
  readonly onPlay: () => void;
  readonly onStop: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <button
        type="button"
        className="lab__button"
        disabled={!props.canPlay}
        onClick={props.onPlay}
      >
        {t("lab.extend.playBoth")}
      </button>
      <button type="button" className="lab__button" onClick={props.onStop}>
        {t("lab.extend.stopBoth")}
      </button>
    </>
  );
}

/** What each tone did, in words. */
function ToneOutcomes(props: { readonly outcomes: readonly ToneOutcome[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <ul className="lab__big">
      {props.outcomes.map((outcome) => (
        <li key={outcome.tone}>
          {outcome.failure === undefined
            ? t("lab.extend.playingOn", { tone: outcome.tone, output: outcome.output })
            : t("lab.extend.couldNotPlay", { tone: outcome.tone, why: outcome.failure })}
        </li>
      ))}
    </ul>
  );
}

/** The output test: tone A on one output and tone B on another at once (voice to earbuds). */
function OutputsPart(props: {
  readonly onOutcomes: (outcomes: Record<string, unknown>) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [outputs, setOutputs] = useState<readonly AudioOutput[] | string | undefined>();
  const [chosen, setChosen] = useState({ a: "", b: "" });
  const [outcomes, setOutcomes] = useState<readonly ToneOutcome[]>([]);
  const tones = useTones();
  const list = typeof outputs === "object" ? outputs : [];
  const play = (): void => {
    const a = list.find((output) => output.id === chosen.a) ?? list[0];
    const b = list.find((output) => output.id === chosen.b) ?? list[1] ?? a;
    if (a === undefined || b === undefined) {
      return;
    }
    void Promise.all([playOn(tones.a, "A", a), playOn(tones.b, "B", b)]).then((played) => {
      setOutcomes(played);
      props.onOutcomes({
        outputCount: list.length,
        outputs: list.map((output) => output.label),
        played,
      });
    });
  };
  return (
    <>
      <p className="lab__lead">{t("lab.extend.outputsExplain")}</p>
      <div className="lab__actions">
        <button
          type="button"
          className="lab__button"
          onClick={() => {
            listOutputs().then(setOutputs, (cause: unknown) => {
              setOutputs(failureText(cause));
            });
          }}
        >
          {t("lab.extend.listOutputs")}
        </button>
        {list.length === 0 ? null : (
          <ToneChoices outputs={list} chosen={chosen} onChoose={setChosen} />
        )}
        <ToneButtons
          canPlay={list.length > 0}
          onPlay={play}
          onStop={() => {
            tones.a.pause();
            tones.b.pause();
          }}
        />
      </div>
      {typeof outputs === "string" ? <p role="alert">{outputs}</p> : null}
      <ToneOutcomes outcomes={outcomes} />
    </>
  );
}

/** The remote's buttons, and the player's state as its window last told it. */
function RemotePart(props: ReturnType<typeof usePlayerChannel>): JSX.Element {
  const { t } = useTranslation();
  const commands: readonly ExtendCommand[] = ["back", "toggle", "forward"];
  return (
    <>
      <h2>{t("lab.extend.remote")}</h2>
      <div className="lab__actions">
        {commands.map((command) => (
          <button
            key={command}
            type="button"
            className="lab__button"
            onClick={() => {
              props.command(command);
            }}
          >
            {t(`lab.extend.${command}`)}
          </button>
        ))}
      </div>
      <output className="lab__huge">
        {props.state === undefined
          ? t("lab.extend.noPlayer")
          : t("lab.extend.playerIs", {
              position: props.state.positionS.toFixed(1),
              playing: yesNo(t, props.state.isPlaying),
              fullscreen: yesNo(t, props.state.isFullscreen),
            })}
      </output>
    </>
  );
}

/** Asks the Window Management API for the screens; answers them, or why it could not. */
async function findScreens(): Promise<readonly ScreenPlace[] | string> {
  const ask: unknown = Reflect.get(globalThis, "getScreenDetails");
  if (typeof ask !== "function") {
    return "no getScreenDetails";
  }
  try {
    return screensOf(await Promise.resolve(Reflect.apply(ask, globalThis, [])));
  } catch (cause) {
    return failureText(cause);
  }
}

/**
 * The primitive extend-to-TV probe (plans/tv-device-tests.md; casting-2026-10-25.md §5.1): the
 * player opened on the TV's screen where the browser can place a window there (the Window
 * Management API, Chromium), this page staying as its remote; and two tones on two outputs at once,
 * for voice in earbuds while the film plays on the TV (review §5 Q7). Where an API is missing, it
 * says so and gives the manual recipe.
 */
export function LabExtend(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [screens, setScreens] = useState<readonly ScreenPlace[] | string | undefined>();
  const [opened, setOpened] = useState<string | undefined>();
  const [outputs, setOutputs] = useState<Record<string, unknown> | undefined>();
  const player = usePlayerChannel();
  const { sending, send } = useLabSending(api, "extend");
  const open = (screen: ScreenPlace | undefined): void => {
    const features = screen === undefined ? "popup" : windowFeaturesOn(screen);
    const href = routeHref({ screen: "lab-extend-player" });
    const window = globalThis.open(href, playerWindowName, features);
    setOpened(window === null ? "blocked" : (screen?.label ?? "a new window"));
  };
  const result = (found = outputs): Record<string, unknown> => ({
    hasGetScreenDetails: "getScreenDetails" in globalThis,
    hasSetSinkId: "setSinkId" in HTMLMediaElement.prototype,
    screens,
    opened,
    playerState: player.state,
    outputs: found,
    userAgent: navigator.userAgent,
  });
  return (
    <LabFrame id="lab-extend" heading={t("lab.extend.heading")}>
      <p className="lab__lead">{t("lab.extend.explain")}</p>
      <ScreensPart
        screens={screens}
        onFind={() => {
          void findScreens().then(setScreens);
        }}
        onOpen={open}
      />
      <RemotePart {...player} />
      <h2>{t("lab.extend.outputs")}</h2>
      {"setSinkId" in HTMLMediaElement.prototype ? (
        <OutputsPart
          onOutcomes={(found) => {
            setOutputs(found);
            send(result(found));
          }}
        />
      ) : (
        <output className="lab__big">{t("lab.extend.noSinkId")}</output>
      )}
      <LabSendButton
        sending={sending}
        onSend={() => {
          send(result());
        }}
      />
    </LabFrame>
  );
}
