// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { useServices } from "./services.tsx";

/**
 * Why the browser asks to reach devices on the person's network, while it waits on their answer
 * (webrtc-transport.md W2c item 8): the page asked for the core's LAN name, and a prompt with no word
 * from the page reads as something to refuse. Gone once they answer, or when they dismiss it.
 */
export function LocalNetworkNote(): JSX.Element | null {
  const { t } = useTranslation();
  const { onLocalNetworkAsking } = useServices();
  const [isAsking, setIsAsking] = useState(false);
  const [isDismissed, setIsDismissed] = useState(false);
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
          setIsDismissed(true);
        }}
      >
        {t("discovery.localNetwork.dismiss")}
      </button>
    </output>
  );
}
