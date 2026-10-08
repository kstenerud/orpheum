// SPDX-License-Identifier: MPL-2.0

import { useEffect, useSyncExternalStore, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { pageUpdateWatch, type UpdateWatch } from "./app-update.ts";
import type { Route } from "./router.ts";

/** The screens a film may be playing on, which an update never interrupts. */
const playingScreens: readonly Route["screen"][] = ["play", "together"];

/**
 * Reloads an out-of-date page at the first safe moment (the owner's choice, 2026-10-08): at once
 * where nothing plays, and as its person leaves the player where a film does, saying meanwhile that
 * it will. A Watch Together guest's page reloads when they leave it.
 */
export function ReloadWhenOutdated(props: {
  readonly route: Route;
  readonly reload?: () => void;
  readonly watch?: UpdateWatch;
}): JSX.Element | null {
  const { t } = useTranslation();
  const watch = props.watch ?? pageUpdateWatch;
  const isOutdated = useSyncExternalStore(watch.subscribe, watch.isOutdated, watch.isOutdated);
  const isPlayingScreen = playingScreens.includes(props.route.screen);
  const reload = props.reload ?? reloadPage;
  useEffect(() => {
    if (isOutdated && !isPlayingScreen) {
      reload();
    }
  }, [isOutdated, isPlayingScreen, reload]);
  return isOutdated && isPlayingScreen ? (
    <output className="reload-when-outdated">{t("play.updateAfterFilm")}</output>
  ) : null;
}

/** Loads the page again, with the server's client. */
function reloadPage(): void {
  window.location.reload();
}
