// SPDX-License-Identifier: MPL-2.0

import { useState } from "react";
import { useTranslation } from "react-i18next";

import type { BannerNotice } from "./banner.tsx";
import { browserStorage } from "./browser-storage.ts";

/** The storage key that says this browser's person dismissed the note (client-keys.ts). */
export const plainHttpNoteDismissedKey = "orpheum.plainHttp.noteDismissed";

/**
 * What a page opened over plain HTTP cannot do, said once above every screen until dismissed: a
 * browser keeps the microphone, the clipboard and installing the app to secure pages, and without a
 * word they only failed when tried (D334 §1). A server reached at home with no discovery is reached
 * so; HTTPS comes with a discovery, the service's or one's own. This browser remembers the
 * dismissal: a household reached at home alone saw the note again at every load (the owner's install,
 * 2026-11-20); a browser whose storage is refused keeps it for the page's life alone. An information
 * Banner, dismissed by its ✕ (D538 §1).
 */
export function usePlainHttpNotice(isSecure?: boolean): BannerNotice | undefined {
  const { t } = useTranslation();
  const [isDismissed, setIsDismissed] = useState(
    () => browserStorage().getItem(plainHttpNoteDismissedKey) === "1",
  );
  // Only a browser that says the page is not secure: one that says nothing is not doubted.
  const said: unknown = Reflect.get(globalThis, "isSecureContext");
  if ((isSecure ?? said !== false) || isDismissed) {
    return undefined;
  }
  return {
    key: "plain http",
    tone: "information",
    text: t("discovery.plainHttp.note"),
    putAway: {
      kind: "dismiss",
      onDismiss: () => {
        browserStorage().setItem(plainHttpNoteDismissedKey, "1");
        setIsDismissed(true);
      },
    },
  };
}
