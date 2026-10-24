// SPDX-License-Identifier: MPL-2.0

import "./back-button.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import type { Route } from "./router.ts";
import { useServices } from "./services.tsx";

/**
 * A drill-down page's Back, at its top: the page before when it was this app's, else up to the
 * page above, up. A browser on a TV keeps its own back out of reach of the remote (the owner,
 * 2026-10-24: TV Bro), so a page one drilled into needs its own way out.
 */
export function BackButton(props: { readonly up: Route }): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  return (
    <button
      type="button"
      className="back-button"
      onClick={() => {
        navigation.back(props.up);
      }}
    >
      {t("common.back")}
    </button>
  );
}
