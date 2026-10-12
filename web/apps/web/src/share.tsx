// SPDX-License-Identifier: MPL-2.0

import "./share.css";

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { dateTimeText } from "./date-time-text.ts";

/**
 * A registration invitation or a Watch Together link to pass on: selectable to copy, with when it
 * stops working; a Watch Together link, which has no expiry, works until it is used or the session
 * ends (D246 §3).
 */
export function Share(props: {
  readonly text: string;
  /** What was issued, with its expiry; undefined for a Watch Together link. */
  readonly invitation: components["schemas"]["IssuedSecretBody"] | undefined;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  return (
    <div className="share">
      <output className="share__link">{props.text}</output>
      <p>
        {props.invitation === undefined
          ? t("share.togetherLink")
          : t("share.link", {
              expires: dateTimeText(props.invitation.expiresAt, i18n.language),
            })}
      </p>
    </div>
  );
}
