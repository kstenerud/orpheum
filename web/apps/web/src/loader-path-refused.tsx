// SPDX-License-Identifier: MPL-2.0

/**
 * What a client origin shows once its WebRTC transport is refused for good (RtcTransport.onRefused):
 * the server no longer accepts what this page proves its connections by, and what else the device
 * holds was refused too, so no try of the page's own will reach it. Left alone, Firefox's page tried
 * every 10 s for good, its Play page spinning, saying nothing (the owner's Mac, staging, 2026-10-29).
 * A reload reaches the server afresh, and says when the device needs to sign in again; signing in
 * again forgets what was refused.
 */

import "./loader-path-refused.css";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

/** The note, with its two ways on. */
export function PathRefusedNote(props: {
  readonly onReload: () => void;
  readonly onSignInAgain: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="loader-path-refused" role="alert">
      <p>{t("loader.pathRefused.text")}</p>
      <button type="button" onClick={props.onReload}>
        {t("loader.pathRefused.reload")}
      </button>
      <button type="button" onClick={props.onSignInAgain}>
        {t("loader.pathRefused.signIn")}
      </button>
    </div>
  );
}
