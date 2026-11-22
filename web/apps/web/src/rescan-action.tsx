// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { identifyProblemText } from "./candidate-chooser.tsx";
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

/** The dialog "Rescan" opens: what a rescan does, and Rescan, which says whether it started. */
function RescanDialog(props: {
  readonly itemId: number;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [note, setNote] = useState("");
  const rescanning = useRequest((cause) => identifyProblemText(t, cause));
  const heading = t("libraries.rescan");
  return (
    <ModalDialog label={heading} onClose={props.onClose}>
      <h2>{heading}</h2>
      <p>{t("libraries.rescanThisHelp")}</p>
      <div className="more-actions__offers">
        <button
          type="button"
          disabled={rescanning.isBusy}
          onClick={() => {
            setNote("");
            rescanning.runRequest(async (idempotencyKey) => {
              const answer = dataOrThrow(
                await api.POST("/items/{id}/rescan", {
                  params: { path: { id: props.itemId } },
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
        >
          {t("libraries.rescanThis")}
        </button>
        {note === "" ? null : <output>{note}</output>}
        {rescanning.problem === undefined ? null : <output>{rescanning.problem}</output>}
      </div>
    </ModalDialog>
  );
}
