// SPDX-License-Identifier: MPL-2.0

import "./play.css";
import "./play-words.ts";

import { useLayoutEffect, useState, type JSX } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";

import { Pill } from "./pill.tsx";

/**
 * "The server restarted. Opening the film again where you were…" while a lost playback is opened
 * again, a centre Pill on the film's stage (D538 §1), so it shows while the film fills the window or
 * the screen. The stage is the film's that is drawn beside it, found once both are: the Play page
 * draws one; where none is drawn yet, the note stands in its own place.
 */
export function ReopeningNote(): JSX.Element {
  const { t } = useTranslation();
  const [stage, setStage] = useState<HTMLElement | null>(null);
  useLayoutEffect(() => {
    setStage(document.querySelector<HTMLElement>(".film-player__stage"));
  }, []);
  const note = (
    <div className="play__reopening">
      <Pill>{t("play.reopening")}</Pill>
    </div>
  );
  return stage === null ? note : createPortal(note, stage);
}
