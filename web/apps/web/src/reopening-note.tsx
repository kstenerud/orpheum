// SPDX-License-Identifier: MPL-2.0

import "./play.css";
import "./play-words.ts";

import { useSyncExternalStore, type JSX } from "react";
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
  const stage = useSyncExternalStore(watchThePage, drawnStage);
  const note = (
    <div className="play__reopening">
      <Pill>{t("play.reopening")}</Pill>
    </div>
  );
  return stage === null ? note : createPortal(note, stage);
}

/** The film's stage the page draws, or null while it draws none. */
function drawnStage(): HTMLElement | null {
  return document.querySelector<HTMLElement>(".film-player__stage");
}

/** Calls changed whenever the page's elements change, so a stage drawn later is found. */
function watchThePage(changed: () => void): () => void {
  const watcher = new MutationObserver(changed);
  watcher.observe(document.body, { childList: true, subtree: true });
  return () => {
    watcher.disconnect();
  };
}
