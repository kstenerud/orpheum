// SPDX-License-Identifier: MPL-2.0

import { use, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { CandidateChooser, identifyProblemText } from "./candidate-chooser.tsx";
import { idempotent } from "./idempotent.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { IsAdminContext } from "./warning-sign.tsx";

/**
 * An admin's "Identified incorrectly" on a film's or series' page: the works its search found,
 * one of which they choose as its match, final as any person's choice (D301 §2); and "Rescan
 * this", which scans its files and matches it again from them as the matcher now matches (D357).
 * Nothing for anyone else.
 */
export function MatchCorrection(props: {
  readonly itemId: number;
  readonly onCorrected: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const isAdmin = use(IsAdminContext);
  const [isChoosing, setIsChoosing] = useState(false);
  if (!isAdmin) {
    return null;
  }
  return (
    <div className="match-correction">
      <button
        type="button"
        aria-expanded={isChoosing}
        onClick={() => {
          setIsChoosing(!isChoosing);
        }}
      >
        {t("title.identifiedIncorrectly")}
      </button>{" "}
      <RescanThis itemId={props.itemId} />
      {isChoosing ? (
        <CandidateChooser
          itemId={props.itemId}
          onChosen={() => {
            setIsChoosing(false);
            props.onCorrected();
          }}
        />
      ) : null}
    </div>
  );
}

/**
 * "Rescan this": the item's files scanned, then the item matched again from them, in the
 * background (D357 §1, §5); it says whether it started, or one already runs.
 */
function RescanThis(props: { readonly itemId: number }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [note, setNote] = useState("");
  const rescanning = useRequest((cause) => identifyProblemText(t, cause));
  return (
    <>
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
    </>
  );
}
