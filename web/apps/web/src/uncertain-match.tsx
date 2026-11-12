// SPDX-License-Identifier: MPL-2.0

import "./uncertain-match.css";

import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { UncertainMatchIcon } from "./icons.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

/** The same cue on a film's or series' page, with what it means, as its reader needs it. */
export function UncertainMatchNote(): JSX.Element {
  const { t } = useTranslation();
  const isAdmin = use(IsAdminContext);
  return (
    <p className="status-note">
      <UncertainMatchIcon isRinged={false} />
      <span>
        <strong>{t("warning.uncertainMatch")}.</strong>{" "}
        {isAdmin ? t("warning.uncertainMatchForAdmin") : t("warning.uncertainMatchForViewer")}
      </span>
    </p>
  );
}
