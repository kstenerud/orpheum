// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { arrivalMark } from "./focus-navigation.ts";
import { FolderBrowser, type Place } from "./folder-picker.tsx";
import { ModalDialog } from "./modal-dialog.tsx";

export type { Place } from "./folder-picker.tsx";

/** Whether two places are one: the same node and path. */
function isSamePlace(a: Place, b: Place): boolean {
  return a.nodeName === b.nodeName && a.path === b.path;
}

/**
 * Saying a root moved (D241 §6), its "This folder moved…" under its "…" (ui-audit-2026-11-04.md §3,
 * A library): a dialog where the admin chooses where it is now, among the connected nodes' roots, or
 * a folder browsed to inside one in the folder browser's dialog (D359 §3, D361 §3), and confirms;
 * the root and its files take the new path with their history, and the next scan confirms each. The
 * root's own place is not offered. "Keep it where it is", the least harmful choice, is focused as it
 * opens.
 */
export function RepointDialog(props: {
  readonly from: Place;
  readonly places: readonly Place[];
  readonly onRepoint: (to: Place) => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const heading = t("libraries.repointHeading");
  return (
    <ModalDialog label={heading} onClose={props.onClose}>
      <h2>{heading}</h2>
      <RepointForm
        from={props.from}
        places={props.places}
        onRepoint={(to) => {
          props.onClose();
          props.onRepoint(to);
        }}
        onKeep={props.onClose}
      />
    </ModalDialog>
  );
}

/**
 * Where the root is now, chosen among the places offered or a folder browsed to inside one, then
 * confirmed, or kept where it is.
 */
function RepointForm(props: {
  readonly from: Place;
  readonly places: readonly Place[];
  readonly onRepoint: (to: Place) => void;
  readonly onKeep: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [browsed, setBrowsed] = useState<Place | undefined>(undefined);
  const offered = props.places.filter((place) => !isSamePlace(place, props.from));
  const choices = browsed === undefined ? offered : [...offered, browsed];
  const [chosenIndex, setChosenIndex] = useState(0);
  return (
    <>
      <RepointChoice
        choices={choices}
        chosenIndex={chosenIndex}
        onChoose={setChosenIndex}
        onConfirm={() => {
          const to = choices[chosenIndex];
          if (to !== undefined) {
            props.onRepoint(to);
          }
        }}
        onKeep={props.onKeep}
      />{" "}
      <FolderBrowser
        opener={t("libraries.browseInside")}
        heading={t("libraries.repointHeading")}
        starts={props.places}
        onChoose={(place) => {
          const known = offered.findIndex((each) => isSamePlace(each, place));
          if (!isSamePlace(place, props.from)) {
            setBrowsed(known === -1 ? place : undefined);
            setChosenIndex(known === -1 ? offered.length : known);
          }
        }}
      />
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
      <button type="button" {...arrivalMark} onClick={props.onKeep}>
        {t("libraries.repointKeep")}
      </button>
    </span>
  );
}
