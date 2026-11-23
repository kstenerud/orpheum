// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import { playerSkippedEvent, skippedStretchOf, type SkippedStretch } from "@orpheum/player";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { clockText } from "./date-time-text.ts";
import { useLatest } from "./latest.ts";
import { DialogClose, ModalDialog } from "./modal-dialog.tsx";
import { Pill } from "./pill.tsx";
import { WarningSign } from "./warning-sign.tsx";

/**
 * The film's stretches its player stepped over, the core unable to make them (toolkit.md item 12),
 * told by the owner's design (2026-11-03): "Can it just skip automatically, and maybe put up an
 * exclamation mark item in the non-fullscreen video page about the problem? If it's able to skip
 * forward, and will skip more than 15 seconds (or simply can't play any further), show a pop-up
 * dialog as well." The sign stays for the viewing; the pop-up shows in full screen too; the player
 * steps over two minutes at most (the owner's agreement, 2026-11-03).
 * D538 §1 (the owner, 2026-11-22) makes the long stretch's pop-up a Pill: it needs no answer.
 */

/** A stretch longer than this, in ms, is told over the film too, not only by the sign. */
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

/** "42:13 to 42:40", a stretch as the timeline tells its times, in locale's digits. */
function stretchText(stretch: SkippedStretch, locale: string): string {
  return `${clockText(stretch.fromMs, locale)}–${clockText(stretch.toMs, locale)}`;
}

/**
 * The sign by the controls under the film (D278's warning sign) once a stretch was stepped over:
 * which, and why; nothing before.
 */
export function SkippedSign(props: {
  readonly stretches: readonly SkippedStretch[];
}): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { stretches } = props;
  if (stretches.length === 0) {
    return null;
  }
  const told = t("skipped.told", {
    stretches: stretches.map((stretch) => stretchText(stretch, i18n.language)).join(", "),
  });
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
  return `${String(stretch.fromMs)}${stretch.isWhereItStops === true ? " cannot go on" : ""}`;
}

/**
 * How long a long stretch's Pill shows over the film (SkippedPopUp), in ms: as long as the other
 * notes over the film that need no answer (castNoteShownMs, comingAndGoingShownMs), long enough to
 * read its line once. A justified constant.
 */
export const skippedShownMs = 5000;

/**
 * What tells a stretch longer than toldInAPopUpPastMs as it is stepped over, or one the film
 * cannot go on past, each once, over the film, its full screen too, where the failure's words under
 * the film are not seen. A long stretch stepped over needs no answer: a Pill in the film's middle
 * for skippedShownMs (D538 §1, dialogs-spec's "Part skipped"), said as it shows. The film that
 * cannot go on stops, which the viewer must know: a dialog, a modal, drawn above the stage.
 */
export function SkippedPopUp(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const stretches = useSkippedStretches(props.videoRef);
  const [closed, setClosed] = useState<readonly string[]>([]);
  const shown = stretches.find(
    (stretch) =>
      (stretch.isWhereItStops === true || stretch.toMs - stretch.fromMs > toldInAPopUpPastMs) &&
      !closed.includes(popUpOf(stretch)),
  );
  const close = (): void => {
    if (shown !== undefined) {
      setClosed((was) => [...was, popUpOf(shown)]);
    }
  };
  if (shown === undefined) {
    return null;
  }
  if (shown.isWhereItStops !== true) {
    return (
      <SkippedPill
        key={popUpOf(shown)}
        told={t("skipped.told", { stretches: stretchText(shown, i18n.language) })}
        onGone={close}
      />
    );
  }
  const summary = t("skipped.cannotGoOn");
  return (
    <ModalDialog
      label={summary}
      role="alertdialog"
      onClose={close}
      heading={summary}
      footer={<DialogClose onClose={close} />}
    >
      <p>{t("skipped.cannotGoOnTold", { from: clockText(shown.fromMs, i18n.language) })}</p>
    </ModalDialog>
  );
}

/** A long stretch's line in a Pill in the film's middle, gone after skippedShownMs (onGone). */
function SkippedPill(props: { readonly told: string; readonly onGone: () => void }): JSX.Element {
  const onGoneRef = useLatest(props.onGone);
  useEffect(() => {
    const timer = setTimeout(() => {
      onGoneRef.current();
    }, skippedShownMs);
    return () => {
      clearTimeout(timer);
    };
  }, [onGoneRef]);
  return (
    <span className="player-overlay-centre skipped-parts__pill">
      <Pill voice="said">{props.told}</Pill>
    </span>
  );
}
