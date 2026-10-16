// SPDX-License-Identifier: MPL-2.0

import "./still-reading.css";

import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { IsAdminContext, WarningSign } from "./warning-sign.tsx";

/**
 * The still-reading icon: an hourglass on the accent colour, for a title shown from its listing
 * whose file its machine has not read yet (D304 §4). Ringed, it is drawn over a poster; unringed,
 * beside words on a page. Decoration beside words that say it.
 */
export function StillReadingIcon(props: { readonly isRinged: boolean }): JSX.Element {
  const size = props.isRinged ? 30 : 26;
  const centre = size / 2;
  const inset = props.isRinged ? 4.5 : 2.5;
  return (
    <svg
      className="still-reading-icon"
      viewBox={`0 0 ${String(size)} ${String(size)}`}
      aria-hidden="true"
      focusable="false"
    >
      {props.isRinged ? (
        <circle className="still-reading-icon__ring" cx={centre} cy={centre} r={15} />
      ) : null}
      <circle className="still-reading-icon__disc" cx={centre} cy={centre} r={13} />
      <g transform={`translate(${String(inset)} ${String(inset)}) scale(0.875)`}>
        <path
          className="still-reading-icon__glass"
          d="M8 5.5h8M8 18.5h8M9 5.5c0 4 6 4.5 6 6.5s-6 2.5-6 6.5M15 5.5c0 4-6 4.5-6 6.5s6 2.5 6 6.5"
        />
      </g>
    </svg>
  );
}

/**
 * The cue over the poster of a title whose file is still being read, opening what it means as a
 * warning sign does (D278): it is shown from its folder's listing, and plays once read, which asking
 * to play it starts at once.
 */
export function StillReadingSign(): JSX.Element {
  const { t } = useTranslation();
  return (
    <WarningSign
      summary={t("warning.stillReading")}
      forAdmin={t("warning.stillReadingForAdmin")}
      forViewer={t("warning.stillReadingForViewer")}
      icon={<StillReadingIcon isRinged={true} />}
    />
  );
}

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
