// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

/**
 * What a page opened over plain HTTP cannot do, said once above every screen until dismissed: a
 * browser keeps the microphone, the clipboard and installing the app to secure pages, and without a
 * word they only failed when tried (D334 §1). A server reached at home with no discovery is reached
 * so; HTTPS comes with a discovery, the service's or one's own.
 */
export function PlainHttpNote(props: { readonly isSecure?: boolean }): JSX.Element | null {
  const { t } = useTranslation();
  const [isDismissed, setIsDismissed] = useState(false);
  // Only a browser that says the page is not secure: one that says nothing is not doubted.
  const said: unknown = Reflect.get(globalThis, "isSecureContext");
  if ((props.isSecure ?? said !== false) || isDismissed) {
    return null;
  }
  return (
    <output className="app-shell__note">
      {t("discovery.plainHttp.note")}{" "}
      <button
        type="button"
        onClick={() => {
          setIsDismissed(true);
        }}
      >
        {t("discovery.plainHttp.dismiss")}
      </button>
    </output>
  );
}
