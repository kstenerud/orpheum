// SPDX-License-Identifier: MPL-2.0

import "./sound.css";

import type { Api } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { setMyDeviceSound, type AudioOutput } from "./accounts.ts";
import { camelKey } from "./catalogue-key.ts";
import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";

/** The three ways a device makes sound, as the question offers them (D184, D222). */
export const audioOutputs: readonly AudioOutput[] = ["tv_speakers", "receiver", "headphones"];

/** A choice of the person's own, which holds in every file they play: its name in PUT /me/prefs. */
type OwnChoice = "isDialogueBoost" | "isLevelling";

/** Keeps one of the person's own choices: the dialogue boost (D83, D222), levelling (D226). */
async function keepOwnChoice(api: Api, choice: OwnChoice, isOn: boolean): Promise<void> {
  throwIfRefused(await api.PUT("/me/prefs", { body: { [choice]: isOn } }));
}

/**
 * The question of how this device makes sound, with what each answer does: asked before its first
 * playback, and again whenever the person changes it. The answer is kept on the device, and
 * answered() follows.
 */
export function SoundQuestion(props: {
  readonly current?: AudioOutput | undefined;
  readonly answered: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, session } = useServices();
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const deviceId = session.deviceId();
  const choose = (output: AudioOutput): void => {
    if (deviceId === undefined) {
      return;
    }
    setMyDeviceSound(api, deviceId, output).then(props.answered, (cause: unknown) => {
      setProblem(problemText(t, cause));
    });
  };
  return (
    <fieldset className="sound">
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
          <strong>{t(`sound.${camelKey(output)}`)}</strong>
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
  readonly changed: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [isChanging, setIsChanging] = useState(false);
  return (
    <div className="sound__line">
      {t("sound.current", { output: t(`sound.${camelKey(props.current)}`) })}{" "}
      <button
        type="button"
        aria-expanded={isChanging}
        onClick={() => {
          setIsChanging(!isChanging);
        }}
      >
        {t("sound.change")}
      </button>
      {isChanging ? <SoundQuestion current={props.current} answered={props.changed} /> : null}
    </div>
  );
}

/**
 * The dialogue boost's switch, starting as the person left it: switching it changes the mix at
 * once and is kept for every file.
 */
export function BoostSwitch(props: {
  readonly isOn: boolean;
  readonly switched: (isOn: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return <OwnSwitch choice="isDialogueBoost" label={t("play.boost")} {...props} />;
}

/**
 * A switch for one of the person's own choices, starting as they left it: switching it acts at
 * once, through switched, and is kept for every file.
 */
export function OwnSwitch(props: {
  readonly choice: OwnChoice;
  readonly label: string;
  readonly isOn: boolean;
  readonly switched: (isOn: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [isOn, setIsOn] = useState(props.isOn);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  return (
    <>
      <label className="play__switch">
        <input
          type="checkbox"
          checked={isOn}
          onChange={(event) => {
            const isNowOn = event.currentTarget.checked;
            setIsOn(isNowOn);
            props.switched(isNowOn);
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
