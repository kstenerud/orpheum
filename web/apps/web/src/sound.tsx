// SPDX-License-Identifier: MPL-2.0

import "./sound.css";

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { useIsGuest } from "./guest-page.ts";
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
export function BoostSwitch(props: OwnChoiceWays): JSX.Element {
  return <BoostBox state={useOwnChoice("isDialogueBoostOn", props)} />;
}

/** The dialogue boost's switch, held by whoever holds the choice. */
export function BoostBox(props: { readonly state: OwnChoiceState }): JSX.Element {
  const { t } = useTranslation();
  return <OwnChoiceBox label={t("play.boost")} state={props.state} />;
}

/** One of the person's own choices as it stands: on or off, the way to switch it, and what failed. */
export interface OwnChoiceState {
  readonly isOn: boolean;
  readonly switchTo: (isOn: boolean) => void;
  /** Why keeping the last switch failed, in the viewer's words; undefined while nothing has. */
  readonly problem: string | undefined;
}

/** What one of the person's own choices starts as, and what follows its switching. */
export interface OwnChoiceWays {
  readonly isInitiallyOn: boolean;
  readonly onSwitched: (isOn: boolean) => void;
  readonly onKept?: ((isOn: boolean) => void) | undefined;
}

/**
 * One of the person's own choices, starting as they left it: switching it acts at once, through
 * onSwitched, and is kept for every file, onKept following once it is. While isOffered, as it is
 * unless said, a cast's remote shows it and works it, as a press here would (sound-panel.ts),
 * whether or not its switch is drawn.
 */
export function useOwnChoice(
  choice: OwnChoice,
  ways: OwnChoiceWays & { readonly isOffered?: boolean },
): OwnChoiceState {
  const { t } = useTranslation();
  const { api } = useServices();
  const [isOn, setIsOn] = useState(ways.isInitiallyOn);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const switchTo = (isNowOn: boolean): void => {
    setIsOn(isNowOn);
    ways.onSwitched(isNowOn);
    keepOwnChoice(api, choice, isNowOn).then(
      () => {
        setProblem(undefined);
        ways.onKept?.(isNowOn);
      },
      (cause: unknown) => {
        setProblem(problemText(t, cause));
      },
    );
  };
  const kind = choice === "isDialogueBoostOn" ? "boost" : "levelling";
  // A guest is refused the person's preferences it would keep (D469 §4, D471), so its remote is
  // offered no choice.
  const isGuest = useIsGuest();
  const isOffered = ways.isOffered !== false && !isGuest;
  useSoundPart(isOffered ? { kind, isOn } : undefined, (chosen) => {
    if ("isOn" in chosen && chosen.isOn !== isOn) {
      switchTo(chosen.isOn);
    }
  });
  return { isOn, switchTo, problem };
}

/** The switch of one of the person's own choices, whoever holds the choice, and what failed. */
export function OwnChoiceBox(props: {
  readonly label: string;
  readonly state: OwnChoiceState;
}): JSX.Element {
  const { isOn, switchTo, problem } = props.state;
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
