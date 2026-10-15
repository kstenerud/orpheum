// SPDX-License-Identifier: MPL-2.0

import { use, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { CandidateChooser } from "./candidate-chooser.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

/**
 * The mark over the poster of a film or series whose match is a guess no person confirmed: a low
 * confidence, said in a corner of its poster (D301 §4; the owner, 2026-10-15). Its parent is
 * positioned.
 */
export function GuessedMark(props: {
  readonly isGuessed: boolean | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  return props.isGuessed === true ? (
    <span className="guessed-mark" title={t("title.guessed")}>
      <span aria-hidden="true">?</span>
      <span className="guessed-mark__words">{t("title.guessed")}</span>
    </span>
  ) : null;
}

/**
 * An admin's "Identified incorrectly" on a film's or series' page: the works its search found,
 * one of which they choose as its match, final as any person's choice (D301 §2). Nothing for
 * anyone else.
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
      </button>
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
