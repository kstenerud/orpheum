// SPDX-License-Identifier: MPL-2.0

import "./back-button.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { useBackTarget } from "./input-layer.tsx";
import type { Route } from "./router.ts";
import { useServices } from "./services.tsx";

/**
 * An on-screen Back: the page before when it was this app's, else up to the page above, up. Only a
 * television's player overlay shows one (D367 §6): TV Bro keeps its remote's Back from the page, and
 * the app cannot tell it from Silk, whose remote's Back is the history's; a computer's and a phone's
 * Back is the browser's, the gesture's or the remote's own. A remote's, a keyboard's or a
 * controller's Back goes where it does while it shows (input-layer.tsx).
 */
export function BackButton(props: { readonly up: Route }): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  useBackTarget(props.up);
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
