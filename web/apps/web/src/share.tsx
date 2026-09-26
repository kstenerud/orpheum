// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import type { Invitation } from "./accounts.ts";

/** When an invitation stops working, in the viewer's words. */
export function expiresText(expiresAtMs: number, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(
    new Date(expiresAtMs),
  );
}

/**
 * A registration link or a device code to pass on: shown large enough to read from across a room,
 * selectable to copy, with when it stops working.
 */
export function Share(props: {
  readonly what: "link" | "code";
  readonly text: string;
  readonly invitation: Invitation;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  return (
    <div className="share">
      <output className={`share__${props.what}`}>{props.text}</output>
      <p>
        {t(`share.${props.what}`, {
          expires: expiresText(props.invitation.expiresAt, i18n.language),
        })}
      </p>
    </div>
  );
}
