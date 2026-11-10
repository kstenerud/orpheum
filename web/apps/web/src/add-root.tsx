// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { FolderBrowser, type Place } from "./folder-picker.tsx";

/**
 * Adding a folder to a library (D359 §3): one of the connected nodes' configured roots, or a folder
 * browsed to inside one, chosen in the folder browser's dialog (D361 §3), which the server adds and
 * scans. Nothing is offered while no node is connected.
 */
export function AddRoot(props: {
  readonly places: readonly Place[];
  readonly onAdd: (place: Place) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.places.length === 0) {
    return null;
  }
  return (
    <FolderBrowser
      opener={t("libraries.addRoot")}
      heading={t("libraries.addRootHeading")}
      starts={props.places}
      onChoose={props.onAdd}
    />
  );
}
