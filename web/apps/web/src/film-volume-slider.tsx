// SPDX-License-Identifier: MPL-2.0

import "./film-volume-slider.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { setFilmVolume, useFilmVolume } from "./film-volume.ts";
import { VolumeRange } from "./volume-range.tsx";

/**
 * The film's volume in this tab, which the host's player follows (D281 §5): in the Watch Together
 * controls, and on the host's player beside each person's voice, where a phone's player has no
 * volume of its own (the owner, 2026-10-12).
 */
export function FilmVolumeSlider(): JSX.Element {
  const { t } = useTranslation();
  const volume = useFilmVolume();
  return (
    <label className="film-volume">
      {t("watchTogether.filmVolume")}{" "}
      <VolumeRange value={volume} most={1} onChange={setFilmVolume} />
    </label>
  );
}
