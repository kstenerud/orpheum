// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

/** A place a root can move to: one of a connected node's configured roots. */
export interface Place {
  readonly nodeName: string;
  readonly path: string;
}

/**
 * Saying a root moved (D241 §6): the admin chooses where it is now, among the connected nodes'
 * roots, and confirms; the root and its files take the new path with their history, and the next
 * scan confirms each. The root's own place is not offered.
 */
export function RepointRoot(props: {
  readonly from: Place;
  readonly places: readonly Place[];
  readonly onRepoint: (to: Place) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const choices = props.places.filter(
    (place) => place.nodeName !== props.from.nodeName || place.path !== props.from.path,
  );
  const [chosenIndex, setChosenIndex] = useState<number | undefined>(undefined);
  if (choices.length === 0) {
    return null;
  }
  if (chosenIndex === undefined) {
    return (
      <button
        type="button"
        onClick={() => {
          setChosenIndex(0);
        }}
      >
        {t("libraries.repoint")}
      </button>
    );
  }
  return (
    <RepointChoice
      choices={choices}
      chosenIndex={chosenIndex}
      onChoose={setChosenIndex}
      onConfirm={() => {
        const to = choices[chosenIndex];
        setChosenIndex(undefined);
        if (to !== undefined) {
          props.onRepoint(to);
        }
      }}
      onKeep={() => {
        setChosenIndex(undefined);
      }}
    />
  );
}

/** Where the root is now, chosen among the places, and confirming or keeping it where it is. */
function RepointChoice(props: {
  readonly choices: readonly Place[];
  readonly chosenIndex: number;
  readonly onChoose: (index: number) => void;
  readonly onConfirm: () => void;
  readonly onKeep: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <span>
      <label>
        {t("libraries.repointTo")}{" "}
        <select
          value={props.chosenIndex}
          onChange={(event) => {
            props.onChoose(Number(event.currentTarget.value));
          }}
        >
          {props.choices.map((place, index) => (
            <option key={`${place.nodeName}:${place.path}`} value={index}>
              {t("libraries.root", { node: place.nodeName, path: place.path })}
            </option>
          ))}
        </select>
      </label>{" "}
      {t("libraries.repointConsequence")}{" "}
      <button type="button" onClick={props.onConfirm}>
        {t("libraries.repointConfirm")}
      </button>{" "}
      <button type="button" onClick={props.onKeep}>
        {t("libraries.repointKeep")}
      </button>
    </span>
  );
}
