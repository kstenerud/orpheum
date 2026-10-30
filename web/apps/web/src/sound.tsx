// SPDX-License-Identifier: MPL-2.0

import "./sound.css";

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useSoundPart } from "./sound-panel.ts";

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
 * The dialogue boost's switch, starting as the person left it: switching it changes the mix at
 * once, where the mix is there to change, and is kept for every file; onKept follows once it is
 * kept, for a film playing as it is to be asked for again with the boost (D336 §3).
 */
export function BoostSwitch(props: {
  readonly isInitiallyOn: boolean;
  readonly onSwitched: (isOn: boolean) => void;
  readonly onKept?: ((isOn: boolean) => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  return <OwnChoiceSwitch choice="isDialogueBoostOn" label={t("play.boost")} {...props} />;
}

/**
 * A switch for one of the person's own choices, starting as they left it: switching it acts at
 * once, through onSwitched, and is kept for every file, onKept following once it is.
 */
export function OwnChoiceSwitch(props: {
  readonly choice: OwnChoice;
  readonly label: string;
  readonly isInitiallyOn: boolean;
  readonly onSwitched: (isOn: boolean) => void;
  readonly onKept?: ((isOn: boolean) => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [isOn, setIsOn] = useState(props.isInitiallyOn);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const switchTo = (isNowOn: boolean): void => {
    setIsOn(isNowOn);
    props.onSwitched(isNowOn);
    keepOwnChoice(api, props.choice, isNowOn).then(
      () => {
        setProblem(undefined);
        props.onKept?.(isNowOn);
      },
      (cause: unknown) => {
        setProblem(problemText(t, cause));
      },
    );
  };
  // A cast's remote shows the switch and works it, as a press here would (sound-panel.ts).
  useSoundPart(
    { kind: props.choice === "isDialogueBoostOn" ? "boost" : "levelling", isOn },
    (choice) => {
      if ("isOn" in choice && choice.isOn !== isOn) {
        switchTo(choice.isOn);
      }
    },
  );
  return (
    <>
      <label className="sound__switch">
        <input
          type="checkbox"
          checked={isOn}
          onChange={(event) => {
            switchTo(event.currentTarget.checked);
          }}
        />
        {props.label}
      </label>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
}
