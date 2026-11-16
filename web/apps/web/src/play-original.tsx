// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { DebugTool } from "./debug-tools.tsx";

/**
 * The button that plays the film's own file in place of its ladder, as it is, from where it stands:
 * playback is never a black box (web-client.md), and a browser that plays the ladder badly can be
 * tried on the file itself. A debug tool, shown only to a person who shows them (the owner,
 * 2026-10-29: "I'm not even sure I want to expose that option to the users tbh. It's useful as a
 * debug tool though."); it loads the file's own source in place of the ladder.
 */
export function PlayOriginal(props: { readonly onChosen: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <DebugTool>
      <button type="button" className="play__original" onClick={props.onChosen}>
        {t("play.playOriginal")}
      </button>
    </DebugTool>
  );
}

/**
 * Word, in place of PlayOriginal, that the original its person chose waits for the link: the film
 * plays its rung of height until the link carries the original, then moves to it (D501 §3; the
 * decision's isOriginalWaitingForTheLink). A debug tool, as the button it answers is.
 */
export function OriginalWaits(props: { readonly height: number }): JSX.Element {
  const { t } = useTranslation();
  return (
    <DebugTool>
      <p className="play__original-waits">
        {t("play.originalWaitsForTheLink", { height: props.height })}
      </p>
    </DebugTool>
  );
}

/**
 * Word that the film moved up to the original its person chose, the link having carried it
 * (D501 §3). A debug tool, as the word that it waited is.
 */
export function OriginalMovedTo(): JSX.Element {
  const { t } = useTranslation();
  return (
    <DebugTool>
      <p className="play__original-moved-to">{t("play.originalMovedTo")}</p>
    </DebugTool>
  );
}
