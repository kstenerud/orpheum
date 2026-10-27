// SPDX-License-Identifier: MPL-2.0

import "./film-volume-slider.css";

import { canSetVolume } from "@orpheum/player";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { setFilmVolume, useFilmVolume } from "./film-volume.ts";
import { VolumeRange } from "./volume-range.tsx";

/**
 * Whether this browser lets a page set a film's volume: an iPhone's does not, its volume buttons
 * alone set it, and a slider there moved nothing (the owner's test, 2026-10-12; idiosyncrasies.md,
 * "iOS: an element's volume is read-only to the page").
 */
export function useIsVolumeSettable(): boolean {
  // A page drawn with no document, as a test draws it to markup, is taken to be able to.
  const [isSettable] = useState(
    () => typeof document === "undefined" || canSetVolume(document.createElement("video")),
  );
  return isSettable;
}

/** Where the page cannot set the film's volume, what does: the phone's own buttons. */
export function VolumeButtonsNote(): JSX.Element {
  const { t } = useTranslation();
  return <p className="film-volume__buttons">{t("together.volumeButtons")}</p>;
}

/**
 * The film's volume in this tab, which the host's player follows (D281 §5): in the Watch Together
 * controls, and on the host's player beside each person's voice (the owner, 2026-10-12); where the
 * page cannot set it, a word that the phone's buttons do.
 */
export function FilmVolumeSlider(): JSX.Element {
  const { t } = useTranslation();
  const volume = useFilmVolume();
  const isSettableHere = useIsVolumeSettable();
  if (!isSettableHere) {
    return <VolumeButtonsNote />;
  }
  return (
    <label className="film-volume">
      {t("watchTogether.filmVolume")}{" "}
      <VolumeRange value={volume} most={1} onChange={setFilmVolume} />
    </label>
  );
}
