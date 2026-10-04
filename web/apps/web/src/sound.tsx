// SPDX-License-Identifier: MPL-2.0

import { useRequest } from "./use-request.ts";
import "./sound.css";

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { setMyDeviceAudioOutput, type AudioOutput } from "./accounts.ts";
import { camelKey } from "./catalogue-key.ts";
import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";

/** The three ways a device makes sound, as the question offers them (D184, D222). */
export const audioOutputs: readonly AudioOutput[] = ["built_in_speakers", "receiver", "headphones"];

/** A way a device makes sound, in the person's words. */
export function audioOutputName(t: TFunction, output: AudioOutput): string {
  return t(`sound.${camelKey(output)}`);
}

/**
 * A choice of the person's own, which holds in every file they play: its name in PUT /me/preferences,
 * taken from the API's own body so a renamed field fails to compile rather than being refused.
 */
type OwnChoice = keyof components["schemas"]["SetMyPreferencesInputBody"];

/** Keeps one of the person's own choices: the dialogue boost (D83, D222), levelling (D226). */
async function keepOwnChoice(api: Api, choice: OwnChoice, isOn: boolean): Promise<void> {
  throwIfRefused(await api.PUT("/me/preferences", { body: { [choice]: isOn } }));
}

/**
 * The question of how this device makes sound, with what each answer does: asked before its first
 * playback, and again whenever the person changes it. The answer is kept on the device, and
 * onAnswered() follows.
 */
export function SoundQuestion(props: {
  readonly current?: AudioOutput | undefined;
  readonly onAnswered: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const deviceId = session.deviceId();
  const choose = (output: AudioOutput): void => {
    if (deviceId === undefined) {
      return;
    }
    runRequest(async () => {
      await setMyDeviceAudioOutput(api, deviceId, output);
      props.onAnswered();
    });
  };
  return (
    <fieldset className="sound" disabled={isBusy}>
      <legend>{t("sound.question")}</legend>
      <p>{t("sound.why")}</p>
      {audioOutputs.map((output) => (
        <button
          key={output}
          type="button"
          className="sound__answer"
          aria-pressed={props.current === output}
          onClick={() => {
            choose(output);
          }}
        >
          <strong>{audioOutputName(t, output)}</strong>
          <span>{t(`sound.${camelKey(output)}Hint`)}</span>
        </button>
      ))}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </fieldset>
  );
}

/** How this device makes sound, under the video, and changing it, which asks for the decision again. */
export function SoundLine(props: {
  readonly current: AudioOutput;
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [isChanging, setIsChanging] = useState(false);
  return (
    <div className="sound__line">
      {t("sound.current", { output: audioOutputName(t, props.current) })}{" "}
      <button
        type="button"
        aria-expanded={isChanging}
        onClick={() => {
          setIsChanging(!isChanging);
        }}
      >
        {t("sound.change")}
      </button>
      {isChanging ? <SoundQuestion current={props.current} onAnswered={props.onChanged} /> : null}
    </div>
  );
}

/**
 * The dialogue boost's switch, starting as the person left it: switching it changes the mix at
 * once and is kept for every file.
 */
export function BoostSwitch(props: {
  readonly isInitiallyOn: boolean;
  readonly onSwitched: (isOn: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return <OwnChoiceSwitch choice="isDialogueBoostOn" label={t("play.boost")} {...props} />;
}

/**
 * A switch for one of the person's own choices, starting as they left it: switching it acts at
 * once, through onSwitched, and is kept for every file.
 */
export function OwnChoiceSwitch(props: {
  readonly choice: OwnChoice;
  readonly label: string;
  readonly isInitiallyOn: boolean;
  readonly onSwitched: (isOn: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [isOn, setIsOn] = useState(props.isInitiallyOn);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  return (
    <>
      <label className="sound__switch">
        <input
          type="checkbox"
          checked={isOn}
          onChange={(event) => {
            const isNowOn = event.currentTarget.checked;
            setIsOn(isNowOn);
            props.onSwitched(isNowOn);
            keepOwnChoice(api, props.choice, isNowOn).then(
              () => {
                setProblem(undefined);
              },
              (cause: unknown) => {
                setProblem(problemText(t, cause));
              },
            );
          }}
        />
        {props.label}
      </label>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
}
