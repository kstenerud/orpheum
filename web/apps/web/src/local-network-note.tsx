// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage } from "./browser-storage.ts";
import { useServices } from "./services.tsx";

/** The storage key that says this browser's person dismissed the note (client-keys.ts). */
export const localNetworkNoteDismissedKey = "orpheum.localNetwork.noteDismissed";

/**
 * Why the browser asks to reach devices on the person's network, while it waits on their answer
 * (webrtc-transport.md W2c item 8): the page asked for the core's LAN name, and a prompt with no word
 * from the page reads as something to refuse. Gone once they answer, or when they dismiss it, which
 * this browser remembers: Silk on a Fire TV leaves the permission waiting for good, showing no
 * prompt, so the note came back at every reload until dismissed again (the owner, 2026-10-29). The
 * browser's storage refused keeps the dismissal for the page's life alone.
 */
export function LocalNetworkNote(): JSX.Element | null {
  const { t } = useTranslation();
  const { onLocalNetworkAsking } = useServices();
  const [isAsking, setIsAsking] = useState(false);
  const [isDismissed, setIsDismissed] = useState(
    () => browserStorage().getItem(localNetworkNoteDismissedKey) === "1",
  );
  useEffect(() => onLocalNetworkAsking?.(setIsAsking), [onLocalNetworkAsking]);
  if (!isAsking || isDismissed) {
    return null;
  }
  return (
    <output className="app-shell__note">
      {t("discovery.localNetwork.asking")}{" "}
      <button
        type="button"
        onClick={() => {
          browserStorage().setItem(localNetworkNoteDismissedKey, "1");
          setIsDismissed(true);
        }}
      >
        {t("discovery.localNetwork.dismiss")}
      </button>
    </output>
  );
}
