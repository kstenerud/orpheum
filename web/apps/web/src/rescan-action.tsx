// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { identifyProblemText } from "./candidate-chooser.tsx";
import { ConfirmDialog } from "./confirm-button.tsx";
import { idempotent } from "./idempotent.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import type { MoreAction } from "./more-actions.tsx";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/**
 * An admin's "Rescan" in a video page's "…" menu, a film's, a series', an episode's or an other
 * video's (D535 §3, §5): the page's files read again entirely, a series' every episode's, what their
 * differences touch thrown out (D392), and a film or a series then matched again unless a person
 * chose its match (D357). It is the one place a video's page offers it; the "Identified incorrectly"
 * dialog does not. The page gives it to an admin alone, and never on an extra's page.
 */
export function rescanAction(
  t: ReturnType<typeof useTranslation>["t"],
  itemId: number,
): MoreAction {
  return {
    key: "rescan",
    label: t("libraries.rescan"),
    dialog: (close) => <RescanDialog itemId={itemId} onClose={close} />,
  };
}

/**
 * The question every Rescan asks first (D535 §6): that it is expensive, a library's that it can take
 * hours, with Yes, No and an unticked "Discard cached data", which has the rescan throw out
 * everything made or fetched for its scope, whatever differs. No, Escape or Back send nothing.
 */
export function RescanQuestion(props: {
  readonly isLong: boolean;
  readonly onRescan: (isDiscarding: boolean) => void;
  readonly onClose: () => void;
  /** Whether Yes closes the question, as it does unless false. */
  readonly closesOnConfirm?: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ConfirmDialog
      heading={t("libraries.rescan")}
      consequence={props.isLong ? t("libraries.rescanAskLibrary") : t("libraries.rescanAskItem")}
      confirmLabel={t("libraries.rescanYes")}
      keepLabel={t("libraries.rescanNo")}
      option={t("libraries.rescanDiscard")}
      closesOnConfirm={props.closesOnConfirm ?? true}
      onConfirm={props.onRescan}
      onClose={props.onClose}
    />
  );
}

/** A rescan request's query: discard, when the question's box was ticked (D535 §6). */
export function discardQuery(isDiscarding: boolean): { readonly discard?: true } {
  return isDiscarding ? { discard: true } : {};
}

/** The dialog "Rescan" opens: the question, then whether the rescan started. */
function RescanDialog(props: {
  readonly itemId: number;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [isAsked, setAsked] = useState(false);
  const [note, setNote] = useState("");
  const rescanning = useRequest((cause) => identifyProblemText(t, cause));
  const heading = t("libraries.rescan");
  if (!isAsked) {
    return (
      <RescanQuestion
        isLong={false}
        closesOnConfirm={false}
        onClose={props.onClose}
        onRescan={(isDiscarding) => {
          setAsked(true);
          rescanning.runRequest(async (idempotencyKey) => {
            const answer = dataOrThrow(
              await api.POST("/items/{id}/rescan", {
                params: { path: { id: props.itemId }, query: discardQuery(isDiscarding) },
                ...idempotent(idempotencyKey),
              }),
            );
            setNote(
              answer.isStarted
                ? t("libraries.rescanThisStarted")
                : t("libraries.rescanThisRunning"),
            );
          });
        }}
      />
    );
  }
  return (
    <ModalDialog label={heading} onClose={props.onClose}>
      <h2>{heading}</h2>
      <p>{t("libraries.rescanThisHelp")}</p>
      {note === "" ? null : <output>{note}</output>}
      {rescanning.problem === undefined ? null : <output>{rescanning.problem}</output>}
    </ModalDialog>
  );
}
