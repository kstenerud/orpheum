// SPDX-License-Identifier: MPL-2.0

import "./uncertain-match.css";

import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Icon } from "./icon.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

/**
 * The uncertain match's icon, the owner's (2026-10-15): a question mark on the caution yellow.
 * Ringed, it is drawn over a poster, the dark ring keeping it apart from any art; unringed, beside
 * words on a page. Decoration beside words that say it.
 */
export function UncertainMatchIcon(props: { readonly isRinged: boolean }): JSX.Element {
  const size = props.isRinged ? 30 : 26;
  const centre = size / 2;
  const inset = props.isRinged ? 4.5 : 2.5;
  return (
    <Icon className="uncertain-match-icon" viewBox={`0 0 ${String(size)} ${String(size)}`}>
      {props.isRinged ? (
        <circle className="uncertain-match-icon__ring" cx={centre} cy={centre} r={15} />
      ) : null}
      <circle className="uncertain-match-icon__disc" cx={centre} cy={centre} r={13} />
      <g transform={`translate(${String(inset)} ${String(inset)}) scale(0.875)`}>
        <path
          className="uncertain-match-icon__curve"
          d="M8.6 8.6a3.4 3.4 0 1 1 4.9 3.05c-.95.48-1.5 1.15-1.5 2.1v.5"
        />
        <circle className="uncertain-match-icon__dot" cx="12" cy="18.8" r="1.7" />
      </g>
    </Icon>
  );
}

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
