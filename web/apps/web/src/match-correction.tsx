// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { CandidateChooser, identifyProblemText } from "./candidate-chooser.tsx";
import { FilingReportButton } from "./filing-report.tsx";
import { idempotent } from "./idempotent.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import type { MoreAction } from "./more-actions.tsx";
import { dataOrThrow, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/**
 * An admin's "Identified incorrectly" on a film's, an episode's or a series' page, one entry of its
 * "…" menu for what all stems from a file wrongly identified (the owner, 2026-10-29: "keep only
 * 'Identified incorrectly', which opens a modal ... offering Rescan, Enter manually, and Report
 * this"). isWork says the item is a film or a series, which alone a rescan and a chosen match apply
 * to (D357 §1, D301 §2); an episode's or another video's offers only its report (D306 §3). The page
 * gives it to an admin alone.
 */
export function identifiedIncorrectlyAction(
  t: ReturnType<typeof useTranslation>["t"],
  item: { readonly itemId: number; readonly isWork: boolean; readonly onCorrected: () => void },
): MoreAction {
  return {
    key: "identified-incorrectly",
    label: t("title.identifiedIncorrectly"),
    dialog: (close) => <IdentifiedIncorrectlyDialog {...item} onClose={close} />,
  };
}

/**
 * An admin's "Identified correctly" beside "Identified incorrectly" on a page whose match is a
 * guess (D301 §4, D307 §2): it confirms the match as it stands, a person's from then on, and the
 * page, told, reads it again without its guessed mark, and so without this entry (the owner,
 * 2026-10-30: "No way to mark a guessed film 'identified correctly'"). The page gives it to an admin
 * alone, on an item whose match is a guess alone.
 */
export function identifiedCorrectlyAction(
  t: ReturnType<typeof useTranslation>["t"],
  item: { readonly itemId: number; readonly onConfirmed: () => void },
): MoreAction {
  return {
    key: "identified-correctly",
    label: t("title.identifiedCorrectly"),
    dialog: (close) => <IdentifiedCorrectlyDialog {...item} onClose={close} />,
  };
}

/** The dialog "Identified correctly" opens: what confirming does, and Confirm. */
function IdentifiedCorrectlyDialog(props: {
  readonly itemId: number;
  readonly onConfirmed: () => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const confirming = useRequest((cause) => identifyProblemText(t, cause));
  const heading = t("title.identifiedCorrectly");
  return (
    <ModalDialog label={heading} onClose={props.onClose}>
      <h2>{heading}</h2>
      <p>{t("title.identifiedCorrectlyExplain")}</p>
      <div className="more-actions__offers">
        <button
          type="button"
          disabled={confirming.isBusy}
          onClick={() => {
            confirming.runRequest(async (idempotencyKey) => {
              throwIfRefused(
                await api.POST("/items/{id}/confirm-match", {
                  params: { path: { id: props.itemId } },
                  ...idempotent(idempotencyKey),
                }),
              );
              props.onClose();
              props.onConfirmed();
            });
          }}
        >
          {t("title.confirmMatch")}
        </button>
        {confirming.problem === undefined ? null : <output>{confirming.problem}</output>}
      </div>
    </ModalDialog>
  );
}

/**
 * The dialog "Identified incorrectly" opens: Rescan, Enter manually, which opens the works its search
 * found to choose one as its match, final as any person's choice (D301 §2), and Report this. A press
 * outside it closes it, as every dialog a menu opens does (the owner, 2026-10-29).
 */
function IdentifiedIncorrectlyDialog(props: {
  readonly itemId: number;
  readonly isWork: boolean;
  readonly onCorrected: () => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [isChoosing, setIsChoosing] = useState(false);
  const heading = t("title.identifiedIncorrectly");
  return (
    <ModalDialog label={heading} onClose={props.onClose}>
      <h2>{heading}</h2>
      <div className="more-actions__offers">
        {props.isWork ? <RescanThis itemId={props.itemId} /> : null}
        {props.isWork ? (
          <button
            type="button"
            aria-expanded={isChoosing}
            onClick={() => {
              setIsChoosing(!isChoosing);
            }}
          >
            {t("identify.enterManually")}
          </button>
        ) : null}
        {isChoosing ? (
          <CandidateChooser
            itemId={props.itemId}
            onChosen={() => {
              props.onClose();
              props.onCorrected();
            }}
          />
        ) : null}
        <FilingReportButton itemId={props.itemId} />
      </div>
    </ModalDialog>
  );
}

/**
 * Rescan: the item's files scanned, then the item matched again from them, in the background (D357
 * §1, §5); and beside it its full rescan, its files read again first (D392 §1). Each says whether it
 * started, or one already runs.
 */
function RescanThis(props: { readonly itemId: number }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [note, setNote] = useState("");
  const rescanning = useRequest((cause) => identifyProblemText(t, cause));
  const rescan = (isFull: boolean): void => {
    setNote("");
    rescanning.runRequest(async (idempotencyKey) => {
      const answer = dataOrThrow(
        await api.POST("/items/{id}/rescan", {
          params: { path: { id: props.itemId }, ...(isFull ? { query: { full: true } } : {}) },
          ...idempotent(idempotencyKey),
        }),
      );
      const started = isFull
        ? t("libraries.fullRescanThisStarted")
        : t("libraries.rescanThisStarted");
      setNote(answer.isStarted ? started : t("libraries.rescanThisRunning"));
    });
  };
  return (
    <div>
      <button
        type="button"
        disabled={rescanning.isBusy}
        onClick={() => {
          rescan(false);
        }}
      >
        {t("libraries.rescanThis")}
      </button>{" "}
      <button
        type="button"
        title={t("libraries.fullRescanHelp")}
        disabled={rescanning.isBusy}
        onClick={() => {
          rescan(true);
        }}
      >
        {t("libraries.fullRescan")}
      </button>
      {note === "" ? null : <output>{note}</output>}
      {rescanning.problem === undefined ? null : <output>{rescanning.problem}</output>}
    </div>
  );
}
