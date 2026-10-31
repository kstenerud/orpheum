// SPDX-License-Identifier: MPL-2.0

import "./share.css";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { copyToClipboard } from "./clipboard.ts";
import { dateTimeText } from "./date-time-text.ts";

/**
 * A registration invitation or a Watch Together link to pass on: selectable to copy, with Copy, and
 * when it stops working; a Watch Together link, which has no expiry, works until it is used or the
 * session ends (D246 §3). A copy the browser refuses, as one that may not use the clipboard does,
 * says so, the link still there to select.
 */
export function Share(props: {
  readonly text: string;
  /** What was issued, with its expiry; undefined for a Watch Together link. */
  readonly invitation: components["schemas"]["IssuedSecretBody"] | undefined;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const [copied, setCopied] = useState<"copied" | "copyRefused" | undefined>(undefined);
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
      <p>
        <button
          type="button"
          onClick={() => {
            copyToClipboard(props.text).then(
              () => {
                setCopied("copied");
              },
              () => {
                setCopied("copyRefused");
              },
            );
          }}
        >
          {t("share.copy")}
        </button>{" "}
        {copied === undefined ? null : <output>{t(`share.${copied}`)}</output>}
      </p>
    </div>
  );
}
