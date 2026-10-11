// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { unavailableText } from "./problem.ts";
import { WarningSign } from "./warning-sign.tsx";

/**
 * The warning sign of a title or film that cannot be played now (D278, D241 §3): an administrator
 * is told why, its machine away or its folder or disk missing, and where it is fixed; anyone else
 * that it is not available, and to tell them.
 */
export function UnplayableSign(props: {
  readonly unavailable: Parameters<typeof unavailableText>[1];
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <WarningSign
      summary={t("warning.cannotPlay")}
      forAdmin={`${unavailableText(t, props.unavailable)} ${t("warning.goToLibraries")}`}
      forViewer={t("warning.cannotPlayForViewer")}
    />
  );
}
