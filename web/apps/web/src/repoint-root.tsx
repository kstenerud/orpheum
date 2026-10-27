// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { FolderPicker, type Place } from "./folder-picker.tsx";

export type { Place } from "./folder-picker.tsx";

/** Whether two places are one: the same node and path. */
function isSamePlace(a: Place, b: Place): boolean {
  return a.nodeName === b.nodeName && a.path === b.path;
}

/**
 * Saying a root moved (D241 §6): the admin chooses where it is now, among the connected nodes'
 * roots, or a folder browsed to inside one (D359 §3), and confirms; the root and its files take the
 * new path with their history, and the next scan confirms each. The root's own place is not offered.
 */
export function RepointRoot(props: {
  readonly from: Place;
  readonly places: readonly Place[];
  readonly onRepoint: (to: Place) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const [browsed, setBrowsed] = useState<Place | undefined>(undefined);
  const offered = props.places.filter((place) => !isSamePlace(place, props.from));
  const choices = browsed === undefined ? offered : [...offered, browsed];
  const [chosenIndex, setChosenIndex] = useState<number | undefined>(undefined);
  if (props.places.length === 0) {
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
  const close = (): void => {
    setChosenIndex(undefined);
    setBrowsed(undefined);
  };
  return (
    <>
      <RepointChoice
        choices={choices}
        chosenIndex={chosenIndex}
        onChoose={setChosenIndex}
        onConfirm={() => {
          const to = choices[chosenIndex];
          close();
          if (to !== undefined) {
            props.onRepoint(to);
          }
        }}
        onKeep={close}
      />
      <fieldset>
        <legend>{t("libraries.browseInside")}</legend>
        <FolderPicker
          starts={props.places}
          onChoose={(place) => {
            const known = offered.findIndex((each) => isSamePlace(each, place));
            if (!isSamePlace(place, props.from)) {
              setBrowsed(known === -1 ? place : undefined);
              setChosenIndex(known === -1 ? offered.length : known);
            }
          }}
        />
      </fieldset>
    </>
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
