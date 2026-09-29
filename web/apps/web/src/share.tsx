// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import type { IssuedSecret } from "./accounts.ts";
import { dateTimeText } from "./date-time-text.ts";

/**
 * A registration link or a device code to pass on: shown large enough to read from across a room,
 * selectable to copy, with when it stops working.
 */
export function Share(props: {
  readonly kind: "link" | "code";
  readonly text: string;
  readonly invitation: IssuedSecret;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  return (
    <div className="share">
      <output className={`share__${props.kind}`}>{props.text}</output>
      <p>
        {t(`share.${props.kind}`, {
          expires: dateTimeText(props.invitation.expiresAt, i18n.language),
        })}
      </p>
    </div>
  );
}
