// SPDX-License-Identifier: MPL-2.0

import { playerSkippedEvent, skippedStretchOf, type SkippedStretch } from "@orpheum/player";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { ModalDialog } from "./modal-dialog.tsx";
import { clockText } from "./timeline.tsx";
import { WarningSign } from "./warning-sign.tsx";

/**
 * The film's stretches its player stepped over, the core unable to make them (toolkit.md item 12),
 * told by the owner's design (2026-11-03): "Can it just skip automatically, and maybe put up an
 * exclamation mark item in the non-fullscreen video page about the problem? If it's able to skip
 * forward, and will skip more than 15 seconds (or simply can't play any further), show a pop-up
 * dialog as well." The sign stays for the viewing; the pop-up shows in full screen too; the player
 * steps over two minutes at most (the owner's agreement, 2026-11-03).
 */

/** A stretch longer than this, in ms, is told in a pop-up too, not only by the sign. */
export const toldInAPopUpPastMs = 15_000;

/**
 * The stretches the player of videoRef stepped over in this viewing, in the order met: a stretch
 * that grows (chunks in a row not made) is one, at its latest length.
 */
export function useSkippedStretches(
  videoRef: RefObject<HTMLVideoElement | null>,
): readonly SkippedStretch[] {
  const [stretches, setStretches] = useState<readonly SkippedStretch[]>([]);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const heard = (event: Event): void => {
      const stretch = skippedStretchOf(event);
      if (stretch !== undefined) {
        setStretches((was) => [...was.filter(({ fromMs }) => fromMs !== stretch.fromMs), stretch]);
      }
    };
    video.addEventListener(playerSkippedEvent, heard);
    return () => {
      video.removeEventListener(playerSkippedEvent, heard);
    };
  }, [videoRef]);
  return stretches;
}

/** "42:13 to 42:40", a stretch as the timeline tells its times. */
function stretchText(stretch: SkippedStretch): string {
  return `${clockText(stretch.fromMs)}–${clockText(stretch.toMs)}`;
}

/**
 * The sign by the controls under the film (D278's warning sign) once a stretch was stepped over:
 * which, and why; nothing before.
 */
export function SkippedSign(props: {
  readonly stretches: readonly SkippedStretch[];
}): JSX.Element | null {
  const { t } = useTranslation();
  const { stretches } = props;
  if (stretches.length === 0) {
    return null;
  }
  const told = t("skipped.told", { stretches: stretches.map(stretchText).join(", ") });
  return (
    <WarningSign
      summary={t("skipped.summary")}
      forAdmin={`${told} ${t("skipped.forAdmin")}`}
      forViewer={told}
    />
  );
}

/**
 * Which pop-up a stretch's is: its start, and whether it cannot go on, so a stretch whose long skip
 * was told and closed is told again as the film cannot go on past it.
 */
function popUpOf(stretch: SkippedStretch): string {
  return `${String(stretch.fromMs)}${stretch.cannotGoOn === true ? " cannot go on" : ""}`;
}

/**
 * The pop-up that tells a stretch longer than toldInAPopUpPastMs as it is stepped over, or one the
 * film cannot go on past, each once, over the film, its full screen too, where the dialog, a modal,
 * is drawn above the stage and the failure's words under the film are not seen.
 */
export function SkippedPopUp(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element | null {
  const { t } = useTranslation();
  const stretches = useSkippedStretches(props.videoRef);
  const [closed, setClosed] = useState<readonly string[]>([]);
  const shown = stretches.find(
    (stretch) =>
      (stretch.cannotGoOn === true || stretch.toMs - stretch.fromMs > toldInAPopUpPastMs) &&
      !closed.includes(popUpOf(stretch)),
  );
  if (shown === undefined) {
    return null;
  }
  const summary = shown.cannotGoOn === true ? t("skipped.cannotGoOn") : t("skipped.summary");
  const told =
    shown.cannotGoOn === true
      ? t("skipped.cannotGoOnTold", { from: clockText(shown.fromMs) })
      : t("skipped.told", { stretches: stretchText(shown) });
  return (
    <ModalDialog
      label={summary}
      role="alertdialog"
      onClose={() => {
        setClosed((was) => [...was, popUpOf(shown)]);
      }}
    >
      <h2>{summary}</h2>
      <p>{told}</p>
    </ModalDialog>
  );
}
