// SPDX-License-Identifier: MPL-2.0

import "./still-reading.css";

import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { StillReadingIcon } from "./icons.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

/** The same cue on a title's page, with what it means, as its reader needs it. */
export function StillReadingNote(): JSX.Element {
  const { t } = useTranslation();
  const isAdmin = use(IsAdminContext);
  return (
    <p className="status-note">
      <StillReadingIcon isRinged={false} />
      <span>
        <strong>{t("warning.stillReading")}.</strong>{" "}
        {isAdmin ? t("warning.stillReadingForAdmin") : t("warning.stillReadingForViewer")}
      </span>
    </p>
  );
}
