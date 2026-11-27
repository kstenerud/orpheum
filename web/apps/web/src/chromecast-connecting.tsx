// SPDX-License-Identifier: MPL-2.0

import "./cast.css";
import "./chromecast-connecting.css";
import "./cast-words.ts";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { useChromecastConnecting } from "./google-cast-connecting.ts";
import { ActionPill, Pill } from "./pill.tsx";

/**
 * Over the film's own picture while a Google Cast device picked in Chrome's picker is being
 * connected (D570 §2): "Connecting to ‹TV›…" by the name Chrome gives it, as the cast's page says
 * it from then on (D569 §1, castee-start.tsx), with Cancel under it, which gives the cast up (§3),
 * the film carrying on here as it stands. Drawn in and out of full screen; the film's transport
 * waits meanwhile (player-controls.tsx's useVideoTarget, player-commands.ts's stageOfVideo).
 */
export function ChromecastConnectingOverTheFilm(): JSX.Element | null {
  const { t } = useTranslation();
  const connecting = useChromecastConnecting();
  if (connecting === undefined) {
    return null;
  }
  return (
    <span className="chromecast-connecting">
      <Pill icon={<span className="cast-remote__spinner" />}>
        {t("cast.connecting", { name: connecting.name })}
      </Pill>
      <ActionPill isDangerous onPress={connecting.cancel}>
        {t("cast.cancel")}
      </ActionPill>
    </span>
  );
}
