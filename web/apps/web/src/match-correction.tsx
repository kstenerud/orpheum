// SPDX-License-Identifier: MPL-2.0

import { use, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { CandidateChooser } from "./candidate-chooser.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

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
