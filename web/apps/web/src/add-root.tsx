// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { FolderPicker, type Place } from "./folder-picker.tsx";

/**
 * Adding a folder to a library (D359 §3): one of the connected nodes' configured roots, or a folder
 * browsed to inside one, which the server adds and scans. Nothing is offered while no node is
 * connected.
 */
export function AddRoot(props: {
  readonly places: readonly Place[];
  readonly onAdd: (place: Place) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const [isBrowsing, setIsBrowsing] = useState(false);
  if (props.places.length === 0) {
    return null;
  }
  if (!isBrowsing) {
    return (
      <button
        type="button"
        onClick={() => {
          setIsBrowsing(true);
        }}
      >
        {t("libraries.addRoot")}
      </button>
    );
  }
  return (
    <fieldset>
      <legend>{t("libraries.addRoot")}</legend>
      <FolderPicker
        starts={props.places}
        onChoose={(place) => {
          setIsBrowsing(false);
          props.onAdd(place);
        }}
      />
      <button
        type="button"
        onClick={() => {
          setIsBrowsing(false);
        }}
      >
        {t("libraries.addRootCancel")}
      </button>
    </fieldset>
  );
}
